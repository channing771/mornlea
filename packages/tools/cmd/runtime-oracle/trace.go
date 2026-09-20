package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const traceSchemaVersion = 1

// `TraceRequest` is one isolated offline replay. `WorkDir` and `OutputPath`
// must stay outside the repository so the oracle never writes live saves or
// tracked fixtures.
type TraceRequest struct {
	Root           string
	WorkDir        string
	OutputPath     string
	SourceRevision string
	Seed           uint64
	TickSchedule   []uint64
}

// `Trace` is the versioned, language-neutral replay identity used by later
// Go/Rust differential comparison. Absolute work paths are omitted so two
// isolated runs of the same corpus produce identical bytes.
type Trace struct {
	SchemaVersion  int           `json:"schema_version"`
	SourceRevision string        `json:"source_revision"`
	CorpusDigest   string        `json:"corpus_digest"`
	Identities     Identities    `json:"identities"`
	Seed           uint64        `json:"seed"`
	TickSchedule   []uint64      `json:"tick_schedule"`
	Inputs         []TraceInput  `json:"inputs"`
	Observations   []Observation `json:"observations"`
}

// `TraceInput` records the ordered checkpoint schedule. Foundation traces
// do not invent gameplay commands; later owners attach domain inputs to the
// same identity fields.
type TraceInput struct {
	Index uint64 `json:"index"`
	Tick  uint64 `json:"tick"`
	Kind  string `json:"kind"`
}

// `Observation` is the normalized checkpoint emitted at one scheduled tick.
type Observation struct {
	Tick           uint64          `json:"tick"`
	FixtureDigests []FixtureDigest `json:"fixture_digests"`
}

// `FixtureDigest` pins one copied coverage fixture after isolated execution.
type FixtureDigest struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// `TraceError` lists every identity or completeness failure for one trace.
type TraceError struct {
	Problems []string
}

func (err *TraceError) Error() string {
	if err == nil || len(err.Problems) == 0 {
		return "runtime-oracle: trace error"
	}
	return "runtime-oracle: " + strings.Join(err.Problems, "; ")
}

// `RunTrace` copies coverage fixtures into an isolated work directory and
// emits a complete replay identity. It reads the frozen inventory and live
// registries but never writes under the repository tree.
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

	corpusDigest, err := fileDigest(inventoryPath)
	if err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: corpus digest: %w", err)
	}
	fixtureDigests, err := copyFixtures(request.Root, request.WorkDir, inventory.Families)
	if err != nil {
		return Trace{}, err
	}

	schedule := append([]uint64(nil), request.TickSchedule...)
	inputs := make([]TraceInput, len(schedule))
	observations := make([]Observation, len(schedule))
	for i, tick := range schedule {
		inputs[i] = TraceInput{Index: uint64(i), Tick: tick, Kind: "checkpoint"}
		cloned := append([]FixtureDigest(nil), fixtureDigests...)
		observations[i] = Observation{Tick: tick, FixtureDigests: cloned}
	}

	trace := Trace{
		SchemaVersion:  traceSchemaVersion,
		SourceRevision: request.SourceRevision,
		CorpusDigest:   corpusDigest,
		Identities:     inventory.Identities,
		Seed:           request.Seed,
		TickSchedule:   schedule,
		Inputs:         inputs,
		Observations:   observations,
	}
	if err := ValidateTrace(trace); err != nil {
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

// `LoadTrace` reads a previously emitted replay. Truncated, non-object, and
// unsupported schema bytes fail before any identity is trusted.
func LoadTrace(path string) (Trace, error) {
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
	var trace Trace
	if err := json.Unmarshal(raw, &trace); err != nil {
		return Trace{}, fmt.Errorf("runtime-oracle: decode trace: %w", err)
	}
	if trace.SchemaVersion != traceSchemaVersion {
		return Trace{}, fmt.Errorf("runtime-oracle: trace schema_version %d, want %d", trace.SchemaVersion, traceSchemaVersion)
	}
	if err := ValidateTrace(trace); err != nil {
		return Trace{}, err
	}
	return trace, nil
}

// `ValidateTrace` fails closed on missing source identity, corpus digest,
// tick schedule, or checkpoint observations. Partial traces are never
// counted as agreement.
func ValidateTrace(trace Trace) error {
	var problems []string
	if strings.TrimSpace(trace.SourceRevision) == "" {
		problems = append(problems, "incomplete identity: source revision")
	}
	if identitiesIncomplete(trace.Identities) {
		problems = append(problems, "incomplete identity")
	}
	if trace.SchemaVersion != traceSchemaVersion {
		problems = append(problems, fmt.Sprintf("incomplete trace: schema_version %d", trace.SchemaVersion))
	}
	if strings.TrimSpace(trace.CorpusDigest) == "" {
		problems = append(problems, "incomplete trace: corpus digest")
	}
	if len(trace.TickSchedule) == 0 {
		problems = append(problems, "incomplete trace: tick schedule")
	}
	if len(trace.Observations) == 0 {
		problems = append(problems, "incomplete trace: observations")
	}
	if len(problems) == 0 {
		return nil
	}
	return &TraceError{Problems: problems}
}

func identitiesIncomplete(id Identities) bool {
	return id.Protocol == 0 || id.ChunkSchema == 0 || id.PlayerSchema == 0 ||
		id.WorldMetadata == 0 || id.CompanionsAISchema == 0 || id.HostileMobsSchema == 0 ||
		id.PassiveMobsSchema == 0 || id.EngineABI == 0 || id.RegionFormat == 0 ||
		id.AgentHTTP == "" || id.AgentMCP == ""
}

func copyFixtures(root, workDir string, families []Family) ([]FixtureDigest, error) {
	seen := map[string]bool{}
	var paths []string
	for _, family := range families {
		for _, src := range family.Sources {
			fixture := src.Path
			if fixture == "" || seen[fixture] {
				continue
			}
			seen[fixture] = true
			paths = append(paths, fixture)
		}
	}
	sort.Strings(paths)

	isolatedRoot := filepath.Join(workDir, "fixtures")
	digests := make([]FixtureDigest, 0, len(paths))
	for _, rel := range paths {
		src := filepath.Join(root, filepath.FromSlash(rel))
		dst := filepath.Join(isolatedRoot, filepath.FromSlash(rel))
		if err := copyFile(src, dst); err != nil {
			return nil, fmt.Errorf("runtime-oracle: copy fixture %s: %w", rel, err)
		}
		digest, err := fileDigest(dst)
		if err != nil {
			return nil, fmt.Errorf("runtime-oracle: digest copied fixture %s: %w", rel, err)
		}
		digests = append(digests, FixtureDigest{Path: rel, Digest: digest})
	}
	return digests, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
