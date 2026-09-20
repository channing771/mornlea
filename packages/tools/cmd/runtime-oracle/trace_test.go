package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceRunIsDeterministicAndIsolated(t *testing.T) {
	root := mustRepoRoot(t)
	sourceHashes := fixtureHashes(t, root)
	revision := "test-revision"
	first := runTrace(t, TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		SourceRevision: revision,
		Seed:           1,
		TickSchedule:   []uint64{0, 1},
	})
	second := runTrace(t, TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		SourceRevision: revision,
		Seed:           1,
		TickSchedule:   []uint64{0, 1},
	})
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
	if err := ValidateTrace(first); err != nil {
		t.Fatalf("complete trace rejected: %v", err)
	}
	after := fixtureHashes(t, root)
	for path, digest := range sourceHashes {
		if after[path] != digest {
			t.Fatalf("live fixture %s was mutated", path)
		}
	}
}

func TestTraceRejectsIncompleteIdentity(t *testing.T) {
	root := mustRepoRoot(t)
	_, err := RunTrace(TraceRequest{
		Root:         root,
		WorkDir:      t.TempDir(),
		TickSchedule: []uint64{0},
		Seed:         1,
	})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("empty source revision: error=%v", err)
	}

	trace := runTrace(t, TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		SourceRevision: "test-revision",
		Seed:           1,
		TickSchedule:   []uint64{0},
	})
	trace.Identities.Protocol = 0
	trace.Identities.AgentHTTP = ""
	err = ValidateTrace(trace)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete identity: error=%v", err)
	}
}

func TestTraceRejectsLivePathWrites(t *testing.T) {
	root := mustRepoRoot(t)
	liveWork := filepath.Join(root, "worlds", "oracle-live-work")
	_, err := RunTrace(TraceRequest{
		Root:           root,
		WorkDir:        liveWork,
		SourceRevision: "test-revision",
		Seed:           1,
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
		SourceRevision: "test-revision",
		Seed:           1,
		TickSchedule:   []uint64{0},
	})
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("live output path: error=%v", err)
	}
}

func TestTraceRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{name: "truncated", data: "{"},
		{name: "array", data: "[]"},
		{name: "future schema", data: `{"schema_version": 99, "source_revision": "x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.json")
			if err := os.WriteFile(path, []byte(tc.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadTrace(path)
			if err == nil {
				t.Fatal("malformed trace was accepted")
			}
		})
	}
}

func TestTraceRejectsIncompleteTraces(t *testing.T) {
	root := mustRepoRoot(t)
	trace := runTrace(t, TraceRequest{
		Root:           root,
		WorkDir:        t.TempDir(),
		SourceRevision: "test-revision",
		Seed:           1,
		TickSchedule:   []uint64{0},
	})
	trace.Observations = nil
	trace.CorpusDigest = ""
	trace.TickSchedule = nil
	err := ValidateTrace(trace)
	if err == nil {
		t.Fatal("incomplete trace was accepted")
	}
	text := err.Error()
	for _, want := range []string{"incomplete trace", "corpus digest", "tick schedule"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %v", want, err)
		}
	}
}

func runTrace(t *testing.T, request TraceRequest) Trace {
	t.Helper()
	trace, err := RunTrace(request)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	return trace
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func fixtureHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	inventory, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{}
	for _, family := range inventory.Families {
		for _, fixture := range family.Fixtures {
			path := filepath.Join(root, filepath.FromSlash(fixture))
			digest, err := fileDigest(path)
			if err != nil {
				t.Fatalf("hash %s: %v", fixture, err)
			}
			hashes[fixture] = digest
		}
	}
	return hashes
}
