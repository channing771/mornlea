//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/internal/storage"
)

func TestApplicationStoreInteractiveUsesSelectedDiskWorld(t *testing.T) {
	worldPath := filepath.Join(t.TempDir(), "chosen-world")
	opened, err := openApplicationStore(context.Background(), Options{
		Seed:      42,
		WorldPath: worldPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := opened.(*storage.DiskStore); !ok {
		t.Fatalf("interactive store=%T，想要 *storage.DiskStore", opened)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worldPath, "world.meta")); err != nil {
		t.Fatalf("选择的世界没有 metadata: %v", err)
	}
}

func TestApplicationStoreBenchmarkUsesMemoryWithoutTouchingWorldPath(t *testing.T) {
	worldPath := filepath.Join(t.TempDir(), "must-not-exist")
	opened, err := openApplicationStore(context.Background(), Options{
		Seed:      BenchmarkSeed,
		Benchmark: true,
		WorldPath: worldPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := opened.(*storage.MemoryStore); !ok {
		t.Fatalf("benchmark store=%T，想要 *storage.MemoryStore", opened)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("benchmark 触碰了 WorldPath，Stat error=%v", err)
	}
}

func TestApplicationStoreCaptureUsesMemoryWithoutTouchingWorldPath(t *testing.T) {
	worldPath := filepath.Join(t.TempDir(), "must-not-exist")
	opened, err := openApplicationStore(context.Background(), Options{
		Seed:       42,
		CaptureDir: filepath.Join(t.TempDir(), "shots"),
		WorldPath:  worldPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := opened.(*storage.MemoryStore); !ok {
		t.Fatalf("capture store=%T，想要 *storage.MemoryStore", opened)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capture 触碰了 WorldPath，Stat error=%v", err)
	}
}

func TestApplicationConnectionRemoteNeverOpensLocalStore(t *testing.T) {
	worldPath := filepath.Join(t.TempDir(), "must-not-exist")
	opened, err := openApplicationStore(context.Background(), Options{
		Seed:      42,
		Connect:   "127.0.0.1:25565",
		WorldPath: worldPath,
	})
	if err != nil || opened != nil {
		t.Fatalf("remote store = (%T, %v), want (nil, nil)", opened, err)
	}
	if _, err := os.Stat(worldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remote touched WorldPath, Stat error=%v", err)
	}
}

func TestApplicationStoreExistingMetadataSeedWins(t *testing.T) {
	worldPath := filepath.Join(t.TempDir(), "existing-world")
	first, err := openApplicationStore(context.Background(), Options{
		Seed:      42,
		WorldPath: worldPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := openApplicationStore(context.Background(), Options{
		Seed:      99,
		WorldPath: worldPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	if got := reopened.Metadata().Seed; got != 42 {
		t.Fatalf("reopened metadata seed=%d，想要既有存档 seed 42", got)
	}
}
