package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 新鲜引擎从晴天起步：初始剩余时长必须已落在晴段分布区间内，
// 首个权威 tick 发布的值与之一致。
func TestEngineStartsClearWithLegalRemaining(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	if engine.weatherKind != core.WeatherClear {
		t.Fatalf("初始天气 = %d，想要晴 %d", engine.weatherKind, core.WeatherClear)
	}
	if engine.weatherRemaining < weatherClearMinTicks || engine.weatherRemaining > weatherClearMaxTicks {
		t.Fatalf("初始剩余时长 = %d，想要落在晴段区间 [%d, %d]",
			engine.weatherRemaining, weatherClearMinTicks, weatherClearMaxTicks)
	}
	result := engine.Step()
	if result.WeatherKind != core.WeatherClear {
		t.Fatalf("首个 tick 天气 = %d，想要晴 %d", result.WeatherKind, core.WeatherClear)
	}
}

// 稳定段内每个完成的权威 tick 恰好递减 1，种类不变，发布值与引擎一致。
func TestWeatherDecrementsExactlyOncePerStep(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.weatherKind = core.WeatherRain
	engine.weatherRemaining = 5000
	for step := uint32(0); step < 3; step++ {
		result := engine.Step()
		if result.WeatherKind != core.WeatherRain {
			t.Fatalf("第 %d 个 tick 天气 = %d，想要雨 %d", step+1, result.WeatherKind, core.WeatherRain)
		}
		if engine.weatherRemaining != 5000-step-1 {
			t.Fatalf("第 %d 个 tick 后剩余时长 = %d，想要 %d",
				step+1, engine.weatherRemaining, 5000-step-1)
		}
	}
}

// 到期掷骰必须进入合法新天气，且剩余时长落在对应分布区间内。
func TestWeatherExpiryRollsLegalSegment(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	for _, kind := range []core.WeatherKind{core.WeatherClear, core.WeatherRain, core.WeatherThunder} {
		engine.weatherKind = kind
		engine.weatherRemaining = 1
		result := engine.Step()
		if result.WeatherKind > core.WeatherThunder {
			t.Fatalf("到期后天气 = %d，想要落在 0..2", result.WeatherKind)
		}
		min, max := weatherIntervalForTest(result.WeatherKind)
		if engine.weatherRemaining < min || engine.weatherRemaining > max {
			t.Fatalf("到期后 %d 段剩余时长 = %d，想要落在 [%d, %d]",
				result.WeatherKind, engine.weatherRemaining, min, max)
		}
	}
}

func weatherIntervalForTest(kind core.WeatherKind) (uint32, uint32) {
	if kind == core.WeatherClear {
		return weatherClearMinTicks, weatherClearMaxTicks
	}
	return weatherRainMinTicks, weatherRainMaxTicks
}

// 到期掷骰的固定分布必须覆盖全部三种天气，雷暴占比最小；
// 纯函数扫描不经过完整 tick，速度只取决于整数哈希。
func TestWeatherRollDistributionCoversAllKinds(t *testing.T) {
	var counts [3]int
	for tick := uint64(1); tick <= 200000; tick++ {
		kind, remaining := rollWeatherSegment(0, tick)
		if kind > core.WeatherThunder {
			t.Fatalf("tick %d 掷出非法天气 %d", tick, kind)
		}
		min, max := weatherIntervalForTest(kind)
		if remaining < min || remaining > max {
			t.Fatalf("tick %d 掷出 %d 段剩余时长 %d，想要落在 [%d, %d]",
				tick, kind, remaining, min, max)
		}
		counts[kind]++
	}
	if counts[core.WeatherClear] == 0 || counts[core.WeatherRain] == 0 || counts[core.WeatherThunder] == 0 {
		t.Fatalf("分布未覆盖全部天气：晴=%d 雨=%d 雷暴=%d", counts[0], counts[1], counts[2])
	}
	if !(counts[core.WeatherThunder] < counts[core.WeatherRain] && counts[core.WeatherRain] < counts[core.WeatherClear]) {
		t.Fatalf("分布比例失序：晴=%d 雨=%d 雷暴=%d，想要雷暴<雨<晴",
			counts[0], counts[1], counts[2])
	}
}

// 雷暴段结束后回到掷骰：全程仿真不得出现无界连锁，
// 新掷出的每段时长始终落在对应区间内，非到期 tick 恰好递减 1。
func TestThunderSegmentsReturnToDiceWithoutUnboundedChaining(t *testing.T) {
	kind, remaining := rollWeatherSegment(0, 0)
	assertFreshSegment(t, 0, kind, remaining)
	maxRun, run, thunders := 0, 0, 0
	if kind == core.WeatherThunder {
		run, thunders = 1, 1
		maxRun = 1
	}
	for tick := uint64(1); tick <= 3000000; tick++ {
		expiring := remaining == 1
		previousKind, previousRemaining := kind, remaining
		kind, remaining = advanceWeatherClock(kind, remaining, 0, tick)
		if !expiring {
			if kind != previousKind || remaining != previousRemaining-1 {
				t.Fatalf("tick %d 非到期推进 = (%d, %d)，想要 (%d, %d) 恰好递减",
					tick, kind, remaining, previousKind, previousRemaining-1)
			}
			continue
		}
		assertFreshSegment(t, tick, kind, remaining)
		if kind == core.WeatherThunder {
			run++
			thunders++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	if thunders == 0 {
		t.Fatal("三百万 tick 内一次雷暴都没掷出，升级概率疑似丢失")
	}
	if maxRun > 3 {
		t.Fatalf("雷暴最长连锁 = %d，想要不超过 3（到期必须回到掷骰）", maxRun)
	}
}

func assertFreshSegment(t *testing.T, tick uint64, kind core.WeatherKind, remaining uint32) {
	t.Helper()
	min, max := weatherIntervalForTest(kind)
	if remaining < min || remaining > max {
		t.Fatalf("tick %d 新掷出 %d 段剩余时长 %d，想要落在 [%d, %d]",
			tick, kind, remaining, min, max)
	}
}

// 同一 tick 的两名 Ready 玩家看到相同的天气，且与 tick 结果一致。
// 天气取非零的雨——零值与「字段根本没搬运」不可分辨。
func TestEnginePublishesSameWeatherToAllPlayers(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 1, 0, 0)
	for _, id := range []SessionID{1, 2} {
		position := mgl32.Vec3{2.5 + float32(id), 1, 0.5}
		engine.RegisterPlayer(id, PlayerRestore{
			Current:        &PlayerLocation{Dimension: core.Overworld, Position: position},
			Safe:           &PlayerLocation{Dimension: core.Overworld, Position: position},
			SpawnDimension: core.Overworld,
		})
	}
	engine.weatherKind = core.WeatherRain
	engine.weatherRemaining = 5000
	result := engine.Step()
	if len(result.Players) != 2 {
		t.Fatalf("玩家更新数量 = %d，想要 2", len(result.Players))
	}
	for _, player := range result.Players {
		if !player.Ready {
			t.Fatalf("会话 %d 未 Ready，场景不成立", player.Session)
		}
		if player.WeatherKind != core.WeatherRain || player.WeatherKind != result.WeatherKind {
			t.Fatalf("会话 %d 天气 = %d，想要 %d（与 tick 结果一致）",
				player.Session, player.WeatherKind, result.WeatherKind)
		}
	}
}

// 非 tick 查询路径读到的是当前权威值，而非零值。
func TestEnginePlayerQueryCarriesCurrentWeather(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	engine.weatherKind = core.WeatherThunder
	engine.weatherRemaining = 100
	player, ok := engine.Player(1)
	if !ok {
		t.Fatal("查询不到会话 1 的玩家")
	}
	if player.WeatherKind != core.WeatherThunder {
		t.Fatalf("查询天气 = %d，想要雷暴 %d", player.WeatherKind, core.WeatherThunder)
	}
}

// 天气推进是确定性的（同种子同 tick 同序列），且稳定路径无堆分配。
func TestWeatherAdvanceIsDeterministicAndAllocationFree(t *testing.T) {
	replay := func() []core.WeatherKind {
		engine := NewEngine(0, 0, 0)
		kinds := make([]core.WeatherKind, 0, 64)
		for range 64 {
			kinds = append(kinds, engine.Step().WeatherKind)
		}
		return kinds
	}
	first, second := replay(), replay()
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 次重放天气 = %d，想要 %d", index, second[index], first[index])
		}
	}

	allocations := testing.AllocsPerRun(64, func() {
		advanceWeatherClock(core.WeatherRain, 5000, 42, 7)
	})
	if allocations != 0 {
		t.Fatalf("天气推进分配次数 = %v，想要 0", allocations)
	}
}
