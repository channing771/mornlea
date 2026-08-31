package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/storage"
)

// TestPlannerRequestExcludesPersonaAndSummary 是「表达平面绝不进入规划输入」
// 的 server 侧反向锁（终审 Should-Fix-1）：伙伴携带非空人设（ResolvedPersona，
// 值等价于任一来源——内联或 personas/ 外部文件，文件来源到达该字段的链路由
// cmd/mornlea-server 的 persona 全链测试锁定）与非空最近对话摘要进入 Planning
// 时，Planner 请求正文 MUST 同时不含二者文本。这补上了既有 planner_test 包级
// 纯函数测试覆盖不到的 server 接线链路，并以「值」而非「字段名」为断言口径
// ——未来任何把 v5 mirror summary 或 ResolvedPersona 塞进快照字符串字段的改动
// 都会被捕获。
func TestPlannerRequestExcludesPersonaAndSummary(t *testing.T) {
	const secretPersona = "内联人设绝密文本乐观但怕黑"
	const secretSummary = "绝密最近摘要伙伴曾挖过煤矿"
	id := chatTestCompanionID(1)
	definitions := []companion.Definition{{
		ID: id, Name: "阿木",
		Persona: secretPersona, ResolvedPersona: secretPersona,
	}}
	host, client, _ := companionManagerHostReady(t, definitions, nil)

	requests := make(chan companionPlanningRequest, 1)
	host.world.companionManager.planner = capturingPlannerTestSeam{requests: requests}

	operation, err := companion.ParseID("66666666-6666-4666-8666-666666666666")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.world.companions.ReplaceActiveMemory(
		id, 1, 0, 1, storage.CompanionIdentity(operation), secretSummary,
	); err != nil {
		t.Fatalf("seed v5 mirror: %v", err)
	}

	sendIntegration(t, client, network.ChatCommand{Text: "@阿木 随便走走"})
	deadline := time.Now().Add(waitDeadline)
	var captured companionPlanningRequest
	gotRequest := false
	for time.Now().Before(deadline) {
		result := host.world.StepForTest()
		receiveCompanionChatTick(t, client, result.Tick)
		select {
		case captured = <-requests:
			gotRequest = true
		default:
		}
		if gotRequest {
			break
		}
	}
	if !gotRequest {
		t.Fatal("规划请求未在窗口内到达假模型")
	}
	raw, err := json.Marshal(struct {
		Instruction string                 `json:"instruction"`
		Snapshot    companion.PlanSnapshot `json:"snapshot"`
	}{Instruction: captured.Instruction, Snapshot: captured.Snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secretPersona) {
		t.Fatalf("规划请求泄漏人设文本：%s", raw)
	}
	if strings.Contains(string(raw), secretSummary) {
		t.Fatalf("规划请求泄漏最近摘要文本：%s", raw)
	}
}

type capturingPlannerTestSeam struct {
	requests chan<- companionPlanningRequest
}

func (p capturingPlannerTestSeam) Plan(ctx context.Context, request companionPlanningRequest) (companionPlanningOutcome, error) {
	select {
	case p.requests <- request:
	case <-ctx.Done():
	}
	return companionPlanningOutcome{}, companion.ErrPlannerInvalidPlan
}
