package storage

import (
	"context"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// metadata_weather_test.go：天气的持久化契约——metadata v4 在 v3 载荷末尾追加
// 天气种类 1 字节与剩余时长 u32（字节布局见 metadata_difficulty_test.go 的
// 当前版本 golden）；v1/v2/v3 世界读入即升级，天气迁移为晴天、剩余时长迁移
// 为零（零表示旧档未记录，恢复时按新世界默认值掷骰）；天气跨重启延续。

// TestMetadataV3MigratesToCurrentWithClearWeatherAndDepthsDefault 覆盖 Scenario
// 「v3 世界迁移默认晴天」：种子、出生信息、世界时间与偏移必须原值保留，天气迁移
// 为晴天、剩余时长为零，`Depths` 出生锚点默认取主世界锚点、种子盐取固定盐，
// 读入即规范为当前版本。
func TestMetadataV3MigratesToCurrentWithClearWeatherAndDepthsDefault(t *testing.T) {
	legacy := Metadata{
		FormatVersion:  legacyMetadataV3Version,
		Seed:           -42,
		SpawnDimension: core.DimensionID(-3),
		SpawnAnchor:    core.ChunkPos{X: 7, Z: -11},
		WorldTimeTicks: 0x0102030405060708,
		DayPhaseOffset: 12399,
	}
	decoded, err := decodeMetadata(encodeLegacyMetadataV3(legacy))
	if err != nil {
		t.Fatal(err)
	}

	want := legacy
	want.FormatVersion = currentMetadataVersion
	want.WeatherKind = core.WeatherClear
	want.WeatherTicksRemaining = 0
	want.DepthsSpawnAnchor = legacy.SpawnAnchor
	want.DepthsSeedSalt = depthsSeedSaltDefault
	if decoded != want {
		t.Fatalf("v3 迁移结果 = %+v，想要 %+v", decoded, want)
	}
}

// TestMetadataWeatherSurvivesRestart 覆盖 Scenario「重启延续天气」在存储层的一半：
// 天气非默认值时保存、关闭并重开同一世界，天气种类与剩余时长必须从保存值继续。
func TestMetadataWeatherSurvivesRestart(t *testing.T) {
	base := Metadata{
		FormatVersion:         currentMetadataVersion,
		Seed:                  99,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 1, Z: 2},
		WorldTimeTicks:        777,
		DayPhaseOffset:        12399,
		WeatherKind:           core.WeatherThunder,
		WeatherTicksRemaining: 4321,
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
	if got.WeatherKind != base.WeatherKind {
		t.Fatalf("重开后天气 = %d，想要 %d", got.WeatherKind, base.WeatherKind)
	}
	if got.WeatherTicksRemaining != base.WeatherTicksRemaining {
		t.Fatalf("重开后剩余时长 = %d，想要 %d", got.WeatherTicksRemaining, base.WeatherTicksRemaining)
	}
	if got.FormatVersion != currentMetadataVersion {
		t.Fatalf("重开后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
}

// TestOpenDiskMigratesV3MetadataFile 覆盖 Scenario「v3 世界迁移默认晴天」的
// 磁盘一半：v3 的 world.meta 首次打开时读出种子、时间与偏移并把天气规范为晴天
// （剩余时长为零），打开本身不得改写磁盘上的 v3 文件，下一次正常保存才升级
// 为当前版本。
func TestOpenDiskMigratesV3MetadataFile(t *testing.T) {
	root := t.TempDir()
	legacy := Metadata{
		FormatVersion:  legacyMetadataV3Version,
		Seed:           -7,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 4, Z: 5},
		WorldTimeTicks: 60,
		DayPhaseOffset: 321,
	}
	path := filepath.Join(root, "world.meta")
	if err := os.WriteFile(path, encodeLegacyMetadataV3(legacy), fs.FileMode(0o600)); err != nil {
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
		t.Fatalf("打开 v3 世界后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
	if got.WorldTimeTicks != legacy.WorldTimeTicks || got.DayPhaseOffset != legacy.DayPhaseOffset {
		t.Fatalf("v3 世界时间/偏移 = (%d, %d)，想要原值 (%d, %d)",
			got.WorldTimeTicks, got.DayPhaseOffset, legacy.WorldTimeTicks, legacy.DayPhaseOffset)
	}
	if got.WeatherKind != core.WeatherClear {
		t.Fatalf("v3 世界天气 = %d，想要晴天 %d", got.WeatherKind, core.WeatherClear)
	}
	if got.WeatherTicksRemaining != 0 {
		t.Fatalf("v3 世界剩余时长 = %d，想要 0", got.WeatherTicksRemaining)
	}
	if got.Seed != legacy.Seed || got.SpawnAnchor != legacy.SpawnAnchor {
		t.Fatalf("v3 世界种子/出生点丢失：%+v", got)
	}

	// 打开本身不得改写磁盘上的 v3 文件；只有正常保存才升级。
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(onDisk[4:8]) != legacyMetadataV3Version {
		t.Fatal("打开 v3 世界时磁盘文件被提前改写")
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

// TestOpenDiskMigratesV4MetadataFile 覆盖 Scenario「v4 旧档默认迁移」的磁盘一半：
// v4 的 world.meta 首次打开时读出全部既有字段并把 `Depths` 出生锚点规范为主世界
// 锚点、种子盐规范为固定盐，打开本身不得改写磁盘上的 v4 文件，下一次正常保存
// 才升级为当前版本。
func TestOpenDiskMigratesV4MetadataFile(t *testing.T) {
	root := t.TempDir()
	legacy := Metadata{
		FormatVersion:         legacyMetadataV4Version,
		Seed:                  -7,
		SpawnDimension:        core.Overworld,
		SpawnAnchor:           core.ChunkPos{X: 4, Z: 5},
		WorldTimeTicks:        60,
		DayPhaseOffset:        321,
		WeatherKind:           core.WeatherRain,
		WeatherTicksRemaining: 4321,
	}
	path := filepath.Join(root, "world.meta")
	if err := os.WriteFile(path, encodeLegacyMetadataV4(legacy), fs.FileMode(0o600)); err != nil {
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
		t.Fatalf("打开 v4 世界后版本 = %d，想要 %d", got.FormatVersion, currentMetadataVersion)
	}
	if got.DepthsSpawnAnchor != legacy.SpawnAnchor {
		t.Fatalf(
			"v4 世界 Depths 锚点 = %+v，想要主世界锚点 %+v",
			got.DepthsSpawnAnchor, legacy.SpawnAnchor,
		)
	}
	if got.DepthsSeedSalt != depthsSeedSaltDefault {
		t.Fatalf("v4 世界种子盐 = %#x，想要 %#x", got.DepthsSeedSalt, depthsSeedSaltDefault)
	}
	if got.WorldTimeTicks != legacy.WorldTimeTicks || got.DayPhaseOffset != legacy.DayPhaseOffset {
		t.Fatalf("v4 世界时间/偏移 = (%d, %d)，想要原值 (%d, %d)",
			got.WorldTimeTicks, got.DayPhaseOffset, legacy.WorldTimeTicks, legacy.DayPhaseOffset)
	}
	if got.WeatherKind != legacy.WeatherKind || got.WeatherTicksRemaining != legacy.WeatherTicksRemaining {
		t.Fatalf("v4 世界天气 = (%d, %d)，想要原值 (%d, %d)",
			got.WeatherKind, got.WeatherTicksRemaining, legacy.WeatherKind, legacy.WeatherTicksRemaining)
	}

	// 打开本身不得改写磁盘上的 v4 文件；只有正常保存才升级。
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(onDisk[4:8]) != legacyMetadataV4Version {
		t.Fatal("打开 v4 世界时磁盘文件被提前改写")
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
	if rewritten.DepthsSpawnAnchor != legacy.SpawnAnchor || rewritten.DepthsSeedSalt != depthsSeedSaltDefault {
		t.Fatalf("升级后维度表丢失：%+v", rewritten)
	}
}
