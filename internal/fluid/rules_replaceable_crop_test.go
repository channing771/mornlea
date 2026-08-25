package fluid

import (
	"fmt"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件是 Replaceable 谓词里「作物」主题的一支（flood-destroys-crops 引入
// 的唯一规则语义变更）：作物格对流动水可替换，冲毁即淹没。与
// replaceable_test.go 的关系是边界互补而非重复——那边钉住一般判定表，这边
// 钉住作物放行的完整边界（八个生长阶段 × 最强/最弱两档新等级）以及放行
// 之后「非作物实心」分支的剩余覆盖面。

// TestReplaceable_CropStages 断言小麦的八个生长阶段对流动水一律可替换。
//
// 阶段编号逐个显式枚举而不是用 core.IsCrop 推导区间端点：测试不得与实现
// 共用同一段闭区间算式，否则 IsCrop 的上下界写错时测试会跟着一起错
// （e2e_test.go 用具名常量数组对抗同一类风险）。两档新等级取判定表的两个
// 极端：newLevel=1 是垂直优先与源的水平传播产出的最强写入，newLevel=7 是
// 水平递减的下界——两端都放行，中间等级没有独立的代码路径。
func TestReplaceable_CropStages(t *testing.T) {
	stages := []core.BlockID{
		core.WheatStage0ID,
		core.WheatStage1ID,
		core.WheatStage2ID,
		core.WheatStage3ID,
		core.WheatStage4ID,
		core.WheatStage5ID,
		core.WheatStage6ID,
		core.WheatStage7ID,
	}
	for _, stage := range stages {
		for _, level := range []uint8{1, 7} {
			t.Run(fmt.Sprintf("WheatStage%d 对等级%d可替换", stage-core.WheatStage0ID, level), func(t *testing.T) {
				if got := Replaceable(stage, level); !got {
					t.Errorf("Replaceable(小麦阶段%d, %d) = false, want true：作物应被流动水冲毁",
						stage-core.WheatStage0ID, level)
				}
			})
		}
	}
}

// TestReplaceable_NonCropSolidsStillBlocked 是作物放行的对照组：耕地两种
// 形态与泥土等非作物实心方块必须仍然不可替换。
//
// 耕地是这条对照的重点——它紧贴作物正下方，是「冲毁作物但不破坏农田」
// 这条玩法边界的直接受害者；两态各测最强/最弱两档新等级，钉住放行的
// 精确上边界恰好停在 core.WheatStage7ID。
func TestReplaceable_NonCropSolidsStillBlocked(t *testing.T) {
	cases := []struct {
		name   string
		target core.BlockID
	}{
		{"干耕地不可替换", core.FarmlandDryID},
		{"湿耕地不可替换", core.FarmlandWetID},
		{"泥土不可替换", core.DirtID},
	}
	for _, c := range cases {
		for _, level := range []uint8{1, 7} {
			t.Run(fmt.Sprintf("%s 对等级%d", c.name, level), func(t *testing.T) {
				if got := Replaceable(c.target, level); got {
					t.Errorf("Replaceable(%v, %d) = true, want false：非作物实心方块仍应挡水",
						c.target, level)
				}
			})
		}
	}
}
