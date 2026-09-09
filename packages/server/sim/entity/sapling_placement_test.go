package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是「树苗种植」主题：`ItemSapling` 的放置分叉——目标格必须是空气、
// 正下方必须是泥土或草，成功恰好消耗 1 个，任何拒绝零副作用。与
// farming_test.go 的种植用例同形（同一套 `plantBelow`/`plantTarget` 夹具与
// 两种瞄法），差别只在落脚方块白名单与拒绝面。

// saplingPlantCount 是种植夹具持有的树苗数量。它必须 ≥ 2 且断言取**精确值**：
// 夹具若只放 0 或 1 个树苗，「拒绝时数量不变」在扣与不扣两种实现下都是同一个
// 读数，差值恒等于零（与 plantSeedCount 同一理由）。
const saplingPlantCount = uint8(4)

// readySaplingPlanter 构造一个持有 saplingPlantCount 个树苗、俯视 plantBelow
// 顶面的玩家；below 写进 plantBelow，决定树苗的落脚方块。
func readySaplingPlanter(
	t *testing.T,
	below core.BlockID,
) (*Engine, SessionID, float32, float32) {
	t.Helper()
	engine, session, yaw, pitch := readyPlantingPlayer(t, below)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemSapling, Count: saplingPlantCount}
	return engine, session, yaw, pitch
}

// placeSapling 发一条放置命令（选中第 0 格的树苗）并推进一个权威 tick。
func placeSapling(engine *Engine, session SessionID, yaw, pitch float32) TickResult {
	return settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlaceBlock, Slot: 0,
		Yaw: yaw, Pitch: pitch,
	}})
}

// TestPlantSaplingOnDirtOrGrass 覆盖 Scenario「泥土上方种植成功」：泥土或草方块
// 正上方的空气格出现树苗，选中 stack 恰好减少 1。
//
// 两种瞄法都必须成立：俯视支撑方块顶面（命中面就是支撑格）与瞄准旁边方块的
// 侧面（命中的是石头、目标格才落在支撑格上方）。判据读的是**目标格正下方**
// 而不是命中面，第二种瞄法是这条位置性判据的守卫——若实现改成"命中面必须是
// 泥土或草"，只有它会红。
func TestPlantSaplingOnDirtOrGrass(t *testing.T) {
	for _, support := range []core.BlockID{core.DirtID, core.GrassID} {
		for _, aim := range []struct {
			name string
			look func(*Engine, SessionID) (float32, float32)
		}{
			{
				name: "俯视支撑方块顶面",
				look: func(engine *Engine, session SessionID) (float32, float32) {
					player := engine.sessions[session].player
					eye := player.state.Position.
						Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
					return lookAtBlockTop(eye, plantBelow)
				},
			},
			{
				// 侧面瞄法：plantTarget 的 +Z 邻格放一块石头，射线命中它的 −Z
				// 侧面，目标格因此正是 plantTarget。
				name: "瞄准旁边方块的侧面",
				look: func(engine *Engine, session SessionID) (float32, float32) {
					side := plantTarget
					side.Z++
					engine.SetBlockForTest(side, core.StoneID)
					player := engine.sessions[session].player
					eye := player.state.Position.
						Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
					return lookAtBlockCenter(eye, side)
				},
			},
		} {
			t.Run(blockLabel(support)+"/"+aim.name, func(t *testing.T) {
				engine, session, _, _ := readySaplingPlanter(t, support)
				yaw, pitch := aim.look(engine, session)

				result := placeSapling(engine, session, yaw, pitch)

				if len(result.Rejected) != 0 {
					t.Fatalf("合法种植被拒绝: %+v", result.Rejected)
				}
				if got := tillBlockAt(t, engine, plantTarget); got != core.SaplingID {
					t.Fatalf("种植结果 = %d，想要树苗 %d", got, core.SaplingID)
				}
				want := core.ItemStack{Item: core.ItemSapling, Count: saplingPlantCount - 1}
				player := engine.sessions[session].player
				if got := player.inventory.Hotbar.Slots[0]; got != want {
					t.Fatalf("种植后栏位 = %+v，想要恰好 −1 的 %+v", got, want)
				}
				// 方块变更必须经既有 recordChange 汇入本 tick 的批次；只改内存
				// 不广播同样能让上面的断言全绿。
				if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
					result.Changes[0].Changes[0] != (BlockChange{
						Position: plantTarget, Block: core.SaplingID,
					}) {
					t.Fatalf("种植没有广播为区块变更: %+v", result.Changes)
				}
			})
		}
	}
}

// TestPlantSaplingRejectsUnsupportedGround 覆盖 Scenario「耕地上方被拒绝」并穷举
// 其余非法支撑：干湿耕地、石头、沙子、砂砾之上都不能种，且树苗数量一个不掉。
//
// 石头与沙子是「不引入耕地以外其它支撑」的守卫：一个写成"只拒绝耕地"的实现
// 能通过耕地两条子用例，却会让石头/沙子之上长出树苗。
func TestPlantSaplingRejectsUnsupportedGround(t *testing.T) {
	for _, below := range []core.BlockID{
		core.FarmlandDryID, core.FarmlandWetID, core.StoneID, core.SandID, core.GravelID,
	} {
		t.Run(blockLabel(below), func(t *testing.T) {
			engine, session, yaw, pitch := readySaplingPlanter(t, below)

			requireSaplingPlacementRejected(
				t, engine, session, yaw, pitch, core.AirID, RejectInvalidBlock,
			)
		})
	}
}

// requireSaplingPlacementRejected 断言一次放置被拒绝且零副作用：恰好一条给定
// 理由码、目标格保留原方块、树苗数量逐字段不变、没有区块变更广播。
func requireSaplingPlacementRejected(
	t *testing.T,
	engine *Engine,
	session SessionID,
	yaw, pitch float32,
	targetBlock core.BlockID,
	reason RejectReason,
) {
	t.Helper()
	result := placeSapling(engine, session, yaw, pitch)

	if len(result.Rejected) != 1 || result.Rejected[0].Reason != reason {
		t.Fatalf("Rejected = %+v，想要恰好一条 %v", result.Rejected, reason)
	}
	if got := tillBlockAt(t, engine, plantTarget); got != targetBlock {
		t.Fatalf("被拒绝的种植改了目标格: %d，想要保留 %d", got, targetBlock)
	}
	want := core.ItemStack{Item: core.ItemSapling, Count: saplingPlantCount}
	player := engine.sessions[session].player
	if got := player.inventory.Hotbar.Slots[0]; got != want {
		t.Fatalf("被拒绝的种植扣了树苗: %+v，想要一字不变的 %+v", got, want)
	}
	if len(result.Changes) != 0 {
		t.Fatalf("被拒绝的种植广播了区块变更: %+v", result.Changes)
	}
}

// TestPlantSaplingRejectsOccupiedTarget 覆盖 Scenario「目标非空气被拒绝」：
// 目标格已被方块或流体占用时拒绝，方块与物品逐字段不变。
//
// 流体格不是射线目标，射线穿水命中水下的支撑格，落点因此正是那格水（与
// farming 的种子用例同形），走树苗自己的空气前置 `RejectInvalidBlock`。
// 「方块占用」这条无法用普通固体构造：固体是射线目标，射线会先命中它、落点
// 变成它前面的格子。因此用开启的下半门当占用物——开启门可穿透（不是射线
// 目标），射线穿过它命中后面的石头，落点正是被门占据的 `plantTarget`，由
// 通用占用判据给出 `RejectOccupied`。两条路径都必须零副作用。
func TestPlantSaplingRejectsOccupiedTarget(t *testing.T) {
	t.Run("流体", func(t *testing.T) {
		engine, session, yaw, pitch := readySaplingPlanter(t, core.DirtID)
		engine.SetBlockForTest(plantTarget, core.WaterSourceID)

		requireSaplingPlacementRejected(
			t, engine, session, yaw, pitch, core.WaterSourceID, RejectInvalidBlock,
		)
	})
	t.Run("方块占用", func(t *testing.T) {
		engine, session, _, _ := readySaplingPlanter(t, core.DirtID)
		engine.SetBlockForTest(plantTarget, core.DoorLowerSouthOpen)
		side := plantTarget
		side.Z++
		engine.SetBlockForTest(side, core.StoneID)
		player := engine.sessions[session].player
		eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		yaw, pitch := lookAtBlockCenter(eye, side)

		requireSaplingPlacementRejected(
			t, engine, session, yaw, pitch, core.DoorLowerSouthOpen, RejectOccupied,
		)
	})
}

// TestPlaceNonSaplingItemsIgnoreSaplingSupport 是「非树苗物品的放置行为一字
// 不变」的守卫：石头支撑之上放石头必须照常成功。
//
// 没有它的话，一个把「下方必须是泥土或草」错加到**全部**放置物上的实现会让
// 上面几条树苗用例全绿——「树苗被正确拒绝」与「所有放置都被拒绝」在树苗用例
// 里读数完全相同（沿 TestPlaceNonSeedItemsIgnoreFarmlandPrecondition 先例）。
func TestPlaceNonSaplingItemsIgnoreSaplingSupport(t *testing.T) {
	engine, session, yaw, pitch := readySaplingPlanter(t, core.StoneID)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[1] = core.ItemStack{
		Item: core.ItemStone, Count: core.MaxStackCount,
	}
	result := settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlaceBlock, Slot: 1,
		Yaw: yaw, Pitch: pitch,
	}})

	if len(result.Rejected) != 0 {
		t.Fatalf("石头之上放置石头被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, plantTarget); got != core.StoneID {
		t.Fatalf("非树苗放置结果 = %d，想要石头 %d", got, core.StoneID)
	}
}
