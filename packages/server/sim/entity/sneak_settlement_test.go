package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// sneakInput 构造一条只差疾跑/潜行位的地面前移输入命令。
func sneakInput(session SessionID, sequence uint64, sneaking, sprinting bool) Command {
	return Command{
		Session: session, Sequence: sequence, Kind: CommandPlayerInput,
		MoveZ: 1, Sneaking: sneaking, Sprinting: sprinting,
	}
}

// sneakSteadyPerTick 把玩家推到终端速度附近，再量后一段的平均单 tick 水平位移。
// 首段 tick 只做收敛不计量：疾跑从静止加速的前两 tick 天然低于终端速度。
func sneakSteadyPerTick(t *testing.T, engine *Engine, session SessionID, sneaking, sprinting bool) float64 {
	t.Helper()
	input := sneakInput(session, 0, sneaking, sprinting)
	var sequence uint64 = 2
	for range 30 {
		input.Sequence = sequence
		sequence++
		advancePlayerMovementTick(engine, []Command{input})
	}
	start := engine.sessions[session].player.state.Position
	const measure = 20
	for range measure {
		input.Sequence = sequence
		sequence++
		advancePlayerMovementTick(engine, []Command{input})
	}
	delta := engine.sessions[session].player.state.Position.Sub(start)
	horizontal := mgl32.Vec2{delta.X(), delta.Z()}
	return float64(horizontal.Len()) / measure
}

// TestSneakSuppressesSprintAccelerationAndExhaustion 覆盖互斥场景的前半边：
// 潜行与疾跑同置时只减速不加速，且本 tick 不计费疾跑疲劳。
func TestSneakSuppressesSprintAccelerationAndExhaustion(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -3, 1)
	player := engine.sessions[session].player
	before := exhaustionOf(player)

	perTick := sneakSteadyPerTick(t, engine, session, true, true)
	tun := engine.physicsTunables
	want := float64(tun.WalkSpeed * tun.SneakSpeedMultiplier * physics.FixedDeltaSeconds)
	if perTick < want*0.5 || perTick > want*1.5 {
		t.Fatalf("潜行+疾跑单 tick 位移=%v，想要 0.3x 区间 [%v,%v]", perTick, want*0.5, want*1.5)
	}
	if got := exhaustionOf(player); got != before {
		t.Fatalf("潜行+疾跑后三层状态=%v，想要保持 %v（疾跑疲劳必须被压制）", got, before)
	}
}

// sneakFurnaceFixture 在玩家脚下放一只已登记槽位的熔炉，返回俯视它的 pitch。
func sneakFurnaceFixture(t *testing.T, engine *Engine) float32 {
	t.Helper()
	furnace := core.BlockPos{}
	engine.SetBlockForTest(furnace, core.FurnaceID)
	index, indexed := world.ChunkBlockIndex(furnace)
	if !indexed {
		t.Fatal("熔炉没有区块索引")
	}
	engine.SetChunkFurnaceForTest(
		core.ChunkKey{Dimension: core.Overworld}, 0,
		world.FurnaceSlot{Generation: 1, Active: true, BlockIndex: index},
	)
	return fluidLookDown
}

// TestSneakOpenContainerAuthoritativelyRejected 覆盖服务端权威拒绝场景：
// sneakingHeld 为真时开容器必须拒绝且不建立查看关系。
func TestSneakOpenContainerAuthoritativelyRejected(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	pitch := sneakFurnaceFixture(t, engine)

	result := applyPlayerCommandsTick(engine, []Command{sneakInput(session, 2, true, false)})
	if len(result.Rejected) != 0 {
		t.Fatalf("潜行输入被拒绝：%+v", result.Rejected)
	}
	if !engine.sessions[session].player.sneakingHeld {
		t.Fatal("夹具失效：潜行输入后 sneakingHeld 未置位")
	}
	result = applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 3, Kind: CommandOpenFurnace, Pitch: pitch,
	}})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("潜行开容器=%+v，想要恰好一次 RejectInvalidInput", result.Rejected)
	}
	if engine.sessions[session].viewContainer {
		t.Fatal("潜行开容器被拒绝后仍建立了查看关系")
	}
	if len(result.Furnaces) != 0 {
		t.Fatalf("潜行开容器被拒绝后仍发布了熔炉状态：%+v", result.Furnaces)
	}
}

// TestSneakControlSprintAcceleratesAndOpens 是非潜行对照组：同条件加速
// 1.3x 且开容器成功，防止门控把正常路径一起掐掉。
func TestSneakControlSprintAcceleratesAndOpens(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -3, 1)
	player := engine.sessions[session].player
	before := exhaustionOf(player)

	perTick := sneakSteadyPerTick(t, engine, session, false, true)
	tun := engine.physicsTunables
	want := float64(tun.WalkSpeed * tun.SprintSpeedMultiplier * physics.FixedDeltaSeconds)
	if perTick < want*0.7 || perTick > want*1.3 {
		t.Fatalf("疾跑单 tick 位移=%v，想要 1.3x 区间 [%v,%v]", perTick, want*0.7, want*1.3)
	}
	if got := exhaustionOf(player); got == before {
		t.Fatalf("疾跑 50 tick 后三层状态仍为 %v，想要累积疾跑疲劳", got)
	}

	engine, session = readyMovementPlayer(t)
	pitch := sneakFurnaceFixture(t, engine)
	result := applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandOpenFurnace, Pitch: pitch,
	}})
	if len(result.Rejected) != 0 {
		t.Fatalf("非潜行开容器被拒绝：%+v", result.Rejected)
	}
	if !engine.sessions[session].viewContainer {
		t.Fatal("非潜行开容器成功后未建立查看关系")
	}
	if len(result.Furnaces) != 1 {
		t.Fatalf("非潜行开容器后熔炉发布=%+v，想要恰好一条", result.Furnaces)
	}
}

// TestSneakHeldClearedOnInvalidInput 钉住锁存卫生：非法输入与其它 held 位
// 同形清零潜行锁存，被拒绝的输入不留下潜行意图。
func TestSneakHeldClearedOnInvalidInput(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	applyPlayerCommandsTick(engine, []Command{sneakInput(session, 2, true, false)})
	if !engine.sessions[session].player.sneakingHeld {
		t.Fatal("夹具失效：潜行输入后 sneakingHeld 未置位")
	}
	result := applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 3, Kind: CommandPlayerInput, MoveX: 2,
	}})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("越界输入=%+v，想要恰好一次 RejectInvalidInput", result.Rejected)
	}
	if engine.sessions[session].player.sneakingHeld {
		t.Fatal("非法输入后 sneakingHeld 仍置位，想要与 miningHeld/eatingHeld 同形清零")
	}
}
