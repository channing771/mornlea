package storage

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage/passive"
	"github.com/channing771/mornlea/packages/shared/core"
)

// fixturePassiveRecords/fixturePassiveRecordsSorted 是 passive 包
// passive_codec_test.go 同名夹具的同构副本：根包 store 测试不能
// 导入子包测试夹具（测试文件不跨包可见），而断言依赖同一份非初值取值承重，
// 故按域持副本，改动任一侧取值时必须同步另一侧。

// fixturePassiveRecords 返回三条字段各异的合法记录。顺序刻意逆序：编码端
// 必须按 ID 升序写出，磁盘形态与调用方传入顺序解耦（hostile 先例）。
func fixturePassiveRecords() []StoredPassiveMob {
	grazer := StoredPassiveMob{
		ID: 0x8000000000000002, Dimension: core.Overworld,
		Position: [3]float32{-12.5, 70.25, 3.5}, Velocity: [3]float32{-1.25, 0, 0.5},
		OnGround: true, Yaw: 1.25, Health: 17,
	}
	idle := StoredPassiveMob{
		ID: 0x4000000000000001, Dimension: core.Overworld,
		Position: [3]float32{0.5, 64, -9.75}, Velocity: [3]float32{0, -3.25, 0},
		OnGround: false, Yaw: -2.5, Health: 20,
	}
	calf := StoredPassiveMob{
		ID: 1, Dimension: core.Overworld,
		Position: [3]float32{8.5, 65.5, 9.75}, Velocity: [3]float32{2, 0, -2},
		OnGround: true, Yaw: 3, Health: 1,
	}
	return []StoredPassiveMob{grazer, idle, calf}
}

func fixturePassiveRecordsSorted() []StoredPassiveMob {
	sorted := slices.Clone(fixturePassiveRecords())
	slices.SortFunc(sorted, func(a, b StoredPassiveMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return sorted
}

type closeablePassiveStore interface {
	PassiveMobStore
	Close() error
}

func TestPassiveStoreContract(t *testing.T) {
	implementations := []struct {
		name            string
		open            func(*testing.T) closeablePassiveStore
		closedAPIErrors bool
	}{
		{
			name: "memory",
			open: func(*testing.T) closeablePassiveStore {
				return NewMemory(Metadata{FormatVersion: currentMetadataVersion, Seed: 42})
			},
		},
		{
			name: "disk",
			open: func(t *testing.T) closeablePassiveStore {
				return openPassiveDisk(t, t.TempDir())
			},
			closedAPIErrors: true,
		},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			testPassiveStoreContract(t, implementation.open, implementation.closedAPIErrors)
		})
	}
}

func testPassiveStoreContract(
	t *testing.T,
	open func(*testing.T) closeablePassiveStore,
	closedAPIErrors bool,
) {
	t.Helper()

	t.Run("missing wraps ErrPassiveMobsNotFound", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		if _, err := store.LoadPassiveMobs(context.Background()); !errors.Is(err, ErrPassiveMobsNotFound) {
			t.Fatalf("missing LoadPassiveMobs error=%v，想要 ErrPassiveMobsNotFound", err)
		}
	})

	t.Run("round trip is canonical and does not alias", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		input := fixturePassiveRecords()
		want := fixturePassiveRecordsSorted()
		if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
			Revision: 7, Records: input,
		}); err != nil {
			t.Fatal(err)
		}
		input[0].Position[0] = 999
		input[0].Health = 3

		loaded, err := store.LoadPassiveMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Revision != 7 || !reflect.DeepEqual(loaded.Records, want) {
			t.Fatalf("loaded=%+v，想要 revision=7 records=%+v", loaded, want)
		}
		loaded.Records[0].Velocity[2] = 998
		again, err := store.LoadPassiveMobs(context.Background())
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
		first := PassiveMobsSave{Revision: 5, Records: fixturePassiveRecords()}
		if err := store.SavePassiveMobs(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		idempotent := PassiveMobsSave{Revision: 5, Records: fixturePassiveRecords()}
		slices.Reverse(idempotent.Records)
		if err := store.SavePassiveMobs(context.Background(), idempotent); err != nil {
			t.Fatalf("idempotent SavePassiveMobs error=%v", err)
		}
		conflict := PassiveMobsSave{Revision: 5, Records: fixturePassiveRecords()[:1]}
		if err := store.SavePassiveMobs(context.Background(), conflict); !errors.Is(err, ErrRevisionConflict) {
			t.Fatalf("same-revision SavePassiveMobs error=%v，想要 ErrRevisionConflict", err)
		}
		lower := PassiveMobsSave{Revision: 4, Records: fixturePassiveRecords()}
		if err := store.SavePassiveMobs(context.Background(), lower); !errors.Is(err, ErrRevisionConflict) {
			t.Fatalf("lower-revision SavePassiveMobs error=%v，想要 ErrRevisionConflict", err)
		}
		got, err := store.LoadPassiveMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := fixturePassiveRecordsSorted()
		if got.Revision != 5 || !reflect.DeepEqual(got.Records, want) {
			t.Fatalf("after conflicts loaded=%+v，想要保持 revision=5 records=%+v", got, want)
		}

		higher := PassiveMobsSave{Revision: 6, Records: fixturePassiveRecords()[:1]}
		if err := store.SavePassiveMobs(context.Background(), higher); err != nil {
			t.Fatal(err)
		}
		got, err = store.LoadPassiveMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// fixturePassiveRecords()[:1] 只有 grazer 一条，编码后无需重排，
		// 恢复值应与其完全一致。
		if got.Revision != 6 || !reflect.DeepEqual(got.Records, fixturePassiveRecords()[:1]) {
			t.Fatalf("higher revision loaded=%+v，想要 grazer 单条记录", got)
		}
	})

	t.Run("canceled operations do not mutate", func(t *testing.T) {
		store := open(t)
		t.Cleanup(func() { _ = store.Close() })
		first := PassiveMobsSave{Revision: 1, Records: fixturePassiveRecords()}
		if err := store.SavePassiveMobs(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.LoadPassiveMobs(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled LoadPassiveMobs error=%v，想要 context.Canceled", err)
		}
		higher := PassiveMobsSave{Revision: 2, Records: fixturePassiveRecords()[:1]}
		if err := store.SavePassiveMobs(ctx, higher); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled SavePassiveMobs error=%v，想要 context.Canceled", err)
		}
		got, err := store.LoadPassiveMobs(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := fixturePassiveRecordsSorted()
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
		if _, err := store.LoadPassiveMobs(context.Background()); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("LoadPassiveMobs after Close error=%v，想要 os.ErrClosed", err)
		}
		if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
			Revision: 1, Records: fixturePassiveRecords(),
		}); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("SavePassiveMobs after Close error=%v，想要 os.ErrClosed", err)
		}
	})
}

func TestDiskPassiveSaveWritesOwnerOnlyFile(t *testing.T) {
	root := t.TempDir()
	store := openPassiveDisk(t, root)
	if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
		Revision: 3, Records: fixturePassiveRecords(),
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "passive_mobs.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("passive_mobs.bin 权限=%v，想要 0600", got)
	}
}

func TestDiskPassiveSaveDoesNotOverwriteCorruptOrFutureFile(t *testing.T) {
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
				binary.LittleEndian.PutUint32(encoded[8:12], passive.CurrentSchema+1)
			},
			wantErr: ErrFutureVersion,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := openPassiveDisk(t, root)
			path := filepath.Join(root, "passive_mobs.bin")
			before, err := passive.Encode(PassiveMobsSave{
				Revision: 1, Records: fixturePassiveRecords(),
			})
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(before)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
				Revision: 2, Records: fixturePassiveRecords()[:1],
			}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("SavePassiveMobs error=%v，想要 %v", err, tc.wantErr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("SavePassiveMobs 覆写了非法正式文件")
			}
		})
	}
}

func TestDiskPassiveOversizedFileIsCorruptAndSaveDoesNotOverwriteIt(t *testing.T) {
	root := t.TempDir()
	store := openPassiveDisk(t, root)
	path := filepath.Join(root, "passive_mobs.bin")
	oversized := bytes.Repeat([]byte{0x5a}, passive.MaxFileLength+1)
	if err := os.WriteFile(path, oversized, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadPassiveMobs(context.Background()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("LoadPassiveMobs error=%v，想要 ErrCorrupt", err)
	}
	if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
		Revision: 2, Records: fixturePassiveRecords(),
	}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("SavePassiveMobs error=%v，想要 ErrCorrupt", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, oversized) {
		t.Fatal("SavePassiveMobs 覆写了超限正式文件")
	}
}

func TestDiskPassiveAtomicReplaceKeepsOldFileOnFailure(t *testing.T) {
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
			store := openPassiveDisk(t, root)
			if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
				Revision: 1, Records: fixturePassiveRecords(),
			}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, "passive_mobs.bin"))
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + tc.name)
			store.passiveReplaceHooks = tc.configure(injected)
			if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
				Revision: 2, Records: fixturePassiveRecords()[:1],
			}); !errors.Is(err, injected) {
				t.Fatalf("SavePassiveMobs error=%v，想要 injected error", err)
			}
			after, err := os.ReadFile(filepath.Join(root, "passive_mobs.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("失败的替换改写了旧正式文件")
			}
			matches, err := filepath.Glob(filepath.Join(root, ".passive_mobs.bin.tmp-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("failed replace leaked temporary files: %v", matches)
			}
		})
	}
}

func TestDiskPassiveAtomicReplaceReportsParentSyncFailureAfterPublish(t *testing.T) {
	root := t.TempDir()
	store := openPassiveDisk(t, root)
	if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
		Revision: 1, Records: fixturePassiveRecords(),
	}); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected directory sync")
	store.passiveReplaceHooks = atomicReplaceHooks{
		openDirectory: func(path string) (metadataDirectory, error) {
			directory, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			return &playerFaultDirectory{File: directory, syncErr: injected}, nil
		},
	}
	if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
		Revision: 2, Records: fixturePassiveRecords()[:1],
	}); !errors.Is(err, injected) {
		t.Fatalf("SavePassiveMobs error=%v，想要 parent Sync error", err)
	}
	got, err := store.LoadPassiveMobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || !reflect.DeepEqual(got.Records, fixturePassiveRecords()[:1]) {
		t.Fatalf("parent sync 失败后 loaded=%+v，想要新值已发布（rename 已发生）", got)
	}
}

func TestWorldBackupIncludesPassiveFileButSkipsTemporaryFiles(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)
	if err := store.SavePassiveMobs(context.Background(), PassiveMobsSave{
		Revision: 1, Records: fixturePassiveRecords(),
	}); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(source, ".passive_mobs.bin.tmp-ignore")
	if err := os.WriteFile(temporary, []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	assertSameFileContents(
		t, filepath.Join(source, "passive_mobs.bin"), filepath.Join(destination, "passive_mobs.bin"),
	)
	if _, err := os.Lstat(filepath.Join(destination, filepath.Base(temporary))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("备份不应包含被动牛临时文件，Lstat 错误: %v", err)
	}
}

func openPassiveDisk(t *testing.T, root string) *DiskStore {
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
