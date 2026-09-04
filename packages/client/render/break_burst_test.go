package render

import (
	"bytes"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

func breakTestDrop(slot uint8, block core.BlockPos, item core.ItemID) ItemDrop {
	return ItemDrop{
		ID: core.DropID{
			Dimension:  core.Overworld,
			Chunk:      core.ChunkPos{X: 0, Z: 0},
			Slot:       slot,
			Generation: 1,
		},
		Block: block,
		Item:  item,
	}
}

func breakTestOrigin(block core.BlockPos) mgl32.Vec3 {
	return mgl32.Vec3{float32(block.X) + 0.5, float32(block.Y) + 0.5, float32(block.Z) + 0.5}
}

func breakPartCenter(part avatarPart) mgl32.Vec3 {
	return mgl32.Vec3{part.transform[12], part.transform[13], part.transform[14]}
}

// TestBreakBurstSpawnsEightSameColorParts 钉住新掉落物触发一次同色 burst：
// 8 粒实心小立方体，颜色与物品基色一致，首帧全部位于方块中心。
func TestBreakBurstSpawnsEightSameColorParts(t *testing.T) {
	tracker := &BreakBursts{}
	block := core.BlockPos{X: 4, Y: 2, Z: -1}
	drop := breakTestDrop(3, block, core.ItemDirt)
	parts := tracker.BuildParts(nil, 10, []ItemDrop{drop})
	if len(parts) != 8 {
		t.Fatalf("burst 实例数 = %d，想要 8", len(parts))
	}
	wantColor := ItemColor(core.ItemDirt)
	wantOrigin := breakTestOrigin(block)
	for index, part := range parts {
		if part.color != wantColor {
			t.Fatalf("粒子 %d 颜色 = %v，想要泥土基色 %v", index, part.color, wantColor)
		}
		if part.material != avatarMaterialSolid {
			t.Fatalf("粒子 %d 材质 = %v，想要实心哨兵 %v", index, part.material, avatarMaterialSolid)
		}
		if got := breakPartCenter(part); got != wantOrigin {
			t.Fatalf("粒子 %d 首帧位置 = %v，想要方块中心 %v", index, got, wantOrigin)
		}
	}
}

// TestBreakBurstVelocitiesAreDistinctAndUpperHemisphere 钉住 8 粒初速两两不同
// 且全部指向上半球：方向完全由掉落物 ID 散列派生，不依赖随机与时间。
func TestBreakBurstVelocitiesAreDistinctAndUpperHemisphere(t *testing.T) {
	id := core.DropID{Dimension: core.Overworld, Slot: 3, Generation: 1}
	seen := make(map[mgl32.Vec3]struct{}, 8)
	for index := range 8 {
		velocity := breakBurstVelocity(id, index)
		if velocity.Y() <= 0 {
			t.Fatalf("粒子 %d 初速 Y = %v，想要上半球（>0）", index, velocity.Y())
		}
		if _, dup := seen[velocity]; dup {
			t.Fatalf("粒子 %d 初速 %v 与之前粒子重复", index, velocity)
		}
		seen[velocity] = struct{}{}
	}
}

// TestBreakBurstTrajectoryFollowsFixedGravity 钉住位置公式「起点 + v·t − g·t²」：
// 年龄 1 时位置恰为起点加初速再扣纵向重力项。
func TestBreakBurstTrajectoryFollowsFixedGravity(t *testing.T) {
	tracker := &BreakBursts{}
	block := core.BlockPos{X: 1, Y: 5, Z: 2}
	drop := breakTestDrop(3, block, core.ItemDirt)
	tracker.BuildParts(nil, 10, []ItemDrop{drop})
	parts := tracker.BuildParts(nil, 11, []ItemDrop{drop})
	if len(parts) != 8 {
		t.Fatalf("burst 实例数 = %d，想要 8", len(parts))
	}
	origin := breakTestOrigin(block)
	for index, part := range parts {
		want := origin.Add(breakBurstVelocity(drop.ID, index))
		want[1] -= breakBurstGravity
		assertVec3Near(t, breakPartCenter(part), want)
	}
}

// TestBreakBurstSizeShrinksWithAge 钉住粒子尺寸随年龄收缩：首帧 0.09，
// 半程恰为一半，到期前趋近于零。
func TestBreakBurstSizeShrinksWithAge(t *testing.T) {
	tracker := &BreakBursts{}
	drop := breakTestDrop(3, core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemDirt)
	drops := []ItemDrop{drop}
	first := tracker.BuildParts(nil, 10, drops)
	if got := first[0].transform[0]; got != breakBurstCubeSize {
		t.Fatalf("首帧边长 = %v，想要 %v", got, breakBurstCubeSize)
	}
	half := tracker.BuildParts(nil, 20, drops)
	if got, want := half[0].transform[0], breakBurstCubeSize*0.5; got != want {
		t.Fatalf("年龄 10 边长 = %v，想要 %v", got, want)
	}
}

// TestBreakBurstExpiresAfter20Ticks 钉住 20 tick 寿命：年龄 19 仍编码，
// 年龄 20 起停止编码。
func TestBreakBurstExpiresAfter20Ticks(t *testing.T) {
	tracker := &BreakBursts{}
	drops := []ItemDrop{breakTestDrop(3, core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemDirt)}
	tracker.BuildParts(nil, 100, drops)
	if got := tracker.BuildParts(nil, 119, drops); len(got) != 8 {
		t.Fatalf("年龄 19 实例数 = %d，想要 8", len(got))
	}
	if got := tracker.BuildParts(nil, 120, drops); len(got) != 0 {
		t.Fatalf("年龄 20 实例数 = %d，想要 0", len(got))
	}
}

// TestBreakBurstEncodingIsDeterministic 钉住同输入逐帧一致：同一 tracker 重复
// 编码、新 tracker 同输入编码的实例字节完全一致；tick 推进则编码变化。
func TestBreakBurstEncodingIsDeterministic(t *testing.T) {
	drops := []ItemDrop{
		breakTestDrop(1, core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemDirt),
		breakTestDrop(2, core.BlockPos{X: 5, Y: 3, Z: 5}, core.ItemGrass),
	}
	tracker := &BreakBursts{}
	first := avatarPartBytes(tracker.BuildParts(nil, 7, drops))
	repeat := avatarPartBytes(tracker.BuildParts(nil, 7, drops))
	if !bytes.Equal(first, repeat) {
		t.Fatal("同一 tracker 同一 tick 两次编码字节不一致")
	}
	fresh := avatarPartBytes((&BreakBursts{}).BuildParts(nil, 7, drops))
	if !bytes.Equal(first, fresh) {
		t.Fatal("不同 tracker 同输入编码字节不一致")
	}
	if later := avatarPartBytes(tracker.BuildParts(nil, 8, drops)); bytes.Equal(first, later) {
		t.Fatal("tick 推进后编码字节未变化")
	}
}

// TestBreakBurstEvictsOldestWhenTableFull 钉住双重定界：跟踪表只留最近 16 个
// 掉落物 ID，第 17 个挤掉最老；17 个全活 burst 的编码总数不超过 64 实例。
func TestBreakBurstEvictsOldestWhenTableFull(t *testing.T) {
	tracker := &BreakBursts{}
	var drops []ItemDrop
	for tick := uint64(0); tick < 17; tick++ {
		drops = append(drops, breakTestDrop(uint8(tick),
			core.BlockPos{X: int32(tick), Y: 3, Z: 0}, core.ItemDirt))
		tracker.BuildParts(nil, tick, drops)
	}
	if got := len(tracker.entries); got != 16 {
		t.Fatalf("跟踪表 = %d 项，想要 16", got)
	}
	for _, entry := range tracker.entries {
		if entry.id.Slot == 0 {
			t.Fatal("最老的 burst 未被淘汰")
		}
	}
	if got := tracker.BuildParts(nil, 16, drops); len(got) != 64 {
		t.Fatalf("编码实例数 = %d，想要上限 64", len(got))
	}
}

// TestBreakBurstRemovedDropDeletesEntry 钉住掉落物消失即删：条目清空后无编码；
// 同 ID 再次出现视为全新 burst。
func TestBreakBurstRemovedDropDeletesEntry(t *testing.T) {
	tracker := &BreakBursts{}
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := []ItemDrop{breakTestDrop(3, block, core.ItemDirt)}
	if got := tracker.BuildParts(nil, 5, drops); len(got) != 8 {
		t.Fatalf("首现实例数 = %d，想要 8", len(got))
	}
	if got := tracker.BuildParts(nil, 6, nil); len(got) != 0 {
		t.Fatalf("掉落物消失后实例数 = %d，想要 0", len(got))
	}
	if len(tracker.entries) != 0 {
		t.Fatalf("掉落物消失后跟踪表 = %d 项，想要 0", len(tracker.entries))
	}
	reburst := tracker.BuildParts(nil, 7, drops)
	if len(reburst) != 8 {
		t.Fatalf("重现实例数 = %d，想要全新 burst 的 8", len(reburst))
	}
	wantOrigin := breakTestOrigin(block)
	for index, part := range reburst {
		if got := breakPartCenter(part); got != wantOrigin {
			t.Fatalf("重现粒子 %d 位置 = %v，想要方块中心 %v", index, got, wantOrigin)
		}
	}
}

// TestBreakBurstDoesNotRetriggerWhilePresent 钉住存续期间只 burst 一次：
// 25 tick 内恰有一帧全部粒子位于起点，20 tick 后自然消失。
func TestBreakBurstDoesNotRetriggerWhilePresent(t *testing.T) {
	tracker := &BreakBursts{}
	block := core.BlockPos{X: 0, Y: 3, Z: 0}
	drops := []ItemDrop{breakTestDrop(3, block, core.ItemDirt)}
	origin := breakTestOrigin(block)
	originFrames := 0
	for tick := uint64(5); tick <= 30; tick++ {
		parts := tracker.BuildParts(nil, tick, drops)
		age := tick - 5
		if age < 20 {
			if len(parts) != 8 {
				t.Fatalf("tick %d 实例数 = %d，想要 8", tick, len(parts))
			}
			atOrigin := true
			for _, part := range parts {
				if breakPartCenter(part) != origin {
					atOrigin = false
					break
				}
			}
			if atOrigin {
				originFrames++
			}
			if age >= 1 && atOrigin {
				t.Fatalf("tick %d 重复触发了首帧 burst", tick)
			}
			continue
		}
		if len(parts) != 0 {
			t.Fatalf("tick %d 实例数 = %d，想要到期消失", tick, len(parts))
		}
	}
	if originFrames != 1 {
		t.Fatalf("起点帧数 = %d，想要恰好 1", originFrames)
	}
}

// TestBreakBurstEvictedDropDoesNotReburstWhilePresent 钉住淘汰抑制：17 个常驻
// 掉落物每个恰 burst 一次；表满后被挤掉的 ID 即使仍存续也不逐帧重 burst。
func TestBreakBurstEvictedDropDoesNotReburstWhilePresent(t *testing.T) {
	tracker := &BreakBursts{}
	var drops []ItemDrop
	var origins []mgl32.Vec3
	for tick := uint64(0); tick < 17; tick++ {
		block := core.BlockPos{X: int32(tick), Y: 3, Z: 0}
		drops = append(drops, breakTestDrop(uint8(tick), block, core.ItemDirt))
		origins = append(origins, breakTestOrigin(block))
	}
	originFrames := make([]int, len(origins))
	countAtOrigin := func(parts []avatarPart, origin mgl32.Vec3) int {
		count := 0
		for _, part := range parts {
			if breakPartCenter(part) == origin {
				count++
			}
		}
		return count
	}
	for tick := uint64(0); tick < 40; tick++ {
		var frame []ItemDrop
		if tick < 17 {
			frame = drops[:tick+1]
		} else {
			frame = drops
		}
		parts := tracker.BuildParts(nil, tick, frame)
		for index, origin := range origins {
			switch got := countAtOrigin(parts, origin); {
			case tick < 17 && index == int(tick):
				if got != 8 {
					t.Fatalf("tick %d 新掉落物起点粒子 = %d，想要 8", tick, got)
				}
				originFrames[index]++
			case got != 0:
				t.Fatalf("tick %d 起点 %v 粒子 = %d，想要 0（重复 burst）", tick, origin, got)
			}
		}
	}
	for index, frames := range originFrames {
		if frames != 1 {
			t.Fatalf("掉落物 %d 起点帧数 = %d，想要恰好 1", index, frames)
		}
	}
	if got := len(tracker.entries); got != 16 {
		t.Fatalf("跟踪表 = %d 项，想要 16", got)
	}
}

// TestBreakBurstShuffledInputEncodesIdentically 钉住输入规范化：同集合乱序输入
// 在新跟踪表上逐字节一致，不依赖调用方顺序。
func TestBreakBurstShuffledInputEncodesIdentically(t *testing.T) {
	drops := []ItemDrop{
		breakTestDrop(5, core.BlockPos{X: 9, Y: 3, Z: 1}, core.ItemDirt),
		breakTestDrop(1, core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemGrass),
		breakTestDrop(9, core.BlockPos{X: -4, Y: 6, Z: 2}, core.ItemStone),
		breakTestDrop(3, core.BlockPos{X: 2, Y: 4, Z: -3}, core.ItemCoal),
	}
	shuffled := append([]ItemDrop(nil), drops...)
	slices.Reverse(shuffled)
	first := avatarPartBytes((&BreakBursts{}).BuildParts(nil, 7, drops))
	second := avatarPartBytes((&BreakBursts{}).BuildParts(nil, 7, shuffled))
	if !bytes.Equal(first, second) {
		t.Fatal("乱序输入编码字节不一致，想要与输入顺序无关")
	}
}

// TestBreakBurstHashMixesDimension 钉住维度进入散列：除维度外完全相同的 ID
// 初速不同，跨维度同槽位复用的掉落物轨迹不重合。
func TestBreakBurstHashMixesDimension(t *testing.T) {
	base := core.DropID{Dimension: core.Overworld, Chunk: core.ChunkPos{X: 1, Z: -2}, Slot: 3, Generation: 1}
	other := base
	other.Dimension = core.Overworld + 1
	if breakBurstHash(base) == breakBurstHash(other) {
		t.Fatal("仅维度不同的 ID 散列相同，想要维度混入散列")
	}
	if breakBurstVelocity(base, 0) == breakBurstVelocity(other, 0) {
		t.Fatal("仅维度不同的 ID 初速相同，想要轨迹区分")
	}
}

// 物品都不编码、不建条目，裂纹 overlay 不受影响（本包无任何裂纹状态改动）。
func TestBreakBurstSkipsUnknownItemsAndEmpty(t *testing.T) {
	tracker := &BreakBursts{}
	if got := tracker.BuildParts(nil, 10, nil); len(got) != 0 {
		t.Fatalf("空输入实例数 = %d，想要 0", len(got))
	}
	unknown := []ItemDrop{breakTestDrop(3, core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemID(4242))}
	if got := tracker.BuildParts(nil, 10, unknown); len(got) != 0 {
		t.Fatalf("未知物品实例数 = %d，想要 0", len(got))
	}
	if len(tracker.entries) != 0 {
		t.Fatalf("未知物品建条目 = %d，想要 0", len(tracker.entries))
	}
}
