package runtime

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件锁定夜行者阶段与权威 tick 的接线：夜间生成和死亡掉落都必须经
// production `Engine.Step` 进入同一批发布。

func readyNightEngine(t *testing.T, seed int64) (*Engine, SessionID) {
	t.Helper()
	engine := NewEngine(3, 0, seed)
	session := SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	for range 16 {
		result := engine.Step()
		feedRuntimeFlatChunks(engine, result)
		if player, ok := engine.Player(session); ok && player.Ready {
			return engine, session
		}
	}
	t.Fatal("锚点玩家未在预算内激活")
	return engine, session
}

func TestStepSpawnsHostileAtNightWithoutMovingIt(t *testing.T) {
	engine, _ := readyNightEngine(t, 0)
	for offset := uint64(0); offset < 400; offset++ {
		engine.SetWorldTimeForTest(13000 + offset)
		result := engine.Step()
		mobs := engine.HostileMobs()
		if len(mobs) == 0 {
			continue
		}
		if len(result.Players) != 1 || len(mobs) != 1 {
			t.Fatalf("夜间权威 tick 发布异常：Players=%d hostiles=%d", len(result.Players), len(mobs))
		}
		mob := mobs[0]
		position := mob.State.Position
		if math.Abs(float64(position.X())-math.Floor(float64(position.X()))-0.5) > 1e-6 ||
			math.Abs(float64(position.Z())-math.Floor(float64(position.Z()))-0.5) > 1e-6 ||
			mob.State.Velocity != (mgl32.Vec3{}) {
			t.Fatalf("新生夜行者当 tick 已被物理推进：%+v", mob.State)
		}
		if !mob.State.OnGround || mob.Health != core.MaxHealth || mob.BurnCooldown != 20 {
			t.Fatalf("新生夜行者身体异常：%+v", mob)
		}
		return
	}
	t.Fatal("预算内没有确定性夜间生成")
}

func TestStepSettlesHostileDeathAndDropInSingleTick(t *testing.T) {
	engine, session := readyNightEngine(t, 0)
	mob := runtimeTestHostile(31)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	mob.Health = 1
	mob.BurnCooldown = 1
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.SetWorldTimeForTest(1000)

	result := engine.Step()

	if mobs := engine.HostileMobs(); len(mobs) != 0 {
		t.Fatalf("灼烧致死未在同一权威 tick 内移除身体：%+v", mobs)
	}
	drops := engine.AppendSessionDrops(session, nil)
	count := 0
	for _, drop := range drops {
		if drop.Item == core.ItemRottenFlesh {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("死亡掉落腐肉=%d，想要恰好 1：%+v", count, drops)
	}
	deathChunk := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	if !containsChunkKey(result.Ready, deathChunk) && !chunkInChanges(result.Changes, deathChunk) {
		t.Fatalf("死亡掉落区块未进入本 tick 的变更发布：Changes=%+v Ready=%+v",
			result.Changes, result.Ready)
	}
}

func runtimeTestHostile(id uint64) HostileMob {
	return HostileMob{
		ID:        id,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{0.5, 1, 0.5},
			OnGround: true,
		},
		Health:       core.MaxHealth,
		BurnCooldown: 20,
	}
}

func feedRuntimeFlatChunks(engine *Engine, result TickResult) {
	for _, key := range result.Acquire {
		engine.SubmitAcquired(AcquiredChunk{
			Key:               key,
			Chunk:             movementFlatChunk(key.Pos),
			Revision:          1,
			PersistedRevision: 1,
		})
	}
}

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
