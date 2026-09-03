package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/companion"
)

func TestAgentLeaseControllerContractRequiresBackgroundFence(t *testing.T) {
	if companionAgentHeartbeatEvery != 5*time.Second || companionAgentLeaseTTL != 15*time.Second {
		t.Fatalf("production lease timing heartbeat=%v ttl=%v", companionAgentHeartbeatEvery, companionAgentLeaseTTL)
	}
	client := &agentControlFake{}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client:           client,
		ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:      "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery:   5 * time.Second,
		LeaseTTL:         15 * time.Second,
	})
	if controller == nil {
		t.Fatal("nil controller")
	}
	controller.Close()
}

func TestAgentLeaseControllerHeartbeatsAndReacquiresWithoutBlockingWorld(t *testing.T) {
	client := &leaseLifecycleFake{}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client: client, ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:    "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery: 5 * time.Millisecond, LeaseTTL: 15 * time.Millisecond,
		NewID: sequentialAgentTestID,
	})
	defer controller.Close()
	waitAgentCondition(t, "initial acquire", func() bool {
		_, ok := controller.current()
		return ok && client.acquireCalls.Load() >= 1
	})
	waitAgentCondition(t, "heartbeat", func() bool { return client.heartbeatCalls.Load() >= 1 })
	client.failHeartbeat.Store(true)
	waitAgentCondition(t, "reacquire after heartbeat failure", func() bool {
		return client.acquireCalls.Load() >= 2
	})
}

func TestAgentLeaseControllerFencesLateAcquireAfterClose(t *testing.T) {
	client := &lateAcquireFake{started: make(chan struct{}), release: make(chan struct{})}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client: client, ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:    "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery: time.Hour, LeaseTTL: companionAgentLeaseTTL,
		NewID: sequentialAgentTestID,
	})
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("Acquire 未开始")
	}
	closed := make(chan struct{})
	go func() {
		controller.Close()
		close(closed)
	}()
	close(client.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("controller.Close 未返回")
	}
	if lease, ok := controller.current(); ok || lease.ID != "" {
		t.Fatalf("late Acquire 安装了 lease=%+v", lease)
	}
}

func TestAgentPlannerBridgeRequiresFrozenRegistryIdentity(t *testing.T) {
	bridge := newCompanionAgentPlanner(agentPlannerOptions{
		Client:           &agentControlFake{},
		Lease:            &companionAgentLeaseController{},
		Registry:         companion.NewSnapshotRegistry(),
		MCPEndpoint:      "http://127.0.0.1:12345/mcp",
		ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:      "22222222-2222-4222-8222-222222222222",
	})
	if bridge == nil {
		t.Fatal("nil bridge")
	}
	_, _ = bridge.Plan(context.Background(), companionPlanningRequest{})
}

func TestAgentPlannerBridgeAppliesDeadlineAndCancelsRun(t *testing.T) {
	client := &blockingAgentPlanFake{blockCancel: true}
	lease := &companionAgentLeaseController{
		leaseTTL: companionAgentLeaseTTL,
		lease: companionAgentLease{
			ID:      "33333333-3333-4333-8333-333333333333",
			Fence:   1,
			Expires: time.Now().Add(time.Minute),
		},
	}
	registry := companion.NewSnapshotRegistry()
	defer registry.Close()
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
		Timeout:          200 * time.Millisecond,
		NewID: func() (string, error) {
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	})
	snapshot := testMCPPlanSnapshot(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	_, err := bridge.Plan(ctx, companionPlanningRequest{
		CompanionID: snapshot.Companion.ID, Generation: 1, Snapshot: snapshot,
		Instruction: "向前走",
	})
	if !errors.Is(err, companion.ErrPlannerUnavailable) {
		t.Fatalf("Plan err=%v，want ErrPlannerUnavailable", err)
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("Plan elapsed=%v，bridge timeout 未生效", elapsed)
	}
	if !client.planCanceled.Load() {
		t.Fatal("Plan context 未被 bridge deadline 取消")
	}
	if client.cancelCalls.Load() != 1 {
		t.Fatalf("CancelRun calls=%d，want 1", client.cancelCalls.Load())
	}
	capability, _ := client.capability.Load().(string)
	if _, lookupErr := registry.Lookup(capability); !errors.Is(lookupErr, companion.ErrSnapshotUnavailable) {
		t.Fatalf("Lookup(after Plan failure) err=%v，want ErrSnapshotUnavailable", lookupErr)
	}
}

func TestAgentPlannerBridgeRejectsUntrustedResponseCorrelation(t *testing.T) {
	client := &mismatchedAgentPlanFake{}
	lease := &companionAgentLeaseController{
		leaseTTL: companionAgentLeaseTTL,
		lease: companionAgentLease{
			ID: "33333333-3333-4333-8333-333333333333", Fence: 1,
			Expires: time.Now().Add(time.Minute),
		},
	}
	registry := companion.NewSnapshotRegistry()
	defer registry.Close()
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
	snapshot := testMCPPlanSnapshot(t)
	_, err := bridge.Plan(context.Background(), companionPlanningRequest{
		CompanionID: snapshot.Companion.ID, Generation: 7, Snapshot: snapshot,
		Instruction: "采一块石头",
	})
	if !errors.Is(err, companion.ErrPlannerUnavailable) {
		t.Fatalf("Plan err=%v，want ErrPlannerUnavailable", err)
	}
	if client.cancelCalls.Load() != 1 {
		t.Fatalf("CancelRun calls=%d，want 1", client.cancelCalls.Load())
	}
}

func TestAgentPlannerBridgeRoundTripsRealClientAndCapability(t *testing.T) {
	registry := companion.NewSnapshotRegistry()
	defer registry.Close()
	var delivered atomic.Bool
	var capability atomic.Value
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/plan" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-agent-secret" {
			t.Errorf("Authorization=%q", got)
		}
		var request companion.PlanRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			t.Errorf("decode PlanRequest: %v", err)
			return
		}
		registered, err := registry.Lookup(request.MCPCapability)
		if err != nil {
			t.Errorf("Lookup(capability during Plan): %v", err)
			return
		}
		if registered.SnapshotID != request.SnapshotID || registered.Digest != request.SnapshotDigest ||
			registered.Generation != request.Generation {
			t.Errorf("registered=%+v request=%+v", registered, request)
			return
		}
		capability.Store(request.MCPCapability)
		delivered.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(companion.PlanResponse{
			ContractVersion: request.ContractVersion, RequestID: request.RequestID,
			ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
			LeaseID: request.LeaseID, RunID: request.RunID, CompanionID: request.CompanionID,
			Generation: request.Generation, SnapshotID: request.SnapshotID,
			SnapshotDigest: request.SnapshotDigest,
			Plan: companion.AgentPlan{Summary: "向前走", Steps: []companion.AgentPlanStep{{
				Kind: "go_to", X: 7, Y: 65, Z: 0,
			}}},
		})
	}))
	defer agentServer.Close()
	client, err := companion.NewAgentClient(companion.AgentServiceSettings{
		Endpoint: agentServer.URL, APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
	}, "test-agent-secret", nil)
	if err != nil {
		t.Fatalf("NewAgentClient: %v", err)
	}
	defer client.Close()
	lease := &companionAgentLeaseController{leaseTTL: companionAgentLeaseTTL, lease: companionAgentLease{
		ID: "33333333-3333-4333-8333-333333333333", Fence: 1, Expires: time.Now().Add(time.Minute),
	}}
	ids := []string{
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
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
	snapshot := testMCPPlanSnapshot(t)
	outcome, err := bridge.Plan(context.Background(), companionPlanningRequest{
		CompanionID: snapshot.Companion.ID, Generation: 9, Attempt: 7, Snapshot: snapshot,
		Instruction: "向前走",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !delivered.Load() || outcome.Attempt != 7 || outcome.Generation != 9 ||
		outcome.RunID == "" || outcome.SnapshotID == "" || outcome.SnapshotDigest == "" {
		t.Fatalf("delivered=%v outcome=%+v", delivered.Load(), outcome)
	}
	issued, _ := capability.Load().(string)
	if _, err := registry.Lookup(issued); !errors.Is(err, companion.ErrSnapshotUnavailable) {
		t.Fatalf("Lookup(after complete)=%v，want ErrSnapshotUnavailable", err)
	}
}

type agentControlFake struct{}

func (*agentControlFake) Acquire(context.Context, companion.AcquireRequest) (companion.AcquireResponse, error) {
	return companion.AcquireResponse{}, companion.ErrAgentUnavailable
}

func (*agentControlFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (*agentControlFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*agentControlFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

type blockingAgentPlanFake struct {
	planCanceled atomic.Bool
	cancelCalls  atomic.Int32
	capability   atomic.Value
	blockCancel  bool
}

func (*blockingAgentPlanFake) Acquire(context.Context, companion.AcquireRequest) (companion.AcquireResponse, error) {
	return companion.AcquireResponse{}, companion.ErrAgentUnavailable
}

func (*blockingAgentPlanFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (f *blockingAgentPlanFake) Plan(ctx context.Context, request companion.PlanRequest) (companion.PlanResponse, error) {
	f.capability.Store(request.MCPCapability)
	<-ctx.Done()
	f.planCanceled.Store(true)
	return companion.PlanResponse{}, ctx.Err()
}

func (f *blockingAgentPlanFake) CancelRun(ctx context.Context, _ companion.CancelRequest) (companion.CancelResponse, error) {
	f.cancelCalls.Add(1)
	if f.blockCancel {
		<-ctx.Done()
		return companion.CancelResponse{}, ctx.Err()
	}
	return companion.CancelResponse{}, nil
}

type mismatchedAgentPlanFake struct {
	cancelCalls atomic.Int32
}

var sequentialAgentTestCounter atomic.Uint64

func sequentialAgentTestID() (string, error) {
	value := sequentialAgentTestCounter.Add(1)
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", value), nil
}

func waitAgentCondition(t *testing.T, name string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", name)
}

type leaseLifecycleFake struct {
	acquireCalls   atomic.Int32
	heartbeatCalls atomic.Int32
	failHeartbeat  atomic.Bool
}

func (f *leaseLifecycleFake) Acquire(_ context.Context, request companion.AcquireRequest) (companion.AcquireResponse, error) {
	call := f.acquireCalls.Add(1)
	return companion.AcquireResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012d", call), LeaseExpiresInMS: 15,
	}, nil
}

func (f *leaseLifecycleFake) Heartbeat(_ context.Context, request companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	f.heartbeatCalls.Add(1)
	if f.failHeartbeat.Load() {
		return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
	}
	return companion.HeartbeatResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, LeaseExpiresInMS: 15,
	}, nil
}

func (*leaseLifecycleFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*leaseLifecycleFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

type lateAcquireFake struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (f *lateAcquireFake) Acquire(_ context.Context, request companion.AcquireRequest) (companion.AcquireResponse, error) {
	f.once.Do(func() { close(f.started) })
	<-f.release
	return companion.AcquireResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", LeaseExpiresInMS: 15000,
	}, nil
}

func (*lateAcquireFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (*lateAcquireFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*lateAcquireFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

func (*mismatchedAgentPlanFake) Acquire(context.Context, companion.AcquireRequest) (companion.AcquireResponse, error) {
	return companion.AcquireResponse{}, companion.ErrAgentUnavailable
}

func (*mismatchedAgentPlanFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (*mismatchedAgentPlanFake) Plan(_ context.Context, request companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, RunID: request.RunID, CompanionID: request.CompanionID,
		Generation: request.Generation, SnapshotID: request.SnapshotID,
		SnapshotDigest: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		Plan:           companion.AgentPlan{Summary: "采石头", Steps: []companion.AgentPlanStep{{Kind: "mine", X: 8, Y: 63, Z: -2}}},
	}, nil
}

func (f *mismatchedAgentPlanFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	f.cancelCalls.Add(1)
	return companion.CancelResponse{}, nil
}
