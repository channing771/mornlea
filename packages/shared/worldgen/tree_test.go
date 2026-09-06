package worldgen_test

// 本文件锁定橡树生成冻结语义中可经生产公共输出观察的部分:固定树冠几何、
// 树冠层中心的「原木优先」合并结果与单点查询/区块输出的一致性。候选选择
// 器与树形的实现细节(候选 hash 位模式、salt 常量等)由 Rust engine 自身的
// 单测覆盖;这里只断言 `GenerateChunk` 与 `BaseBlockAt` 两条生产出口上的
// 可观察结果,语料前提一律用生产公共输出核实。

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// absInt32 是树冠几何断言用的局部绝对值 helper。
func absInt32(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}

// TestOakTreeBlockAtUsesFixedCrownAndLogPriority 锁定新规则下普通橡树的树冠几何与
// 合并优先级:树干占据各层树冠的中心格(原木优先于树叶),下两层是缺四角
// 的 5×5 树叶、每层 21 个非空气格,顶下层是 3×3 树叶,顶层是含中心的十字
// 树叶;蓬松档在顶上再加一层去角 3×3 顶(形状同十字),标准档顶上回到空气。
// 四层共有几何由 `assertOrdinaryCrownLayers` 断言,档位经生产输出判定后走
// 各自的完整断言。全部断言作用在生产 `GenerateChunk` 输出上;语料取
// seed 42 cell (-1,-1) 的根列 (-4,*,-4)、树高 5(普通档,新规则 5..7 内),
// 其树冠完整落在 chunk (-1,-1) 内部,单区块即可覆盖全部断言格。
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
	if height := topY - surface; height < 5 || height > 7 {
		t.Fatalf("语料前提失效: 树高 %d，想要普通档 5..7", height)
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

	assertOrdinaryCrownLayers(t, at, rootX, topY, rootZ)

	// 档位判定:蓬松档顶上第二层中心是树叶,标准档该格回到空气。
	if got := at(rootX, topY+2, rootZ); got == core.LeavesID {
		assertFluffyTop(t, at, rootX, topY, rootZ)
	} else if got != core.AirID {
		t.Fatalf("canopy tier (%d,%d,%d)=%d，想要树叶(蓬松)或空气(标准)", rootX, topY+2, rootZ, got)
	}
}

// assertOrdinaryCrownLayers 断言普通树冠共有的四层几何:顶下两层去角 5×5
// (中心被树干占据,每层 21 个非空气格)、顶下层完整 3×3、顶上一层十字。
// 标准与蓬松两档共有,与珍异球状大冠无关。
func assertOrdinaryCrownLayers(t *testing.T, at func(x, y, z int32) core.BlockID, rootX, topY, rootZ int32) {
	t.Helper()
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
}

// assertFluffyTop 断言蓬松档的顶上第二层:去角 3×3 树叶顶(中心与四个水平
// 相邻格是树叶、四角是空气),再往上一格回到空气。
func assertFluffyTop(t *testing.T, at func(x, y, z int32) core.BlockID, rootX, topY, rootZ int32) {
	t.Helper()
	for z := rootZ - 1; z <= rootZ+1; z++ {
		for x := rootX - 1; x <= rootX+1; x++ {
			got := at(x, topY+2, z)
			if absInt32(x-rootX)+absInt32(z-rootZ) <= 1 {
				if got != core.LeavesID {
					t.Fatalf("fluffy top (%d,%d,%d)=%d，想要 LeavesID", x, topY+2, z, got)
				}
				continue
			}
			if got != core.AirID {
				t.Fatalf("fluffy corner (%d,%d,%d)=%d，想要空气", x, topY+2, z, got)
			}
		}
	}
	if got := at(rootX, topY+3, rootZ); got != core.AirID {
		t.Fatalf("above canopy (%d,%d,%d)=%d，想要空气", rootX, topY+3, rootZ, got)
	}
}

// assertStandardTopAir 断言标准档顶上第二层整个 3×3 回到空气。
func assertStandardTopAir(t *testing.T, at func(x, y, z int32) core.BlockID, rootX, topY, rootZ int32) {
	t.Helper()
	for z := rootZ - 1; z <= rootZ+1; z++ {
		for x := rootX - 1; x <= rootX+1; x++ {
			if got := at(x, topY+2, z); got != core.AirID {
				t.Fatalf("standard above (%d,%d,%d)=%d，想要空气", x, topY+2, z, got)
			}
		}
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
	// 覆盖正负坐标的六个区块外加含珍异大树的 chunk (-1,0)(根列 (-7,*,15)、
	// 树高 8,大冠与分杈落界内),新规则的冠形与分杈一致性一并锁定。
	for _, chunkPos := range []core.ChunkPos{{X: -1, Z: -1}, {X: 0, Z: -1}, {X: 0, Z: 0}, {X: 1, Z: 0}, {X: 0, Z: 1}, {X: 1, Z: 1}, {X: -1, Z: 0}} {
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

// TestOakTrunkColumnsAreContinuous 锁定树干整体生成语义:生长在草地上的
// 每根树干都是一段无空洞的连续原木(树干路径非空则整树丢弃,分杈只跳过
// 固体格,都不允许留下半截树干),且单点查询与整块输出在树干段逐格一致
// (原木优先于树叶,合并顺序不改变结果)。扫描 chunk [-2,2)²,结构与多样
// 性测试的树干发现同源(草地表+表上第一格原木),树高只可能是普通 5..7 或
// 珍异 8..12。
func TestOakTrunkColumnsAreContinuous(t *testing.T) {
	const seed = int64(42)
	production := worldgen.New(seed, false)
	trunks := 0
	for cx := int32(-2); cx < 2; cx++ {
		for cz := int32(-2); cz < 2; cz++ {
			chunk := production.GenerateChunk(core.ChunkPos{X: cx, Z: cz})
			for lx := 0; lx < core.SectionSize; lx++ {
				for lz := 0; lz < core.SectionSize; lz++ {
					wx := cx*core.SectionSize + int32(lx)
					wz := cz*core.SectionSize + int32(lz)
					surface := production.HeightAt(wx, wz)
					if chunk.BlockAt(lx, surface, lz) != core.GrassID {
						continue
					}
					if chunk.BlockAt(lx, surface+1, lz) != core.OakLogID {
						continue
					}
					trunks++
					top := surface + 1
					for top+1 < core.MaxY && chunk.BlockAt(lx, top+1, lz) == core.OakLogID {
						top++
					}
					if height := top - surface; height < 5 || height > 12 {
						t.Fatalf("(%d,%d) 树干高 %d 越出 5..12,疑似半截树干", wx, wz, height)
					}
					for y := surface + 1; y <= top; y++ {
						if got := chunk.BlockAt(lx, y, lz); got != core.OakLogID {
							t.Fatalf("(%d,%d,%d) 树干空洞=%d,想要 OakLogID", wx, y, wz, got)
						}
						if got := production.BaseBlockAt(core.BlockPos{X: wx, Y: y, Z: wz}); got != core.OakLogID {
							t.Fatalf("BaseBlockAt(%d,%d,%d)=%d,树干与整块分叉", wx, y, wz, got)
						}
					}
				}
			}
		}
	}
	if trunks == 0 {
		t.Fatal("语料失效:扫描区内没有树干")
	}
}

// TestTreeColumnsRespectWorldHeightBounds 锁定世界高度上界语义:统一上界
// 守卫要求根加树高加冠顶两层落在有效范围内(即树干顶上两层仍在界内),
// 触界候选整体丢弃,因此任何实际生成的树冠顶都不触界;世界高度外的单点
// 查询一律为空,整块输出天然只含界内 Y。这里锁定的是连续性包络,单点与
// 整块一致由一致性测试锁定。
func TestTreeColumnsRespectWorldHeightBounds(t *testing.T) {
	const seed = int64(42)
	production := worldgen.New(seed, false)
	trees := 0
	for cx := int32(-2); cx < 2; cx++ {
		for cz := int32(-2); cz < 2; cz++ {
			chunk := production.GenerateChunk(core.ChunkPos{X: cx, Z: cz})
			for lx := 0; lx < core.SectionSize; lx++ {
				for lz := 0; lz < core.SectionSize; lz++ {
					wx := cx*core.SectionSize + int32(lx)
					wz := cz*core.SectionSize + int32(lz)
					surface := production.HeightAt(wx, wz)
					if chunk.BlockAt(lx, surface, lz) != core.GrassID {
						continue
					}
					if chunk.BlockAt(lx, surface+1, lz) != core.OakLogID {
						continue
					}
					trees++
					top := surface + 1
					for top+1 < core.MaxY && chunk.BlockAt(lx, top+1, lz) == core.OakLogID {
						top++
					}
					if top+2 >= core.MaxY {
						t.Fatalf("(%d,%d) 树冠顶 %d 触及世界高度上界 %d", wx, wz, top+2, core.MaxY)
					}
				}
			}
		}
	}
	if trees == 0 {
		t.Fatal("语料失效:扫描区内没有树干")
	}
	for wx := int32(-32); wx < 32; wx++ {
		for wz := int32(-32); wz < 32; wz++ {
			if got := production.BaseBlockAt(core.BlockPos{X: wx, Y: core.MaxY, Z: wz}); got != core.AirID {
				t.Fatalf("上界外 BaseBlockAt(%d,%d,%d)=%d,想要空气", wx, core.MaxY, wz, got)
			}
			if got := production.BaseBlockAt(core.BlockPos{X: wx, Y: core.MinY - 1, Z: wz}); got != core.AirID {
				t.Fatalf("下界外 BaseBlockAt(%d,%d,%d)=%d,想要空气", wx, core.MinY-1, wz, got)
			}
		}
	}
}

// branchLogDirections 收集顶下第 4/5 层、轴向 1..3 格内的原木格,按水平方向
// 归组并记录各方向的顺轴距离。方向键是轴向符号 (dx 与 dz 各取 -1/0/1),
// 普通树该映射为空(分杈是珍异专属),珍异树含 1..2 个方向。判定只用生产
// 公共出口,不复制哈希位模式。
func branchLogDirections(production *worldgen.Generator, rootX, rootZ, topY int32) map[[2]int32][]int32 {
	dirs := map[[2]int32][]int32{}
	for _, y := range []int32{topY - 4, topY - 5} {
		for dz := int32(-3); dz <= 3; dz++ {
			for dx := int32(-3); dx <= 3; dx++ {
				if dx == 0 && dz == 0 {
					continue
				}
				if dx != 0 && dz != 0 {
					continue
				}
				along := absInt32(dx) + absInt32(dz)
				if along < 1 || along > 3 {
					continue
				}
				if production.BaseBlockAt(core.BlockPos{X: rootX + dx, Y: y, Z: rootZ + dz}) != core.OakLogID {
					continue
				}
				key := [2]int32{0, 0}
				if dx > 0 {
					key[0] = 1
				} else if dx < 0 {
					key[0] = -1
				}
				if dz > 0 {
					key[1] = 1
				} else if dz < 0 {
					key[1] = -1
				}
				dirs[key] = append(dirs[key], along)
			}
		}
	}
	return dirs
}

// TestOrdinaryOakHasNoBranches 锁定普通树不带分杈:三棵不同位置的普通语料
// (含负坐标与跨界树)在顶下两层、轴向 1..3 格内除树干外不得出现原木。
// 语料高度经生产输出核实为普通档 5..7。
func TestOrdinaryOakHasNoBranches(t *testing.T) {
	production := worldgen.New(42, false)
	for _, root := range [][2]int32{{-4, -4}, {16, 18}, {-32, -4}} {
		rootX, rootZ := root[0], root[1]
		surface := production.HeightAt(rootX, rootZ)
		if got := production.TerrainBlockAt(core.BlockPos{X: rootX, Y: surface, Z: rootZ}); got != core.GrassID {
			t.Fatalf("语料前提失效: (%d,%d) 地表=%d，想要 GrassID", rootX, rootZ, got)
		}
		topY := surface
		for topY+1 < core.MaxY && production.BaseBlockAt(core.BlockPos{X: rootX, Y: topY + 1, Z: rootZ}) == core.OakLogID {
			topY++
		}
		if height := topY - surface; height < 5 || height > 7 {
			t.Fatalf("语料前提失效: (%d,%d) 树高 %d，想要普通档 5..7", rootX, rootZ, height)
		}
		if dirs := branchLogDirections(production, rootX, rootZ, topY); len(dirs) != 0 {
			t.Fatalf("(%d,%d) 普通树出现分杈方向 %v", rootX, rootZ, dirs)
		}
	}
}

// TestOakHeightAndCrownTiersAreDiverse 锁定多样性契约:固定种子下 chunk
// [-2,2)² 内生长在草地上的全部树干列(确定性扫描顺序)中,普通树高 5、6、7
// 格均出现,标准与蓬松冠形均出现,珍异大树小概率出现(至少一棵且少于普通
// 树)。另对首棵标准树与首棵蓬松树做完整冠形几何断言,几何断言走生产单点
// 出口(整块与单点一致由一致性测试锁定)。
func TestOakHeightAndCrownTiersAreDiverse(t *testing.T) {
	const seed = int64(42)
	production := worldgen.New(seed, false)
	heights := map[int32]int{}
	standard, fluffy, rare := 0, 0, 0
	var firstStandard, firstFluffy [3]int32
	seenStandard, seenFluffy := false, false
	for cx := int32(-2); cx < 2; cx++ {
		for cz := int32(-2); cz < 2; cz++ {
			chunk := production.GenerateChunk(core.ChunkPos{X: cx, Z: cz})
			for lx := 0; lx < core.SectionSize; lx++ {
				for lz := 0; lz < core.SectionSize; lz++ {
					wx := cx*core.SectionSize + int32(lx)
					wz := cz*core.SectionSize + int32(lz)
					surface := production.HeightAt(wx, wz)
					if chunk.BlockAt(lx, surface, lz) != core.GrassID {
						continue
					}
					if chunk.BlockAt(lx, surface+1, lz) != core.OakLogID {
						continue
					}
					top := surface + 1
					for top+1 < core.MaxY && chunk.BlockAt(lx, top+1, lz) == core.OakLogID {
						top++
					}
					height := top - surface
					heights[height]++
					if height >= 8 {
						if height > 12 {
							t.Fatalf("(%d,%d) 树高 %d 越出珍异档 8..12", wx, wz, height)
						}
						rare++
						continue
					}
					if height < 5 || height > 7 {
						t.Fatalf("(%d,%d) 树高 %d 越出普通档 5..7", wx, wz, height)
					}
					if chunk.BlockAt(lx, top+2, lz) == core.LeavesID {
						fluffy++
						if !seenFluffy {
							firstFluffy = [3]int32{wx, top, wz}
							seenFluffy = true
						}
					} else {
						standard++
						if !seenStandard {
							firstStandard = [3]int32{wx, top, wz}
							seenStandard = true
						}
					}
				}
			}
		}
	}
	for _, height := range []int32{5, 6, 7} {
		if heights[height] == 0 {
			t.Fatalf("普通树高 %d 格未出现,分布=%v", height, heights)
		}
	}
	if standard == 0 || fluffy == 0 {
		t.Fatalf("冠形两档必须都出现(标准=%d, 蓬松=%d)", standard, fluffy)
	}
	if rare == 0 || rare >= standard+fluffy {
		t.Fatalf("珍异大树必须小概率出现(珍异=%d, 普通=%d)", rare, standard+fluffy)
	}

	at := func(x, y, z int32) core.BlockID {
		return production.BaseBlockAt(core.BlockPos{X: x, Y: y, Z: z})
	}
	assertOrdinaryCrownLayers(t, at, firstStandard[0], firstStandard[1], firstStandard[2])
	assertStandardTopAir(t, at, firstStandard[0], firstStandard[1], firstStandard[2])
	assertOrdinaryCrownLayers(t, at, firstFluffy[0], firstFluffy[1], firstFluffy[2])
	assertFluffyTop(t, at, firstFluffy[0], firstFluffy[1], firstFluffy[2])
	if dirs := branchLogDirections(production, firstStandard[0], firstStandard[2], firstStandard[1]); len(dirs) != 0 {
		t.Fatalf("首棵标准树出现分杈方向 %v", dirs)
	}
	if dirs := branchLogDirections(production, firstFluffy[0], firstFluffy[2], firstFluffy[1]); len(dirs) != 0 {
		t.Fatalf("首棵蓬松树出现分杈方向 %v", dirs)
	}
}
