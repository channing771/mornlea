package server

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestAgentPlannerOutcomeRevalidatesCurrentWorldAtTick(t *testing.T) {
	t.Run("dense mine target changed", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		body, active := manager.body(definition.ID)
		if !active {
			t.Fatal("companion body inactive")
		}
		target := core.BlockPos{
			X: int32(body.Position[0]) + 1,
			Y: int32(body.Position[1]) - 1,
			Z: int32(body.Position[2]),
		}
		host.world.engine.SetBlockForTest(target, core.ChestID)
		plan := companion.Plan{Summary: "采集容器", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepMine, X: target.X, Y: target.Y, Z: target.Z,
		}}}
		outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
		// 两者都属于 frozen validator 允许采掘的容器；必须依赖 dense target
		// 精确槽变化而不是 exposed projection 才能拒绝。
		host.world.engine.SetBlockForTest(target, core.FurnaceID)
		manager.applyPlannerOutcome(outcome)
		facts := manager.takeEventFacts()
		assertAgentWorldChanged(t, facts)
	})

	t.Run("place inventory changed", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		body := manager.bodies[definition.ID]
		body.Inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 1}
		manager.bodies[definition.ID] = body
		target := core.BlockPos{X: int32(body.Position[0]) + 1, Y: int32(body.Position[1]), Z: int32(body.Position[2])}
		plan := companion.Plan{Summary: "放木板", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepPlace, X: target.X, Y: target.Y, Z: target.Z, Block: core.OakPlanksID,
		}}}
		outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
		body.Inventory = core.Inventory{}
		manager.bodies[definition.ID] = body
		manager.applyPlannerOutcome(outcome)
		facts := manager.takeEventFacts()
		assertAgentWorldChanged(t, facts)
	})

	t.Run("place target changed from air to occupied", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		body := manager.bodies[definition.ID]
		body.Inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 1}
		manager.bodies[definition.ID] = body
		target := core.BlockPos{
			X: int32(body.Position[0]) + 1,
			Y: int32(body.Position[1]),
			Z: int32(body.Position[2]),
		}
		host.world.engine.SetBlockForTest(target, core.AirID)
		plan := companion.Plan{Summary: "放木板", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepPlace, X: target.X, Y: target.Y, Z: target.Z,
			Block: core.OakPlanksID,
		}}}
		outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
		host.world.engine.SetBlockForTest(target, core.StoneID)
		manager.applyPlannerOutcome(outcome)
		assertAgentWorldChanged(t, manager.takeEventFacts())
	})

	t.Run("follow target offline", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		issuer := stopTestIssuer(integrationIdentity(0x72, "跟随目标"))
		manager.onlinePlayers = func() []companion.PlanPlayer {
			return []companion.PlanPlayer{{ID: issuer.playerID, Position: issuer.position}}
		}
		plan := companion.Plan{Summary: "跟随玩家", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepFollow, PlayerID: issuer.playerID,
		}}}
		outcome := prepareAgentRevalidationOutcomeWithIssuer(t, manager, definition, issuer, plan)
		manager.onlinePlayers = func() []companion.PlanPlayer { return nil }
		manager.applyPlannerOutcome(outcome)
		facts := manager.takeEventFacts()
		assertAgentWorldChanged(t, facts)
	})

	t.Run("follow target moved", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		issuer := stopTestIssuer(integrationIdentity(0x73, "移动目标"))
		position := issuer.position
		manager.onlinePlayers = func() []companion.PlanPlayer {
			return []companion.PlanPlayer{{ID: issuer.playerID, Position: position}}
		}
		plan := companion.Plan{Summary: "跟随玩家", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepFollow, PlayerID: issuer.playerID,
		}}}
		outcome := prepareAgentRevalidationOutcomeWithIssuer(t, manager, definition, issuer, plan)
		position[0] += 2
		manager.applyPlannerOutcome(outcome)
		assertAgentWorldChanged(t, manager.takeEventFacts())
	})

	t.Run("planned chunk revision changed", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		body := manager.bodies[definition.ID]
		body.Inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 1}
		manager.bodies[definition.ID] = body
		target := core.BlockPos{
			X: int32(body.Position[0]) + 1,
			Y: int32(body.Position[1]),
			Z: int32(body.Position[2]),
		}
		plan := companion.Plan{Summary: "放木板", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepPlace, X: target.X, Y: target.Y, Z: target.Z,
			Block: core.OakPlanksID,
		}}}
		outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
		neighbor := target
		if target.X&core.SectionMask == core.SectionMask {
			neighbor.X--
		} else {
			neighbor.X++
		}
		host.world.engine.SetBlockForTest(neighbor, core.ChestID)
		host.world.engine.TouchChunkForTest(core.ChunkKey{
			Dimension: core.Overworld,
			Pos:       target.Chunk(),
		})
		manager.applyPlannerOutcome(outcome)
		assertAgentWorldChanged(t, manager.takeEventFacts())
	})

	t.Run("unrelated projection block changed", func(t *testing.T) {
		host, definition := newAgentRevalidationHost(t)
		host.world.stepMu.Lock()
		defer host.world.stepMu.Unlock()
		manager := host.world.companionManager
		manager.refreshBodies()
		body := manager.bodies[definition.ID]
		target := core.BlockPos{
			X: int32(body.Position[0]) + 1,
			Y: int32(body.Position[1]),
			Z: int32(body.Position[2]),
		}
		plan := companion.Plan{Summary: "前进", Steps: []companion.PlanStep{{
			Kind: companion.PlanStepGoTo, X: target.X, Y: target.Y, Z: target.Z,
		}}}
		outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
		unrelated := target
		unrelated.X -= core.SectionSize
		if unrelated.Chunk() == target.Chunk() {
			t.Fatalf("unrelated chunk=%v equals target chunk", unrelated.Chunk())
		}
		host.world.engine.SetBlockForTest(unrelated, core.ChestID)
		manager.applyPlannerOutcome(outcome)
		assertAgentStarted(t, manager.takeEventFacts())
	})

	for _, testCase := range []struct {
		name  string
		block core.BlockID
	}{
		{name: "chest", block: core.ChestID},
		{name: "furnace", block: core.FurnaceID},
	} {
		testCase := testCase
		t.Run("unchanged dense container remains valid "+testCase.name, func(t *testing.T) {
			host, definition := newAgentRevalidationHost(t)
			host.world.stepMu.Lock()
			defer host.world.stepMu.Unlock()
			manager := host.world.companionManager
			manager.refreshBodies()
			body := manager.bodies[definition.ID]
			target := core.BlockPos{
				X: int32(body.Position[0]) + 1,
				Y: int32(body.Position[1]) - 1,
				Z: int32(body.Position[2]),
			}
			host.world.engine.SetBlockForTest(target, testCase.block)
			plan := companion.Plan{Summary: "采集容器", Steps: []companion.PlanStep{{
				Kind: companion.PlanStepMine, X: target.X, Y: target.Y, Z: target.Z,
			}}}
			outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
			manager.applyPlannerOutcome(outcome)
			assertAgentStarted(t, manager.takeEventFacts())
		})
	}
}

func newAgentRevalidationHost(t *testing.T) (*Host, companion.Definition) {
	t.Helper()
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host := newCompanionManagerHost(t, []companion.Definition{definition}, nil, nil)
	waitIntegrationCondition(t, "companion body activation", func() bool {
		host.world.StepForTest()
		host.world.stepMu.Lock()
		_, active := host.world.companionManager.body(definition.ID)
		host.world.stepMu.Unlock()
		return active
	})
	return host, definition
}

func prepareAgentRevalidationOutcome(
	t *testing.T,
	manager *companionManager,
	definition companion.Definition,
	plan companion.Plan,
) plannerOutcome {
	t.Helper()
	return prepareAgentRevalidationOutcomeWithIssuer(
		t, manager, definition, stopTestIssuer(integrationIdentity(0x71, "发令者")), plan,
	)
}

func prepareAgentRevalidationOutcomeWithIssuer(
	t *testing.T,
	manager *companionManager,
	definition companion.Definition,
	issuer companionTaskIssuer,
	plan companion.Plan,
) plannerOutcome {
	t.Helper()
	slot := manager.slots[definition.ID]
	if !manager.enqueueCommand(definition, companion.TaskCommand("执行计划"), issuer) || !slot.queue.BeginHead() {
		t.Fatal("prepare planning queue failed")
	}
	slot.currentIssuer, slot.issuers = slot.issuers[0], slot.issuers[1:]
	current, _ := slot.queue.Current()
	slot.currentCommand = current.Command
	if !slot.queue.BeginPlanning() {
		t.Fatal("BeginPlanning=false")
	}
	body, active := manager.body(definition.ID)
	if !active {
		t.Fatal("companion body inactive")
	}
	snapshot, err := manager.buildPlanSnapshot(definition, current.Command, issuer, body)
	if err != nil {
		t.Fatalf("buildPlanSnapshot: %v", err)
	}
	if err := companion.ValidatePlanAgainstSnapshot(plan, snapshot); err != nil {
		t.Fatalf("frozen plan invalid: %v", err)
	}
	_, snapshotDigest, err := companion.CanonicalSnapshotDigest(snapshot)
	if err != nil {
		t.Fatalf("CanonicalSnapshotDigest: %v", err)
	}
	slot.planningAttempt++
	slot.planningInFlight = true
	return plannerOutcome{
		id: definition.ID, generation: slot.queue.Generation(), attempt: slot.planningAttempt,
		snapshot: snapshot, snapshotDigest: snapshotDigest,
		result: companionPlanningOutcome{
			Plan: plan, Generation: slot.queue.Generation(), Attempt: slot.planningAttempt,
			RunID:          "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			SnapshotID:     "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			SnapshotDigest: snapshotDigest,
			requestIdentity: companionPlanningIdentity{
				RunID:          "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				SnapshotID:     "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
				SnapshotDigest: snapshotDigest,
			},
		},
	}
}

func assertAgentWorldChanged(t *testing.T, facts []taskEventFact) {
	t.Helper()
	if len(facts) != 1 || facts[0].event.Kind != companion.TaskEventFailed ||
		facts[0].event.Reason != companion.TaskFailWorldChanged {
		t.Fatalf("facts=%+v，want TaskFailed(WorldChanged)", facts)
	}
}

func assertAgentStarted(t *testing.T, facts []taskEventFact) {
	t.Helper()
	if len(facts) != 1 || facts[0].event.Kind != companion.TaskEventStarted {
		t.Fatalf("facts=%+v，want one TaskStarted", facts)
	}
}
