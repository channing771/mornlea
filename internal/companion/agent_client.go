package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
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
	ContractVersion  string          `json:"contract_version"`
	RequestID        string          `json:"request_id"`
	ClientInstanceID string          `json:"client_instance_id"`
	NamespaceID      string          `json:"namespace_id"`
	LeaseID          string          `json:"lease_id"`
	RunID            string          `json:"run_id"`
	CompanionID      string          `json:"companion_id"`
	Generation       uint64          `json:"generation"`
	SnapshotID       string          `json:"snapshot_id"`
	SnapshotDigest   string          `json:"snapshot_digest"`
	Plan             json.RawMessage `json:"plan"`
}
type AgentDialogueRequest struct {
	ContractVersion  string          `json:"contract_version"`
	RequestID        string          `json:"request_id"`
	ClientInstanceID string          `json:"client_instance_id"`
	NamespaceID      string          `json:"namespace_id"`
	LeaseID          string          `json:"lease_id"`
	RunID            string          `json:"run_id"`
	CompanionID      string          `json:"companion_id"`
	Generation       uint64          `json:"generation"`
	MemoryEpoch      uint64          `json:"memory_epoch"`
	DeadlineUnixMS   int64           `json:"deadline_unix_ms"`
	Persona          string          `json:"persona"`
	Fact             json.RawMessage `json:"fact"`
	Environment      json.RawMessage `json:"environment"`
	Terminal         bool            `json:"terminal"`
}
type AgentDialogueResponse struct {
	ContractVersion  string          `json:"contract_version"`
	RequestID        string          `json:"request_id"`
	ClientInstanceID string          `json:"client_instance_id"`
	NamespaceID      string          `json:"namespace_id"`
	LeaseID          string          `json:"lease_id"`
	RunID            string          `json:"run_id"`
	CompanionID      string          `json:"companion_id"`
	Generation       uint64          `json:"generation"`
	MemoryEpoch      uint64          `json:"memory_epoch"`
	Line             string          `json:"line"`
	MemoryProposal   json.RawMessage `json:"memory_proposal,omitempty"`
	Terminal         bool            `json:"terminal"`
}
type MemoryReconcileRequest struct {
	ContractVersion      string          `json:"contract_version"`
	RequestID            string          `json:"request_id"`
	ClientInstanceID     string          `json:"client_instance_id"`
	NamespaceID          string          `json:"namespace_id"`
	LeaseID              string          `json:"lease_id"`
	CompanionID          string          `json:"companion_id"`
	MemoryEpoch          uint64          `json:"memory_epoch"`
	Active               bool            `json:"active"`
	Mirror               json.RawMessage `json:"mirror,omitempty"`
	TombstoneOperationID string          `json:"tombstone_operation_id,omitempty"`
}
type MemoryReconcileResponse struct {
	ContractVersion      string          `json:"contract_version"`
	RequestID            string          `json:"request_id"`
	ClientInstanceID     string          `json:"client_instance_id"`
	NamespaceID          string          `json:"namespace_id"`
	LeaseID              string          `json:"lease_id"`
	CompanionID          string          `json:"companion_id"`
	MemoryEpoch          uint64          `json:"memory_epoch"`
	Active               bool            `json:"active"`
	Memory               json.RawMessage `json:"memory,omitempty"`
	TombstoneOperationID string          `json:"tombstone_operation_id,omitempty"`
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
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DisableCompression = true
		transport.MaxResponseHeaderBytes = AgentMaxHeaderBytes
		client = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
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
	return out, err
}
func (c *AgentClient) Release(ctx context.Context, in LeaseRequest) (ReleaseResponse, error) {
	var out ReleaseResponse
	err := c.post(ctx, "/v1/namespaces/release", in, &out)
	return out, err
}
func (c *AgentClient) Plan(ctx context.Context, in PlanRequest) (PlanResponse, error) {
	var out PlanResponse
	err := c.post(ctx, "/v1/plan", in, &out)
	return out, err
}
func (c *AgentClient) Dialogue(ctx context.Context, in AgentDialogueRequest) (AgentDialogueResponse, error) {
	var out AgentDialogueResponse
	err := c.post(ctx, "/v1/dialogue", in, &out)
	return out, err
}
func (c *AgentClient) ReconcileMemory(ctx context.Context, in MemoryReconcileRequest) (MemoryReconcileResponse, error) {
	var out MemoryReconcileResponse
	err := c.post(ctx, "/v1/memory/reconcile", in, &out)
	return out, err
}
func (c *AgentClient) CommitMemory(ctx context.Context, in MemoryCommitRequest) (MemoryCommitResponse, error) {
	var out MemoryCommitResponse
	err := c.post(ctx, "/v1/memory/commit", in, &out)
	return out, err
}
func (c *AgentClient) DeleteMemory(ctx context.Context, in MemoryDeleteRequest) (MemoryDeleteResponse, error) {
	var out MemoryDeleteResponse
	err := c.post(ctx, "/v1/memory/delete", in, &out)
	return out, err
}
func (c *AgentClient) CancelRun(ctx context.Context, in CancelRequest) (CancelResponse, error) {
	var out CancelResponse
	err := c.post(ctx, "/v1/runs/cancel", in, &out)
	return out, err
}

func (c *AgentClient) get(ctx context.Context, path string, authenticated bool, out interface{}) error {
	request, err := http.NewRequestWithContext(c.context(ctx), http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return fmt.Errorf("%w: construct request", ErrAgentUnavailable)
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+c.credential)
	}
	request.Header.Set("Accept-Encoding", "identity")
	return c.send(request, out)
}
func (c *AgentClient) post(ctx context.Context, path string, value interface{}, out interface{}) error {
	body, err := json.Marshal(value)
	if err != nil || len(body) > AgentMaxRequestBodyBytes {
		return fmt.Errorf("%w: invalid request", ErrAgentUnavailable)
	}
	request, err := http.NewRequestWithContext(c.context(ctx), http.MethodPost, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: construct request", ErrAgentUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+c.credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	return c.send(request, out)
}
func (c *AgentClient) context(ctx context.Context) context.Context {
	merged, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-c.lifetime.Done():
			cancel()
		case <-merged.Done():
		}
	}()
	return merged
}
func (c *AgentClient) send(request *http.Request, out interface{}) error {
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeAgentError(body)
	}
	if err := strictDecodeJSON(body, out); err != nil {
		return ErrAgentUnavailable
	}
	return nil
}

func decodeAgentError(body []byte) error {
	var response struct {
		ContractVersion string  `json:"contract_version"`
		RequestID       *string `json:"request_id"`
		Error           struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if strictDecodeJSON(body, &response) != nil || response.ContractVersion != AgentContractVersion || !stableAgentError(response.Error.Code) {
		return ErrAgentUnavailable
	}
	if response.Error.Code == "invalid_model_output" {
		return ErrAgentInvalidModelOutput
	}
	return &AgentError{Code: response.Error.Code}
}
func stableAgentError(code string) bool {
	switch code {
	case "invalid_request", "unauthorized", "unsupported_version", "namespace_conflict", "overloaded", "deadline_exceeded", "agent_unavailable", "invalid_model_output", "memory_conflict", "not_found", "internal_error":
		return true
	}
	return false
}

func strictDecodeJSON(body []byte, out interface{}) error {
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
func agentURL(endpoint string) (*url.URL, error) { return url.Parse(endpoint) }
