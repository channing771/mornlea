package runtime

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// weather_restore_test.go：天气恢复接线——宿主装配在首个权威 tick 之前把
// metadata 的天气值恢复进引擎；v4 存档值原样恢复，旧档迁移的零剩余时长按
// 新世界默认值掷骰，非法种类归一为晴天。

// TestRestoreWeatherRecoversSavedValues 锁定 v4 恢复路径：存档的种类与剩余时长
// 原样回到引擎，首个权威 tick 从恢复值继续递减而不是回到默认。
func TestRestoreWeatherRecoversSavedValues(t *testing.T) {
	engine := NewEngine(0, 0, 42)
	engine.RestoreWeather(core.WeatherRain, 5000)
	if got := engine.WeatherKind(); got != core.WeatherRain {
		t.Fatalf("恢复后天气 = %d，想要雨 %d", got, core.WeatherRain)
	}
	if got := engine.WeatherTicksRemaining(); got != 5000 {
		t.Fatalf("恢复后剩余时长 = %d，想要 5000", got)
	}
	result := engine.Step()
	if result.WeatherKind != core.WeatherRain {
		t.Fatalf("首个 tick 天气 = %d，想要雨 %d", result.WeatherKind, core.WeatherRain)
	}
	if got := engine.WeatherTicksRemaining(); got != 4999 {
		t.Fatalf("首个 tick 后剩余时长 = %d，想要 4999", got)
	}
}

// TestRestoreWeatherDefaultsZeroRemainingToFreshSegment 锁定旧档迁移路径：
// 剩余时长为零表示旧版本未记录天气，恢复时按与新世界相同的默认值掷骰——
// 迁移世界与同种子新世界行为一致，首个 tick 仍为晴天且只递减。
func TestRestoreWeatherDefaultsZeroRemainingToFreshSegment(t *testing.T) {
	engine := NewEngine(0, 0, 42)
	engine.RestoreWeather(core.WeatherClear, 0)
	want := rollWeatherDuration(42, 0, core.WeatherClear)
	if got := engine.WeatherKind(); got != core.WeatherClear {
		t.Fatalf("迁移恢复后天气 = %d，想要晴 %d", got, core.WeatherClear)
	}
	if got := engine.WeatherTicksRemaining(); got != want {
		t.Fatalf("迁移恢复后剩余时长 = %d，想要新世界默认值 %d", got, want)
	}
	result := engine.Step()
	if result.WeatherKind != core.WeatherClear {
		t.Fatalf("迁移后首个 tick 天气 = %d，想要晴 %d", result.WeatherKind, core.WeatherClear)
	}
	if got := engine.WeatherTicksRemaining(); got != want-1 {
		t.Fatalf("迁移后首个 tick 剩余时长 = %d，想要 %d", got, want-1)
	}
}

// TestRestoreWeatherZeroRemainingPreservesKind 锁定零值分支只补时长、不改种类：
// 假设的 v4 `(Rain,0)` 恢复后种类仍为雨，剩余时长按雨段分布掷骰；
// 迁移 `(Clear,0)` 的行为不变（由上一测试覆盖）。
func TestRestoreWeatherZeroRemainingPreservesKind(t *testing.T) {
	engine := NewEngine(0, 0, 42)
	engine.RestoreWeather(core.WeatherRain, 0)
	if got := engine.WeatherKind(); got != core.WeatherRain {
		t.Fatalf("零剩余时长恢复后天气 = %d，想要雨 %d", got, core.WeatherRain)
	}
	want := rollWeatherDuration(42, 0, core.WeatherRain)
	if got := engine.WeatherTicksRemaining(); got != want {
		t.Fatalf("零剩余时长恢复后剩余时长 = %d，想要雨段默认值 %d", got, want)
	}
	result := engine.Step()
	if result.WeatherKind != core.WeatherRain {
		t.Fatalf("恢复后首个 tick 天气 = %d，想要雨 %d", result.WeatherKind, core.WeatherRain)
	}
	if got := engine.WeatherTicksRemaining(); got != want-1 {
		t.Fatalf("恢复后首个 tick 剩余时长 = %d，想要 %d", got, want-1)
	}
}

// TestRestoreWeatherClampsIllegalKind 锁定损坏防御：非法种类不得进入权威状态
// （非 tick 查询路径不经过推进归一，直接读引擎值下发）。
func TestRestoreWeatherClampsIllegalKind(t *testing.T) {
	engine := NewEngine(0, 0, 42)
	engine.RestoreWeather(core.WeatherKind(7), 100)
	if got := engine.WeatherKind(); got != core.WeatherClear {
		t.Fatalf("非法种类恢复后天气 = %d，想要晴 %d", got, core.WeatherClear)
	}
	if got := engine.WeatherTicksRemaining(); got != 100 {
		t.Fatalf("非法种类恢复后剩余时长 = %d，想要原值 100", got)
	}
}

// TestFreshEngineWeatherGettersMatchInitialSegment 锁定读接口与构造初值的一致：
// 查询路径读到的必须是当前权威值，而非零值。
func TestFreshEngineWeatherGettersMatchInitialSegment(t *testing.T) {
	engine := NewEngine(0, 0, 42)
	if got := engine.WeatherKind(); got != core.WeatherClear {
		t.Fatalf("初始天气 = %d，想要晴 %d", got, core.WeatherClear)
	}
	want := rollWeatherDuration(42, 0, core.WeatherClear)
	if got := engine.WeatherTicksRemaining(); got != want {
		t.Fatalf("初始剩余时长 = %d，想要 %d", got, want)
	}
}
