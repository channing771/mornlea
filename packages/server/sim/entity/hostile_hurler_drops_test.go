package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件锁定掷骨者的死亡掉落契约：骨头 0..2 根（确定性哈希决定数量）+ 弓以
// 1/8 确定性概率额外掉落 1 把，输入为 (worldSeed, 权威 tick, 敌怪 ID)；掉落
// 走既有 `PrepareDropBatch` 预演与提交路径、批量原子（容量不足整批保留，
// 绝不部分掉落）；骨头在本变更内不得获得任何合成配方或熔炼映射。

// hurlerDeathEngine 构造一只生命归零的掷骨者与固定世界时间，掉落数量哈希
// 因此可预先用同一 sampler 求值比对。
func hurlerDeathEngine(t *testing.T, id uint64, worldTime uint64) *Engine {
	t.Helper()
	engine, _ := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	mob := validTestHostile(id)
	mob.Kind = HostileKindBoneThrower
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复掷骨者：%v", err)
	}
	engine.worldTime.Store(worldTime)
	engine.hostiles.entries[0].health = 0
	return engine
}

// settleHurlerDeath 直接驱动死亡结算，返回登记的 pending（调用方负责断言）。
func settleHurlerDeath(engine *Engine) {
	engine.settleHostileDeaths(engine.newMutation())
}

func TestHurlerDeathDropsBonesAndBowDeterministically(t *testing.T) {
	const (
		id        = uint64(21)
		worldTime = uint64(4000)
	)
	// 相同世界种子与相同击杀 tick 的重放逐件一致；掉落内容与 sampler 判定
	// 逐项吻合（骨头数量 0..2、弓按 1/8 判定）。
	run := func() []core.ItemStack {
		engine := hurlerDeathEngine(t, id, worldTime)
		settleHurlerDeath(engine)
		if len(engine.hostiles.entries) != 0 {
			t.Fatal("死亡后掷骨者仍留在集合中，想要同 tick 移除")
		}
		var stacks []core.ItemStack
		dimension := engine.dimension(core.Overworld)
		for _, pos := range dimension.ReadyChunkPositions(nil) {
			chunk, ready := dimension.ReadyChunk(pos)
			if !ready {
				continue
			}
			for slot := range core.DropsPerChunk {
				if drop := chunk.Drop(slot); drop.Active {
					stacks = append(stacks, drop.Stack)
				}
			}
		}
		return stacks
	}
	first, second := run(), run()
	if len(first) != len(second) {
		t.Fatalf("两次重放掉落堆数=%d/%d，想要一致", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 堆掉落重放不一致：%+v vs %+v", index, first[index], second[index])
		}
	}
	bones, bow := updates.Sampler{}.HostileHurlerDropRolls(0, worldTime, id)
	wantStacks := 0
	if bones > 0 {
		wantStacks++
	}
	if bow {
		wantStacks++
	}
	if len(first) != wantStacks {
		t.Fatalf("掉落堆数=%d，想要 %d（骨头 %d、弓 %v）", len(first), wantStacks, bones, bow)
	}
	for _, stack := range first {
		switch stack.Item {
		case core.ItemBone:
			if stack.Count != bones {
				t.Fatalf("骨头掉落数=%d，想要 %d", stack.Count, bones)
			}
		case core.ItemBow:
			if stack.Count != 1 {
				t.Fatalf("弓掉落数=%d，想要 1", stack.Count)
			}
		default:
			t.Fatalf("掷骨者掉落了意外物品 %d", stack.Item)
		}
	}
}

func TestHurlerDeathDropBatchIsAtomicWhenCapacityShort(t *testing.T) {
	// 场景「掉落容量不足整体保留」：探得一批「骨头 ≥1 且掉弓」的判定样本，
	// 把全部已加载 chunk 只留 1 个空槽（装不下骨头+弓两堆）——整批必须被拒，
	// 绝不出现「掉了骨头没掉弓」的部分掉落。
	const worldTime = uint64(4000)
	var id uint64
	found := false
	for candidate := uint64(1); candidate < 4096; candidate++ {
		bones, bow := updates.Sampler{}.HostileHurlerDropRolls(0, worldTime, candidate)
		if bones >= 1 && bow {
			id, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("探针窗口内没有找到「骨头+弓」判定样本，夹具失效")
	}

	engine := hurlerDeathEngine(t, id, worldTime)
	dimension := engine.dimension(core.Overworld)
	for _, pos := range dimension.ReadyChunkPositions(nil) {
		chunk, ready := dimension.ReadyChunk(pos)
		if !ready {
			continue
		}
		for slot := range core.DropsPerChunk {
			if slot == 0 {
				continue // 每个 chunk 恰留 1 个空槽
			}
			chunk.SetDrop(slot, world.DropSlot{
				Generation: 1,
				Active:     true,
				Stack:      core.ItemStack{Item: core.ItemTorch, Count: 1},
				BlockIndex: uint32(slot),
			})
		}
	}
	settleHurlerDeath(engine)
	if got := countLoadedDrops(t, engine, core.ItemBone); got != 0 {
		t.Fatalf("容量不足时掉落了 %d 堆骨头，想要整批保留（0）", got)
	}
	if got := countLoadedDrops(t, engine, core.ItemBow); got != 0 {
		t.Fatalf("容量不足时掉落了 %d 把弓，想要整批保留（0）", got)
	}
	for _, pos := range dimension.ReadyChunkPositions(nil) {
		chunk, ready := dimension.ReadyChunk(pos)
		if !ready {
			continue
		}
		for slot := range core.DropsPerChunk {
			if drop := chunk.Drop(slot); drop.Active && drop.Stack.Item == core.ItemTorch {
				if drop.Stack.Count != 1 {
					t.Fatalf("既有火把堆被破坏：%+v", drop.Stack)
				}
			}
		}
	}
}

func TestNightwalkerDeathKeepsSingleRottenFlesh(t *testing.T) {
	// 夜行者掉落路径不变（共享死亡结算，kind 分派只改掉落批内容）：仍为
	// 恰好 1 个腐肉。
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(22)
	mob.Kind = HostileKindNightwalker
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.worldTime.Store(4000)
	engine.hostiles.entries[0].health = 0
	settleHurlerDeath(engine)
	if got := countLoadedDrops(t, engine, core.ItemRottenFlesh); got != 1 {
		t.Fatalf("夜行者死亡腐肉=%d，想要恰好 1", got)
	}
	if got := countLoadedDrops(t, engine, core.ItemBone); got != 0 {
		t.Fatalf("夜行者死亡掉落了 %d 堆骨头，想要 0", got)
	}
}

func TestBoneHasNoCraftingOrSmeltingRecipe(t *testing.T) {
	// 场景「骨头不可合成」：全部固定配方与熔炼映射中不得出现以骨头为原料或
	// 产物的条目；获取路径只有掷骨者掉落。
	for id := core.RecipeStoneBricks; ; id++ {
		pattern, ok := core.Recipe(id)
		if !ok {
			break
		}
		if pattern.Output.Item == core.ItemBone {
			t.Fatalf("配方 %d 的产物是骨头，违反「骨头不可合成」", id)
		}
		for _, cell := range pattern.Cells {
			if cell == core.ItemBone {
				t.Fatalf("配方 %d 以骨头为原料，违反「骨头不可合成」", id)
			}
		}
	}
	for input := core.ItemID(1); input < core.ItemIDMax; input++ {
		if output, ok := core.SmeltingOutput(input); ok &&
			(input == core.ItemBone || output == core.ItemBone) {
			t.Fatalf("骨头出现在熔炼映射（%d → %d），违反「骨头不可熔炼」", input, output)
		}
	}
}
