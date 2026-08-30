package fluid

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// eval_differential_test.go：Rust eval kernel 与 Go oracle 的逐项差分门禁。
//
// 迁移期契约：`Queue.Advance` 的单格求值已改走 `nativeabi.FluidEvalBatch`，
// 本文件用测试侧 Go oracle `evalCell`（oracle_test.go，逐字保留迁移前的
// `rules.go` 实现）对 kernel 做差分——每条用例把 pos 的 7 格邻域经生产同一条
// 编码路径送进 kernel，解码输出得到写集，与 oracle 写集逐项逐位比对
// （目标格 + BlockID）。任何一侧的规则漂移都会在此显形；迁移结束、oracle
// 退役后，位级防线由 eval_golden_test.go 的源码字面向量接管。
//
// 用例覆盖面与 kernel 的 Rust 单测分支表一致：垂直优先、水平扩散等级 +1、
// 等级 7 到界、存活判定（上方流体 / 更强水平邻居）、非源消亡写空气、陈旧项
// 空写、作物可替换、门四态（开启可流入 / 关闭与上半不可）、源不可替换、
// 弱水被强水替换，另加 `core.BarrierID` 邻格（sim 侧 scope 外的读语义）。

// evalDifferentialCase 是一条差分用例：在世界 w 里对 pos 做一次单格求值。
type evalDifferentialCase struct {
	name string
	w    *memWorld
	pos  core.BlockPos
}

// evalDifferentialCases 逐分支构造差分用例。全部世界直接用 `memWorld` 搭建：
// 未写入的格读作空气，与 kernel 的「槽位值即所见」语义一致；sim 生产侧
// scope 外读作 Barrier 的语义由用例「Barrier 邻格」显式覆盖。
func evalDifferentialCases() []evalDifferentialCase {
	at := func(x, y, z int32) core.BlockPos { return core.BlockPos{X: x, Y: y, Z: z} }
	newCase := func(name string, pos core.BlockPos, setup func(w *memWorld)) evalDifferentialCase {
		w := newMemWorld()
		setup(w)
		return evalDifferentialCase{name: name, w: w, pos: pos}
	}
	return []evalDifferentialCase{
		newCase("源垂直优先-下方空气写等级1", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterSourceID)
		}),
		newCase("源下方实心-水平扩散等级1", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterSourceID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
		}),
		newCase("等级3存活-水平传播等级4", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel3ID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			// +z 方向放一个源：既提供存活支撑，又因「源不可替换」占据该方向。
			w.SetBlock(at(0, 10, 1), core.WaterSourceID)
		}),
		newCase("等级7到界-上方流体保活但不传播", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel7ID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			w.SetBlock(at(0, 11, 0), core.WaterLevel1ID)
		}),
		newCase("非源无支撑-自格写空气", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel3ID)
		}),
		newCase("上方流体存活-下方弱水被替换为等级1", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel2ID)
			w.SetBlock(at(0, 11, 0), core.WaterLevel1ID)
			w.SetBlock(at(0, 9, 0), core.WaterLevel5ID)
		}),
		newCase("陈旧项-自格非流体产出空写集", at(0, 10, 0), func(w *memWorld) {
			// 不写入 pos：读作空气，等价「入队后被玩家挖掉」的陈旧待更新项。
		}),
		newCase("作物邻格可替换-水平写入作物", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterSourceID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			w.SetBlock(at(1, 10, 0), core.WheatStage2ID)
			w.SetBlock(at(-1, 10, 0), core.PotatoStage5ID)
			w.SetBlock(at(0, 10, 1), core.CarrotStage1ID)
		}),
		newCase("门四态-开启可流入关闭与上半不可", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterSourceID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			w.SetBlock(at(1, 10, 0), core.DoorLowerSouthOpen)
			w.SetBlock(at(-1, 10, 0), core.DoorLowerNorthClosed)
			w.SetBlock(at(0, 10, 1), core.DoorUpper)
		}),
		newCase("Barrier邻格-视作实心不可替换", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterSourceID)
			// 下方与 +x 都是 Barrier：垂直优先被挡，水平只剩三个空气方向。
			w.SetBlock(at(0, 9, 0), core.BarrierID)
			w.SetBlock(at(1, 10, 0), core.BarrierID)
		}),
		newCase("源不可替换-水平邻居是源", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel2ID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			// +x 的源提供存活支撑，且自身不可被写入。
			w.SetBlock(at(1, 10, 0), core.WaterSourceID)
		}),
		newCase("弱水被强水替换-水平方向等级2覆盖等级5", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel1ID)
			w.SetBlock(at(0, 11, 0), core.WaterSourceID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			w.SetBlock(at(1, 10, 0), core.WaterLevel5ID)
		}),
		newCase("同等级流动水不可替换", at(0, 10, 0), func(w *memWorld) {
			w.SetBlock(at(0, 10, 0), core.WaterLevel3ID)
			w.SetBlock(at(0, 11, 0), core.WaterLevel1ID)
			w.SetBlock(at(0, 9, 0), core.StoneID)
			w.SetBlock(at(1, 10, 0), core.WaterLevel3ID)
		}),
	}
}

// runEvalBatch 走生产路径对 pos 做一次 kernel 求值并返回解码写集：
// `beginEvalBatch`/`enqueueEvalItem` 编码（与 `Advance` 阶段一同一条路径）、
// `finishEvalBatch` 调用 `nativeabi.FluidEvalBatch` 并解码合并。与 oracle 的
// 比对因此覆盖「编码 → kernel → 解码」整条链，而不是只盯 kernel 字节。
func runEvalBatch(w FluidWorld, pos core.BlockPos) map[core.BlockPos]core.BlockID {
	q := NewQueue()
	q.beginEvalBatch()
	q.enqueueEvalItem(w, pos)
	writes := make(map[core.BlockPos]core.BlockID, 4)
	q.finishEvalBatch(writes)
	return writes
}

// TestFluidEvalBatchMatchesOracle 是迁移期的差分门禁：kernel 写集与 oracle
// `evalCell` 写集逐项逐位一致（目标格 + BlockID）。
func TestFluidEvalBatchMatchesOracle(t *testing.T) {
	for _, tc := range evalDifferentialCases() {
		t.Run(tc.name, func(t *testing.T) {
			want := evalCell(tc.pos, tc.w)
			got := runEvalBatch(tc.w, tc.pos)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("kernel 写集与 oracle 不一致：\ngot  %v\nwant %v", got, want)
			}
		})
	}
}
