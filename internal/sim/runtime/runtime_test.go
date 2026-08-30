package runtime

import (
	"reflect"
	"sync"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

func TestRuntimeStepPhaseOrder(t *testing.T) {
	engine := NewEngine(1, 0, 0)
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

func TestRuntimeInboxConcurrentEnqueueAndStableCommandSort(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	// 并发 Enqueue 多个命令，验证串行 Step 后的稳定排序按 Session/Sequence
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(seq uint64) {
			defer wg.Done()
			engine.Enqueue(Command{Session: 1, Sequence: seq, Kind: CommandResync, Dimension: core.Overworld, Chunk: core.ChunkPos{X: int32(seq)}})
		}(uint64(i + 1))
	}
	wg.Wait()
	result := engine.Step()
	if len(result.Resync) != 10 {
		t.Fatalf("Resync 数量=%d，想要 10", len(result.Resync))
	}
	for i := 1; i < len(result.Resync); i++ {
		if result.Resync[i].Sequence < result.Resync[i-1].Sequence {
			t.Fatalf("命令未稳定排序: %+v", result.Resync)
		}
	}
}

func TestRuntimeMutationCommitOnceAndPublishOrder(t *testing.T) {
	engine := NewEngine(1, 0, 0)
	engine.RegisterObserverSession(1)
	engine.Enqueue(Command{Session: 1, Sequence: 1, Kind: CommandTrustedObserverCenter, Dimension: core.Overworld, Center: core.ChunkPos{}})
	result := engine.Step()
	// 验证一次 mutation 提交后的发布顺序：Ready 已排序，Tick 单调
	if result.Tick != 1 {
		t.Fatalf("Tick=%d，想要 1", result.Tick)
	}
	// 多次 Step 保证 worldTime 单调，tick +1
	prevTick := result.Tick
	prevWorldTime := result.WorldTimeTicks
	result2 := engine.Step()
	if result2.Tick != prevTick+1 || result2.WorldTimeTicks != prevWorldTime+1 {
		t.Fatalf("Tick/WorldTime 未单调: %+v %+v", result, result2)
	}
}

func TestRuntimeComposesRealmAndEntity(t *testing.T) {
	engine := NewEngine(1, 42, 0x1234)
	if engine.realm == nil {
		t.Fatalf("realm.State 未组合")
	}
	if engine.entities == nil {
		t.Fatalf("entity.State 未组合")
	}
	if engine.SeedForTest() != 0x1234 {
		t.Fatalf("seed 未透传")
	}
	// 验证 subscriptions 与时钟探针存在
	if engine.viewRadius != 1 {
		t.Fatalf("viewRadius=%d，想要 1", engine.viewRadius)
	}

	t.Run("玩家状态来自 entity owner", func(t *testing.T) {
		engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
		runtimeHash, ok := engine.PlayerHash(1)
		if !ok {
			t.Fatal("runtime 未登记玩家")
		}
		entityHash, ok := engine.entities.PlayerHash(1)
		if !ok {
			t.Fatal("runtime 登记的玩家在 entity owner 中不可见")
		}
		if entityHash != runtimeHash {
			t.Fatalf("玩家权威状态分叉：runtime=%x entity=%x", runtimeHash, entityHash)
		}
	})

	t.Run("夜行者状态来自 entity owner", func(t *testing.T) {
		mob := HostileMob{
			ID:        1,
			Dimension: core.Overworld,
			State: physics.State{
				Position: [3]float32{0.5, 1, 0.5},
				OnGround: true,
			},
			Health:       core.MaxHealth,
			BurnCooldown: 20,
		}
		if err := engine.RestoreHostile(mob); err != nil {
			t.Fatalf("runtime 恢复夜行者：%v", err)
		}
		ownerMobs := engine.entities.HostileMobs()
		if len(ownerMobs) != 1 || ownerMobs[0] != mob {
			t.Fatalf("runtime 恢复的夜行者在 entity owner 中不一致：%+v", ownerMobs)
		}
	})

	t.Run("runtime 不保留实体镜像字段", func(t *testing.T) {
		engineType := reflect.TypeOf(*engine)
		forbidden := map[string]struct{}{
			"sessions": {}, "companions": {}, "hostiles": {}, "hostileLight": {},
			"dropKeySeen": {}, "dropKeyScratch": {}, "containerViewerScratch": {},
			"dropSessionScratch": {}, "tramplePending": {},
		}
		var realmOwners, entityOwners int
		for index := 0; index < engineType.NumField(); index++ {
			field := engineType.Field(index)
			if _, mirrored := forbidden[field.Name]; mirrored {
				t.Errorf("runtime.Engine 仍保留实体权威字段 %q", field.Name)
			}
			if field.Type.Kind() != reflect.Pointer {
				continue
			}
			elem := field.Type.Elem()
			switch {
			case elem.PkgPath() == "github.com/channing771/mornlea/internal/sim/realm" && elem.Name() == "State":
				realmOwners++
			case elem.PkgPath() == "github.com/channing771/mornlea/internal/sim/entity" && elem.Name() == "State":
				entityOwners++
			}
		}
		if realmOwners != 1 || entityOwners != 1 {
			t.Fatalf("runtime.Engine 组合 owner 数量 realm=%d entity=%d，想要各 1", realmOwners, entityOwners)
		}
	})
}
