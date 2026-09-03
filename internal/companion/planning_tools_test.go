package companion

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/pathfind"
)

func TestPlanningToolsConsumeValidMachineFixtures(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	valid := contractLoadGolden(t, "contracts/companion-agent/mcp-v1/golden/valid.json")
	schemas := contractLoadSchemas(t)
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
	seen := make(map[string]int)
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
		seen[tool]++
		if len(result.Canonical) > PlanningToolCanonicalLimit(tool) {
			t.Errorf("tool %s canonical=%d，超过 manifest 上限", tool, len(result.Canonical))
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
			var input map[string]any
			if err := json.Unmarshal(testCase.Value, &input); err != nil {
				t.Fatal(err)
			}
			queryContext = input
		}
		if err := schemas.validateDefinition("mcp-v1/schema.json", resultSchema[tool], value, queryContext); err != nil {
			t.Errorf("tool %s 输出未通过 checked-in schema: %v\n%s", tool, err, result.Canonical)
		}
	}
	for _, tool := range PlanningToolNames() {
		if seen[tool] == 0 {
			t.Errorf("valid golden 没有驱动 tool %s", tool)
		}
	}
}

func TestPlanningToolsRejectInvalidInputGoldens(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	invalid := contractLoadGolden(t, "contracts/companion-agent/mcp-v1/golden/invalid.json")
	toolByInput := map[string]string{
		"inspect_inventory_input":   ToolInspectInventory,
		"find_visible_blocks_input": ToolFindVisibleBlocks,
		"query_terrain_input":       ToolQueryTerrain,
	}
	for _, testCase := range invalid.Cases {
		if tool, ok := toolByInput[testCase.Schema]; ok {
			if _, err := ExecutePlanningTool(context.Background(), lease, tool, testCase.Value); !errors.Is(err, ErrPlanningToolInvalidInput) {
				t.Errorf("fixture %q err=%v，want ErrPlanningToolInvalidInput", testCase.Name, err)
			}
			continue
		}
		if testCase.Schema != "validate_plan_input" {
			continue
		}
		result, err := ExecutePlanningTool(context.Background(), lease, ToolValidatePlan, testCase.Value)
		if err != nil {
			t.Errorf("validator fixture %q 返回 protocol error: %v", testCase.Name, err)
			continue
		}
		if got := planningResultCode(t, result.Canonical); got != ValidatorInvalidSchema {
			t.Errorf("validator fixture %q code=%q，want %q", testCase.Name, got, ValidatorInvalidSchema)
		}
	}
}

func TestPlanningToolsDomainFailuresAndOrdering(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()

	find, err := ExecutePlanningTool(context.Background(), lease, ToolFindVisibleBlocks,
		json.RawMessage(`{"block_names":["missing_block"],"limit":64}`))
	if err != nil || !find.DomainFailure || string(find.Canonical) != `{"code":"unknown_block","hint":"unknown canonical block name"}` {
		t.Fatalf("find domain failure=%s err=%v", find.Canonical, err)
	}
	query, err := ExecutePlanningTool(context.Background(), lease, ToolQueryTerrain,
		json.RawMessage(`{"positions":[{"x":4,"y":64,"z":-1},{"x":999,"y":64,"z":0}]}`))
	if err != nil || !query.DomainFailure || string(query.Canonical) != `{"code":"out_of_bounds","hint":"position is outside the frozen projection"}` {
		t.Fatalf("query domain failure=%s err=%v", query.Canonical, err)
	}

	ordered, err := ExecutePlanningTool(context.Background(), lease, ToolQueryTerrain,
		json.RawMessage(`{"positions":[{"x":10,"y":63,"z":-2},{"x":8,"y":63,"z":-2},{"x":10,"y":63,"z":-2}]}`))
	if err != nil {
		t.Fatalf("query ordered: %v", err)
	}
	var decoded struct {
		Terrain []struct {
			Position digestBlockPosition `json:"position"`
			Block    string              `json:"block_name"`
		} `json:"terrain"`
	}
	if err := json.Unmarshal(ordered.Canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Terrain) != 3 || decoded.Terrain[0].Position.X != 10 || decoded.Terrain[1].Position.X != 8 ||
		decoded.Terrain[2].Position.X != 10 || decoded.Terrain[0].Block != "furnace" {
		t.Fatalf("query 未保留顺序/重复: %+v", decoded.Terrain)
	}
	const wantOrdered = `{"terrain":[{"block_name":"furnace","height":63,"position":{"x":10,"y":63,"z":-2}},{"block_name":"stone","height":63,"position":{"x":8,"y":63,"z":-2}},{"block_name":"furnace","height":63,"position":{"x":10,"y":63,"z":-2}}]}`
	if string(ordered.Canonical) != wantOrdered {
		t.Fatalf("query exact wire=%s", ordered.Canonical)
	}

	visible, err := ExecutePlanningTool(context.Background(), lease, ToolFindVisibleBlocks,
		json.RawMessage(`{"block_names":["furnace","stone"],"limit":2}`))
	if err != nil {
		t.Fatalf("find visible: %v", err)
	}
	if strings.Index(string(visible.Canonical), `"x":8`) > strings.Index(string(visible.Canonical), `"x":10`) {
		t.Fatalf("find visible 未保持坐标顺序: %s", visible.Canonical)
	}

	inventory, err := ExecutePlanningTool(context.Background(), lease, ToolInspectInventory,
		json.RawMessage(`{"offset":3,"limit":3}`))
	if err != nil || !strings.Contains(string(inventory.Canonical), `"slot":4`) || strings.Contains(string(inventory.Canonical), `"slot":0`) {
		t.Fatalf("inventory paging=%s err=%v", inventory.Canonical, err)
	}
}

func TestPlanningAffordancesBoundsLargestSnapshotByCompleteCoordinatePrefix(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	lease.Snapshot = planningToolLargestAffordanceSnapshot(t)

	first, err := ExecutePlanningTool(context.Background(), lease, ToolListAffordances, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_affordances 最大合法快照: %v", err)
	}
	second, err := ExecutePlanningTool(context.Background(), lease, ToolListAffordances, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_affordances 重复执行: %v", err)
	}
	if !json.Valid(first.Canonical) || len(first.Canonical) > PlanningToolCanonicalLimit(ToolListAffordances) ||
		!bytes.Equal(first.Canonical, second.Canonical) {
		t.Fatalf("bounded affordances len/valid/stable=%d/%v/%v", len(first.Canonical), json.Valid(first.Canonical), bytes.Equal(first.Canonical, second.Canonical))
	}
	var decoded planningAffordancesResult
	if err := json.Unmarshal(first.Canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.OnlinePlayers) != MaxPlanOnlinePlayers || len(decoded.VisibleBlocks) == 0 ||
		len(decoded.VisibleBlocks) >= MaxPlanExposedBlocks {
		t.Fatalf("bounded affordances players/blocks=%d/%d", len(decoded.OnlinePlayers), len(decoded.VisibleBlocks))
	}
	for index, block := range decoded.VisibleBlocks {
		want := lease.Snapshot.ExposedBlocks[index]
		if block.Position != digestPosition(want.Pos) || block.BlockID != want.Block {
			t.Fatalf("visible_blocks[%d]=%+v，不是快照坐标前缀 %+v", index, block, want)
		}
	}
	next := lease.Snapshot.ExposedBlocks[len(decoded.VisibleBlocks)]
	drop := "stone"
	withNext := decoded
	withNext.VisibleBlocks = append(append([]planningVisibleBlock(nil), decoded.VisibleBlocks...), planningVisibleBlock{
		BlockID: next.Block, BlockName: "stone", DropItem: &drop,
		MineSemantics: "single_drop", Position: digestPosition(next.Pos),
	})
	encodedNext, err := canonicalJSON(withNext)
	if err != nil {
		t.Fatal(err)
	}
	if len(encodedNext) <= PlanningToolCanonicalLimit(ToolListAffordances) {
		t.Fatalf("visible_blocks=%d 不是最长完整前缀: next len=%d", len(decoded.VisibleBlocks), len(encodedNext))
	}
}

func TestPlanningAffordancesByteBoundEmptyAndFirstItemSemantics(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	emptyLease := lease
	emptyLease.Snapshot.ExposedBlocks = nil
	empty, err := ExecutePlanningTool(context.Background(), emptyLease, ToolListAffordances, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("empty list_affordances: %v", err)
	}
	var emptyResult planningAffordancesResult
	if err := json.Unmarshal(empty.Canonical, &emptyResult); err != nil || emptyResult.VisibleBlocks == nil || len(emptyResult.VisibleBlocks) != 0 {
		t.Fatalf("empty visible_blocks=%v err=%v wire=%s", emptyResult.VisibleBlocks, err, empty.Canonical)
	}
	oneLease := lease
	oneLease.Snapshot.ExposedBlocks = oneLease.Snapshot.ExposedBlocks[:1]
	one, err := ExecutePlanningTool(context.Background(), oneLease, ToolListAffordances, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("one list_affordances: %v", err)
	}
	var oneResult planningAffordancesResult
	if err := json.Unmarshal(one.Canonical, &oneResult); err != nil || len(oneResult.VisibleBlocks) != 1 {
		t.Fatalf("one visible_blocks=%v err=%v wire=%s", oneResult.VisibleBlocks, err, one.Canonical)
	}
}

func TestPlanningFindBlocksRejectsStandaloneBoundedNameGoldens(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	invalid := contractLoadGolden(t, "contracts/companion-agent/mcp-v1/golden/invalid.json")
	seen := 0
	for _, testCase := range invalid.Cases {
		if testCase.Schema != "bounded_name" {
			continue
		}
		seen++
		t.Run(testCase.Name, func(t *testing.T) {
			var nameJSON []byte
			if testCase.ValueUTF8Hex != "" {
				raw, err := hex.DecodeString(testCase.ValueUTF8Hex)
				if err != nil {
					t.Fatal(err)
				}
				nameJSON = append(append([]byte{'"'}, raw...), '"')
			} else {
				nameJSON = append([]byte(nil), testCase.Value...)
			}
			input := append([]byte(`{"block_names":[`), nameJSON...)
			input = append(input, []byte(`],"limit":1}`)...)
			result, err := ExecutePlanningTool(context.Background(), lease, ToolFindVisibleBlocks, input)
			if !errors.Is(err, ErrPlanningToolInvalidInput) || result.Structured != nil || len(result.Canonical) != 0 || result.DomainFailure {
				t.Fatalf("bounded_name fixture result=%+v err=%v", result, err)
			}
		})
	}
	if seen < 6 {
		t.Fatalf("bounded_name invalid goldens=%d，want >=6", seen)
	}

	validBoundary, err := ExecutePlanningTool(context.Background(), lease, ToolFindVisibleBlocks,
		json.RawMessage(`{"block_names":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"],"limit":1}`))
	if err != nil || !validBoundary.DomainFailure || planningResultCode(t, validBoundary.Canonical) != ValidatorUnknownBlock {
		t.Fatalf("64-byte unknown name result=%s domain=%v err=%v", validBoundary.Canonical, validBoundary.DomainFailure, err)
	}
}

func TestPlanningToolFindBlocksValidatesAllNamesBeforeLookup(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	invalid := contractLoadGolden(t, "contracts/companion-agent/mcp-v1/golden/invalid.json")
	wantReasons := map[string]struct{}{
		"name_utf8_bytes": {},
		"name_blank":      {},
		"name_control":    {},
		"name_nul":        {},
	}
	seen := 0
	for _, testCase := range invalid.Cases {
		if testCase.Schema != "bounded_name" {
			continue
		}
		if _, ok := wantReasons[testCase.Reason]; !ok {
			continue
		}
		seen++
		t.Run(testCase.Name, func(t *testing.T) {
			input := append([]byte(`{"block_names":["not_a_registered_block",`), testCase.Value...)
			input = append(input, []byte(`],"limit":1}`)...)
			result, err := ExecutePlanningTool(context.Background(), lease, ToolFindVisibleBlocks, input)
			if !errors.Is(err, ErrPlanningToolInvalidInput) || result.Structured != nil ||
				len(result.Canonical) != 0 || result.DomainFailure {
				t.Fatalf("ordered bounded_name result=%+v err=%v", result, err)
			}
		})
	}
	if seen != len(wantReasons) {
		t.Fatalf("ordered bounded_name invalid goldens=%d，want %d", seen, len(wantReasons))
	}
}

func TestPlanningValidatorStableCodesAndDenseMine(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	validMine := `{"plan":{"summary":"采一块石头","steps":[{"kind":"mine","x":8,"y":63,"z":-2}]}}`
	cases := []struct {
		name  string
		lease SnapshotLease
		input string
		code  string
	}{
		{"invalid_schema", lease, `{"plan":{"summary":"x","steps":[]}}`, ValidatorInvalidSchema},
		{"out_of_bounds", lease, `{"plan":{"summary":"x","steps":[{"kind":"mine","x":100,"y":64,"z":0}]}}`, ValidatorOutOfBounds},
		{"unknown_player", lease, `{"plan":{"summary":"x","steps":[{"kind":"follow","player_id":"3c4d5e6f-7081-4a92-8b3e-2a3b4c5d6e7f"}]}}`, ValidatorUnknownPlayer},
		{"unmineable_target", lease, `{"plan":{"summary":"x","steps":[{"kind":"mine","x":4,"y":64,"z":-1}]}}`, ValidatorUnmineableTarget},
		{"unknown_block", lease, `{"plan":{"summary":"x","steps":[{"kind":"place","x":4,"y":64,"z":-1,"block":"diamond_ore"}]}}`, ValidatorUnknownBlock},
		{"missing_item", lease, `{"plan":{"summary":"x","steps":[{"kind":"place","x":4,"y":64,"z":-1,"block":"oak_planks"}]}}`, ValidatorMissingItem},
		{"snapshot_mismatch", func() SnapshotLease { changed := lease; changed.Digest = strings.Repeat("0", 64); return changed }(), validMine, ValidatorSnapshotMismatch},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := ExecutePlanningTool(context.Background(), testCase.lease, ToolValidatePlan, json.RawMessage(testCase.input))
			if err != nil {
				t.Fatalf("ExecutePlanningTool: %v", err)
			}
			if got := planningResultCode(t, result.Canonical); got != testCase.code {
				t.Fatalf("code=%q，want %q: %s", got, testCase.code, result.Canonical)
			}
			var failure map[string]any
			if err := json.Unmarshal(result.Canonical, &failure); err != nil || len(failure) != 3 || failure["accepted"] != false {
				t.Fatalf("failure shape=%v err=%v", failure, err)
			}
		})
	}

	// 目标不在 256 条 exposed 摘要中，但 dense projection 仍必须精确接受。
	dense := lease
	dense.Snapshot.ExposedBlocks = nil
	_, dense.Digest, _ = CanonicalSnapshotDigest(dense.Snapshot)
	result, err := ExecutePlanningTool(context.Background(), dense, ToolValidatePlan, json.RawMessage(validMine))
	if err != nil || planningResultCode(t, result.Canonical) != "" || !strings.Contains(string(result.Canonical), `"accepted":true`) {
		t.Fatalf("dense mine result=%s err=%v", result.Canonical, err)
	}
}

func TestPlanningValidatorMineSemanticsGolden(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()
	// `contractGolden` 只描述通用 value case；mine fixture 使用独立形状，直接读取。
	data := contractReadObject(t, "contracts/companion-agent/mcp-v1/golden/mine-validation.json")
	cases := contractArray(t, data["cases"], "mine cases")
	for _, rawCase := range cases {
		testCase := contractObject(t, rawCase, "mine case")
		name := contractString(t, testCase["name"], "mine name")
		block := core.BlockID(contractInteger(t, testCase["block_id"], "block_id"))
		accepted := contractBool(t, testCase["accepted"], "accepted")
		t.Run(name, func(t *testing.T) {
			current := lease
			if !current.Snapshot.Terrain.SetBlock(core.BlockPos{X: 8, Y: 63, Z: -2}, block) {
				t.Fatal("SetBlock=false")
			}
			_, current.Digest, _ = CanonicalSnapshotDigest(current.Snapshot)
			result, err := ExecutePlanningTool(context.Background(), current, ToolValidatePlan,
				json.RawMessage(`{"plan":{"summary":"x","steps":[{"kind":"mine","x":8,"y":63,"z":-2}]}}`))
			if err != nil {
				t.Fatalf("validator: %v", err)
			}
			if accepted && planningResultCode(t, result.Canonical) != "" {
				t.Fatalf("应接受: %s", result.Canonical)
			}
			if !accepted && planningResultCode(t, result.Canonical) != ValidatorUnmineableTarget {
				t.Fatalf("应拒绝 unmineable_target: %s", result.Canonical)
			}
		})
	}
}

func TestPlanningToolsObserveRegistryCancellation(t *testing.T) {
	clock := newSnapshotFakeClock(time.Unix(1_800_000_300, 0))
	registry := newSnapshotRegistry(clock, snapshotTestEntropy())
	snapshot := planningToolSnapshot(t)
	registration, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 1, snapshot, clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := registry.Lookup(registration.Capability)
	if err != nil {
		t.Fatal(err)
	}
	registry.Cancel(registration.SnapshotID)
	if _, err := ExecutePlanningTool(context.Background(), lease, ToolGetPlanningContext, json.RawMessage(`{}`)); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("取消后 tool err=%v", err)
	}
}

func TestPlanningToolsRaceRegistryCancelAndExpiry(t *testing.T) {
	for _, mode := range []string{"cancel", "expiry"} {
		t.Run(mode, func(t *testing.T) {
			clock := newSnapshotFakeClock(time.Unix(1_800_000_325, 0))
			registry := newSnapshotRegistry(clock, snapshotTestEntropy())
			t.Cleanup(registry.Close)
			snapshot := planningToolSnapshot(t)
			registration, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 1, snapshot, clock.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			start := make(chan struct{})
			errorsSeen := make(chan error, 8)
			var workers sync.WaitGroup
			for range 8 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					for range 32 {
						lease, lookupErr := registry.Lookup(registration.Capability)
						if lookupErr != nil {
							if !errors.Is(lookupErr, ErrSnapshotUnavailable) {
								errorsSeen <- lookupErr
							}
							return
						}
						_, toolErr := ExecutePlanningTool(context.Background(), lease, ToolQueryTerrain,
							json.RawMessage(`{"positions":[{"x":8,"y":63,"z":-2},{"x":10,"y":63,"z":-2}]}`))
						if toolErr != nil && !errors.Is(toolErr, ErrSnapshotUnavailable) {
							errorsSeen <- toolErr
							return
						}
					}
				}()
			}
			close(start)
			if mode == "cancel" {
				registry.Cancel(registration.SnapshotID)
			} else {
				clock.Advance(snapshotExpiryGrace + time.Second)
			}
			workers.Wait()
			close(errorsSeen)
			for workerErr := range errorsSeen {
				t.Errorf("worker error: %v", workerErr)
			}
			if _, err := registry.Lookup(registration.Capability); !errors.Is(err, ErrSnapshotUnavailable) {
				t.Fatalf("生命周期结束后 Lookup=%v", err)
			}
		})
	}
}

func TestPlanningToolsCheckpointEveryBoundedLoop(t *testing.T) {
	lease, cleanup := planningToolLease(t)
	defer cleanup()

	contextChecks := &planningCheckpointContext{}
	if _, err := ExecutePlanningTool(contextChecks, lease, ToolGetPlanningContext, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if got := contextChecks.Count(); got < 4 {
		t.Fatalf("context chunk loop checkpoints=%d，want >=4", got)
	}

	findChecks := &planningCheckpointContext{}
	if _, err := ExecutePlanningTool(findChecks, lease, ToolFindVisibleBlocks,
		json.RawMessage(`{"block_names":["stone","furnace"],"limit":2}`)); err != nil {
		t.Fatal(err)
	}
	if got := findChecks.Count(); got < 8 {
		t.Fatalf("find bounded loop checkpoints=%d，want >=8", got)
	}

	validatorChecks := &planningCheckpointContext{}
	if _, err := ExecutePlanningTool(validatorChecks, lease, ToolValidatePlan,
		json.RawMessage(`{"plan":{"summary":"x","steps":[{"kind":"mine","x":8,"y":63,"z":-2}]}}`)); err != nil {
		t.Fatal(err)
	}
	if got := validatorChecks.Count(); got < 8 {
		t.Fatalf("validator bounded loop/encoding checkpoints=%d，want >=8", got)
	}

	missingItemChecks := &planningCheckpointContext{}
	if _, err := ExecutePlanningTool(missingItemChecks, lease, ToolValidatePlan,
		json.RawMessage(`{"plan":{"summary":"x","steps":[{"kind":"place","x":8,"y":63,"z":-2,"block":"oak_planks"}]}}`)); err != nil {
		t.Fatal(err)
	}
	if got := missingItemChecks.Count(); got < 42 {
		t.Fatalf("validator inventory loop checkpoints=%d，want >=42", got)
	}
}

func TestPlanningValidatePlanDiscardsResultWhenCanceledInsideDigestPlane(t *testing.T) {
	for _, mode := range []string{"context", "registry"} {
		t.Run(mode, func(t *testing.T) {
			clock := newSnapshotFakeClock(time.Unix(1_800_000_350, 0))
			registry := newSnapshotRegistry(clock, snapshotTestEntropy())
			t.Cleanup(registry.Close)
			snapshot := planningToolSnapshot(t)
			registration, err := registry.Register(
				testNamespaceUUID, snapshot.Companion.ID, 1, snapshot, clock.Now().Add(time.Minute),
			)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := registry.Lookup(registration.Capability)
			if err != nil {
				t.Fatal(err)
			}
			probe := &planningCancelProbeContext{trigger: 25_000, cancelContext: mode == "context"}
			if mode == "registry" {
				probe.onTrigger = func() { registry.Cancel(registration.SnapshotID) }
			}
			result, err := ExecutePlanningTool(probe, lease, ToolValidatePlan,
				json.RawMessage(`{"plan":{"summary":"x","steps":[{"kind":"mine","x":8,"y":63,"z":-2}]}}`))
			if !probe.triggered.Load() {
				t.Fatalf("digest plane 未观察 %s 取消，checkpoints=%d", mode, probe.Count())
			}
			if !errors.Is(err, ErrSnapshotUnavailable) || result.Structured != nil || len(result.Canonical) != 0 || result.DomainFailure {
				t.Fatalf("canceled validate_plan result=%+v err=%v", result, err)
			}
		})
	}
}

type planningCheckpointContext struct {
	context.Context
	checks atomic.Int32
}

func (c *planningCheckpointContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *planningCheckpointContext) Done() <-chan struct{}       { return nil }
func (c *planningCheckpointContext) Err() error {
	c.checks.Add(1)
	return nil
}
func (c *planningCheckpointContext) Value(key any) any { return nil }
func (c *planningCheckpointContext) Count() int32      { return c.checks.Load() }

type planningCancelProbeContext struct {
	context.Context
	checks        atomic.Int32
	trigger       int32
	triggered     atomic.Bool
	cancelContext bool
	onTrigger     func()
}

func (c *planningCancelProbeContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *planningCancelProbeContext) Done() <-chan struct{}       { return nil }
func (c *planningCancelProbeContext) Err() error {
	count := c.checks.Add(1)
	if count == c.trigger {
		c.triggered.Store(true)
		if c.onTrigger != nil {
			c.onTrigger()
		}
	}
	if c.cancelContext && count >= c.trigger {
		return context.Canceled
	}
	return nil
}
func (c *planningCancelProbeContext) Value(any) any { return nil }
func (c *planningCancelProbeContext) Count() int32  { return c.checks.Load() }

func planningToolLease(t *testing.T) (SnapshotLease, func()) {
	t.Helper()
	clock := newSnapshotFakeClock(time.Unix(1_800_000_250, 0))
	registry := newSnapshotRegistry(clock, snapshotTestEntropy())
	snapshot := planningToolSnapshot(t)
	registration, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 1, snapshot, clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	lease, err := registry.Lookup(registration.Capability)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	return lease, registry.Close
}

func planningToolSnapshot(t *testing.T) PlanSnapshot {
	t.Helper()
	issuer, err := core.ParsePlayerID("99999999-9999-4999-8999-999999999999")
	if err != nil {
		t.Fatal(err)
	}
	companionID, err := ParseID("66666666-6666-4666-8666-666666666666")
	if err != nil {
		t.Fatal(err)
	}
	terrain := NewTerrainProjection(core.BlockPos{X: -11, Y: 56, Z: -18})
	for x := int32(-11); x <= 21; x++ {
		for z := int32(-18); z <= 14; z++ {
			if !terrain.SetReadyColumn(x, z, 63) {
				t.Fatal("SetReadyColumn=false")
			}
		}
	}
	for pos, block := range map[core.BlockPos]core.BlockID{
		{X: 8, Y: 63, Z: -2}:  core.StoneID,
		{X: 9, Y: 63, Z: -2}:  core.ChestID,
		{X: 10, Y: 63, Z: -2}: core.FurnaceID,
	} {
		if !terrain.SetBlock(pos, block) {
			t.Fatalf("SetBlock(%+v)=false", pos)
		}
	}
	var inventory core.Inventory
	inventory.Hotbar.Selected = 0
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemChest, Count: 1}
	snapshot := PlanSnapshot{
		Command: "采一块石头后跟上玩家",
		Issuer: PlanPlayer{
			ID: issuer, Position: [3]float32{4.5, 64, -1.5}, Yaw: 90,
			LookHit: core.BlockPos{X: 8, Y: 63, Z: -2}, HasLookHit: true,
		},
		Companion: PlanCompanion{
			ID: companionID, Position: [3]float32{5.5, 64, -1.5},
			Inventory: inventory, TaskStatus: "规划中",
		},
		ExposedBlocks: []PlanBlock{
			{Pos: core.BlockPos{X: 8, Y: 63, Z: -2}, Block: core.StoneID},
			{Pos: core.BlockPos{X: 9, Y: 63, Z: -2}, Block: core.ChestID},
			{Pos: core.BlockPos{X: 10, Y: 63, Z: -2}, Block: core.FurnaceID},
		},
		Heights:        terrain.Heights(),
		Terrain:        terrain,
		ChunkRevisions: []pathfind.ChunkRevision{{Chunk: core.ChunkPos{X: 0, Z: -1}, Revision: 17}},
		OnlinePlayers: []PlanPlayer{{
			ID: issuer, Position: [3]float32{4.5, 64, -1.5}, Yaw: 90,
		}},
		WorldTimeTicks: 1200,
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("planning tool snapshot: %v", err)
	}
	return snapshot
}

func planningToolLargestAffordanceSnapshot(t *testing.T) PlanSnapshot {
	t.Helper()
	snapshot := planningToolSnapshot(t)
	snapshot.ExposedBlocks = make([]PlanBlock, 0, MaxPlanExposedBlocks)
	for index := range MaxPlanExposedBlocks {
		pos := core.BlockPos{
			X: -11 + int32(index/33),
			Y: 63,
			Z: -18 + int32(index%33),
		}
		if !snapshot.Terrain.SetBlock(pos, core.StoneID) {
			t.Fatalf("SetBlock(%+v)=false", pos)
		}
		snapshot.ExposedBlocks = append(snapshot.ExposedBlocks, PlanBlock{Pos: pos, Block: core.StoneID})
	}
	snapshot.OnlinePlayers = make([]PlanPlayer, 0, MaxPlanOnlinePlayers)
	for index := range MaxPlanOnlinePlayers {
		id, err := core.ParsePlayerID(fmt.Sprintf("10000000-0000-4000-8000-%012d", index+1))
		if err != nil {
			t.Fatal(err)
		}
		snapshot.OnlinePlayers = append(snapshot.OnlinePlayers, PlanPlayer{
			ID:       id,
			Position: [3]float32{math.MaxFloat32, -math.MaxFloat32, float32(index)},
		})
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("largest affordance snapshot: %v", err)
	}
	return snapshot
}

func planningResultCode(t *testing.T, canonical []byte) string {
	t.Helper()
	var result struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(canonical, &result); err != nil {
		t.Fatalf("result JSON: %v: %s", err, canonical)
	}
	return result.Code
}

func contractInteger(t *testing.T, value any, path string) int64 {
	t.Helper()
	number, ok := value.(json.Number)
	if ok {
		parsed, err := number.Int64()
		if err == nil {
			return parsed
		}
	}
	if floating, ok := value.(float64); ok {
		return int64(floating)
	}
	t.Fatalf("%s 不是整数: %T", path, value)
	return 0
}
