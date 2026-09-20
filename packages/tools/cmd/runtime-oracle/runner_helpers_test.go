package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

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
	"protocol.frame":         "decode",
	"domain.values":          "admit",
	"domain.identity_values": "admit",
}

// runDomainAdmit dispatches one admission case to the producer that owns its
// family. The domain families share one admission operation because each
// case asks a Go authority whether one value is admitted, so the operation name
// registered in `goFamilyOperations` cannot select the family; the family
// itself does, and a family with no producer is a hard error rather than a
// silently skipped case.
func runDomainAdmit(c CaseSpec, input []byte) (Outcome, []byte, error) {
	switch c.Family {
	case domainValuesFamily:
		return runDomainValues(c, input)
	case domainIdentityFamily:
		return runDomainIdentityValues(c, input)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names admission family %q, which has no Go producer", c.ID, c.Family)
	}
}

// producedCheckpoint pairs one produced outcome with the checkpoint that
// produced it, so ordering and reporting can never mix two executions.
type producedCheckpoint struct {
	tick    uint64
	caseID  string
	c       CaseSpec
	outcome Outcome
}

// RunCases executes the manifest selection through the registered Go producers
// and returns one observation per declared checkpoint.
//
// The runner owns the evidence boundary: it resolves each case input under the
// corpus byte budget, proves the input digest matches the manifest, invokes the
// registered producer once per declared checkpoint, and derives every
// observation from the values the producer returned. Expected outcomes are read
// only for the digest the observation carries and are never handed to a
// producer, so an independent execution cannot be shaped by the recorded
// expectation.
func RunCases(root string, manifest Inventory, operations map[string]GoOperation, familyOperations map[string]string) ([]Observation, error) {
	if len(manifest.Cases) == 0 {
		return nil, fmt.Errorf("runtime-oracle: manifest selection is empty")
	}
	if len(operations) == 0 || len(familyOperations) == 0 {
		return nil, fmt.Errorf("runtime-oracle: no registered Go producers")
	}

	var produced []producedCheckpoint
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
			produced = append(produced, producedCheckpoint{tick: tick, caseID: c.ID, c: c, outcome: outcome})
		}
	}
	return orderObservations(produced)
}

// orderObservations sorts the produced checkpoints into the deterministic
// (tick, case) order the trace schema requires and stamps each observation with
// the expected digest its case declares.
func orderObservations(produced []producedCheckpoint) ([]Observation, error) {
	if len(produced) > MaxObservations {
		return nil, fmt.Errorf("runtime-oracle: observations count %d exceeds budget %d", len(produced), MaxObservations)
	}
	sorted := append([]producedCheckpoint(nil), produced...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].tick != sorted[j].tick {
			return sorted[i].tick < sorted[j].tick
		}
		return sorted[i].caseID < sorted[j].caseID
	})
	observations := make([]Observation, 0, len(sorted))
	for _, entry := range sorted {
		observations = append(observations, Observation{
			Tick:           entry.tick,
			CaseID:         entry.caseID,
			Outcome:        entry.outcome,
			ExpectedDigest: entry.c.Expected.SHA256,
		})
	}
	return observations, nil
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
func traceFromObservations(manifest Inventory, observations []Observation) (Trace, error) {
	digest, err := CanonicalCorpusDigest(manifest)
	if err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: corpus digest: %w", err)
	}
	caseByID := make(map[string]CaseSpec, len(manifest.Cases))
	for _, c := range manifest.Cases {
		caseByID[c.ID] = c
	}
	inputs := make([]TraceInput, 0, len(observations))
	tickSet := make(map[uint64]bool, len(observations))
	for i, obs := range observations {
		c, ok := caseByID[obs.CaseID]
		if !ok {
			return Trace{}, fmt.Errorf("runtime-oracle: observation names unknown case %s", obs.CaseID)
		}
		inputs = append(inputs, TraceInput{
			Index:       uint64(i),
			CaseID:      obs.CaseID,
			Tick:        obs.Tick,
			InputDigest: c.Input.SHA256,
		})
		tickSet[obs.Tick] = true
	}
	schedule := make([]uint64, 0, len(tickSet))
	for tick := range tickSet {
		schedule = append(schedule, tick)
	}
	sort.Slice(schedule, func(i, j int) bool { return schedule[i] < schedule[j] })
	return Trace{
		SchemaVersion:  traceSchemaVersion,
		SourceRevision: manifest.SourceRevision,
		CorpusDigest:   digest,
		Identities:     manifest.Identities,
		Seed:           "0",
		TickSchedule:   schedule,
		Inputs:         inputs,
		Observations:   observations,
	}, nil
}

// exportExecutedEvidence publishes the executed corpus evidence into the
// harness-owned directory named by RUNTIME_ORACLE_EXPORT_DIR and returns that
// directory. An unset variable exports nothing, so evidence never lands in the
// repository by default; a named directory must be fresh so an export can never
// overwrite evidence an earlier run published. The report itself is published
// through ExportTrace, which gives the export the same containment, symlink and
// no-replace gates the production reports use, and the per-case fixtures are
// written into the directory that publication just created and validated.
func exportExecutedEvidence(t *testing.T, root string, manifest Inventory, observations []Observation) (string, error) {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if value == "" {
		return "", nil
	}
	absDir, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: resolve export directory %s: %w", value, err)
	}
	// A named directory has to be both fresh and directly addressed: an existing
	// entry is refused, and an entry that is itself a symlink is refused rather
	// than followed, so a redirect can never move an export outside the
	// directory the harness named.
	if info, statErr := os.Lstat(absDir); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("runtime-oracle: symlink component rejected: %s", absDir)
		}
		return "", fmt.Errorf("runtime-oracle: export directory %s is not fresh", value)
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("runtime-oracle: stat export directory %s: %w", value, statErr)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		return "", err
	}
	report := filepath.Join(absDir, "runtime-corpus-frame.json")
	if err := ExportTrace(root, report, trace, manifest); err != nil {
		return "", fmt.Errorf("runtime-oracle: export executed evidence: %w", err)
	}

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
		writeExportFile(t, absDir, filepath.Join(c.ID, "input.bin"), input)
		writeExportFile(t, absDir, filepath.Join(c.ID, "expected.json"), expected)
		writeExportFile(t, absDir, filepath.Join(c.ID, "outcome.json"), outcome)
	}
	return absDir, nil
}

// writeExportFile writes one exported artifact below dir. The joined path is
// re-checked against the export root so a case identity can never steer a write
// outside the harness-owned directory.
func writeExportFile(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	target := filepath.Join(dir, rel)
	contained, relErr := filepath.Rel(dir, target)
	if relErr != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		t.Fatalf("runtime-oracle: export path %s escapes %s", target, dir)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("runtime-oracle: create export directory: %v", err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatalf("runtime-oracle: write export file: %v", err)
	}
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
