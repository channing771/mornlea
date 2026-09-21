package companion

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// This file is the package-local producer for the two externally owned Agent
// contract families. It executes every committed golden fixture case through the
// same pure JSON Schema 2020-12 validator the cross-language contract tests use,
// so a recorded outcome is what the checked-in contract actually decides about
// the value rather than a copy of the fixture's declared expectation. No Agent
// service, listener, model call or world authority is involved: the producer only
// reads immutable fixtures and calls in-process functions, and the frozen corpus
// files it materializes are read-only inputs for later consumers.

// contractGoldenCase is an alias for the golden case shape the shared fixture
// loader decodes, so this producer reuses that loader instead of defining a
// second, drifting description of the same file.
type contractGoldenCase = struct {
	Name          string                `json:"name"`
	Schema        string                `json:"schema"`
	Reason        string                `json:"reason,omitempty"`
	Value         json.RawMessage       `json:"value"`
	ValueUTF8Hex  string                `json:"value_utf8_hex,omitempty"`
	Context       json.RawMessage       `json:"context,omitempty"`
	ExpectedError contractExpectedError `json:"expected_error,omitempty"`
}

const (
	// agentContractCorpusRelDir is the repository-relative directory holding the
	// frozen Agent contract corpus cases.
	agentContractCorpusRelDir = "testdata/runtime-migration/cases/agent"
	// agentContractHTTPFamily and agentContractMCPFamily are the two externally
	// owned contract families this producer covers. Their eventual owner stays
	// the companion Agent service, so no Rust consumer is claimed here.
	agentContractHTTPFamily = "agent.http"
	agentContractMCPFamily  = "agent.mcp"
	// agentContractVersion is the application contract version both families
	// publish.
	agentContractVersion = "v1"
	// agentContractConsumer is the manifest consumer these two externally owned
	// families record. It is not a Rust consumer name: the executed evidence is
	// Go, and the companion Agent service stays the eventual owner.
	agentContractConsumer = "external:agent-contract"
	// agentContractInvalidValue is the frozen structural rejection category a
	// contract rejection is recorded under when it does not name a more specific
	// frozen condition. The precise rejection always travels in the outcome
	// fields, so classifying coarsely never loses evidence.
	agentContractInvalidValue = "invalid-value"
	// runtimeOracleExportDirEnv names the harness-owned directory explicit
	// fixture export writes into.
	runtimeOracleExportDirEnv = "RUNTIME_ORACLE_EXPORT_DIR"
)

// agentContractSource is one golden fixture file the producer executes.
type agentContractSource struct {
	// dir is the corpus subdirectory this source's cases are frozen under.
	dir string
	// relative is the repository-relative golden fixture path.
	relative string
	// document is the schema document the fixture's cases validate against.
	document string
	// family is the corpus family the source's cases belong to.
	family string
	// mine marks the mine-validation fixture. Its cases carry no JSON schema
	// definition, so they are executed by the Go mine rule instead.
	mine bool
	// invalid records the fixture file's declared role. It is only a coverage
	// cross-check on the producer's own output and is never a recorded outcome.
	invalid bool
}

// agentContractSources is the ordered set of golden fixtures the producer
// executes. The order fixes the source index every case label carries, so two
// fixtures that share a fixture name still produce distinct corpus labels.
var agentContractSources = []agentContractSource{
	{dir: "http-valid", relative: "packages/contracts/companion-agent/http-v1/golden/valid.json", document: "http-v1/schema.json", family: agentContractHTTPFamily},
	{dir: "http-invalid", relative: "packages/contracts/companion-agent/http-v1/golden/invalid.json", document: "http-v1/schema.json", family: agentContractHTTPFamily, invalid: true},
	{dir: "mcp-valid", relative: "packages/contracts/companion-agent/mcp-v1/golden/valid.json", document: "mcp-v1/schema.json", family: agentContractMCPFamily},
	{dir: "mcp-invalid", relative: "packages/contracts/companion-agent/mcp-v1/golden/invalid.json", document: "mcp-v1/schema.json", family: agentContractMCPFamily, invalid: true},
	{dir: "mcp-mine-validation", relative: "packages/contracts/companion-agent/mcp-v1/golden/mine-validation.json", document: "mcp-v1/schema.json", family: agentContractMCPFamily, mine: true},
}

// agentContractInput is the frozen, self-describing corpus input for one case.
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the contract. The value a schema case was validated
// against travels either as inline JSON or, when the fixture encodes an
// invalid-UTF-8 payload as hex, as the hex form the golden fixture itself uses,
// so a replaying consumer can reconstruct the exact bytes from the input alone.
type agentContractInput struct {
	CaseKind      string          `json:"case_kind"`
	Consumer      string          `json:"consumer"`
	Source        string          `json:"source"`
	SourceIndex   int             `json:"source_index"`
	Fixture       string          `json:"fixture"`
	Document      string          `json:"document"`
	Schema        string          `json:"schema,omitempty"`
	Value         json.RawMessage `json:"value,omitempty"`
	ValueUTF8Hex  string          `json:"value_utf8_hex,omitempty"`
	Context       json.RawMessage `json:"context,omitempty"`
	BlockSymbol   string          `json:"block_symbol,omitempty"`
	BlockID       int             `json:"block_id,omitempty"`
	MineSemantics string          `json:"mine_semantics,omitempty"`
}

// agentContractOutcome is the normalized result one executed case publishes.
type agentContractOutcome struct {
	Kind     string         `json:"kind"`
	Category string         `json:"category,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

// agentContractRecord pairs one executed case with its frozen corpus identity.
type agentContractRecord struct {
	source      agentContractSource
	sourceIndex int
	fixture     string
	label       string
	input       agentContractInput
	outcome     agentContractOutcome
}

// TestRuntimeAgentContractOracle executes every valid and invalid fixture case
// of the HTTP and MCP Agent contracts, plus the mine-validation fixture, through
// the existing pure contract validator and freezes the produced evidence.
//
// The run is the producer side of the corpus: it never reads a recorded outcome
// to decide what to publish, it derives every label from the fixture name and
// its source index, and it refuses a duplicate label so two cases can never
// collapse into one corpus identity.
func TestRuntimeAgentContractOracle(t *testing.T) {
	schemas := contractLoadSchemas(t)
	records := agentContractExecute(t, schemas)
	if len(records) == 0 {
		t.Fatal("agent contract producer executed no fixture case")
	}

	labels := make(map[string]string, len(records))
	accepted, rejected := 0, 0
	for _, record := range records {
		if previous, duplicate := labels[record.label]; duplicate {
			t.Fatalf("agent contract label %q is used by both %q and %q", record.label, previous, record.fixture)
		}
		labels[record.label] = record.fixture
		switch record.outcome.Kind {
		case "ok":
			accepted++
			if record.source.invalid {
				t.Errorf("fixture %q comes from a rejecting source but was accepted", record.fixture)
			}
		case "error":
			rejected++
			if !record.source.invalid && !record.source.mine {
				t.Errorf("fixture %q comes from an accepting source but was rejected", record.fixture)
			}
		default:
			t.Fatalf("fixture %q published outcome kind %q, want ok or error", record.fixture, record.outcome.Kind)
		}
	}

	agentContractSyncCorpus(t, records)
	if accepted == 0 || rejected == 0 {
		t.Fatalf("agent contract producer recorded %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// agentContractCommittedInput mirrors the on-disk shape of one frozen corpus
// input for the self-containment check. It is deliberately independent of
// agentContractInput so the check reads what is actually committed rather than
// what the producer's struct happens to describe.
type agentContractCommittedInput struct {
	CaseKind     string          `json:"case_kind"`
	Value        json.RawMessage `json:"value"`
	ValueUTF8Hex string          `json:"value_utf8_hex"`
}

// TestRuntimeAgentContractCorpusInputsAreSelfContaining walks every committed
// corpus input and pins that a schema case carries the value it validates. A
// golden fixture may encode an invalid-UTF-8 payload as hex, so the input has to
// record that form as well: a consumer that replays the corpus cannot re-derive
// those bytes from the golden alone, and an input without either member cannot
// be re-executed at all.
func TestRuntimeAgentContractCorpusInputsAreSelfContaining(t *testing.T) {
	t.Parallel()

	root := contractFixturePath(t, agentContractCorpusRelDir)
	var inputs []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".input.json") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		inputs = append(inputs, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("walk agent contract corpus: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatal("agent contract corpus holds no committed input")
	}
	sort.Strings(inputs)
	for _, relative := range inputs {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read corpus input %s: %v", relative, err)
		}
		var input agentContractCommittedInput
		if err := json.Unmarshal(data, &input); err != nil {
			t.Fatalf("decode corpus input %s: %v", relative, err)
		}
		if input.CaseKind != "schema" {
			continue
		}
		if len(input.Value) == 0 && input.ValueUTF8Hex == "" {
			t.Errorf("corpus input %s carries neither value nor value_utf8_hex", relative)
		}
	}
}

// TestRuntimeAgentContractOracleOutcomesAgreeWithDeclaredRejections pins that an
// executed rejection reproduces the path, keyword and rule the fixture declares,
// without the producer ever reading that declaration to build the outcome. The
// comparison is a second, independent check on evidence that execution already
// produced.
func TestRuntimeAgentContractOracleOutcomesAgreeWithDeclaredRejections(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	records := agentContractExecute(t, schemas)
	for _, source := range agentContractSources {
		if source.mine {
			continue
		}
		golden := contractLoadGolden(t, source.relative)
		if got := agentContractSourceCaseCount(records, source); got != len(golden.Cases) {
			t.Fatalf("%s executed %d cases, want %d", source.relative, got, len(golden.Cases))
		}
		for _, testCase := range golden.Cases {
			record := agentContractRecordFor(t, records, source, testCase.Name)
			if !source.invalid {
				if record.outcome.Kind != "ok" {
					t.Errorf("fixture %q was rejected by the executed validator", testCase.Name)
				}
				continue
			}
			err := agentContractValidate(t, schemas, source, testCase)
			var validationError *contractValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("fixture %q returned a non-structured rejection: %v", testCase.Name, err)
			}
			recordedRule, _ := record.outcome.Fields["rule"].(string)
			if got := record.outcome.Fields["path"]; got != validationError.Path {
				t.Errorf("fixture %q recorded path %v, want %s", testCase.Name, got, validationError.Path)
			}
			if got := record.outcome.Fields["keyword"]; got != validationError.Keyword {
				t.Errorf("fixture %q recorded keyword %v, want %s", testCase.Name, got, validationError.Keyword)
			}
			if recordedRule != validationError.Rule {
				t.Errorf("fixture %q recorded rule %q, want %q", testCase.Name, recordedRule, validationError.Rule)
			}
			if validationError.Path != testCase.ExpectedError.Path ||
				validationError.Keyword != testCase.ExpectedError.Keyword ||
				validationError.Rule != testCase.ExpectedError.Rule {
				t.Errorf("fixture %q executed rejection differs from the declared expectation", testCase.Name)
			}
		}
	}
}

// TestRuntimeAgentContractOracleRedMutations builds the explicit mutations the
// contract change names and records what the validator actually says about each
// one. The expected rejections are pinned from observed execution, so a mutation
// that stops being rejected fails here instead of silently disappearing from the
// evidence.
func TestRuntimeAgentContractOracleRedMutations(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	httpValid := agentContractSourceByDir("http-valid")
	mcpValid := agentContractSourceByDir("mcp-valid")

	upperUUID := func(t *testing.T, value map[string]any) map[string]any {
		t.Helper()
		// The acquire fixture's identity is all digits, so uppercasing it in
		// place would change nothing. The mutation names an upper-cased UUIDv4
		// instead, which is the form the canonical identity rule rejects.
		value["namespace_id"] = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
		return value
	}
	planSteps := func(t *testing.T, value map[string]any) []any {
		t.Helper()
		plan, ok := value["plan"].(map[string]any)
		if !ok {
			t.Fatalf("plan fixture has no plan object")
		}
		steps, ok := plan["steps"].([]any)
		if !ok {
			t.Fatalf("plan fixture has no steps array")
		}
		return steps
	}
	stepByKind := func(t *testing.T, steps []any, kind string) map[string]any {
		t.Helper()
		for _, raw := range steps {
			step, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("plan step is not an object")
			}
			if step["kind"] == kind {
				return step
			}
		}
		t.Fatalf("plan has no %q step", kind)
		return nil
	}

	mutations := []struct {
		name        string
		source      agentContractSource
		fixture     string
		mutate      func(t *testing.T, value map[string]any) map[string]any
		wantPath    string
		wantKeyword string
		wantRule    string
	}{
		{
			name:    "delete lease_id",
			source:  httpValid,
			fixture: "heartbeat carries lease only",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				delete(value, "lease_id")
				return value
			},
			wantPath:    "$.lease_id",
			wantKeyword: "required",
			wantRule:    "lease_id",
		},
		{
			name:    "delete plan generation",
			source:  httpValid,
			fixture: "planner run carries snapshot identity",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				delete(value, "generation")
				return value
			},
			wantPath:    "$.generation",
			wantKeyword: "required",
			wantRule:    "generation",
		},
		{
			// The dialogue request schema is a oneOf over the nonterminal and
			// terminal variants, so a missing identity field surfaces as the
			// parent-level oneOf rejection the shared validator publishes rather
			// than as a branch-specific required error.
			name:    "delete dialogue memory_epoch",
			source:  httpValid,
			fixture: "nonterminal dialogue run",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				delete(value, "memory_epoch")
				return value
			},
			wantPath:    "$",
			wantKeyword: "oneOf",
		},
		{
			name:    "set new_memory_epoch to zero",
			source:  httpValid,
			fixture: "memory delete advances epoch",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				value["new_memory_epoch"] = json.Number("0")
				return value
			},
			wantPath:    "$.new_memory_epoch",
			wantKeyword: "minimum",
		},
		{
			name:        "upper case the namespace UUID",
			source:      httpValid,
			fixture:     "namespace acquire omits lease",
			mutate:      upperUUID,
			wantPath:    "$.namespace_id",
			wantKeyword: "pattern",
		},
		{
			name:    "declare contract_version v2",
			source:  httpValid,
			fixture: "namespace acquire omits lease",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				value["contract_version"] = "v2"
				return value
			},
			wantPath:    "$.contract_version",
			wantKeyword: "const",
		},
		{
			name:    "inject snapshot_id into get_planning_context_input",
			source:  mcpValid,
			fixture: "fixed context input exposes no runtime identity",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				value["snapshot_id"] = "77777777-7777-4777-8777-777777777777"
				return value
			},
			wantPath:    "$.snapshot_id",
			wantKeyword: "additionalProperties",
		},
		{
			name:    "declare an unknown step kind",
			source:  mcpValid,
			fixture: "validator accepts the complete delivered step set",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				stepByKind(t, planSteps(t, value), "go_to")["kind"] = "teleport"
				return value
			},
			wantPath:    "$.plan.steps[0]",
			wantKeyword: "oneOf",
		},
		{
			name:    "place without a block",
			source:  mcpValid,
			fixture: "validator accepts the complete delivered step set",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				steps := planSteps(t, value)
				delete(stepByKind(t, steps, "place"), "block")
				return value
			},
			wantPath:    "$.plan.steps[2]",
			wantKeyword: "oneOf",
		},
		{
			name:    "follow before the last step",
			source:  mcpValid,
			fixture: "validator accepts the complete delivered step set",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				steps := planSteps(t, value)
				reordered := make([]any, 0, len(steps))
				reordered = append(reordered, steps[len(steps)-1])
				reordered = append(reordered, steps[:len(steps)-1]...)
				value["plan"].(map[string]any)["steps"] = reordered
				return value
			},
			wantPath:    "$.plan.steps[0]",
			wantKeyword: "x-mornlea-rules",
			wantRule:    "follow_must_be_last",
		},
		{
			name:    "empty the plan steps",
			source:  mcpValid,
			fixture: "validator accepts the complete delivered step set",
			mutate: func(t *testing.T, value map[string]any) map[string]any {
				t.Helper()
				plan, ok := value["plan"].(map[string]any)
				if !ok {
					t.Fatalf("plan fixture has no plan object")
				}
				plan["steps"] = []any{}
				return value
			},
			wantPath:    "$.plan.steps",
			wantKeyword: "minItems",
		},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			testCase := agentContractGoldenByName(t, mutation.source, mutation.fixture)
			value, ok := agentContractObjectValue(t, testCase.Value).(map[string]any)
			if !ok {
				t.Fatalf("fixture %q is not a JSON object", mutation.fixture)
			}
			mutated := mutation.mutate(t, value)
			var context any
			if len(testCase.Context) != 0 {
				context = contractDecodeRaw(t, testCase.Context, mutation.fixture+"/context")
			}
			err := schemas.validateDefinition(mutation.source.document, testCase.Schema, mutated, context)
			if err == nil {
				t.Fatalf("mutation %q was accepted by %s", mutation.name, testCase.Schema)
			}
			var validationError *contractValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("mutation %q returned a non-structured rejection: %v", mutation.name, err)
			}
			if validationError.Path != mutation.wantPath ||
				validationError.Keyword != mutation.wantKeyword ||
				validationError.Rule != mutation.wantRule {
				t.Fatalf("mutation %q rejected as {%s %s %s}, want {%s %s %s}",
					mutation.name, validationError.Path, validationError.Keyword, validationError.Rule,
					mutation.wantPath, mutation.wantKeyword, mutation.wantRule)
			}
			outcome, err := agentContractRejectionOutcome(testCase.Schema, validationError)
			if err != nil {
				t.Fatalf("mutation %q outcome: %v", mutation.name, err)
			}
			if outcome.Kind != "error" {
				t.Fatalf("mutation %q published kind %q, want error", mutation.name, outcome.Kind)
			}
		})
	}
}

// TestRuntimeAgentContractOracleManifestBindings binds the executed fixtures to
// the manifest surface they exercise: the HTTP identity profiles and endpoint
// table, and the exact six MCP tools with their input and result schemas.
func TestRuntimeAgentContractOracleManifestBindings(t *testing.T) {
	t.Parallel()

	httpManifest := contractReadObject(t, "packages/contracts/companion-agent/http-v1/manifest.json")
	if got := contractString(t, httpManifest["application_contract_version"], "http application version"); got != agentContractVersion {
		t.Fatalf("HTTP application contract version = %q, want %s", got, agentContractVersion)
	}
	profiles := contractObject(t, httpManifest["identity_profiles"], "http identity profiles")
	contractAssertExactKeys(t, profiles, []string{
		"health", "acquire", "lease", "plan_run", "dialogue_run",
		"memory_reconcile", "memory_commit", "memory_delete", "cancel", "error",
	}, "HTTP identity profiles")
	for _, profile := range []struct {
		name   string
		fields []string
	}{
		{"acquire", []string{"contract_version", "request_id", "client_instance_id", "namespace_id"}},
		{"lease", []string{"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id"}},
		{"plan_run", []string{"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id", "companion_id", "generation", "snapshot_id", "snapshot_digest"}},
		{"dialogue_run", []string{"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "run_id", "companion_id", "generation", "memory_epoch"}},
		{"memory_delete", []string{"contract_version", "request_id", "client_instance_id", "namespace_id", "lease_id", "companion_id", "old_memory_epoch", "new_memory_epoch", "tombstone_operation_id"}},
	} {
		contractAssertStringList(t, profiles[profile.name], profile.fields, "HTTP identity profile "+profile.name)
	}
	wantEndpoints := []string{
		"GET /livez",
		"GET /readyz",
		"POST /v1/namespaces/acquire",
		"POST /v1/namespaces/heartbeat",
		"POST /v1/namespaces/release",
		"POST /v1/plan",
		"POST /v1/dialogue",
		"POST /v1/memory/reconcile",
		"POST /v1/memory/commit",
		"POST /v1/memory/delete",
		"POST /v1/runs/cancel",
	}
	routes := contractArray(t, httpManifest["routes"], "http routes")
	if len(routes) != len(wantEndpoints) {
		t.Fatalf("HTTP route count = %d, want %d", len(routes), len(wantEndpoints))
	}
	seenEndpoints := make(map[string]struct{}, len(routes))
	for index, rawRoute := range routes {
		route := contractObject(t, rawRoute, fmt.Sprintf("http routes[%d]", index))
		endpoint := contractString(t, route["method"], "http route method") + " " + contractString(t, route["path"], "http route path")
		known := false
		for _, want := range wantEndpoints {
			if endpoint == want {
				known = true
				break
			}
		}
		if !known {
			t.Fatalf("HTTP route %q is not part of the executed contract surface", endpoint)
		}
		if _, duplicate := seenEndpoints[endpoint]; duplicate {
			t.Fatalf("HTTP route %q is declared twice", endpoint)
		}
		seenEndpoints[endpoint] = struct{}{}
	}
	if len(seenEndpoints) != len(wantEndpoints) {
		t.Fatalf("HTTP endpoints seen = %d, want %d", len(seenEndpoints), len(wantEndpoints))
	}

	mcpManifest := contractReadObject(t, "packages/contracts/companion-agent/mcp-v1/manifest.json")
	if got := contractString(t, mcpManifest["application_contract_version"], "mcp application version"); got != agentContractVersion {
		t.Fatalf("MCP application contract version = %q, want %s", got, agentContractVersion)
	}
	if got := contractString(t, mcpManifest["mcp_protocol_version"], "mcp protocol version"); got != "2025-11-25" {
		t.Fatalf("MCP protocol version = %q, want 2025-11-25", got)
	}
	if got := contractString(t, mcpManifest["endpoint_path"], "mcp endpoint path"); got != "/mcp" {
		t.Fatalf("MCP endpoint path = %q, want /mcp", got)
	}
	tools := contractArray(t, mcpManifest["tools"], "mcp tools")
	if len(tools) != 6 {
		t.Fatalf("MCP tool count = %d, want 6", len(tools))
	}
	wantToolSchemas := map[string][2]string{
		"get_planning_context": {"get_planning_context_input", "get_planning_context_result"},
		"list_affordances":     {"list_affordances_input", "list_affordances_result"},
		"inspect_inventory":    {"inspect_inventory_input", "inspect_inventory_result"},
		"find_visible_blocks":  {"find_visible_blocks_input", "find_visible_blocks_result"},
		"query_terrain":        {"query_terrain_input", "query_terrain_result"},
		"validate_plan":        {"validate_plan_input", "validate_plan_result"},
	}
	seenTools := make(map[string]struct{}, len(tools))
	for index, rawTool := range tools {
		tool := contractObject(t, rawTool, fmt.Sprintf("mcp tools[%d]", index))
		name := contractString(t, tool["name"], "mcp tool name")
		want, known := wantToolSchemas[name]
		if !known {
			t.Fatalf("MCP tool %q is not part of the executed contract surface", name)
		}
		if _, duplicate := seenTools[name]; duplicate {
			t.Fatalf("MCP tool %q is declared twice", name)
		}
		seenTools[name] = struct{}{}
		if got := contractString(t, tool["input_schema"], name+" input schema"); got != want[0] {
			t.Errorf("MCP tool %q input schema = %q, want %q", name, got, want[0])
		}
		if got := contractString(t, tool["result_schema"], name+" result schema"); got != want[1] {
			t.Errorf("MCP tool %q result schema = %q, want %q", name, got, want[1])
		}
	}
	for _, name := range PlanningToolNames() {
		if _, executed := seenTools[name]; !executed {
			t.Errorf("Go planning tool %q has no manifest entry on the executed contract surface", name)
		}
	}
}

// TestRuntimeAgentContractOracleDTOCodecsAgreeWithSchemaValidator cross-checks
// the two independent Go validators over every HTTP fixture: the checked-in JSON
// schema and the actual DTO codec mapping must reach the same verdict, so a
// fixture one validator accepts and the other rejects is a contract break rather
// than an unnoticed disagreement.
func TestRuntimeAgentContractOracleDTOCodecsAgreeWithSchemaValidator(t *testing.T) {
	t.Parallel()

	schemas := contractLoadSchemas(t)
	for _, source := range agentContractSources {
		if source.mine || source.family != agentContractHTTPFamily {
			continue
		}
		for _, testCase := range contractLoadGolden(t, source.relative).Cases {
			schemaAccepted := agentContractValidate(t, schemas, source, testCase) == nil
			dtoAccepted := agentGoldenCodecValid(testCase.Schema, testCase.Value)
			if schemaAccepted != dtoAccepted {
				t.Errorf("fixture %q: schema validator accepted=%v, DTO codec accepted=%v", testCase.Name, schemaAccepted, dtoAccepted)
			}
		}
	}
}

// TestRuntimeAgentContractOraclePlanningToolsExecuteFixtures re-executes the six
// MCP tool inputs through the real Go planning-tool path using the same
// immutable test snapshot the existing machine-fixture test uses, and validates
// each canonical result against the checked-in result schema. The Go tool
// registry and the manifest must name the same six tools.
func TestRuntimeAgentContractOraclePlanningToolsExecuteFixtures(t *testing.T) {
	t.Parallel()

	lease, cleanup := planningToolLease(t)
	defer cleanup()
	schemas := contractLoadSchemas(t)
	valid := contractLoadGolden(t, "packages/contracts/companion-agent/mcp-v1/golden/valid.json")
	toolByInput := map[string]string{
		"get_planning_context_input": ToolGetPlanningContext,
		"list_affordances_input":     ToolListAffordances,
		"inspect_inventory_input":    ToolInspectInventory,
		"find_visible_blocks_input":  ToolFindVisibleBlocks,
		"query_terrain_input":        ToolQueryTerrain,
		"validate_plan_input":        ToolValidatePlan,
	}
	resultSchema := map[string]string{
		ToolGetPlanningContext: "get_planning_context_result",
		ToolListAffordances:    "list_affordances_result",
		ToolInspectInventory:   "inspect_inventory_result",
		ToolFindVisibleBlocks:  "find_visible_blocks_result",
		ToolQueryTerrain:       "query_terrain_result",
		ToolValidatePlan:       "validate_plan_result",
	}
	executed := make(map[string]int, len(toolByInput))
	for _, testCase := range valid.Cases {
		tool, ok := toolByInput[testCase.Schema]
		if !ok {
			continue
		}
		result, err := ExecutePlanningTool(context.Background(), lease, tool, testCase.Value)
		if err != nil {
			t.Errorf("fixture %q tool %s: %v", testCase.Name, tool, err)
			continue
		}
		executed[tool]++
		if len(result.Canonical) > PlanningToolCanonicalLimit(tool) {
			t.Errorf("tool %s canonical=%d exceeds the manifest limit", tool, len(result.Canonical))
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(result.Canonical)))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Errorf("tool %s canonical JSON: %v", tool, err)
			continue
		}
		var queryContext any
		if tool == ToolQueryTerrain {
			queryContext = agentContractObjectValue(t, testCase.Value)
		}
		if err := schemas.validateDefinition("mcp-v1/schema.json", resultSchema[tool], value, queryContext); err != nil {
			t.Errorf("tool %s result rejected by the checked-in schema: %v", tool, err)
		}
	}
	for _, tool := range PlanningToolNames() {
		if executed[tool] == 0 {
			t.Errorf("valid MCP golden did not drive tool %s", tool)
		}
	}
}

// agentContractExecute runs every fixture case of every source through the
// existing validator and returns one record per case in source order.
func agentContractExecute(t *testing.T, schemas contractSchemaSet) []agentContractRecord {
	t.Helper()

	var records []agentContractRecord
	labels := make(map[string]string)
	for index, source := range agentContractSources {
		if source.mine {
			records = append(records, agentContractExecuteMine(t, index, source)...)
			continue
		}
		for _, testCase := range contractLoadGolden(t, source.relative).Cases {
			label, err := agentContractLabel(testCase.Name, index)
			if err != nil {
				t.Fatalf("%s: %v", source.relative, err)
			}
			if previous, duplicate := labels[label]; duplicate {
				t.Fatalf("agent contract label %q is used by both %q and %q", label, previous, testCase.Name)
			}
			labels[label] = testCase.Name

			value := agentContractCaseValue(t, testCase, source.relative+"/"+testCase.Name)
			var context any
			if len(testCase.Context) != 0 {
				context = contractDecodeRaw(t, testCase.Context, source.relative+"/"+testCase.Name+"/context")
			}
			outcome, err := agentContractOutcomeFor(testCase.Schema, value,
				schemas.validateDefinition(source.document, testCase.Schema, value, context))
			if err != nil {
				t.Fatalf("fixture %q: %v", testCase.Name, err)
			}
			records = append(records, agentContractRecord{
				source:      source,
				sourceIndex: index,
				fixture:     testCase.Name,
				label:       label,
				input: agentContractInput{
					CaseKind:     "schema",
					Consumer:     agentContractConsumer,
					Source:       source.relative,
					SourceIndex:  index,
					Fixture:      testCase.Name,
					Document:     source.document,
					Schema:       testCase.Schema,
					Value:        testCase.Value,
					ValueUTF8Hex: testCase.ValueUTF8Hex,
					Context:      testCase.Context,
				},
				outcome: outcome,
			})
		}
	}
	return records
}

// agentContractExecuteMine runs the mine-validation fixture through the Go mine
// rule, which is the authority behind the MCP tool's unmineable-target decision.
func agentContractExecuteMine(t *testing.T, index int, source agentContractSource) []agentContractRecord {
	t.Helper()

	document := contractReadObject(t, source.relative)
	failureCode := contractString(t, document["failure_code"], "mine failure code")
	var records []agentContractRecord
	for caseIndex, rawCase := range contractArray(t, document["cases"], "mine cases") {
		testCase := contractObject(t, rawCase, fmt.Sprintf("mine cases[%d]", caseIndex))
		fixture := contractString(t, testCase["name"], "mine case name")
		label, err := agentContractLabel(fixture, index)
		if err != nil {
			t.Fatalf("%s: %v", source.relative, err)
		}
		symbol := contractString(t, testCase["block_symbol"], "mine block symbol")
		blockID := int(contractInt64(t, testCase["block_id"], symbol+" block id"))
		semantics := contractString(t, testCase["mine_semantics"], symbol+" mine semantics")
		block := core.BlockID(blockID)
		_, hasDrop := core.BlockDrop(block)
		mineable := planMineableBlock(block)

		fields := map[string]any{
			"block_symbol":   symbol,
			"block_id":       blockID,
			"mine_semantics": semantics,
			"has_block_drop": hasDrop,
			"mineable":       mineable,
		}
		outcome := agentContractOutcome{Kind: "ok", Category: "mine", Fields: fields}
		if !mineable {
			fields["code"] = failureCode
			outcome = agentContractOutcome{Kind: "error", Category: agentContractInvalidValue, Fields: fields}
		}
		if accepted := contractBool(t, testCase["accepted"], symbol+" accepted"); accepted != mineable {
			t.Errorf("mine golden %s accepted=%v but planMineableBlock=%v", symbol, accepted, mineable)
		}
		if hasDrop != contractBool(t, testCase["has_block_drop"], symbol+" has_block_drop") {
			t.Errorf("mine golden %s has_block_drop disagrees with core.BlockDrop", symbol)
		}
		records = append(records, agentContractRecord{
			source:      source,
			sourceIndex: index,
			fixture:     fixture,
			label:       label,
			input: agentContractInput{
				CaseKind:      "mine",
				Consumer:      agentContractConsumer,
				Source:        source.relative,
				SourceIndex:   index,
				Fixture:       fixture,
				Document:      source.document,
				BlockSymbol:   symbol,
				BlockID:       blockID,
				MineSemantics: semantics,
			},
			outcome: outcome,
		})
	}
	return records
}

// agentContractOutcomeFor normalizes one validator result. An accepted value
// publishes its schema and the canonical key set of the value that was actually
// validated; a rejection publishes the exact path, keyword and rule the
// validator produced plus a frozen rejection category.
func agentContractOutcomeFor(schema string, value any, err error) (agentContractOutcome, error) {
	if err == nil {
		keys, _ := agentContractValueKeys(value)
		return agentContractOutcome{
			Kind:     "ok",
			Category: schema,
			Fields:   map[string]any{"schema": schema, "keys": keys},
		}, nil
	}
	var validationError *contractValidationError
	if !errors.As(err, &validationError) {
		return agentContractOutcome{}, fmt.Errorf("validator returned a non-structured rejection: %w", err)
	}
	return agentContractRejectionOutcome(schema, validationError)
}

// agentContractRejectionOutcome records one structured rejection.
func agentContractRejectionOutcome(schema string, validationError *contractValidationError) (agentContractOutcome, error) {
	fields := map[string]any{
		"schema":  schema,
		"path":    validationError.Path,
		"keyword": validationError.Keyword,
	}
	if validationError.Rule != "" {
		fields["rule"] = validationError.Rule
	}
	return agentContractOutcome{
		Kind:     "error",
		Category: agentContractRejectionCategory(validationError.Path, validationError.Keyword),
		Fields:   fields,
	}, nil
}

// agentContractRejectionCategory maps one structured rejection onto the frozen
// structural vocabulary. The path and keyword name the wire condition, so a
// version declaration, an unknown enumeration member and a non-canonical
// identity each keep their own frozen category; every other rejection is a value
// outside the authoritative contract.
func agentContractRejectionCategory(path, keyword string) string {
	switch {
	case path == "$.contract_version":
		return "unsupported-version"
	case keyword == "enum":
		return "invalid-enum"
	case keyword == "pattern":
		return "invalid-identity"
	default:
		return agentContractInvalidValue
	}
}

// agentContractValueKeys projects the sorted top-level keys of an accepted JSON
// object. A non-object value has no keys, so the projection stays empty instead
// of inventing one.
func agentContractValueKeys(value any) ([]string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return []string{}, true
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, true
}

// agentContractLabel derives the deterministic corpus label for one fixture
// case: the lowercase slug of the fixture name plus the index of its source, so
// the same fixture name in two sources stays two distinct corpus identities.
func agentContractLabel(name string, sourceIndex int) (string, error) {
	slug := agentContractLabelSlug(name)
	if slug == "" {
		return "", fmt.Errorf("fixture name %q has no lowercase slug characters", name)
	}
	return slug + "-" + strconv.Itoa(sourceIndex), nil
}

// agentContractLabelSlug lowercases a fixture name and folds every run of
// non-alphanumeric characters into one separator, so the label is stable across
// runs and safe as a file name and a corpus identity suffix.
func agentContractLabelSlug(name string) string {
	var builder strings.Builder
	previousSeparator := false
	for _, symbol := range strings.ToLower(name) {
		alphanumeric := (symbol >= 'a' && symbol <= 'z') || (symbol >= '0' && symbol <= '9')
		if alphanumeric {
			builder.WriteRune(symbol)
			previousSeparator = false
			continue
		}
		if builder.Len() > 0 && !previousSeparator {
			builder.WriteByte('-')
		}
		previousSeparator = true
	}
	return strings.TrimSuffix(builder.String(), "-")
}

// agentContractSourceByDir returns the source frozen under one corpus
// subdirectory.
func agentContractSourceByDir(dir string) agentContractSource {
	for _, source := range agentContractSources {
		if source.dir == dir {
			return source
		}
	}
	return agentContractSource{}
}

// agentContractCaseValue decodes one fixture value exactly the way the shared
// schema validator decodes it, including the invalid-UTF-8 hex form.
func agentContractCaseValue(t *testing.T, testCase contractGoldenCase, label string) any {
	t.Helper()
	if testCase.ValueUTF8Hex != "" {
		rawValue, err := hex.DecodeString(testCase.ValueUTF8Hex)
		if err != nil {
			t.Fatalf("%s value_utf8_hex is not valid hex: %v", label, err)
		}
		return string(rawValue)
	}
	return contractDecodeRaw(t, testCase.Value, label)
}

// agentContractObjectValue decodes a fixture value for callers that need the
// decoded form rather than the validated form.
func agentContractObjectValue(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	return contractDecodeRaw(t, raw, "agent contract value")
}

// agentContractGoldenByName returns the named case of one golden fixture.
func agentContractGoldenByName(t *testing.T, source agentContractSource, name string) contractGoldenCase {
	t.Helper()
	for _, testCase := range contractLoadGolden(t, source.relative).Cases {
		if testCase.Name == name {
			return testCase
		}
	}
	t.Fatalf("%s has no case %q", source.relative, name)
	return contractGoldenCase{}
}

// agentContractValidate validates one fixture case and returns the validator's
// own result.
func agentContractValidate(t *testing.T, schemas contractSchemaSet, source agentContractSource, testCase contractGoldenCase) error {
	t.Helper()
	value := agentContractCaseValue(t, testCase, source.relative+"/"+testCase.Name)
	var context any
	if len(testCase.Context) != 0 {
		context = contractDecodeRaw(t, testCase.Context, source.relative+"/"+testCase.Name+"/context")
	}
	return schemas.validateDefinition(source.document, testCase.Schema, value, context)
}

// agentContractRecordFor returns the executed record for one fixture case.
func agentContractRecordFor(t *testing.T, records []agentContractRecord, source agentContractSource, fixture string) agentContractRecord {
	t.Helper()
	for _, record := range records {
		if record.source.relative == source.relative && record.fixture == fixture {
			return record
		}
	}
	t.Fatalf("no executed record for %s/%s", source.relative, fixture)
	return agentContractRecord{}
}

// agentContractSourceCaseCount counts the executed records of one source.
func agentContractSourceCaseCount(records []agentContractRecord, source agentContractSource) int {
	count := 0
	for _, record := range records {
		if record.source.relative == source.relative {
			count++
		}
	}
	return count
}

// agentContractSyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the validator produces now, so a drifted artifact fails instead of being
// regenerated. The explicit update flag rewrites them, which is the only way a
// frozen artifact changes.
func agentContractSyncCorpus(t *testing.T, records []agentContractRecord) {
	t.Helper()

	root := contractFixturePath(t, agentContractCorpusRelDir)
	want := make(map[string][]byte, len(records)*2)
	for _, record := range records {
		input, err := json.MarshalIndent(record.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", record.label, err)
		}
		outcome, err := json.MarshalIndent(record.outcome, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus outcome for %s: %v", record.label, err)
		}
		relative := filepath.ToSlash(filepath.Join(record.source.dir, record.label))
		want[relative+".input.json"] = append(input, '\n')
		want[relative+".expected.json"] = append(outcome, '\n')
	}

	for relative, data := range want {
		target := filepath.Join(root, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed validator", relative)
		}
	}

	var committed []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		committed = append(committed, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("walk corpus directory: %v", err)
	}
	sort.Strings(committed)
	for _, relative := range committed {
		if _, expected := want[relative]; !expected {
			t.Errorf("frozen corpus case %s is not produced by any executed fixture case", relative)
		}
	}

	var relatives []string
	for relative := range want {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	assets := make([]generatedAsset, 0, len(relatives))
	for _, relative := range relatives {
		assets = append(assets, generatedAsset{
			RelativePath: relative,
			Data:         want[relative],
		})
	}
	repoRoot := filepath.Clean(contractFixturePath(t, ""))
	exportGeneratedAssetsFromEnvironment(t, repoRoot, "companion/agent-contract", assets)
}

type generatedAsset struct {
	RelativePath string
	Data         []byte
}

var companionValidProducerIDs = map[string]bool{
	"companion/agent-contract": true,
}

func isLivePath(root, target string) (bool, error) {
	if strings.TrimSpace(target) == "" {
		return false, nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("companion: resolve repository root: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false, fmt.Errorf("companion: resolve path %s: %w", target, err)
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absTarget); err == nil {
		absTarget = resolved
	} else {
		parent := filepath.Dir(absTarget)
		if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			absTarget = filepath.Join(resolvedParent, filepath.Base(absTarget))
		}
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, fmt.Errorf("companion: compare path %s: %w", target, err)
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." {
		return false, nil
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func exportGeneratedAssets(
	repoRoot string,
	exportRoot string,
	producerID string,
	assets []generatedAsset,
) (string, error) {
	if !companionValidProducerIDs[producerID] {
		return "", fmt.Errorf("companion: unrecognized producer ID: %q", producerID)
	}
	if strings.TrimSpace(exportRoot) == "" {
		return "", fmt.Errorf("companion: export root cannot be empty")
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("companion: resolve repository root %s: %w", repoRoot, err)
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}

	absExport, err := filepath.Abs(exportRoot)
	if err != nil {
		return "", fmt.Errorf("companion: resolve export root %s: %w", exportRoot, err)
	}

	// Walk upward from absExport until an existing ancestor is found.
	var missing []string
	existing := absExport
	for {
		_, statErr := os.Lstat(existing)
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("companion: stat export root %s: %w", existing, statErr)
		}
		missing = append(missing, filepath.Base(existing))
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("companion: export root %s has no existing ancestor", exportRoot)
		}
		existing = parent
	}
	for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
		missing[i], missing[j] = missing[j], missing[i]
	}

	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("companion: resolve export ancestor %s: %w", existing, err)
	}
	resolvedExport := filepath.Join(append([]string{resolvedExisting}, missing...)...)

	// Check repository containment on both resolved export path and existing ancestor.
	live, err := isLivePath(absRoot, resolvedExport)
	if err != nil {
		return "", err
	}
	if live {
		return "", fmt.Errorf("companion: live-path write rejected: export root %s is inside repository", exportRoot)
	}
	liveExisting, err := isLivePath(absRoot, existing)
	if err != nil {
		return "", err
	}
	if liveExisting {
		return "", fmt.Errorf("companion: live-path write rejected: export ancestor %s is inside repository", existing)
	}

	// Reject symlink components below the nearest existing ancestor.
	for component := existing; ; component = filepath.Dir(component) {
		if component == "/" || component == "." || component == filepath.Dir(component) {
			break
		}
		if component == "/var" || component == "/tmp" || component == "/etc" {
			break
		}
		info, statErr := os.Lstat(component)
		if statErr != nil {
			return "", fmt.Errorf("companion: stat export path %s: %w", component, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("companion: symlink component rejected: %s", component)
		}
	}

	// Check and create the fixed producer child.
	producerChild := filepath.Join(absExport, filepath.FromSlash(producerID))
	if _, statErr := os.Lstat(producerChild); statErr == nil {
		return "", fmt.Errorf("companion: producer child already exists: %s", producerChild)
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("companion: stat producer child %s: %w", producerChild, statErr)
	}

	if err := os.MkdirAll(producerChild, 0o755); err != nil {
		return "", fmt.Errorf("companion: create producer directory %s: %w", producerChild, err)
	}

	for _, asset := range assets {
		if strings.TrimSpace(asset.RelativePath) == "" {
			return "", fmt.Errorf("companion: empty asset relative path")
		}
		if strings.Contains(asset.RelativePath, "\\") {
			return "", fmt.Errorf("companion: backslash rejected in relative path: %s", asset.RelativePath)
		}
		if filepath.IsAbs(asset.RelativePath) || strings.HasPrefix(asset.RelativePath, "/") {
			return "", fmt.Errorf("companion: absolute path rejected: %s", asset.RelativePath)
		}
		for _, part := range strings.Split(asset.RelativePath, "/") {
			if part == "." || part == ".." {
				return "", fmt.Errorf("companion: relative path contains ./..: %s", asset.RelativePath)
			}
		}
		cleaned := filepath.Clean(asset.RelativePath)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("companion: path escapes producer directory: %s", asset.RelativePath)
		}

		target := filepath.Join(producerChild, filepath.FromSlash(asset.RelativePath))
		rel, relErr := filepath.Rel(producerChild, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("companion: path escapes producer directory: %s", asset.RelativePath)
		}

		targetDir := filepath.Dir(target)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return "", fmt.Errorf("companion: create asset directory %s: %w", targetDir, err)
		}

		for d := targetDir; d != producerChild && len(d) > len(producerChild); d = filepath.Dir(d) {
			info, lstatErr := os.Lstat(d)
			if lstatErr != nil {
				return "", fmt.Errorf("companion: stat asset dir %s: %w", d, lstatErr)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("companion: symlink component rejected: %s", d)
			}
		}

		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return "", fmt.Errorf("companion: create exclusive asset %s: %w", asset.RelativePath, err)
		}
		if _, err := f.Write(asset.Data); err != nil {
			f.Close()
			return "", fmt.Errorf("companion: write asset %s: %w", asset.RelativePath, err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("companion: close asset %s: %w", asset.RelativePath, err)
		}
	}

	return producerChild, nil
}

func exportGeneratedAssetsFromEnvironment(
	t *testing.T,
	repoRoot string,
	producerID string,
	assets []generatedAsset,
) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	dir, err := exportGeneratedAssets(repoRoot, exportRoot, producerID, assets)
	if err != nil {
		t.Fatalf("export generated assets for %s: %v", producerID, err)
	}
	return dir
}

// TestCompanionExportGeneratedAssets pins that the companion exporter follows
// the same validation order as the runtime oracle exporter.
func TestCompanionExportGeneratedAssets(t *testing.T) {
	repoRoot := filepath.Clean(contractFixturePath(t, ""))

	// 1. Live path containment rejection
	contained := filepath.Join(repoRoot, "companion-export-probe")
	assets := []generatedAsset{{RelativePath: "test.json", Data: []byte("{}\n")}}
	_, err := exportGeneratedAssets(repoRoot, contained, "companion/agent-contract", assets)
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected live-path rejection, got: %v", err)
	}

	// 2. Symlink ancestor rejection
	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	exportRoot := filepath.Join(link, "export")
	_, err = exportGeneratedAssets(repoRoot, exportRoot, "companion/agent-contract", assets)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got: %v", err)
	}

	// 3. Escaping relative path rejection
	freshExport := filepath.Join(t.TempDir(), "escaping-export")
	badAssets := []generatedAsset{{RelativePath: "../escaped.json", Data: []byte("{}\n")}}
	_, err = exportGeneratedAssets(repoRoot, freshExport, "companion/agent-contract", badAssets)
	if err == nil {
		t.Fatalf("expected escaping path rejection, got nil")
	}

	// 4. Successful export and preexisting child rejection
	targetExport := filepath.Join(t.TempDir(), "valid-export")
	pub, err := exportGeneratedAssets(repoRoot, targetExport, "companion/agent-contract", assets)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	wantPub := filepath.Join(targetExport, "companion", "agent-contract")
	if pub != wantPub {
		t.Fatalf("published = %s, want %s", pub, wantPub)
	}

	// Re-export must fail
	_, err = exportGeneratedAssets(repoRoot, targetExport, "companion/agent-contract", assets)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists rejection, got: %v", err)
	}
}
