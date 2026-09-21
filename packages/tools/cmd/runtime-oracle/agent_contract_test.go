package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This file is the oracle side of the two externally owned Agent contract
// families. It does not execute the Agent wire contract itself: the schema
// validator that decides those cases is package-local test code in
// packages/shared/companion, and importing a test-only validator from here would
// either duplicate it or turn the corpus into a tautology. Instead this file
// verifies the evidence that producer published: it binds every committed corpus
// case to a manifest entry with reviewed digests, proves the frozen corpus covers
// exactly the committed golden fixtures, pins the case identities to the external
// consumer the change names, and validates an exported report through the
// production trace publisher and validator.

const (
	// agentHTTPFamily and agentMCPFamily are the two externally owned contract
	// families. Their eventual owner is the companion Agent service, so no Rust
	// consumer is registered for them.
	agentHTTPFamily = "agent.http"
	agentMCPFamily  = "agent.mcp"
	// agentVersion is the application contract version both families publish.
	agentVersion = "v1"
	// agentOperation is the manifest operation name for a contract-fixture
	// validation case.
	agentOperation = "agent-contract"
	// agentConsumer is the manifest consumer value the change pins for these two
	// families. It is deliberately not a Rust consumer name: the executed
	// evidence is Go, and claiming a Rust Agent runtime would assert parity that
	// does not exist.
	agentConsumer = "external:agent-contract"
	// agentCorpusRelDir is the repository-relative directory holding the frozen
	// Agent contract corpus cases.
	agentCorpusRelDir = "testdata/runtime-migration/cases/agent"
	// agentProducerTestRelPath and agentProducerTestName locate the package-local
	// producer that executes the contract validator. A corpus whose producer test
	// is missing has no independently executed evidence at all.
	agentProducerTestRelPath = "packages/shared/companion/runtime_contract_oracle_test.go"
	agentProducerTestName    = "TestRuntimeAgentContractOracle"
	// agentCorpusReportName is the published report file name for the executed
	// Agent contract evidence.
	agentCorpusReportName = "runtime-corpus-agent.json"
)

// agentContractSource is one golden fixture the frozen corpus must cover.
type agentContractSource struct {
	dir      string
	relative string
	family   string
}

// agentContractSources is the ordered golden fixture set. The order is the source
// index the corpus labels carry, so the coverage gate can name a case by fixture
// name and source index without re-deriving the producer's slug.
var agentContractSources = []agentContractSource{
	{dir: "http-valid", relative: "packages/contracts/companion-agent/http-v1/golden/valid.json", family: agentHTTPFamily},
	{dir: "http-invalid", relative: "packages/contracts/companion-agent/http-v1/golden/invalid.json", family: agentHTTPFamily},
	{dir: "mcp-valid", relative: "packages/contracts/companion-agent/mcp-v1/golden/valid.json", family: agentMCPFamily},
	{dir: "mcp-invalid", relative: "packages/contracts/companion-agent/mcp-v1/golden/invalid.json", family: agentMCPFamily},
	{dir: "mcp-mine-validation", relative: "packages/contracts/companion-agent/mcp-v1/golden/mine-validation.json", family: agentMCPFamily},
}

// agentFamilySources is the merged provenance set each family records. The two
// schema documents and the mine-validation golden are the entries this node adds;
// the manifests and the two primary goldens were already frozen.
var agentFamilySources = map[string][]string{
	agentHTTPFamily: {
		"packages/contracts/companion-agent/http-v1/manifest.json",
		"packages/contracts/companion-agent/http-v1/schema.json",
		"packages/contracts/companion-agent/http-v1/golden/valid.json",
		"packages/contracts/companion-agent/http-v1/golden/invalid.json",
		"packages/contracts/companion-agent/mcp-v1/schema.json",
	},
	agentMCPFamily: {
		"packages/contracts/companion-agent/mcp-v1/manifest.json",
		"packages/contracts/companion-agent/mcp-v1/schema.json",
		"packages/contracts/companion-agent/mcp-v1/golden/valid.json",
		"packages/contracts/companion-agent/mcp-v1/golden/invalid.json",
		"packages/contracts/companion-agent/mcp-v1/golden/mine-validation.json",
	},
}

// agentLabelPattern is the shape a corpus label must have: a lowercase slug plus
// the source index it was derived from.
var agentLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-[0-9]+$`)

// agentCaseEnvelope is the part of a frozen corpus input this file reads. It is
// the case's own provenance and consumer identity, not its expectation.
type agentCaseEnvelope struct {
	Source      string `json:"source"`
	SourceIndex int    `json:"source_index"`
	Fixture     string `json:"fixture"`
	CaseKind    string `json:"case_kind"`
	Consumer    string `json:"consumer"`
}

// TestAgentContractOracleManifestReconcilesExecutedAgentCases binds every frozen
// Agent contract case to a manifest entry and proves the resulting manifest still
// reconciles against the live registries, so a corpus case cannot exist without a
// registered family, version, operation and consumer.
func TestAgentContractOracleManifestReconcilesExecutedAgentCases(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := agentWorkingManifest(t, root)
	if len(manifest.Cases) == 0 {
		t.Fatal("frozen agent corpus contains no case")
	}

	_, families, live := discoverLive(t)
	if _, err := ReconcileWorking(root, manifest, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("agent working manifest drifted from current registries: %v", err)
	}
	agentAssertGoldenCoverage(t, root, manifest)
}

// TestAgentContractOracleCaseIdentitiesAreExternalAgentContract pins the manifest
// identity of every Agent contract case: the operation, the version, the family
// and the external consumer the change names for these two externally owned
// families.
func TestAgentContractOracleCaseIdentitiesAreExternalAgentContract(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := agentWorkingManifest(t, root)

	labels := make(map[string]string, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != agentOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, agentOperation)
		}
		if c.Version != agentVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, agentVersion)
		}
		if c.RustConsumer != agentConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, agentConsumer)
		}
		// The frozen corpus input records the same consumer the producer pinned,
		// so a producer and a manifest that disagree about the consumer fail here
		// instead of both drifting to a value nobody reviewed.
		envelope := agentReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != agentConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, agentConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		if c.Family != agentHTTPFamily && c.Family != agentMCPFamily {
			t.Fatalf("case %s belongs to family %q, want one of the two agent families", c.ID, c.Family)
		}
		prefix := c.Family + "/" + agentVersion + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !agentLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug plus source index", c.ID, label)
		}
		if previous, duplicate := labels[label]; duplicate {
			t.Fatalf("label %q is used by both %s and %s", label, previous, c.ID)
		}
		labels[label] = c.ID
	}
}

// TestAgentContractOracleRunnerEnumeratesEveryAgentCase runs the frozen selection
// through the production runner with a recording producer, so the runner contract
// holds for the Agent families too: one invocation per declared checkpoint, with
// the case specification and the exact input bytes, and an observation built from
// what the producer returned.
func TestAgentContractOracleRunnerEnumeratesEveryAgentCase(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := agentWorkingManifest(t, root)
	spy := &recordingOperation{outcome: Outcome{Kind: "ok", Category: agentOperation}}

	observations, err := RunCases(root, manifest,
		map[string]GoOperation{agentOperation: spy.run},
		map[string]string{agentHTTPFamily: agentOperation, agentMCPFamily: agentOperation})
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(manifest.Cases) != spy.calls {
		t.Fatalf("producer invoked %d times, want %d (one per case)", spy.calls, len(manifest.Cases))
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for index, c := range manifest.Cases {
		if spy.seen[index].ID != c.ID {
			t.Fatalf("producer received case %s at position %d, want %s", spy.seen[index].ID, index, c.ID)
		}
		want, readErr := readCaseInput(root, c)
		if readErr != nil {
			t.Fatalf("read case input: %v", readErr)
		}
		if string(spy.inputs[index]) != string(want) {
			t.Fatalf("producer received tampered input for case %s", c.ID)
		}
	}
	for _, obs := range observations {
		if !outcomesEqual(obs.Outcome, spy.outcome) {
			t.Fatalf("observation for %s carries %#v, want the producer's own %#v", obs.CaseID, obs.Outcome, spy.outcome)
		}
		if obs.ExpectedDigest == "" {
			t.Fatalf("observation for %s carries no expected digest", obs.CaseID)
		}
	}
}

// TestAgentContractOracleReportPublishesAndValidates assembles the executed Agent
// contract evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is the
// identity and content check a later Rust acceptance step performs; it does not
// claim any Rust Agent behaviour.
func TestAgentContractOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := agentWorkingManifest(t, root)
	observations := agentObservationsFromCorpus(t, root, manifest)

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("executed agent evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, agentCorpusReportName)
	if err := ExportTrace(root, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTrace(target, manifest)
	if err != nil {
		t.Fatalf("published report does not validate: %v", err)
	}
	if loaded.SchemaVersion != traceSchemaVersion {
		t.Fatalf("report schema_version = %d, want %d", loaded.SchemaVersion, traceSchemaVersion)
	}
	if loaded.SourceRevision != manifest.SourceRevision {
		t.Fatalf("report source revision = %s, want %s", loaded.SourceRevision, manifest.SourceRevision)
	}
	if loaded.CorpusDigest != trace.CorpusDigest {
		t.Fatalf("report corpus digest = %s, want %s", loaded.CorpusDigest, trace.CorpusDigest)
	}
	if len(loaded.Inputs) != len(manifest.Cases) || len(loaded.Observations) != len(manifest.Cases) {
		t.Fatalf("report carries %d inputs and %d observations, want %d each",
			len(loaded.Inputs), len(loaded.Observations), len(manifest.Cases))
	}
	for index, obs := range loaded.Observations {
		if !outcomesEqual(obs.Outcome, observations[index].Outcome) {
			t.Fatalf("report observation for %s does not match the frozen corpus outcome", obs.CaseID)
		}
	}
	agentAssertOutcomeKinds(t, loaded.Observations)
}

// TestAgentContractOracleReportRejectsTamperedOutcome pins that a changed result
// fails the report even though every case file and digest still exists.
func TestAgentContractOracleReportRejectsTamperedOutcome(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := agentWorkingManifest(t, root)
	observations := agentObservationsFromCorpus(t, root, manifest)

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("untampered evidence failed trace validation: %v", err)
	}

	trace.Observations[0].Outcome.Kind = "ok"
	if trace.Observations[0].Outcome.Kind == "error" {
		trace.Observations[0].Outcome.Category = "tampered"
	}
	err = ValidateTrace(trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "does not match normalized expected outcome") {
		t.Fatalf("expected a tampered-outcome rejection, got: %v", err)
	}
}

// TestAgentContractOracleRejectsTamperedInputDigest pins that a producer never
// executes bytes the manifest does not name, for the Agent families as well.
func TestAgentContractOracleRejectsTamperedInputDigest(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := agentWorkingManifest(t, root)
	for index := range manifest.Cases {
		manifest.Cases[index].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest,
		map[string]GoOperation{agentOperation: (&recordingOperation{}).run},
		map[string]string{agentHTTPFamily: agentOperation, agentMCPFamily: agentOperation})
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestAgentContractOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it. A corpus whose package-local producer
// test is gone has no independent execution, only frozen files.
func TestAgentContractOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(agentProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", agentProducerTestRelPath, err)
	}
	declaration := "func " + agentProducerTestName + "(t *testing.T) {"
	if !strings.Contains(string(data), declaration) {
		t.Fatalf("%s does not declare %s", agentProducerTestRelPath, declaration)
	}
}

// TestAgentContractOracleDoesNotClaimAgentExecution pins the ownership boundary:
// the schema validator that decides these cases is package-local test code in
// packages/shared/companion, so this package must not register a producer that
// would let a manifest claim the oracle executed the Agent contract itself.
func TestAgentContractOracleDoesNotClaimAgentExecution(t *testing.T) {
	if _, registered := goOperations[agentOperation]; registered {
		t.Fatalf("runtime-oracle registered a %q producer; the Agent contract validator is package-local to packages/shared/companion", agentOperation)
	}
	for family := range goFamilyOperations {
		if family == agentHTTPFamily || family == agentMCPFamily {
			t.Fatalf("runtime-oracle registered agent family %q for execution; the producer is package-local to packages/shared/companion", family)
		}
	}
}

// agentWorkingManifest assembles the manifest this node executes inside a
// harness-owned temporary directory.
//
// The frozen manifest does not yet carry the Agent contract cases, because the
// controller merges manifest fragments after acceptance. The working manifest is
// therefore the frozen one with the frozen corpus's Agent cases registered and
// the two agent families given their merged provenance set, which also proves the
// proposed provenance reconciles against disk.
func agentWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	agentCases := agentCorpusCases(t, root)

	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          agentCases,
	}
	for index := range cloned.Families {
		family := cloned.Families[index].ID
		if family != agentHTTPFamily && family != agentMCPFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Cases = agentFamilyCaseIDs(agentCases, family)
		sources := make([]SourceSpec, 0, len(agentFamilySources[family]))
		for _, relative := range agentFamilySources[family] {
			hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatalf("hash provenance source %s: %v", relative, err)
			}
			sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
		}
		cloned.Families[index].Sources = sources
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load working manifest: %v", err)
	}
	return loaded
}

// agentCorpusCases reads the frozen Agent contract corpus and registers one case
// per committed input, with digests proven against the files on disk.
func agentCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(agentCorpusRelDir))
	var inputs []string
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".input.json") {
			inputs = append(inputs, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk agent corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no agent contract case under %s", agentCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 2 {
			t.Fatalf("corpus case %s is not directly inside one source directory", filepath.ToSlash(relative))
		}
		source, known := agentContractSourceByDir(parts[0])
		if !known {
			t.Fatalf("corpus case %s lives in unregistered source directory %q", filepath.ToSlash(relative), parts[0])
		}
		label := strings.TrimSuffix(parts[1], ".input.json")
		if label == "" || !agentLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug plus source index", filepath.ToSlash(relative), label)
		}
		envelope := agentReadEnvelope(t, input)
		if envelope.Source != source.relative {
			t.Fatalf("corpus case %s names source %q, want %q", label, envelope.Source, source.relative)
		}
		if envelope.SourceIndex != agentContractSourceIndex(source) {
			t.Fatalf("corpus case %s names source index %d, want %d", label, envelope.SourceIndex, agentContractSourceIndex(source))
		}
		if strings.TrimSpace(envelope.Fixture) == "" {
			t.Fatalf("corpus case %s names no fixture", label)
		}

		inputHash, err := hashFile(input)
		if err != nil {
			t.Fatalf("hash %s: %v", input, err)
		}
		expectedPath := strings.TrimSuffix(input, ".input.json") + ".expected.json"
		expectedHash, err := hashFile(expectedPath)
		if err != nil {
			t.Fatalf("hash %s: %v", expectedPath, err)
		}
		cases = append(cases, CaseSpec{
			ID:           source.family + "/" + agentVersion + "/" + label,
			Family:       source.family,
			Version:      agentVersion,
			Operation:    agentOperation,
			Input:        AssetRef{Path: agentCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: agentCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: agentConsumer,
		})
	}
	return cases
}

// agentAssertGoldenCoverage proves the frozen corpus covers exactly the committed
// golden fixtures: every fixture case of every source is present once, and no
// corpus case names a fixture that does not exist.
func agentAssertGoldenCoverage(t *testing.T, root string, manifest Inventory) {
	t.Helper()

	wantFixtures := make(map[string]bool)
	wantBySource := make(map[string]int)
	for _, source := range agentContractSources {
		fixtures := agentGoldenFixtureNames(t, root, source.relative)
		wantBySource[source.relative] = len(fixtures)
		for _, fixture := range fixtures {
			key := source.relative + "\x00" + fixture
			if wantFixtures[key] {
				t.Fatalf("golden %s declares fixture %q twice", source.relative, fixture)
			}
			wantFixtures[key] = true
		}
	}

	seen := make(map[string]int, len(manifest.Cases))
	seenBySource := make(map[string]int, len(agentContractSources))
	for _, c := range manifest.Cases {
		envelope := agentReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		key := envelope.Source + "\x00" + envelope.Fixture
		if !wantFixtures[key] {
			t.Fatalf("corpus case %s names fixture %q which is not in %s", c.ID, envelope.Fixture, envelope.Source)
		}
		seen[key]++
		seenBySource[envelope.Source]++
	}
	for key := range wantFixtures {
		if seen[key] != 1 {
			t.Fatalf("fixture %q is covered %d times, want exactly 1", strings.ReplaceAll(key, "\x00", " / "), seen[key])
		}
	}
	for source, want := range wantBySource {
		if seenBySource[source] != want {
			t.Fatalf("source %s is covered by %d cases, want %d", source, seenBySource[source], want)
		}
	}
}

// agentGoldenFixtureNames reads the case names of one golden fixture.
func agentGoldenFixtureNames(t *testing.T, root, relative string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read golden %s: %v", relative, err)
	}
	var document struct {
		Cases []struct {
			Name string `json:"name"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode golden %s: %v", relative, err)
	}
	if len(document.Cases) == 0 {
		t.Fatalf("golden %s has no case", relative)
	}
	names := make([]string, 0, len(document.Cases))
	for _, testCase := range document.Cases {
		names = append(names, testCase.Name)
	}
	return names
}

// agentObservationsFromCorpus rebuilds one observation per frozen case from the
// outcome the package-local producer executed, which is the evidence a report
// carries.
func agentObservationsFromCorpus(t *testing.T, root string, manifest Inventory) []Observation {
	t.Helper()
	observations := make([]Observation, 0, len(manifest.Cases))
	for _, c := range manifest.Cases {
		observations = append(observations, Observation{
			Tick:           0,
			CaseID:         c.ID,
			Outcome:        readExpectedOutcome(t, root, c),
			ExpectedDigest: c.Expected.SHA256,
		})
	}
	return observations
}

// agentAssertOutcomeKinds proves the executed evidence distinguishes accepted
// values from rejections instead of publishing one constant answer.
func agentAssertOutcomeKinds(t *testing.T, observations []Observation) {
	t.Helper()
	accepted, rejected := 0, 0
	for _, obs := range observations {
		switch obs.Outcome.Kind {
		case "ok":
			accepted++
		case "error":
			rejected++
		default:
			t.Fatalf("observation for %s publishes kind %q, want ok or error", obs.CaseID, obs.Outcome.Kind)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// agentReadEnvelope reads the provenance envelope of one frozen corpus input.
func agentReadEnvelope(t *testing.T, path string) agentCaseEnvelope {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope agentCaseEnvelope
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// agentFamilyCaseIDs lists the case identities of one family in corpus order.
func agentFamilyCaseIDs(cases []CaseSpec, family string) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		if c.Family == family {
			ids = append(ids, c.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// agentContractSourceByDir resolves one corpus source directory.
func agentContractSourceByDir(dir string) (agentContractSource, bool) {
	for _, source := range agentContractSources {
		if source.dir == dir {
			return source, true
		}
	}
	return agentContractSource{}, false
}

// agentContractSourceIndex returns the ordered index of one source.
func agentContractSourceIndex(source agentContractSource) int {
	for index, candidate := range agentContractSources {
		if candidate.relative == source.relative {
			return index
		}
	}
	return -1
}

// agentCorpusPath renders one absolute corpus path as the repository-relative
// slash path the manifest requires.
func agentCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}
