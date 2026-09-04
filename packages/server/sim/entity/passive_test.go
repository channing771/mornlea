package entity

import (
	"math"
	"sort"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件锁定被动牛的身体契约：按 `id` 严格升序、容量 32 的排序切片（无
// `map`）、恢复入口的记录校验、与玩家/夜行者完全相同的 per-actor
// `physics.Step` 积分（含 `physics.SubmersionFlags` 浸没标志的复用）、受击
// 逃跑不反击、死亡同 tick 移除并掉落 1 个生牛肉。

// validTestPassive 返回一条可通过恢复校验的被动牛记录基线，各用例按需改字段。
func validTestPassive(id uint64) PassiveMob {
	return PassiveMob{
		ID:        id,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{0.5, 1, 0.5},
			OnGround: true,
		},
		Health: core.MaxHealth,
	}
}

// clearPassivesForTest 清空被动集合，供探针循环复用同一个引擎。
func clearPassivesForTest(engine *Engine) {
	engine.passives.entries = engine.passives.entries[:0]
}

func passiveIDs(mobs []PassiveMob) []uint64 {
	out := make([]uint64, 0, len(mobs))
	for _, mob := range mobs {
		out = append(out, mob.ID)
	}
	return out
}

func TestPassiveRestoreKeepsIDsStrictlySorted(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	// 故意乱序恢复：内部集合必须按 `id` 升序落位。
	for _, id := range []uint64{30, 10, 20} {
		if err := engine.RestorePassive(validTestPassive(id)); err != nil {
			t.Fatalf("恢复被动牛 %d：%v", id, err)
		}
	}
	got := engine.PassiveMobs()
	if len(got) != 3 || got[0].ID != 10 || got[1].ID != 20 || got[2].ID != 30 {
		t.Fatalf("恢复后 `id` 序列=%v，想要 [10 20 30]", passiveIDs(got))
	}
	if !sort.SliceIsSorted(engine.passives.entries, func(i, j int) bool {
		return engine.passives.entries[i].id < engine.passives.entries[j].id
	}) {
		t.Fatal("内部被动切片未按 `id` 升序维护")
	}
}

func TestPassiveRestoreRejectsDuplicateAndThirtyThird(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	if err := engine.RestorePassive(validTestPassive(7)); err != nil {
		t.Fatalf("恢复被动牛 7：%v", err)
	}
	if err := engine.RestorePassive(validTestPassive(7)); err == nil {
		t.Fatal("重复 `id` 被接受，想要拒绝")
	}
	for id := uint64(1); id <= 31; id++ {
		if err := engine.RestorePassive(validTestPassive(id * 100)); err != nil {
			t.Fatalf("恢复被动牛 %d：%v", id*100, err)
		}
	}
	if len(engine.passives.entries) != maxPassives {
		t.Fatalf("被动牛数量=%d，想要 %d", len(engine.passives.entries), maxPassives)
	}
	if err := engine.RestorePassive(validTestPassive(9999)); err == nil {
		t.Fatal("第 33 头被动牛被接受，想要拒绝")
	}
	if len(engine.passives.entries) != maxPassives {
		t.Fatalf("拒绝后被动牛数量=%d，想要仍为 %d", len(engine.passives.entries), maxPassives)
	}
}

func TestPassiveRestoreValidatesRecordFields(t *testing.T) {
	cases := map[string]func(*PassiveMob){
		"零 `id`": func(m *PassiveMob) { m.ID = 0 },
		"未知维度":   func(m *PassiveMob) { m.Dimension = core.DimensionID(7) },
		"位置非有限":  func(m *PassiveMob) { m.State.Position = mgl32.Vec3{float32(math.NaN()), 1, 0.5} },
		"速度非有限":  func(m *PassiveMob) { m.State.Velocity = mgl32.Vec3{0, float32(math.Inf(1)), 0} },
		"位置高于世界": func(m *PassiveMob) { m.State.Position = mgl32.Vec3{0.5, core.MaxY, 0.5} },
		"位置低于世界": func(m *PassiveMob) { m.State.Position = mgl32.Vec3{0.5, core.MinY - 1, 0.5} },
		"生命为零":   func(m *PassiveMob) { m.Health = 0 },
		"生命超过上限": func(m *PassiveMob) { m.Health = core.MaxHealth + 1 },
	}
	for name, mutate := range cases {
		engine := NewEngine(0, 0, 0)
		mob := validTestPassive(42)
		mutate(&mob)
		if err := engine.RestorePassive(mob); err == nil {
			t.Fatalf("%s：非法记录被接受，想要拒绝", name)
		}
		if len(engine.passives.entries) != 0 {
			t.Fatalf("%s：被拒绝的记录仍留在集合中", name)
		}
	}
}

func TestPassiveMovementReusesPlayerPhysicsStep(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 与玩家出生同形的落点：从半空坠落至 `flat` 世界地表。
	start := physics.State{Position: mgl32.Vec3{2.5, 10, 2.5}}
	mob := validTestPassive(11)
	mob.State = start
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
	engine.advancePassiveMovement()
	entry := &engine.passives.entries[0]
	want := physics.Step(start, entry.input, source).State
	if entry.state != want {
		t.Fatalf("被动牛积分结果=%+v，想要与 `physics.Step` 完全一致的 %+v", entry.state, want)
	}
	if entry.state.OnGround {
		t.Fatal("单步后不应已落地")
	}
}

func TestPassiveWanderStaysWithinHomeNeighborhood(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -2, 2, -2, 2)
	// 闲时看人会让 6 格内的牛原地看人：把玩家摆到 6 格外，漫游才不受干扰。
	placeSessionPlayer(engine, session, mgl32.Vec3{30.5, 1, 30.5})
	mob := validTestPassive(12)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	start := engine.passives.entries[0].state.Position
	moved := false
	for range 300 {
		engine.advancePassiveMovement()
		if len(engine.passives.entries) != 1 {
			t.Fatal("漫游中被动牛意外消失")
		}
		position := engine.passives.entries[0].state.Position
		if position.Sub(start).Len() > 0.01 {
			moved = true
		}
		// 出生区块邻域：出生区块 (0,0) 的切比雪夫距离 1 范围，即方块
		// [-16,32) × [-16,32)。
		if position.X() < -16 || position.X() >= 32 || position.Z() < -16 || position.Z() >= 32 {
			t.Fatalf("漫游位置 %+v 离开出生区块邻域", position)
		}
		if got := countLoadedDrops(t, engine, core.ItemRawBeef); got != 0 {
			t.Fatal("漫游产生了掉落物，想要 0")
		}
	}
	if !moved {
		t.Fatal("300 tick 内被动牛没有任何位移，想要有界漫游")
	}
}

func TestPassiveMovementNeverPassesThroughWalls(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	// 以 (2,2) 为中心围一圈两格高的石墙，牛在环内出生。
	for x := int32(0); x <= 4; x++ {
		for z := int32(0); z <= 4; z++ {
			if x != 0 && x != 4 && z != 0 && z != 4 {
				continue
			}
			engine.SetBlockForTest(core.BlockPos{X: x, Y: 1, Z: z}, core.StoneID)
			engine.SetBlockForTest(core.BlockPos{X: x, Y: 2, Z: z}, core.StoneID)
		}
	}
	mob := validTestPassive(13)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	for range 100 {
		engine.advancePassiveMovement()
		position := engine.passives.entries[0].state.Position
		x := int32(math.Floor(float64(position.X())))
		z := int32(math.Floor(float64(position.Z())))
		if x < 1 || x > 3 || z < 1 || z > 3 {
			t.Fatalf("被动牛穿墙到 (%d,%d)，想要留在石墙环内", x, z)
		}
	}
}

func TestPassiveDamageTriggersFleeAwayFromSource(t *testing.T) {
	engine, player := readyMovementPlayer(t)
	mob := validTestPassive(14)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	playerHealth := engine.sessions[player].player.health
	source := mgl32.Vec3{10.5, 1, 2.5}
	if !engine.DamagePassive(14, 3, source) {
		t.Fatal("已知 `id` 的伤害被拒绝，想要受理")
	}
	entry := &engine.passives.entries[0]
	if entry.health != core.MaxHealth-3 {
		t.Fatalf("受击后生命=%d，想要 %d", entry.health, core.MaxHealth-3)
	}
	before := entry.state.Position.Sub(source).Len()
	for range 20 {
		engine.advancePassiveMovement()
	}
	after := engine.passives.entries[0].state.Position.Sub(source).Len()
	if after <= before+0.5 {
		t.Fatalf("逃跑前后与伤害源距离=%v→%v，想要显著拉开", before, after)
	}
	// 被动牛不反击：玩家生命在逃跑全程不受任何伤害。
	if got := engine.sessions[player].player.health; got != playerHealth {
		t.Fatalf("玩家生命=%d，想要逃跑期间保持 %d", got, playerHealth)
	}
}

func TestPassiveDamageRejectsUnknownIDAndIgnoresNonPositive(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestPassive(15)
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	if engine.DamagePassive(999, 3, mgl32.Vec3{10.5, 1, 2.5}) {
		t.Fatal("未知 `id` 的伤害被受理，想要拒绝")
	}
	for _, damage := range []int32{0, -5} {
		if !engine.DamagePassive(15, damage, mgl32.Vec3{10.5, 1, 2.5}) {
			t.Fatalf("伤害 %d：已知 `id` 被拒绝，想要受理并忽略", damage)
		}
		entry := &engine.passives.entries[0]
		if entry.health != core.MaxHealth {
			t.Fatalf("伤害 %d：生命=%d，想要不变", damage, entry.health)
		}
		if entry.fleeTicks != 0 {
			t.Fatalf("伤害 %d：进入逃跑，想要保持漫游", damage)
		}
	}
}

func TestPassiveFleeEndsAndResumesWander(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -2, 2, -2, 2)
	mob := validTestPassive(16)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	engine.DamagePassive(16, 1, mgl32.Vec3{10.5, 1, 2.5})
	if engine.passives.entries[0].fleeTicks == 0 {
		t.Fatal("受击后未进入逃跑")
	}
	for range 120 {
		engine.advancePassiveMovement()
	}
	if got := engine.passives.entries[0].fleeTicks; got != 0 {
		t.Fatalf("逃跑 120 tick 后剩余=%d，想要已恢复漫游", got)
	}
	if len(engine.passives.entries) != 1 {
		t.Fatal("逃跑结束后被动牛消失，想要恢复漫游并保留")
	}
}

func TestPassiveDeathDropsSingleRawBeef(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestPassive(17)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	// 满血一击致死：同一 tick 内移除 + 掉落。
	engine.DamagePassive(17, int32(core.MaxHealth), mgl32.Vec3{10.5, 1, 2.5})
	pending := engine.newMutation()
	engine.settlePassiveDeaths(pending)
	if len(engine.passives.entries) != 0 {
		t.Fatal("死亡后被动牛仍留在集合中，想要同 tick 移除")
	}
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 0, Z: 0}}
	if !pending.Has(key) {
		t.Fatal("死亡掉落未登记区块变更 `barrier`")
	}
	if got := countLoadedDrops(t, engine, core.ItemRawBeef); got != 1 {
		t.Fatalf("死亡掉落生牛肉=%d，想要恰好 1", got)
	}
}

func TestPassiveDeathDropRingsToNeighborChunkWhenDeathChunkFull(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 装载 3×3 邻域并填满死亡区块（原点）的全部掉落槽。
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	dimension := engine.dimension(core.Overworld)
	origin, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("原点区块未就绪")
	}
	for slot := range core.DropsPerChunk {
		origin.SetDrop(slot, world.DropSlot{
			Generation: 1,
			Active:     true,
			Stack:      core.ItemStack{Item: core.ItemTorch, Count: 1},
			BlockIndex: uint32(slot),
		})
	}
	mob := validTestPassive(18)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	// 死亡结算的入口条件是生命归零；恢复校验只收健康个体，这里直接置零
	// 等价于刚好扣完的瞬间。
	engine.passives.entries[0].health = 0
	pending := engine.newMutation()
	engine.settlePassiveDeaths(pending)
	if len(engine.passives.entries) != 0 {
		t.Fatal("掉落槽全满时死亡未完成，想要仍移除")
	}
	if got := countLoadedDrops(t, engine, core.ItemRawBeef); got != 1 {
		t.Fatalf("环形尝试后生牛肉=%d，想要恰好 1", got)
	}
	neighbor := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -1, Z: -1}}
	if !pending.Has(neighbor) {
		t.Fatal("环形掉落未在邻接区块登记变更 `barrier`")
	}
}

func TestPassiveDeathWithAllSlotsFullOmitsDropDeterministically(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 3×3 邻域全部填满：死亡完成但确定性省略掉落，且不动任何已有物品。
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	dimension := engine.dimension(core.Overworld)
	for _, pos := range dimension.ReadyChunkPositions(nil) {
		chunk, ready := dimension.ReadyChunk(pos)
		if !ready {
			continue
		}
		for slot := range core.DropsPerChunk {
			chunk.SetDrop(slot, world.DropSlot{
				Generation: 1,
				Active:     true,
				Stack:      core.ItemStack{Item: core.ItemTorch, Count: 1},
				BlockIndex: uint32(slot),
			})
		}
	}
	mob := validTestPassive(19)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	engine.passives.entries[0].health = 0
	engine.settlePassiveDeaths(engine.newMutation())
	if len(engine.passives.entries) != 0 {
		t.Fatal("全满时死亡未完成，想要仍移除")
	}
	if got := countLoadedDrops(t, engine, core.ItemRawBeef); got != 0 {
		t.Fatalf("全满时出现了 %d 个生牛肉，想要 0", got)
	}
	if got := countLoadedDrops(t, engine, core.ItemTorch); got != 9*int(core.DropsPerChunk) {
		t.Fatalf("既有掉落物被破坏：火把堆=%d，想要 %d", got, 9*int(core.DropsPerChunk))
	}
}

func TestPassiveFallingBelowWorldIsRemoved(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 挖穿脚底支撑：被动牛坠出世界后没有玩家那样的重生路径，必须在落地
	// 无望时被确定性移除，且不产生掉落。
	engine.SetBlockForTest(core.BlockPos{X: 2, Y: 0, Z: 2}, core.AirID)
	mob := validTestPassive(20)
	mob.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	for range 200 {
		engine.advancePassiveMovement()
		if len(engine.passives.entries) == 0 {
			break
		}
	}
	if len(engine.passives.entries) != 0 {
		t.Fatal("坠出世界的被动牛 200 tick 内未被移除")
	}
	if got := countLoadedDrops(t, engine, core.ItemRawBeef); got != 0 {
		t.Fatalf("坠落移除产生了 %d 个生牛肉，想要 0", got)
	}
}
