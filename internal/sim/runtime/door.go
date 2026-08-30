package runtime

import (
	"errors"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// doorLowerID 返回对应方向与开合态的下半门 ID。
func doorLowerID(dir int, open bool) core.BlockID {
	switch dir {
	case 0:
		if open {
			return core.DoorLowerSouthOpen
		}
		return core.DoorLowerSouthClosed
	case 1:
		if open {
			return core.DoorLowerWestOpen
		}
		return core.DoorLowerWestClosed
	case 2:
		if open {
			return core.DoorLowerNorthOpen
		}
		return core.DoorLowerNorthClosed
	case 3:
		if open {
			return core.DoorLowerEastOpen
		}
		return core.DoorLowerEastClosed
	default:
		return core.AirID
	}
}

// yawToDoorDir 把 yaw 映射到门的四向编码：南0、西1、北2、东3。
func yawToDoorDir(yaw float32) int {
	normalized := math.Mod(float64(yaw)+math.Pi, 2*math.Pi)
	if normalized < 0 {
		normalized += 2 * math.Pi
	}
	yawNorm := float32(normalized - math.Pi)
	// South: [-45°,45°), West: [45°,135°), North: [135°,180°)∪(-180°,-135°], East: [-135°,-45°)
	if yawNorm >= -math.Pi/4 && yawNorm < math.Pi/4 {
		return 0
	}
	if yawNorm >= math.Pi/4 && yawNorm < 3*math.Pi/4 {
		return 1
	}
	if yawNorm >= 3*math.Pi/4 || yawNorm < -3*math.Pi/4 {
		return 2
	}
	return 3
}

// isSolidSupport 报告方块是否可作为门的下方支撑（复用 Opaque 权威语义）。
func isSolidSupport(id core.BlockID) bool {
	// Farmland 干/湿均为实心支撑（与 assets.Registry.Opaque 一致，Opaque 对耕地为 true）。
	// sim 不直接依赖 assets 以避免循环，此处显式复用 Opaque 判定式并以 Farmland 正例断言守护。
	return core.IsFarmland(id) || (core.RegisteredBlock(id) && id != core.AirID && id != core.GlassID && id != core.LeavesID && !core.IsFluid(id) && !core.IsCrop(id) && !core.IsDoor(id))
}

// tryPlaceDoor 尝试在 lower 放置下半门并在 upper 放置上半。
// 校验 lower/upper 可替换（空气）且下方实心，跨区块未就绪拒绝，原子双格写入。
func (engine *Engine) tryPlaceDoor(dimensionID core.DimensionID, lower core.BlockPos, dir int, pending *pendingChunkChanges) (RejectReason, bool) {
	if dir < 0 || dir > 3 {
		return RejectInvalidBlock, true
	}
	upper := core.BlockPos{X: lower.X, Y: lower.Y + 1, Z: lower.Z}
	if lower.Y < core.MinY || lower.Y >= core.MaxY || upper.Y < core.MinY || upper.Y >= core.MaxY {
		return RejectChunkNotReady, true
	}
	dimension := engine.dimension(dimensionID)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	lowerBlock, lowerReady := dimension.BlockAt(lower)
	if !lowerReady {
		return RejectChunkNotReady, true
	}
	upperBlock, upperReady := dimension.BlockAt(upper)
	if !upperReady {
		return RejectChunkNotReady, true
	}
	// 可替换：严格要求空气（与 spec 上空语义一致），流体视为占用
	if lowerBlock != core.AirID || upperBlock != core.AirID {
		return RejectOccupied, true
	}
	below := core.BlockPos{X: lower.X, Y: lower.Y - 1, Z: lower.Z}
	belowBlock, belowReady := dimension.BlockAt(below)
	if !belowReady {
		return RejectChunkNotReady, true
	}
	if !isSolidSupport(belowBlock) {
		return RejectInvalidBlock, true
	}
	// 原子双格写入
	lowerID := doorLowerID(dir, false)
	upperID := core.DoorUpper
	oldLower, _, errLower := dimension.SetBlock(lower, lowerID)
	if errLower != nil {
		return mapSetBlockError(errLower), true
	}
	oldUpper, _, errUpper := dimension.SetBlock(upper, upperID)
	if errUpper != nil {
		// 回滚下半
		_, _, _ = dimension.SetBlock(lower, oldLower)
		return mapSetBlockError(errUpper), true
	}
	// 两个半都可能在同一区块，recordChange 分别汇入 pending（同一 key 会合并）
	engine.recordChange(dimensionID, lower, lowerID, pending)
	engine.recordChange(dimensionID, upper, upperID, pending)
	_ = oldLower
	_ = oldUpper
	return 0, false
}

// handleInteractDoor 处理对门方块的右键交互，上下联动切换 Closed<->Open。
func handleInteractDoor(engine *Engine, dimensionID core.DimensionID, pos core.BlockPos, pending *pendingChunkChanges) bool {
	dimension := engine.dimension(dimensionID)
	if dimension == nil {
		return false
	}
	block, ready := dimension.BlockAt(pos)
	if !ready {
		return false
	}
	var lowerPos core.BlockPos
	switch {
	case core.IsDoorLower(block):
		lowerPos = pos
	case core.IsDoorUpper(block):
		lowerPos = core.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z}
	default:
		return false
	}
	lowerID, lowerReady := dimension.BlockAt(lowerPos)
	if !lowerReady || !core.IsDoorLower(lowerID) {
		return false
	}
	upperPos := core.BlockPos{X: lowerPos.X, Y: lowerPos.Y + 1, Z: lowerPos.Z}
	upperID, upperReady := dimension.BlockAt(upperPos)
	if !upperReady || !core.IsDoorUpper(upperID) {
		return false
	}
	dir := core.DoorDir(lowerID)
	if dir < 0 {
		return false
	}
	open := core.IsDoorOpen(lowerID)
	newLower := doorLowerID(dir, !open)
	_, _, err := dimension.SetBlock(lowerPos, newLower)
	if err != nil {
		return false
	}
	engine.recordChange(dimensionID, lowerPos, newLower, pending)
	// upper 保持 DoorUpper，不改，但逻辑关联已通过 lower 翻转体现
	return true
}

// executeInteractDoor 通过权威射线定位目标并分发到 handleInteractDoor。
func (engine *Engine) executeInteractDoor(command Command, pending *pendingChunkChanges) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimensionID := session.dimension
	dimension := engine.dimension(dimensionID)
	if dimension == nil || !session.hasView {
		return RejectInvalidRay, true
	}
	origin := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(command.Yaw, command.Pitch)
	hit, ok, err := core.RaycastBlocks(origin, direction, engine.tunables.InteractionReach, blockRaycastSampler(dimension))
	if err != nil {
		if errors.Is(err, ErrChunkNotReady) {
			return RejectChunkNotReady, true
		}
		return RejectInvalidRay, true
	}
	if !ok {
		return RejectNoTarget, true
	}
	block, ready := dimension.BlockAt(hit.Block)
	if !ready {
		return RejectChunkNotReady, true
	}
	if !core.IsDoor(block) {
		return 0, false
	}
	if !handleInteractDoor(engine, dimensionID, hit.Block, pending) {
		return RejectNoTarget, true
	}
	return 0, false
}
