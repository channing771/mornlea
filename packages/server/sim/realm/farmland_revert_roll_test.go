package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestFarmlandRevertRollDeterministic(t *testing.T) {
	position := core.BlockPos{X: 5, Y: 1, Z: -3}
	first := sampler.FarmlandRevertRoll(12345, 99, core.Overworld, position)
	second := sampler.FarmlandRevertRoll(12345, 99, core.Overworld, position)
	if first != second {
		t.Fatalf("同一输入两次判定不一致 %v vs %v", first, second)
	}
	otherTick := sampler.FarmlandRevertRoll(12345, 100, core.Overworld, position)
	if first == otherTick {
		otherDimension := sampler.FarmlandRevertRoll(12345, 99, core.DimensionID(1), position)
		if first == otherDimension {
			t.Logf("相邻 tick/维度下判定均相同（偶然），测试仍通过")
		}
	}
}
