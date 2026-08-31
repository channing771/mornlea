package server

import (
	"testing"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
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
	slot.planningInFlight = true
	return plannerOutcome{
		id: definition.ID, generation: slot.queue.Generation(), snapshot: snapshot,
		result: companionPlanningOutcome{Plan: plan, Generation: slot.queue.Generation(), Correlated: true},
	}
}

func assertAgentWorldChanged(t *testing.T, facts []taskEventFact) {
	t.Helper()
	if len(facts) != 1 || facts[0].event.Kind != companion.TaskEventFailed ||
		facts[0].event.Reason != companion.TaskFailWorldChanged {
		t.Fatalf("facts=%+v，want TaskFailed(WorldChanged)", facts)
	}
}
