package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
)

// TestMornleaServerPersonaFileReachesCompanionDefinition 锁定 persona 文件只进入
// 伙伴定义装配；Task 9 的生产路径不再构造 direct-model Dialogue。
func TestMornleaServerPersonaFileReachesCompanionDefinition(t *testing.T) {
	const companionName = "阿木"
	const filePersona = "沉稳寡言的老向导，说话简短，喜欢用矿物打比方。"
	dir := t.TempDir()
	id, err := companion.ParseID("00112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.AI = &config.AI{
		AgentService: companion.AgentServiceSettings{
			Endpoint: "http://127.0.0.1:8123", APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
		},
		TaskTimeoutMinutes: 10,
		Companions:         []companion.Definition{{ID: id, Name: companionName}},
	}
	configPath := filepath.Join(dir, "config.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	personasDir := filepath.Join(dir, "personas")
	if err := os.MkdirAll(personasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personasDir, companionName+".txt"), []byte(filePersona), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MORNLEA_TEST_AGENT_KEY", "test-agent-secret")

	stop := errors.New("stop after server config capture")
	err = run(context.Background(), []string{"--config", configPath}, dependencies{
		openDisk: func(context.Context, string, storage.OpenOptions) (storage.WorldStore, error) {
			return storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42}), nil
		},
		listenTCP: func(string) (network.Listener, error) { return mornleaServerTestListener{}, nil },
		newHost: func(_ context.Context, serverConfig server.Config, _ server.Generator, _ storage.WorldStore) (mornleaServerHost, error) {
			if len(serverConfig.Companions) != 1 || serverConfig.Companions[0].ResolvedPersona != filePersona {
				t.Fatalf("companions=%+v", serverConfig.Companions)
			}
			if serverConfig.AgentService != cfg.AI.AgentService ||
				serverConfig.AgentCredential != "test-agent-secret" || serverConfig.TaskTimeoutMinutes != 10 {
				t.Fatalf("Agent config=%+v credential=%q timeout=%d",
					serverConfig.AgentService, serverConfig.AgentCredential, serverConfig.TaskTimeoutMinutes)
			}
			return nil, stop
		},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("run error=%v，want %v", err, stop)
	}
}
