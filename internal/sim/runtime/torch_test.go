package runtime

import (
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/world"
)

// torch_test.go：可放置火把在 sim 层的行为——五向放置与拒绝、支撑失效的
// 六邻居有界复核与掉落、火把配方的合成验收。全部经真实命令与 Step 推进，
// 不直接调用放置内部函数。

// —— 火把用例共享的夹具与观察助手 ——

// torchLookDown 是近似垂直向下的俯视角（与既有交互测试同形，避开
// validPlayerLook 的 ±(π/2 − 0.01) 边界）。
const torchLookDown = -float32(math.Pi)/2 + 0.01

// readyTorchPlayer 构造一个快捷栏栏位 0 握着 8 个火把、栏位 1 握着 8 个泥土
// 的已激活玩家；两个栏位供「先放火把再放参照方块」的用例复用。
func readyTorchPlayer(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemTorch, Count: 8}
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 8}
	player.inventory.Hotbar.Selected = 0
	return engine, session
}

// torchEye 返回玩家眼睛的世界坐标。
func torchEye(engine *Engine, session SessionID) mgl32.Vec3 {
	player := engine.sessions[session].player
	return player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
}

// torchTopFaceCenter 返回方块顶面中心的世界坐标（顶面命中的瞄准点）。
func torchTopFaceCenter(target core.BlockPos) mgl32.Vec3 {
	return mgl32.Vec3{
		float32(target.X) + 0.5,
		float32(target.Y) + 1,
		float32(target.Z) + 0.5,
	}
}

// torchPlace 从栏位 slot 发起一次放置命令并推进一个权威 tick。
func torchPlace(
	engine *Engine,
	session SessionID,
	sequence uint64,
	slot uint8,
	yaw, pitch float32,
) TickResult {
	engine.Enqueue(Command{
		Session:  session,
		Sequence: sequence,
		Kind:     CommandPlaceBlock,
		Slot:     slot,
		Yaw:      yaw,
		Pitch:    pitch,
	})
	return engine.Step()
}

// torchChunkDrops 收集中心区块的全部活动掉落物。
func torchChunkDrops(t *testing.T, engine *Engine) []world.DropSlot {
	t.Helper()
	chunk, _, ok := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})
	if !ok {
		t.Fatal("中心区块不可用")
	}
	var drops []world.DropSlot
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active {
			drops = append(drops, drop)
		}
	}
	return drops
}

// —— 五向放置与拒绝 ——

// TestTorchPlacementFormsPerFace 覆盖 spec 场景「落地形态」「四向墙面形态」：
// 顶面命中得到落地形态、四个水平侧面得到形态名与命中面同名的墙面形态，
// 同一权威 tick 内原子写方块、扣一格物品并经既有变更路径广播。
func TestTorchPlacementFormsPerFace(t *testing.T) {
	tests := []struct {
		name       string
		support    core.BlockPos
		aim        mgl32.Vec3
		wantForm   core.BlockID
		wantTarget core.BlockPos
	}{
		{
			name:       "顶面命中得到落地形态",
			support:    core.BlockPos{X: 8, Y: 1, Z: 10},
			aim:        mgl32.Vec3{8.5, 2, 10.5},
			wantForm:   core.TorchStandingID,
			wantTarget: core.BlockPos{X: 8, Y: 2, Z: 10},
		},
		{
			name:       "NegZ 侧面得到墙 -Z 形态",
			support:    core.BlockPos{X: 8, Y: 2, Z: 11},
			aim:        mgl32.Vec3{8.5, 2.5, 11.5},
			wantForm:   core.TorchWallNegZID,
			wantTarget: core.BlockPos{X: 8, Y: 2, Z: 10},
		},
		{
			name:       "PosZ 侧面得到墙 +Z 形态",
			support:    core.BlockPos{X: 8, Y: 2, Z: 5},
			aim:        mgl32.Vec3{8.5, 2.5, 5.5},
			wantForm:   core.TorchWallPosZID,
			wantTarget: core.BlockPos{X: 8, Y: 2, Z: 6},
		},
		{
			name:       "PosX 侧面得到墙 +X 形态",
			support:    core.BlockPos{X: 5, Y: 2, Z: 8},
			aim:        mgl32.Vec3{5.5, 2.5, 8.5},
			wantForm:   core.TorchWallPosXID,
			wantTarget: core.BlockPos{X: 6, Y: 2, Z: 8},
		},
		{
			name:       "NegX 侧面得到墙 -X 形态",
			support:    core.BlockPos{X: 11, Y: 2, Z: 8},
			aim:        mgl32.Vec3{11.5, 2.5, 8.5},
			wantForm:   core.TorchWallNegXID,
			wantTarget: core.BlockPos{X: 10, Y: 2, Z: 8},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := readyTorchPlayer(t)
			// 支撑块全部摆在区块中央，四个水平方向的射线都不出已加载区块。
			player := engine.sessions[session].player
			player.state.Position = mgl32.Vec3{8.5, 1, 8.5}
			engine.SetBlockForTest(tc.support, core.StoneID)
			yaw, pitch := lookAtPoint(torchEye(engine, session), tc.aim)

			result := torchPlace(engine, session, 2, 0, yaw, pitch)

			if len(result.Rejected) != 0 {
				t.Fatalf("合法火把放置被拒绝: %+v", result.Rejected)
			}
			if got := tillBlockAt(t, engine, tc.wantTarget); got != tc.wantForm {
				t.Fatalf("放置结果 = %d，想要形态 %d", got, tc.wantForm)
			}
			if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemTorch, Count: 7}) {
				t.Fatalf("放置后栏位 = %+v，想要恰好扣一格火把", got)
			}
			if len(result.PlacementSuccesses) != 1 {
				t.Fatalf("放置成功发布 = %+v", result.PlacementSuccesses)
			}
			if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
				result.Changes[0].Changes[0] != (BlockChange{Position: tc.wantTarget, Block: tc.wantForm}) {
				t.Fatalf("放置没有走既有变更广播: %+v", result.Changes)
			}
		})
	}
}

// TestTorchPlacementRejectsWithoutConsumption 覆盖 spec 场景「底面与支撑不合法拒绝」
// 「未加载目标拒绝」与「玩家占位拒绝」：每条路径都拒绝、不写方块、不扣火把。
func TestTorchPlacementRejectsWithoutConsumption(t *testing.T) {
	tests := []struct {
		name       string
		arrange    func(t *testing.T, engine *Engine, session SessionID)
		aim        func(t *testing.T, engine *Engine, session SessionID) (yaw, pitch float32)
		wantReason RejectReason
	}{
		{
			name: "底面命中拒绝",
			arrange: func(t *testing.T, engine *Engine, session SessionID) {
				engine.SetBlockForTest(core.BlockPos{X: 0, Y: 4, Z: 3}, core.StoneID)
			},
			aim: func(t *testing.T, engine *Engine, session SessionID) (float32, float32) {
				// 从眼睛瞄准头顶方块的底面中心：射线在 y=4 平面进入该格的
				// 列内，命中面是底面。
				return lookAtPoint(torchEye(engine, session), mgl32.Vec3{0.5, 4, 3.5})
			},
			wantReason: RejectInvalidBlock,
		},
		{
			name: "非实心支撑拒绝（作物）",
			arrange: func(t *testing.T, engine *Engine, session SessionID) {
				engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: 3}, core.WheatStage3ID)
			},
			aim: func(t *testing.T, engine *Engine, session SessionID) (float32, float32) {
				return lookAtBlockCenter(torchEye(engine, session), core.BlockPos{X: 0, Y: 1, Z: 3})
			},
			wantReason: RejectInvalidBlock,
		},
		{
			name: "目标格为流体拒绝",
			arrange: func(t *testing.T, engine *Engine, session SessionID) {
				engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 4}, core.StoneID)
				engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.WaterSourceID)
			},
			aim: func(t *testing.T, engine *Engine, session SessionID) (float32, float32) {
				return lookAtBlockCenter(torchEye(engine, session), core.BlockPos{X: 0, Y: 2, Z: 4})
			},
			wantReason: RejectInvalidBlock,
		},
		{
			name: "目标格玩家占位拒绝",
			arrange: func(t *testing.T, engine *Engine, session SessionID) {
			},
			aim: func(t *testing.T, engine *Engine, session SessionID) (float32, float32) {
				return 0, torchLookDown
			},
			wantReason: RejectOccupied,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := readyTorchPlayer(t)
			player := engine.sessions[session].player
			tc.arrange(t, engine, session)
			yaw, pitch := tc.aim(t, engine, session)

			result := torchPlace(engine, session, 2, 0, yaw, pitch)

			if len(result.Rejected) != 1 || result.Rejected[0].Reason != tc.wantReason {
				t.Fatalf("Rejected = %+v，想要 reason %v", result.Rejected, tc.wantReason)
			}
			if len(result.Changes) != 0 || len(result.PlacementSuccesses) != 0 {
				t.Fatalf("拒绝路径产生了结果: Changes=%+v Successes=%+v",
					result.Changes, result.PlacementSuccesses)
			}
			if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemTorch, Count: 8}) {
				t.Fatalf("拒绝路径扣了火把: %+v", got)
			}
		})
	}

	t.Run("未加载目标拒绝", func(t *testing.T) {
		engine, session := readyTorchPlayer(t)
		player := engine.sessions[session].player
		// 把玩家挪到区块 +X 边缘：射线朝 +X 越过边界进入未加载区块。
		player.state.Position = mgl32.Vec3{15.5, 1, 0.5}
		yaw := -float32(math.Pi) / 2

		result := torchPlace(engine, session, 2, 0, yaw, 0)

		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectChunkNotReady {
			t.Fatalf("Rejected = %+v，想要 RejectChunkNotReady", result.Rejected)
		}
		if len(result.Changes) != 0 {
			t.Fatalf("未加载目标产生了修改: %+v", result.Changes)
		}
		if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemTorch, Count: 8}) {
			t.Fatalf("未加载目标扣了火把: %+v", got)
		}
	})
}

// —— 支撑失效的六邻居有界复核 ——
// 当前方块集里唯一能把实心支撑变非实心的写者是采掘，下面用例都用采掘触发；
// 复核机制本身写者无关——挂点在 recordChange 汇聚的 pending 变更上，流体、
// 作物与放置写者一经出现即被同一批次覆盖。

// TestMiningSupportRemovesStandingTorchAndDropsSameTick 覆盖 spec 场景
// 「采掘支撑格掉落依附火把」：玩家采掘支撑格的完成 tick 内，火把同批变空气、
// 生成一枚火把掉落，并与支撑格变更共享同一批 revision 与广播。
func TestMiningSupportRemovesStandingTorchAndDropsSameTick(t *testing.T) {
	engine, session := readyTorchPlayer(t)
	support := core.BlockPos{X: 0, Y: 1, Z: 4}
	torchPos := core.BlockPos{X: 0, Y: 2, Z: 4}
	engine.SetBlockForTest(support, core.DirtID)
	eye := torchEye(engine, session)

	// 先经真实命令把落地火把放到支撑格正上方。
	placeYaw, placePitch := lookAtPoint(eye, torchTopFaceCenter(support))
	placed := torchPlace(engine, session, 2, 0, placeYaw, placePitch)
	if len(placed.Rejected) != 0 || tillBlockAt(t, engine, torchPos) != core.TorchStandingID {
		t.Fatalf("火把没有放到支撑格上方: Rejected=%+v block=%d",
			placed.Rejected, tillBlockAt(t, engine, torchPos))
	}

	// 裸手挖泥土 5 tick：第 5 个 tick 完成采掘，火把必须在同一 tick 被移除。
	mineYaw, minePitch := lookAtBlockCenter(eye, support)
	engine.Enqueue(Command{
		Session: session, Sequence: 3, Kind: CommandPlayerInput,
		Yaw: mineYaw, Pitch: minePitch, Mining: true,
	})
	var result TickResult
	for range 5 {
		result = engine.Step()
	}
	if len(result.Rejected) != 0 {
		t.Fatalf("采掘被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, support); got != core.AirID {
		t.Fatalf("支撑格 = %d，想要空气", got)
	}
	if got := tillBlockAt(t, engine, torchPos); got != core.AirID {
		t.Fatalf("失去支撑的火把 = %d，想要空气", got)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("完成 tick 应只有一个变更批次: %+v", result.Changes)
	}
	batch := result.Changes[0]
	if batch.NewRevision != batch.BaseRevision+1 {
		t.Fatalf("revision = %d→%d，想要同批恰好一次推进",
			batch.BaseRevision, batch.NewRevision)
	}
	got := map[core.BlockPos]core.BlockID{}
	for _, change := range batch.Changes {
		got[change.Position] = change.Block
	}
	want := map[core.BlockPos]core.BlockID{support: core.AirID, torchPos: core.AirID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("完成 tick 变更 = %+v，想要 %+v", got, want)
	}

	// 掉落：泥土一枚（采掘产物）+ 火把一枚（支撑失效产物），火把落在火把格。
	drops := torchChunkDrops(t, engine)
	if len(drops) != 2 {
		t.Fatalf("活动掉落物 = %d 个，想要 2 个: %+v", len(drops), drops)
	}
	counts := map[core.ItemID]uint8{}
	for _, drop := range drops {
		counts[drop.Stack.Item] += drop.Stack.Count
	}
	if counts[core.ItemDirt] != 1 || counts[core.ItemTorch] != 1 {
		t.Fatalf("掉落物数量 = %+v，想要 1 泥土 + 1 火把", counts)
	}
	for _, drop := range drops {
		if drop.Stack.Item != core.ItemTorch {
			continue
		}
		position, ok := world.BlockPosFromChunkIndex(core.ChunkPos{}, drop.BlockIndex)
		if !ok || position != torchPos {
			t.Fatalf("火把掉落位置 = %+v，想要火把原格", position)
		}
		if drop.PickupDelayTicks != tuning.DefaultTunables().DropPickupDelayTicks {
			t.Fatalf("火把掉落拾取延迟 = %d", drop.PickupDelayTicks)
		}
	}
}

// TestCompanionMiningSupportRemovesWallTorchesAndDrops 覆盖 spec 场景
// 「伙伴采掘同样生效」：支撑格被伙伴以权威 mine 路径移除时，同一有界批次内
// 落地与墙面两枚依赖火把都变空气并各掉落一枚火把。
func TestCompanionMiningSupportRemovesWallTorchesAndDrops(t *testing.T) {
	fixture := readyCompanionMining(t, core.DirtID, core.ItemNone)
	// 支撑格 (4,1,5) 的正上方放落地火把、+X 侧放墙 +X 形态，两枚火把都只依赖
	// 该支撑格；伙伴射线自 +Z 侧射向支撑格中心，不穿过任何一个火把格。
	standing := core.BlockPos{X: 4, Y: 2, Z: 5}
	wall := core.BlockPos{X: 5, Y: 1, Z: 5}
	fixture.engine.SetBlockForTest(standing, core.TorchStandingID)
	fixture.engine.SetBlockForTest(wall, core.TorchWallPosXID)

	var result TickResult
	for range 5 {
		result = holdCompanionMineAction(t, fixture)
	}
	if got := companionMiningBlockAt(t, fixture); got != core.AirID {
		t.Fatalf("支撑格 = %d，想要空气", got)
	}
	for _, torchPos := range []core.BlockPos{standing, wall} {
		if got := tillBlockAt(t, fixture.engine, torchPos); got != core.AirID {
			t.Fatalf("火把 %+v = %d，想要空气", torchPos, got)
		}
	}
	if len(result.Changes) != 1 {
		t.Fatalf("完成 tick 应只有一个变更批次: %+v", result.Changes)
	}
	batch := result.Changes[0]
	if batch.NewRevision != batch.BaseRevision+1 {
		t.Fatalf("revision = %d→%d，想要同批恰好一次推进",
			batch.BaseRevision, batch.NewRevision)
	}
	got := map[core.BlockPos]core.BlockID{}
	for _, change := range batch.Changes {
		got[change.Position] = change.Block
	}
	want := map[core.BlockPos]core.BlockID{
		fixture.target: core.AirID,
		standing:       core.AirID,
		wall:           core.AirID,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("完成 tick 变更 = %+v，想要 %+v", got, want)
	}

	// 伙伴采掘产物直入背包，火把掉落留在世界：两枚、各在原火把格。
	if count := companionItemCount(fixture.entry, core.ItemDirt); count != 1 {
		t.Fatalf("伙伴背包泥土 = %d，想要 1", count)
	}
	drops := torchChunkDrops(t, fixture.engine)
	if len(drops) != 2 {
		t.Fatalf("活动掉落物 = %d 个，想要 2 枚火把: %+v", len(drops), drops)
	}
	positions := map[core.BlockPos]bool{}
	for _, drop := range drops {
		if drop.Stack != (core.ItemStack{Item: core.ItemTorch, Count: 1}) {
			t.Fatalf("掉落物 = %+v，想要 1 个火把", drop.Stack)
		}
		position, ok := world.BlockPosFromChunkIndex(core.ChunkPos{}, drop.BlockIndex)
		if !ok {
			t.Fatal("火把掉落没有位置")
		}
		positions[position] = true
	}
	if !positions[standing] || !positions[wall] {
		t.Fatalf("火把掉落位置 = %+v，想要两枚各自落在原火把格", positions)
	}
}

// TestNonSupportNeighborChangeKeepsTorch 覆盖 spec 场景「非支撑邻居变化不影响」：
// 火把相邻格（贴着火把本体的 -Z 侧）被放置泥土，火把保持原位且不产生掉落。
func TestNonSupportNeighborChangeKeepsTorch(t *testing.T) {
	engine, session := readyTorchPlayer(t)
	support := core.BlockPos{X: 0, Y: 1, Z: 4}
	torchPos := core.BlockPos{X: 0, Y: 2, Z: 4}
	engine.SetBlockForTest(support, core.DirtID)
	eye := torchEye(engine, session)

	placeYaw, placePitch := lookAtPoint(eye, torchTopFaceCenter(support))
	placed := torchPlace(engine, session, 2, 0, placeYaw, placePitch)
	if len(placed.Rejected) != 0 || tillBlockAt(t, engine, torchPos) != core.TorchStandingID {
		t.Fatalf("火把没有放到支撑格上方: Rejected=%+v", placed.Rejected)
	}

	// 瞄准火把本体的 -Z 面，把泥土放进它正前方的相邻格：该格是火把的六邻居
	// 之一，但不是支撑格——火把必须保持原位。
	dirtYaw, dirtPitch := lookAtBlockCenter(eye, torchPos)
	result := torchPlace(engine, session, 3, 1, dirtYaw, dirtPitch)

	if len(result.Rejected) != 0 {
		t.Fatalf("泥土放置被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, core.BlockPos{X: 0, Y: 2, Z: 3}); got != core.DirtID {
		t.Fatalf("相邻格 = %d，想要泥土", got)
	}
	if got := tillBlockAt(t, engine, torchPos); got != core.TorchStandingID {
		t.Fatalf("非支撑邻居变化移除了火把: %d", got)
	}
	if drops := torchChunkDrops(t, engine); len(drops) != 0 {
		t.Fatalf("非支撑邻居变化产生了掉落: %+v", drops)
	}
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 {
		t.Fatalf("本 tick 应只有泥土一条变更: %+v", result.Changes)
	}
}

// TestSupportRemovalKeepsTorchWhenDropCapacityFull 覆盖 spec 场景「掉落容量
// 不足时火把保留原位」：支撑格被采掘但火把所在区块的掉落槽已满时，火把整体
// 保留原位——无空气写入、无掉落、广播里没有火把的第二条变更；清空容量并让
// 该格再次发生权威变化后，复核重新触发并完成移除与掉落。
func TestSupportRemovalKeepsTorchWhenDropCapacityFull(t *testing.T) {
	engine, session := readyTorchPlayer(t)
	support := core.BlockPos{X: 0, Y: 1, Z: 4}
	torchPos := core.BlockPos{X: 0, Y: 2, Z: 4}
	engine.SetBlockForTest(support, core.DirtID)
	engine.SetBlockForTest(torchPos, core.TorchStandingID)
	// 预占 32 个掉落槽中的 31 个：采掘的泥土掉落取走最后一个，火把的补位
	// 预检随即落空——构造「支撑已移除而容量全满」的精确临界。
	key := core.ChunkKey{Dimension: core.Overworld}
	elsewhere, ok := world.ChunkBlockIndex(core.BlockPos{X: 5, Y: 0, Z: 5})
	if !ok {
		t.Fatal("占位方块没有区块索引")
	}
	for slot := 0; slot < core.DropsPerChunk-1; slot++ {
		engine.SetChunkDropForTest(key, slot, world.DropSlot{
			Generation:       1,
			Active:           true,
			Stack:            core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount},
			BlockIndex:       elsewhere,
			PickupDelayTicks: tuning.DefaultTunables().DropPickupDelayTicks,
		})
	}
	eye := torchEye(engine, session)

	// 采掘支撑格：泥土掉落占用最后一个槽，容量全满，火把必须保留原位。
	mineYaw, minePitch := lookAtBlockCenter(eye, support)
	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandPlayerInput,
		Yaw: mineYaw, Pitch: minePitch, Mining: true,
	})
	var mined TickResult
	for range 5 {
		mined = engine.Step()
	}
	if len(mined.Rejected) != 0 {
		t.Fatalf("采掘被拒绝: %+v", mined.Rejected)
	}
	if got := tillBlockAt(t, engine, support); got != core.AirID {
		t.Fatalf("支撑格 = %d，想要空气", got)
	}
	if got := tillBlockAt(t, engine, torchPos); got != core.TorchStandingID {
		t.Fatalf("容量不足仍移除了火把: %d", got)
	}
	if len(mined.Changes) != 1 || len(mined.Changes[0].Changes) != 1 ||
		mined.Changes[0].Changes[0] != (BlockChange{Position: support, Block: core.AirID}) {
		t.Fatalf("完成 tick 广播应只有支撑格一条变更: %+v", mined.Changes)
	}
	if drops := torchChunkDrops(t, engine); len(drops) != core.DropsPerChunk {
		t.Fatalf("活动掉落物 = %d，想要恰好占满全部槽位", len(drops))
	}

	// 清空容量，再让支撑格发生两次权威变化：放回泥土（实心支撑，火把理应
	// 保留）之后采掘（非实心），第二次复核完成移除与掉落。松开按住的采掘键，
	// 否则放置 tick 内的 advanceMining 会对刚放回的泥土预先进度 +1，把第二次
	// 采掘的完成时机提前一格。
	for slot := range core.DropsPerChunk {
		engine.SetChunkDropForTest(key, slot, world.DropSlot{})
	}
	engine.sessions[session].player.miningHeld = false
	placeYaw, placePitch := lookAtPoint(eye, torchTopFaceCenter(core.BlockPos{X: 0, Y: 0, Z: 4}))
	placed := torchPlace(engine, session, 3, 1, placeYaw, placePitch)
	if len(placed.Rejected) != 0 || tillBlockAt(t, engine, support) != core.DirtID {
		t.Fatalf("支撑格没有放回泥土: Rejected=%+v", placed.Rejected)
	}
	if got := tillBlockAt(t, engine, torchPos); got != core.TorchStandingID {
		t.Fatalf("放回实心支撑却移除了火把: %d", got)
	}

	engine.Enqueue(Command{
		Session: session, Sequence: 4, Kind: CommandPlayerInput,
		Yaw: mineYaw, Pitch: minePitch, Mining: true,
	})
	var remined TickResult
	for range 5 {
		remined = engine.Step()
	}
	if got := tillBlockAt(t, engine, support); got != core.AirID {
		t.Fatalf("支撑格 = %d，想要空气", got)
	}
	if got := tillBlockAt(t, engine, torchPos); got != core.AirID {
		t.Fatalf("容量恢复后火把没有被移除: %d", got)
	}
	if len(remined.Changes) != 1 {
		t.Fatalf("完成 tick 应只有一个变更批次: %+v", remined.Changes)
	}
	changed := map[core.BlockPos]core.BlockID{}
	for _, change := range remined.Changes[0].Changes {
		changed[change.Position] = change.Block
	}
	want := map[core.BlockPos]core.BlockID{support: core.AirID, torchPos: core.AirID}
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("完成 tick 变更 = %+v，想要 %+v", changed, want)
	}
	drops := torchChunkDrops(t, engine)
	if len(drops) != 2 {
		t.Fatalf("活动掉落物 = %d，想要 1 泥土 + 1 火把: %+v", len(drops), drops)
	}
	counts := map[core.ItemID]uint8{}
	for _, drop := range drops {
		counts[drop.Stack.Item] += drop.Stack.Count
	}
	if counts[core.ItemDirt] != 1 || counts[core.ItemTorch] != 1 {
		t.Fatalf("掉落物数量 = %+v，想要 1 泥土 + 1 火把", counts)
	}
}

// —— 火把配方验收（零新分支：既有网格匹配与取出路径） ——

// readyTorchCraftingPlayer 构造一个已激活、快捷栏持有煤炭与木棍的玩家。
func readyTorchCraftingPlayer(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	engine := NewEngine(0, 0, 0)
	session := SessionID(1)
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemCoal, Count: 4}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStick, Count: 4}
	engine.RegisterPlayer(session, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
		Inventory:      inventory,
	})
	requested := engine.Step()
	for _, key := range requested.Acquire {
		engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
	}
	engine.Step()
	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Chunk:     movementFlatChunk(core.ChunkPos{}),
	})
	ready := engine.Step()
	if len(ready.Ready) != 1 {
		t.Fatalf("平坦区块未就绪: %+v", ready.Ready)
	}
	return engine, session
}

// TestTorchCraftedInPersonalGridYieldsFour 验收「煤上棍下」配方经既有
// 格子合成闭环生效：2×2 个人网格摆纵向两格，一次取出恰好得到 4 个火把，
// 网格按形状各消费一份。
func TestTorchCraftedInPersonalGridYieldsFour(t *testing.T) {
	engine, session := readyTorchCraftingPlayer(t)
	// 统一视图格：网格 0..3、背包 9..44（快捷栏栏位 i → 视图格 9+i）。
	// 煤炭进网格 0（上行），木棍进网格 2（下行）——煤炭位于木棍正上方。
	engine.Enqueue(Command{
		Session: session, Sequence: 1, Kind: CommandMoveCraftingStack, Slot: 9, ToSlot: 0,
	})
	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandMoveCraftingStack, Slot: 10, ToSlot: 2,
	})
	moved := engine.Step()
	if len(moved.Rejected) != 0 {
		t.Fatalf("网格移动被拒绝: %+v", moved.Rejected)
	}
	grid, output, ok := engine.PlayerCrafting(session)
	if !ok || grid.Size != CraftingGridSizePersonal {
		t.Fatalf("个人网格不可用: %+v", grid)
	}
	if output != (core.ItemStack{Item: core.ItemTorch, Count: 4}) {
		t.Fatalf("派生产物 = %+v，想要 4 个火把", output)
	}

	engine.Enqueue(Command{
		Session: session, Sequence: 3, Kind: CommandTakeCraftingOutput,
	})
	taken := engine.Step()
	if len(taken.Rejected) != 0 {
		t.Fatalf("产物取出被拒绝: %+v", taken.Rejected)
	}
	player := engine.sessions[session].player
	total := 0
	for slot := range player.inventory.Hotbar.Slots {
		if player.inventory.Hotbar.Slots[slot].Item == core.ItemTorch {
			total += int(player.inventory.Hotbar.Slots[slot].Count)
		}
	}
	for slot := range player.inventory.Backpack {
		if player.inventory.Backpack[slot].Item == core.ItemTorch {
			total += int(player.inventory.Backpack[slot].Count)
		}
	}
	if total != 4 {
		t.Fatalf("背包火把总数 = %d，想要恰好 4 个", total)
	}
	// 一次取出对匹配形状的每个非空格恰减 1：4+4 的摆放取出后应剩 3 煤 + 3 棍，
	// 形状仍在网格上、派生产物仍是 4 个火把（与既有格子合成语义逐字一致）。
	grid, output, _ = engine.PlayerCrafting(session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemCoal, Count: 3}) ||
		grid.Slots[2] != (core.ItemStack{Item: core.ItemStick, Count: 3}) {
		t.Fatalf("取出后网格 = [0]%+v [2]%+v，想要各恰减一份", grid.Slots[0], grid.Slots[2])
	}
	if output != (core.ItemStack{Item: core.ItemTorch, Count: 4}) {
		t.Fatalf("取出后派生产物 = %+v，想要仍是 4 个火把", output)
	}
}

// —— 伙伴不获得火把能力 ——

// TestCompanionMineableBlockRejectsTorchForms 锁定 spec 场景「伙伴拒绝处理火把」
// 的模拟执行侧（第二重校验）：五种火把形态在 core.BlockDrop 里都有单一产物
// 登记，「必须有单一 BlockDrop」的通用判据会放行它们，因此显式拒绝是承重墙，
// 与农业方块的防御同一样式。
func TestCompanionMineableBlockRejectsTorchForms(t *testing.T) {
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if _, ok := core.BlockDrop(id); !ok {
			t.Fatalf("火把形态 %d 已不在 core.BlockDrop 中，本用例的前提失效", id)
		}
		if companionMineableBlock(id) {
			t.Fatalf("companionMineableBlock(%d) = true，伙伴必须被显式拒绝", id)
		}
	}
}

// TestCompanionPlaceableBlockRejectsTorchForms 锁定放置半边的模拟执行侧：火把
// 由既有往返校验**天然**拒绝——BlockDrop(火把) = 火把物品，而 ItemPlacement
// (火把物品) 失败（火把形态由命中面决定，不存在无面映射），往返不成立。本断言
// 钉死这个天然拒绝：未来若把火把追加进 ItemPlacement，这里必须先红，防止伙伴
// 静默获得火把放置能力。
func TestCompanionPlaceableBlockRejectsTorchForms(t *testing.T) {
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if _, ok := companionPlaceableBlock(id); ok {
			t.Fatalf("companionPlaceableBlock(%d) 放行，伙伴不得获得火把放置能力", id)
		}
	}
}
