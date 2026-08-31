package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/pathfind"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testMCPAuthority = "127.0.0.1:43127"
	testMCPNamespace = "4d5e6f70-8192-4aa3-8b4f-3a4b5c6d7e8f"
)

func TestMCPContractLoadsEmbeddedManifestSchemasInOrder(t *testing.T) {
	contract, err := loadCompanionMCPContract()
	if err != nil {
		t.Fatalf("loadCompanionMCPContract: %v", err)
	}
	if contract.ProtocolVersion != "2025-11-25" || contract.EndpointPath != "/mcp" ||
		contract.RequestBodyBytes != 256<<10 || contract.WireResponseBytes != 160<<10 {
		t.Fatalf("contract metadata=%+v", contract)
	}
	wantNames := companion.PlanningToolNames()
	if len(contract.Tools) != len(wantNames) {
		t.Fatalf("tools=%d，want %d", len(contract.Tools), len(wantNames))
	}
	for index, tool := range contract.Tools {
		if tool.Name != wantNames[index] {
			t.Errorf("tools[%d]=%q，want %q", index, tool.Name, wantNames[index])
		}
		if len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 {
			t.Errorf("tool %s schema 为空", tool.Name)
		}
		var input map[string]any
		if err := json.Unmarshal(tool.InputSchema, &input); err != nil || input["type"] != "object" {
			t.Errorf("tool %s input schema 未 resolve 为 object: %v/%v", tool.Name, input, err)
		}
		if tool.CanonicalResultBytes != companion.PlanningToolCanonicalLimit(tool.Name) {
			t.Errorf("tool %s canonical limit=%d，want %d", tool.Name, tool.CanonicalResultBytes, companion.PlanningToolCanonicalLimit(tool.Name))
		}
	}
}

func TestMCPContractDecoderRejectsDuplicateAndMalformedTrailingJSON(t *testing.T) {
	for name, data := range map[string][]byte{
		"duplicate":          []byte(`{"value":1,"value":2}`),
		"malformed trailing": []byte(`{"value":1}x`),
	} {
		t.Run(name, func(t *testing.T) {
			var decoded map[string]any
			if err := decodeEmbeddedMCPJSON(data, &decoded); err == nil {
				t.Fatalf("decodeEmbeddedMCPJSON(%s)=nil", data)
			}
		})
	}
}

func TestMCPOuterRejectsRawProtocolMatrixBeforeSDK(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	var calls atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	})
	handler := newCompanionMCPOuterHandler(testMCPAuthority, registry, next)
	validInitialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	validList := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		mutate func(*http.Request)
	}{
		{"GET", http.MethodGet, "/mcp", validInitialize, nil},
		{"wrong path", http.MethodPost, "/other", validInitialize, nil},
		{"missing Host", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Host = "" }},
		{"wrong Host", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Host = "127.0.0.1:9" }},
		{"missing bearer", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Del("Authorization") }},
		{"malformed bearer", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Set("Authorization", "Basic abc") }},
		{"wrong bearer", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }},
		{"duplicate bearer", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Add("Authorization", "Bearer extra") }},
		{"wrong origin", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:9") }},
		{"https origin", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Set("Origin", "https://"+testMCPAuthority) }},
		{"duplicate origin", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) {
			r.Header.Add("Origin", "http://"+testMCPAuthority)
			r.Header.Add("Origin", "http://"+testMCPAuthority)
		}},
		{"missing content type", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Del("Content-Type") }},
		{"wrong content type", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{"duplicate content type", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }},
		{"invalid utf8", http.MethodPost, "/mcp", string([]byte{'{', 0xff, '}'}), nil},
		{"empty", http.MethodPost, "/mcp", "", nil},
		{"trailing", http.MethodPost, "/mcp", validInitialize + `{}`, nil},
		{"array batch", http.MethodPost, "/mcp", `[` + validInitialize + `]`, nil},
		{"ping", http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`, nil},
		{"subscription", http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"subscriptions/listen","params":{}}`, nil},
		{"unknown method", http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`, nil},
		{"bad jsonrpc", http.MethodPost, "/mcp", `{"jsonrpc":"1.0","id":1,"method":"tools/list","params":{}}`, nil},
		{"missing request id", http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"tools/list","params":{}}`, nil},
		{"notification has id", http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"notifications/initialized","params":{}}`, nil},
		{"initialize wrong version", http.MethodPost, "/mcp", strings.Replace(validInitialize, "2025-11-25", "2025-06-18", 1), nil},
		{"initialize wrong header", http.MethodPost, "/mcp", validInitialize, func(r *http.Request) { r.Header.Set("Mcp-Protocol-Version", "2025-06-18") }},
		{"post-init missing header", http.MethodPost, "/mcp", validList, func(r *http.Request) { r.Header.Del("Mcp-Protocol-Version") }},
		{"post-init wrong header", http.MethodPost, "/mcp", validList, func(r *http.Request) { r.Header.Set("Mcp-Protocol-Version", "2025-06-18") }},
		{"oversize", http.MethodPost, "/mcp", strings.Repeat("x", (256<<10)+1), nil},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			before := calls.Load()
			request := testMCPRequest(testCase.method, testCase.path, testCase.body, registration.Capability)
			if testCase.mutate != nil {
				testCase.mutate(request)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code < 400 {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			assertMCPOuterErrorJSON(t, recorder)
			if calls.Load() != before {
				t.Fatalf("invalid request reached SDK: calls=%d/%d", before, calls.Load())
			}
			if strings.Contains(recorder.Body.String(), registration.Capability) || strings.Contains(recorder.Body.String(), "LEAK-ME-NOT") {
				t.Fatalf("error body 泄露输入: %s", recorder.Body.String())
			}
		})
	}

	for _, origin := range []string{"", "http://" + testMCPAuthority} {
		request := testMCPRequest(http.MethodPost, "/mcp", validInitialize, registration.Capability)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("valid origin %q status=%d body=%s", origin, recorder.Code, recorder.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("valid SDK calls=%d，want 2", calls.Load())
	}
}

func TestMCPSDKCapabilitiesSchemasToolsAndDomainResult(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	handler, err := newCompanionMCPHandler(testMCPAuthority, registry)
	if err != nil {
		t.Fatalf("newCompanionMCPHandler: %v", err)
	}
	initialize := testMCPRoundTrip(t, handler, registration.Capability, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	result := mcpResultObject(t, initialize)
	capabilities := result["capabilities"].(map[string]any)
	if !reflect.DeepEqual(capabilities, map[string]any{"tools": map[string]any{"listChanged": false}}) {
		t.Fatalf("initialize capabilities=%#v", capabilities)
	}
	if result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("protocolVersion=%v", result["protocolVersion"])
	}

	initialized := testMCPRoundTrip(t, handler, registration.Capability, "2025-11-25", `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
	if initialized.Code != http.StatusAccepted || initialized.Body.Len() != 0 {
		t.Fatalf("initialized status/body=%d/%q", initialized.Code, initialized.Body.String())
	}

	listed := testMCPRoundTrip(t, handler, registration.Capability, "2025-11-25", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	listedResult := mcpResultObject(t, listed)
	tools := listedResult["tools"].([]any)
	contract, _ := loadCompanionMCPContract()
	if len(tools) != len(contract.Tools) {
		t.Fatalf("tools=%d", len(tools))
	}
	for index, rawTool := range tools {
		tool := rawTool.(map[string]any)
		if tool["name"] != contract.Tools[index].Name {
			t.Errorf("tools[%d]=%v，want %s", index, tool["name"], contract.Tools[index].Name)
		}
		assertMCPJSONEqual(t, tool["inputSchema"], contract.Tools[index].InputSchema)
		assertMCPJSONEqual(t, tool["outputSchema"], contract.Tools[index].OutputSchema)
	}

	call := testMCPRoundTrip(t, handler, registration.Capability, "2025-11-25", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_planning_context","arguments":{}}}`)
	assertMCPStructuredAndTextEqual(t, call, false, "")
	domain := testMCPRoundTrip(t, handler, registration.Capability, "2025-11-25", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"find_visible_blocks","arguments":{"block_names":["missing_block"],"limit":1}}}`)
	assertMCPStructuredAndTextEqual(t, domain, false, "unknown_block")
	invalid := testMCPRoundTrip(t, handler, registration.Capability, "2025-11-25", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"inspect_inventory","arguments":{"offset":99,"limit":1}}}`)
	assertMCPStructuredAndTextEqual(t, invalid, true, "")
	unknown := testMCPRoundTrip(t, handler, registration.Capability, "2025-11-25", `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"LEAK-ME-NOT","arguments":{}}}`)
	if strings.Contains(unknown.Body.String(), "LEAK-ME-NOT") || unknown.Body.String() != `{"error":{"code":-32603,"message":"unavailable"},"id":7,"jsonrpc":"2.0"}` {
		t.Fatalf("unknown tool error 未受控: %s", unknown.Body.String())
	}

	request := testMCPRequest(http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":6,"method":"tools/list","params":{}}`, registration.Capability)
	request.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	request.Header.Set("Accept", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code < 400 {
		t.Fatalf("SDK 接受了缺 text/event-stream 的 Accept: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestMCPOuterBuffersResponseAndChecksCancellation(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	valid := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	tests := []struct {
		name string
		next http.Handler
	}{
		{"oversize", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, strings.Repeat("L", (160<<10)+1))
		})},
		{"sse", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: LEAK-ME-NOT")
		})},
		{"session", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "LEAK-ME-NOT")
			_, _ = io.WriteString(w, `{}`)
		})},
		{"empty session header", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header()["Mcp-Session-Id"] = []string{""}
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
		})},
		{"duplicate content type", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Add("Content-Type", "application/json")
			w.Header().Add("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
		})},
		{"handler error detail", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"LEAK-ME-NOT"}}`)
		})},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			handler := newCompanionMCPOuterHandler(testMCPAuthority, registry, testCase.next)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, testMCPRequest(http.MethodPost, "/mcp", valid, registration.Capability))
			if testCase.name == "handler error detail" {
				if recorder.Code != http.StatusOK || recorder.Body.String() != `{"error":{"code":-32603,"message":"unavailable"},"id":1,"jsonrpc":"2.0"}` {
					t.Fatalf("handler error 未受控: status=%d body=%q", recorder.Code, recorder.Body.String())
				}
				return
			}
			if recorder.Code < 400 || recorder.Body.Len() > 1024 || strings.Contains(recorder.Body.String(), "LEAK-ME-NOT") || strings.Contains(recorder.Body.String(), strings.Repeat("L", 64)) {
				t.Fatalf("uncontrolled response status=%d len=%d body=%q", recorder.Code, recorder.Body.Len(), recorder.Body.String())
			}
			assertMCPOuterErrorJSON(t, recorder)
		})
	}

	t.Run("notification content type bypass", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusAccepted)
		})
		body := `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`
		recorder := httptest.NewRecorder()
		newCompanionMCPOuterHandler(testMCPAuthority, registry, next).ServeHTTP(recorder,
			testMCPRequest(http.MethodPost, "/mcp", body, registration.Capability))
		if recorder.Code < 400 {
			t.Fatalf("notification SSE header escaped: %d %v", recorder.Code, recorder.Header())
		}
		assertMCPOuterErrorJSON(t, recorder)
	})

	canceling := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		registry.Cancel(registration.SnapshotID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"LEAK-ME-NOT":true}}`)
	})
	recorder := httptest.NewRecorder()
	newCompanionMCPOuterHandler(testMCPAuthority, registry, canceling).ServeHTTP(recorder,
		testMCPRequest(http.MethodPost, "/mcp", valid, registration.Capability))
	if recorder.Code < 400 || strings.Contains(recorder.Body.String(), "LEAK-ME-NOT") {
		t.Fatalf("canceled response committed: %d %s", recorder.Code, recorder.Body.String())
	}
	assertMCPOuterErrorJSON(t, recorder)
}

func TestMCPOuterChecksRequestCancellationBeforeAndAfterSDK(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	valid := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

	var calls atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"LEAK-ME-NOT":true}}`)
	})
	handler := newCompanionMCPOuterHandler(testMCPAuthority, registry, next)
	beforeContext, cancelBefore := context.WithCancel(context.Background())
	cancelBefore()
	before := testMCPRequest(http.MethodPost, "/mcp", valid, registration.Capability).WithContext(beforeContext)
	beforeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(beforeRecorder, before)
	if calls.Load() != 0 || beforeRecorder.Code < 400 {
		t.Fatalf("pre-canceled request calls/status=%d/%d", calls.Load(), beforeRecorder.Code)
	}
	assertMCPOuterErrorJSON(t, beforeRecorder)

	afterContext, cancelAfter := context.WithCancel(context.Background())
	afterNext := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cancelAfter()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"LEAK-ME-NOT":true}}`)
	})
	after := testMCPRequest(http.MethodPost, "/mcp", valid, registration.Capability).WithContext(afterContext)
	afterRecorder := httptest.NewRecorder()
	newCompanionMCPOuterHandler(testMCPAuthority, registry, afterNext).ServeHTTP(afterRecorder, after)
	if afterRecorder.Code < 400 || strings.Contains(afterRecorder.Body.String(), "LEAK-ME-NOT") {
		t.Fatalf("mid-request cancellation committed: %d %s", afterRecorder.Code, afterRecorder.Body.String())
	}
	assertMCPOuterErrorJSON(t, afterRecorder)
}

func TestMCPOuterAcceptsExactRequestAndResponseByteLimits(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	responsePrefix := `{"jsonrpc":"2.0","id":1,"result":{"padding":"`
	responseSuffix := `"}}`
	response := responsePrefix + strings.Repeat("r", companionMCPResponseBytes-len(responsePrefix)-len(responseSuffix)) + responseSuffix
	if len(response) != companionMCPResponseBytes || !json.Valid([]byte(response)) {
		t.Fatalf("response fixture len/valid=%d/%v", len(response), json.Valid([]byte(response)))
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	})
	requestPrefix := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_planning_context","arguments":{}}}`
	requestBody := requestPrefix + strings.Repeat(" ", companionMCPRequestBytes-len(requestPrefix))
	if len(requestBody) != companionMCPRequestBytes || !json.Valid([]byte(requestBody)) {
		t.Fatalf("request fixture len/valid=%d/%v", len(requestBody), json.Valid([]byte(requestBody)))
	}
	recorder := httptest.NewRecorder()
	newCompanionMCPOuterHandler(testMCPAuthority, registry, next).ServeHTTP(recorder,
		testMCPRequest(http.MethodPost, "/mcp", requestBody, registration.Capability))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != companionMCPResponseBytes || !json.Valid(recorder.Body.Bytes()) {
		t.Fatalf("exact limits status/len=%d/%d", recorder.Code, recorder.Body.Len())
	}
}

func TestMCPErrorPayloadsAreExactValidJSON(t *testing.T) {
	toolError := companionMCPToolError()
	if !toolError.IsError || toolError.StructuredContent != nil || len(toolError.Content) != 1 {
		t.Fatalf("tool error shape=%#v", toolError)
	}
	text, ok := toolError.Content[0].(*mcp.TextContent)
	if !ok || text.Text != `{"code":"unavailable"}` || !json.Valid([]byte(text.Text)) {
		t.Fatalf("tool error text=%T/%q", toolError.Content[0], text.Text)
	}

	recorder := httptest.NewRecorder()
	writeCompanionMCPError(recorder, http.StatusBadRequest, "invalid_request")
	if recorder.Body.String() != `{"error":{"code":"invalid_request"}}` {
		t.Fatalf("outer error wire=%q", recorder.Body.String())
	}
	assertMCPOuterErrorJSON(t, recorder)
}

func TestMCPServiceLoopbackCloseAndServeFailureIsolation(t *testing.T) {
	registry := companion.NewSnapshotRegistry()
	service, err := newCompanionMCPService(registry)
	if err != nil {
		t.Fatalf("newCompanionMCPService: %v", err)
	}
	parsed, err := url.Parse(service.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() || parsed.Port() == "" || parsed.Path != "/mcp" {
		t.Fatalf("endpoint=%q", service.Endpoint())
	}
	service.Close()
	service.Close()
	select {
	case <-service.Done():
	case <-time.After(time.Second):
		t.Fatal("Close 未 unblock Serve")
	}

	host := mustNewHost(t, hostTestConfig(), flatTestGenerator{}, newHostTestStore())
	before := host.world.StepForTest().Tick
	isolated, err := newCompanionMCPService(companion.NewSnapshotRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if err := isolated.listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case serveErr := <-isolated.Done():
		if serveErr == nil || errors.Is(serveErr, http.ErrServerClosed) {
			t.Fatalf("unexpected Serve error=%v", serveErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve error 未返回")
	}
	after := host.world.StepForTest().Tick
	if after <= before {
		t.Fatalf("MCP Serve error 停止世界: %d -> %d", before, after)
	}
	isolated.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := host.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}

func TestHostMCPAssemblyOnlyWhenCompanionsEnabled(t *testing.T) {
	empty := mustNewHost(t, hostTestConfig(), flatTestGenerator{}, newHostTestStore())
	if empty.companionMCP != nil || empty.companionSnapshots != nil {
		t.Fatal("无伙伴 Host 不得构造 MCP/registry")
	}
	emptyCtx, emptyCancel := context.WithTimeout(context.Background(), time.Second)
	defer emptyCancel()
	if err := empty.Shutdown(emptyCtx); err != nil {
		t.Fatal(err)
	}

	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	host := mustNewHost(t, config, flatTestGenerator{}, newCompanionBootstrapStore())
	if host.companionMCP == nil || host.companionSnapshots == nil || host.companionMCP.Endpoint() == "" {
		t.Fatal("有伙伴 Host 缺少 MCP/registry")
	}
	done := host.companionMCP.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := host.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Host.Shutdown 未关闭 MCP Serve")
	}
}

func testMCPRegistry(t *testing.T) (*companion.SnapshotRegistry, companion.SnapshotRegistration) {
	t.Helper()
	registry := companion.NewSnapshotRegistry()
	t.Cleanup(registry.Close)
	snapshot := testMCPPlanSnapshot(t)
	registration, err := registry.Register(testMCPNamespace, snapshot.Companion.ID, 1, snapshot, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("register MCP snapshot: %v", err)
	}
	return registry, registration
}

func testMCPPlanSnapshot(t *testing.T) companion.PlanSnapshot {
	t.Helper()
	issuer, err := core.ParsePlayerID("99999999-9999-4999-8999-999999999999")
	if err != nil {
		t.Fatal(err)
	}
	companionID, err := companion.ParseID("66666666-6666-4666-8666-666666666666")
	if err != nil {
		t.Fatal(err)
	}
	terrain := companion.NewTerrainProjection(core.BlockPos{X: -11, Y: 56, Z: -18})
	for x := int32(-11); x <= 21; x++ {
		for z := int32(-18); z <= 14; z++ {
			if !terrain.SetReadyColumn(x, z, 63) {
				t.Fatal("SetReadyColumn=false")
			}
		}
	}
	if !terrain.SetBlock(core.BlockPos{X: 8, Y: 63, Z: -2}, core.StoneID) {
		t.Fatal("SetBlock=false")
	}
	snapshot := companion.PlanSnapshot{
		Command: "采一块石头",
		Issuer: companion.PlanPlayer{
			ID: issuer, Position: [3]float32{4.5, 64, -1.5},
			LookHit: core.BlockPos{X: 8, Y: 63, Z: -2}, HasLookHit: true,
		},
		Companion: companion.PlanCompanion{
			ID: companionID, Position: [3]float32{5.5, 64, -1.5}, TaskStatus: "规划中",
		},
		ExposedBlocks: []companion.PlanBlock{{Pos: core.BlockPos{X: 8, Y: 63, Z: -2}, Block: core.StoneID}},
		Heights:       terrain.Heights(),
		Terrain:       terrain,
		OnlinePlayers: []companion.PlanPlayer{{ID: issuer, Position: [3]float32{4.5, 64, -1.5}}},
		ChunkRevisions: []pathfind.ChunkRevision{{
			Chunk: core.ChunkPos{X: 0, Z: -1}, Revision: 17,
		}},
		WorldTimeTicks: 1200,
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snapshot
}

func testMCPRequest(method, path, body, capability string) *http.Request {
	request := httptest.NewRequest(method, "http://"+testMCPAuthority+path, strings.NewReader(body))
	request.Host = testMCPAuthority
	request.Header.Set("Authorization", "Bearer "+capability)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if strings.Contains(body, `"method":"tools/`) || strings.Contains(body, `"method":"notifications/`) {
		request.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	}
	return request
}

func testMCPRoundTrip(t *testing.T, handler http.Handler, capability, protocol, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := testMCPRequest(http.MethodPost, "/mcp", body, capability)
	if protocol == "" {
		request.Header.Del("Mcp-Protocol-Version")
	} else {
		request.Header.Set("Mcp-Protocol-Version", protocol)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code >= 400 || strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") || recorder.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("MCP status/header=%d/%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	return recorder
}

func mcpResultObject(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(recorder.Body.Bytes()))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("MCP response JSON: %v: %s", err, recorder.Body.String())
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP response missing result: %#v", envelope)
	}
	return result
}

func assertMCPJSONEqual(t *testing.T, actual any, raw json.RawMessage) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var expected any
	if err := decoder.Decode(&expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON mismatch\nactual=%#v\nexpected=%#v", actual, expected)
	}
}

func assertMCPStructuredAndTextEqual(t *testing.T, recorder *httptest.ResponseRecorder, wantError bool, wantCode string) {
	t.Helper()
	result := mcpResultObject(t, recorder)
	if got, _ := result["isError"].(bool); got != wantError {
		t.Fatalf("isError=%v，want %v: %#v", got, wantError, result)
	}
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content=%d", len(content))
	}
	text := content[0].(map[string]any)["text"].(string)
	var textValue any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&textValue); err != nil {
		t.Fatal(err)
	}
	if wantError {
		if result["structuredContent"] != nil {
			t.Fatalf("error result 不得带 structuredContent: %#v", result)
		}
		if !reflect.DeepEqual(textValue, map[string]any{"code": "unavailable"}) || text != `{"code":"unavailable"}` {
			t.Fatalf("error TextContent=%s/%#v", text, textValue)
		}
		return
	}
	if !reflect.DeepEqual(textValue, result["structuredContent"]) {
		t.Fatalf("structured/text mismatch: %#v / %#v", result["structuredContent"], textValue)
	}
	if wantCode != "" {
		if code := textValue.(map[string]any)["code"]; code != wantCode {
			t.Fatalf("code=%v，want %s", code, wantCode)
		}
	}
}

func assertMCPOuterErrorJSON(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("Content-Type") != "application/json" || !json.Valid(recorder.Body.Bytes()) {
		t.Fatalf("outer error 非 JSON: header=%v body=%q", recorder.Header(), recorder.Body.String())
	}
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil || decoded.Error.Code == "" {
		t.Fatalf("outer error shape=%+v err=%v body=%q", decoded, err, recorder.Body.String())
	}
	want := `{"error":{"code":"` + decoded.Error.Code + `"}}`
	if recorder.Body.String() != want {
		t.Fatalf("outer error wire=%q，want %q", recorder.Body.String(), want)
	}
}
