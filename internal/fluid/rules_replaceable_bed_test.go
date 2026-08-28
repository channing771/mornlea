package fluid

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestBedCellsAreOccupiedForFluid 锁定 spec Requirement「床是双格八形态方块且
// 放置原子」的流体一半：床尾与床头两格都被流体视为占据格——八个形态对任意
// 写入等级都不可替换，流动水不得流入或冲毁床。
//
// 床不需要 Replaceable 里的专门分支：床方块不是空气、不是作物、不是流体、
// 也不是可流入的开启门，判定自然落到「非作物的实心方块不可替换」的末行。
// 本用例把这一既得语义钉成显式契约，防止未来在判定表里加分支时无意放行床。
func TestBedCellsAreOccupiedForFluid(t *testing.T) {
	bedForms := []core.BlockID{
		core.BedFootSouthID, core.BedFootWestID, core.BedFootNorthID, core.BedFootEastID,
		core.BedHeadSouthID, core.BedHeadWestID, core.BedHeadNorthID, core.BedHeadEastID,
	}
	for _, form := range bedForms {
		for _, newLevel := range []uint8{1, 4, 7} {
			if Replaceable(form, newLevel) {
				t.Fatalf("Replaceable(床形态 %d, %d) = true：床两格必须被视为占据格", form, newLevel)
			}
		}
	}
}
