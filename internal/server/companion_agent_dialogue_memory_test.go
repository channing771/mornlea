package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/storage"
)

func TestAgentDialogueBridgeRoundTripsWithoutGoMirrorSummary(t *testing.T) {
	var received map[string]any
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/dialogue" || request.Method != http.MethodPost {
			http.NotFound(w, request)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode Dialogue request: %v", err)
			return
		}
		if _, exists := received["summary"]; exists {
			t.Error("Go mirror summary leaked into Agent Dialogue request")
		}
		response := companion.AgentDialogueResponse{
			ContractVersion:  received["contract_version"].(string),
			RequestID:        received["request_id"].(string),
			ClientInstanceID: received["client_instance_id"].(string),
			NamespaceID:      received["namespace_id"].(string),
			LeaseID:          received["lease_id"].(string),
			RunID:            received["run_id"].(string),
			CompanionID:      received["companion_id"].(string),
			Generation:       uint64(received["generation"].(float64)),
			MemoryEpoch:      uint64(received["memory_epoch"].(float64)),
			Line:             "已经完成了。",
			MemoryProposal: &companion.AgentMemoryProposal{
				OperationID:  "66666666-6666-4666-8666-666666666666",
				BaseRevision: 7,
				Summary:      "最近完成了一项任务",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
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
		ID: "33333333-3333-4333-8333-333333333333", Fence: 1,
		Expires: time.Now().Add(time.Minute),
	}}
	ids := []string{
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
	}
	bridge := newCompanionAgentDialogue(agentDialogueOptions{
		Client: client, Lease: lease,
		ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:      "22222222-2222-4222-8222-222222222222",
		NewID: func() (string, error) {
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	})
	id := chatTestCompanionID(1)
	outcome, err := bridge.Dialogue(context.Background(), companionDialogueRequest{
		CompanionID: id,
		Generation:  9,
		MemoryEpoch: 3,
		Persona:     "沉稳",
		Fact: companion.AgentDialogueFact{
			Kind: "terminal", State: "completed", Reason: "none",
		},
		Environment: companion.AgentDialogueEnvironment{
			ExposedBlocks: []companion.AgentVisibleBlock{}, Heights: []companion.AgentHeight{},
		},
		Terminal: true,
	})
	if err != nil {
		t.Fatalf("Dialogue: %v", err)
	}
	if outcome.Line != "已经完成了。" || outcome.Proposal == nil ||
		outcome.Proposal.OperationID != "66666666-6666-4666-8666-666666666666" ||
		outcome.Proposal.BaseRevision != 7 {
		t.Fatalf("outcome=%+v", outcome)
	}
	if received["persona"] != "沉稳" || received["memory_epoch"] != float64(3) {
		t.Fatalf("request=%+v", received)
	}
}

func TestTerminalDialogueReservationSurvivesLaterGeneration(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, client, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	issuerIdentity := integrationIdentity(0x71, "发令者")
	issuer := stopTestIssuer(issuerIdentity)
	injectRunningCompanionTask(t, host, definition.ID, issuer, "完成任务", []companion.PlanStep{{
		Kind: companion.PlanStepGoTo, X: 0, Y: 1, Z: 0,
	}})

	host.world.stepMu.Lock()
	manager := host.world.companionManager
	slot := manager.slots[definition.ID]
	generation := slot.queue.Generation()
	if events := slot.queue.CompleteStep(); len(events) != 1 || events[0].Kind != companion.TaskEventCompleted {
		host.world.stepMu.Unlock()
		t.Fatalf("CompleteStep events=%+v", events)
	}
	slot.dialogueInFlight = true
	slot.dialogueAttempt = 7
	manager.applyDialogueOutcome(dialogueOutcome{
		id: definition.ID, generation: generation, attempt: 7,
		node:   companion.DialogueNode{Kind: companion.DialogueNodeTerminal, State: companion.TaskCompleted},
		issuer: issuer, memoryEpoch: 1,
		result: companionDialogueResult{
			Generation: generation, MemoryEpoch: 1, Line: "已经完成了。",
			Proposal: &companion.AgentMemoryProposal{
				OperationID: "66666666-6666-4666-8666-666666666666",
				Summary:     "最近完成了一项任务",
			},
		},
	})
	if slot.dialogueReservation == nil {
		host.world.stepMu.Unlock()
		t.Fatal("valid terminal proposal did not create accepted reservation")
	}
	if !slot.queue.Enqueue(companion.TaskCommand("继续工作")) || !slot.queue.BeginHead() {
		host.world.stepMu.Unlock()
		t.Fatal("failed to advance FIFO after accepted reservation")
	}
	if slot.queue.Generation() == generation {
		host.world.stepMu.Unlock()
		t.Fatal("new task did not advance generation")
	}
	if slot.dialogueReservation == nil ||
		slot.dialogueReservation.operationID != "66666666-6666-4666-8666-666666666666" {
		host.world.stepMu.Unlock()
		t.Fatal("later generation revoked accepted reservation")
	}
	host.world.stepMu.Unlock()

	result := host.world.StepForTest()
	receiveCompanionChatTick(t, client, result.Tick)
}

func TestCommittedReservationReplacesMirrorAndBroadcastsOnce(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	issuer := stopTestIssuer(integrationIdentity(0x72, "发令者"))
	operationText := "66666666-6666-4666-8666-666666666666"
	operation, err := companion.ParseID(operationText)
	if err != nil {
		t.Fatal(err)
	}

	host.world.stepMu.Lock()
	manager := host.world.companionManager
	slot := manager.slots[definition.ID]
	slot.dialogueReservation = &companionDialogueReservation{
		operationID: operationText, memoryEpoch: 1, baseRevision: 0,
		summary: "最近完成了一项任务", line: "已经完成了。", issuer: issuer,
	}
	outcome := memoryCommitOutcome{
		id: definition.ID, memoryEpoch: 1, operationID: operationText,
		committedRevision: 1,
	}
	manager.applyMemoryCommitOutcome(outcome)
	manager.applyMemoryCommitOutcome(outcome)
	lifecycle, ok := manager.companions.MemoryLifecycle(definition.ID)
	effects := manager.dialogueEffects
	reservation := slot.dialogueReservation
	host.world.stepMu.Unlock()

	if !ok || lifecycle.MemoryRevision != 1 ||
		lifecycle.MemoryOperationID != [16]byte(operation) ||
		lifecycle.Summary != "最近完成了一项任务" {
		t.Fatalf("lifecycle=%+v ok=%v", lifecycle, ok)
	}
	if reservation != nil || effects != 1 {
		t.Fatalf("reservation=%+v effects=%d", reservation, effects)
	}
}

func TestDialogueOutcomeRejectsAdvancedNodeWithinSameGeneration(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	issuer := stopTestIssuer(integrationIdentity(0x74, "发令者"))
	injectRunningCompanionTask(t, host, definition.ID, issuer, "走两步", []companion.PlanStep{
		{Kind: companion.PlanStepGoTo, X: 0, Y: 1, Z: 0},
		{Kind: companion.PlanStepGoTo, X: 1, Y: 1, Z: 0},
	})
	host.world.stepMu.Lock()
	manager := host.world.companionManager
	slot := manager.slots[definition.ID]
	generation := slot.queue.Generation()
	slot.dialogueInFlight = true
	slot.dialogueAttempt = 9
	if events := slot.queue.CompleteStep(); len(events) != 1 || events[0].Kind != companion.TaskEventProgress {
		host.world.stepMu.Unlock()
		t.Fatalf("CompleteStep events=%+v", events)
	}
	manager.applyDialogueOutcome(dialogueOutcome{
		id: definition.ID, generation: generation, attempt: 9, taskStepIndex: 0,
		node: companion.DialogueNode{Kind: companion.DialogueNodeStart}, issuer: issuer,
		memoryEpoch: 1,
		result: companionDialogueResult{
			Generation: generation, MemoryEpoch: 1, Line: "刚出发。",
		},
	})
	effects := manager.dialogueEffects
	host.world.stepMu.Unlock()
	if effects != 0 {
		t.Fatalf("advanced same-generation node applied effects=%d", effects)
	}
}

func TestAgentMemoryReconcileSendsCurrentCanonicalZeroOnly(t *testing.T) {
	var received companion.MemoryReconcileRequest
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/memory/reconcile" {
			http.NotFound(w, request)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode reconcile: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(companion.MemoryReconcileResponse{
			ContractVersion: received.ContractVersion, RequestID: received.RequestID,
			ClientInstanceID: received.ClientInstanceID, NamespaceID: received.NamespaceID,
			LeaseID: received.LeaseID, CompanionID: received.CompanionID,
			MemoryEpoch: received.MemoryEpoch, Active: true, Memory: received.Mirror,
		})
	}))
	defer agentServer.Close()
	client, err := companion.NewAgentClient(companion.AgentServiceSettings{
		Endpoint: agentServer.URL, APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
	}, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	lease := &companionAgentLeaseController{leaseTTL: companionAgentLeaseTTL, lease: companionAgentLease{
		ID: "33333333-3333-4333-8333-333333333333", Fence: 2, Expires: time.Now().Add(time.Minute),
	}}
	bridge := newCompanionAgentDialogue(agentDialogueOptions{
		Client: client, Lease: lease,
		ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:      "22222222-2222-4222-8222-222222222222",
		NewID:            func() (string, error) { return "44444444-4444-4444-8444-444444444444", nil },
	})
	lifecycle := storage.StoredCompanionLifecycle{
		ID: chatTestCompanionID(1), Active: true, MemoryEpoch: 3,
	}
	result, err := bridge.ReconcileMemory(context.Background(), lifecycle)
	if err != nil {
		t.Fatalf("ReconcileMemory: %v", err)
	}
	if received.MemoryEpoch != 3 || !received.Active || received.Mirror == nil ||
		received.Mirror.Revision != 0 || received.Mirror.OperationID != nil ||
		received.Mirror.Summary != "" || received.TombstoneOperationID != nil {
		t.Fatalf("canonical-zero request=%+v", received)
	}
	if result.Lifecycle != lifecycle {
		t.Fatalf("result=%+v want=%+v", result.Lifecycle, lifecycle)
	}
}

func TestReconcileConfirmsUnknownCommitExactlyOnce(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	issuer := stopTestIssuer(integrationIdentity(0x73, "发令者"))
	operationText := "66666666-6666-4666-8666-666666666666"
	operation, _ := companion.ParseID(operationText)
	host.world.stepMu.Lock()
	manager := host.world.companionManager
	slot := manager.slots[definition.ID]
	slot.dialogueReservation = &companionDialogueReservation{
		operationID: operationText, memoryEpoch: 1, baseRevision: 0,
		summary: "提交结果不明", line: "已经完成了。", issuer: issuer,
	}
	manager.applyMemoryCommitOutcome(memoryCommitOutcome{
		id: definition.ID, err: context.DeadlineExceeded,
	})
	if slot.dialogueReservation == nil || manager.dialogueEffects != 0 ||
		!manager.memoryReconcileRequested {
		host.world.stepMu.Unlock()
		t.Fatal("unknown commit did not preserve reservation for reconcile")
	}
	outcome := memoryReconcileOutcome{lifecycles: []storage.StoredCompanionLifecycle{{
		ID: definition.ID, Active: true, MemoryEpoch: 1, MemoryRevision: 1,
		MemoryOperationID: storage.CompanionIdentity(operation), Summary: "提交结果不明",
	}}}
	manager.applyMemoryReconcileOutcome(outcome)
	manager.applyMemoryReconcileOutcome(outcome)
	lifecycle, _ := manager.companions.MemoryLifecycle(definition.ID)
	effects := manager.dialogueEffects
	reservation := slot.dialogueReservation
	host.world.stepMu.Unlock()
	if lifecycle.MemoryRevision != 1 || lifecycle.MemoryOperationID != storage.CompanionIdentity(operation) ||
		lifecycle.Summary != "提交结果不明" || reservation != nil || effects != 1 {
		t.Fatalf("lifecycle=%+v reservation=%+v effects=%d", lifecycle, reservation, effects)
	}
}

type recordingMemoryReconciler struct {
	fence    atomic.Uint64
	requests chan storage.StoredCompanionLifecycle
}

func (*recordingMemoryReconciler) Dialogue(context.Context, companionDialogueRequest) (companionDialogueResult, error) {
	return companionDialogueResult{}, companion.ErrAgentUnavailable
}

func (*recordingMemoryReconciler) CommitMemory(context.Context, companionMemoryCommitRequest) (companionMemoryCommitResult, error) {
	return companionMemoryCommitResult{}, companion.ErrAgentUnavailable
}

func (r *recordingMemoryReconciler) currentMemoryFence() (uint64, bool) {
	return r.fence.Load(), true
}

func (r *recordingMemoryReconciler) ReconcileMemory(
	_ context.Context,
	lifecycle storage.StoredCompanionLifecycle,
) (companionMemoryReconcileResult, error) {
	r.requests <- lifecycle
	return companionMemoryReconcileResult{Lifecycle: lifecycle}, nil
}

func TestManagerReconcilesCurrentMirrorOnAcquireAndReacquire(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	reconciler := &recordingMemoryReconciler{requests: make(chan storage.StoredCompanionLifecycle, 2)}
	reconciler.fence.Store(1)
	manager := host.world.companionManager
	host.world.stepMu.Lock()
	manager.dialogue = reconciler
	manager.slots[definition.ID].memoryReady = false
	manager.dispatchMemoryReconcile()
	host.world.stepMu.Unlock()
	first := receiveMemoryLifecycle(t, reconciler.requests)
	waitMemoryReconcileResult(t, manager)
	host.world.stepMu.Lock()
	manager.applyMemoryReconcileOutcomes()
	readyAfterAcquire := manager.slots[definition.ID].memoryReady
	reconciler.fence.Store(2)
	manager.dispatchMemoryReconcile()
	host.world.stepMu.Unlock()
	second := receiveMemoryLifecycle(t, reconciler.requests)
	if first != second || first.ID != definition.ID || !first.Active || first.MemoryEpoch != 1 {
		t.Fatalf("acquire=%+v reacquire=%+v", first, second)
	}
	if !readyAfterAcquire {
		t.Fatal("successful acquire reconcile did not enable Dialogue")
	}
}

func receiveMemoryLifecycle(
	t *testing.T,
	results <-chan storage.StoredCompanionLifecycle,
) storage.StoredCompanionLifecycle {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(waitDeadline):
		t.Fatal("timed out waiting for memory reconcile request")
		return storage.StoredCompanionLifecycle{}
	}
}

func waitMemoryReconcileResult(t *testing.T, manager *companionManager) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for len(manager.memoryReconcileResults) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(manager.memoryReconcileResults) == 0 {
		t.Fatal("timed out waiting for memory reconcile result")
	}
}

func TestAgentMemoryDeleteCarriesTombstoneEpochFence(t *testing.T) {
	var received companion.MemoryDeleteRequest
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode delete: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(companion.MemoryDeleteResponse{
			ContractVersion: received.ContractVersion, RequestID: received.RequestID,
			ClientInstanceID: received.ClientInstanceID, NamespaceID: received.NamespaceID,
			LeaseID: received.LeaseID, CompanionID: received.CompanionID,
			MemoryEpoch:          received.NewMemoryEpoch,
			TombstoneOperationID: received.TombstoneOperationID,
		})
	}))
	defer agentServer.Close()
	client, err := companion.NewAgentClient(companion.AgentServiceSettings{
		Endpoint: agentServer.URL, APIKeyEnv: "MORNLEA_TEST_AGENT_KEY",
	}, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	lease := &companionAgentLeaseController{leaseTTL: companionAgentLeaseTTL, lease: companionAgentLease{
		ID: "33333333-3333-4333-8333-333333333333", Fence: 4, Expires: time.Now().Add(time.Minute),
	}}
	bridge := newCompanionAgentDialogue(agentDialogueOptions{
		Client: client, Lease: lease,
		ClientInstanceID: "11111111-1111-4111-8111-111111111111",
		NamespaceID:      "22222222-2222-4222-8222-222222222222",
		NewID:            func() (string, error) { return "44444444-4444-4444-8444-444444444444", nil },
	})
	result, err := bridge.DeleteMemory(context.Background(), companionMemoryDeleteRequest{
		CompanionID: chatTestCompanionID(1), OldMemoryEpoch: 2, NewMemoryEpoch: 3,
		TombstoneOperationID: "66666666-6666-4666-8666-666666666666",
	})
	if err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if received.OldMemoryEpoch != 2 || received.NewMemoryEpoch != 3 ||
		received.TombstoneOperationID != "66666666-6666-4666-8666-666666666666" ||
		result.MemoryEpoch != 3 || result.TombstoneOperationID != received.TombstoneOperationID {
		t.Fatalf("request=%+v result=%+v", received, result)
	}
}

type shutdownAgentRuntime struct {
	store    *companionBootstrapStore
	acquired chan struct{}
	releases atomic.Int32
	closed   atomic.Bool
}

func (f *shutdownAgentRuntime) Acquire(_ context.Context, request companion.AcquireRequest) (companion.AcquireResponse, error) {
	select {
	case <-f.acquired:
	default:
		close(f.acquired)
	}
	return companion.AcquireResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID:          "33333333-3333-4333-8333-333333333333",
		LeaseExpiresInMS: int(companionAgentLeaseTTL / time.Millisecond),
	}, nil
}

func (*shutdownAgentRuntime) Heartbeat(_ context.Context, request companion.LeaseRequest) (companion.HeartbeatResponse, error) {
	return companion.HeartbeatResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, LeaseExpiresInMS: int(companionAgentLeaseTTL / time.Millisecond),
	}, nil
}

func (*shutdownAgentRuntime) Plan(context.Context, companion.PlanRequest) (companion.PlanResponse, error) {
	return companion.PlanResponse{}, companion.ErrAgentUnavailable
}

func (*shutdownAgentRuntime) Dialogue(context.Context, companion.AgentDialogueRequest) (companion.AgentDialogueResponse, error) {
	return companion.AgentDialogueResponse{}, companion.ErrAgentUnavailable
}

func (*shutdownAgentRuntime) CommitMemory(context.Context, companion.MemoryCommitRequest) (companion.MemoryCommitResponse, error) {
	return companion.MemoryCommitResponse{}, companion.ErrAgentUnavailable
}

func (*shutdownAgentRuntime) ReconcileMemory(_ context.Context, request companion.MemoryReconcileRequest) (companion.MemoryReconcileResponse, error) {
	return companion.MemoryReconcileResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, CompanionID: request.CompanionID,
		MemoryEpoch: request.MemoryEpoch, Active: request.Active,
		Memory: request.Mirror, TombstoneOperationID: request.TombstoneOperationID,
	}, nil
}

func (*shutdownAgentRuntime) DeleteMemory(context.Context, companion.MemoryDeleteRequest) (companion.MemoryDeleteResponse, error) {
	return companion.MemoryDeleteResponse{}, companion.ErrAgentUnavailable
}

func (*shutdownAgentRuntime) CancelRun(context.Context, companion.CancelRequest) (companion.CancelResponse, error) {
	return companion.CancelResponse{}, companion.ErrAgentUnavailable
}

func (f *shutdownAgentRuntime) Release(_ context.Context, request companion.LeaseRequest) (companion.ReleaseResponse, error) {
	f.releases.Add(1)
	f.store.hostTestStore.mu.Lock()
	f.store.hostTestStore.events = append(f.store.hostTestStore.events, "release")
	f.store.hostTestStore.mu.Unlock()
	return companion.ReleaseResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, Released: true,
	}, nil
}

func (f *shutdownAgentRuntime) Close() {
	f.closed.Store(true)
	f.store.hostTestStore.mu.Lock()
	f.store.hostTestStore.events = append(f.store.hostTestStore.events, "agent-close")
	f.store.hostTestStore.mu.Unlock()
}

func TestHostShutdownRetriesPersistenceBeforeReleaseAndClose(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	config.companionPlanner = nil
	fake := &shutdownAgentRuntime{store: store, acquired: make(chan struct{})}
	config.companionAgentClientFactory = func(companion.AgentServiceSettings, string) (companionAgentRuntimeClient, error) {
		return fake, nil
	}
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	select {
	case <-fake.acquired:
	case <-time.After(waitDeadline):
		t.Fatal("timed out waiting for namespace acquire")
	}
	operation := companionBootstrapIdentity(0x79)
	if err := host.world.companions.ReplaceActiveMemory(id, 1, 0, 1, operation, "最终 mirror"); err != nil {
		t.Fatalf("seed dirty mirror: %v", err)
	}
	saveErr := errors.New("final companion save failed")
	store.mu.Lock()
	store.saveErrors = []error{saveErr}
	store.mu.Unlock()
	store.hostTestStore.mu.Lock()
	store.hostTestStore.events = nil
	store.hostTestStore.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	err = host.Shutdown(ctx)
	cancel()
	if !errors.Is(err, saveErr) || fake.releases.Load() != 0 || fake.closed.Load() ||
		store.hostTestStore.closeCount() != 0 {
		t.Fatalf("first shutdown err=%v releases=%d agentClosed=%v storeCloses=%d events=%v",
			err, fake.releases.Load(), fake.closed.Load(), store.hostTestStore.closeCount(), store.eventsSnapshot())
	}

	ctx, cancel = context.WithTimeout(context.Background(), waitDeadline)
	err = host.Shutdown(ctx)
	cancel()
	if err != nil {
		t.Fatalf("retry Shutdown: %v", err)
	}
	events := store.eventsSnapshot()
	wantOrdered := []string{"companion-save", "sync", "release", "agent-close", "close"}
	position := 0
	for _, event := range events {
		if position < len(wantOrdered) && event == wantOrdered[position] {
			position++
		}
	}
	if position != len(wantOrdered) || fake.releases.Load() != 1 || !fake.closed.Load() ||
		!slices.Contains(events, "release") {
		t.Fatalf("shutdown events=%v want ordered=%v releases=%d agentClosed=%v",
			events, wantOrdered, fake.releases.Load(), fake.closed.Load())
	}
}
