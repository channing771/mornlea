package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/storage"
)

const (
	companionAgentLeaseTTL           = 15 * time.Second
	companionAgentHeartbeatEvery     = 5 * time.Second
	companionAgentPlanTimeout        = 30 * time.Second
	companionAgentPlanTimeoutMax     = 60 * time.Second
	companionAgentDialogueTimeout    = 30 * time.Second
	companionAgentDialogueTimeoutMax = 60 * time.Second
	companionAgentCancelTimeout      = 100 * time.Millisecond
	companionAgentReleaseTimeout     = 5 * time.Second
)

type companionAgentControlClient interface {
	Acquire(context.Context, companion.AcquireRequest) (companion.AcquireResponse, error)
	Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error)
	Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error)
	CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error)
}

type companionAgentRuntimeClient interface {
	companionAgentControlClient
	companionAgentDialogueClient
	Release(context.Context, companion.LeaseRequest) (companion.ReleaseResponse, error)
	ReconcileMemory(context.Context, companion.MemoryReconcileRequest) (companion.MemoryReconcileResponse, error)
	DeleteMemory(context.Context, companion.MemoryDeleteRequest) (companion.MemoryDeleteResponse, error)
	Close()
}

type companionAgentDialogueClient interface {
	Dialogue(context.Context, companion.AgentDialogueRequest) (companion.AgentDialogueResponse, error)
	CommitMemory(context.Context, companion.MemoryCommitRequest) (companion.MemoryCommitResponse, error)
	ReconcileMemory(context.Context, companion.MemoryReconcileRequest) (companion.MemoryReconcileResponse, error)
	DeleteMemory(context.Context, companion.MemoryDeleteRequest) (companion.MemoryDeleteResponse, error)
}

type companionAgentClientFactory func(
	companion.AgentServiceSettings,
	string,
) (companionAgentRuntimeClient, error)

type agentLeaseControllerOptions struct {
	Client           companionAgentControlClient
	ClientInstanceID string
	NamespaceID      string
	HeartbeatEvery   time.Duration
	LeaseTTL         time.Duration
	NewID            func() (string, error)
}

type companionAgentLease struct {
	ID      string
	Fence   uint64
	Expires time.Time
}

// companionAgentLeaseController 在独立控制 goroutine 中维护 namespace lease。
// 世界 tick 从不等待 acquire 或 heartbeat；失败只会令 Agent 能力暂时不可用。
type companionAgentLeaseController struct {
	client           companionAgentControlClient
	clientInstanceID string
	namespaceID      string
	heartbeatEvery   time.Duration
	leaseTTL         time.Duration
	newID            func() (string, error)

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	closed bool
	fence  uint64
	lease  companionAgentLease
}

func newCompanionAgentLeaseController(options agentLeaseControllerOptions) *companionAgentLeaseController {
	heartbeatEvery := options.HeartbeatEvery
	if heartbeatEvery <= 0 {
		heartbeatEvery = companionAgentHeartbeatEvery
	}
	leaseTTL := options.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = companionAgentLeaseTTL
	}
	newID := options.NewID
	if newID == nil {
		newID = newAgentRequestID
	}
	ctx, cancel := context.WithCancel(context.Background())
	controller := &companionAgentLeaseController{
		client: options.Client, clientInstanceID: options.ClientInstanceID,
		namespaceID: options.NamespaceID, heartbeatEvery: heartbeatEvery,
		leaseTTL: leaseTTL, newID: newID, ctx: ctx, cancel: cancel, done: make(chan struct{}),
	}
	go controller.run()
	return controller
}

func (c *companionAgentLeaseController) run() {
	defer close(c.done)
	c.refresh()
	ticker := time.NewTicker(c.heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.refresh()
		}
	}
}

func (c *companionAgentLeaseController) refresh() {
	if c.client == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.fence++
	fence := c.fence
	lease := c.lease
	c.mu.Unlock()

	requestID, err := c.newID()
	if err != nil {
		c.clear(fence)
		return
	}
	rpcContext, rpcDeadline, cancelRPC, ok := c.controlRPCContext(lease)
	if !ok {
		c.clear(fence)
		return
	}
	defer cancelRPC()
	if lease.ID == "" {
		response, acquireErr := c.client.Acquire(rpcContext, companion.AcquireRequest{
			ContractVersion: companion.AgentContractVersion, RequestID: requestID,
			ClientInstanceID: c.clientInstanceID, NamespaceID: c.namespaceID,
		})
		if acquireErr != nil || rpcContext.Err() != nil || !time.Now().Before(rpcDeadline) {
			c.clear(fence)
			return
		}
		c.install(fence, response.LeaseID, response.LeaseExpiresInMS, fence)
		return
	}
	response, heartbeatErr := c.client.Heartbeat(rpcContext, companion.LeaseRequest{
		ContractVersion: companion.AgentContractVersion, RequestID: requestID,
		ClientInstanceID: c.clientInstanceID, NamespaceID: c.namespaceID, LeaseID: lease.ID,
	})
	if heartbeatErr != nil || rpcContext.Err() != nil || !time.Now().Before(rpcDeadline) {
		c.clear(fence)
		return
	}
	c.install(fence, response.LeaseID, response.LeaseExpiresInMS, lease.Fence)
}

// controlRPCContext 为单次 acquire/heartbeat 建立独立硬 deadline。上界是
// heartbeat interval；已有 lease 时还必须早于当前 lease 失效时刻，防止迟到
// heartbeat 把已经失效的租约重新延长。
func (c *companionAgentLeaseController) controlRPCContext(
	lease companionAgentLease,
) (context.Context, time.Time, context.CancelFunc, bool) {
	now := time.Now()
	deadline := now.Add(c.heartbeatEvery)
	if lease.ID != "" && lease.Expires.Before(deadline) {
		deadline = lease.Expires
	}
	if !deadline.After(now) {
		return nil, time.Time{}, func() {}, false
	}
	ctx, cancel := context.WithDeadline(c.ctx, deadline)
	return ctx, deadline, cancel, true
}

func (c *companionAgentLeaseController) install(fence uint64, leaseID string, expiresInMS int, leaseFence uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || fence != c.fence || expiresInMS != int(c.leaseTTL/time.Millisecond) {
		return
	}
	c.lease = companionAgentLease{ID: leaseID, Fence: leaseFence, Expires: time.Now().Add(c.leaseTTL)}
}

func (c *companionAgentLeaseController) clear(fence uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || fence != c.fence {
		return
	}
	c.lease = companionAgentLease{}
}

func (c *companionAgentLeaseController) current() (companionAgentLease, bool) {
	if c == nil {
		return companionAgentLease{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.lease.ID == "" || !time.Now().Before(c.lease.Expires) {
		return companionAgentLease{}, false
	}
	return c.lease, true
}

func (c *companionAgentLeaseController) stillCurrent(lease companionAgentLease) bool {
	current, ok := c.current()
	return ok && current.ID == lease.ID && current.Fence == lease.Fence
}

func (c *companionAgentLeaseController) Close() {
	_, _ = c.Freeze()
	c.mu.Lock()
	c.lease = companionAgentLease{}
	c.mu.Unlock()
}

// Freeze 停止 acquire/heartbeat 并保留冻结时仍有效的 lease，供持久化成功后的
// namespace Release 使用。closed/fence 围栏保证迟到控制结果不能复活状态。
func (c *companionAgentLeaseController) Freeze() (companionAgentLease, bool) {
	if c == nil {
		return companionAgentLease{}, false
	}
	c.mu.Lock()
	if c.closed {
		lease := c.lease
		c.mu.Unlock()
		return lease, lease.ID != "" && time.Now().Before(lease.Expires)
	}
	c.closed = true
	c.fence++
	lease := c.lease
	c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.done != nil {
		<-c.done
	}
	return lease, lease.ID != "" && time.Now().Before(lease.Expires)
}

type agentPlannerOptions struct {
	Client           companionAgentControlClient
	Lease            *companionAgentLeaseController
	Registry         *companion.SnapshotRegistry
	MCPEndpoint      string
	ClientInstanceID string
	NamespaceID      string
	Timeout          time.Duration
	NewID            func() (string, error)
}

type companionPlanningRequest struct {
	CompanionID companion.ID
	Generation  uint64
	Attempt     uint64
	Snapshot    companion.PlanSnapshot
	Instruction string
}

type companionPlanningOutcome struct {
	Plan            companion.Plan
	RunID           string
	SnapshotID      string
	SnapshotDigest  string
	Generation      uint64
	Attempt         uint64
	requestIdentity companionPlanningIdentity
}

type companionPlanningIdentity struct {
	RunID          string
	SnapshotID     string
	SnapshotDigest string
}

type companionAgentPlanner struct {
	client           companionAgentControlClient
	lease            *companionAgentLeaseController
	registry         *companion.SnapshotRegistry
	mcpEndpoint      string
	clientInstanceID string
	namespaceID      string
	timeout          time.Duration
	newID            func() (string, error)
}

type agentDialogueOptions struct {
	Client           companionAgentDialogueClient
	Lease            *companionAgentLeaseController
	ClientInstanceID string
	NamespaceID      string
	Timeout          time.Duration
	NewID            func() (string, error)
}

type companionDialogueRequest struct {
	CompanionID companion.ID
	Generation  uint64
	MemoryEpoch uint64
	Persona     string
	Fact        companion.AgentDialogueFact
	Environment companion.AgentDialogueEnvironment
	Terminal    bool
}

type companionDialogueResult struct {
	RunID       string
	Generation  uint64
	MemoryEpoch uint64
	Line        string
	Proposal    *companion.AgentMemoryProposal
}

type companionMemoryCommitRequest struct {
	CompanionID  companion.ID
	MemoryEpoch  uint64
	BaseRevision uint64
	OperationID  string
	Summary      string
}

type companionMemoryCommitResult struct {
	MemoryEpoch       uint64
	OperationID       string
	CommittedRevision uint64
}

type companionMemoryReconcileResult struct {
	Lifecycle storage.StoredCompanionLifecycle
}

type companionMemoryDeleteRequest struct {
	CompanionID          companion.ID
	OldMemoryEpoch       uint64
	NewMemoryEpoch       uint64
	TombstoneOperationID string
}

type companionMemoryDeleteResult struct {
	MemoryEpoch          uint64
	TombstoneOperationID string
}

// companionAgentDialogue 只把 Go 权威事实映射到 Agent HTTP v1。最近摘要由
// Python MemoryState 自行读取，Go 恢复镜像不属于这条请求的输入。
type companionAgentDialogue struct {
	client           companionAgentDialogueClient
	lease            *companionAgentLeaseController
	clientInstanceID string
	namespaceID      string
	timeout          time.Duration
	newID            func() (string, error)
}

func newCompanionAgentDialogue(options agentDialogueOptions) *companionAgentDialogue {
	timeout := options.Timeout
	if timeout <= 0 || timeout > companionAgentDialogueTimeoutMax {
		timeout = companionAgentDialogueTimeout
	}
	newID := options.NewID
	if newID == nil {
		newID = newAgentRequestID
	}
	return &companionAgentDialogue{
		client: options.Client, lease: options.Lease,
		clientInstanceID: options.ClientInstanceID, namespaceID: options.NamespaceID,
		timeout: timeout, newID: newID,
	}
}

func (d *companionAgentDialogue) CommitMemory(
	ctx context.Context,
	request companionMemoryCommitRequest,
) (companionMemoryCommitResult, error) {
	if d == nil || d.client == nil || d.lease == nil || request.BaseRevision == ^uint64(0) {
		return companionMemoryCommitResult{}, companion.ErrAgentUnavailable
	}
	lease, ok := d.lease.current()
	if !ok {
		return companionMemoryCommitResult{}, companion.ErrAgentUnavailable
	}
	requestID, err := d.newID()
	if err != nil {
		return companionMemoryCommitResult{}, companion.ErrAgentUnavailable
	}
	deadline := time.Now().Add(d.timeout)
	if callerDeadline, hasDeadline := ctx.Deadline(); hasDeadline && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	rpcContext, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	response, err := d.client.CommitMemory(rpcContext, companion.MemoryCommitRequest{
		ContractVersion: companion.AgentContractVersion,
		RequestID:       requestID, ClientInstanceID: d.clientInstanceID,
		NamespaceID: d.namespaceID, LeaseID: lease.ID,
		CompanionID: request.CompanionID.String(), MemoryEpoch: request.MemoryEpoch,
		BaseRevision: request.BaseRevision, OperationID: request.OperationID,
		Summary: request.Summary,
	})
	if err != nil || !d.lease.stillCurrent(lease) ||
		response.CommittedRevision != request.BaseRevision+1 {
		return companionMemoryCommitResult{}, companion.ErrAgentUnavailable
	}
	return companionMemoryCommitResult{
		MemoryEpoch: response.MemoryEpoch, OperationID: response.OperationID,
		CommittedRevision: response.CommittedRevision,
	}, nil
}

func (d *companionAgentDialogue) currentMemoryFence() (uint64, bool) {
	if d == nil || d.lease == nil {
		return 0, false
	}
	lease, ok := d.lease.current()
	return lease.Fence, ok
}

func (d *companionAgentDialogue) ReconcileMemory(
	ctx context.Context,
	lifecycle storage.StoredCompanionLifecycle,
) (companionMemoryReconcileResult, error) {
	if d == nil || d.client == nil || d.lease == nil {
		return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
	}
	lease, ok := d.lease.current()
	if !ok {
		return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
	}
	requestID, err := d.newID()
	if err != nil {
		return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
	}
	request := companion.MemoryReconcileRequest{
		ContractVersion: companion.AgentContractVersion,
		RequestID:       requestID, ClientInstanceID: d.clientInstanceID,
		NamespaceID: d.namespaceID, LeaseID: lease.ID,
		CompanionID: lifecycle.ID.String(), MemoryEpoch: lifecycle.MemoryEpoch,
		Active: lifecycle.Active,
	}
	if lifecycle.Active {
		request.Mirror = &companion.AgentMemoryState{
			Revision: lifecycle.MemoryRevision, Summary: lifecycle.Summary,
		}
		if lifecycle.MemoryRevision != 0 {
			operation := companion.ID(lifecycle.MemoryOperationID).String()
			request.Mirror.OperationID = &operation
		}
	} else {
		tombstone := companion.ID(lifecycle.TombstoneOperationID).String()
		request.TombstoneOperationID = &tombstone
	}
	deadline := time.Now().Add(d.timeout)
	if callerDeadline, hasDeadline := ctx.Deadline(); hasDeadline && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	rpcContext, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	response, err := d.client.ReconcileMemory(rpcContext, request)
	if err != nil || !d.lease.stillCurrent(lease) {
		return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
	}
	result := storage.StoredCompanionLifecycle{
		ID: lifecycle.ID, Active: response.Active, MemoryEpoch: response.MemoryEpoch,
	}
	if response.Active {
		if response.Memory == nil {
			return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
		}
		result.MemoryRevision = response.Memory.Revision
		result.Summary = response.Memory.Summary
		if response.Memory.OperationID != nil {
			operation, parseErr := companion.ParseID(*response.Memory.OperationID)
			if parseErr != nil {
				return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
			}
			result.MemoryOperationID = storage.CompanionIdentity(operation)
		}
	} else {
		if response.TombstoneOperationID == nil {
			return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
		}
		tombstone, parseErr := companion.ParseID(*response.TombstoneOperationID)
		if parseErr != nil {
			return companionMemoryReconcileResult{}, companion.ErrAgentUnavailable
		}
		result.TombstoneOperationID = storage.CompanionIdentity(tombstone)
	}
	return companionMemoryReconcileResult{Lifecycle: result}, nil
}

func (d *companionAgentDialogue) DeleteMemory(
	ctx context.Context,
	request companionMemoryDeleteRequest,
) (companionMemoryDeleteResult, error) {
	if d == nil || d.client == nil || d.lease == nil ||
		request.OldMemoryEpoch == ^uint64(0) || request.NewMemoryEpoch != request.OldMemoryEpoch+1 {
		return companionMemoryDeleteResult{}, companion.ErrAgentUnavailable
	}
	lease, ok := d.lease.current()
	if !ok {
		return companionMemoryDeleteResult{}, companion.ErrAgentUnavailable
	}
	requestID, err := d.newID()
	if err != nil {
		return companionMemoryDeleteResult{}, companion.ErrAgentUnavailable
	}
	deadline := time.Now().Add(d.timeout)
	if callerDeadline, hasDeadline := ctx.Deadline(); hasDeadline && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	rpcContext, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	response, err := d.client.DeleteMemory(rpcContext, companion.MemoryDeleteRequest{
		ContractVersion: companion.AgentContractVersion,
		RequestID:       requestID, ClientInstanceID: d.clientInstanceID,
		NamespaceID: d.namespaceID, LeaseID: lease.ID,
		CompanionID: request.CompanionID.String(), OldMemoryEpoch: request.OldMemoryEpoch,
		NewMemoryEpoch:       request.NewMemoryEpoch,
		TombstoneOperationID: request.TombstoneOperationID,
	})
	if err != nil || !d.lease.stillCurrent(lease) {
		return companionMemoryDeleteResult{}, companion.ErrAgentUnavailable
	}
	return companionMemoryDeleteResult{
		MemoryEpoch:          response.MemoryEpoch,
		TombstoneOperationID: response.TombstoneOperationID,
	}, nil
}

func (d *companionAgentDialogue) Dialogue(
	ctx context.Context,
	request companionDialogueRequest,
) (companionDialogueResult, error) {
	lease, ok := d.lease.current()
	if !ok || d.client == nil {
		return companionDialogueResult{}, companion.ErrAgentUnavailable
	}
	deadline := time.Now().Add(d.timeout)
	if callerDeadline, hasDeadline := ctx.Deadline(); hasDeadline && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	requestID, err := d.newID()
	if err != nil {
		return companionDialogueResult{}, companion.ErrAgentUnavailable
	}
	runID, err := d.newID()
	if err != nil {
		return companionDialogueResult{}, companion.ErrAgentUnavailable
	}
	runContext, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	response, err := d.client.Dialogue(runContext, companion.AgentDialogueRequest{
		ContractVersion: companion.AgentContractVersion, RequestID: requestID,
		ClientInstanceID: d.clientInstanceID, NamespaceID: d.namespaceID,
		LeaseID: lease.ID, RunID: runID, CompanionID: request.CompanionID.String(),
		Generation: request.Generation, MemoryEpoch: request.MemoryEpoch,
		DeadlineUnixMS: deadline.UnixMilli(), Persona: request.Persona,
		FactNode: request.Fact, Environment: request.Environment, Terminal: request.Terminal,
	})
	if err != nil || !d.lease.stillCurrent(lease) {
		return companionDialogueResult{}, companion.ErrAgentUnavailable
	}
	return companionDialogueResult{
		RunID: response.RunID, Generation: response.Generation,
		MemoryEpoch: response.MemoryEpoch, Line: response.Line,
		Proposal: response.MemoryProposal,
	}, nil
}

func newCompanionAgentPlanner(options agentPlannerOptions) *companionAgentPlanner {
	timeout := options.Timeout
	if timeout <= 0 || timeout > companionAgentPlanTimeoutMax {
		timeout = companionAgentPlanTimeout
	}
	newID := options.NewID
	if newID == nil {
		newID = newAgentRequestID
	}
	return &companionAgentPlanner{
		client: options.Client, lease: options.Lease, registry: options.Registry,
		mcpEndpoint: options.MCPEndpoint, clientInstanceID: options.ClientInstanceID,
		namespaceID: options.NamespaceID, timeout: timeout, newID: newID,
	}
}

func (p *companionAgentPlanner) Plan(ctx context.Context, request companionPlanningRequest) (companionPlanningOutcome, error) {
	lease, ok := p.lease.current()
	if !ok || p.client == nil || p.registry == nil {
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	deadline := time.Now().Add(p.timeout)
	if callerDeadline, hasDeadline := ctx.Deadline(); hasDeadline && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	registration, err := p.registry.Register(p.namespaceID, request.CompanionID, request.Generation, request.Snapshot, deadline)
	if err != nil {
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	planContext, cancelPlan := context.WithDeadline(ctx, deadline)
	defer cancelPlan()
	completed := false
	runID := ""
	leaseForRun := lease
	defer func() {
		if !completed {
			p.registry.Cancel(registration.SnapshotID)
			p.cancelRun(leaseForRun, runID)
		}
	}()
	requestID, err := p.newID()
	if err != nil {
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	runID, err = p.newID()
	if err != nil {
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	planRequest := companion.PlanRequest{
		ContractVersion: companion.AgentContractVersion, RequestID: requestID,
		ClientInstanceID: p.clientInstanceID, NamespaceID: p.namespaceID, LeaseID: lease.ID,
		RunID: runID, CompanionID: request.CompanionID.String(), Generation: request.Generation,
		SnapshotID: registration.SnapshotID, SnapshotDigest: registration.Digest,
		DeadlineUnixMS: deadline.UnixMilli(), MCPEndpoint: p.mcpEndpoint,
		MCPCapability: registration.Capability, Instruction: request.Instruction,
	}
	response, err := p.client.Plan(planContext, planRequest)
	if err != nil {
		if errors.Is(err, companion.ErrAgentInvalidModelOutput) {
			return companionPlanningOutcome{}, companion.ErrPlannerInvalidPlan
		}
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	if !p.lease.stillCurrent(lease) {
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	if response.ContractVersion != planRequest.ContractVersion ||
		response.RequestID != planRequest.RequestID ||
		response.ClientInstanceID != planRequest.ClientInstanceID ||
		response.NamespaceID != planRequest.NamespaceID ||
		response.LeaseID != planRequest.LeaseID || response.RunID != planRequest.RunID ||
		response.CompanionID != planRequest.CompanionID || response.Generation != planRequest.Generation ||
		response.SnapshotID != planRequest.SnapshotID || response.SnapshotDigest != planRequest.SnapshotDigest {
		return companionPlanningOutcome{}, companion.ErrPlannerUnavailable
	}
	plan, err := companion.DecodeAgentPlan(response.Plan, request.Snapshot)
	if err != nil {
		return companionPlanningOutcome{}, companion.ErrPlannerInvalidPlan
	}
	p.registry.Complete(registration.SnapshotID)
	completed = true
	return companionPlanningOutcome{
		Plan: plan, RunID: response.RunID, SnapshotID: response.SnapshotID,
		SnapshotDigest: response.SnapshotDigest, Generation: response.Generation,
		Attempt: request.Attempt,
		requestIdentity: companionPlanningIdentity{
			RunID: planRequest.RunID, SnapshotID: planRequest.SnapshotID,
			SnapshotDigest: planRequest.SnapshotDigest,
		},
	}, nil
}

func (p *companionAgentPlanner) cancelRun(lease companionAgentLease, runID string) {
	if runID == "" || p.client == nil {
		return
	}
	requestID, err := p.newID()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), companionAgentCancelTimeout)
	defer cancel()
	_, _ = p.client.CancelRun(ctx, companion.CancelRequest{
		ContractVersion: companion.AgentContractVersion, RequestID: requestID,
		ClientInstanceID: p.clientInstanceID, NamespaceID: p.namespaceID,
		LeaseID: lease.ID, RunID: runID,
	})
}

func newAgentRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("生成 Agent 身份: %w", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	hexID := hex.EncodeToString(value[:])
	return hexID[:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:], nil
}
