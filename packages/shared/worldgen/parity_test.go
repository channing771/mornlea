package worldgen_test

// 本文件锁定跨区块橡树拼合的生产语义:同一棵橡树的树干、树冠与分杈横跨
// 区块边界时,相邻区块各自独立生成,两侧输出都必须与同一种子下的单点查询
// 语义逐格一致。对照物是生产自身的公共出口(`GenerateChunk` 与
// `BaseBlockAt`),不再存在任何旧 Go 实现副本。新规则下冠形与分杈的最大
// 水平伸展为 3 格,扫描包围盒取根 ±3、顶上两层;旧规则的 ±2 半径会在珍异
// 大冠与分杈处漏扫。

import (
	"slices"

	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// assertTreeAcrossChunksConsistent 锁定单棵给定根列的橡树在相邻区块间的
// 拼合语义:各区块独立生成后,根列包围盒(根 ±3、树干底到顶上两层)内每一格
// 都必须与单点查询语义(`BaseBlockAt`)逐格一致——若区块侧写入与单点查询
// 对跨界树的合并规则分叉,此处立刻变红;每个传入区块还都必须真实落下该树
// 的方块。语料前提(草地地表、树干存在、树高区间)全部经生产公共输出核实,
// 树高区间即新规则档位的黑盒表达(普通 5..7、珍异 8..12)。返回地表与树干
// 顶,供调用方追加档位相关断言(珍异大冠宽度、分杈)。
func assertTreeAcrossChunksConsistent(t *testing.T, seed int64, rootX, rootZ int32, chunks []core.ChunkPos, minHeight, maxHeight int32) (int32, int32) {
	t.Helper()
	production := worldgen.New(seed, false)

	surface := production.HeightAt(rootX, rootZ)
	pos := core.BlockPos{X: rootX, Y: surface, Z: rootZ}
	if got := production.TerrainBlockAt(pos); got != core.GrassID {
		t.Fatalf("语料前提失效: (%d,%d) 地表=%d，想要 GrassID", rootX, rootZ, got)
	}

	generated := map[core.ChunkPos]interface {
		BlockAt(x int, y int32, z int) core.BlockID
	}{}
	for _, chunkPos := range chunks {
		generated[chunkPos] = production.GenerateChunk(chunkPos)
	}
	blockAt := func(pos core.BlockPos) (core.BlockID, bool) {
		chunk, covered := generated[pos.Chunk()]
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
	if height := topY - surface; height < minHeight || height > maxHeight {
		t.Fatalf("根列 (%d,*,%d) 无树干或树高 %d 越出语料档位 %d..%d", rootX, rootZ, topY-surface, minHeight, maxHeight)
	}

	treeBlocksPerChunk := map[core.ChunkPos]int{}
	for y := surface + 1; y <= topY+2; y++ {
		for z := rootZ - 3; z <= rootZ+3; z++ {
			for x := rootX - 3; x <= rootX+3; x++ {
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
	for _, chunkPos := range chunks {
		if treeBlocksPerChunk[chunkPos] == 0 {
			t.Fatalf("chunk %+v 内没有该树的方块,拼合语料失效", chunkPos)
		}
	}
	return surface, topY
}

// TestOakTreeSpansChunkBorderConsistently 锁定跨区块普通橡树拼合:
// seed 42 的橡树根列在 (16,*,18)、树高 5(普通档),树冠 x∈[14,18]
// 横跨 chunk (0,1) 与 (1,1)。包围盒按新规则半径取根 ±3、顶上两层。
func TestOakTreeSpansChunkBorderConsistently(t *testing.T) {
	assertTreeAcrossChunksConsistent(t, 42, 16, 18,
		[]core.ChunkPos{{X: 0, Z: 1}, {X: 1, Z: 1}}, 5, 7)
}

// TestRareOakTreeSpansChunkBorderConsistently 锁定跨区块珍异大树拼合:
// seed 42 的珍异橡树根列在 (31,*,38)、树高 10,球状大冠宽达 ±3 格、横跨
// chunk (1,2) 与 (2,2)。除逐格一致外,另断言大冠旁侧突出主干 2 格以上、
// 分杈为横向 1..2 条(顺轴连续、只占原始空气格由合并优先级保证)。
func TestRareOakTreeSpansChunkBorderConsistently(t *testing.T) {
	const seed = int64(42)
	const rootX, rootZ = int32(31), int32(38)
	production := worldgen.New(seed, false)

	_, topY := assertTreeAcrossChunksConsistent(t, seed, rootX, rootZ,
		[]core.ChunkPos{{X: 1, Z: 2}, {X: 2, Z: 2}}, 8, 12)

	wide := false
	for _, y := range []int32{topY - 3, topY - 2, topY - 1} {
		for dz := int32(-3); dz <= 3 && !wide; dz++ {
			for dx := int32(-3); dx <= 3; dx++ {
				radial := absInt32(dx)
				if other := absInt32(dz); other > radial {
					radial = other
				}
				if radial != 3 {
					continue
				}
				if production.BaseBlockAt(core.BlockPos{X: rootX + dx, Y: y, Z: rootZ + dz}) == core.LeavesID {
					wide = true
					break
				}
			}
		}
	}
	if !wide {
		t.Fatal("珍异大冠旁侧未突出主干 2 格以上")
	}

	dirs := branchLogDirections(production, rootX, rootZ, topY)
	if len(dirs) == 0 || len(dirs) > 2 {
		t.Fatalf("珍异分杈必须为横向 1..2 条,实际 %d 个方向 %v", len(dirs), dirs)
	}
	for dir, alongs := range dirs {
		slices.Sort(alongs)
		for i, along := range alongs {
			if along != int32(i+1) {
				t.Fatalf("分杈方向 %v 不顺轴连续: %v", dir, alongs)
			}
		}
	}
}

// TestOakTreeNegativeCoordinatesConsistent 锁定负坐标跨界橡树:seed 42 的
// 普通橡树根列在 (-32,*,-4)、树高 5,树冠横跨 chunk (-3,-1) 与 (-2,-1)。
// 负坐标走同一候选规则,拼合语义与正坐标一致。
func TestOakTreeNegativeCoordinatesConsistent(t *testing.T) {
	assertTreeAcrossChunksConsistent(t, 42, -32, -4,
		[]core.ChunkPos{{X: -3, Z: -1}, {X: -2, Z: -1}}, 5, 7)
}
