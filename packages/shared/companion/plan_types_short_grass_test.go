package companion

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// plan_types_short_grass_test.go：短草的伙伴防御清单——planner 契约层必须**显式**
// 拒绝把短草作为 mine 目标（change natural-grass-seeds）。与 internal/sim 采掘
// 完成分叉处的 `companionMineableBlock` 是同一规则的两处实现（companion 不得
// 依赖 sim，依赖方向相反），两处必须同时显式拒绝。

// TestPlanMineableBlockExplicitlyRejectsWildGrass 锁定计划生成侧的显式拒绝。
// 承重墙是源码守卫：短草当前没有 `core.BlockDrop` 登记，通用「单一掉落」判据
// 碰巧也会拒绝它——但种子的概率掉落只属于玩家采掘，契约禁止依赖这一巧合；
// 若未来短草获得 BlockDrop 登记，只有函数体内点名的 `core.IsWildGrass` 谓词
// 还站着。
func TestPlanMineableBlockExplicitlyRejectsWildGrass(t *testing.T) {
	if !planFunctionMentionsIdentifier(t, "plan_types.go", "planMineableBlock", "IsWildGrass") {
		t.Fatal("planMineableBlock 没有显式点名 core.IsWildGrass；" +
			"伙伴拒绝短草不得依赖缺失 BlockDrop 的巧合")
	}
	if planMineableBlock(core.ShortGrassID) {
		t.Fatal("planMineableBlock(ShortGrassID) = true，短草必须是显式拒绝的 mine 目标")
	}
}

// planFunctionMentionsIdentifier 用 go/parser 检查 file 内名为 functionName 的
// 函数声明是否在其函数体中提到 identifier。测试进程的工作目录就是本包目录，
// 直接按文件名解析。
func planFunctionMentionsIdentifier(
	t *testing.T,
	file, functionName, identifier string,
) bool {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, file, nil, 0)
	if err != nil {
		t.Fatalf("解析 %s: %v", file, err)
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName {
			continue
		}
		mentioned := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			if ident, ok := node.(*ast.Ident); ok && ident.Name == identifier {
				mentioned = true
			}
			return true
		})
		return mentioned
	}
	t.Fatalf("%s 中没有找到函数 %s，源码守卫会静默失效", file, functionName)
	return false
}
