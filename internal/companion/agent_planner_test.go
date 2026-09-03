package companion

import (
	"errors"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestAgentPlannerDecodesThroughFrozenSnapshotRules(t *testing.T) {
	snapshot := testSnapshot()
	chest := core.BlockPos{X: 8, Y: 63, Z: -2}
	farming := core.BlockPos{X: 9, Y: 63, Z: -2}
	if !snapshot.Terrain.SetBlock(chest, core.ChestID) {
		t.Fatalf("SetBlock(%+v, chest)=false", chest)
	}
	if !snapshot.Terrain.SetBlock(farming, core.FarmlandDryID) {
		t.Fatalf("SetBlock(%+v, farmland)=false", farming)
	}

	plan, err := DecodeAgentPlan(AgentPlan{
		Summary: "采集箱子",
		Steps:   []AgentPlanStep{{Kind: "mine", X: chest.X, Y: chest.Y, Z: chest.Z}},
	}, snapshot)
	if err != nil {
		t.Fatalf("DecodeAgentPlan(chest): %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Block != 0 || plan.Steps[0].Kind != PlanStepMine {
		t.Fatalf("plan=%+v", plan)
	}

	_, err = DecodeAgentPlan(AgentPlan{
		Summary: "采集农田",
		Steps:   []AgentPlanStep{{Kind: "mine", X: farming.X, Y: farming.Y, Z: farming.Z}},
	}, snapshot)
	if !errors.Is(err, ErrPlannerInvalidPlan) {
		t.Fatalf("farming err=%v，want ErrPlannerInvalidPlan", err)
	}
}

func TestAgentPlannerRejectsAgentDTOThatCannotBecomeDomainPlan(t *testing.T) {
	snapshot := testSnapshot()
	cases := []AgentPlan{
		{Summary: "", Steps: []AgentPlanStep{{Kind: "go_to", X: 0, Y: 1, Z: 0}}},
		{Summary: "未知类型", Steps: []AgentPlanStep{{Kind: "attack", X: 0, Y: 1, Z: 0}}},
		{Summary: "未知方块", Steps: []AgentPlanStep{{Kind: "place", X: 0, Y: 1, Z: 0, Block: "not_registered"}}},
		{Summary: "离线跟随", Steps: []AgentPlanStep{{Kind: "follow", PlayerID: "77777777-7777-4777-8777-777777777777"}}},
	}
	for index, candidate := range cases {
		if _, err := DecodeAgentPlan(candidate, snapshot); !errors.Is(err, ErrPlannerInvalidPlan) {
			t.Fatalf("case[%d] err=%v，want ErrPlannerInvalidPlan", index, err)
		}
	}
}

func TestAgentPlannerRejectsTypedDTOExclusiveFields(t *testing.T) {
	snapshot := testSnapshot()
	cases := []AgentPlan{
		{Summary: "越权方块", Steps: []AgentPlanStep{{Kind: "go_to", X: 8, Y: 63, Z: -2, Block: "oak_planks"}}},
		{Summary: "越权玩家", Steps: []AgentPlanStep{{Kind: "mine", X: 8, Y: 63, Z: -2, PlayerID: testPlayerUUID}}},
		{Summary: "越权坐标", Steps: []AgentPlanStep{{Kind: "follow", X: 1, PlayerID: testPlayerUUID}}},
	}
	for index, candidate := range cases {
		if _, err := DecodeAgentPlan(candidate, snapshot); !errors.Is(err, ErrPlannerInvalidPlan) {
			t.Fatalf("case[%d] err=%v，want ErrPlannerInvalidPlan", index, err)
		}
	}
}
