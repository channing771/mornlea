package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	endpoint string
	cancel   context.CancelFunc
	command  *exec.Cmd
	done     chan error
	stderr   *bytes.Buffer
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
			handler, err := newCompanionMCPHandler(authority, current)
			if err != nil {
				return nil, err
			}
			return transcript.wrap(handler), nil
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
	python := os.Getenv("MORNLEA_COMPANION_AGENT_PYTHON")
	if python == "" {
		python = filepath.Join(repositoryRoot, "services", "companion-agent", ".venv", "bin", "python")
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
	if live, err := client.Live(ctx); err != nil || live.Status != "live" {
		t.Fatalf("live=%+v err=%v", live, err)
	}
	if ready, err := client.Ready(ctx); err != nil || ready.Status != "ready" {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}

	var acquire companion.AcquireRequest
	loadCrossLanguageHTTPGolden(t, "namespace acquire omits lease", &acquire)
	acquire.NamespaceID = testMCPNamespace
	grant, err := client.Acquire(ctx, acquire)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if grant.LeaseExpiresInMS != 15_000 || grant.LeaseID == "" {
		t.Fatalf("lease=%+v", grant)
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
	if planned.SnapshotID != plan.SnapshotID || planned.SnapshotDigest != plan.SnapshotDigest ||
		len(planned.Plan.Steps) != 1 || planned.Plan.Steps[0].Kind != "mine" {
		t.Fatalf("unexpected plan correlation or candidate: %+v", planned)
	}

	release := companion.LeaseRequest{
		ContractVersion: plan.ContractVersion, RequestID: plan.RequestID,
		ClientInstanceID: plan.ClientInstanceID, NamespaceID: plan.NamespaceID, LeaseID: plan.LeaseID,
	}
	released, err := client.Release(ctx, release)
	if err != nil || !released.Released {
		t.Fatalf("release=%+v err=%v", released, err)
	}
}

func startCrossLanguageAgentProcess(t *testing.T, repositoryRoot, credential string) *crossLanguageAgentProcess {
	t.Helper()
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
		"http_bearer_token": credential,
		"port":              port,
		"sqlite_path":       filepath.Join(t.TempDir(), "memory.sqlite3"),
	})
	if err != nil {
		_ = listenerFile.Close()
		_ = listener.Close()
		t.Fatal(err)
	}
	python := os.Getenv("MORNLEA_COMPANION_AGENT_PYTHON")
	if python == "" {
		python = filepath.Join(repositoryRoot, "services", "companion-agent", ".venv", "bin", "python")
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
	return &crossLanguageAgentProcess{
		endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		cancel:   cancel, command: command, done: done, stderr: stderr,
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

func crossLanguageRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve integration test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
