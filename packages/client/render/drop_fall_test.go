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
		parts := falls.buildItemDropParts(nil, tick, drops)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), dropFallBaseY(block, tick, drops[0].ID))
	}
}

// TestDropFallsAtConstantRateAboveSupport 钉住远支撑上方的恒速下落：
// 未着陆前每 tick 恰好下降 0.15 格。
func TestDropFallsAtConstantRateAboveSupport(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 13, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, 3, true)}
	for tick := uint64(0); tick <= 10; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		want := dropFallWant(block, tick, drops[0].ID, float32(tick)*dropFallBlocksPerTick)
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), want)
	}
}

// TestDropLandsOnSupportAndKeepsFloating 钉住 3 格下落着陆：偏移按
// min（年龄×0.15，生成高度−支撑高度）钳制，着陆后正弦浮动与自转继续。
func TestDropLandsOnSupportAndKeepsFloating(t *testing.T) {
	falls := &DropFalls{}
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := []ItemDrop{fallTestDrop(fallTestID(3), block, core.ItemDirt, 0, true)}
	const maxFall = float32(3)
	for tick := uint64(0); tick <= 30; tick++ {
		parts := falls.buildItemDropParts(nil, tick, drops)
		if len(parts) != 1 {
			t.Fatalf("tick %d 实例数 = %d，想要 1", tick, len(parts))
		}
		fallen := float32(tick) * dropFallBlocksPerTick
		if fallen > maxFall {
			fallen = maxFall
		}
		want := dropFallWant(block, tick, drops[0].ID, fallen)
		assertCenterYNear(t, tick, dropPartCenterY(parts[0]), want)
	}
	landed := avatarPartBytes(falls.buildItemDropParts(nil, 25, drops))
	floating := avatarPartBytes(falls.buildItemDropParts(nil, 26, drops))
	if bytes.Equal(landed, floating) {
		t.Fatal("着陆后两帧字节一致，想要浮动与自转继续")
	}
	if got := dropPartCenterY(falls.buildItemDropParts(nil, 26, drops)[0]); got >= float32(block.Y)+dropBaseAltitude {
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
		parts := falls.buildItemDropParts(nil, tick, drops)
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
	first := avatarPartBytes(falls.buildItemDropParts(nil, 7, drops))
	repeat := avatarPartBytes(falls.buildItemDropParts(nil, 7, drops))
	if !bytes.Equal(first, repeat) {
		t.Fatal("同一 first-seen 表同一 tick 两次编码字节不一致")
	}
	fresh := avatarPartBytes((&DropFalls{}).buildItemDropParts(nil, 7, drops))
	if !bytes.Equal(first, fresh) {
		t.Fatal("不同 first-seen 表同输入编码字节不一致")
	}
	if later := avatarPartBytes(falls.buildItemDropParts(nil, 8, drops)); bytes.Equal(first, later) {
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
	falls.buildItemDropParts(nil, 0, drops)
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
