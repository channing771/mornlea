package fluid

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// eval_bench_test.go：native 批量求值路径的 Advance 基准（record-only）。
//
// 场景是溃坝：1200 个水源同 tick 到期，预算 512（与 sim 默认 tunable 一致）
// 使单次 Advance 走满批量批（512 项编码 → 一次 FluidEvalBatch → 解码合并 →
// 排序提交）。盆地封闭保证不逃逸到无限空气世界。每轮迭代重建场景——水体
// 会收敛到不动点，复用同一场景会让后面的迭代退化成空转，量不到真实批。

// benchFluidScene 构造一个单次推进即走满预算的活跃水体场景。
// 供本文件与 fluid_oracle_bench 构建标签下的 oracle 对照基准共用。
func benchFluidScene() (*memWorld, *Queue) {
	w := newBasin(0, 0, 19, 19, 0, 16)
	fillBox(w, 0, 1, 0, 4, 12, 19, core.WaterSourceID)
	q := NewQueue()
	seedFromFluid(w, q, 0, 0) // delay=0：全部立即到期
	return w, q
}

func BenchmarkAdvanceEval(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		w, q := benchFluidScene()
		b.StartTimer()
		q.Advance(0, w, testBudget, testDelay)
	}
}
