package worldgen_test

// 本文件覆盖 natural-grass-generation 变更的「确定性自然短草」需求:生产
// Rust worldgen 在草地表面按世界坐标确定性散布单格短草,橡树与海水优先,
// 高度/地形/远环语义不变。全部断言走生产公共出口(`GenerateChunk`、
// `BaseBlockAt`、`HeightAt`、`TerrainBlockAt`、`lod.GenerateShell`),不在
// 测试侧复制分布算法。

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/lod"
	"github.com/channing771/mornlea/internal/worldgen"
)

// grassTestSeed 与黄金摘要同一语料种子,保证断言世界与 golden 基线同源。
const grassTestSeed int64 = 42

// grassTestChunks 覆盖正/负坐标与远端区块的语料。
var grassTestChunks = []core.ChunkPos{
	{X: 0, Z: 0}, {X: 1, Z: 0}, {X: -1, Z: -1}, {X: 37, Z: -104},
}

// scanShortGrass 扫描区块,统计:短草格数、其正下方为草地的格数、草地表面
// 目标格仍为空气的列数(空隙)。返回值:(短草数, 下方为草地的短草数, 空隙列数)。
func scanShortGrass(t *testing.T, generator *worldgen.Generator, pos core.ChunkPos) (int, int, int) {
	t.Helper()
	chunk := generator.GenerateChunk(pos)
	shortGrass := 0
	onGrass := 0
	gaps := 0
	for x := 0; x < core.SectionSize; x++ {
		for z := 0; z < core.SectionSize; z++ {
			for y := int32(core.MinY); y < core.MaxY; y++ {
				if chunk.BlockAt(x, y, z) != core.ShortGrassID {
					continue
				}
				shortGrass++
				if chunk.BlockAt(x, y-1, z) == core.GrassID {
					onGrass++
				}
				// 短草不向上生长:正上方不允许再叠短草(树叶等既有内容
				// 可以合法悬垂在短草上方,不构成叠加)。
				if above := chunk.BlockAt(x, y+1, z); above == core.ShortGrassID {
					t.Fatalf("chunk(%d,%d) (%d,%d,%d) 短草上方又叠短草",
						pos.X, pos.Z, x, y, z)
				}
			}
			// 草地表面目标格为空气的列计入空隙(证明短草是稀疏散布而非全铺)。
			height := generator.HeightAt(pos.X*core.SectionSize+int32(x), pos.Z*core.SectionSize+int32(z))
			if chunk.BlockAt(x, height, z) == core.GrassID && chunk.BlockAt(x, height+1, z) == core.AirID {
				gaps++
			}
		}
	}
	return shortGrass, onGrass, gaps
}

// TestShortGrassGrowsOnGrassSurfaceWithGaps 钉住散布的存在性与稀疏性:
// 语料中必须同时出现短草与空隙,且每株短草都长在 GrassID 表面正上方。
func TestShortGrassGrowsOnGrassSurfaceWithGaps(t *testing.T) {
	generator := worldgen.New(grassTestSeed, false)
	totalShortGrass := 0
	totalOnGrass := 0
	totalGaps := 0
	for _, pos := range grassTestChunks {
		shortGrass, onGrass, gaps := scanShortGrass(t, generator, pos)
		totalShortGrass += shortGrass
		totalOnGrass += onGrass
		totalGaps += gaps
	}
	if totalShortGrass == 0 {
		t.Fatal("语料没有任何短草，自然短草层未生成")
	}
	if totalOnGrass != totalShortGrass {
		t.Fatalf("短草 %d 株中 %d 株不在 GrassID 表面正上方", totalShortGrass, totalShortGrass-totalOnGrass)
	}
	if totalGaps == 0 {
		t.Fatal("语料没有空隙列，短草散布退化为全铺")
	}
}

// preUpgradeGoldenDigests 是升级前(自然短草引入前)以同一算法对 seed 42
// 四个区块记录的 SHA256 摘要——变更实现前从 testdata/golden_seed42.txt
// 逐字誊写冻结。归一化等价证明:把新输出中的 ShortGrassID 归一为 AirID
// 后重新计算摘要，必须与这些升级前摘要逐字节一致;这证明既有高度、地形
// 材料、矿石、橡树与海水方块没有被短草层改写(短草只写在此前的空气格)。
var preUpgradeGoldenDigests = map[core.ChunkPos]string{
	{X: 0, Z: 0}:     "758c980abb0e63edd71b187a461805763f40f816e154ab233cf70cf6c6212c2d",
	{X: 1, Z: 0}:     "49a4124a04cca24f9033a9569c6d390700204e908132e8342ab1b222e476727d",
	{X: -1, Z: -1}:   "52355bb97f2a04ded395f65f737f65d6c81177b3a25a748083d7add856630444",
	{X: 37, Z: -104}: "68532f6abf257f3d874a1bccbd39ee1a305546269820725a7070b2eb04a99576",
}

// TestShortGrassNormalizedOutputMatchesPreUpgradeDigests 是兼容性对照:
// 新输出把 ShortGrassID 归一为 AirID 后，与升级前冻结摘要逐字节相同。
func TestShortGrassNormalizedOutputMatchesPreUpgradeDigests(t *testing.T) {
	generator := worldgen.New(grassTestSeed, false)
	for _, pos := range grassTestChunks {
		chunk := generator.GenerateChunk(pos)
		digest := sha256.New()
		for y := int32(core.MinY); y < core.MaxY; y++ {
			for z := 0; z < core.SectionSize; z++ {
				for x := 0; x < core.SectionSize; x++ {
					id := chunk.BlockAt(x, y, z)
					if id == core.ShortGrassID {
						// 归一化:装饰短草在升级前的世界里是空气。
						id = core.AirID
					}
					_, _ = digest.Write([]byte{byte(id), byte(id >> 8)})
				}
			}
		}
		got := hex.EncodeToString(digest.Sum(nil))
		if got != preUpgradeGoldenDigests[pos] {
			t.Fatalf("chunk(%d,%d) 归一化摘要 %s != 升级前摘要 %s：既有方块被短草层改写",
				pos.X, pos.Z, got, preUpgradeGoldenDigests[pos])
		}
	}
}

// TestShortGrassWholeChunkAndSinglePointParity 锁定整块与单点两条生产出口
// 的逐格一致(含负坐标与区块边界列),并确认短草在两条出口同时可见。
func TestShortGrassWholeChunkAndSinglePointParity(t *testing.T) {
	generator := worldgen.New(grassTestSeed, false)
	seenShortGrass := 0
	for _, pos := range grassTestChunks {
		chunk := generator.GenerateChunk(pos)
		for y := int32(core.MinY); y < core.MaxY; y++ {
			for z := 0; z < core.SectionSize; z++ {
				for x := 0; x < core.SectionSize; x++ {
					position := core.BlockPos{
						X: pos.X*core.SectionSize + int32(x),
						Y: y,
						Z: pos.Z*core.SectionSize + int32(z),
					}
					if got := generator.BaseBlockAt(position); got != chunk.BlockAt(x, y, z) {
						t.Fatalf("chunk(%d,%d) %+v: BaseBlockAt=%d，GenerateChunk=%d",
							pos.X, pos.Z, position, got, chunk.BlockAt(x, y, z))
					}
					if chunk.BlockAt(x, y, z) == core.ShortGrassID {
						seenShortGrass++
					}
				}
			}
		}
	}
	if seenShortGrass == 0 {
		t.Fatal("语料没有任何短草，parity 断言没有覆盖装饰层")
	}
}

// TestHeightTerrainAndLodIgnoreShortGrass 钉住装饰语义:`HeightAt` 仍是
// 最高地形表面(短草不抬高)，`TerrainBlockAt` 在装饰格仍返回空气，装饰格
// 上方是空气；远环壳 quad 的材质里不出现短草编号(LOD 只表达地形表面)。
func TestHeightTerrainAndLodIgnoreShortGrass(t *testing.T) {
	generator := worldgen.New(grassTestSeed, false)
	checked := 0
	for _, pos := range grassTestChunks {
		chunk := generator.GenerateChunk(pos)
		for x := 0; x < core.SectionSize; x++ {
			for z := 0; z < core.SectionSize; z++ {
				wx := pos.X*core.SectionSize + int32(x)
				wz := pos.Z*core.SectionSize + int32(z)
				height := generator.HeightAt(wx, wz)
				if chunk.BlockAt(x, height, z) == core.ShortGrassID {
					t.Fatalf("chunk(%d,%d) (%d,%d) 高度图指向短草，HeightAt 不得被装饰抬高",
						pos.X, pos.Z, wx, wz)
				}
				if chunk.BlockAt(x, height+1, z) != core.ShortGrassID {
					continue
				}
				checked++
				if got := generator.TerrainBlockAt(core.BlockPos{X: wx, Y: height + 1, Z: wz}); got != core.AirID {
					t.Fatalf("TerrainBlockAt(%d,%d,%d)=%d，想要 AirID(地形语义忽略短草)",
						wx, height+1, wz, got)
				}
				// 短草不向上生长:装饰格上方允许有树叶等既有内容,但不得
				// 再出现短草。
				if got := generator.BaseBlockAt(core.BlockPos{X: wx, Y: height + 2, Z: wz}); got == core.ShortGrassID {
					t.Fatalf("短草上方 (%d,%d,%d) 又叠短草:短草只有单格",
						wx, height+2, wz)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("语料没有任何短草，忽略语义断言空转")
	}

	// 远环:生产 header 驱动壳生成,任何 quad 的材质都不允许是短草编号。
	shell, err := lod.GenerateShell(generator.Header(), core.ChunkPos{X: -3, Z: 2}, 4)
	if err != nil {
		t.Fatalf("生成远环壳失败: %v", err)
	}
	quads, err := lod.DecodeQuads(shell)
	if err != nil {
		t.Fatalf("解码远环壳失败: %v", err)
	}
	if len(quads) == 0 {
		t.Fatal("远环壳为空")
	}
	for _, quad := range quads {
		if quad.Material == uint16(core.ShortGrassID) {
			t.Fatalf("远环 quad 材质出现短草编号 %d：LOD 不得表达装饰短草", core.ShortGrassID)
		}
	}
}

// TestShortGrassFixedHitAndMissSamples 钉住固定 1/4 样本:两个事先核实后
// 冻结的负坐标草地列,命中列必有短草、相邻未命中列必须保持空气。任何对
// salt、坐标语义或判定阈值的改动都会让这对样本翻转。
func TestShortGrassFixedHitAndMissSamples(t *testing.T) {
	generator := worldgen.New(grassTestSeed, false)
	// 固定样本(经生产黑盒出口核实后冻结,均位于负坐标区块):
	// 命中列 (-32, 64, -32) 的 surface+1 是短草;未命中列 (-32, 64, -31)
	// 的 surface+1 是空气。两列地表同为 GrassID,差异只能来自判定本身。
	samples := []struct {
		wx, wz      int32
		wantAt      core.BlockID
		description string
	}{
		{wx: -32, wz: -32, wantAt: core.ShortGrassID, description: "命中列"},
		{wx: -32, wz: -31, wantAt: core.AirID, description: "未命中列"},
	}
	for _, sample := range samples {
		height := generator.HeightAt(sample.wx, sample.wz)
		if got := generator.BaseBlockAt(core.BlockPos{X: sample.wx, Y: height, Z: sample.wz}); got != core.GrassID {
			t.Fatalf("%s (%d,%d) 地表=%d，想要 GrassID(样本前提失效)",
				sample.description, sample.wx, sample.wz, got)
		}
		got := generator.BaseBlockAt(core.BlockPos{X: sample.wx, Y: height + 1, Z: sample.wz})
		if got != sample.wantAt {
			t.Fatalf("%s (%d,%d) surface+1=%d，想要 %d",
				sample.description, sample.wx, sample.wz, got, sample.wantAt)
		}
		// 整块出口与单点出口对同一样本必须一致。
		chunk := generator.GenerateChunk(core.ChunkPos{
			X: sample.wx >> core.SectionShift, Z: sample.wz >> core.SectionShift,
		})
		if block := chunk.BlockAt(int(sample.wx&core.SectionMask), height+1, int(sample.wz&core.SectionMask)); block != got {
			t.Fatalf("%s (%d,%d) 整块=%d 单点=%d",
				sample.description, sample.wx, sample.wz, block, got)
		}
	}
}

// TestShortGrassKeepsTreeAndWaterPriority 锁定既有内容优先:短草格下方
// 必须是草地(树干列、海面与任何非空气目标格都不长草);湿世界(注水开启)
// 里海平面及以下不出现短草——那些格已被水占据。
func TestShortGrassKeepsTreeAndWaterPriority(t *testing.T) {
	const seaLevelY int32 = 64
	dry := worldgen.New(grassTestSeed, false)
	wet := worldgen.New(grassTestSeed, true)
	for _, pos := range grassTestChunks {
		dryChunk := dry.GenerateChunk(pos)
		wetChunk := wet.GenerateChunk(pos)
		for y := int32(core.MinY); y < core.MaxY; y++ {
			for z := 0; z < core.SectionSize; z++ {
				for x := 0; x < core.SectionSize; x++ {
					if dryChunk.BlockAt(x, y, z) == core.ShortGrassID {
						if below := dryChunk.BlockAt(x, y-1, z); below != core.GrassID {
							t.Fatalf("chunk(%d,%d) (%d,%d,%d) 短草下方是 %d，想要 GrassID",
								pos.X, pos.Z, x, y, z, below)
						}
					}
					if y <= seaLevelY && wetChunk.BlockAt(x, y, z) == core.ShortGrassID {
						t.Fatalf("chunk(%d,%d) (%d,%d,%d) 湿世界海平面及以下出现短草，海水优先被破坏",
							pos.X, pos.Z, x, y, z)
					}
					// 海平面以上:注水不改写任何格,两态短草必须一致。
					if y > seaLevelY {
						if dryChunk.BlockAt(x, y, z) != wetChunk.BlockAt(x, y, z) {
							t.Fatalf("chunk(%d,%d) (%d,%d,%d) 注水改写了海平面以上的格: dry=%d wet=%d",
								pos.X, pos.Z, x, y, z, dryChunk.BlockAt(x, y, z), wetChunk.BlockAt(x, y, z))
						}
					}
				}
			}
		}
	}
}
