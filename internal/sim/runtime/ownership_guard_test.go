package runtime

import (
	"strings"
	"testing"
)

func TestOwnershipAnalyzerRejectsSyntheticViolations(t *testing.T) {
	t.Run("production 声明藏在任意文件仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{
			{name: "engine.go", contents: `package entity; type State struct{}`},
			{name: "callbacks.go", contents: `package entity; type StepHooks struct{}`},
			{name: "dispatch.go", contents: `package entity; func (*State) Step() {}`},
			{name: "fixture_test.go", contents: `package entity; type Engine struct{}`},
		})
		requireOwnershipViolation(t, violations, "StepHooks")
		requireOwnershipViolation(t, violations, "State.Step")
	})

	t.Run("改名且嵌套的五类 inbox 仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Command struct{}
				type CompanionAction struct{}
				type HostileAction struct{}
				type AcquiredChunk struct{}
				type GeneratedChunk struct{}
				type renamedQueues struct {
					one []Command
					two [8]CompanionAction
					three chan HostileAction
					four map[int]AcquiredChunk
					five []GeneratedChunk
				}
				type nestedHolder struct { hidden *renamedQueues }
				type Engine struct { nested nestedHolder }
				func (*Engine) Step() {}
				func (*Engine) EnqueueRenamed() {}
				func (*Engine) SubmitRenamed() {}
			`,
		}})
		for _, want := range []string{
			"Command inbox", "CompanionAction inbox", "HostileAction inbox",
			"AcquiredChunk inbox", "GeneratedChunk inbox", "Engine.Step",
			"Engine.EnqueueRenamed", "Engine.SubmitRenamed",
		} {
			requireOwnershipViolation(t, violations, want)
		}
	})

	t.Run("拆分 helper 的完整编排仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Engine struct{}
				func TestCopiedTick() { driveCopiedTick(&Engine{}) }
				func driveCopiedTick(engine *Engine) {
					applyEntityStages(engine)
					applyRealmStages(engine)
					finishCopiedTick(engine)
				}
				func applyEntityStages(*Engine) {
					tick.ApplyPlayerCommands(nil, nil)
					tick.ApplyCompanionActions(nil)
					tick.AdvanceActors()
					tick.AdvanceHostiles(nil, nil)
					tick.SettleGameplay(nil)
					tick.SettleTramples()
					tick.FinishWorld(nil)
				}
				func applyRealmStages(*Engine) {
					state.AdvanceFluids(nil, nil)
					state.AdvanceFarmlandMoisture(nil, nil)
					state.AdvanceCrops(nil, nil)
					state.SweepUnsupportedTorches(nil)
					state.SweepUnsupportedBeds(nil)
				}
				func finishCopiedTick(*Engine) {
					mutation.Commit()
					tick.Publish(nil)
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "driveCopiedTick")
		requireOwnershipViolation(t, violations, "TestCopiedTick")
	})

	t.Run("Test 经 callback 参数拆分的完整编排仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Engine struct{}
				func runCallbacks(entityPart func(), realmPart func(), finishPart func()) {
					entityPart()
					realmPart()
					finishPart()
				}
				func TestCallbackSplit() {
					runCallbacks(applyEntityStages, applyRealmStages, finishCopiedTick)
				}
				func applyEntityStages() {
					tick.ApplyPlayerCommands(nil, nil)
					tick.ApplyCompanionActions(nil)
					tick.AdvanceActors()
					tick.AdvanceHostiles(nil, nil)
					tick.SettleGameplay(nil)
					tick.SettleTramples()
					tick.FinishWorld(nil)
				}
				func applyRealmStages() {
					state.AdvanceFluids(nil, nil)
					state.AdvanceFarmlandMoisture(nil, nil)
					state.AdvanceCrops(nil, nil)
					state.SweepUnsupportedTorches(nil)
					state.SweepUnsupportedBeds(nil)
				}
				func finishCopiedTick() {
					mutation.Commit()
					tick.Publish(nil)
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "TestCallbackSplit")
	})

	t.Run("Benchmark 经 callback 参数拆分的完整编排仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Engine struct{}
				func runCallbacks(entityPart func(), realmPart func(), finishPart func()) {
					entityPart()
					realmPart()
					finishPart()
				}
				func BenchmarkCallbackSplit() {
					runCallbacks(applyEntityStages, applyRealmStages, finishCopiedTick)
				}
				func applyEntityStages() {
					tick.ApplyPlayerCommands(nil, nil)
					tick.ApplyCompanionActions(nil)
					tick.AdvanceActors()
					tick.AdvanceHostiles(nil, nil)
					tick.SettleGameplay(nil)
					tick.SettleTramples()
					tick.FinishWorld(nil)
				}
				func applyRealmStages() {
					state.AdvanceFluids(nil, nil)
					state.AdvanceFarmlandMoisture(nil, nil)
					state.AdvanceCrops(nil, nil)
					state.SweepUnsupportedTorches(nil)
					state.SweepUnsupportedBeds(nil)
				}
				func finishCopiedTick() {
					mutation.Commit()
					tick.Publish(nil)
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "BenchmarkCallbackSplit")
	})

	t.Run("assigned FuncLit 经 callback 参数传播仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Engine struct{}
				func runCallbacks(parts ...func()) {
					for _, part := range parts { part() }
				}
				func TestAssignedCallbacks() {
					entityPart := func() {
						tick.ApplyPlayerCommands(nil, nil)
						tick.ApplyCompanionActions(nil)
						tick.AdvanceActors()
						tick.AdvanceHostiles(nil, nil)
						tick.SettleGameplay(nil)
						tick.SettleTramples()
						tick.FinishWorld(nil)
					}
					realmPart := func() {
						state.AdvanceFluids(nil, nil)
						state.AdvanceFarmlandMoisture(nil, nil)
						state.AdvanceCrops(nil, nil)
						state.SweepUnsupportedTorches(nil)
						state.SweepUnsupportedBeds(nil)
					}
					finishPart := func() {
						mutation.Commit()
						tick.Publish(nil)
					}
					entityAlias := entityPart
					runCallbacks(entityAlias, realmPart, finishPart)
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "TestAssignedCallbacks")
	})

	t.Run("IIFE 经 callback 参数传播仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Engine struct{}
				func TestIIFECallbacks() {
					(func(parts ...func()) {
						for _, part := range parts { part() }
					})(
						func() {
							tick.ApplyPlayerCommands(nil, nil)
							tick.ApplyCompanionActions(nil)
							tick.AdvanceActors()
							tick.AdvanceHostiles(nil, nil)
							tick.SettleGameplay(nil)
							tick.SettleTramples()
							tick.FinishWorld(nil)
						},
						func() {
							state.AdvanceFluids(nil, nil)
							state.AdvanceFarmlandMoisture(nil, nil)
							state.AdvanceCrops(nil, nil)
							state.SweepUnsupportedTorches(nil)
							state.SweepUnsupportedBeds(nil)
						},
						func() {
							mutation.Commit()
							tick.Publish(nil)
						},
					)
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "TestIIFECallbacks")
	})

	t.Run("variadic callback 转发的完整编排仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Engine struct{}
				func runCallbacks(parts ...func()) {
					for _, part := range parts { part() }
				}
				func forwardCallbacks(parts ...func()) { runCallbacks(parts...) }
				func TestVariadicCallbacks() {
					forwardCallbacks(
						func() {
							tick.ApplyPlayerCommands(nil, nil)
							tick.ApplyCompanionActions(nil)
							tick.AdvanceActors()
							tick.AdvanceHostiles(nil, nil)
							tick.SettleGameplay(nil)
							tick.SettleTramples()
							tick.FinishWorld(nil)
						},
						func() {
							state.AdvanceFluids(nil, nil)
							state.AdvanceFarmlandMoisture(nil, nil)
							state.AdvanceCrops(nil, nil)
							state.SweepUnsupportedTorches(nil)
							state.SweepUnsupportedBeds(nil)
						},
						func() {
							mutation.Commit()
							tick.Publish(nil)
						},
					)
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "TestVariadicCallbacks")
	})

	t.Run("多层 generic holder 中的 inbox 仍拒绝", func(t *testing.T) {
		violations := analyzeOwnershipSources([]ownershipSource{{
			name: "fixture_test.go",
			contents: `package entity
				type Command struct{}
				type Queue[T any] struct { items []T }
				type Holder[T any] struct { nested Queue[T] }
				type Outer[T any] struct { holder Holder[T] }
				type Engine struct {
					safe Outer[int]
					hidden Outer[Command]
				}
			`,
		}})
		requireOwnershipViolation(t, violations, "Command inbox")
	})
}

func TestOwnershipAnalyzerAcceptsSyntheticNarrowFixtures(t *testing.T) {
	violations := analyzeOwnershipSources([]ownershipSource{{
		name: "fixture_test.go",
		contents: `package entity
			type Command struct{}
			type CompanionAction struct{}
			type HostileAction struct{}
			type AcquiredChunk struct{}
			type GeneratedChunk struct{}
			type narrowHolder struct { command Command }
			type Engine struct {
				commands int
				holder narrowHolder
			}
			type unrelated struct {
				commands []Command
				companionActions []CompanionAction
				hostileActions []HostileAction
				acquired []AcquiredChunk
				generated []GeneratedChunk
			}
			func (*unrelated) Step() {}
			func (*unrelated) EnqueueAnything() {}
			func (*unrelated) SubmitAnything() {}
			func advanceActorsOnly(*Engine) {
				tick.AdvanceActors()
				tick.Publish(nil)
			}
		`,
	}})
	if len(violations) != 0 {
		t.Fatalf("合法窄夹具被误拒绝：%s", strings.Join(violations, "; "))
	}
}

func TestOwnershipAnalyzerAcceptsSyntheticCallbacksAndGenerics(t *testing.T) {
	violations := analyzeOwnershipSources([]ownershipSource{{
		name: "fixture_test.go",
		contents: `package entity
			type Command struct{}
			type Queue[T any] struct { items []T }
			type Holder[T any] struct { nested Queue[T] }
			type Engine struct { values Holder[int] }
			type unrelated struct { values Queue[Command] }
			func (*unrelated) Step() {}
			func (*unrelated) EnqueueAnything() {}
			func (*unrelated) SubmitAnything() {}
			func runCallback(part func()) { part() }
			func TestNarrowCallback() {
				runCallback(func() {
					tick.AdvanceActors()
					tick.Publish(nil)
				})
			}
		`,
	}})
	if len(violations) != 0 {
		t.Fatalf("合法 callback 或 generic 夹具被误拒绝：%s", strings.Join(violations, "; "))
	}
}

func requireOwnershipViolation(t *testing.T, violations []string, want string) {
	t.Helper()
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return
		}
	}
	t.Fatalf("违规清单缺少 %q：%s", want, strings.Join(violations, "; "))
}
