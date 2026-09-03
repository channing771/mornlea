package runtime

import (
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

// TickTunables 是一次权威 tick 按值复用的不可变参数束。两组参数来自彼此独立
// 的活动快照；捕获只保证每组各读取一次，不构成跨参数组的原子事务。
type TickTunables struct {
	Simulation tuning.Tunables
	Physics    physics.Tunables
}

// ActiveTickTunables 分别读取一次 simulation 与 physics 活动快照。
func ActiveTickTunables() TickTunables {
	return TickTunables{
		Simulation: tuning.ActiveTunables(),
		Physics:    physics.ActiveTunables(),
	}
}
