package render

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// isSnowPart 按实例纵向尺度区分雪点与雨丝：雪是 0.09 立方体，雨是纵向
// 0.6 的细丝（与 `BuildWeatherParts` 的编码同值）。
func isSnowPart(part avatarPart) bool {
	height := mgl32.Vec3{part.transform[4], part.transform[5], part.transform[6]}.Len()
	return math.Abs(float64(height-0.09)) < 1e-6
}

// TestWeatherDaylightCapLocksSpecCeilings 锁定昼夜亮度的天气压暗上限：晴天
// 不压暗，雨天等效天空光不超过 12/15，雷暴不超过 10/15（spec 原文逐字值）。
func TestWeatherDaylightCapLocksSpecCeilings(t *testing.T) {
	if got := WeatherDaylightCap(core.WeatherClear); got != 1 {
		t.Fatalf("晴天压暗上限 = %v，想要 1（不压暗）", got)
	}
	if got := WeatherDaylightCap(core.WeatherRain); got != 12.0/15.0 {
		t.Fatalf("雨天压暗上限 = %v，想要 12/15", got)
	}
	if got := WeatherDaylightCap(core.WeatherThunder); got != 10.0/15.0 {
		t.Fatalf("雷暴压暗上限 = %v，想要 10/15", got)
	}
}

// TestApplyWeatherDaylightClampsAboveCeiling 只压上限：上限之下原样通过，
// 上限之上钳制，午夜低亮度不受影响。
func TestApplyWeatherDaylightClampsAboveCeiling(t *testing.T) {
	noon := DayNightAt(6000, 0).Daylight
	if noon != 1 {
		t.Fatalf("夹具无效：正午 daylight = %v，想要 1", noon)
	}
	if got := ApplyWeatherDaylight(noon, core.WeatherClear, 0); got != noon {
		t.Fatalf("晴天正午 = %v，想要 %v（恢复既有亮度）", got, noon)
	}
	if got := ApplyWeatherDaylight(noon, core.WeatherRain, 0); got != 12.0/15.0 {
		t.Fatalf("雨天正午 = %v，想要 12/15", got)
	}
	midnight := DayNightAt(18000, 0).Daylight
	if got := ApplyWeatherDaylight(midnight, core.WeatherThunder, 79); got != midnight {
		t.Fatalf("雷暴午夜 = %v，想要 %v（低亮度不压暗）", got, midnight)
	}
}

// TestThunderFlashLiftIsBoundedAndPeriodic 锁定雷暴闪光的固定频率与幅度
// 上限：只在雷暴出现，单帧增量恒在上限内，同 tick 重放一致、无伤害语义
// （纯呈现值，不触碰任何权威状态）。
func TestThunderFlashLiftIsBoundedAndPeriodic(t *testing.T) {
	const maxLift = 0.22
	for tick := uint64(0); tick < 400; tick++ {
		lift := ThunderFlashLift(tick)
		if lift < 0 || lift > maxLift {
			t.Fatalf("tick %d 的闪光增量 = %v，想要落在 [0,%v]", tick, lift, maxLift)
		}
		if lift != ThunderFlashLift(tick) {
			t.Fatalf("tick %d 的闪光不确定，同 tick 重放不一致", tick)
		}
	}
	var lit int
	for tick := uint64(0); tick < 80; tick++ {
		if ThunderFlashLift(tick) > 0 {
			lit++
		}
		if ThunderFlashLift(tick) != ThunderFlashLift(tick+80) {
			t.Fatalf("tick %d 的闪光周期不是 80 tick", tick)
		}
	}
	if lit == 0 || lit >= 80 {
		t.Fatalf("80 tick 周期内点亮 %d tick，想要少数帧闪光、多数帧黑暗", lit)
	}
}

// TestApplyWeatherDaylightAddsThunderFlash 只在雷暴叠加闪光：雨与晴无增量，
// 雷暴增量恒为当 tick 的闪光值且总量仍封顶在 1。
func TestApplyWeatherDaylightAddsThunderFlash(t *testing.T) {
	noon := DayNightAt(6000, 0).Daylight
	var flashTick uint64
	for tick := uint64(0); tick < 80; tick++ {
		if ThunderFlashLift(tick) > 0 {
			flashTick = tick
			break
		}
	}
	if got := ApplyWeatherDaylight(noon, core.WeatherRain, flashTick); got != 12.0/15.0 {
		t.Fatalf("雨天在闪光 tick 的亮度 = %v，想要仍为 12/15（雨无闪光）", got)
	}
	want := float32(10.0/15.0 + ThunderFlashLift(flashTick))
	if want > 1 {
		want = 1
	}
	if got := ApplyWeatherDaylight(noon, core.WeatherThunder, flashTick); got != want {
		t.Fatalf("雷暴在闪光 tick 的亮度 = %v，想要 %v", got, want)
	}
	var darkTick uint64 = 79
	if ThunderFlashLift(darkTick) != 0 {
		darkTick = 70
	}
	if got := ApplyWeatherDaylight(noon, core.WeatherThunder, darkTick); got != 10.0/15.0 {
		t.Fatalf("雷暴在黑暗 tick 的亮度 = %v，想要 10/15", got)
	}
}

// TestWeatherSkyGrayLocksFactors 灰度因子：晴天为零（恒等），雨/雷暴为固定
// 正值且雷暴更灰。
func TestWeatherSkyGrayLocksFactors(t *testing.T) {
	if got := WeatherSkyGray(core.WeatherClear); got != 0 {
		t.Fatalf("晴天灰度 = %v，想要 0", got)
	}
	rain, thunder := WeatherSkyGray(core.WeatherRain), WeatherSkyGray(core.WeatherThunder)
	if rain <= 0 || rain >= 1 || thunder <= 0 || thunder >= 1 || thunder <= rain {
		t.Fatalf("灰度 rain=%v thunder=%v，想要 0<rain<thunder<1", rain, thunder)
	}
}

// TestWeatherSkyColorDesaturatesButKeepsAlpha 天空变灰：晴天恒等，雨/雷暴
// 向亮度灰靠拢（每通道与灰的距离缩小），alpha 不变。
func TestWeatherSkyColorDesaturatesButKeepsAlpha(t *testing.T) {
	noon := DayNightAt(6000, 0).ClearColor
	if got := WeatherSkyColor(noon, core.WeatherClear); got != noon {
		t.Fatalf("晴天天空色 = %v，想要恒等 %v", got, noon)
	}
	for _, kind := range []core.WeatherKind{core.WeatherRain, core.WeatherThunder} {
		got := WeatherSkyColor(noon, kind)
		if got[3] != noon[3] {
			t.Fatalf("天气 %d 的天空 alpha = %v，想要 %v", kind, got[3], noon[3])
		}
		luma := 0.299*got[0] + 0.587*got[1] + 0.114*got[2]
		for channel := range 3 {
			before := float64(noon[channel] - (0.299*noon[0] + 0.587*noon[1] + 0.114*noon[2]))
			after := float64(got[channel] - luma)
			if math.Abs(after) > math.Abs(before) {
				t.Fatalf("天气 %d 通道 %d 离灰更远：%v → %v", kind, channel, noon, got)
			}
		}
	}
	rain := WeatherSkyColor(noon, core.WeatherRain)
	thunder := WeatherSkyColor(noon, core.WeatherThunder)
	rainDist := math.Abs(float64(rain[0]-rain[1])) + math.Abs(float64(rain[1]-rain[2]))
	thunderDist := math.Abs(float64(thunder[0]-thunder[1])) + math.Abs(float64(thunder[1]-thunder[2]))
	if thunderDist >= rainDist {
		t.Fatalf("雷暴天空 %v 不比雨天 %v 更灰", thunder, rain)
	}
}

// TestSnowLineMatchesEngineContract 雪线复用既有地表雪线常量：Go 侧判定值
// 必须与 Rust 世界生成 `SNOW_LINE` 同值，纯本地只读、不进权威不同步。
func TestSnowLineMatchesEngineContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "engine", "crates", "mornlea_engine", "src", "worldgen.rs"))
	if err != nil {
		t.Skipf("读不到 Rust 世界生成源码：%v", err)
	}
	const wantPrefix = "const SNOW_LINE: i32 = "
	found := -1
	for line := range strings.Lines(string(raw)) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, wantPrefix) {
			var value int
			rest := strings.TrimSuffix(strings.TrimPrefix(trimmed, wantPrefix), ";")
			if _, err := fmt.Sscanf(rest, "%d", &value); err != nil {
				t.Fatalf("解析 SNOW_LINE 失败：%q", trimmed)
			}
			found = value
		}
	}
	if found < 0 {
		t.Fatal("Rust worldgen.rs 里找不到 SNOW_LINE 常量")
	}
	if WeatherSnowLineY != float32(found) {
		t.Fatalf("Go 雪线 = %v，想要与 Rust SNOW_LINE=%d 同值", WeatherSnowLineY, found)
	}
}

// TestBuildWeatherPartsClearEmitsNothing 晴天无粒子：输出为空且不触碰复用
// 缓冲的已有内容长度之外。
func TestBuildWeatherPartsClearEmitsNothing(t *testing.T) {
	parts := BuildWeatherParts(nil, mgl32.Vec3{0, 70, 0}, 0, 1234, core.WeatherClear)
	if len(parts) != 0 {
		t.Fatalf("晴天粒子数 = %d，想要 0", len(parts))
	}
}

// TestBuildWeatherPartsClearTruncatesReusedBuffer 晴天截断复用缓冲：先雨后
// 晴复用同一 `dst`，晴天必须把长度清零，不得残留旧 256 粒（否则直接复用者
// 会在晴天误绘降水）。
func TestBuildWeatherPartsClearTruncatesReusedBuffer(t *testing.T) {
	cam := mgl32.Vec3{0, 40, 0}
	dst := BuildWeatherParts(nil, cam, 0, 500, core.WeatherRain)
	if len(dst) != WeatherMaxParticles {
		t.Fatalf("雨天粒子数 = %d，想要 %d", len(dst), WeatherMaxParticles)
	}
	if cleared := BuildWeatherParts(dst, cam, 0, 501, core.WeatherClear); len(cleared) != 0 {
		t.Fatalf("复用缓冲的晴天粒子数 = %d，想要 0", len(cleared))
	}
}

// TestBuildWeatherPartsSelectsFormByHeightRelativeToSnowline 降水形态只由
// 本地高度相对雪线确定：整列在线上的相机全为雪、整列在线下的全为雨，跨线
// 相机按粒子世界高度逐粒分形（spec 高度决定雨雪形态场景）。
func TestBuildWeatherPartsSelectsFormByHeightRelativeToSnowline(t *testing.T) {
	high := BuildWeatherParts(nil, mgl32.Vec3{0, 120, 0}, 0, 500, core.WeatherRain)
	if len(high) != WeatherMaxParticles {
		t.Fatalf("高处粒子数 = %d，想要 %d", len(high), WeatherMaxParticles)
	}
	for index, part := range high {
		if !isSnowPart(part) {
			t.Fatalf("雪线上方粒子 %d 不是雪形：%+v", index, part)
		}
	}
	low := BuildWeatherParts(nil, mgl32.Vec3{0, 40, 0}, 0, 500, core.WeatherThunder)
	if len(low) != WeatherMaxParticles {
		t.Fatalf("低处粒子数 = %d，想要 %d", len(low), WeatherMaxParticles)
	}
	for index, part := range low {
		if isSnowPart(part) {
			t.Fatalf("雪线下方粒子 %d 不是雨形：%+v", index, part)
		}
	}
	// 跨线相机（雪线 ± 列高内）：两种形态必须同时出现，且分界恰为雪线。
	mixed := BuildWeatherParts(nil, mgl32.Vec3{0, WeatherSnowLineY + 2, 0}, 0, 500, core.WeatherRain)
	var snow, rain int
	for _, part := range mixed {
		center := part.transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
		if center.Y() >= WeatherSnowLineY {
			snow++
			if !isSnowPart(part) {
				t.Fatalf("雪线上粒子不是雪形：y=%v", center.Y())
			}
		} else {
			rain++
			if isSnowPart(part) {
				t.Fatalf("雪线下粒子不是雨形：y=%v", center.Y())
			}
		}
	}
	if snow == 0 || rain == 0 {
		t.Fatalf("跨线相机 snow=%d rain=%d，想要两种形态同时出现", snow, rain)
	}
}

// TestBuildWeatherPartsFallsWithAuthoritativeTick 粒子随权威 tick 下落：
// 同序号粒子在后一 tick 的世界高度更低（无堆积：位置是 tick 的纯函数，
// 不存逐帧状态），同输入重放逐字节一致。
func TestBuildWeatherPartsFallsWithAuthoritativeTick(t *testing.T) {
	cam := mgl32.Vec3{0, 40, 0}
	before := BuildWeatherParts(nil, cam, 0, 100, core.WeatherRain)
	after := BuildWeatherParts(nil, cam, 0, 101, core.WeatherRain)
	if len(before) != len(after) {
		t.Fatalf("tick 前后粒子数 %d → %d，想要不变", len(before), len(after))
	}
	var fell int
	for index := range before {
		yBefore := before[index].transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Y()
		yAfter := after[index].transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Y()
		if yAfter < yBefore {
			fell++
		}
	}
	if fell == 0 {
		t.Fatal("tick 前进后没有粒子下落")
	}
	replay := BuildWeatherParts(nil, cam, 0, 100, core.WeatherRain)
	for index := range before {
		if replay[index] != before[index] {
			t.Fatalf("粒子 %d 同输入重放不一致", index)
		}
	}
}

// TestBuildWeatherPartsStaysInCameraForwardBox 粒子落在相机周围有界体内：
// 固定上限数量、水平/纵向偏移有界（相机前方有界数量）。
func TestBuildWeatherPartsStaysInCameraForwardBox(t *testing.T) {
	cam := mgl32.Vec3{100, 60, -40}
	parts := BuildWeatherParts(nil, cam, 0, 77, core.WeatherThunder)
	if len(parts) != WeatherMaxParticles {
		t.Fatalf("粒子数 = %d，想要固定上限 %d", len(parts), WeatherMaxParticles)
	}
	for index, part := range parts {
		center := part.transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3()
		offset := center.Sub(cam)
		if math.Abs(float64(offset.X())) > 12.5 || math.Abs(float64(offset.Z())) > 18.5 ||
			offset.Y() > 10.5 || offset.Y() < -10.5 {
			t.Fatalf("粒子 %d 偏移 %v 超出有界体", index, offset)
		}
	}
}

// TestBuildWeatherPartsSteadyFrameZeroAlloc 部件级稳定天气帧零分配：
// 复用调用方缓冲时部件构建与字节编码都不分配（spec 稳定帧零分配）。
// 生产入口（`EncodeWeatherInstances`/`EncodeWeatherState`）的断言见
// `weather_streams_test.go`。
func TestBuildWeatherPartsSteadyFrameZeroAlloc(t *testing.T) {
	cam := mgl32.Vec3{0, 40, 0}
	parts := BuildWeatherParts(make([]avatarPart, 0, WeatherMaxParticles), cam, 0, 99, core.WeatherRain)
	buf := make([]byte, WeatherMaxParticles*avatarInstanceBytes)
	encodeAvatarPartsInto(buf, parts)
	allocs := testing.AllocsPerRun(20, func() {
		reused := BuildWeatherParts(parts[:0], cam, 0, 99, core.WeatherRain)
		encodeAvatarPartsInto(buf, reused)
	})
	if allocs != 0 {
		t.Fatalf("稳定天气帧分配 = %v，想要 0", allocs)
	}
}
