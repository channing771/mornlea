package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestInterestObserverSamplesEachObserverAndNilIsOptional(t *testing.T) {
	config := hostTestConfig()
	var samples atomic.Int64
	config.InterestObserver = func(duration time.Duration) {
		if duration < 0 {
			t.Errorf("negative interest duration: %v", duration)
		}
		samples.Add(1)
	}
	// 夜行者发布从 tick 末的 Engine 值快照取数：publish 需要 Engine 存在
	// （与 queueReadyAndResync 的订阅判定同一结构前提）。
	running := &Server{
		config:   config,
		engine:   runtime.NewEngine(0, 0, 0),
		sessions: make(map[contract.SessionID]*session),
	}
	for index := 1; index <= 8; index++ {
		id := contract.SessionID(index)
		running.sessions[id] = &session{
			id: id, playerID: playerID(byte(index)),
			outbox:           make(chan network.ServerMessage, config.OutboxCapacity),
			publications:     make(map[core.ChunkKey]*publication),
			pendingSnapshots: make(map[core.ChunkKey]snapshotRequest),
			visiblePlayers:   make(map[core.PlayerID]visiblePlayer),
		}
	}
	running.publish(contract.TickResult{Forget: make(map[contract.SessionID][]core.ChunkKey)})
	if got := samples.Load(); got != 8 {
		t.Fatalf("interest samples=%d, want one per observer (8)", got)
	}

	nilConfig := DefaultConfig(1)
	if nilConfig.InterestObserver != nil {
		t.Fatalf("default InterestObserver type=%T, want nil fast path", nilConfig.InterestObserver)
	}
}

func TestHostStatsAreScalarBoundedSnapshotsWithNoNestedLocking(t *testing.T) {
	host := mustNewHost(t, hostTestConfig(), flatTestGenerator{}, newHostTestStore())
	t.Cleanup(func() {
		host.world.stepMu.Lock()
		clear(host.world.sessions)
		host.world.stepMu.Unlock()
		shutdownServerForTest(t, host.world)
		host.players.CloseWorker()
	})
	host.mu.Lock()
	for index := 1; index <= 8; index++ {
		host.activeBySession[contract.SessionID(index)] = &activeLogin{Session: contract.SessionID(index)}
	}
	host.mu.Unlock()
	host.world.stepMu.Lock()
	host.world.sessions[1] = &session{outbox: make(chan network.ServerMessage, 8)}
	host.world.sessions[2] = &session{outbox: make(chan network.ServerMessage, 8)}
	host.world.sessions[1].outbox <- network.PlayerState{}
	host.world.sessions[2].outbox <- network.PlayerState{}
	host.world.sessions[2].outbox <- network.PlayerState{}
	host.world.stepMu.Unlock()
	// 玩家队列深度改由子包通过 QueueDepths 提供，Stats 仅做有界快照。
	// 该测试保留 Stats 的标量与并发边界断言，对玩家队列的精确值
	// 改为从子包的实时深度读取，初建时为 0/0。
	jobDepth, doneDepth := host.players.QueueDepths()
	if got := host.Stats(); got != (HostStats{
		ActivePlayers: 8, MaxSessionOutboxDepth: 2,
		PlayerSaveJobDepth: jobDepth, PlayerSaveDoneDepth: doneDepth,
	}) {
		t.Fatalf("Stats=%+v", got)
	}
	if jobDepth != 0 || doneDepth != 0 {
		t.Fatalf("初始玩家队列深度 = %d/%d，想要 0/0", jobDepth, doneDepth)
	}

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for range 1000 {
			stats := host.Stats()
			if stats.ActivePlayers < 0 || stats.ActivePlayers > 8 ||
				stats.MaxSessionOutboxDepth < 0 || stats.MaxSessionOutboxDepth > 8 ||
				stats.PlayerSaveJobDepth < 0 || stats.PlayerSaveJobDepth > 16 ||
				stats.PlayerSaveDoneDepth < 0 || stats.PlayerSaveDoneDepth > 2 {
				t.Errorf("unbounded stats: %+v", stats)
				return
			}
		}
	}()
	go func() {
		defer group.Done()
		for index := range 1000 {
			host.mu.Lock()
			if index%2 == 0 {
				host.activeBySession[8] = &activeLogin{Session: 8}
			} else {
				delete(host.activeBySession, 8)
			}
			host.mu.Unlock()
		}
	}()
	group.Wait()
}

func TestHostRunAtInputBoundaryWaitsForDistinctIngressAndExcludesWorldStep(t *testing.T) {
	host := &Host{world: &Server{
		ctx: context.Background(), lifecycle: serverClosed,
		incoming: make(chan incomingCommand, 8),
	}}
	boundaryEntered := make(chan struct{})
	boundaryDone := make(chan error, 1)
	go func() {
		boundaryDone <- host.RunAtInputBoundary(context.Background(), 9, 2, func() error {
			close(boundaryEntered)
			return nil
		})
	}()
	<-boundaryEntered

	stepDone := make(chan struct{})
	go func() {
		host.world.StepForTest()
		close(stepDone)
	}()
	select {
	case <-stepDone:
		t.Fatal("world step entered while step boundary action was running")
	case <-time.After(25 * time.Millisecond):
	}

	for range 2 {
		host.world.enqueueIncoming(context.Background(), incomingCommand{
			Session: 1,
			Command: contract.Command{Session: 1, Sequence: 9, Kind: contract.CommandPlayerInput},
		})
	}
	select {
	case err := <-boundaryDone:
		t.Fatalf("duplicate session completed ingress boundary: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	host.world.enqueueIncoming(context.Background(), incomingCommand{
		Session: 2,
		Command: contract.Command{Session: 2, Sequence: 9, Kind: contract.CommandPlayerInput},
	})
	if err := <-boundaryDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-stepDone:
	case <-time.After(waitDeadline):
		t.Fatal("world step did not resume after step boundary action")
	}
}

func TestHostRunAtInputBoundaryDeadlineClearsWaiterAndReleasesWorldStep(t *testing.T) {
	host := &Host{world: &Server{
		ctx: context.Background(), lifecycle: serverClosed,
		incoming: make(chan incomingCommand, 8),
	}}
	boundaryCtx, cancelBoundary := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelBoundary()
	err := host.RunAtInputBoundary(boundaryCtx, 7, 2, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("input boundary error=%v, want deadline exceeded", err)
	}
	if boundary := host.world.inputBoundary.Load(); boundary != nil {
		t.Fatalf("expired input boundary still registered: %+v", boundary)
	}
	stepDone := make(chan struct{})
	go func() {
		host.world.StepForTest()
		close(stepDone)
	}()
	select {
	case <-stepDone:
	case <-time.After(waitDeadline):
		t.Fatal("world step remained blocked after input boundary deadline")
	}
}
