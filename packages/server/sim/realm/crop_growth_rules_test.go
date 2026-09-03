package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestGrowCropIsExhaustivelySpecified(t *testing.T) {
	stages := [8]core.BlockID{
		core.WheatStage0ID, core.WheatStage1ID, core.WheatStage2ID, core.WheatStage3ID,
		core.WheatStage4ID, core.WheatStage5ID, core.WheatStage6ID, core.WheatStage7ID,
	}
	wantNext := [8]core.BlockID{
		core.WheatStage1ID, core.WheatStage2ID, core.WheatStage3ID, core.WheatStage4ID,
		core.WheatStage5ID, core.WheatStage6ID, core.WheatStage7ID, core.WheatStage7ID,
	}
	wantChanged := [8]bool{true, true, true, true, true, true, true, false}
	for stage, block := range stages {
		for _, environment := range []struct{ wet, sky bool }{
			{true, true}, {true, false}, {false, true}, {false, false},
		} {
			next, changed := growCrop(block, environment.wet, environment.sky)
			expectNext, expectChanged := block, false
			if environment.wet && environment.sky {
				expectNext, expectChanged = wantNext[stage], wantChanged[stage]
			}
			if next != expectNext || changed != expectChanged {
				t.Errorf("growCrop(阶段 %d, wet=%v, sky=%v) = (%d, %v)，想要 (%d, %v)",
					stage, environment.wet, environment.sky, next, changed, expectNext, expectChanged)
			}
		}
	}
}

func TestGrowCropLeavesNonCropsAlone(t *testing.T) {
	for _, block := range []core.BlockID{
		core.AirID, core.StoneID, core.DirtID, core.GrassID,
		core.FarmlandDryID, core.FarmlandWetID, core.WaterSourceID, core.WaterLevel1ID,
	} {
		next, changed := growCrop(block, true, true)
		if next != block || changed {
			t.Errorf("growCrop(%d) = (%d, %v)，非作物必须原样返回", block, next, changed)
		}
	}
}
