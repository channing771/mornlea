package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定被动牛的小麦引诱跟随：同维最近 `active` 玩家权威选中格握着小
// 麦且水平距离不超过引诱半径时，牛转向该玩家并在止步距离内停下；切走小麦、
// 超距、异维或离线即恢复漫游；逃跑优先于引诱，吃草事件中不引诱；引诱不消耗
// 小麦、不增殖个体。

// newTemptEngine 返回带一名 `active` 玩家的引诱测试引擎：玩家落在原点草地，
// 周围 3×3 区块已装载，被动集合只含用例亲手恢复的牛。
func newTemptEngine(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	engine, session := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	return engine, session
}

// holdHotbar 让该玩家权威选中格握着指定物品（选中格恒为 0 号格）。
func holdHotbar(engine *Engine, session SessionID, item core.ItemID, count uint8) {
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: item, Count: count}
	player.inventory.Hotbar.Selected = 0
}

// placeSessionPlayer 把该玩家身体直接摆到指定落点（只调位置，不走传送结算）。
func placeSessionPlayer(engine *Engine, session SessionID, position mgl32.Vec3) {
	engine.sessions[session].player.state.Position = position
}

// horizontalDist 返回两点间的水平距离。
func horizontalDist(from, to mgl32.Vec3) float32 {
	dx := to.X() - from.X()
	dz := to.Z() - from.Z()
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

func TestPassiveTemptFollowsWheatHolderAndStops(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 21, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 5)
	placeSessionPlayer(engine, session, mgl32.Vec3{8.5, 1, 2.5})
	// 6 格外持麦靠近：有限步内必须走进止步距离。
	stopped := false
	for range 300 {
		engine.advancePassiveMovement()
		cow := engine.passives.entries[0].state.Position
		player := engine.sessions[session].player.state.Position
		if horizontalDist(cow, player) <= 2.5 {
			stopped = true
			break
		}
	}
	if !stopped {
		t.Fatal("持麦 6 格外 300 tick 内未走进止步距离，想要引诱跟随")
	}
	// 止步后只给中性输入：残余速度的摩擦滑行有界，绝不继续走向玩家。
	atStop := horizontalDist(
		engine.passives.entries[0].state.Position,
		engine.sessions[session].player.state.Position,
	)
	if input := engine.passiveStepInput(&engine.passives.entries[0]); input.MoveZ != 0 {
		t.Fatalf("止步后输入=%+v，想要中性输入", input)
	}
	for range 10 {
		engine.advancePassiveMovement()
	}
	final := horizontalDist(
		engine.passives.entries[0].state.Position,
		engine.sessions[session].player.state.Position,
	)
	if final > 2.5+0.2 {
		t.Fatalf("止步后距离=%v，想要停在 2.5 格附近（+0.2 滑行容差）", final)
	}
	if atStop-final > 0.2 {
		t.Fatalf("止步后仍在走近：距离 %v→%v，想要不再主动靠近", atStop, final)
	}
	// 引诱不消耗小麦、不增殖个体。
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got.Count != 5 {
		t.Fatalf("跟随后小麦余量=%+v，想要原样保留 5 个", got)
	}
	if len(engine.passives.entries) != 1 {
		t.Fatalf("跟随后被动牛=%d 头，想要仍为 1 头", len(engine.passives.entries))
	}
}

func TestPassiveTemptRadiusBoundary(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 22, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	// 恰好 8 格是边界内：必须命中目标且单步走近。
	placeSessionPlayer(engine, session, mgl32.Vec3{10.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	if _, ok := engine.passiveTemptTarget(entry); !ok {
		t.Fatal("水平 8 格持麦未命中引诱目标，想要边界含入")
	}
	before := horizontalDist(entry.state.Position, engine.sessions[session].player.state.Position)
	engine.advancePassiveMovement()
	after := horizontalDist(engine.passives.entries[0].state.Position, engine.sessions[session].player.state.Position)
	if after >= before {
		t.Fatalf("边界持麦前后距离=%v→%v，想要走近", before, after)
	}
	// 8 格外即超距：目标必须为空。
	placeSessionPlayer(engine, session, mgl32.Vec3{11, 1, 2.5})
	if _, ok := engine.passiveTemptTarget(&engine.passives.entries[0]); ok {
		t.Fatal("水平 8.5 格持麦仍命中目标，想要超距恢复漫游")
	}
}

func TestPassiveTemptStopsExactlyAtTwoAndHalf(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 23, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	placeSessionPlayer(engine, session, mgl32.Vec3{5, 1, 2.5})
	entry := &engine.passives.entries[0]
	// 恰好 2.5 格仍在引诱半径内，但必须止步：输入中性（位移冻结），朝向仍
	// 每 tick 有界转向持麦玩家。
	if _, ok := engine.passiveTemptTarget(entry); !ok {
		t.Fatal("水平 2.5 格持麦未命中目标，想要半径内保持引诱态")
	}
	yaw := entry.yaw
	input := engine.passiveStepInput(entry)
	if input.MoveZ != 0 {
		t.Fatalf("止步距离内输入=%+v，想要中性输入", input)
	}
	want := normalizeYaw(float32(math.Atan2(float64(-(5 - 2.5)), float64(-(2.5 - 2.5)))))
	if got := turnYawToward(yaw, want, passiveIdleLookMaxTurn); entry.yaw != got {
		t.Fatalf("止步后朝向=%v，想要有界转向 %v", entry.yaw, got)
	}
	if entry.yaw == yaw {
		t.Fatal("止步后朝向不变，想要原地转向玩家")
	}
	before := entry.state.Position
	engine.advancePassiveMovement()
	if moved := horizontalDist(before, engine.passives.entries[0].state.Position); moved > 1e-6 {
		t.Fatalf("止步距离内水平位移=%v，想要原地不动", moved)
	}
}

// TestPassiveTemptStoppedCowTurnsToFacePlayer 锁定止步转向收敛：持麦玩家横
// 移到牛的另一侧，止步牛原地转向玩家、位置不变，朝向约束随漫游恢复解除。
func TestPassiveTemptStoppedCowTurnsToFacePlayer(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 26, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	placeSessionPlayer(engine, session, mgl32.Vec3{5, 1, 2.5})
	entry := &engine.passives.entries[0]
	// 先转向东侧玩家收敛。
	for range 40 {
		engine.advancePassiveMovement()
	}
	east := normalizeYaw(float32(math.Atan2(float64(-(5 - 2.5)), float64(-(2.5 - 2.5)))))
	if entry.yaw != east {
		t.Fatalf("东侧收敛朝向=%v，想要 %v", entry.yaw, east)
	}
	// 玩家横移到西侧：牛原地转向，位置不动。
	placeSessionPlayer(engine, session, mgl32.Vec3{0, 1, 2.5})
	west := normalizeYaw(float32(math.Atan2(float64(-(0 - 2.5)), float64(-(2.5 - 2.5)))))
	for range 40 {
		before := entry.state.Position
		engine.advancePassiveMovement()
		if moved := horizontalDist(before, engine.passives.entries[0].state.Position); moved > 1e-6 {
			t.Fatalf("转向时水平位移=%v，想要原地不动", moved)
		}
	}
	if entry.yaw != west {
		t.Fatalf("西侧收敛朝向=%v，想要 %v", entry.yaw, west)
	}
}

func TestPassiveTemptNeedsWheatInSelectedSlot(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 24, mgl32.Vec3{2.5, 1, 2.5})
	placeSessionPlayer(engine, session, mgl32.Vec3{8.5, 1, 2.5})
	player := engine.sessions[session].player
	// 小麦在 0 号格、选中 1 号格泥土：选中格说了算，必须不引诱。
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemWheat, Count: 1}
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	player.inventory.Hotbar.Selected = 1
	if _, ok := engine.passiveTemptTarget(&engine.passives.entries[0]); ok {
		t.Fatal("选中格为泥土时命中目标，想要只认选中格")
	}
	// 空手（选中格为空）同样不引诱。
	player.inventory.Hotbar = core.Hotbar{}
	if _, ok := engine.passiveTemptTarget(&engine.passives.entries[0]); ok {
		t.Fatal("空手时命中目标，想要无麦不跟随")
	}
}

func TestPassiveTemptSwitchAwayResumesWander(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 25, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	placeSessionPlayer(engine, session, mgl32.Vec3{8.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	if _, ok := engine.passiveTemptTarget(entry); !ok {
		t.Fatal("对照失效：持麦时未命中目标")
	}
	// 切走小麦：目标清空；玩家同时退到 6 格外（闲时看人够不着），输入回到
	// 确定性漫游派生。
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	player.inventory.Hotbar.Selected = 1
	placeSessionPlayer(engine, session, mgl32.Vec3{10.5, 1, 2.5})
	if _, ok := engine.passiveTemptTarget(entry); ok {
		t.Fatal("切走小麦后仍命中目标，想要下一 tick 恢复漫游")
	}
	engine.tick.Store(77)
	input := engine.passiveStepInput(entry)
	base := splitmix64(uint64(engine.seed) ^ uint64(77) ^ entry.id)
	wantYaw := normalizeYaw(float32(base&0xFFFFFF) * (2 * math.Pi / 0x1000000))
	if input.MoveZ != 1 || input.Yaw != wantYaw {
		t.Fatalf("切走后输入=%+v，想要漫游派生 (MoveZ=1,Yaw=%v)", input, wantYaw)
	}
}

func TestPassiveTemptIgnoresOtherDimensionAndOffline(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 26, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	placeSessionPlayer(engine, session, mgl32.Vec3{4.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	if _, ok := engine.passiveTemptTarget(entry); !ok {
		t.Fatal("对照失效：同维持麦未命中目标")
	}
	// 异维玩家即使贴身持麦也不得被选中。
	engine.sessions[session].dimension = core.Overworld + 1
	if _, ok := engine.passiveTemptTarget(entry); ok {
		t.Fatal("异维持麦命中目标，想要只认同维玩家")
	}
	// 离线（待重生）玩家同样排除。
	engine.sessions[session].dimension = core.Overworld
	engine.sessions[session].player.lifecycle = PlayerPendingSpawn
	if _, ok := engine.passiveTemptTarget(entry); ok {
		t.Fatal("离线持麦命中目标，想要只认 active 玩家")
	}
}

func TestPassiveTemptPicksNearestHolder(t *testing.T) {
	engine, first := newTemptEngine(t)
	restoreGrazeCow(t, engine, 27, mgl32.Vec3{2.5, 1, 2.5})
	second := SessionID(2)
	engine.RegisterSession(second, core.Overworld, core.ChunkPos{})
	advanceActorsTick(engine)
	holdHotbar(engine, first, core.ItemWheat, 1)
	holdHotbar(engine, second, core.ItemWheat, 1)
	near := mgl32.Vec3{2.5, 1, 5.5}
	far := mgl32.Vec3{8.5, 1, 2.5}
	placeSessionPlayer(engine, first, far)
	placeSessionPlayer(engine, second, near)
	target, ok := engine.passiveTemptTarget(&engine.passives.entries[0])
	if !ok {
		t.Fatal("双持麦无命中目标，想要选中最近者")
	}
	if target != near {
		t.Fatalf("引诱目标=%v，想要最近的 %v", target, near)
	}
}

func TestPassiveTemptFleeBeatsTempt(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 28, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	playerPos := mgl32.Vec3{8.5, 1, 2.5}
	placeSessionPlayer(engine, session, playerPos)
	// 伤害来源即持麦玩家站位：逃跑方向与引诱方向正相反，位移即可裁决优先级。
	if !engine.DamagePassive(28, 1, playerPos) {
		t.Fatal("已知个体的有效伤害被拒绝，想要受理")
	}
	for range 20 {
		engine.advancePassiveMovement()
	}
	cow := engine.passives.entries[0].state.Position
	if cow.X() >= 2.5 {
		t.Fatalf("受击后牛 X=%v，想要远离持麦玩家（X 减小）", cow.X())
	}
	if got := horizontalDist(cow, playerPos); got <= 6 {
		t.Fatalf("受击后与持麦玩家距离=%v，想要拉开到 6 格以上", got)
	}
}

func TestPassiveTemptFrozenWhileGrazing(t *testing.T) {
	engine, session := newTemptEngine(t)
	restoreGrazeCow(t, engine, 29, mgl32.Vec3{2.5, 1, 2.5})
	holdHotbar(engine, session, core.ItemWheat, 1)
	placeSessionPlayer(engine, session, mgl32.Vec3{8.5, 1, 2.5})
	entry := &engine.passives.entries[0]
	// 白盒摆出事件中态：引诱输入不得生效，移动保持冻结。
	entry.grazeTicks = 20
	yaw := entry.yaw
	input := engine.passiveStepInput(entry)
	if input.MoveZ != 0 || input.Yaw != yaw {
		t.Fatalf("吃草事件中输入=%+v，想要中性输入（不引诱转向）", input)
	}
	before := entry.state.Position
	engine.advancePassiveMovement()
	if engine.passives.entries[0].state.Position != before {
		t.Fatal("吃草事件中牛位移，想要静止低头")
	}
}
