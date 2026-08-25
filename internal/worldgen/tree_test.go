package worldgen_test

// 本文件锁定橡树生成冻结语义中可经生产公共输出观察的部分:固定树冠几何、
// 树冠层中心的「原木优先」合并结果与单点查询/区块输出的一致性。候选选择
// 器与树形的实现细节(候选 hash 位模式、salt 常量等)由 Rust engine 自身的
// 单测覆盖;这里只断言 `GenerateChunk` 与 `BaseBlockAt` 两条生产出口上的
// 可观察结果,语料前提一律用生产公共输出核实。

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/worldgen"
)

// absInt32 是树冠几何断言用的局部绝对值 helper。
func absInt32(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}

// TestOakTreeBlockAtUsesFixedCrownAndLogPriority 锁定冻结的树冠几何与
// 合并优先级:树干占据各层树冠的中心格(原木优先于树叶),下两层是缺四角
// 的 5×5 树叶、每层 21 个非空气格,顶下层是 3×3 树叶,顶层是含中心的十字
// 树叶,再往上一格回到空气。全部断言作用在生产 `GenerateChunk` 输出上;
// 语料取 seed 42 cell (-1,-1) 的根列 (-4,*,-4)、树高 5,其树冠完整落在
// chunk (-1,-1) 内部,单区块即可覆盖全部断言格。
func TestOakTreeBlockAtUsesFixedCrownAndLogPriority(t *testing.T) {
	const seed = int64(42)
	const rootX, rootZ = int32(-4), int32(-4)
	generator := worldgen.New(seed, false)

	surface := generator.HeightAt(rootX, rootZ)
	if got := generator.TerrainBlockAt(core.BlockPos{X: rootX, Y: surface, Z: rootZ}); got != core.GrassID {
		t.Fatalf("语料前提失效: (%d,%d) 地表=%d，想要 GrassID", rootX, rootZ, got)
	}
	rootY := surface + 1

	// 树干顶自下而上扫描得出,同时核实「该列真的长着树」的语料前提。
	topY := int32(-1)
	for y := rootY; y < core.MaxY; y++ {
		if generator.BaseBlockAt(core.BlockPos{X: rootX, Y: y, Z: rootZ}) != core.OakLogID {
			break
		}
		topY = y
	}
	if height := topY - surface; height != 5 {
		t.Fatalf("语料前提失效: 树高 %d，想要冻结语料的 5", height)
	}

	chunk := generator.GenerateChunk(core.ChunkPos{X: -1, Z: -1})
	at := func(x, y, z int32) core.BlockID {
		lx, _, lz := core.BlockPos{X: x, Y: y, Z: z}.Local()
		return chunk.BlockAt(lx, y, lz)
	}

	for y := rootY; y <= topY; y++ {
		if got := at(rootX, y, rootZ); got != core.OakLogID {
			t.Fatalf("trunk y=%d is %d，想要 OakLogID", y, got)
		}
	}

	// 下两层树冠:5×5 缺四角,中心被树干占据(原木优先),共 21 个非空气格。
	for _, y := range []int32{topY - 2, topY - 1} {
		occupied := 0
		for z := rootZ - 2; z <= rootZ+2; z++ {
			for x := rootX - 2; x <= rootX+2; x++ {
				got := at(x, y, z)
				if x == rootX && z == rootZ {
					if got != core.OakLogID {
						t.Fatalf("crown center y=%d is %d，想要 OakLogID", y, got)
					}
					occupied++
					continue
				}
				if absInt32(x-rootX) == 2 && absInt32(z-rootZ) == 2 {
					if got != core.AirID {
						t.Fatalf("crown corner (%d,%d,%d)=%d，想要空气", x, y, z, got)
					}
					continue
				}
				if got != core.LeavesID {
					t.Fatalf("crown leaf (%d,%d,%d)=%d，想要 LeavesID", x, y, z, got)
				}
				occupied++
			}
		}
		if occupied != 21 {
			t.Fatalf("crown y=%d occupied=%d，想要 21", y, occupied)
		}
	}

	// 顶下层:3×3 全占,中心是树干、其余是树叶。
	for z := rootZ - 1; z <= rootZ+1; z++ {
		for x := rootX - 1; x <= rootX+1; x++ {
			want := core.LeavesID
			if x == rootX && z == rootZ {
				want = core.OakLogID
			}
			if got := at(x, topY, z); got != want {
				t.Fatalf("top-under (%d,%d,%d)=%d，想要 %d", x, topY, z, got, want)
			}
		}
	}

	// 顶层:含中心的十字树叶(|dx|+|dz|≤1),四个对角色是空气。
	for z := rootZ - 1; z <= rootZ+1; z++ {
		for x := rootX - 1; x <= rootX+1; x++ {
			got := at(x, topY+1, z)
			if absInt32(x-rootX)+absInt32(z-rootZ) <= 1 {
				if got != core.LeavesID {
					t.Fatalf("top cross (%d,%d,%d)=%d，想要 LeavesID", x, topY+1, z, got)
				}
				continue
			}
			if got != core.AirID {
				t.Fatalf("top diagonal (%d,%d,%d)=%d，想要空气", x, topY+1, z, got)
			}
		}
	}
	if got := at(rootX, topY+2, rootZ); got != core.AirID {
		t.Fatalf("above canopy (%d,%d,%d)=%d，想要空气", rootX, topY+2, rootZ, got)
	}
}

// TestGeneratedChunkKeepsIntersectingLogs 覆盖两棵不同橡树的叶/干交叉:
// seed 42 的 chunk (0,4) 在 (0,82,72) 处叶与干重叠,生产结果必须是原木
// ——合并规则是「原木优先」。若退回成「原木只覆盖空气」,此测试必须失败。
// 语料前提不再依赖任何内部实现:树干连续性与相邻树叶的存在性都直接从
// 生产公共出口读出。
func TestGeneratedChunkKeepsIntersectingLogs(t *testing.T) {
	intersection := core.BlockPos{X: 0, Y: 82, Z: 72}
	generator := worldgen.New(42, false)

	// 语料前提一:交叉格在树干列上——下方一格仍是原木,证明确实有一棵
	// 树的主干穿过该列,而非孤立的巧合方块。
	belowTrunk := core.BlockPos{X: intersection.X, Y: intersection.Y - 1, Z: intersection.Z}
	lx, _, lz := belowTrunk.Local()
	chunk := generator.GenerateChunk(belowTrunk.Chunk())
	if got := chunk.BlockAt(lx, belowTrunk.Y, lz); got != core.OakLogID {
		t.Fatalf("语料前提失效: 交叉格下方 %+v=%d，想要树干", belowTrunk, got)
	}

	// 语料前提二:交叉格的水平邻格存在树叶,证明确实有另一棵树的树冠
	// 与这根树干在该格重叠。
	neighborLeaf := false
	for _, offset := range [][2]int32{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		pos := core.BlockPos{
			X: intersection.X + offset[0], Y: intersection.Y, Z: intersection.Z + offset[1],
		}
		nx, _, nz := pos.Local()
		neighborChunk := generator.GenerateChunk(pos.Chunk())
		if got := neighborChunk.BlockAt(nx, pos.Y, nz); got == core.LeavesID {
			neighborLeaf = true
			break
		}
	}
	if !neighborLeaf {
		t.Fatal("语料前提失效: 交叉格邻格没有树叶,叶/干重叠前提不成立")
	}

	if got := chunk.BlockAt(lx, intersection.Y, lz); got != core.OakLogID {
		t.Fatalf("生产区块交叉格=%d，想要 OakLogID", got)
	}
	if got := generator.BaseBlockAt(intersection); got != core.OakLogID {
		t.Fatalf("BaseBlockAt 交叉格=%d，想要 OakLogID", got)
	}
}

func TestBaseBlockAtMatchesGeneratedChunkWithOakTrees(t *testing.T) {
	generator := worldgen.New(42, false)
	for _, chunkPos := range []core.ChunkPos{{X: -1, Z: -1}, {X: 0, Z: -1}, {X: 0, Z: 0}, {X: 1, Z: 0}, {X: 0, Z: 1}, {X: 1, Z: 1}} {
		chunk := generator.GenerateChunk(chunkPos)
		baseX := chunkPos.X << core.SectionShift
		baseZ := chunkPos.Z << core.SectionShift
		for z := int32(0); z < core.SectionSize; z++ {
			for x := int32(0); x < core.SectionSize; x++ {
				for y := int32(core.MinY); y < core.MaxY; y++ {
					position := core.BlockPos{X: baseX + x, Y: y, Z: baseZ + z}
					if got, want := generator.BaseBlockAt(position), chunk.BlockAt(int(x), y, int(z)); got != want {
						t.Fatalf("chunk=%+v BaseBlockAt(%+v)=%d，GenerateChunk=%d", chunkPos, position, got, want)
					}
				}
			}
		}
	}
}
