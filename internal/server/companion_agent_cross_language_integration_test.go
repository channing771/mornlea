package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
)

type crossLanguageMCPProbeRequest struct {
	Request companion.PlanRequest `json:"request"`
}

type crossLanguageAgentProcess struct {
	endpoint       string
	blockReadyPath string
	cancel         context.CancelFunc
	command        *exec.Cmd
	done           chan error
	stderr         *bytes.Buffer
}

type crossLanguageMCPProbeResult struct {
	ProtocolVersion       string   `json:"protocol_version"`
	ImplementationVersion string   `json:"implementation_version"`
	Tools                 []string `json:"tools"`
	Calls                 []string `json:"calls"`
}

type crossLanguageMCPTranscript struct {
	mu      sync.Mutex
	entries []string
}

type crossLanguageMCPStatusWriter struct {
	http.ResponseWriter
	status int
}

func (writer *crossLanguageMCPStatusWriter) WriteHeader(status int) {
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *crossLanguageMCPStatusWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	return writer.ResponseWriter.Write(body)
}

func (transcript *crossLanguageMCPTranscript) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		method := request.Method
		if request.Method == http.MethodPost {
			var envelope struct {
				Method string `json:"method"`
			}
			if json.Unmarshal(body, &envelope) == nil && envelope.Method != "" {
				method = envelope.Method
			}
		}
		tracked := &crossLanguageMCPStatusWriter{ResponseWriter: writer}
		next.ServeHTTP(tracked, request)
		transcript.mu.Lock()
		transcript.entries = append(transcript.entries, fmt.Sprintf("%s:%d:%s", method, tracked.status, writer.Header().Get("Content-Type")))
		transcript.mu.Unlock()
	})
}

func (transcript *crossLanguageMCPTranscript) String() string {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	return strings.Join(transcript.entries, ",")
}

func TestMCPAgentCrossLanguageIntegration(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	transcript := &crossLanguageMCPTranscript{}
	service, err := newCompanionMCPServiceWithDependencies(
		registry,
		net.Listen,
		func(authority string, current *companion.SnapshotRegistry) (http.Handler, error) {
			handler, err := newCompanionMCPSDKHandler()
			if err != nil {
				return nil, err
			}
			return newCompanionMCPOuterHandler(authority, current, transcript.wrap(handler)), nil
		},
	)
	if err != nil {
		t.Fatalf("start real MCP service: %v", err)
	}
	t.Cleanup(service.Close)

	request := companion.PlanRequest{
		ContractVersion:  "v1",
		RequestID:        "11111111-1111-4111-8111-111111111111",
		ClientInstanceID: "22222222-2222-4222-8222-222222222222",
		NamespaceID:      testMCPNamespace,
		LeaseID:          "44444444-4444-4444-8444-444444444444",
		RunID:            "55555555-5555-4555-8555-555555555555",
		CompanionID:      "66666666-6666-4666-8666-666666666666",
		Generation:       1,
		SnapshotID:       registration.SnapshotID,
		SnapshotDigest:   registration.Digest,
		DeadlineUnixMS:   time.Now().Add(15 * time.Second).UnixMilli(),
		MCPEndpoint:      service.Endpoint(),
		MCPCapability:    registration.Capability,
		Instruction:      "采一块石头",
	}
	payload, err := json.Marshal(crossLanguageMCPProbeRequest{Request: request})
	if err != nil {
		t.Fatal(err)
	}

	repositoryRoot := crossLanguageRepositoryRoot(t)
	python, ok := crossLanguagePythonPath(t, repositoryRoot)
	if !ok {
		t.Skipf("companion Agent Python 解释器不可用（%s）；真实进程合同由 make companion-agent-integration 运行", python)
	}
	helper := filepath.Join(repositoryRoot, "services", "companion-agent", "tests", "integration", "process.py")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, helper, "mcp-probe")
	command.Dir = filepath.Join(repositoryRoot, "services", "companion-agent")
	command.Stdin = bytes.NewReader(payload)
	command.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=*")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("real Python MCP probe failed: %v (stderr bytes=%d, status=%q, transcript=%s)",
			err, stderr.Len(), stdout.String(), transcript.String())
	}
	if ctx.Err() != nil {
		t.Fatalf("real Python MCP probe timeout: %v", ctx.Err())
	}

	var result crossLanguageMCPProbeResult
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode Python MCP probe result: %v", err)
	}
	wantTools := companion.PlanningToolNames()
	if result.ProtocolVersion != "2025-11-25" || result.ImplementationVersion != "v1" ||
		!reflect.DeepEqual(result.Tools, wantTools) {
		t.Fatalf("unexpected MCP contract summary: protocol=%q implementation=%q tools=%v",
			result.ProtocolVersion, result.ImplementationVersion, result.Tools)
	}
	wantCalls := []string{"get_planning_context", "query_terrain", "validate_plan"}
	if !reflect.DeepEqual(result.Calls, wantCalls) {
		t.Fatalf("tool calls=%v，want %v", result.Calls, wantCalls)
	}
	wantTranscript := strings.Join([]string{
		"initialize:200:application/json",
		"notifications/initialized:202:",
		"tools/list:200:application/json",
		"tools/call:200:application/json",
		"tools/call:200:application/json",
		"tools/call:200:application/json",
	}, ",")
	if got := transcript.String(); got != wantTranscript || strings.Contains(got, "ping") {
		t.Fatalf("MCP transcript=%q，want %q", got, wantTranscript)
	}
	assertCrossLanguageMCPOuterSurface(t, service.Endpoint(), registration.Capability)
	wantOuterTranscript := wantTranscript + ",initialize:200:application/json"
	if got := transcript.String(); got != wantOuterTranscript {
		t.Fatalf("outer gate rejection reached SDK dispatch: transcript=%q", got)
	}
}

func TestMCPAgentCrossLanguageCancellationIntegration(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	var barrierOnce sync.Once
	service, err := newCompanionMCPServiceWithDependencies(
		registry,
		net.Listen,
		func(authority string, current *companion.SnapshotRegistry) (http.Handler, error) {
			handler, err := newCompanionMCPSDKHandler()
			if err != nil {
				return nil, err
			}
			blockedSDK := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, readErr := io.ReadAll(request.Body)
				if readErr != nil {
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				request.Body = io.NopCloser(bytes.NewReader(body))
				if bytes.Contains(body, []byte(`"method":"tools/call"`)) {
					barrierOnce.Do(func() { close(entered) })
					<-release
					defer close(exited)
				}
				handler.ServeHTTP(writer, request)
			})
			return newCompanionMCPOuterHandler(authority, current, blockedSDK), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	request := companion.PlanRequest{
		ContractVersion: "v1", RequestID: "11111111-1111-4111-8111-111111111111",
		ClientInstanceID: "22222222-2222-4222-8222-222222222222", NamespaceID: testMCPNamespace,
		LeaseID: "44444444-4444-4444-8444-444444444444", RunID: "55555555-5555-4555-8555-555555555555",
		CompanionID: "66666666-6666-4666-8666-666666666666", Generation: 1,
		SnapshotID: registration.SnapshotID, SnapshotDigest: registration.Digest,
		DeadlineUnixMS: time.Now().Add(10 * time.Second).UnixMilli(), MCPEndpoint: service.Endpoint(),
		MCPCapability: registration.Capability, Instruction: "采一块石头",
	}
	payload, err := json.Marshal(crossLanguageMCPProbeRequest{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := crossLanguageRepositoryRoot(t)
	python, ok := crossLanguagePythonPath(t, repositoryRoot)
	if !ok {
		t.Skipf("companion Agent Python 解释器不可用（%s）；真实进程合同由 make companion-agent-integration 运行", python)
	}
	processContext, processCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer processCancel()
	command := exec.CommandContext(processContext, python,
		filepath.Join(repositoryRoot, "services", "companion-agent", "tests", "integration", "process.py"),
		"mcp-cancel-probe")
	command.Dir = filepath.Join(repositoryRoot, "services", "companion-agent")
	command.Stdin = bytes.NewReader(payload)
	command.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=*")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		processCancel()
		<-done
		t.Fatalf("Python MCP cancel probe did not reach tool barrier (stderr bytes=%d)", stderr.Len())
	}
	if !registry.Cancel(registration.SnapshotID) {
		t.Fatal("registry cancellation did not remove snapshot")
	}
	if _, err := registry.Lookup(registration.Capability); !errors.Is(err, companion.ErrSnapshotUnavailable) {
		t.Fatalf("lookup after registry cancellation err=%v", err)
	}
	select {
	case err := <-done:
		if err != nil || stdout.String() != `{"status":"cancelled"}`+"\n" {
			t.Fatalf("Python MCP cancel probe err=%v stderr bytes=%d status=%q", err, stderr.Len(), stdout.String())
		}
	case <-time.After(5 * time.Second):
		processCancel()
		<-done
		t.Fatal("Python MCP timeout did not converge")
	}
	closeStarted := time.Now()
	service.Close()
	if elapsed := time.Since(closeStarted); elapsed > 500*time.Millisecond {
		t.Fatalf("MCP Close waited for blocked handler: %s", elapsed)
	}
	close(release)
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("late MCP handler did not converge after release")
	}
}

func TestCompanionAgentHTTPProcessIntegration(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	mcpService, err := newCompanionMCPService(registry)
	if err != nil {
		t.Fatalf("start real MCP service: %v", err)
	}
	t.Cleanup(mcpService.Close)

	repositoryRoot := crossLanguageRepositoryRoot(t)
	const credential = "integration-http-token-do-not-log"
	agentProcess := startCrossLanguageAgentProcess(t, repositoryRoot, credential)
	t.Cleanup(agentProcess.close)

	client, err := companion.NewAgentClient(companion.AgentServiceSettings{
		Endpoint: agentProcess.endpoint, APIKeyEnv: "INTEGRATION_UNUSED",
	}, credential, nil)
	if err != nil {
		t.Fatalf("NewAgentClient: %v", err)
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var expectedLive companion.LiveResponse
	loadCrossLanguageHTTPGolden(t, "live probe has no application identity", &expectedLive)
	if live, err := client.Live(ctx); err != nil || live != expectedLive {
		t.Fatalf("live contract mismatch (status=%q err_type=%T)", live.Status, err)
	}
	var expectedReady companion.ReadyResponse
	loadCrossLanguageHTTPGolden(t, "ready probe has no application identity", &expectedReady)
	if ready, err := client.Ready(ctx); err != nil || ready != expectedReady {
		t.Fatalf("ready contract mismatch (status=%q err_type=%T)", ready.Status, err)
	}
	assertCrossLanguageHTTPErrorSurface(t, agentProcess.endpoint, credential)

	var acquire companion.AcquireRequest
	loadCrossLanguageHTTPGolden(t, "namespace acquire omits lease", &acquire)
	acquire.NamespaceID = testMCPNamespace
	grant, err := client.Acquire(ctx, acquire)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	var expectedGrant companion.AcquireResponse
	loadCrossLanguageHTTPGolden(t, "namespace acquire returns fencing lease", &expectedGrant)
	expectedGrant.NamespaceID = acquire.NamespaceID
	expectedGrant.LeaseID = grant.LeaseID
	if grant != expectedGrant || grant.LeaseID == "" {
		t.Fatalf("lease contract mismatch (ttl=%d lease_present=%t)", grant.LeaseExpiresInMS, grant.LeaseID != "")
	}
	var heartbeatRequest companion.LeaseRequest
	loadCrossLanguageHTTPGolden(t, "heartbeat carries lease only", &heartbeatRequest)
	heartbeatRequest.ClientInstanceID = acquire.ClientInstanceID
	heartbeatRequest.NamespaceID = acquire.NamespaceID
	heartbeatRequest.LeaseID = grant.LeaseID
	heartbeat, err := client.Heartbeat(ctx, heartbeatRequest)
	var expectedHeartbeat companion.HeartbeatResponse
	loadCrossLanguageHTTPGolden(t, "heartbeat echoes lease", &expectedHeartbeat)
	expectedHeartbeat.ClientInstanceID = heartbeatRequest.ClientInstanceID
	expectedHeartbeat.NamespaceID = heartbeatRequest.NamespaceID
	expectedHeartbeat.LeaseID = heartbeatRequest.LeaseID
	if err != nil || heartbeat != expectedHeartbeat {
		t.Fatalf("heartbeat contract mismatch (ttl=%d lease_match=%t err_type=%T)",
			heartbeat.LeaseExpiresInMS, heartbeat.LeaseID == grant.LeaseID, err)
	}
	conflict := acquire
	conflict.RequestID = "11111111-1111-4111-8111-111111111118"
	conflict.ClientInstanceID = "22222222-2222-4222-8222-222222222223"
	if _, err := client.Acquire(ctx, conflict); !crossLanguageAgentErrorIs(err, "namespace_conflict") {
		t.Fatalf("namespace conflict err=%v", err)
	}

	var plan companion.PlanRequest
	loadCrossLanguageHTTPGolden(t, "planner run carries snapshot identity", &plan)
	plan.NamespaceID = acquire.NamespaceID
	plan.ClientInstanceID = acquire.ClientInstanceID
	plan.LeaseID = grant.LeaseID
	plan.Generation = 1
	plan.SnapshotID = registration.SnapshotID
	plan.SnapshotDigest = registration.Digest
	plan.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	plan.MCPEndpoint = mcpService.Endpoint()
	plan.MCPCapability = registration.Capability
	plan.Instruction = "采一块石头"
	planned, err := client.Plan(ctx, plan)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	var expectedPlanned companion.PlanResponse
	loadCrossLanguageHTTPGolden(t, "planner response echoes all run and snapshot identity", &expectedPlanned)
	expectedPlanned.RequestID = plan.RequestID
	expectedPlanned.ClientInstanceID = plan.ClientInstanceID
	expectedPlanned.NamespaceID = plan.NamespaceID
	expectedPlanned.LeaseID = plan.LeaseID
	expectedPlanned.RunID = plan.RunID
	expectedPlanned.CompanionID = plan.CompanionID
	expectedPlanned.Generation = plan.Generation
	expectedPlanned.SnapshotID = plan.SnapshotID
	expectedPlanned.SnapshotDigest = plan.SnapshotDigest
	expectedPlanned.Plan = planned.Plan
	if !reflect.DeepEqual(planned, expectedPlanned) || len(planned.Plan.Steps) != 1 ||
		planned.Plan.Steps[0].Kind != "mine" {
		t.Fatalf("plan contract mismatch (snapshot_id_match=%t digest_match=%t steps=%d)",
			planned.SnapshotID == plan.SnapshotID, planned.SnapshotDigest == plan.SnapshotDigest, len(planned.Plan.Steps))
	}
	registry.Complete(registration.SnapshotID)

	blockingSnapshot := testMCPPlanSnapshot(t)
	blockingSnapshot.Command = "BLOCK_UNTIL_CANCEL"
	blockingRegistration, err := registry.Register(
		testMCPNamespace, blockingSnapshot.Companion.ID, 2, blockingSnapshot, time.Now().Add(15*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	blockingPlan := plan
	blockingPlan.RequestID = "11111111-1111-4111-8111-111111111112"
	blockingPlan.RunID = "55555555-5555-4555-8555-555555555556"
	blockingPlan.Generation = 2
	blockingPlan.SnapshotID = blockingRegistration.SnapshotID
	blockingPlan.SnapshotDigest = blockingRegistration.Digest
	blockingPlan.MCPCapability = blockingRegistration.Capability
	blockingPlan.Instruction = blockingSnapshot.Command
	blockingPlan.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	blockingResult := make(chan error, 1)
	go func() {
		_, planErr := client.Plan(ctx, blockingPlan)
		blockingResult <- planErr
	}()
	waitCrossLanguageBarrier(t, agentProcess.blockReadyPath, 5*time.Second)
	var cancelRequest companion.CancelRequest
	loadCrossLanguageHTTPGolden(t, "cancel carries run identity but no companion identity", &cancelRequest)
	cancelRequest.ClientInstanceID = blockingPlan.ClientInstanceID
	cancelRequest.NamespaceID = blockingPlan.NamespaceID
	cancelRequest.LeaseID = blockingPlan.LeaseID
	cancelRequest.RunID = blockingPlan.RunID
	cancelResponse, err := client.CancelRun(ctx, cancelRequest)
	var expectedCancel companion.CancelResponse
	loadCrossLanguageHTTPGolden(t, "cancel response echoes run identity", &expectedCancel)
	expectedCancel.ClientInstanceID = cancelRequest.ClientInstanceID
	expectedCancel.NamespaceID = cancelRequest.NamespaceID
	expectedCancel.LeaseID = cancelRequest.LeaseID
	expectedCancel.RunID = cancelRequest.RunID
	if err != nil || cancelResponse != expectedCancel {
		t.Fatalf("cancel contract mismatch (cancelled=%t err_type=%T)", cancelResponse.Cancelled, err)
	}
	select {
	case planErr := <-blockingResult:
		if planErr == nil {
			t.Fatal("cancelled plan returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled plan did not converge")
	}
	registry.Cancel(blockingRegistration.SnapshotID)
	_ = os.Remove(agentProcess.blockReadyPath)

	afterCancelSnapshot := testMCPPlanSnapshot(t)
	afterCancelRegistration, err := registry.Register(
		testMCPNamespace, afterCancelSnapshot.Companion.ID, 3, afterCancelSnapshot, time.Now().Add(15*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	afterCancelPlan := plan
	afterCancelPlan.RequestID = "11111111-1111-4111-8111-111111111114"
	afterCancelPlan.RunID = "55555555-5555-4555-8555-555555555557"
	afterCancelPlan.Generation = 3
	afterCancelPlan.SnapshotID = afterCancelRegistration.SnapshotID
	afterCancelPlan.SnapshotDigest = afterCancelRegistration.Digest
	afterCancelPlan.MCPCapability = afterCancelRegistration.Capability
	afterCancelPlan.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	if _, err := client.Plan(ctx, afterCancelPlan); err != nil {
		t.Fatalf("plan immediately after explicit cancel: %v", err)
	}
	registry.Complete(afterCancelRegistration.SnapshotID)

	timeoutSnapshot := testMCPPlanSnapshot(t)
	timeoutSnapshot.Command = "BLOCK_UNTIL_CANCEL"
	timeoutRegistration, err := registry.Register(
		testMCPNamespace, timeoutSnapshot.Companion.ID, 4, timeoutSnapshot, time.Now().Add(15*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	timeoutPlan := plan
	timeoutPlan.RequestID = "11111111-1111-4111-8111-111111111115"
	timeoutPlan.RunID = "55555555-5555-4555-8555-555555555558"
	timeoutPlan.Generation = 4
	timeoutPlan.SnapshotID = timeoutRegistration.SnapshotID
	timeoutPlan.SnapshotDigest = timeoutRegistration.Digest
	timeoutPlan.MCPCapability = timeoutRegistration.Capability
	timeoutPlan.Instruction = timeoutSnapshot.Command
	timeoutPlan.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	timeoutContext, timeoutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	timeoutResult := make(chan error, 1)
	go func() {
		_, planErr := client.Plan(timeoutContext, timeoutPlan)
		timeoutResult <- planErr
	}()
	waitCrossLanguageBarrier(t, agentProcess.blockReadyPath, time.Second)
	select {
	case planErr := <-timeoutResult:
		if planErr == nil {
			t.Fatal("caller-timeout plan returned success")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("caller-timeout plan did not return")
	}
	timeoutCancel()
	registry.Cancel(timeoutRegistration.SnapshotID)
	_ = os.Remove(agentProcess.blockReadyPath)

	afterTimeoutSnapshot := testMCPPlanSnapshot(t)
	afterTimeoutRegistration, err := registry.Register(
		testMCPNamespace, afterTimeoutSnapshot.Companion.ID, 5, afterTimeoutSnapshot, time.Now().Add(15*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	afterTimeoutPlan := plan
	afterTimeoutPlan.RequestID = "11111111-1111-4111-8111-111111111116"
	afterTimeoutPlan.RunID = "55555555-5555-4555-8555-555555555559"
	afterTimeoutPlan.Generation = 5
	afterTimeoutPlan.SnapshotID = afterTimeoutRegistration.SnapshotID
	afterTimeoutPlan.SnapshotDigest = afterTimeoutRegistration.Digest
	afterTimeoutPlan.MCPCapability = afterTimeoutRegistration.Capability
	afterTimeoutPlan.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	if _, err := client.Plan(ctx, afterTimeoutPlan); err != nil {
		t.Fatalf("plan immediately after caller timeout: %v", err)
	}
	registry.Complete(afterTimeoutRegistration.SnapshotID)

	var active companion.MemoryReconcileRequest
	loadCrossLanguageHTTPGolden(t, "active memory reconcile carries mirror", &active)
	active.ClientInstanceID = acquire.ClientInstanceID
	active.NamespaceID = acquire.NamespaceID
	active.LeaseID = grant.LeaseID
	reconciled, err := client.ReconcileMemory(ctx, active)
	var expectedReconciled companion.MemoryReconcileResponse
	loadCrossLanguageHTTPGolden(t, "active reconcile response returns runtime memory", &expectedReconciled)
	expectedReconciled.ClientInstanceID = active.ClientInstanceID
	expectedReconciled.NamespaceID = active.NamespaceID
	expectedReconciled.LeaseID = active.LeaseID
	if err != nil || !reflect.DeepEqual(reconciled, expectedReconciled) {
		t.Fatalf("active reconcile contract mismatch (active=%t memory_present=%t err_type=%T)",
			reconciled.Active, reconciled.Memory != nil, err)
	}

	var nonterminal companion.AgentDialogueRequest
	loadCrossLanguageHTTPGolden(t, "nonterminal dialogue run", &nonterminal)
	nonterminal.ClientInstanceID = acquire.ClientInstanceID
	nonterminal.NamespaceID = acquire.NamespaceID
	nonterminal.LeaseID = grant.LeaseID
	nonterminal.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	spoken, err := client.Dialogue(ctx, nonterminal)
	var expectedSpoken companion.AgentDialogueResponse
	loadCrossLanguageHTTPGolden(t, "nonterminal dialogue response has no proposal", &expectedSpoken)
	expectedSpoken.RequestID = nonterminal.RequestID
	expectedSpoken.ClientInstanceID = nonterminal.ClientInstanceID
	expectedSpoken.NamespaceID = nonterminal.NamespaceID
	expectedSpoken.LeaseID = nonterminal.LeaseID
	expectedSpoken.RunID = nonterminal.RunID
	expectedSpoken.CompanionID = nonterminal.CompanionID
	expectedSpoken.Generation = nonterminal.Generation
	expectedSpoken.MemoryEpoch = nonterminal.MemoryEpoch
	if err != nil || !reflect.DeepEqual(spoken, expectedSpoken) {
		t.Fatalf("nonterminal dialogue contract mismatch (proposal_present=%t err_type=%T)",
			spoken.MemoryProposal != nil, err)
	}

	var terminal companion.AgentDialogueRequest
	loadCrossLanguageHTTPGolden(t, "terminal dialogue request carries completed fact", &terminal)
	terminal.ClientInstanceID = acquire.ClientInstanceID
	terminal.NamespaceID = acquire.NamespaceID
	terminal.LeaseID = grant.LeaseID
	terminal.DeadlineUnixMS = time.Now().Add(15 * time.Second).UnixMilli()
	proposed, err := client.Dialogue(ctx, terminal)
	var expectedProposed companion.AgentDialogueResponse
	loadCrossLanguageHTTPGolden(t, "terminal dialogue response proposes memory without committing", &expectedProposed)
	expectedProposed.RequestID = terminal.RequestID
	expectedProposed.ClientInstanceID = terminal.ClientInstanceID
	expectedProposed.NamespaceID = terminal.NamespaceID
	expectedProposed.LeaseID = terminal.LeaseID
	expectedProposed.RunID = terminal.RunID
	expectedProposed.CompanionID = terminal.CompanionID
	expectedProposed.Generation = terminal.Generation
	expectedProposed.MemoryEpoch = terminal.MemoryEpoch
	if proposed.MemoryProposal != nil && expectedProposed.MemoryProposal != nil {
		expectedProposed.MemoryProposal.OperationID = proposed.MemoryProposal.OperationID
	}
	if err != nil || proposed.MemoryProposal == nil || !reflect.DeepEqual(proposed, expectedProposed) {
		t.Fatalf("terminal dialogue contract mismatch (proposal_present=%t err_type=%T)",
			proposed.MemoryProposal != nil, err)
	}
	beforeCommit, err := client.ReconcileMemory(ctx, active)
	if err != nil || beforeCommit.Memory == nil || beforeCommit.Memory.Revision != 3 {
		t.Fatalf("proposal commit boundary mismatch (memory_present=%t revision_is_three=%t err_type=%T)",
			beforeCommit.Memory != nil, beforeCommit.Memory != nil && beforeCommit.Memory.Revision == 3, err)
	}

	proposal := proposed.MemoryProposal
	var commit companion.MemoryCommitRequest
	loadCrossLanguageHTTPGolden(t, "memory commit carries CAS identity", &commit)
	commit.ClientInstanceID = terminal.ClientInstanceID
	commit.NamespaceID = terminal.NamespaceID
	commit.LeaseID = terminal.LeaseID
	commit.CompanionID = terminal.CompanionID
	commit.MemoryEpoch = terminal.MemoryEpoch
	commit.BaseRevision = proposal.BaseRevision
	commit.OperationID = proposal.OperationID
	commit.Summary = proposal.Summary
	committed, err := client.CommitMemory(ctx, commit)
	var expectedCommitted companion.MemoryCommitResponse
	loadCrossLanguageHTTPGolden(t, "memory commit response echoes operation and revision", &expectedCommitted)
	expectedCommitted.ClientInstanceID = commit.ClientInstanceID
	expectedCommitted.NamespaceID = commit.NamespaceID
	expectedCommitted.LeaseID = commit.LeaseID
	expectedCommitted.CompanionID = commit.CompanionID
	expectedCommitted.MemoryEpoch = commit.MemoryEpoch
	expectedCommitted.OperationID = commit.OperationID
	if err != nil || committed != expectedCommitted {
		t.Fatalf("commit contract mismatch (revision=%d err_type=%T)", committed.CommittedRevision, err)
	}
	replayed, err := client.CommitMemory(ctx, commit)
	if err != nil || replayed.CommittedRevision != committed.CommittedRevision ||
		replayed.OperationID != committed.OperationID {
		t.Fatalf("commit replay mismatch (revision_match=%t operation_match=%t err_type=%T)",
			replayed.CommittedRevision == committed.CommittedRevision,
			replayed.OperationID == committed.OperationID, err)
	}
	active.Mirror = &companion.AgentMemoryState{
		Revision: committed.CommittedRevision, OperationID: &proposal.OperationID, Summary: proposal.Summary,
	}
	afterCommit, err := client.ReconcileMemory(ctx, active)
	if err != nil || afterCommit.Memory == nil || afterCommit.Memory.Revision != 4 ||
		afterCommit.Memory.OperationID == nil || *afterCommit.Memory.OperationID != proposal.OperationID {
		t.Fatalf("committed reconcile mismatch (memory_present=%t operation_present=%t err_type=%T)",
			afterCommit.Memory != nil, afterCommit.Memory != nil && afterCommit.Memory.OperationID != nil, err)
	}

	var deletedRequest companion.MemoryDeleteRequest
	loadCrossLanguageHTTPGolden(t, "memory delete advances epoch", &deletedRequest)
	deletedRequest.ClientInstanceID = acquire.ClientInstanceID
	deletedRequest.NamespaceID = acquire.NamespaceID
	deletedRequest.LeaseID = grant.LeaseID
	deleted, err := client.DeleteMemory(ctx, deletedRequest)
	var expectedDeleted companion.MemoryDeleteResponse
	loadCrossLanguageHTTPGolden(t, "memory delete response returns current epoch", &expectedDeleted)
	expectedDeleted.ClientInstanceID = deletedRequest.ClientInstanceID
	expectedDeleted.NamespaceID = deletedRequest.NamespaceID
	expectedDeleted.LeaseID = deletedRequest.LeaseID
	if err != nil || deleted != expectedDeleted {
		t.Fatalf("delete contract mismatch (epoch_match=%t err_type=%T)",
			deleted.MemoryEpoch == deletedRequest.NewMemoryEpoch, err)
	}
	var inactive companion.MemoryReconcileRequest
	loadCrossLanguageHTTPGolden(t, "inactive memory reconcile carries tombstone only", &inactive)
	inactive.ClientInstanceID = acquire.ClientInstanceID
	inactive.NamespaceID = acquire.NamespaceID
	inactive.LeaseID = grant.LeaseID
	inactiveState, err := client.ReconcileMemory(ctx, inactive)
	var expectedInactive companion.MemoryReconcileResponse
	loadCrossLanguageHTTPGolden(t, "inactive reconcile response returns tombstone only", &expectedInactive)
	expectedInactive.ClientInstanceID = inactive.ClientInstanceID
	expectedInactive.NamespaceID = inactive.NamespaceID
	expectedInactive.LeaseID = inactive.LeaseID
	if err != nil || !reflect.DeepEqual(inactiveState, expectedInactive) {
		t.Fatalf("inactive reconcile mismatch (active=%t memory_present=%t tombstone_present=%t err_type=%T)",
			inactiveState.Active, inactiveState.Memory != nil, inactiveState.TombstoneOperationID != nil, err)
	}

	activeZero := companion.MemoryReconcileRequest{
		ContractVersion: terminal.ContractVersion, RequestID: terminal.RequestID,
		ClientInstanceID: terminal.ClientInstanceID, NamespaceID: terminal.NamespaceID,
		LeaseID: terminal.LeaseID, CompanionID: terminal.CompanionID,
		MemoryEpoch: 9, Active: true, Mirror: &companion.AgentMemoryState{},
	}
	zeroState, err := client.ReconcileMemory(ctx, activeZero)
	var zeroRevision uint64
	if zeroState.Memory != nil {
		zeroRevision = zeroState.Memory.Revision
	}
	if err != nil || zeroState.Memory == nil || zeroState.Memory.Revision != 0 ||
		zeroState.Memory.OperationID != nil || zeroState.Memory.Summary != "" {
		t.Fatalf("canonical-zero reconcile mismatch (memory_present=%t revision=%d operation_present=%t err_type=%T)",
			zeroState.Memory != nil, zeroRevision, zeroState.Memory != nil && zeroState.Memory.OperationID != nil, err)
	}
	finalTombstone := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	finalDelete := companion.MemoryDeleteRequest{
		ContractVersion: terminal.ContractVersion, RequestID: terminal.RequestID,
		ClientInstanceID: terminal.ClientInstanceID, NamespaceID: terminal.NamespaceID,
		LeaseID: terminal.LeaseID, CompanionID: terminal.CompanionID,
		OldMemoryEpoch: 9, NewMemoryEpoch: 10, TombstoneOperationID: finalTombstone,
	}
	if _, err := client.DeleteMemory(ctx, finalDelete); err != nil {
		t.Fatalf("final delete: %v", err)
	}
	finalInactive := companion.MemoryReconcileRequest{
		ContractVersion: terminal.ContractVersion, RequestID: terminal.RequestID,
		ClientInstanceID: terminal.ClientInstanceID, NamespaceID: terminal.NamespaceID,
		LeaseID: terminal.LeaseID, CompanionID: terminal.CompanionID,
		MemoryEpoch: 10, Active: false, TombstoneOperationID: &finalTombstone,
	}
	finalState, err := client.ReconcileMemory(ctx, finalInactive)
	if err != nil || finalState.Active || finalState.Memory != nil {
		t.Fatalf("final inactive reconcile mismatch (active=%t memory_present=%t err_type=%T)",
			finalState.Active, finalState.Memory != nil, err)
	}

	var release companion.LeaseRequest
	loadCrossLanguageHTTPGolden(t, "heartbeat carries lease only", &release)
	release.ClientInstanceID = plan.ClientInstanceID
	release.NamespaceID = plan.NamespaceID
	release.LeaseID = plan.LeaseID
	released, err := client.Release(ctx, release)
	var expectedReleased companion.ReleaseResponse
	loadCrossLanguageHTTPGolden(t, "release echoes lease", &expectedReleased)
	expectedReleased.ClientInstanceID = release.ClientInstanceID
	expectedReleased.NamespaceID = release.NamespaceID
	expectedReleased.LeaseID = release.LeaseID
	if err != nil || released != expectedReleased {
		t.Fatalf("release contract mismatch (released=%t err_type=%T)", released.Released, err)
	}
	if _, err := client.Heartbeat(ctx, heartbeatRequest); !crossLanguageAgentErrorIs(err, "not_found") {
		t.Fatalf("stale heartbeat err=%v", err)
	}
}

func startCrossLanguageAgentProcess(t *testing.T, repositoryRoot, credential string) *crossLanguageAgentProcess {
	t.Helper()
	// skip 必须先于 listener 创建，避免 Python 不可用时泄漏已监听的端口。
	python, ok := crossLanguagePythonPath(t, repositoryRoot)
	if !ok {
		t.Skipf("companion Agent Python 解释器不可用（%s）；真实进程合同由 make companion-agent-integration 运行", python)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		_ = listener.Close()
		t.Fatal("integration listener is not TCP")
	}
	listenerFile, err := tcpListener.File()
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	control, err := json.Marshal(map[string]any{
		"block_ready_path":  filepath.Join(t.TempDir(), "model-ready"),
		"http_bearer_token": credential,
		"port":              port,
		"sqlite_path":       filepath.Join(t.TempDir(), "memory.sqlite3"),
	})
	if err != nil {
		_ = listenerFile.Close()
		_ = listener.Close()
		t.Fatal(err)
	}
	helper := filepath.Join(repositoryRoot, "services", "companion-agent", "tests", "integration", "process.py")
	processContext, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(processContext, python, helper, "http-server")
	command.Dir = filepath.Join(repositoryRoot, "services", "companion-agent")
	command.Stdin = bytes.NewReader(control)
	command.ExtraFiles = []*os.File{listenerFile}
	command.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=*")
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		_ = listenerFile.Close()
		_ = listener.Close()
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		cancel()
		_ = listenerFile.Close()
		_ = listener.Close()
		t.Fatalf("start real Python Agent process: %v", err)
	}
	_ = listenerFile.Close()
	_ = listener.Close()
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
			return
		}
		ready <- ""
	}()
	select {
	case line := <-ready:
		if line != `{"status":"ready"}` {
			cancel()
			<-done
			t.Fatalf("real Python Agent readiness failed (stderr bytes=%d, status=%q)", stderr.Len(), line)
		}
	case <-time.After(15 * time.Second):
		cancel()
		<-done
		t.Fatalf("real Python Agent readiness timeout (stderr bytes=%d)", stderr.Len())
	}
	var controlValue struct {
		BlockReadyPath string `json:"block_ready_path"`
	}
	if err := json.Unmarshal(control, &controlValue); err != nil {
		t.Fatal(err)
	}
	return &crossLanguageAgentProcess{
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", port), blockReadyPath: controlValue.BlockReadyPath,
		cancel: cancel, command: command, done: done, stderr: stderr,
	}
}

func (process *crossLanguageAgentProcess) close() {
	if process == nil {
		return
	}
	process.cancel()
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		_ = process.command.Process.Kill()
		<-process.done
	}
}

func waitCrossLanguageBarrier(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("integration model barrier was not reached within %s", timeout)
}

func crossLanguageAgentErrorIs(err error, code string) bool {
	var agentError *companion.AgentError
	return errors.As(err, &agentError) && agentError.Code == code
}

func assertCrossLanguageHTTPErrorSurface(t *testing.T, endpoint, credential string) {
	t.Helper()
	var valid map[string]any
	loadCrossLanguageHTTPGolden(t, "namespace acquire omits lease", &valid)
	validBody, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	requestID, ok := valid["request_id"].(string)
	if !ok || requestID == "" {
		t.Fatal("HTTP golden request identity is missing")
	}
	unknown := make(map[string]any, len(valid)+1)
	for key, value := range valid {
		unknown[key] = value
	}
	unknown["unknown"] = "sensitive-instruction-must-not-return"
	unknownBody, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := make(map[string]any, len(valid))
	for key, value := range valid {
		unsupported[key] = value
	}
	unsupported["contract_version"] = "v2"
	unsupportedBody, err := json.Marshal(unsupported)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		body        []byte
		token       string
		host        string
		contentType string
		status      int
		code        string
		echoRequest bool
	}{
		{name: "unauthorized", body: validBody, token: "wrong-token", status: http.StatusUnauthorized, code: "unauthorized"},
		{name: "unsupported version", body: unsupportedBody, token: credential, status: http.StatusUpgradeRequired, code: "unsupported_version", echoRequest: true},
		{name: "unknown field", body: unknownBody, token: credential, status: http.StatusBadRequest, code: "invalid_request", echoRequest: true},
		{name: "trailing body", body: append(append([]byte(nil), validBody...), []byte(" trailing")...), token: credential, status: http.StatusBadRequest, code: "invalid_request"},
		{name: "oversize", body: bytes.Repeat([]byte(" "), (256<<10)+1), token: credential, status: http.StatusBadRequest, code: "invalid_request"},
		{name: "wrong content type", body: validBody, token: credential, contentType: "text/plain", status: http.StatusBadRequest, code: "invalid_request"},
		{name: "wrong host", body: validBody, token: credential, host: "127.0.0.1:1", status: http.StatusUnauthorized, code: "unauthorized"},
	}
	transport := &http.Transport{DisableKeepAlives: true, DisableCompression: true, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, endpoint+"/v1/namespaces/acquire", bytes.NewReader(testCase.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+testCase.token)
			contentType := testCase.contentType
			if contentType == "" {
				contentType = "application/json"
			}
			request.Header.Set("Content-Type", contentType)
			if testCase.host != "" {
				request.Host = testCase.host
			}
			var response *http.Response
			if testCase.name == "oversize" {
				connection, dialErr := net.DialTimeout("tcp4", request.URL.Host, 2*time.Second)
				if dialErr != nil {
					t.Fatal(dialErr)
				}
				defer connection.Close()
				_, writeErr := fmt.Fprintf(connection,
					"POST %s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nExpect: 100-continue\r\nConnection: close\r\n\r\n",
					request.URL.RequestURI(), request.URL.Host, testCase.token, len(testCase.body))
				if writeErr != nil {
					t.Fatal(writeErr)
				}
				response, err = http.ReadResponse(bufio.NewReader(connection), request)
			} else {
				response, err = client.Do(request)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				RequestID *string `json:"request_id"`
				Error     struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			decodeErr := json.Unmarshal(body, &envelope)
			correlationMatches := (!testCase.echoRequest && envelope.RequestID == nil) ||
				(testCase.echoRequest && envelope.RequestID != nil && *envelope.RequestID == requestID)
			if response.StatusCode != testCase.status || len(body) > 64<<10 ||
				decodeErr != nil || envelope.Error.Code != testCase.code || !correlationMatches {
				t.Fatalf("status=%d bytes=%d code=%q", response.StatusCode, len(body), envelope.Error.Code)
			}
			text := string(body)
			if strings.Contains(text, credential) || strings.Contains(text, "sensitive-instruction") ||
				response.Header.Get("Location") != "" {
				t.Fatalf("error response leaked sensitive input or redirect")
			}
		})
	}
}

func assertCrossLanguageMCPOuterSurface(t *testing.T, endpoint, capability string) {
	t.Helper()
	initialize := []byte(`{"jsonrpc":"2.0","id":90,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"integration","version":"v1"}}}`)
	validOrigin, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(initialize))
	if err != nil {
		t.Fatal(err)
	}
	validOrigin.Header.Set("Authorization", "Bearer "+capability)
	validOrigin.Header.Set("Content-Type", "application/json")
	validOrigin.Header.Set("Accept", "application/json, text/event-stream")
	validOrigin.Header.Set("Origin", strings.TrimSuffix(endpoint, "/mcp"))
	transport := &http.Transport{DisableKeepAlives: true, DisableCompression: true, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(validOrigin)
	if err != nil {
		t.Fatal(err)
	}
	validBody, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK ||
		strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") ||
		response.Header.Get("Mcp-Session-Id") != "" || len(validBody) > 160<<10 {
		t.Fatalf("exact Origin initialization status=%d bytes=%d err=%v", response.StatusCode, len(validBody), readErr)
	}

	toolsList := []byte(`{"jsonrpc":"2.0","id":91,"method":"tools/list","params":{}}`)
	tests := []struct {
		name        string
		method      string
		body        []byte
		capability  string
		protocol    string
		contentType string
		host        string
		origin      string
	}{
		{name: "GET", method: http.MethodGet, body: initialize, capability: capability},
		{name: "batch", method: http.MethodPost, body: []byte("[" + string(initialize) + "]"), capability: capability},
		{name: "ping", method: http.MethodPost, body: []byte(`{"jsonrpc":"2.0","id":92,"method":"ping","params":{}}`), capability: capability, protocol: "2025-11-25"},
		{name: "subscription", method: http.MethodPost, body: []byte(`{"jsonrpc":"2.0","id":93,"method":"subscriptions/listen","params":{}}`), capability: capability, protocol: "2025-11-25"},
		{name: "unknown method", method: http.MethodPost, body: []byte(`{"jsonrpc":"2.0","id":94,"method":"resources/list","params":{}}`), capability: capability, protocol: "2025-11-25"},
		{name: "missing protocol", method: http.MethodPost, body: toolsList, capability: capability},
		{name: "wrong protocol", method: http.MethodPost, body: toolsList, capability: capability, protocol: "2025-06-18"},
		{name: "wrong content type", method: http.MethodPost, body: toolsList, capability: capability, protocol: "2025-11-25", contentType: "text/plain"},
		{name: "wrong host", method: http.MethodPost, body: toolsList, capability: capability, protocol: "2025-11-25", host: "127.0.0.1:1"},
		{name: "wrong origin", method: http.MethodPost, body: toolsList, capability: capability, protocol: "2025-11-25", origin: "http://127.0.0.1:1"},
		{name: "missing bearer", method: http.MethodPost, body: toolsList, protocol: "2025-11-25"},
		{name: "wrong bearer", method: http.MethodPost, body: toolsList, capability: "wrong-capability", protocol: "2025-11-25"},
		{name: "oversize", method: http.MethodPost, body: bytes.Repeat([]byte("x"), (256<<10)+1), capability: capability, protocol: "2025-11-25"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request, err := http.NewRequest(testCase.method, endpoint, bytes.NewReader(testCase.body))
			if err != nil {
				t.Fatal(err)
			}
			if testCase.capability != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.capability)
			}
			contentType := testCase.contentType
			if contentType == "" {
				contentType = "application/json"
			}
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Accept", "application/json, text/event-stream")
			if testCase.name == "oversize" {
				request.Header.Set("Expect", "100-continue")
			}
			if testCase.protocol != "" {
				request.Header.Set("Mcp-Protocol-Version", testCase.protocol)
			}
			if testCase.host != "" {
				request.Host = testCase.host
			}
			if testCase.origin != "" {
				request.Header.Set("Origin", testCase.origin)
			}
			var response *http.Response
			if testCase.name == "oversize" {
				connection, dialErr := net.DialTimeout("tcp4", request.URL.Host, 2*time.Second)
				if dialErr != nil {
					t.Fatal(dialErr)
				}
				defer connection.Close()
				_, writeErr := fmt.Fprintf(connection,
					"POST %s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nMcp-Protocol-Version: 2025-11-25\r\nContent-Length: %d\r\nExpect: 100-continue\r\nConnection: close\r\n\r\n",
					request.URL.RequestURI(), request.URL.Host, capability, len(testCase.body))
				if writeErr != nil {
					t.Fatal(writeErr)
				}
				response, err = http.ReadResponse(bufio.NewReader(connection), request)
			} else {
				response, err = client.Do(request)
			}
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(io.LimitReader(response.Body, (160<<10)+1))
			_ = response.Body.Close()
			if readErr != nil || response.StatusCode < 400 || len(body) > 160<<10 ||
				strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") ||
				response.Header.Get("Mcp-Session-Id") != "" || response.Header.Get("Location") != "" {
				t.Fatalf("status=%d bytes=%d err=%v", response.StatusCode, len(body), readErr)
			}
			var envelope map[string]any
			if json.Unmarshal(body, &envelope) != nil || envelope["error"] == nil ||
				strings.Contains(string(body), capability) || strings.Contains(string(body), registrationLeakSentinel) {
				t.Fatalf("MCP rejection is not stable bounded JSON")
			}
		})
	}
}

const registrationLeakSentinel = "sensitive-instruction-must-not-return"

func loadCrossLanguageHTTPGolden(t *testing.T, name string, target any) {
	t.Helper()
	root := crossLanguageRepositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "contracts", "companion-agent", "http-v1", "golden", "valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Cases []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range document.Cases {
		if testCase.Name == name {
			if err := json.Unmarshal(testCase.Value, target); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("HTTP golden %q not found", name)
}

// crossLanguagePythonPath 解析伙伴 Agent Python 解释器路径：优先取
// `MORNLEA_COMPANION_AGENT_PYTHON`，为空则回落到仓库内 uv 管理的
// `services/companion-agent/.venv/bin/python`。用 `os.Stat` 判定最终路径
// 存在且是常规文件，缺失或不可用返回 (path, false)。真实进程合同由
// `make companion-agent-integration`（CI integration job）承载；普通 race
// 分片没有 Python 环境，此时测试跳过而非失败——跳过不削弱 CI 覆盖，
// 因为 integration job 会真实运行同一批 Go↔Python 进程合同。
func crossLanguagePythonPath(t *testing.T, repositoryRoot string) (string, bool) {
	t.Helper()
	python := os.Getenv("MORNLEA_COMPANION_AGENT_PYTHON")
	if python == "" {
		python = filepath.Join(repositoryRoot, "services", "companion-agent", ".venv", "bin", "python")
	}
	info, err := os.Stat(python)
	if err != nil || !info.Mode().IsRegular() {
		return python, false
	}
	return python, true
}

func crossLanguageRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve integration test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
