package worldgen_test

// 本文件锁定跨区块橡树拼合的生产语义:同一棵橡树的树冠横跨区块边界时,
// 相邻两个区块各自独立生成,两侧输出都必须与同一种子下的单点查询语义
// 逐格一致。对照物是生产自身的公共出口(`GenerateChunk` 与 `BaseBlockAt`),
// 不再存在任何旧 Go 实现副本。

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// TestOakTreeSpansChunkBorderConsistently 锁定跨区块橡树拼合:
// seed 42 cell (2,2) 的橡树根列在 (16,*,18),树冠 x∈[14,18]
// 横跨 chunk (0,1) 与 (1,1)。两个区块独立生成后,根列包围盒内每一格都
// 必须与单点查询语义(`BaseBlockAt`)逐格一致——若区块侧写入与单点查询
// 对跨界树的合并规则分叉,此处立刻变红;两侧还都必须真实落下树块。
// 语料前提(草地地表、树干存在、树高 4..6)全部经生产公共输出核实,
// 树高区间即冻结语义「最低高度 4、最高 6」的黑盒表达。
func TestOakTreeSpansChunkBorderConsistently(t *testing.T) {
	const seed = int64(42)
	const rootX, rootZ = int32(16), int32(18)
	production := worldgen.New(seed, false)

	surface := production.HeightAt(rootX, rootZ)
	pos := core.BlockPos{X: rootX, Y: surface, Z: rootZ}
	if got := production.TerrainBlockAt(pos); got != core.GrassID {
		t.Fatalf("语料前提失效: (%d,%d) 地表=%d，想要 GrassID", rootX, rootZ, got)
	}

	left := production.GenerateChunk(core.ChunkPos{X: 0, Z: 1})
	right := production.GenerateChunk(core.ChunkPos{X: 1, Z: 1})
	chunks := map[core.ChunkPos]interface {
		BlockAt(x int, y int32, z int) core.BlockID
	}{
		{X: 0, Z: 1}: left,
		{X: 1, Z: 1}: right,
	}
	blockAt := func(pos core.BlockPos) (core.BlockID, bool) {
		chunk, covered := chunks[pos.Chunk()]
		if !covered {
			return core.AirID, false
		}
		lx, _, lz := pos.Local()
		return chunk.BlockAt(lx, pos.Y, lz), true
	}

	// 树干顶自下而上扫描得出,不把候选选择器的内部 hash 位带进黑盒断言;
	// 扫描同时核实了「该列真的长着树」这一语料前提。
	topY := int32(-1)
	for y := surface + 1; y < core.MaxY; y++ {
		if got, _ := blockAt(core.BlockPos{X: rootX, Y: y, Z: rootZ}); got != core.OakLogID {
			break
		}
		topY = y
	}
	if height := topY - surface; height < 4 || height > 6 {
		t.Fatalf("根列 (%d,*,%d) 无树干或树高 %d 越出冻结语义 4..6", rootX, rootZ, topY-surface)
	}

	treeBlocksPerChunk := map[core.ChunkPos]int{}
	for y := surface + 1; y <= topY+1; y++ {
		for z := rootZ - 2; z <= rootZ+2; z++ {
			for x := rootX - 2; x <= rootX+2; x++ {
				pos := core.BlockPos{X: x, Y: y, Z: z}
				got, covered := blockAt(pos)
				if !covered {
					continue
				}
				if want := production.BaseBlockAt(pos); got != want {
					t.Fatalf("跨界树 %+v: 区块=%d 单点查询=%d", pos, got, want)
				}
				if got == core.OakLogID || got == core.LeavesID {
					treeBlocksPerChunk[pos.Chunk()]++
				}
			}
		}
	}
	for _, chunkPos := range []core.ChunkPos{{X: 0, Z: 1}, {X: 1, Z: 1}} {
		if treeBlocksPerChunk[chunkPos] == 0 {
			t.Fatalf("chunk %+v 内没有该树的方块,拼合语料失效", chunkPos)
		}
	}
}
