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

// metadata_dayphase_test.go：显示相位偏移的持久化契约——metadata v3 在 v2 载荷
// 末尾追加 `DayPhaseOffset` u64；metadata v4 又在 v3 载荷末尾追加天气种类 1 字节
// 与剩余时长 u32（见 metadata_weather_test.go 的天气契约）；v1/v2/v3 世界读入即
// 升级，偏移迁移为零，显示相位行为不变；偏移跨重启延续。

// encodeLegacyMetadataV2 手工构造一份 CRC 有效的 metadata v2 字节。
// 生产代码只写当前版本，v2 样本必须由测试自己保留。
func encodeLegacyMetadataV2(metadata Metadata) []byte {
	encoded := make([]byte, 0, metadataHeaderLength+legacyMetadataV2PayloadLength+metadataChecksumLength)
	encoded = append(encoded, metadataMagic[:]...)
	encoded = binary.LittleEndian.AppendUint32(encoded, legacyMetadataV2Version)
	encoded = binary.LittleEndian.AppendUint32(encoded, legacyMetadataV2PayloadLength)
	encoded = binary.LittleEndian.AppendUint64(encoded, uint64(metadata.Seed))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnDimension))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.X))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.Z))
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.WorldTimeTicks)
	return binary.LittleEndian.AppendUint32(
		encoded, crc32.Checksum(encoded, metadataCRCTable),
	)
}

// TestMetadataV5GoldenBytes 冻结当前版本的字节布局：v4 的全部既有字段原位不动
// （纯尾部追加），维度数 u32（固定为 2）、`Depths` 出生锚点 X/Z 各 u32 与种子盐
// u64 再其后，CRC32C 收尾。布局一旦漂移，这里逐字节比对会先红。
func TestMetadataV5GoldenBytes(t *testing.T) {
	metadata := Metadata{
		FormatVersion:         currentMetadataVersion,
		Seed:                  -42,
		SpawnDimension:        core.DimensionID(-3),
		SpawnAnchor:           core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks:        0x0102030405060708,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
		// 锚点与盐取非零值：零值与「字段根本没搬运」不可分辨。
		DepthsSpawnAnchor: core.ChunkPos{X: -5, Z: 9},
		DepthsSeedSalt:    0x0123456789ABCDEF,
	}
	encoded, err := encodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := []byte{
		'M', 'C', 'G', 'M',
		5, 0, 0, 0,
		61, 0, 0, 0,
		0xd6, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xfd, 0xff, 0xff, 0xff,
		7, 0, 0, 0,
		0xf5, 0xff, 0xff, 0xff,
		8, 7, 6, 5, 4, 3, 2, 1,
		// DayPhaseOffset = 12399 = 0x306F，小端。
		0x6f, 0x30, 0, 0, 0, 0, 0, 0,
		// 天气种类 = 雨（取非零值：零值与「字段根本没搬运」不可分辨），
		// 剩余时长 = 5000 = 0x1388，小端。
		0x01,
		0x88, 0x13, 0, 0,
		// 维度数 = 2，小端。
		2, 0, 0, 0,
		// Depths 出生锚点 = (-5, 9)，小端。
		0xfb, 0xff, 0xff, 0xff,
		9, 0, 0, 0,
		// 种子盐 = 0x0123456789ABCDEF，小端。
		0xef, 0xcd, 0xab, 0x89, 0x67, 0x45, 0x23, 0x01,
	}
	if len(encoded) != len(wantPrefix)+4 || !bytes.Equal(encoded[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("metadata v5 字节 = %x，想要前缀 %x 加 CRC32C", encoded, wantPrefix)
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

// TestMetadataV4LegacyGoldenBytes 冻结 v4 的字节布局（本变更一字不改）：它是
// 「v4 旧档仍然可读」的字节级证据，与 encodeLegacyMetadataV4 的现场构造互相
// 印证，防止 legacy 助手本身悄悄偏离历史格式。
func TestMetadataV4LegacyGoldenBytes(t *testing.T) {
	legacy := Metadata{
		FormatVersion:         legacyMetadataV4Version,
		Seed:                  -42,
		SpawnDimension:        core.DimensionID(-3),
		SpawnAnchor:           core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks:        0x0102030405060708,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
	}
	encoded := encodeLegacyMetadataV4(legacy)

	wantPrefix := []byte{
		'M', 'C', 'G', 'M',
		4, 0, 0, 0,
		41, 0, 0, 0,
		0xd6, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xfd, 0xff, 0xff, 0xff,
		7, 0, 0, 0,
		0xf5, 0xff, 0xff, 0xff,
		8, 7, 6, 5, 4, 3, 2, 1,
		// DayPhaseOffset = 12399 = 0x306F，小端。
		0x6f, 0x30, 0, 0, 0, 0, 0, 0,
		// 天气种类 = 雨（取非零值：零值与「字段根本没搬运」不可分辨），
		// 剩余时长 = 5000 = 0x1388，小端。
		0x01,
		0x88, 0x13, 0, 0,
	}
	if len(encoded) != len(wantPrefix)+4 || !bytes.Equal(encoded[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("metadata v4 字节 = %x，想要前缀 %x 加 CRC32C", encoded, wantPrefix)
	}
	wantCRC := crc32.Checksum(wantPrefix, metadataCRCTable)
	if got := binary.LittleEndian.Uint32(encoded[len(wantPrefix):]); got != wantCRC {
		t.Fatalf("metadata CRC32C = %#x，想要 %#x", got, wantCRC)
	}
}

// encodeLegacyMetadataV3 手工构造一份 CRC 有效的 metadata v3 字节。
// 生产代码只写当前版本，v3 样本必须由测试自己保留。
func encodeLegacyMetadataV3(metadata Metadata) []byte {
	encoded := make([]byte, 0, metadataHeaderLength+legacyMetadataV3PayloadLength+metadataChecksumLength)
	encoded = append(encoded, metadataMagic[:]...)
	encoded = binary.LittleEndian.AppendUint32(encoded, legacyMetadataV3Version)
	encoded = binary.LittleEndian.AppendUint32(encoded, legacyMetadataV3PayloadLength)
	encoded = binary.LittleEndian.AppendUint64(encoded, uint64(metadata.Seed))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnDimension))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.X))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.Z))
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.WorldTimeTicks)
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.DayPhaseOffset)
	return binary.LittleEndian.AppendUint32(
		encoded, crc32.Checksum(encoded, metadataCRCTable),
	)
}

// TestMetadataV3LegacyGoldenBytes 冻结 v3 的字节布局（本变更一字不改）：它是
// 「v3 旧档仍然可读」的字节级证据，与 encodeLegacyMetadataV3 的现场构造互相
// 印证，防止 legacy 助手本身悄悄偏离历史格式。
func TestMetadataV3LegacyGoldenBytes(t *testing.T) {
	legacy := Metadata{
		FormatVersion:  legacyMetadataV3Version,
		Seed:           -42,
		SpawnDimension: core.DimensionID(-3),
		SpawnAnchor:    core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks: 0x0102030405060708,
		DayPhaseOffset: 12399,
	}
	encoded := encodeLegacyMetadataV3(legacy)

	wantPrefix := []byte{
		'M', 'C', 'G', 'M',
		3, 0, 0, 0,
		36, 0, 0, 0,
		0xd6, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xfd, 0xff, 0xff, 0xff,
		7, 0, 0, 0,
		0xf5, 0xff, 0xff, 0xff,
		8, 7, 6, 5, 4, 3, 2, 1,
		0x6f, 0x30, 0, 0, 0, 0, 0, 0,
	}
	if len(encoded) != len(wantPrefix)+4 || !bytes.Equal(encoded[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("metadata v3 字节 = %x，想要前缀 %x 加 CRC32C", encoded, wantPrefix)
	}
	wantCRC := crc32.Checksum(wantPrefix, metadataCRCTable)
	if got := binary.LittleEndian.Uint32(encoded[len(wantPrefix):]); got != wantCRC {
		t.Fatalf("metadata CRC32C = %#x，想要 %#x", got, wantCRC)
	}
}

// TestMetadataV2LegacyGoldenBytes 冻结 v2 的字节布局（本变更一字不改）：它是
// 「v2 旧档仍然可读」的字节级证据，与 encodeLegacyMetadataV2 的现场构造互相
// 印证，防止 legacy 助手本身悄悄偏离历史格式。
func TestMetadataV2LegacyGoldenBytes(t *testing.T) {
	legacy := Metadata{
		FormatVersion:  legacyMetadataV2Version,
		Seed:           -42,
		SpawnDimension: core.DimensionID(-3),
		SpawnAnchor:    core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks: 0x0102030405060708,
	}
	encoded := encodeLegacyMetadataV2(legacy)

	wantPrefix := []byte{
		'M', 'C', 'G', 'M',
		2, 0, 0, 0,
		28, 0, 0, 0,
		0xd6, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xfd, 0xff, 0xff, 0xff,
		7, 0, 0, 0,
		0xf5, 0xff, 0xff, 0xff,
		8, 7, 6, 5, 4, 3, 2, 1,
	}
	if len(encoded) != len(wantPrefix)+4 || !bytes.Equal(encoded[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("metadata v2 字节 = %x，想要前缀 %x 加 CRC32C", encoded, wantPrefix)
	}
	wantCRC := crc32.Checksum(wantPrefix, metadataCRCTable)
	if got := binary.LittleEndian.Uint32(encoded[len(wantPrefix):]); got != wantCRC {
		t.Fatalf("metadata CRC32C = %#x，想要 %#x", got, wantCRC)
	}
}

// TestMetadataV2MigratesToV5WithZeroOffsetClearWeatherAndDepthsDefault 覆盖 Scenario
// 「v2 世界迁移偏移为零」与「v3 世界迁移默认晴天」的旧档一半，外加 v4 旧档的
// `Depths` 默认：种子、出生信息与世界时间必须原值保留，偏移迁移为 0，天气迁移
// 为晴天、剩余时长为零，`Depths` 出生锚点默认取主世界锚点、种子盐取固定盐，
// 读入即规范为当前版本。
func TestMetadataV2MigratesToV5WithZeroOffsetClearWeatherAndDepthsDefault(t *testing.T) {
	legacy := Metadata{
		FormatVersion:  legacyMetadataV2Version,
		Seed:           -42,
		SpawnDimension: core.DimensionID(-3),
		SpawnAnchor:    core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks: 0x0102030405060708,
	}
	decoded, err := decodeMetadata(encodeLegacyMetadataV2(legacy))
	if err != nil {
		t.Fatal(err)
	}

	want := legacy
	want.FormatVersion = currentMetadataVersion
	want.DayPhaseOffset = 0
	want.WeatherKind = core.WeatherClear
	want.WeatherTicksRemaining = 0
	want.DepthsSpawnAnchor = legacy.SpawnAnchor
	want.DepthsSeedSalt = depthsSeedSaltDefault
	if decoded != want {
		t.Fatalf("v2 迁移结果 = %+v，想要 %+v", decoded, want)
	}
}

// TestMetadataV1MigratesToV5WithZeroTimeOffsetClearWeatherAndDepthsDefault 锁定 v1 的
// 四步迁移链：世界时间与偏移双双为零，天气为晴天、剩余时长为零，`Depths` 出生
// 锚点默认取主世界锚点、种子盐取固定盐，种子与出生信息原值保留。
func TestMetadataV1MigratesToV5WithZeroTimeOffsetClearWeatherAndDepthsDefault(t *testing.T) {
	legacy := Metadata{
		FormatVersion:  legacyMetadataVersion,
		Seed:           -42,
		SpawnDimension: core.DimensionID(-3),
		SpawnAnchor:    core.ChunkPos{X: 7, Z: -11},
	}
	decoded, err := decodeMetadata(encodeLegacyMetadataV1(legacy))
	if err != nil {
		t.Fatal(err)
	}

	want := legacy
	want.FormatVersion = currentMetadataVersion
	want.WorldTimeTicks = 0
	want.DayPhaseOffset = 0
	want.WeatherKind = core.WeatherClear
	want.WeatherTicksRemaining = 0
	want.DepthsSpawnAnchor = legacy.SpawnAnchor
	want.DepthsSeedSalt = depthsSeedSaltDefault
	if decoded != want {
		t.Fatalf("v1 迁移结果 = %+v，想要 %+v", decoded, want)
	}
}

// TestMetadataVersionsRejectMalformedBytes 是五个版本的共同损坏拒绝矩阵：
// 未来版本拒绝、零版本拒绝、每个版本截断拒绝、声明其他版本载荷长度拒绝、
// 维度数损坏拒绝、CRC 损坏拒绝。矩阵覆盖全部五个版本的两两长度混淆，任何一段
// 长度校验被删都会让这里变红。
func TestMetadataVersionsRejectMalformedBytes(t *testing.T) {
	current := Metadata{
		FormatVersion:         currentMetadataVersion,
		Seed:                  42,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 3, Z: -2},
		WorldTimeTicks:        12345,
		DayPhaseOffset:        678,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
		DepthsSpawnAnchor:     core.ChunkPos{X: -5, Z: 9},
		DepthsSeedSalt:        depthsSeedSaltDefault,
	}
	v5 := mustEncodeMetadataForTest(t, current)
	v4 := encodeLegacyMetadataV4(Metadata{
		FormatVersion:         legacyMetadataV4Version,
		Seed:                  42,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 3, Z: -2},
		WorldTimeTicks:        12345,
		DayPhaseOffset:        678,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 5000,
	})
	v3 := encodeLegacyMetadataV3(Metadata{
		FormatVersion:  legacyMetadataV3Version,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
		WorldTimeTicks: 12345,
		DayPhaseOffset: 678,
	})
	v2 := encodeLegacyMetadataV2(Metadata{
		FormatVersion:  legacyMetadataV2Version,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
		WorldTimeTicks: 12345,
	})
	v1 := encodeLegacyMetadataV1(Metadata{
		FormatVersion:  legacyMetadataVersion,
		Seed:           42,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
	})
	setPayloadLength := func(src []byte, length uint32) []byte {
		wrong := bytes.Clone(src)
		binary.LittleEndian.PutUint32(wrong[8:12], length)
		return wrong
	}
	corruptCRC := func(src []byte) []byte {
		corrupt := bytes.Clone(src)
		corrupt[len(corrupt)-5] ^= 0xff
		return corrupt
	}

	tests := []struct {
		name    string
		bytes   func() []byte
		wantErr error
	}{
		{"未来版本", func() []byte {
			future := bytes.Clone(v5)
			binary.LittleEndian.PutUint32(future[4:8], currentMetadataVersion+1)
			return future
		}, ErrFutureVersion},
		{"零版本", func() []byte {
			past := bytes.Clone(v5)
			binary.LittleEndian.PutUint32(past[4:8], 0)
			return past
		}, ErrCorrupt},
		{"v5 截断", func() []byte { return bytes.Clone(v5[:len(v5)-1]) }, ErrCorrupt},
		{"v4 截断", func() []byte { return bytes.Clone(v4[:len(v4)-1]) }, ErrCorrupt},
		{"v3 截断", func() []byte { return bytes.Clone(v3[:len(v3)-1]) }, ErrCorrupt},
		{"v2 截断", func() []byte { return bytes.Clone(v2[:len(v2)-1]) }, ErrCorrupt},
		{"v1 截断", func() []byte { return bytes.Clone(v1[:len(v1)-1]) }, ErrCorrupt},
		{"v5 声明 v4 长度", func() []byte {
			return setPayloadLength(v5, legacyMetadataV4PayloadLength)
		}, ErrCorrupt},
		{"v5 声明 v3 长度", func() []byte {
			return setPayloadLength(v5, legacyMetadataV3PayloadLength)
		}, ErrCorrupt},
		{"v5 声明 v2 长度", func() []byte {
			return setPayloadLength(v5, legacyMetadataV2PayloadLength)
		}, ErrCorrupt},
		{"v5 声明 v1 长度", func() []byte {
			return setPayloadLength(v5, legacyMetadataPayloadLength)
		}, ErrCorrupt},
		{"v5 维度数损坏", func() []byte {
			badDimCount := bytes.Clone(v5)
			binary.LittleEndian.PutUint32(badDimCount[metadataHeaderLength+41:], 3)
			binary.LittleEndian.PutUint32(
				badDimCount[len(badDimCount)-metadataChecksumLength:],
				crc32.Checksum(badDimCount[:len(badDimCount)-metadataChecksumLength], metadataCRCTable),
			)
			return badDimCount
		}, ErrCorrupt},
		{"v4 声明 v5 长度", func() []byte {
			return setPayloadLength(v4, metadataPayloadLength)
		}, ErrCorrupt},
		{"v4 声明 v3 长度", func() []byte {
			return setPayloadLength(v4, legacyMetadataV3PayloadLength)
		}, ErrCorrupt},
		{"v4 声明 v2 长度", func() []byte {
			return setPayloadLength(v4, legacyMetadataV2PayloadLength)
		}, ErrCorrupt},
		{"v4 声明 v1 长度", func() []byte {
			return setPayloadLength(v4, legacyMetadataPayloadLength)
		}, ErrCorrupt},
		{"v3 声明 v5 长度", func() []byte {
			return setPayloadLength(v3, metadataPayloadLength)
		}, ErrCorrupt},
		{"v3 声明 v4 长度", func() []byte {
			return setPayloadLength(v3, legacyMetadataV4PayloadLength)
		}, ErrCorrupt},
		{"v2 声明 v5 长度", func() []byte {
			return setPayloadLength(v2, metadataPayloadLength)
		}, ErrCorrupt},
		{"v2 声明 v4 长度", func() []byte {
			return setPayloadLength(v2, legacyMetadataV4PayloadLength)
		}, ErrCorrupt},
		{"v2 声明 v3 长度", func() []byte {
			return setPayloadLength(v2, legacyMetadataV3PayloadLength)
		}, ErrCorrupt},
		{"v2 声明 v1 长度", func() []byte {
			return setPayloadLength(v2, legacyMetadataPayloadLength)
		}, ErrCorrupt},
		{"v1 声明 v5 长度", func() []byte {
			return setPayloadLength(v1, metadataPayloadLength)
		}, ErrCorrupt},
		{"v1 声明 v4 长度", func() []byte {
			return setPayloadLength(v1, legacyMetadataV4PayloadLength)
		}, ErrCorrupt},
		{"v1 声明 v3 长度", func() []byte {
			return setPayloadLength(v1, legacyMetadataV3PayloadLength)
		}, ErrCorrupt},
		{"v1 声明 v2 长度", func() []byte {
			return setPayloadLength(v1, legacyMetadataV2PayloadLength)
		}, ErrCorrupt},
		{"v5 CRC 损坏", func() []byte { return corruptCRC(v5) }, ErrCorrupt},
		{"v4 CRC 损坏", func() []byte { return corruptCRC(v4) }, ErrCorrupt},
		{"v3 CRC 损坏", func() []byte { return corruptCRC(v3) }, ErrCorrupt},
		{"v2 CRC 损坏", func() []byte { return corruptCRC(v2) }, ErrCorrupt},
		{"v5 尾随字节", func() []byte { return append(bytes.Clone(v5), 0) }, ErrCorrupt},
		{"v4 尾随字节", func() []byte { return append(bytes.Clone(v4), 0) }, ErrCorrupt},
		{"v3 尾随字节", func() []byte { return append(bytes.Clone(v3), 0) }, ErrCorrupt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeMetadata(tc.bytes()); !errors.Is(err, tc.wantErr) {
				t.Fatalf("decodeMetadata 错误 = %v，想要 %v", err, tc.wantErr)
			}
		})
	}
}

// TestMetadataOffsetSurvivesRestart 覆盖 Scenario「重启延续世界时间与偏移」：
// 偏移非零时保存、关闭并重开同一世界，绝对时间与偏移都必须从保存值继续。
func TestMetadataOffsetSurvivesRestart(t *testing.T) {
	base := Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           99,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 1, Z: 2},
		WorldTimeTicks: 777,
		DayPhaseOffset: 12399,
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
	got := reopened.Metadata()
	if got.WorldTimeTicks != base.WorldTimeTicks {
		t.Fatalf("重开后世界时间 = %d，想要 %d", got.WorldTimeTicks, base.WorldTimeTicks)
	}
	if got.DayPhaseOffset != base.DayPhaseOffset {
		t.Fatalf("重开后偏移 = %d，想要 %d", got.DayPhaseOffset, base.DayPhaseOffset)
	}
	if got.FormatVersion != currentMetadataVersion {
		t.Fatalf("重开后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
}

// TestOpenDiskMigratesV2MetadataFile 覆盖 Scenario「v2 世界迁移偏移为零」的
// 磁盘一半：v2 的 world.meta 首次打开时读出世界时间、偏移规范为 0，打开本身
// 不得改写磁盘上的 v2 文件，下一次正常保存才升级为当前版本。
func TestOpenDiskMigratesV2MetadataFile(t *testing.T) {
	root := t.TempDir()
	legacy := Metadata{
		FormatVersion:  legacyMetadataV2Version,
		Seed:           -7,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 4, Z: 5},
		WorldTimeTicks: 60,
	}
	path := filepath.Join(root, "world.meta")
	if err := os.WriteFile(path, encodeLegacyMetadataV2(legacy), fs.FileMode(0o600)); err != nil {
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
		t.Fatalf("打开 v2 世界后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
	if got.WorldTimeTicks != legacy.WorldTimeTicks {
		t.Fatalf("v2 世界时间 = %d，想要原值 %d", got.WorldTimeTicks, legacy.WorldTimeTicks)
	}
	if got.DayPhaseOffset != 0 {
		t.Fatalf("v2 世界偏移 = %d，想要 0", got.DayPhaseOffset)
	}
	if got.Seed != legacy.Seed || got.SpawnAnchor != legacy.SpawnAnchor {
		t.Fatalf("v2 世界种子/出生点丢失：%+v", got)
	}

	// 打开本身不得改写磁盘上的 v2 文件；只有正常保存才升级。
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(onDisk[4:8]) != legacyMetadataV2Version {
		t.Fatal("打开 v2 世界时磁盘文件被提前改写")
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
}

func mustEncodeMetadataForTest(t *testing.T, metadata Metadata) []byte {
	t.Helper()
	encoded, err := encodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
