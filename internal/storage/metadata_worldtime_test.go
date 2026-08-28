package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// encodeLegacyMetadataV1 手工构造一份 CRC 有效的 metadata v1 字节。
// 生产代码只写 v2，v1 golden 必须由测试自己保留。
func encodeLegacyMetadataV1(metadata Metadata) []byte {
	encoded := make([]byte, 0, metadataHeaderLength+legacyMetadataPayloadLength+metadataChecksumLength)
	encoded = append(encoded, metadataMagic[:]...)
	encoded = binary.LittleEndian.AppendUint32(encoded, legacyMetadataVersion)
	encoded = binary.LittleEndian.AppendUint32(encoded, legacyMetadataPayloadLength)
	encoded = binary.LittleEndian.AppendUint64(encoded, uint64(metadata.Seed))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnDimension))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.X))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.Z))
	return binary.LittleEndian.AppendUint32(
		encoded, crc32.Checksum(encoded, metadataCRCTable),
	)
}

// 本文件原有的 v2 golden、v1→v2 迁移与 v2 时代损坏矩阵已随版本升级由
// metadata_dayphase_test.go 的 TestMetadataV3GoldenBytes（当前布局）、
// TestMetadataV2LegacyGoldenBytes（冻结的 v2 旧档字节）、
// TestMetadataV1MigratesToV3WithZeroTimeAndOffset 与
// TestMetadataVersionsRejectMalformedBytes（覆盖三个版本的两两长度混淆）承接。

// failingReplaceFile 在指定阶段注入失败的临时文件。
type failingReplaceFile struct {
	*os.File
	writeErr error
	syncErr  error
}

func (file *failingReplaceFile) Write(p []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return file.File.Write(p)
}

func (file *failingReplaceFile) Sync() error {
	if file.syncErr != nil {
		return file.syncErr
	}
	return file.File.Sync()
}

func TestMetadataTempFailureKeepsPreviousFile(t *testing.T) {
	injected := errors.New("injected temp failure")
	tests := []struct {
		name  string
		hooks func() atomicReplaceHooks
	}{
		{
			name: "临时写入失败",
			hooks: func() atomicReplaceHooks {
				return atomicReplaceHooks{createTemp: func(dir, pattern string) (atomicReplaceFile, error) {
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return &failingReplaceFile{File: f, writeErr: injected}, nil
				}}
			},
		},
		{
			name: "临时文件 fsync 失败",
			hooks: func() atomicReplaceHooks {
				return atomicReplaceHooks{createTemp: func(dir, pattern string) (atomicReplaceFile, error) {
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return &failingReplaceFile{File: f, syncErr: injected}, nil
				}}
			},
		},
		{
			name: "rename 失败",
			hooks: func() atomicReplaceHooks {
				return atomicReplaceHooks{rename: func(string, string) error { return injected }}
			},
		},
	}

	previous, err := encodeMetadata(Metadata{
		FormatVersion: currentMetadataVersion, Seed: 1, WorldTimeTicks: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := encodeMetadata(Metadata{
		FormatVersion: currentMetadataVersion, Seed: 1, WorldTimeTicks: 99,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "world.meta")
			if err := os.WriteFile(path, previous, 0o600); err != nil {
				t.Fatal(err)
			}

			if err := replaceFileAtomicallyWithHooks(
				path, next, 0o600, tc.hooks(),
			); !errors.Is(err, injected) {
				t.Fatalf("替换错误 = %v，想要注入错误", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, previous) {
				t.Fatal("失败的原子替换破坏了旧 metadata")
			}
			decoded, err := decodeMetadata(got)
			if err != nil {
				t.Fatalf("旧 metadata 不再可解码：%v", err)
			}
			if decoded.WorldTimeTicks != 7 {
				t.Fatalf("旧世界时间 = %d，想要 7", decoded.WorldTimeTicks)
			}
			matches, err := filepath.Glob(filepath.Join(root, ".world.meta.tmp-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("失败后残留临时文件 %v", matches)
			}
		})
	}
}

func TestMetadataDirectorySyncFailureLeavesCompleteVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "world.meta")
	previous, err := encodeMetadata(Metadata{
		FormatVersion: currentMetadataVersion, Seed: 5, WorldTimeTicks: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := encodeMetadata(Metadata{
		FormatVersion: currentMetadataVersion, Seed: 5, WorldTimeTicks: 99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected directory sync failure")
	err = replaceFileAtomicallyWithHooks(path, next, 0o600, atomicReplaceHooks{
		openDirectory: func(string) (metadataDirectory, error) {
			return &injectedMetadataDirectory{syncErr: injected}, nil
		},
	})
	if !errors.Is(err, injected) {
		t.Fatalf("替换错误 = %v，想要注入错误", err)
	}

	// rename 之后失败：重开必须得到完整旧版或完整新版，不得是半份文件。
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeMetadata(got)
	if err != nil {
		t.Fatalf("目录同步失败后 metadata 不可解码：%v", err)
	}
	if decoded.WorldTimeTicks != 7 && decoded.WorldTimeTicks != 99 {
		t.Fatalf("世界时间 = %d，想要 7 或 99", decoded.WorldTimeTicks)
	}
}

func TestStoreSaveMetadataSharesValueSemantics(t *testing.T) {
	base := Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           99,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 1, Z: 2},
		// 偏移取非零值：保存路径漏写偏移会在读数上暴露，零值夹具下
		// 「写了 0」与「没写」无法区分。
		DayPhaseOffset: 23999,
	}
	ctx := context.Background()

	root := t.TempDir()
	disk, err := OpenDisk(ctx, root, OpenOptions{Create: base})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })

	stores := map[string]Store{"memory": NewMemory(base), "disk": disk}
	for name, store := range stores {
		if got := store.Metadata().WorldTimeTicks; got != 0 {
			t.Fatalf("%s: 初始世界时间 = %d，想要 0", name, got)
		}
		updated := base
		updated.WorldTimeTicks = 4242
		// 偏移归零的保存：读回应是「时间前进、偏移归零」的完整值语义。
		updated.DayPhaseOffset = 0
		if err := store.SaveMetadata(ctx, updated); err != nil {
			t.Fatalf("%s: SaveMetadata: %v", name, err)
		}
		if got := store.Metadata(); got != updated {
			t.Fatalf("%s: 保存后 Metadata = %+v，想要 %+v", name, got, updated)
		}
	}

	// 磁盘上的字节必须已经落盘，重开世界要能继续。
	if err := disk.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := disk.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDisk(ctx, root, OpenOptions{Create: base})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got := reopened.Metadata().WorldTimeTicks; got != 4242 {
		t.Fatalf("重开后世界时间 = %d，想要 4242", got)
	}
	if got := reopened.Metadata().DayPhaseOffset; got != 0 {
		t.Fatalf("重开后偏移 = %d，想要 0", got)
	}
}

func TestOpenDiskMigratesLegacyMetadataFile(t *testing.T) {
	root := t.TempDir()
	legacy := Metadata{
		FormatVersion:  legacyMetadataVersion,
		Seed:           -7,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 4, Z: 5},
	}
	if err := os.WriteFile(
		filepath.Join(root, "world.meta"), encodeLegacyMetadataV1(legacy), fs.FileMode(0o600),
	); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store, err := OpenDisk(ctx, root, OpenOptions{Create: Metadata{FormatVersion: currentMetadataVersion}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	got := store.Metadata()
	if got.FormatVersion != currentMetadataVersion {
		t.Fatalf("打开 v1 世界后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
	if got.WorldTimeTicks != 0 {
		t.Fatalf("v1 世界初始时间 = %d，想要 0", got.WorldTimeTicks)
	}
	if got.Seed != legacy.Seed || got.SpawnAnchor != legacy.SpawnAnchor {
		t.Fatalf("v1 世界种子/出生点丢失：%+v", got)
	}

	// 打开本身不得改写磁盘上的 v1 文件；只有正常保存才升级为 v2。
	onDisk, err := os.ReadFile(filepath.Join(root, "world.meta"))
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(onDisk[4:8]) != legacyMetadataVersion {
		t.Fatal("打开 v1 世界时磁盘文件被提前改写")
	}

	updated := got
	updated.WorldTimeTicks = 60
	if err := store.SaveMetadata(ctx, updated); err != nil {
		t.Fatal(err)
	}
	onDisk, err = os.ReadFile(filepath.Join(root, "world.meta"))
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(onDisk[4:8]) != currentMetadataVersion {
		t.Fatal("保存后磁盘文件未升级为 v2")
	}
}
