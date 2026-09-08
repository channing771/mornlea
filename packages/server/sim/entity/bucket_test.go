package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

// readyBucketPlayer 构造一名握着指定物品、瞄准共用目标格的玩家：取水用
// `lookAtBlockCenter` 直瞄目标格中心，放水用 `lookAtBlockTop` 瞄目标格正下方
// 草块顶面（穿水命中 + 贴面落点，落点恰好回到目标格）。
func readyBucketPlayer(
	t *testing.T,
	held core.ItemStack,
	target core.BlockID,
	place bool,
) (*Engine, SessionID, float32, float32) {
	t.Helper()
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(tillTarget, target)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = held
	player.inventory.Hotbar.Selected = 0
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	if place {
		below := tillTarget
		below.Y--
		yaw, pitch := lookAtBlockTop(eye, below)
		return engine, session, yaw, pitch
	}
	yaw, pitch := lookAtBlockCenter(eye, tillTarget)
	return engine, session, yaw, pitch
}

func collectBucket(engine *Engine, session SessionID, yaw, pitch float32) TickResult {
	return settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandCollectWater, Yaw: yaw, Pitch: pitch,
	}})
}

func placeBucket(engine *Engine, session SessionID, yaw, pitch float32) TickResult {
	return settlePlayerInteractionsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlaceWater, Yaw: yaw, Pitch: pitch,
	}})
}

// bucketMoistureTick 与翻地 helper 同形：命令收集、交互结算之后推进湿度重判，
// 再提交发布——放水变湿/取水变干必须在同一权威 tick 内可见。
func bucketMoistureTick(engine *Engine, commands []Command) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands(commands, &tick.result)
	tick.context.SettleGameplay(&tick.result)
	environment := engine.realm.NewEnvironmentMutation(
		tick.mutation,
		engine.tick.Load(),
		realm.EnvironmentConfig{},
	)
	engine.realm.AdvanceFarmlandMoisture(engine.activeInterestKeys(), environment)
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}

var (
	bucketEmptyHeld = core.ItemStack{Item: core.ItemEmptyBucket, Count: 1}
	bucketWaterHeld = core.ItemStack{Item: core.ItemWaterBucket, Count: 1}
)

// TestBucketCollectAndPlaceAtomic 锁定取/放的同 tick 原子性：取水后源变空气且
// 原格空桶变水桶；放水后落点变源且原格水桶变空桶。
func TestBucketCollectAndPlaceAtomic(t *testing.T) {
	engine, session, yaw, pitch := readyBucketPlayer(t, bucketEmptyHeld, core.WaterSourceID, false)
	result := collectBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("合法取水被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.AirID {
		t.Fatalf("取水后方块 = %d，想要空气", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketWaterHeld {
		t.Fatalf("取水后栏位 = %+v，想要 %+v", got, bucketWaterHeld)
	}

	player := engine.sessions[session].player
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	below := tillTarget
	below.Y--
	placeYaw, placePitch := lookAtBlockTop(eye, below)
	result = placeBucket(engine, session, placeYaw, placePitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("合法放水被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.WaterSourceID {
		t.Fatalf("放水后方块 = %d，想要源", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketEmptyHeld {
		t.Fatalf("放水后栏位 = %+v，想要 %+v", got, bucketEmptyHeld)
	}
}

// TestBucketCollectRejectsNonSource 锁定非源不可取：实心直接命中、流动水身后
// 有固体时都拒绝为 RejectNotFluidSource，且方块与栏位零副作用。
func TestBucketCollectRejectsNonSource(t *testing.T) {
	engine, session, yaw, pitch := readyBucketPlayer(t, bucketEmptyHeld, core.StoneID, false)
	result := collectBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNotFluidSource {
		t.Fatalf("实心 Rejected=%+v，想要 RejectNotFluidSource", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.StoneID {
		t.Fatalf("非源取水改了方块: %d", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketEmptyHeld {
		t.Fatalf("非源取水改了栏位: %+v", got)
	}

	// 流动水本身被射线穿过：身后无固体时射线落空（NoTarget），身后有固体时命
	// 中固体并以非源拒绝。这里以后者锁定拒绝原因。
	engine, session, yaw, pitch = readyBucketPlayer(t, bucketEmptyHeld, core.WaterLevel3ID, false)
	backing := tillTarget
	backing.Z++
	engine.SetBlockForTest(backing, core.StoneID)
	result = collectBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNotFluidSource {
		t.Fatalf("流动水 Rejected=%+v，想要 RejectNotFluidSource", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.WaterLevel3ID {
		t.Fatalf("流动水取水改了方块: %d", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketEmptyHeld {
		t.Fatalf("流动水取水改了栏位: %+v", got)
	}
}

// TestBucketPlaceRejectsSourceAndSolid 锁定落点只接受空气与流动水：源格（射线
// 穿过、贴面落点回到源格）与实心遮挡（侧面命中、落点是身后的石头）都拒绝为
// RejectOccupied，且零副作用。
func TestBucketPlaceRejectsSourceAndSolid(t *testing.T) {
	engine, session, yaw, pitch := readyBucketPlayer(t, bucketWaterHeld, core.WaterSourceID, true)
	result := placeBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectOccupied {
		t.Fatalf("源落点 Rejected=%+v，想要 RejectOccupied", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.WaterSourceID {
		t.Fatalf("源落点改了方块: %d", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketWaterHeld {
		t.Fatalf("源落点改了栏位: %+v", got)
	}

	// 实心拒绝的真实路径是起点卡在实心内：射线原点格即命中且没有朝向面，
	// 落点无从谈起。把眼睛所在格填成石头后放水。
	engine, session, yaw, pitch = readyBucketPlayer(t, bucketWaterHeld, core.AirID, true)
	eyeCell := core.BlockPos{X: 0, Y: 2, Z: 0}
	engine.SetBlockForTest(eyeCell, core.StoneID)
	result = placeBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectOccupied {
		t.Fatalf("实心卡住 Rejected=%+v，想要 RejectOccupied", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.AirID {
		t.Fatalf("实心卡住改了目标: %d", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketWaterHeld {
		t.Fatalf("实心卡住改了栏位: %+v", got)
	}
}

// TestBucketPlaceAcceptsFlowingWater 锁定覆盖流动水合法：流动格变源，原格水桶
// 变空桶。
func TestBucketPlaceAcceptsFlowingWater(t *testing.T) {
	engine, session, yaw, pitch := readyBucketPlayer(t, bucketWaterHeld, core.WaterLevel1ID, true)
	result := placeBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("覆盖流动水被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.WaterSourceID {
		t.Fatalf("覆盖流动水后方块 = %d，想要源", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketEmptyHeld {
		t.Fatalf("覆盖流动水后栏位 = %+v，想要 %+v", got, bucketEmptyHeld)
	}
}

// TestBucketRejectsBucketMismatch 锁定桶态错：命令与手持不匹配一律拒绝为
// RejectBucketMismatch，且零副作用。
func TestBucketRejectsBucketMismatch(t *testing.T) {
	tests := []struct {
		name   string
		kind   CommandKind
		held   core.ItemStack
		target core.BlockID
		place  bool
	}{
		{"空桶命令持水桶", CommandCollectWater, bucketWaterHeld, core.WaterSourceID, false},
		{"空桶命令持石头", CommandCollectWater, core.ItemStack{Item: core.ItemStone, Count: 4}, core.WaterSourceID, false},
		{"空桶命令持空栏", CommandCollectWater, core.ItemStack{}, core.WaterSourceID, false},
		{"水桶命令持空桶", CommandPlaceWater, bucketEmptyHeld, core.AirID, true},
		{"水桶命令持石头", CommandPlaceWater, core.ItemStack{Item: core.ItemStone, Count: 4}, core.AirID, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, session, yaw, pitch := readyBucketPlayer(t, test.held, test.target, test.place)
			result := settlePlayerInteractionsTick(engine, []Command{{
				Session: session, Sequence: 2, Kind: test.kind, Yaw: yaw, Pitch: pitch,
			}})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectBucketMismatch {
				t.Fatalf("Rejected=%+v，想要 RejectBucketMismatch", result.Rejected)
			}
			if got := tillBlockAt(t, engine, tillTarget); got != test.target {
				t.Fatalf("桶态错改了方块: %d", got)
			}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != test.held {
				t.Fatalf("桶态错改了栏位: %+v", got)
			}
		})
	}
}

// TestBucketRespectsInteractionReach 锁定超距：触及距离外取/放都拒绝为
// RejectNoTarget，且零副作用。
func TestBucketRespectsInteractionReach(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })
	tunables := tuning.DefaultTunables()
	tunables.InteractionReach = 2
	tuning.SetTunables(tunables)

	engine, session, yaw, pitch := readyBucketPlayer(t, bucketEmptyHeld, core.WaterSourceID, false)
	result := collectBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNoTarget {
		t.Fatalf("超距取水 Rejected=%+v，想要 RejectNoTarget", result.Rejected)
	}
	if got := tillBlockAt(t, engine, tillTarget); got != core.WaterSourceID {
		t.Fatalf("超距取水改了方块: %d", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketEmptyHeld {
		t.Fatalf("超距取水改了栏位: %+v", got)
	}

	engine, session, yaw, pitch = readyBucketPlayer(t, bucketWaterHeld, core.AirID, true)
	result = placeBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNoTarget {
		t.Fatalf("超距放水 Rejected=%+v，想要 RejectNoTarget", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketWaterHeld {
		t.Fatalf("超距放水改了栏位: %+v", got)
	}
}

// TestBucketChunkNotReadyRejected 锁定区块未就绪：射线进入未就绪区块时取/放都
// 拒绝为 RejectChunkNotReady，且不扣桶。
func TestBucketChunkNotReadyRejected(t *testing.T) {
	setup := func(t *testing.T, held core.ItemStack) (*Engine, SessionID, float32, float32) {
		t.Helper()
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		player.state.Position = mgl32.Vec3{15.5, 1, 0.5}
		player.inventory.Hotbar.Slots[0] = held
		player.inventory.Hotbar.Selected = 0
		far := core.BlockPos{X: 16, Y: 1, Z: 0}
		eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		yaw, pitch := lookAtBlockCenter(eye, far)
		return engine, session, yaw, pitch
	}
	engine, session, yaw, pitch := setup(t, bucketEmptyHeld)
	if result := collectBucket(engine, session, yaw, pitch); len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != RejectChunkNotReady {
		t.Fatalf("取水未就绪 Rejected=%+v，想要 RejectChunkNotReady", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketEmptyHeld {
		t.Fatalf("取水未就绪扣了桶: %+v", got)
	}

	engine, session, yaw, pitch = setup(t, bucketWaterHeld)
	if result := placeBucket(engine, session, yaw, pitch); len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != RejectChunkNotReady {
		t.Fatalf("放水未就绪 Rejected=%+v，想要 RejectChunkNotReady", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != bucketWaterHeld {
		t.Fatalf("放水未就绪扣了桶: %+v", got)
	}
}

// TestBucketMoistureFollowsMembership 锁定湿度同 tick 联动：放水后附近干耕地变
// 湿，取水后变回干。
func TestBucketMoistureFollowsMembership(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	farmland := core.BlockPos{X: 0, Y: 1, Z: 2}
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = bucketWaterHeld
	player.inventory.Hotbar.Selected = 0
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	below := tillTarget
	below.Y--
	placeYaw, placePitch := lookAtBlockTop(eye, below)
	collectYaw, collectPitch := lookAtBlockCenter(eye, tillTarget)

	result := bucketMoistureTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlaceWater, Yaw: placeYaw, Pitch: placePitch,
	}})
	if len(result.Rejected) != 0 {
		t.Fatalf("放水被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 放水后耕地 = %d，想要湿耕地", got)
	}
	// 放水成功后原格已是空桶，直接取水。
	result = bucketMoistureTick(engine, []Command{{
		Session: session, Sequence: 3, Kind: CommandCollectWater, Yaw: collectYaw, Pitch: collectPitch,
	}})
	if len(result.Rejected) != 0 {
		t.Fatalf("取水被拒绝: %+v", result.Rejected)
	}
	if got := tillBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("同 tick 取水后耕地 = %d，想要干耕地", got)
	}
}

// TestBucketSuccessSuppressesMiningOnlyThatTick 锁定桶成功当 tick 抑制采掘：同
// tick 采掘进度清零但按住意图保留，下一 tick 从头累积。
func TestBucketSuccessSuppressesMiningOnlyThatTick(t *testing.T) {
	engine, sessions, targets := readyMiningPlayers(t, 1)
	session := sessions[0]
	player := engine.sessions[session].player
	source := core.BlockPos{X: 3, Y: 1, Z: 8}
	engine.SetBlockForTest(source, core.WaterSourceID)
	player.inventory.Hotbar.Slots[0] = bucketEmptyHeld
	player.inventory.Hotbar.Selected = 0
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	bucketYaw, bucketPitch := lookAtBlockCenter(eye, source)

	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands([]Command{
		{Session: session, Sequence: 11, Kind: CommandPlayerInput, Yaw: 0, Pitch: miningTestPitch, Mining: true},
		{Session: session, Sequence: 12, Kind: CommandCollectWater, Yaw: bucketYaw, Pitch: bucketPitch},
	}, &tick.result)
	tick.context.SettleGameplay(&tick.result)
	tick.context.FinishWorld(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	publishFixture(engine, &tick)

	if len(tick.result.Rejected) != 0 {
		t.Fatalf("同 tick 取水被拒绝: %+v", tick.result.Rejected)
	}
	if got := tillBlockAt(t, engine, source); got != core.AirID {
		t.Fatalf("取水后方块 = %d，想要空气", got)
	}
	if player.mining != (miningState{}) {
		t.Fatalf("桶成功同 tick mining=%+v，想要零值", player.mining)
	}
	if !player.miningHeld {
		t.Fatal("桶成功清空了 miningHeld")
	}

	next := engine.beginTick()
	next.context.ApplyPlayerCommands([]Command{
		{Session: session, Sequence: 13, Kind: CommandPlayerInput, Yaw: 0, Pitch: miningTestPitch, Mining: true},
	}, &next.result)
	next.context.FinishWorld(&next.result)
	commitMutation(next.mutation, &next.result)
	publishFixture(engine, &next)
	if player.mining.progressTicks != 1 || player.mining.target != targets[0] {
		t.Fatalf("下一 tick mining=%+v，想要目标 %+v 从 1 开始", player.mining, targets[0])
	}
}

// TestCompanionCannotPlaceOrMineWater 锁定伙伴路径直接拒绝：伙伴放置水源意图零
// 副作用（哪怕背包里真的有水桶也不扣），采掘防御清单对源与流动水都关闭。
func TestCompanionCannotPlaceOrMineWater(t *testing.T) {
	fixture := readyCompanionPlacement(t)
	fixture.entry.inventory.Hotbar.Slots[1] = bucketWaterHeld
	before := fixture.entry.inventory

	result := placeCompanionAction(t, fixture, core.WaterSourceID, fixture.target)
	assertCompanionPlaceRejected(t, fixture, core.AirID, result)
	if got := fixture.entry.inventory; got != before {
		t.Fatalf("伙伴放水改了背包: %+v", got)
	}

	for _, block := range []core.BlockID{core.WaterSourceID, core.WaterLevel1ID} {
		if companionMineableBlock(block) {
			t.Fatalf("companionMineableBlock(%d) = true，流体必须是显式拒绝的 mine 目标", block)
		}
	}
}

// TestCompanionMineableBlockExplicitlyMentionsFluid 锁定采掘防御清单对流体的
// 显式拒绝：水今天没有 `core.BlockDrop` 登记，通用判据碰巧也会拒绝它，但若
// 未来水登记了掉落，只有这里点名 `core.IsFluid` 的显式谓词还站着。
func TestCompanionMineableBlockExplicitlyMentionsFluid(t *testing.T) {
	if !companionFunctionMentionsIdentifier(t, "mining.go", "companionMineableBlock", "IsFluid") {
		t.Fatal("companionMineableBlock 没有显式点名 core.IsFluid；" +
			"伙伴拒绝流体不得依赖缺失 BlockDrop 的巧合")
	}
	for _, block := range []core.BlockID{
		core.WaterSourceID,
		core.WaterLevel1ID, core.WaterLevel2ID, core.WaterLevel3ID, core.WaterLevel4ID,
		core.WaterLevel5ID, core.WaterLevel6ID, core.WaterLevel7ID,
	} {
		if companionMineableBlock(block) {
			t.Fatalf("companionMineableBlock(%d) = true，流体必须是显式拒绝的伙伴采掘目标", block)
		}
	}
}

// TestCompanionPlaceableBlockExplicitlyRejectsFluid 锁定放置防御清单对流体的
// 显式拒绝：水今天走不到往返校验（无 `core.BlockDrop` 登记），但巧合性阻挡
// 不是契约——点名 `core.IsFluid` 的显式谓词才是在未来登记变更下仍然成立的拒绝。
func TestCompanionPlaceableBlockExplicitlyRejectsFluid(t *testing.T) {
	if !companionFunctionMentionsIdentifier(t, "companion_placement.go", "companionPlaceableBlock", "IsFluid") {
		t.Fatal("companionPlaceableBlock 没有显式点名 core.IsFluid；" +
			"伙伴拒绝流体不得依赖缺失 BlockDrop 的巧合")
	}
	for _, block := range []core.BlockID{
		core.WaterSourceID,
		core.WaterLevel1ID, core.WaterLevel2ID, core.WaterLevel3ID, core.WaterLevel4ID,
		core.WaterLevel5ID, core.WaterLevel6ID, core.WaterLevel7ID,
	} {
		if _, ok := companionPlaceableBlock(block); ok {
			t.Fatalf("companionPlaceableBlock(%d) 放行，流体必须是显式拒绝的伙伴放置目标", block)
		}
	}
}

// TestBucketSuccessPublishesPlacementSequence 锁定取/放成功复用放置成功序号
// 通道：客户端音频只消费严格递增的成功序号，桶成功必须同样产出
// `PlacementSuccesses`，否则取/放成功在客户端永远无声。
func TestBucketSuccessPublishesPlacementSequence(t *testing.T) {
	engine, session, yaw, pitch := readyBucketPlayer(t, bucketEmptyHeld, core.WaterSourceID, false)
	result := collectBucket(engine, session, yaw, pitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("合法取水被拒绝: %+v", result.Rejected)
	}
	if len(result.PlacementSuccesses) != 1 || result.PlacementSuccesses[0].Session != session ||
		result.PlacementSuccesses[0].Sequence != 2 {
		t.Fatalf("取水成功发布 = %+v，想要 [{session 序号 2}]", result.PlacementSuccesses)
	}

	player := engine.sessions[session].player
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	below := tillTarget
	below.Y--
	placeYaw, placePitch := lookAtBlockTop(eye, below)
	result = placeBucket(engine, session, placeYaw, placePitch)
	if len(result.Rejected) != 0 {
		t.Fatalf("合法放水被拒绝: %+v", result.Rejected)
	}
	if len(result.PlacementSuccesses) != 1 || result.PlacementSuccesses[0].Session != session ||
		result.PlacementSuccesses[0].Sequence != 2 {
		t.Fatalf("放水成功发布 = %+v，想要 [{session 序号 2}]", result.PlacementSuccesses)
	}
}
