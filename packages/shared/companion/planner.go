// 本文件实现 Agent 候选计划到权威领域计划的严格转换与纯校验。
//
// 安全边界（spec：companion-planner）：玩家指令文本、方块名与模型输出全部视为
// 不可信数据，权限边界只有本地 JSON schema 白名单；不执行模型返回的代码、
// URL、工具名或任意函数调用。错误上下文绝不包含密钥或响应正文原文。
package companion

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/channing771/mornlea/packages/shared/core"
)

// MaxPlanResponseBytes 是候选计划 canonical JSON 的硬上限。
const MaxPlanResponseBytes = 64 << 10

var (
	// ErrPlannerUnavailable 表示 Agent 规划能力暂时不可用。
	ErrPlannerUnavailable = errors.New("companion: planner 不可用")
	// ErrPlannerInvalidPlan 表示模型输出不符合受限计划 schema：非法 JSON、
	// 未知字段、尾随数据、空计划、未交付步骤类型、非法数值或不满足 kind
	// 契约约束（follow 非最后一步或目标离线、mine 越界或目标不可采掘、
	// place 方块不在注册表或未持有）。上层把它映射为 InvalidPlan 类任务失败
	// 原因，且不重试、不降级、不改写。
	ErrPlannerInvalidPlan = errors.New("companion: planner 返回非法计划")
)

// planPlaceItemNames 返回 place 注册表全部方块名的字典序列表。
func planPlaceItemNames() []string {
	names := make([]string, 0, len(planPlaceItems))
	for name := range planPlaceItems {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PlanningPlaceBlockNames 返回 place 交付集合的 canonical 名称独立副本，供
// MCP checked-in schema consistency 门禁使用。
func PlanningPlaceBlockNames() []string { return planPlaceItemNames() }

// planWireStep 是计划步骤的解码中间形。
//
// 解码不走普通 struct 反射路径，而是自定义 UnmarshalJSON 先解到
// map[string]json.RawMessage 中间形（与 dialogue_types.go 的 map+isJSONNull
// 方案同构）。为什么必须如此：指针字段只能表达「值合法解出」与「未解出」，
// 而 encoding/json 把「字段缺席」与「字段出现且值为显式 JSON null」同样解成
// nil 指针——排他矩阵会把 null 与缺席折叠，让 follow+x:null、go_to+block:null
// 这类「专属外字段出现且为 null」的非法步骤被静默接受。M5E 契约收紧后显式
// null 一律视为「字段出现」，与 M5D summary:null 裁决同心智：出现事实由
// appeared 记录，与指针值（null 时为 nil）正交。
type planWireStep struct {
	Kind     string
	X        *int32
	Y        *int32
	Z        *int32
	Block    *string
	PlayerID *string
	// appeared 记录步骤 JSON 对象中真实出现的键（值为显式 null 也算出现）。
	// appeared 命中且指针非 nil 表示「出现且值合法」；命中而指针为 nil 表示
	// 「出现为显式 null」；未命中表示「字段缺席」。排他矩阵以 appeared 判定
	// 字段出现，不再从指针推断。
	appeared map[string]struct{}
}

// has 报告字段是否在步骤 JSON 中出现。显式 null 同样算出现——null 与缺席
// 不等价是 M5E null 契约收紧的核心语义，排他矩阵据此把「专属外字段为 null」
// 与「专属外字段携带非法值」归入同一拒绝路径。
func (s planWireStep) has(field string) bool {
	_, ok := s.appeared[field]
	return ok
}

// UnmarshalJSON 把单个步骤对象严格解码为中间形：先解到键→原始值的 map，
// 再逐键处理。键白名单（kind/x/y/z/block/player_id）之外的键一律拒绝——
// 自定义解码接管步骤内部后，外层 json.Decoder 的 DisallowUnknownFields 不再
// 覆盖步骤对象，未知键拒绝必须在此手工等价保留，既有严格性不降。显式 null
// 只记入 appeared、不填指针（值非法由 decodePlanStep 拒绝）；非 null 值按
// 强类型严格解码，类型不符（如 "x":"1"）在此失败，语义与原先的 struct
// 反射解码一致。
func (s *planWireStep) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	appeared := make(map[string]struct{}, len(fields))
	for name, raw := range fields {
		appeared[name] = struct{}{}
		var err error
		switch name {
		case "kind":
			// null 是 no-op（Kind 保持 ""），由 decodePlanStep 按 kind 未交付拒绝。
			err = json.Unmarshal(raw, &s.Kind)
		case "x":
			s.X, err = decodeWireValue[int32](raw)
		case "y":
			s.Y, err = decodeWireValue[int32](raw)
		case "z":
			s.Z, err = decodeWireValue[int32](raw)
		case "block":
			s.Block, err = decodeWireValue[string](raw)
		case "player_id":
			s.PlayerID, err = decodeWireValue[string](raw)
		default:
			return fmt.Errorf("步骤含未知字段 %q", name)
		}
		if err != nil {
			return fmt.Errorf("字段 %s 解码失败: %w", name, err)
		}
	}
	s.appeared = appeared
	return nil
}

// decodeWireValue 把步骤字段的原始 JSON 值解成强类型指针：显式 null 返回
// nil 指针（字段出现、值非法，由排他矩阵或必填校验拒绝），非 null 值按 T
// 严格解码，类型不符在此失败。
func decodeWireValue[T any](raw json.RawMessage) (*T, error) {
	if isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

// planWire 是计划文本的解码中间形；缺字段由解码后的显式校验兜底。
type planWire struct {
	Summary string         `json:"summary"`
	Steps   []planWireStep `json:"steps"`
}

// DecodeAgentPlan 把 Agent HTTP contract 的候选 DTO 转换为权威领域计划，并
// 复用既有步骤排他矩阵、领域校验和冻结快照校验。Agent 返回值始终是不可信
// 候选；调用方不得绕过此函数直接提交任务。
func DecodeAgentPlan(candidate AgentPlan, snapshot PlanSnapshot) (Plan, error) {
	if !validAgentPlan(candidate) {
		return Plan{}, fmt.Errorf("companion: planner typed DTO 不满足严格形状: %w", ErrPlannerInvalidPlan)
	}
	wire := planWire{Summary: candidate.Summary, Steps: make([]planWireStep, 0, len(candidate.Steps))}
	for _, step := range candidate.Steps {
		parsed := planWireStep{Kind: step.Kind, appeared: make(map[string]struct{}, 6)}
		switch step.Kind {
		case "go_to", "mine":
			parsed.X, parsed.Y, parsed.Z = &step.X, &step.Y, &step.Z
			parsed.appeared["x"], parsed.appeared["y"], parsed.appeared["z"] = struct{}{}, struct{}{}, struct{}{}
		case "place":
			parsed.X, parsed.Y, parsed.Z, parsed.Block = &step.X, &step.Y, &step.Z, &step.Block
			parsed.appeared["x"], parsed.appeared["y"], parsed.appeared["z"] = struct{}{}, struct{}{}, struct{}{}
			parsed.appeared["block"] = struct{}{}
		case "follow":
			parsed.PlayerID = &step.PlayerID
			parsed.appeared["player_id"] = struct{}{}
		}
		wire.Steps = append(wire.Steps, parsed)
	}

	plan := Plan{Summary: wire.Summary, Steps: make([]PlanStep, 0, len(wire.Steps))}
	for index, step := range wire.Steps {
		parsed, err := decodePlanStep(index, step)
		if err != nil {
			return Plan{}, fmt.Errorf("companion: planner %w: %w", ErrPlannerInvalidPlan, err)
		}
		plan.Steps = append(plan.Steps, parsed)
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, fmt.Errorf("companion: %w: %w", ErrPlannerInvalidPlan, err)
	}
	if err := validatePlanStepsAgainstSnapshot(plan.Steps, snapshot); err != nil {
		return Plan{}, fmt.Errorf("companion: planner %w: %w", ErrPlannerInvalidPlan, err)
	}
	return plan, nil
}

// decodePlanStep 按步骤的 kind 专属字段矩阵把中间形归一为强类型 PlanStep：
// go_to/mine 必须携带 x/y/z 且不得携带 block/player_id；place 必须携带
// x/y/z/block 且不得携带 player_id；follow 必须只携带 player_id。kind 必须
// 逐字等于交付全集之一（大小写敏感）；block 名查固定注册表归一为 BlockID，
// player_id 按 canonical UUIDv4 文本解析为 PlayerID。
//
// null 与缺席不等价（M5E 契约收紧）：显式 JSON null 一律视为「字段出现」，
// 专属外字段携带 null 与携带非法值的拒绝语义完全一致——排他判定看 has()
// 记录的出现事实而不是指针是否非 nil。必填字段（x/y/z、block、player_id）
// 携带 null 与缺席同样被拒绝，但拒绝理由分立：null 是出现的非法载荷，不是
// 缺字段。
func decodePlanStep(index int, step planWireStep) (PlanStep, error) {
	switch step.Kind {
	case "go_to", "mine":
		if step.has("block") || step.has("player_id") {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] kind %s 携带专属外字段 block/player_id", index, step.Kind)
		}
		// 缺席与显式 null 都不构成有限整数坐标，同因拒绝；二者的区别只体现在
		// 上方的排他矩阵（null 算出现）。
		if step.X == nil || step.Y == nil || step.Z == nil {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] 坐标不是整数", index)
		}
		kind := PlanStepGoTo
		if step.Kind == "mine" {
			kind = PlanStepMine
		}
		return PlanStep{Kind: kind, X: *step.X, Y: *step.Y, Z: *step.Z}, nil
	case "place":
		if step.has("player_id") {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] kind place 携带专属外字段 player_id", index)
		}
		if step.X == nil || step.Y == nil || step.Z == nil {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] 坐标不是整数", index)
		}
		if !step.has("block") {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] 缺少 block 字段", index)
		}
		if step.Block == nil {
			// 走到这里 block 已出现且为显式 null：与缺席分立拒绝（见函数 GoDoc）。
			return PlanStep{}, fmt.Errorf("计划 steps[%d] block 字段是显式 null", index)
		}
		item, ok := planPlaceItems[*step.Block]
		if !ok {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] 方块名不在注册表", index)
		}
		block, ok := core.ItemPlacement(item)
		if !ok {
			// 注册表测试锁定名字 ↔ 可放置方块双射，这里是防御双保险：坏表
			// 只会让 place 全部被拒，不会绕过注册表约束。
			return PlanStep{}, fmt.Errorf("计划 steps[%d] 方块名对应的物品不可放置", index)
		}
		return PlanStep{Kind: PlanStepPlace, X: *step.X, Y: *step.Y, Z: *step.Z, Block: block}, nil
	case "follow":
		if step.has("x") || step.has("y") || step.has("z") || step.has("block") {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] kind follow 携带专属外字段 x/y/z/block", index)
		}
		if !step.has("player_id") {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] 缺少 player_id 字段", index)
		}
		if step.PlayerID == nil {
			// 走到这里 player_id 已出现且为显式 null：与缺席分立拒绝。
			return PlanStep{}, fmt.Errorf("计划 steps[%d] player_id 字段是显式 null", index)
		}
		playerID, err := core.ParsePlayerID(*step.PlayerID)
		if err != nil {
			return PlanStep{}, fmt.Errorf("计划 steps[%d] follow 目标不是 canonical UUIDv4 文本", index)
		}
		return PlanStep{Kind: PlanStepFollow, PlayerID: playerID}, nil
	default:
		return PlanStep{}, fmt.Errorf("计划 steps[%d] kind 未交付", index)
	}
}

// validatePlanStepsAgainstSnapshot 校验依赖规划快照的步骤契约：follow 目标必须
// 来自快照在线玩家集合；mine 目标必须能在 dense frozen projection 中精确
// lookup 且满足 `planMineableBlock`，不依赖最多 256 条 ExposedBlocks；place
// 方块必须能在快照背包中找到对应物品。
func validatePlanStepsAgainstSnapshot(steps []PlanStep, snapshot PlanSnapshot) error {
	online := make(map[core.PlayerID]struct{}, len(snapshot.OnlinePlayers))
	for _, player := range snapshot.OnlinePlayers {
		online[player.ID] = struct{}{}
	}
	for index, step := range steps {
		switch step.Kind {
		case PlanStepFollow:
			if _, ok := online[step.PlayerID]; !ok {
				return fmt.Errorf("计划 steps[%d] follow 目标不在快照在线玩家集合", index)
			}
		case PlanStepMine:
			target := core.BlockPos{X: step.X, Y: step.Y, Z: step.Z}
			block, _, ok := snapshot.Terrain.Lookup(target)
			if !ok {
				return fmt.Errorf("计划 steps[%d] mine 目标超出冻结投影或列未 ready", index)
			}
			if !planMineableBlock(block) {
				return fmt.Errorf("计划 steps[%d] mine 目标方块不可采掘", index)
			}
		case PlanStepPlace:
			item, ok := planPlaceBlocks[step.Block]
			if !ok {
				return fmt.Errorf("计划 steps[%d] place 方块不在注册表", index)
			}
			if !planInventoryHolds(snapshot.Companion.Inventory, item) {
				return fmt.Errorf("计划 steps[%d] place 对应物品未在快照背包中持有", index)
			}
		}
	}
	return nil
}

// ValidatePlanAgainstSnapshot 以领域计划与一份权威快照重放纯校验规则。服务端
// 在 Agent 返回后的 tick 边界用它重新核对当前世界，拒绝规划期间已漂移的
// 目标、在线玩家或背包事实。
func ValidatePlanAgainstSnapshot(plan Plan, snapshot PlanSnapshot) error {
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("companion: %w: %w", ErrPlannerInvalidPlan, err)
	}
	if err := validatePlanStepsAgainstSnapshot(plan.Steps, snapshot); err != nil {
		return fmt.Errorf("companion: planner %w: %w", ErrPlannerInvalidPlan, err)
	}
	return nil
}

// trimTrailingSlash 去掉 endpoint 末尾的斜杠，保证路径拼接唯一。
func trimTrailingSlash(endpoint string) string {
	for len(endpoint) > 0 && endpoint[len(endpoint)-1] == '/' {
		endpoint = endpoint[:len(endpoint)-1]
	}
	return endpoint
}
