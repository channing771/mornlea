package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/companion"
)

func TestAgentPlannerClassifiesMalformedPlanSuccessResponseAsInvalidPlan(t *testing.T) {
	tests := []struct {
		name       string
		mutateBody func(map[string]any, []byte) []byte
		want       error
		wantReason companion.TaskFailReason
	}{
		{
			name: "response body overflow",
			mutateBody: func(_ map[string]any, body []byte) []byte {
				return append(body, bytes.Repeat([]byte(" "), companion.AgentMaxResponseBodyBytes-len(body)+1)...)
			},
			want: companion.ErrPlannerInvalidPlan, wantReason: companion.TaskFailInvalidPlan,
		},
		{
			name: "unknown top level field",
			mutateBody: func(response map[string]any, _ []byte) []byte {
				response["unexpected"] = true
				body, _ := json.Marshal(response)
				return body
			},
			want: companion.ErrPlannerInvalidPlan, wantReason: companion.TaskFailInvalidPlan,
		},
		{
			name: "trailing json",
			mutateBody: func(_ map[string]any, body []byte) []byte {
				return append(body, []byte("\n{}")...)
			},
			want: companion.ErrPlannerInvalidPlan, wantReason: companion.TaskFailInvalidPlan,
		},
		{
			name: "identity mismatch remains unavailable",
			mutateBody: func(response map[string]any, _ []byte) []byte {
				response["snapshot_digest"] = strings.Repeat("f", 64)
				body, _ := json.Marshal(response)
				return body
			},
			want: companion.ErrPlannerUnavailable, wantReason: companion.TaskFailPlannerUnavailable,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			bridge, snapshot := newPlanResponseRepairBridge(t, testCase.mutateBody)
			_, err := bridge.Plan(context.Background(), companionPlanningRequest{
				CompanionID: snapshot.Companion.ID, Generation: 9, Attempt: 3,
				Snapshot: snapshot, Instruction: "向前走",
			})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Plan err=%v，want %v", err, testCase.want)
			}
			assertPlanResponseRepairTaskReason(t, err, testCase.wantReason)
		})
	}
}

func newPlanResponseRepairBridge(
	t *testing.T,
	mutateBody func(map[string]any, []byte) []byte,
) (*companionAgentPlanner, companion.PlanSnapshot) {
	t.Helper()
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/plan" || request.Method != http.MethodPost {
			http.NotFound(w, request)
			return
		}
		var planRequest companion.PlanRequest
		if err := json.NewDecoder(request.Body).Decode(&planRequest); err != nil {
			t.Errorf("decode PlanRequest: %v", err)
			return
		}
		response := map[string]any{
			"contract_version":   planRequest.ContractVersion,
			"request_id":         planRequest.RequestID,
			"client_instance_id": planRequest.ClientInstanceID,
			"namespace_id":       planRequest.NamespaceID,
			"lease_id":           planRequest.LeaseID,
			"run_id":             planRequest.RunID,
			"companion_id":       planRequest.CompanionID,
			"generation":         planRequest.Generation,
			"snapshot_id":        planRequest.SnapshotID,
			"snapshot_digest":    planRequest.SnapshotDigest,
			"plan": map[string]any{
				"summary": "向前走",
				"steps":   []any{map[string]any{"kind": "go_to", "x": 7, "y": 65, "z": 0}},
			},
		}
		body, err := json.Marshal(response)
		if err != nil {
			t.Errorf("marshal PlanResponse: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mutateBody(response, body))
	}))
	t.Cleanup(agentServer.Close)

	client, err := companion.NewAgentClient(companion.AgentServiceSettings{
		Endpoint: agentServer.URL, APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
	}, "test-agent-secret", nil)
	if err != nil {
		t.Fatalf("NewAgentClient: %v", err)
	}
	t.Cleanup(client.Close)
	registry := companion.NewSnapshotRegistry()
	t.Cleanup(registry.Close)
	lease := &companionAgentLeaseController{
		leaseTTL: companionAgentLeaseTTL,
		lease: companionAgentLease{
			ID: "33333333-3333-4333-8333-333333333333", Fence: 1,
			Expires: time.Now().Add(time.Minute),
		},
	}
	ids := []string{
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
		"66666666-6666-4666-8666-666666666666",
	}
	bridge := newCompanionAgentPlanner(agentPlannerOptions{
		Client: client, Lease: lease, Registry: registry,
		MCPEndpoint:      "http://127.0.0.1:12345/mcp",
		ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:      "22222222-2222-4222-8222-222222222222",
		NewID: func() (string, error) {
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	})
	return bridge, testMCPPlanSnapshot(t)
}

func assertPlanResponseRepairTaskReason(
	t *testing.T,
	plannerErr error,
	want companion.TaskFailReason,
) {
	t.Helper()
	host, definition := newAgentRevalidationHost(t)
	host.world.stepMu.Lock()
	defer host.world.stepMu.Unlock()
	manager := host.world.companionManager
	manager.refreshBodies()
	body, active := manager.body(definition.ID)
	if !active {
		t.Fatal("companion body inactive")
	}
	plan := companion.Plan{Summary: "前进", Steps: []companion.PlanStep{{
		Kind: companion.PlanStepGoTo,
		X:    int32(body.Position[0]) + 1, Y: int32(body.Position[1]), Z: int32(body.Position[2]),
	}}}
	outcome := prepareAgentRevalidationOutcome(t, manager, definition, plan)
	outcome.result = companionPlanningOutcome{}
	outcome.err = plannerErr
	manager.applyPlannerOutcome(outcome)
	facts := manager.takeEventFacts()
	if len(facts) != 1 || facts[0].event.Kind != companion.TaskEventFailed ||
		facts[0].event.Reason != want {
		t.Fatalf("facts=%+v，want TaskFailed(%v)", facts, want)
	}
}
