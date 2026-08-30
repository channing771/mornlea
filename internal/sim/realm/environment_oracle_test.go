package realm

// environment_oracle_test.go：流体重扫的冻结 Go oracle（仅测试）。
//
// 本文件是旧生产实现的逐字迁移：`enqueueChunkFluids` 及其两个不动点判据
// （`fluidSourceIsFixedPoint`/`fluidSectionIsFixedPoint`）是重扫扫描语义的
// 冻结判定面，生产路径已改经 `fluid.ScanRescanRegion` 送入 Rust engine
// kernel。oracle 保留在此供 `rescan_differential_test.go` 做逐位差分与
// 续扫游标核对；`fluid.Replaceable` 的调用因此只存在于测试侧，生产代码对它
// 的调用清零（本体保留在 `internal/fluid/rules.go`）。

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/fluid"
	"github.com/channing771/mornlea/internal/world"
)

func fluidRescanBlockAt(dimension *Dimension, position core.BlockPos) core.BlockID {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return core.BarrierID
	}
	chunk, ready := dimension.ReadyChunk(position.Chunk())
	if !ready {
		return core.BarrierID
	}
	x, _, z := position.Local()
	return chunk.BlockAt(x, position.Y, z)
}

var fluidSealedSourceOffsets = [5][3]int32{
	{0, -1, 0},
	{1, 0, 0},
	{-1, 0, 0},
	{0, 0, 1},
	{0, 0, -1},
}

func fluidSourceIsFixedPoint(
	dimension *Dimension,
	section *world.Section,
	localX, localY, localZ int,
	position core.BlockPos,
) bool {
	for _, offset := range fluidSealedSourceOffsets {
		nx, ny, nz := localX+int(offset[0]), localY+int(offset[1]), localZ+int(offset[2])
		var neighbor core.BlockID
		if uint(nx) < core.SectionSize && uint(ny) < core.SectionSize && uint(nz) < core.SectionSize {
			neighbor = section.Blocks.Get(nx, ny, nz)
		} else {
			neighbor = fluidRescanBlockAt(dimension, core.BlockPos{
				X: position.X + offset[0],
				Y: position.Y + offset[1],
				Z: position.Z + offset[2],
			})
		}
		if fluid.Replaceable(neighbor, 1) {
			return false
		}
	}
	return true
}

func fluidSectionUnreplaceable(dimension *Dimension, pos core.ChunkPos, sectionIndex int) bool {
	if sectionIndex < 0 || sectionIndex >= core.SectionsPerChunk {
		return true
	}
	chunk, ready := dimension.ReadyChunk(pos)
	if !ready {
		return true
	}
	id, uniform := chunk.Section(sectionIndex).Blocks.IsUniform()
	return uniform && !fluid.Replaceable(id, 1)
}

func fluidSectionIsFixedPoint(dimension *Dimension, pos core.ChunkPos, sectionIndex int) bool {
	if !fluidSectionUnreplaceable(dimension, pos, sectionIndex-1) {
		return false
	}
	for _, plane := range fluidBoundaryPlanes {
		neighborPos := core.ChunkPos{X: pos.X + plane.dx, Z: pos.Z + plane.dz}
		if !fluidSectionUnreplaceable(dimension, neighborPos, sectionIndex) {
			return false
		}
	}
	return true
}

func (state *State) enqueueChunkFluids(
	queue *fluid.Queue,
	dimension *Dimension,
	chunk *world.Chunk,
	pos core.ChunkPos,
	x0, x1, z0, z1 int,
	now, delay uint64,
	budget int,
	section_ *int,
) (spent int, done bool) {
	baseX := pos.X << core.SectionShift
	baseZ := pos.Z << core.SectionShift
	for ; *section_ < core.SectionsPerChunk; *section_++ {
		if spent >= budget {
			return spent, false
		}
		sectionIndex := *section_
		section := chunk.Section(sectionIndex)
		if id, uniform := section.Blocks.IsUniform(); uniform {
			if !core.IsFluid(id) {
				spent++
				continue
			}
			if id == core.WaterSourceID && fluidSectionIsFixedPoint(dimension, pos, sectionIndex) {
				spent++
				continue
			}
		}
		baseY := int32(sectionIndex<<core.SectionShift) + core.MinY
		for localY := range core.SectionSize {
			for localZ := z0; localZ <= z1; localZ++ {
				for localX := x0; localX <= x1; localX++ {
					spent++
					id := section.Blocks.Get(localX, localY, localZ)
					if !core.IsFluid(id) {
						continue
					}
					position := core.BlockPos{
						X: baseX + int32(localX),
						Y: baseY + int32(localY),
						Z: baseZ + int32(localZ),
					}
					if id == core.WaterSourceID && fluidSourceIsFixedPoint(
						dimension, section, localX, localY, localZ, position,
					) {
						continue
					}
					queue.Enqueue(position, now, delay)
				}
			}
		}
	}
	*section_ = 0
	return spent, true
}
