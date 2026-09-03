package companion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentClientRound1RejectsDuplicateResponseObjectKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live","status":"live"}`))
	}))
	defer server.Close()
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "AGENT_TEST_KEY"}, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Live(context.Background()); !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("duplicate response error = %v, want unavailable", err)
	}
}

func TestAgentClientRound1RejectsInvalidRequestBeforeNetwork(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "AGENT_TEST_KEY"}, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	request := AcquireRequest{ContractVersion: "v2", RequestID: agentRequestID, ClientInstanceID: agentClientID, NamespaceID: agentNamespace}
	if _, err := client.Acquire(context.Background(), request); !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("Acquire error = %v", err)
	}
	if called {
		t.Fatal("invalid request reached network")
	}
}

func TestAgentClientRound1Ready503NotReadyIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready"}`))
	}))
	defer server.Close()
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "AGENT_TEST_KEY"}, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	got, err := client.Ready(context.Background())
	if err != nil || got.Status != "not_ready" {
		t.Fatalf("Ready = %+v, %v", got, err)
	}
}
