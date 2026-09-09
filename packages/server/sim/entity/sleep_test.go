package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
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

// interactBed 只运行床交互所需的玩家命令与玩法结算阶段。
func interactBed(
	engine *Engine, session SessionID, sequence uint64, yaw, pitch float32,
) TickResult {
	engine.SetWorldTimeForTest(18000)
	return settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: sequence, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch,
	}})
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
// 季节化相位不在夜间窗时使用床必须被拒绝（沿用既有冻结拒绝枚举，不新增 wire
// 值），入睡状态与重生点都保持原样。季节偏移钉在分点（探测 tick 的年相位恰为
// 0，昼弧恒 12000、warp 恒等），使 12999/23001 的边界断言保持未季节化语义——
// 分点下行为不变是季节 warp 的恒等锚。
func TestBedInteractOutsideNightWindowRejected(t *testing.T) {
	for _, phase := range []uint64{0, 12999, 23001, 23999} {
		engine, _, _ := doorTestReadyEngine(t, core.Hotbar{})
		session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
		player := engine.sessions[session].player
		// 预置一个哨兵重生点，锁定「拒绝不改重生点」。
		player.respawnPresent = true
		player.respawnPos = core.BlockPos{X: 9, Y: 8, Z: 7}
		player.respawnDim = core.Overworld
		// 分点对齐：yearIndex(探测 tick) ≡ 0 ⇒ 昼弧 12000 ⇒ 季节化相位恒等
		// 线性相位。
		engine.State.seasonOffset = (core.YearTicks - phase%core.YearTicks) % core.YearTicks

		engine.SetWorldTimeForTest(phase)
		result := settlePlayerInteractionsTick(engine, []Command{{
			Session: session, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch,
		}})
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

// TestBedNightFollowsSeasonalEffectivePhase 锁定判夜消费季节化相位：同一线性
// 时刻（线性相位 12000）在分点与冬季给出相反判定——分点（昼弧 12000）下
// 季节化相位 12000 未入夜、入睡被拒；冬季（昼弧 8400）下同一时刻的季节化相位
// 已被压进夜窗、入睡必须接受。两组夹具只差季节偏移，能区分经/不经季节 warp
// 的判定。
func TestBedNightFollowsSeasonalEffectivePhase(t *testing.T) {
	const linearNoonDusk uint64 = 60000 // 线性相位 = 60000 % 24000 = 12000。
	// 分点对齐（yearIndex=144000）与冬至对齐（yearIndex=216000）。
	for _, tc := range []struct {
		seasonOffset uint64
		wantRejected bool
	}{
		{seasonOffset: 144000 - linearNoonDusk%core.YearTicks, wantRejected: true},
		{seasonOffset: 216000 - linearNoonDusk%core.YearTicks, wantRejected: false},
	} {
		engine := twoPlayerWorld(t)
		session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
		engine.State.seasonOffset = tc.seasonOffset
		engine.SetWorldTimeForTest(linearNoonDusk)
		result := settlePlayerInteractionsTick(engine, []Command{{
			Session: session, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw, Pitch: pitch,
		}})
		if tc.wantRejected {
			if len(result.Rejected) != 1 {
				t.Fatalf("分点相位 12000 未入夜，入睡应被拒绝: %+v", result.Rejected)
			}
			continue
		}
		if len(result.Rejected) != 0 {
			t.Fatalf("冬季线性相位 12000 的季节化相位已入夜，入睡被拒: %+v", result.Rejected)
		}
		if !engine.sessions[session].player.sleeping {
			t.Fatal("冬季季节化夜间入睡后入睡位未置位")
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
	applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 11, Kind: CommandPlayerInput, Yaw: yaw + 0.5, Pitch: pitch,
	}})
	if !player.sleeping {
		t.Fatal("仅转头不应取消入睡")
	}

	// 移动输入：取消入睡，重生点保留。
	applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 12, Kind: CommandPlayerInput, MoveX: 1, Yaw: yaw, Pitch: pitch,
	}})
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
		advanceActorsTick(engine)
	}
	for session := SessionID(1); session <= 2; session++ {
		if player, ok := engine.Player(session, core.WeatherClear); !ok || !player.Ready {
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

// TestSleepThroughNightLandsOnSeasonalMorning 覆盖 spec 场景「跳夜落在当前季节
// 的早晨」：冬季（昼弧最短）全员入睡跳夜后，季节化显示相位必须落在冬季昼弧
// 的早晨段起点，绝对世界时间不被回写（仍由权威 tick 每 tick 恰好 +1 推进）。
// 冬季是昼弧与分点差最大的季节，反解必须经 `core.EffectiveMorningOffset` 的
// 季节化算式完成。
func TestSleepThroughNightLandsOnSeasonalMorning(t *testing.T) {
	engine := twoPlayerWorld(t)
	for _, id := range []SessionID{1, 2} {
		engine.sessions[id].player.sleeping = true
	}
	const settleWorldTime uint64 = 18000
	// 落点时刻 completed=18001 钉在冬至（yearIndex=216000，昼弧 8400）。
	engine.State.seasonOffset = 216000 + core.YearTicks - (settleWorldTime+1)%core.YearTicks
	engine.SetWorldTimeForTest(settleWorldTime)
	engine.settleSleepThroughNight()

	arc := core.DayArcTicks(core.YearPhaseAt(settleWorldTime+1, engine.seasonOffset))
	if arc >= core.DayLengthTicks/2 {
		t.Fatalf("前置失败：冬季昼弧 %d 不短于分点 12000", arc)
	}
	if got := core.EffectiveDayPhaseAt(settleWorldTime+1, engine.DayPhaseOffset(), engine.seasonOffset); got != 0 {
		t.Fatalf("冬季跳夜后的季节化相位 = %d，想要落在早晨段起点 0", got)
	}
	if got := engine.worldTime.Load(); got != settleWorldTime {
		t.Fatalf("跳夜回写了绝对世界时间：%d，想要保持 %d", got, settleWorldTime)
	}
	for _, id := range []SessionID{1, 2} {
		if engine.sessions[id].player.sleeping {
			t.Fatalf("会话 %d 跳夜后入睡位未清除", id)
		}
	}
}

// —— 死亡重生：个人重生点延迟校验 ——

// respawnWhenDead 把会话玩家打到 0 血并推进权威 tick，直到其重新激活；返回
// 重生完成后的玩家更新。
func respawnWhenDead(t *testing.T, engine *Engine, session SessionID) PlayerUpdate {
	t.Helper()
	engine.sessions[session].player.applyDamage(int32(core.MaxHealth))
	advanceHostilesTick(engine, nil) // 死亡阶段：`settleDeaths` 落位、转入待重生。
	for range 8 {
		advanceActorsTick(engine)
		if player, ok := engine.Player(session, core.WeatherClear); ok && player.Ready {
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
		tick := engine.beginTick()
		tick.context.FinishWorld(&tick.result)
		engine.realm.SweepUnsupportedBeds(tick.mutation)
		commitMutation(tick.mutation, &tick.result)
		result = publishFixture(engine, &tick)
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
	if result := settlePlayerInteractionsTick(engine, []Command{
		{Session: session1, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw1, Pitch: pitch1},
		{Session: session2, Sequence: 10, Kind: CommandInteractBed, Yaw: yaw2, Pitch: pitch2},
	}); len(result.Rejected) != 0 {
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

// TestDeathCrossDimensionBedFallsBackToDeathDimensionAnchor 覆盖多维 delta
// 规约「跨维床失效回落本维锚点」：重生点床在主世界、死亡时位于 depths 时，
// 延迟校验不得跨维复用那张床——回落死亡所在维的出生锚点，且主世界床记录保留
// 到返回后仍可用。
func TestDeathCrossDimensionBedFallsBackToDeathDimensionAnchor(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}
	// 搬运到 depths：模拟传送落位后的权威状态——会话维度与出生锚点均为新维，
	// 个人重生点仍指向主世界的床。候选列按新锚点重建：生产侧传送经注销重建，
	// `RegisterPlayer` 本来就会做这件事，这里手动搬运才需显式重算。
	depthsAnchor := core.ChunkPos{X: 8, Z: -8}
	loadFlatChunks(t, engine.realm.EnsureDimension(core.Depths), 7, 9, -9, -7)
	engine.sessions[session].dimension = core.Depths
	moved := engine.sessions[session].player
	moved.anchor = depthsAnchor
	moved.candidates = spawnCandidates(depthsAnchor, engine.tunables.SpawnRadius)
	moved.candidateChunks = spawnCandidateChunks(moved.candidates)
	moved.spawnWanted = map[core.ChunkPos]struct{}{depthsAnchor: {}}

	player := respawnWhenDead(t, engine, session)
	if player.Dimension != core.Depths {
		t.Fatalf("跨维死亡后维度 = %d，想要留在 depths", player.Dimension)
	}
	foot := (core.BlockPos{
		X: int32(player.State.Position.X()),
		Z: int32(player.State.Position.Z()),
	}).Chunk()
	if foot != depthsAnchor {
		t.Fatalf("跨维死亡后落点区块 = %+v，想要 depths 锚点 %+v", foot, depthsAnchor)
	}
	if !engine.sessions[session].player.respawnPresent {
		t.Fatal("跨维回落不得清除主世界床记录，返回后仍须可用")
	}
}
