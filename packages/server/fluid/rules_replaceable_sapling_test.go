package fluid

import (
	"fmt"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是 `Replaceable` 谓词里「树苗」主题的一支（oak-sapling-regrowth 引入
// 的流体规则语义变更）：树苗格对流动水可替换，水淹即冲毁并掉落 1 个树苗——
// 掉落结算由权威写入侧（sim）完成，本包只回答「能不能写」。与
// rules_replaceable_grass_test.go 的关系是植物语义的第三名成员：判定面按
// `core.IsPlant` 收口（作物 ∪ 短草 ∪ 树苗），这里钉住树苗的完整边界与「树苗
// 脚下的支撑方块仍然挡水」这条直接对照。

// TestReplaceable_SaplingReplaceableAtAllLevels 断言 `SaplingID` 对全部七档新水
// 等级一律可替换：newLevel=1 是垂直优先与源的水平传播产出的最强写入，
// newLevel=7 是水平递减下界，两端放行即覆盖全部中间等级（判定表没有按等级
// 分叉的独立路径）。树苗被冲毁后的「掉落 1 个自身、容量不足原子拒绝」结算是
// 权威写入侧（sim）的职责，本包只回答「能不能写」。
func TestReplaceable_SaplingReplaceableAtAllLevels(t *testing.T) {
	for level := uint8(1); level <= 7; level++ {
		t.Run(fmt.Sprintf("树苗对等级%d可替换", level), func(t *testing.T) {
			if got := Replaceable(core.SaplingID, level); !got {
				t.Errorf("Replaceable(SaplingID, %d) = false, want true：树苗应被流动水覆盖冲毁", level)
			}
		})
	}
}

// TestReplaceable_SaplingSupportsStillBlock 是树苗放行的对照组：树苗脚下的泥土
// 与草方块必须仍然不可替换。水冲过树苗格会冲毁植株并掉出树苗，但不能把地表
// 也冲走——支撑白名单与种植规则同源（只有泥土与草能长树苗），水流过之后地表
// 必须还在，否则被冲毁的树苗连重种的落脚点都没有。
func TestReplaceable_SaplingSupportsStillBlock(t *testing.T) {
	cases := []struct {
		name   string
		target core.BlockID
	}{
		{"泥土不可替换", core.DirtID},
		{"草方块不可替换", core.GrassID},
	}
	for _, c := range cases {
		for _, level := range []uint8{1, 7} {
			t.Run(fmt.Sprintf("%s 对等级%d", c.name, level), func(t *testing.T) {
				if got := Replaceable(c.target, level); got {
					t.Errorf("Replaceable(%v, %d) = true, want false：树苗支撑方块仍应挡水",
						c.target, level)
				}
			})
		}
	}
}
