package companion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentClientClassifiesPlanOneOfViolationsAsInvalidModelOutput(t *testing.T) {
	tests := []struct {
		name  string
		plan  string
		valid bool
	}{
		{name: "go_to block null", plan: `{"summary":"bad","steps":[{"kind":"go_to","x":1,"y":2,"z":3,"block":null}]}`},
		{name: "go_to block empty", plan: `{"summary":"bad","steps":[{"kind":"go_to","x":1,"y":2,"z":3,"block":""}]}`},
		{name: "go_to block nonempty", plan: `{"summary":"bad","steps":[{"kind":"go_to","x":1,"y":2,"z":3,"block":"oak_planks"}]}`},
		{name: "follow x null", plan: `{"summary":"bad","steps":[{"kind":"follow","player_id":"77777777-7777-4777-8777-777777777777","x":null}]}`},
		{name: "follow x zero", plan: `{"summary":"bad","steps":[{"kind":"follow","player_id":"77777777-7777-4777-8777-777777777777","x":0}]}`},
		{name: "follow x nonzero", plan: `{"summary":"bad","steps":[{"kind":"follow","player_id":"77777777-7777-4777-8777-777777777777","x":1}]}`},
		{name: "unknown kind", plan: `{"summary":"bad","steps":[{"kind":"attack"}]}`},
		{name: "unknown step field", plan: `{"summary":"bad","steps":[{"kind":"go_to","x":1,"y":2,"z":3,"speed":1}]}`},
		{name: "wrong coordinate type", plan: `{"summary":"bad","steps":[{"kind":"go_to","x":"1","y":2,"z":3}]}`},
		{name: "required zero coordinates", plan: `{"summary":"origin","steps":[{"kind":"go_to","x":0,"y":0,"z":0}]}`, valid: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request PlanRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				response := map[string]any{
					"contract_version":   request.ContractVersion,
					"request_id":         request.RequestID,
					"client_instance_id": request.ClientInstanceID,
					"namespace_id":       request.NamespaceID,
					"lease_id":           request.LeaseID,
					"run_id":             request.RunID,
					"companion_id":       request.CompanionID,
					"generation":         request.Generation,
					"snapshot_id":        request.SnapshotID,
					"snapshot_digest":    request.SnapshotDigest,
					"plan":               json.RawMessage(testCase.plan),
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			defer server.Close()

			client, err := NewAgentClient(AgentServiceSettings{
				Endpoint: server.URL, APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
			}, "test-agent-secret", nil)
			if err != nil {
				t.Fatalf("NewAgentClient: %v", err)
			}
			defer client.Close()
			request := repairPlanRequest()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = client.Plan(ctx, request)
			if testCase.valid {
				if err != nil {
					t.Fatalf("Plan err=%v，want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrAgentInvalidModelOutput) {
				t.Fatalf("Plan err=%v，want ErrAgentInvalidModelOutput", err)
			}
			if errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("Plan err=%v，不得归为 ErrAgentUnavailable", err)
			}
		})
	}
}

func repairPlanRequest() PlanRequest {
	return PlanRequest{
		ContractVersion:  AgentContractVersion,
		RequestID:        "11111111-1111-4111-8111-111111111111",
		ClientInstanceID: "22222222-2222-4222-8222-222222222222",
		NamespaceID:      "33333333-3333-4333-8333-333333333333",
		LeaseID:          "44444444-4444-4444-8444-444444444444",
		RunID:            "55555555-5555-4555-8555-555555555555",
		CompanionID:      "66666666-6666-4666-8666-666666666666",
		Generation:       1,
		SnapshotID:       "77777777-7777-4777-8777-777777777777",
		SnapshotDigest:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DeadlineUnixMS:   time.Now().Add(time.Second).UnixMilli(),
		MCPEndpoint:      "http://127.0.0.1:12345/mcp",
		MCPCapability:    "repair-capability",
		Instruction:      "测试",
	}
}
