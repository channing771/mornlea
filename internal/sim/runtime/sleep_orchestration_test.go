package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

var (
	sleepBedFoot = core.BlockPos{X: 3, Y: 1, Z: 5}
	sleepBedHead = core.BlockPos{X: 3, Y: 1, Z: 6}
)

func doorTestReadyEngine(t *testing.T, hotbar core.Hotbar) (*Engine, SessionID, core.ChunkPos) {
	t.Helper()
	engine := NewEngine(0, 0, 0)
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 0, 0, 0)
	session := SessionID(1)
	engine.RegisterPlayer(session, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
		Inventory:      core.Inventory{Hotbar: hotbar},
	})
	result := engine.Step()
	player := onlyRuntimePlayerUpdate(t, result, session)
	if !player.Ready {
		t.Fatalf("玩家未在床夹具中激活：%+v", result.Players)
	}
	return engine, session, core.ChunkPos{}
}

func placeSleepBed(
	t *testing.T, engine *Engine, target core.BlockPos, distance float32,
) (SessionID, float32, float32) {
	t.Helper()
	engine.SetBlockForTest(sleepBedFoot, core.BedFootSouthID)
	engine.SetBlockForTest(sleepBedHead, core.BedHeadSouthID)
	session := SessionID(1)
	position := mgl32.Vec3{3.5, 1, float32(sleepBedFoot.Z+1) + distance}
	engine.SetPlayerPositionForTest(session, position)
	eye := position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, target)
	return session, yaw, pitch
}

func interactBed(
	engine *Engine, session SessionID, sequence uint64, yaw, pitch float32,
) TickResult {
	engine.SetWorldTimeForTest(18000)
	engine.Enqueue(Command{
		Session: session, Sequence: sequence, Kind: CommandInteractBed,
		Yaw: yaw, Pitch: pitch,
	})
	return engine.Step()
}

func twoPlayerWorld(t *testing.T) *Engine {
	t.Helper()
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	engine.RegisterSession(2, core.Overworld, core.ChunkPos{})
	for range 8 {
		engine.Step()
		first, firstOK := engine.Player(1)
		second, secondOK := engine.Player(2)
		if firstOK && first.Ready && secondOK && second.Ready {
			return engine
		}
	}
	t.Fatal("双玩家床夹具未激活")
	return engine
}

func sleepWorldTwoPlayers(t *testing.T) (*Engine, SessionID, SessionID, float32, float32, float32, float32) {
	t.Helper()
	engine := twoPlayerWorld(t)
	foot2 := core.BlockPos{X: 7, Y: 1, Z: 5}
	engine.SetBlockForTest(foot2, core.BedFootSouthID)
	engine.SetBlockForTest(core.BlockPos{X: 7, Y: 1, Z: 6}, core.BedHeadSouthID)
	session1, yaw1, pitch1 := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	position2 := mgl32.Vec3{7.5, 1, 9.5}
	engine.SetPlayerPositionForTest(2, position2)
	eye2 := position2.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw2, pitch2 := lookAtBlockCenter(eye2, foot2)
	return engine, session1, 2, yaw1, pitch1, yaw2, pitch2
}

// TestSleepThroughNightJumpsPhaseToDayStart 覆盖 spec 场景「单人即时跳夜」：
// 唯一活跃玩家入睡的同一 tick 结束时，显示相位必须落到周期起点（白昼）、入睡
// 状态清除，且绝对世界时间只按既有节奏推进恰好 1 tick。
func TestSleepThroughNightJumpsPhaseToDayStart(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	engine.SetWorldTimeForTest(18000)
	engine.Enqueue(Command{Session: session, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch})
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	// 入睡命令与跳夜结算在同一个 tick 内完成：相位从周期起点起步。
	if got := engine.DayPhaseOffset(); got != uint16((24000-(18000+1)%24000)%24000) {
		t.Fatalf("跳夜 offset = %d，想要 %d", got, (24000-(18000+1)%24000)%24000)
	}
	if phase := core.DisplayDayPhase(result.WorldTimeTicks, engine.DayPhaseOffset()); phase != 0 {
		t.Fatalf("跳夜后显示相位 = %d，想要 0", phase)
	}
	if result.WorldTimeTicks != 18001 {
		t.Fatalf("跳夜 tick 世界时间 = %d，想要 18001（绝对时间每 tick 恰好 +1）", result.WorldTimeTicks)
	}
	offset := engine.DayPhaseOffset()
	next := engine.Step()
	if engine.DayPhaseOffset() != offset ||
		core.DisplayDayPhase(next.WorldTimeTicks, engine.DayPhaseOffset()) != 1 {
		t.Fatalf("跳夜后仍重复结算入睡：offset=%d next=%+v", engine.DayPhaseOffset(), next)
	}
}

// TestSleepThroughNightPublishesOffsetInSameTick 锁定「offset 变更随下一份权威
// 状态切换生效」的权威半部：跳夜 tick 发布的 `PlayerUpdate` 必须已经携带新偏移
// （跳夜结算先于玩家发布执行），客户端由此在跳夜后的第一份权威状态上立即看到
// 白昼相位；绝对世界时间照常推进，两者互不干扰。
func TestSleepThroughNightPublishesOffsetInSameTick(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	engine.SetWorldTimeForTest(18000)
	engine.Enqueue(Command{Session: session, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch})
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	want := uint16((24000 - (18000+1)%24000) % 24000)
	found := false
	for _, update := range result.Players {
		if update.Session != session {
			continue
		}
		found = true
		if update.DayPhaseOffset != want {
			t.Fatalf("跳夜 tick 发布的偏移 = %d，想要 %d", update.DayPhaseOffset, want)
		}
		if update.WorldTimeTicks != 18001 {
			t.Fatalf("跳夜 tick 发布的世界时间 = %d，想要 18001", update.WorldTimeTicks)
		}
	}
	if !found {
		t.Fatal("跳夜 tick 未发布该玩家的权威状态")
	}

	// 未跳夜的普通 tick 发布既有偏移：偏移不随 tick 推进漂移，客户端持续读到
	// 同一权威单值。
	next := engine.Step()
	for _, update := range next.Players {
		if update.Session == session && update.DayPhaseOffset != want {
			t.Fatalf("后续 tick 发布的偏移 = %d，想要保持 %d", update.DayPhaseOffset, want)
		}
	}
}

// TestSleepThroughNightWaitsForAllActivePlayers 覆盖 spec 场景「有玩家未入睡
// 则不跳」：任一活跃玩家未入睡时，偏移不得变化、已入睡玩家保持入睡；全员入睡
// 后才在 tick 边界完成跳夜。
func TestSleepThroughNightWaitsForAllActivePlayers(t *testing.T) {
	engine, session1, session2, yaw1, pitch1, yaw2, pitch2 := sleepWorldTwoPlayers(t)
	engine.SetWorldTimeForTest(18000)
	// 只有一人入睡：不跳。
	engine.Enqueue(Command{Session: session1, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw1, Pitch: pitch1})
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	if engine.DayPhaseOffset() != 0 {
		t.Fatalf("有玩家未入睡时偏移 = %d，想要 0", engine.DayPhaseOffset())
	}
	// 第二人也入睡：同一 tick 完成跳夜。
	engine.Enqueue(Command{Session: session2, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw2, Pitch: pitch2})
	result = engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("第二人入睡被拒绝: %+v", result.Rejected)
	}
	if phase := core.DisplayDayPhase(result.WorldTimeTicks, engine.DayPhaseOffset()); phase != 0 {
		t.Fatalf("全员入睡后显示相位 = %d，想要 0", phase)
	}
	offset := engine.DayPhaseOffset()
	next := engine.Step()
	if engine.DayPhaseOffset() != offset ||
		core.DisplayDayPhase(next.WorldTimeTicks, engine.DayPhaseOffset()) != 1 {
		t.Fatalf("全员跳夜后仍重复结算入睡：offset=%d next=%+v", engine.DayPhaseOffset(), next)
	}
}

// TestSleepThroughNightOverwritesPreviousOffset 锁定「再次入睡覆盖旧 offset」：
// 第一次跳夜的偏移被第二次跳夜按当期绝对时间重算覆盖，不叠加也不保留旧值。
// 第二次入睡的时刻必须把旧偏移计入显示相位（相位 = (worldTime + 旧offset)
// % 24000），否则入睡会被旧偏移推出的白昼拒绝。
func TestSleepThroughNightOverwritesPreviousOffset(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("第一次入睡被拒绝: %+v", result.Rejected)
	}
	first := engine.DayPhaseOffset()
	if first == 0 {
		t.Fatal("前置失败：第一次跳夜应有非零偏移")
	}

	// 选第二次入睡时刻 36001：第一次跳夜后偏移为 (24000−18001)=5999，
	// (36001%24000 + 5999) % 24000 = 18000，恰好落在夜间窗内。
	engine.SetWorldTimeForTest(36001)
	engine.Enqueue(Command{Session: session, Sequence: 11, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch})
	if result := engine.Step(); len(result.Rejected) != 0 {
		t.Fatalf("第二次入睡被拒绝: %+v", result.Rejected)
	}
	second := engine.DayPhaseOffset()
	// 本 tick 完成后绝对时间 36002，offset = (24000 − 36002%24000) % 24000。
	want := uint16((24000 - 36002%24000) % 24000)
	if second != want {
		t.Fatalf("第二次跳夜 offset = %d，想要 %d", second, want)
	}
	if second == first {
		t.Fatal("第二次跳夜应覆盖旧 offset，而不是保留旧值")
	}
}

// TestSleepThroughNightKeepsCropPaceIdentical 覆盖 spec 场景「跳夜不加速模拟」
// 的对拍：两个种子、地形、夹具与绝对时间序列完全一致的世界，唯一差别是跳夜
// （A 跳、B 不跳）。跳夜后以相同绝对 tick 推进，两世界的区块逐位一致（含作物
// 生长与水源格），且绝对时间节奏相同；同时以「B 的作物确实推进过」排除对拍的
// 空转假绿。
func TestSleepThroughNightKeepsCropPaceIdentical(t *testing.T) {
	build := func(t *testing.T, withSleep bool) *Engine {
		engine, session := readyCropWorld(t)
		applyCropFixture(t, engine, cropFixture{
			farmland:      core.FarmlandWetID,
			crop:          core.WheatStage0ID,
			waterDistance: 2,
			waterDY:       0,
		})
		engine.SetWorldTimeForTest(18000)
		// 作物夹具的草皮在 y=1（门夹具在 y=0），床与玩家整体抬一层：床尾
		// (3,2,5)、床头 (3,2,6)，玩家站在 y=2 顶面。
		bedFoot := core.BlockPos{X: 3, Y: 2, Z: 5}
		engine.SetBlockForTest(bedFoot, core.BedFootSouthID)
		engine.SetBlockForTest(core.BlockPos{X: 3, Y: 2, Z: 6}, core.BedHeadSouthID)
		// 玩家拨到床边：withSleep 为真的世界入睡并跳夜，为假的世界保持醒着
		//（其余输入完全一致）。
		position := mgl32.Vec3{3.5, 2, 9.5}
		engine.SetPlayerPositionForTest(session, position)
		eye := position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		yaw, pitch := lookAtBlockCenter(eye, bedFoot)
		if withSleep {
			engine.Enqueue(Command{Session: session, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch})
		} else {
			engine.Enqueue(Command{Session: session, Sequence: 10, Kind: CommandPlayerInput, Yaw: yaw, Pitch: pitch})
		}
		engine.Step()
		return engine
	}
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })

	withJump := build(t, true)
	withoutJump := build(t, false)
	if withJump.DayPhaseOffset() == 0 {
		t.Fatal("前置失败：跳夜世界应有非零偏移")
	}
	if withoutJump.DayPhaseOffset() != 0 {
		t.Fatal("前置失败：对照世界应无偏移")
	}

	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	for range cropFixtureTicks {
		a := withJump.Step()
		b := withoutJump.Step()
		if a.WorldTimeTicks != b.WorldTimeTicks {
			t.Fatalf("绝对时间分岔：跳夜 %d vs 对照 %d", a.WorldTimeTicks, b.WorldTimeTicks)
		}
	}
	hashA, _, okA := withJump.ChunkHash(key)
	hashB, _, okB := withoutJump.ChunkHash(key)
	if !okA || !okB {
		t.Fatal("对拍区块未就绪")
	}
	if hashA != hashB {
		t.Fatal("跳夜与未跳夜世界的区块逐位不一致：跳夜影响了绝对时间驱动的模拟")
	}
	// 非空转检查：对照世界的作物在窗口内确实推进过，否则逐位一致可能是双方
	// 都没发生任何模拟。
	if got := cropBlockAt(t, withoutJump, cropFixtureCrop); got == core.WheatStage0ID {
		t.Fatalf("%d 个 tick 后对照世界作物毫无推进，对拍空转", cropFixtureTicks)
	}
}
