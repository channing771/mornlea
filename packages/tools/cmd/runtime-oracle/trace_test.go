package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTraceIdentity(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)

	t.Run("negative table", func(t *testing.T) {
		// 1. schedule [1,2,3] with only tick999
		t.Run("schedule [1,2,3] with only tick999", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.TickSchedule = []uint64{1, 2, 3}
			base.Inputs[0].Tick = 999
			base.Observations[0].Tick = 999
			err := ValidateTrace(base, manifest)
			if err == nil {
				t.Fatal("expected failure for schedule [1,2,3] with only tick999")
			}
		})

		// 2. duplicate tick2
		t.Run("duplicate tick2", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.TickSchedule = []uint64{0, 2, 2, 3}
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "duplicate tick") {
				t.Fatalf("expected duplicate tick error, got: %v", err)
			}
		})

		// 3. reversed schedule
		t.Run("reversed schedule", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.TickSchedule = []uint64{3, 2, 1}
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "strictly increasing") {
				t.Fatalf("expected strictly increasing error, got: %v", err)
			}
		})

		// 4. unknown case
		t.Run("unknown case", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.Inputs[0].CaseID = "nonexistent.family/1/fake"
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "unknown case") {
				t.Fatalf("expected unknown case error, got: %v", err)
			}
		})

		// 5. missing second input
		t.Run("missing second input", func(t *testing.T) {
			m2 := manifestWithTwoCases(manifest)
			base := validTraceForManifest(m2)
			base.Inputs = base.Inputs[:1] // drop second input
			err := ValidateTrace(base, m2)
			if err == nil || !strings.Contains(err.Error(), "inputs") {
				t.Fatalf("expected input count error, got: %v", err)
			}
		})

		// 6. duplicate index0
		t.Run("duplicate index0", func(t *testing.T) {
			m2 := manifestWithTwoCases(manifest)
			base := validTraceForManifest(m2)
			base.Inputs[1].Index = 0
			err := ValidateTrace(base, m2)
			if err == nil || !strings.Contains(err.Error(), "duplicate input index") && !strings.Contains(err.Error(), "non-contiguous") {
				t.Fatalf("expected duplicate index error, got: %v", err)
			}
		})

		// 7. fake nonempty digest
		t.Run("fake nonempty digest", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.CorpusDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "corpus digest") {
				t.Fatalf("expected corpus digest mismatch error, got: %v", err)
			}
		})

		// 8. source SHA mismatch
		t.Run("source SHA mismatch", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.SourceRevision = "ffffffffffffffffffffffffffffffffffffffff"
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "source revision") {
				t.Fatalf("expected source revision mismatch error, got: %v", err)
			}
		})

		// 9. one changed normalized scalar
		t.Run("one changed normalized scalar", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.Observations[0].Outcome.Fields["packet_id"] = json.Number("1")
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "outcome does not match") {
				t.Fatalf("expected outcome mismatch error, got: %v", err)
			}
		})

		// 10. empty selection
		t.Run("empty selection", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.SelectedCases = []string{}
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "empty selection") {
				t.Fatalf("expected empty selection error, got: %v", err)
			}
		})

		// 11. schema1
		t.Run("schema1", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			base.SchemaVersion = 1
			err := ValidateTrace(base, manifest)
			if err == nil || !strings.Contains(err.Error(), "schema_version") {
				t.Fatalf("expected schema version error, got: %v", err)
			}
		})

		// 12. missing observation
		t.Run("missing observation", func(t *testing.T) {
			m2 := manifestWithTwoCases(manifest)
			base := validTraceForManifest(m2)
			base.Observations = base.Observations[:1]
			err := ValidateTrace(base, m2)
			if err == nil || !strings.Contains(err.Error(), "observations") {
				t.Fatalf("expected missing observation error, got: %v", err)
			}
		})

		// 13. subset trace labeling itself complete
		t.Run("subset labeling itself complete", func(t *testing.T) {
			m2 := manifestWithTwoCases(manifest)
			base := validTraceForManifest(m2)
			base.SelectedCases = []string{m2.Cases[0].ID}
			base.Coverage = "complete"
			base.TickSchedule = []uint64{0}
			base.Inputs = base.Inputs[:1]
			base.Observations = base.Observations[:1]
			err := ValidateTrace(base, m2)
			if err == nil || !strings.Contains(err.Error(), "cannot label itself complete") {
				t.Fatalf("expected subset cannot label complete error, got: %v", err)
			}
		})
	})

	t.Run("positive table", func(t *testing.T) {
		// 1. one real framing case at0
		t.Run("one real framing case at0", func(t *testing.T) {
			base := validTraceForManifest(manifest)
			if err := ValidateTrace(base, manifest); err != nil {
				t.Fatalf("expected valid trace to pass, got: %v", err)
			}
		})

		// 2. two declared cases at1/2 with exact observations
		t.Run("two declared cases at1/2 with exact observations", func(t *testing.T) {
			m2 := manifestWithTwoCasesAt1And2(manifest)
			base := validTraceForManifestAt1And2(m2)
			if err := ValidateTrace(base, m2); err != nil {
				t.Fatalf("expected two declared cases at1/2 to pass, got: %v", err)
			}

			// verify same inputs repeated produce identical JSON
			json1, err1 := json.Marshal(base)
			if err1 != nil {
				t.Fatalf("marshal 1: %v", err1)
			}
			json2, err2 := json.Marshal(base)
			if err2 != nil {
				t.Fatalf("marshal 2: %v", err2)
			}
			if string(json1) != string(json2) {
				t.Fatalf("non-deterministic json:\n%s\nvs\n%s", json1, json2)
			}
		})
	})
}

func TestTraceRunIsDeterministicAndIsolated(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)
	revision := manifest.SourceRevision

	first, err := RunTrace(TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		SourceRevision: revision,
		Seed:           "1",
		TickSchedule:   []uint64{0},
	})
	if err != nil {
		t.Fatalf("RunTrace 1: %v", err)
	}

	second, err := RunTrace(TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		SourceRevision: revision,
		Seed:           "1",
		TickSchedule:   []uint64{0},
	})
	if err != nil {
		t.Fatalf("RunTrace 2: %v", err)
	}

	if first.CorpusDigest == "" || first.CorpusDigest != second.CorpusDigest {
		t.Fatalf("corpus digest drifted: %q vs %q", first.CorpusDigest, second.CorpusDigest)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("trace bytes drifted\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
	if err := ValidateTrace(first, manifest); err != nil {
		t.Fatalf("complete trace rejected: %v", err)
	}
}

func TestTraceRejectsIncompleteIdentity(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)

	_, err := RunTrace(TraceRequest{
		Root:         root,
		WorkDir:      t.TempDir(),
		TickSchedule: []uint64{0},
		Seed:         "1",
	})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("empty source revision: error=%v", err)
	}

	trace := validTraceForManifest(manifest)
	trace.Identities.Protocol = 0
	trace.Identities.AgentHTTP = ""
	err = ValidateTrace(trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete identity: error=%v", err)
	}
}

func TestTraceRejectsLivePathWrites(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)
	liveWork := filepath.Join(root, "worlds", "oracle-live-work")
	_, err := RunTrace(TraceRequest{
		Root:           root,
		WorkDir:        liveWork,
		SourceRevision: manifest.SourceRevision,
		Seed:           "1",
		TickSchedule:   []uint64{0},
	})
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("live work dir: error=%v", err)
	}
	if _, statErr := os.Stat(liveWork); !os.IsNotExist(statErr) {
		t.Fatalf("live work dir was created: %v", statErr)
	}

	output := filepath.Join(root, "packages", "server", "storage", "chunk", "testdata", "chunk-v9.bin")
	_, err = RunTrace(TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		OutputPath:     output,
		SourceRevision: manifest.SourceRevision,
		Seed:           "1",
		TickSchedule:   []uint64{0},
	})
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("live output path: error=%v", err)
	}
}

func TestTraceRejectsMalformedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)

	cases := []struct {
		name string
		data string
	}{
		{name: "truncated", data: "{"},
		{name: "array", data: "[]"},
		{name: "future schema", data: `{"schema_version": 99, "source_revision": "x"}`},
		{name: "duplicate key", data: `{"schema_version": 2, "schema_version": 2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.json")
			if err := os.WriteFile(path, []byte(tc.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadTrace(path, manifest)
			if err == nil {
				t.Fatal("malformed trace was accepted")
			}
		})
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func loadRealManifest(t *testing.T, root string) Inventory {
	t.Helper()
	manifestPath := filepath.Join(root, filepath.FromSlash(InventoryRelPath))
	manifest, err := LoadInventory(manifestPath)
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}
	return manifest
}

func validTraceForManifest(manifest Inventory) Trace {
	digest, _ := CanonicalCorpusDigest(manifest)
	schedule := []uint64{0}
	inputs := make([]TraceInput, 0)
	observations := make([]Observation, 0)
	idx := uint64(0)
	for _, c := range manifest.Cases {
		for _, cpStr := range c.Checkpoints {
			tick, _ := strconv.ParseUint(cpStr, 10, 64)
			inputs = append(inputs, TraceInput{
				Index:       idx,
				CaseID:      c.ID,
				Tick:        tick,
				InputDigest: c.Input.SHA256,
			})
			var outcome Outcome
			if c.ID == "protocol.frame/45/valid" {
				outcome = Outcome{
					Kind:     "ok",
					Category: "frame",
					Fields: map[string]any{
						"packet_id":   json.Number("0"),
						"payload":     "2d",
						"payload_len": json.Number("1"),
					},
				}
			}
			observations = append(observations, Observation{
				Tick:           tick,
				CaseID:         c.ID,
				Outcome:        outcome,
				ExpectedDigest: c.Expected.SHA256,
			})
			idx++
		}
	}
	return Trace{
		SchemaVersion:  2,
		SourceRevision: manifest.SourceRevision,
		CorpusDigest:   digest,
		Identities:     manifest.Identities,
		Seed:           "1",
		TickSchedule:   schedule,
		Inputs:         inputs,
		Observations:   observations,
	}
}

func manifestWithTwoCases(manifest Inventory) Inventory {
	clone := manifest
	clone.Cases = make([]CaseSpec, len(manifest.Cases))
	copy(clone.Cases, manifest.Cases)
	c2 := clone.Cases[0]
	c2.ID = "protocol.frame/45/valid-second"
	clone.Cases = append(clone.Cases, c2)
	return clone
}

func manifestWithTwoCasesAt1And2(manifest Inventory) Inventory {
	clone := manifest
	c1 := manifest.Cases[0]
	c1.ID = "protocol.frame/45/case-one"
	c1.Checkpoints = []string{"1"}

	c2 := manifest.Cases[0]
	c2.ID = "protocol.frame/45/case-two"
	c2.Checkpoints = []string{"2"}

	clone.Cases = []CaseSpec{c1, c2}
	return clone
}

func validTraceForManifestAt1And2(manifest Inventory) Trace {
	digest, _ := CanonicalCorpusDigest(manifest)
	schedule := []uint64{1, 2}
	inputs := []TraceInput{
		{
			Index:       0,
			CaseID:      manifest.Cases[0].ID,
			Tick:        1,
			InputDigest: manifest.Cases[0].Input.SHA256,
		},
		{
			Index:       1,
			CaseID:      manifest.Cases[1].ID,
			Tick:        2,
			InputDigest: manifest.Cases[1].Input.SHA256,
		},
	}
	outcome := Outcome{
		Kind:     "ok",
		Category: "frame",
		Fields: map[string]any{
			"packet_id":   json.Number("0"),
			"payload":     "2d",
			"payload_len": json.Number("1"),
		},
	}
	observations := []Observation{
		{
			Tick:           1,
			CaseID:         manifest.Cases[0].ID,
			Outcome:        outcome,
			ExpectedDigest: manifest.Cases[0].Expected.SHA256,
		},
		{
			Tick:           2,
			CaseID:         manifest.Cases[1].ID,
			Outcome:        outcome,
			ExpectedDigest: manifest.Cases[1].Expected.SHA256,
		},
	}
	return Trace{
		SchemaVersion:  2,
		SourceRevision: manifest.SourceRevision,
		CorpusDigest:   digest,
		Identities:     manifest.Identities,
		Seed:           "1",
		TickSchedule:   schedule,
		Inputs:         inputs,
		Observations:   observations,
	}
}

var _ = reflect.DeepEqual
var _ = fmt.Sprintf
