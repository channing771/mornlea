package config

import (
	"bytes"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/companion"
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
		body := `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"taskTimeoutMinutes":` + strconv.Itoa(minutes) + `,"companions":[` + aiCompanionEntry + `]}`
		if _, err := Load(writeAIConfig(t, body)); err != nil {
			t.Fatalf("timeout %d: %v", minutes, err)
		}
	}
	for _, minutes := range []int{0, 61} {
		body := `{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"taskTimeoutMinutes":` + strconv.Itoa(minutes) + `,"companions":[` + aiCompanionEntry + `]}`
		if _, err := Load(writeAIConfig(t, body)); err == nil || !strings.Contains(err.Error(), "ai.taskTimeoutMinutes") {
			t.Fatalf("timeout %d error = %v", minutes, err)
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

func TestAIConfigAgentServiceEndpointMatrix(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	for _, endpoint := range []string{
		"http://127.0.0.1:1",
		"http://127.0.0.1:8080",
		"http://127.0.0.1:65535",
		"http://[::1]:1",
		"http://[::1]:8080",
		"http://[::1]:65535",
	} {
		body := `{"agentService":{"endpoint":` + strconv.Quote(endpoint) + `,"apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`
		if _, err := Load(writeAIConfig(t, body)); err != nil {
			t.Fatalf("endpoint %q: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"http://user@127.0.0.1:8080",
		"http://127.0.0.1:8080?query=1",
		"http://127.0.0.1:8080#fragment",
		"http://127.0.0.1:0",
		"http://127.0.0.1:70000",
		"http://[::1]:0",
		"http://[::1]:70000",
		"http://localhost:8080",
		"http://203.0.113.1:8080",
		"https://127.0.0.1:8080",
	} {
		body := `{"agentService":{"endpoint":` + strconv.Quote(endpoint) + `,"apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`
		if _, err := Load(writeAIConfig(t, body)); err == nil || !strings.Contains(err.Error(), "ai.agentService") {
			t.Fatalf("endpoint %q error = %v", endpoint, err)
		}
	}
}

func TestAIConfigDisabledFormsIgnoreAgentAndLegacySettings(t *testing.T) {
	for _, body := range []string{
		`{"agentService":{"endpoint":"https://public.invalid","apiKeyEnv":"MISSING_AGENT_KEY"}}`,
		`{"agentService":{"endpoint":"https://public.invalid","apiKeyEnv":"MISSING_AGENT_KEY"},"companions":null}`,
		`{"agentService":{"endpoint":"https://public.invalid","apiKeyEnv":"MISSING_AGENT_KEY"},"companions":[]}`,
		`{"agentService":42,"taskTimeoutMinutes":"not-an-integer"}`,
		`{"agentService":42,"taskTimeoutMinutes":0,"companions":null}`,
		`{"taskTimeoutMinutes":61,"companions":[]}`,
		`{"endpoint":null,"model":null,"apiKeyEnv":"MISSING_PROVIDER_KEY","companions":[]}`,
	} {
		loaded, err := Load(writeAIConfig(t, body))
		if err != nil || loaded.AI != nil {
			t.Fatalf("disabled config %s = %+v, %v", body, loaded.AI, err)
		}
	}
}

func TestAIConfigLegacyFieldsIndividuallyRequireMigration(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	agentService := `"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},`
	for field, value := range map[string]string{
		"endpoint":  `"https://provider.invalid"`,
		"model":     `"provider-model"`,
		"apiKeyEnv": `"PROVIDER_SECRET_ENV"`,
	} {
		body := `{` + agentService + strconv.Quote(field) + `:` + value + `,"companions":[` + aiCompanionEntry + `]}`
		_, err := Load(writeAIConfig(t, body))
		if err == nil || !strings.Contains(err.Error(), "ai."+field) || !strings.Contains(err.Error(), "ai.agentService") || !strings.Contains(err.Error(), "Python") {
			t.Fatalf("legacy %s error = %v", field, err)
		}
		if strings.Contains(err.Error(), "PROVIDER_SECRET_ENV") {
			t.Fatalf("legacy error leaked value: %v", err)
		}
	}
}

func TestAIConfigCaseFoldCollisionMatrix(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	for _, body := range []string{
		`{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"AgentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[` + aiCompanionEntry + `]}`,
		`{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"taskTimeoutMinutes":1,"TASKTIMEOUTMINUTES":2,"companions":[` + aiCompanionEntry + `]}`,
		`{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY","APIKEYENV":"OTHER"},"companions":[` + aiCompanionEntry + `]}`,
		`{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[{"id":"00112233-4455-4677-8899-aabbccddeeff","ID":"10112233-4455-4677-8899-aabbccddeeff","name":"阿木"}]}`,
		`{"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY"},"companions":[{"id":"00112233-4455-4677-8899-aabbccddeeff","name":"阿木","NAME":"阿木"}]}`,
	} {
		if _, err := Load(writeAIConfig(t, body)); err == nil || !strings.Contains(err.Error(), "大小写冲突") {
			t.Fatalf("case-fold collision %s error = %v", body, err)
		}
	}
}

func TestAIConfigNestedUnknownFieldsWarnWithExactPath(t *testing.T) {
	t.Setenv("MORNLEA_AGENT_TEST_KEY", "agent-secret")
	previous := slog.Default()
	var records bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&records, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	body := `{"futureAI":true,"agentService":{"endpoint":"http://127.0.0.1:8080","apiKeyEnv":"MORNLEA_AGENT_TEST_KEY","futureTransport":true},"companions":[{"id":"00112233-4455-4677-8899-aabbccddeeff","name":"阿木","futureCompanion":true}]}`
	loaded, err := Load(writeAIConfig(t, body))
	if err != nil || loaded.AI == nil {
		t.Fatalf("Load = %+v, %v", loaded.AI, err)
	}
	for _, path := range []string{"ai.futureAI", "ai.agentService.futureTransport", "ai.companions[0].futureCompanion"} {
		if !strings.Contains(records.String(), `"field":"`+path+`"`) {
			t.Errorf("logs %q lack %q", records.String(), path)
		}
	}
}
