package server

import (
	"fmt"
	"log/slog"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

type Generator interface {
	GenerateChunk(core.DimensionID, core.ChunkPos) *world.Chunk
}

type TerrainProbe struct {
	generator *worldgen.Generator
	// dimension 是探针归属的维度,构造后不变;高度查询一律落到该维的高度图。
	dimension core.DimensionID
}

// NewTerrainProbe 创建只查询地表高度的探针。
//
// 固定传 fluidEnabled=false:探针只用 HeightAt,而高度图由地形噪声决定,
// 与海平面注水无关,传 false 可避免它依赖调用方的配置。
func NewTerrainProbe(seed int64) *TerrainProbe {
	return NewTerrainProbeForDimension(seed, core.Overworld)
}

// NewTerrainProbeForDimension 创建归属指定维度的高度探针:传送落点的新维
// 出生扫描用它读该维高度图,而不是主世界高度。
func NewTerrainProbeForDimension(seed int64, dim core.DimensionID) *TerrainProbe {
	return &TerrainProbe{generator: worldgen.NewForDimension(seed, false, dim), dimension: dim}
}

func (probe *TerrainProbe) HeightAt(x, z int32) int32 {
	return probe.generator.HeightAt(probe.dimension, x, z)
}

func runGeneration(
	generator Generator,
	key core.ChunkKey,
) (result contract.GeneratedChunk) {
	result.Dimension = key.Dimension
	result.Pos = key.Pos
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Chunk = nil
			result.Err = fmt.Errorf(
				"generator panic at dimension=%d chunk=(%d,%d): %v",
				key.Dimension,
				key.Pos.X,
				key.Pos.Z,
				recovered,
			)
			slog.Error(
				"区块生成 worker panic 已隔离",
				"dimension",
				key.Dimension,
				"chunk_x",
				key.Pos.X,
				"chunk_z",
				key.Pos.Z,
				"panic",
				recovered,
			)
		}
	}()
	result.Chunk = generator.GenerateChunk(key.Dimension, key.Pos)
	if result.Chunk == nil {
		result.Err = fmt.Errorf(
			"generator returned nil at dimension=%d chunk=(%d,%d)",
			key.Dimension,
			key.Pos.X,
			key.Pos.Z,
		)
	}
	return result
}
