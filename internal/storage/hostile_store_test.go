package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

type closeableHostileStore interface {
	HostileMobStore
	Close() error
}

func TestHostileStoreContract(t *testing.T) {
	implementations := []struct {
		name            string
		open            func(*testing.T) closeableHostileStore
		closedAPIErrors bool
	}{
		{
			name: "memory",
			open: func(*testing.T) closeableHostileStore {
				return NewMemory(Metadata{FormatVersion: currentMetadataVersion, Seed: 42})
			},
		},
		{
			name: "disk",
			open: func(t *testing.T) closeableHostileStore {
				return openHostileDisk(t, t.TempDir())
			},
			closedAPIErrors: true,
		},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			testHostileStoreContract(t, implementation.open, implementation.closedAPIErrors)
		})
	}
}

func testHostileStoreContract(
	t *testing.T,
	open func(*testing.T) closeableHostileStore,
	closedAPIErrors bool,
) {
	t.Helper()

	t.Run("missing wraps ErrHostileMobsNotFound", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		if _, err := store.LoadHostileMobs(context.Background()); !errors.Is(err, ErrHostileMobsNotFound) {
			t.Fatalf("missing LoadHostileMobs error=%v，想要 ErrHostileMobsNotFound", err)
		}
	})

	t.Run("round trip is canonical and does not alias", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		input := fixtureHostileRecords()
		want := fixtureHostileRecordsSorted()
		if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
			Revision: 7, Records: input,
		}); err != nil {
			t.Fatal(err)
		}
		input[0].Position[0] = 999
		input[0].Health = 3

		loaded, err := store.LoadHostileMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Revision != 7 || !reflect.DeepEqual(loaded.Records, want) {
			t.Fatalf("loaded=%+v，想要 revision=7 records=%+v", loaded, want)
		}
		loaded.Records[0].Velocity[2] = 998
		again, err := store.LoadHostileMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if again.Revision != 7 || !reflect.DeepEqual(again.Records, want) {
			t.Fatalf("second load=%+v，想要保持 %+v", again, want)
		}
	})

	t.Run("revision idempotency and conflicts preserve the value", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		first := HostileMobsSave{Revision: 5, Records: fixtureHostileRecords()}
		if err := store.SaveHostileMobs(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		idempotent := HostileMobsSave{Revision: 5, Records: fixtureHostileRecords()}
		slices.Reverse(idempotent.Records)
		if err := store.SaveHostileMobs(context.Background(), idempotent); err != nil {
			t.Fatalf("idempotent SaveHostileMobs error=%v", err)
		}
		conflict := HostileMobsSave{Revision: 5, Records: fixtureHostileRecords()[:1]}
		if err := store.SaveHostileMobs(context.Background(), conflict); !errors.Is(err, ErrRevisionConflict) {
			t.Fatalf("same-revision SaveHostileMobs error=%v，想要 ErrRevisionConflict", err)
		}
		lower := HostileMobsSave{Revision: 4, Records: fixtureHostileRecords()}
		if err := store.SaveHostileMobs(context.Background(), lower); !errors.Is(err, ErrRevisionConflict) {
			t.Fatalf("lower-revision SaveHostileMobs error=%v，想要 ErrRevisionConflict", err)
		}
		got, err := store.LoadHostileMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := fixtureHostileRecordsSorted()
		if got.Revision != 5 || !reflect.DeepEqual(got.Records, want) {
			t.Fatalf("after conflicts loaded=%+v，想要保持 revision=5 records=%+v", got, want)
		}

		higher := HostileMobsSave{Revision: 6, Records: fixtureHostileRecords()[:1]}
		if err := store.SaveHostileMobs(context.Background(), higher); err != nil {
			t.Fatal(err)
		}
		got, err = store.LoadHostileMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// fixtureHostileRecords()[:1] 只有 tracking 一条，编码后无需重排，
		// 恢复值应与其完全一致。
		if got.Revision != 6 || !reflect.DeepEqual(got.Records, fixtureHostileRecords()[:1]) {
			t.Fatalf("higher revision loaded=%+v，想要 tracking 单条记录", got)
		}
	})

	t.Run("canceled operations do not mutate", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		first := HostileMobsSave{Revision: 1, Records: fixtureHostileRecords()}
		if err := store.SaveHostileMobs(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.LoadHostileMobs(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled LoadHostileMobs error=%v，想要 context.Canceled", err)
		}
		higher := HostileMobsSave{Revision: 2, Records: fixtureHostileRecords()[:1]}
		if err := store.SaveHostileMobs(ctx, higher); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled SaveHostileMobs error=%v，想要 context.Canceled", err)
		}
		got, err := store.LoadHostileMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := fixtureHostileRecordsSorted()
		if got.Revision != 1 || !reflect.DeepEqual(got.Records, want) {
			t.Fatalf("after cancellation loaded=%+v，想要保持 revision=1 records=%+v", got, want)
		}
	})

	t.Run("close is idempotent", func(t *testing.T) {
		store := open(t)
		if err := store.Close(); err != nil {
			t.Fatalf("first Close=%v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("second Close=%v", err)
		}
		if !closedAPIErrors {
			return
		}
		if _, err := store.LoadHostileMobs(context.Background()); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("LoadHostileMobs after Close error=%v，想要 os.ErrClosed", err)
		}
		if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
			Revision: 1, Records: fixtureHostileRecords(),
		}); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("SaveHostileMobs after Close error=%v，想要 os.ErrClosed", err)
		}
	})
}

func TestDiskHostileSaveWritesOwnerOnlyFile(t *testing.T) {
	root := t.TempDir()
	store := openHostileDisk(t, root)
	if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
		Revision: 3, Records: fixtureHostileRecords(),
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "hostile_mobs.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("hostile_mobs.bin 权限=%v，想要 0600", got)
	}
}

func TestDiskHostileSaveDoesNotOverwriteCorruptOrFutureFile(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func([]byte)
		wantErr error
	}{
		{
			name:    "corrupt",
			mutate:  func(encoded []byte) { encoded[len(encoded)-1] ^= 0xff },
			wantErr: ErrCorrupt,
		},
		{
			name: "future",
			mutate: func(encoded []byte) {
				binary.LittleEndian.PutUint32(encoded[8:12], currentHostileSchema+1)
			},
			wantErr: ErrFutureVersion,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := openHostileDisk(t, root)
			path := filepath.Join(root, "hostile_mobs.bin")
			before, err := encodeHostileMobs(HostileMobsSave{
				Revision: 1, Records: fixtureHostileRecords(),
			})
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(before)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
				Revision: 2, Records: fixtureHostileRecords()[:1],
			}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("SaveHostileMobs error=%v，想要 %v", err, tc.wantErr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("SaveHostileMobs 覆写了非法正式文件")
			}
		})
	}
}

func TestDiskHostileOversizedFileIsCorruptAndSaveDoesNotOverwriteIt(t *testing.T) {
	root := t.TempDir()
	store := openHostileDisk(t, root)
	path := filepath.Join(root, "hostile_mobs.bin")
	oversized := bytes.Repeat([]byte{0x5a}, maxHostileFileLength+1)
	if err := os.WriteFile(path, oversized, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadHostileMobs(context.Background()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("LoadHostileMobs error=%v，想要 ErrCorrupt", err)
	}
	if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
		Revision: 2, Records: fixtureHostileRecords(),
	}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("SaveHostileMobs error=%v，想要 ErrCorrupt", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, oversized) {
		t.Fatal("SaveHostileMobs 覆写了超限正式文件")
	}
}

func TestDiskHostileAtomicReplaceKeepsOldFileOnFailure(t *testing.T) {
	tests := []struct {
		name      string
		configure func(error) atomicReplaceHooks
	}{
		{
			name: "temporary write",
			configure: func(injected error) atomicReplaceHooks {
				return atomicReplaceHooks{createTemp: playerFaultCreateTemp(injected, nil, nil)}
			},
		},
		{
			name: "temporary sync",
			configure: func(injected error) atomicReplaceHooks {
				return atomicReplaceHooks{createTemp: playerFaultCreateTemp(nil, injected, nil)}
			},
		},
		{
			name: "temporary close",
			configure: func(injected error) atomicReplaceHooks {
				return atomicReplaceHooks{createTemp: playerFaultCreateTemp(nil, nil, injected)}
			},
		},
		{
			name: "rename",
			configure: func(injected error) atomicReplaceHooks {
				return atomicReplaceHooks{rename: func(string, string) error { return injected }}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := openHostileDisk(t, root)
			if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
				Revision: 1, Records: fixtureHostileRecords(),
			}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, "hostile_mobs.bin"))
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + tc.name)
			store.hostileReplaceHooks = tc.configure(injected)
			if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
				Revision: 2, Records: fixtureHostileRecords()[:1],
			}); !errors.Is(err, injected) {
				t.Fatalf("SaveHostileMobs error=%v，想要 injected error", err)
			}
			after, err := os.ReadFile(filepath.Join(root, "hostile_mobs.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("失败的替换改写了旧正式文件")
			}
			matches, err := filepath.Glob(filepath.Join(root, ".hostile_mobs.bin.tmp-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("failed replace leaked temporary files: %v", matches)
			}
		})
	}
}

func TestDiskHostileAtomicReplaceReportsParentSyncFailureAfterPublish(t *testing.T) {
	root := t.TempDir()
	store := openHostileDisk(t, root)
	if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
		Revision: 1, Records: fixtureHostileRecords(),
	}); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected directory sync")
	store.hostileReplaceHooks = atomicReplaceHooks{
		openDirectory: func(path string) (metadataDirectory, error) {
			directory, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			return &playerFaultDirectory{File: directory, syncErr: injected}, nil
		},
	}
	if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
		Revision: 2, Records: fixtureHostileRecords()[:1],
	}); !errors.Is(err, injected) {
		t.Fatalf("SaveHostileMobs error=%v，想要 parent Sync error", err)
	}
	got, err := store.LoadHostileMobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || !reflect.DeepEqual(got.Records, fixtureHostileRecords()[:1]) {
		t.Fatalf("parent sync 失败后 loaded=%+v，想要新值已发布（rename 已发生）", got)
	}
}

func TestWorldBackupIncludesHostileFileButSkipsTemporaryFiles(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)
	if err := store.SaveHostileMobs(context.Background(), HostileMobsSave{
		Revision: 1, Records: fixtureHostileRecords(),
	}); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(source, ".hostile_mobs.bin.tmp-ignore")
	if err := os.WriteFile(temporary, []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	assertSameFileContents(
		t, filepath.Join(source, "hostile_mobs.bin"), filepath.Join(destination, "hostile_mobs.bin"),
	)
	if _, err := os.Lstat(filepath.Join(destination, filepath.Base(temporary))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("备份不应包含夜行者临时文件，Lstat 错误: %v", err)
	}
}

func openHostileDisk(t *testing.T, root string) *DiskStore {
	t.Helper()
	store, err := OpenDisk(context.Background(), root, OpenOptions{
		Create: Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return store
}
