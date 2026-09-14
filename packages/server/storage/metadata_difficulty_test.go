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

	"github.com/channing771/mornlea/packages/shared/core"
)

// metadata_difficulty_test.go：难度的持久化契约——metadata v6 在 v5 载荷末尾
// （`DepthsSeedSalt` 之后）纯尾部追加 1 字节难度（载荷 62 字节）；v1..v5 世界
// 读入即升级，难度迁移恒为 normal，打开旧档不得改写磁盘，下一次正常保存才写
// v6；CRC 有效但难度字节非法以 `ErrCorrupt` 拒绝；新世界创建必须要求当前
// 版本；难度跨重启延续。

// TestMetadataV6GoldenBytes 冻结当前版本的字节布局：v5 的全部既有字段原位不动
// （纯尾部追加），难度 1 字节再其后，CRC32C 收尾。布局一旦漂移，这里逐字节比对
// 会先红。
func TestMetadataV6GoldenBytes(t *testing.T) {
	metadata := Metadata{
		FormatVersion:         currentMetadataVersion,
		Seed:                  -42,
		SpawnDimension:        core.DimensionID(-3),
		SpawnAnchor:           core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks:        0x0102030405060708,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
		DepthsSpawnAnchor:     core.ChunkPos{X: -5, Z: 9},
		DepthsSeedSalt:        0x0123456789ABCDEF,
		// 难度取非零值：零值与「字段根本没搬运」不可分辨。
		Difficulty: core.DifficultyHard,
	}
	encoded, err := encodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := []byte{
		'M', 'C', 'G', 'M',
		6, 0, 0, 0,
		62, 0, 0, 0,
		0xd6, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xfd, 0xff, 0xff, 0xff,
		7, 0, 0, 0,
		0xf5, 0xff, 0xff, 0xff,
		8, 7, 6, 5, 4, 3, 2, 1,
		// DayPhaseOffset = 12399 = 0x306F，小端。
		0x6f, 0x30, 0, 0, 0, 0, 0, 0,
		// 天气种类 = 雨，剩余时长 = 5000 = 0x1388，小端。
		0x01,
		0x88, 0x13, 0, 0,
		// 维度数 = 2，小端。
		2, 0, 0, 0,
		// Depths 出生锚点 = (-5, 9)，小端。
		0xfb, 0xff, 0xff, 0xff,
		9, 0, 0, 0,
		// 种子盐 = 0x0123456789ABCDEF，小端。
		0xef, 0xcd, 0xab, 0x89, 0x67, 0x45, 0x23, 0x01,
		// 难度 = hard（2）。
		0x02,
	}
	if len(encoded) != len(wantPrefix)+4 || !bytes.Equal(encoded[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("metadata v6 字节 = %x，想要前缀 %x 加 CRC32C", encoded, wantPrefix)
	}
	wantCRC := crc32.Checksum(wantPrefix, metadataCRCTable)
	if got := binary.LittleEndian.Uint32(encoded[len(wantPrefix):]); got != wantCRC {
		t.Fatalf("metadata CRC32C = %#x，想要 %#x", got, wantCRC)
	}

	decoded, err := decodeMetadata(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != metadata {
		t.Fatalf("往返 = %+v，想要 %+v", decoded, metadata)
	}
}

// TestMetadataDifficultySurvivesRestart 覆盖 Scenario「v6 往返保值」在存储层的
// 一半：以 hard 难度保存、关闭并重开同一世界，难度与其余字段必须从保存值继续。
func TestMetadataDifficultySurvivesRestart(t *testing.T) {
	base := Metadata{
		FormatVersion:         currentMetadataVersion,
		Seed:                  99,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 1, Z: 2},
		WorldTimeTicks:        777,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherThunder,
		WeatherTicksRemaining: 4321,
		DepthsSpawnAnchor:     core.ChunkPos{X: -3, Z: 8},
		DepthsSeedSalt:        0x0123456789ABCDEF,
		Difficulty:            core.DifficultyHard,
	}
	ctx := context.Background()

	root := t.TempDir()
	disk, err := OpenDisk(ctx, root, OpenOptions{Create: base})
	if err != nil {
		t.Fatal(err)
	}
	if err := disk.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := disk.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDisk(ctx, root, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got := reopened.Metadata(); got != base {
		t.Fatalf("重开后 metadata = %+v，想要 %+v", got, base)
	}
}

// TestMetadataLegacyVersionsMigrateToNormalDifficulty 锁定 v1..v5 的共同迁移
// 语义：这些版本没有难度字节，读入即升级为当前版本且难度恒补 normal——迁移
// 不得猜测旧档无法表达的意图。
func TestMetadataLegacyVersionsMigrateToNormalDifficulty(t *testing.T) {
	legacy := Metadata{
		Seed:                  -42,
		SpawnDimension:        core.DimensionID(-3),
		SpawnAnchor:           core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks:        0x0102030405060708,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
		DepthsSpawnAnchor:     core.ChunkPos{X: -5, Z: 9},
		DepthsSeedSalt:        0x0123456789ABCDEF,
	}
	tests := []struct {
		name    string
		encoded []byte
	}{
		{"v1", encodeLegacyMetadataV1(Metadata{
			Seed: legacy.Seed, SpawnDimension: legacy.SpawnDimension, SpawnAnchor: legacy.SpawnAnchor,
		})},
		{"v2", encodeLegacyMetadataV2(Metadata{
			Seed: legacy.Seed, SpawnDimension: legacy.SpawnDimension, SpawnAnchor: legacy.SpawnAnchor,
			WorldTimeTicks: legacy.WorldTimeTicks,
		})},
		{"v3", encodeLegacyMetadataV3(Metadata{
			Seed: legacy.Seed, SpawnDimension: legacy.SpawnDimension, SpawnAnchor: legacy.SpawnAnchor,
			WorldTimeTicks: legacy.WorldTimeTicks, DayPhaseOffset: legacy.DayPhaseOffset,
		})},
		{"v4", encodeLegacyMetadataV4(Metadata{
			Seed: legacy.Seed, SpawnDimension: legacy.SpawnDimension, SpawnAnchor: legacy.SpawnAnchor,
			WorldTimeTicks: legacy.WorldTimeTicks, DayPhaseOffset: legacy.DayPhaseOffset,
			WeatherKind: legacy.WeatherKind, WeatherTicksRemaining: legacy.WeatherTicksRemaining,
		})},
		{"v5", encodeLegacyMetadataV5(legacy)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := decodeMetadata(tc.encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Difficulty != core.DifficultyNormal {
				t.Fatalf("%s 迁移后难度 = %d，想要 normal(0)", tc.name, uint8(decoded.Difficulty))
			}
			if decoded.FormatVersion != currentMetadataVersion {
				t.Fatalf("%s 迁移后版本 = %d，想要 %d", tc.name, decoded.FormatVersion, currentMetadataVersion)
			}
		})
	}
}

// TestMetadataV5MigratesToV6WithNormalDifficulty 覆盖 Scenario「v5 世界迁移默认
// 普通难度」的编解码一半：v5 缺失的只有难度字节，其余字段必须逐字段原值保留。
func TestMetadataV5MigratesToV6WithNormalDifficulty(t *testing.T) {
	legacy := Metadata{
		FormatVersion:         legacyMetadataV5Version,
		Seed:                  -42,
		SpawnDimension:        core.DimensionID(-3),
		SpawnAnchor:           core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks:        0x0102030405060708,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
		DepthsSpawnAnchor:     core.ChunkPos{X: -5, Z: 9},
		DepthsSeedSalt:        0x0123456789ABCDEF,
	}
	decoded, err := decodeMetadata(encodeLegacyMetadataV5(legacy))
	if err != nil {
		t.Fatal(err)
	}

	want := legacy
	want.FormatVersion = currentMetadataVersion
	want.Difficulty = core.DifficultyNormal
	if decoded != want {
		t.Fatalf("v5 迁移结果 = %+v，想要 %+v", decoded, want)
	}
}

// TestOpenDiskMigratesV5MetadataFile 覆盖 Scenario「v5 世界迁移默认普通难度」的
// 磁盘一半：v5 的 world.meta 首次打开时读出全部既有字段并把难度规范为 normal，
// 打开本身不得改写磁盘上的 v5 文件，下一次正常保存才升级为 v6。
func TestOpenDiskMigratesV5MetadataFile(t *testing.T) {
	root := t.TempDir()
	legacy := Metadata{
		FormatVersion:         legacyMetadataV5Version,
		Seed:                  -7,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 4, Z: 5},
		WorldTimeTicks:        60,
		DayPhaseOffset:        321,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 4321,
		DepthsSpawnAnchor:     core.ChunkPos{X: -5, Z: 9},
		DepthsSeedSalt:        0x0123456789ABCDEF,
	}
	path := filepath.Join(root, "world.meta")
	legacyBytes := encodeLegacyMetadataV5(legacy)
	if err := os.WriteFile(path, legacyBytes, fs.FileMode(0o600)); err != nil {
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
		t.Fatalf("打开 v5 世界后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
	if got.Difficulty != core.DifficultyNormal {
		t.Fatalf("打开 v5 世界后难度 = %d，想要 normal(0)", uint8(got.Difficulty))
	}
	if got.Seed != legacy.Seed || got.SpawnAnchor != legacy.SpawnAnchor ||
		got.WorldTimeTicks != legacy.WorldTimeTicks || got.DayPhaseOffset != legacy.DayPhaseOffset ||
		got.WeatherKind != legacy.WeatherKind ||
		got.WeatherTicksRemaining != legacy.WeatherTicksRemaining ||
		got.DepthsSpawnAnchor != legacy.DepthsSpawnAnchor ||
		got.DepthsSeedSalt != legacy.DepthsSeedSalt {
		t.Fatalf("v5 迁移丢了既有字段：%+v", got)
	}

	// 打开本身不得改写磁盘上的 v5 文件；只有正常保存才升级。
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, legacyBytes) {
		t.Fatal("打开 v5 世界时磁盘文件被提前改写")
	}

	updated := got
	updated.WorldTimeTicks = 61
	if err := store.SaveMetadata(ctx, updated); err != nil {
		t.Fatal(err)
	}
	onDisk, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(onDisk[4:8]) != currentMetadataVersion {
		t.Fatal("保存后磁盘文件未升级为当前版本")
	}
	rewritten, err := decodeMetadata(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	if rewritten.Difficulty != core.DifficultyNormal {
		t.Fatalf("升级写出的难度 = %d，想要迁移得到的 normal(0)", uint8(rewritten.Difficulty))
	}
	if rewritten.WorldTimeTicks != 61 {
		t.Fatalf("升级写出的世界时间 = %d，想要 61", rewritten.WorldTimeTicks)
	}
}

// TestMetadataRejectsInvalidDifficultyByte 覆盖 Scenario「非法难度字节以损坏
// 拒绝」：CRC 重新算有效、只有难度字节越界时，读取必须以 `ErrCorrupt` 失败，
// 且不得产生部分填充的世界状态。
func TestMetadataRejectsInvalidDifficultyByte(t *testing.T) {
	encoded := mustEncodeMetadataForTest(t, Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
		Difficulty:     core.DifficultyHard,
	})
	difficultyOffset := metadataHeaderLength + int(metadataPayloadLength) - 1
	encoded[difficultyOffset] = 3
	binary.LittleEndian.PutUint32(
		encoded[len(encoded)-metadataChecksumLength:],
		crc32.Checksum(encoded[:len(encoded)-metadataChecksumLength], metadataCRCTable),
	)

	decoded, err := decodeMetadata(encoded)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("decodeMetadata 错误 = %v，想要 %v", err, ErrCorrupt)
	}
	if decoded != (Metadata{}) {
		t.Fatalf("失败路径返回了部分填充的世界状态：%+v", decoded)
	}
}

// TestMetadataRejectsInvalidDifficultyOnEncode 钉住写出侧的难度域校验：构造
// 含非法难度的快照时编码必须失败，防止把读回即损坏的世界写上磁盘。
func TestMetadataRejectsInvalidDifficultyOnEncode(t *testing.T) {
	_, err := encodeMetadata(Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
		Difficulty:     core.Difficulty(3),
	})
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("encodeMetadata 错误 = %v，想要 %v", err, ErrCorrupt)
	}
}

// TestOpenDiskCreateRequiresCurrentMetadataVersion 钉住新世界创建的版本门槛：
// `OpenOptions.Create` 必须要求当前版本，不得把旧结构写进新世界，拒绝后也不得
// 留下半份 metadata 文件。
func TestOpenDiskCreateRequiresCurrentMetadataVersion(t *testing.T) {
	root := t.TempDir()
	legacyCreate := Metadata{
		FormatVersion:  legacyMetadataV5Version,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
		DepthsSeedSalt: depthsSeedSaltDefault,
	}
	_, err := OpenDisk(context.Background(), root, OpenOptions{Create: legacyCreate})
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("OpenDisk 错误 = %v，想要 %v", err, ErrCorrupt)
	}
	if _, statErr := os.Stat(filepath.Join(root, "world.meta")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("拒绝创建后 world.meta 不应存在，stat = %v", statErr)
	}
}
