package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestMetadataEncodingIsDeterministic(t *testing.T) {
	metadata := Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           -42,
		SpawnDimension: core.DimensionID(-3),
		SpawnAnchor:    core.ChunkPos{X: 7, Z: -11},
	}

	one, err := encodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	two, err := encodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one, two) {
		t.Fatal("same metadata encoded differently")
	}

	// 精确字节由 TestMetadataV2GoldenBytes 覆盖，这里只保证确定性与往返。
	wantLength := metadataHeaderLength + int(metadataPayloadLength) + metadataChecksumLength
	if len(one) != wantLength {
		t.Fatalf("metadata length = %d, want %d", len(one), wantLength)
	}
	checksumOffset := wantLength - metadataChecksumLength
	wantCRC := crc32.Checksum(one[:checksumOffset], crc32.MakeTable(crc32.Castagnoli))
	if got := binary.LittleEndian.Uint32(one[checksumOffset:]); got != wantCRC {
		t.Fatalf("metadata CRC32C = %#x, want %#x", got, wantCRC)
	}

	decoded, err := decodeMetadata(one)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != metadata {
		t.Fatalf("metadata roundtrip = %+v, want %+v", decoded, metadata)
	}
}

func TestMetadataRejectsMalformedBytes(t *testing.T) {
	valid, err := encodeMetadata(Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		bytes   func() []byte
		wantErr error
	}{
		{
			name: "CRC corruption",
			bytes: func() []byte {
				corrupt := bytes.Clone(valid)
				corrupt[12] ^= 0xff
				return corrupt
			},
			wantErr: ErrCorrupt,
		},
		{
			name: "future version",
			bytes: func() []byte {
				future := bytes.Clone(valid)
				binary.LittleEndian.PutUint32(future[4:8], currentMetadataVersion+1)
				return future
			},
			wantErr: ErrFutureVersion,
		},
		{
			name: "past version",
			bytes: func() []byte {
				past := bytes.Clone(valid)
				binary.LittleEndian.PutUint32(past[4:8], 0)
				return past
			},
			wantErr: ErrCorrupt,
		},
		{
			name:    "short",
			bytes:   func() []byte { return bytes.Clone(valid[:len(valid)-1]) },
			wantErr: ErrCorrupt,
		},
		{
			name: "trailing",
			bytes: func() []byte {
				return append(bytes.Clone(valid), 0)
			},
			wantErr: ErrCorrupt,
		},
		{
			name: "wrong payload length",
			bytes: func() []byte {
				wrongLength := bytes.Clone(valid)
				binary.LittleEndian.PutUint32(wrongLength[8:12], 19)
				return wrongLength
			},
			wantErr: ErrCorrupt,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeMetadata(tc.bytes()); !errors.Is(err, tc.wantErr) {
				t.Fatalf("decodeMetadata error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestMetadataAtomicReplaceFailurePreservesPreviousFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "world.meta")
	if err := os.WriteFile(path, []byte("previous"), 0o640); err != nil {
		t.Fatal(err)
	}
	bystander := filepath.Join(root, ".world.meta.tmp-keep")
	if err := os.WriteFile(bystander, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}

	rename := func(string, string) error { return errors.New("injected rename failure") }
	if err := replaceFileAtomicallyWithHooks(
		path,
		[]byte("replacement"),
		0o600,
		atomicReplaceHooks{rename: rename, openDirectory: openMetadataDirectory},
	); err == nil {
		t.Fatal("replaceFileAtomically unexpectedly succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous" {
		t.Fatalf("previous file = %q after failed replace", got)
	}
	got, err = os.ReadFile(bystander)
	if err != nil {
		t.Fatalf("bystander temp was removed: %v", err)
	}
	if string(got) != "unrelated" {
		t.Fatalf("bystander temp = %q", got)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".world.meta.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != bystander {
		t.Fatalf("temporary files after failed replace = %v, want only %q", matches, bystander)
	}
}

type injectedMetadataDirectory struct {
	syncErr  error
	closeErr error
}

// encodeLegacyMetadataV4 手工构造一份 CRC 有效的 metadata v4 字节。
// 生产代码只写当前版本，v4 样本必须由测试自己保留。
func encodeLegacyMetadataV4(metadata Metadata) []byte {
	encoded := make([]byte, 0, metadataHeaderLength+41+metadataChecksumLength)
	encoded = append(encoded, metadataMagic[:]...)
	encoded = binary.LittleEndian.AppendUint32(encoded, 4)
	encoded = binary.LittleEndian.AppendUint32(encoded, 41)
	encoded = binary.LittleEndian.AppendUint64(encoded, uint64(metadata.Seed))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnDimension))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.X))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.Z))
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.WorldTimeTicks)
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.DayPhaseOffset)
	encoded = append(encoded, byte(metadata.WeatherKind))
	encoded = binary.LittleEndian.AppendUint32(encoded, metadata.WeatherTicksRemaining)
	return binary.LittleEndian.AppendUint32(
		encoded, crc32.Checksum(encoded, metadataCRCTable),
	)
}

// TestMetadataV4MigratesToV5DepthsDefault 覆盖「v4 旧档默认迁移」：v4 文件缺失
// 维度表尾部，读入即规范为当前版本，`Depths` 出生锚点默认取主世界锚点，种子盐
// 取世界生成侧的固定盐。
func TestMetadataV4MigratesToV5DepthsDefault(t *testing.T) {
	v4 := Metadata{
		FormatVersion:         4,
		Seed:                  42,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 3, Z: -2},
		WorldTimeTicks:        12345,
		DayPhaseOffset:        678,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
	}
	loaded, err := decodeMetadata(encodeLegacyMetadataV4(v4))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FormatVersion != 5 {
		t.Fatalf("迁移后版本 = %d，想要 5", loaded.FormatVersion)
	}
	if loaded.DepthsSpawnAnchor != loaded.SpawnAnchor {
		t.Fatalf(
			"Depths 出生锚点 = %+v，想要主世界锚点 %+v",
			loaded.DepthsSpawnAnchor, loaded.SpawnAnchor,
		)
	}
	if loaded.DepthsSeedSalt != 0x9E3779B97F4A7C15 {
		t.Fatalf("Depths 种子盐 = %#x，想要 0x9E3779B97F4A7C15", loaded.DepthsSeedSalt)
	}
	if loaded.Seed != v4.Seed || loaded.SpawnAnchor != v4.SpawnAnchor ||
		loaded.WorldTimeTicks != v4.WorldTimeTicks || loaded.DayPhaseOffset != v4.DayPhaseOffset ||
		loaded.WeatherKind != v4.WeatherKind || loaded.WeatherTicksRemaining != v4.WeatherTicksRemaining {
		t.Fatalf("v4 迁移丢了既有字段：%+v", loaded)
	}
}

func (directory *injectedMetadataDirectory) Sync() error {
	return directory.syncErr
}

func (directory *injectedMetadataDirectory) Close() error {
	return directory.closeErr
}

func TestMetadataAtomicReplaceReleasesTempPathAfterRename(t *testing.T) {
	tests := []struct {
		name          string
		openDirectory func(error) func(string) (metadataDirectory, error)
	}{
		{
			name: "directory open failure",
			openDirectory: func(injected error) func(string) (metadataDirectory, error) {
				return func(string) (metadataDirectory, error) { return nil, injected }
			},
		},
		{
			name: "directory sync failure",
			openDirectory: func(injected error) func(string) (metadataDirectory, error) {
				return func(string) (metadataDirectory, error) {
					return &injectedMetadataDirectory{syncErr: injected}, nil
				}
			},
		},
		{
			name: "directory close failure",
			openDirectory: func(injected error) func(string) (metadataDirectory, error) {
				return func(string) (metadataDirectory, error) {
					return &injectedMetadataDirectory{closeErr: injected}, nil
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "world.meta")
			if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
				t.Fatal(err)
			}

			var reusedPath string
			rename := func(oldPath, newPath string) error {
				if err := os.Rename(oldPath, newPath); err != nil {
					return err
				}
				reusedPath = oldPath
				return os.WriteFile(reusedPath, []byte("bystander"), 0o600)
			}
			injected := errors.New("injected " + tc.name)
			err := replaceFileAtomicallyWithHooks(
				path,
				[]byte("replacement"),
				0o600,
				atomicReplaceHooks{
					rename:        rename,
					openDirectory: tc.openDirectory(injected),
				},
			)
			if !errors.Is(err, injected) {
				t.Fatalf("replace error = %v, want injected directory error", err)
			}

			canonical, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(canonical) != "replacement" {
				t.Fatalf("canonical after successful rename = %q", canonical)
			}
			bystander, readErr := os.ReadFile(reusedPath)
			if readErr != nil {
				t.Fatalf("reused temp path was removed: %v", readErr)
			}
			if string(bystander) != "bystander" {
				t.Fatalf("reused temp path = %q", bystander)
			}
		})
	}
}
