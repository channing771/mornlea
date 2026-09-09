package core_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestSaplingHasNoCraftingSmeltingOrStarterKitSource 钉住 spec 条款「树苗 MUST NOT
// 通过合成、初始材料包或任何其他途径获得」。树苗的共享获取面只有两条：采掘或
// 环境移除树苗方块掉 1 个（`BlockDrop`），以及树叶采掘时按冻结坐标判定的额外
// 掉落（`packages/server/sim` 侧）。因此本包能观察到的三条通用获取路径——合成
// 配方、熔炼配方、初始材料包——必须全部对树苗关闭；任何一条悄悄放行都会让
// 「木材可再生」的闭环绕过种树这一唯一入口。
//
// 前两条是对注册表求值：配方表按末项常量穷举，熔炼按 `ItemIDMax` 穷举产物。
// 第三条的清单是服务端持久化单元的 `starterMaterialItems`，shared 模块不得
// import server，所以按本仓既有的源码守卫手法（与 `packages/shared/companion`
// 和 `packages/audit` 的守卫同形）解析该声明并断言清单里没有树苗：守卫找不到
// 声明即失败，不允许静默失效。
func TestSaplingHasNoCraftingSmeltingOrStarterKitSource(t *testing.T) {
	// 合成：固定配方表里没有任何一条产出树苗。枚举界用末项常量而不是裸数字，
	// 追加配方时循环自然延伸；条数与末项常量一致，注册表留洞会被钉出来。
	recipes := 0
	for id := core.RecipeStoneBricks; ; id++ {
		pattern, ok := core.Recipe(id)
		if !ok {
			break
		}
		recipes++
		if pattern.Output.Item == core.ItemSapling {
			t.Fatalf("配方 %d 产出树苗：%+v", id, pattern.Output)
		}
	}
	if recipes != int(core.RecipeBucket) {
		t.Fatalf("配方注册表枚举到 %d 条，想要与末项常量一致的 %d 条（注册表出现空洞？）",
			recipes, core.RecipeBucket)
	}
	// 熔炼：没有任何物品的熔炼产物是树苗。
	for item := core.ItemID(0); item < core.ItemIDMax; item++ {
		if out, ok := core.SmeltingOutput(item); ok && out == core.ItemSapling {
			t.Fatalf("物品 %d 熔炼产出树苗", item)
		}
	}
	// 初始材料包：一次性材料清单里不得出现树苗。
	path := filepath.Join("..", "..", "server", "server", "persistence", "players_snapshot.go")
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatalf("解析初始材料包清单 %s: %v", path, err)
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for index, name := range spec.Names {
			if name.Name != "starterMaterialItems" || index >= len(spec.Values) {
				continue
			}
			found = true
			ast.Inspect(spec.Values[index], func(inner ast.Node) bool {
				ident, ok := inner.(*ast.Ident)
				if ok && (ident.Name == "ItemSapling" || ident.Name == "SaplingID") {
					t.Errorf("初始材料包含树苗标识符 %s", ident.Name)
				}
				return true
			})
		}
		return true
	})
	if !found {
		t.Fatalf("未在 %s 找到 `starterMaterialItems` 声明，本守卫会静默失效", path)
	}
}
