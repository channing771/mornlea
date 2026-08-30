package fluid

import "github.com/channing771/mornlea/internal/core"

// Replaceable 报告：若把流体等级为 newLevel 的流动水（newLevel 取 1..7）写入
// target 所在的格，target 现有的内容是否允许被改写。
//
// 判定表（对应 task-2-brief.md 2.1 与 spec.md「流动规则」的多条 Scenario；
// 作物行为由 flood-destroys-crops 引入，见 design.md D1/D5）：
//
//	目标格                        可替换？
//	空气                          是
//	流体等级更大（更弱）的流动水    是
//	作物（小麦、马铃薯、胡萝卜任意生长阶段） 是——水淹即冲毁；冲毁后的掉落结算由权威写入侧
//	                                （sim）在流体写入汇聚点完成，本包不感知物品
//	源方块                        否——「源不可被流动方块替换」
//	流体等级更小或相等的流动水      否——防止弱水倒灌强水，也防止无意义的同值改写
//	非作物的实心方块                否——「实心方块不可替换」；作物已在上方单独
//	                                放行，故此行适用范围不含作物
//
// newLevel 由调用方按当前正在做的传播（垂直恒为 1，水平为 N+1）算出，本函数
// 不关心它是如何得来的，只做纯粹的比较。
func Replaceable(target core.BlockID, newLevel uint8) bool {
	if target == core.AirID {
		return true
	}
	if core.IsDoor(target) {
		// 门：开启可流入（视作空气），关闭实心不可流入，上半视作实心
		if target == core.DoorUpper {
			return false
		}
		switch target {
		case core.DoorLowerSouthOpen, core.DoorLowerWestOpen, core.DoorLowerNorthOpen, core.DoorLowerEastOpen:
			return true
		default:
			return false
		}
	}
	if core.IsCrop(target) {
		// 作物对流动水可替换。放行点必须在本函数而不能在 `evalCell` 里特判：
		// 垂直优先与水平递减这两处写入判定，加上 sim 重扫侧的两个不动点捷径
		// （`fluidSourceIsFixedPoint` 与 `fluidSectionIsFixedPoint`），全部经由
		// 这一个谓词读世界，改一处即全链一致；若捷径另用一套保守判定，
		// 「邻作物的源」会被误判成不动点跳过入队，水面在农田边永久卡死
		// （design.md D1 的被否决方案）。本包只回答「能不能写」，作物被冲毁后
		// 的掉落结算是权威写入侧（sim）的职责，fluid 包不感知物品。
		return true
	}
	if !core.IsFluid(target) {
		// 非空气、非作物、非流体、非可流入开启门：非作物的实心方块，一律不可替换。
		return false
	}
	if target == core.WaterSourceID {
		// 源方块的流体等级读作 0，若不特判会被 0 > newLevel 误判为不可替换——
		// 语义上确实不可替换，但要靠这条显式分支表达“源永不可替换”这条独立
		// 规则，而不是恰好靠等级比较凑对。
		return false
	}
	return core.FluidLevel(target) > newLevel
}

// strongerWrite 在同一 tick 内合并两个写往同一目标格的候选值，返回应当生效
// 的一个：流体候选优先于空气候选；两者都是流体时取流体等级更小（更强）的
// 一个（spec.md「同 tick 冲突写入取最强者」）。
//
// core.FluidLevel 对 core.AirID 也返回 0（见 internal/core/fluid.go），
// 若不先用 IsFluid 分流，裸比较 FluidLevel 会让空气被误判成「等级 0，最强」
// 而赢——因此这里显式分两步比较，不能只写一行 FluidLevel 比大小。
//
// 本函数是可交换、可结合的（strongerWrite(a,b)==strongerWrite(b,a)，且对
// 一组候选值反复两两合并的结果与合并次序无关），调用方可以按任意次序把一组
// 候选值两两 fold 到一起，结果确定——这是 Advance 用它合并同 tick 冲突写入
// 时不必关心处理次序的原因。
func strongerWrite(a, b core.BlockID) core.BlockID {
	aFluid, bFluid := core.IsFluid(a), core.IsFluid(b)
	if aFluid != bFluid {
		if aFluid {
			return a
		}
		return b
	}
	if !aFluid {
		// 两者都不是流体：在 Advance 当前的写入来源下只会是 AirID 与
		// AirID（evalCell 从不产出非流体、非空气的写入值），值相同，
		// 返回哪个都一样。
		return a
	}
	if core.FluidLevel(a) <= core.FluidLevel(b) {
		return a
	}
	return b
}

// horizontalNeighbors 返回 pos 的四个水平相邻格（不含上下）。
func horizontalNeighbors(pos core.BlockPos) [4]core.BlockPos {
	return [4]core.BlockPos{
		{X: pos.X + 1, Y: pos.Y, Z: pos.Z},
		{X: pos.X - 1, Y: pos.Y, Z: pos.Z},
		{X: pos.X, Y: pos.Y, Z: pos.Z + 1},
		{X: pos.X, Y: pos.Y, Z: pos.Z - 1},
	}
}

// sixNeighbors 返回 pos 的六个面邻格（上下 + 四个水平方向）。
func sixNeighbors(pos core.BlockPos) [6]core.BlockPos {
	h := horizontalNeighbors(pos)
	return [6]core.BlockPos{
		{X: pos.X, Y: pos.Y + 1, Z: pos.Z},
		{X: pos.X, Y: pos.Y - 1, Z: pos.Z},
		h[0], h[1], h[2], h[3],
	}
}
