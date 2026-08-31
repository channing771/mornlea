package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	return c.send(request, out)
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
	return c.send(request, out)
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
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.RunID) && validID(request.CompanionID) && validID(request.SnapshotID) && request.Generation > 0 && request.DeadlineUnixMS > 0 && len(request.Instruction) <= 1024
	case AgentDialogueRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.RunID) && validID(request.CompanionID) && request.Generation > 0 && request.MemoryEpoch > 0 && request.DeadlineUnixMS > 0 && len(request.Persona) <= 4096
	case MemoryCommitRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.CompanionID) && validID(request.OperationID) && request.MemoryEpoch > 0 && len(request.Summary) <= 2048
	case MemoryDeleteRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.CompanionID) && validID(request.TombstoneOperationID) && request.OldMemoryEpoch > 0 && request.NewMemoryEpoch > 0
	case CancelRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.RunID)
	case MemoryReconcileRequest:
		return validBase(request.ContractVersion, request.RequestID, request.ClientInstanceID, request.NamespaceID) && validID(request.LeaseID) && validID(request.CompanionID) && request.MemoryEpoch > 0
	default:
		return false
	}
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
