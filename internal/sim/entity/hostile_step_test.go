package entity

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定夜行者阶段与权威 tick 的接线：阶段顺序插入在统一物理（玩家 →
// 伙伴）之后、流体推进之前；夜间生成/白昼灼烧/远离消失/死亡掉落全部经
// `Engine.Step` 生效，死亡掉落与其它区块写者共用同一批 revision 与 Ready
// 发布；新生个体当 tick 不位移。

// readyNightEngine 构造带一名已激活锚点玩家的端到端引擎：订阅请求的区块按
// "干净磁盘读回"喂成平地（复用共享 feeder），视野半径足以覆盖 24..48 的
// 候选环（区块 ±3）。
func readyNightEngine(t *testing.T, seed int64) (*Engine, SessionID) {
	t.Helper()
	engine := NewEngine(3, 0, seed)
	session := SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	for range 16 {
		result := engine.Step()
		feedCompanionActionRequests(t, engine, result)
		if player := engine.sessions[session].player; player != nil && player.lifecycle == PlayerActive {
			return engine, session
		}
	}
	t.Fatal("锚点玩家未在预算内激活")
	return engine, session
}

func TestStepSpawnsHostileAtNightWithoutMovingIt(t *testing.T) {
	engine, _ := readyNightEngine(t, 0)
	// 先探得一个门槛通过的夜间 tick，再清空集合让生成经完整权威 tick 发生。
	tick := findSpawningTick(t, engine, 13000, 1, 400)
	position := spawnedPosition(t, engine)
	clearHostilesForTest(engine)

	engine.worldTime.Store(tick)
	result := engine.Step()
	if len(result.Players) != 1 {
		t.Fatalf("Players=%d，想要 1", len(result.Players))
	}
	if len(engine.hostiles.entries) != 1 {
		t.Fatalf("夜间权威 tick 未生成夜行者：数量=%d", len(engine.hostiles.entries))
	}
	entry := &engine.hostiles.entries[0]
	// 生成次序先于物理：新生个体当 tick 不位移、保持生成姿势。
	if entry.state.Position != position {
		t.Fatalf("新生夜行者当 tick 位移到 %+v，想要保持在 %+v", entry.state.Position, position)
	}
	if !entry.state.OnGround || entry.health != core.MaxHealth {
		t.Fatalf("新生夜行者身体异常：%+v", entry.state)
	}
	// 同一 tick 的灼烧判定看到的是夜间相位：计时保持满周期。
	if entry.burnCooldown != hostileCooldownPeriodTicks {
		t.Fatalf("夜间灼烧剩余=%d，想要满周期", entry.burnCooldown)
	}
}

func TestStepSettlesHostileDeathAndDropInSingleTick(t *testing.T) {
	engine, _ := readyNightEngine(t, 0)
	mob := validTestHostile(31)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	mob.Health = 1
	mob.BurnCooldown = 1
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.worldTime.Store(testDayTick)

	result := engine.Step()

	if len(engine.hostiles.entries) != 0 {
		t.Fatal("灼烧致死未在同一权威 tick 内移除身体")
	}
	if got := countLoadedDrops(t, engine, core.ItemRottenFlesh); got != 1 {
		t.Fatalf("死亡掉落腐肉=%d，想要恰好 1", got)
	}
	deathChunk := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 0, Z: 0}}
	if !containsChunkKey(result.Ready, deathChunk) && !chunkInChanges(result.Changes, deathChunk) {
		t.Fatalf("死亡掉落区块未进入本 tick 的变更发布：Changes=%+v Ready=%+v",
			result.Changes, result.Ready)
	}
}

// chunkInChanges 报告区块是否出现在本 tick 的变更批次中（零方块 barrier 也算）。
func chunkInChanges(changes []ChunkChangeBatch, want core.ChunkKey) bool {
	for _, batch := range changes {
		if batch.Dimension == want.Dimension && batch.Chunk == want.Pos {
			return true
		}
	}
	return false
}

func containsChunkKey(keys []core.ChunkKey, want core.ChunkKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func TestStepRunsHostilePhasesBetweenPhysicsAndFluid(t *testing.T) {
	// 阶段顺序契约的夜行者部分：hostile 阶段（生成 → 移动 → 灼烧 → 远离 →
	// 死亡）作为单一阶段事件出现在统一物理之后、流体推进之前。
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	requested := engine.Step()
	for _, key := range requested.Acquire {
		engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
	}
	generated := engine.Step()
	for _, key := range generated.Generate {
		engine.SubmitGenerated(GeneratedChunk{
			Dimension: core.Overworld,
			Chunk:     movementFlatChunk(key.Pos),
		})
	}
	engine.Step()

	var phases []stepPhase
	engine.stepPhaseObserver = func(phase stepPhase) { phases = append(phases, phase) }
	engine.Step()
	engine.stepPhaseObserver = nil

	want := []stepPhase{
		phasePlayerCommands, phaseCompanionActions, phasePhysicsAdvance,
		phaseHostileAdvance, phaseFluidAdvance, phaseFarmlandMoistureAdvance,
		phaseCropAdvance,
	}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("阶段顺序=%v，想要 %v", phases, want)
	}
}
