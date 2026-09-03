package core

// CombatTargetKind 是战斗目标的稳定身份类别。
type CombatTargetKind uint8

const (
	CombatTargetPlayer CombatTargetKind = iota + 1
	CombatTargetHostile
)

// Valid 报告类别能否进入权威战斗与线上确认。
func (kind CombatTargetKind) Valid() bool {
	return kind == CombatTargetPlayer || kind == CombatTargetHostile
}
