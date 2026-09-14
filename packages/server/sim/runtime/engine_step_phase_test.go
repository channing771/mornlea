package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"testing"
)

// 本文件钉住方块更新相位的固定子序：旧的流体/湿度/作物三个相位收敛为单一
// phaseBlockUpdates 后，相位观察面只剩一个入口通知，子序证据从「三个相位的
// 进入顺序」转为对 StepWithTunables 源序的直接断言——重扫 → 定时面（流体 →
// 湿度）→ 随机面（踩踏/雪印结算 → 作物抽样）。这是编排契约，不是行为测试：
// 行为不变性由 crop_random_replay_kat_test（256 tick 逐位重放）与湿度同 tick
// 结算测试承担，这里保证它们依赖的调用顺序不被悄悄重排。

// expectedStepSequence 是 StepWithTunables 中被钉住的关键调用（含相位通知的
// 实参），按执行顺序排列。重扫不在此列：流体边界重扫在 AdvanceFluids 内部、
// 湿度 scope 登记与全块重扫在 AdvanceFarmlandMoisture 内部，均属被调函数的
// 内部编排，收敛未触碰。
var expectedStepSequence = []string{
	"notifyStepPhase:phasePlayerCommands",
	"notifyStepPhase:phaseCompanionActions",
	"notifyStepPhase:phasePhysicsAdvance",
	"notifyStepPhase:phaseHostileAdvance",
	"notifyStepPhase:phaseBlockUpdates",
	"AdvanceFluids",
	"NewEnvironmentMutation",
	"AdvanceFarmlandMoisture",
	"SettleTramples",
	"SettleSnowFootprints",
	"AdvanceCrops",
	"FinishWorld",
	"SweepUnsupportedWildPlants",
	"SweepUnsupportedSaplings",
	"SweepUnsupportedTorches",
	"SweepUnsupportedBeds",
	"finishRealmMutation",
}

func TestStepWithTunablesPinsBlockUpdatesSubOrder(t *testing.T) {
	source, err := os.ReadFile("engine_step.go")
	if err != nil {
		t.Fatalf("读取 engine_step.go：%v", err)
	}
	got := watchedStepCalls(t, "engine_step.go", string(source))
	if !reflect.DeepEqual(got, expectedStepSequence) {
		t.Fatalf("StepWithTunables 关键调用序=%v，想要 %v", got, expectedStepSequence)
	}
}

// TestStepSequenceGuardRejectsSwappedOrder 证明守卫真的咬人：把随机面挪到湿度
// 重判之前的合成源码必须被判定为偏离，避免解析器退化成永远返回期望序列。
func TestStepSequenceGuardRejectsSwappedOrder(t *testing.T) {
	const swapped = `package runtime

func (engine *Engine) StepWithTunables(tickTunables TickTunables) TickResult {
	engine.notifyStepPhase(phaseHostileAdvance)
	engine.notifyStepPhase(phaseBlockUpdates)
	engine.realm.AdvanceFluids(active, pending)
	tick.SettleTramples()
	engine.realm.AdvanceCrops(active, pending)
	engine.realm.AdvanceFarmlandMoisture(active, environment)
	return TickResult{}
}
`
	got := watchedStepCalls(t, "swapped.go", swapped)
	if reflect.DeepEqual(got, expectedStepSequence) {
		t.Fatalf("合成乱序源码未被守卫识别：%v", got)
	}
	// 乱序的具体形态必须可读出来：合成源里随机面（AdvanceCrops）写在湿度重判
	//（AdvanceFarmlandMoisture）之前，按源序收集时它的下标必须更小。
	var crops, moisture int
	for index, call := range got {
		switch call {
		case "AdvanceCrops":
			crops = index
		case "AdvanceFarmlandMoisture":
			moisture = index
		}
	}
	if crops > moisture {
		t.Fatalf("守卫未按源序收集调用：AdvanceCrops 下标=%d 应小于 AdvanceFarmlandMoisture 下标=%d", crops, moisture)
	}
}

// watchedStepCalls 按源序收集 StepWithTunables 中的关键调用：相位通知记录为
// 「函数名:实参名」，其余被钉调用记录函数名。ast.Inspect 以前序遍历保证收集
// 顺序与书写顺序一致。
func watchedStepCalls(t *testing.T, name, source string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
	if err != nil {
		t.Fatalf("解析 %s：%v", name, err)
	}
	var sequence []string
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || function.Name.Name != "StepWithTunables" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.Ident:
				if callee.Name == "finishRealmMutation" {
					sequence = append(sequence, callee.Name)
				}
			case *ast.SelectorExpr:
				switch callee.Sel.Name {
				case "notifyStepPhase":
					if len(call.Args) == 1 {
						if identifier, ok := call.Args[0].(*ast.Ident); ok {
							sequence = append(sequence, "notifyStepPhase:"+identifier.Name)
						}
					}
				case "AdvanceFluids", "NewEnvironmentMutation", "AdvanceFarmlandMoisture",
					"SettleTramples", "SettleSnowFootprints", "AdvanceCrops", "FinishWorld",
					"SweepUnsupportedWildPlants", "SweepUnsupportedSaplings",
					"SweepUnsupportedTorches", "SweepUnsupportedBeds":
					sequence = append(sequence, callee.Sel.Name)
				}
			}
			return true
		})
	}
	if len(sequence) == 0 {
		t.Fatalf("%s 中未找到 StepWithTunables 的任何关键调用", name)
	}
	return sequence
}
