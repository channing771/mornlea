package core

// CombatTargetKind 是战斗目标的稳定身份类别。
type CombatTargetKind uint8

const (
	CombatTargetPlayer CombatTargetKind = iota + 1
	CombatTargetHostile
	// CombatTargetPassive 是被动牛受害者类别：值按追加顺序排在既有种类之后，
	// 近战等距仲裁时既有种类优先，既有行为不变。
	CombatTargetPassive
)

// Valid 报告类别能否进入权威战斗与线上确认。
func (kind CombatTargetKind) Valid() bool {
	return kind == CombatTargetPlayer || kind == CombatTargetHostile || kind == CombatTargetPassive
}
