package companion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type agentGoldenFile struct {
	Cases []agentGoldenCase `json:"cases"`
}
type agentGoldenCase struct {
	Name   string          `json:"name"`
	Schema string          `json:"schema"`
	Value  json.RawMessage `json:"value"`
}

func TestAgentContractGoldenDrivesActualDTOCodecs(t *testing.T) {
	root := filepath.Join("..", "..", "packages", "contracts", "companion-agent", "http-v1", "golden")
	for _, fixtureName := range []string{"valid.json", "invalid.json"} {
		contents, err := os.ReadFile(filepath.Join(root, fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		var fixture agentGoldenFile
		if err := json.Unmarshal(contents, &fixture); err != nil {
			t.Fatal(err)
		}
		for _, item := range fixture.Cases {
			t.Run(fixtureName+"/"+item.Name, func(t *testing.T) {
				valid := agentGoldenCodecValid(item.Schema, item.Value)
				if fixtureName == "valid.json" && !valid {
					t.Fatal("checked-in valid fixture rejected by actual DTO codec")
				}
				if fixtureName == "invalid.json" && valid {
					t.Fatal("checked-in invalid fixture accepted by actual DTO codec")
				}
			})
		}
	}
}

func agentGoldenCodecValid(schema string, value []byte) bool {
	switch schema {
	case "live_response":
		var v LiveResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "ready_response":
		var v ReadyResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "acquire_request":
		var v AcquireRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "acquire_response":
		var v AcquireResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "lease_request":
		var v LeaseRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "heartbeat_response":
		var v HeartbeatResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "release_response":
		var v ReleaseResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "plan_request":
		var v PlanRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "plan_response":
		var v PlanResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "dialogue_request", "dialogue_nonterminal_request", "dialogue_terminal_request":
		var v AgentDialogueRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "dialogue_response", "dialogue_nonterminal_response", "dialogue_terminal_response":
		var v AgentDialogueResponse
		if strictDecodeJSON(value, &v) != nil || !validAgentResponse(&v) {
			return false
		}
		if schema == "dialogue_nonterminal_response" {
			return v.MemoryProposal == nil
		}
		if schema == "dialogue_terminal_response" {
			return v.MemoryProposal != nil
		}
		return true
	case "memory_reconcile_request", "memory_reconcile_active_request", "memory_reconcile_inactive_request":
		var v MemoryReconcileRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "memory_reconcile_response", "memory_reconcile_active_response", "memory_reconcile_inactive_response":
		var v MemoryReconcileResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "memory_commit_request":
		var v MemoryCommitRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "memory_commit_response":
		var v MemoryCommitResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "memory_delete_request":
		var v MemoryDeleteRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "memory_delete_response":
		var v MemoryDeleteResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "cancel_request":
		var v CancelRequest
		return strictDecodeJSON(value, &v) == nil && validAgentRequest(v)
	case "cancel_response":
		var v CancelResponse
		return strictDecodeJSON(value, &v) == nil && validAgentResponse(&v)
	case "dialogue_terminal_fact_node", "dialogue_terminal_failed_fact_node", "dialogue_terminal_nonfailed_fact_node":
		var v AgentDialogueFact
		return strictDecodeJSON(value, &v) == nil && validDialogueFact(v, true)
	case "dialogue_nonterminal_fact_node":
		var v AgentDialogueFact
		return strictDecodeJSON(value, &v) == nil && validDialogueFact(v, false)
	case "dialogue_line":
		var v string
		return strictDecodeJSON(value, &v) == nil && validDialogueLine(v)
	case "persona_text":
		var v string
		return strictDecodeJSON(value, &v) == nil && validAgentMemoryText(v, 4096)
	case "memory_summary":
		var v string
		return strictDecodeJSON(value, &v) == nil && validAgentMemoryText(v, 2048)
	case "mcp_endpoint":
		var v string
		return strictDecodeJSON(value, &v) == nil && validMCPEndpoint(v)
	case "instruction_text":
		var v string
		return strictDecodeJSON(value, &v) == nil && validAgentNonBlankText(v, 1024)
	case "memory_state_zero":
		var v AgentMemoryState
		return strictDecodeJSON(value, &v) == nil && validMemoryState(v)
	case "error_response":
		var v agentErrorResponse
		return validAgentErrorEnvelope(value, &v)
	}
	panic(fmt.Sprintf("未处理的 Agent golden schema %q", schema))
}
