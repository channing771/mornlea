package server

import (
	"fmt"
	"strings"
	"testing"
)

type loginWaitRecorder struct {
	helperCalls int
	fatalCalls  int
	fatal       string
}

func (recorder *loginWaitRecorder) Helper() {
	recorder.helperCalls++
}

func (recorder *loginWaitRecorder) Fatalf(format string, args ...any) {
	recorder.fatalCalls++
	recorder.fatal = fmt.Sprintf(format, args...)
}

func TestIntegrationLoginTickBudget(t *testing.T) {
	t.Run("初始满足时零推进", func(t *testing.T) {
		recorder := &loginWaitRecorder{}
		steps := 0
		waitIntegrationLoginReady(
			recorder,
			"initial-ready",
			func() bool { return true },
			func() string { return "ready=true" },
			func() { steps++ },
		)
		if recorder.helperCalls != 1 || recorder.fatalCalls != 0 || steps != 0 {
			t.Fatalf("初始满足结果: helper=%d fatal=%d steps=%d", recorder.helperCalls, recorder.fatalCalls, steps)
		}
	})

	t.Run("预算内成立时精确推进", func(t *testing.T) {
		recorder := &loginWaitRecorder{}
		steps := 0
		const wantSteps = 7
		waitIntegrationLoginReady(
			recorder,
			"eventual-ready",
			func() bool { return steps == wantSteps },
			func() string { return fmt.Sprintf("steps=%d", steps) },
			func() { steps++ },
		)
		if recorder.helperCalls != 1 || recorder.fatalCalls != 0 || steps != wantSteps {
			t.Fatalf("预算内成立结果: helper=%d fatal=%d steps=%d，想要 steps=%d", recorder.helperCalls, recorder.fatalCalls, steps, wantSteps)
		}
	})

	t.Run("永不满足时耗尽预算并报告动态诊断", func(t *testing.T) {
		recorder := &loginWaitRecorder{}
		steps := 0
		diagnosticsCalls := 0
		waitIntegrationLoginReady(
			recorder,
			"never-ready",
			func() bool { return false },
			func() string {
				diagnosticsCalls++
				return fmt.Sprintf("steps=%d ready=false inventory=false view=false", steps)
			},
			func() { steps++ },
		)
		const wantBudget = 3000
		if integrationLoginTickBudget != wantBudget || recorder.helperCalls != 1 ||
			recorder.fatalCalls != 1 || diagnosticsCalls != 1 || steps != wantBudget {
			t.Fatalf("预算耗尽结果: budget=%d helper=%d fatal=%d diagnostics=%d steps=%d，想要 budget=%d fatal=1 diagnostics=1 steps=%d",
				integrationLoginTickBudget, recorder.helperCalls, recorder.fatalCalls, diagnosticsCalls, steps, wantBudget, wantBudget)
		}
		diagnostic := fmt.Sprintf("steps=%d ready=false inventory=false view=false", wantBudget)
		for _, fragment := range []string{"never-ready", fmt.Sprint(wantBudget), diagnostic} {
			if !strings.Contains(recorder.fatal, fragment) {
				t.Fatalf("预算耗尽诊断 %q 缺少 %q", recorder.fatal, fragment)
			}
		}
	})
}
