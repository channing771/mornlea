package server

import (
	"context"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/companion"
)

func hostTestAgentService() companion.AgentServiceSettings {
	return companion.AgentServiceSettings{
		Endpoint: "http://127.0.0.1:1", APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
	}
}

func TestNewHostRejectsCompanionsWithoutAgentServiceSettings(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.AgentService = companion.AgentServiceSettings{}
	config.AgentCredential = "test-agent-secret"

	host, err := NewHost(context.Background(), config, flatTestGenerator{}, newCompanionBootstrapStore())
	if err == nil {
		cleanupCompanionBootstrapHost(t, host)
		t.Fatal("缺少 Agent service 设置的伙伴配置被 NewHost 接受")
	}
	if !strings.Contains(err.Error(), "agentService") || strings.Contains(err.Error(), "test-agent-secret") {
		t.Fatalf("错误信息=%v", err)
	}
}

func TestNewHostRejectsMissingResolvedAgentCredential(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.AgentService = hostTestAgentService()
	config.AgentCredential = ""
	if host, err := NewHost(context.Background(), config, flatTestGenerator{}, newCompanionBootstrapStore()); err == nil {
		cleanupCompanionBootstrapHost(t, host)
		t.Fatal("空 Agent credential 被 NewHost 接受")
	}
}

func TestNewHostAcceptsLoopbackAgentWithCredential(t *testing.T) {
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	config.AgentService = hostTestAgentService()
	config.AgentCredential = "test-agent-secret"
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, newCompanionBootstrapStore())
	if err != nil {
		t.Fatalf("loopback Agent 配置被拒绝: %v", err)
	}
	cleanupCompanionBootstrapHost(t, host)
}
