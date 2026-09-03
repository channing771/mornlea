package lod

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// 本文件钉住 worldgen 装饰短草与远环壳的交界语义:短草只存在于近环区块,
// 远环壳 quad 材质不得出现短草编号。断言主体是 LOD 的材质边界(worldgen
// 只提供语料),因此与 lod 同目录放置;测试入口名与迁出前的 worldgen
// 外部测试保持一致,`-run` 过滤器继续兼容。

// grassShellTestSeed 与 worldgen 短草测试同源语料种子。
const grassShellTestSeed int64 = 42

// TestHeightTerrainAndLodIgnoreShortGrass 钉住装饰语义:`HeightAt` 仍是
// 最高地形表面(短草不抬高)，`TerrainBlockAt` 在装饰格仍返回空气，装饰格
// 上方是空气；远环壳 quad 的材质里不出现短草编号(LOD 只表达地形表面)。
func TestHeightTerrainAndLodIgnoreShortGrass(t *testing.T) {
	generator := worldgen.New(grassShellTestSeed, false)
	checked := 0
	for _, pos := range []core.ChunkPos{
		{X: 0, Z: 0}, {X: 1, Z: 0}, {X: -1, Z: -1}, {X: 37, Z: -104},
	} {
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
	shell, err := GenerateShell(generator.Header(), core.ChunkPos{X: -3, Z: 2}, 4)
	if err != nil {
		t.Fatalf("生成远环壳失败: %v", err)
	}
	quads, err := DecodeQuads(shell)
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
