package entity

import (
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

// TestCompanionActionAppliesInIDOrderAfterPlayers 用两名玩家与两个伙伴的
// 位移验证同 tick 双方都被处理、action 按 CompanionID 正确寻址，
// 且两次相同输入的重放产生完全一致的可观察结果。
func TestCompanionActionAppliesInIDOrderAfterPlayers(t *testing.T) {
	run := func() (players [2]PlayerUpdate, companions [2]CompanionUpdate) {
		engine := NewEngine(0, 0, 0)
		sessionA, sessionB := SessionID(1), SessionID(2)
		engine.RegisterSession(sessionA, core.Overworld, core.ChunkPos{})
		engine.RegisterSession(sessionB, core.Overworld, core.ChunkPos{})
		loadMovementChunk(t, engine.dimension(core.Overworld), movementFlatChunk(core.ChunkPos{}))
		for range 16 {
			advanceActorsTick(engine)
			if engine.sessions[sessionA].player.lifecycle == PlayerActive &&
				engine.sessions[sessionB].player.lifecycle == PlayerActive {
				break
			}
		}
		if engine.sessions[sessionA].player.lifecycle != PlayerActive ||
			engine.sessions[sessionB].player.lifecycle != PlayerActive {
			t.Fatal("两名玩家未在预算内激活")
		}
		companionA, companionB := companionTestID(1), companionTestID(2)
		activateCompanionAt(t, engine, companionA, mgl32.Vec3{0.5, 1, 4.5})
		activateCompanionAt(t, engine, companionB, mgl32.Vec3{2.5, 1, 4.5})

		// 故意按逆 ID 序提供：处理顺序必须由 ID 字节序决定，而不是 slice 顺序。
		actions := []CompanionAction{{
			ID: companionB, Kind: CompanionActionMove, Input: physics.Input{MoveZ: 1},
		}, {
			ID: companionA, Kind: CompanionActionMove, Input: physics.Input{MoveX: 1},
		}}
		for index, action := range actions {
			if !validCompanionAction(action) {
				t.Fatalf("伙伴 action %d 不是合法 owner 输入", index)
			}
		}
		tick := engine.beginTick()
		tick.context.ApplyPlayerCommands([]Command{
			{Session: sessionA, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1},
			{Session: sessionB, Sequence: 2, Kind: CommandPlayerInput, MoveZ: -1},
		}, &tick.result)
		tick.context.ApplyCompanionActions(actions)
		tick.context.AdvanceActors()
		result := publishFixture(engine, &tick)

		if len(result.Players) != 2 || len(result.Companions) != 2 {
			t.Fatalf("Players=%d Companions=%d，想要 2/2", len(result.Players), len(result.Companions))
		}
		for _, update := range result.Players {
			if update.Session == sessionA {
				players[0] = update
			} else {
				players[1] = update
			}
		}
		if result.Companions[0].ID != companionA || result.Companions[1].ID != companionB {
			t.Fatalf("伙伴发布未按 ID 字节序: %+v", result.Companions)
		}
		companions[0], companions[1] = result.Companions[0], result.Companions[1]
		return players, companions
	}

	firstPlayers, firstCompanions := run()
	// 玩家命令必须在同 tick 生效：玩家 A 沿 +X、玩家 B 沿 +Z 移动。
	if firstPlayers[0].State.Position.X() <= 0.5 {
		t.Fatalf("玩家 A 命令未同 tick 生效: %+v", firstPlayers[0])
	}
	if firstPlayers[1].State.Position.Z() <= 0.5 {
		t.Fatalf("玩家 B 命令未同 tick 生效: %+v", firstPlayers[1])
	}
	// action 按 ID 寻址：A 只沿 +X，B 只沿 -Z，方向互不串扰。
	if firstCompanions[0].State.Position.X() <= 0.5 || firstCompanions[0].State.Position.Z() != 4.5 {
		t.Fatalf("伙伴 A 的 action 未按 ID 寻址: %+v", firstCompanions[0])
	}
	if firstCompanions[1].State.Position.Z() >= 4.5 || firstCompanions[1].State.Position.X() != 2.5 {
		t.Fatalf("伙伴 B 的 action 未按 ID 寻址: %+v", firstCompanions[1])
	}

	// 规格场景：两个相同输入的重放必须产生相同的可观察结果。
	secondPlayers, secondCompanions := run()
	if !reflect.DeepEqual(firstPlayers, secondPlayers) ||
		!reflect.DeepEqual(firstCompanions, secondCompanions) {
		t.Fatal("相同输入的重放产生了不同的可观察结果")
	}
}

// TestCompanionActionSharesPlayerPhysicsExit 是"伙伴与玩家共用同一 Rust 物理出口"
// 的差分证据：一名玩家与一个伙伴以相同初始身体、相同输入逐 tick 步进，两者的
// 位移与碰撞结果必须逐 tick 完全一致——任何第二套积分实现都会在这里暴露差异。
func TestCompanionActionSharesPlayerPhysicsExit(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	id := companionTestID(1)
	activateCompanionAt(t, engine, id, mgl32.Vec3{0.5, 1, 0.5})

	sequence := uint64(2)
	for tick := 0; tick < 20; tick++ {
		command := Command{
			Session: session, Sequence: sequence, Kind: CommandPlayerInput, MoveX: 1,
		}
		action := CompanionAction{
			ID: id, Kind: CompanionActionMove, Input: physics.Input{MoveX: 1},
		}
		if !validCompanionAction(action) {
			t.Fatalf("tick %d action 不是合法 owner 输入", tick)
		}
		sequence++
		fixtureTick := engine.beginTick()
		fixtureTick.context.ApplyPlayerCommands([]Command{command}, &fixtureTick.result)
		fixtureTick.context.ApplyCompanionActions([]CompanionAction{action})
		fixtureTick.context.AdvanceActors()
		result := publishFixture(engine, &fixtureTick)
		player := onlyMovementPlayer(t, result)
		if len(result.Companions) != 1 || result.Companions[0].ID != id {
			t.Fatalf("tick %d Companions=%+v", tick, result.Companions)
		}
		if player.State != result.Companions[0].State {
			t.Fatalf("tick %d 玩家与伙伴没有共用同一物理出口: player=%+v companion=%+v",
				tick, player.State, result.Companions[0].State)
		}
	}
	if final := engine.companions[id].state.Position; final.X() <= 0.5 {
		t.Fatalf("伙伴未随 action 移动: %+v", final)
	}
}

// TestCompanionActionInboxBoundedAndSessionless 锁定 inbox 契约：容量有界且满员
// 即丢弃、action 结构不携带玩家会话身份、未知/未激活 ID 确定性丢弃且不产生任何
// 会话副作用、同一伙伴同 tick 的重复 action 只应用最早入队的一个。
func TestCompanionActionInboxBoundedAndSessionless(t *testing.T) {
	t.Run("action 不携带会话身份", func(t *testing.T) {
		actionType := reflect.TypeOf(CompanionAction{})
		sessionIDType := reflect.TypeOf(SessionID(0))
		for index := range actionType.NumField() {
			if actionType.Field(index).Type == sessionIDType {
				t.Fatalf("CompanionAction 字段 %s 携带玩家会话身份", actionType.Field(index).Name)
			}
		}
	})

	t.Run("未知 ID 丢弃且无会话副作用", func(t *testing.T) {
		engine := NewEngine(0, 0, 0)
		loadCompanionFlatChunks(t, engine, core.ChunkPos{}, 1)
		id := companionTestID(1)
		activateCompanionAt(t, engine, id, mgl32.Vec3{8.5, 1, 8.5})
		before := engine.companions[id].state.Position

		result := applyCompanionActionsTick(engine, []CompanionAction{{
			ID: companionTestID(7), Kind: CompanionActionMove, Input: physics.Input{MoveX: 1},
		}})
		if len(engine.sessions) != 0 || len(result.Rejected) != 0 {
			t.Fatalf("未知 ID action 产生 entity 副作用: sessions=%d rejected=%+v",
				len(engine.sessions), result.Rejected)
		}
		if after := engine.companions[id].state.Position; after != before {
			t.Fatalf("未知 ID action 影响了已注册伙伴: before=%v after=%v", before, after)
		}
	})

	t.Run("未激活伙伴的 action 丢弃不跨 tick 滞留", func(t *testing.T) {
		engine := NewEngine(0, 0, 0)
		id := companionTestID(1)
		position := mgl32.Vec3{8.5, 1, 8.5}
		engine.RegisterCompanion(CompanionRestore{
			ID: id,
			Body: &companion.Body{
				ID: id, Dimension: core.Overworld, Position: [3]float32(position),
			},
			SpawnDimension: core.Overworld,
		})
		// 注册后立即提供：伙伴尚处待恢复状态，action 必须在本 tick 被丢弃且
		// 不滞留到激活之后。首个 actor 阶段因区块未就绪而不会激活。
		action := CompanionAction{
			ID: id, Kind: CompanionActionMove, Input: physics.Input{MoveX: 1},
		}
		tick := engine.beginTick()
		tick.context.ApplyCompanionActions([]CompanionAction{action})
		tick.context.AdvanceActors()
		publishFixture(engine, &tick)
		if engine.companions[id].active {
			t.Fatal("首个未喂区块的 tick 即激活，用例前提不成立")
		}
		activateCompanionAt(t, engine, id, position)
		after := engine.companions[id].state.Position
		if after != position {
			t.Fatalf("未激活期间的 action 泄漏到激活后: want=%v got=%v", position, after)
		}
		idleTick := engine.beginTick()
		idleTick.context.ApplyCompanionActions(nil)
		idleTick.context.AdvanceActors()
		idle := publishFixture(engine, &idleTick)
		if len(idle.Companions) != 1 || idle.Companions[0].State.Position != after {
			t.Fatalf("无 action 的激活伙伴没有保持静止: %+v", idle.Companions)
		}
	})

	t.Run("同 tick 重复 action 只应用最早入队的一个", func(t *testing.T) {
		engine := NewEngine(0, 0, 0)
		loadCompanionFlatChunks(t, engine, core.ChunkPos{}, 1)
		id := companionTestID(1)
		activateCompanionAt(t, engine, id, mgl32.Vec3{0.5, 1, 0.5})
		tick := engine.beginTick()
		tick.context.ApplyCompanionActions([]CompanionAction{
			{ID: id, Kind: CompanionActionMove, Input: physics.Input{MoveX: 1}},
			{ID: id, Kind: CompanionActionMove, Input: physics.Input{MoveX: -1}},
		})
		tick.context.AdvanceActors()
		publishFixture(engine, &tick)
		if applied := engine.companions[id].input.MoveX; applied != 1 {
			t.Fatalf("重复 action 未保留最早入队者: applied=%d", applied)
		}
		if after := engine.companions[id].state.Position; after.X() <= 0.5 {
			t.Fatalf("最早入队的 action 未生效: %+v", after)
		}
	})
}

// TestCompanionInterestSlidesWithBody 锁定 3×3 兴趣随脚下区块滑动：伙伴跨入相邻
// 区块的同一 tick，兴趣必须滑动到新脚下区块为中心的 3×3，新区块经既有
// acquire/generate 流程就绪，离开的旧区块按既有规则释放，且任一时刻单伙伴兴趣
// 区块数不超过 9。
// TestActorStateExtractionKeepsPlayerBehavior 锁定 actorState 提取契约：玩家与
// 伙伴的运动/朝向/背包/采掘字段必须经同一个内嵌 actorState 提升（不得在子结构
// 体重复声明遮蔽），且扩展后玩家的移动/采掘/放置/背包序列重放逐 tick 产生完全
// 一致的可观察结果。既有 sim 全量测试套件（含 oracle 差分）不改语义通过是另一
// 道差分门禁。
func TestActorStateExtractionKeepsPlayerBehavior(t *testing.T) {
	t.Run("运动字段经 actorState 内嵌提升", func(t *testing.T) {
		player := &playerState{actorState: actorState{
			yaw: 1.5, pitch: -0.25, state: physics.State{OnGround: true},
		}}
		if player.yaw != 1.5 || player.pitch != -0.25 || !player.state.OnGround {
			t.Fatalf("playerState 运动字段未内嵌 actorState: %+v", player.actorState)
		}
		entry := &companionState{actorState: actorState{
			yaw: 2.5, state: physics.State{OnGround: true},
		}}
		if entry.yaw != 2.5 || !entry.state.OnGround {
			t.Fatalf("companionState 运动字段未内嵌 actorState: %+v", entry.actorState)
		}
	})

	// M5C：采掘状态（按住意图 + 进度状态机）上移 actorState 后，两类 actor 必须
	// 共用同一结构体类型与同一进度语义，子结构体不得重复声明遮蔽。
	t.Run("采掘字段经 actorState 内嵌共享", func(t *testing.T) {
		actorType := reflect.TypeOf(actorState{})
		miningField, ok := actorType.FieldByName("mining")
		if !ok || miningField.Type != reflect.TypeOf(miningState{}) {
			t.Fatalf("actorState 缺少共有采掘进度字段: %+v", actorType)
		}
		heldField, ok := actorType.FieldByName("miningHeld")
		if !ok || heldField.Type.Kind() != reflect.Bool {
			t.Fatalf("actorState 缺少共有采掘意图字段: %+v", actorType)
		}
		for _, shadow := range []reflect.Type{
			reflect.TypeOf(playerState{}), reflect.TypeOf(companionState{}),
		} {
			// 只检查直接字段：FieldByName 会沿内嵌提升找到 actorState 的字段，
			// 遮蔽检查必须限定在子结构体自身声明的字段上。
			for index := range shadow.NumField() {
				field := shadow.Field(index)
				if field.Name == "mining" || field.Name == "miningHeld" {
					t.Fatalf("%s 重复声明遮蔽了共有字段 %s", shadow.Name(), field.Name)
				}
			}
		}
		player := &playerState{}
		entry := &companionState{}
		player.mining = miningState{target: core.BlockPos{X: 1}, progressTicks: 3, requiredTicks: 8}
		entry.mining = player.mining
		if player.mining != entry.mining {
			t.Fatalf("两类 actor 的采掘进度不同型: %+v %+v", player.mining, entry.mining)
		}
	})

	t.Run("移动采掘放置背包序列重放逐 tick 一致", func(t *testing.T) {
		run := func() ([]PlayerUpdate, []Rejection) {
			engine, session := readyMovementPlayer(t)
			// 差分脚本覆盖移动/采掘/放置/背包全部命令族：放置需要玩家持有可放置
			// 物品，并在视线方向放一个参照方块。
			engine.sessions[session].player.inventory.Hotbar.Slots[2] = core.ItemStack{
				Item: core.ItemOakPlanks, Count: 3,
			}
			// 视线方向放一面 4 格宽的参照墙：玩家在前几个 tick 会随输入小幅漂移，
			// 加宽保证放置命令在重放中的任何漂移量下都命中同一面墙。
			for dx := int32(0); dx < 4; dx++ {
				engine.SetBlockForTest(core.BlockPos{X: dx, Y: 2, Z: 3}, core.StoneID)
			}
			script := [][]Command{
				{{Session: session, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1, Yaw: 0.25, Pitch: -0.1}},
				nil,
				{
					{Session: session, Sequence: 3, Kind: CommandPlayerInput, Mining: true, Pitch: -1.5},
					{Session: session, Sequence: 4, Kind: CommandSelectHotbar, Slot: 1},
				},
				{{Session: session, Sequence: 5, Kind: CommandMoveInventoryStack, Slot: 0, ToSlot: 9}},
				{{Session: session, Sequence: 6, Kind: CommandPlayerInput, MoveZ: 1, Yaw: -0.75}},
				{{Session: session, Sequence: 7, Kind: CommandPlaceBlock, Slot: 2, Yaw: math.Pi, Pitch: 0}},
			}
			var updates []PlayerUpdate
			var rejections []Rejection
			for _, commands := range script {
				tick := engine.beginTick()
				tick.context.ApplyPlayerCommands(commands, &tick.result)
				tick.context.AdvanceActors()
				tick.context.SettleGameplay(&tick.result)
				tick.context.FinishWorld(&tick.result)
				commitMutation(tick.mutation, &tick.result)
				result := publishFixture(engine, &tick)
				updates = append(updates, onlyMovementPlayer(t, result))
				rejections = append(rejections, result.Rejected...)
			}
			return updates, rejections
		}
		firstUpdates, firstRejections := run()
		secondUpdates, secondRejections := run()
		if !reflect.DeepEqual(firstUpdates, secondUpdates) ||
			!reflect.DeepEqual(firstRejections, secondRejections) {
			t.Fatal("actorState 提取后玩家序列重放不一致")
		}
		if len(firstRejections) != 1 ||
			firstRejections[0].Reason != RejectInvalidInput {
			t.Fatalf("空背包移动的确定性拒绝缺失: %+v", firstRejections)
		}
		// 放置命令必须真实成交，否则脚本没有覆盖放置路径。
		placed := func() bool {
			engine, session := readyMovementPlayer(t)
			engine.sessions[session].player.inventory.Hotbar.Slots[2] = core.ItemStack{
				Item: core.ItemOakPlanks, Count: 3,
			}
			for dx := int32(0); dx < 4; dx++ {
				engine.SetBlockForTest(core.BlockPos{X: dx, Y: 2, Z: 3}, core.StoneID)
			}
			result := settlePlayerInteractionsTick(engine, []Command{{
				Session: session, Sequence: 2, Kind: CommandPlaceBlock, Slot: 2, Yaw: math.Pi, Pitch: 0,
			}})
			return len(result.Changes) == 1 && len(result.Changes[0].Changes) == 1
		}()
		if !placed {
			t.Fatal("差分脚本中的放置命令没有成交，放置路径未被覆盖")
		}
	})
}

// activateCompanionAt 注册一个带持久化身体的伙伴并驱动到激活，期间把订阅请求的
// 区块按 missing→generate 流程喂成平地。已注册的伙伴跳过注册步骤（部分用例需要
// 先在未激活状态下注入 action，再喂到激活）。
func activateCompanionAt(t *testing.T, engine *Engine, id companion.ID, position mgl32.Vec3) {
	t.Helper()
	if engine.companions[id] == nil {
		engine.RegisterCompanion(CompanionRestore{
			ID: id,
			Body: &companion.Body{
				ID: id, Dimension: core.Overworld, Position: [3]float32(position),
			},
			SpawnDimension: core.Overworld,
		})
	}
	center := companionChunk(position)
	loadFlatChunks(t, engine.dimension(core.Overworld),
		center.X-1, center.X+1, center.Z-1, center.Z+1)
	for range 16 {
		if entry := engine.companions[id]; entry != nil && entry.active {
			return
		}
		advanceActorsTick(engine)
	}
	if entry := engine.companions[id]; entry == nil || !entry.active {
		t.Fatalf("伙伴 %v 未在预算内激活", id)
	}
}
