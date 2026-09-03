package companion

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestDialogueEnvDigestValidate(t *testing.T) {
	valid := DialogueEnvDigest{
		ExposedBlocks: []PlanBlock{
			{Pos: core.BlockPos{X: -1, Y: 1, Z: 0}, Block: core.StoneID},
			{Pos: core.BlockPos{X: 1, Y: 1, Z: 0}, Block: core.DirtID},
		},
		Heights: []PlanHeight{{X: -1, Z: 0, Height: 1}, {X: 1, Z: 0, Height: core.MinY - 1}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid digest: %v", err)
	}
	invalid := valid
	invalid.ExposedBlocks = append([]PlanBlock(nil), valid.ExposedBlocks...)
	invalid.ExposedBlocks[1].Pos = invalid.ExposedBlocks[0].Pos
	if err := invalid.Validate(); err == nil {
		t.Fatal("duplicate exposed block position accepted")
	}
}
