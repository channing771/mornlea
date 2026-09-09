package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件是「树苗与树叶采掘」主题：树苗 1 tick、任意手持、掉落自身且零工具
// 磨损；树叶沿用 5 tick 与自身掉落，并按独立冻结判定额外掉落 1 个树苗，两堆
// 掉落作为同一次原子预演结算。与 mining_test.go 的短草用例同形（同一套位置
// 稳定判定与容量原子性），差别只在判定 salt 与「树叶自身掉落 + 树苗」的批量。

// plantMiningPlayer 复用短草采掘夹具的玩家摆位与区块装载，只把目标方块换成
// 用例给定的植物方块（树苗或树叶）——三者都是零碰撞植物，采掘射线与摆位同形。
func plantMiningPlayer(
	t *testing.T,
	target core.BlockPos,
	block core.BlockID,
	held core.ItemStack,
) (*Engine, SessionID, core.BlockPos) {
	t.Helper()
	engine, session, target := shortGrassMiningPlayer(t, target, held)
	engine.SetBlockForTest(target, block)
	return engine, session, target
}

// leavesSaplingHitPositions / leavesSaplingMissPositions 是树叶→树苗判定的固定
// 样本（测试引擎固定 world seed=0、Overworld）：命中/未命中坐标按规格冻结的
// salt 与折叠链事先算出，不在测试运行时搜索。负坐标样本钉住「有符号坐标先转
// uint32」的位模式语义。
//
// 未命中样本刻意取短草种子判定的**命中**坐标（mining_test.go 的
// shortGrassSeedHitPositions）：同坐标、相反判定，说明两条哈希流确实由不同
// salt 派生，改一处不会静默连带另一处。
var (
	leavesSaplingHitPositions = [...]core.BlockPos{
		{X: -6, Y: 1, Z: 5}, {X: 7, Y: 1, Z: 5}, {X: 16, Y: 1, Z: 5},
	}
	leavesSaplingMissPositions = [...]core.BlockPos{
		{X: 6, Y: 1, Z: 5}, {X: 14, Y: 1, Z: 5}, {X: -9, Y: 1, Z: 5},
	}
)

// TestSaplingMiningRuleOneTickAnyHeld 覆盖 Scenario「任意手持状态一个 tick 采掘
// 树苗」的规则半边：树苗与手持完全无关，任意状态都是 1 tick 且 harvestable=true。
// 0 是 miningRule 的「不可采掘」哨兵，树苗用 0 会永远挖不动。
func TestSaplingMiningRuleOneTickAnyHeld(t *testing.T) {
	helds := [...]core.ItemID{
		core.ItemNone,
		core.ItemDirt,
		core.ItemStone,
		core.ItemStonePickaxe,
		core.ItemIronPickaxe,
		core.ItemBrokenStonePickaxe,
		core.ItemBrokenIronPickaxe,
		core.ItemStoneHoe,
		core.ItemIronHoe,
		core.ItemWoodenSword,
		core.ItemIronSword,
	}
	for _, held := range helds {
		ticks, harvestable := miningRule(core.SaplingID, held)
		if ticks != 1 || !harvestable {
			t.Fatalf("miningRule(SaplingID, %d) = (%d, %v)，想要 (1, true)", held, ticks, harvestable)
		}
	}
}

// requireMiningDroppedSapling 断言目标区块恰好存在一个活动掉落槽，且是恰好
// 1 个树苗、锚定在目标格、带既有 mining pickup delay——树苗必须进入世界掉落物
// 系统而不是背包。
func requireMiningDroppedSapling(t *testing.T, engine *Engine, target core.BlockPos) {
	t.Helper()
	record := miningTargetRecord(t, engine, target)
	blockIndex, ok := world.ChunkBlockIndex(target)
	if !ok {
		t.Fatalf("树苗目标 %+v 没有区块索引", target)
	}
	found := 0
	for slot := range core.DropsPerChunk {
		drop := record.Chunk.Drop(slot)
		if !drop.Active {
			continue
		}
		found++
		if drop.Stack != (core.ItemStack{Item: core.ItemSapling, Count: 1}) ||
			drop.BlockIndex != blockIndex ||
			drop.PickupDelayTicks != engine.tunables.DropPickupDelayTicks {
			t.Fatalf("掉落槽 %d = %+v，想要锚定目标格的 1 个树苗", slot, drop)
		}
	}
	if found != 1 {
		t.Fatalf("活动掉落槽数=%d，想要恰好 1（树苗恰好掉一个自身）", found)
	}
}

// TestMiningSaplingDropsOneSaplingIntoWorld 覆盖 Scenario「任意手持状态一个 tick
// 采掘树苗」的结算半边：任意手持状态 1 tick 完成，目标格变空气、世界恰好出现
// 1 个树苗掉落物、背包不被结算直接写入、区块 revision 恰好推进一次。
func TestMiningSaplingDropsOneSaplingIntoWorld(t *testing.T) {
	helds := [...]core.ItemStack{
		{},
		{Item: core.ItemDirt, Count: 1},
		{Item: core.ItemStonePickaxe, Count: 1, Durability: fullToolDurability(core.ItemStonePickaxe)},
		{Item: core.ItemIronSword, Count: 1, Durability: fullToolDurability(core.ItemIronSword)},
	}
	for _, held := range helds {
		engine, session, target := plantMiningPlayer(t, core.BlockPos{X: 0, Y: 1, Z: 5}, core.SaplingID, held)
		player := engine.sessions[session].player
		beforeRevision := miningTargetRecord(t, engine, target).Revision

		result := advanceMiningOnce(engine)

		if len(result.Rejected) != 0 {
			t.Fatalf("采掘树苗被拒绝=%+v", result.Rejected)
		}
		record := miningTargetRecord(t, engine, target)
		x, _, z := target.Local()
		if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
			t.Fatalf("采掘后方块=%d，想要空气", got)
		}
		requireMiningDroppedSapling(t, engine, target)
		if record.Revision != beforeRevision+1 {
			t.Fatalf("revision=%d，想要恰好推进一次 %d", record.Revision, beforeRevision+1)
		}
		if player.inventoryDirty {
			t.Fatal("采掘树苗不应触碰背包（树苗走世界掉落物）")
		}
	}
}

// TestMiningSaplingZeroDurabilityWearForAnyTool 覆盖 tool-durability 的第四类
// 豁免：被移除方块是树苗时，任何选中物（空手、普通物品、完好工具、锄头、剑，
// 含耐久恰好为 1 的工具）都零磨损——耐久 1 的工具不得转为损坏形态，也没有
// 额外 inventory dirty。
func TestMiningSaplingZeroDurabilityWearForAnyTool(t *testing.T) {
	tests := []struct {
		name string
		held core.ItemStack
	}{
		{name: "空手"},
		{name: "普通物品", held: core.ItemStack{Item: core.ItemDirt, Count: 3}},
		{name: "完好石镐", held: core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: fullToolDurability(core.ItemStonePickaxe)}},
		{name: "完好铁镐", held: core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: fullToolDurability(core.ItemIronPickaxe)}},
		{name: "完好锄头", held: core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: fullToolDurability(core.ItemStoneHoe)}},
		{name: "完好剑", held: core.ItemStack{Item: core.ItemStoneSword, Count: 1, Durability: fullToolDurability(core.ItemStoneSword)}},
		{name: "耐久1的石镐不损坏", held: core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 1}},
		{name: "耐久1的铁镐不损坏", held: core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 1}},
		{name: "耐久1的锄头不损坏", held: core.ItemStack{Item: core.ItemIronHoe, Count: 1, Durability: 1}},
		{name: "损坏形态工具", held: core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, session, target := plantMiningPlayer(t, core.BlockPos{X: 0, Y: 1, Z: 5}, core.SaplingID, test.held)
			player := engine.sessions[session].player

			result := advanceMiningOnce(engine)

			if len(result.Rejected) != 0 {
				t.Fatalf("采掘树苗被拒绝=%+v", result.Rejected)
			}
			record := miningTargetRecord(t, engine, target)
			x, _, z := target.Local()
			if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
				t.Fatalf("采掘后方块=%d，想要空气", got)
			}
			if got := player.inventory.Hotbar.Slots[0]; got != test.held {
				t.Fatalf("采掘树苗磨损了工具: got=%+v want=%+v", got, test.held)
			}
			if player.inventoryDirty {
				t.Fatal("零磨损豁免不应产生 inventory dirty")
			}
		})
	}
}

// TestMiningSaplingExemptionKeepsOtherBlockToolRules 覆盖 Scenario「树苗豁免不
// 改变其他方块与工具规则」：同一把完好锄头先采掘树苗（零磨损），再破坏一个
// 非作物非树苗的普通方块（泥土），后者必须仍按既有规则恰好扣一点耐久；树苗
// 也不得被「作物 × 锄头」豁免分类吞掉——它是独立的第四类。
func TestMiningSaplingExemptionKeepsOtherBlockToolRules(t *testing.T) {
	if hoeHarvestDurabilityExempt(core.SaplingID, core.ItemStoneHoe) {
		t.Fatal("树苗不是作物，不得命中「作物 × 锄头」豁免")
	}
	full := fullToolDurability(core.ItemStoneHoe)
	held := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: full}
	engine, session, target := plantMiningPlayer(t, core.BlockPos{X: 0, Y: 1, Z: 5}, core.SaplingID, held)
	player := engine.sessions[session].player

	if result := advanceMiningOnce(engine); len(result.Rejected) != 0 {
		t.Fatalf("采掘树苗被拒绝=%+v", result.Rejected)
	}
	if got := player.inventory.Hotbar.Slots[0]; got != held {
		t.Fatalf("采掘树苗磨损了锄头: got=%+v want=%+v", got, held)
	}

	// 同一坐标换泥土继续采：锄头破坏非树苗普通方块仍扣恰好一点耐久。
	engine.SetBlockForTest(target, core.DirtID)
	for range 5 {
		advanceMiningOnce(engine)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("锄头破坏泥土后方块=%d，想要空气", got)
	}
	if got := player.inventory.Hotbar.Slots[0].Durability; got != full-1 {
		t.Fatalf("锄头破坏泥土后耐久=%d，想要 %d", got, full-1)
	}
	if !player.inventoryDirty {
		t.Fatal("破坏泥土扣减耐久没有标记 inventoryDirty")
	}
}

// TestLeavesSaplingDropSaltIsFrozenAndIndependent 钉住树叶→树苗判定的 salt
// 常量：值按 design 决策 D2 冻结，且与 entity 侧其他哈希流（作物生长、小麦/
// 马铃薯/胡萝卜产量、毒土豆、短草种子掉落）的 salt 互不相同——同源会让「这格
// 额外掉树苗」与其他判定出现结构性相关。
func TestLeavesSaplingDropSaltIsFrozenAndIndependent(t *testing.T) {
	if leavesSaplingDropSalt != 0x5341_504C_494E_4753 {
		t.Fatalf("leavesSaplingDropSalt = %#x，规格冻结为 0x5341_504C_494E_4753",
			leavesSaplingDropSalt)
	}
	for name, salt := range map[string]uint64{
		"cropGrowthRollSalt":     cropGrowthRollSalt,
		"cropYieldRollSalt":      cropYieldRollSalt,
		"cropYieldPotatoSalt":    cropYieldPotatoSalt,
		"cropYieldCarrotSalt":    cropYieldCarrotSalt,
		"poisonPotatoSalt":       poisonPotatoSalt,
		"shortGrassSeedDropSalt": shortGrassSeedDropSalt,
	} {
		if salt == leavesSaplingDropSalt {
			t.Fatalf("树苗掉落 salt 与 %s 相同，两条哈希流必须互相独立", name)
		}
	}
}

// TestLeavesSaplingDropRollMatchesFrozenVerdicts 钉住判定函数本身：固定命中/
// 未命中坐标（含负坐标的 uint32 位模式样本）的判定必须与事先算出的常量一致、
// 纯函数重放恒等、维度折入哈希、连续 4096 个坐标的命中率落在 1/8 附近且两侧
// 都存在。签名刻意不含 tick、玩家与手持——重试稳定性由类型形状直接保证。
func TestLeavesSaplingDropRollMatchesFrozenVerdicts(t *testing.T) {
	for _, pos := range leavesSaplingHitPositions {
		for replay := 0; replay < 2; replay++ {
			if !leavesSaplingDropRoll(0, core.Overworld, pos) {
				t.Fatalf("leavesSaplingDropRoll(0, Overworld, %v) = false，固定命中坐标必须恒命中", pos)
			}
		}
	}
	for _, pos := range leavesSaplingMissPositions {
		if leavesSaplingDropRoll(0, core.Overworld, pos) {
			t.Fatalf("leavesSaplingDropRoll(0, Overworld, %v) = true，固定未命中坐标必须恒未命中", pos)
		}
	}
	// 维度折入：同一坐标在另一维度判定翻转。core 当前只注册 Overworld，这里只
	// 验证纯整数链确实折叠了维度——漏折会让两个维度的同坐标树叶永远同判定。
	if leavesSaplingDropRoll(0, core.DimensionID(1), leavesSaplingHitPositions[0]) {
		t.Fatal("维度未折入哈希链：另一维度的同坐标不应复用 Overworld 的命中判定")
	}
	// 分布 sanity：连续 4096 个坐标的命中率接近 1/8（理论 512），两侧都必须出现，
	// 防 `& 7` 退化成恒真/恒假。
	hits := 0
	for x := int32(0); x < 4096; x++ {
		if leavesSaplingDropRoll(0, core.Overworld, core.BlockPos{X: x, Y: 1, Z: 5}) {
			hits++
		}
	}
	if hits < 256 || hits > 768 || hits == 0 || hits == 4096 {
		t.Fatalf("4096 个坐标命中 %d 次，想要接近 1/8（512）且两侧都存在", hits)
	}
}

// TestMiningLeavesHitDropsLeavesAndOneSapling 覆盖 Scenario「树叶命中树苗判定
// 额外掉落一个树苗」：命中坐标上采掘 5 tick 完成，树叶格变空气，世界恰好出现
// 既有树叶掉落与 1 个树苗掉落，背包不被结算直接写入，revision 恰好推进一次。
func TestMiningLeavesHitDropsLeavesAndOneSapling(t *testing.T) {
	engine, session, target := plantMiningPlayer(t, leavesSaplingHitPositions[1], core.LeavesID, core.ItemStack{})
	player := engine.sessions[session].player
	beforeRevision := miningTargetRecord(t, engine, target).Revision

	var result TickResult
	for range 5 {
		result = advanceMiningOnce(engine)
	}

	if len(result.Rejected) != 0 {
		t.Fatalf("采掘树叶被拒绝=%+v", result.Rejected)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("采掘后方块=%d，想要空气", got)
	}
	want := map[core.ItemID]uint8{core.ItemLeaves: 1, core.ItemSapling: 1}
	if got := miningDropTotals(record.Chunk); !equalMiningDropTotals(got, want) {
		t.Fatalf("掉落=%+v，想要恰好 %+v", got, want)
	}
	if record.Revision != beforeRevision+1 {
		t.Fatalf("revision=%d，想要恰好推进一次 %d", record.Revision, beforeRevision+1)
	}
	if player.inventoryDirty {
		t.Fatal("采掘树叶不应触碰背包（掉落走世界掉落物）")
	}
}

// TestMiningLeavesMissDropsLeavesOnly 覆盖 Scenario「未命中判定不产生树苗」：
// 未命中坐标上采掘完成，树叶被移除并掉落既有树叶物品，世界掉落物不新增树苗。
func TestMiningLeavesMissDropsLeavesOnly(t *testing.T) {
	engine, _, target := plantMiningPlayer(t, leavesSaplingMissPositions[0], core.LeavesID, core.ItemStack{})
	beforeRevision := miningTargetRecord(t, engine, target).Revision

	var result TickResult
	for range 5 {
		result = advanceMiningOnce(engine)
	}

	if len(result.Rejected) != 0 {
		t.Fatalf("未命中采掘被拒绝=%+v", result.Rejected)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("未命中采掘后方块=%d，想要空气", got)
	}
	if got := miningDropTotals(record.Chunk); !equalMiningDropTotals(got, map[core.ItemID]uint8{core.ItemLeaves: 1}) {
		t.Fatalf("掉落=%+v，想要恰好 1 个树叶物品", got)
	}
	if record.Revision != beforeRevision+1 {
		t.Fatalf("revision=%d，想要恰好推进一次 %d", record.Revision, beforeRevision+1)
	}
}

// TestMiningLeavesMissSucceedsWithOneFreeDropSlot 覆盖 Scenario「树叶未命中树苗
// 判定时容量已满仍可采掘」：只留一个掉落槽时未命中路径照样成功——树苗容量
// MUST NOT 被预留或要求。
func TestMiningLeavesMissSucceedsWithOneFreeDropSlot(t *testing.T) {
	engine, _, target := plantMiningPlayer(t, leavesSaplingMissPositions[0], core.LeavesID, core.ItemStack{})
	fillMiningDropsLeavingOneSlot(engine, target)
	beforeRevision := miningTargetRecord(t, engine, target).Revision

	var result TickResult
	for range 5 {
		result = advanceMiningOnce(engine)
	}

	if len(result.Rejected) != 0 {
		t.Fatalf("只留一个掉落槽的未命中采掘被拒绝=%+v", result.Rejected)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("未命中采掘后方块=%d，想要空气", got)
	}
	leaves := 0
	for slot := range core.DropsPerChunk {
		drop := record.Chunk.Drop(slot)
		if drop.Active && drop.Stack.Item == core.ItemLeaves {
			leaves++
		}
	}
	if leaves != 1 {
		t.Fatalf("树叶掉落槽数=%d，想要恰好 1", leaves)
	}
	if record.Revision != beforeRevision+1 {
		t.Fatalf("revision=%d，想要恰好推进一次 %d", record.Revision, beforeRevision+1)
	}
}

// TestMiningLeavesHitCapacityFullRejectsAtomicallyAndRetryStaysHit 覆盖 Scenario
// 「树叶命中树苗掉落但容量已满时原子拒绝」与「相同坐标重试结果不变」：命中且只留
// **恰好一个**空掉落槽时 → RejectDropCapacity 且树叶、掉落槽、revision、工具与
// 疲劳全部不变。
//
// 「只留一个空槽」是这条用例的承重设计：一个空槽足够放下树叶自身掉落，因此若实现
// 把树叶与树苗拆成两次预演（或先落树叶、再补树苗），第一次预演就会成功、树叶被
// 移除并进槽——下面的「方块与 `DropsHash` 逐字节不变」立刻红。只有两堆进同一次
// `PrepareDropBatch` 才会整体拒绝。
//
// 树叶 5 tick 完成，因此每轮 5 tick 是一次完整重试；再释放一个槽（合计两个空槽）
// 后同一坐标必须结算为「树叶 + 树苗」，重试不会把命中重掷成未命中，也不会只掉
// 树叶。
func TestMiningLeavesHitCapacityFullRejectsAtomicallyAndRetryStaysHit(t *testing.T) {
	held := core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: fullToolDurability(core.ItemIronPickaxe)}
	engine, session, target := plantMiningPlayer(t, leavesSaplingHitPositions[1], core.LeavesID, held)
	player := engine.sessions[session].player
	fillMiningDropsLeavingOneSlot(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeHash := record.Chunk.Hash()
	beforeDrops := record.Chunk.DropsHash()
	beforeRevision := record.Revision
	beforeExhaustion := exhaustionOf(player)

	for attempt := 1; attempt <= 2; attempt++ {
		var result TickResult
		for range 5 {
			result = advanceMiningOnce(engine)
		}
		if len(result.Rejected) != 1 || result.Rejected[0] != (Rejection{
			Session: session, Sequence: 10, Reason: RejectDropCapacity,
		}) {
			t.Fatalf("第 %d 轮容量拒绝=%+v", attempt, result.Rejected)
		}
		if got := record.Chunk.Hash(); got != beforeHash || record.Revision != beforeRevision {
			t.Fatalf("第 %d 轮容量失败修改了区块或 revision: hash=%x/%x revision=%d/%d",
				attempt, got, beforeHash, record.Revision, beforeRevision)
		}
		if got := record.Chunk.DropsHash(); got != beforeDrops {
			t.Fatalf("第 %d 轮容量失败修改了掉落槽: %x/%x", attempt, got, beforeDrops)
		}
		if got := player.inventory.Hotbar.Slots[0]; got != held {
			t.Fatalf("第 %d 轮容量拒绝修改了工具: got=%+v want=%+v", attempt, got, held)
		}
		if got := exhaustionOf(player); got != beforeExhaustion {
			t.Fatalf("第 %d 轮容量拒绝累积了疲劳: %v 想要 %v", attempt, got, beforeExhaustion)
		}
		if player.inventoryDirty {
			t.Fatalf("第 %d 轮容量拒绝标记了 inventoryDirty", attempt)
		}
	}

	// 再释放一个槽（合计两个空槽）后重试：树叶与树苗是不同的物品，两堆各占一个
	// 空槽，因此这次必须整体成功。
	key := core.ChunkKey{Dimension: core.Overworld, Pos: target.Chunk()}
	engine.SetChunkDropForTest(key, 0, world.DropSlot{})
	var result TickResult
	for range 5 {
		result = advanceMiningOnce(engine)
	}
	if len(result.Rejected) != 0 {
		t.Fatalf("释放容量后的重试被拒绝=%+v", result.Rejected)
	}
	record = miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("重试完成后方块=%d，想要空气", got)
	}
	// 释放的槽之外仍留着占位泥土堆，因此这里只数两件产物各自的槽，不断言
	// 「唯一活动槽」（与短草重试用例同形）。
	drops := miningDropTotals(record.Chunk)
	if drops[core.ItemLeaves] != 1 || drops[core.ItemSapling] != 1 {
		t.Fatalf("重试后的掉落=%+v，想要恰好 1 个树叶与 1 个树苗", drops)
	}
	// 树叶不在耐久豁免之列：成功采掘恰好磨损铁镐一点（容量拒绝的每一轮都不磨）。
	worn := held
	worn.Durability--
	if got := player.inventory.Hotbar.Slots[0]; got != worn {
		t.Fatalf("重试完成后工具=%+v，想要恰好磨损一点到 %+v", got, worn)
	}
}

// TestCompanionMineableBlockAdmitsSaplingByGenericRule 钉住伙伴采掘树苗沿用
// 「具有单一 `BlockDrop` 的非容器、非农业、非流体方块」通用规则：树苗恰有单一
// 掉落，因此**不得**为它新增显式拒绝（伙伴植树属未裁决语义，但采掘不是）。
func TestCompanionMineableBlockAdmitsSaplingByGenericRule(t *testing.T) {
	if !companionMineableBlock(core.SaplingID) {
		t.Fatal("companionMineableBlock(SaplingID) = false，树苗必须按通用单一掉落规则被允许")
	}
}

// TestCompanionMiningSaplingKeepsToolDurability 覆盖 Scenario「伙伴采掘树苗同样
// 不扣耐久」：四项耐久豁免按「被移除方块」判定，对玩家与伙伴的权威采掘结算同样
// 成立。伙伴选中任一完好工具（含耐久恰好为 1 的工具与损坏形态）采掘树苗后，工具
// 的 item、数量与耐久必须逐字段不变。
//
// 探针刻意不是 `inventoryDirty`：伙伴采掘的产物直入背包，该标记由入包本身合法
// 置位，无法区分「耐久写入」与「产物入包」两种来源。改用「结算后背包 = 结算前
// 背包 + 恰好 1 个树苗」的等式——任何耐久写入（扣一点或转损坏形态）都会破坏
// 它，而空手与损坏工具不会产生额外的树苗堆。
func TestCompanionMiningSaplingKeepsToolDurability(t *testing.T) {
	tests := []struct {
		name string
		held core.ItemStack
	}{
		{name: "空手"},
		{name: "完好石镐", held: core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: fullToolDurability(core.ItemStonePickaxe)}},
		{name: "完好铁镐", held: core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: fullToolDurability(core.ItemIronPickaxe)}},
		{name: "完好锄头", held: core.ItemStack{Item: core.ItemIronHoe, Count: 1, Durability: fullToolDurability(core.ItemIronHoe)}},
		{name: "完好剑", held: core.ItemStack{Item: core.ItemStoneSword, Count: 1, Durability: fullToolDurability(core.ItemStoneSword)}},
		{name: "耐久1的铁镐不损坏", held: core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 1}},
		{name: "耐久1的锄头不损坏", held: core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: 1}},
		{name: "损坏形态工具", held: core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := readyCompanionMining(t, core.SaplingID, core.ItemNone)
			entry := fixture.entry
			if test.held != (core.ItemStack{}) {
				entry.inventory.Hotbar.Slots[0] = test.held
			}
			before := entry.inventory
			want, leftover := before.AddStack(core.ItemStack{Item: core.ItemSapling, Count: 1})
			if leftover.Count != 0 {
				t.Fatalf("夹具背包放不下树苗产物: %+v", leftover)
			}

			result := advanceMiningOnce(fixture.engine)

			if len(result.Rejected) != 0 {
				t.Fatalf("伙伴采掘树苗被拒绝=%+v", result.Rejected)
			}
			if got := companionMiningBlockAt(t, fixture); got != core.AirID {
				t.Fatalf("采掘后方块=%d，想要空气", got)
			}
			if entry.inventory != want {
				t.Fatalf("结算后背包=%+v，想要「结算前 + 1 个树苗」=%+v（工具必须逐字段不变）",
					entry.inventory, want)
			}
			if got := companionItemCount(entry, core.ItemSapling); got != 1 {
				t.Fatalf("伙伴树苗产物=%d，想要恰好 1", got)
			}
		})
	}
}

// TestCompanionMiningLeavesNeverRollsSapling 覆盖 Scenario「伙伴采掘树叶不触发
// 树苗判定」：目标坐标刻意选在玩家树叶→树苗判定的固定命中点上，伙伴完成采掘
// 后必须只获得既有树叶掉落。额外树苗的概率判定只属于玩家采掘路径
// （`completeMining` 的树叶分支），伙伴走 `completeCompanionMining` 的通用单件
// 结算，不得镜像该判定——本用例与 `TestMiningLeavesHitDropsLeavesAndOneSapling`
// 共用同一个命中坐标，两侧对照说明判定确实只按 actor 类型分叉。
func TestCompanionMiningLeavesNeverRollsSapling(t *testing.T) {
	// 公共场景把目标方块放在 (4, 1, 5)，而玩家判定的固定命中坐标不在那里，
	// 因此这里把目标整体换成命中坐标上的树叶：伙伴站位与视线不变（命中点仍在
	// 默认 InteractionReach 内、射线无遮挡），采掘意图按直写路径补齐。
	fixture := newCompanionMiningScene(t, core.AirID, core.ItemNone)
	target := leavesSaplingHitPositions[1]
	if !leavesSaplingDropRoll(fixture.engine.seed, core.Overworld, target) {
		t.Fatalf("夹具前提失效：%v 不再命中玩家树叶→树苗判定", target)
	}
	fixture.engine.SetBlockForTest(target, core.LeavesID)
	fixture.target = target
	fixture.entry.miningHeld = true
	fixture.entry.miningTarget = target

	var result TickResult
	for range 5 {
		result = advanceMiningOnce(fixture.engine)
	}

	if len(result.Rejected) != 0 {
		t.Fatalf("伙伴采掘树叶被拒绝=%+v", result.Rejected)
	}
	if got := companionMiningBlockAt(t, fixture); got != core.AirID {
		t.Fatalf("采掘后方块=%d，想要空气", got)
	}
	if got := companionItemCount(fixture.entry, core.ItemLeaves); got != 1 {
		t.Fatalf("伙伴树叶产物=%d，想要恰好 1", got)
	}
	if got := companionItemCount(fixture.entry, core.ItemSapling); got != 0 {
		t.Fatalf("伙伴背包出现树苗=%d，树叶→树苗判定必须只作用于玩家采掘", got)
	}
	// 伙伴产物直入背包：世界里既不得出现树叶掉落，也不得出现树苗掉落。
	if drops := miningDropTotals(miningTargetRecord(t, fixture.engine, target).Chunk); len(drops) != 0 {
		t.Fatalf("世界掉落物=%+v，伙伴采掘不得写入世界掉落物", drops)
	}
}
