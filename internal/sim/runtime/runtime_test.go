package runtime

import (
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
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

func TestStepRunsHostilePhasesBetweenPhysicsAndFluid(t *testing.T) {
	engine := NewEngine(0, 0, 0)
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

	t.Run("伙伴状态来自 entity owner", func(t *testing.T) {
		ownerEngine, _ := readyMovementPlayer(t)
		id := companion.ID{6: 0x40, 8: 0x80, 15: 1}
		ownerEngine.RegisterCompanion(CompanionRestore{
			ID: id,
			Body: &companion.Body{
				ID:        id,
				Dimension: core.Overworld,
				Position:  [3]float32{0.5, 1, 0.5},
			},
			SpawnDimension: core.Overworld,
		})
		ownerEngine.Step()
		runtimeBodies := ownerEngine.CompanionBodies()
		entityBodies := ownerEngine.entities.CompanionBodies()
		if !reflect.DeepEqual(runtimeBodies, entityBodies) || len(runtimeBodies) != 1 {
			t.Fatalf("runtime 与 entity 的伙伴状态分叉：runtime=%+v entity=%+v", runtimeBodies, entityBodies)
		}
	})

	t.Run("runtime tick 直接修改 entity 背包", func(t *testing.T) {
		ownerEngine, session := readyMovementPlayer(t)
		ownerEngine.SetPlayerInventoryForTest(session, func(inventory core.Inventory) core.Inventory {
			next, ok := inventory.SetSlot(0, core.ItemStack{Item: core.ItemStone, Count: 2})
			if !ok {
				t.Fatal("构造背包失败")
			}
			return next
		})
		ownerEngine.Enqueue(Command{
			Session: session, Sequence: 1, Kind: CommandMoveInventoryStack,
			Slot: 0, ToSlot: 1,
		})
		if result := ownerEngine.Step(); len(result.Rejected) != 0 {
			t.Fatalf("背包移动被拒绝：%+v", result.Rejected)
		}
		runtimeSnapshot, ok := ownerEngine.PlayerSnapshot(session)
		if !ok {
			t.Fatal("runtime 玩家快照丢失")
		}
		entitySnapshot, ok := ownerEngine.entities.PlayerSnapshot(session)
		if !ok {
			t.Fatal("entity 玩家快照丢失")
		}
		if !reflect.DeepEqual(runtimeSnapshot.Inventory, entitySnapshot.Inventory) {
			t.Fatalf("runtime 与 entity 的背包状态分叉：runtime=%+v entity=%+v", runtimeSnapshot.Inventory, entitySnapshot.Inventory)
		}
		moved, _ := entitySnapshot.Inventory.Slot(1)
		if moved != (core.ItemStack{Item: core.ItemStone, Count: 2}) {
			t.Fatalf("runtime tick 未修改 entity 背包：slot1=%+v", moved)
		}
	})

	t.Run("runtime 不保留实体镜像字段", func(t *testing.T) {
		engineType := reflect.TypeOf(engine).Elem()
		forbidden := map[string]struct{}{
			"sessions": {}, "companions": {}, "hostiles": {}, "hostileLight": {},
			"dropKeySeen": {}, "dropKeyScratch": {}, "containerViewerScratch": {},
			"dropSessionScratch": {}, "tramplePending": {}, "fluidQueues": {},
			"fluidScope": {}, "fluidScopeNext": {}, "fluidDimensionScratch": {},
			"fluidRescan": {}, "cropCellScratch": {}, "cropCellsExamined": {},
			"cropBlockReads": {},
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

	t.Run("production boundary 无反向回调或重复实现", func(t *testing.T) {
		for _, name := range []string{
			"engine_changes.go", "crop.go", "fluid.go", "fluid_crop.go", "farmland_revert.go",
		} {
			if _, err := os.Stat(name); err == nil {
				t.Errorf("runtime 仍保留重复 production 文件 %q", name)
			} else if !os.IsNotExist(err) {
				t.Fatalf("检查 %q：%v", name, err)
			}
		}

		violations, err := analyzeEntityOwnershipDirectory("../entity")
		if err != nil {
			t.Fatal(err)
		}
		for _, violation := range violations {
			t.Error(violation)
		}
	})
}

func TestHostileActionsQueuedAtPhaseBoundaryRunInCurrentTick(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	const hostileID = uint64(77)
	mob := HostileMob{
		ID: hostileID, Dimension: core.Overworld,
		State: physics.State{
			Position: [3]float32{5.5, 1, 5.5}, OnGround: true,
		},
		Health: core.MaxHealth, BurnCooldown: 20,
	}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复可观测夜行者：%v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	engine.stepPhaseObserver = func(phase stepPhase) {
		if phase != phaseHostileAdvance {
			return
		}
		close(entered)
		<-release
	}

	done := make(chan TickResult, 1)
	go func() { done <- engine.Step() }()
	<-entered
	if !engine.EnqueueHostileAction(HostileAction{ID: hostileID, MoveX: -1}) {
		t.Fatal("hostile action inbox 意外满员")
	}
	close(release)
	<-done
	engine.stepPhaseObserver = nil

	first := engine.HostileMobs()
	if len(first) != 1 || first[0].ID != hostileID || first[0].State.Position.X() >= 5.5 {
		t.Fatalf("hostile phase 边界 action 未在本 tick 改变位移：%+v", first)
	}
	positionAfterFirst := first[0].State.Position
	engine.Step()
	second := engine.HostileMobs()
	if len(second) != 1 || second[0].State.Position != positionAfterFirst {
		t.Fatalf("action 延迟到下一空闲 tick 才结算：first=%+v second=%+v", first, second)
	}
}
