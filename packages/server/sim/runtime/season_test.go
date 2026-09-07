package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 季节派生接线的公共夹具：seed=0 的季节偏移是 core golden 锚（158435），
// 由它解出「年内序号恰为 72000·k」的绝对世界时间，并把显示相位拨到
// p=7800——夏至昼弧 15600 下季节化相位恰为正午 6000，温度锚点因此可手算。
const (
	testSeasonSeed        = int64(0)
	testSeasonOffset      = uint64(158435)
	testSolsticeWorldTime = uint64((int64(72000) - 158435 + core.YearTicks) % core.YearTicks) // 201565
	// 夏至正午的显示偏移：worldTime%24000 = 9565，白昼支路 p=7800 → e=6000。
	testSolsticeDayPhaseOffset = uint16(
		(int64(7800) - int64(testSolsticeWorldTime)%(int64(core.DayLengthTicks)) + core.DayLengthTicks) % core.DayLengthTicks,
	)
)

// newSeasonAnchorEngine 构造一个拨到夏至正午的引擎并注册两名玩家：
// 一名悬在海平面以上 24 格（温度锚 Y=88），一名在海平面（Y=64）。
func newSeasonAnchorEngine(t *testing.T) *Engine {
	t.Helper()
	engine := NewEngine(0, testSolsticeWorldTime-1, testSeasonSeed)
	engine.RestoreDayPhaseOffset(testSolsticeDayPhaseOffset)
	engine.SetWeatherForTest(core.WeatherRain, 5000)
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 1, 0, 0)
	for id, y := range map[SessionID]float32{1: 88, 2: 64} {
		position := mgl32.Vec3{2.5 + float32(id) - 1, y, 0.5}
		engine.RegisterPlayer(id, PlayerRestore{
			Current:        &PlayerLocation{Dimension: core.Overworld, Position: position},
			Safe:           &PlayerLocation{Dimension: core.Overworld, Position: position},
			SpawnDimension: core.Overworld,
		})
		engine.SetPlayerPositionForTest(id, position)
	}
	return engine
}

// 季节偏移由 seed 派生且与 worldTime 无关：同 seed 重启（即使恢复的时间不同）
// 必须得到同一偏移——零持久化裁决成立的前提。
func TestEngineSeasonOffsetDerivedFromSeedStableAcrossRestart(t *testing.T) {
	first := NewEngine(0, 0, testSeasonSeed)
	second := NewEngine(0, 987654, testSeasonSeed)
	if first.seasonOffset != testSeasonOffset || second.seasonOffset != testSeasonOffset {
		t.Fatalf("同 seed 偏移 = %d/%d，想要冻结锚点 %d（core 盐被改动？）",
			first.seasonOffset, second.seasonOffset, testSeasonOffset)
	}
	if other := NewEngine(0, 0, 42); other.seasonOffset == testSeasonOffset {
		t.Fatalf("seed=42 偏移与 seed=0 同为 %d，哈希隔离疑似失效", other.seasonOffset)
	}
}

// Step 尾部派生季节进 TickResult，并按人复制进每份 PlayerUpdate；
// 夏至正午海平面以上 24 格与海平面的温度按钢锚点手算：
// 季节基线 30、正午日内项 0、雨 −4，Y=88 再 −30 恰 −4℃，Y=64 无递减为 26℃。
// （tick 内物理对悬空玩家的亚格级位移落在取整容差内，不影响断言值。）
func TestStepDerivesSeasonAndTemperature(t *testing.T) {
	engine := newSeasonAnchorEngine(t)
	result := engine.Step()
	if result.WorldTimeTicks != testSolsticeWorldTime {
		t.Fatalf("场景世界时间 = %d，想要 %d", result.WorldTimeTicks, testSolsticeWorldTime)
	}
	if result.Season != core.SeasonSummer {
		t.Fatalf("tick 结果季节 = %d，想要夏 %d", result.Season, core.SeasonSummer)
	}
	if result.SeasonProgress != 0 {
		t.Fatalf("夏首 tick 季内进度 = %d，想要 0", result.SeasonProgress)
	}
	temperatures := map[SessionID]int8{}
	for _, player := range result.Players {
		if player.Season != result.Season || player.SeasonProgress != result.SeasonProgress {
			t.Fatalf("会话 %d 季节 %d/%d 与 tick 结果 %d/%d 不一致",
				player.Session, player.Season, player.SeasonProgress, result.Season, result.SeasonProgress)
		}
		temperatures[player.Session] = player.Temperature
	}
	if got := temperatures[1]; got != -4 {
		t.Fatalf("会话 1（Y=88，雨）温度 = %d，想要 -4（夏至正午高山雪线锚再叠加降雨降温）", got)
	}
	if got := temperatures[2]; got != 26 {
		t.Fatalf("会话 2（Y=64，雨）温度 = %d，想要 26（夏至正午海平面 30 再叠加降雨降温）", got)
	}
}

// 季内进度随时间推进：夏至之后再过半季（36000 tick），量化进度恰为 128，
// 季节仍为夏——量化步进与季节边界都由整数年内序号决定，不经浮点相位。
func TestStepSeasonProgressAdvancesHalfSeasonLater(t *testing.T) {
	const midSummerWorldTime = testSolsticeWorldTime + 36000
	engine := NewEngine(0, midSummerWorldTime-1, testSeasonSeed)
	engine.RestoreDayPhaseOffset(uint16(
		(int64(7800) - int64(midSummerWorldTime)%core.DayLengthTicks + core.DayLengthTicks) % core.DayLengthTicks,
	))
	result := engine.Step()
	if result.Season != core.SeasonSummer {
		t.Fatalf("半季之后季节 = %d，想要仍为夏 %d", result.Season, core.SeasonSummer)
	}
	if result.SeasonProgress != 128 {
		t.Fatalf("半季之后进度 = %d，想要 128（36000·256/72000）", result.SeasonProgress)
	}
}

// 非 tick 查询路径与 tick 发布路径同源：`Player` 必须携带当前权威时间下的
// 季节与温度（温度按查询瞬间的玩家 Y 与当 tick 天气求值）。
func TestEnginePlayerQueryCarriesSeasonAndTemperature(t *testing.T) {
	engine := NewEngine(0, testSolsticeWorldTime, testSeasonSeed)
	engine.RestoreDayPhaseOffset(testSolsticeDayPhaseOffset)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	engine.SetWeatherForTest(core.WeatherRain, 5000)
	engine.SetPlayerPositionForTest(1, mgl32.Vec3{0.5, 88, 0.5})
	player, ok := engine.Player(1)
	if !ok {
		t.Fatal("查询不到会话 1 的玩家")
	}
	if player.Season != core.SeasonSummer {
		t.Fatalf("查询季节 = %d，想要夏 %d", player.Season, core.SeasonSummer)
	}
	if player.Temperature != -4 {
		t.Fatalf("查询温度 = %d，想要 -4（夏至正午 Y=88 且降雨）", player.Temperature)
	}
}
