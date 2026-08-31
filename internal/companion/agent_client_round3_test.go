package companion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentClientRejectsExplicitNullForEveryNonNullableShape(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	testCases := []struct {
		name      string
		operation string
		variant   string
		mutate    func(map[string]any)
		want      error
	}{
		{name: "identity string", operation: "namespace_acquire", variant: "success", mutate: func(object map[string]any) { object["request_id"] = nil }},
		{name: "nested object", operation: "plan", variant: "success", mutate: func(object map[string]any) { object["plan"] = nil }, want: ErrAgentInvalidModelOutput},
		{name: "number", operation: "plan", variant: "success", mutate: func(object map[string]any) { object["generation"] = nil }, want: ErrAgentInvalidModelOutput},
		{name: "position x", operation: "plan", variant: "success", mutate: func(object map[string]any) { firstAgentPlanStep(t, object)["x"] = nil }, want: ErrAgentInvalidModelOutput},
		{name: "position y", operation: "plan", variant: "success", mutate: func(object map[string]any) { firstAgentPlanStep(t, object)["y"] = nil }, want: ErrAgentInvalidModelOutput},
		{name: "position z", operation: "plan", variant: "success", mutate: func(object map[string]any) { firstAgentPlanStep(t, object)["z"] = nil }, want: ErrAgentInvalidModelOutput},
		{name: "base revision", operation: "dialogue", variant: "terminal-true", mutate: func(object map[string]any) { agentNestedObject(t, object, "memory_proposal")["base_revision"] = nil }},
		{name: "line string", operation: "dialogue", variant: "terminal-true", mutate: func(object map[string]any) { object["line"] = nil }},
		{name: "revision", operation: "memory_reconcile", variant: "active-true", mutate: func(object map[string]any) { agentNestedObject(t, object, "memory")["revision"] = nil }},
		{name: "summary", operation: "memory_reconcile", variant: "active-true", mutate: func(object map[string]any) { agentNestedObject(t, object, "memory")["summary"] = nil }},
		{name: "active bool", operation: "memory_reconcile", variant: "active-true", mutate: func(object map[string]any) { object["active"] = nil }},
		{name: "cancelled bool", operation: "run_cancel", variant: "success", mutate: func(object map[string]any) { object["cancelled"] = nil }},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			success := agentManifestSuccess(t, manifest, goldens, testCase.operation, testCase.variant)
			responseBody := mutateAgentJSON(t, success.ResponseBody, testCase.mutate)
			client := newManifestAgentClient(t, fixedAgentResponseHandler(success.Status, responseBody))
			got, err := callAgentOperation(t, client, testCase.operation, success.RequestBody)
			assertZeroAgentResult(t, got)
			want := testCase.want
			if want == nil {
				want = ErrAgentUnavailable
			}
			if !errors.Is(err, want) {
				t.Fatalf("explicit null error = %v", err)
			}
		})
	}
}

func TestAgentClientAllowsOnlySchemaNullableFields(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	success := agentManifestSuccess(t, manifest, goldens, "memory_reconcile", "active-true")
	requestBody := mutateAgentJSON(t, success.RequestBody, func(object map[string]any) {
		mirror := agentNestedObject(t, object, "mirror")
		mirror["revision"] = json.Number("0")
		mirror["operation_id"] = nil
		mirror["summary"] = ""
	})
	responseBody := mutateAgentJSON(t, success.ResponseBody, func(object map[string]any) {
		memory := agentNestedObject(t, object, "memory")
		memory["revision"] = json.Number("0")
		memory["operation_id"] = nil
		memory["summary"] = ""
	})
	client := newManifestAgentClient(t, fixedAgentResponseHandler(success.Status, responseBody))
	got, err := callAgentOperation(t, client, "memory_reconcile", requestBody)
	if err != nil {
		t.Fatalf("nullable memory state: %v", err)
	}
	response, ok := got.(MemoryReconcileResponse)
	if !ok || response.Memory == nil || response.Memory.OperationID != nil || response.Memory.Revision != 0 {
		t.Fatalf("nullable memory state = %#v", got)
	}
}

func TestAgentClientOutboundHeaderBudgetMatchesRawWire(t *testing.T) {
	endpoint, observed := startRawAgentHeaderServer(t)
	requestBody, err := json.Marshal(validAgentAcquireRequest())
	if err != nil {
		t.Fatal(err)
	}
	baseHeaders := [][2]string{
		{"Host", strings.TrimPrefix(endpoint, "http://")},
		{"User-Agent", "mornlea-companion-agent-http-v1"},
		{"Authorization", "Bearer "},
		{"Content-Type", "application/json"},
		{"Accept-Encoding", "identity"},
		{"Connection", "close"},
		{"Content-Length", strconv.Itoa(len(requestBody))},
	}
	baseBudget := independentAgentHeaderBudget(baseHeaders)
	secretLength := AgentMaxHeaderBytes - baseBudget
	if secretLength <= 0 {
		t.Fatalf("raw header fixture base budget = %d", baseBudget)
	}

	exact, err := NewAgentClient(AgentServiceSettings{Endpoint: endpoint, APIKeyEnv: "RAW_HEADER_KEY"}, strings.Repeat("s", secretLength), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(exact.Close)
	got, err := exact.Acquire(context.Background(), validAgentAcquireRequest())
	if err != nil || got.LeaseID != agentLeaseID {
		t.Fatalf("exact raw header request = %+v, %v", got, err)
	}
	select {
	case raw := <-observed:
		if raw.budget != AgentMaxHeaderBytes {
			t.Fatalf("raw header budget = %d, want %d; line count=%d", raw.budget, AgentMaxHeaderBytes, len(raw.lines))
		}
		expected := append([][2]string(nil), baseHeaders...)
		expected[2][1] += strings.Repeat("s", secretLength)
		assertRawAgentHeaders(t, raw.lines, expected)
	case <-time.After(2 * time.Second):
		t.Fatal("raw server did not observe exact request")
	}

	oversized, err := NewAgentClient(AgentServiceSettings{Endpoint: endpoint, APIKeyEnv: "RAW_HEADER_KEY"}, strings.Repeat("s", secretLength+1), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oversized.Close)
	got, err = oversized.Acquire(context.Background(), validAgentAcquireRequest())
	if got != (AcquireResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("oversized raw header request = %+v, %v", got, err)
	}
	select {
	case raw := <-observed:
		t.Fatalf("oversized request touched network with %d header bytes", raw.budget)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAgentClientRejectsDuplicateJSONContentType(t *testing.T) {
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live"}`))
	}))
	got, err := client.Live(context.Background())
	if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("duplicate Content-Type = %+v, %v", got, err)
	}
}

func TestAgentClientCallerCancellationAfterResponseHeadersReturnsContextError(t *testing.T) {
	flushed := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(flushed)
		<-release
	}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		response LiveResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Live(ctx)
		done <- struct {
			response LiveResponse
			err      error
		}{response: response, err: err}
	}()
	waitAgentSignal(t, flushed)
	cancel()
	result := waitAgentLiveResult(t, done)
	if result.response != (LiveResponse{}) || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancelled body read = %+v, %v", result.response, result.err)
	}
}

func TestAgentClientCloseAfterResponseHeadersReturnsContextError(t *testing.T) {
	flushed := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(flushed)
		<-release
	}))
	done := make(chan struct {
		response LiveResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Live(context.Background())
		done <- struct {
			response LiveResponse
			err      error
		}{response: response, err: err}
	}()
	waitAgentSignal(t, flushed)
	client.Close()
	result := waitAgentLiveResult(t, done)
	if result.response != (LiveResponse{}) || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("closed body read = %+v, %v", result.response, result.err)
	}
}

func TestAgentClientDeadlineAfterResponseHeadersReturnsContextError(t *testing.T) {
	flushed := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(flushed)
		<-release
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct {
		response LiveResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Live(ctx)
		done <- struct {
			response LiveResponse
			err      error
		}{response: response, err: err}
	}()
	waitAgentSignal(t, flushed)
	result := waitAgentLiveResult(t, done)
	if result.response != (LiveResponse{}) || !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("deadline body read = %+v, %v", result.response, result.err)
	}
}

func TestAgentClientCloseLinearizesAdmissionBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live"}`))
	}))
	client.Close()
	for range 64 {
		got, err := client.Live(context.Background())
		if got != (LiveResponse{}) || !errors.Is(err, context.Canceled) {
			t.Fatalf("call after Close = %+v, %v", got, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("calls after Close touched network %d times", calls.Load())
	}
}

func TestAgentClientFormattingAndLoggingRedactCredentialForPointerAndValue(t *testing.T) {
	const secret = "round3-secret-never-print"
	client := newManifestAgentClientWithCredential(t, secret, fixedAgentResponseHandler(http.StatusOK, []byte(`{"status":"live"}`)))
	values := []any{client, *client}
	for _, value := range values {
		for _, format := range []string{"%v", "%+v", "%#v", "%q"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, secret) {
				t.Fatalf("format %q leaked credential: %q", format, formatted)
			}
		}
	}
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	logger.Info("agent", "pointer", client, "value", *client)
	if strings.Contains(output.String(), secret) {
		t.Fatalf("structured log leaked credential: %q", output.String())
	}
}

func TestAgentClientLargestValidTypedRequestUsesProductionPreflight(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	success := agentManifestSuccess(t, manifest, goldens, "dialogue", "terminal-true")
	var request AgentDialogueRequest
	mustStrictDecodeAgent(t, success.RequestBody, &request)
	request.Generation = math.MaxUint64
	request.MemoryEpoch = math.MaxUint64
	request.DeadlineUnixMS = math.MaxInt64
	request.Persona = strings.Repeat("<", 4096)
	request.FactNode = AgentDialogueFact{Kind: "terminal", State: "failed", Reason: "planner_unavailable"}
	request.Environment.ExposedBlocks = make([]AgentVisibleBlock, 256)
	for index := range request.Environment.ExposedBlocks {
		request.Environment.ExposedBlocks[index] = AgentVisibleBlock{
			Position: AgentBlockPosition{X: math.MinInt32, Y: -64, Z: math.MinInt32},
			BlockID:  math.MaxUint16 - 1,
		}
	}
	request.Environment.Heights = make([]AgentHeight, 1089)
	for index := range request.Environment.Heights {
		request.Environment.Heights[index] = AgentHeight{X: math.MinInt32, Z: math.MinInt32, Height: -65}
	}
	var response AgentDialogueResponse
	mustStrictDecodeAgent(t, success.ResponseBody, &response)
	response.Generation = request.Generation
	response.MemoryEpoch = request.MemoryEpoch
	responseBody, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	observedBody := make(chan []byte, 1)
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, incoming *http.Request) {
		body, readErr := io.ReadAll(incoming.Body)
		if readErr != nil {
			t.Errorf("read maximum request: %v", readErr)
		}
		observedBody <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	}))
	got, err := client.Dialogue(context.Background(), request)
	if err != nil || got.Generation != request.Generation {
		t.Fatalf("maximum typed Dialogue = %+v, %v", got, err)
	}
	body := <-observedBody
	if len(body) >= AgentMaxRequestBodyBytes {
		t.Fatalf("largest legal typed request = %d bytes, cap = %d", len(body), AgentMaxRequestBodyBytes)
	}
	t.Logf("maximum-dimension legal Dialogue request uses %d/%d bytes", len(body), AgentMaxRequestBodyBytes)
	var decoded AgentDialogueRequest
	if err := strictDecodeJSON(body, &decoded); err != nil || !validAgentRequest(decoded) {
		t.Fatalf("server-observed maximum request is not production-valid: %v", err)
	}

	var calls atomic.Int32
	oversizedClient := newManifestAgentClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	request.Persona = strings.Repeat("<", AgentMaxRequestBodyBytes)
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) <= AgentMaxRequestBodyBytes {
		t.Fatalf("oversized public request fixture = %d bytes, err=%v", len(encoded), err)
	}
	got, err = oversizedClient.Dialogue(context.Background(), request)
	if got != (AgentDialogueResponse{}) || !errors.Is(err, ErrAgentUnavailable) || calls.Load() != 0 {
		t.Fatalf("oversized public request = %+v, %v, calls=%d", got, err, calls.Load())
	}
}

func agentManifestSuccess(t *testing.T, manifest agentHTTPManifest, goldens map[string][]json.RawMessage, operation, variant string) agentRouteSuccessCase {
	t.Helper()
	for _, route := range manifest.Routes {
		if route.Operation != operation {
			continue
		}
		for _, success := range routeSuccessCases(t, route, goldens) {
			if success.Name == variant {
				return success
			}
		}
	}
	t.Fatalf("manifest lacks %s/%s", operation, variant)
	return agentRouteSuccessCase{}
}

func firstAgentPlanStep(t *testing.T, object map[string]any) map[string]any {
	t.Helper()
	plan := agentNestedObject(t, object, "plan")
	steps, ok := plan["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("plan steps = %T %#v", plan["steps"], plan["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("plan step = %T %#v", steps[0], steps[0])
	}
	return step
}

func agentNestedObject(t *testing.T, object map[string]any, field string) map[string]any {
	t.Helper()
	nested, ok := object[field].(map[string]any)
	if !ok {
		t.Fatalf("field %s = %T %#v", field, object[field], object[field])
	}
	return nested
}

type rawAgentHeaders struct {
	budget int
	lines  [][2]string
}

func startRawAgentHeaderServer(t *testing.T) (string, <-chan rawAgentHeaders) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	observed := make(chan rawAgentHeaders, 4)
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			reader := bufio.NewReader(connection)
			_, readErr := reader.ReadString('\n')
			lines := make([][2]string, 0, 8)
			budget := 0
			for readErr == nil {
				line, lineErr := reader.ReadString('\n')
				readErr = lineErr
				if line == "\r\n" {
					break
				}
				budget += len(line)
				name, value, ok := strings.Cut(strings.TrimSuffix(line, "\r\n"), ": ")
				if ok {
					lines = append(lines, [2]string{name, value})
				}
			}
			observed <- rawAgentHeaders{budget: budget, lines: lines}
			body := validAgentAcquireResponseJSON()
			_, _ = fmt.Fprintf(connection, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", len(body))
			_, _ = connection.Write(body)
			_ = connection.Close()
		}
	}()
	return "http://" + listener.Addr().String(), observed
}

func independentAgentHeaderBudget(lines [][2]string) int {
	total := 0
	for _, line := range lines {
		total += len(line[0]) + 2 + len(line[1]) + 2
	}
	return total
}

func assertRawAgentHeaders(t *testing.T, got, want [][2]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("raw header line count = %d, want %d", len(got), len(want))
	}
	wantByName := make(map[string]string, len(want))
	for _, line := range want {
		wantByName[strings.ToLower(line[0])] = line[1]
	}
	seen := make(map[string]bool, len(got))
	for _, line := range got {
		name := strings.ToLower(line[0])
		value, ok := wantByName[name]
		if !ok || seen[name] || value != line[1] {
			t.Fatalf("raw header %q missing, duplicated, unexpected, or changed", name)
		}
		seen[name] = true
	}
}

func waitAgentSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("Agent test server did not flush response headers")
	}
}

func waitAgentLiveResult(t *testing.T, done <-chan struct {
	response LiveResponse
	err      error
}) struct {
	response LiveResponse
	err      error
} {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("Agent client did not return after cancellation")
		return struct {
			response LiveResponse
			err      error
		}{}
	}
}
