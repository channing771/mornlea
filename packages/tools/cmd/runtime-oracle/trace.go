package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const traceSchemaVersion = 2

// TraceRequest is one isolated offline replay request.
type TraceRequest struct {
	Root           string
	WorkDir        string
	OutputPath     string
	SourceRevision string
	Seed           string
	TickSchedule   []uint64
	SelectedCases  []string
	Coverage       string
}

// Trace is the versioned, language-neutral replay identity.
type Trace struct {
	SchemaVersion  int           `json:"schema_version"`
	SourceRevision string        `json:"source_revision"`
	CorpusDigest   string        `json:"corpus_digest"`
	Identities     Identities    `json:"identities"`
	Seed           string        `json:"seed"`
	Coverage       string        `json:"coverage,omitempty"`
	SelectedCases  []string      `json:"selected_cases,omitempty"`
	TickSchedule   []uint64      `json:"tick_schedule"`
	Inputs         []TraceInput  `json:"inputs"`
	Observations   []Observation `json:"observations"`
}

// TraceInput records one ordered checkpoint input.
type TraceInput struct {
	Index                uint64 `json:"index"`
	CaseID               string `json:"case_id"`
	Tick                 uint64 `json:"tick"`
	InputDigest          string `json:"input_digest"`
	ArgumentsDigest      string `json:"arguments_digest,omitempty"`
	TypedArgumentsDigest string `json:"typed_arguments_digest,omitempty"`
}

// Observation is the normalized checkpoint outcome emitted at one scheduled tick.
type Observation struct {
	Tick           uint64  `json:"tick"`
	CaseID         string  `json:"case_id"`
	Outcome        Outcome `json:"outcome"`
	ExpectedDigest string  `json:"expected_digest"`
}

// Outcome represents normalized result fields or error classification.
type Outcome struct {
	Kind                 string         `json:"kind"`
	Category             string         `json:"category,omitempty"`
	Fields               map[string]any `json:"fields,omitempty"`
	PayloadDigest        string         `json:"payload_digest,omitempty"`
	EncodedPayloadDigest string         `json:"encoded_payload_digest,omitempty"`
	LogicalPayloadDigest string         `json:"logical_payload_digest,omitempty"`
}

// CheckpointKey uniquely identifies one scheduled observation.
type CheckpointKey struct {
	Tick   uint64
	CaseID string
}

// TraceError lists every identity or completeness failure for one trace.
type TraceError struct {
	Problems []string
}

func (err *TraceError) Error() string {
	if err == nil || len(err.Problems) == 0 {
		return "runtime-oracle: trace error"
	}
	return "runtime-oracle: " + strings.Join(err.Problems, "; ")
}

// RunTrace executes an isolated trace run and emits a complete replay identity.
func RunTrace(request TraceRequest) (Trace, error) {
	if strings.TrimSpace(request.SourceRevision) == "" {
		return Trace{}, fmt.Errorf("runtime-oracle: incomplete identity: source revision")
	}
	if strings.TrimSpace(request.Root) == "" {
		return Trace{}, fmt.Errorf("runtime-oracle: incomplete identity: repository root")
	}
	if strings.TrimSpace(request.WorkDir) == "" {
		return Trace{}, fmt.Errorf("runtime-oracle: incomplete identity: work dir")
	}
	if live, err := isLivePath(request.Root, request.WorkDir); err != nil {
		return Trace{}, err
	} else if live {
		return Trace{}, fmt.Errorf("runtime-oracle: live-path write rejected: work dir %s", request.WorkDir)
	}
	if request.OutputPath != "" {
		if live, err := isLivePath(request.Root, request.OutputPath); err != nil {
			return Trace{}, err
		} else if live {
			return Trace{}, fmt.Errorf("runtime-oracle: live-path write rejected: output path %s", request.OutputPath)
		}
	}

	families, live, err := Discover(request.Root)
	if err != nil {
		return Trace{}, err
	}
	inventoryPath := filepath.Join(request.Root, filepath.FromSlash(InventoryRelPath))
	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		return Trace{}, err
	}
	if err := Reconcile(request.Root, inventory, families, live); err != nil {
		return Trace{}, err
	}

	if err := os.MkdirAll(request.WorkDir, 0o755); err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: create work dir: %w", err)
	}

	corpusDigest, err := CanonicalCorpusDigest(inventory)
	if err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: corpus digest: %w", err)
	}

	if request.Seed == "" {
		request.Seed = "0"
	}

	manifestCaseByID := make(map[string]CaseSpec, len(inventory.Cases))
	for _, c := range inventory.Cases {
		manifestCaseByID[c.ID] = c
	}

	var selectedCases []CaseSpec
	if len(request.SelectedCases) > 0 {
		for _, id := range request.SelectedCases {
			c, ok := manifestCaseByID[id]
			if !ok {
				return Trace{}, fmt.Errorf("runtime-oracle: unknown selected case: %s", id)
			}
			selectedCases = append(selectedCases, c)
		}
	} else {
		selectedCases = inventory.Cases
	}

	type pendingCP struct {
		tick   uint64
		caseID string
		c      CaseSpec
	}
	var cps []pendingCP
	tickSet := make(map[uint64]bool)
	for _, c := range selectedCases {
		for _, cpStr := range c.Checkpoints {
			tick, err := strconv.ParseUint(cpStr, 10, 64)
			if err != nil {
				return Trace{}, fmt.Errorf("runtime-oracle: parse checkpoint tick %q: %w", cpStr, err)
			}
			cps = append(cps, pendingCP{tick: tick, caseID: c.ID, c: c})
			tickSet[tick] = true
		}
	}

	sort.SliceStable(cps, func(i, j int) bool {
		if cps[i].tick != cps[j].tick {
			return cps[i].tick < cps[j].tick
		}
		return cps[i].caseID < cps[j].caseID
	})

	inputs := make([]TraceInput, len(cps))
	observations := make([]Observation, len(cps))
	for i, cp := range cps {
		inputs[i] = TraceInput{
			Index:       uint64(i),
			CaseID:      cp.caseID,
			Tick:        cp.tick,
			InputDigest: cp.c.Input.SHA256,
		}

		expectedPath := filepath.Join(request.Root, filepath.FromSlash(cp.c.Expected.Path))
		data, err := os.ReadFile(expectedPath)
		if err != nil {
			return Trace{}, fmt.Errorf("runtime-oracle: read expected file %s: %w", expectedPath, err)
		}
		var outcome Outcome
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&outcome); err != nil {
			return Trace{}, fmt.Errorf("runtime-oracle: decode expected file %s: %w", expectedPath, err)
		}

		observations[i] = Observation{
			Tick:           cp.tick,
			CaseID:         cp.caseID,
			Outcome:        outcome,
			ExpectedDigest: cp.c.Expected.SHA256,
		}
	}

	var schedule []uint64
	if len(request.TickSchedule) > 0 {
		schedule = append([]uint64(nil), request.TickSchedule...)
	} else {
		for t := range tickSet {
			schedule = append(schedule, t)
		}
		sort.Slice(schedule, func(i, j int) bool { return schedule[i] < schedule[j] })
	}

	trace := Trace{
		SchemaVersion:  traceSchemaVersion,
		SourceRevision: request.SourceRevision,
		CorpusDigest:   corpusDigest,
		Identities:     inventory.Identities,
		Seed:           request.Seed,
		Coverage:       request.Coverage,
		SelectedCases:  request.SelectedCases,
		TickSchedule:   schedule,
		Inputs:         inputs,
		Observations:   observations,
	}

	if err := ValidateTrace(trace, inventory); err != nil {
		return Trace{}, err
	}

	if request.OutputPath != "" {
		encoded, err := json.MarshalIndent(trace, "", "  ")
		if err != nil {
			return Trace{}, fmt.Errorf("runtime-oracle: encode trace: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(request.OutputPath), 0o755); err != nil {
			return Trace{}, fmt.Errorf("runtime-oracle: create trace output dir: %w", err)
		}
		if err := os.WriteFile(request.OutputPath, append(encoded, '\n'), 0o644); err != nil {
			return Trace{}, fmt.Errorf("runtime-oracle: write trace: %w", err)
		}
	}

	return trace, nil
}

// LoadTrace reads a trace artifact and validates it against the manifest.
func LoadTrace(path string, manifest Inventory) (Trace, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: read trace: %w", err)
	}
	if info.Size() > MaxManifestBytes {
		return Trace{}, fmt.Errorf("runtime-oracle: trace file size %d exceeds budget %d", info.Size(), MaxManifestBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: read trace: %w", err)
	}
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: malformed trace: %w", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed[0] != '{' {
		return Trace{}, fmt.Errorf("runtime-oracle: malformed trace: expected object")
	}
	if err := validateNoDuplicateKeys(data); err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: duplicate trace json keys: %w", err)
	}
	var trace Trace
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&trace); err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: decode trace: %w", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		return Trace{}, err
	}
	return trace, nil
}

// ValidateTrace enforces schema budgets, manifest binding, strictly increasing
// schedule, exact input membership/indices, exact observations, and normalized
// outcome equality.
func ValidateTrace(trace Trace, manifest Inventory) error {
	var problems []string

	// 1. Schema / byte / count budgets
	if trace.SchemaVersion != traceSchemaVersion {
		problems = append(problems, fmt.Sprintf("trace schema_version %d, want %d", trace.SchemaVersion, traceSchemaVersion))
	}
	if len(trace.Inputs) > MaxCases {
		problems = append(problems, fmt.Sprintf("trace inputs count %d exceeds budget %d", len(trace.Inputs), MaxCases))
	}
	if len(trace.Observations) > MaxObservations {
		problems = append(problems, fmt.Sprintf("trace observations count %d exceeds budget %d", len(trace.Observations), MaxObservations))
	}
	if len(trace.TickSchedule) > MaxObservations {
		problems = append(problems, fmt.Sprintf("trace tick schedule count %d exceeds budget %d", len(trace.TickSchedule), MaxObservations))
	}

	// 2. Actual manifest / content binding
	if !sourceRevPattern.MatchString(trace.SourceRevision) {
		problems = append(problems, fmt.Sprintf("invalid source revision format %q", trace.SourceRevision))
	} else if trace.SourceRevision != manifest.SourceRevision {
		problems = append(problems, fmt.Sprintf("trace source revision %s does not match manifest %s", trace.SourceRevision, manifest.SourceRevision))
	}

	if !hexSha256Pattern.MatchString(trace.CorpusDigest) {
		problems = append(problems, fmt.Sprintf("invalid corpus digest format %q", trace.CorpusDigest))
	} else {
		manifestDigest, err := CanonicalCorpusDigest(manifest)
		if err != nil {
			problems = append(problems, fmt.Sprintf("compute canonical corpus digest: %v", err))
		} else if trace.CorpusDigest != manifestDigest {
			problems = append(problems, fmt.Sprintf("trace corpus digest %s does not match manifest canonical digest %s", trace.CorpusDigest, manifestDigest))
		}
	}

	if identitiesIncomplete(trace.Identities) {
		problems = append(problems, "incomplete identity")
	} else if trace.Identities != manifest.Identities {
		problems = append(problems, "trace identities do not match manifest identities")
	}

	if strings.TrimSpace(trace.Seed) == "" {
		problems = append(problems, "empty seed")
	} else if _, err := strconv.ParseUint(trace.Seed, 10, 64); err != nil {
		problems = append(problems, fmt.Sprintf("invalid seed decimal string %q", trace.Seed))
	}

	manifestCaseByID := make(map[string]CaseSpec, len(manifest.Cases))
	for _, c := range manifest.Cases {
		manifestCaseByID[c.ID] = c
	}

	var selectedCases map[string]CaseSpec
	if trace.SelectedCases != nil {
		if len(trace.SelectedCases) == 0 {
			problems = append(problems, "empty selection")
		} else {
			selectedCases = make(map[string]CaseSpec, len(trace.SelectedCases))
			seen := make(map[string]bool, len(trace.SelectedCases))
			for _, id := range trace.SelectedCases {
				if seen[id] {
					problems = append(problems, fmt.Sprintf("duplicate selected case %s", id))
					continue
				}
				seen[id] = true
				c, ok := manifestCaseByID[id]
				if !ok {
					problems = append(problems, fmt.Sprintf("unknown selected case %s", id))
					continue
				}
				selectedCases[id] = c
			}
			if len(trace.SelectedCases) < len(manifest.Cases) {
				if trace.Coverage == "complete" || strings.Contains(trace.Coverage, "complete") {
					problems = append(problems, "subset trace cannot label itself complete inventory coverage")
				}
			}
		}
	} else {
		if trace.Coverage == "subset" {
			problems = append(problems, "subset trace must list its selected IDs")
		}
		if len(manifest.Cases) == 0 {
			problems = append(problems, "empty selection")
		} else {
			selectedCases = manifestCaseByID
		}
	}

	// 3. Strictly increasing nonempty schedule
	if len(trace.TickSchedule) == 0 {
		problems = append(problems, "empty tick schedule")
	} else {
		for i := 1; i < len(trace.TickSchedule); i++ {
			if trace.TickSchedule[i] == trace.TickSchedule[i-1] {
				problems = append(problems, fmt.Sprintf("duplicate tick %d in schedule", trace.TickSchedule[i]))
			} else if trace.TickSchedule[i] < trace.TickSchedule[i-1] {
				problems = append(problems, fmt.Sprintf("schedule not strictly increasing: tick %d after %d", trace.TickSchedule[i], trace.TickSchedule[i-1]))
			}
		}
	}

	if len(selectedCases) == 0 || len(trace.TickSchedule) == 0 {
		if len(problems) > 0 {
			return &TraceError{Problems: problems}
		}
		return nil
	}

	scheduleTicks := make(map[uint64]bool, len(trace.TickSchedule))
	for _, t := range trace.TickSchedule {
		scheduleTicks[t] = true
	}

	expectedKeys := make(map[CheckpointKey]bool)
	expectedCasesByTick := make(map[uint64][]CaseSpec)
	for _, c := range selectedCases {
		for _, cpStr := range c.Checkpoints {
			cpTick, err := strconv.ParseUint(cpStr, 10, 64)
			if err != nil {
				problems = append(problems, fmt.Sprintf("case %s has invalid checkpoint tick %q", c.ID, cpStr))
				continue
			}
			if !scheduleTicks[cpTick] {
				problems = append(problems, fmt.Sprintf("case %s checkpoint tick %d not in tick schedule", c.ID, cpTick))
			}
			key := CheckpointKey{Tick: cpTick, CaseID: c.ID}
			expectedKeys[key] = true
			expectedCasesByTick[cpTick] = append(expectedCasesByTick[cpTick], c)
		}
	}

	for _, t := range trace.TickSchedule {
		if len(expectedCasesByTick[t]) == 0 {
			problems = append(problems, fmt.Sprintf("schedule tick %d has no corresponding checkpoints", t))
		}
	}

	// 4. Exact input membership and indices
	if len(trace.Inputs) != len(expectedKeys) {
		problems = append(problems, fmt.Sprintf("trace has %d inputs, expected %d", len(trace.Inputs), len(expectedKeys)))
	}
	seenInputKeys := make(map[CheckpointKey]bool, len(trace.Inputs))
	seenInputIndices := make(map[uint64]bool, len(trace.Inputs))
	for i, in := range trace.Inputs {
		if in.Index != uint64(i) {
			problems = append(problems, fmt.Sprintf("input at position %d has non-contiguous index %d", i, in.Index))
		}
		if seenInputIndices[in.Index] {
			problems = append(problems, fmt.Sprintf("duplicate input index %d", in.Index))
		}
		seenInputIndices[in.Index] = true

		c, ok := selectedCases[in.CaseID]
		if !ok {
			problems = append(problems, fmt.Sprintf("input has unknown case %s", in.CaseID))
			continue
		}
		key := CheckpointKey{Tick: in.Tick, CaseID: in.CaseID}
		if !expectedKeys[key] {
			problems = append(problems, fmt.Sprintf("unexpected input key at tick %d for case %s", in.Tick, in.CaseID))
		}
		if seenInputKeys[key] {
			problems = append(problems, fmt.Sprintf("duplicate input at tick %d for case %s", in.Tick, in.CaseID))
		}
		seenInputKeys[key] = true

		if in.InputDigest != c.Input.SHA256 {
			problems = append(problems, fmt.Sprintf("input digest %s does not match case %s input %s", in.InputDigest, in.CaseID, c.Input.SHA256))
		}
	}

	// 5. Exact observation key set
	if len(trace.Observations) != len(expectedKeys) {
		problems = append(problems, fmt.Sprintf("trace has %d observations, expected %d", len(trace.Observations), len(expectedKeys)))
	}
	seenObsKeys := make(map[CheckpointKey]bool, len(trace.Observations))
	for _, obs := range trace.Observations {
		c, ok := selectedCases[obs.CaseID]
		if !ok {
			problems = append(problems, fmt.Sprintf("observation has unknown case %s", obs.CaseID))
			continue
		}
		key := CheckpointKey{Tick: obs.Tick, CaseID: obs.CaseID}
		if !expectedKeys[key] {
			problems = append(problems, fmt.Sprintf("unexpected observation key at tick %d for case %s", obs.Tick, obs.CaseID))
		}
		if seenObsKeys[key] {
			problems = append(problems, fmt.Sprintf("duplicate observation at tick %d for case %s", obs.Tick, obs.CaseID))
		}
		seenObsKeys[key] = true

		if obs.ExpectedDigest != c.Expected.SHA256 {
			problems = append(problems, fmt.Sprintf("observation expected digest %s does not match case %s expected %s", obs.ExpectedDigest, obs.CaseID, c.Expected.SHA256))
		}

		// 6. Normalized outcome equality
		root, err := RepositoryRoot()
		if err == nil {
			expectedPath := c.Expected.Path
			if !filepath.IsAbs(expectedPath) {
				expectedPath = filepath.Join(root, filepath.FromSlash(expectedPath))
			}
			if data, err := os.ReadFile(expectedPath); err == nil {
				var expectedOutcome Outcome
				dec := json.NewDecoder(bytes.NewReader(data))
				dec.UseNumber()
				if err := dec.Decode(&expectedOutcome); err == nil {
					if !outcomesEqual(obs.Outcome, expectedOutcome) {
						problems = append(problems, fmt.Sprintf("observation for case %s tick %d outcome does not match normalized expected outcome %s", obs.CaseID, obs.Tick, c.Expected.Path))
					}
				}
			}
		}
	}

	for key := range expectedKeys {
		if !seenObsKeys[key] {
			problems = append(problems, fmt.Sprintf("missing observation at tick %d for case %s", key.Tick, key.CaseID))
		}
	}

	if len(problems) > 0 {
		return &TraceError{Problems: problems}
	}
	return nil
}

func outcomesEqual(a, b Outcome) bool {
	aBytes, err1 := json.Marshal(a)
	bBytes, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var aMap, bMap any
	decA := json.NewDecoder(bytes.NewReader(aBytes))
	decA.UseNumber()
	decB := json.NewDecoder(bytes.NewReader(bBytes))
	decB.UseNumber()
	if err := decA.Decode(&aMap); err != nil {
		return false
	}
	if err := decB.Decode(&bMap); err != nil {
		return false
	}
	return reflect.DeepEqual(aMap, bMap)
}

func identitiesIncomplete(id Identities) bool {
	return id.Protocol == 0 || id.ChunkSchema == 0 || id.PlayerSchema == 0 ||
		id.WorldMetadata == 0 || id.CompanionsAISchema == 0 || id.HostileMobsSchema == 0 ||
		id.PassiveMobsSchema == 0 || id.EngineABI == 0 || id.RegionFormat == 0 ||
		id.AgentHTTP == "" || id.AgentMCP == ""
}

func isLivePath(root, target string) (bool, error) {
	if strings.TrimSpace(target) == "" {
		return false, nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("runtime-oracle: resolve repository root: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false, fmt.Errorf("runtime-oracle: resolve path %s: %w", target, err)
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absTarget); err == nil {
		absTarget = resolved
	} else {
		parent := filepath.Dir(absTarget)
		if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			absTarget = filepath.Join(resolvedParent, filepath.Base(absTarget))
		}
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, fmt.Errorf("runtime-oracle: compare path %s: %w", target, err)
	}
	if rel == "." {
		return true, nil
	}
	return !strings.HasPrefix(rel, ".."), nil
}
