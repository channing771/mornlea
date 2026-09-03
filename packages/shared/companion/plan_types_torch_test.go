package companion

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// plan_types_torch_test.go：可放置火把的伙伴防御清单——mine 侧的显式拒绝与
// place 侧的注册表天然拒绝，锁定计划生成处的防守（spec 场景「伙伴拒绝处理
// 火把」）。与 internal/sim 采掘完成分叉处的 `companionMineableBlock` 是同一
// 规则的两处实现，两处必须同时拒绝。

// TestPlanMineableBlockRejectsTorchForms 锁定计划生成侧对火把的**显式**拒绝。
// 前置守卫是这条用例的承重墙：五种火把形态在 core.BlockDrop 里都有单一产物
// 登记（火把掉回一个火把），「必须有单一 BlockDrop」的通用判据会**放行**它们。
// 伙伴不获得火把能力是冻结契约，在扩出新的火把处置语义之前一律不可作为
// mine 目标。
func TestPlanMineableBlockRejectsTorchForms(t *testing.T) {
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if _, ok := core.BlockDrop(id); !ok {
			t.Fatalf("火把形态 %d 已不在 core.BlockDrop 中，本用例的前提失效", id)
		}
		if planMineableBlock(id) {
			t.Fatalf("planMineableBlock(%d) = true，伙伴必须被显式拒绝", id)
		}
	}
}

// TestPlanPlaceRegistryExcludesTorchForms 锁定 place 半边的计划生成侧防守：
// place 注册表从 core.ItemPlacement 反查构造，火把物品没有无面映射（形态由
// 命中面决定）、注册不成立，火把形态因此无法作为 place 步骤表达。断言钉死
// 这个天然拒绝，防止未来注册表扩入火把时伙伴静默获得放置能力。
func TestPlanPlaceRegistryExcludesTorchForms(t *testing.T) {
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if item, ok := planPlaceBlocks[id]; ok {
			t.Fatalf("place 注册表含火把形态 %d → 物品 %d，伙伴不得获得火把放置能力", id, item)
		}
	}
}
