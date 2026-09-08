package companion

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// plan_types_sapling_test.go：树苗的伙伴边界（spec 需求「伙伴交互边界显式」）。
// 本能力不给伙伴增加任何新的放置权限，也不为树苗新增显式采掘拒绝；伙伴采掘
// 树叶沿用既有的单一 `BlockDrop` 结算，玩家专属的树叶→树苗概率判定不得被
// 镜像到伙伴侧。与 internal/sim 采掘完成分叉处的 `companionMineableBlock` 是
// 同一规则的两处实现（companion 不得依赖 sim，依赖方向相反），两处必须一致。

// TestPlanPlaceRegistryExcludesSapling 锁定 place 半边的计划生成侧防守：树苗是
// 玩家可放置物品（`core.ItemPlacement` 有映射），但伙伴 place 注册表刻意不收
// ——收进去就等于顺手给伙伴开了植树权限，而伙伴植树属未裁决的农业/植被语义
// （与作物种植同一口径，豁免条目写在注册表锁定用例里）。本用例同时钉住名字
// 索引、方块反查索引与计划校验三条路径，任何一条放行都会红。
func TestPlanPlaceRegistryExcludesSapling(t *testing.T) {
	name, ok := core.CanonicalItemName(core.ItemSapling)
	if !ok {
		t.Fatal("core 没有登记树苗的 canonical 名字，本用例前提失效")
	}
	if _, ok := core.ItemPlacement(core.ItemSapling); !ok {
		t.Fatal("树苗不再是可放置物品，place 豁免条目的前提失效")
	}
	if item, ok := planPlaceItems[name]; ok {
		t.Fatalf("place 注册表含名字 %s → 物品 %d，伙伴不得获得树苗放置能力", name, item)
	}
	if item, ok := planPlaceBlocks[core.SaplingID]; ok {
		t.Fatalf("place 注册表含树苗方块 %d → 物品 %d，伙伴不得获得树苗放置能力", core.SaplingID, item)
	}
	plan := Plan{
		Summary: "在泥土上种一株树苗",
		Steps:   []PlanStep{{Kind: PlanStepPlace, Y: 64, Block: core.SaplingID}},
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("含树苗 place 步骤的计划被接受，Planner 契约必须拒绝它")
	}
}

// TestPlanMineableBlockAdmitsSaplingByGenericRule 钉住伙伴采掘树苗沿用「具有
// 单一 `BlockDrop` 的非容器、非农业、非流体方块」通用规则：树苗恰有单一掉落，
// 因此**不得**为它新增显式拒绝（伙伴植树属未裁决语义，但采掘不是）。源码守卫
// 是这条用例的承重墙——断言 `planMineableBlock` 函数体不点名 `core.IsSapling`，
// 防止将来把树苗顺手塞进农业/植被防御清单时伙伴采掘被静默关掉。
func TestPlanMineableBlockAdmitsSaplingByGenericRule(t *testing.T) {
	drop, ok := core.BlockDrop(core.SaplingID)
	if !ok || drop != core.ItemSapling {
		t.Fatalf("core.BlockDrop(SaplingID) = (%d, %v)，通用单一掉落判据的前提失效", drop, ok)
	}
	if !planMineableBlock(core.SaplingID) {
		t.Fatal("planMineableBlock(SaplingID) = false，树苗必须按通用单一掉落规则被允许")
	}
	if planFunctionMentionsIdentifier(t, "plan_types.go", "planMineableBlock", "IsSapling") {
		t.Fatal("planMineableBlock 点名了 core.IsSapling，伙伴采掘树苗不得新增显式拒绝")
	}
}

// TestPlanLeavesMiningKeepsSingleLeavesDrop 钉住伙伴采掘树叶的产出契约（spec
// 场景「伙伴采掘树叶不触发树苗判定」的计划侧）：树叶仍是通用规则下的合法 mine
// 目标，其唯一掉落是既有 `ItemLeaves`；树苗只可能由树苗方块自身掉落，树叶的
// 单一掉落登记不得变成树苗。玩家专属的树叶→树苗概率判定只存在于 internal/sim
// 的 `completeMining` 树叶分支，伙伴走通用单件结算，本包不得出现它的镜像。
func TestPlanLeavesMiningKeepsSingleLeavesDrop(t *testing.T) {
	drop, ok := core.BlockDrop(core.LeavesID)
	if !ok || drop != core.ItemLeaves {
		t.Fatalf("core.BlockDrop(LeavesID) = (%d, %v)，伙伴树叶结算的单件掉落前提失效", drop, ok)
	}
	if !planMineableBlock(core.LeavesID) {
		t.Fatal("planMineableBlock(LeavesID) = false，树叶必须仍是通用规则下的合法 mine 目标")
	}
	saplingDrop, ok := core.BlockDrop(core.SaplingID)
	if !ok || saplingDrop != core.ItemSapling {
		t.Fatalf("core.BlockDrop(SaplingID) = (%d, %v)，树苗必须只由树苗方块自身掉落", saplingDrop, ok)
	}
}

// TestCompanionProductionHasNoSaplingBranch 是边界的总守卫：伙伴侧对树苗的唯一
// 认知是测试内的 place 豁免条目，生产代码不得出现任何树苗分支——没有 place
// 注册项、没有 mine 显式拒绝，也不存在玩家树叶→树苗判定的镜像实现。任何镜像
// 都必须点名 `core.ItemSapling`/`core.SaplingID`/`core.IsSapling` 才能产出或
// 判定树苗，因此逐个标识符扫描本包的非测试源码即可把这类分支钉死。
func TestCompanionProductionHasNoSaplingBranch(t *testing.T) {
	for _, identifier := range []string{"ItemSapling", "SaplingID", "IsSapling"} {
		if planProductionMentionsIdentifier(t, identifier) {
			t.Fatalf("本包生产代码点名了 %s，伙伴侧不得有树苗分支"+
				"（豁免只存在于测试内的 planPlaceExempt）", identifier)
		}
	}
}

// planProductionMentionsIdentifier 用 go/parser 解析本包目录下的全部非测试
// .go 文件，报告是否有任意标识符等于 identifier。测试进程的工作目录就是本包
// 目录，直接按目录解析；解析失败意味着守卫会静默失效，因此直接失败。
func planProductionMentionsIdentifier(t *testing.T, identifier string) bool {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseDir(fileSet, ".", nil, 0)
	if err != nil {
		t.Fatalf("解析本包目录: %v", err)
	}
	for _, pkg := range parsed {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			mentioned := false
			ast.Inspect(file, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok && ident.Name == identifier {
					mentioned = true
				}
				return true
			})
			if mentioned {
				return true
			}
		}
	}
	return false
}
