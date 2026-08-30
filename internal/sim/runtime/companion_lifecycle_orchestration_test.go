package runtime

import (
	"errors"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
)

func TestCompanionRestoreChunkInitiallyMissingThenRestores(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := runtimeCompanionTestID(1)
	position := mgl32.Vec3{0.5, 1, 0.5}
	key := core.ChunkKey{Dimension: core.Overworld}
	engine.RegisterCompanion(CompanionRestore{
		ID: id,
		Body: &companion.Body{
			ID: id, Dimension: core.Overworld, Position: position,
		},
		SpawnDimension: core.Overworld,
	})

	requested := engine.Step()
	if !reflect.DeepEqual(requested.Acquire, []core.ChunkKey{key}) || len(requested.Companions) != 0 {
		t.Fatalf("首次请求=%+v updates=%+v", requested.Acquire, requested.Companions)
	}
	engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
	missing := engine.Step()
	if !reflect.DeepEqual(missing.Generate, []core.ChunkKey{key}) || len(missing.Companions) != 0 {
		t.Fatalf("missing 处理=%+v updates=%+v", missing.Generate, missing.Companions)
	}
	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Chunk:     movementFlatChunk(core.ChunkPos{}),
	})
	restored := engine.Step().Companions
	if len(restored) != 1 || restored[0].ID != id || restored[0].State.Position != position {
		t.Fatalf("区块就绪后未恢复原位置: %+v", restored)
	}
}

func TestCompanionRestoreRetriesRetainedChunkFailures(t *testing.T) {
	tests := []struct {
		name         string
		reachFailure func(*testing.T, *Engine, core.ChunkKey) TickResult
	}{
		{
			name: "load error",
			reachFailure: func(t *testing.T, engine *Engine, key core.ChunkKey) TickResult {
				engine.SubmitAcquired(AcquiredChunk{Key: key, Err: errors.New("temporary load failure")})
				return engine.Step()
			},
		},
		{
			name: "loaded chunk apply error",
			reachFailure: func(t *testing.T, engine *Engine, key core.ChunkKey) TickResult {
				engine.SubmitAcquired(AcquiredChunk{Key: key})
				return engine.Step()
			},
		},
		{
			name: "generation error",
			reachFailure: func(t *testing.T, engine *Engine, key core.ChunkKey) TickResult {
				advanceRuntimeCompanionRestoreToGenerating(t, engine, key)
				engine.SubmitGenerated(GeneratedChunk{
					Dimension: key.Dimension,
					Pos:       key.Pos,
					Err:       errors.New("temporary generation failure"),
				})
				return engine.Step()
			},
		},
		{
			name: "generated chunk apply error",
			reachFailure: func(t *testing.T, engine *Engine, key core.ChunkKey) TickResult {
				advanceRuntimeCompanionRestoreToGenerating(t, engine, key)
				engine.SubmitGenerated(GeneratedChunk{Dimension: key.Dimension, Pos: key.Pos})
				return engine.Step()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := NewEngine(0, 0, 0)
			id := runtimeCompanionTestID(1)
			key := core.ChunkKey{Dimension: core.Overworld}
			engine.RegisterCompanion(CompanionRestore{
				ID: id,
				Body: &companion.Body{
					ID: id, Dimension: core.Overworld, Position: [3]float32{0.5, 1, 0.5},
				},
				SpawnDimension: core.Overworld,
			})
			first := engine.Step()
			if !reflect.DeepEqual(first.Acquire, []core.ChunkKey{key}) {
				t.Fatalf("首次 Acquire=%+v，想要 [%+v]", first.Acquire, key)
			}

			failed := test.reachFailure(t, engine, key)
			if !reflect.DeepEqual(failed.Acquire, []core.ChunkKey{key}) || len(failed.Companions) != 0 {
				t.Fatalf("失败后 Acquire=%+v Companions=%+v，想要重试且不激活",
					failed.Acquire, failed.Companions)
			}
			advanceRuntimeCompanionRestoreToGenerating(t, engine, key)
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension,
				Pos:       key.Pos,
				Chunk:     movementFlatChunk(key.Pos),
			})
			restored := engine.Step().Companions
			if len(restored) != 1 || restored[0].ID != id ||
				restored[0].State.Position != (mgl32.Vec3{0.5, 1, 0.5}) {
				t.Fatalf("重试成功后未恢复: %+v", restored)
			}
		})
	}
}

func advanceRuntimeCompanionRestoreToGenerating(t *testing.T, engine *Engine, key core.ChunkKey) {
	t.Helper()
	engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
	result := engine.Step()
	if !reflect.DeepEqual(result.Generate, []core.ChunkKey{key}) || len(result.Companions) != 0 {
		t.Fatalf("Generate=%+v Companions=%+v，想要生成且不激活", result.Generate, result.Companions)
	}
}

func TestCompanionInterestIsThreeByThreeAndDoesNotConsumeSessions(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 0, 0, 0)
	id := runtimeCompanionTestID(1)
	position := mgl32.Vec3{8.5, 1, 8.5}
	engine.RegisterCompanion(CompanionRestore{
		ID: id,
		Body: &companion.Body{
			ID: id, Dimension: core.Overworld, Position: position,
		},
		SpawnDimension: core.Overworld,
	})
	result := engine.Step()

	if len(engine.subscriptions) != 0 || len(result.Forget) != 0 {
		t.Fatalf("伙伴污染玩家 session: sessions=%d forget=%+v", len(engine.subscriptions), result.Forget)
	}
	if len(result.Companions) != 1 || len(engine.wanted) != 9 {
		t.Fatalf("active updates/interest=%d/%d，想要 1/9", len(result.Companions), len(engine.wanted))
	}
	if len(result.Acquire) != 8 || result.Acquire[0].Pos != (core.ChunkPos{X: -1}) {
		t.Fatalf("伙伴独占区块未按伙伴距离排序: %+v", result.Acquire)
	}
	for dz := int32(-1); dz <= 1; dz++ {
		for dx := int32(-1); dx <= 1; dx++ {
			if !engine.WantsChunk(core.ChunkKey{
				Dimension: core.Overworld,
				Pos:       core.ChunkPos{X: dx, Z: dz},
			}) {
				t.Fatalf("active 兴趣缺少 (%d,%d)", dx, dz)
			}
		}
	}
	if engine.WantsChunk(core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 2}}) {
		t.Fatal("active 兴趣越过 3x3")
	}
}
