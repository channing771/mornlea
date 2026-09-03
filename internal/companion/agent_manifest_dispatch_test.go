package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type agentHTTPManifest struct {
	ContentType      string                   `json:"content_type"`
	IdentityProfiles map[string][]string      `json:"identity_profiles"`
	Routes           []agentHTTPManifestRoute `json:"routes"`
	Errors           []agentHTTPManifestError `json:"errors"`
	Limits           map[string]int           `json:"limits"`
}

type agentHTTPManifestRoute struct {
	Operation               string                     `json:"operation"`
	Method                  string                     `json:"method"`
	Path                    string                     `json:"path"`
	Authentication          string                     `json:"authentication"`
	IdentityProfile         string                     `json:"identity_profile"`
	RequestSchema           *string                    `json:"request_schema"`
	Responses               []agentHTTPManifestReply   `json:"responses"`
	ErrorCodes              []string                   `json:"error_codes"`
	RequestResponseVariants *agentHTTPManifestVariants `json:"request_response_variants"`
}

type agentHTTPManifestReply struct {
	Status      int    `json:"status"`
	Schema      string `json:"schema"`
	StatusValue string `json:"status_value"`
}

type agentHTTPManifestVariants struct {
	Discriminator string                     `json:"discriminator"`
	Variants      []agentHTTPManifestVariant `json:"variants"`
}

type agentHTTPManifestVariant struct {
	Value          bool   `json:"value"`
	RequestSchema  string `json:"request_schema"`
	ResponseSchema string `json:"response_schema"`
	MemoryProposal string `json:"memory_proposal"`
}

type agentHTTPManifestError struct {
	Code   string `json:"code"`
	Status int    `json:"status"`
}

type agentRouteSuccessCase struct {
	Name         string
	RequestBody  json.RawMessage
	ResponseBody json.RawMessage
	Status       int
}

func TestAgentManifestDrivesAllPublicRouteDispatch(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	if len(manifest.Routes) != 11 {
		t.Fatalf("manifest routes = %d, want 11", len(manifest.Routes))
	}

	seen := make(map[string]bool, len(manifest.Routes))
	caseCount := 0
	for _, route := range manifest.Routes {
		route := route
		if seen[route.Operation] {
			t.Fatalf("duplicate manifest operation %q", route.Operation)
		}
		seen[route.Operation] = true
		cases := routeSuccessCases(t, route, goldens)
		caseCount += len(cases)
		for _, success := range cases {
			success := success
			t.Run(route.Operation+"/"+success.Name, func(t *testing.T) {
				called := 0
				client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					called++
					if request.Method != route.Method {
						t.Errorf("method = %q, want %q", request.Method, route.Method)
					}
					if request.URL.Path != route.Path {
						t.Errorf("path = %q, want %q", request.URL.Path, route.Path)
					}
					wantAuthorization := "Bearer manifest-secret"
					if route.Authentication == "none" {
						wantAuthorization = ""
					}
					if got := request.Header.Get("Authorization"); got != wantAuthorization {
						t.Errorf("Authorization = %q, want %q", got, wantAuthorization)
					}
					if got := request.Header.Get("Accept-Encoding"); got != "identity" {
						t.Errorf("Accept-Encoding = %q", got)
					}
					body, err := io.ReadAll(request.Body)
					if err != nil {
						t.Errorf("read request: %v", err)
					}
					if route.RequestSchema == nil {
						if len(body) != 0 {
							t.Errorf("health request body = %q", body)
						}
						if got := request.Header.Get("Content-Type"); got != "" {
							t.Errorf("health Content-Type = %q", got)
						}
					} else {
						if got := request.Header.Get("Content-Type"); got != manifest.ContentType {
							t.Errorf("Content-Type = %q, want %q", got, manifest.ContentType)
						}
						assertAgentJSONEqual(t, body, success.RequestBody)
						assertManifestIdentityProfile(t, manifest, route, body)
					}
					w.Header().Set("Content-Type", manifest.ContentType)
					w.WriteHeader(success.Status)
					_, _ = w.Write(success.ResponseBody)
				}))
				got, err := callAgentOperation(t, client, route.Operation, success.RequestBody)
				if err != nil {
					t.Fatalf("dispatch: %v", err)
				}
				if called != 1 {
					t.Fatalf("network calls = %d, want 1", called)
				}
				encoded, err := json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				assertAgentJSONEqual(t, encoded, success.ResponseBody)
			})
		}
	}
	if caseCount != 14 {
		t.Fatalf("success cases = %d, want 14 manifest/golden variants", caseCount)
	}
}

func TestAgentManifestDrivesEveryDeclaredErrorStatusAndCode(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	statuses := make(map[string]int, len(manifest.Errors))
	allCodes := make([]string, 0, len(manifest.Errors))
	for _, item := range manifest.Errors {
		statuses[item.Code] = item.Status
		allCodes = append(allCodes, item.Code)
	}

	for _, route := range manifest.Routes {
		route := route
		success := routeSuccessCases(t, route, goldens)[0]
		for _, code := range route.ErrorCodes {
			code := code
			t.Run(route.Operation+"/"+code, func(t *testing.T) {
				requestID := manifestRequestID(success.RequestBody)
				body := marshalManifestError(t, requestID, code)
				client := newManifestAgentClient(t, fixedAgentResponseHandler(statuses[code], body))
				got, err := callAgentOperation(t, client, route.Operation, success.RequestBody)
				assertZeroAgentResult(t, got)
				if code == "invalid_model_output" {
					if !errors.Is(err, ErrAgentInvalidModelOutput) {
						t.Fatalf("error = %v, want invalid model output", err)
					}
				} else {
					var agentErr *AgentError
					if !errors.As(err, &agentErr) || agentErr.Code != code {
						t.Fatalf("error = %v, want AgentError(%s)", err, code)
					}
				}

				wrongStatus := statuses[code] + 1
				client = newManifestAgentClient(t, fixedAgentResponseHandler(wrongStatus, body))
				got, err = callAgentOperation(t, client, route.Operation, success.RequestBody)
				assertZeroAgentResult(t, got)
				if !errors.Is(err, ErrAgentUnavailable) {
					t.Fatalf("wrong status/code error = %v", err)
				}
			})
		}

		t.Run(route.Operation+"/undeclared-code", func(t *testing.T) {
			declared := make(map[string]bool, len(route.ErrorCodes))
			for _, code := range route.ErrorCodes {
				declared[code] = true
			}
			var code string
			for _, candidate := range allCodes {
				if !declared[candidate] {
					code = candidate
					break
				}
			}
			if code == "" {
				t.Fatal("route unexpectedly declares every stable error")
			}
			body := marshalManifestError(t, manifestRequestID(success.RequestBody), code)
			client := newManifestAgentClient(t, fixedAgentResponseHandler(statuses[code], body))
			got, err := callAgentOperation(t, client, route.Operation, success.RequestBody)
			assertZeroAgentResult(t, got)
			if !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("undeclared code error = %v", err)
			}
		})

		t.Run(route.Operation+"/undeclared-success-status", func(t *testing.T) {
			status := http.StatusCreated
			for _, reply := range route.Responses {
				if reply.Status == status {
					status = http.StatusAccepted
				}
			}
			client := newManifestAgentClient(t, fixedAgentResponseHandler(status, success.ResponseBody))
			got, err := callAgentOperation(t, client, route.Operation, success.RequestBody)
			assertZeroAgentResult(t, got)
			if !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("undeclared success status error = %v", err)
			}
		})
	}
}

func TestAgentManifestIdentityProfilesDriveCorrelationRejection(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	for _, route := range manifest.Routes {
		route := route
		if route.RequestSchema == nil {
			continue
		}
		for _, success := range routeSuccessCases(t, route, goldens) {
			success := success
			request := decodeAgentJSONObject(t, success.RequestBody)
			response := decodeAgentJSONObject(t, success.ResponseBody)
			fields := append([]string(nil), manifest.IdentityProfiles[route.IdentityProfile]...)
			if route.Operation == "memory_reconcile" && response["tombstone_operation_id"] != nil {
				fields = append(fields, "tombstone_operation_id")
			}
			mutations := 0
			for _, field := range fields {
				field := field
				requestField := field
				responseField := field
				if route.Operation == "memory_delete" && field == "new_memory_epoch" {
					responseField = "memory_epoch"
				}
				requestValue, requestHas := request[requestField]
				responseValue, responseHas := response[responseField]
				if !requestHas {
					t.Fatalf("manifest identity %q is absent from %s request", requestField, route.Operation)
				}
				if !responseHas {
					continue
				}
				if responseValue == nil || !reflect.DeepEqual(requestValue, responseValue) {
					t.Fatalf("golden correlation %s.%s = %v, request = %v", route.Operation, responseField, responseValue, requestValue)
				}
				mutations++
				t.Run(route.Operation+"/"+success.Name+"/"+responseField, func(t *testing.T) {
					mutated := cloneAgentJSONObject(t, success.ResponseBody)
					mutated[responseField] = differentAgentIdentityValue(t, responseField, responseValue)
					body := marshalAgentJSONObject(t, mutated)
					client := newManifestAgentClient(t, fixedAgentResponseHandler(success.Status, body))
					got, err := callAgentOperation(t, client, route.Operation, success.RequestBody)
					assertZeroAgentResult(t, got)
					if !errors.Is(err, ErrAgentUnavailable) {
						t.Fatalf("correlation error = %v", err)
					}
				})
			}
			if mutations == 0 {
				t.Fatalf("manifest identity profile %q produced no response correlation checks", route.IdentityProfile)
			}
		}
	}
}

func TestAgentClientStrictJSONFailuresReturnOnlyZeroValues(t *testing.T) {
	malformed := []struct {
		name string
		body []byte
	}{
		{name: "duplicate", body: []byte(`{"status":"live","status":"live"}`)},
		{name: "invalid UTF-8", body: []byte{'{', '"', 's', 't', 'a', 't', 'u', 's', '"', ':', '"', 0xff, '"', '}'}},
		{name: "isolated high surrogate", body: []byte(`{"status":"\ud800"}`)},
		{name: "isolated low surrogate", body: []byte(`{"status":"\udc00"}`)},
		{name: "mismatched surrogate pair", body: []byte(`{"status":"\ud800\u0041"}`)},
		{name: "nonfinite", body: []byte(`{"status":NaN}`)},
		{name: "trailing", body: []byte(`{"status":"live"}{}`)},
		{name: "null object", body: []byte(`null`)},
		{name: "wrong type", body: []byte(`{"status":1}`)},
		{name: "missing", body: []byte(`{}`)},
		{name: "unknown", body: []byte(`{"status":"live","extra":true}`)},
	}
	for _, testCase := range malformed {
		t.Run(testCase.name, func(t *testing.T) {
			client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, testCase.body))
			got, err := client.Live(context.Background())
			if got != (LiveResponse{}) {
				t.Fatalf("failure returned partial value %+v", got)
			}
			if !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAgentStrictJSONAllowsEscapedLiteralSurrogateText(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body []byte
		want string
	}{
		{name: "escaped literal", body: []byte(`{"text":"\\ud800"}`), want: `\ud800`},
		{name: "valid pair", body: []byte(`{"text":"\ud83d\ude00"}`), want: "😀"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var value struct {
				Text string `json:"text"`
			}
			if err := strictDecodeJSON(testCase.body, &value); err != nil {
				t.Fatalf("valid surrogate representation rejected: %v", err)
			}
			if value.Text != testCase.want {
				t.Fatalf("decoded text = %q, want %q", value.Text, testCase.want)
			}
		})
	}
}

func TestAgentClientStrictErrorEnvelopeAndNullableRequestIdentity(t *testing.T) {
	request := validAgentAcquireRequest()
	validNull := []byte(`{"contract_version":"v1","request_id":null,"error":{"code":"invalid_request"}}`)
	client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusBadRequest, validNull))
	got, err := client.Acquire(context.Background(), request)
	var agentErr *AgentError
	if got != (AcquireResponse{}) || !errors.As(err, &agentErr) || agentErr.Code != "invalid_request" {
		t.Fatalf("nullable request identity = %+v, %v", got, err)
	}

	const responseSecret = "response-body-secret"
	invalid := []struct {
		name string
		body []byte
	}{
		{name: "mismatched request", body: []byte(`{"contract_version":"v1","request_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","error":{"code":"invalid_request"}}`)},
		{name: "missing request", body: []byte(`{"contract_version":"v1","error":{"code":"invalid_request"}}`)},
		{name: "noncanonical request", body: []byte(`{"contract_version":"v1","request_id":"NOT-A-UUID","error":{"code":"invalid_request"}}`)},
		{name: "domain identity", body: []byte(`{"contract_version":"v1","request_id":null,"namespace_id":"33333333-3333-4333-8333-333333333333","error":{"code":"invalid_request"}}`)},
		{name: "wrong version", body: []byte(`{"contract_version":"v2","request_id":null,"error":{"code":"invalid_request"}}`)},
		{name: "missing code", body: []byte(`{"contract_version":"v1","request_id":null,"error":{}}`)},
		{name: "unknown code", body: []byte(`{"contract_version":"v1","request_id":null,"error":{"code":"future_error"}}`)},
		{name: "untrusted detail", body: []byte(`{"contract_version":"v1","request_id":null,"error":{"code":"invalid_request","detail":"` + responseSecret + `"}}`)},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusBadRequest, testCase.body))
			got, err := client.Acquire(context.Background(), request)
			if got != (AcquireResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("invalid error envelope = %+v, %v", got, err)
			}
			if strings.Contains(err.Error(), responseSecret) {
				t.Fatalf("error leaked response body: %v", err)
			}
		})
	}
}

func TestAgentClientStrictNestedRequiredAndVariantFields(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	goldens := loadAgentValidGoldenValues(t)
	routes := make(map[string]agentHTTPManifestRoute, len(manifest.Routes))
	for _, route := range manifest.Routes {
		routes[route.Operation] = route
	}

	tests := []struct {
		name      string
		operation string
		request   json.RawMessage
		response  json.RawMessage
		want      error
	}{
		{
			name: "plan step missing zero-capable x", operation: "plan",
			request: routeSuccessCases(t, routes["plan"], goldens)[0].RequestBody,
			response: mutateAgentJSON(t, firstAgentGolden(t, goldens, "plan_response"), func(object map[string]any) {
				step := object["plan"].(map[string]any)["steps"].([]any)[0].(map[string]any)
				delete(step, "x")
			}),
			want: ErrAgentInvalidModelOutput,
		},
		{
			name: "plan step explicit null known foreign field", operation: "plan",
			request: routeSuccessCases(t, routes["plan"], goldens)[0].RequestBody,
			response: mutateAgentJSON(t, firstAgentGolden(t, goldens, "plan_response"), func(object map[string]any) {
				step := object["plan"].(map[string]any)["steps"].([]any)[0].(map[string]any)
				step["player_id"] = nil
			}),
			want: ErrAgentInvalidModelOutput,
		},
		{
			name: "plan digest uppercase", operation: "plan",
			request: routeSuccessCases(t, routes["plan"], goldens)[0].RequestBody,
			response: mutateAgentJSON(t, firstAgentGolden(t, goldens, "plan_response"), func(object map[string]any) {
				object["snapshot_digest"] = strings.Repeat("A", 64)
			}),
		},
		{
			name: "terminal proposal missing zero-capable base revision", operation: "dialogue",
			request: goldenWithBooleanField(t, goldens["dialogue_request"], "terminal", true),
			response: mutateAgentJSON(t, goldenWithObjectField(t, goldens["dialogue_response"], "memory_proposal", true), func(object map[string]any) {
				delete(object["memory_proposal"].(map[string]any), "base_revision")
			}),
		},
		{
			name: "nonterminal proposal explicit null", operation: "dialogue",
			request: goldenWithBooleanField(t, goldens["dialogue_request"], "terminal", false),
			response: mutateAgentJSON(t, goldenWithObjectField(t, goldens["dialogue_response"], "memory_proposal", false), func(object map[string]any) {
				object["memory_proposal"] = nil
			}),
		},
		{
			name: "active memory zero state missing summary", operation: "memory_reconcile",
			request: goldenWithBooleanField(t, goldens["memory_reconcile_request"], "active", true),
			response: mutateAgentJSON(t, goldenWithBooleanField(t, goldens["memory_reconcile_response"], "active", true), func(object map[string]any) {
				memory := object["memory"].(map[string]any)
				memory["revision"] = json.Number("0")
				memory["operation_id"] = nil
				delete(memory, "summary")
			}),
		},
		{
			name: "cancel missing boolean", operation: "run_cancel",
			request: firstAgentGolden(t, goldens, "cancel_request"),
			response: mutateAgentJSON(t, firstAgentGolden(t, goldens, "cancel_response"), func(object map[string]any) {
				delete(object, "cancelled")
			}),
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, testCase.response))
			got, err := callAgentOperation(t, client, testCase.operation, testCase.request)
			assertZeroAgentResult(t, got)
			want := testCase.want
			if want == nil {
				want = ErrAgentUnavailable
			}
			if !errors.Is(err, want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAgentClientAcceptsCancelFalseDeclaredBySchema(t *testing.T) {
	goldens := loadAgentValidGoldenValues(t)
	response := mutateAgentJSON(t, firstAgentGolden(t, goldens, "cancel_response"), func(object map[string]any) {
		object["cancelled"] = false
	})
	client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, response))
	got, err := callAgentOperation(t, client, "run_cancel", firstAgentGolden(t, goldens, "cancel_request"))
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got.(CancelResponse).Cancelled {
		t.Fatalf("CancelRun = %+v, want explicit false", got)
	}
}

func TestAgentClientRejectsInvalidRequestsBeforeDispatch(t *testing.T) {
	goldens := loadAgentValidGoldenValues(t)
	var acquire AcquireRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "acquire_request"), &acquire)
	var lease LeaseRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "lease_request"), &lease)
	var plan PlanRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "plan_request"), &plan)
	var dialogue AgentDialogueRequest
	mustStrictDecodeAgent(t, goldenWithBooleanField(t, goldens["dialogue_request"], "terminal", false), &dialogue)
	var reconcile MemoryReconcileRequest
	mustStrictDecodeAgent(t, goldenWithBooleanField(t, goldens["memory_reconcile_request"], "active", true), &reconcile)
	var commit MemoryCommitRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "memory_commit_request"), &commit)
	var deleteRequest MemoryDeleteRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "memory_delete_request"), &deleteRequest)
	var cancel CancelRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "cancel_request"), &cancel)

	badAcquire := acquire
	badAcquire.ContractVersion = "v2"
	badLease := lease
	badLease.LeaseID = ""
	blankInstruction := plan
	blankInstruction.Instruction = "\u3000"
	missingMCPPort := plan
	missingMCPPort.MCPEndpoint = "http://127.0.0.1/mcp"
	encodedMCPPath := plan
	encodedMCPPath.MCPEndpoint = "http://127.0.0.1:45831/%6dcp"
	invalidMCPPort := plan
	invalidMCPPort.MCPEndpoint = "http://127.0.0.1:70000/mcp"
	controlCapability := plan
	controlCapability.MCPCapability = "cap\u0085ability"
	nilEnvironment := dialogue
	nilEnvironment.Environment = AgentDialogueEnvironment{}
	badReconcile := reconcile
	badReconcile.Mirror = nil
	badCommit := commit
	badCommit.Summary = "bad\x00summary"
	badDelete := deleteRequest
	badDelete.NewMemoryEpoch = 0
	badCancel := cancel
	badCancel.RunID = ""

	tests := []struct {
		name string
		call func(*AgentClient) (any, error)
	}{
		{name: "acquire version", call: func(client *AgentClient) (any, error) { return client.Acquire(context.Background(), badAcquire) }},
		{name: "lease identity", call: func(client *AgentClient) (any, error) { return client.Heartbeat(context.Background(), badLease) }},
		{name: "instruction blank", call: func(client *AgentClient) (any, error) { return client.Plan(context.Background(), blankInstruction) }},
		{name: "MCP port missing", call: func(client *AgentClient) (any, error) { return client.Plan(context.Background(), missingMCPPort) }},
		{name: "MCP encoded path", call: func(client *AgentClient) (any, error) { return client.Plan(context.Background(), encodedMCPPath) }},
		{name: "MCP invalid port", call: func(client *AgentClient) (any, error) { return client.Plan(context.Background(), invalidMCPPort) }},
		{name: "MCP capability control", call: func(client *AgentClient) (any, error) { return client.Plan(context.Background(), controlCapability) }},
		{name: "dialogue arrays null", call: func(client *AgentClient) (any, error) { return client.Dialogue(context.Background(), nilEnvironment) }},
		{name: "reconcile variant", call: func(client *AgentClient) (any, error) {
			return client.ReconcileMemory(context.Background(), badReconcile)
		}},
		{name: "summary NUL", call: func(client *AgentClient) (any, error) { return client.CommitMemory(context.Background(), badCommit) }},
		{name: "delete epoch", call: func(client *AgentClient) (any, error) { return client.DeleteMemory(context.Background(), badDelete) }},
		{name: "cancel identity", call: func(client *AgentClient) (any, error) { return client.CancelRun(context.Background(), badCancel) }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			client := newManifestAgentClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			got, err := testCase.call(client)
			assertZeroAgentResult(t, got)
			if !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("error = %v", err)
			}
			if called {
				t.Fatal("invalid request reached network")
			}
		})
	}
}

func TestAgentClientAllowsSchemaTextControlsOutsideRestrictedFields(t *testing.T) {
	goldens := loadAgentValidGoldenValues(t)
	requestBody := goldenWithBooleanField(t, goldens["dialogue_request"], "terminal", false)
	var request AgentDialogueRequest
	mustStrictDecodeAgent(t, requestBody, &request)
	request.Persona = "line one\nline two"
	response := goldenWithObjectField(t, goldens["dialogue_response"], "memory_proposal", false)
	client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, response))
	got, err := client.Dialogue(context.Background(), request)
	if err != nil || got.Line == "" {
		t.Fatalf("Dialogue = %+v, %v", got, err)
	}

	var commit MemoryCommitRequest
	mustStrictDecodeAgent(t, firstAgentGolden(t, goldens, "memory_commit_request"), &commit)
	commit.Summary = "line one\nline two"
	client = newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, firstAgentGolden(t, goldens, "memory_commit_response")))
	if got, err := client.CommitMemory(context.Background(), commit); err != nil || got.CommittedRevision == 0 {
		t.Fatalf("CommitMemory = %+v, %v", got, err)
	}
}

func loadAgentHTTPManifest(t *testing.T) agentHTTPManifest {
	t.Helper()
	path := filepath.Join("..", "..", "packages", "contracts", "companion-agent", "http-v1", "manifest.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest agentHTTPManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func loadAgentValidGoldenValues(t *testing.T) map[string][]json.RawMessage {
	t.Helper()
	path := filepath.Join("..", "..", "packages", "contracts", "companion-agent", "http-v1", "golden", "valid.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture agentGoldenFile
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	values := make(map[string][]json.RawMessage)
	for _, item := range fixture.Cases {
		values[item.Schema] = append(values[item.Schema], item.Value)
	}
	return values
}

func routeSuccessCases(t *testing.T, route agentHTTPManifestRoute, goldens map[string][]json.RawMessage) []agentRouteSuccessCase {
	t.Helper()
	if route.Operation == "ready" {
		cases := make([]agentRouteSuccessCase, 0, len(route.Responses))
		for _, response := range route.Responses {
			body, err := json.Marshal(ReadyResponse{Status: response.StatusValue})
			if err != nil {
				t.Fatal(err)
			}
			cases = append(cases, agentRouteSuccessCase{Name: response.StatusValue, Status: response.Status, ResponseBody: body})
		}
		return cases
	}
	if route.Operation == "dialogue" {
		if route.RequestResponseVariants == nil || route.RequestResponseVariants.Discriminator != "terminal" {
			t.Fatal("dialogue manifest lacks terminal variants")
		}
		cases := make([]agentRouteSuccessCase, 0, len(route.RequestResponseVariants.Variants))
		for _, variant := range route.RequestResponseVariants.Variants {
			request := goldenWithBooleanField(t, goldens["dialogue_request"], "terminal", variant.Value)
			response := goldenWithObjectField(t, goldens["dialogue_response"], "memory_proposal", variant.MemoryProposal == "required")
			cases = append(cases, agentRouteSuccessCase{
				Name: fmt.Sprintf("terminal-%t", variant.Value), RequestBody: request,
				ResponseBody: response, Status: route.Responses[0].Status,
			})
		}
		return cases
	}
	if route.Operation == "memory_reconcile" {
		requests := goldens["memory_reconcile_request"]
		cases := make([]agentRouteSuccessCase, 0, len(requests))
		seen := make(map[bool]bool, len(requests))
		for _, request := range requests {
			active, ok := decodeAgentJSONObject(t, request)["active"].(bool)
			if !ok || seen[active] {
				t.Fatalf("memory reconcile golden has invalid or duplicate active discriminator %v", active)
			}
			seen[active] = true
			cases = append(cases, agentRouteSuccessCase{
				Name:         fmt.Sprintf("active-%t", active),
				RequestBody:  request,
				ResponseBody: goldenWithBooleanField(t, goldens["memory_reconcile_response"], "active", active),
				Status:       route.Responses[0].Status,
			})
		}
		if !seen[true] || !seen[false] {
			t.Fatal("memory reconcile goldens do not cover both schema variants")
		}
		return cases
	}
	if len(route.Responses) != 1 {
		t.Fatalf("operation %s has %d unhandled responses", route.Operation, len(route.Responses))
	}
	var request json.RawMessage
	if route.RequestSchema != nil {
		request = firstAgentGolden(t, goldens, *route.RequestSchema)
	}
	return []agentRouteSuccessCase{{
		Name: "success", RequestBody: request,
		ResponseBody: firstAgentGolden(t, goldens, route.Responses[0].Schema), Status: route.Responses[0].Status,
	}}
}

func firstAgentGolden(t *testing.T, goldens map[string][]json.RawMessage, schema string) json.RawMessage {
	t.Helper()
	if len(goldens[schema]) == 0 {
		t.Fatalf("valid golden lacks schema %q", schema)
	}
	return goldens[schema][0]
}

func goldenWithBooleanField(t *testing.T, values []json.RawMessage, field string, want bool) json.RawMessage {
	t.Helper()
	for _, value := range values {
		object := decodeAgentJSONObject(t, value)
		if got, ok := object[field].(bool); ok && got == want {
			return value
		}
	}
	t.Fatalf("golden lacks %s=%t", field, want)
	return nil
}

func goldenWithObjectField(t *testing.T, values []json.RawMessage, field string, want bool) json.RawMessage {
	t.Helper()
	for _, value := range values {
		object := decodeAgentJSONObject(t, value)
		_, present := object[field]
		if present == want {
			return value
		}
	}
	t.Fatalf("golden lacks %s presence=%t", field, want)
	return nil
}

func newManifestAgentClient(t *testing.T, handler http.Handler) *AgentClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "MANIFEST_AGENT_KEY"}, "manifest-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func fixedAgentResponseHandler(status int, body []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

func callAgentOperation(t *testing.T, client *AgentClient, operation string, body json.RawMessage) (any, error) {
	t.Helper()
	ctx := context.Background()
	switch operation {
	case "live":
		return client.Live(ctx)
	case "ready":
		return client.Ready(ctx)
	case "namespace_acquire":
		var request AcquireRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.Acquire(ctx, request)
	case "namespace_heartbeat":
		var request LeaseRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.Heartbeat(ctx, request)
	case "namespace_release":
		var request LeaseRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.Release(ctx, request)
	case "plan":
		var request PlanRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.Plan(ctx, request)
	case "dialogue":
		var request AgentDialogueRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.Dialogue(ctx, request)
	case "memory_reconcile":
		var request MemoryReconcileRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.ReconcileMemory(ctx, request)
	case "memory_commit":
		var request MemoryCommitRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.CommitMemory(ctx, request)
	case "memory_delete":
		var request MemoryDeleteRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.DeleteMemory(ctx, request)
	case "run_cancel":
		var request CancelRequest
		mustStrictDecodeAgent(t, body, &request)
		return client.CancelRun(ctx, request)
	default:
		t.Fatalf("public client has no dispatch for manifest operation %q", operation)
		return nil, nil
	}
}

func mustStrictDecodeAgent(t *testing.T, body []byte, out any) {
	t.Helper()
	if err := strictDecodeJSON(body, out); err != nil {
		t.Fatalf("decode golden request: %v", err)
	}
}

func assertManifestIdentityProfile(t *testing.T, manifest agentHTTPManifest, route agentHTTPManifestRoute, body []byte) {
	t.Helper()
	fields, ok := manifest.IdentityProfiles[route.IdentityProfile]
	if !ok {
		t.Fatalf("unknown identity profile %q", route.IdentityProfile)
	}
	object := decodeAgentJSONObject(t, body)
	for _, field := range fields {
		if _, exists := object[field]; !exists {
			t.Errorf("request lacks manifest identity %q", field)
		}
	}
	if len(fields) == 0 && len(object) != 0 {
		t.Errorf("health profile unexpectedly has body fields %v", sortedAgentKeys(object))
	}
}

func manifestRequestID(body []byte) *string {
	if len(body) == 0 {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return nil
	}
	var requestID string
	if json.Unmarshal(object["request_id"], &requestID) != nil {
		return nil
	}
	return &requestID
}

func marshalManifestError(t *testing.T, requestID *string, code string) []byte {
	t.Helper()
	body, err := json.Marshal(struct {
		ContractVersion string  `json:"contract_version"`
		RequestID       *string `json:"request_id"`
		Error           struct {
			Code string `json:"code"`
		} `json:"error"`
	}{ContractVersion: AgentContractVersion, RequestID: requestID, Error: struct {
		Code string `json:"code"`
	}{Code: code}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func assertZeroAgentResult(t *testing.T, got any) {
	t.Helper()
	if got == nil {
		t.Fatal("dispatch returned nil instead of its typed zero value")
	}
	want := reflect.Zero(reflect.TypeOf(got)).Interface()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failure returned partial value %+v", got)
	}
}

func assertAgentJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	gotValue := decodeAgentJSONValue(t, got)
	wantValue := decodeAgentJSONValue(t, want)
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

func decodeAgentJSONObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	value := decodeAgentJSONValue(t, body)
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("JSON is %T, want object", value)
	}
	return object
}

func decodeAgentJSONValue(t *testing.T, body []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode JSON %q: %v", body, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("JSON has trailing value: %v", err)
	}
	return value
}

func cloneAgentJSONObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	return decodeAgentJSONObject(t, append([]byte(nil), body...))
}

func mutateAgentJSON(t *testing.T, body []byte, mutate func(map[string]any)) json.RawMessage {
	t.Helper()
	object := cloneAgentJSONObject(t, body)
	mutate(object)
	return marshalAgentJSONObject(t, object)
}

func marshalAgentJSONObject(t *testing.T, object map[string]any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func differentAgentIdentityValue(t *testing.T, field string, value any) any {
	t.Helper()
	switch typed := value.(type) {
	case string:
		if field == "contract_version" {
			return "v2"
		}
		if field == "snapshot_digest" {
			if typed != strings.Repeat("f", 64) {
				return strings.Repeat("f", 64)
			}
			return strings.Repeat("e", 64)
		}
		candidate := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		if typed == candidate {
			candidate = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		}
		return candidate
	case json.Number:
		integer, err := typed.Int64()
		if err != nil {
			t.Fatal(err)
		}
		return json.Number(fmt.Sprintf("%d", integer+1))
	default:
		t.Fatalf("cannot mutate identity field %s of type %T", field, value)
		return nil
	}
}

func sortedAgentKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestAgentManifestFixtureHelpersStayMachineDriven(t *testing.T) {
	manifest := loadAgentHTTPManifest(t)
	if manifest.Limits["request_body_bytes"] != AgentMaxRequestBodyBytes ||
		manifest.Limits["response_body_bytes"] != AgentMaxResponseBodyBytes ||
		manifest.Limits["header_bytes"] != AgentMaxHeaderBytes {
		t.Fatalf("manifest limits drifted: %+v", manifest.Limits)
	}
	operations := make([]string, 0, len(manifest.Routes))
	for _, route := range manifest.Routes {
		operations = append(operations, route.Operation)
	}
	if got := strings.Join(operations, ","); got == "" {
		t.Fatal("manifest has no operations")
	}
}
