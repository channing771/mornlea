package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestCompanionActionInboxBoundedAndSessionless(t *testing.T) {
	t.Run("inbox 有界且满员即丢弃", func(t *testing.T) {
		engine := NewEngine(0, 0, 0)
		for suffix := byte(1); suffix <= companion.MaxActive; suffix++ {
			if !engine.EnqueueCompanionAction(CompanionAction{ID: runtimeCompanionTestID(suffix)}) {
				t.Fatalf("第 %d 个 action 未入队", suffix)
			}
		}
		if engine.EnqueueCompanionAction(CompanionAction{ID: runtimeCompanionTestID(9)}) {
			t.Fatal("超出容量的 action 未被拒绝")
		}
		engine.Step()
		if !engine.EnqueueCompanionAction(CompanionAction{ID: runtimeCompanionTestID(9)}) {
			t.Fatal("tick 边界排空后 inbox 仍拒绝新 action")
		}
	})
}

func TestHostileActionInboxIsBoundedAndDropsOverflow(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	for id := uint64(1); id <= maxHostileActionsPerTick; id++ {
		if !engine.EnqueueHostileAction(HostileAction{ID: id}) {
			t.Fatalf("第 %d 条意图被拒绝，想要 inbox 容量覆盖全集合", id)
		}
	}
	if engine.EnqueueHostileAction(HostileAction{ID: maxHostileActionsPerTick}) {
		t.Fatal("超容意图被接受，想要非阻塞丢弃")
	}
}

func TestCompanionInterestSlidesWithBody(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := runtimeCompanionTestID(1)
	position := mgl32.Vec3{8.5, 1, 8.5}
	engine.RegisterCompanion(CompanionRestore{
		ID: id,
		Body: &companion.Body{
			ID: id, Dimension: core.Overworld, Position: [3]float32(position),
		},
		SpawnDimension: core.Overworld,
	})
	activateRuntimeCompanion(t, engine, id)

	if len(engine.wanted) != 9 || !engine.WantsChunk(core.ChunkKey{
		Dimension: core.Overworld, Pos: core.ChunkPos{X: -1},
	}) {
		t.Fatalf("激活后兴趣不是脚下 3x3: wanted=%+v", engine.wanted)
	}

	var final CompanionUpdate
	for tick := 0; tick < 200; tick++ {
		if !engine.EnqueueCompanionAction(CompanionAction{
			ID: id, Kind: CompanionActionMove, Input: physics.Input{MoveX: 1},
		}) {
			t.Fatalf("tick %d action 未入队", tick)
		}
		result := engine.Step()
		feedRuntimeFlatChunks(engine, result)
		if len(engine.wanted) > 9 {
			t.Fatalf("tick %d 兴趣超过 9 个区块: %d", tick, len(engine.wanted))
		}
		if len(result.Companions) != 1 {
			t.Fatalf("tick %d Companions=%+v", tick, result.Companions)
		}
		final = result.Companions[0]
		if final.State.Position.X() >= 16 {
			break
		}
	}
	if final.State.Position.X() < 16 {
		t.Fatalf("伙伴未在预算内跨入相邻区块: %+v", final)
	}

	for dz := int32(-1); dz <= 1; dz++ {
		for dx := int32(0); dx <= 2; dx++ {
			key := core.ChunkKey{
				Dimension: core.Overworld,
				Pos:       core.ChunkPos{X: dx, Z: dz},
			}
			if !engine.WantsChunk(key) {
				t.Fatalf("滑动后的兴趣缺少新区块 %+v", key)
			}
		}
	}
	if len(engine.wanted) != 9 {
		t.Fatalf("滑动后兴趣=%d，想要 9", len(engine.wanted))
	}
	if engine.WantsChunk(core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -1}}) {
		t.Fatal("滑动后旧边缘区块仍在兴趣内")
	}
	if _, exists := engine.dimension(core.Overworld).Info(core.ChunkPos{X: -1}); exists {
		t.Fatal("离开兴趣的干净区块未被释放")
	}
}

func activateRuntimeCompanion(t *testing.T, engine *Engine, id companion.ID) {
	t.Helper()
	for range 16 {
		result := engine.Step()
		feedRuntimeFlatChunks(engine, result)
		for _, update := range result.Companions {
			if update.ID == id {
				return
			}
		}
	}
	t.Fatalf("伙伴 %v 未在预算内激活", id)
}

func runtimeCompanionTestID(suffix byte) companion.ID {
	return companion.ID{6: 0x40, 8: 0x80, 15: suffix}
}
