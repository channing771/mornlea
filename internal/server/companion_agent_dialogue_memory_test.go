package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/server/persistence"
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
	// 本测试只锁定 generation 独立性；白盒 reservation 在断言后清理，避免
	// cleanup 关服把未确认 memory 正确识别为不可 Release。
	slot.dialogueReservation = nil
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
	outcome := memoryReconcileOutcome{results: []memoryReconcileCompanionOutcome{{
		id: definition.ID,
		lifecycle: storage.StoredCompanionLifecycle{
			ID: definition.ID, Active: true, MemoryEpoch: 1, MemoryRevision: 1,
			MemoryOperationID: storage.CompanionIdentity(operation), Summary: "提交结果不明",
		},
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

type scriptedMemoryReconciler struct {
	fence          atomic.Uint64
	calls          atomic.Int32
	dialogueCalls  atomic.Int32
	commitRequests chan companionMemoryCommitRequest
	reconcile      func(storage.StoredCompanionLifecycle) (storage.StoredCompanionLifecycle, error)
}

func (r *scriptedMemoryReconciler) Dialogue(context.Context, companionDialogueRequest) (companionDialogueResult, error) {
	r.dialogueCalls.Add(1)
	return companionDialogueResult{}, companion.ErrAgentUnavailable
}

func (r *scriptedMemoryReconciler) CommitMemory(
	_ context.Context,
	request companionMemoryCommitRequest,
) (companionMemoryCommitResult, error) {
	if r.commitRequests == nil {
		return companionMemoryCommitResult{}, companion.ErrAgentUnavailable
	}
	r.commitRequests <- request
	return companionMemoryCommitResult{
		MemoryEpoch:       request.MemoryEpoch,
		OperationID:       request.OperationID,
		CommittedRevision: request.BaseRevision + 1,
	}, nil
}

func (r *scriptedMemoryReconciler) currentMemoryFence() (uint64, bool) {
	return r.fence.Load(), true
}

func (r *scriptedMemoryReconciler) ReconcileMemory(
	_ context.Context,
	lifecycle storage.StoredCompanionLifecycle,
) (companionMemoryReconcileResult, error) {
	r.calls.Add(1)
	if r.reconcile == nil {
		return companionMemoryReconcileResult{Lifecycle: lifecycle}, nil
	}
	result, err := r.reconcile(lifecycle)
	return companionMemoryReconcileResult{Lifecycle: result}, err
}

func TestMemoryReconcileUnavailableDoesNotStopLaterCompanion(t *testing.T) {
	definitions := []companion.Definition{
		{ID: chatTestCompanionID(1), Name: "阿木"},
		{ID: chatTestCompanionID(2), Name: "阿石"},
	}
	host, _, _ := companionManagerHostReady(t, definitions, nil)
	operation := companionBootstrapIdentity(0x79)
	reconciler := &scriptedMemoryReconciler{}
	reconciler.fence.Store(1)
	reconciler.reconcile = func(lifecycle storage.StoredCompanionLifecycle) (storage.StoredCompanionLifecycle, error) {
		if lifecycle.ID == definitions[0].ID {
			return storage.StoredCompanionLifecycle{}, companion.ErrAgentUnavailable
		}
		lifecycle.MemoryRevision = 1
		lifecycle.MemoryOperationID = operation
		lifecycle.Summary = "远端较新的 mirror"
		return lifecycle, nil
	}

	manager := host.world.companionManager
	host.world.stepMu.Lock()
	manager.dialogue = reconciler
	for _, definition := range definitions {
		manager.slots[definition.ID].memoryReady = false
	}
	manager.dispatchMemoryReconcile()
	host.world.stepMu.Unlock()
	waitMemoryReconcileResult(t, manager)
	host.world.stepMu.Lock()
	manager.applyMemoryReconcileOutcomes()
	firstReady := manager.slots[definitions[0].ID].memoryReady
	secondReady := manager.slots[definitions[1].ID].memoryReady
	second, _ := manager.companions.MemoryLifecycle(definitions[1].ID)
	host.world.stepMu.Unlock()

	if got := reconciler.calls.Load(); got != 2 {
		t.Fatalf("reconcile calls=%d, want both companions", got)
	}
	if firstReady || !secondReady || second.MemoryRevision != 1 ||
		second.MemoryOperationID != operation || second.Summary != "远端较新的 mirror" {
		t.Fatalf("firstReady=%v secondReady=%v second=%+v", firstReady, secondReady, second)
	}
}

func TestMemoryReconcileAcquireIncludesInactiveTombstoneWithoutPausingActive(t *testing.T) {
	activeID := chatTestCompanionID(1)
	inactiveID := chatTestCompanionID(2)
	tombstone := companionBootstrapIdentity(0x7a)
	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42})
	companions := persistence.NewCompanions(store, storage.StoredCompanions{
		Revision:         7,
		AgentNamespaceID: companionBootstrapIdentity(0x70),
		Records: []companion.Body{
			companionDialogueWiringBody(1, 10),
			companionDialogueWiringBody(2, 20),
		},
		Lifecycles: []storage.StoredCompanionLifecycle{
			{ID: activeID, Active: true, MemoryEpoch: 3},
			{ID: inactiveID, Active: false, MemoryEpoch: 4, TombstoneOperationID: tombstone},
		},
	}, persistence.Options{AutosaveTicks: 10, RetryBaseTicks: 2, RetryMaxTicks: 8})
	t.Cleanup(companions.Close)

	var callsMu sync.Mutex
	var calls []storage.StoredCompanionLifecycle
	reconciler := &scriptedMemoryReconciler{}
	reconciler.fence.Store(1)
	reconciler.reconcile = func(lifecycle storage.StoredCompanionLifecycle) (storage.StoredCompanionLifecycle, error) {
		callsMu.Lock()
		calls = append(calls, lifecycle)
		callsMu.Unlock()
		if !lifecycle.Active {
			return storage.StoredCompanionLifecycle{}, companion.ErrAgentUnavailable
		}
		return lifecycle, nil
	}
	manager := newCompanionManager(nil, Config{
		Companions:         []companion.Definition{{ID: activeID, Name: "阿木"}},
		TaskTimeoutMinutes: 10,
	}, unavailablePlannerTestSeam{}, reconciler, companions)
	t.Cleanup(manager.beginShutdown)

	for _, fence := range []uint64{1, 2} {
		reconciler.fence.Store(fence)
		manager.dispatchMemoryReconcile()
		waitMemoryReconcileResult(t, manager)
		manager.applyMemoryReconcileOutcomes()
		if !manager.slots[activeID].memoryReady {
			t.Fatalf("fence %d 的 inactive 故障暂停了 active 伙伴", fence)
		}
	}

	callsMu.Lock()
	defer callsMu.Unlock()
	if len(calls) != 4 {
		t.Fatalf("reconcile calls=%+v，想要 acquire/reacquire 均发送 active 与 inactive", calls)
	}
	for index, lifecycle := range calls {
		if index%2 == 0 {
			if lifecycle.ID != activeID || !lifecycle.Active || lifecycle.MemoryEpoch != 3 {
				t.Fatalf("active call[%d]=%+v", index, lifecycle)
			}
			continue
		}
		if lifecycle.ID != inactiveID || lifecycle.Active || lifecycle.MemoryEpoch != 4 ||
			lifecycle.TombstoneOperationID != tombstone {
			t.Fatalf("inactive call[%d]=%+v", index, lifecycle)
		}
	}
}

func TestMemoryReconcileRejectsStaleFenceBeforeMutation(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	reconciler := &recordingMemoryReconciler{requests: make(chan storage.StoredCompanionLifecycle, 1)}
	reconciler.fence.Store(2)
	manager := host.world.companionManager
	remote := storage.StoredCompanionLifecycle{
		ID: definition.ID, Active: true, MemoryEpoch: 1, MemoryRevision: 1,
		MemoryOperationID: companionBootstrapIdentity(0x79), Summary: "旧 lease 的远端 mirror",
	}

	host.world.stepMu.Lock()
	manager.dialogue = reconciler
	manager.memoryReconcileInFlight = true
	manager.memoryReconcileResults <- memoryReconcileOutcome{
		fence: 1, results: []memoryReconcileCompanionOutcome{{
			id: definition.ID, lifecycle: remote,
		}},
	}
	manager.applyMemoryReconcileOutcomes()
	local, _ := manager.companions.MemoryLifecycle(definition.ID)
	ready := manager.slots[definition.ID].memoryReady
	manager.dispatchMemoryReconcile()
	host.world.stepMu.Unlock()

	if local.MemoryRevision != 0 || local.MemoryOperationID != (storage.CompanionIdentity{}) ||
		local.Summary != "" || ready {
		t.Fatalf("stale fence mutated state: lifecycle=%+v ready=%v", local, ready)
	}
	request := receiveMemoryLifecycle(t, reconciler.requests)
	if request.MemoryRevision != 0 || request.MemoryOperationID != (storage.CompanionIdentity{}) ||
		request.Summary != "" {
		t.Fatalf("fresh fence replayed stale state: %+v", request)
	}
}

func TestMemoryReconcileErrorRetriesWithBoundedBackoff(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	var failures atomic.Int32
	reconciler := &scriptedMemoryReconciler{}
	reconciler.fence.Store(1)
	reconciler.reconcile = func(lifecycle storage.StoredCompanionLifecycle) (storage.StoredCompanionLifecycle, error) {
		if failures.Add(1) == 1 {
			return storage.StoredCompanionLifecycle{}, companion.ErrAgentUnavailable
		}
		return lifecycle, nil
	}
	manager := host.world.companionManager
	host.world.stepMu.Lock()
	manager.dialogue = reconciler
	manager.slots[definition.ID].memoryReady = false
	manager.dispatchMemoryReconcile()
	host.world.stepMu.Unlock()
	waitMemoryReconcileResult(t, manager)
	host.world.stepMu.Lock()
	manager.applyMemoryReconcileOutcomes()
	if manager.memoryReconcileFence == 1 || manager.memoryReconcileRetryWait != 1 {
		host.world.stepMu.Unlock()
		t.Fatalf("failed reconcile completed fence or lost backoff: fence=%d wait=%d",
			manager.memoryReconcileFence, manager.memoryReconcileRetryWait)
	}
	manager.dispatchMemoryReconcile()
	if got := reconciler.calls.Load(); got != 1 {
		host.world.stepMu.Unlock()
		t.Fatalf("retry ignored first backoff tick: calls=%d", got)
	}
	manager.dispatchMemoryReconcile()
	host.world.stepMu.Unlock()
	waitMemoryReconcileResult(t, manager)
	host.world.stepMu.Lock()
	manager.applyMemoryReconcileOutcomes()
	ready := manager.slots[definition.ID].memoryReady
	fence := manager.memoryReconcileFence
	host.world.stepMu.Unlock()
	if !ready || fence != 1 || reconciler.calls.Load() != 2 {
		t.Fatalf("retry did not recover: ready=%v fence=%d calls=%d",
			ready, fence, reconciler.calls.Load())
	}
}

func TestMemoryReconcileConflictIsolatesRemoteHigherAndPreservesFIFO(t *testing.T) {
	definitions := []companion.Definition{
		{ID: chatTestCompanionID(1), Name: "阿木"},
		{ID: chatTestCompanionID(2), Name: "阿石"},
	}
	store := newHostTestStore()
	config := hostTestConfig()
	config.Companions = definitions
	config.MaxPlayers = 2
	config.HeartbeatInterval = time.Hour
	config.HeartbeatTimeout = time.Hour
	host := mustNewHost(t, config, flatTestGenerator{}, store)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), longWaitDeadline)
		defer cancel()
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("Host.Shutdown: %v", err)
		}
	})
	client := openCompanionChatClient(t, host, "memory", integrationIdentity(0x71, "发令者"))
	_ = stepUntilCompanionManagerReady(t, host, []network.ClientEndpoint{client}, definitions[0].ID)
	manager := host.world.companionManager
	localOperation := companionBootstrapIdentity(0x77)
	conflictOperation := companionBootstrapIdentity(0x78)
	higherOperation := companionBootstrapIdentity(0x79)
	if err := manager.companions.ReplaceActiveMemory(
		definitions[0].ID, 1, 0, 1, localOperation, "本地 mirror",
	); err != nil {
		t.Fatal(err)
	}

	host.world.stepMu.Lock()
	for _, definition := range definitions {
		if !manager.slots[definition.ID].queue.Enqueue(companion.TaskCommand("保持 FIFO")) {
			host.world.stepMu.Unlock()
			t.Fatal("failed to seed FIFO")
		}
	}
	beforeFirst := manager.slots[definitions[0].ID].queue.Len()
	beforeSecond := manager.slots[definitions[1].ID].queue.Len()
	manager.applyMemoryReconcileOutcome(memoryReconcileOutcome{
		fence: 1,
		results: []memoryReconcileCompanionOutcome{
			{id: definitions[0].ID, lifecycle: storage.StoredCompanionLifecycle{
				ID: definitions[0].ID, Active: true, MemoryEpoch: 1, MemoryRevision: 1,
				MemoryOperationID: conflictOperation, Summary: "分叉 mirror",
			}},
			{id: definitions[1].ID, lifecycle: storage.StoredCompanionLifecycle{
				ID: definitions[1].ID, Active: true, MemoryEpoch: 1, MemoryRevision: 1,
				MemoryOperationID: higherOperation, Summary: "远端较新的 mirror",
			}},
		},
	})
	first := manager.slots[definitions[0].ID]
	second := manager.slots[definitions[1].ID]
	firstLifecycle, _ := manager.companions.MemoryLifecycle(definitions[0].ID)
	secondLifecycle, _ := manager.companions.MemoryLifecycle(definitions[1].ID)
	fifoUnchanged := first.queue.Len() == beforeFirst && second.queue.Len() == beforeSecond
	host.world.stepMu.Unlock()
	if first.memoryReady || !second.memoryReady || !fifoUnchanged ||
		firstLifecycle.MemoryOperationID != localOperation ||
		secondLifecycle.MemoryOperationID != higherOperation ||
		secondLifecycle.Summary != "远端较新的 mirror" {
		t.Fatalf("firstReady=%v secondReady=%v fifo=%v first=%+v second=%+v",
			first.memoryReady, second.memoryReady, fifoUnchanged, firstLifecycle, secondLifecycle)
	}
	if err := manager.companions.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCompanions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Lifecycles) != 2 || loaded.Lifecycles[0].MemoryOperationID != localOperation ||
		loaded.Lifecycles[1].MemoryOperationID != higherOperation {
		t.Fatalf("persisted lifecycles=%+v", loaded.Lifecycles)
	}
	beforeTick := host.world.TickCount()
	result := host.world.StepForTest()
	_ = receiveCompanionChatTick(t, client, result.Tick)
	if result.Tick <= beforeTick {
		t.Fatalf("reconcile conflict stopped tick: before=%d after=%d", beforeTick, result.Tick)
	}
}

func TestUnknownCommitOldMirrorRetriesSameOperationWithoutDialogue(t *testing.T) {
	definition := companion.Definition{ID: chatTestCompanionID(1), Name: "阿木"}
	host, _, _ := companionManagerHostReady(t, []companion.Definition{definition}, nil)
	reconciler := &scriptedMemoryReconciler{
		commitRequests: make(chan companionMemoryCommitRequest, 1),
	}
	reconciler.fence.Store(1)
	manager := host.world.companionManager
	operationText := "66666666-6666-4666-8666-666666666666"

	host.world.stepMu.Lock()
	manager.dialogue = reconciler
	slot := manager.slots[definition.ID]
	slot.dialogueReservation = &companionDialogueReservation{
		operationID: operationText, memoryEpoch: 1, baseRevision: 0,
		summary: "结果不明后幂等重提", line: "已经完成了。",
		issuer: stopTestIssuer(integrationIdentity(0x73, "发令者")),
	}
	manager.applyMemoryCommitOutcome(memoryCommitOutcome{
		id: definition.ID, err: context.DeadlineExceeded,
	})
	manager.applyMemoryReconcileOutcome(memoryReconcileOutcome{
		fence: 1,
		results: []memoryReconcileCompanionOutcome{{
			id: definition.ID,
			lifecycle: storage.StoredCompanionLifecycle{
				ID: definition.ID, Active: true, MemoryEpoch: 1,
			},
		}},
	})
	host.world.stepMu.Unlock()

	var retried companionMemoryCommitRequest
	select {
	case retried = <-reconciler.commitRequests:
	case <-time.After(2 * time.Second):
		t.Fatal("old remote mirror did not retry the accepted operation")
	}
	if retried.CompanionID != definition.ID || retried.MemoryEpoch != 1 ||
		retried.BaseRevision != 0 || retried.OperationID != operationText ||
		retried.Summary != "结果不明后幂等重提" {
		t.Fatalf("retried commit=%+v", retried)
	}
	waitIntegrationCondition(t, "幂等 commit 结果", func() bool {
		return len(manager.memoryCommitResults) != 0
	})
	host.world.stepMu.Lock()
	manager.applyMemoryCommitOutcomes()
	manager.applyMemoryCommitOutcomes()
	lifecycle, _ := manager.companions.MemoryLifecycle(definition.ID)
	reservation := slot.dialogueReservation
	effects := manager.dialogueEffects
	host.world.stepMu.Unlock()
	if lifecycle.MemoryRevision != 1 || lifecycle.Summary != "结果不明后幂等重提" ||
		reservation != nil || effects != 1 || reconciler.dialogueCalls.Load() != 0 {
		t.Fatalf("lifecycle=%+v reservation=%+v effects=%d dialogueCalls=%d",
			lifecycle, reservation, effects, reconciler.dialogueCalls.Load())
	}
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
	store         *companionBootstrapStore
	acquired      chan struct{}
	commitStarted chan struct{}
	commitRelease chan struct{}
	commitContext chan shutdownCommitContext
	commitOnce    sync.Once
	mu            sync.Mutex
	releaseErrors []error
	releaseLeases []string
	commits       atomic.Int32
	releases      atomic.Int32
	closed        atomic.Bool
}

type shutdownCommitContext struct {
	err         error
	deadline    time.Time
	hasDeadline bool
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

func (f *shutdownAgentRuntime) CommitMemory(
	ctx context.Context,
	request companion.MemoryCommitRequest,
) (companion.MemoryCommitResponse, error) {
	if f.commitStarted == nil || f.commitRelease == nil {
		return companion.MemoryCommitResponse{}, companion.ErrAgentUnavailable
	}
	f.commits.Add(1)
	deadline, hasDeadline := ctx.Deadline()
	if f.commitContext != nil {
		f.commitContext <- shutdownCommitContext{
			err: ctx.Err(), deadline: deadline, hasDeadline: hasDeadline,
		}
	}
	f.commitOnce.Do(func() { close(f.commitStarted) })
	select {
	case <-f.commitRelease:
	case <-ctx.Done():
		return companion.MemoryCommitResponse{}, ctx.Err()
	}
	return companion.MemoryCommitResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, CompanionID: request.CompanionID,
		MemoryEpoch:       request.MemoryEpoch,
		OperationID:       request.OperationID,
		CommittedRevision: request.BaseRevision + 1,
	}, nil
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
	f.mu.Lock()
	f.releaseLeases = append(f.releaseLeases, request.LeaseID)
	var releaseErr error
	if len(f.releaseErrors) != 0 {
		releaseErr = f.releaseErrors[0]
		f.releaseErrors = f.releaseErrors[1:]
	}
	f.mu.Unlock()
	f.store.hostTestStore.mu.Lock()
	f.store.hostTestStore.events = append(f.store.hostTestStore.events, "release")
	f.store.hostTestStore.mu.Unlock()
	if releaseErr != nil {
		return companion.ReleaseResponse{}, releaseErr
	}
	return companion.ReleaseResponse{
		ContractVersion: request.ContractVersion, RequestID: request.RequestID,
		ClientInstanceID: request.ClientInstanceID, NamespaceID: request.NamespaceID,
		LeaseID: request.LeaseID, Released: true,
	}, nil
}

func (f *shutdownAgentRuntime) releaseLeaseSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.releaseLeases)
}

func (f *shutdownAgentRuntime) Close() {
	f.closed.Store(true)
	f.store.hostTestStore.mu.Lock()
	f.store.hostTestStore.events = append(f.store.hostTestStore.events, "agent-close")
	f.store.hostTestStore.mu.Unlock()
}

func TestHostShutdownWaitsForCommitDerivedFromQueuedDialogueOutcome(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	config.companionPlanner = nil
	fake := &shutdownAgentRuntime{
		store: store, acquired: make(chan struct{}),
		commitStarted: make(chan struct{}), commitRelease: make(chan struct{}),
		commitContext: make(chan shutdownCommitContext, 1),
	}
	var releaseCommit sync.Once
	unblockCommit := func() { releaseCommit.Do(func() { close(fake.commitRelease) }) }
	defer unblockCommit()
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
	issuerIdentity := integrationIdentity(0x73, "发令者")
	issuer := stopTestIssuer(issuerIdentity)
	manager := host.world.companionManager
	host.world.stepMu.Lock()
	manager.onlinePlayers = nil
	slot := manager.slots[id]
	slot.currentIssuer = issuer
	slot.dialogueInFlight = true
	slot.dialogueAttempt = 9
	manager.dialogueResults <- dialogueOutcome{
		id: id, generation: slot.queue.Generation(), attempt: 9,
		node:   companion.DialogueNode{Kind: companion.DialogueNodeTerminal, State: companion.TaskCompleted},
		issuer: issuer, memoryEpoch: 1,
		result: companionDialogueResult{
			Generation: slot.queue.Generation(), MemoryEpoch: 1, Line: "已经完成了。",
			Proposal: &companion.AgentMemoryProposal{
				OperationID:  "66666666-6666-4666-8666-666666666666",
				BaseRevision: 0, Summary: "关服前确认的 mirror",
			},
		},
	}
	host.world.stepMu.Unlock()

	shutdownDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	go func() { shutdownDone <- host.Shutdown(ctx) }()
	select {
	case <-fake.commitStarted:
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before derived commit started: %v", err)
	case <-time.After(waitDeadline):
		t.Fatal("Shutdown did not derive queued terminal commit")
	}
	commitContext := <-fake.commitContext
	if commitContext.err != nil || !commitContext.hasDeadline ||
		time.Until(commitContext.deadline) > companionAgentDialogueTimeout+time.Second {
		t.Fatalf("derived commit context err=%v deadline=%v hasDeadline=%v",
			commitContext.err, commitContext.deadline, commitContext.hasDeadline)
	}
	time.Sleep(50 * time.Millisecond)
	premature := fake.releases.Load() != 0 || fake.closed.Load() || store.hostTestStore.closeCount() != 0
	unblockCommit()
	shutdownErr := <-shutdownDone
	if premature {
		t.Fatalf("Release/close ran while derived commit was blocked: releases=%d agentClosed=%v storeCloses=%d",
			fake.releases.Load(), fake.closed.Load(), store.hostTestStore.closeCount())
	}
	if shutdownErr != nil {
		t.Fatalf("Shutdown: %v", shutdownErr)
	}
	saves := store.companionSaveSnapshot()
	latest := saves[len(saves)-1]
	if len(latest.Lifecycles) != 1 || latest.Lifecycles[0].MemoryRevision != 1 ||
		latest.Lifecycles[0].Summary != "关服前确认的 mirror" || fake.releases.Load() != 1 {
		t.Fatalf("latest=%+v releases=%d", latest, fake.releases.Load())
	}
}

func TestHostShutdownRetriesMemoryFinalizationAfterCallerTimeout(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	config.companionPlanner = nil
	fake := &shutdownAgentRuntime{
		store: store, acquired: make(chan struct{}),
		commitStarted: make(chan struct{}), commitRelease: make(chan struct{}),
		commitContext: make(chan shutdownCommitContext, 4),
	}
	var releaseCommit sync.Once
	unblockCommit := func() { releaseCommit.Do(func() { close(fake.commitRelease) }) }
	defer unblockCommit()
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

	issuer := stopTestIssuer(integrationIdentity(0x74, "发令者"))
	manager := host.world.companionManager
	host.world.stepMu.Lock()
	manager.onlinePlayers = nil
	slot := manager.slots[id]
	slot.currentIssuer = issuer
	slot.dialogueInFlight = true
	slot.dialogueAttempt = 10
	manager.dialogueResults <- dialogueOutcome{
		id: id, generation: slot.queue.Generation(), attempt: 10,
		node:   companion.DialogueNode{Kind: companion.DialogueNodeTerminal, State: companion.TaskCompleted},
		issuer: issuer, memoryEpoch: 1,
		result: companionDialogueResult{
			Generation: slot.queue.Generation(), MemoryEpoch: 1, Line: "超时后完成。",
			Proposal: &companion.AgentMemoryProposal{
				OperationID:  "77777777-7777-4777-8777-777777777777",
				BaseRevision: 0, Summary: "第二次关服确认的 mirror",
			},
		},
	}
	host.world.stepMu.Unlock()

	firstCtx, firstCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err = host.Shutdown(firstCtx)
	firstCancel()
	if !errors.Is(err, context.DeadlineExceeded) || fake.releases.Load() != 0 ||
		fake.closed.Load() || store.hostTestStore.closeCount() != 0 {
		t.Fatalf("first Shutdown err=%v releases=%d agentClosed=%v storeCloses=%d",
			err, fake.releases.Load(), fake.closed.Load(), store.hostTestStore.closeCount())
	}
	firstCommitContext := <-fake.commitContext
	if firstCommitContext.err != nil || !firstCommitContext.hasDeadline {
		t.Fatalf("first commit context=%+v", firstCommitContext)
	}
	workerDone := make(chan struct{})
	go func() {
		manager.waitGroup.Wait()
		close(workerDone)
	}()
	select {
	case <-workerDone:
	case <-time.After(waitDeadline):
		t.Fatal("finalization worker leaked after caller timeout")
	}

	unblockCommit()
	secondCtx, secondCancel := context.WithTimeout(context.Background(), waitDeadline)
	err = host.Shutdown(secondCtx)
	secondCancel()
	if err != nil {
		t.Fatalf("retry Shutdown: %v", err)
	}
	if fake.commits.Load() < 2 || fake.releases.Load() != 1 || !fake.closed.Load() ||
		store.hostTestStore.closeCount() != 1 {
		t.Fatalf("commits=%d releases=%d agentClosed=%v storeCloses=%d",
			fake.commits.Load(), fake.releases.Load(), fake.closed.Load(),
			store.hostTestStore.closeCount())
	}
	saves := store.companionSaveSnapshot()
	latest := saves[len(saves)-1]
	if len(latest.Lifecycles) != 1 || latest.Lifecycles[0].MemoryRevision != 1 ||
		latest.Lifecycles[0].Summary != "第二次关服确认的 mirror" {
		t.Fatalf("latest=%+v", latest)
	}
}

func TestHostShutdownRetriesFailedReleaseWithSameFrozenLease(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	config.companionPlanner = nil
	releaseErr := errors.New("release unavailable")
	fake := &shutdownAgentRuntime{
		store: store, acquired: make(chan struct{}), releaseErrors: []error{releaseErr},
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	err = host.Shutdown(ctx)
	cancel()
	if !errors.Is(err, companion.ErrAgentUnavailable) || fake.releases.Load() != 1 ||
		fake.closed.Load() || store.hostTestStore.closeCount() != 0 || host.companionRuntimeClosed {
		t.Fatalf("first shutdown err=%v releases=%d agentClosed=%v storeCloses=%d runtimeClosed=%v",
			err, fake.releases.Load(), fake.closed.Load(), store.hostTestStore.closeCount(),
			host.companionRuntimeClosed)
	}

	ctx, cancel = context.WithTimeout(context.Background(), waitDeadline)
	err = host.Shutdown(ctx)
	cancel()
	if err != nil {
		t.Fatalf("retry Shutdown: %v", err)
	}
	leases := fake.releaseLeaseSnapshot()
	if fake.releases.Load() != 2 || len(leases) != 2 || leases[0] == "" || leases[0] != leases[1] ||
		!fake.closed.Load() || store.hostTestStore.closeCount() != 1 || !host.companionRuntimeClosed {
		t.Fatalf("leases=%v releases=%d agentClosed=%v storeCloses=%d runtimeClosed=%v",
			leases, fake.releases.Load(), fake.closed.Load(), store.hostTestStore.closeCount(),
			host.companionRuntimeClosed)
	}
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
