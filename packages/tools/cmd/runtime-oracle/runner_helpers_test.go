package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	// runtimeOracleExportDirEnv names the harness-owned directory explicit
	// fixture export writes into. There is no repository default: an unset
	// variable means nothing is exported, so executed evidence never lands in
	// the tree by accident.
	runtimeOracleExportDirEnv = "RUNTIME_ORACLE_EXPORT_DIR"
)

// generatedAsset pairs a relative slash path with raw bytes.
type generatedAsset struct {
	RelativePath string
	Data         []byte
}

var validProducerIDs = map[string]bool{
	"runtime-oracle/protocol-frame":           true,
	"runtime-oracle/domain-identity-values":   true,
	"runtime-oracle/domain-values":            true,
	"runtime-oracle/domain-command-control":   true,
	"runtime-oracle/domain-command-inventory": true,
	"runtime-oracle/domain-event-player":      true,
	"runtime-oracle/domain-event-world":       true,
	"runtime-oracle/domain-event-inventory":   true,
	"runtime-oracle/domain-event-people":      true,
	"companion/agent-contract":                true,
}

func exportGeneratedAssets(
	repoRoot string,
	exportRoot string,
	producerID string,
	assets []generatedAsset,
) (string, error) {
	if !validProducerIDs[producerID] {
		return "", fmt.Errorf("runtime-oracle: unrecognized producer ID: %q", producerID)
	}
	if strings.TrimSpace(exportRoot) == "" {
		return "", fmt.Errorf("runtime-oracle: export root cannot be empty")
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: resolve repository root %s: %w", repoRoot, err)
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}

	absExport, err := filepath.Abs(exportRoot)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: resolve export root %s: %w", exportRoot, err)
	}

	// Walk upward from absExport until an existing ancestor is found.
	var missing []string
	existing := absExport
	for {
		_, statErr := os.Lstat(existing)
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("runtime-oracle: stat export root %s: %w", existing, statErr)
		}
		missing = append(missing, filepath.Base(existing))
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("runtime-oracle: export root %s has no existing ancestor", exportRoot)
		}
		existing = parent
	}
	for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
		missing[i], missing[j] = missing[j], missing[i]
	}

	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: resolve export ancestor %s: %w", existing, err)
	}
	resolvedExport := filepath.Join(append([]string{resolvedExisting}, missing...)...)

	// Check repository containment on both resolved export path and existing ancestor.
	live, err := isLivePath(absRoot, resolvedExport)
	if err != nil {
		return "", err
	}
	if live {
		return "", fmt.Errorf("runtime-oracle: live-path write rejected: export root %s is inside repository", exportRoot)
	}
	liveExisting, err := isLivePath(absRoot, existing)
	if err != nil {
		return "", err
	}
	if liveExisting {
		return "", fmt.Errorf("runtime-oracle: live-path write rejected: export ancestor %s is inside repository", existing)
	}

	// Reject symlink components below the nearest existing ancestor.
	for component := existing; ; component = filepath.Dir(component) {
		if component == "/" || component == "." || component == filepath.Dir(component) {
			break
		}
		if component == "/var" || component == "/tmp" || component == "/etc" {
			break
		}
		info, statErr := os.Lstat(component)
		if statErr != nil {
			return "", fmt.Errorf("runtime-oracle: stat export path %s: %w", component, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("runtime-oracle: symlink component rejected: %s", component)
		}
	}

	// Check and create the fixed producer child.
	producerChild := filepath.Join(absExport, filepath.FromSlash(producerID))
	if _, statErr := os.Lstat(producerChild); statErr == nil {
		return "", fmt.Errorf("runtime-oracle: producer child already exists: %s", producerChild)
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("runtime-oracle: stat producer child %s: %w", producerChild, statErr)
	}

	if err := os.MkdirAll(producerChild, 0o755); err != nil {
		return "", fmt.Errorf("runtime-oracle: create producer directory %s: %w", producerChild, err)
	}

	for _, asset := range assets {
		if strings.TrimSpace(asset.RelativePath) == "" {
			return "", fmt.Errorf("runtime-oracle: empty asset relative path")
		}
		if strings.Contains(asset.RelativePath, "\\") {
			return "", fmt.Errorf("runtime-oracle: backslash rejected in relative path: %s", asset.RelativePath)
		}
		if filepath.IsAbs(asset.RelativePath) || strings.HasPrefix(asset.RelativePath, "/") {
			return "", fmt.Errorf("runtime-oracle: absolute path rejected: %s", asset.RelativePath)
		}
		for _, part := range strings.Split(asset.RelativePath, "/") {
			if part == "." || part == ".." {
				return "", fmt.Errorf("runtime-oracle: relative path contains ./..: %s", asset.RelativePath)
			}
		}
		cleaned := filepath.Clean(asset.RelativePath)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("runtime-oracle: path escapes producer directory: %s", asset.RelativePath)
		}

		target := filepath.Join(producerChild, filepath.FromSlash(asset.RelativePath))
		rel, relErr := filepath.Rel(producerChild, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("runtime-oracle: path escapes producer directory: %s", asset.RelativePath)
		}

		targetDir := filepath.Dir(target)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return "", fmt.Errorf("runtime-oracle: create asset directory %s: %w", targetDir, err)
		}

		for d := targetDir; d != producerChild && len(d) > len(producerChild); d = filepath.Dir(d) {
			info, lstatErr := os.Lstat(d)
			if lstatErr != nil {
				return "", fmt.Errorf("runtime-oracle: stat asset dir %s: %w", d, lstatErr)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("runtime-oracle: symlink component rejected: %s", d)
			}
		}

		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return "", fmt.Errorf("runtime-oracle: create exclusive asset %s: %w", asset.RelativePath, err)
		}
		if _, err := f.Write(asset.Data); err != nil {
			f.Close()
			return "", fmt.Errorf("runtime-oracle: write asset %s: %w", asset.RelativePath, err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("runtime-oracle: close asset %s: %w", asset.RelativePath, err)
		}
	}

	return producerChild, nil
}

func exportGeneratedAssetsFromEnvironment(
	t *testing.T,
	repoRoot string,
	producerID string,
	assets []generatedAsset,
) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	dir, err := exportGeneratedAssets(repoRoot, exportRoot, producerID, assets)
	if err != nil {
		t.Fatalf("export generated assets for %s: %v", producerID, err)
	}
	return dir
}

// GoOperation executes one corpus case through a real Go producer.
//
// The producer receives only the case specification and the case input bytes.
// The recorded expected outcome deliberately stays outside this signature: a
// producer that could read the expectation could be written to agree with the
// recorded evidence instead of with the production codec, which would turn the
// corpus into a tautology. The returned bytes are the producer's own encoded
// output and are digested into the observation when the family publishes one.
type GoOperation func(CaseSpec, []byte) (Outcome, []byte, error)

// goOperations is the test-only registry keyed by the manifest operation name.
// A manifest naming an operation with no registered producer fails the run
// instead of silently dropping that coverage.
var goOperations = map[string]GoOperation{
	"decode": runFrameDecode,
	"admit":  runDomainAdmit,
}

// goFamilyOperations binds every family this package can execute to the
// operation that executes it. A manifest naming an unsupported family is a hard
// error, so a newly added corpus case cannot pass by being ignored.
var goFamilyOperations = map[string]string{
	"protocol.frame":           "decode",
	"domain.values":            "admit",
	"domain.identity_values":   "admit",
	"domain.command_control":   "admit",
	"domain.command_inventory": "admit",
	"domain.event":             "admit",
}

// runDomainAdmit dispatches one admission case to the producer that owns its
// family. The domain families share one admission operation because each case
// asks a Go authority whether one value is admitted, so the operation name
// registered in `goFamilyOperations` cannot select the family; the family
// itself does, and a family with no producer is a hard error rather than a
// silently skipped case.
//
// The `domain.event` family is shared by two producers, so its case routes
// through `runDomainEvent`, which selects the producer by the rule the case
// names.
func runDomainAdmit(c CaseSpec, input []byte) (Outcome, []byte, error) {
	switch c.Family {
	case domainValuesFamily:
		return runDomainValues(c, input)
	case domainIdentityFamily:
		return runDomainIdentityValues(c, input)
	case domainControlFamily:
		return runDomainControl(c, input)
	case domainCommandInventoryFamily:
		return runDomainCommandInventory(c, input)
	case domainEventPlayerFamily:
		return runDomainEvent(c, input)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names admission family %q, which has no Go producer", c.ID, c.Family)
	}
}

// RunCases executes the manifest selection through the registered Go producers
// and returns one executed observation per declared checkpoint.
//
// The runner owns the evidence boundary: it resolves each case input under the
// corpus byte budget, proves the input digest matches the manifest, invokes the
// registered producer once per declared checkpoint, and derives every
// executed observation from the values the producer returned. Expected outcomes
// are never read or handed to a producer, so an independent execution cannot
// be shaped by the recorded expectation.
func RunCases(root string, manifest Inventory, operations map[string]GoOperation, familyOperations map[string]string) ([]ExecutedObservation, error) {
	if len(manifest.Cases) == 0 {
		return nil, fmt.Errorf("runtime-oracle: manifest selection is empty")
	}
	if len(operations) == 0 || len(familyOperations) == 0 {
		return nil, fmt.Errorf("runtime-oracle: no registered Go producers")
	}

	var produced []ExecutedObservation
	for _, c := range manifest.Cases {
		operation, supported := familyOperations[c.Family]
		if !supported {
			return nil, fmt.Errorf("runtime-oracle: family %s has no registered Go producer", c.Family)
		}
		producer, registered := operations[operation]
		if !registered {
			return nil, fmt.Errorf("runtime-oracle: operation %q has no registered Go producer", operation)
		}
		if c.Operation != operation {
			return nil, fmt.Errorf("runtime-oracle: case %s declares operation %q but family %s is executed as %q", c.ID, c.Operation, c.Family, operation)
		}
		if len(c.Checkpoints) == 0 {
			return nil, fmt.Errorf("runtime-oracle: case %s declares no checkpoints", c.ID)
		}

		input, err := readCaseInput(root, c)
		if err != nil {
			return nil, fmt.Errorf("runtime-oracle: case %s input: %w", c.ID, err)
		}
		for _, checkpoint := range c.Checkpoints {
			tick, err := strconv.ParseUint(checkpoint, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("runtime-oracle: case %s checkpoint %q: %w", c.ID, checkpoint, err)
			}
			outcome, encoded, err := producer(c, input)
			if err != nil {
				return nil, fmt.Errorf("runtime-oracle: case %s tick %d: %w", c.ID, tick, err)
			}
			if int64(len(encoded)) > MaxBinaryBytes {
				return nil, fmt.Errorf("runtime-oracle: case %s encoded output %d bytes exceeds budget %d", c.ID, len(encoded), MaxBinaryBytes)
			}
			// The encoded digest is derived from what the producer returned, so
			// the observation stays a record of execution rather than of intent.
			if len(encoded) > 0 {
				sum := sha256.Sum256(encoded)
				outcome.EncodedPayloadDigest = fmt.Sprintf("sha256:%x", sum)
			}
			produced = append(produced, ExecutedObservation{Tick: tick, CaseID: c.ID, Outcome: outcome})
		}
	}
	return produced, nil
}

// readCaseInput loads one case input under the corpus byte budget and proves
// its digest matches the manifest, so a producer never executes bytes the
// manifest does not name.
func readCaseInput(root string, c CaseSpec) ([]byte, error) {
	if err := validateAsset(root, c.Input, c.InputFormat == "json", MaxBinaryBytes); err != nil {
		return nil, err
	}
	full := filepath.Join(root, filepath.FromSlash(c.Input.Path))
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxBinaryBytes {
		return nil, fmt.Errorf("input %d bytes exceeds budget %d", len(data), MaxBinaryBytes)
	}
	return data, nil
}

// readExpectedOutcome decodes the normalized outcome one case records, which is
// the expectation an independent execution has to reproduce.
func readExpectedOutcome(t *testing.T, root string, c CaseSpec) Outcome {
	t.Helper()
	outcome, err := decodeExpectedOutcome(root, c)
	if err != nil {
		t.Fatalf("read expected outcome for %s: %v", c.ID, err)
	}
	return outcome
}

// decodeExpectedOutcome reads and normalizes the expectation one case records
// without a test handle, so a fixture that has no *testing.T to fail through
// derives its observation from the corpus instead of restating an outcome.
func decodeExpectedOutcome(root string, c CaseSpec) (Outcome, error) {
	full := filepath.Join(root, filepath.FromSlash(c.Expected.Path))
	data, err := os.ReadFile(full)
	if err != nil {
		return Outcome{}, fmt.Errorf("read %s: %w", full, err)
	}
	var outcome Outcome
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&outcome); err != nil {
		return Outcome{}, fmt.Errorf("decode %s: %w", full, err)
	}
	return outcome, nil
}

// traceFromObservations assembles executed observations into a trace identity
// so the evidence is checked by the same completeness rules a published report
// must satisfy.
func traceFromObservations(manifest Inventory, observations []ExecutedObservation) (Trace, error) {
	return BuildTrace(TraceRequest{}, manifest, observations)
}

// exportExecutedEvidence publishes the executed corpus evidence into the
// harness-owned directory named by RUNTIME_ORACLE_EXPORT_DIR and returns that
// directory. An unset variable exports nothing, so evidence never lands in the
// repository by default; a named directory must be fresh so an export can never
// overwrite evidence an earlier run published. The report itself is published
// through ExportTrace, which gives the export the same containment, symlink and
// no-replace gates the production reports use, and the per-case fixtures are
// written into the directory that publication just created and validated.
func exportExecutedEvidence(t *testing.T, root string, manifest Inventory, observations []ExecutedObservation) (string, error) {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if value == "" {
		return "", nil
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		return "", err
	}
	reportTarget := filepath.Join(t.TempDir(), "runtime-corpus-frame.json")
	if err := ExportTrace(root, reportTarget, trace, manifest); err != nil {
		return "", fmt.Errorf("runtime-oracle: export executed evidence: %w", err)
	}
	reportData, err := os.ReadFile(reportTarget)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: read exported report: %w", err)
	}

	var assets []generatedAsset
	assets = append(assets, generatedAsset{
		RelativePath: "runtime-corpus-frame.json",
		Data:         reportData,
	})

	caseByID := make(map[string]CaseSpec, len(manifest.Cases))
	for _, c := range manifest.Cases {
		caseByID[c.ID] = c
	}
	for _, obs := range observations {
		c, ok := caseByID[obs.CaseID]
		if !ok {
			return "", fmt.Errorf("runtime-oracle: observation names unknown case %s", obs.CaseID)
		}
		input, err := readCaseInput(root, c)
		if err != nil {
			return "", fmt.Errorf("runtime-oracle: export case %s input: %w", c.ID, err)
		}
		expected, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.Expected.Path)))
		if err != nil {
			return "", fmt.Errorf("runtime-oracle: export case %s expectation: %w", c.ID, err)
		}
		outcome, err := marshalCanonicalJSON(obs.Outcome)
		if err != nil {
			return "", fmt.Errorf("runtime-oracle: export case %s outcome: %w", c.ID, err)
		}
		assets = append(assets,
			generatedAsset{RelativePath: filepath.ToSlash(filepath.Join(c.ID, "input.bin")), Data: input},
			generatedAsset{RelativePath: filepath.ToSlash(filepath.Join(c.ID, "expected.json")), Data: expected},
			generatedAsset{RelativePath: filepath.ToSlash(filepath.Join(c.ID, "outcome.json")), Data: outcome},
		)
	}

	return exportGeneratedAssets(root, value, "runtime-oracle/protocol-frame", assets)
}

// marshalCanonicalJSON renders one value with recursively sorted keys so two
// exports of the same evidence are byte-identical.
func marshalCanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generic any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, generic); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// computeTrackedCorpusDigest computes a deterministic SHA256 digest over all
// files under testdata/runtime-migration to prove the tree remains unchanged.
func computeTrackedCorpusDigest(t *testing.T, repoRoot string) string {
	t.Helper()
	casesDir := filepath.Join(repoRoot, "testdata", "runtime-migration")
	hasher := sha256.New()
	err := filepath.WalkDir(casesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(casesDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hasher, "%s\x00%x\n", filepath.ToSlash(rel), sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatalf("compute tracked corpus digest: %v", err)
	}
	return fmt.Sprintf("sha256:%x", hasher.Sum(nil))
}

// assertTrackedCorpusUnchanged asserts the tracked corpus digest matches the baseline.
func assertTrackedCorpusUnchanged(t *testing.T, repoRoot, before string) {
	t.Helper()
	after := computeTrackedCorpusDigest(t, repoRoot)
	if before != after {
		t.Fatalf("tracked corpus was modified: before %s, after %s", before, after)
	}
}

// TestExportGeneratedAssetsRejectsRepositoryContainedRoots pins that an export
// root inside the repository is refused before anything is created.
func TestExportGeneratedAssetsRejectsRepositoryContainedRoots(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	exportRoot := filepath.Join(root, "export-contained-probe")
	assets := []generatedAsset{{RelativePath: "test.json", Data: []byte("{}\n")}}
	_, err := exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-values", assets)
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected live-path rejection, got: %v", err)
	}
	if _, statErr := os.Lstat(exportRoot); !os.IsNotExist(statErr) {
		t.Fatalf("export directory was created inside repository: %v", statErr)
	}
}

// TestExportGeneratedAssetsRejectsSymlinkedAncestor pins that an export root
// reached through a symlink ancestor is refused.
func TestExportGeneratedAssetsRejectsSymlinkedAncestor(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(parent, "symlink-ancestor")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatal(err)
	}
	exportRoot := filepath.Join(symlinkDir, "export-target")
	assets := []generatedAsset{{RelativePath: "test.json", Data: []byte("{}\n")}}
	_, err := exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-values", assets)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got: %v", err)
	}
	entries, readErr := os.ReadDir(realDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("export wrote through symlink ancestor: %v", entries)
	}
}

// TestExportGeneratedAssetsRejectsEscapingRelativePath pins that asset paths
// escaping the producer child, carrying backslashes, or containing ./.. are rejected.
func TestExportGeneratedAssetsRejectsEscapingRelativePath(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	exportRoot := filepath.Join(t.TempDir(), "escaping-test")
	badPaths := []string{
		"../escaped.json",
		"/absolute.json",
		"sub/../../escaped.json",
		"sub\\backslash.json",
		"./current.json",
		"sub/./current.json",
		"sub/../escaped.json",
	}
	for _, bad := range badPaths {
		assets := []generatedAsset{{RelativePath: bad, Data: []byte("{}\n")}}
		_, err := exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-values", assets)
		if err == nil {
			t.Errorf("expected rejection for escaping relative path %q, got nil", bad)
		}
	}
}

// TestExportGeneratedAssetsRejectsPreexistingProducerChild pins that a producer
// directory that already exists is refused and cannot be overwritten.
func TestExportGeneratedAssetsRejectsPreexistingProducerChild(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	exportRoot := filepath.Join(t.TempDir(), "preexisting-test")
	assets := []generatedAsset{{RelativePath: "initial.txt", Data: []byte("first\n")}}
	published, err := exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-values", assets)
	if err != nil {
		t.Fatalf("initial export failed: %v", err)
	}
	wantPublished := filepath.Join(exportRoot, "runtime-oracle", "domain-values")
	if published != wantPublished {
		t.Fatalf("published = %s, want %s", published, wantPublished)
	}

	secondAssets := []generatedAsset{{RelativePath: "initial.txt", Data: []byte("second\n")}}
	_, err = exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-values", secondAssets)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists rejection, got: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(published, "initial.txt"))
	if readErr != nil {
		t.Fatalf("read asset: %v", readErr)
	}
	if string(data) != "first\n" {
		t.Fatalf("existing asset was overwritten: %q", string(data))
	}
}

// TestExportGeneratedAssetsSuccessfulMultiProducerExport pins that multiple
// distinct producers can export into the same external export root.
func TestExportGeneratedAssetsSuccessfulMultiProducerExport(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	exportRoot := filepath.Join(t.TempDir(), "multi-export-test")
	assets1 := []generatedAsset{
		{RelativePath: "values.json", Data: []byte("{\"val\":1}\n")},
	}
	pub1, err := exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-values", assets1)
	if err != nil {
		t.Fatalf("producer 1 export failed: %v", err)
	}

	assets2 := []generatedAsset{
		{RelativePath: "identity.json", Data: []byte("{\"id\":2}\n")},
	}
	pub2, err := exportGeneratedAssets(root, exportRoot, "runtime-oracle/domain-identity-values", assets2)
	if err != nil {
		t.Fatalf("producer 2 export failed: %v", err)
	}

	wantPub1 := filepath.Join(exportRoot, "runtime-oracle", "domain-values")
	wantPub2 := filepath.Join(exportRoot, "runtime-oracle", "domain-identity-values")
	if pub1 != wantPub1 {
		t.Fatalf("producer 1 path = %s, want %s", pub1, wantPub1)
	}
	if pub2 != wantPub2 {
		t.Fatalf("producer 2 path = %s, want %s", pub2, wantPub2)
	}

	d1, err := os.ReadFile(filepath.Join(pub1, "values.json"))
	if err != nil || string(d1) != "{\"val\":1}\n" {
		t.Fatalf("unexpected asset 1 content: %q, err: %v", string(d1), err)
	}
	d2, err := os.ReadFile(filepath.Join(pub2, "identity.json"))
	if err != nil || string(d2) != "{\"id\":2}\n" {
		t.Fatalf("unexpected asset 2 content: %q, err: %v", string(d2), err)
	}
}
