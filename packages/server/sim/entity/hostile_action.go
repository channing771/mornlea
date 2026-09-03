package entity

import (
	"math"

	"github.com/channing771/mornlea/packages/shared/physics"
)

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
// 冻结（记录目标会话），结算统一推迟到同 tick 稍后的 advanceCombat。
//
// 与伙伴输入同款的每 tick 重写语义：无意图的夜行者与未收到任何意图的 tick 都
// 回到中性输入，重力与碰撞照常生效，夜行者在地面保持静止。
func (engine *engineContext) applyHostileActions(actions []HostileAction) {
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
