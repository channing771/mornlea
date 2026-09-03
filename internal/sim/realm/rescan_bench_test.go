package realm

// rescan_bench_test.go：流体重扫 native 路径的整块基准（record-only）。
//
// 场景是单次 `rescanChunkFluids` 整块重扫（五平面 + 盒编码 + kernel + 入队），
// 预算给足让单次调用扫完。海洋型（全均匀段，捷径命中为主）与地表型（混杂
// 逐格段）各一例；数值只记录，不作为门禁。

import (
	"testing"

	"github.com/channing771/mornlea/internal/fluid"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// benchRescanChunk 场景与驱动的共用入口：ready 区块按生成路径入维度后，重复
// 执行整块重扫。`rescanChunkFluids` 完成即重置游标，队列按位置去重，重复迭代
// 稳定在「编码 5 个盒 + 5 次 kernel 扫描 + 幂等入队」的真实每 tick 工作量。
func benchRescanChunk(b *testing.B, chunks ...*world.Chunk) {
	b.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	for _, chunk := range chunks {
		if !dimension.BeginGeneration(chunk.Pos) {
			b.Fatalf("区块 %+v 未开始生成", chunk.Pos)
		}
		if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
			b.Fatalf("区块 %+v 进入 Ready 失败: %v", chunk.Pos, err)
		}
	}
	queue := fluid.NewQueue()
	b.ResetTimer()
	for range b.N {
		state.rescanChunkFluids(queue, dimension, chunks[0].Pos, 0, 5, 1<<20)
	}
}

// BenchmarkRescanChunk/ocean：海洋型区块（0..7 段均匀石、8..13 段均匀水源、
// 14..23 段均匀空气）带一个均匀空气分叉段，四邻就绪。
func BenchmarkRescanChunk(b *testing.B) {
	b.Run("ocean", func(b *testing.B) {
		benchRescanChunk(
			b,
			buildOceanChunk(core.ChunkPos{}, 12),
			buildOceanChunk(core.ChunkPos{X: 1, Z: 0}, -1),
			buildOceanChunk(core.ChunkPos{X: -1, Z: 0}, -1),
			buildOceanChunk(core.ChunkPos{X: 0, Z: 1}, -1),
			buildOceanChunk(core.ChunkPos{X: 0, Z: -1}, -1),
		)
	})
	b.Run("surface", func(b *testing.B) {
		benchRescanChunk(
			b,
			buildSurfaceChunk(core.ChunkPos{}),
			buildSurfaceChunk(core.ChunkPos{X: 1, Z: 0}),
			buildSurfaceChunk(core.ChunkPos{X: -1, Z: 0}),
			buildSurfaceChunk(core.ChunkPos{X: 0, Z: 1}),
			buildSurfaceChunk(core.ChunkPos{X: 0, Z: -1}),
		)
	})
}
