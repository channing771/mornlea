package companion

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/pathfind"
)

// TestContractFixtureFilesExist 先锁定共享契约的固定入口。Go 与 Python 后续都从
// 这些 versioned 文件读取同一份 schema、manifest 与 golden，避免各自复制定义。
func TestContractFixtureFilesExist(t *testing.T) {
	t.Parallel()

	for _, relative := range []string{
		"packages/contracts/companion-agent/http-v1/schema.json",
		"packages/contracts/companion-agent/http-v1/manifest.json",
		"packages/contracts/companion-agent/http-v1/golden/valid.json",
		"packages/contracts/companion-agent/http-v1/golden/invalid.json",
		"packages/contracts/companion-agent/mcp-v1/schema.json",
		"packages/contracts/companion-agent/mcp-v1/manifest.json",
		"packages/contracts/companion-agent/mcp-v1/golden/valid.json",
		"packages/contracts/companion-agent/mcp-v1/golden/invalid.json",
		"packages/contracts/companion-agent/mcp-v1/golden/mine-validation.json",
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
		contractAssertKnownMornleaExtensions(t, document, name)
		if err := contractAuditSchemaKeywords(document, name); err != nil {
			t.Errorf("%s 使用 validator 未支持的 schema 关键字: %v", name, err)
		}
	}

	for _, fixture := range []struct {
		document string
		path     string
		valid    bool
	}{
		{"http-v1/schema.json", "packages/contracts/companion-agent/http-v1/golden/valid.json", true},
		{"http-v1/schema.json", "packages/contracts/companion-agent/http-v1/golden/invalid.json", false},
		{"mcp-v1/schema.json", "packages/contracts/companion-agent/mcp-v1/golden/valid.json", true},
		{"mcp-v1/schema.json", "packages/contracts/companion-agent/mcp-v1/golden/invalid.json", false},
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
			var value any
			if testCase.ValueUTF8Hex != "" {
				rawValue, err := hex.DecodeString(testCase.ValueUTF8Hex)
				if err != nil {
					t.Fatalf("%s/%s 的 value_utf8_hex 非法: %v", fixture.path, testCase.Name, err)
				}
				value = string(rawValue)
			} else {
				value = contractDecodeRaw(t, testCase.Value, fixture.path+"/"+testCase.Name)
			}
			var context any
			if len(testCase.Context) != 0 {
				context = contractDecodeRaw(t, testCase.Context, fixture.path+"/"+testCase.Name+"/context")
			}
			err := schemas.validateDefinition(fixture.document, testCase.Schema, value, context)
			if fixture.valid && err != nil {
				t.Errorf("合法 golden %q 未通过 %s: %v", testCase.Name, testCase.Schema, err)
			}
			if fixture.valid {
				if testCase.ExpectedError != (contractExpectedError{}) {
					t.Errorf("合法 golden %q 不得声明 expected_error", testCase.Name)
				}
				continue
			}
			if testCase.ExpectedError.Path == "" || testCase.ExpectedError.Keyword == "" {
				t.Errorf("非法 golden %q 缺少 expected_error.path/keyword", testCase.Name)
				continue
			}
			if err == nil {
				t.Errorf("非法 golden %q 被 %s 接受（reason=%s）", testCase.Name, testCase.Schema, testCase.Reason)
				continue
			}
			var validationError *contractValidationError
			if !errors.As(err, &validationError) {
				t.Errorf("非法 golden %q 返回非结构化错误: %v", testCase.Name, err)
				continue
			}
			if validationError.Path != testCase.ExpectedError.Path ||
				validationError.Keyword != testCase.ExpectedError.Keyword ||
				validationError.Rule != testCase.ExpectedError.Rule {
				t.Errorf("非法 golden %q 错误 = {%s %s %s}，want {%s %s %s}（reason=%s）",
					testCase.Name, validationError.Path, validationError.Keyword, validationError.Rule,
					testCase.ExpectedError.Path, testCase.ExpectedError.Keyword, testCase.ExpectedError.Rule,
					testCase.Reason)
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

	manifest := contractReadObject(t, "packages/contracts/companion-agent/http-v1/manifest.json")
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
	contractAssertSchemaContainsRequired(t, schemas, "dialogue_nonterminal_request", wantProfiles["dialogue_run"])
	contractAssertSchemaContainsRequired(t, schemas, "dialogue_terminal_request", wantProfiles["dialogue_run"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_commit_request", wantProfiles["memory_commit"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_delete_request", wantProfiles["memory_delete"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_reconcile_active_request", wantProfiles["memory_reconcile"])
	contractAssertSchemaContainsRequired(t, schemas, "memory_reconcile_inactive_request", wantProfiles["memory_reconcile"])
	contractAssertSchemaFields(t, schemas, "cancel_request", wantProfiles["cancel"], []string{"companion_id", "generation", "memory_epoch"})
	contractAssertSchemaFields(t, schemas, "error_response", append(wantProfiles["error"], "error"), []string{"client_instance_id", "namespace_id", "lease_id", "run_id", "companion_id"})

	dialogueRoute := contractFindObjectByStringField(t, routes, "operation", "dialogue", "http routes")
	variants := contractObject(t, dialogueRoute["request_response_variants"], "dialogue request/response variants")
	if got := contractString(t, variants["discriminator"], "dialogue discriminator"); got != "terminal" {
		t.Fatalf("Dialogue discriminator = %q，want terminal", got)
	}
	wantVariants := []struct {
		value             bool
		request, response string
		proposal          string
	}{
		{false, "dialogue_nonterminal_request", "dialogue_nonterminal_response", "forbidden"},
		{true, "dialogue_terminal_request", "dialogue_terminal_response", "required"},
	}
	rawVariants := contractArray(t, variants["variants"], "dialogue variants")
	if len(rawVariants) != len(wantVariants) {
		t.Fatalf("Dialogue variant 数 = %d，want %d", len(rawVariants), len(wantVariants))
	}
	for index, want := range wantVariants {
		variant := contractObject(t, rawVariants[index], fmt.Sprintf("dialogue variants[%d]", index))
		if got := contractBool(t, variant["value"], "dialogue variant value"); got != want.value {
			t.Errorf("Dialogue variant[%d] value = %v，want %v", index, got, want.value)
		}
		for field, expected := range map[string]string{
			"request_schema":  want.request,
			"response_schema": want.response,
			"memory_proposal": want.proposal,
		} {
			if got := contractString(t, variant[field], "dialogue variant "+field); got != expected {
				t.Errorf("Dialogue variant[%d] %s = %q，want %q", index, field, got, expected)
			}
		}
		schemas.definition(t, "http-v1/schema.json", want.request)
		schemas.definition(t, "http-v1/schema.json", want.response)
	}
}

// TestContractFixtureMCPManifestConsistency 锁定共同 wire、SDK 前 allowlist、仅
// Tools capability、六个只读/纯校验工具以及 canonical 与双份 wire 上限。
func TestContractFixtureMCPManifestConsistency(t *testing.T) {
	t.Parallel()

	manifest := contractReadObject(t, "packages/contracts/companion-agent/mcp-v1/manifest.json")
	if got := contractString(t, manifest["application_contract_version"], "mcp application version"); got != "v1" {
		t.Fatalf("MCP application version = %q，want v1", got)
	}
	if got := contractString(t, manifest["mcp_protocol_version"], "mcp protocol version"); got != "2025-11-25" {
		t.Fatalf("MCP wire version = %q，want 2025-11-25", got)
	}
	if got := contractString(t, manifest["transport"], "mcp transport"); got != "streamable_http" {
		t.Fatalf("MCP transport = %q，want streamable_http", got)
	}
	if got := contractString(t, manifest["endpoint_path"], "mcp endpoint path"); got != "/mcp" {
		t.Fatalf("MCP endpoint path = %q，want /mcp", got)
	}
	for field, want := range map[string]bool{"stateless": true, "json_response": true, "sse": false, "sessions": false} {
		if got := contractBool(t, manifest[field], "mcp "+field); got != want {
			t.Errorf("MCP %s = %v，want %v", field, got, want)
		}
	}
	network := contractObject(t, manifest["network"], "mcp network")
	if got := contractString(t, network["scheme"], "mcp network scheme"); got != "http" {
		t.Errorf("MCP scheme = %q，want http", got)
	}
	if got := contractString(t, network["bind_host"], "mcp bind host"); got != "loopback_ip_literal" {
		t.Errorf("MCP bind host = %q，want loopback_ip_literal", got)
	}
	if got := contractString(t, network["request_host"], "mcp request Host"); got != "listener_authority" {
		t.Errorf("MCP request Host policy = %q，want listener_authority", got)
	}
	if contractBool(t, network["redirects"], "mcp redirects") {
		t.Error("MCP 不得跟随 redirect")
	}
	origin := contractObject(t, network["origin"], "mcp origin")
	if !contractBool(t, origin["missing_allowed"], "mcp missing Origin") {
		t.Fatal("Python httpx 不带 Origin 时必须合法")
	}
	if got := contractString(t, origin["present_value"], "mcp present Origin"); got != "listener_loopback_origin" {
		t.Fatalf("MCP present Origin policy = %q", got)
	}
	authentication := contractObject(t, manifest["authentication"], "mcp authentication")
	if got := contractString(t, authentication["scheme"], "mcp auth scheme"); got != "bearer" {
		t.Errorf("MCP auth scheme = %q，want bearer", got)
	}
	if got := contractString(t, authentication["credential"], "mcp auth credential"); got != "per_run_capability" {
		t.Errorf("MCP credential = %q，want per_run_capability", got)
	}
	if !contractBool(t, authentication["required"], "mcp auth required") {
		t.Error("MCP Bearer capability 必须为 required")
	}
	protocolHeader := contractObject(t, manifest["protocol_header"], "mcp protocol header")
	if got := contractString(t, protocolHeader["name"], "mcp protocol header name"); got != "Mcp-Protocol-Version" {
		t.Errorf("MCP protocol header = %q，want Mcp-Protocol-Version", got)
	}
	if !contractBool(t, protocolHeader["required_after_initialize"], "mcp protocol header required") {
		t.Error("initialize 后必须携带 MCP protocol header")
	}
	if got := contractString(t, protocolHeader["value"], "mcp protocol header value"); got != "2025-11-25" {
		t.Errorf("MCP protocol header value = %q，want 2025-11-25", got)
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
		codes     []string
	}
	wantTools := map[string]toolExpectation{
		"get_planning_context": {"get_planning_context_input", "get_planning_context_result", 24576, false, true, []string{}},
		"list_affordances":     {"list_affordances_input", "list_affordances_result", 24576, true, false, []string{}},
		"inspect_inventory":    {"inspect_inventory_input", "inspect_inventory_result", 8192, true, false, []string{}},
		"find_visible_blocks":  {"find_visible_blocks_input", "find_visible_blocks_result", 16384, true, false, []string{"unknown_block"}},
		"query_terrain":        {"query_terrain_input", "query_terrain_result", 16384, true, false, []string{"out_of_bounds"}},
		"validate_plan":        {"validate_plan_input", "validate_plan_result", 73728, false, true, []string{"invalid_schema", "out_of_bounds", "unknown_player", "unmineable_target", "unknown_block", "missing_item", "snapshot_mismatch"}},
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
		contractAssertStringList(t, tool["domain_result_codes"], want.codes, "MCP tool "+name+" domain result codes")
		if name == "list_affordances" {
			resultSchema := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", result), name+" result schema")
			if !contractJSONEqual(tool["semantic_rules"], resultSchema["x-mornlea-rules"]) {
				t.Errorf("MCP tool %s semantic_rules 未与 result schema 单源一致", name)
			}
		}
		if name == "find_visible_blocks" || name == "query_terrain" {
			resultSchema := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", result), name+" public result schema")
			contractAssertExactKeys(t, resultSchema, []string{"oneOf"}, "MCP tool "+name+" public result schema")
			branches := contractArray(t, resultSchema["oneOf"], name+" result oneOf")
			if len(branches) != 2 {
				t.Fatalf("MCP tool %s result oneOf 分支 = %d，want 2", name, len(branches))
			}
			successName := name + "_success_result"
			failureName := name + "_failure_result"
			for branchIndex, wantRef := range []string{"#/$defs/" + successName, "#/$defs/" + failureName} {
				branch := contractObject(t, branches[branchIndex], fmt.Sprintf("%s result oneOf[%d]", name, branchIndex))
				if got := contractString(t, branch["$ref"], name+" result branch ref"); got != wantRef {
					t.Errorf("MCP tool %s result oneOf[%d] = %q，want %q", name, branchIndex, got, wantRef)
				}
			}
			successSchema := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", successName), name+" success result schema")
			if !contractJSONEqual(tool["semantic_rules"], successSchema["x-mornlea-rules"]) {
				t.Errorf("MCP tool %s semantic_rules 未与 success result schema 单源一致", name)
			}
			failureSchema := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", failureName), name+" failure result schema")
			contractAssertExactKeys(t, failureSchema, []string{"type", "additionalProperties", "required", "properties"}, "MCP tool "+name+" failure result schema")
			contractAssertStringList(t, failureSchema["required"], []string{"code", "hint"}, name+" failure required")
			failureProperties := contractObject(t, failureSchema["properties"], name+" failure properties")
			contractAssertExactKeys(t, failureProperties, []string{"code", "hint"}, "MCP tool "+name+" failure properties")
			codeSchema := contractObject(t, failureProperties["code"], name+" failure code")
			if got := contractString(t, codeSchema["const"], name+" failure code const"); got != want.codes[0] {
				t.Errorf("MCP tool %s failure code = %q，want %q", name, got, want.codes[0])
			}
			hintSchema := contractObject(t, failureProperties["hint"], name+" failure hint")
			if got := contractString(t, hintSchema["$ref"], name+" failure hint ref"); got != "#/$defs/validator_hint" {
				t.Errorf("MCP tool %s failure hint ref = %q，want validator_hint", name, got)
			}
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
	contractAssertStringList(t, validateTool["domain_result_codes"], wantCodes, "validate_plan domain result codes")
	validatorResult := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", "validate_plan_result"), "validate_plan result")
	validatorBranches := contractArray(t, validatorResult["oneOf"], "validate_plan result oneOf")
	if len(validatorBranches) != 2 {
		t.Fatalf("validate_plan result oneOf 分支 = %d，want 2", len(validatorBranches))
	}
	failureBranch := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", "validate_plan_failure_result"), "validate_plan failure branch")
	failureProperties := contractObject(t, failureBranch["properties"], "validate_plan failure properties")
	validatorCodeSchema := contractObject(t, failureProperties["code"], "validate_plan failure code")
	contractAssertStringList(t, validatorCodeSchema["enum"], wantCodes, "validate_plan schema codes")
	planSchema := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", "plan"), "plan schema")
	planProperties := contractObject(t, planSchema["properties"], "plan properties")
	stepsSchema := contractObject(t, planProperties["steps"], "plan steps")
	stepItems := contractObject(t, stepsSchema["items"], "plan step items")
	stepBranches := contractArray(t, stepItems["oneOf"], "plan step branches")
	wantStepRefs := []string{"#/$defs/go_to_step", "#/$defs/mine_step", "#/$defs/place_step", "#/$defs/follow_step"}
	if len(stepBranches) != len(wantStepRefs) {
		t.Fatalf("plan step branch 数 = %d，want %d", len(stepBranches), len(wantStepRefs))
	}
	for index, wantRef := range wantStepRefs {
		branch := contractObject(t, stepBranches[index], fmt.Sprintf("plan step branch[%d]", index))
		if got := contractString(t, branch["$ref"], "plan step ref"); got != wantRef {
			t.Errorf("plan step branch[%d] = %q，want %q", index, got, wantRef)
		}
	}
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

	validGolden := contractLoadGolden(t, "packages/contracts/companion-agent/mcp-v1/golden/valid.json")
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

// TestContractFixtureMCPDomainLimits 防止共享 schema 脱离 Go 权威常量，导致
// 冻结快照或物品堆在跨语言边界被静默扩大。
func TestContractFixtureMCPDomainLimits(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	contextResult := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", "get_planning_context_result"), "planning context result")
	contextProperties := contractObject(t, contextResult["properties"], "planning context properties")
	chunkRevisions := contractObject(t, contextProperties["chunk_revisions"], "chunk revisions")
	if got := contractInt64(t, chunkRevisions["maxItems"], "chunk revisions maxItems"); got != pathfind.MaxPlanChunkRevisions {
		t.Fatalf("chunk_revisions maxItems = %d，want pathfind.MaxPlanChunkRevisions=%d", got, pathfind.MaxPlanChunkRevisions)
	}

	inventoryResult := contractObject(t, schemas.definition(t, "mcp-v1/schema.json", "inspect_inventory_result"), "inventory result")
	inventoryProperties := contractObject(t, inventoryResult["properties"], "inventory result properties")
	slots := contractObject(t, inventoryProperties["slots"], "inventory slots")
	slotSchema := contractObject(t, slots["items"], "inventory slot schema")
	slotProperties := contractObject(t, slotSchema["properties"], "inventory slot properties")
	count := contractObject(t, slotProperties["count"], "inventory count")
	if got := contractInt64(t, count["maximum"], "inventory count maximum"); got != core.MaxStackCount {
		t.Fatalf("inventory count maximum = %d，want core.MaxStackCount=%d", got, core.MaxStackCount)
	}
}

// TestContractFixtureTextRulesMatchAuthority 把跨语言文本字段的 UTF-8 字节
// 上限与既有 Go 校验常量对齐，并钉住台词、人设与摘要的字符纪律。
func TestContractFixtureTextRulesMatchAuthority(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	cases := []struct {
		document   string
		definition string
		maximum    int64
		flags      []string
	}{
		{"http-v1/schema.json", "instruction_text", MaxPlanCommandBytes, []string{"valid_utf8", "no_unicode_control", "non_blank"}},
		{"http-v1/schema.json", "persona_text", MaxPersonaBytes, []string{"valid_utf8", "no_nul"}},
		{"http-v1/schema.json", "dialogue_line", MaxDialogueLineBytes, []string{"valid_utf8", "no_nul", "no_unicode_control", "no_edge_unicode_whitespace"}},
		{"http-v1/schema.json", "memory_summary", MaxDialogueSummaryBytes, []string{"valid_utf8", "no_nul"}},
		{"http-v1/schema.json", "mcp_capability", 512, []string{"valid_utf8", "no_unicode_control"}},
		{"http-v1/schema.json", "mcp_endpoint", 256, []string{"valid_utf8", "no_unicode_control"}},
		{"mcp-v1/schema.json", "instruction_text", MaxPlanCommandBytes, []string{"valid_utf8", "no_unicode_control", "non_blank"}},
		{"mcp-v1/schema.json", "plan_summary", MaxPlanSummaryBytes, []string{"valid_utf8", "no_unicode_control", "non_blank"}},
		{"mcp-v1/schema.json", "task_status_text", MaxPlanTaskStatusBytes, []string{"valid_utf8", "no_unicode_control"}},
		{"mcp-v1/schema.json", "bounded_name", 64, []string{"valid_utf8", "no_unicode_control", "non_blank"}},
		{"mcp-v1/schema.json", "validator_hint", 256, []string{"valid_utf8", "no_nul", "no_unicode_control", "no_edge_unicode_whitespace"}},
	}
	for _, testCase := range cases {
		schema := contractObject(t, schemas.definition(t, testCase.document, testCase.definition), testCase.definition)
		rule := contractFindRule(t, schema, "text", testCase.definition)
		if got := contractInt64(t, rule["max_utf8_bytes"], testCase.definition+" max UTF-8 bytes"); got != testCase.maximum {
			t.Errorf("%s max_utf8_bytes = %d，want %d", testCase.definition, got, testCase.maximum)
		}
		if testCase.definition == "validator_hint" {
			if got := contractInt64(t, rule["min_utf8_bytes"], "validator_hint min UTF-8 bytes"); got != 1 {
				t.Errorf("validator_hint min_utf8_bytes = %d，want 1", got)
			}
		}
		for _, flag := range testCase.flags {
			if !contractBool(t, rule[flag], testCase.definition+" "+flag) {
				t.Errorf("%s 必须声明 %s=true", testCase.definition, flag)
			}
		}
	}
}

// TestContractFixtureRejectsUnknownMachineRules 保证新增私有扩展或规则不会被
// 测试 validator 静默当作普通 annotation，从而掩盖未实现的跨语言语义。
func TestContractFixtureRejectsUnknownMachineRules(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	tests := []struct {
		name    string
		schema  map[string]any
		keyword string
		rule    string
	}{
		{
			name:    "未知扩展",
			schema:  map[string]any{"type": "string", "x-mornlea-not-implemented": true},
			keyword: "x-mornlea-extension",
			rule:    "x-mornlea-not-implemented",
		},
		{
			name: "未知规则",
			schema: map[string]any{
				"type":            "string",
				"x-mornlea-rules": []any{map[string]any{"name": "not_implemented"}},
			},
			keyword: "x-mornlea-rules",
			rule:    "not_implemented",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := schemas.validate("http-v1/schema.json", testCase.schema, "value", "$", nil)
			var validationError *contractValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("未知 machine rule 返回非结构化错误: %v", err)
			}
			if validationError.Path != "$" || validationError.Keyword != testCase.keyword || validationError.Rule != testCase.rule {
				t.Fatalf("未知 machine rule 错误 = {%s %s %s}，want {$ %s %s}",
					validationError.Path, validationError.Keyword, validationError.Rule, testCase.keyword, testCase.rule)
			}
		})
	}
}

// TestContractFixtureOneOfFailureUsesParentPath 保证复合 schema 零匹配时只返回
// 父级 `oneOf` 错误，不从分支中挑选可能受 map 遍历顺序影响的叶级错误。
func TestContractFixtureOneOfFailureUsesParentPath(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	schema := map[string]any{
		"oneOf": []any{
			map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"kind", "left"},
				"properties": map[string]any{
					"kind": map[string]any{"const": "left"},
					"left": map[string]any{"type": "integer"},
				},
			},
			map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"kind", "right"},
				"properties": map[string]any{
					"kind":  map[string]any{"const": "right"},
					"right": map[string]any{"type": "string"},
				},
			},
		},
	}
	value := map[string]any{"kind": "unknown", "left": "not-an-integer", "right": json.Number("1")}
	err := schemas.validate("http-v1/schema.json", schema, value, "$.candidate", nil)
	var validationError *contractValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("oneOf 零匹配返回非结构化错误: %v", err)
	}
	if validationError.Path != "$.candidate" || validationError.Keyword != "oneOf" || validationError.Rule != "" {
		t.Fatalf("oneOf 零匹配错误 = {%s %s %s}，want {$.candidate oneOf <empty>}",
			validationError.Path, validationError.Keyword, validationError.Rule)
	}
}

// TestContractFixtureRejectsUnsupportedSchemaKeywords 保证 fixture validator 的
// 标准关键字能力是显式 allowlist；新增未实现验证语义时必须先扩展 validator。
func TestContractFixtureRejectsUnsupportedSchemaKeywords(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	tests := []struct {
		keyword string
		schema  map[string]any
		value   any
	}{
		{"allOf", map[string]any{"type": "string", "allOf": []any{map[string]any{"minLength": json.Number("1")}}}, "value"},
		{"not", map[string]any{"type": "string", "not": map[string]any{"const": "forbidden"}}, "value"},
		{"contains", map[string]any{"type": "array", "contains": map[string]any{"type": "integer"}}, []any{json.Number("1")}},
		{"unevaluatedProperties", map[string]any{"type": "object", "unevaluatedProperties": false}, map[string]any{}},
	}
	for _, testCase := range tests {
		t.Run(testCase.keyword, func(t *testing.T) {
			err := schemas.validate("http-v1/schema.json", testCase.schema, testCase.value, "$", nil)
			var validationError *contractValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("未实现关键字 %s 返回非结构化错误: %v", testCase.keyword, err)
			}
			if validationError.Path != "$" || validationError.Keyword != "schema-keyword" || validationError.Rule != testCase.keyword {
				t.Fatalf("未实现关键字 %s 错误 = {%s %s %s}，want {$ schema-keyword %s}",
					testCase.keyword, validationError.Path, validationError.Keyword, validationError.Rule, testCase.keyword)
			}
		})
	}
}

// TestContractFixtureSchemaKeywordAuditUnderstandsContainers 防止 allowlist 把
// annotation 或 `properties`、`$defs` 下恰好同名的成员误判为 schema keyword。
func TestContractFixtureSchemaKeywordAuditUnderstandsContainers(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	schema := map[string]any{
		"$comment":             "annotation",
		"title":                "annotation",
		"description":          "annotation",
		"default":              map[string]any{"x-mornlea-annotation-data": true},
		"examples":             []any{map[string]any{}},
		"readOnly":             true,
		"writeOnly":            false,
		"deprecated":           false,
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"allOf":              map[string]any{"type": "string"},
			"not":                map[string]any{"type": "integer"},
			"x-mornlea-property": map[string]any{"type": "string"},
		},
		"$defs": map[string]any{
			"contains":              map[string]any{"type": "string"},
			"unevaluatedProperties": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}},
			"x-mornlea-definition":  map[string]any{"type": "string"},
		},
	}
	value := map[string]any{"allOf": "value", "not": json.Number("1"), "x-mornlea-property": "value"}
	if err := schemas.validate("http-v1/schema.json", schema, value, "$", nil); err != nil {
		t.Fatalf("annotation 或 schema container 成员名被误报: %v", err)
	}
	contractAssertKnownMornleaExtensions(t, schema, "synthetic schema")
}

// TestContractFixtureMineValidationMatchesAuthority 把跨语言 mine golden 与当前
// Go 权威规则逐项对照，防止服务抽离时误拒 Chest/Furnace 或放开未交付语义。
func TestContractFixtureMineValidationMatchesAuthority(t *testing.T) {
	t.Parallel()

	document := contractReadObject(t, "packages/contracts/companion-agent/mcp-v1/golden/mine-validation.json")
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
		Name          string                `json:"name"`
		Schema        string                `json:"schema"`
		Reason        string                `json:"reason,omitempty"`
		Value         json.RawMessage       `json:"value"`
		ValueUTF8Hex  string                `json:"value_utf8_hex,omitempty"`
		Context       json.RawMessage       `json:"context,omitempty"`
		ExpectedError contractExpectedError `json:"expected_error,omitempty"`
	} `json:"cases"`
}

type contractExpectedError struct {
	Path    string `json:"path"`
	Keyword string `json:"keyword"`
	Rule    string `json:"rule,omitempty"`
}

type contractValidationError struct {
	Path    string
	Keyword string
	Rule    string
	Detail  string
}

func (err *contractValidationError) Error() string {
	if err.Rule != "" {
		return fmt.Sprintf("%s: %s/%s: %s", err.Path, err.Keyword, err.Rule, err.Detail)
	}
	return fmt.Sprintf("%s: %s: %s", err.Path, err.Keyword, err.Detail)
}

type contractSchemaSet struct {
	documents map[string]map[string]any
}

func contractLoadSchemas(t *testing.T) contractSchemaSet {
	t.Helper()
	return contractSchemaSet{documents: map[string]map[string]any{
		"http-v1/schema.json": contractReadObject(t, "packages/contracts/companion-agent/http-v1/schema.json"),
		"mcp-v1/schema.json":  contractReadObject(t, "packages/contracts/companion-agent/mcp-v1/schema.json"),
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

func (schemas contractSchemaSet) validateDefinition(document, name string, value, context any) error {
	root, ok := schemas.documents[document]
	if !ok {
		return contractViolation("$", "schema", "document", "缺少 schema document %q", document)
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		return contractViolation("$", "schema", "$defs", "%s 缺少 $defs", document)
	}
	schema, ok := definitions[name]
	if !ok {
		return contractViolation("$", "schema", "definition", "%s 缺少 definition %q", document, name)
	}
	return schemas.validate(document, schema, value, "$", context)
}

// validate 先审计完整 schema tree，再执行本 change 实际使用的 JSON Schema
// 2020-12 子集；未实现的标准关键字和私有扩展都必须硬失败。
func (schemas contractSchemaSet) validate(document string, rawSchema, value any, valuePath string, context any) error {
	if err := contractAuditSchemaKeywords(rawSchema, valuePath); err != nil {
		return err
	}
	return schemas.validateChecked(document, rawSchema, value, valuePath, context)
}

// validateChecked 只处理已经通过 capability audit 的 schema。它刻意留在
// 测试文件，作用是交叉检查 fixtures，不成为未来 transport 的旁路。
func (schemas contractSchemaSet) validateChecked(document string, rawSchema, value any, valuePath string, context any) error {
	switch schema := rawSchema.(type) {
	case bool:
		if schema {
			return nil
		}
		return contractViolation(valuePath, "falseSchema", "", "被 false schema 拒绝")
	case map[string]any:
		if rawRef, ok := schema["$ref"]; ok {
			ref, ok := rawRef.(string)
			if !ok {
				return contractViolation(valuePath, "schema", "$ref", "$ref 不是字符串")
			}
			targetDocument, target, err := schemas.resolveRef(document, ref)
			if err != nil {
				return contractViolation(valuePath, "schema", "$ref", "%v", err)
			}
			return schemas.validateChecked(targetDocument, target, value, valuePath, context)
		}
		if rawOneOf, ok := schema["oneOf"]; ok {
			branches, ok := rawOneOf.([]any)
			if !ok || len(branches) == 0 {
				return contractViolation(valuePath, "schema", "oneOf", "oneOf 非法")
			}
			matched := 0
			for _, branch := range branches {
				err := schemas.validateChecked(document, branch, value, valuePath, context)
				if err == nil {
					matched++
				}
			}
			if matched == 0 {
				return contractViolation(valuePath, "oneOf", "", "没有匹配分支")
			}
			if matched > 1 {
				return contractViolation(valuePath, "oneOf", "", "匹配分支数 %d，want 1", matched)
			}
		}
		if rawType, ok := schema["type"]; ok {
			typeName, ok := rawType.(string)
			if !ok {
				return contractViolation(valuePath, "schema", "type", "schema type 不是字符串")
			}
			if err := contractValidateJSONType(typeName, value); err != nil {
				return contractViolation(valuePath, "type", typeName, "%v", err)
			}
		}
		if expected, ok := schema["const"]; ok && !contractJSONEqual(expected, value) {
			return contractViolation(valuePath, "const", "", "不等于 const")
		}
		if rawEnum, ok := schema["enum"]; ok {
			values, ok := rawEnum.([]any)
			if !ok {
				return contractViolation(valuePath, "schema", "enum", "enum 非数组")
			}
			found := false
			for _, candidate := range values {
				if contractJSONEqual(candidate, value) {
					found = true
					break
				}
			}
			if !found {
				return contractViolation(valuePath, "enum", "", "不在 enum 中")
			}
		}
		if text, ok := value.(string); ok {
			if rawMin, exists := schema["minLength"]; exists && utf8.RuneCountInString(text) < int(contractSchemaInteger(rawMin)) {
				return contractViolation(valuePath, "minLength", "", "短于 minLength")
			}
			if rawMax, exists := schema["maxLength"]; exists && utf8.RuneCountInString(text) > int(contractSchemaInteger(rawMax)) {
				return contractViolation(valuePath, "maxLength", "", "长于 maxLength")
			}
			if rawPattern, exists := schema["pattern"]; exists {
				pattern, ok := rawPattern.(string)
				if !ok {
					return contractViolation(valuePath, "schema", "pattern", "pattern 不是字符串")
				}
				compiled, err := regexp.Compile(pattern)
				if err != nil {
					return contractViolation(valuePath, "schema", "pattern", "pattern 非法: %v", err)
				}
				if !compiled.MatchString(text) {
					return contractViolation(valuePath, "pattern", "", "不匹配 pattern")
				}
			}
			if format, exists := schema["format"]; exists && format == "uri" {
				parsed, err := url.Parse(text)
				if err != nil || parsed.Scheme == "" || parsed.Host == "" {
					return contractViolation(valuePath, "format", "uri", "不是 absolute URI")
				}
			}
		}
		if number, ok := value.(json.Number); ok {
			if rawMin, exists := schema["minimum"]; exists {
				comparison, err := contractCompareJSONNumbers(number, rawMin)
				if err != nil || comparison < 0 {
					return contractViolation(valuePath, "minimum", "", "小于 minimum")
				}
			}
			if rawMax, exists := schema["maximum"]; exists {
				comparison, err := contractCompareJSONNumbers(number, rawMax)
				if err != nil || comparison > 0 {
					return contractViolation(valuePath, "maximum", "", "大于 maximum")
				}
			}
		}
		if array, ok := value.([]any); ok {
			if rawMin, exists := schema["minItems"]; exists && len(array) < int(contractSchemaInteger(rawMin)) {
				return contractViolation(valuePath, "minItems", "", "少于 minItems")
			}
			if rawMax, exists := schema["maxItems"]; exists && len(array) > int(contractSchemaInteger(rawMax)) {
				return contractViolation(valuePath, "maxItems", "", "多于 maxItems")
			}
			if unique, _ := schema["uniqueItems"].(bool); unique {
				seen := make(map[string]struct{}, len(array))
				for index, item := range array {
					encoded, _ := json.Marshal(item)
					key := string(encoded)
					if _, duplicate := seen[key]; duplicate {
						return contractViolation(fmt.Sprintf("%s[%d]", valuePath, index), "uniqueItems", "", "数组成员重复")
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
					if err := schemas.validateChecked(document, itemSchema, item, fmt.Sprintf("%s[%d]", valuePath, index), context); err != nil {
						return err
					}
				}
			}
		}
		if object, ok := value.(map[string]any); ok {
			if rawRequired, exists := schema["required"]; exists {
				required, ok := rawRequired.([]any)
				if !ok {
					return contractViolation(valuePath, "schema", "required", "required 非数组")
				}
				for _, rawName := range required {
					name, ok := rawName.(string)
					if !ok {
						return contractViolation(valuePath, "schema", "required", "required 成员不是字符串")
					}
					if _, exists := object[name]; !exists {
						return contractViolation(valuePath+"."+name, "required", name, "缺少 required property")
					}
				}
			}
			properties, _ := schema["properties"].(map[string]any)
			for name, member := range object {
				propertySchema, known := properties[name]
				if !known {
					if additional, exists := schema["additionalProperties"]; exists && additional == false {
						return contractViolation(valuePath+"."+name, "additionalProperties", "", "含未知 property")
					}
					continue
				}
				if err := schemas.validateChecked(document, propertySchema, member, valuePath+"."+name, context); err != nil {
					return err
				}
			}
		}
		if rules, exists := schema["x-mornlea-rules"]; exists {
			if err := schemas.validateMornleaRules(document, rules, value, valuePath, context); err != nil {
				return err
			}
		}
		return nil
	default:
		return contractViolation(valuePath, "schema", "type", "schema 不是 object 或 boolean")
	}
}

var contractSupportedSchemaKeywords = map[string]struct{}{
	"$comment":             {},
	"$defs":                {},
	"$id":                  {},
	"$ref":                 {},
	"$schema":              {},
	"additionalProperties": {},
	"const":                {},
	"default":              {},
	"deprecated":           {},
	"description":          {},
	"enum":                 {},
	"examples":             {},
	"format":               {},
	"items":                {},
	"maxItems":             {},
	"maxLength":            {},
	"maximum":              {},
	"minItems":             {},
	"minLength":            {},
	"minimum":              {},
	"oneOf":                {},
	"pattern":              {},
	"prefixItems":          {},
	"properties":           {},
	"readOnly":             {},
	"required":             {},
	"title":                {},
	"type":                 {},
	"uniqueItems":          {},
	"writeOnly":            {},
	"x-mornlea-rules":      {},
}

// contractAuditSchemaKeywords 遍历 schema-valued 位置并拒绝 validator 未实现
// 的关键字。annotation 的任意载荷，以及 `properties`/`$defs` 的成员名，都不
// 是 schema keyword；只有这些容器里的成员值会继续按 schema 审计。
func contractAuditSchemaKeywords(rawSchema any, valuePath string) error {
	switch schema := rawSchema.(type) {
	case bool:
		return nil
	case map[string]any:
		keywords := make([]string, 0, len(schema))
		for keyword := range schema {
			keywords = append(keywords, keyword)
		}
		sort.Strings(keywords)
		for _, keyword := range keywords {
			if strings.HasPrefix(keyword, "x-mornlea-") && keyword != "x-mornlea-rules" {
				return contractViolation(valuePath, "x-mornlea-extension", keyword, "未知或未实现的扩展")
			}
			if _, ok := contractSupportedSchemaKeywords[keyword]; !ok {
				return contractViolation(valuePath, "schema-keyword", keyword, "validator 未实现 schema keyword")
			}
		}
		for _, keyword := range keywords {
			child := schema[keyword]
			switch keyword {
			case "$defs", "properties":
				members, ok := child.(map[string]any)
				if !ok {
					return contractViolation(valuePath, "schema", keyword, "%s 必须是 object", keyword)
				}
				names := make([]string, 0, len(members))
				for name := range members {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					if err := contractAuditSchemaKeywords(members[name], valuePath+"."+keyword+"."+name); err != nil {
						return err
					}
				}
			case "oneOf", "prefixItems":
				children, ok := child.([]any)
				if !ok {
					return contractViolation(valuePath, "schema", keyword, "%s 必须是 array", keyword)
				}
				for index, nested := range children {
					if err := contractAuditSchemaKeywords(nested, fmt.Sprintf("%s.%s[%d]", valuePath, keyword, index)); err != nil {
						return err
					}
				}
			case "items":
				if err := contractAuditSchemaKeywords(child, valuePath+".items"); err != nil {
					return err
				}
			case "additionalProperties":
				if _, ok := child.(bool); !ok {
					return contractViolation(valuePath, "schema-keyword", "additionalProperties", "只实现 boolean 形态")
				}
			}
		}
		return nil
	default:
		return contractViolation(valuePath, "schema", "type", "schema 不是 object 或 boolean")
	}
}

func contractViolation(path, keyword, rule, format string, args ...any) error {
	return &contractValidationError{Path: path, Keyword: keyword, Rule: rule, Detail: fmt.Sprintf(format, args...)}
}

func (schemas contractSchemaSet) validateMornleaRules(document string, rawRules, value any, valuePath string, context any) error {
	rules, ok := rawRules.([]any)
	if !ok || len(rules) == 0 {
		return contractViolation(valuePath, "schema", "x-mornlea-rules", "规则列表必须是非空 array")
	}
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return contractViolation(valuePath, "schema", "x-mornlea-rules", "规则必须是 object")
		}
		name, ok := rule["name"].(string)
		if !ok || name == "" {
			return contractViolation(valuePath, "schema", "x-mornlea-rules", "规则缺少 name")
		}
		var err error
		switch name {
		case "text":
			err = contractValidateTextRule(rule, value, valuePath)
		case "follow_must_be_last":
			err = contractValidateFollowLastRule(rule, value, valuePath)
		case "loopback_mcp_url":
			err = contractValidateLoopbackMCPURLRule(rule, value, valuePath)
		case "sorted_positions":
			err = contractValidateSortedPositionsRule(rule, value, valuePath)
		case "positions_match_context":
			err = contractValidatePositionsMatchRule(rule, value, valuePath, context)
		default:
			return contractViolation(valuePath, "x-mornlea-rules", name, "未知或未实现的规则")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func contractValidateTextRule(rule map[string]any, value any, valuePath string) error {
	if err := contractRejectUnknownRuleFields(rule, "text", "name", "valid_utf8", "min_utf8_bytes", "max_utf8_bytes", "no_nul", "no_unicode_control", "no_edge_unicode_whitespace", "non_blank"); err != nil {
		return contractViolation(valuePath, "schema", "text", "%v", err)
	}
	text, ok := value.(string)
	if !ok {
		return contractViolation(valuePath, "x-mornlea-rules", "text.type", "text rule 只能用于 string")
	}
	if contractRuleBool(rule, "valid_utf8") && !utf8.ValidString(text) {
		return contractViolation(valuePath, "x-mornlea-rules", "text.valid_utf8", "不是有效 UTF-8")
	}
	if minimum, ok := contractOptionalRuleInteger(rule, "min_utf8_bytes"); ok && int64(len(text)) < minimum {
		return contractViolation(valuePath, "x-mornlea-rules", "text.min_utf8_bytes", "UTF-8 字节数 %d 小于 %d", len(text), minimum)
	}
	if maximum, ok := contractOptionalRuleInteger(rule, "max_utf8_bytes"); ok && int64(len(text)) > maximum {
		return contractViolation(valuePath, "x-mornlea-rules", "text.max_utf8_bytes", "UTF-8 字节数 %d 超过 %d", len(text), maximum)
	}
	if contractRuleBool(rule, "no_nul") && strings.ContainsRune(text, 0) {
		return contractViolation(valuePath, "x-mornlea-rules", "text.no_nul", "包含 NUL")
	}
	if contractRuleBool(rule, "no_unicode_control") {
		for _, character := range text {
			if unicode.IsControl(character) {
				return contractViolation(valuePath, "x-mornlea-rules", "text.no_unicode_control", "包含 Unicode control")
			}
		}
	}
	if contractRuleBool(rule, "no_edge_unicode_whitespace") && strings.TrimSpace(text) != text {
		return contractViolation(valuePath, "x-mornlea-rules", "text.no_edge_unicode_whitespace", "首尾包含 Unicode whitespace")
	}
	if contractRuleBool(rule, "non_blank") && strings.TrimSpace(text) == "" {
		return contractViolation(valuePath, "x-mornlea-rules", "text.non_blank", "文本为空白")
	}
	return nil
}

func contractValidateFollowLastRule(rule map[string]any, value any, valuePath string) error {
	if err := contractRejectUnknownRuleFields(rule, "follow_must_be_last", "name"); err != nil {
		return contractViolation(valuePath, "schema", "follow_must_be_last", "%v", err)
	}
	plan, ok := value.(map[string]any)
	if !ok {
		return contractViolation(valuePath, "x-mornlea-rules", "follow_must_be_last", "规则只能用于 plan object")
	}
	steps, _ := plan["steps"].([]any)
	for index, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		if step["kind"] == "follow" && index != len(steps)-1 {
			return contractViolation(fmt.Sprintf("%s.steps[%d]", valuePath, index), "x-mornlea-rules", "follow_must_be_last", "follow 不是最后一步")
		}
	}
	return nil
}

func contractValidateLoopbackMCPURLRule(rule map[string]any, value any, valuePath string) error {
	if err := contractRejectUnknownRuleFields(rule, "loopback_mcp_url", "name", "scheme", "path", "port_required", "forbid_userinfo", "forbid_query", "forbid_fragment"); err != nil {
		return contractViolation(valuePath, "schema", "loopback_mcp_url", "%v", err)
	}
	text, ok := value.(string)
	if !ok {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url", "规则只能用于 string")
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.parse", "URL 解析失败")
	}
	if scheme, _ := rule["scheme"].(string); parsed.Scheme != scheme {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.scheme", "scheme = %q", parsed.Scheme)
	}
	if contractRuleBool(rule, "forbid_userinfo") && parsed.User != nil {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.userinfo", "包含 userinfo")
	}
	if contractRuleBool(rule, "forbid_query") && (parsed.RawQuery != "" || parsed.ForceQuery) {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.query", "包含 query")
	}
	if contractRuleBool(rule, "forbid_fragment") && strings.Contains(text, "#") {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.fragment", "包含 fragment")
	}
	if expectedPath, _ := rule["path"].(string); parsed.EscapedPath() != expectedPath {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.path", "path = %q", parsed.EscapedPath())
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.host", "host 不是 loopback IP literal")
	}
	if contractRuleBool(rule, "port_required") {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return contractViolation(valuePath, "x-mornlea-rules", "loopback_mcp_url.port", "缺少或非法 port")
		}
	}
	return nil
}

func contractValidateSortedPositionsRule(rule map[string]any, value any, valuePath string) error {
	if err := contractRejectUnknownRuleFields(rule, "sorted_positions", "name", "array_property", "position_property", "axes", "strict"); err != nil {
		return contractViolation(valuePath, "schema", "sorted_positions", "%v", err)
	}
	object, _ := value.(map[string]any)
	arrayProperty, _ := rule["array_property"].(string)
	positionProperty, _ := rule["position_property"].(string)
	items, _ := object[arrayProperty].([]any)
	axes, err := contractRuleStringList(rule["axes"])
	if err != nil || len(axes) == 0 {
		return contractViolation(valuePath, "schema", "sorted_positions", "axes 非法")
	}
	if !contractRuleBool(rule, "strict") {
		return contractViolation(valuePath, "schema", "sorted_positions", "strict 必须为 true")
	}
	var previous map[string]any
	for index, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		position, _ := item[positionProperty].(map[string]any)
		if previous != nil && contractComparePosition(previous, position, axes) >= 0 {
			return contractViolation(fmt.Sprintf("%s.%s[%d].%s", valuePath, arrayProperty, index, positionProperty), "x-mornlea-rules", "sorted_positions", "坐标不是严格升序")
		}
		previous = position
	}
	return nil
}

func contractValidatePositionsMatchRule(rule map[string]any, value any, valuePath string, context any) error {
	if err := contractRejectUnknownRuleFields(rule, "positions_match_context", "name", "context_property", "result_property", "position_property"); err != nil {
		return contractViolation(valuePath, "schema", "positions_match_context", "%v", err)
	}
	object, _ := value.(map[string]any)
	contextObject, ok := context.(map[string]any)
	if !ok {
		return contractViolation(valuePath, "x-mornlea-rules", "positions_match_context.context", "缺少 input context")
	}
	contextProperty, _ := rule["context_property"].(string)
	resultProperty, _ := rule["result_property"].(string)
	positionProperty, _ := rule["position_property"].(string)
	expected, _ := contextObject[contextProperty].([]any)
	results, _ := object[resultProperty].([]any)
	if len(expected) != len(results) {
		return contractViolation(valuePath+"."+resultProperty, "x-mornlea-rules", "positions_match_context.length", "结果数 %d 与输入数 %d 不同", len(results), len(expected))
	}
	for index, rawResult := range results {
		result, _ := rawResult.(map[string]any)
		if !contractJSONEqual(result[positionProperty], expected[index]) {
			return contractViolation(fmt.Sprintf("%s.%s[%d].%s", valuePath, resultProperty, index, positionProperty), "x-mornlea-rules", "positions_match_context", "位置与输入顺序不一致")
		}
	}
	return nil
}

func contractRejectUnknownRuleFields(rule map[string]any, ruleName string, allowed ...string) error {
	known := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		known[field] = struct{}{}
	}
	for field := range rule {
		if _, ok := known[field]; !ok {
			return fmt.Errorf("规则 %s 含未知字段 %s", ruleName, field)
		}
	}
	return nil
}

func contractRuleBool(rule map[string]any, name string) bool {
	value, _ := rule[name].(bool)
	return value
}

func contractOptionalRuleInteger(rule map[string]any, name string) (int64, bool) {
	value, ok := rule[name]
	if !ok {
		return 0, false
	}
	return contractSchemaInteger(value), true
}

func contractRuleStringList(value any) ([]string, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, errors.New("不是 array")
	}
	result := make([]string, len(raw))
	for index, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("成员不是 string")
		}
		result[index] = text
	}
	return result, nil
}

func contractComparePosition(left, right map[string]any, axes []string) int {
	for _, axis := range axes {
		leftNumber, _ := left[axis].(json.Number)
		rightNumber, _ := right[axis].(json.Number)
		comparison, _ := contractCompareJSONNumbers(leftNumber, rightNumber)
		if comparison != 0 {
			return comparison
		}
	}
	return 0
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
		hasValue := len(testCase.Value) != 0
		hasUTF8Hex := testCase.ValueUTF8Hex != ""
		if testCase.Name == "" || testCase.Schema == "" || hasValue == hasUTF8Hex {
			t.Fatalf("golden %s cases[%d] 必须包含 name/schema 与且仅一个 value/value_utf8_hex", relative, index)
		}
		if hasUTF8Hex {
			rawValue, err := hex.DecodeString(testCase.ValueUTF8Hex)
			if err != nil || utf8.Valid(rawValue) {
				t.Fatalf("golden %s cases[%d] 的 value_utf8_hex 必须是非法 UTF-8 bytes", relative, index)
			}
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
	return contractFindObjectByStringField(t, values, "name", name, label)
}

func contractFindObjectByStringField(t *testing.T, values []any, field, expected, label string) map[string]any {
	t.Helper()
	for index, value := range values {
		object := contractObject(t, value, fmt.Sprintf("%s[%d]", label, index))
		if contractString(t, object[field], label+" "+field) == expected {
			return object
		}
	}
	t.Fatalf("%s 找不到 %s=%q", label, field, expected)
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

func contractAssertKnownMornleaExtensions(t *testing.T, value any, valuePath string) {
	t.Helper()
	knownRules := map[string]struct{}{
		"text":                    {},
		"follow_must_be_last":     {},
		"loopback_mcp_url":        {},
		"sorted_positions":        {},
		"positions_match_context": {},
	}
	var visit func(any, string)
	visit = func(current any, path string) {
		schema, ok := current.(map[string]any)
		if !ok {
			return
		}
		for keyword := range schema {
			if strings.HasPrefix(keyword, "x-mornlea-") && keyword != "x-mornlea-rules" {
				t.Errorf("schema %s 声明未知或未实现扩展 %s", path, keyword)
			}
		}
		if child, exists := schema["x-mornlea-rules"]; exists {
			rules, ok := child.([]any)
			if !ok || len(rules) == 0 {
				t.Errorf("schema %s 的 x-mornlea-rules 必须是非空 array", path)
			} else {
				for index, rawRule := range rules {
					rule, ok := rawRule.(map[string]any)
					if !ok {
						t.Errorf("schema %s 的 machine rule[%d] 不是 object", path, index)
						continue
					}
					name, ok := rule["name"].(string)
					if !ok || name == "" {
						t.Errorf("schema %s 的 machine rule[%d] 缺少 name", path, index)
						continue
					}
					if _, ok := knownRules[name]; !ok {
						t.Errorf("schema %s 声明未知或未实现 machine rule %q", path, name)
					}
				}
			}
		}
		for _, keyword := range []string{"$defs", "properties"} {
			members, _ := schema[keyword].(map[string]any)
			for name, child := range members {
				visit(child, path+"."+keyword+"."+name)
			}
		}
		for _, keyword := range []string{"oneOf", "prefixItems"} {
			children, _ := schema[keyword].([]any)
			for index, child := range children {
				visit(child, fmt.Sprintf("%s.%s[%d]", path, keyword, index))
			}
		}
		if child, exists := schema["items"]; exists {
			visit(child, path+".items")
		}
	}
	visit(value, valuePath)
}

func contractFindRule(t *testing.T, schema map[string]any, name, label string) map[string]any {
	t.Helper()
	for index, rawRule := range contractArray(t, schema["x-mornlea-rules"], label+" rules") {
		rule := contractObject(t, rawRule, fmt.Sprintf("%s rules[%d]", label, index))
		if contractString(t, rule["name"], label+" rule name") == name {
			return rule
		}
	}
	t.Fatalf("%s 缺少 machine rule %q", label, name)
	return nil
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
