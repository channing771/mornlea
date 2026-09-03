package server

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
)

func TestNewHostRequiresAgentServiceRuntimeConfig(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.AgentService = companion.AgentServiceSettings{}
	config.AgentCredential = ""
	config.TaskTimeoutMinutes = 10
	store := newCompanionBootstrapStore()
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err == nil {
		cleanupCompanionBootstrapHost(t, host)
		t.Fatal("缺少 agentService 的伙伴配置被 NewHost 接受")
	}
	if !strings.Contains(err.Error(), "agentService") {
		t.Fatalf("错误信息缺少 agentService 定位: %v", err)
	}
	probes, loads, saves := store.companionCallCounts()
	if loads != 0 || probes != 0 || saves != 0 || store.hostileLoadCount() != 0 {
		t.Fatalf("静态 Agent 配置失败后发生 storage I/O: loads=%d probes=%d saves=%d hostileLoads=%d",
			loads, probes, saves, store.hostileLoadCount())
	}
}

func TestNewHostWiresPersistedNamespaceIntoAgentRuntime(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.companionPlanner = nil
	fake := &hostAgentRuntimeFake{acquires: make(chan companion.AcquireRequest, 1)}
	config.companionAgentClientFactory = func(
		settings companion.AgentServiceSettings,
		credential string,
	) (companionAgentRuntimeClient, error) {
		if settings != config.AgentService || credential != config.AgentCredential {
			t.Fatalf("Agent factory settings=%+v credential=%q", settings, credential)
		}
		return fake, nil
	}
	store := newCompanionBootstrapStore()
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	select {
	case request := <-fake.acquires:
		saves := store.companionSaveSnapshot()
		if len(saves) == 0 {
			t.Fatal("Agent acquire 发生前未保存 v5 namespace")
		}
		wantNamespace := companion.ID(saves[len(saves)-1].AgentNamespaceID).String()
		if request.NamespaceID != wantNamespace {
			t.Fatalf("Acquire namespace=%q，want persisted %q", request.NamespaceID, wantNamespace)
		}
	case <-time.After(waitDeadline):
		t.Fatal("timed out waiting for Agent acquire")
	}
	if host.companionAgent != fake || host.companionLease == nil {
		t.Fatal("Host 未持有 Agent client/lease 生命周期")
	}
	if _, ok := host.world.companionManager.planner.(*companionAgentPlanner); !ok {
		t.Fatalf("production planner=%T，want *companionAgentPlanner", host.world.companionManager.planner)
	}
	if _, ok := host.world.companionManager.dialogue.(*companionAgentDialogue); !ok {
		t.Fatalf("production dialogue=%T，want Agent Dialogue bridge", host.world.companionManager.dialogue)
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := host.Shutdown(shutdownContext); err != nil {
		t.Fatalf("Host.Shutdown: %v", err)
	}
	if !fake.closed.Load() {
		t.Fatal("Host shutdown 未关闭 Agent client")
	}
}

func TestNewHostRollsBackAgentRuntimeWhenMCPConstructionFails(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.companionPlanner = nil
	fake := &hostAgentRuntimeFake{acquires: make(chan companion.AcquireRequest, 1)}
	config.companionAgentClientFactory = func(
		companion.AgentServiceSettings,
		string,
	) (companionAgentRuntimeClient, error) {
		return fake, nil
	}
	mcpErr := errors.New("mcp construction failed")
	config.companionMCPFactory = func(*companion.SnapshotRegistry) (*companionMCPService, error) {
		return nil, mcpErr
	}
	store := newCompanionBootstrapStore()
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if host != nil || !errors.Is(err, mcpErr) {
		t.Fatalf("NewHost=%v err=%v，want nil/%v", host, err, mcpErr)
	}
	if !fake.closed.Load() {
		t.Fatal("MCP 构造失败未逆序关闭 Agent client")
	}
}

type hostAgentRuntimeFake struct {
	acquires chan companion.AcquireRequest
	closed   atomic.Bool
}

func (f *hostAgentRuntimeFake) Acquire(_ context.Context, request companion.AcquireRequest) (companion.AcquireResponse, error) {
	select {
	case f.acquires <- request:
	default:
	}
	return companion.AcquireResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) Dialogue(context.Context, companion.AgentDialogueRequest) (companion.AgentDialogueResponse, error) {
	return companion.AgentDialogueResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) CommitMemory(context.Context, companion.MemoryCommitRequest) (companion.MemoryCommitResponse, error) {
	return companion.MemoryCommitResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) ReconcileMemory(context.Context, companion.MemoryReconcileRequest) (companion.MemoryReconcileResponse, error) {
	return companion.MemoryReconcileResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) DeleteMemory(context.Context, companion.MemoryDeleteRequest) (companion.MemoryDeleteResponse, error) {
	return companion.MemoryDeleteResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) Release(context.Context, companion.LeaseRequest) (companion.ReleaseResponse, error) {
	return companion.ReleaseResponse{}, companion.ErrAgentUnavailable
}

func (*hostAgentRuntimeFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

func (f *hostAgentRuntimeFake) Close() { f.closed.Store(true) }

func TestNewHostRejectsMissingAgentCredentialWithoutLeakingValue(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.AgentService = companion.AgentServiceSettings{
		Endpoint: "http://127.0.0.1:1", APIKeyEnv: "MORNLEA_AGENT_KEY",
	}
	config.AgentCredential = ""
	config.TaskTimeoutMinutes = 10
	store := newCompanionBootstrapStore()
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err == nil {
		cleanupCompanionBootstrapHost(t, host)
		t.Fatal("空 Agent credential 被 NewHost 接受")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("错误泄漏 credential: %v", err)
	}
	probes, loads, saves := store.companionCallCounts()
	if loads != 0 || probes != 0 || saves != 0 || store.hostileLoadCount() != 0 {
		t.Fatalf("空 credential 失败后发生 storage I/O: loads=%d probes=%d saves=%d hostileLoads=%d",
			loads, probes, saves, store.hostileLoadCount())
	}
}
