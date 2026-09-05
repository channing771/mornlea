package render

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func scatterTestDrops(count int, dimension core.DimensionID, block core.BlockPos) []ItemDrop {
	drops := make([]ItemDrop, count)
	for index := range drops {
		item := core.ItemStone
		if index%2 == 1 {
			item = core.ItemRawBeef
		}
		drops[index] = ItemDrop{
			ID: core.DropID{
				Dimension: dimension, Chunk: block.Chunk(), Slot: uint8(index), Generation: uint32(index + 1),
			},
			Block: block, Item: item, SupportY: float32(block.Y), HasSupport: true,
		}
	}
	return drops
}

func scatterPartCenter(part avatarPart) [3]float32 {
	return [3]float32{part.transform[12], part.transform[13], part.transform[14]}
}

func scatterScale(part avatarPart, item core.ItemID) float32 {
	base := dropCubeSize
	if itemDropFlake(item) {
		base = dropFlakeSize
	}
	return dropPartScale(part) / base
}

func scatterAxisGap(leftMin, leftMax, rightMin, rightMax float32) float32 {
	return max(rightMin-leftMax, leftMin-rightMax)
}

// TestDropScatterMixedGroupsStayInsideAndApart 钉住真实矩阵几何：1/4/16/32
// 个方块与薄片混合堆在全部自转相位中不越原方块，任意两堆至少沿一个 XZ
// 轴保持净空；最密组仍保留约 0.012 格的理论余量。
func TestDropScatterMixedGroupsStayInsideAndApart(t *testing.T) {
	cases := []struct {
		count int
		block core.BlockPos
	}{
		{count: 1, block: core.BlockPos{X: 0, Y: 3, Z: 0}},
		{count: 4, block: core.BlockPos{X: -17, Y: 3, Z: 22}},
		{count: 16, block: core.BlockPos{X: 31, Y: 3, Z: -33}},
		{count: 32, block: core.BlockPos{X: -65, Y: 3, Z: -48}},
	}
	for _, tc := range cases {
		drops := scatterTestDrops(tc.count, core.Overworld, tc.block)
		falls := &DropFalls{}
		for tick := uint64(0); tick < dropSpinPeriod; tick++ {
			parts := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
			if len(parts) != tc.count {
				t.Fatalf("数量 %d tick %d 实例=%d", tc.count, tick, len(parts))
			}
			bounds := make([]avatarBounds, len(parts))
			for index, part := range parts {
				bounds[index] = transformedUnitCubeBounds(part.transform)
				if bounds[index].min.X() < float32(tc.block.X)-1e-5 || bounds[index].max.X() > float32(tc.block.X+1)+1e-5 ||
					bounds[index].min.Z() < float32(tc.block.Z)-1e-5 || bounds[index].max.Z() > float32(tc.block.Z+1)+1e-5 {
					t.Fatalf("数量 %d tick %d 堆 %d 越界: %+v", tc.count, tick, index, bounds[index])
				}
			}
			for left := range bounds {
				for right := left + 1; right < len(bounds); right++ {
					gapX := scatterAxisGap(bounds[left].min.X(), bounds[left].max.X(), bounds[right].min.X(), bounds[right].max.X())
					gapZ := scatterAxisGap(bounds[left].min.Z(), bounds[left].max.Z(), bounds[right].min.Z(), bounds[right].max.Z())
					if max(gapX, gapZ) < 0.0115 {
						t.Fatalf("数量 %d tick %d 堆 %d/%d XZ 净空=%v/%v", tc.count, tick, left, right, gapX, gapZ)
					}
				}
			}
		}
		if tc.count == 1 {
			center := scatterPartCenter(falls.buildItemDropParts(nil, 0, drops, dropFallTestGravity, dropFallTestTerminal)[0])
			if math.Abs(float64(center[0]-(float32(tc.block.X)+0.5))) < 0.008 ||
				math.Abs(float64(center[2]-(float32(tc.block.Z)+0.5))) < 0.008 {
				t.Fatalf("单堆偏移不明显: center=%v", center)
			}
		}
	}
}

// TestDropScatterUsesBoundedDensityScaleAndAuxiliaryLayer 钉住各密度档的缩放，
// 并确认后 16 堆获得只用于可见性的辅助层而不复用 XZ 位置。
func TestDropScatterUsesBoundedDensityScaleAndAuxiliaryLayer(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  float32
	}{
		{count: 1, want: 1},
		{count: 4, want: 0.75298804},
		{count: 16, want: 0.37649402},
		{count: 32, want: 0.25099602},
	} {
		drops := scatterTestDrops(tc.count, core.Overworld, core.BlockPos{X: 2, Y: 3, Z: -4})
		parts := (&DropFalls{}).buildItemDropParts(nil, 0, drops, dropFallTestGravity, dropFallTestTerminal)
		for index, part := range parts {
			if got := scatterScale(part, drops[index].Item); math.Abs(float64(got-tc.want)) > 2e-5 {
				t.Fatalf("数量 %d 堆 %d scale=%v，想要 %v", tc.count, index, got, tc.want)
			}
		}
	}

	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := scatterTestDrops(32, core.Overworld, block)
	for index := range drops {
		drops[index].Item = core.ItemStone
	}
	parts := (&DropFalls{}).buildItemDropParts(nil, 0, drops, dropFallTestGravity, dropFallTestTerminal)
	if rise := scatterPartCenter(parts[16])[1] - scatterPartCenter(parts[0])[1]; rise < 0.13 {
		t.Fatalf("第二层抬升=%v，想要计入薄片全高与两倍浮动净空", rise)
	}
	seen := make(map[[2]float32]struct{}, len(parts))
	for index, part := range parts {
		center := scatterPartCenter(part)
		key := [2]float32{center[0], center[2]}
		if _, exists := seen[key]; exists {
			t.Fatalf("堆 %d 复用了 XZ 位置 %v", index, key)
		}
		seen[key] = struct{}{}
	}

	for index := range drops {
		drops[index].Item = core.ItemRawBeef
	}
	lowerTopMax := float32(-math.MaxFloat32)
	upperBottomMin := float32(math.MaxFloat32)
	for tick := uint64(0); tick < dropFloatPeriod; tick++ {
		phaseParts := (&DropFalls{}).buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal)
		lower := transformedUnitCubeBounds(phaseParts[0].transform)
		upper := transformedUnitCubeBounds(phaseParts[16].transform)
		lowerTopMax = max(lowerTopMax, lower.max.Y())
		upperBottomMin = min(upperBottomMin, upper.min.Y())
	}
	if gap := upperBottomMin - lowerTopMax; gap < 0.019 || gap > 0.021 {
		t.Fatalf("跨全部浮动相位的层间净空=%v，想要约 0.02", gap)
	}
}

// TestDropScatterReorderIsByteStableAndKeepsCallerInput 钉住排序只使用固定
// scratch：同一集合反向输入仍逐字节一致，且不同维度的同坐标组各自按四堆布局。
func TestDropScatterReorderIsByteStableAndKeepsCallerInput(t *testing.T) {
	block := core.BlockPos{X: -2, Y: 5, Z: 7}
	drops := append(
		scatterTestDrops(4, core.Overworld+1, block),
		scatterTestDrops(4, core.Overworld, block)...,
	)
	original := append([]ItemDrop(nil), drops...)
	reversed := append([]ItemDrop(nil), drops...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	first := avatarPartBytes((&DropFalls{}).buildItemDropParts(nil, 17, drops, dropFallTestGravity, dropFallTestTerminal))
	second := avatarPartBytes((&DropFalls{}).buildItemDropParts(nil, 17, reversed, dropFallTestGravity, dropFallTestTerminal))
	if !bytes.Equal(first, second) {
		t.Fatal("同一集合反向输入后的实例字节不一致")
	}
	if !reflect.DeepEqual(drops, original) {
		t.Fatal("散落排序改写了调用者切片")
	}
	parts := (&DropFalls{}).buildItemDropParts(nil, 17, drops, dropFallTestGravity, dropFallTestTerminal)
	for index, part := range parts {
		item := core.ItemStone
		if index%2 == 1 {
			item = core.ItemRawBeef
		}
		if got := scatterScale(part, item); math.Abs(float64(got-0.75298804)) > 2e-5 {
			t.Fatalf("维度分组后堆 %d scale=%v，想要四堆档", index, got)
		}
	}
}

// TestDropScatterHiddenDeathDropReservesCell 钉住死亡前期隐藏堆仍参与分组与
// 槽位分配：显现时不会因先前不可见而挤占已有堆位置。
func TestDropScatterHiddenDeathDropReservesCell(t *testing.T) {
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := scatterTestDrops(2, core.Overworld, block)
	drops[0].DeathTick = 100
	withHidden := &DropFalls{}
	parts := withHidden.buildItemDropParts(nil, 109, drops, dropFallTestGravity, dropFallTestTerminal)
	if len(parts) != 1 {
		t.Fatalf("死亡前期实例=%d，想要只显示普通堆", len(parts))
	}
	alone := (&DropFalls{}).buildItemDropParts(nil, 109, drops[1:], dropFallTestGravity, dropFallTestTerminal)
	if scatterPartCenter(parts[0]) == scatterPartCenter(alone[0]) {
		t.Fatal("隐藏堆未占槽，普通堆退回单堆位置")
	}
}

// TestDropScatterDeathLinkedFallStartsWhenFirstVisible 钉住隐藏期只占 XZ 槽位、
// 不累计下落年龄：首次可见帧仍位于出生底面，下一帧才开始原有重力积分。
func TestDropScatterDeathLinkedFallStartsWhenFirstVisible(t *testing.T) {
	block := core.BlockPos{X: 0, Y: 5, Z: 0}
	drop := scatterTestDrops(1, core.Overworld, block)[0]
	drop.DeathTick = 100
	drop.SupportY = 0
	drops := []ItemDrop{drop}
	falls := &DropFalls{}
	for _, tick := range []uint64{100, 109} {
		if got := falls.buildItemDropParts(nil, tick, drops, dropFallTestGravity, dropFallTestTerminal); len(got) != 0 {
			t.Fatalf("隐藏 tick %d 实例=%d，想要 0", tick, len(got))
		}
	}

	wantCenter := func(tick uint64, unit, fallen float32) float32 {
		phase := dropAnimationPhase(tick, drop.ID)
		bob := dropFloatHeight * unit
		return float32(block.Y) - fallen + dropCubeSize*unit/2 + dropScatterFloorGap +
			bob + bob*float32(math.Sin(float64(phase.float)))
	}
	first := falls.buildItemDropParts(nil, 110, drops, dropFallTestGravity, dropFallTestTerminal)
	if len(first) != 1 {
		t.Fatalf("首次可见实例=%d，想要 1", len(first))
	}
	assertCenterYNear(t, 110, dropPartCenterY(first[0]), wantCenter(110, 1.0/11, 0))
	next := falls.buildItemDropParts(nil, 111, drops, dropFallTestGravity, dropFallTestTerminal)
	assertCenterYNear(t, 111, dropPartCenterY(next[0]), wantCenter(111, 2.0/11, 0.08))
}

// TestDropScatterBottomAnchorsToSupportDuringScaleIn 钉住实际缩放半高与非负
// 浮动从支撑顶面锚定；异常高于生成面的有效支撑也不得被薄片穿过。
func TestDropScatterBottomAnchorsToSupportDuringScaleIn(t *testing.T) {
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	high := scatterTestDrops(1, core.Overworld, block)
	high[0].SupportY = 4
	highBounds := transformedUnitCubeBounds((&DropFalls{}).buildItemDropParts(nil, 0, high, dropFallTestGravity, dropFallTestTerminal)[0].transform)
	if highBounds.min.Y() < 4.019 || highBounds.min.Y() > 4.181 {
		t.Fatalf("高支撑底面=%v，想要支撑上 0.02..0.18", highBounds.min.Y())
	}

	linked := scatterTestDrops(1, core.Overworld, block)
	linked[0].DeathTick = 100
	first := transformedUnitCubeBounds((&DropFalls{}).buildItemDropParts(nil, 110, linked, dropFallTestGravity, dropFallTestTerminal)[0].transform)
	if first.min.Y() < 3.019 || first.min.Y() > 3.04 {
		t.Fatalf("渐显首帧底面=%v，想要从支撑附近长出", first.min.Y())
	}
	full := transformedUnitCubeBounds((&DropFalls{}).buildItemDropParts(nil, 120, linked, dropFallTestGravity, dropFallTestTerminal)[0].transform)
	if full.min.Y() < 3.019 || full.min.Y() > 3.181 {
		t.Fatalf("渐显完成底面=%v，想要保留非负浮动", full.min.Y())
	}

	mixed := scatterTestDrops(32, core.Overworld, block)
	for index := range mixed {
		if index%2 == 0 {
			mixed[index].DeathTick = 100
		}
	}
	mixedParts := (&DropFalls{}).buildItemDropParts(nil, 110, mixed, dropFallTestGravity, dropFallTestTerminal)
	if len(mixedParts) != len(mixed) {
		t.Fatalf("渐显混合组实例=%d，想要 %d", len(mixedParts), len(mixed))
	}
	for left := range mixedParts {
		leftBounds := transformedUnitCubeBounds(mixedParts[left].transform)
		for right := left + 1; right < len(mixedParts); right++ {
			rightBounds := transformedUnitCubeBounds(mixedParts[right].transform)
			gapX := scatterAxisGap(leftBounds.min.X(), leftBounds.max.X(), rightBounds.min.X(), rightBounds.max.X())
			gapZ := scatterAxisGap(leftBounds.min.Z(), leftBounds.max.Z(), rightBounds.min.Z(), rightBounds.max.Z())
			if max(gapX, gapZ) < 0.0115 {
				t.Fatalf("渐显混合堆 %d/%d XZ 净空=%v/%v", left, right, gapX, gapZ)
			}
		}
	}
}

// TestDropScatterReusesFixedScratchWithoutAllocating 钉住预热后的最大镜像输入
// 不为排序或分组新建临时切片。
func TestDropScatterReusesFixedScratchWithoutAllocating(t *testing.T) {
	drops := testItemDrops(maxItemDrops)
	parts := make([]avatarPart, 0, maxItemDrops)
	falls := &DropFalls{}
	for range 2 {
		falls.buildItemDropParts(parts[:0], 0, drops, dropFallTestGravity, dropFallTestTerminal)
	}
	if allocations := testing.AllocsPerRun(20, func() {
		falls.buildItemDropParts(parts[:0], 0, drops, dropFallTestGravity, dropFallTestTerminal)
	}); allocations != 0 {
		t.Fatalf("最大输入稳定帧分配=%v，想要 0", allocations)
	}
}
