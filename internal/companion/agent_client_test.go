package companion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	agentRequestID = "11111111-1111-4111-8111-111111111111"
	agentClientID  = "22222222-2222-4222-8222-222222222222"
	agentNamespace = "33333333-3333-4333-8333-333333333333"
	agentLeaseID   = "44444444-4444-4444-8444-444444444444"
)

func TestAgentClientAcquireSendsBearerAndRejectsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, "/v1/namespaces/acquire", http.StatusFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer agent-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept-Encoding"); got != "identity" {
			t.Fatalf("Accept-Encoding = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"contract_version":"v1","request_id":"` + agentRequestID + `","client_instance_id":"` + agentClientID + `","namespace_id":"` + agentNamespace + `","lease_id":"` + agentLeaseID + `","lease_expires_in_ms":15000}`))
	}))
	defer server.Close()
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "AGENT_TEST_KEY"}, "agent-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	request := AcquireRequest{ContractVersion: AgentContractVersion, RequestID: agentRequestID, ClientInstanceID: agentClientID, NamespaceID: agentNamespace}
	response, err := client.Acquire(context.Background(), request)
	if err != nil || response.LeaseID != agentLeaseID {
		t.Fatalf("Acquire = %+v, %v", response, err)
	}
	client.endpoint += "/redirect"
	if _, err = client.Acquire(context.Background(), request); !errors.Is(err, ErrAgentUnavailable) || strings.Contains(err.Error(), "agent-secret") {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestAgentClientRejectsCorrelationMismatchAndCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/wait") {
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"contract_version":"v1","request_id":"55555555-5555-4555-8555-555555555555","client_instance_id":"` + agentClientID + `","namespace_id":"` + agentNamespace + `","lease_id":"` + agentLeaseID + `","lease_expires_in_ms":15000}`))
	}))
	defer server.Close()
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "AGENT_TEST_KEY"}, "agent-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	request := AcquireRequest{ContractVersion: AgentContractVersion, RequestID: agentRequestID, ClientInstanceID: agentClientID, NamespaceID: agentNamespace}
	if _, err := client.Acquire(context.Background(), request); !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("correlation mismatch = %v", err)
	}
	client.endpoint += "/wait"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		response AcquireResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Acquire(ctx, request)
		done <- struct {
			response AcquireResponse
			err      error
		}{response: response, err: err}
	}()
	<-started
	cancel()
	result := <-done
	if result.response != (AcquireResponse{}) || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancelled acquire = %+v, %v", result.response, result.err)
	}
	close(release)
}
