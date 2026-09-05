package render

import (
	"bytes"
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// fallTestDrop 构造带支撑信息的掉落物渲染输入：支撑缺席时按生成高度保持。
func fallTestDrop(id core.DropID, block core.BlockPos, item core.ItemID, supportY float32, hasSupport bool) ItemDrop {
	return ItemDrop{ID: id, Block: block, Item: item, SupportY: supportY, HasSupport: hasSupport}
}

func fallTestID(slot uint8) core.DropID {
	return core.DropID{Dimension: core.Overworld, Chunk: core.ChunkPos{}, Slot: slot, Generation: 1}
}

// dropFallTestGravity 是重力积分测试的显式传参：生产取
// `physics.ActiveTunables` 的 Gravity/TerminalFallSpeed（默认 32/78.4），
// `render` 不读全局 tunables，测试传显式值。
const (
	dropFallTestGravity  = float32(32)
	dropFallTestTerminal = float32(78.4)
	dropFallTestDT       = float32(0.05)
)

// testFallWant 是测试侧的独立下落 oracle：逐 tick 半隐式欧拉（`v += g·dt`、
// 终端钳制、`y -= v·dt`），与实现闭式解独立推导、逐位对照。
func testFallWant(age uint64, gravity, terminal float32) float32 {
	var velocity, fallen float32
	for tick := uint64(0); tick < age; tick++ {
		velocity += gravity * dropFallTestDT
		if velocity > terminal {
			velocity = terminal
		}
		fallen += velocity * dropFallTestDT
	}
	return fallen
}

// dropFallBaseY 返回无下落时的呈现基准高度：生成中心高度 + 正弦浮动，
// 下落偏移只在此基础上扣除，浮动本身永不停。
func dropFallBaseY(block core.BlockPos, serverTick uint64, id core.DropID) float32 {
	return float32(block.Y) + dropBaseAltitude + dropFallSine(serverTick, id)
}

// dropFallSine 返回指定 tick 的正弦浮动项：期望高度按与实现相同的结合顺序
// （基准 − 偏移 + 浮动）组装，避免 float32 结合律引入的末位漂移。
func dropFallSine(serverTick uint64, id core.DropID) float32 {
	phase := dropAnimationPhase(serverTick, id)
	return dropFloatHeight * float32(math.Sin(float64(phase.float)))
}

// dropFallWant 按实现结合顺序组装期望中心高度。
func dropFallWant(block core.BlockPos, serverTick uint64, id core.DropID, fallen float32) float32 {
	return float32(block.Y) + dropBaseAltitude - fallen + dropFallSine(serverTick, id)
}

func dropPartCenterY(part avatarPart) float32 {
	return part.transform[13]
}

// assertCenterYNear 按既有 `assertVec3Near` 的 1e-5 口径比对高度：公式在
// 不同结合顺序下有 1-ulp 级浮点噪声，断言行为不钉二进制位（逐字节一致性
// 另由确定性测试覆盖）。
func assertCenterYNear(t *testing.T, tick uint64, got, want float32) {
	t.Helper()
	if diff := got - want; diff > 1e-5 || diff < -1e-5 {
		t.Fatalf("tick %d 中心 Y = %v，想要 %v", tick, got, want)
	}
}

// TestDropFallKeepsSpawnHeightWithoutSupport 钉住无支撑信息时保持生成高度：
// 镜像无数据或支撑超深时不下落，呈现高度恒为生成基准（含正弦浮动）。
func TestDropFallKeepsSpawnHeightWithoutSupport(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, 0, false)}
	for tick := uint64(0); tick <= 30; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), dropFallBaseY(block, tick, drops[0].ID))
	}
}

// TestDropFallAcceleratesAboveSupport 钉住远支撑上方的重力加速下落：每 tick
// `v += g·dt`、终端钳制、`y -= v·dt`（`dt=0.05`，与角色重力同形），增量递增。
func TestDropFallAcceleratesAboveSupport(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 13, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, 3, true)}
	for tick := uint64(0); tick <= 10; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		want := dropFallWant(block, tick, drops[0].ID, testFallWant(tick, dropFallTestGravity, dropFallTestTerminal))
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), want)
	}
}

// TestDropFallIncrementsGrowWhileFalling 钉住逐 tick 下降增量递增：重力积分
// 未着陆、未达终端时每 tick 比上一 tick 多落 `g·dt²`。
func TestDropFallIncrementsGrowWhileFalling(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 13, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, -100, true)}
	heights := make([]float32, 0, 6)
	for tick := uint64(0); tick <= 5; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		heights = append(heights, dropPartCenterY(parts[0])-dropFallSine(tick, drops[0].ID))
	}
	for tick := 2; tick <= 5; tick++ {
		prev := heights[tick-1] - heights[tick-2]
		curr := heights[tick] - heights[tick-1]
		// 下降为负向：后一 tick 多落约 `g·dt²=0.08`，容差取浮动扣除后的 1e-4。
		if diff := curr - prev; diff > -0.07 || diff < -0.09 {
			t.Fatalf("tick %d 增量差 = %v，想要约 -0.08（递增加速）", tick, diff)
		}
	}
}

// TestDropFallTerminalVelocityClampsInDeepShaft 钉住深竖井终端钳制：小终端
// 下增量收敛为 `terminal·dt`，不再加速。
func TestDropFallTerminalVelocityClampsInDeepShaft(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 50, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, -100, true)}
	const gravity = float32(32)
	const terminal = float32(2)
	heights := make([]float32, 0, 12)
	for tick := uint64(0); tick <= 11; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops, gravity, terminal)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		want := dropFallWant(block, tick, drops[0].ID, testFallWant(tick, gravity, terminal))
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), want)
		heights = append(heights, dropPartCenterY(parts[0])-dropFallSine(tick, drops[0].ID))
	}
	for tick := 4; tick <= 11; tick++ {
		if diff := heights[tick-1] - heights[tick]; diff < 0.09 || diff > 0.11 {
			t.Fatalf("tick %d 终端增量 = %v，想要 terminal·dt=0.1", tick, diff)
		}
	}
}

// TestDropLandsOnSupportAndKeepsFloating 钉住 3 格下落着陆：偏移按重力积分
// `min（积分下落，生成高度−支撑高度）`钳制，约 9 tick 着陆，着陆后正弦浮动
// 与自转继续。
func TestDropLandsOnSupportAndKeepsFloating(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, 0, true)}
	const maxFall = float32(3)
	for tick := uint64(0); tick <= 30; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		fallen := testFallWant(tick, dropFallTestGravity, dropFallTestTerminal)
		if fallen > maxFall {
			fallen = maxFall
		}
		want := dropFallWant(block, tick, drops[0].ID, fallen)
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), want)
	}
	if got := testFallWant(8, dropFallTestGravity, dropFallTestTerminal); got >= maxFall {
		t.Fatalf("tick 8 积分下落 = %v，想要未着陆（<3）", got)
	}
	if got := testFallWant(9, dropFallTestGravity, dropFallTestTerminal); got < maxFall {
		t.Fatalf("tick 9 积分下落 = %v，想要已着陆（≥3）", got)
	}
	landed := avatarPartBytes(falls.buildItemDropParts(nil, 25, drops, dropFallTestGravity, dropFallTestTerminal))
	floating := avatarPartBytes(falls.buildItemDropParts(nil, 26, drops, dropFallTestGravity, dropFallTestTerminal))
	if bytes.Equal(landed, floating) {
		t.Fatal("着陆后两帧字节一致，想要浮动与自转继续")
	}
	if got := dropPartCenterY(falls.buildItemDropParts(nil, 26, drops, dropFallTestGravity, dropFallTestTerminal)[0]); got >= float32(block.Y)+dropBaseAltitude {
		t.Fatalf("着陆后中心 Y = %v，想要守在支撑面上方", got)
	}
}

// TestDropOnGroundHasZeroOffset 钉住有支撑贴地时零偏移：生成高度等于
// 支撑高度，任何年龄都不下落。
func TestDropOnGroundHasZeroOffset(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 1, Z: 3}
	drops := []ItemDrop{fallTestDrop(fallTestID(0), block, core.ItemRawBeef, 1, true)}
	for tick := uint64(0); tick <= 30; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), dropFallBaseY(block, tick, drops[0].ID))
	}
}

// TestDropFallEncodingIsDeterministic 钉住同输入逐帧一致：同一 first-seen
// 表重复编码、新表同输入编码逐字节一致；tick 推进则编码变化。
func TestDropFallEncodingIsDeterministic(t *testing.T) {
	drops := []ItemDrop{
		fallTestDrop(fallTestID(1), core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemDirt, 0, true),
		fallTestDrop(fallTestID(2), core.BlockPos{X: 5, Y: 3, Z: 5}, core.ItemGrass, 0, false),
	}
	falls := &DropFalls{}
	first := avatarPartBytes(falls.buildItemDropParts(nil, 7, drops, dropFallTestGravity, dropFallTestTerminal))
	repeat := avatarPartBytes(falls.buildItemDropParts(nil, 7, drops, dropFallTestGravity, dropFallTestTerminal))
	if !bytes.Equal(first, repeat) {
		t.Fatal("同一 first-seen 表同一 tick 两次编码字节不一致")
	}
	fresh := avatarPartBytes((&DropFalls{}).buildItemDropParts(nil, 7, drops, dropFallTestGravity, dropFallTestTerminal))
	if !bytes.Equal(first, fresh) {
		t.Fatal("不同 first-seen 表同输入编码字节不一致")
	}
	if later := avatarPartBytes(falls.buildItemDropParts(nil, 8, drops, dropFallTestGravity, dropFallTestTerminal)); bytes.Equal(first, later) {
		t.Fatal("tick 推进后编码字节未变化")
	}
}

// TestDropFallsFirstSeenTableIsBounded 钉住首现表定界 128：满表后环形淘汰
// 最老的 ID；被淘汰但仍存续的 ID 下次出现视为新掉落（年龄重起，见实现注释）。
func TestDropFallsFirstSeenTableIsBounded(t *testing.T) {
	falls := &DropFalls{}
	drops := make([]ItemDrop, 0, dropFallMaxTracked+1)
	for slot := range dropFallMaxTracked + 1 {
		id := core.DropID{Dimension: core.Overworld, Chunk: core.ChunkPos{X: int32(slot)}, Slot: 0, Generation: 1}
		drops = append(drops, fallTestDrop(id, core.BlockPos{X: int32(slot), Y: 3, Z: 0}, core.ItemDirt, 0, false))
	}
	falls.buildItemDropParts(nil, 0, drops, dropFallTestGravity, dropFallTestTerminal)
	if got := len(falls.ids); got != dropFallMaxTracked {
		t.Fatalf("首现表 = %d 项，想要定界 %d", got, dropFallMaxTracked)
	}
	if got := falls.age(1, drops[0].ID); got != 0 {
		t.Fatalf("被淘汰 ID 年龄 = %d，想要重起为 0", got)
	}
}

// TestDropFallAgeClampsTickRegression 钉住 tick 回退时年龄钳制为零，不下溢。
func TestDropFallAgeClampsTickRegression(t *testing.T) {
	falls := &DropFalls{}
	id := fallTestID(3)
	if got := falls.age(10, id); got != 0 {
		t.Fatalf("首现年龄 = %d，想要 0", got)
	}
	if got := falls.age(5, id); got != 0 {
		t.Fatalf("回退年龄 = %d，想要钳制为 0", got)
	}
	if got := falls.age(12, id); got != 2 {
		t.Fatalf("推进年龄 = %d，想要 2", got)
	}
}

// TestBreakBurstParticlesClampToSupportFloor 钉住粒子以同一支撑高度为地板：
// 有支撑时粒子中心不低于支撑顶面；无支撑信息时不钳制（下坠穿过原高度）。
func TestBreakBurstParticlesClampToSupportFloor(t *testing.T) {
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	id := fallTestID(3)
	clampedTracker := &BreakBursts{}
	supported := []ItemDrop{fallTestDrop(id, block, core.ItemDirt, 0, true)}
	clampedTracker.BuildParts(nil, 0, supported)
	clamped := clampedTracker.BuildParts(nil, 19, supported)
	if len(clamped) != 8 {
		t.Fatalf("钳制实例数 = %d，想要 8", len(clamped))
	}
	size := breakBurstCubeSize * (1 - float32(19)/float32(breakBurstLifetimeTicks))
	floor := float32(0) + size/2
	for index, part := range clamped {
		if got := breakPartCenter(part)[1]; got < floor {
			t.Fatalf("粒子 %d 中心 Y = %v，想要不低于地板 %v", index, got, floor)
		}
	}
	freeTracker := &BreakBursts{}
	unsupported := []ItemDrop{fallTestDrop(id, block, core.ItemDirt, 0, false)}
	freeTracker.BuildParts(nil, 0, unsupported)
	free := freeTracker.BuildParts(nil, 19, unsupported)
	sunk := false
	for _, part := range free {
		if breakPartCenter(part)[1] < float32(block.Y)+0.5 {
			sunk = true
			break
		}
	}
	if !sunk {
		t.Fatal("无支撑粒子未下坠，想要不钳制")
	}
}
