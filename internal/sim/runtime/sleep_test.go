package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// sleep_test.go：入睡、跳夜与个人重生点在权威 sim 层的行为。夜间窗 13000..23000
// 经 `core.DisplayDayPhase` 读取引擎 `dayPhaseOffset` 后判定，与夜行者行共享同一
// 份夜间定义；重生点是每玩家内存态（持久化由存档批次承接）。

// sleepBedFoot / sleepBedHead 是睡眠用例的床位置：床尾 (3,1,5)、南向床头在其
// +Z 邻格 (3,1,6)。世界夹具地面顶面为 y=0 草皮，y=1 全空气（除 (0,2,5) 石头，
// 用例已避开该列），草皮满足 `isSolidSupport`，无需另设支撑。
var (
	sleepBedFoot = core.BlockPos{X: 3, Y: 1, Z: 5}
	sleepBedHead = core.BlockPos{X: 3, Y: 1, Z: 6}
)

// placeSleepBed 在夹具世界里放置一张南向床并让玩家站在床尾南侧数格、瞄准指定
// 半格的中心。返回会话与瞄准角；distance 控制玩家与床尾的 Z 向间距（保持可达
// 且不被床自身格挡）。
func placeSleepBed(
	t *testing.T, engine *Engine, target core.BlockPos, distance float32,
) (SessionID, float32, float32) {
	t.Helper()
	engine.SetBlockForTest(sleepBedFoot, core.BedFootSouthID)
	engine.SetBlockForTest(sleepBedHead, core.BedHeadSouthID)
	session := SessionID(1)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{3.5, 1, float32(sleepBedFoot.Z+1) + distance}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, target)
	return session, yaw, pitch
}

// interactBed 发一条床交互命令并推进一个权威 tick。
func interactBed(
	engine *Engine, session SessionID, sequence uint64, yaw, pitch float32,
) TickResult {
	engine.SetWorldTimeForTest(18000)
	engine.Enqueue(Command{Session: session, Sequence: sequence, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch})
	return engine.Step()
}

// TestBedInteractAtNightSleepsAndRecordsFootRespawn 覆盖 spec 场景「夜间入睡」：
// 显示相位在 13000..23000（含两端边界）时对床尾或床头右键都进入入睡状态，并把
// 个人重生点记录为该床床尾格；交互不产生任何方块变更。用双活跃玩家夹具观察
// 入睡中间状态（另一人醒着，跳夜不触发，偏移保持 0）。
func TestBedInteractAtNightSleepsAndRecordsFootRespawn(t *testing.T) {
	for _, target := range []core.BlockPos{sleepBedFoot, sleepBedHead} {
		engine := twoPlayerWorld(t)
		session, yaw, pitch := placeSleepBed(t, engine, target, 3.5)
		result := interactBed(engine, session, 10, yaw, pitch)
		if len(result.Rejected) != 0 {
			t.Fatalf("瞄准 %+v 的夜间入睡被拒绝: %+v", target, result.Rejected)
		}
		if len(result.Changes) != 0 {
			t.Fatalf("入睡不应产生方块变更: %+v", result.Changes)
		}
		player := engine.sessions[session].player
		if !player.sleeping {
			t.Fatalf("瞄准 %+v 后玩家应处于入睡状态", target)
		}
		if engine.DayPhaseOffset() != 0 {
			t.Fatalf("有玩家未入睡时偏移 = %d，想要 0", engine.DayPhaseOffset())
		}
		if !player.respawnPresent || player.respawnPos != sleepBedFoot ||
			player.respawnDim != core.Overworld {
			t.Fatalf("瞄准 %+v 后重生点 = (present %v, %+v, %d)，想要床尾 %+v",
				target, player.respawnPresent, player.respawnPos, player.respawnDim, sleepBedFoot)
		}
	}
}

// TestBedInteractOutsideNightWindowRejected 覆盖 spec 场景「白天使用被拒绝」：
// 相位不在夜间窗时使用床必须被拒绝（沿用既有冻结拒绝枚举，不新增 wire 值），
// 入睡状态与重生点都保持原样。
func TestBedInteractOutsideNightWindowRejected(t *testing.T) {
	for _, phase := range []uint64{0, 12999, 23001, 23999} {
		engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
		session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
		player := engine.sessions[session].player
		// 预置一个哨兵重生点，锁定「拒绝不改重生点」。
		player.respawnPresent = true
		player.respawnPos = core.BlockPos{X: 9, Y: 8, Z: 7}
		player.respawnDim = core.Overworld

		engine.SetWorldTimeForTest(phase)
		engine.Enqueue(Command{Session: session, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch})
		result := engine.Step()
		if len(result.Rejected) != 1 {
			t.Fatalf("相位 %d 使用床应被拒绝: %+v", phase, result.Rejected)
		}
		if player.sleeping {
			t.Fatalf("相位 %d 使用床被拒后玩家不应入睡", phase)
		}
		if !player.respawnPresent || player.respawnPos != (core.BlockPos{X: 9, Y: 8, Z: 7}) {
			t.Fatalf("相位 %d 拒绝后重生点被改动: (present %v, %+v)",
				phase, player.respawnPresent, player.respawnPos)
		}
	}
}

// TestBedInteractNonBedTargetIsSilentNoop 锁定与门交互同构的无效目标语义：
// 床交互命令瞄准非床方块时静默成功（零拒绝、零状态变化），不拒绝也不入睡。
func TestBedInteractNonBedTargetIsSilentNoop(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	session := SessionID(1)
	player := engine.sessions[session].player
	player.state.Position = mgl32.Vec3{0.5, 1, 8.5}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	// 瞄准夹具里 (0,2,5) 的石头：不是床，交互应为 no-op。
	yaw, pitch := lookAtBlockCenter(eye, core.BlockPos{X: 0, Y: 2, Z: 5})
	result := interactBed(engine, session, 10, yaw, pitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("非床目标不应拒绝: %+v", result.Rejected)
	}
	if player.sleeping || player.respawnPresent {
		t.Fatal("非床目标不应产生入睡状态或重生点")
	}
}

// TestMovementInputCancelsSleepingKeepsRespawnPoint 覆盖 spec 场景「移动取消
// 入睡但保留重生点」：带移动分量的有效输入清掉入睡位，重生点仍指向床尾格；
// 只转头（无移动分量）的中性输入不清入睡位。
func TestMovementInputCancelsSleepingKeepsRespawnPoint(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if !player.sleeping {
		t.Fatal("前置失败：玩家应已入睡")
	}

	// 中性输入（仅转头）：入睡保持。
	engine.Enqueue(Command{Session: session, Sequence: 11, Kind: CommandPlayerInput, Yaw: yaw + 0.5, Pitch: pitch})
	engine.Step()
	if !player.sleeping {
		t.Fatal("仅转头不应取消入睡")
	}

	// 移动输入：取消入睡，重生点保留。
	engine.Enqueue(Command{Session: session, Sequence: 12, Kind: CommandPlayerInput, MoveX: 1, Yaw: yaw, Pitch: pitch})
	engine.Step()
	if player.sleeping {
		t.Fatal("移动输入应取消入睡")
	}
	if !player.respawnPresent || player.respawnPos != sleepBedFoot {
		t.Fatalf("取消入睡后重生点丢失: (present %v, %+v)", player.respawnPresent, player.respawnPos)
	}
}

// TestDamageCancelsSleepingKeepsRespawnPoint 锁定受击取消：`applyDamage` 是全部
// 伤害来源共用的唯一结算入口，真正挨一下（非正伤害是 no-op）必须清掉入睡位并
// 保留重生点。
func TestDamageCancelsSleepingKeepsRespawnPoint(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player

	// 非正伤害是 no-op（摔落曲线在安全高度产出负值），不应惊醒玩家。
	player.applyDamage(0)
	if !player.sleeping {
		t.Fatal("非正伤害不应取消入睡")
	}

	player.applyDamage(2)
	if player.sleeping {
		t.Fatal("受击应取消入睡")
	}
	if !player.respawnPresent || player.respawnPos != sleepBedFoot {
		t.Fatalf("受击后重生点丢失: (present %v, %+v)", player.respawnPresent, player.respawnPos)
	}
}

// —— 全员入睡跳夜 ——

// twoPlayerWorld 构造双活跃玩家夹具（无床）：两名玩家都在唯一区块内激活。
// 需要观察「入睡」这一中间状态的用例必须用它而不是单人世界——单人世界里
// 入睡位在同一个 tick 内就会被跳夜结算清掉，无从断言。
func twoPlayerWorld(t *testing.T) *Engine {
	t.Helper()
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	// 第二名玩家复用同一锚点列区块：区块已就绪，出生扫描即刻激活。
	engine.RegisterSession(2, core.Overworld, core.ChunkPos{})
	for range 8 {
		engine.Step()
	}
	for session := SessionID(1); session <= 2; session++ {
		if player, ok := engine.Player(session); !ok || !player.Ready {
			t.Fatalf("会话 %d 未激活: %+v", session, player)
		}
	}
	return engine
}

// sleepWorldTwoPlayers 在双活跃玩家夹具上放两张南向床并让两人各自瞄准床尾，
// 返回两人会话与瞄准角。床 2 在床 1 同一 Z 行东侧，两人站位互不遮挡。
func sleepWorldTwoPlayers(t *testing.T) (*Engine, SessionID, SessionID, float32, float32, float32, float32) {
	t.Helper()
	engine := twoPlayerWorld(t)
	foot2 := core.BlockPos{X: 7, Y: 1, Z: 5}
	engine.SetBlockForTest(foot2, core.BedFootSouthID)
	engine.SetBlockForTest(core.BlockPos{X: 7, Y: 1, Z: 6}, core.BedHeadSouthID)

	session1, yaw1, pitch1 := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	player2 := engine.sessions[2].player
	player2.state.Position = mgl32.Vec3{7.5, 1, 9.5}
	eye2 := player2.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
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
	if engine.sessions[session].player.sleeping {
		t.Fatal("跳夜后入睡状态应被清除")
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
	if !engine.sessions[session1].player.sleeping {
		t.Fatal("已入睡玩家应保持入睡")
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
	for _, session := range []SessionID{session1, session2} {
		if engine.sessions[session].player.sleeping {
			t.Fatalf("会话 %d 跳夜后应清醒", session)
		}
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
		player := engine.sessions[session].player
		player.state.Position = mgl32.Vec3{3.5, 2, 9.5}
		eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
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

// —— 死亡重生：个人重生点延迟校验 ——

// respawnWhenDead 把会话玩家打到 0 血并推进权威 tick，直到其重新激活；返回
// 重生完成后的玩家更新。
func respawnWhenDead(t *testing.T, engine *Engine, session SessionID) PlayerUpdate {
	t.Helper()
	engine.sessions[session].player.applyDamage(int32(core.MaxHealth))
	engine.Step() // 死亡结算 tick：settleDeaths 落位、转入待重生。
	for range 8 {
		engine.Step()
		if player, ok := engine.Player(session); ok && player.Ready {
			return player
		}
	}
	t.Fatal("死亡后玩家未能在预期 tick 内重生激活")
	return PlayerUpdate{}
}

// TestDeathRespawnsAtBedFootWhenBedIntact 覆盖 spec 场景「床完好时重生在床尾」：
// 重生点两格仍为同一张床时，死亡重生回到床尾格（站立在床顶面），生命与饥饿按
// 既有重生规则恢复。
func TestDeathRespawnsAtBedFootWhenBedIntact(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	player := respawnWhenDead(t, engine, session)
	pos := player.State.Position
	if float32(sleepBedFoot.X)+0.5 != pos.X() || float32(sleepBedFoot.Z)+0.5 != pos.Z() {
		t.Fatalf("重生位置 = %+v，想要床尾格中心 (%v, %v)", pos, float32(sleepBedFoot.X)+0.5, float32(sleepBedFoot.Z)+0.5)
	}
	// 站在床顶面：脚底高度为床尾格 + 9/16，而不是格底或上方一格。
	if top := float32(sleepBedFoot.Y) + 0.5625; pos.Y() < top-0.001 || pos.Y() > top+0.001 {
		t.Fatalf("重生脚底高度 = %v，想要床顶面 %v", pos.Y(), top)
	}
	if !player.Ready || player.Health != core.MaxHealth {
		t.Fatalf("重生后生命值 = %d，想要 %d", player.Health, core.MaxHealth)
	}
	if got := engine.sessions[session].player.hunger; got != core.MaxHunger {
		t.Fatalf("重生后饥饿 = %d，想要固定初值 %d", got, core.MaxHunger)
	}
	if !engine.sessions[session].player.respawnPresent {
		t.Fatal("床完好时重生点记录不应被清除")
	}
}

// TestDeathFallsBackToAnchorWhenBedMined 覆盖 spec 场景「床被破坏后回落出生
// 锚点」：床被采掘（两格皆空）后死亡，重生位置不再指向床，且重生点记录被清除。
func TestDeathFallsBackToAnchorWhenBedMined(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	engine.SetBlockForTest(sleepBedFoot, core.AirID)
	engine.SetBlockForTest(sleepBedHead, core.AirID)

	player := respawnWhenDead(t, engine, session)
	pos := player.State.Position
	if pos.X() >= float32(sleepBedFoot.X) && pos.X() < float32(sleepBedFoot.X+1) &&
		pos.Z() >= float32(sleepBedFoot.Z) && pos.Z() < float32(sleepBedFoot.Z+1) {
		t.Fatalf("床已采掘后重生位置仍为床尾格: %+v", pos)
	}
	if engine.sessions[session].player.respawnPresent {
		t.Fatal("床已破坏，重生点记录应被清除")
	}
}

// TestDeathFallsBackWhenBedHalfMissing 锁定半破坏边界：只拆床头（床尾残留）
// 同样判「两格不再同属一床」，重生回落锚点并清记录，世界不残留对半床的重生。
func TestDeathFallsBackWhenBedHalfMissing(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	engine.SetBlockForTest(sleepBedHead, core.AirID)

	player := respawnWhenDead(t, engine, session)
	if pos := player.State.Position; pos.Z() >= float32(sleepBedFoot.Z) && pos.Z() < float32(sleepBedFoot.Z+1) &&
		pos.X() >= float32(sleepBedFoot.X) && pos.X() < float32(sleepBedFoot.X+1) {
		t.Fatalf("半破坏床不应再作为重生点: %+v", pos)
	}
	if engine.sessions[session].player.respawnPresent {
		t.Fatal("半破坏床的重生点记录应被清除")
	}
}

// TestDeathFallsBackWhenBedSupportSwept 锁定支撑失效路径：床下支撑被真实采掘
// 后由既有的支撑失效复核当 tick 整床清除（并掉落），其后的死亡重生等价于「床
// 已不存在」——回落锚点并清记录（D3：无需事件式清除，延迟校验自然覆盖）。
// 支撑移除必须走生产写路径（真实采掘），`SetBlockForTest` 不汇入 pending、
// 不会触发支撑复核。
func TestDeathFallsBackWhenBedSupportSwept(t *testing.T) {
	engine := twoPlayerWorld(t)
	// 抬高一层的床：床尾 (3,2,5)、床头 (3,2,6)，正下方各一块泥土支柱（泥土
	// 徒手 5 tick，与床支撑失效先例同一几何）。
	foot := core.BlockPos{X: 3, Y: 2, Z: 5}
	head := core.BlockPos{X: 3, Y: 2, Z: 6}
	headSupport := core.BlockPos{X: 3, Y: 1, Z: 6}
	engine.SetBlockForTest(core.BlockPos{X: 3, Y: 1, Z: 5}, core.DirtID)
	engine.SetBlockForTest(headSupport, core.DirtID)
	engine.SetBlockForTest(foot, core.BedFootSouthID)
	engine.SetBlockForTest(head, core.BedHeadSouthID)

	// 玩家 1 夜间入睡：重生点记录床尾格。
	session1, yaw, pitch := func() (SessionID, float32, float32) {
		player := engine.sessions[1].player
		player.state.Position = mgl32.Vec3{3.5, 1, 9.5}
		eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		y, p := lookAtBlockCenter(eye, head)
		return 1, y, p
	}()
	if result := interactBed(engine, session1, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}

	// 玩家 2 从东侧平视采掘床头支柱：射线在命中支柱前不穿过抬高一层的床。
	player2 := engine.sessions[2].player
	player2.state.Position = mgl32.Vec3{5.5, 1, 7.5}
	eye2 := player2.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw2, pitch2 := lookAtBlockCenter(eye2, headSupport)
	player2.yaw = yaw2
	player2.pitch = pitch2
	player2.miningHeld = true
	var result TickResult
	for range 5 {
		result = engine.Step()
	}
	if len(result.Rejected) != 0 {
		t.Fatalf("采掘被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, foot); got != core.AirID {
		t.Fatalf("支撑失效后床尾应被扫除，实际 %d", got)
	}
	if got := tillBlockAt(t, engine, head); got != core.AirID {
		t.Fatalf("支撑失效后床头应被扫除，实际 %d", got)
	}

	respawnWhenDead(t, engine, session1)
	if engine.sessions[session1].player.respawnPresent {
		t.Fatal("支撑失效清除后重生点记录应被清除")
	}
	if pos := engine.sessions[session1].player.state.Position; pos.X() >= 3 && pos.X() < 4 &&
		pos.Z() >= 5 && pos.Z() < 6 && pos.Y() >= 2 && pos.Y() < 3 {
		t.Fatalf("支撑失效清除后不应重生在床尾: %+v", pos)
	}
}

// TestRespawnPointsIndependentAcrossPlayers 覆盖 spec 场景「重生点互不影响」：
// 两名玩家各自睡不同的床，其中一张床被破坏且其主人死亡重生——该玩家回落锚点
// 并清记录，另一名玩家的重生点必须原样保留，且仍能重生在自己的床尾格。
func TestRespawnPointsIndependentAcrossPlayers(t *testing.T) {
	engine, session1, session2, yaw1, pitch1, yaw2, pitch2 := sleepWorldTwoPlayers(t)
	engine.SetWorldTimeForTest(18000)
	engine.Enqueue(Command{Session: session1, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw1, Pitch: pitch1})
	engine.Enqueue(Command{Session: session2, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw2, Pitch: pitch2})
	if result := engine.Step(); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}

	// 破坏玩家 1 的床并让其死亡重生。
	engine.SetBlockForTest(sleepBedFoot, core.AirID)
	engine.SetBlockForTest(sleepBedHead, core.AirID)
	respawnWhenDead(t, engine, session1)
	if engine.sessions[session1].player.respawnPresent {
		t.Fatal("床 1 已破坏，玩家 1 的重生点应被清除")
	}
	// 玩家 2 的重生点不受影响，死亡后仍回到自己的床尾格。
	player2 := engine.sessions[session2].player
	if !player2.respawnPresent || player2.respawnPos.X != 7 || player2.respawnPos.Z != 5 {
		t.Fatalf("玩家 2 重生点被波及: (present %v, %+v)", player2.respawnPresent, player2.respawnPos)
	}
	update := respawnWhenDead(t, engine, session2)
	if pos := update.State.Position; pos.X() != 7.5 || pos.Z() != 5.5 {
		t.Fatalf("玩家 2 应重生在自己的床尾格，实际 %+v", pos)
	}
}

// TestDeathWithUnverifiedRespawnKeepsRecord 锁定「未验证不等于失效」的边界：
// 重生点指向的区块未加载时，本次死亡无法证明床已损坏——重生回落锚点（重生
// 不得因等待远处区块而停摆），但记录保留给下一次死亡再验。
func TestDeathWithUnverifiedRespawnKeepsRecord(t *testing.T) {
	engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	session := SessionID(1)
	player := engine.sessions[session].player
	// 直接把重生点指向未加载的远处区块（床从未在世界里存在过）。
	player.respawnPresent = true
	player.respawnPos = core.BlockPos{X: 40, Y: 1, Z: 40}
	player.respawnDim = core.Overworld

	update := respawnWhenDead(t, engine, session)
	if pos := update.State.Position; pos.X() >= 40 && pos.X() < 41 && pos.Z() >= 40 && pos.Z() < 41 {
		t.Fatalf("未验证的重生点不应直接用于重生: %+v", pos)
	}
	if !engine.sessions[session].player.respawnPresent {
		t.Fatal("未验证失效的重生点记录不应被清除")
	}
}
