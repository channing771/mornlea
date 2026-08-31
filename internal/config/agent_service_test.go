package config

import (
	"os"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/companion"
)

func writeAIConfig(t *testing.T, aiBody string) string {
	t.Helper()
	return writeConfig(t, `{"version":1,"ai":`+aiBody+`}`)
}

const aiCompanionEntry = `{"id":"00112233-4455-4677-8899-aabbccddeeff","name":"阿木"}`

func TestAIConfigAgentServiceRequiresLoopbackCredential(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	valid := `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`
	loaded, err := Load(writeAIConfig(t, valid))
	if err != nil {
		t.Fatalf("Load(valid): %v", err)
	}
	if loaded.AI == nil || loaded.AI.AgentService.Endpoint != "http://127.0.0.1:8080" {
		t.Fatalf("AgentService = %+v", loaded.AI)
	}

	for name, body := range map[string]string{
		"hostname":          `{"agentService":{"endpoint":"http://localhost:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`,
		"https":             `{"agentService":{"endpoint":"https://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`,
		"missing key":       `{"agentService":{"endpoint":"http://127.0.0.1:8080"},"companions":[` + aiCompanionEntry + `]}`,
		"empty environment": `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_EMPTY"},"companions":[` + aiCompanionEntry + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeAIConfig(t, body))
			if err == nil || !strings.Contains(err.Error(), "ai.agentService") {
				t.Fatalf("Load error = %v, want agentService diagnostic", err)
			}
		})
	}
}

func TestAIConfigLegacyModelFieldsRequireMigrationWhenEnabled(t *testing.T) {
	path := writeAIConfig(t, `{"endpoint":"https://provider.invalid","model":"model","apiKeyEnv":"PROVIDER_KEY","companions":[`+aiCompanionEntry+`]}`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted legacy direct-model configuration")
	}
	for _, want := range []string{"ai.endpoint", "ai.agentService", "Python"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("migration error %q lacks %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "PROVIDER_KEY") {
		t.Errorf("migration error leaked environment variable name: %q", err)
	}

	loaded, err := Load(writeAIConfig(t, `{"endpoint":"https://provider.invalid","model":"model","apiKeyEnv":"PROVIDER_KEY"}`))
	if err != nil || loaded.AI != nil {
		t.Fatalf("disabled legacy config = %+v, %v", loaded.AI, err)
	}
}

func TestAIConfigAgentTimeoutDefaultsAndBounds(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	for _, minutes := range []int{1, 60} {
		body := `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"taskTimeoutMinutes":` + string(rune('0'+minutes)) + `,"companions":[` + aiCompanionEntry + `]}`
		if minutes == 60 {
			body = `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"taskTimeoutMinutes":60,"companions":[` + aiCompanionEntry + `]}`
		}
		if _, err := Load(writeAIConfig(t, body)); err != nil {
			t.Fatalf("timeout %d: %v", minutes, err)
		}
	}
	loaded, err := Load(writeAIConfig(t, `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[`+aiCompanionEntry+`]}`))
	if err != nil || loaded.AI.TaskTimeout() != companion.TaskTimeoutDefaultMinutes {
		t.Fatalf("default timeout = %v, %v", loaded.AI, err)
	}
	if _, ok := os.LookupEnv("MORNLEA_AGENT_TEST_KEY"); !ok {
		t.Fatal("test credential unexpectedly missing")
	}
}

func TestAIConfigAgentServiceRejectsCaseFoldCollisionsAndWarnsNestedUnknown(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	for _, body := range []string{
		`{"agentService":{"endpoint":"http://127.0.0.1:8080","ENDPOINT":"http://127.0.0.1:8081","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`,
		`{"companions":[],"Companions":[` + aiCompanionEntry + `]}`,
	} {
		if _, err := Load(writeAIConfig(t, body)); err == nil {
			t.Fatalf("Load accepted case-fold collision: %s", body)
		}
	}
}
