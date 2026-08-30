package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// bed_test.go：床在 sim 层的行为——同区块原子放置（床尾 + 朝向侧床头）、
// 整单拒绝矩阵、采掘任一半双清恰好掉 1、支撑失效整床清除，以及放置与采掘
// 分派的接入。床的方向编码沿用门先例（南 0、西 1、北 2、东 3），坐标约定
// 与 `core.BedHeadNeighbor` 一致（南 +Z、西 −X、北 −Z、东 +X）。

// hotbarWithBed 返回栏位 0 装有 count 个床物品的快捷栏（与 hotbarWithDoor 同形）。
func hotbarWithBed(count uint8) core.Hotbar {
	var h core.Hotbar
	if count > 0 {
		h.Slots[0] = core.ItemStack{Item: core.ItemBed, Count: count}
	}
	return h
}

// bedPlaceScenario 把 foot 及其朝向侧床头格清为空气、下方支撑换成石头，
// 返回床头格坐标。doorTestReadyEngine 的地面上方除 (0,2,5) 一块石头外全为
// 空气，用例统一避开该列。
func bedPlaceScenario(t *testing.T, engine *Engine, foot core.BlockPos, dir int) core.BlockPos {
	t.Helper()
	head := core.BedHeadNeighbor(foot, dir)
	engine.SetBlockForTest(foot, core.AirID)
	engine.SetBlockForTest(head, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: foot.X, Y: foot.Y - 1, Z: foot.Z}, core.StoneID)
	engine.SetBlockForTest(core.BlockPos{X: head.X, Y: head.Y - 1, Z: head.Z}, core.StoneID)
	return head
}

// TestTryPlaceBedWritesBothHalvesPerDirection 覆盖 spec 场景「正常放置写入
// 两格」：四向放置都把床尾写入 foot、床头写入其朝向侧邻格，两格变化合并进
// 同一份 pending（同区块原子双写，同一 key 合并）。
func TestTryPlaceBedWritesBothHalvesPerDirection(t *testing.T) {
	for dir := 0; dir < 4; dir++ {
		engine, _, _ := doorTestReadyEngine(t, hotbarWithBed(1))
		foot := core.BlockPos{X: 3, Y: 1, Z: 4}
		head := bedPlaceScenario(t, engine, foot, dir)
		pending := engine.newMutation()
		reason, rejected := engine.tryPlaceBed(core.Overworld, foot, dir, pending)
		if rejected {
			t.Fatalf("dir %d 放置被拒绝 reason %d", dir, reason)
		}
		if got, _ := engine.dimension(core.Overworld).BlockAt(foot); got != core.BedFootID(dir) {
			t.Fatalf("dir %d foot=%d want %d", dir, got, core.BedFootID(dir))
		}
		if got, _ := engine.dimension(core.Overworld).BlockAt(head); got != core.BedHeadID(dir) {
			t.Fatalf("dir %d head=%d want %d", dir, got, core.BedHeadID(dir))
		}
		engine.finishChanges(pending, &TickResult{})
	}
}

// TestTryPlaceBedRejectionsWithoutWrites 覆盖 spec 场景「床头格被占据时拒绝」
// 与支撑边界：任一条件不满足都必须整单拒绝——两格保持空气、不产生任何待发布
// 变更；流体格与门先例一致视为占据。
func TestTryPlaceBedRejectionsWithoutWrites(t *testing.T) {
	cases := []struct {
		name  string
		mutdb func(t *testing.T, engine *Engine, foot, head core.BlockPos)
		dir   int
	}{
		{
			name: "床头格被占据",
			mutdb: func(t *testing.T, engine *Engine, foot, head core.BlockPos) {
				engine.SetBlockForTest(head, core.StoneID)
			},
			dir: 0,
		},
		{
			name: "床尾格被占据",
			mutdb: func(t *testing.T, engine *Engine, foot, head core.BlockPos) {
				engine.SetBlockForTest(foot, core.StoneID)
			},
			dir: 0,
		},
		{
			name: "床头下方非实心",
			mutdb: func(t *testing.T, engine *Engine, foot, head core.BlockPos) {
				engine.SetBlockForTest(core.BlockPos{X: head.X, Y: head.Y - 1, Z: head.Z}, core.AirID)
			},
			dir: 0,
		},
		{
			name: "床尾下方非实心",
			mutdb: func(t *testing.T, engine *Engine, foot, head core.BlockPos) {
				engine.SetBlockForTest(core.BlockPos{X: foot.X, Y: foot.Y - 1, Z: foot.Z}, core.AirID)
			},
			dir: 0,
		},
		{
			name: "床头格为流体视作占据",
			mutdb: func(t *testing.T, engine *Engine, foot, head core.BlockPos) {
				engine.SetBlockForTest(head, core.WaterSourceID)
			},
			dir: 0,
		},
		{
			name: "床尾格为流体视作占据",
			mutdb: func(t *testing.T, engine *Engine, foot, head core.BlockPos) {
				engine.SetBlockForTest(foot, core.WaterLevel1ID)
			},
			dir: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, _, _ := doorTestReadyEngine(t, hotbarWithBed(1))
			foot := core.BlockPos{X: 3, Y: 1, Z: 4}
			head := bedPlaceScenario(t, engine, foot, tc.dir)
			tc.mutdb(t, engine, foot, head)
			pending := engine.newMutation()
			_, rejected := engine.tryPlaceBed(core.Overworld, foot, tc.dir, pending)
			if !rejected {
				t.Fatalf("%s 应整单拒绝", tc.name)
			}
			if got, _ := engine.dimension(core.Overworld).BlockAt(foot); got == core.BedFootID(tc.dir) {
				t.Fatalf("%s 床尾被写入 %d", tc.name, got)
			}
			if got, _ := engine.dimension(core.Overworld).BlockAt(head); got == core.BedHeadID(tc.dir) {
				t.Fatalf("%s 床头被写入 %d", tc.name, got)
			}
			if pending.Len() != 0 {
				t.Fatalf("%s 拒绝产生了待发布变更 %+v", tc.name, pending)
			}
		})
	}
	for _, dir := range []int{-1, 4} {
		engine, _, _ := doorTestReadyEngine(t, hotbarWithBed(1))
		pending := engine.newMutation()
		if _, rejected := engine.tryPlaceBed(core.Overworld, core.BlockPos{X: 3, Y: 1, Z: 4}, dir, pending); !rejected {
			t.Fatalf("dir %d 应拒绝", dir)
		}
		if pending.Len() != 0 {
			t.Fatalf("dir %d 拒绝产生了待发布变更", dir)
		}
	}
}

// TestTryPlaceBedCrossChunkRejected 覆盖 spec 场景「跨区块或支撑不足整单
// 拒绝」：床头格落在未加载区块时整单拒绝，两格零写入、不产生待发布变更
// （不消耗由放置分派的不扣料路径保证，见 TestBedPlacementViaCommand）。
func TestTryPlaceBedCrossChunkRejected(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, hotbarWithBed(1))
	// 床尾 (15,1,4) 在区块 (0,0)，朝东的床头 (16,1,4) 落在未加载的 (1,0)。
	foot := core.BlockPos{X: 15, Y: 1, Z: 4}
	engine.SetBlockForTest(foot, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: 15, Y: 0, Z: 4}, core.StoneID)
	pending := engine.newMutation()
	_, rejected := engine.tryPlaceBed(core.Overworld, foot, 3, pending)
	if !rejected {
		t.Fatal("床头格跨区块未就绪应整单拒绝")
	}
	if got, _ := engine.dimension(core.Overworld).BlockAt(foot); got != core.AirID {
		t.Fatalf("床尾格被写入 %d", got)
	}
	if pending.Len() != 0 {
		t.Fatalf("拒绝产生了待发布变更 %+v", pending)
	}
}

// TestBedPlacementViaCommand 经真实放置命令覆盖分派接入与消耗语义：放置成功
// 时同 tick 原子完成两格写入、恰好扣 1 个床物品并发布既有变更广播；床头格
// 跨区块未就绪时整单拒绝且不扣物品。方向派生与门共享 `yawToDoorDir`：
// yaw=0（视线 −Z）派生方向 0，床头落在床尾 +Z 邻格。
func TestBedPlacementViaCommand(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{6.5, 1, 9.5}
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemBed, Count: 8}
	player.inventory.Hotbar.Selected = 0
	eye := torchEye(engine, session)
	// 瞄准草地顶面，落点为床尾 (6,1,6)；yaw=0 派生南向，床头落在 (6,1,7)。
	yaw, pitch := lookAtPoint(eye, mgl32.Vec3{6.5, 1, 6.5})
	result := settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlaceBlock,
		Slot: 0, Yaw: yaw, Pitch: pitch,
	}})
	if len(result.Rejected) != 0 {
		t.Fatalf("合法床放置被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, core.BlockPos{X: 6, Y: 1, Z: 6}); got != core.BedFootSouthID {
		t.Fatalf("床尾 = %d，想要 %d", got, core.BedFootSouthID)
	}
	if got := tillBlockAt(t, engine, core.BlockPos{X: 6, Y: 1, Z: 7}); got != core.BedHeadSouthID {
		t.Fatalf("床头 = %d，想要 %d", got, core.BedHeadSouthID)
	}
	if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemBed, Count: 7}) {
		t.Fatalf("放置后栏位 = %+v，想要恰好扣 1 个床物品", got)
	}
	if len(result.PlacementSuccesses) != 1 {
		t.Fatalf("放置成功发布 = %+v", result.PlacementSuccesses)
	}
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 2 {
		t.Fatalf("两格变更应合并为同区块同一批: %+v", result.Changes)
	}
}

// TestBedPlacementViaCommandCrossChunkKeepsItem 锁定「跨区块未就绪整单拒绝
// 不消耗」走完整命令路径的观察：床头格在未加载区块时拒绝，物品不被扣除。
func TestBedPlacementViaCommandCrossChunkKeepsItem(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{12.5, 1, 4.5}
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemBed, Count: 8}
	player.inventory.Hotbar.Selected = 0
	eye := torchEye(engine, session)
	// 瞄准 (15,0,4) 顶面，落点床尾 (15,1,4)；朝 +X 的 yaw 派生东向，床头
	// (16,1,4) 落在未加载区块。
	yaw, pitch := lookAtPoint(eye, mgl32.Vec3{15.5, 1, 4.5})
	result := settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlaceBlock,
		Slot: 0, Yaw: yaw, Pitch: pitch,
	}})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectChunkNotReady {
		t.Fatalf("跨区块放置应拒绝 ChunkNotReady: %+v", result.Rejected)
	}
	if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemBed, Count: 8}) {
		t.Fatalf("拒绝后栏位 = %+v，想要不消耗", got)
	}
	if got := tillBlockAt(t, engine, core.BlockPos{X: 15, Y: 1, Z: 4}); got != core.AirID {
		t.Fatalf("床尾格被写入 %d", got)
	}
}

// —— 采掘：任一半双清两格、恰好掉落 1 个床物品 ——

// TestBedMiningRuleMatchesPlanksPrice 锁定采掘规则：床与木板同价 15 tick、
// 任意手持（含徒手与工具）都可采掘且产物可收获。
func TestBedMiningRuleMatchesPlanksPrice(t *testing.T) {
	beds := []core.BlockID{core.BedFootSouthID, core.BedHeadEastID}
	held := []core.ItemID{core.ItemNone, core.ItemIronPickaxe, core.ItemOakPlanks}
	for _, bed := range beds {
		for _, tool := range held {
			required, harvestable := miningRule(bed, tool)
			if required != 15 || !harvestable {
				t.Fatalf("miningRule(bed %d, held %d) = (%d, %v)，想要 (15, true)", bed, tool, required, harvestable)
			}
		}
	}
}

// TestCompleteMiningBedClearsBothHalvesAndDropsOne 覆盖 spec 场景「采掘床头
// 两格全清」：命中任一半都原子双清，DoDrop=true 时恰好掉落 1 个床物品，
// DoDrop=false 时仍双清但零掉落（门采掘先例同构）。
func TestCompleteMiningBedClearsBothHalvesAndDropsOne(t *testing.T) {
	foot := core.BlockPos{X: 2, Y: 1, Z: 5}
	head := core.BlockPos{X: 2, Y: 1, Z: 6}
	for _, hitHead := range []bool{false, true} {
		for _, harvestable := range []bool{true, false} {
			engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
			engine.SetBlockForTest(foot, core.BedFootSouthID)
			engine.SetBlockForTest(head, core.BedHeadSouthID)
			target := foot
			if hitHead {
				target = head
			}
			pending := engine.newMutation()
			block, _ := engine.dimension(core.Overworld).BlockAt(target)
			reason, rejected := engine.completeMining(core.Overworld, target, block, harvestable, pending)
			if rejected {
				t.Fatalf("hitHead %v harvestable %v completeMining rejected %d", hitHead, harvestable, reason)
			}
			engine.finishChanges(pending, &TickResult{})
			for _, pos := range []core.BlockPos{foot, head} {
				if got, _ := engine.dimension(core.Overworld).BlockAt(pos); got != core.AirID {
					t.Fatalf("hitHead %v harvestable %v: %+v 未清空 = %d", hitHead, harvestable, pos, got)
				}
			}
			chunk, ready := engine.dimension(core.Overworld).ReadyChunk(foot.Chunk())
			if !ready {
				t.Fatal("bed chunk is not ready")
			}
			wantCount := uint8(0)
			if harvestable {
				wantCount = 1
			}
			if found := miningDropTotals(chunk)[core.ItemBed]; found != wantCount {
				t.Fatalf("hitHead %v harvestable %v: 掉落 ItemBed=%d want %d", hitHead, harvestable, found, wantCount)
			}
		}
	}
}

// TestCompleteMiningBedDropCapacityAtomic 锁定掉落槽容量不足时的原子性：
// 整单拒绝、区块零写入零推进、无待发布变更、床保持原状（门先例同一取舍）。
func TestCompleteMiningBedDropCapacityAtomic(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	foot := core.BlockPos{X: 2, Y: 1, Z: 5}
	head := core.BlockPos{X: 2, Y: 1, Z: 6}
	engine.SetBlockForTest(foot, core.BedFootSouthID)
	engine.SetBlockForTest(head, core.BedHeadSouthID)
	fillMiningDrops(engine, foot)
	record := miningTargetRecord(t, engine, foot)
	beforeHash := record.Chunk.Hash()
	beforeRevision := record.Revision
	pending := engine.newMutation()
	block, _ := engine.dimension(core.Overworld).BlockAt(foot)
	reason, rejected := engine.completeMining(core.Overworld, foot, block, true, pending)
	if !rejected || reason != RejectDropCapacity {
		t.Fatalf("掉落满仓应拒绝 DropCapacity got %d rejected %v", reason, rejected)
	}
	if got := record.Chunk.Hash(); got != beforeHash {
		t.Fatal("容量失败不应修改区块")
	}
	if record.Revision != beforeRevision {
		t.Fatal("容量失败不应推进 revision")
	}
	if pending.Len() != 0 {
		t.Fatalf("容量失败不应产生待发布变更 %+v", pending)
	}
	if got, _ := engine.dimension(core.Overworld).BlockAt(foot); got != core.BedFootSouthID {
		t.Fatalf("容量失败床尾被改 %d", got)
	}
	if got, _ := engine.dimension(core.Overworld).BlockAt(head); got != core.BedHeadSouthID {
		t.Fatalf("容量失败床头被改 %d", got)
	}
}

// TestBedMiningViaPlayerPath 走真实采掘命令路径（advanceMining 分派）：徒手
// 采掘床头 15 tick 完成后双清两格并恰好掉落 1 个床物品。
func TestBedMiningViaPlayerPath(t *testing.T) {
	engine, session, _ := doorTestReadyEngine(t, core.Hotbar{})
	foot := core.BlockPos{X: 2, Y: 1, Z: 5}
	head := core.BlockPos{X: 2, Y: 1, Z: 6}
	engine.SetBlockForTest(foot, core.BedFootSouthID)
	engine.SetBlockForTest(head, core.BedHeadSouthID)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{2.5, 1, 9.5}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	// 采掘靠玩家一侧的床头格；床尾在更远一侧，同 tick 一并清空。
	yaw, pitch := lookAtBlockCenter(eye, head)
	player.yaw = yaw
	player.pitch = pitch
	player.miningHeld = true
	var result TickResult
	for range 15 {
		result = finishWorldTick(engine)
	}
	if len(result.Rejected) != 0 {
		t.Fatalf("采掘被拒绝: %+v", result.Rejected)
	}
	for _, pos := range []core.BlockPos{foot, head} {
		if got := tillBlockAt(t, engine, pos); got != core.AirID {
			t.Fatalf("%+v = %d，想要空气", pos, got)
		}
	}
	counts := map[core.ItemID]uint8{}
	for _, drop := range torchChunkDrops(t, engine) {
		counts[drop.Stack.Item] += drop.Stack.Count
	}
	if counts[core.ItemBed] != 1 {
		t.Fatalf("掉落物 = %+v，想要恰好 1 个床物品", counts)
	}
}

// —— 支撑失效：整床清除并掉落 ——

// TestBedSupportFailureClearsWholeBedAndDrops 覆盖 spec「支撑失效导致的清除
// SHALL 同样整床移除并掉落」：采掘床头下方支撑格的完成 tick 内，支撑格与
// 床双格在同一批变更中一起变空气，并恰好新增 1 个床物品掉落。
//
// 场景把床抬高一格放在泥土支柱上：完整地面上床的支撑格只能从下方（侧面）
// 够到——俯视射线会被床自身格挡、浅平射线会被地面截停，恰好复现游戏内
// 「挖床脚须从侧面挖」的几何。泥土徒手 5 tick。
func TestBedSupportFailureClearsWholeBedAndDrops(t *testing.T) {
	engine, session, _ := doorTestReadyEngine(t, core.Hotbar{})
	foot := core.BlockPos{X: 4, Y: 2, Z: 5}
	head := core.BlockPos{X: 4, Y: 2, Z: 6}
	footSupport := core.BlockPos{X: 4, Y: 1, Z: 5}
	headSupport := core.BlockPos{X: 4, Y: 1, Z: 6}
	engine.SetBlockForTest(footSupport, core.DirtID)
	engine.SetBlockForTest(headSupport, core.DirtID)
	engine.SetBlockForTest(foot, core.BedFootSouthID)
	engine.SetBlockForTest(head, core.BedHeadSouthID)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{4.5, 1, 9.5}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	// 自 +Z 侧平视瞄向床头支撑格的侧面中心：射线在命中支撑格前不会穿过
	// 床双格（床在支撑格正上方一格）。
	yaw, pitch := lookAtPoint(eye, mgl32.Vec3{4.5, 1.5, 7})
	player.yaw = yaw
	player.pitch = pitch
	player.miningHeld = true
	var result TickResult
	for range 4 {
		finishWorldTick(engine)
	}
	tick := engine.beginTick()
	tick.context.FinishWorld(&tick.result)
	engine.realm.SweepUnsupportedBeds(tick.mutation)
	commitMutation(tick.mutation, &tick.result)
	result = publishFixture(engine, &tick)
	if len(result.Rejected) != 0 {
		t.Fatalf("采掘被拒绝: %+v", result.Rejected)
	}
	for _, pos := range []core.BlockPos{headSupport, foot, head} {
		if got := tillBlockAt(t, engine, pos); got != core.AirID {
			t.Fatalf("%+v = %d，想要空气", pos, got)
		}
	}
	if got := tillBlockAt(t, engine, footSupport); got != core.DirtID {
		t.Fatalf("床尾支撑格不应受影响 = %d", got)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("完成 tick 应只有一个变更批次: %+v", result.Changes)
	}
	batch := result.Changes[0]
	if batch.NewRevision != batch.BaseRevision+1 {
		t.Fatalf("revision = %d→%d，想要同批恰好一次推进", batch.BaseRevision, batch.NewRevision)
	}
	changed := map[core.BlockPos]core.BlockID{}
	for _, change := range batch.Changes {
		changed[change.Position] = change.Block
	}
	want := map[core.BlockPos]core.BlockID{
		headSupport: core.AirID,
		foot:        core.AirID,
		head:        core.AirID,
	}
	if len(changed) != 3 {
		t.Fatalf("变更 = %+v，想要 %+v", changed, want)
	}
	for pos, block := range want {
		if got, ok := changed[pos]; !ok || got != block {
			t.Fatalf("变更 = %+v，缺少 %+v→%d", changed, pos, block)
		}
	}
	counts := map[core.ItemID]uint8{}
	for _, drop := range torchChunkDrops(t, engine) {
		counts[drop.Stack.Item] += drop.Stack.Count
	}
	if counts[core.ItemBed] != 1 || counts[core.ItemDirt] != 1 {
		t.Fatalf("掉落物 = %+v，想要 1 泥土（采掘产物）+ 1 床（支撑失效产物）", counts)
	}
}

// TestCompanionMiningBedClearsBothHalves 锁定伙伴采掘床的双格语义：与玩家
// 同价采掘任一半，两格一起清空，产物 1 个床物品直入伙伴背包（伙伴与玩家的
// 差别只在产物去向），世界中不得残留半床。
func TestCompanionMiningBedClearsBothHalves(t *testing.T) {
	fixture := readyCompanionMining(t, core.BedFootEastID, core.ItemNone)
	// 东向床：床头在床尾 +X 邻格，不在伙伴射线路径上。
	head := core.BlockPos{X: fixture.target.X + 1, Y: fixture.target.Y, Z: fixture.target.Z}
	fixture.engine.SetBlockForTest(head, core.BedHeadEastID)
	var result TickResult
	for range 15 {
		result = holdCompanionMineAction(t, fixture)
	}
	if len(result.Rejected) != 0 {
		t.Fatalf("伙伴采掘被拒绝: %+v", result.Rejected)
	}
	if got := companionMiningBlockAt(t, fixture); got != core.AirID {
		t.Fatalf("床尾 = %d，想要空气", got)
	}
	if got := tillBlockAt(t, fixture.engine, head); got != core.AirID {
		t.Fatalf("床头 = %d，想要空气", got)
	}
	if got := companionItemCount(fixture.entry, core.ItemBed); got != 1 {
		t.Fatalf("伙伴背包床物品 = %d，想要恰好 1", got)
	}
}
