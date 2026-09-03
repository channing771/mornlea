package chunk

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/internal/storage/region"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

type compactChunkExpectation struct {
	key      core.ChunkKey
	revision uint64
	hash     [32]byte
}

func TestRegionCompactReplacesFragmentedFileWithoutChangingChunks(t *testing.T) {
	r, path, key, expected := seededFragmentedRegion(t)
	defer r.Close()

	policy := region.SpacePolicy{WasteRatio: 0.20, MinWaste: region.SectorSize}
	if !r.ShouldCompact(policy) {
		t.Fatal("fragmented region did not meet compaction policy")
	}
	if r.ShouldCompact(region.SpacePolicy{WasteRatio: 0.51, MinWaste: region.SectorSize}) {
		t.Fatal("region compacted below the waste-ratio threshold")
	}
	if r.ShouldCompact(region.SpacePolicy{WasteRatio: 0.20, MinWaste: 3 * region.SectorSize}) {
		t.Fatal("region compacted below the minimum-waste threshold")
	}

	wantGeneration := r.bank.Generation
	wantSize := compactRegionSize(r.bank)
	if err := r.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.activeBank != 0 || r.bank.Generation != wantGeneration {
		t.Fatalf(
			"compacted bank = %d generation %d, want bank A generation %d",
			r.activeBank, r.bank.Generation, wantGeneration,
		)
	}
	assertCompactChunks(t, r, expected)
	info, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != wantSize {
		t.Fatalf("compacted size = %d, want %d", info.Size(), wantSize)
	}
	assertNoRegionCompactTemps(t, path, "")

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertCompactChunks(t, reopened, expected)
}

func TestRegionCompactFailureReopensCompleteCanonical(t *testing.T) {
	tests := []struct {
		name          string
		install       func(*Region, error)
		wantCompacted bool
	}{
		{
			name: "before temp sync",
			install: func(r *Region, injected error) {
				r.compactionHooks.BeforeTempSync = func() error { return injected }
			},
		},
		{
			name: "rename",
			install: func(r *Region, injected error) {
				r.compactionHooks.Rename = func(string, string) error { return injected }
			},
		},
		{
			name: "directory sync",
			install: func(r *Region, injected error) {
				r.compactionHooks.SyncDirectory = func(string) error { return injected }
			},
			wantCompacted: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, path, key, expected := seededFragmentedRegion(t)
			defer r.Close()
			before, err := r.file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			compactedSize := compactRegionSize(r.bank)
			bystander := filepath.Join(
				filepath.Dir(path), "."+filepath.Base(path)+".compact-keep",
			)
			if err := os.WriteFile(bystander, []byte("bystander"), 0o600); err != nil {
				t.Fatal(err)
			}

			injected := errors.New("injected " + tc.name + " failure")
			tc.install(r, injected)
			if err := r.Compact(context.Background()); !errors.Is(err, injected) {
				t.Fatalf("compact error = %v, want injected failure", err)
			}

			// The in-memory region must be usable even when compact closed the old
			// descriptor before the injected replacement failure.
			assertCompactChunks(t, r, expected)
			info, err := r.file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			wantSize := before.Size()
			if tc.wantCompacted {
				wantSize = compactedSize
			}
			if info.Size() != wantSize {
				t.Fatalf("canonical size after failure = %d, want %d", info.Size(), wantSize)
			}
			assertNoRegionCompactTemps(t, path, bystander)

			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenRegion(context.Background(), path, key)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			assertCompactChunks(t, reopened, expected)
		})
	}
}

type closeObservedRegionFile struct {
	File
	closed bool
}

func (file *closeObservedRegionFile) Close() error {
	err := file.File.Close()
	file.closed = true
	return err
}

func TestRegionCompactClosesCanonicalBeforeRename(t *testing.T) {
	seeded, path, key, expected := seededFragmentedRegion(t)
	if err := seeded.Close(); err != nil {
		t.Fatal(err)
	}

	var old *closeObservedRegionFile
	r, err := openRegionWithHooks(
		context.Background(), path, key,
		regionFileHooks{Open: func(name string, flag int, mode fs.FileMode) (File, error) {
			file, err := os.OpenFile(name, flag, mode)
			if err != nil {
				return nil, err
			}
			observed := &closeObservedRegionFile{File: file}
			if old == nil {
				old = observed
			}
			return observed, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.compactionHooks.Rename = func(temporary, canonical string) error {
		if !old.closed {
			return errors.New("canonical descriptor remained open during rename")
		}
		return os.Rename(temporary, canonical)
	}

	if err := r.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCompactChunks(t, r, expected)
}

func seededFragmentedRegion(
	t *testing.T,
) (*Region, string, region.RegionKey, []compactChunkExpectation) {
	t.Helper()
	key := region.RegionKey{Dimension: core.Overworld, X: 0, Z: 0}
	path := filepath.Join(t.TempDir(), "r.0.0.region")
	r, err := CreateRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	keys := []core.ChunkKey{
		{Dimension: core.Overworld, Pos: core.ChunkPos{X: 1, Z: 1}},
		{Dimension: core.Overworld, Pos: core.ChunkPos{X: 2, Z: 2}},
	}
	for revision := uint64(1); revision <= 3; revision++ {
		saves := make([]ChunkSave, 0, len(keys))
		for index, chunkKey := range keys {
			chunk := world.NewChunk(chunkKey.Pos)
			block := core.StoneID
			if (revision+uint64(index))%2 == 0 {
				block = core.DirtID
			}
			chunk.SetBlock(index+1, 0, index+1, block)
			saves = append(saves, ChunkSave{Key: chunkKey, Revision: revision, Chunk: chunk})
		}
		if _, err := r.Save(context.Background(), saves); err != nil {
			r.Close()
			t.Fatal(err)
		}
	}

	expected := make([]compactChunkExpectation, 0, len(keys))
	for _, chunkKey := range keys {
		stored, err := r.Load(context.Background(), chunkKey)
		if err != nil {
			r.Close()
			t.Fatal(err)
		}
		expected = append(expected, compactChunkExpectation{
			key: chunkKey, revision: stored.PersistedRevision, hash: stored.Chunk.Hash(),
		})
	}
	return r, path, key, expected
}

func compactRegionSize(bank region.Bank) int64 {
	size := int64(region.DataStartSector * region.SectorSize)
	for _, entry := range bank.Entries {
		size += int64(entry.SectorCount) * region.SectorSize
	}
	return size
}

func assertCompactChunks(t *testing.T, r *Region, expected []compactChunkExpectation) {
	t.Helper()
	for _, want := range expected {
		got, err := r.Load(context.Background(), want.key)
		if err != nil {
			t.Fatal(err)
		}
		if got.Revision != want.revision || got.PersistedRevision != want.revision ||
			got.Chunk.Hash() != want.hash {
			t.Fatalf("chunk %v after compact = %+v hash=%x, want revision %d hash=%x",
				want.key, got, got.Chunk.Hash(), want.revision, want.hash)
		}
	}
}

func assertNoRegionCompactTemps(t *testing.T, path, bystander string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(
		filepath.Dir(path), "."+filepath.Base(path)+".compact-*",
	))
	if err != nil {
		t.Fatal(err)
	}
	if bystander == "" {
		if len(matches) != 0 {
			t.Fatalf("temporary compact files remain: %v", matches)
		}
		return
	}
	if len(matches) != 1 || matches[0] != bystander {
		t.Fatalf("compact temps = %v, want only bystander %q", matches, bystander)
	}
	got, err := os.ReadFile(bystander)
	if err != nil {
		t.Fatalf("read bystander: %v", err)
	}
	if string(got) != "bystander" {
		t.Fatalf("bystander contents = %q", got)
	}
}
