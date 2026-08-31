package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	// AgentContractVersion 是伙伴 Agent HTTP application contract 的当前版本。
	AgentContractVersion = "v1"
	// AgentMaxRequestBodyBytes 限制发送到 Agent 的 JSON body。
	AgentMaxRequestBodyBytes = 256 << 10
	// AgentMaxResponseBodyBytes 限制从 Agent 读取的 JSON body。
	AgentMaxResponseBodyBytes = 64 << 10
	// AgentMaxHeaderBytes 限制 HTTP response header。
	AgentMaxHeaderBytes = 16 << 10
)

var (
	// ErrAgentUnavailable 表示传输、协议、认证或关联校验失败，正文不会进入错误。
	ErrAgentUnavailable = errors.New("companion: agent 不可用")
	// ErrAgentInvalidModelOutput 表示 Agent 已验证的模型输出不符合计划合同。
	ErrAgentInvalidModelOutput = errors.New("companion: agent 返回非法模型输出")
)

// AgentError 是 Agent manifest 枚举的稳定错误。它不保存服务端原始正文。
type AgentError struct {
	Code string
}

func (e *AgentError) Error() string { return "companion: agent error " + e.Code }

type agentIdentity struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
}

// AcquireRequest 建立 namespace fencing lease。
type AcquireRequest struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
}
type AcquireResponse struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	LeaseExpiresInMS int    `json:"lease_expires_in_ms"`
}
type LeaseRequest struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
}
type HeartbeatResponse struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	LeaseExpiresInMS int    `json:"lease_expires_in_ms"`
}
type ReleaseResponse struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	Released         bool   `json:"released"`
}
type LiveResponse struct {
	Status string `json:"status"`
}
type ReadyResponse struct {
	Status string `json:"status"`
}
type PlanRequest struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	RunID            string `json:"run_id"`
	CompanionID      string `json:"companion_id"`
	Generation       uint64 `json:"generation"`
	SnapshotID       string `json:"snapshot_id"`
	SnapshotDigest   string `json:"snapshot_digest"`
	DeadlineUnixMS   int64  `json:"deadline_unix_ms"`
	MCPEndpoint      string `json:"mcp_endpoint"`
	MCPCapability    string `json:"mcp_capability"`
	Instruction      string `json:"instruction"`
}
type PlanResponse struct {
	ContractVersion  string    `json:"contract_version"`
	RequestID        string    `json:"request_id"`
	ClientInstanceID string    `json:"client_instance_id"`
	NamespaceID      string    `json:"namespace_id"`
	LeaseID          string    `json:"lease_id"`
	RunID            string    `json:"run_id"`
	CompanionID      string    `json:"companion_id"`
	Generation       uint64    `json:"generation"`
	SnapshotID       string    `json:"snapshot_id"`
	SnapshotDigest   string    `json:"snapshot_digest"`
	Plan             AgentPlan `json:"plan"`
}

// AgentPlan 是 Agent HTTP contract 的候选计划 wire DTO；它不直接成为权威任务。
type AgentPlan struct {
	Summary string          `json:"summary"`
	Steps   []AgentPlanStep `json:"steps"`
}
type AgentPlanStep struct {
	Kind     string `json:"kind"`
	X        int32  `json:"x,omitempty"`
	Y        int32  `json:"y,omitempty"`
	Z        int32  `json:"z,omitempty"`
	Block    string `json:"block,omitempty"`
	PlayerID string `json:"player_id,omitempty"`
}
type AgentBlockPosition struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	Z int32 `json:"z"`
}
type AgentVisibleBlock struct {
	Position AgentBlockPosition `json:"position"`
	BlockID  uint16             `json:"block_id"`
}
type AgentHeight struct {
	X      int32 `json:"x"`
	Z      int32 `json:"z"`
	Height int16 `json:"height"`
}
type AgentDialogueEnvironment struct {
	ExposedBlocks []AgentVisibleBlock `json:"exposed_blocks"`
	Heights       []AgentHeight       `json:"heights"`
}

// AgentDialogueFact 是由 kind/state 组成的 closed union；Validate 区分 terminal 与普通节点。
type AgentDialogueFact struct {
	Kind     string `json:"kind"`
	StepKind string `json:"step_kind,omitempty"`
	State    string `json:"state,omitempty"`
	Reason   string `json:"reason,omitempty"`
}
type AgentMemoryState struct {
	Revision    uint64  `json:"revision"`
	OperationID *string `json:"operation_id"`
	Summary     string  `json:"summary"`
}
type AgentMemoryProposal struct {
	OperationID  string `json:"operation_id"`
	BaseRevision uint64 `json:"base_revision"`
	Summary      string `json:"summary"`
}
type AgentDialogueRequest struct {
	ContractVersion  string                   `json:"contract_version"`
	RequestID        string                   `json:"request_id"`
	ClientInstanceID string                   `json:"client_instance_id"`
	NamespaceID      string                   `json:"namespace_id"`
	LeaseID          string                   `json:"lease_id"`
	RunID            string                   `json:"run_id"`
	CompanionID      string                   `json:"companion_id"`
	Generation       uint64                   `json:"generation"`
	MemoryEpoch      uint64                   `json:"memory_epoch"`
	DeadlineUnixMS   int64                    `json:"deadline_unix_ms"`
	Persona          string                   `json:"persona"`
	FactNode         AgentDialogueFact        `json:"fact_node"`
	Environment      AgentDialogueEnvironment `json:"environment"`
	Terminal         bool                     `json:"terminal"`
}
type AgentDialogueResponse struct {
	ContractVersion  string               `json:"contract_version"`
	RequestID        string               `json:"request_id"`
	ClientInstanceID string               `json:"client_instance_id"`
	NamespaceID      string               `json:"namespace_id"`
	LeaseID          string               `json:"lease_id"`
	RunID            string               `json:"run_id"`
	CompanionID      string               `json:"companion_id"`
	Generation       uint64               `json:"generation"`
	MemoryEpoch      uint64               `json:"memory_epoch"`
	Line             string               `json:"line"`
	MemoryProposal   *AgentMemoryProposal `json:"memory_proposal,omitempty"`
}
type MemoryReconcileRequest struct {
	ContractVersion      string            `json:"contract_version"`
	RequestID            string            `json:"request_id"`
	ClientInstanceID     string            `json:"client_instance_id"`
	NamespaceID          string            `json:"namespace_id"`
	LeaseID              string            `json:"lease_id"`
	CompanionID          string            `json:"companion_id"`
	MemoryEpoch          uint64            `json:"memory_epoch"`
	Active               bool              `json:"active"`
	Mirror               *AgentMemoryState `json:"mirror"`
	TombstoneOperationID *string           `json:"tombstone_operation_id"`
}
type MemoryReconcileResponse struct {
	ContractVersion      string            `json:"contract_version"`
	RequestID            string            `json:"request_id"`
	ClientInstanceID     string            `json:"client_instance_id"`
	NamespaceID          string            `json:"namespace_id"`
	LeaseID              string            `json:"lease_id"`
	CompanionID          string            `json:"companion_id"`
	MemoryEpoch          uint64            `json:"memory_epoch"`
	Active               bool              `json:"active"`
	Memory               *AgentMemoryState `json:"memory"`
	TombstoneOperationID *string           `json:"tombstone_operation_id"`
}
type MemoryCommitRequest struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	CompanionID      string `json:"companion_id"`
	MemoryEpoch      uint64 `json:"memory_epoch"`
	BaseRevision     uint64 `json:"base_revision"`
	OperationID      string `json:"operation_id"`
	Summary          string `json:"summary"`
}
type MemoryCommitResponse struct {
	ContractVersion   string `json:"contract_version"`
	RequestID         string `json:"request_id"`
	ClientInstanceID  string `json:"client_instance_id"`
	NamespaceID       string `json:"namespace_id"`
	LeaseID           string `json:"lease_id"`
	CompanionID       string `json:"companion_id"`
	MemoryEpoch       uint64 `json:"memory_epoch"`
	OperationID       string `json:"operation_id"`
	CommittedRevision uint64 `json:"committed_revision"`
}
type MemoryDeleteRequest struct {
	ContractVersion      string `json:"contract_version"`
	RequestID            string `json:"request_id"`
	ClientInstanceID     string `json:"client_instance_id"`
	NamespaceID          string `json:"namespace_id"`
	LeaseID              string `json:"lease_id"`
	CompanionID          string `json:"companion_id"`
	OldMemoryEpoch       uint64 `json:"old_memory_epoch"`
	NewMemoryEpoch       uint64 `json:"new_memory_epoch"`
	TombstoneOperationID string `json:"tombstone_operation_id"`
}
type MemoryDeleteResponse struct {
	ContractVersion      string `json:"contract_version"`
	RequestID            string `json:"request_id"`
	ClientInstanceID     string `json:"client_instance_id"`
	NamespaceID          string `json:"namespace_id"`
	LeaseID              string `json:"lease_id"`
	CompanionID          string `json:"companion_id"`
	MemoryEpoch          uint64 `json:"memory_epoch"`
	TombstoneOperationID string `json:"tombstone_operation_id"`
}
type CancelRequest struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	RunID            string `json:"run_id"`
}
type CancelResponse struct {
	ContractVersion  string `json:"contract_version"`
	RequestID        string `json:"request_id"`
	ClientInstanceID string `json:"client_instance_id"`
	NamespaceID      string `json:"namespace_id"`
	LeaseID          string `json:"lease_id"`
	RunID            string `json:"run_id"`
	Cancelled        bool   `json:"cancelled"`
}

// AgentClient 是有界、无重试的 Agent HTTP v1 调用方。
type AgentClient struct {
	endpoint   string
	credential string
	http       *http.Client
	lifetime   context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
}

func NewAgentClient(settings AgentServiceSettings, credential string, client *http.Client) (*AgentClient, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	if credential == "" {
		return nil, errors.New("companion: agent credential 为空")
	}
	// Agent transport 由 client 专有，避免外部 client 的 proxy、redirect、retry 或
	// idle connection 生命周期穿透 loopback/credential 边界。保留参数仅为已有
	// 构造调用的源码兼容，不能把不受控 transport 注入生产 client。
	_ = client
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	transport.MaxResponseHeaderBytes = AgentMaxHeaderBytes
	client = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	lifetime, cancel := context.WithCancel(context.Background())
	return &AgentClient{endpoint: trimTrailingSlash(settings.Endpoint), credential: credential, http: client, lifetime: lifetime, cancel: cancel}, nil
}

func (c *AgentClient) Close() { c.closeOnce.Do(func() { c.cancel(); c.http.CloseIdleConnections() }) }

func (c *AgentClient) Live(ctx context.Context) (LiveResponse, error) {
	var out LiveResponse
	err := c.get(ctx, "/livez", false, &out)
	return out, err
}
func (c *AgentClient) Ready(ctx context.Context) (ReadyResponse, error) {
	var out ReadyResponse
	err := c.get(ctx, "/readyz", true, &out)
	if err == errAgentNotReady {
		return out, nil
	}
	return out, err
}
func (c *AgentClient) Acquire(ctx context.Context, in AcquireRequest) (AcquireResponse, error) {
	var out AcquireResponse
	err := c.post(ctx, "/v1/namespaces/acquire", in, &out)
	if err == nil && (out.RequestID != in.RequestID || out.ClientInstanceID != in.ClientInstanceID || out.NamespaceID != in.NamespaceID) {
		err = ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) Heartbeat(ctx context.Context, in LeaseRequest) (HeartbeatResponse, error) {
	var out HeartbeatResponse
	err := c.post(ctx, "/v1/namespaces/heartbeat", in, &out)
	if err == nil && !sameLeaseIdentity(in, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) {
		return HeartbeatResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) Release(ctx context.Context, in LeaseRequest) (ReleaseResponse, error) {
	var out ReleaseResponse
	err := c.post(ctx, "/v1/namespaces/release", in, &out)
	if err == nil && !sameLeaseIdentity(in, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) {
		return ReleaseResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) Plan(ctx context.Context, in PlanRequest) (PlanResponse, error) {
	var out PlanResponse
	err := c.post(ctx, "/v1/plan", in, &out)
	if err == nil && !(sameLeaseIdentity(LeaseRequest{in.ContractVersion, in.RequestID, in.ClientInstanceID, in.NamespaceID, in.LeaseID}, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) && out.RunID == in.RunID && out.CompanionID == in.CompanionID && out.Generation == in.Generation && out.SnapshotID == in.SnapshotID && out.SnapshotDigest == in.SnapshotDigest) {
		return PlanResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) Dialogue(ctx context.Context, in AgentDialogueRequest) (AgentDialogueResponse, error) {
	var out AgentDialogueResponse
	err := c.post(ctx, "/v1/dialogue", in, &out)
	if err == nil && !(sameLeaseIdentity(LeaseRequest{in.ContractVersion, in.RequestID, in.ClientInstanceID, in.NamespaceID, in.LeaseID}, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) && out.RunID == in.RunID && out.CompanionID == in.CompanionID && out.Generation == in.Generation && out.MemoryEpoch == in.MemoryEpoch && ((in.Terminal && out.MemoryProposal != nil) || (!in.Terminal && out.MemoryProposal == nil))) {
		return AgentDialogueResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) ReconcileMemory(ctx context.Context, in MemoryReconcileRequest) (MemoryReconcileResponse, error) {
	var out MemoryReconcileResponse
	err := c.post(ctx, "/v1/memory/reconcile", in, &out)
	if err == nil && !(sameLeaseIdentity(LeaseRequest{in.ContractVersion, in.RequestID, in.ClientInstanceID, in.NamespaceID, in.LeaseID}, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) && out.CompanionID == in.CompanionID && out.MemoryEpoch == in.MemoryEpoch && out.Active == in.Active) {
		return MemoryReconcileResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) CommitMemory(ctx context.Context, in MemoryCommitRequest) (MemoryCommitResponse, error) {
	var out MemoryCommitResponse
	err := c.post(ctx, "/v1/memory/commit", in, &out)
	if err == nil && !(sameLeaseIdentity(LeaseRequest{in.ContractVersion, in.RequestID, in.ClientInstanceID, in.NamespaceID, in.LeaseID}, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) && out.CompanionID == in.CompanionID && out.MemoryEpoch == in.MemoryEpoch && out.OperationID == in.OperationID) {
		return MemoryCommitResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) DeleteMemory(ctx context.Context, in MemoryDeleteRequest) (MemoryDeleteResponse, error) {
	var out MemoryDeleteResponse
	err := c.post(ctx, "/v1/memory/delete", in, &out)
	if err == nil && !(sameLeaseIdentity(LeaseRequest{in.ContractVersion, in.RequestID, in.ClientInstanceID, in.NamespaceID, in.LeaseID}, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) && out.CompanionID == in.CompanionID && out.MemoryEpoch == in.NewMemoryEpoch && out.TombstoneOperationID == in.TombstoneOperationID) {
		return MemoryDeleteResponse{}, ErrAgentUnavailable
	}
	return out, err
}
func (c *AgentClient) CancelRun(ctx context.Context, in CancelRequest) (CancelResponse, error) {
	var out CancelResponse
	err := c.post(ctx, "/v1/runs/cancel", in, &out)
	if err == nil && !(sameLeaseIdentity(LeaseRequest{in.ContractVersion, in.RequestID, in.ClientInstanceID, in.NamespaceID, in.LeaseID}, out.ContractVersion, out.RequestID, out.ClientInstanceID, out.NamespaceID, out.LeaseID) && out.RunID == in.RunID) {
		return CancelResponse{}, ErrAgentUnavailable
	}
	return out, err
}

func sameLeaseIdentity(in LeaseRequest, version, requestID, clientID, namespaceID, leaseID string) bool {
	return in.ContractVersion == version && in.RequestID == requestID && in.ClientInstanceID == clientID && in.NamespaceID == namespaceID && in.LeaseID == leaseID
}

func (c *AgentClient) get(ctx context.Context, path string, authenticated bool, out interface{}) error {
	requestContext, stop := c.context(ctx)
	defer stop()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return fmt.Errorf("%w: construct request", ErrAgentUnavailable)
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+c.credential)
	}
	request.Header.Set("Accept-Encoding", "identity")
	if requestHeaderBytes(request.Header) > AgentMaxHeaderBytes {
		return ErrAgentUnavailable
	}
	return c.send(request, out, "")
}
func (c *AgentClient) post(ctx context.Context, path string, value interface{}, out interface{}) error {
	if !validAgentRequest(value) {
		return ErrAgentUnavailable
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > AgentMaxRequestBodyBytes {
		return fmt.Errorf("%w: invalid request", ErrAgentUnavailable)
	}
	requestContext, stop := c.context(ctx)
	defer stop()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: construct request", ErrAgentUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+c.credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	if requestHeaderBytes(request.Header) > AgentMaxHeaderBytes {
		return ErrAgentUnavailable
	}
	return c.send(request, out, requestIDOf(value))
}

func requestHeaderBytes(headers http.Header) int {
	total := 0
	for name, values := range headers {
		for _, value := range values {
			total += len(name) + len(value) + 4
		}
	}
	return total
}
func (c *AgentClient) context(ctx context.Context) (context.Context, func()) {
	merged, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.lifetime, cancel)
	return merged, func() { stop(); cancel() }
}
func (c *AgentClient) send(request *http.Request, out interface{}, expectedRequestID string) error {
	response, err := c.http.Do(request)
	if err != nil {
		if request.Context().Err() != nil {
			return request.Context().Err()
		}
		return fmt.Errorf("%w: request failed", ErrAgentUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return ErrAgentUnavailable
	}
	if response.Header.Get("Content-Type") != "application/json" {
		return ErrAgentUnavailable
	}
	if response.ContentLength > AgentMaxResponseBodyBytes {
		return ErrAgentUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, AgentMaxResponseBodyBytes+1))
	if err != nil || len(body) > AgentMaxResponseBodyBytes {
		return ErrAgentUnavailable
	}
	if response.StatusCode == http.StatusServiceUnavailable && request.URL.Path == "/readyz" {
		if err := strictDecodeJSON(body, out); err != nil {
			return ErrAgentUnavailable
		}
		ready, ok := out.(*ReadyResponse)
		if !ok || ready.Status != "not_ready" {
			return ErrAgentUnavailable
		}
		return errAgentNotReady
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeAgentError(body, request.URL.Path, response.StatusCode, expectedRequestID)
	}
	if err := strictDecodeJSON(body, out); err != nil {
		return ErrAgentUnavailable
	}
	if !validAgentResponse(out) {
		return ErrAgentUnavailable
	}
	return nil
}

func decodeAgentError(body []byte, path string, status int, expectedRequestID string) error {
	var response struct {
		ContractVersion string  `json:"contract_version"`
		RequestID       *string `json:"request_id"`
		Error           struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if !validAgentErrorEnvelope(body, &response) || !routeAllowsAgentError(path, status, response.Error.Code) || (response.RequestID != nil && (expectedRequestID == "" || *response.RequestID != expectedRequestID)) {
		return ErrAgentUnavailable
	}
	if response.Error.Code == "invalid_model_output" {
		return ErrAgentInvalidModelOutput
	}
	return &AgentError{Code: response.Error.Code}
}
func validAgentErrorEnvelope(body []byte, response *struct {
	ContractVersion string  `json:"contract_version"`
	RequestID       *string `json:"request_id"`
	Error           struct {
		Code string `json:"code"`
	} `json:"error"`
}) bool {
	return strictDecodeJSON(body, response) == nil && response.ContractVersion == AgentContractVersion && (response.RequestID == nil || validCanonicalAgentID(*response.RequestID)) && stableAgentError(response.Error.Code)
}
func requestIDOf(value interface{}) string {
	switch request := value.(type) {
	case AcquireRequest:
		return request.RequestID
	case LeaseRequest:
		return request.RequestID
	case PlanRequest:
		return request.RequestID
	case AgentDialogueRequest:
		return request.RequestID
	case MemoryReconcileRequest:
		return request.RequestID
	case MemoryCommitRequest:
		return request.RequestID
	case MemoryDeleteRequest:
		return request.RequestID
	case CancelRequest:
		return request.RequestID
	}
	return ""
}
func routeAllowsAgentError(path string, status int, code string) bool {
	if agentErrorStatus(code) != status {
		return false
	}
	switch path {
	case "/readyz":
		return code == "unauthorized" || code == "internal_error"
	case "/v1/namespaces/acquire":
		return code == "invalid_request" || code == "unauthorized" || code == "unsupported_version" || code == "namespace_conflict" || code == "internal_error"
	case "/v1/namespaces/heartbeat", "/v1/namespaces/release", "/v1/runs/cancel":
		return code == "invalid_request" || code == "unauthorized" || code == "unsupported_version" || code == "not_found" || code == "internal_error"
	case "/v1/plan":
		return code == "invalid_request" || code == "unauthorized" || code == "unsupported_version" || code == "overloaded" || code == "deadline_exceeded" || code == "agent_unavailable" || code == "invalid_model_output" || code == "not_found" || code == "internal_error"
	case "/v1/dialogue":
		return code == "invalid_request" || code == "unauthorized" || code == "unsupported_version" || code == "overloaded" || code == "deadline_exceeded" || code == "agent_unavailable" || code == "invalid_model_output" || code == "memory_conflict" || code == "not_found" || code == "internal_error"
	case "/v1/memory/reconcile", "/v1/memory/commit", "/v1/memory/delete":
		return code == "invalid_request" || code == "unauthorized" || code == "unsupported_version" || code == "memory_conflict" || code == "not_found" || code == "internal_error"
	}
	return false
}
func agentErrorStatus(code string) int {
	switch code {
	case "invalid_request":
		return 400
	case "unauthorized":
		return 401
	case "unsupported_version":
		return 426
	case "namespace_conflict", "memory_conflict":
		return 409
	case "overloaded":
		return 429
	case "deadline_exceeded":
		return 504
	case "agent_unavailable":
		return 503
	case "invalid_model_output":
		return 422
	case "not_found":
		return 404
	case "internal_error":
		return 500
	}
	return 0
}
func stableAgentError(code string) bool {
	switch code {
	case "invalid_request", "unauthorized", "unsupported_version", "namespace_conflict", "overloaded", "deadline_exceeded", "agent_unavailable", "invalid_model_output", "memory_conflict", "not_found", "internal_error":
		return true
	}
	return false
}

func strictDecodeJSON(body []byte, out interface{}) error {
	if !utf8.Valid(body) || invalidJSONSurrogate(body) || hasDuplicateJSONKeys(body) {
		return errors.New("invalid strict JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("trailing JSON")
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

var errAgentNotReady = errors.New("companion: agent not ready")

func validAgentRequest(value interface{}) bool {
	validID := func(value string) bool {
		_, err := ParseID(value)
		return err == nil && len(value) == 36 && value == strings.ToLower(value)
	}
	validBase := func(version, requestID, clientID, namespace string) bool {
		return version == AgentContractVersion && validID(requestID) && validID(clientID) && validID(namespace)
	}
	switch request := value.(type) {
	case AcquireRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID)
	case LeaseRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID)
	case PlanRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.RunID) && validID(request.CompanionID) && validID(request.SnapshotID) && request.Generation > 0 && request.DeadlineUnixMS > 0 && validSHA256(request.SnapshotDigest) && validMCPEndpoint(request.MCPEndpoint) && validAgentText(request.MCPCapability, 512, true) && validAgentText(request.Instruction, 1024, true)
	case AgentDialogueRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.RunID) && validID(request.CompanionID) && request.Generation > 0 && request.MemoryEpoch > 0 && request.DeadlineUnixMS > 0 && validAgentText(request.Persona, 4096, false) && validDialogueFact(request.FactNode, request.Terminal) && validDialogueEnvironment(request.Environment)
	case MemoryCommitRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.CompanionID) && validID(request.OperationID) && request.MemoryEpoch > 0 && len(request.Summary) <= 2048
	case MemoryDeleteRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.CompanionID) && validID(request.TombstoneOperationID) && request.OldMemoryEpoch > 0 && request.NewMemoryEpoch > 0
	case CancelRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.RunID)
	case MemoryReconcileRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.CompanionID) && request.MemoryEpoch > 0 && ((request.Active && request.Mirror != nil && request.TombstoneOperationID == nil && validMemoryState(*request.Mirror)) || (!request.Active && request.Mirror == nil && request.TombstoneOperationID != nil && validID(*request.TombstoneOperationID)))
	default:
		return false
	}
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, digit := range value {
		if !(digit >= '0' && digit <= '9' || digit >= 'a' && digit <= 'f') {
			return false
		}
	}
	return true
}
func validMCPEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "/mcp" {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	return ip != nil && ip.IsLoopback()
}

func validAgentText(value string, maximum int, required bool) bool {
	if (required && value == "") || len(value) > maximum || strings.ContainsRune(value, 0) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7F {
			return false
		}
	}
	return utf8.ValidString(value)
}
func validDialogueFact(fact AgentDialogueFact, terminal bool) bool {
	if terminal {
		if fact.Kind != "terminal" {
			return false
		}
		switch fact.State {
		case "completed", "timed_out", "stopped":
			return fact.Reason == "none"
		case "failed":
			switch fact.Reason {
			case "planner_unavailable", "invalid_plan", "path_unreachable", "world_changed", "inventory_full":
				return true
			}
			return false
		}
		return false
	}
	switch fact.Kind {
	case "start", "first_arrival", "idle":
		return fact.StepKind == "" && fact.State == "" && fact.Reason == ""
	case "progress":
		return (fact.StepKind == "go_to" || fact.StepKind == "mine" || fact.StepKind == "place") && fact.State == "" && fact.Reason == ""
	}
	return false
}
func validDialogueEnvironment(environment AgentDialogueEnvironment) bool {
	if len(environment.ExposedBlocks) > 256 || len(environment.Heights) > 1089 {
		return false
	}
	for _, block := range environment.ExposedBlocks {
		if block.Position.Y < -64 || block.Position.Y > 319 || block.BlockID == 0 || block.BlockID == 65535 {
			return false
		}
	}
	for _, height := range environment.Heights {
		if height.Height < -65 || height.Height > 319 {
			return false
		}
	}
	return true
}
func validMemoryState(state AgentMemoryState) bool {
	if state.Revision == 0 {
		return state.OperationID == nil && state.Summary == ""
	}
	return state.OperationID != nil && validCanonicalAgentID(*state.OperationID) && validAgentText(state.Summary, 2048, false)
}
func validMemoryProposal(proposal AgentMemoryProposal) bool {
	return validCanonicalAgentID(proposal.OperationID) && validAgentText(proposal.Summary, 2048, false)
}
func validAgentPlan(plan AgentPlan) bool {
	if !validAgentText(plan.Summary, MaxPlanSummaryBytes, true) || len(plan.Steps) == 0 || len(plan.Steps) > 5000 {
		return false
	}
	for index, step := range plan.Steps {
		switch step.Kind {
		case "go_to", "mine":
			if step.Y < -64 || step.Y > 319 || step.Block != "" || step.PlayerID != "" {
				return false
			}
		case "place":
			if step.Y < -64 || step.Y > 319 || step.Block == "" || step.PlayerID != "" {
				return false
			}
		case "follow":
			if !validCanonicalAgentID(step.PlayerID) || index != len(plan.Steps)-1 || step.Block != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func validAgentResponse(value interface{}) bool {
	validBase := func(version, requestID, clientID, namespace string) bool {
		return version == AgentContractVersion && validCanonicalAgentID(requestID) && validCanonicalAgentID(clientID) && validCanonicalAgentID(namespace)
	}
	switch response := value.(type) {
	case *LiveResponse:
		return response.Status == "live"
	case *ReadyResponse:
		return response.Status == "ready" || response.Status == "not_ready"
	case *AcquireResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && response.LeaseExpiresInMS == 15000
	case *HeartbeatResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && response.LeaseExpiresInMS == 15000
	case *ReleaseResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && response.Released
	case *PlanResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && validCanonicalAgentID(response.RunID) && validCanonicalAgentID(response.CompanionID) && validCanonicalAgentID(response.SnapshotID) && response.Generation > 0 && len(response.SnapshotDigest) == 64 && validAgentPlan(response.Plan)
	case *AgentDialogueResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && validCanonicalAgentID(response.RunID) && validCanonicalAgentID(response.CompanionID) && response.Generation > 0 && response.MemoryEpoch > 0 && validDialogueLine(response.Line) && (response.MemoryProposal == nil || validMemoryProposal(*response.MemoryProposal))
	case *MemoryReconcileResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && validCanonicalAgentID(response.CompanionID) && response.MemoryEpoch > 0 && ((response.Active && response.Memory != nil && response.TombstoneOperationID == nil && validMemoryState(*response.Memory)) || (!response.Active && response.Memory == nil && response.TombstoneOperationID != nil && validCanonicalAgentID(*response.TombstoneOperationID)))
	case *MemoryCommitResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && validCanonicalAgentID(response.CompanionID) && validCanonicalAgentID(response.OperationID) && response.MemoryEpoch > 0 && response.CommittedRevision > 0
	case *MemoryDeleteResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && validCanonicalAgentID(response.CompanionID) && validCanonicalAgentID(response.TombstoneOperationID) && response.MemoryEpoch > 0
	case *CancelResponse:
		return validBase(response.ContractVersion, response.RequestID, response.ClientInstanceID, response.NamespaceID) && validCanonicalAgentID(response.LeaseID) && validCanonicalAgentID(response.RunID) && response.Cancelled
	}
	return false
}
func validCanonicalAgentID(value string) bool {
	_, err := ParseID(value)
	return err == nil && len(value) == 36 && value == strings.ToLower(value)
}
func validDialogueLine(value string) bool {
	return validAgentText(value, 256, true) && strings.TrimSpace(value) == value
}

func invalidJSONSurrogate(body []byte) bool {
	for index := 0; index+5 < len(body); index++ {
		if body[index] != '\\' || body[index+1] != 'u' {
			continue
		}
		value, ok := hex4(body[index+2 : index+6])
		if !ok {
			return true
		}
		if value >= 0xD800 && value <= 0xDBFF {
			if index+11 >= len(body) || body[index+6] != '\\' || body[index+7] != 'u' {
				return true
			}
			low, ok := hex4(body[index+8 : index+12])
			if !ok || low < 0xDC00 || low > 0xDFFF {
				return true
			}
			index += 11
		} else if value >= 0xDC00 && value <= 0xDFFF {
			return true
		}
	}
	return false
}

func hex4(value []byte) (int, bool) {
	result := 0
	for _, digit := range value {
		result <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			result += int(digit - '0')
		case digit >= 'a' && digit <= 'f':
			result += int(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			result += int(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return result, true
}

func hasDuplicateJSONKeys(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	return duplicateJSONValue(decoder)
}

func duplicateJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return true
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			keys := make(map[string]struct{})
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return true
				}
				name, ok := key.(string)
				if !ok {
					return true
				}
				if _, exists := keys[name]; exists {
					return true
				}
				keys[name] = struct{}{}
				if duplicateJSONValue(decoder) {
					return true
				}
			}
			_, err = decoder.Token()
			return err != nil
		case '[':
			for decoder.More() {
				if duplicateJSONValue(decoder) {
					return true
				}
			}
			_, err = decoder.Token()
			return err != nil
		}
	}
	return false
}
