package companion

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/core"
)

// TestContractFixtureFilesExist 先锁定共享契约的固定入口。Go 与 Python 后续都从
// 这些 versioned 文件读取同一份 schema、manifest 与 golden，避免各自复制定义。
func TestContractFixtureFilesExist(t *testing.T) {
	t.Parallel()

	for _, relative := range []string{
		"contracts/companion-agent/http-v1/schema.json",
		"contracts/companion-agent/http-v1/manifest.json",
		"contracts/companion-agent/http-v1/golden/valid.json",
		"contracts/companion-agent/http-v1/golden/invalid.json",
		"contracts/companion-agent/mcp-v1/schema.json",
		"contracts/companion-agent/mcp-v1/manifest.json",
		"contracts/companion-agent/mcp-v1/golden/valid.json",
		"contracts/companion-agent/mcp-v1/golden/invalid.json",
		"contracts/companion-agent/mcp-v1/golden/mine-validation.json",
	} {
		data, err := os.ReadFile(contractFixturePath(t, relative))
		if err != nil {
			t.Fatalf("读取共享契约 %s: %v", relative, err)
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatalf("共享契约 %s 不是合法 JSON: %v", relative, err)
		}
	}
}

func contractFixturePath(t *testing.T, relative string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位共享契约测试文件")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", relative)
}

// TestContractFixtureSchemasValidateGoldens 让合法与非法样本真正经过同一份
// checked-in schema。测试只实现本契约使用的 JSON Schema 2020-12 子集，生产
// transport 不依赖这里的 helper；Python 与 Go 后续仍可直接使用标准 validator。
func TestContractFixtureSchemasValidateGoldens(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	for name, document := range schemas.documents {
		if got := contractString(t, document["$schema"], name+".$schema"); got != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s draft = %q，want JSON Schema 2020-12", name, got)
		}
		contractAssertStrictObjects(t, document, name)
	}

	for _, fixture := range []struct {
		document string
		path     string
		valid    bool
	}{
		{"http-v1/schema.json", "contracts/companion-agent/http-v1/golden/valid.json", true},
		{"http-v1/schema.json", "contracts/companion-agent/http-v1/golden/invalid.json", false},
		{"mcp-v1/schema.json", "contracts/companion-agent/mcp-v1/golden/valid.json", true},
		{"mcp-v1/schema.json", "contracts/companion-agent/mcp-v1/golden/invalid.json", false},
	} {
		golden := contractLoadGolden(t, fixture.path)
		if len(golden.Cases) == 0 {
			t.Fatalf("%s 没有 golden case", fixture.path)
		}
		seenNames := make(map[string]struct{}, len(golden.Cases))
		for _, testCase := range golden.Cases {
			if _, duplicate := seenNames[testCase.Name]; duplicate {
				t.Fatalf("%s case 名称重复: %q", fixture.path, testCase.Name)
			}
			seenNames[testCase.Name] = struct{}{}
			value := contractDecodeRaw(t, testCase.Value, fixture.path+"/"+testCase.Name)
			err := schemas.validateDefinition(fixture.document, testCase.Schema, value)
			if err == nil {
				err = contractValidateGoldenSemantics(testCase.Schema, value)
			}
			if fixture.valid && err != nil {
				t.Errorf("合法 golden %q 未通过 %s: %v", testCase.Name, testCase.Schema, err)
			}
			if !fixture.valid && err == nil {
				t.Errorf("非法 golden %q 被 %s 接受（reason=%s）", testCase.Name, testCase.Schema, testCase.Reason)
			}
		}
	}

	httpPlanRef := contractObject(t,
		contractObject(t, schemas.definition(t, "http-v1/schema.json", "plan_response"), "http.plan_response")["properties"],
		"http.plan_response.properties")
	planProperty := contractObject(t, httpPlanRef["plan"], "http.plan_response.properties.plan")
	if got := contractString(t, planProperty["$ref"], "http plan $ref"); got != "urn:mornlea:companion-agent:mcp-tools:v1#/$defs/plan" {
		t.Fatalf("HTTP plan schema 没有复用 MCP 单源: %q", got)
	}
}

// TestContractFixtureHTTPManifestConsistency 锁定健康、acquire、lease、run、
// memory、cancel 的身份例外，以及 method/path/status/error 和各层字节上限。
func TestContractFixtureHTTPManifestConsistency(t *testing.T) {
	t.Parallel()

	manifest := contractReadObject(t, "contracts/companion-agent/http-v1/manifest.json")
	if got := contractString(t, manifest["application_contract_version"], "http version"); got != "v1" {
		t.Fatalf("HTTP contract version = %q，want v1", got)
	}
	registry := contractArray(t, manifest["schema_registry"], "http schema registry")
	if len(registry) != 2 {
		t.Fatalf("HTTP schema registry 数 = %d，want 2", len(registry))
	}
	wantRegistry := []struct{ id, path string }{
		{"urn:mornlea:companion-agent:http:v1", "schema.json"},
		{"urn:mornlea:companion-agent:mcp-tools:v1", "../mcp-v1/schema.json"},
	}
	for index, want := range wantRegistry {
		entry := contractObject(t, registry[index], fmt.Sprintf("http schema registry[%d]", index))
		if got := contractString(t, entry["id"], "schema registry id"); got != want.id {
			t.Errorf("HTTP schema registry[%d] id = %q，want %q", index, got, want.id)
		}
		if got := contractString(t, entry["path"], "schema registry path"); got != want.path {
			t.Errorf("HTTP schema registry[%d] path = %q，want %q", index, got, want.path)
		}
	}
	contractAssertExactIntegers(t, contractObject(t, manifest["limits"], "http limits"), map[string]int64{
		"request_body_bytes":  262144,
		"response_body_bytes": 65536,
		"header_bytes":        16384,
		"instruction_bytes":   1024,
		"persona_bytes":       4096,
		"line_bytes":          256,
		"summary_bytes":       2048,
		"plan_bytes":          65536,
	})
	contractAssertExactIntegers(t, contractObject(t, manifest["lease"], "http lease"), map[string]int64{
		"ttl_ms":                15000,
		"heartbeat_interval_ms": 5000,
	})
	if contractBool(t, contractObject(t, manifest["lease"], "http lease")["reacquire_reuses_lease_id"], "http lease reuse") {
		t.Fatal("namespace reacquire 不得复用 lease ID")
	}
	contractAssertExactIntegers(t, contractObject(t, manifest["run_limits"], "http run limits"), map[string]int64{
		"global":                  4,
		"per_namespace_companion": 1,
		"queue_capacity":          0,
		"default_timeout_ms":      30000,
		"hard_timeout_ms":         60000,
	})

	wantProfiles := map[string][]string{
		"health":           {},
		"acquire":          {"contract_version", "request_id", "client_instance_id", "namespace_id"},
		"lease":            {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id"},
		"plan_run":         {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id", "companion_id", "generation", "snapshot_id", "snapshot_digest"},
		"dialogue_run":     {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id", "companion_id", "generation", "memory_epoch"},
		"memory_reconcile": {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id", "memory_epoch"},
		"memory_commit":    {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id", "memory_epoch", "base_revision", "operation_id"},
		"memory_delete":    {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id", "old_memory_epoch", "new_memory_epoch", "tombstone_operation_id"},
		"cancel":           {"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id"},
		"error":            {"contract_version", "request_id"},
	}
	profiles := contractObject(t, manifest["identity_profiles"], "http identity profiles")
	contractAssertExactKeys(t, profiles, contractMapKeys(wantProfiles), "HTTP identity profiles")
	for name, want := range wantProfiles {
		contractAssertStringList(t, profiles[name], want, "HTTP identity profile "+name)
	}

	type routeExpectation struct {
		method     string
		path       string
		profile    string
		request    any
		statuses   []int64
		errorCodes []string
	}
	wantRoutes := map[string]routeExpectation{
		"live":  {"GET", "/livez", "health", nil, []int64{200}, []string{}},
		"ready": {"GET", "/readyz", "health", nil, []int64{200, 503}, []string{"unauthorized", "internal_error"}},
		"namespace_acquire": {"POST", "/v1/namespaces/acquire", "acquire", "acquire_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "namespace_conflict", "internal_error"}},
		"namespace_heartbeat": {"POST", "/v1/namespaces/heartbeat", "lease", "lease_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "not_found", "internal_error"}},
		"namespace_release": {"POST", "/v1/namespaces/release", "lease", "lease_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "not_found", "internal_error"}},
		"plan": {"POST", "/v1/plan", "plan_run", "plan_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "overloaded", "deadline_exceeded", "agent_unavailable", "invalid_model_output", "not_found", "internal_error"}},
		"dialogue": {"POST", "/v1/dialogue", "dialogue_run", "dialogue_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "overloaded", "deadline_exceeded", "agent_unavailable", "invalid_model_output", "memory_conflict", "not_found", "internal_error"}},
		"memory_reconcile": {"POST", "/v1/memory/reconcile", "memory_reconcile", "memory_reconcile_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "memory_conflict", "not_found", "internal_error"}},
		"memory_commit": {"POST", "/v1/memory/commit", "memory_commit", "memory_commit_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "memory_conflict", "not_found", "internal_error"}},
		"memory_delete": {"POST", "/v1/memory/delete", "memory_delete", "memory_delete_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "memory_conflict", "not_found", "internal_error"}},
		"run_cancel": {"POST", "/v1/runs/cancel", "cancel", "cancel_request", []int64{200},
			[]string{"invalid_request", "unauthorized", "unsupported_version", "not_found", "internal_error"}},
	}
	schemas := contractLoadSchemas(t)
	routes := contractArray(t, manifest["routes"], "http routes")
	if len(routes) != len(wantRoutes) {
		t.Fatalf("HTTP route 数 = %d，want %d", len(routes), len(wantRoutes))
	}
	seenRoutes := make(map[string]struct{}, len(routes))
	seenMethodPath := make(map[string]struct{}, len(routes))
	for index, rawRoute := range routes {
		route := contractObject(t, rawRoute, fmt.Sprintf("http routes[%d]", index))
		operation := contractString(t, route["operation"], "http route operation")
		want, ok := wantRoutes[operation]
		if !ok {
			t.Fatalf("HTTP route 出现未知 operation %q", operation)
		}
		if _, duplicate := seenRoutes[operation]; duplicate {
			t.Fatalf("HTTP route operation 重复: %q", operation)
		}
		seenRoutes[operation] = struct{}{}
		method := contractString(t, route["method"], operation+" method")
		path := contractString(t, route["path"], operation+" path")
		if method != want.method || path != want.path {
			t.Errorf("HTTP route %s = %s %s，want %s %s", operation, method, path, want.method, want.path)
		}
		methodPath := method + " " + path
		if _, duplicate := seenMethodPath[methodPath]; duplicate {
			t.Fatalf("HTTP method/path 重复: %s", methodPath)
		}
		seenMethodPath[methodPath] = struct{}{}
		if got := contractString(t, route["identity_profile"], operation+" profile"); got != want.profile {
			t.Errorf("HTTP route %s identity profile = %q，want %q", operation, got, want.profile)
		}
		if want.request == nil {
			if route["request_schema"] != nil {
				t.Errorf("HTTP health route %s 不应有 request schema", operation)
			}
		} else {
			requestName := contractString(t, route["request_schema"], operation+" request schema")
			if requestName != want.request {
				t.Errorf("HTTP route %s request schema = %q，want %q", operation, requestName, want.request)
			}
			schemas.definition(t, "http-v1/schema.json", requestName)
		}
		responses := contractArray(t, route["responses"], operation+" responses")
		if len(responses) != len(want.statuses) {
			t.Errorf("HTTP route %s response 数 = %d，want %d", operation, len(responses), len(want.statuses))
		}
		for responseIndex, rawResponse := range responses {
			response := contractObject(t, rawResponse, operation+" response")
			status := contractInt64(t, response["status"], operation+" status")
			if responseIndex >= len(want.statuses) || status != want.statuses[responseIndex] {
				t.Errorf("HTTP route %s response[%d] status = %d，want %v", operation, responseIndex, status, want.statuses)
			}
			schemas.definition(t, "http-v1/schema.json", contractString(t, response["schema"], operation+" response schema"))
		}
		contractAssertStringList(t, route["error_codes"], want.errorCodes, "HTTP route "+operation+" error codes")
		wantAuthentication := "bearer"
		if operation == "live" {
			wantAuthentication = "none"
		}
		if got := contractString(t, route["authentication"], operation+" authentication"); got != wantAuthentication {
			t.Errorf("HTTP route %s authentication = %q，want %q", operation, got, wantAuthentication)
		}
	}

	wantErrors := map[string]int64{
		"invalid_request":      400,
		"unauthorized":         401,
		"unsupported_version":  426,
		"namespace_conflict":   409,
		"overloaded":           429,
		"deadline_exceeded":    504,
		"agent_unavailable":    503,
		"invalid_model_output": 422,
		"memory_conflict":      409,
		"not_found":            404,
		"internal_error":       500,
	}
	gotErrors := make(map[string]int64)
	for _, rawError := range contractArray(t, manifest["errors"], "http errors") {
		entry := contractObject(t, rawError, "http error")
		code := contractString(t, entry["code"], "http error code")
		if _, duplicate := gotErrors[code]; duplicate {
			t.Fatalf("HTTP error code 重复: %q", code)
		}
		gotErrors[code] = contractInt64(t, entry["status"], "http error status")
	}
	if len(gotErrors) != len(wantErrors) {
		t.Fatalf("HTTP error 数 = %d，want %d", len(gotErrors), len(wantErrors))
	}
	for code, wantStatus := range wantErrors {
		if gotErrors[code] != wantStatus {
			t.Errorf("HTTP error %s status = %d，want %d", code, gotErrors[code], wantStatus)
		}
	}
	errorSchema := contractObject(t, schemas.definition(t, "http-v1/schema.json", "error_response"), "http error response")
	errorProperties := contractObject(t, errorSchema["properties"], "http error response properties")
	errorPayload := contractObject(t, errorProperties["error"], "http error payload")
	errorPayloadProperties := contractObject(t, errorPayload["properties"], "http error payload properties")
	errorCodeSchema := contractObject(t, errorPayloadProperties["code"], "http error code schema")
	errorCodeSet := contractStringSet(t, errorCodeSchema["enum"], "http error enum")
	if len(errorCodeSet) != len(wantErrors) {
		t.Errorf("HTTP error schema code 数 = %d，want %d", len(errorCodeSet), len(wantErrors))
	}
	for code := range wantErrors {
		if _, ok := errorCodeSet[code]; !ok {
			t.Errorf("HTTP error schema 缺少稳定 code %s", code)
		}
	}
	if got := contractString(t, manifest["error_schema"], "http error schema"); got != "error_response" {
		t.Fatalf("HTTP error schema = %q，want error_response", got)
	}
	if manifest["unauthenticated_request_id"] != nil {
		t.Fatal("认证或 strict request ID 解析前的错误必须使用 null request ID")
	}

	contractAssertSchemaFields(t, schemas, "acquire_request", wantProfiles["acquire"], []string{"lease_id"})
	contractAssertSchemaFields(t, schemas, "lease_request", wantProfiles["lease"], []string{"run_id", "companion_id", "generation"})
	contractAssertSchemaContainsRequired(t, schemas, "plan_request", wantProfiles["plan_run"])
	contractAssertSchemaContainsRequired(t, schemas, "dialogue_request", wantProfiles["dialogue_run"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_commit_request", wantProfiles["memory_commit"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_delete_request", wantProfiles["memory_delete"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_reconcile_active_request", wantProfiles["memory_reconcile"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_reconcile_inactive_request", wantProfiles["memory_reconcile"])
	contractAssertSchemaFields(t, schemas, "cancel_request", wantProfiles["cancel"], []string{"companion_id", "generation", "memory_epoch"})
	contractAssertSchemaFields(t, schemas, "error_response", append(wantProfiles["error"], "error"), []string{"client_instance_id", "namespace_id", "lease_id", "run_id", "companion_id"})
}

// TestContractFixtureMCPManifestConsistency 锁定共同 wire、SDK 前 allowlist、仅
// Tools capability、六个只读/纯校验工具以及 canonical 与双份 wire 上限。
func TestContractFixtureMCPManifestConsistency(t *testing.T) {
	t.Parallel()

	manifest := contractReadObject(t, "contracts/companion-agent/mcp-v1/manifest.json")
	if got := contractString(t, manifest["application_contract_version"], "mcp application version"); got != "v1" {
		t.Fatalf("MCP application version = %q，want v1", got)
	}
	if got := contractString(t, manifest["mcp_protocol_version"], "mcp protocol version"); got != "2025-11-25" {
		t.Fatalf("MCP wire version = %q，want 2025-11-25", got)
	}
	if got := contractString(t, manifest["transport"], "mcp transport"); got != "streamable_http" {
		t.Fatalf("MCP transport = %q，want streamable_http", got)
	}
	for field, want := range map[string]bool{"stateless": true, "json_response": true, "sse": false, "sessions": false} {
		if got := contractBool(t, manifest[field], "mcp "+field); got != want {
			t.Errorf("MCP %s = %v，want %v", field, got, want)
		}
	}
	origin := contractObject(t, manifest["origin"], "mcp origin")
	if !contractBool(t, origin["missing_allowed"], "mcp missing Origin") {
		t.Fatal("Python httpx 不带 Origin 时必须合法")
	}
	if got := contractString(t, origin["present_value"], "mcp present Origin"); got != "listener_loopback_origin" {
		t.Fatalf("MCP present Origin policy = %q", got)
	}

	methods := contractObject(t, manifest["methods"], "mcp methods")
	contractAssertStringList(t, methods["allowed"], []string{"initialize", "notifications/initialized", "tools/list", "tools/call"}, "MCP allowed methods")
	contractAssertStringList(t, methods["rejected"], []string{"ping", "subscriptions/listen"}, "MCP explicitly rejected methods")
	contractAssertStringList(t, methods["http_methods"], []string{"POST"}, "MCP HTTP methods")
	if contractBool(t, methods["batch_allowed"], "MCP batch") {
		t.Fatal("MCP JSON-RPC batch 必须在 SDK 前被拒绝")
	}

	capabilities := contractObject(t, manifest["capabilities"], "mcp capabilities")
	contractAssertExactKeys(t, capabilities, []string{"tools", "logging", "prompts", "resources"}, "MCP capabilities")
	toolsCapability := contractObject(t, capabilities["tools"], "mcp tools capability")
	if contractBool(t, toolsCapability["listChanged"], "mcp listChanged") {
		t.Fatal("MCP Tools.listChanged 必须为 false")
	}
	for _, name := range []string{"logging", "prompts", "resources"} {
		if contractBool(t, capabilities[name], "mcp capability "+name) {
			t.Errorf("MCP 不得广告 %s capability", name)
		}
	}

	limits := contractObject(t, manifest["limits"], "mcp limits")
	contractAssertExactIntegers(t, limits, map[string]int64{
		"request_body_bytes":   262144,
		"wire_response_bytes":  163840,
		"plan_input_bytes":     65536,
		"validator_hint_bytes": 256,
	})
	if !contractBool(t, limits["structured_content"], "mcp structured content") ||
		!contractBool(t, limits["json_text_content_fallback"], "mcp text fallback") {
		t.Fatal("MCP 必须同时生成 StructuredContent 与 JSON TextContent fallback")
	}

	type toolExpectation struct {
		input     string
		result    string
		canonical int64
		model     bool
		fixed     bool
	}
	wantTools := map[string]toolExpectation{
		"get_planning_context": {"get_planning_context_input", "get_planning_context_result", 24576, false, true},
		"list_affordances":     {"list_affordances_input", "list_affordances_result", 24576, true, false},
		"inspect_inventory":    {"inspect_inventory_input", "inspect_inventory_result", 8192, true, false},
		"find_visible_blocks":  {"find_visible_blocks_input", "find_visible_blocks_result", 16384, true, false},
		"query_terrain":        {"query_terrain_input", "query_terrain_result", 16384, true, false},
		"validate_plan":        {"validate_plan_input", "validate_plan_result", 73728, false, true},
	}
	schemas := contractLoadSchemas(t)
	tools := contractArray(t, manifest["tools"], "mcp tools")
	if len(tools) != 6 {
		t.Fatalf("MCP tool 数 = %d，want 6", len(tools))
	}
	seenTools := make(map[string]struct{}, len(tools))
	maxCanonical := int64(0)
	for index, rawTool := range tools {
		tool := contractObject(t, rawTool, fmt.Sprintf("mcp tools[%d]", index))
		name := contractString(t, tool["name"], "mcp tool name")
		want, ok := wantTools[name]
		if !ok {
			t.Fatalf("MCP 暴露未知工具 %q", name)
		}
		if _, duplicate := seenTools[name]; duplicate {
			t.Fatalf("MCP 工具重复: %q", name)
		}
		seenTools[name] = struct{}{}
		input := contractString(t, tool["input_schema"], name+" input schema")
		result := contractString(t, tool["result_schema"], name+" result schema")
		if input != want.input || result != want.result {
			t.Errorf("MCP tool %s schema = %s/%s，want %s/%s", name, input, result, want.input, want.result)
		}
		schemas.definition(t, "mcp-v1/schema.json", input)
		schemas.definition(t, "mcp-v1/schema.json", result)
		if got := contractInt64(t, tool["canonical_result_bytes"], name+" canonical result"); got != want.canonical {
			t.Errorf("MCP tool %s canonical result = %d，want %d", name, got, want.canonical)
		} else if got > maxCanonical {
			maxCanonical = got
		}
		if got := contractBool(t, tool["model_visible"], name+" model visible"); got != want.model {
			t.Errorf("MCP tool %s model_visible = %v，want %v", name, got, want.model)
		}
		if got := contractBool(t, tool["fixed_graph_call"], name+" fixed graph call"); got != want.fixed {
			t.Errorf("MCP tool %s fixed_graph_call = %v，want %v", name, got, want.fixed)
		}
	}
	if maxCanonical != 72<<10 {
		t.Fatalf("最大 canonical tool result = %d，want 72 KiB", maxCanonical)
	}
	wireLimit := contractInt64(t, limits["wire_response_bytes"], "mcp wire response")
	if wireLimit != 160<<10 || wireLimit <= 2*maxCanonical {
		t.Fatalf("MCP 双份 wire 上限 = %d，必须是 160 KiB 且大于 2×72 KiB", wireLimit)
	}
	validateTool := contractFindNamedObject(t, tools, "validate_plan", "mcp tools")
	if got := contractInt64(t, validateTool["canonical_input_bytes"], "validate_plan input bytes"); got != 64<<10 {
		t.Fatalf("validate_plan canonical input = %d，want 64 KiB", got)
	}
	if got := contractInt64(t, validateTool["maximum_calls_per_run"], "validate_plan calls"); got != 2 {
		t.Fatalf("validate_plan calls/run = %d，want 2", got)
	}

	wantCodes := []string{"invalid_schema", "out_of_bounds", "unknown_player", "unmineable_target", "unknown_block", "missing_item", "snapshot_mismatch"}
	contractAssertStringList(t, manifest["validator_codes"], wantCodes, "MCP validator codes")
	validatorResult := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", "validate_plan_result"), "validate_plan result")
	validatorBranches := contractArray(t, validatorResult["oneOf"], "validate_plan result oneOf")
	if len(validatorBranches) != 2 {
		t.Fatalf("validate_plan result oneOf 分支 = %d，want 2", len(validatorBranches))
	}
	failureBranch := contractObject(t, validatorBranches[1], "validate_plan failure branch")
	failureProperties := contractObject(t, failureBranch["properties"], "validate_plan failure properties")
	validatorCodeSchema := contractObject(t, failureProperties["code"], "validate_plan failure code")
	contractAssertStringList(t, validatorCodeSchema["enum"], wantCodes, "validate_plan schema codes")
	runtimeFields := []string{"snapshot_id", "namespace_id", "companion_id", "capability"}
	contractAssertStringList(t, manifest["runtime_injected_fields"], runtimeFields, "MCP runtime-injected identity")
	for name, want := range wantTools {
		inputSchema := schemas.definition(t, "mcp-v1/schema.json", want.input)
		for _, field := range runtimeFields {
			if contractSchemaMentionsProperty(inputSchema, field) {
				t.Errorf("MCP model-visible schema %s 不得声明 runtime field %s", name, field)
			}
		}
	}

	validGolden := contractLoadGolden(t, "contracts/companion-agent/mcp-v1/golden/valid.json")
	covered := make(map[string]struct{}, len(validGolden.Cases))
	for _, testCase := range validGolden.Cases {
		covered[testCase.Schema] = struct{}{}
	}
	for _, want := range wantTools {
		for _, schemaName := range []string{want.input, want.result} {
			if _, ok := covered[schemaName]; !ok {
				t.Errorf("MCP schema %s 缺少合法 golden", schemaName)
			}
		}
	}

	dependencies := contractObject(t, manifest["dependencies"], "mcp dependencies")
	if got := contractString(t, dependencies["go_mcp_sdk"], "go mcp sdk"); got != "v1.7.0" {
		t.Errorf("Go MCP SDK pin = %q，want v1.7.0", got)
	}
	if got := contractString(t, dependencies["python_mcp"], "python mcp range"); got != ">=1.28.1,<2" {
		t.Errorf("Python MCP range = %q，want >=1.28.1,<2", got)
	}
}

// TestContractFixtureMineValidationMatchesAuthority 把跨语言 mine golden 与当前
// Go 权威规则逐项对照，防止服务抽离时误拒 Chest/Furnace 或放开未交付语义。
func TestContractFixtureMineValidationMatchesAuthority(t *testing.T) {
	t.Parallel()

	document := contractReadObject(t, "contracts/companion-agent/mcp-v1/golden/mine-validation.json")
	if got := contractString(t, document["failure_code"], "mine failure code"); got != "unmineable_target" {
		t.Fatalf("mine failure code = %q，want unmineable_target", got)
	}
	blocks := map[string]core.BlockID{
		"StoneID":         core.StoneID,
		"FurnaceID":       core.FurnaceID,
		"ChestID":         core.ChestID,
		"FarmlandDryID":   core.FarmlandDryID,
		"WheatStage0ID":   core.WheatStage0ID,
		"WheatStage7ID":   core.WheatStage7ID,
		"TorchStandingID": core.TorchStandingID,
		"BedrockID":       core.BedrockID,
	}
	wantSemantics := map[string]bool{
		"single_drop":            true,
		"container_batch":        true,
		"forbidden_farming":      false,
		"forbidden_torch":        false,
		"no_drop":                false,
		"undelivered_multi_drop": false,
	}
	seenSymbols := make(map[string]struct{}, len(blocks))
	seenSemantics := make(map[string]struct{}, len(wantSemantics))
	for index, rawCase := range contractArray(t, document["cases"], "mine cases") {
		testCase := contractObject(t, rawCase, fmt.Sprintf("mine cases[%d]", index))
		symbol := contractString(t, testCase["block_symbol"], "mine block symbol")
		block, ok := blocks[symbol]
		if !ok {
			t.Fatalf("mine golden 出现未知 block symbol %q", symbol)
		}
		if _, duplicate := seenSymbols[symbol]; duplicate {
			t.Fatalf("mine golden block symbol 重复: %q", symbol)
		}
		seenSymbols[symbol] = struct{}{}
		if got := contractInt64(t, testCase["block_id"], symbol+" block ID"); got != int64(block) {
			t.Errorf("mine golden %s ID = %d，Go = %d", symbol, got, block)
		}
		semantics := contractString(t, testCase["mine_semantics"], symbol+" semantics")
		wantAccepted, ok := wantSemantics[semantics]
		if !ok {
			t.Fatalf("mine golden %s 语义分类未知: %q", symbol, semantics)
		}
		seenSemantics[semantics] = struct{}{}
		accepted := contractBool(t, testCase["accepted"], symbol+" accepted")
		if accepted != wantAccepted {
			t.Errorf("mine golden %s accepted = %v，分类 %s want %v", symbol, accepted, semantics, wantAccepted)
		}
		if got := planMineableBlock(block); got != accepted {
			t.Errorf("planMineableBlock(%s) = %v，golden = %v", symbol, got, accepted)
		}
		_, hasDrop := core.BlockDrop(block)
		if got := contractBool(t, testCase["has_block_drop"], symbol+" block drop"); got != hasDrop {
			t.Errorf("core.BlockDrop(%s) = %v，golden = %v", symbol, hasDrop, got)
		}
		if accepted {
			if testCase["code"] != nil {
				t.Errorf("mine accepted case %s 必须使用 null code", symbol)
			}
		} else if got := contractString(t, testCase["code"], symbol+" failure code"); got != "unmineable_target" {
			t.Errorf("mine rejected case %s code = %q", symbol, got)
		}
	}
	if len(seenSymbols) != len(blocks) {
		t.Errorf("mine golden 覆盖 block symbol = %d，want %d", len(seenSymbols), len(blocks))
	}
	if len(seenSemantics) != len(wantSemantics) {
		t.Errorf("mine golden 覆盖语义分类 = %d，want %d", len(seenSemantics), len(wantSemantics))
	}
}

type contractGolden struct {
	Contract string `json:"contract"`
	Cases    []struct {
		Name   string          `json:"name"`
		Schema string          `json:"schema"`
		Reason string          `json:"reason,omitempty"`
		Value  json.RawMessage `json:"value"`
	} `json:"cases"`
}

type contractSchemaSet struct {
	documents map[string]map[string]any
}

func contractLoadSchemas(t *testing.T) contractSchemaSet {
	t.Helper()
	return contractSchemaSet{documents: map[string]map[string]any{
		"http-v1/schema.json": contractReadObject(t, "contracts/companion-agent/http-v1/schema.json"),
		"mcp-v1/schema.json":  contractReadObject(t, "contracts/companion-agent/mcp-v1/schema.json"),
	}}
}

func (schemas contractSchemaSet) definition(t *testing.T, document, name string) any {
	t.Helper()
	root, ok := schemas.documents[document]
	if !ok {
		t.Fatalf("共享契约缺少 schema document %q", document)
	}
	definitions := contractObject(t, root["$defs"], document+".$defs")
	definition, ok := definitions[name]
	if !ok {
		t.Fatalf("%s 缺少 schema definition %q", document, name)
	}
	return definition
}

func (schemas contractSchemaSet) validateDefinition(document, name string, value any) error {
	root, ok := schemas.documents[document]
	if !ok {
		return fmt.Errorf("缺少 schema document %q", document)
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s 缺少 $defs", document)
	}
	schema, ok := definitions[name]
	if !ok {
		return fmt.Errorf("%s 缺少 definition %q", document, name)
	}
	return schemas.validate(document, schema, value, "$")
}

// validate 实现本 change schema 实际使用的 JSON Schema 2020-12 关键字。它
// 刻意留在测试文件，作用是交叉检查 fixtures，不成为未来 transport 的旁路。
func (schemas contractSchemaSet) validate(document string, rawSchema, value any, valuePath string) error {
	switch schema := rawSchema.(type) {
	case bool:
		if schema {
			return nil
		}
		return fmt.Errorf("%s 被 false schema 拒绝", valuePath)
	case map[string]any:
		if rawRef, ok := schema["$ref"]; ok {
			ref, ok := rawRef.(string)
			if !ok {
				return fmt.Errorf("%s $ref 不是字符串", valuePath)
			}
			targetDocument, target, err := schemas.resolveRef(document, ref)
			if err != nil {
				return fmt.Errorf("%s: %w", valuePath, err)
			}
			return schemas.validate(targetDocument, target, value, valuePath)
		}
		if rawOneOf, ok := schema["oneOf"]; ok {
			branches, ok := rawOneOf.([]any)
			if !ok || len(branches) == 0 {
				return fmt.Errorf("%s oneOf 非法", valuePath)
			}
			matched := 0
			for _, branch := range branches {
				if schemas.validate(document, branch, value, valuePath) == nil {
					matched++
				}
			}
			if matched != 1 {
				return fmt.Errorf("%s 匹配 oneOf 分支数 %d，want 1", valuePath, matched)
			}
		}
		if rawType, ok := schema["type"]; ok {
			typeName, ok := rawType.(string)
			if !ok {
				return fmt.Errorf("%s schema type 不是字符串", valuePath)
			}
			if err := contractValidateJSONType(typeName, value); err != nil {
				return fmt.Errorf("%s: %w", valuePath, err)
			}
		}
		if expected, ok := schema["const"]; ok && !contractJSONEqual(expected, value) {
			return fmt.Errorf("%s 不等于 const", valuePath)
		}
		if rawEnum, ok := schema["enum"]; ok {
			values, ok := rawEnum.([]any)
			if !ok {
				return fmt.Errorf("%s enum 非数组", valuePath)
			}
			found := false
			for _, candidate := range values {
				if contractJSONEqual(candidate, value) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s 不在 enum 中", valuePath)
			}
		}
		if text, ok := value.(string); ok {
			if rawMin, exists := schema["minLength"]; exists && utf8.RuneCountInString(text) < int(contractSchemaInteger(rawMin)) {
				return fmt.Errorf("%s 短于 minLength", valuePath)
			}
			if rawMax, exists := schema["maxLength"]; exists && utf8.RuneCountInString(text) > int(contractSchemaInteger(rawMax)) {
				return fmt.Errorf("%s 长于 maxLength", valuePath)
			}
			if rawPattern, exists := schema["pattern"]; exists {
				pattern, ok := rawPattern.(string)
				if !ok {
					return fmt.Errorf("%s pattern 不是字符串", valuePath)
				}
				compiled, err := regexp.Compile(pattern)
				if err != nil {
					return fmt.Errorf("%s pattern 非法: %w", valuePath, err)
				}
				if !compiled.MatchString(text) {
					return fmt.Errorf("%s 不匹配 pattern", valuePath)
				}
			}
			if format, exists := schema["format"]; exists && format == "uri" {
				parsed, err := url.Parse(text)
				if err != nil || parsed.Scheme == "" || parsed.Host == "" {
					return fmt.Errorf("%s 不是 absolute URI", valuePath)
				}
			}
		}
		if number, ok := value.(json.Number); ok {
			if rawMin, exists := schema["minimum"]; exists {
				comparison, err := contractCompareJSONNumbers(number, rawMin)
				if err != nil || comparison < 0 {
					return fmt.Errorf("%s 小于 minimum", valuePath)
				}
			}
			if rawMax, exists := schema["maximum"]; exists {
				comparison, err := contractCompareJSONNumbers(number, rawMax)
				if err != nil || comparison > 0 {
					return fmt.Errorf("%s 大于 maximum", valuePath)
				}
			}
		}
		if array, ok := value.([]any); ok {
			if rawMin, exists := schema["minItems"]; exists && len(array) < int(contractSchemaInteger(rawMin)) {
				return fmt.Errorf("%s 少于 minItems", valuePath)
			}
			if rawMax, exists := schema["maxItems"]; exists && len(array) > int(contractSchemaInteger(rawMax)) {
				return fmt.Errorf("%s 多于 maxItems", valuePath)
			}
			if unique, _ := schema["uniqueItems"].(bool); unique {
				seen := make(map[string]struct{}, len(array))
				for index, item := range array {
					encoded, _ := json.Marshal(item)
					key := string(encoded)
					if _, duplicate := seen[key]; duplicate {
						return fmt.Errorf("%s[%d] 违反 uniqueItems", valuePath, index)
					}
					seen[key] = struct{}{}
				}
			}
			prefix, _ := schema["prefixItems"].([]any)
			for index, item := range array {
				var itemSchema any
				hasSchema := false
				if index < len(prefix) {
					itemSchema, hasSchema = prefix[index], true
				} else if rawItems, exists := schema["items"]; exists {
					itemSchema, hasSchema = rawItems, true
				}
				if hasSchema {
					if err := schemas.validate(document, itemSchema, item, fmt.Sprintf("%s[%d]", valuePath, index)); err != nil {
						return err
					}
				}
			}
		}
		if object, ok := value.(map[string]any); ok {
			if rawRequired, exists := schema["required"]; exists {
				required, ok := rawRequired.([]any)
				if !ok {
					return fmt.Errorf("%s required 非数组", valuePath)
				}
				for _, rawName := range required {
					name, ok := rawName.(string)
					if !ok {
						return fmt.Errorf("%s required 成员不是字符串", valuePath)
					}
					if _, exists := object[name]; !exists {
						return fmt.Errorf("%s 缺少 required property %s", valuePath, name)
					}
				}
			}
			properties, _ := schema["properties"].(map[string]any)
			for name, member := range object {
				propertySchema, known := properties[name]
				if !known {
					if additional, exists := schema["additionalProperties"]; exists && additional == false {
						return fmt.Errorf("%s 含未知 property %s", valuePath, name)
					}
					continue
				}
				if err := schemas.validate(document, propertySchema, member, valuePath+"."+name); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("%s schema 不是 object 或 boolean", valuePath)
	}
}

func (schemas contractSchemaSet) resolveRef(document, ref string) (string, any, error) {
	parts := strings.SplitN(ref, "#", 2)
	targetDocument := document
	if parts[0] != "" {
		if strings.HasPrefix(parts[0], "urn:") {
			targetDocument = ""
			for candidate, root := range schemas.documents {
				if root["$id"] == parts[0] {
					targetDocument = candidate
					break
				}
			}
			if targetDocument == "" {
				return "", nil, fmt.Errorf("$ref %q 指向未注册的 $id", ref)
			}
		} else {
			targetDocument = pathpkg.Clean(pathpkg.Join(pathpkg.Dir(document), parts[0]))
		}
	}
	root, ok := schemas.documents[targetDocument]
	if !ok {
		return "", nil, fmt.Errorf("$ref %q 指向未知 document %q", ref, targetDocument)
	}
	var current any = root
	if len(parts) == 1 || parts[1] == "" {
		return targetDocument, current, nil
	}
	pointer := parts[1]
	if !strings.HasPrefix(pointer, "/") {
		return "", nil, fmt.Errorf("$ref %q 不是 JSON pointer", ref)
	}
	for _, escaped := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		name := strings.ReplaceAll(strings.ReplaceAll(escaped, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("$ref %q 在 %q 前不是 object", ref, name)
		}
		current, ok = object[name]
		if !ok {
			return "", nil, fmt.Errorf("$ref %q 缺少成员 %q", ref, name)
		}
	}
	return targetDocument, current, nil
}

func contractReadObject(t *testing.T, relative string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(contractFixturePath(t, relative))
	if err != nil {
		t.Fatalf("读取共享契约 %s: %v", relative, err)
	}
	value := contractDecodeRaw(t, data, relative)
	return contractObject(t, value, relative)
}

func contractLoadGolden(t *testing.T, relative string) contractGolden {
	t.Helper()
	data, err := os.ReadFile(contractFixturePath(t, relative))
	if err != nil {
		t.Fatalf("读取 golden %s: %v", relative, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var golden contractGolden
	if err := decoder.Decode(&golden); err != nil {
		t.Fatalf("解码 golden %s: %v", relative, err)
	}
	if err := contractRequireJSONEOF(decoder); err != nil {
		t.Fatalf("golden %s 含尾随数据: %v", relative, err)
	}
	if golden.Contract == "" {
		t.Fatalf("golden %s 缺少 contract", relative)
	}
	for index, testCase := range golden.Cases {
		if testCase.Name == "" || testCase.Schema == "" || len(testCase.Value) == 0 {
			t.Fatalf("golden %s cases[%d] 缺少 name/schema/value", relative, index)
		}
	}
	return golden
}

func contractDecodeRaw(t *testing.T, data []byte, label string) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("%s 不是合法 JSON: %v", label, err)
	}
	if err := contractRequireJSONEOF(decoder); err != nil {
		t.Fatalf("%s 含尾随 JSON: %v", label, err)
	}
	return value
}

func contractRequireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("存在第二个 JSON value")
		}
		return err
	}
	return nil
}

func contractObject(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s 不是 JSON object（%T）", label, value)
	}
	return object
}

func contractArray(t *testing.T, value any, label string) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("%s 不是 JSON array（%T）", label, value)
	}
	return array
}

func contractString(t *testing.T, value any, label string) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("%s 不是 JSON string（%T）", label, value)
	}
	return text
}

func contractBool(t *testing.T, value any, label string) bool {
	t.Helper()
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("%s 不是 JSON boolean（%T）", label, value)
	}
	return flag
}

func contractInt64(t *testing.T, value any, label string) int64 {
	t.Helper()
	number, ok := value.(json.Number)
	if !ok {
		t.Fatalf("%s 不是 JSON number（%T）", label, value)
	}
	integer, err := number.Int64()
	if err != nil {
		t.Fatalf("%s 不是 int64: %v", label, err)
	}
	return integer
}

func contractAssertExactIntegers(t *testing.T, object map[string]any, want map[string]int64) {
	t.Helper()
	for name, expected := range want {
		if got := contractInt64(t, object[name], name); got != expected {
			t.Errorf("%s = %d，want %d", name, got, expected)
		}
	}
}

func contractAssertStringList(t *testing.T, value any, want []string, label string) {
	t.Helper()
	array := contractArray(t, value, label)
	if len(array) != len(want) {
		t.Fatalf("%s 长度 = %d，want %d", label, len(array), len(want))
	}
	for index, expected := range want {
		if got := contractString(t, array[index], fmt.Sprintf("%s[%d]", label, index)); got != expected {
			t.Errorf("%s[%d] = %q，want %q", label, index, got, expected)
		}
	}
}

func contractAssertExactKeys(t *testing.T, object map[string]any, want []string, label string) {
	t.Helper()
	if len(object) != len(want) {
		t.Fatalf("%s key 数 = %d，want %d", label, len(object), len(want))
	}
	for _, name := range want {
		if _, ok := object[name]; !ok {
			t.Errorf("%s 缺少 key %q", label, name)
		}
	}
}

func contractMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func contractFindNamedObject(t *testing.T, values []any, name, label string) map[string]any {
	t.Helper()
	for index, value := range values {
		object := contractObject(t, value, fmt.Sprintf("%s[%d]", label, index))
		if contractString(t, object["name"], label+" name") == name {
			return object
		}
	}
	t.Fatalf("%s 找不到 name=%q", label, name)
	return nil
}

func contractAssertStrictObjects(t *testing.T, value any, valuePath string) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		if current["type"] == "object" {
			additional, ok := current["additionalProperties"]
			if !ok || additional != false {
				t.Errorf("object schema %s 必须显式 additionalProperties=false", valuePath)
			}
			if _, ok := current["properties"].(map[string]any); !ok {
				t.Errorf("object schema %s 必须声明 properties object", valuePath)
			}
		}
		for name, child := range current {
			contractAssertStrictObjects(t, child, valuePath+"/"+name)
		}
	case []any:
		for index, child := range current {
			contractAssertStrictObjects(t, child, fmt.Sprintf("%s/%d", valuePath, index))
		}
	}
}

func contractAssertSchemaContainsRequired(t *testing.T, schemas contractSchemaSet, definition string, want []string) {
	t.Helper()
	schema := contractObject(t, schemas.definition(t, "http-v1/schema.json", definition), "http "+definition)
	required := contractStringSet(t, schema["required"], definition+" required")
	for _, name := range want {
		if _, ok := required[name]; !ok {
			t.Errorf("HTTP schema %s required 缺少 %s", definition, name)
		}
	}
}

func contractAssertSchemaFields(t *testing.T, schemas contractSchemaSet, definition string, requiredFields, forbiddenFields []string) {
	t.Helper()
	schema := contractObject(t, schemas.definition(t, "http-v1/schema.json", definition), "http "+definition)
	required := contractStringSet(t, schema["required"], definition+" required")
	properties := contractObject(t, schema["properties"], definition+" properties")
	if len(required) != len(requiredFields) {
		t.Errorf("HTTP schema %s required 数 = %d，want %d", definition, len(required), len(requiredFields))
	}
	for _, name := range requiredFields {
		if _, ok := required[name]; !ok {
			t.Errorf("HTTP schema %s required 缺少 %s", definition, name)
		}
		if _, ok := properties[name]; !ok {
			t.Errorf("HTTP schema %s properties 缺少 %s", definition, name)
		}
	}
	for _, name := range forbiddenFields {
		if _, ok := properties[name]; ok {
			t.Errorf("HTTP schema %s 不得声明 identity field %s", definition, name)
		}
	}
}

func contractStringSet(t *testing.T, value any, label string) map[string]struct{} {
	t.Helper()
	result := make(map[string]struct{})
	for index, raw := range contractArray(t, value, label) {
		name := contractString(t, raw, fmt.Sprintf("%s[%d]", label, index))
		if _, duplicate := result[name]; duplicate {
			t.Errorf("%s 含重复成员 %q", label, name)
		}
		result[name] = struct{}{}
	}
	return result
}

func contractSchemaMentionsProperty(value any, name string) bool {
	switch current := value.(type) {
	case map[string]any:
		if properties, ok := current["properties"].(map[string]any); ok {
			if _, found := properties[name]; found {
				return true
			}
		}
		for _, child := range current {
			if contractSchemaMentionsProperty(child, name) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if contractSchemaMentionsProperty(child, name) {
				return true
			}
		}
	}
	return false
}

func contractValidateJSONType(typeName string, value any) error {
	switch typeName {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("不是 object（%T）", value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("不是 array（%T）", value)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("不是 string（%T）", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("不是 boolean（%T）", value)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("不是 null（%T）", value)
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return fmt.Errorf("不是 number（%T）", value)
		}
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("不是 integer（%T）", value)
		}
		rational, ok := new(big.Rat).SetString(number.String())
		if !ok || !rational.IsInt() {
			return fmt.Errorf("number %q 不是 integer", number)
		}
	default:
		return fmt.Errorf("测试 validator 不支持 schema type %q", typeName)
	}
	return nil
}

func contractSchemaInteger(value any) int64 {
	number, ok := value.(json.Number)
	if !ok {
		panic(fmt.Sprintf("schema integer 类型为 %T", value))
	}
	integer, err := number.Int64()
	if err != nil {
		panic(fmt.Sprintf("schema integer %q 非法: %v", number, err))
	}
	return integer
}

func contractCompareJSONNumbers(left json.Number, right any) (int, error) {
	rightNumber, ok := right.(json.Number)
	if !ok {
		return 0, fmt.Errorf("schema numeric bound 类型为 %T", right)
	}
	leftRat, ok := new(big.Rat).SetString(left.String())
	if !ok {
		return 0, fmt.Errorf("非法 JSON number %q", left)
	}
	rightRat, ok := new(big.Rat).SetString(rightNumber.String())
	if !ok {
		return 0, fmt.Errorf("非法 schema number %q", rightNumber)
	}
	return leftRat.Cmp(rightRat), nil
}

func contractJSONEqual(left, right any) bool {
	leftNumber, leftIsNumber := left.(json.Number)
	rightNumber, rightIsNumber := right.(json.Number)
	if leftIsNumber && rightIsNumber {
		comparison, err := contractCompareJSONNumbers(leftNumber, rightNumber)
		return err == nil && comparison == 0
	}
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

// contractValidateGoldenSemantics 补足标准 JSON Schema 无法表达的有限序列
// 约束。扩展规则仍在 checked-in schema 的 x-mornlea-rules 中显式声明。
func contractValidateGoldenSemantics(schemaName string, value any) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var rawPlan any
	switch schemaName {
	case "validate_plan_input", "plan_response":
		rawPlan = object["plan"]
	default:
		return nil
	}
	plan, ok := rawPlan.(map[string]any)
	if !ok {
		return nil
	}
	steps, ok := plan["steps"].([]any)
	if !ok {
		return nil
	}
	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		if step["kind"] == "follow" && index != len(steps)-1 {
			return fmt.Errorf("plan.steps[%d] follow 不是最后一步", index)
		}
	}
	return nil
}
