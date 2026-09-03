package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
)

func TestAgentLeaseControlDeadlineRetriesHungAcquire(t *testing.T) {
	client := &repairLeaseDeadlineFake{hangAcquireOnce: true}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client: client, ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:    "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery: 20 * time.Millisecond, LeaseTTL: 60 * time.Millisecond,
		NewID: sequentialAgentTestID,
	})
	defer controller.Close()
	waitRepairLeaseCondition(t, "reacquire after acquire deadline", func() bool {
		_, ok := controller.current()
		return client.acquireCalls.Load() >= 2 && ok
	})
}

func TestAgentLeaseControlDeadlineClearsHungHeartbeatAndReacquires(t *testing.T) {
	client := &repairLeaseDeadlineFake{hangHeartbeatOnce: true}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client: client, ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:    "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery: 20 * time.Millisecond, LeaseTTL: 60 * time.Millisecond,
		NewID: sequentialAgentTestID,
	})
	defer controller.Close()
	waitRepairLeaseCondition(t, "heartbeat timeout then reacquire", func() bool {
		return client.heartbeatCalls.Load() >= 1 && client.acquireCalls.Load() >= 2
	})
}

func TestAgentLeaseControlCloseCancelsBoundedRPC(t *testing.T) {
	client := &repairLeaseCloseFake{started: make(chan struct{})}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client: client, ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:    "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery: time.Second, LeaseTTL: 2 * time.Second,
		NewID: sequentialAgentTestID,
	})
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("Acquire 未开始")
	}
	done := make(chan struct{})
	go func() {
		controller.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Close 未取消并等待 control RPC")
	}
}

func TestAgentLeaseControlDropsLateResultFromClientIgnoringDeadline(t *testing.T) {
	client := &repairLateControlFake{
		started: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{}),
	}
	controller := newCompanionAgentLeaseController(agentLeaseControllerOptions{
		Client: client, ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:    "22222222-2222-4222-8222-222222222222",
		HeartbeatEvery: 20 * time.Millisecond, LeaseTTL: 60 * time.Millisecond,
		NewID: sequentialAgentTestID,
	})
	defer controller.Close()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("Acquire 未开始")
	}
	time.Sleep(35 * time.Millisecond)
	close(client.release)
	select {
	case <-client.returned:
	case <-time.After(time.Second):
		t.Fatal("忽略 deadline 的 Acquire 未返回")
	}
	time.Sleep(5 * time.Millisecond)
	if lease, ok := controller.current(); ok || lease.ID != "" {
		t.Fatalf("late control result installed lease=%+v", lease)
	}
}

func waitRepairLeaseCondition(t *testing.T, name string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", name)
}

type repairLeaseDeadlineFake struct {
	acquireCalls      atomic.Int32
	heartbeatCalls    atomic.Int32
	hangAcquireOnce   bool
	hangHeartbeatOnce bool
	mu                sync.Mutex
}

func (f *repairLeaseDeadlineFake) Acquire(ctx context.Context, request companion.AcquireRequest) (companion.AcquireResponse, error) {
	f.acquireCalls.Add(1)
	f.mu.Lock()
	hang := f.hangAcquireOnce
	f.hangAcquireOnce = false
	f.mu.Unlock()
	if hang {
		<-ctx.Done()
		return companion.AcquireResponse{}, ctx.Err()
	}
	return companion.AcquireResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", LeaseExpiresInMS: 60,
	}, nil
}

func (f *repairLeaseDeadlineFake) Heartbeat(ctx context.Context, request companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	f.heartbeatCalls.Add(1)
	f.mu.Lock()
	hang := f.hangHeartbeatOnce
	f.hangHeartbeatOnce = false
	f.mu.Unlock()
	if hang {
		<-ctx.Done()
		return companion.HeartbeatResponse{}, ctx.Err()
	}
	return companion.HeartbeatResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, LeaseExpiresInMS: 60,
	}, nil
}

func (*repairLeaseDeadlineFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*repairLeaseDeadlineFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

type repairLeaseCloseFake struct {
	once    sync.Once
	started chan struct{}
}

func (f *repairLeaseCloseFake) Acquire(ctx context.Context, _ companion.AcquireRequest) (companion.AcquireResponse, error) {
	f.once.Do(func() { close(f.started) })
	<-ctx.Done()
	return companion.AcquireResponse{}, ctx.Err()
}

func (*repairLeaseCloseFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (*repairLeaseCloseFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*repairLeaseCloseFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

type repairLateControlFake struct {
	once     sync.Once
	started  chan struct{}
	release  chan struct{}
	returned chan struct{}
	calls    atomic.Int32
}

func (f *repairLateControlFake) Acquire(_ context.Context, request companion.AcquireRequest) (companion.AcquireResponse, error) {
	if f.calls.Add(1) != 1 {
		return companion.AcquireResponse{}, errors.New("later acquire unavailable")
	}
	f.once.Do(func() { close(f.started) })
	<-f.release
	close(f.returned)
	return companion.AcquireResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", LeaseExpiresInMS: 60,
	}, nil
}

func (*repairLateControlFake) Heartbeat(context.Context, companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{}, companion.ErrAgentUnavailable
}

func (*repairLateControlFake) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*repairLateControlFake) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}
