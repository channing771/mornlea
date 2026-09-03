package fluid

import "github.com/channing771/mornlea/packages/shared/core"

// oracle_test.go：迁移期差分门禁的 Go oracle。
//
// `evalCell`/`flowingSurvives` 原是 `rules.go` 的生产实现，rust-engine-fluid
// 变更把单格求值迁入 Rust engine kernel（`nativeabi.FluidEvalBatch`）后，这两
// 个函数逐字移入测试侧：生产路径不再调用它们，它们只作为 eval_differential_
// test.go 的对照 oracle 与 rules_test.go 的既有断言基础存在。逐字保留（而非
// 重写）是刻意的——差分门禁的价值正是「迁移前的实现原样钉在那里」，任何对
// oracle 的改写都会让两侧漂移失去参照。kernel 全量接管且差分退役后本文件
// 随之删除。

// flowingSurvives 判定非源流动格 pos（当前方块 self，流体等级为
// core.FluidLevel(self)）在本 tick 是否继续存活：上方是任意流体，或水平邻居
// 中存在流体等级严格小于自身的流体（源的等级读作 0，天然满足“更小”）。
//
// 只调用一次 w.BlockAt 读每个邻居，且只在 Advance 提交写入之前被调用，因此
// 读到的都是本 tick 起始时的世界状态——这是避免同 tick 内读写交错导致振荡的
// 关键（design.md 的 Risk「流动规则的存活判定产生振荡」）。
func flowingSurvives(pos core.BlockPos, self core.BlockID, w FluidWorld) bool {
	level := core.FluidLevel(self)
	above := core.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}
	if core.IsFluid(w.BlockAt(above)) {
		return true
	}
	for _, n := range horizontalNeighbors(pos) {
		nb := w.BlockAt(n)
		if core.IsFluid(nb) && core.FluidLevel(nb) < level {
			return true
		}
	}
	return false
}

// evalCell 对 pos 求值一次完整的单格流动规则（spec.md「流动规则」全部
// Scenario），返回本次求值想要做出的写入集合：key 是目标格，value 是新
// 方块编号。返回空 map 表示本次求值不产生任何变化。
//
// evalCell 本身不写入 w，只读取——调用方（Queue.Advance）负责把多次 evalCell
// 的结果合并后一次性提交，从而保证本 tick 内的存活/替换判定全部只看 tick
// 起始时的状态。
func evalCell(pos core.BlockPos, w FluidWorld) map[core.BlockPos]core.BlockID {
	writes := make(map[core.BlockPos]core.BlockID)

	self := w.BlockAt(pos)
	if !core.IsFluid(self) {
		// 队列里的格在真正被处理前可能已经因为别的原因变成非流体（比如被
		// 玩家挖掉后又放了实心方块）；这种陈旧待更新项直接跳过，不产生变化。
		return writes
	}

	if self != core.WaterSourceID {
		// 规则「源方块永不自然消失」+「流动方块失去支撑后消失」：只有非源
		// 流动格才需要做存活判定。
		if !flowingSurvives(pos, self, w) {
			writes[pos] = core.AirID
			return writes // 本格本 tick 消失，不再谈传播。
		}
	}

	// 规则「垂直优先」：下方可替换时只向下写最强流动水（等级 1），本次
	// MUST NOT 再向任何水平方向传播。
	below := core.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z}
	if Replaceable(w.BlockAt(below), 1) {
		writes[below] = core.WaterLevel1ID
		return writes
	}

	// 规则「水平传播递减」+「水平传播上界」：下方不可替换时才水平扩散，
	// 等级从当前格的等级 +1；源的等级读作 0，其水平邻居因此得到等级 1。
	nextLevel := core.FluidLevel(self) + 1
	if nextLevel > 7 {
		// 等级 7 已是传播下界，世界中不得出现等级 > 7 的流体方块。
		return writes
	}
	// 用算式而非 switch/查表算出目标编号，依赖 packages/shared/core/block.go 里
	// WaterSourceID..WaterLevel7ID 这 8 个编号连续排布、且 WaterLevelN ==
	// WaterSourceID+N 这一稳定约定（该文件注释里明确写了这些编号只能追加、
	// 不能重排，所以这条依赖是安全的）。测试侧（e2e_test.go）刻意不复用
	// 同一算式，改用具名常量数组断言，避免两边用同一条算式互相"对齐出错"。
	nextID := core.WaterSourceID + core.BlockID(nextLevel)
	for _, n := range horizontalNeighbors(pos) {
		if Replaceable(w.BlockAt(n), nextLevel) {
			writes[n] = nextID
		}
	}
	return writes
}
