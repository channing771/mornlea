package fluid

import (
	"fmt"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是 `Replaceable` 谓词里「短草」主题的一支（natural-grass-seeds 引入的
// 流体规则语义变更）：短草格对流动水可替换，水淹即清除且零掉落。与
// rules_replaceable_crop_test.go 的关系是植物语义的第二名成员——判定面按
// `core.IsPlant` 收口（作物 ∪ 短草），这里钉住短草的完整边界与「短草脚下
// 的草方块仍然挡水」这条直接对照。

// TestReplaceable_ShortGrassReplaceableAtAllLevels 断言 `ShortGrassID` 对全部
// 七档新水等级一律可替换：newLevel=1 是垂直优先与源的水平传播产出的最强
// 写入，newLevel=7 是水平递减下界，两端放行即覆盖全部中间等级（判定表没有
// 按等级分叉的独立路径）。短草被覆盖后的零掉落结算是权威写入侧（sim）的
// 职责，本包只回答「能不能写」。
func TestReplaceable_ShortGrassReplaceableAtAllLevels(t *testing.T) {
	for level := uint8(1); level <= 7; level++ {
		t.Run(fmt.Sprintf("短草对等级%d可替换", level), func(t *testing.T) {
			if got := Replaceable(core.ShortGrassID, level); !got {
				t.Errorf("Replaceable(ShortGrassID, %d) = false, want true：短草应被流动水覆盖清除", level)
			}
		})
	}
}

// TestReplaceable_GrassBlockStillBlocks 是短草放行的对照组：短草脚下的草方块
// （`GrassID`）必须仍然不可替换。短草只长在草方块上，这条对照钉住「清草不
// 清草皮」的玩法边界——水流过草地会除掉植株，但不能把地表也冲走。
func TestReplaceable_GrassBlockStillBlocks(t *testing.T) {
	for _, level := range []uint8{1, 7} {
		t.Run(fmt.Sprintf("草方块对等级%d不可替换", level), func(t *testing.T) {
			if got := Replaceable(core.GrassID, level); got {
				t.Errorf("Replaceable(GrassID, %d) = true, want false：草方块仍应挡水", level)
			}
		})
	}
}
