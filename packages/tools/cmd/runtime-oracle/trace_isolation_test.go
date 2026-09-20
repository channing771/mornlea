package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeRepository builds a throwaway repository-shaped directory so the path and
// export cases never touch the real tree. It carries the one marker
// RepositoryRoot looks for plus a sentinel the assertions can inspect.
func fakeRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sentinel.txt"), []byte("sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// treeSnapshot records every path below dir so a failure path can be proven to
// have left the tree untouched.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		snapshot[rel] = info.Mode().String()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// traceFixture returns one trace that satisfies the frozen manifest, so export
// cases fail for the reason under test instead of for an invalid payload.
func traceFixture(t *testing.T) (Trace, Inventory) {
	t.Helper()
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)
	trace := validTraceForManifest(manifest)
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("fixture trace must validate: %v", err)
	}
	return trace, manifest
}

func TestTraceIsolationWorkspaceIsExclusivelyOwnedOutsideRepository(t *testing.T) {
	root := mustRepoRoot(t)
	dir, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	if cleanup == nil {
		t.Fatal("NewTraceWorkspace returned no cleanup")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("stat workspace: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workspace %s is not a directory", dir)
	}
	if live, err := isLivePath(root, dir); err != nil {
		cleanup()
		t.Fatal(err)
	} else if live {
		cleanup()
		t.Fatalf("workspace %s resolves inside the repository", dir)
	}

	// The harness owns the directory, so it must be writable by the run.
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write into workspace: %v", err)
	}

	cleanup()
	if _, statErr := os.Lstat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("cleanup left the workspace behind: %v", statErr)
	}
	// Cleanup is idempotent, matching the idempotent close discipline used
	// elsewhere in the repository, and must not resurrect any content.
	cleanup()
	if _, statErr := os.Lstat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("cleanup left workspace content behind: %v", statErr)
	}
}

func TestTraceIsolationWorkspaceRejectsRepositoryResidentTemporaryDirectory(t *testing.T) {
	root := mustRepoRoot(t)
	inside := filepath.Join(root, "packages", "tools", "cmd", "runtime-oracle", ".tmp-workspace-probe")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(inside)
	t.Setenv("TMPDIR", inside)

	dir, cleanup, err := NewTraceWorkspace(root)
	if err == nil {
		cleanup()
		t.Fatalf("a workspace inside the repository was accepted: %s", dir)
	}
	if !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected live-path rejection, got: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("rejected workspace was left behind: %v", statErr)
	}
}

func TestTraceIsolationRunTraceLeavesNoWorkspaceBehind(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)
	// Pinning the temporary root makes the "no leftover workspace" assertion
	// deterministic instead of scanning a directory other runs may share.
	tempHome := t.TempDir()
	t.Setenv("TMPDIR", tempHome)

	if _, err := RunTrace(TraceRequest{
		Root:           root,
		SourceRevision: manifest.SourceRevision,
		Seed:           "1",
		TickSchedule:   []uint64{0},
	}); err != nil {
		t.Fatalf("RunTrace: %v", err)
	}

	entries, err := os.ReadDir(tempHome)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), traceWorkspacePrefix) {
			t.Fatalf("run left workspace %s behind", entry.Name())
		}
	}
}

func TestTracePathRejectsRepositorySymlinkAncestorWithMissingSuffix(t *testing.T) {
	repo := fakeRepository(t)
	before := treeSnapshot(t, repo)

	// The ancestor exists only as a symlink into the repository and the
	// published name itself does not exist yet, so containment has to be
	// judged on the resolved ancestor rather than on the lexical target.
	link := filepath.Join(filepath.Dir(repo), "repo-link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(link, "missing-a", "missing-b", "out.json")

	trace, manifest := traceFixture(t)
	err := ExportTrace(repo, target, trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected live-path rejection, got: %v", err)
	}
	if after := treeSnapshot(t, repo); !reflect.DeepEqual(before, after) {
		t.Fatalf("repository tree changed: before=%v after=%v", before, after)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "missing-a")); !os.IsNotExist(statErr) {
		t.Fatalf("missing directory was created inside the repository: %v", statErr)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("report was published: %v", statErr)
	}
}

func TestTracePathAllowsLexicalSiblingOutsideRepository(t *testing.T) {
	repo := fakeRepository(t)
	target := filepath.Join(filepath.Dir(repo), "sibling-reports", "out.json")

	trace, manifest := traceFixture(t)
	if err := ExportTrace(repo, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace to a lexical sibling: %v", err)
	}
	if _, err := LoadTrace(target, manifest); err != nil {
		t.Fatalf("LoadTrace published report: %v", err)
	}
}

func TestTracePathAllowsRepositorySiblingNamedDotDotCache(t *testing.T) {
	repo := fakeRepository(t)
	// A directory whose name starts with ".." is a sibling of the repository
	// rather than a component inside it, so the containment rule must not read
	// the ".." prefix as an escape from the repository.
	target := filepath.Join(filepath.Dir(repo), "..cache", "out.json")

	trace, manifest := traceFixture(t)
	if err := ExportTrace(repo, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace to a sibling named ..cache: %v", err)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatalf("stat published report: %v", statErr)
	}
}

func TestTracePathRejectsDotDotCacheChildOfRepositoryRoot(t *testing.T) {
	repo := fakeRepository(t)
	// The same "..cache" name as a child of the root is inside the repository,
	// which is the case a plain ".." prefix check would miss.
	target := filepath.Join(repo, "..cache", "out.json")

	trace, manifest := traceFixture(t)
	err := ExportTrace(repo, target, trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected live-path rejection, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "..cache")); !os.IsNotExist(statErr) {
		t.Fatalf("directory was created inside the repository: %v", statErr)
	}
}

func TestTracePathRejectsSymlinkAncestorResolvingOutsideRepository(t *testing.T) {
	repo := fakeRepository(t)
	parent := filepath.Dir(repo)
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(link, "out.json")

	trace, manifest := traceFixture(t)
	err := ExportTrace(repo, target, trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got: %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("report was published through a symlink: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "out.json")); !os.IsNotExist(statErr) {
		t.Fatalf("report was published through the symlink destination: %v", statErr)
	}
}

func TestTraceOutputRefusesPreexistingTarget(t *testing.T) {
	repo := fakeRepository(t)
	target := filepath.Join(filepath.Dir(repo), "reports", "out.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	trace, manifest := traceFixture(t)
	err := ExportTrace(repo, target, trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected preexisting target rejection, got: %v", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "sentinel\n" {
		t.Fatalf("preexisting report was modified: %q", data)
	}
	entries, readErr := os.ReadDir(filepath.Dir(target))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("staged file was left behind: %v", entries)
	}
}

func TestTraceOutputPublishesAtomicallyAndLeavesNoStagedFile(t *testing.T) {
	repo := fakeRepository(t)
	target := filepath.Join(filepath.Dir(repo), "reports", "missing-a", "missing-b", "out.json")

	trace, manifest := traceFixture(t)
	if err := ExportTrace(repo, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTrace(target, manifest)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	loadedJSON, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	traceJSON, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if string(loadedJSON) != string(traceJSON) {
		t.Fatalf("published report drifted\nwant=%s\ngot=%s", traceJSON, loadedJSON)
	}

	// Publication is no-replace: a second export must fail and leave the
	// already published report intact.
	if err := ExportTrace(repo, target, trace, manifest); err == nil {
		t.Fatal("second export replaced an existing report")
	}
	entries, readErr := os.ReadDir(filepath.Dir(target))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 || entries[0].Name() != "out.json" {
		t.Fatalf("staged file was left behind: %v", entries)
	}
}

func TestTraceOutputRejectsInvalidTraceWithNonexistentPath(t *testing.T) {
	repo := fakeRepository(t)
	reports := filepath.Join(filepath.Dir(repo), "reports")
	target := filepath.Join(reports, "missing-a", "missing-b", "out.json")

	trace, manifest := traceFixture(t)
	trace.SchemaVersion = 1
	err := ExportTrace(repo, target, trace, manifest)
	if err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("expected validation failure, got: %v", err)
	}
	if _, statErr := os.Stat(reports); !os.IsNotExist(statErr) {
		t.Fatalf("directories were created for an invalid trace: %v", statErr)
	}
}

func TestTraceIORejectsSymlinkedSourceFixture(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := loadRealManifest(t, root)
	relative := manifest.Cases[0].Input.Path
	full := filepath.Join(root, filepath.FromSlash(relative))
	backup := full + ".isolation-backup"
	if err := os.Rename(full, backup); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		_ = os.Remove(full)
		_ = os.Rename(backup, full)
	}
	defer restore()
	if err := os.Symlink(backup, full); err != nil {
		t.Fatal(err)
	}

	_, err := RunTrace(TraceRequest{
		Root:           root,
		SourceRevision: manifest.SourceRevision,
		Seed:           "1",
		TickSchedule:   []uint64{0},
	})
	if err == nil || !strings.Contains(err.Error(), "symlink component") {
		t.Fatalf("expected symlinked fixture rejection, got: %v", err)
	}

	restore()
	info, statErr := os.Lstat(full)
	if statErr != nil {
		t.Fatalf("fixture was not restored: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("fixture is still a symlink")
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("restored fixture is not a regular file: %v", info.Mode())
	}
	if _, statErr := os.Stat(backup); !os.IsNotExist(statErr) {
		t.Fatalf("backup fixture was left behind: %v", statErr)
	}
}

func TestTraceIORejectsUnwritableReportDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial is not observable for the superuser")
	}
	repo := fakeRepository(t)
	reports := filepath.Join(filepath.Dir(repo), "reports")
	if err := os.Mkdir(reports, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(reports, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(reports, 0o755)

	target := filepath.Join(reports, "out.json")
	trace, manifest := traceFixture(t)
	err := ExportTrace(repo, target, trace, manifest)
	if err == nil {
		t.Fatal("export into an unwritable directory succeeded")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("report was published: %v", statErr)
	}
	entries, readErr := os.ReadDir(reports)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("staged file was left behind: %v", entries)
	}
}
