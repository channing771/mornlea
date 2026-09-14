package entity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"testing"
)

// 本文件钉住 hostile 阶段内部的固定子序（`AdvanceHostiles` 的编排契约）：
// 生成/意图/移动 → 近战结算 → 灼烧 → 远离消失 → 投射物推进 → 夜行者死亡结算
// → 玩家死亡结算。投射物阶段必须在两个死亡结算之前：弹击致死必须与近战致死
// 同 tick 完成掉落与移除，任何 0 血实体不得存活到下一权威 tick。远离消失与
// 夜行者死亡结算的相对对调（distant 先于结算）不产生观察差异——distant 只改
// 集合成员与计数，结算只处理 0 血成员。

// expectedAdvanceHostilesSequence 是 `AdvanceHostiles` 中被钉住的调用，按执行
// 顺序排列。
var expectedAdvanceHostilesSequence = []string{
	"advanceHostiles",
	"advanceCombat",
	"advanceHostileBurn",
	"advanceHostileDistant",
	"advanceProjectiles",
	"settleHostileDeaths",
	"settleDeaths",
}

func TestAdvanceHostilesPinsStageOrder(t *testing.T) {
	source, err := os.ReadFile("tick.go")
	if err != nil {
		t.Fatalf("读取 tick.go：%v", err)
	}
	got := advanceHostilesCallOrder(t, "tick.go", string(source))
	if !reflect.DeepEqual(got, expectedAdvanceHostilesSequence) {
		t.Fatalf("AdvanceHostiles 阶段调用序=%v，想要 %v", got, expectedAdvanceHostilesSequence)
	}
}

// TestAdvanceHostilesOrderGuardRejectsSwappedOrder 证明守卫真的咬人：把投射物
// 阶段挪到夜行者死亡结算之后的合成源码必须被判定为偏离，避免解析器退化成
// 永远返回期望序列。
func TestAdvanceHostilesOrderGuardRejectsSwappedOrder(t *testing.T) {
	const swapped = `package entity

func (tick *TickContext) AdvanceHostiles(actions []HostileAction, result *TickResult) {
	engine := &tick.engine
	pending := tick.mutation
	engine.advanceHostiles(actions)
	engine.advanceCombat(result)
	engine.advanceHostileBurn(engine.worldTime.Load())
	engine.advanceHostileDistant()
	engine.settleHostileDeaths(pending)
	engine.advanceProjectiles(result)
	engine.settleDeaths(pending)
}
`
	got := advanceHostilesCallOrder(t, "swapped.go", swapped)
	if reflect.DeepEqual(got, expectedAdvanceHostilesSequence) {
		t.Fatalf("合成乱序源码未被守卫识别：%v", got)
	}
}

// advanceHostilesCallOrder 按源序收集 `AdvanceHostiles` 中对 engine 的方法调用：
// ast.Inspect 以前序遍历保证收集顺序与书写顺序一致。
func advanceHostilesCallOrder(t *testing.T, name, source string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
	if err != nil {
		t.Fatalf("解析 %s：%v", name, err)
	}
	var sequence []string
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || function.Name.Name != "AdvanceHostiles" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// 只统计对 engine 的直接方法调用：嵌套表达式（如 worldTime.Load）
			// 的接收者不是 Ident，天然被过滤。
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || receiver.Name != "engine" {
				return true
			}
			sequence = append(sequence, selector.Sel.Name)
			return true
		})
	}
	if len(sequence) == 0 {
		t.Fatalf("%s 中未找到 AdvanceHostiles 的任何调用", name)
	}
	return sequence
}
