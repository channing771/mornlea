package core_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestSaplingHasNoCraftingSmeltingOrStartingInventory 钉住 spec 条款「树苗 MUST NOT
// 通过合成、初始背包或任何其他途径获得」。树苗的共享获取面只有两条：采掘或
// 环境移除树苗方块掉 1 个（`BlockDrop`），以及树叶采掘时按冻结坐标判定的额外
// 掉落（`packages/server/sim` 侧）。因此本包能观察到的三条通用获取路径——合成
// 配方、熔炼配方、新玩家的初始背包——必须全部对树苗关闭；任何一条悄悄放行都会
// 让「木材可再生」的闭环绕过种树这一唯一入口。
//
// 前两条是对注册表求值：配方表按末项常量穷举，熔炼按 `ItemIDMax` 穷举产物。
// 第三条是契约「缺失玩家的初始背包恒为空」的跨模块源码守卫：shared 模块不得
// import server，所以沿用本仓既有的源码守卫手法（与 `packages/shared/companion`
// 及 `packages/server/sim` 的守卫同形）解析玩家快照源文件 `players_snapshot.go`。
// 守卫的失败面就是源码里全部可定位的退化形状：
//
//  1. 锚点函数 `newMissingCachedPlayer` 找不到（被改名或被移走），守卫失去射程；
//  2. 函数体里不再构造 `contract.PlayerSnapshot` 字面量，快照字段表搬出了射程；
//  3. `Inventory` 字段取到非空字面量或调用表达式（显式空字面量
//     `core.Inventory{}` 是唯一允许的形状）；
//  4. 函数体内任何一条赋值语句写穿 `Inventory` 或 `Inventory.Backpack`（含下标链）；
//  5. 函数体里出现任何物品标识符（`Item` 前缀，点路径选择子与裸名同样命中）；
//  6. 整个文件里出现任何树苗标识符（射程覆盖既有玩家的恢复与写回路径）。
//
// 后四条判据刻意从严：抓的是语法形状而非语义上是否真的非空，宁可对契约合规但
// 形状可疑的写法误报，也不放过一次真实发放。守卫失败时给出源位置并直接报红，
// 不允许静默失效。
func TestSaplingHasNoCraftingSmeltingOrStartingInventory(t *testing.T) {
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
	if recipes != int(core.RecipeIronBoots) {
		t.Fatalf("配方注册表枚举到 %d 条，想要与末项常量一致的 %d 条（注册表出现空洞？）",
			recipes, core.RecipeIronBoots)
	}
	// 熔炼：没有任何物品的熔炼产物是树苗。
	for item := core.ItemID(0); item < core.ItemIDMax; item++ {
		if out, ok := core.SmeltingOutput(item); ok && out == core.ItemSapling {
			t.Fatalf("物品 %d 熔炼产出树苗", item)
		}
	}
	// 初始背包：缺失玩家的快照构造处不得装入任何物品，自然也不得装入树苗。
	path := filepath.Join("..", "..", "server", "server", "persistence", "players_snapshot.go")
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatalf("解析缺失玩家快照构造 %s: %v", path, err)
	}
	guardMissingPlayerSnapshotStartsEmpty(t, path, fileSet, parsed)
	guardSnapshotSourceMentionsNoSapling(t, path, fileSet, parsed)
}

// guardMissingPlayerSnapshotStartsEmpty 是「缺失玩家的初始背包恒为空」在服务端
// 源码侧的守卫。锚点是 `newMissingCachedPlayer`：它必须在自身函数体里写出
// `contract.PlayerSnapshot` 字面量——字段表写在这里，守卫的射程才真的覆盖
// 「背包初值取什么」这一决策。构造函数一旦退化成转发（字段表搬到守卫看不见的
// 地方），本守卫会失去射程，因此这种形态直接报红而不是静默通过。
//
// 函数体内的两条禁令互补：
//   - 任何物品标识符（点路径 `core.Item...` 的选择子也是独立标识符，裸名同理）
//     都说明初值点名了某件物品；
//   - `Inventory` 只允许写成显式空字面量（与零值同义，是契约要求的形状）；其余
//     形状一律按发放处理，包括非空字面量、调用表达式，以及把它写在赋值目标里的
//     任何形态——赋值判据只看目标是否写穿 `Inventory`/`Inventory.Backpack`
//     （含 `Inventory.Backpack[i]` 这类下标链），所以契约合规的
//     `Inventory: core.Inventory{}` 若落在赋值左侧同样报红。
//
// 两条禁令都刻意从严：判据抓的是语法形状而不是语义上是否真的非空，宁可对合规
// 但形状可疑的写法误报，也不放过一次真实发放。
func guardMissingPlayerSnapshotStartsEmpty(t *testing.T, path string, fileSet *token.FileSet, parsed *ast.File) {
	t.Helper()
	const constructorName = "newMissingCachedPlayer"
	constructor := findFunctionDeclaration(parsed, constructorName)
	if constructor == nil {
		t.Fatalf("%s 中没有找到函数 %s，本守卫会静默失效", path, constructorName)
	}
	buildsSnapshot := false
	ast.Inspect(constructor.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.Ident:
			// 物品标识符的形状是 `Item` 前缀；这里对整个标识符判定，所以
			// `core.ItemCobblestone` 与点导入后的 `ItemCobblestone` 都命中。
			if strings.HasPrefix(node.Name, "Item") {
				t.Errorf("%s：%s 的 %s 提到物品标识符 %s：缺失玩家的初始背包必须是空的",
					fileSet.Position(node.Pos()), path, constructorName, node.Name)
			}
		case *ast.CompositeLit:
			if isPlayerSnapshotLiteral(node) {
				buildsSnapshot = true
			}
		case *ast.KeyValueExpr:
			guardSnapshotInventoryFieldStaysEmpty(t, path, fileSet, node)
		case *ast.AssignStmt:
			for _, target := range node.Lhs {
				if mentionsInventory(target) {
					t.Errorf("%s：%s 的 %s 通过赋值写穿 `Inventory`/`Inventory.Backpack`："+
						"缺失玩家的初始背包必须是空的（判据只看赋值目标，"+
						"契约合规的空字面量赋值同样按发放处理，守卫刻意从严）",
						fileSet.Position(target.Pos()), path, constructorName)
				}
			}
		}
		return true
	})
	if !buildsSnapshot {
		t.Fatalf("%s 的 %s 没有在自身函数体里构造 contract.PlayerSnapshot 字面量；"+
			"快照字段表已被移出本守卫的射程，本守卫会静默失效", path, constructorName)
	}
}

// guardSnapshotInventoryFieldStaysEmpty 检查快照的一个字段初始值：键是
// `Inventory` 时，取值只允许显式的空字面量 `core.Inventory{}`——它与
// `core.Inventory` 零值同义，正是契约要求的形状（写出来是给读者看的，不是发放
// 机制）。其余取值一律按发放处理：非空字面量、调用表达式，乃至函数体里任何一处
// 以 `Inventory` 为键的字段（哪怕它并不属于快照字面量）。判据刻意从严，宁可误报
// 也不静默放过。
func guardSnapshotInventoryFieldStaysEmpty(t *testing.T, path string, fileSet *token.FileSet, field *ast.KeyValueExpr) {
	t.Helper()
	key, ok := field.Key.(*ast.Ident)
	if !ok || key.Name != "Inventory" {
		return
	}
	if literal, ok := field.Value.(*ast.CompositeLit); ok && len(literal.Elts) == 0 {
		return
	}
	t.Errorf("%s：`Inventory` 的取值不是显式空字面量 `core.Inventory{}`；"+
		"非空字面量、调用表达式或函数体内任何 `Inventory:` 键都按发放处理："+
		"缺失玩家的初始背包必须是空的",
		fileSet.Position(field.Pos()))
}

// guardSnapshotSourceMentionsNoSapling 是树苗专项禁令：玩家快照源文件里不得出现
// 任何树苗标识符。它的射程比上面的构造函数守卫更宽（覆盖整个文件，包括既有玩家
// 的恢复与写回路径），因为「任何其他途径」同样包括「经存档或其他构造路径把树苗
// 重新塞进背包」。
func guardSnapshotSourceMentionsNoSapling(t *testing.T, path string, fileSet *token.FileSet, parsed *ast.File) {
	t.Helper()
	ast.Inspect(parsed, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if identifier.Name == "ItemSapling" || identifier.Name == "SaplingID" {
			t.Errorf("%s：%s 提到树苗标识符 %s：玩家快照不得经任何构造路径携带树苗",
				fileSet.Position(identifier.Pos()), path, identifier.Name)
		}
		return true
	})
}

// findFunctionDeclaration 返回已解析文件里名为 name 的函数声明，找不到返回 nil。
func findFunctionDeclaration(parsed *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name {
			return function
		}
	}
	return nil
}

// isPlayerSnapshotLiteral 报告一个字面量是否以快照类型为准，即类型写成限定形式
// `contract.PlayerSnapshot`。本仓没有任何点导入，裸名形式不可达，因此不为此留
// 容错分支。
func isPlayerSnapshotLiteral(literal *ast.CompositeLit) bool {
	if selector, ok := literal.Type.(*ast.SelectorExpr); ok {
		return selector.Sel.Name == "PlayerSnapshot"
	}
	return false
}

// mentionsInventory 报告一个表达式是否选到了 `Inventory` 字段，用于判断赋值目标
// 是不是在写背包：`Inventory` 本身与 `Inventory.Backpack[i]` 这类下标链都命中。
func mentionsInventory(expression ast.Expr) bool {
	mentions := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Name == "Inventory" {
			mentions = true
		}
		return true
	})
	return mentions
}
