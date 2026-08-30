package companion

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestPlanMineableBlockAllowsContainerTargets 锁定 planner 契约层对容器目标
// （箱子/熔炉）的放行（change companion-mine-containers D2）。`planMineableBlock`
// 与 internal/sim 的 `companionMineableBlock` 是同一防御清单的两处实现
// （companion 不得依赖 sim，依赖方向相反），Task 1 已在 sim 侧放开容器，本用例
// 证明计划侧同步放开：容器的产物是「本体 + 全部内容物堆」的批量全或无结算
// （任一堆放不下即整体不结算），容量安全由结算形状承担，不再由目标清单承担。
//
// 用全集枚举而不是点名断言：遍历整个方块编号域，把 `core.IsCrop`/
// `core.IsFarmland` 命中的编号逐一断言仍被显式拒绝（农业语义尚未裁决，
// 本 change 冻结），把两个容器编号断言放行；同时用计数守卫防止谓词意外空转
// 让循环空转通过。单一 `core.BlockDrop` 判据对其余方块不变：无掉落方块
// （基岩）仍拒绝、普通方块（石头）仍放行。
func TestPlanMineableBlockAllowsContainerTargets(t *testing.T) {
	farmingSeen := 0
	for block := core.BlockID(0); block < core.BlockIDMax; block++ {
		switch {
		case core.IsCrop(block) || core.IsFarmland(block):
			farmingSeen++
			if planMineableBlock(block) {
				t.Fatalf("planMineableBlock(%d) = true，农业方块必须保持显式拒绝", block)
			}
		case block == core.ChestID || block == core.FurnaceID:
			if !planMineableBlock(block) {
				t.Fatalf("planMineableBlock(%d) = false，容器必须是合法 mine 目标", block)
			}
		}
	}
	if farmingSeen != 26 {
		t.Fatalf("农业编号全集计数 = %d，want 26（谓词空转会让回归断言失效）", farmingSeen)
	}
	// 单一掉落判据回归：无掉落方块仍拒绝、普通方块仍放行，容器放行不是
	// 「全域放开」。
	if planMineableBlock(core.BedrockID) {
		t.Fatal("planMineableBlock(core.BedrockID) = true，无单一掉落的方块必须仍被拒绝")
	}
	if !planMineableBlock(core.StoneID) {
		t.Fatal("planMineableBlock(core.StoneID) = false，普通方块必须仍被放行")
	}
}
