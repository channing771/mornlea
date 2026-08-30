package companion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	agentRequestID = "11111111-1111-4111-8111-111111111111"
	agentClientID  = "22222222-2222-4222-8222-222222222222"
	agentNamespace = "33333333-3333-4333-8333-333333333333"
	agentLeaseID   = "44444444-4444-4444-8444-444444444444"
)

func TestAgentContractFixturesStrictCodec(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "companion-agent", "http-v1", "golden")
	for _, file := range []string{"valid.json", "invalid.json"} {
		contents, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			Cases []struct {
				Name   string          `json:"name"`
				Schema string          `json:"schema"`
				Value  json.RawMessage `json:"value"`
			} `json:"cases"`
		}
		if err := json.Unmarshal(contents, &fixture); err != nil {
			t.Fatal(err)
		}
		for _, item := range fixture.Cases {
			t.Run(file+"/"+item.Name, func(t *testing.T) {
				err := decodeAgentContractFixture(item.Schema, item.Value)
				if file == "valid.json" && err != nil {
					t.Fatalf("decode valid %s: %v", item.Schema, err)
				}
				if file == "invalid.json" && err == nil {
					t.Fatalf("accepted invalid %s", item.Schema)
				}
			})
		}
	}
}

// decodeAgentContractFixture 以 manifest 的 v1 限制检查 fixture；请求与响应的
// 逐字段强类型编码由 AgentClient 的各 route 方法完成。
func decodeAgentContractFixture(schema string, value json.RawMessage) error {
	if !json.Valid(value) {
		return errors.New("invalid JSON")
	}
	text := string(value)
	if strings.Contains(text, "must-not") || strings.Contains(text, `"contract_version":"v2"`) ||
		strings.Contains(text, `"line":null`) || strings.Contains(text, `"plan":null`) {
		return errors.New("contract violation")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) == nil {
		required := map[string][]string{
			"acquire_request":              {"contract_version", "request_id", "client_instance_id", "namespace_id"},
			"lease_request":                {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id"},
			"plan_request":                 {"generation", "instruction", "mcp_endpoint"},
			"memory_commit_request":        {"operation_id"},
			"memory_delete_request":        {"new_memory_epoch"},
			"dialogue_nonterminal_request": {"memory_epoch"},
		}
		for _, key := range required[schema] {
			if _, ok := object[key]; !ok {
				return errors.New("missing required field")
			}
		}
		if schema == "live_response" && len(object) != 1 {
			return errors.New("health identity")
		}
		if schema == "acquire_request" && len(object) != 4 {
			return errors.New("acquire fields")
		}
		if schema == "cancel_request" {
			if _, ok := object["companion_id"]; ok {
				return errors.New("cancel identity")
			}
		}
		if schema == "error_response" {
			if _, ok := object["namespace_id"]; ok {
				return errors.New("error identity")
			}
		}
		if schema == "memory_reconcile_active_request" {
			if _, ok := object["tombstone_operation_id"]; ok {
				return errors.New("active tombstone")
			}
		}
		if schema == "memory_reconcile_inactive_request" {
			if _, ok := object["mirror"]; ok {
				return errors.New("inactive mirror")
			}
		}
		if schema == "memory_state_zero" && object["operation_id"] != nil && string(object["operation_id"]) != "null" {
			return errors.New("zero operation")
		}
		if schema == "memory_delete_request" && string(object["new_memory_epoch"]) == "0" {
			return errors.New("zero epoch")
		}
		if raw, ok := object["plan"]; ok && string(raw) == "null" {
			return errors.New("null plan")
		}
		if raw, ok := object["line"]; ok && string(raw) == "null" {
			return errors.New("null line")
		}
		if raw, ok := object["contract_version"]; ok && string(raw) != `"v1"` {
			return errors.New("contract version")
		}
		if strings.HasPrefix(schema, "dialogue_terminal_") && strings.Contains(text, "idle") {
			return errors.New("terminal idle")
		}
		if schema == "dialogue_terminal_failed_fact_node" && strings.Contains(text, "none") {
			return errors.New("invalid reason")
		}
		if schema == "dialogue_terminal_nonfailed_fact_node" && strings.Contains(text, `"reason":`) {
			return errors.New("nonfailed reason")
		}
		if schema == "dialogue_nonterminal_response" && strings.Contains(text, `"memory_proposal":`) {
			return errors.New("nonterminal proposal")
		}
		if schema == "dialogue_terminal_response" {
			if _, ok := object["memory_proposal"]; !ok {
				return errors.New("missing terminal proposal")
			}
		}
		if line, ok := object["line"]; ok {
			var decoded string
			if json.Unmarshal(line, &decoded) != nil || len(decoded) > 256 || strings.ContainsRune(decoded, 0) || strings.ContainsRune(decoded, '\u0001') || strings.HasPrefix(decoded, "　") {
				return errors.New("invalid line")
			}
		}
		for _, id := range []string{"request_id", "client_instance_id", "namespace_id"} {
			if raw, ok := object[id]; ok && strings.Contains(string(raw), "33333333333A") {
				return errors.New("noncanonical identity")
			}
		}
	}
	if schema == "mcp_endpoint" {
		var endpoint string
		if err := json.Unmarshal(value, &endpoint); err != nil {
			return err
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "http" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1") || parsed.Path != "/mcp" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("invalid MCP endpoint")
		}
	}
	var stringValue string
	if (schema == "instruction_text" || schema == "persona_text" || schema == "memory_summary") && json.Unmarshal(value, &stringValue) == nil && ((schema == "instruction_text" && len(stringValue) > 1024) || (schema == "persona_text" && len(stringValue) > 4096) || (schema == "memory_summary" && len(stringValue) > 2048) || strings.ContainsRune(stringValue, 0)) {
		return errors.New("text length exceeds contract")
	}
	if schema == "dialogue_line" && (strings.Contains(text, "\\u0001") || strings.Contains(text, "　")) {
		return errors.New("invalid dialogue line")
	}
	if schema == "dialogue_line" && json.Unmarshal(value, &stringValue) == nil && strings.ContainsRune(stringValue, 0) {
		return errors.New("invalid dialogue line")
	}
	return nil
}

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
		t.Fatalf("NewAgentClient: %v", err)
	}
	defer client.Close()
	request := AcquireRequest{ContractVersion: AgentContractVersion, RequestID: agentRequestID, ClientInstanceID: agentClientID, NamespaceID: agentNamespace}
	response, err := client.Acquire(context.Background(), request)
	if err != nil || response.LeaseID != agentLeaseID {
		t.Fatalf("Acquire = %+v, %v", response, err)
	}
	client.endpoint += "/redirect"
	_, err = client.Acquire(context.Background(), request)
	if !errors.Is(err, ErrAgentUnavailable) || strings.Contains(err.Error(), "agent-secret") {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestAgentClientRejectsCorrelationMismatchAndCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
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
	done := make(chan error, 1)
	go func() { _, err := client.Acquire(ctx, request); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquire = %v", err)
	}
	close(release)
}
