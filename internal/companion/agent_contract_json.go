package companion

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

func decodeAgentObject(data []byte, out any, required []string, optional ...string) error {
	var fields map[string]agentJSONField
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("agent contract 要求 JSON object")
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, field := range required {
		allowed[field] = struct{}{}
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("agent contract 缺少字段 %q", field)
		}
	}
	for _, field := range optional {
		allowed[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("agent contract 未知字段 %q", field)
		}
	}
	return json.Unmarshal(data, out)
}

type agentJSONField struct {
	Null bool
}

func (field *agentJSONField) UnmarshalJSON(data []byte) error {
	field.Null = bytes.Equal(bytes.TrimSpace(data), []byte("null"))
	return nil
}

type liveResponseJSON LiveResponse

func (response *LiveResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*liveResponseJSON)(response), []string{"status"})
}

type readyResponseJSON ReadyResponse

func (response *ReadyResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*readyResponseJSON)(response), []string{"status"})
}

type acquireRequestJSON AcquireRequest

func (request *AcquireRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*acquireRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id",
	})
}

type acquireResponseJSON AcquireResponse

func (response *AcquireResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*acquireResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "lease_expires_in_ms",
	})
}

type leaseRequestJSON LeaseRequest

func (request *LeaseRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*leaseRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id",
	})
}

type heartbeatResponseJSON HeartbeatResponse

func (response *HeartbeatResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*heartbeatResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "lease_expires_in_ms",
	})
}

type releaseResponseJSON ReleaseResponse

func (response *ReleaseResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*releaseResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "released",
	})
}

type planRequestJSON PlanRequest

func (request *PlanRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*planRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id",
		"companion_id", "generation", "snapshot_id", "snapshot_digest", "deadline_unix_ms", "mcp_endpoint",
		"mcp_capability", "instruction",
	})
}

type planResponseJSON PlanResponse

func (response *PlanResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*planResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id",
		"companion_id", "generation", "snapshot_id", "snapshot_digest", "plan",
	})
}

type agentPlanJSON AgentPlan

func (plan *AgentPlan) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentPlanJSON)(plan), []string{"summary", "steps"})
}

type agentPlanStepJSON AgentPlanStep

func (step *AgentPlanStep) UnmarshalJSON(data []byte) error {
	var discriminator struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	var fields map[string]agentJSONField
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, ok := fields["kind"]; !ok || discriminator.Kind == "" {
		return errors.New("agent plan step 缺少合法 kind")
	}
	switch discriminator.Kind {
	case "go_to", "mine":
		return decodeAgentObject(data, (*agentPlanStepJSON)(step), []string{"kind", "x", "y", "z"})
	case "place":
		return decodeAgentObject(data, (*agentPlanStepJSON)(step), []string{"kind", "x", "y", "z", "block"})
	case "follow":
		return decodeAgentObject(data, (*agentPlanStepJSON)(step), []string{"kind", "player_id"})
	default:
		return fmt.Errorf("agent plan step kind %q 不受支持", discriminator.Kind)
	}
}

func (step AgentPlanStep) MarshalJSON() ([]byte, error) {
	switch step.Kind {
	case "go_to", "mine":
		return json.Marshal(struct {
			Kind string `json:"kind"`
			X    int32  `json:"x"`
			Y    int32  `json:"y"`
			Z    int32  `json:"z"`
		}{Kind: step.Kind, X: step.X, Y: step.Y, Z: step.Z})
	case "place":
		return json.Marshal(struct {
			Kind  string `json:"kind"`
			X     int32  `json:"x"`
			Y     int32  `json:"y"`
			Z     int32  `json:"z"`
			Block string `json:"block"`
		}{Kind: step.Kind, X: step.X, Y: step.Y, Z: step.Z, Block: step.Block})
	case "follow":
		return json.Marshal(struct {
			Kind     string `json:"kind"`
			PlayerID string `json:"player_id"`
		}{Kind: step.Kind, PlayerID: step.PlayerID})
	default:
		return nil, fmt.Errorf("agent plan step kind %q 不受支持", step.Kind)
	}
}

type agentBlockPositionJSON AgentBlockPosition

func (position *AgentBlockPosition) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentBlockPositionJSON)(position), []string{"x", "y", "z"})
}

type agentVisibleBlockJSON AgentVisibleBlock

func (block *AgentVisibleBlock) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentVisibleBlockJSON)(block), []string{"position", "block_id"})
}

type agentHeightJSON AgentHeight

func (height *AgentHeight) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentHeightJSON)(height), []string{"x", "z", "height"})
}

type agentDialogueEnvironmentJSON AgentDialogueEnvironment

func (environment *AgentDialogueEnvironment) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentDialogueEnvironmentJSON)(environment), []string{"exposed_blocks", "heights"})
}

type agentDialogueFactJSON AgentDialogueFact

func (fact *AgentDialogueFact) UnmarshalJSON(data []byte) error {
	var discriminator struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	var fields map[string]agentJSONField
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, ok := fields["kind"]; !ok || discriminator.Kind == "" {
		return errors.New("agent dialogue fact 缺少合法 kind")
	}
	switch discriminator.Kind {
	case "start", "first_arrival", "idle":
		return decodeAgentObject(data, (*agentDialogueFactJSON)(fact), []string{"kind"})
	case "progress":
		return decodeAgentObject(data, (*agentDialogueFactJSON)(fact), []string{"kind", "step_kind"})
	case "terminal":
		return decodeAgentObject(data, (*agentDialogueFactJSON)(fact), []string{"kind", "state", "reason"})
	default:
		return fmt.Errorf("agent dialogue fact kind %q 不受支持", discriminator.Kind)
	}
}

func (fact AgentDialogueFact) MarshalJSON() ([]byte, error) {
	switch fact.Kind {
	case "start", "first_arrival", "idle":
		return json.Marshal(struct {
			Kind string `json:"kind"`
		}{Kind: fact.Kind})
	case "progress":
		return json.Marshal(struct {
			Kind     string `json:"kind"`
			StepKind string `json:"step_kind"`
		}{Kind: fact.Kind, StepKind: fact.StepKind})
	case "terminal":
		return json.Marshal(struct {
			Kind   string `json:"kind"`
			State  string `json:"state"`
			Reason string `json:"reason"`
		}{Kind: fact.Kind, State: fact.State, Reason: fact.Reason})
	default:
		return nil, fmt.Errorf("agent dialogue fact kind %q 不受支持", fact.Kind)
	}
}

type agentMemoryStateJSON AgentMemoryState

func (state *AgentMemoryState) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentMemoryStateJSON)(state), []string{"revision", "operation_id", "summary"})
}

type agentMemoryProposalJSON AgentMemoryProposal

func (proposal *AgentMemoryProposal) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentMemoryProposalJSON)(proposal), []string{"operation_id", "base_revision", "summary"})
}

type agentDialogueRequestJSON AgentDialogueRequest

func (request *AgentDialogueRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentDialogueRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id",
		"companion_id", "generation", "memory_epoch", "deadline_unix_ms", "persona", "fact_node", "environment", "terminal",
	})
}

type agentDialogueResponseJSON AgentDialogueResponse

func (response *AgentDialogueResponse) UnmarshalJSON(data []byte) error {
	var fields map[string]agentJSONField
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	required := []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id",
		"companion_id", "generation", "memory_epoch", "line",
	}
	if proposal, terminal := fields["memory_proposal"]; terminal {
		if proposal.Null {
			return errors.New("agent dialogue response 的 memory_proposal 不得为 null")
		}
		required = append(required, "memory_proposal")
	}
	return decodeAgentObject(data, (*agentDialogueResponseJSON)(response), required)
}

type memoryReconcileRequestJSON MemoryReconcileRequest

func (request *MemoryReconcileRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*memoryReconcileRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id",
		"memory_epoch", "active", "tombstone_operation_id", "mirror",
	})
}

type memoryReconcileResponseJSON MemoryReconcileResponse

func (response *MemoryReconcileResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*memoryReconcileResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id",
		"memory_epoch", "active", "tombstone_operation_id", "memory",
	})
}

type memoryCommitRequestJSON MemoryCommitRequest

func (request *MemoryCommitRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*memoryCommitRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id",
		"memory_epoch", "base_revision", "operation_id", "summary",
	})
}

type memoryCommitResponseJSON MemoryCommitResponse

func (response *MemoryCommitResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*memoryCommitResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id",
		"memory_epoch", "operation_id", "committed_revision",
	})
}

type memoryDeleteRequestJSON MemoryDeleteRequest

func (request *MemoryDeleteRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*memoryDeleteRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id",
		"old_memory_epoch", "new_memory_epoch", "tombstone_operation_id",
	})
}

type memoryDeleteResponseJSON MemoryDeleteResponse

func (response *MemoryDeleteResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*memoryDeleteResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id",
		"memory_epoch", "tombstone_operation_id",
	})
}

type cancelRequestJSON CancelRequest

func (request *CancelRequest) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*cancelRequestJSON)(request), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id",
	})
}

type cancelResponseJSON CancelResponse

func (response *CancelResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*cancelResponseJSON)(response), []string{
		"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id", "cancelled",
	})
}

type agentErrorResponse struct {
	ContractVersion string           `json:"contract_version"`
	RequestID       *string          `json:"request_id"`
	Error           agentErrorDetail `json:"error"`
}

type agentErrorDetail struct {
	Code string `json:"code"`
}

type agentErrorDetailJSON agentErrorDetail

func (detail *agentErrorDetail) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentErrorDetailJSON)(detail), []string{"code"})
}

type agentErrorResponseJSON agentErrorResponse

func (response *agentErrorResponse) UnmarshalJSON(data []byte) error {
	return decodeAgentObject(data, (*agentErrorResponseJSON)(response), []string{"contract_version", "request_id", "error"})
}
