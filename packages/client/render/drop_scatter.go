package render

import (
	"cmp"
	"math"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	dropScatterMargin      = float32(0.05)
	dropScatterWidth       = float32(0.9)
	dropScatterJitterRatio = float32(0.04)
	dropScatterLayerSize   = 16
	dropScatterLayerGap    = float32(0.02)
	dropScatterFloorGap    = float32(0.02)
)

type dropScatterPlacement struct {
	x, z      float32
	scale     float32
	bob       float32
	layerRise float32
}

// compareDropScatterInputs 先按完整同格键、再按完整 `DropID` 排序，使分组与
// 输出顺序都不依赖调用者输入顺序。`BlockPos` 不含维度，因此维度单独比较。
func compareDropScatterInputs(left, right ItemDrop) int {
	if order := cmp.Compare(left.ID.Dimension, right.ID.Dimension); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Block.X, right.Block.X); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Block.Y, right.Block.Y); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Block.Z, right.Block.Z); order != 0 {
		return order
	}
	return left.ID.Compare(right.ID)
}

func sameDropScatterGroup(left, right ItemDrop) bool {
	return left.ID.Dimension == right.ID.Dimension && left.Block == right.Block
}

// dropScatterGridSize 用固定阈值实现 `ceil(sqrt(n))`；生产同格数量由权威
// 区块 32 槽约束，因此最多走 6×6 档且没有数据相关搜索。
func dropScatterGridSize(count int) int {
	switch {
	case count <= 1:
		return 1
	case count <= 4:
		return 2
	case count <= 9:
		return 3
	case count <= 16:
		return 4
	case count <= 25:
		return 5
	default:
		return 6
	}
}

// dropScatterPlacementFor 把稳定排序后的 rank 映射到唯一 XZ cell。抖动每轴
// 取 `0.25J..J` 的非零幅度并使用独立盐；缩放恒为 1 以保证可辨认，`bob` 取全幅
// 浮动高度，层距按全尺寸薄片高度与两倍浮动幅度自然变大；XZ 不钳制，密集档允
// 许边缘探出以保留中心可辨距离。
func dropScatterPlacementFor(count, rank int, id core.DropID) dropScatterPlacement {
	grid := dropScatterGridSize(count)
	cell := dropScatterWidth / float32(grid)
	jitterLimit := dropScatterJitterRatio * cell
	scale := float32(1)
	bob := dropFloatHeight
	layerStep := dropFlakeSize + 2*bob + dropScatterLayerGap
	column, row := rank%grid, rank/grid
	return dropScatterPlacement{
		x:         dropScatterMargin + (float32(column)+0.5)*cell + dropScatterJitter(id, 0x9e3779b9, jitterLimit),
		z:         dropScatterMargin + (float32(row)+0.5)*cell + dropScatterJitter(id, 0x85ebca6b, jitterLimit),
		scale:     scale,
		bob:       bob,
		layerRise: float32(rank/dropScatterLayerSize) * layerStep,
	}
}

func dropScatterJitter(id core.DropID, salt uint32, limit float32) float32 {
	hash := uint32(id.Dimension)*0x9e3779b1 ^ uint32(id.Chunk.X)*0x85ebca77 ^
		uint32(id.Chunk.Z)*0xc2b2ae3d ^ uint32(id.Slot)*0x27d4eb2f ^
		id.Generation*0x165667b1 ^ salt
	hash ^= hash >> 16
	hash *= 0x7feb352d
	hash ^= hash >> 15
	hash *= 0x846ca68b
	hash ^= hash >> 16
	magnitude := (0.25 + 0.75*float32(hash&0x7fff)/0x7fff) * limit
	if hash&0x8000 != 0 {
		return -magnitude
	}
	return magnitude
}

func dropScatterBaseY(drop ItemDrop, age uint64, gravity, terminal float32) float32 {
	base := float32(drop.Block.Y) - dropFallOffset(
		age, float32(drop.Block.Y), drop.SupportY, drop.HasSupport, gravity, terminal,
	)
	if drop.HasSupport {
		base = max(base, drop.SupportY)
	}
	return base
}

func dropScatterCenterY(base, halfHeight, layerRise, bob, phase float32) float32 {
	return base + layerRise + halfHeight + dropScatterFloorGap + bob +
		bob*float32(math.Sin(float64(phase)))
}
