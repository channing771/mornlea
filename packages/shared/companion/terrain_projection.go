package companion

import (
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// TerrainWidth、TerrainHeight 与 TerrainDepth 是规划冻结投影的固定尺寸。
	TerrainWidth  = 33
	TerrainHeight = 17
	TerrainDepth  = 33
	// TerrainHorizontalRadius 与 TerrainVerticalRadius 是相对伙伴 floor 格的闭区间半径。
	TerrainHorizontalRadius = (TerrainWidth - 1) / 2
	TerrainVerticalRadius   = (TerrainHeight - 1) / 2

	// TerrainColumnCount 是 `(x,z)` ready/height 平面的固定列数。
	TerrainColumnCount = TerrainWidth * TerrainDepth
	// TerrainBlockCount 是 `(x,y,z)` 方块平面的固定槽数。
	TerrainBlockCount = TerrainWidth * TerrainHeight * TerrainDepth
	// TerrainReadyBitmapBytes 是 1,089-bit ready bitmap 的固定字节数。
	TerrainReadyBitmapBytes = (TerrainColumnCount + 7) / 8
	// TerrainDataPlaneBytes 是 ready、height 与 block 三个紧凑平面的逐字节总量。
	TerrainDataPlaneBytes = TerrainReadyBitmapBytes + TerrainColumnCount*2 + TerrainBlockCount*2
)

// TerrainProjection 是一次规划在 tick 边界冻结的固定 terrain data plane。
// 数组值语义使普通赋值就是完整深拷贝；发送给 worker 或 registry 后不得再调用
// 构造期 mutator。
type TerrainProjection struct {
	origin       core.BlockPos
	readyColumns [TerrainReadyBitmapBytes]byte
	heights      [TerrainColumnCount]int16
	blocks       [TerrainBlockCount]core.BlockID
}

// NewTerrainProjection 创建全列未 ready 的规范投影。未 ready height 固定为
// `core.MinY-1`，方块平面保持规范空气值。
func NewTerrainProjection(origin core.BlockPos) TerrainProjection {
	projection := TerrainProjection{origin: origin}
	for index := range projection.heights {
		projection.heights[index] = int16(core.MinY - 1)
	}
	return projection
}

// Origin 返回固定投影原点。
func (p TerrainProjection) Origin() core.BlockPos { return p.origin }

// SetReadyColumn 在构造期把投影内一列标记为 ready 并写入冻结 height。height 可
// 为合法世界方块 Y，或以 `core.MinY-1` 表示 ready 空列。
func (p *TerrainProjection) SetReadyColumn(x, z, height int32) bool {
	index, ok := p.columnIndex(x, z)
	if !ok || (height != core.MinY-1 && (height < core.MinY || height >= core.MaxY)) {
		return false
	}
	p.readyColumns[index/8] |= 1 << (index % 8)
	p.heights[index] = int16(height)
	return true
}

// SetBlock 在构造期写入 ready 列中 world-valid 投影槽的精确方块编号。
func (p *TerrainProjection) SetBlock(pos core.BlockPos, block core.BlockID) bool {
	blockIndex, ok := p.blockIndex(pos)
	if !ok || pos.Y < core.MinY || pos.Y >= core.MaxY || !core.RegisteredBlock(block) {
		return false
	}
	columnIndex, _ := p.columnIndex(pos.X, pos.Z)
	if !p.readyAt(columnIndex) {
		return false
	}
	p.blocks[blockIndex] = block
	return true
}

// ColumnReady 报告世界 `(x,z)` 是否落在投影内且构造时所在区块 ready。
func (p TerrainProjection) ColumnReady(x, z int32) bool {
	index, ok := p.columnIndex(x, z)
	return ok && p.readyAt(index)
}

// Lookup 返回 ready、world-valid 投影位置的冻结方块与列高。范围外或未 ready
// 统一 fail closed，不把规范化为空气的不可观察槽暴露给调用方。
func (p TerrainProjection) Lookup(pos core.BlockPos) (core.BlockID, int32, bool) {
	if pos.Y < core.MinY || pos.Y >= core.MaxY {
		return core.AirID, 0, false
	}
	blockIndex, ok := p.blockIndex(pos)
	if !ok {
		return core.AirID, 0, false
	}
	columnIndex, _ := p.columnIndex(pos.X, pos.Z)
	if !p.readyAt(columnIndex) {
		return core.AirID, 0, false
	}
	return p.blocks[blockIndex], int32(p.heights[columnIndex]), true
}

// Heights 返回全部 ready 列按 `(x,z)` 字典序排列的 legacy 摘要。该摘要只供
// 既有 Planner/Dialogue 接口；专用 digest DTO 不会再次编码它。
func (p TerrainProjection) Heights() []PlanHeight {
	heights := make([]PlanHeight, 0, TerrainColumnCount)
	for xOffset := 0; xOffset < TerrainWidth; xOffset++ {
		for zOffset := 0; zOffset < TerrainDepth; zOffset++ {
			index := xOffset*TerrainDepth + zOffset
			if !p.readyAt(index) {
				continue
			}
			heights = append(heights, PlanHeight{
				X:      p.origin.X + int32(xOffset),
				Z:      p.origin.Z + int32(zOffset),
				Height: int32(p.heights[index]),
			})
		}
	}
	return heights
}

// ExposedBlocks 只从冻结 data plane 派生按 `(x,y,z)` 排序的暴露方块。投影外
// 但仍在世界内的邻居及未 ready 列都视为 unknown/non-air；只有世界垂直边界
// 外视为空气。结果在遍历中已是字典序，达到固定 cap 后即可停止。
func (p TerrainProjection) ExposedBlocks() []PlanBlock {
	exposed := make([]PlanBlock, 0, MaxPlanExposedBlocks)
	for xOffset := 0; xOffset < TerrainWidth; xOffset++ {
		for yOffset := 0; yOffset < TerrainHeight; yOffset++ {
			for zOffset := 0; zOffset < TerrainDepth; zOffset++ {
				pos := core.BlockPos{
					X: p.origin.X + int32(xOffset),
					Y: p.origin.Y + int32(yOffset),
					Z: p.origin.Z + int32(zOffset),
				}
				block, _, ok := p.Lookup(pos)
				if !ok || block == core.AirID || !p.hasFrozenAirNeighbor(pos) {
					continue
				}
				exposed = append(exposed, PlanBlock{Pos: pos, Block: block})
				if len(exposed) == MaxPlanExposedBlocks {
					return exposed
				}
			}
		}
	}
	return exposed
}

// Validate 校验 data plane 的规范化、范围、编号与 unused-bit 不变量。
func (p TerrainProjection) Validate() error {
	return p.validateWithCheckpoint(nil)
}

func (p TerrainProjection) validateWithCheckpoint(checkpoint func() error) error {
	if checkpoint != nil {
		if err := checkpoint(); err != nil {
			return err
		}
	}
	if !projectionCoordinateRangeValid(p.origin) {
		return fmt.Errorf("companion: terrain projection 原点会导致坐标溢出")
	}
	if p.readyColumns[TerrainReadyBitmapBytes-1]&0xfe != 0 {
		return fmt.Errorf("companion: terrain projection ready bitmap unused bits 非零")
	}
	for column := 0; column < TerrainColumnCount; column++ {
		if checkpoint != nil {
			if err := checkpoint(); err != nil {
				return err
			}
		}
		height := int32(p.heights[column])
		ready := p.readyAt(column)
		if !ready && height != core.MinY-1 {
			return fmt.Errorf("companion: terrain projection 未 ready 列 %d height 未规范化", column)
		}
		if ready && height != core.MinY-1 && (height < core.MinY || height >= core.MaxY) {
			return fmt.Errorf("companion: terrain projection ready 列 %d height=%d 越界", column, height)
		}
	}
	for index, block := range p.blocks {
		if checkpoint != nil {
			if err := checkpoint(); err != nil {
				return err
			}
		}
		xOffset, yOffset, zOffset := terrainOffsets(index)
		column := xOffset*TerrainDepth + zOffset
		y := p.origin.Y + int32(yOffset)
		observable := p.readyAt(column) && y >= core.MinY && y < core.MaxY
		if !observable && block != core.AirID {
			return fmt.Errorf("companion: terrain projection 不可观察槽 %d 未规范化为空气", index)
		}
		if observable && !core.RegisteredBlock(block) {
			return fmt.Errorf("companion: terrain projection 槽 %d 方块 %d 未注册", index, block)
		}
	}
	if checkpoint != nil {
		return checkpoint()
	}
	return nil
}

func (p TerrainProjection) hasFrozenAirNeighbor(pos core.BlockPos) bool {
	neighbors := [...]core.BlockPos{
		{X: pos.X - 1, Y: pos.Y, Z: pos.Z},
		{X: pos.X + 1, Y: pos.Y, Z: pos.Z},
		{X: pos.X, Y: pos.Y - 1, Z: pos.Z},
		{X: pos.X, Y: pos.Y + 1, Z: pos.Z},
		{X: pos.X, Y: pos.Y, Z: pos.Z - 1},
		{X: pos.X, Y: pos.Y, Z: pos.Z + 1},
	}
	for _, neighbor := range neighbors {
		if neighbor.Y < core.MinY || neighbor.Y >= core.MaxY {
			return true
		}
		block, _, ok := p.Lookup(neighbor)
		if ok && block == core.AirID {
			return true
		}
	}
	return false
}

func (p TerrainProjection) columnIndex(x, z int32) (int, bool) {
	dx := int64(x) - int64(p.origin.X)
	dz := int64(z) - int64(p.origin.Z)
	if dx < 0 || dx >= TerrainWidth || dz < 0 || dz >= TerrainDepth {
		return 0, false
	}
	return int(dx)*TerrainDepth + int(dz), true
}

func (p TerrainProjection) blockIndex(pos core.BlockPos) (int, bool) {
	column, ok := p.columnIndex(pos.X, pos.Z)
	if !ok {
		return 0, false
	}
	dx := column / TerrainDepth
	dz := column % TerrainDepth
	dy := int64(pos.Y) - int64(p.origin.Y)
	if dy < 0 || dy >= TerrainHeight {
		return 0, false
	}
	return (dx*TerrainHeight+int(dy))*TerrainDepth + dz, true
}

func (p TerrainProjection) readyAt(index int) bool {
	return p.readyColumns[index/8]&(1<<(index%8)) != 0
}

func terrainOffsets(index int) (x, y, z int) {
	x = index / (TerrainHeight * TerrainDepth)
	remainder := index % (TerrainHeight * TerrainDepth)
	y = remainder / TerrainDepth
	z = remainder % TerrainDepth
	return x, y, z
}

func projectionCoordinateRangeValid(origin core.BlockPos) bool {
	return int64(origin.X)+TerrainWidth-1 <= int64(^uint32(0)>>1) &&
		int64(origin.Y)+TerrainHeight-1 <= int64(^uint32(0)>>1) &&
		int64(origin.Z)+TerrainDepth-1 <= int64(^uint32(0)>>1)
}
