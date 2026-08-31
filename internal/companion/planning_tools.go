package companion

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/core"
)

const (
	ToolGetPlanningContext = "get_planning_context"
	ToolListAffordances    = "list_affordances"
	ToolInspectInventory   = "inspect_inventory"
	ToolFindVisibleBlocks  = "find_visible_blocks"
	ToolQueryTerrain       = "query_terrain"
	ToolValidatePlan       = "validate_plan"

	ValidatorInvalidSchema    = "invalid_schema"
	ValidatorOutOfBounds      = "out_of_bounds"
	ValidatorUnknownPlayer    = "unknown_player"
	ValidatorUnmineableTarget = "unmineable_target"
	ValidatorUnknownBlock     = "unknown_block"
	ValidatorMissingItem      = "missing_item"
	ValidatorSnapshotMismatch = "snapshot_mismatch"

	maxValidatorHintBytes = 256
)

var (
	// ErrPlanningToolInvalidInput 表示普通观察工具参数不符合 checked-in schema。
	ErrPlanningToolInvalidInput = errors.New("companion: planning tool 参数非法")
	// ErrPlanningToolResultTooLarge 表示 canonical tool result 超过 manifest 上限。
	ErrPlanningToolResultTooLarge = errors.New("companion: planning tool 结果超限")
)

var planningToolOrder = [...]string{
	ToolGetPlanningContext,
	ToolListAffordances,
	ToolInspectInventory,
	ToolFindVisibleBlocks,
	ToolQueryTerrain,
	ToolValidatePlan,
}

var planningToolLimits = map[string]int{
	ToolGetPlanningContext: 24 << 10,
	ToolListAffordances:    24 << 10,
	ToolInspectInventory:   8 << 10,
	ToolFindVisibleBlocks:  16 << 10,
	ToolQueryTerrain:       16 << 10,
	ToolValidatePlan:       72 << 10,
}

// PlanningToolResult 同时保存 SDK StructuredContent 的值与恰好相同对象的
// canonical JSON TextContent。DomainFailure 只用于 find/query 的正常失败分支。
type PlanningToolResult struct {
	Structured    any
	Canonical     []byte
	DomainFailure bool
}

// PlanningToolNames 返回 manifest 固定广告顺序的独立副本。
func PlanningToolNames() []string {
	return append([]string(nil), planningToolOrder[:]...)
}

// PlanningToolCanonicalLimit 返回工具的 canonical result 上限；未知工具为零。
func PlanningToolCanonicalLimit(name string) int { return planningToolLimits[name] }

// ExecutePlanningTool 只读取 SnapshotLease 的冻结副本并执行一个有界纯工具。
// 所有成功和正常 domain failure 都同时生成 structured value 与 canonical JSON。
func ExecutePlanningTool(ctx context.Context, lease SnapshotLease, name string, input json.RawMessage) (PlanningToolResult, error) {
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return PlanningToolResult{}, err
	}
	var value any
	domainFailure := false
	var err error
	switch name {
	case ToolGetPlanningContext:
		value, err = planningContextTool(ctx, lease, input)
	case ToolListAffordances:
		value, err = planningAffordancesTool(ctx, lease, input)
	case ToolInspectInventory:
		value, err = planningInventoryTool(ctx, lease, input)
	case ToolFindVisibleBlocks:
		value, domainFailure, err = planningFindBlocksTool(ctx, lease, input)
	case ToolQueryTerrain:
		value, domainFailure, err = planningTerrainTool(ctx, lease, input)
	case ToolValidatePlan:
		value, err = planningValidatePlanTool(ctx, lease, input)
	default:
		err = ErrPlanningToolInvalidInput
	}
	if err != nil {
		return PlanningToolResult{}, err
	}
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return PlanningToolResult{}, err
	}
	canonical, err := canonicalJSON(value)
	if err != nil {
		return PlanningToolResult{}, fmt.Errorf("companion: planning tool 编码失败")
	}
	limit := PlanningToolCanonicalLimit(name)
	if limit == 0 || len(canonical) > limit {
		return PlanningToolResult{}, ErrPlanningToolResultTooLarge
	}
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return PlanningToolResult{}, err
	}
	return PlanningToolResult{Structured: value, Canonical: canonical, DomainFailure: domainFailure}, nil
}

type planningContextResult struct {
	ChunkRevisions []planningChunkRevision  `json:"chunk_revisions"`
	Companion      planningContextCompanion `json:"companion"`
	Instruction    string                   `json:"instruction"`
	Issuer         planningContextIssuer    `json:"issuer"`
	SnapshotDigest string                   `json:"snapshot_digest"`
	WorldTimeTicks uint64                   `json:"world_time_ticks"`
}

type planningChunkRevision struct {
	Revision uint64 `json:"revision"`
	X        int32  `json:"x"`
	Z        int32  `json:"z"`
}

type planningContextIssuer struct {
	LookHit  *digestBlockPosition `json:"look_hit"`
	Pitch    float32              `json:"pitch"`
	PlayerID string               `json:"player_id"`
	Position [3]float32           `json:"position"`
	Yaw      float32              `json:"yaw"`
}

type planningContextCompanion struct {
	CompanionID string     `json:"companion_id"`
	Pitch       float32    `json:"pitch"`
	Position    [3]float32 `json:"position"`
	TaskStatus  string     `json:"task_status"`
	Yaw         float32    `json:"yaw"`
}

func planningContextTool(ctx context.Context, lease SnapshotLease, input json.RawMessage) (any, error) {
	if err := decodePlanningToolInput(input, &struct{}{}); err != nil {
		return nil, err
	}
	snapshot := lease.Snapshot
	issuer := planningContextIssuer{
		Pitch: snapshot.Issuer.Pitch, PlayerID: snapshot.Issuer.ID.String(),
		Position: snapshot.Issuer.Position, Yaw: snapshot.Issuer.Yaw,
	}
	if snapshot.Issuer.HasLookHit {
		look := digestPosition(snapshot.Issuer.LookHit)
		issuer.LookHit = &look
	}
	result := planningContextResult{
		ChunkRevisions: make([]planningChunkRevision, len(snapshot.ChunkRevisions)),
		Companion: planningContextCompanion{
			CompanionID: snapshot.Companion.ID.String(), Pitch: snapshot.Companion.Pitch,
			Position: snapshot.Companion.Position, TaskStatus: snapshot.Companion.TaskStatus,
			Yaw: snapshot.Companion.Yaw,
		},
		Instruction: snapshot.Command, Issuer: issuer, SnapshotDigest: lease.Digest,
		WorldTimeTicks: snapshot.WorldTimeTicks,
	}
	for index, revision := range snapshot.ChunkRevisions {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, err
		}
		result.ChunkRevisions[index] = planningChunkRevision{
			Revision: revision.Revision, X: revision.Chunk.X, Z: revision.Chunk.Z,
		}
	}
	return result, nil
}

type planningAffordancesResult struct {
	OnlinePlayers []planningOnlinePlayer `json:"online_players"`
	StepKinds     [4]string              `json:"step_kinds"`
	VisibleBlocks []planningVisibleBlock `json:"visible_blocks"`
}

type planningOnlinePlayer struct {
	PlayerID string     `json:"player_id"`
	Position [3]float32 `json:"position"`
}

type planningVisibleBlock struct {
	BlockID       core.BlockID        `json:"block_id"`
	BlockName     string              `json:"block_name"`
	DropItem      *string             `json:"drop_item"`
	MineSemantics string              `json:"mine_semantics"`
	Position      digestBlockPosition `json:"position"`
}

func planningAffordancesTool(ctx context.Context, lease SnapshotLease, input json.RawMessage) (any, error) {
	if err := decodePlanningToolInput(input, &struct{}{}); err != nil {
		return nil, err
	}
	snapshot := lease.Snapshot
	result := planningAffordancesResult{
		OnlinePlayers: make([]planningOnlinePlayer, 0, len(snapshot.OnlinePlayers)),
		StepKinds:     [4]string{"go_to", "follow", "mine", "place"},
		VisibleBlocks: make([]planningVisibleBlock, 0, len(snapshot.ExposedBlocks)),
	}
	for _, player := range snapshot.OnlinePlayers {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, err
		}
		result.OnlinePlayers = append(result.OnlinePlayers, planningOnlinePlayer{
			PlayerID: player.ID.String(), Position: player.Position,
		})
	}
	for _, block := range snapshot.ExposedBlocks {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, err
		}
		blockName, ok := core.CanonicalBlockName(block.Block)
		if !ok {
			return nil, ErrSnapshotUnavailable
		}
		semantics, drop, err := planningMineDescription(block.Block)
		if err != nil {
			return nil, err
		}
		result.VisibleBlocks = append(result.VisibleBlocks, planningVisibleBlock{
			BlockID: block.Block, BlockName: blockName, DropItem: drop,
			MineSemantics: semantics, Position: digestPosition(block.Pos),
		})
	}
	return result, nil
}

type planningInventoryInput struct {
	Limit  *int `json:"limit"`
	Offset *int `json:"offset"`
}

type planningInventoryResult struct {
	Slots []planningInventorySlot `json:"slots"`
}

type planningInventorySlot struct {
	Count uint8  `json:"count"`
	Item  string `json:"item"`
	Slot  int    `json:"slot"`
}

func planningInventoryTool(ctx context.Context, lease SnapshotLease, input json.RawMessage) (any, error) {
	var decoded planningInventoryInput
	if err := decodePlanningToolInput(input, &decoded); err != nil || decoded.Offset == nil || decoded.Limit == nil ||
		*decoded.Offset < 0 || *decoded.Offset >= core.InventorySlots || *decoded.Limit < 1 || *decoded.Limit > core.InventorySlots {
		return nil, ErrPlanningToolInvalidInput
	}
	end := min(*decoded.Offset+*decoded.Limit, core.InventorySlots)
	result := planningInventoryResult{Slots: make([]planningInventorySlot, 0, end-*decoded.Offset)}
	for slot := *decoded.Offset; slot < end; slot++ {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, err
		}
		stack, _ := lease.Snapshot.Companion.Inventory.Slot(uint8(slot))
		if stack.Item == core.ItemNone {
			continue
		}
		name, ok := core.CanonicalItemName(stack.Item)
		if !ok {
			return nil, ErrSnapshotUnavailable
		}
		result.Slots = append(result.Slots, planningInventorySlot{Count: stack.Count, Item: name, Slot: slot})
	}
	return result, nil
}

type planningFindBlocksInput struct {
	BlockNames []string `json:"block_names"`
	Limit      *int     `json:"limit"`
}

type planningFindBlocksResult struct {
	Matches []planningBlockMatch `json:"matches"`
}

type planningBlockMatch struct {
	BlockName string              `json:"block_name"`
	DropItem  *string             `json:"drop_item"`
	Position  digestBlockPosition `json:"position"`
}

type planningDomainFailure struct {
	Code string `json:"code"`
	Hint string `json:"hint"`
}

func planningFindBlocksTool(ctx context.Context, lease SnapshotLease, input json.RawMessage) (any, bool, error) {
	var decoded planningFindBlocksInput
	if err := decodePlanningToolInput(input, &decoded); err != nil || decoded.Limit == nil ||
		len(decoded.BlockNames) < 1 || len(decoded.BlockNames) > 16 || *decoded.Limit < 1 || *decoded.Limit > 64 {
		return nil, false, ErrPlanningToolInvalidInput
	}
	wanted := make(map[core.BlockID]struct{}, len(decoded.BlockNames))
	seenNames := make(map[string]struct{}, len(decoded.BlockNames))
	for _, name := range decoded.BlockNames {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, false, err
		}
		if _, duplicate := seenNames[name]; duplicate || len(name) == 0 || len(name) > 64 {
			return nil, false, ErrPlanningToolInvalidInput
		}
		seenNames[name] = struct{}{}
		block, ok := core.BlockIDByCanonicalName(name)
		if !ok {
			return planningDomainFailure{Code: ValidatorUnknownBlock, Hint: "unknown canonical block name"}, true, nil
		}
		wanted[block] = struct{}{}
	}
	result := planningFindBlocksResult{Matches: make([]planningBlockMatch, 0, *decoded.Limit)}
	for _, block := range lease.Snapshot.ExposedBlocks {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, false, err
		}
		if _, ok := wanted[block.Block]; !ok {
			continue
		}
		blockName, ok := core.CanonicalBlockName(block.Block)
		if !ok {
			return nil, false, ErrSnapshotUnavailable
		}
		_, drop, err := planningMineDescription(block.Block)
		if err != nil {
			return nil, false, err
		}
		result.Matches = append(result.Matches, planningBlockMatch{
			BlockName: blockName, DropItem: drop, Position: digestPosition(block.Pos),
		})
		if len(result.Matches) == *decoded.Limit {
			break
		}
	}
	return result, false, nil
}

type planningTerrainInput struct {
	Positions []digestBlockPosition `json:"positions"`
}

type planningTerrainResult struct {
	Terrain []planningTerrainPoint `json:"terrain"`
}

type planningTerrainPoint struct {
	BlockName string              `json:"block_name"`
	Height    int32               `json:"height"`
	Position  digestBlockPosition `json:"position"`
}

func planningTerrainTool(ctx context.Context, lease SnapshotLease, input json.RawMessage) (any, bool, error) {
	var decoded planningTerrainInput
	if err := decodePlanningToolInput(input, &decoded); err != nil || len(decoded.Positions) < 1 || len(decoded.Positions) > 64 {
		return nil, false, ErrPlanningToolInvalidInput
	}
	result := planningTerrainResult{Terrain: make([]planningTerrainPoint, 0, len(decoded.Positions))}
	for _, position := range decoded.Positions {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, false, err
		}
		pos := core.BlockPos{X: position.X, Y: position.Y, Z: position.Z}
		if !validPlanBlockY(pos.Y) {
			return nil, false, ErrPlanningToolInvalidInput
		}
		block, height, ok := lease.Snapshot.Terrain.Lookup(pos)
		if !ok {
			return planningDomainFailure{Code: ValidatorOutOfBounds, Hint: "position is outside the frozen projection"}, true, nil
		}
		name, ok := core.CanonicalBlockName(block)
		if !ok {
			return nil, false, ErrSnapshotUnavailable
		}
		result.Terrain = append(result.Terrain, planningTerrainPoint{
			BlockName: name, Height: height, Position: position,
		})
	}
	return result, false, nil
}

type planningValidateInput struct {
	Plan json.RawMessage `json:"plan"`
}

type planningValidateSuccess struct {
	Accepted       bool         `json:"accepted"`
	Plan           planningPlan `json:"plan"`
	SnapshotDigest string       `json:"snapshot_digest"`
}

type planningValidateFailure struct {
	Accepted bool   `json:"accepted"`
	Code     string `json:"code"`
	Hint     string `json:"hint"`
}

type planningPlan struct {
	Steps   []planningPlanStep `json:"steps"`
	Summary string             `json:"summary"`
}

type planningPlanStep struct {
	Block    string `json:"block,omitempty"`
	Kind     string `json:"kind"`
	PlayerID string `json:"player_id,omitempty"`
	X        *int32 `json:"x,omitempty"`
	Y        *int32 `json:"y,omitempty"`
	Z        *int32 `json:"z,omitempty"`
}

func planningValidatePlanTool(ctx context.Context, lease SnapshotLease, input json.RawMessage) (any, error) {
	plan, wire, code, err := decodePlanningCandidate(ctx, lease, input)
	if err != nil {
		return nil, err
	}
	if code != "" {
		return planningValidatorFailure(code), nil
	}
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return nil, err
	}
	_, digest, err := CanonicalSnapshotDigest(lease.Snapshot)
	if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(lease.Digest)) != 1 {
		return planningValidatorFailure(ValidatorSnapshotMismatch), nil
	}
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return nil, err
	}
	online := make(map[core.PlayerID]struct{}, len(lease.Snapshot.OnlinePlayers))
	for _, player := range lease.Snapshot.OnlinePlayers {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, err
		}
		online[player.ID] = struct{}{}
	}
	for _, step := range plan.Steps {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return nil, err
		}
		switch step.Kind {
		case PlanStepFollow:
			if _, ok := online[step.PlayerID]; !ok {
				return planningValidatorFailure(ValidatorUnknownPlayer), nil
			}
		case PlanStepMine:
			block, _, ok := lease.Snapshot.Terrain.Lookup(core.BlockPos{X: step.X, Y: step.Y, Z: step.Z})
			if !ok {
				return planningValidatorFailure(ValidatorOutOfBounds), nil
			}
			if !planMineableBlock(block) {
				return planningValidatorFailure(ValidatorUnmineableTarget), nil
			}
		case PlanStepPlace:
			item, ok := planPlaceBlocks[step.Block]
			if !ok {
				return planningValidatorFailure(ValidatorUnknownBlock), nil
			}
			holds, err := planInventoryHoldsWithCheckpoint(
				lease.Snapshot.Companion.Inventory,
				item,
				func() error { return planningToolCheckpoint(ctx, lease) },
			)
			if err != nil {
				return nil, err
			}
			if !holds {
				return planningValidatorFailure(ValidatorMissingItem), nil
			}
		}
	}
	canonicalPlan, err := planningPlanFromWire(ctx, lease, plan, wire)
	if err != nil {
		return nil, err
	}
	return planningValidateSuccess{
		Accepted: true, Plan: canonicalPlan, SnapshotDigest: lease.Digest,
	}, nil
}

func decodePlanningCandidate(ctx context.Context, lease SnapshotLease, input json.RawMessage) (Plan, planWire, string, error) {
	if len(input) > MaxPlanResponseBytes {
		return Plan{}, planWire{}, ValidatorInvalidSchema, nil
	}
	var decoded planningValidateInput
	if err := decodePlanningToolObject(input, &decoded); err != nil || len(decoded.Plan) == 0 || isJSONNull(decoded.Plan) {
		return Plan{}, planWire{}, ValidatorInvalidSchema, nil
	}
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return Plan{}, planWire{}, "", err
	}
	canonical, err := canonicalJSON(json.RawMessage(input))
	if err != nil || len(canonical) > MaxPlanResponseBytes {
		return Plan{}, planWire{}, ValidatorInvalidSchema, nil
	}
	if err := planningToolCheckpoint(ctx, lease); err != nil {
		return Plan{}, planWire{}, "", err
	}
	var wire planWire
	if err := decodePlanningToolObject(decoded.Plan, &wire); err != nil {
		return Plan{}, planWire{}, ValidatorInvalidSchema, nil
	}
	if err := validatePlanText("计划 summary", wire.Summary, MaxPlanSummaryBytes, true); err != nil || len(wire.Steps) == 0 || len(wire.Steps) > 5000 {
		return Plan{}, planWire{}, ValidatorInvalidSchema, nil
	}
	plan := Plan{Summary: wire.Summary, Steps: make([]PlanStep, 0, len(wire.Steps))}
	for index, wireStep := range wire.Steps {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return Plan{}, planWire{}, "", err
		}
		if wireStep.Kind == "place" && wireStep.X != nil && wireStep.Y != nil && wireStep.Z != nil &&
			wireStep.Block != nil && !wireStep.has("player_id") {
			if _, ok := planPlaceItems[*wireStep.Block]; !ok {
				return Plan{}, planWire{}, ValidatorUnknownBlock, nil
			}
		}
		step, err := decodePlanStep(index, wireStep)
		if err != nil {
			return Plan{}, planWire{}, ValidatorInvalidSchema, nil
		}
		if (step.Kind == PlanStepGoTo || step.Kind == PlanStepMine || step.Kind == PlanStepPlace) && !validPlanBlockY(step.Y) {
			return Plan{}, planWire{}, ValidatorOutOfBounds, nil
		}
		plan.Steps = append(plan.Steps, step)
	}
	if err := validPlanStepsWithCheckpoint(plan.Steps, func() error {
		return planningToolCheckpoint(ctx, lease)
	}); err != nil {
		if errors.Is(err, ErrSnapshotUnavailable) {
			return Plan{}, planWire{}, "", err
		}
		return Plan{}, planWire{}, ValidatorInvalidSchema, nil
	}
	return plan, wire, "", nil
}

func planningPlanFromWire(ctx context.Context, lease SnapshotLease, plan Plan, wire planWire) (planningPlan, error) {
	result := planningPlan{Summary: plan.Summary, Steps: make([]planningPlanStep, 0, len(plan.Steps))}
	for index, step := range plan.Steps {
		if err := planningToolCheckpoint(ctx, lease); err != nil {
			return planningPlan{}, err
		}
		wireStep := wire.Steps[index]
		dto := planningPlanStep{Kind: wireStep.Kind}
		switch step.Kind {
		case PlanStepGoTo, PlanStepMine:
			x, y, z := step.X, step.Y, step.Z
			dto.X, dto.Y, dto.Z = &x, &y, &z
		case PlanStepPlace:
			x, y, z := step.X, step.Y, step.Z
			dto.X, dto.Y, dto.Z = &x, &y, &z
			dto.Block, _ = core.CanonicalBlockName(step.Block)
		case PlanStepFollow:
			dto.PlayerID = step.PlayerID.String()
		}
		result.Steps = append(result.Steps, dto)
	}
	return result, nil
}

func planningValidatorFailure(code string) planningValidateFailure {
	hints := map[string]string{
		ValidatorInvalidSchema:    "candidate plan does not match the strict schema",
		ValidatorOutOfBounds:      "position is outside the frozen projection",
		ValidatorUnknownPlayer:    "follow target is not online in this snapshot",
		ValidatorUnmineableTarget: "target is not mineable in this snapshot",
		ValidatorUnknownBlock:     "block name is not in the place registry",
		ValidatorMissingItem:      "required place item is missing from inventory",
		ValidatorSnapshotMismatch: "snapshot digest does not match the frozen view",
	}
	hint := hints[code]
	if len(hint) == 0 || len(hint) > maxValidatorHintBytes {
		hint = "candidate plan was rejected"
	}
	return planningValidateFailure{Accepted: false, Code: code, Hint: hint}
}

func planningMineDescription(block core.BlockID) (string, *string, error) {
	var semantics string
	switch {
	case block == core.ChestID || block == core.FurnaceID:
		semantics = "container_batch"
	case block == core.WheatStage7ID || block == core.PotatoStage7ID || block == core.CarrotStage7ID:
		semantics = "undelivered_multi_drop"
	case core.IsCrop(block) || core.IsFarmland(block):
		semantics = "forbidden_farming"
	case core.IsTorch(block):
		semantics = "forbidden_torch"
	default:
		if _, ok := core.BlockDrop(block); ok {
			semantics = "single_drop"
		} else {
			semantics = "no_drop"
		}
	}
	dropID, ok := core.BlockDrop(block)
	if !ok {
		return semantics, nil, nil
	}
	dropName, ok := core.CanonicalItemName(dropID)
	if !ok {
		return "", nil, ErrSnapshotUnavailable
	}
	return semantics, &dropName, nil
}

func planningToolCheckpoint(ctx context.Context, lease SnapshotLease) error {
	if err := lease.Checkpoint(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: request context", ErrSnapshotUnavailable)
	}
	return nil
}

func decodePlanningToolInput(data []byte, target any) error {
	if err := decodePlanningToolObject(data, target); err != nil {
		return ErrPlanningToolInvalidInput
	}
	return nil
}

func decodePlanningToolObject(data []byte, target any) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' || !utf8.Valid(trimmed) {
		return ErrPlanningToolInvalidInput
	}
	if err := validatePlanningJSONShape(trimmed); err != nil {
		return ErrPlanningToolInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrPlanningToolInvalidInput
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return ErrPlanningToolInvalidInput
	}
	return nil
}

func validatePlanningJSONShape(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumePlanningJSONValue(decoder); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON 尾随 token %v", token)
		}
		return err
	}
	return nil
}

func consumePlanningJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return fmt.Errorf("object key 不是字符串")
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("object key %q 重复", name)
			}
			seen[name] = struct{}{}
			if err := consumePlanningJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("object 未闭合")
		}
	case '[':
		for decoder.More() {
			if err := consumePlanningJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("array 未闭合")
		}
	default:
		return fmt.Errorf("非法 JSON delimiter")
	}
	return nil
}
