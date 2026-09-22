package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/network/codec"
)

const (
	// frameFamily is the only corpus family this package executes today.
	frameFamily = "protocol.frame"
	// corpusCasesRelDir is the repository-relative directory holding every
	// committed corpus case asset.
	corpusCasesRelDir = "testdata/runtime-migration/cases"
	// frameVersion is the protocol version the framing contract is pinned to.
	frameVersion = "45"
	// frameValidCaseID is the frozen case committed with the corpus.
	frameValidCaseID = frameFamily + "/" + frameVersion + "/valid"
	// frameNoncanonicalCaseID is the second frozen framing case: its manifest
	// entry landed with the corpus merge, so the runner reads it from the frozen
	// manifest like every other case instead of building an entry itself.
	frameNoncanonicalCaseID = frameFamily + "/" + frameVersion + "/noncanonical-length"
)

// frameRejectionCategory resolves the language-neutral rejection category for
// one real Go framing failure.
//
// The Go reader reports the shared `errInvalidUvarint` sentinel for a
// non-canonical length prefix, so the category names the wire condition rather
// than the Go sentinel: `invalid-varint` is exactly that wire condition, the
// Rust consumer rejects the same vector with its own non-canonical error, and
// the category is the only signal both sides publish. A failure with no
// mapping is a hard error, because an unclassified rejection must never be
// recorded as evidence.
func frameRejectionCategory(err error) (string, bool) {
	if strings.Contains(err.Error(), "invalid uvarint") {
		return "invalid-varint", true
	}
	return "", false
}

// runFrameDecode executes one framing case through the real Go frame reader.
//
// The producer only reads and writes frames through `codec.ReadFrame`; it never
// reimplements the length-prefix rule, so the recorded outcome is whatever the
// production codec decides about these exact bytes.
func runFrameDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	reader := bytes.NewReader(input)
	packetID, payload, err := codec.ReadFrame(reader)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: frame reader hit an unexpected end of input: %w", c.ID, err)
		}
		category, classified := frameRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified framing rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if reader.Len() > 0 {
		return Outcome{Kind: "error", Category: "trailing"}, nil, nil
	}
	return Outcome{
		Kind:     "ok",
		Category: "frame",
		Fields: map[string]any{
			"packet_id":   packetID,
			"payload":     hex.EncodeToString(payload),
			"payload_len": len(payload),
		},
	}, nil, nil
}

// frameWorkingManifest assembles the manifest this node executes inside a
// harness-owned temporary directory.
//
// The frozen manifest now carries the merged Agent contract cases alongside the
// two framing cases, and this package registers no Go producer for the Agent
// families: their schema validator is package-local to
// packages/shared/companion. The working manifest is therefore a family-scoped
// selection of the frozen one: `Cases` holds exactly the `protocol.frame` cases
// the frozen manifest registers, and every other family's case list is cleared,
// because `Reconcile` requires each family's `cases` list to match the cases
// this selection registers for it. Appending a locally built entry instead
// would register the frozen case twice and fail reconciliation as a duplicate,
// and leaving the other families listed would hand their cases to a runner with
// no producer for them.
func frameWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	assertFrameCasesAreFrozen(t, frozen)

	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          frameCases(frozen),
	}
	frameCaseIDs := frameFamilyCaseIDs(cloned.Cases, frameFamily)
	for index := range cloned.Families {
		// A family this selection does not execute registers no case, so its
		// list must be empty for reconciliation to accept the scoped manifest.
		if cloned.Families[index].ID != frameFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Cases = frameCaseIDs
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

// assertFrameCasesAreFrozen pins that the frozen corpus registers exactly the
// framing cases this package executes.
//
// The family case list and the case index are two separate views of the same
// registration, so both are checked: a case executed without a family entry, or
// listed in a family without a case record, is a registration break that would
// otherwise surface as a confusing coverage failure instead of a precise one.
func assertFrameCasesAreFrozen(t *testing.T, manifest Inventory) {
	t.Helper()
	want := []string{frameValidCaseID, frameNoncanonicalCaseID}

	var family *Family
	for i := range manifest.Families {
		if manifest.Families[i].ID == frameFamily {
			family = &manifest.Families[i]
			break
		}
	}
	if family == nil {
		t.Fatalf("frozen manifest has no %s family", frameFamily)
	}
	if len(family.Cases) != len(want) {
		t.Fatalf("%s family lists %v, want %v", frameFamily, family.Cases, want)
	}
	for i, id := range want {
		if family.Cases[i] != id {
			t.Fatalf("%s family lists %v, want %v", frameFamily, family.Cases, want)
		}
	}

	for _, id := range want {
		found := false
		for _, c := range manifest.Cases {
			if c.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("case %s is missing from the frozen manifest case index", id)
		}
	}
}

// frameCases selects the frozen manifest's framing cases, preserving the
// manifest's own case order.
//
// The order matters because the scoped family case list is built from the same
// selection, so the working manifest reproduces the frozen manifest's framing
// family list exactly rather than re-deriving a different one.
func frameCases(manifest Inventory) []CaseSpec {
	cases := make([]CaseSpec, 0, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Family == frameFamily {
			cases = append(cases, c)
		}
	}
	return cases
}

// frameFamilyCaseIDs lists the case identities of one family in selection
// order.
func frameFamilyCaseIDs(cases []CaseSpec, family string) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		if c.Family == family {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// frameCaseByID indexes a manifest selection by case identity.
func frameCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}

// TestProtocolOracleFrameIndependentOutcomes executes both framing cases
// through the real Go frame reader and requires every produced observation to
// reproduce the normalized outcome the corpus records.
func TestProtocolOracleFrameIndependentOutcomes(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)

	_, families, live := discoverLive(t)
	if _, err := ReconcileWorking(root, manifest, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("working manifest drifted from current registries: %v", err)
	}

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("produced %d observations, want 2 (one per case)", len(observations))
	}

	for _, obs := range observations {
		c := frameCaseByID(t, manifest, obs.CaseID)
		expected := readExpectedOutcome(t, root, c)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", obs.CaseID, obs.Outcome, expected)
		}
	}

	valid := frameObservation(t, observations, frameValidCaseID)
	if valid.Outcome.Kind != "ok" || valid.Outcome.Category != "frame" {
		t.Fatalf("valid framing case produced %#v, want an accepted frame", valid.Outcome)
	}
	if got := valid.Outcome.Fields["packet_id"]; got != uint32(0) {
		t.Fatalf("valid framing case decoded packet id %#v, want 0", got)
	}
	if got := valid.Outcome.Fields["payload"]; got != "2d" {
		t.Fatalf("valid framing case decoded payload %#v, want 2d", got)
	}

	noncanonical := frameObservation(t, observations, frameNoncanonicalCaseID)
	if noncanonical.Outcome.Kind != "error" {
		t.Fatalf("noncanonical length case produced %#v, want a structural error", noncanonical.Outcome)
	}
	if noncanonical.Outcome.Category != "invalid-varint" {
		t.Fatalf("noncanonical length case category %q, want invalid-varint", noncanonical.Outcome.Category)
	}
	if len(noncanonical.Outcome.Fields) != 0 {
		t.Fatalf("rejection published fields %#v, want none", noncanonical.Outcome.Fields)
	}

	// The executed evidence has to satisfy the same completeness rules a
	// published trace report does, so a run that produced wrong or partial
	// observations cannot be mistaken for evidence.
	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	for _, obs := range trace.Observations {
		c := frameCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed evidence failed trace validation: %v", err)
	}
}

// TestProtocolOracleFrameOutcomesDistinguishCases pins that the producer is not
// returning one constant answer: the valid vector and the noncanonical vector
// must produce different normalized outcomes.
func TestProtocolOracleFrameOutcomesDistinguishCases(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	valid := frameObservation(t, observations, frameValidCaseID)
	noncanonical := frameObservation(t, observations, frameNoncanonicalCaseID)
	if outcomesEqual(valid.Outcome, noncanonical.Outcome) {
		t.Fatalf("both framing cases produced the same outcome %#v", valid.Outcome)
	}
}

// TestProtocolOracleFrameRunnerRejectsTrailingBytes pins that the frame producer rejects
// bytes after the first decoded frame, matching Rust.
func TestProtocolOracleFrameRunnerRejectsTrailingBytes(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	c := frameCaseByID(t, manifest, frameValidCaseID)
	input, err := readCaseInput(root, c)
	if err != nil {
		t.Fatalf("readCaseInput: %v", err)
	}
	trailingInput := append(append([]byte(nil), input...), 0xff)
	outcome, _, err := runFrameDecode(c, trailingInput)
	if err != nil {
		t.Fatalf("runFrameDecode returned error: %v", err)
	}
	if outcome.Kind != "error" || outcome.Category != "trailing" {
		t.Fatalf("expected trailing rejection (kind=error, category=trailing), got: %#v", outcome)
	}
}

// TestProtocolOracleFrameRunnerRejectsUnregisteredOperation pins that a
// manifest selection naming an operation with no registered producer fails the
// run instead of dropping that coverage.
func TestProtocolOracleFrameRunnerRejectsUnregisteredOperation(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	_, err := RunCases(root, manifest, goOperations, map[string]string{frameFamily: "encode"})
	if err == nil || !strings.Contains(err.Error(), `operation "encode" has no registered Go producer`) {
		t.Fatalf("expected an unregistered-operation failure, got: %v", err)
	}
}

// TestProtocolOracleFrameRunnerRejectsUnregisteredFamily pins that a manifest
// selection naming a family this package cannot execute fails the run.
func TestProtocolOracleFrameRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "protocol.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family protocol.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestProtocolOracleFrameRunnerRejectsOperationFamilyMismatch pins that a case
// cannot declare one operation and be executed by another.
func TestProtocolOracleFrameRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "encode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation/family mismatch failure, got: %v", err)
	}
}

// TestProtocolOracleFrameRunnerRejectsTamperedInput pins that a producer never
// executes bytes the manifest does not name.
func TestProtocolOracleFrameRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	for i := range manifest.Cases {
		if manifest.Cases[i].ID == frameNoncanonicalCaseID {
			manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
		}
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// recordingOperation is a producer that records exactly what the runner handed
// it, so the runner's evidence boundary can be asserted instead of assumed.
type recordingOperation struct {
	outcome Outcome
	calls   int
	seen    []CaseSpec
	inputs  [][]byte
}

func (op *recordingOperation) run(c CaseSpec, input []byte) (Outcome, []byte, error) {
	op.calls++
	op.seen = append(op.seen, c)
	op.inputs = append(op.inputs, append([]byte(nil), input...))
	return op.outcome, nil, nil
}

// TestProtocolOracleFrameRunnerHandsProducerOnlyCaseAndInput pins the runner
// contract: the producer is invoked once per declared checkpoint with the case
// specification and the input bytes, and the observation carries the value the
// producer returned rather than the outcome the corpus records.
func TestProtocolOracleFrameRunnerHandsProducerOnlyCaseAndInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	// The recorded expectation for this case is an accepted frame; the spy
	// returns a structural error. If the runner substituted the expectation,
	// the observation would come back accepted and this assertion would fail.
	spy := &recordingOperation{outcome: Outcome{Kind: "error", Category: "spy"}}

	observations, err := RunCases(root, manifest,
		map[string]GoOperation{"decode": spy.run},
		map[string]string{frameFamily: "decode"})
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if spy.calls != 2 {
		t.Fatalf("producer invoked %d times, want 2 (one per declared checkpoint)", spy.calls)
	}
	if len(spy.seen) != 2 || len(spy.inputs) != 2 {
		t.Fatalf("producer recorded %d cases and %d inputs, want 2 each", len(spy.seen), len(spy.inputs))
	}
	for i, c := range spy.seen {
		if c.ID != manifest.Cases[i].ID || c.Operation != "decode" {
			t.Fatalf("producer received case %#v, want %s", c, manifest.Cases[i].ID)
		}
		want, err := readCaseInput(root, manifest.Cases[i])
		if err != nil {
			t.Fatalf("read case input: %v", err)
		}
		if string(spy.inputs[i]) != string(want) {
			t.Fatalf("producer received %x, want %x", spy.inputs[i], want)
		}
	}
	for _, obs := range observations {
		if !outcomesEqual(obs.Outcome, spy.outcome) {
			t.Fatalf("observation for %s carries %#v, want the producer's own %#v", obs.CaseID, obs.Outcome, spy.outcome)
		}
	}
}

// TestProtocolOracleFrameRunnerInvokesProducerOncePerCheckpoint pins that a
// multi-checkpoint case produces one observation and one invocation per
// checkpoint, in the deterministic (tick, case) order.
func TestProtocolOracleFrameRunnerInvokesProducerOncePerCheckpoint(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	for i := range manifest.Cases {
		if manifest.Cases[i].ID == frameNoncanonicalCaseID {
			manifest.Cases[i].Checkpoints = []string{"0", "1"}
		}
	}
	spy := &recordingOperation{outcome: Outcome{Kind: "ok", Category: "spy"}}

	observations, err := RunCases(root, manifest,
		map[string]GoOperation{"decode": spy.run},
		map[string]string{frameFamily: "decode"})
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if spy.calls != 3 {
		t.Fatalf("producer invoked %d times, want 3 (one per declared checkpoint)", spy.calls)
	}
	if len(observations) != 3 {
		t.Fatalf("produced %d observations, want 3", len(observations))
	}
	for i := 1; i < len(observations); i++ {
		if observations[i].Tick < observations[i-1].Tick {
			t.Fatalf("observations are not ordered by tick: %d after %d", observations[i].Tick, observations[i-1].Tick)
		}
	}
}

// TestProtocolOracleFrameExportWritesEvidenceWhenHarnessNamesDirectory pins the
// explicit export path: the harness names a fresh directory outside the
// repository, and the executed evidence plus the per-case fixtures land there.
func TestProtocolOracleFrameExportWritesEvidenceWhenHarnessNamesDirectory(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	exportDir := filepath.Join(t.TempDir(), "runtime-oracle-export")
	t.Setenv(runtimeOracleExportDirEnv, exportDir)
	published, err := exportExecutedEvidence(t, root, manifest, observations)
	if err != nil {
		t.Fatalf("exportExecutedEvidence: %v", err)
	}
	wantPublished := filepath.Join(exportDir, "runtime-oracle", "protocol-frame")
	if published != wantPublished {
		t.Fatalf("exported into %s, want %s", published, wantPublished)
	}

	report := filepath.Join(published, "runtime-corpus-frame.json")
	if _, err := LoadTraceAtRoot(root, report, manifest); err != nil {
		t.Fatalf("exported report does not validate: %v", err)
	}
	for _, c := range manifest.Cases {
		for _, name := range []string{"input.bin", "expected.json", "outcome.json"} {
			path := filepath.Join(published, filepath.FromSlash(c.ID), name)
			info, statErr := os.Lstat(path)
			if statErr != nil {
				t.Fatalf("exported fixture %s is missing: %v", path, statErr)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("exported fixture %s is a symlink", path)
			}
		}
	}
}

// TestProtocolOracleFrameExportRejectsRepositoryResidentDirectory pins that an
// export directory inside the repository is refused before anything is written.
func TestProtocolOracleFrameExportRejectsRepositoryResidentDirectory(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	inside := filepath.Join(root, "runtime-oracle-export-probe")
	t.Setenv(runtimeOracleExportDirEnv, inside)
	if _, err := exportExecutedEvidence(t, root, manifest, observations); err == nil ||
		!strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected a live-path rejection, got: %v", err)
	}
	if _, statErr := os.Lstat(inside); !os.IsNotExist(statErr) {
		t.Fatalf("export directory was created inside the repository: %v", statErr)
	}
}

// TestProtocolOracleFrameExportRejectsSymlinkedDirectory pins that an export
// addressed through a symlink is refused rather than followed.
func TestProtocolOracleFrameExportRejectsSymlinkedDirectory(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "export-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtimeOracleExportDirEnv, link)
	if _, err := exportExecutedEvidence(t, root, manifest, observations); err == nil ||
		!strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlink rejection, got: %v", err)
	}
	entries, readErr := os.ReadDir(real)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("export wrote through the symlink: %v", entries)
	}
}

// TestProtocolOracleFrameExportRefusesNonFreshDirectory pins that an export can
// never overwrite evidence an earlier run published.
func TestProtocolOracleFrameExportRefusesNonFreshDirectory(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	exportDir := filepath.Join(t.TempDir(), "runtime-oracle-export")
	producerDir := filepath.Join(exportDir, "runtime-oracle", "protocol-frame")
	if err := os.MkdirAll(producerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(producerDir, "sentinel"), []byte("sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtimeOracleExportDirEnv, exportDir)
	if _, err := exportExecutedEvidence(t, root, manifest, observations); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists rejection, got: %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(producerDir, "sentinel"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "sentinel\n" {
		t.Fatalf("earlier evidence was modified: %q", data)
	}
}

// TestProtocolOracleFrameExportDoesNothingWithoutEnvironmentDirectory pins that
// there is no repository default: with the variable unset, nothing is exported
// and no path is invented.
func TestProtocolOracleFrameExportDoesNothingWithoutEnvironmentDirectory(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := frameWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	t.Setenv(runtimeOracleExportDirEnv, "")
	published, err := exportExecutedEvidence(t, root, manifest, observations)
	if err != nil {
		t.Fatalf("exportExecutedEvidence: %v", err)
	}
	if published != "" {
		t.Fatalf("exported into %s with the variable unset", published)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "runtime-oracle-export") {
			t.Fatalf("an export directory appeared in the repository: %s", entry.Name())
		}
	}
}

// corpusOutcomeKinds is the closed outcome-kind vocabulary the execution
// contract publishes: a corpus outcome is either accepted or a structural,
// admission, or storage error, and nothing else.
var corpusOutcomeKinds = map[string]bool{
	"ok":    true,
	"error": true,
}

// corpusStructuralCategories is the frozen structural-error vocabulary the
// execution contract names for a corpus rejection.
var corpusStructuralCategories = map[string]bool{
	"truncated":           true,
	"trailing":            true,
	"invalid-varint":      true,
	"capacity":            true,
	"unsupported-version": true,
	"invalid-enum":        true,
	"invalid-identity":    true,
	"invalid-value":       true,
	"integrity":           true,
}

// corpusAdmissionCategories is the frozen login admission vocabulary, which is
// separate from the structural set because an admission decision is a policy
// result rather than a decoding failure.
var corpusAdmissionCategories = map[string]bool{
	"handshake-version-mismatch": true,
	"login-invalid-identity":     true,
	"login-protocol-violation":   true,
}

// corpusStorageCategories is the frozen storage vocabulary: the Go storage
// sentinels are normalized to these two values instead of their error prose.
var corpusStorageCategories = map[string]bool{
	"corrupt":        true,
	"future-version": true,
}

// TestCorpusOutcomeVocabularyMatchesExecutionContract pins every committed
// corpus expectation to the outcome vocabulary the execution contract freezes.
//
// A corpus file that publishes a kind or a rejection category outside the
// contract becomes a frozen artifact later packet nodes inherit, so the check
// reads the whole cases tree rather than only the cases this package executes
// today. Accepted outcomes may name their own subject category; a rejection has
// to be classifiable by both independent implementations.
func TestCorpusOutcomeVocabularyMatchesExecutionContract(t *testing.T) {
	root := mustRepoRoot(t)
	casesDir := filepath.Join(root, filepath.FromSlash(corpusCasesRelDir))

	var files []string
	if err := filepath.WalkDir(casesDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".expected.json") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk corpus cases directory: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.expected.json under %s: the vocabulary check would pass vacuously", corpusCasesRelDir)
	}
	sort.Strings(files)

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var outcome Outcome
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&outcome); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if !corpusOutcomeKinds[outcome.Kind] {
			t.Fatalf("%s publishes kind %q, want one of ok|error", path, outcome.Kind)
		}
		if outcome.Kind != "error" {
			continue
		}
		if corpusStructuralCategories[outcome.Category] ||
			corpusAdmissionCategories[outcome.Category] ||
			corpusStorageCategories[outcome.Category] {
			continue
		}
		t.Fatalf("%s publishes rejection category %q, which is not in the frozen execution contract vocabulary", path, outcome.Category)
	}
}

// frameObservation returns the observation produced for one case.
func frameObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, obs := range observations {
		if obs.CaseID == id {
			return obs
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}
