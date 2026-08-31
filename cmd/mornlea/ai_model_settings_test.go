package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
)

// TestRunLocalAgentSettingsReachServerConfig 验证普通本地模式只把 Agent 服务
// 设置、credential 与任务超时送进 server.Config。
func TestRunLocalAgentSettingsReachServerConfig(t *testing.T) {
	id, err := companion.ParseID("00112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	definitions := []companion.Definition{{ID: id, Name: "阿木"}}
	options := application.LocalConnectionTestOptions()
	options.Companions = definitions
	options.AgentService = companion.AgentServiceSettings{
		Endpoint: "http://127.0.0.1:8123", APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
	}
	options.AgentCredential = "secret-value"
	options.TaskTimeoutMinutes = 20
	want := errors.New("stop after server config")
	dependencies := application.NewConnectionTestDependencies(t)
	dependencies.OpenStore = func(context.Context, application.Options) (storage.WorldStore, error) {
		return application.NewConnectionTestStore(42), nil
	}
	dependencies.NewHost = func(_ context.Context, config server.Config, _ server.Generator, _ storage.WorldStore) (application.Host, error) {
		if config.AgentService != options.AgentService || config.AgentCredential != "secret-value" ||
			config.TaskTimeoutMinutes != 20 {
			t.Fatalf("server AgentService=%+v credential=%q timeout=%d",
				config.AgentService, config.AgentCredential, config.TaskTimeoutMinutes)
		}
		return nil, want
	}
	_, gotErr := application.NewWithDependencies(options, dependencies)
	if !errors.Is(gotErr, want) {
		t.Fatalf("newApplication error = %v，want %v", gotErr, want)
	}
}

// TestRunResolvesAgentCredentialFromEnvironment 验证 --config 路径把 APIKeyEnv
// 指向的环境变量解析进 Options；config 文件本体不含密钥。
func TestRunResolvesAgentCredentialFromEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	id, err := companion.ParseID("00112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.AI = &config.AI{
		AgentService: companion.AgentServiceSettings{
			Endpoint: "http://127.0.0.1:8123", APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
		},
		TaskTimeoutMinutes: 15,
		Companions:         []companion.Definition{{ID: id, Name: "阿木"}},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MORNLEA_TEST_AGENT_KEY", "env-secret-value")

	var gotKey string
	var gotService companion.AgentServiceSettings
	var gotTimeout int
	stop := errors.New("stop after config capture")
	err = runWithDependencies([]string{"--config", path}, runDependencies{
		loadIdentity: func(*string) (network.Identity, error) { return network.Identity{}, nil },
		newApplication: func(options application.Options) (*application.Application, error) {
			gotKey = options.AgentCredential
			gotService = options.AgentService
			gotTimeout = options.TaskTimeoutMinutes
			return nil, stop
		},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("run error = %v，want %v", err, stop)
	}
	if gotKey != "env-secret-value" {
		t.Fatalf("AgentCredential = %q，want 从环境变量解析", gotKey)
	}
	if gotService != cfg.AI.AgentService || gotTimeout != 15 {
		t.Fatalf("AgentService=%+v timeout=%d，want %+v/15", gotService, gotTimeout, cfg.AI.AgentService)
	}
}
