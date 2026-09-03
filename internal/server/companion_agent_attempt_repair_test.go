package server

import (
	"testing"

	"github.com/channing771/mornlea/internal/companion"
)

func TestCompanionPlannerUnmatchedOutcomeDoesNotClearActiveGate(t *testing.T) {
	t.Run("stale generation", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		outcome.generation++
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		assertRepairPlanningState(t, slot)
	})

	t.Run("stale attempt", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		outcome.attempt++
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		assertRepairPlanningState(t, slot)
	})

	t.Run("terminal state", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		slot.queue.FailPlanning(companion.TaskFailPlannerUnavailable)
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		if _, ok := slot.queue.Current(); ok {
			t.Fatal("terminal state changed by stale outcome")
		}
	})

	t.Run("empty bridge identity", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		outcome.result.RunID = ""
		outcome.result.SnapshotID = ""
		outcome.result.SnapshotDigest = ""
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		assertRepairPlanningState(t, slot)
	})

	t.Run("mismatched snapshot digest", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		outcome.result.RunID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		outcome.result.SnapshotID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		outcome.result.SnapshotDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		assertRepairPlanningState(t, slot)
	})

	t.Run("mismatched snapshot id", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		outcome.result.SnapshotID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		assertRepairPlanningState(t, slot)
	})

	t.Run("mismatched result attempt", func(t *testing.T) {
		host, definition, manager, outcome := repairPlanningOutcome(t)
		slot := manager.slots[definition.ID]
		outcome.result.Attempt++
		manager.applyPlannerOutcome(outcome)
		assertRepairPlanningGateHeld(t, host, manager, slot, definition.ID)
		assertRepairPlanningState(t, slot)
	})
}

func repairPlanningOutcome(t *testing.T) (*Host, companion.Definition, *companionManager, plannerOutcome) {
	t.Helper()
	host, definition := newAgentRevalidationHost(t)
	manager := host.world.companionManager
	manager.refreshBodies()
	body, active := manager.body(definition.ID)
	if !active {
		t.Fatal("companion body inactive")
	}
	plan := companion.Plan{Summary: "前进", Steps: []companion.PlanStep{{
		Kind: companion.PlanStepGoTo,
		X:    int32(body.Position[0]) + 1, Y: int32(body.Position[1]), Z: int32(body.Position[2]),
	}}}
	outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
	return host, definition, manager, outcome
}

func assertRepairPlanningGateHeld(
	t *testing.T,
	host *Host,
	manager *companionManager,
	slot *companionTaskSlot,
	id companion.ID,
) {
	t.Helper()
	if !slot.planningInFlight {
		t.Fatal("unmatched outcome cleared active planning gate")
	}
	dialogue := newFakeDialogueModel(t)
	manager.replaceDialogueForTest(t, dialogue)
	manager.requestDialogue(id, companion.DialogueNode{Kind: companion.DialogueNodeStart})
	if requests, _, _ := dialogue.snapshotCounts(); requests != 0 || slot.dialogueInFlight {
		t.Fatalf("unmatched outcome opened Dialogue gate: requests=%d inFlight=%v",
			requests, slot.dialogueInFlight)
	}
	_ = host
}

func assertRepairPlanningState(t *testing.T, slot *companionTaskSlot) {
	t.Helper()
	current, ok := slot.queue.Current()
	if !ok || current.State != companion.TaskPlanning {
		t.Fatalf("current=%+v ok=%v，want Planning unchanged", current, ok)
	}
}
