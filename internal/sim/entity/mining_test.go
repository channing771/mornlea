package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

const miningTestPitch = float32(-0.4)

func TestMiningRule(t *testing.T) {
	tests := []struct {
		block       core.BlockID
		held        core.ItemID
		ticks       uint16
		harvestable bool
	}{
		{core.DirtID, core.ItemNone, 5, true},
		{core.GrassID, core.ItemDirt, 5, true},
		{core.StoneID, core.ItemNone, 30, true},
		{core.StoneID, core.ItemBrokenStonePickaxe, 30, true},
		{core.StoneID, core.ItemDirt, 30, false},
		{core.StoneID, core.ItemStonePickaxe, 15, true},
		{core.StoneID, core.ItemIronPickaxe, 8, true},
		{core.StoneBrickID, core.ItemNone, 30, false},
		{core.StoneBrickID, core.ItemStonePickaxe, 15, true},
		{core.FurnaceID, core.ItemIronPickaxe, 8, true},
		{core.CoalOreID, core.ItemStonePickaxe, 15, true},
		{core.CoalOreID, core.ItemBrokenIronPickaxe, 30, false},
		{core.IronOreID, core.ItemIronPickaxe, 8, true},
		{core.IronBlockID, core.ItemNone, 40, false},
		{core.IronBlockID, core.ItemStonePickaxe, 20, false},
		{core.IronBlockID, core.ItemIronPickaxe, 10, true},
		{core.LightBlockID, core.ItemNone, 30, false},
		{core.LightBlockID, core.ItemStonePickaxe, 15, true},
		{core.LightBlockID, core.ItemIronPickaxe, 8, true},
		{core.LightBlockID, core.ItemStone, 30, false},
		{core.BedrockID, core.ItemIronPickaxe, 0, false},
		{core.AirID, core.ItemNone, 0, false},
		{core.BarrierID, core.ItemIronPickaxe, 0, false},
	}
	for _, test := range tests {
		ticks, harvestable := miningRule(test.block, test.held)
		if ticks != test.ticks || harvestable != test.harvestable {
			t.Fatalf("miningRule(%d, %d) = %d, %v，想要 %d, %v",
				test.block, test.held, ticks, harvestable, test.ticks, test.harvestable)
		}
	}
}

func TestCommonBlockMaterialMiningRules(t *testing.T) {
	assertMiningRule := func(block core.BlockID, held core.ItemID, wantTicks uint16, wantHarvestable bool) {
		t.Helper()
		gotTicks, gotHarvestable := miningRule(block, held)
		if gotTicks != wantTicks || gotHarvestable != wantHarvestable {
			t.Fatalf("miningRule(%d,%d)=(%d,%v)，想要 (%d,%v)",
				block, held, gotTicks, gotHarvestable, wantTicks, wantHarvestable)
		}
	}
	for _, block := range []core.BlockID{
		core.SandID, core.GravelID, core.LeavesID, core.GlassID,
		core.WhiteWoolID, core.ClayID, core.SnowBlockID,
	} {
		assertMiningRule(block, core.ItemCoal, 5, true)
	}
	for _, block := range []core.BlockID{core.OakLogID, core.OakPlanksID} {
		assertMiningRule(block, core.ItemCoal, 15, true)
	}
	for _, block := range []core.BlockID{
		core.CobblestoneID, core.SmoothStoneID, core.BrickID,
		core.RoofTileID, core.MossyCobblestoneID,
	} {
		assertMiningRule(block, core.ItemNone, 30, true)
		assertMiningRule(block, core.ItemBrokenStonePickaxe, 30, true)
		assertMiningRule(block, core.ItemStonePickaxe, 15, true)
		assertMiningRule(block, core.ItemIronPickaxe, 8, true)
		assertMiningRule(block, core.ItemCoal, 30, false)
	}
}

func TestMiningProgressPublishesAndIncrementsExactlyOnce(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	session := sessions[0]
	result := finishWorldTick(engine)

	if len(result.Players) != 1 {
		t.Fatalf("Players=%+v，想要一个", result.Players)
	}
	want := MiningUpdate{
		Active: true, Target: targets[0], ProgressTicks: 1,
		RequiredTicks: 30, Harvestable: true,
	}
	if got := result.Players[0].Mining; got != want {
		t.Fatalf("首 tick Mining=%+v，想要 %+v", got, want)
	}

	finishWorldTick(engine)
	if got := engine.sessions[session].player.mining.update(); got.ProgressTicks != 2 {
		t.Fatalf("连续命中进度=%+v，想要恰好 2", got)
	}
}

func TestMiningReleaseClearsSameTick(t *testing.T) {
	engine, sessions, _ := readyMiningPlayers(t, 1)
	advanceMiningOnce(engine)
	player := engine.sessions[sessions[0]].player
	player.miningHeld = false
	advanceMiningOnce(engine)
	if player.mining != (miningState{}) {
		t.Fatalf("松键后 mining=%+v，想要零值", player.mining)
	}
}

// TestCombatHitClearsMiningOnlyForThatTick 覆盖命中实体时采掘进度必须清零；
// `miningHeld` 保留，下一 tick 仍由持续输入决定。
func TestCombatHitClearsMiningOnlyForThatTick(t *testing.T) {
	engine, sessions, _ := readyMiningPlayers(t, 1)
	player := engine.sessions[sessions[0]].player
	player.mining = miningState{progressTicks: 3, requiredTicks: 5}
	engine.RegisterSession(SessionID(2), core.Overworld, core.ChunkPos{})
	advanceActorsTick(engine)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 8.5}, 0)
	setMeleePlayer(engine, SessionID(2), mgl32.Vec3{0.5, 1, 7}, 0)
	player.pitch = miningTestPitch
	player.miningHeld = true

	tick := engine.beginTick()
	tick.context.AdvanceActors()
	tick.context.AdvanceHostiles(nil, &tick.result)
	tick.context.FinishWorld(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	publishFixture(engine, &tick)
	if player.mining != (miningState{}) {
		t.Fatalf("命中玩家后 mining=%+v，想要零值", player.mining)
	}
	if !player.miningHeld {
		t.Fatal("命中玩家后 miningHeld 被清空")
	}

	setMeleePlayer(engine, SessionID(2), mgl32.Vec3{4.5, 1, 7}, 0)
	next := engine.beginTick()
	next.context.AdvanceHostiles(nil, &next.result)
	next.context.FinishWorld(&next.result)
	commitMutation(next.mutation, &next.result)
	publishFixture(engine, &next)
	if player.mining.progressTicks != 1 {
		t.Fatalf("目标移出射线后的下一 tick mining=%+v，想要 progress=1", player.mining)
	}
}

// TestCombatMissKeepsMiningProgress 覆盖无合法实体目标时，既有采掘状态机逐 tick
// 保持不变。
func TestCombatMissKeepsMiningProgress(t *testing.T) {
	engine, sessions, _ := readyMiningPlayers(t, 1)
	player := engine.sessions[sessions[0]].player

	finishWorldTick(engine)
	if player.mining.progressTicks != 1 {
		t.Fatalf("无玩家目标首 tick mining=%+v，想要 progress=1", player.mining)
	}
	finishWorldTick(engine)
	if player.mining.progressTicks != 2 {
		t.Fatalf("无玩家目标次 tick mining=%+v，想要 progress=2", player.mining)
	}
}

func TestMiningTargetBlockAndToolChangesRestartAtOne(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Engine, *sessionState, core.BlockPos)
		want   core.BlockPos
	}{
		{
			name: "视角目标变化",
			mutate: func(engine *Engine, session *sessionState, _ core.BlockPos) {
				engine.SetBlockForTest(core.BlockPos{X: 1, Y: 1, Z: 5}, core.StoneID)
				session.player.yaw = -0.3
			},
			want: core.BlockPos{X: 1, Y: 1, Z: 5},
		},
		{
			name: "选中工具变化",
			mutate: func(_ *Engine, session *sessionState, _ core.BlockPos) {
				full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
				session.player.inventory.Hotbar.Slots[0] = core.ItemStack{
					Item: core.ItemStonePickaxe, Count: 1, Durability: full,
				}
			},
			want: core.BlockPos{X: 0, Y: 1, Z: 5},
		},
		{
			name: "方块 ID 变化",
			mutate: func(engine *Engine, _ *sessionState, target core.BlockPos) {
				engine.SetBlockForTest(target, core.DirtID)
			},
			want: core.BlockPos{X: 0, Y: 1, Z: 5},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			advanceMiningOnce(engine)
			session := engine.sessions[sessions[0]]
			test.mutate(engine, session, targets[0])
			advanceMiningOnce(engine)
			got := session.player.mining
			if got.target != test.want || got.progressTicks != 1 {
				t.Fatalf("变化后 mining=%+v，想要目标 %+v 从 1 开始", got, test.want)
			}
		})
	}
}

func TestMiningInvalidTargetsClearWithoutRejection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Engine, *sessionState, core.BlockPos)
	}{
		{
			name: "超距",
			mutate: func(engine *Engine, session *sessionState, target core.BlockPos) {
				engine.SetBlockForTest(target, core.AirID)
				engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 1}, core.StoneID)
				session.player.pitch = 0
			},
		},
		{
			name: "区块未就绪",
			mutate: func(engine *Engine, session *sessionState, target core.BlockPos) {
				engine.SetBlockForTest(target, core.AirID)
				session.player.state.Position = mgl32.Vec3{8.5, 1, 1.5}
				session.player.pitch = 0
				engine.dimension(core.Overworld).RequestUnload(core.ChunkPos{Z: -1})
			},
		},
		{
			name: "视野丢失",
			mutate: func(engine *Engine, session *sessionState, _ core.BlockPos) {
				engine.setSessionViewForTest(
					session.id,
					SessionView{Ready: false, Center: core.ChunkPos{}},
					false,
				)
			},
		},
		{
			name: "打开熔炉",
			mutate: func(_ *Engine, session *sessionState, _ core.BlockPos) {
				session.viewContainer = true
			},
		},
		{
			name: "reset",
			mutate: func(_ *Engine, session *sessionState, _ core.BlockPos) {
				session.player.reset = true
			},
		},
		{
			name: "基岩",
			mutate: func(engine *Engine, _ *sessionState, target core.BlockPos) {
				engine.SetBlockForTest(target, core.BedrockID)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			advanceMiningOnce(engine)
			session := engine.sessions[sessions[0]]
			test.mutate(engine, session, targets[0])
			result := advanceMiningOnce(engine)
			if session.player.mining != (miningState{}) {
				t.Fatalf("无效目标后 mining=%+v，想要零值", session.player.mining)
			}
			if len(result.Rejected) != 0 {
				t.Fatalf("无效目标产生拒绝=%+v", result.Rejected)
			}
		})
	}
}

func TestMiningLifecyclePathsClearIntentAndProgress(t *testing.T) {
	t.Run("beginReset", func(t *testing.T) {
		engine, sessions, _ := readyMiningPlayers(t, 1)
		advanceMiningOnce(engine)
		player := engine.sessions[sessions[0]].player
		player.attackCooldownTicks = 3
		player.hurtCooldownTicks = 4
		player.meleeSuppressedMining = true
		player.beginReset()
		if player.miningHeld || player.mining != (miningState{}) ||
			player.attackCooldownTicks != 0 || player.hurtCooldownTicks != 0 ||
			player.meleeSuppressedMining {
			t.Fatalf("beginReset 后 held=%v mining=%+v attack=%d hurt=%d suppressed=%v",
				player.miningHeld, player.mining, player.attackCooldownTicks,
				player.hurtCooldownTicks, player.meleeSuppressedMining)
		}
	})

	t.Run("非法输入", func(t *testing.T) {
		engine, sessions, _ := readyMiningPlayers(t, 1)
		advanceMiningOnce(engine)
		session := sessions[0]
		result := applyPlayerCommandsTick(engine, []Command{{
			Session: session, Sequence: 101, Kind: CommandPlayerInput,
			Yaw: float32(math.NaN()), Mining: true,
		}})
		player := engine.sessions[session].player
		if player.miningHeld || player.mining != (miningState{}) {
			t.Fatalf("非法输入后 held=%v mining=%+v", player.miningHeld, player.mining)
		}
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
			t.Fatalf("非法输入拒绝=%+v", result.Rejected)
		}
	})

	t.Run("未就绪输入", func(t *testing.T) {
		engine, sessions, _ := readyMiningPlayers(t, 1)
		advanceMiningOnce(engine)
		session := sessions[0]
		player := engine.sessions[session].player
		player.lifecycle = PlayerPendingSpawn
		result := applyPlayerCommandsTick(engine, []Command{{
			Session: session, Sequence: 102, Kind: CommandPlayerInput, Mining: true,
		}})
		if player.miningHeld || player.mining != (miningState{}) {
			t.Fatalf("未就绪输入后 held=%v mining=%+v", player.miningHeld, player.mining)
		}
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectPlayerNotReady {
			t.Fatalf("未就绪拒绝=%+v", result.Rejected)
		}
	})
}

func TestMiningEightSessionsKeepIndependentSortedStates(t *testing.T) {
	engine, _, targets := readyMiningPlayers(t, 8)
	result := advanceMiningOnce(engine)
	engine.publishPlayers(&result)
	if len(result.Players) != 8 {
		t.Fatalf("Players=%d，想要 8", len(result.Players))
	}
	for index, update := range result.Players {
		wantSession := SessionID(index + 1)
		if update.Session != wantSession {
			t.Fatalf("Players[%d].Session=%d，想要 %d", index, update.Session, wantSession)
		}
		want := MiningUpdate{
			Active: true, Target: targets[index], ProgressTicks: 1,
			RequiredTicks: 30, Harvestable: true,
		}
		if update.Mining != want {
			t.Fatalf("session %d Mining=%+v，想要 %+v", update.Session, update.Mining, want)
		}
	}
}

func TestMiningCompletionUsesFixedToolAndDropRules(t *testing.T) {
	tests := []struct {
		name     string
		block    core.BlockID
		held     core.ItemID
		ticks    int
		wantItem core.ItemID
	}{
		{"裸手采石掉落石头", core.StoneID, core.ItemNone, 30, core.ItemStone},
		{"普通物品采石不掉落", core.StoneID, core.ItemDirt, 30, core.ItemNone},
		{"石镐采煤矿", core.CoalOreID, core.ItemStonePickaxe, 15, core.ItemCoal},
		{"石镐采铁矿", core.IronOreID, core.ItemStonePickaxe, 15, core.ItemRawIron},
		{"错误工具采煤矿", core.CoalOreID, core.ItemDirt, 30, core.ItemNone},
		{"错误工具采铁矿", core.IronOreID, core.ItemDirt, 30, core.ItemNone},
		{"石镐采铁块不掉落", core.IronBlockID, core.ItemStonePickaxe, 20, core.ItemNone},
		{"铁镐采铁块", core.IronBlockID, core.ItemIronPickaxe, 10, core.ItemIronBlock},
		{"错误工具采发光块不掉落", core.LightBlockID, core.ItemStone, 30, core.ItemNone},
		{"石镐采发光块", core.LightBlockID, core.ItemStonePickaxe, 15, core.ItemLightBlock},
		{"铁镐采发光块", core.LightBlockID, core.ItemIronPickaxe, 8, core.ItemLightBlock},
		{"裸手采沙子", core.SandID, core.ItemNone, 5, core.ItemSand},
		{"普通物品采原木", core.OakLogID, core.ItemCoal, 15, core.ItemOakLog},
		{"石镐采圆石", core.CobblestoneID, core.ItemStonePickaxe, 15, core.ItemCobblestone},
		{"普通物品采砖块不掉落", core.BrickID, core.ItemCoal, 30, core.ItemNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			target := targets[0]
			engine.SetBlockForTest(target, test.block)
			setMiningHeldItem(engine.sessions[sessions[0]].player, test.held)
			var result TickResult
			for range test.ticks {
				result = advanceMiningOnce(engine)
			}
			record := miningTargetRecord(t, engine, target)
			x, _, z := target.Local()
			if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
				t.Fatalf("完成后方块=%d，想要空气", got)
			}
			if len(result.Rejected) != 0 {
				t.Fatalf("完成被拒绝=%+v", result.Rejected)
			}
			gotDrops := miningDropTotals(record.Chunk)
			if test.wantItem == core.ItemNone {
				if len(gotDrops) != 0 {
					t.Fatalf("错误工具掉落=%+v，想要空", gotDrops)
				}
			} else if gotDrops[test.wantItem] != 1 || len(gotDrops) != 1 {
				t.Fatalf("掉落=%+v，想要一个 %d", gotDrops, test.wantItem)
			}
		})
	}
}

func TestMiningConsumesOneDurabilityPerBrokenBlock(t *testing.T) {
	tests := []struct {
		name  string
		tool  core.ItemID
		block core.BlockID
		ticks int
	}{
		{name: "石镐", tool: core.ItemStonePickaxe, block: core.CoalOreID, ticks: 15},
		{name: "铁镐", tool: core.ItemIronPickaxe, block: core.IronBlockID, ticks: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			engine.SetBlockForTest(targets[0], test.block)
			setMiningHeldItem(player, test.tool)
			full, _ := core.ItemMaxDurability(test.tool)

			for range test.ticks {
				advanceMiningOnce(engine)
			}

			got := player.inventory.Hotbar.Slots[0]
			if got != (core.ItemStack{Item: test.tool, Count: 1, Durability: full - 1}) {
				t.Fatalf("破坏一个方块后栏位 = %+v，想要 %d 耐久 %d", got, test.tool, full-1)
			}
			if !player.inventoryDirty {
				t.Fatal("扣减耐久没有标记 inventoryDirty")
			}
		})
	}
}

func TestMiningIntactSwordsDoNotConsumeDurability(t *testing.T) {
	tests := []struct {
		name  string
		stack core.ItemStack
	}{
		{"木剑", core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: 29}},
		{"石剑", core.ItemStack{Item: core.ItemStoneSword, Count: 1, Durability: 65}},
		{"铁剑", core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 125}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			target := targets[0]
			engine.SetBlockForTest(target, core.DirtID)
			player.inventory.Hotbar.Slots[0] = test.stack

			var result TickResult
			for range 5 {
				result = advanceMiningOnce(engine)
			}

			if len(result.Rejected) != 0 {
				t.Fatalf("剑采掘泥土被拒绝 = %+v", result.Rejected)
			}
			record := miningTargetRecord(t, engine, target)
			x, _, z := target.Local()
			if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
				t.Fatalf("剑采掘完成后方块 = %d，想要空气", got)
			}
			if got := player.inventory.Hotbar.Slots[0]; got != test.stack {
				t.Fatalf("剑采掘后栏位 = %+v，想要保持 %+v", got, test.stack)
			}
			if player.inventoryDirty {
				t.Fatal("剑采掘不改背包，inventoryDirty 应保持 false")
			}
		})
	}
}

func TestMiningWrongToolStillConsumesDurability(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	player := engine.sessions[sessions[0]].player
	target := targets[0]
	engine.SetBlockForTest(target, core.IronBlockID)
	setMiningHeldItem(player, core.ItemStonePickaxe)
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)

	for range 20 {
		advanceMiningOnce(engine)
	}

	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("用错等级的工具完成后方块 = %d，想要空气", got)
	}
	if got := miningDropTotals(record.Chunk); len(got) != 0 {
		t.Fatalf("用错等级的工具产生了掉落 = %+v", got)
	}
	if got := player.inventory.Hotbar.Slots[0].Durability; got != full-1 {
		t.Fatalf("用错等级的工具破坏方块后耐久 = %d，想要 %d", got, full-1)
	}
	if !player.inventoryDirty {
		t.Fatal("用错等级的工具扣减耐久没有标记 inventoryDirty")
	}
}

// TestMiningHoeHarvestMatureCropKeepsDurability 覆盖 Scenario「锄头收获作物不扣
// 耐久」的成熟分支：完好石锄/铁锄收获成熟小麦，耐久保持满值（整 `core.ItemStack`
// 比较钉死栏位不变）、两类掉落各落在 [1,3] 区间（数量由确定性哈希决定）、
// `inventoryDirty` 保持 false——掉落进世界不触碰背包，豁免路径没有任何背包变化
// 需要发布。
func TestMiningHoeHarvestMatureCropKeepsDurability(t *testing.T) {
	tests := []struct {
		name string
		tool core.ItemID
	}{
		{name: "石锄", tool: core.ItemStoneHoe},
		{name: "铁锄", tool: core.ItemIronHoe},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			target := targets[0]
			engine.SetBlockForTest(target, core.WheatStage7ID)
			setMiningHeldItem(player, test.tool)
			full, _ := core.ItemMaxDurability(test.tool)

			result := advanceMiningOnce(engine)

			if len(result.Rejected) != 0 {
				t.Fatalf("锄头收获成熟小麦被拒绝 = %+v", result.Rejected)
			}
			record := miningTargetRecord(t, engine, target)
			x, _, z := target.Local()
			if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
				t.Fatalf("收获后方块 = %d，想要空气", got)
			}
			got := miningDropTotals(record.Chunk)
			if len(got) != 2 ||
				got[core.ItemWheat] < 1 || got[core.ItemWheat] > 3 ||
				got[core.ItemWheatSeeds] < 1 || got[core.ItemWheatSeeds] > 3 {
				t.Fatalf("成熟小麦掉落 = %+v，想要 1..3 小麦 + 1..3 种子", got)
			}
			want := core.ItemStack{Item: test.tool, Count: 1, Durability: full}
			if slot := player.inventory.Hotbar.Slots[0]; slot != want {
				t.Fatalf("锄头收获作物后栏位 = %+v，想要耐久保持 %d", slot, full)
			}
			if player.inventoryDirty {
				t.Fatal("豁免路径不改背包，inventoryDirty 应保持 false")
			}
		})
	}
}

// TestMiningHoeHarvestImmatureCropKeepsDurability 覆盖 Scenario「锄头收获作物不扣
// 耐久」的未成熟分支：豁免判据是 `core.IsCrop`（全部八个生长阶段），不是"成熟"；
// 未成熟掉落仍是恰好 1 颗种子。
func TestMiningHoeHarvestImmatureCropKeepsDurability(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	player := engine.sessions[sessions[0]].player
	target := targets[0]
	engine.SetBlockForTest(target, core.WheatStage3ID)
	setMiningHeldItem(player, core.ItemStoneHoe)
	full, _ := core.ItemMaxDurability(core.ItemStoneHoe)

	result := advanceMiningOnce(engine)

	if len(result.Rejected) != 0 {
		t.Fatalf("锄头收获未成熟作物被拒绝 = %+v", result.Rejected)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("收获后方块 = %d，想要空气", got)
	}
	got := miningDropTotals(record.Chunk)
	if len(got) != 1 || got[core.ItemWheatSeeds] != 1 {
		t.Fatalf("未成熟作物掉落 = %+v，想要恰好 1 颗种子", got)
	}
	want := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: full}
	if slot := player.inventory.Hotbar.Slots[0]; slot != want {
		t.Fatalf("锄头收获未成熟作物后栏位 = %+v，想要耐久保持 %d", slot, full)
	}
	if player.inventoryDirty {
		t.Fatal("豁免路径不改背包，inventoryDirty 应保持 false")
	}
}

// TestMiningHoeHarvestExemptionExcludesPickaxe 是豁免的工具侧对照：持石镐收获
// 成熟作物仍按既有规则扣恰好一点耐久并标记 `inventoryDirty`——豁免只认锄头
// （`core.TillingTool`），不外溢到其他工具。
func TestMiningHoeHarvestExemptionExcludesPickaxe(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	player := engine.sessions[sessions[0]].player
	engine.SetBlockForTest(targets[0], core.WheatStage7ID)
	setMiningHeldItem(player, core.ItemStonePickaxe)
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)

	result := advanceMiningOnce(engine)

	if len(result.Rejected) != 0 {
		t.Fatalf("石镐收获成熟作物被拒绝 = %+v", result.Rejected)
	}
	want := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full - 1}
	if slot := player.inventory.Hotbar.Slots[0]; slot != want {
		t.Fatalf("石镐收获作物后栏位 = %+v，想要耐久 %d", slot, full-1)
	}
	if !player.inventoryDirty {
		t.Fatal("石镐收获作物扣减耐久没有标记 inventoryDirty")
	}
}

// TestMiningHoeHarvestExemptionExcludesNonCrop 是豁免的方块侧对照：持完好石锄
// 破坏泥土（5 tick）仍扣恰好一点耐久——豁免只认作物（`core.IsCrop`），不外溢
// 到非作物方块。
func TestMiningHoeHarvestExemptionExcludesNonCrop(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	player := engine.sessions[sessions[0]].player
	engine.SetBlockForTest(targets[0], core.DirtID)
	setMiningHeldItem(player, core.ItemStoneHoe)
	full, _ := core.ItemMaxDurability(core.ItemStoneHoe)

	for range 5 {
		advanceMiningOnce(engine)
	}

	record := miningTargetRecord(t, engine, targets[0])
	x, _, z := targets[0].Local()
	if got := record.Chunk.BlockAt(x, targets[0].Y, z); got != core.AirID {
		t.Fatalf("石锄破坏泥土后方块 = %d，想要空气", got)
	}
	want := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: full - 1}
	if slot := player.inventory.Hotbar.Slots[0]; slot != want {
		t.Fatalf("石锄破坏泥土后栏位 = %+v，想要耐久 %d", slot, full-1)
	}
	if !player.inventoryDirty {
		t.Fatal("石锄破坏泥土扣减耐久没有标记 inventoryDirty")
	}
}

func TestMiningTurnsToolIntoBrokenFormAtZero(t *testing.T) {
	tests := []struct {
		name     string
		tool     core.ItemID
		broken   core.ItemID
		block    core.BlockID
		wantDrop core.ItemID
		ticks    int
	}{
		{
			name: "石镐", tool: core.ItemStonePickaxe, broken: core.ItemBrokenStonePickaxe,
			block: core.CoalOreID, wantDrop: core.ItemCoal, ticks: 15,
		},
		{
			name: "铁镐", tool: core.ItemIronPickaxe, broken: core.ItemBrokenIronPickaxe,
			block: core.IronBlockID, wantDrop: core.ItemIronBlock, ticks: 10,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			target := targets[0]
			engine.SetBlockForTest(target, test.block)
			setMiningHeldItem(player, test.tool)
			player.inventory.Hotbar.Slots[0].Durability = 1

			var result TickResult
			for range test.ticks {
				result = advanceMiningOnce(engine)
			}

			if len(result.Rejected) != 0 {
				t.Fatalf("最后一次采掘被拒绝 = %+v", result.Rejected)
			}
			record := miningTargetRecord(t, engine, target)
			x, _, z := target.Local()
			if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
				t.Fatalf("完成后方块 = %d，想要空气", got)
			}
			if got := miningDropTotals(record.Chunk); got[test.wantDrop] != 1 || len(got) != 1 {
				t.Fatalf("最后一次采掘掉落 = %+v，想要一个 %d", got, test.wantDrop)
			}
			if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: test.broken, Count: 1}) {
				t.Fatalf("耐久耗尽后栏位 = %+v，想要一个损坏工具 %d", got, test.broken)
			}
			if !player.inventoryDirty {
				t.Fatal("工具损坏没有标记 inventoryDirty")
			}
		})
	}
}

func TestMiningProtectedAndUnreadyRejectionsPreserveDurability(t *testing.T) {
	tests := []struct {
		name   string
		block  core.BlockID
		want   RejectReason
		mutate func(*Engine, core.BlockPos)
	}{
		{
			name: "受保护方块", block: core.BedrockID, want: RejectProtectedBlock,
			mutate: func(engine *Engine, target core.BlockPos) {
				engine.SetBlockForTest(target, core.BedrockID)
			},
		},
		{
			name: "区块未就绪", block: core.StoneID, want: RejectChunkNotReady,
			mutate: func(engine *Engine, target core.BlockPos) {
				engine.dimension(core.Overworld).RequestUnload(target.Chunk())
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			setMiningHeldItem(player, core.ItemStonePickaxe)
			before := player.inventory.Hotbar.Slots[0]
			target := targets[0]
			test.mutate(engine, target)

			reason, rejected := engine.completeMining(
				core.Overworld,
				target,
				test.block,
				true,
				engine.newMutation(),
			)
			if !rejected || reason != test.want {
				t.Fatalf("拒绝 = (%d, %v)，想要 (%d, true)", reason, rejected, test.want)
			}
			if got := player.inventory.Hotbar.Slots[0]; got != before {
				t.Fatalf("拒绝路径修改了工具：got=%+v want=%+v", got, before)
			}
			if player.inventoryDirty {
				t.Fatal("拒绝路径标记了 inventoryDirty")
			}
		})
	}
}

func TestCompleteMiningRejectsNoOp(t *testing.T) {
	tests := []struct {
		name  string
		block core.BlockID
		setup func(*testing.T, *Engine, core.BlockPos)
	}{
		{
			name:  "普通方块",
			block: core.StoneID,
			setup: func(_ *testing.T, engine *Engine, target core.BlockPos) {
				engine.SetBlockForTest(target, core.AirID)
			},
		},
		{
			name:  "熔炉",
			block: core.FurnaceID,
			setup: func(t *testing.T, engine *Engine, target core.BlockPos) {
				setMiningFurnace(t, engine, target, world.FurnaceSlot{Generation: 1})
				engine.SetBlockForTest(target, core.AirID)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, _, targets := readyMiningPlayers(t, 1)
			target := targets[0]
			test.setup(t, engine, target)
			record := miningTargetRecord(t, engine, target)
			beforeHash := record.Chunk.Hash()
			beforeDropsHash := record.Chunk.DropsHash()
			beforeFurnace := record.Chunk.Furnace(0)
			beforeRevision := record.Revision
			pending := engine.newMutation()

			reason, rejected := engine.completeMining(
				core.Overworld, target, test.block, true, pending,
			)

			if !rejected || reason != RejectNoTarget {
				t.Fatalf("no-op 完成结果 = (%d, %v)，想要 (%d, true)",
					reason, rejected, RejectNoTarget)
			}
			if got := record.Chunk.Hash(); got != beforeHash {
				t.Fatalf("no-op 完成修改了方块：hash=%x/%x", got, beforeHash)
			}
			if got := record.Chunk.DropsHash(); got != beforeDropsHash {
				t.Fatalf("no-op 完成修改了掉落物：hash=%x/%x", got, beforeDropsHash)
			}
			if got := record.Chunk.Furnace(0); got != beforeFurnace {
				t.Fatalf("no-op 完成修改了熔炉：got=%+v want=%+v", got, beforeFurnace)
			}
			if record.Revision != beforeRevision || pending.Len() != 0 {
				t.Fatalf("no-op 完成修改了 revision 或 pending：revision=%d/%d pending=%+v",
					record.Revision, beforeRevision, pending)
			}
		})
	}
}

func TestMiningDoesNotConsumeDurabilityFromNonTools(t *testing.T) {
	tests := []struct {
		name  string
		stack core.ItemStack
	}{
		{name: "空手"},
		{name: "普通物品", stack: core.ItemStack{Item: core.ItemDirt, Count: 1}},
		{name: "损坏物品", stack: core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			engine.SetBlockForTest(targets[0], core.DirtID)
			player.inventory.Hotbar.Slots[0] = test.stack

			for range 5 {
				advanceMiningOnce(engine)
			}

			if got := player.inventory.Hotbar.Slots[0]; got != test.stack {
				t.Fatalf("非工具栏位被修改：got=%+v want=%+v", got, test.stack)
			}
			if player.inventoryDirty {
				t.Fatal("非工具完成采掘却标记了 inventoryDirty")
			}
		})
	}
}

func TestBrokenToolMinesLikeBareHand(t *testing.T) {
	blocks := []core.BlockID{
		core.DirtID,
		core.GrassID,
		core.StoneID,
		core.StoneBrickID,
		core.FurnaceID,
		core.CoalOreID,
		core.IronOreID,
		core.IronBlockID,
		core.BedrockID,
		core.BarrierID,
	}
	for _, block := range blocks {
		bareTicks, bareHarvest := miningRule(block, core.ItemNone)
		for _, broken := range []core.ItemID{
			core.ItemBrokenStonePickaxe,
			core.ItemBrokenIronPickaxe,
		} {
			ticks, harvest := miningRule(block, broken)
			if ticks != bareTicks || harvest != bareHarvest {
				t.Fatalf("方块 %d 损坏工具 %d = (%d,%v)，空手 = (%d,%v)，两者必须一致",
					block, broken, ticks, harvest, bareTicks, bareHarvest)
			}
		}
	}
}

func TestMiningResetsProgressWhenHeldToolLeavesHand(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*playerState)
	}{
		{name: "换到空栏位", mutate: func(player *playerState) {
			player.inventory.Hotbar.Selected = 5
		}},
		{name: "丢弃工具", mutate: func(player *playerState) {
			player.inventory.Hotbar.Slots[0] = core.ItemStack{}
		}},
		{name: "工具损坏", mutate: func(player *playerState) {
			player.inventory.Hotbar.Slots[0] = core.ItemStack{
				Item: core.ItemBrokenStonePickaxe, Count: 1,
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			player := engine.sessions[sessions[0]].player
			engine.SetBlockForTest(targets[0], core.CoalOreID)
			setMiningHeldItem(player, core.ItemStonePickaxe)

			for range 3 {
				advanceMiningOnce(engine)
			}
			if player.mining.progressTicks != 3 {
				t.Fatalf("进度 = %d，想要 3", player.mining.progressTicks)
			}

			test.mutate(player)
			advanceMiningOnce(engine)

			if player.mining.progressTicks != 1 {
				t.Fatalf("工具离手后进度 = %d，想要重新从 1 开始", player.mining.progressTicks)
			}
			if player.mining.held == core.ItemStonePickaxe {
				t.Fatal("状态机仍记录着已经离手的石镐")
			}
		})
	}
}

func TestMiningHarvestableCapacityFailureIsAtomicRejectsOnceAndPreservesDurability(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	session, target := sessions[0], targets[0]
	player := engine.sessions[session].player
	engine.SetBlockForTest(target, core.LightBlockID)
	setMiningHeldItem(player, core.ItemStonePickaxe)
	beforeTool := player.inventory.Hotbar.Slots[0]
	fillMiningDrops(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeHash := record.Chunk.Hash()
	beforeDropsHash := record.Chunk.DropsHash()
	beforeRevision := record.Revision

	var result TickResult
	for range 15 {
		result = advanceMiningOnce(engine)
	}
	if len(result.Rejected) != 1 || result.Rejected[0] != (Rejection{
		Session: session, Sequence: 10, Reason: RejectDropCapacity,
	}) {
		t.Fatalf("容量拒绝=%+v", result.Rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash || record.Revision != beforeRevision {
		t.Fatalf("容量失败修改了区块或 revision: hash=%x/%x revision=%d/%d",
			got, beforeHash, record.Revision, beforeRevision)
	}
	if got := record.Chunk.DropsHash(); got != beforeDropsHash {
		t.Fatalf("容量失败修改了掉落槽: drops=%x/%x", got, beforeDropsHash)
	}
	if engine.sessions[session].player.mining != (miningState{}) {
		t.Fatalf("容量失败后 mining=%+v，想要清零", engine.sessions[session].player.mining)
	}
	if got := player.inventory.Hotbar.Slots[0]; got != beforeTool {
		t.Fatalf("容量拒绝修改了工具：got=%+v want=%+v", got, beforeTool)
	}
	if player.inventoryDirty {
		t.Fatal("容量拒绝标记了 inventoryDirty")
	}

	next := advanceMiningOnce(engine)
	if len(next.Rejected) != 0 || engine.sessions[session].player.mining.progressTicks != 1 {
		t.Fatalf("继续按住下一 tick=%+v mining=%+v，想要新一轮 1",
			next.Rejected, engine.sessions[session].player.mining)
	}
}

func TestMiningWrongToolCompletesWithFullDropCapacity(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	engine.SetBlockForTest(target, core.LightBlockID)
	setMiningHeldItem(engine.sessions[sessions[0]].player, core.ItemDirt)
	fillMiningDrops(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeDrops := miningDropTotals(record.Chunk)
	beforeRevision := record.Revision

	var result TickResult
	for range 30 {
		result = advanceMiningOnce(engine)
	}
	if len(result.Rejected) != 0 {
		t.Fatalf("错误工具被掉落容量阻止=%+v", result.Rejected)
	}
	x, _, z := target.Local()
	revision := requireChunkInfo(t, engine.dimension(core.Overworld), target.Chunk()).Revision
	if record.Chunk.BlockAt(x, target.Y, z) != core.AirID || revision != beforeRevision+1 {
		t.Fatalf("错误工具未原子完成: revision=%d want=%d", revision, beforeRevision+1)
	}
	if got := miningDropTotals(record.Chunk); !equalMiningDropTotals(got, beforeDrops) {
		t.Fatalf("错误工具修改了掉落物: got=%+v want=%+v", got, beforeDrops)
	}
}

func TestMiningFurnaceCompletionDropsBodyByToolAndPreservesContents(t *testing.T) {
	tests := []struct {
		name     string
		held     core.ItemID
		ticks    int
		fuel     uint8
		wantBody uint8
	}{
		{"错误工具只掉非空内容", core.ItemDirt, 30, 0, 0},
		{"石镐掉本体和全部内容", core.ItemStonePickaxe, 15, 3, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions, targets := readyMiningPlayers(t, 1)
			target := targets[0]
			setMiningHeldItem(engine.sessions[sessions[0]].player, test.held)
			furnace := world.FurnaceSlot{
				Generation: 3,
				Input:      core.ItemStack{Item: core.ItemRawIron, Count: 2},
				Output:     core.ItemStack{Item: core.ItemIronIngot, Count: 4},
			}
			if test.fuel != 0 {
				furnace.Fuel = core.ItemStack{Item: core.ItemCoal, Count: test.fuel}
			}
			furnace = setMiningFurnace(t, engine, target, furnace)
			for range test.ticks {
				advanceMiningOnce(engine)
			}
			record := miningTargetRecord(t, engine, target)
			if got := record.Chunk.Furnace(0); got.Active || got.Generation != 3 {
				t.Fatalf("完成后熔炉槽=%+v", got)
			}
			drops := miningDropTotals(record.Chunk)
			wantKinds := 2 + int(test.wantBody)
			if test.fuel != 0 {
				wantKinds++
			}
			if drops[core.ItemFurnace] != test.wantBody ||
				drops[furnace.Input.Item] != furnace.Input.Count ||
				drops[furnace.Output.Item] != furnace.Output.Count ||
				drops[furnace.Fuel.Item] != furnace.Fuel.Count ||
				len(drops) != wantKinds {
				t.Fatalf("熔炉掉落=%+v，body=%d", drops, test.wantBody)
			}
		})
	}
}

func TestMiningWrongToolEmptyFurnaceDropsNothing(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	setMiningHeldItem(engine.sessions[sessions[0]].player, core.ItemDirt)
	setMiningFurnace(t, engine, target, world.FurnaceSlot{Generation: 7})
	for range 30 {
		advanceMiningOnce(engine)
	}
	record := miningTargetRecord(t, engine, target)
	if drops := miningDropTotals(record.Chunk); len(drops) != 0 {
		t.Fatalf("错误工具破坏空熔炉掉落=%+v，想要空", drops)
	}
	if got := record.Chunk.Furnace(0); got.Active || got.Generation != 7 {
		t.Fatalf("空熔炉完成后槽=%+v", got)
	}
}

func TestMiningFurnaceBatchCapacityFailureIsAtomic(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	setMiningHeldItem(engine.sessions[sessions[0]].player, core.ItemIronPickaxe)
	setMiningFurnace(t, engine, target, world.FurnaceSlot{
		Generation: 5,
		Input:      core.ItemStack{Item: core.ItemRawIron, Count: 1},
		Fuel:       core.ItemStack{Item: core.ItemCoal, Count: 1},
		Output:     core.ItemStack{Item: core.ItemIronIngot, Count: 1},
	})
	fillMiningDrops(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeHash := record.Chunk.Hash()
	beforeRevision := record.Revision

	var result TickResult
	for range 8 {
		result = advanceMiningOnce(engine)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectDropCapacity {
		t.Fatalf("熔炉容量拒绝=%+v", result.Rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash || record.Revision != beforeRevision {
		t.Fatalf("熔炉容量失败修改状态: hash=%x/%x revision=%d/%d",
			got, beforeHash, record.Revision, beforeRevision)
	}
}

func TestMiningTwoSessionsCompleteOneTargetOnce(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 2)
	target := core.BlockPos{X: 0, Y: 2, Z: 5}
	engine.SetBlockForTest(targets[0], core.AirID)
	engine.SetBlockForTest(targets[1], core.AirID)
	engine.SetBlockForTest(target, core.StoneID)
	for _, id := range sessions {
		player := engine.sessions[id].player
		player.state.Position = mgl32.Vec3{0.5, 1, 8.5}
		player.pitch = 0
		setMiningHeldItem(player, core.ItemStonePickaxe)
	}
	for range 14 {
		advanceMiningOnce(engine)
	}
	result := advanceMiningOnce(engine)
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 {
		t.Fatalf("竞争完成 Changes=%+v，想要一次", result.Changes)
	}
	record := miningTargetRecord(t, engine, target)
	if drops := miningDropTotals(record.Chunk); drops[core.ItemStone] != 1 || len(drops) != 1 {
		t.Fatalf("竞争完成掉落=%+v，想要一个石头", drops)
	}
	if engine.sessions[sessions[1]].player.mining != (miningState{}) {
		t.Fatalf("后处理会话未看到空气并清零: %+v", engine.sessions[sessions[1]].player.mining)
	}
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	if got := engine.sessions[sessions[0]].player.inventory.Hotbar.Slots[0].Durability; got != full-1 {
		t.Fatalf("实际移除方块的玩家耐久 = %d，想要 %d", got, full-1)
	}
	if got := engine.sessions[sessions[1]].player.inventory.Hotbar.Slots[0].Durability; got != full {
		t.Fatalf("未移除方块的玩家耐久 = %d，想要保持 %d", got, full)
	}
}

func TestAuthoritativeMiningEightPlayersDoesNotAllocate(t *testing.T) {
	engine, sessions, _ := readyMiningPlayers(t, 8)
	pending := engine.newMutation()
	result := TickResult{}
	engine.advanceMining(pending, &result)
	for _, session := range engine.sessions {
		session.player.mining = miningState{}
	}

	allocations := testing.AllocsPerRun(1000, func() {
		result.Rejected = result.Rejected[:0]
		engine.advanceMining(pending, &result)
		for _, session := range engine.sessions {
			session.player.mining = miningState{}
		}
	})
	if allocations != 0 {
		t.Fatalf("八人采掘每 tick 分配=%f，想要 0", allocations)
	}

	engine.advanceMining(pending, &result)
	for _, id := range sessions {
		if got := engine.sessions[id].player.mining.progressTicks; got != 1 {
			t.Fatalf("session %d 预热后单次进度=%d，想要 1", id, got)
		}
	}
}

func BenchmarkAuthoritativeMiningEightPlayers(b *testing.B) {
	engine := readyMiningBenchmarkPlayers(b)
	pending := engine.newMutation()
	result := TickResult{}
	engine.advanceMining(pending, &result)
	for _, session := range engine.sessions {
		session.player.mining = miningState{}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result.Rejected = result.Rejected[:0]
		engine.advanceMining(pending, &result)
		for _, session := range engine.sessions {
			session.player.mining = miningState{}
		}
	}
}

func advanceMiningOnce(engine *Engine) TickResult {
	tick := engine.beginTick()
	tick.context.FinishWorld(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	return tick.result
}

func setMiningHeldItem(player *playerState, item core.ItemID) {
	if item == core.ItemNone {
		player.inventory.Hotbar.Slots[0] = core.ItemStack{}
		return
	}
	full, _ := core.ItemMaxDurability(item)
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: item, Count: 1, Durability: full}
}

type miningTargetChunk struct {
	Chunk    *world.Chunk
	Revision uint64
}

func miningTargetRecord(t *testing.T, engine *Engine, target core.BlockPos) miningTargetChunk {
	t.Helper()
	info, exists := engine.dimension(core.Overworld).Info(target.Chunk())
	if !exists || info.Chunk == nil {
		t.Fatalf("目标区块 %+v 未就绪", target.Chunk())
	}
	return miningTargetChunk{Chunk: info.Chunk, Revision: info.Revision}
}

func miningDropTotals(chunk *world.Chunk) map[core.ItemID]uint8 {
	totals := make(map[core.ItemID]uint8)
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if drop.Active {
			totals[drop.Stack.Item] += drop.Stack.Count
		}
	}
	return totals
}

func equalMiningDropTotals(left, right map[core.ItemID]uint8) bool {
	if len(left) != len(right) {
		return false
	}
	for item, count := range left {
		if right[item] != count {
			return false
		}
	}
	return true
}

func fillMiningDrops(engine *Engine, target core.BlockPos) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: target.Chunk()}
	for slot := range core.DropsPerChunk {
		engine.SetChunkDropForTest(key, slot, world.DropSlot{
			Generation: 1,
			Active:     true,
			Stack:      core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount},
		})
	}
}

func setMiningFurnace(
	t *testing.T,
	engine *Engine,
	target core.BlockPos,
	furnace world.FurnaceSlot,
) world.FurnaceSlot {
	t.Helper()
	index, ok := world.ChunkBlockIndex(target)
	if !ok {
		t.Fatalf("熔炉目标 %+v 没有区块索引", target)
	}
	engine.SetBlockForTest(target, core.FurnaceID)
	furnace.Active = true
	furnace.BlockIndex = index
	engine.SetChunkFurnaceForTest(
		core.ChunkKey{Dimension: core.Overworld, Pos: target.Chunk()}, 0, furnace,
	)
	return furnace
}

func readyMiningPlayers(
	t *testing.T,
	count int,
) (*Engine, []SessionID, []core.BlockPos) {
	t.Helper()
	engine, _ := readyMovementPlayer(t)
	for id := count; id >= 2; id-- {
		engine.RegisterSession(SessionID(id), core.Overworld, core.ChunkPos{})
	}
	if count > 1 {
		advanceActorsTick(engine)
	}
	sessions := make([]SessionID, count)
	targets := make([]core.BlockPos, count)
	for index := range count {
		id := SessionID(index + 1)
		sessions[index] = id
		target := core.BlockPos{X: int32(index * 2), Y: 1, Z: 5}
		targets[index] = target
		engine.SetBlockForTest(target, core.StoneID)
		session := engine.sessions[id]
		session.player.state.Position = mgl32.Vec3{float32(index*2) + 0.5, 1, 8.5}
		session.player.yaw = 0
		session.player.pitch = miningTestPitch
		session.player.miningHeld = true
		session.player.lastInputSequence = uint64(10 + index)
		session.player.reset = false
	}
	return engine, sessions, targets
}

func readyMiningBenchmarkPlayers(b *testing.B) *Engine {
	b.Helper()
	engine := NewEngine(0, 0, 0)
	dimension := engine.dimension(core.Overworld)
	if !dimension.BeginGeneration(core.ChunkPos{}) {
		b.Fatal("benchmark 区块未开始生成")
	}
	if err := dimension.ApplyGenerated(core.ChunkPos{}, movementFlatChunk(core.ChunkPos{})); err != nil {
		b.Fatal(err)
	}
	for id := 8; id >= 1; id-- {
		engine.RegisterSession(SessionID(id), core.Overworld, core.ChunkPos{})
	}
	engine.advancePendingPlayers()
	for index := range 8 {
		id := SessionID(index + 1)
		target := core.BlockPos{X: int32(index * 2), Y: 1, Z: 5}
		engine.SetBlockForTest(target, core.StoneID)
		session := engine.sessions[id]
		session.player.state.Position = mgl32.Vec3{float32(index*2) + 0.5, 1, 8.5}
		session.player.pitch = miningTestPitch
		session.player.miningHeld = true
		session.player.reset = false
	}
	return engine
}

func TestMiningRuleOrdinaryItemsAreNotBareHand(t *testing.T) {
	ordinary := [...]core.ItemID{
		core.ItemStone,
		core.ItemDirt,
		core.ItemGrass,
		core.ItemStoneBrick,
		core.ItemCoal,
		core.ItemRawIron,
		core.ItemIronIngot,
		core.ItemFurnace,
		core.ItemIronBlock,
	}
	for _, held := range ordinary {
		ticks, drop := miningRule(core.StoneID, held)
		if ticks != 30 || drop {
			t.Fatalf("普通物品 %d 采石规则 = %d, %v，想要 30, false", held, ticks, drop)
		}
	}
}

func TestHarvestPotatoMature1to4(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	engine.SetBlockForTest(target, core.PotatoStage7ID)
	tickBefore := engine.tick.Load()
	seed := engine.SeedForTest()
	result := advanceMiningOnce(engine)
	if len(result.Rejected) != 0 {
		t.Fatalf("PotatoStage7 mature harvest rejected=%+v", result.Rejected)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("PotatoStage7 harvest后方块=%d, want Air", got)
	}
	drops := miningDropTotals(record.Chunk)
	n := drops[core.ItemPotato]
	if n < 1 || n > 4 {
		t.Fatalf("PotatoStage7 mature count=%d not in [1,4] drops=%+v", n, drops)
	}
	expected := cropYieldRollsPotato(seed, tickBefore, core.Overworld, target)
	if n != expected {
		t.Fatalf("Potato yield %d != expected hash %d seed=%d tick=%d pos=%v", n, expected, seed, tickBefore, target)
	}
	// 重放一致
	a := cropYieldRollsPotato(seed, tickBefore, core.Overworld, target)
	b := cropYieldRollsPotato(seed, tickBefore, core.Overworld, target)
	if a != b {
		t.Fatalf("cropYieldRollsPotato not deterministic %d vs %d", a, b)
	}
	// 未成熟应只掉1个自身
	engine2, _, targets2 := readyMiningPlayers(t, 1)
	target2 := targets2[0]
	engine2.SetBlockForTest(target2, core.PotatoStage3ID)
	result2 := advanceMiningOnce(engine2)
	if len(result2.Rejected) != 0 {
		t.Fatalf("PotatoStage3 harvest rejected=%+v", result2.Rejected)
	}
	drops2 := miningDropTotals(miningTargetRecord(t, engine2, target2).Chunk)
	if drops2[core.ItemPotato] != 1 || len(drops2) != 1 {
		t.Fatalf("PotatoStage3 unripe drops=%+v want 1 potato", drops2)
	}
	_ = sessions
}

func TestHarvestCarrotMature1to4(t *testing.T) {
	engine, _, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	engine.SetBlockForTest(target, core.CarrotStage7ID)
	tickBefore := engine.tick.Load()
	seed := engine.SeedForTest()
	result := advanceMiningOnce(engine)
	if len(result.Rejected) != 0 {
		t.Fatalf("CarrotStage7 mature harvest rejected=%+v", result.Rejected)
	}
	record := miningTargetRecord(t, engine, target)
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
		t.Fatalf("CarrotStage7 harvest后方块=%d, want Air", got)
	}
	drops := miningDropTotals(record.Chunk)
	n := drops[core.ItemCarrot]
	if n < 1 || n > 4 {
		t.Fatalf("CarrotStage7 mature count=%d not in [1,4] drops=%+v", n, drops)
	}
	expected := cropYieldRollsCarrot(seed, tickBefore, core.Overworld, target)
	if n != expected {
		t.Fatalf("Carrot yield %d != expected %d seed=%d tick=%d", n, expected, seed, tickBefore)
	}
	a := cropYieldRollsCarrot(seed, tickBefore, core.Overworld, target)
	b := cropYieldRollsCarrot(seed, tickBefore, core.Overworld, target)
	if a != b {
		t.Fatalf("cropYieldRollsCarrot not deterministic")
	}
	// 未成熟
	engine2, _, targets2 := readyMiningPlayers(t, 1)
	target2 := targets2[0]
	engine2.SetBlockForTest(target2, core.CarrotStage3ID)
	result2 := advanceMiningOnce(engine2)
	if len(result2.Rejected) != 0 {
		t.Fatalf("CarrotStage3 harvest rejected=%+v", result2.Rejected)
	}
	drops2 := miningDropTotals(miningTargetRecord(t, engine2, target2).Chunk)
	if drops2[core.ItemCarrot] != 1 || len(drops2) != 1 {
		t.Fatalf("CarrotStage3 unripe drops=%+v want 1 carrot", drops2)
	}
}

func TestPoisonousPotato2Percent(t *testing.T) {
	// 枚举 200 个 pos，统计 poisonRoll 为真者 ≈2%，且同一输入重放一致
	trues := 0
	total := 0
	for x := int32(0); x < 200; x++ {
		pos := core.BlockPos{X: x, Y: 2, Z: 5}
		a := poisonRoll(42, 100, core.Overworld, pos)
		b := poisonRoll(42, 100, core.Overworld, pos)
		if a != b {
			t.Fatalf("poisonRoll not deterministic at %v %v vs %v", pos, a, b)
		}
		if a {
			trues++
		}
		total++
	}
	if trues == 0 || trues == total {
		t.Fatalf("poisonRoll degenerated trues=%d total=%d", trues, total)
	}
	ratio := float64(trues) / float64(total)
	if ratio < 0.005 || ratio > 0.05 {
		t.Fatalf("poisonRoll ratio %.3f not ~0.02 trues=%d total=%d", ratio, trues, total)
	}
	// 集成：对成熟马铃薯的实际收获验证毒土豆与收获数量一致
	// 在同一区块内寻找 poison 为真与为假的坐标，避免跨区块未就绪
	engine, _, targets := readyMiningPlayers(t, 1)
	session := SessionID(1)
	tickBefore := engine.tick.Load()
	seed := engine.SeedForTest()
	// 候选坐标会改写采掘夹具预置的石头目标；先清理它，避免它
	// 恰好落在玩家与作物之间时遮挡权威射线。
	engine.SetBlockForTest(targets[0], core.AirID)
	var truePos, falsePos *core.BlockPos
	for x := int32(0); x < 16; x++ {
		for z := int32(0); z < 16; z++ {
			pos := core.BlockPos{X: x, Y: 1, Z: z}
			if pos.Chunk() != targets[0].Chunk() {
				continue
			}
			isPoison := poisonRoll(seed, tickBefore, core.Overworld, pos)
			if isPoison && truePos == nil {
				cp := pos
				truePos = &cp
			}
			if !isPoison && falsePos == nil {
				cp := pos
				falsePos = &cp
			}
			if truePos != nil && falsePos != nil {
				break
			}
		}
		if truePos != nil && falsePos != nil {
			break
		}
	}
	if truePos == nil || falsePos == nil {
		t.Fatalf("failed to find true/false poison pos in chunk seed=%d tick=%d", seed, tickBefore)
	}
	// 真值位置收获应有毒土豆
	target := *truePos
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{float32(target.X) + 0.5, 1, float32(target.Z) + 3.5}
	player.yaw = 0
	player.pitch = miningTestPitch
	engine.SetBlockForTest(target, core.PotatoStage7ID)
	wantPoison := poisonRoll(seed, tickBefore, core.Overworld, target)
	result := advanceMiningOnce(engine)
	if len(result.Rejected) != 0 {
		t.Fatalf("potato mature at truePos rejected=%+v", result.Rejected)
	}
	drops := miningDropTotals(miningTargetRecord(t, engine, target).Chunk)
	if wantPoison {
		if drops[core.ItemPoisonousPotato] != 1 {
			t.Fatalf("expected poisonous at %v drops=%+v", target, drops)
		}
	} else {
		if drops[core.ItemPoisonousPotato] != 0 {
			t.Fatalf("unexpected poisonous at %v drops=%+v", target, drops)
		}
	}
	// 假值位置：新建引擎保持同一 tick/seed 以重放一致
	engine2, _, targets2 := readyMiningPlayers(t, 1)
	// 调整 tick 到与第一个引擎相同的 tickBefore（readyMiningPlayers 每次 tick 可能相同，但显式对齐）
	engine2.tick.Store(tickBefore)
	seed2 := engine2.SeedForTest()
	engine2.SetBlockForTest(targets2[0], core.AirID)
	target2 := *falsePos
	if target2.Chunk() != targets2[0].Chunk() {
		t.Fatalf("falsePos chunk mismatch")
	}
	player2 := engine2.sessions[session].player
	player2.state.Position = mgl32.Vec3{float32(target2.X) + 0.5, 1, float32(target2.Z) + 3.5}
	player2.yaw = 0
	player2.pitch = miningTestPitch
	engine2.SetBlockForTest(target2, core.PotatoStage7ID)
	tickBefore2 := engine2.tick.Load()
	wantPoison2 := poisonRoll(seed2, tickBefore2, core.Overworld, target2)
	result2 := advanceMiningOnce(engine2)
	if len(result2.Rejected) != 0 {
		t.Fatalf("potato mature at falsePos rejected=%+v", result2.Rejected)
	}
	drops2 := miningDropTotals(miningTargetRecord(t, engine2, target2).Chunk)
	if wantPoison2 && drops2[core.ItemPoisonousPotato] != 1 {
		t.Fatalf("expected poisonous at falsePos but got %v poison=%v", drops2, wantPoison2)
	}
	if !wantPoison2 && drops2[core.ItemPoisonousPotato] != 0 {
		t.Fatalf("unexpected poisonous at falsePos %v", drops2)
	}
}

func TestHarvestPotatoMatureCapacityIsAtomic(t *testing.T) {
	engine, _, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	engine.SetBlockForTest(target, core.PotatoStage7ID)
	fillMiningDrops(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeHash := record.Chunk.Hash()
	beforeDrops := record.Chunk.DropsHash()
	beforeRevision := record.Revision
	result := advanceMiningOnce(engine)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectDropCapacity {
		t.Fatalf("potato mature capacity reject=%+v want DropCapacity", result.Rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash || record.Revision != beforeRevision {
		t.Fatalf("potato capacity failure changed block hash=%x/%x rev=%d/%d", got, beforeHash, record.Revision, beforeRevision)
	}
	if got := record.Chunk.DropsHash(); got != beforeDrops {
		t.Fatalf("potato capacity failure changed drops %x vs %x", got, beforeDrops)
	}
	x, _, z := target.Local()
	if got := record.Chunk.BlockAt(x, target.Y, z); got != core.PotatoStage7ID {
		t.Fatalf("potato capacity failure removed block %d", got)
	}
}

func TestHarvestCarrotMatureCapacityIsAtomic(t *testing.T) {
	engine, _, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	engine.SetBlockForTest(target, core.CarrotStage7ID)
	fillMiningDrops(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeHash := record.Chunk.Hash()
	beforeRevision := record.Revision
	result := advanceMiningOnce(engine)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectDropCapacity {
		t.Fatalf("carrot mature capacity reject=%+v", result.Rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash || record.Revision != beforeRevision {
		t.Fatalf("carrot capacity changed hash")
	}
}

func TestHarvestPotatoUnripeCapacityIsAtomic(t *testing.T) {
	engine, _, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	engine.SetBlockForTest(target, core.PotatoStage2ID)
	fillMiningDrops(engine, target)
	record := miningTargetRecord(t, engine, target)
	beforeHash := record.Chunk.Hash()
	result := advanceMiningOnce(engine)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectDropCapacity {
		t.Fatalf("potato unripe capacity reject=%+v", result.Rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash {
		t.Fatalf("unripe capacity changed hash")
	}
}
