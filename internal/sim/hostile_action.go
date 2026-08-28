package sim

import (
	"math"

	"github.com/channing771/mornlea/internal/physics"
)

// HostileAction 是有界追逐 worker 在 tick 边界提交的夜行者意图，按稳定 ID 寻
// 址。MoveX/MoveZ 是**世界轴**方向量（不是 `physics.Input` 的体轴分量）：模
// 长被消费端归一，方向折算成朝向后以既有 per-actor `physics.Step` 前进，因此
// 字段面无需携带 yaw；Jump 表达本 tick 起跳意图；AttackTarget 冻结一次近战攻
// 击意图，TargetSession 指出被攻击玩家的会话——会话到玩家的解析归 server 编
// 排层所有，`sim` 不维护 PlayerID 到会话的映射，攻击载荷因此直接携带会话身
// 份并由结算阶段做距离/维度/存活重验。
type HostileAction struct {
	// ID 是被寻址夜行者的稳定非零 ID；未知 ID 的意图确定性丢弃。
	ID uint64
	// MoveX/MoveZ 只表达方向，取值必须有限且落在 [-1,1]；零向量表示本 tick
	// 不移动（保持朝向）。
	MoveX, MoveZ float32
	// Jump 只在有移动意图时有意义；无移动时照常透传，由权威物理裁决。
	Jump bool
	// AttackTarget 为 true 时 TargetSession 必须非零；二者成对约束与
	// CompanionAction 的判别载荷纪律一致。
	AttackTarget bool
	// TargetSession 是攻击意图的目标玩家会话；仅在 AttackTarget 时有效。
	TargetSession SessionID
}

// EnqueueHostileAction 可由 server 编排层在持有 `stepMu` 的 tick 路径并发安
// 全调用，把一条夜行者意图投递进有界 inbox。容量按全集合计：每 tick 每只夜
// 行者至多一条合法意图（≤`maxHostiles` 条），满员时立即丢弃并返回 false，绝
// 不阻塞权威 tick——丢弃是安全的：位置始终权威，下一 tick 重新提交即可。
func (engine *Engine) EnqueueHostileAction(action HostileAction) bool {
	engine.inboxMu.Lock()
	defer engine.inboxMu.Unlock()
	if len(engine.hostileActions) >= maxHostiles {
		return false
	}
	engine.hostileActions = append(engine.hostileActions, action)
	return true
}

// takeHostileActions 在夜行者阶段的 tick 边界排空 inbox；返回的切片此后只被
// 权威 tick 单写者触碰。
func (engine *Engine) takeHostileActions() []HostileAction {
	engine.inboxMu.Lock()
	actions := append([]HostileAction(nil), engine.hostileActions...)
	engine.hostileActions = engine.hostileActions[:0]
	engine.inboxMu.Unlock()
	return actions
}

// validHostileAction 按载荷做防御校验：方向量必须有限且落在 [-1,1]（与玩家/
// 伙伴输入同界），攻击意图必须携带非零目标会话。意图来自服务端编排层而非网
// 络，这里仍是防御性校验——非法载荷确定性丢弃，绝不把坏意图喂给权威物理。
func validHostileAction(action HostileAction) bool {
	if !finiteInputComponent(action.MoveX) || !finiteInputComponent(action.MoveZ) {
		return false
	}
	if action.MoveX < -1 || action.MoveX > 1 || action.MoveZ < -1 || action.MoveZ > 1 {
		return false
	}
	if action.AttackTarget && action.TargetSession == 0 {
		return false
	}
	return true
}

// applyHostileActions 是权威 tick 夜行者阶段的第一步（生成判定之后、统一物理
// 之前）：先把全部夜行者的输入复位为中性（仅保留当前 yaw）、攻击意图清零，
// 再按入队顺序取每个 ID 最早的一条合法意图。重复或非法意图确定性丢弃，未知
// ID 的意图同样丢弃。移动意图把世界轴方向折算为朝向（yaw=0 面 -Z，故
// yaw=atan2(-x,-z)）并以前进挡前进，实际位移永远由权威物理决定；攻击意图只
// 冻结（记录目标会话），结算统一推迟到同 tick 稍后的 advanceHostileMelee。
//
// 与伙伴输入同款的每 tick 重写语义：无意图的夜行者与未收到任何意图的 tick 都
// 回到中性输入，重力与碰撞照常生效，夜行者在地面保持静止。
func (engine *Engine) applyHostileActions(actions []HostileAction) {
	for index := range engine.hostiles.entries {
		entry := &engine.hostiles.entries[index]
		entry.input = physics.Input{Yaw: entry.yaw}
		entry.attackIntent = false
		entry.attackTargetSession = 0
	}
	if len(actions) == 0 {
		return
	}
	seen := make(map[uint64]struct{}, len(actions))
	for _, action := range actions {
		if _, duplicate := seen[action.ID]; duplicate {
			continue
		}
		if !validHostileAction(action) {
			continue
		}
		index := engine.hostiles.findIndex(action.ID)
		if index < 0 {
			continue
		}
		seen[action.ID] = struct{}{}
		entry := &engine.hostiles.entries[index]
		if action.MoveX != 0 || action.MoveZ != 0 {
			yaw := normalizeYaw(float32(math.Atan2(
				float64(-action.MoveX), float64(-action.MoveZ),
			)))
			entry.input = physics.Input{MoveZ: 1, Jump: action.Jump, Yaw: yaw}
			entry.yaw = yaw
		} else {
			entry.input = physics.Input{Yaw: entry.yaw, Jump: action.Jump}
		}
		if action.AttackTarget {
			entry.attackIntent = true
			entry.attackTargetSession = action.TargetSession
		}
	}
}
