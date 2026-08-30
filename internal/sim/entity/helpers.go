package entity

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
)

func finiteInputComponent(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func normalizeYaw(yaw float32) float32 {
	for yaw > math.Pi {
		yaw -= 2 * math.Pi
	}
	for yaw < -math.Pi {
		yaw += 2 * math.Pi
	}
	return yaw
}

func validPlayerInput(command Command) bool {
	if !finiteInputComponent(command.Yaw) || !finiteInputComponent(command.Pitch) {
		return false
	}
	if command.MoveX < -1 || command.MoveX > 1 || command.MoveZ < -1 || command.MoveZ > 1 {
		return false
	}
	return true
}

func validPlayerLook(yaw, pitch float32) bool {
	const maxPitch = float32(math.Pi/2 - 0.01)
	return finiteInputComponent(yaw) && finiteInputComponent(pitch) &&
		pitch >= -maxPitch && pitch <= maxPitch
}

func placementOverlapsPlayer(block core.BlockID, position core.BlockPos, playerPosition mgl32.Vec3) bool {
	playerBounds := physics.PlayerBounds(playerPosition)
	boxes := physics.BlockCollisionBoxes(block, true)
	offset := mgl32.Vec3{float32(position.X), float32(position.Y), float32(position.Z)}
	for index := 0; index < min(int(boxes.Count), len(boxes.Boxes)); index++ {
		box := core.AABB{
			Min: boxes.Boxes[index].Min.Add(offset),
			Max: boxes.Boxes[index].Max.Add(offset),
		}
		if playerBounds.Overlaps(box) {
			return true
		}
	}
	return false
}

func blockRaycastSampler(dimension *Dimension) func(core.BlockPos) (bool, error) {
	return func(position core.BlockPos) (bool, error) {
		block, ready := dimension.BlockAt(position)
		if !ready {
			return false, ErrChunkNotReady
		}
		if core.IsDoorUpper(block) {
			below := core.BlockPos{X: position.X, Y: position.Y - 1, Z: position.Z}
			lower, lowerReady := dimension.BlockAt(below)
			if !lowerReady || !core.IsDoorLower(lower) {
				return true, nil
			}
			return core.IsDoorOpen(lower) == false, nil
		}
		return core.InteractionTarget(block), nil
	}
}

func torchSupportOffset(block core.BlockID) (core.BlockPos, bool) {
	switch block {
	case core.TorchStandingID:
		return core.BlockPos{Y: -1}, true
	case core.TorchWallPosXID:
		return core.BlockPos{X: -1}, true
	case core.TorchWallNegXID:
		return core.BlockPos{X: 1}, true
	case core.TorchWallPosZID:
		return core.BlockPos{Z: -1}, true
	case core.TorchWallNegZID:
		return core.BlockPos{Z: 1}, true
	default:
		return core.BlockPos{}, false
	}
}

func torchSupport(block core.BlockID, pos core.BlockPos) (core.BlockPos, bool) {
	offset, ok := torchSupportOffset(block)
	if !ok {
		return core.BlockPos{}, false
	}
	return core.BlockPos{X: pos.X + offset.X, Y: pos.Y + offset.Y, Z: pos.Z + offset.Z}, true
}

func torchSupportBlockSolid(id core.BlockID) bool {
	if id == core.AirID || core.IsFluid(id) || core.IsCrop(id) || core.IsTorch(id) || core.IsDoorUpper(id) {
		return false
	}
	return core.RegisteredBlock(id)
}

func torchCellOverlapsPlayer(position core.BlockPos, playerPosition mgl32.Vec3) bool {
	// 火把零碰撞，整格 AABB 判交
	playerBounds := physics.PlayerBounds(playerPosition)
	cell := core.AABB{
		Min: mgl32.Vec3{float32(position.X), float32(position.Y), float32(position.Z)},
		Max: mgl32.Vec3{float32(position.X + 1), float32(position.Y + 1), float32(position.Z + 1)},
	}
	return playerBounds.Overlaps(cell)
}

func mapSetBlockError(err error) RejectReason {
	if err == ErrChunkNotReady || err == ErrBlockOutOfWorld {
		return RejectChunkNotReady
	}
	return RejectInvalidRay
}

func adjacentBlock(block core.BlockPos, face core.BlockFace) core.BlockPos {
	switch face {
	case core.BlockFaceNegX:
		block.X--
	case core.BlockFacePosX:
		block.X++
	case core.BlockFaceNegY:
		block.Y--
	case core.BlockFacePosY:
		block.Y++
	case core.BlockFaceNegZ:
		block.Z--
	case core.BlockFacePosZ:
		block.Z++
	}
	return block
}

func yawToDoorDir(yaw float32) int {
	normalized := math.Mod(float64(yaw)+math.Pi, 2*math.Pi)
	if normalized < 0 {
		normalized += 2 * math.Pi
	}
	yawNorm := float32(normalized - math.Pi)
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

func blockCenterVec3(target core.BlockPos) mgl32.Vec3 {
	return mgl32.Vec3{
		float32(target.X) + 0.5,
		float32(target.Y) + 0.5,
		float32(target.Z) + 0.5,
	}
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func respawnPositionFromBlock(pos core.BlockPos) [3]float32 {
	return [3]float32{float32(pos.X) + 0.5, float32(pos.Y) + 0.5, float32(pos.Z) + 0.5}
}

func respawnBlockFromPosition(position [3]float32) core.BlockPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(position[0]))),
		Y: int32(math.Floor(float64(position[1]))),
		Z: int32(math.Floor(float64(position[2]))),
	}
}

func (engine *Engine) touchChunk(key core.ChunkKey, mutation *realm.Mutation) {
	mutation.Touch(key)
}

func (engine *Engine) noteTrampleLanding(session *sessionState, player *playerState) {}

func (engine *Engine) sortedActiveSessions() []SessionID {
	sessions := make([]SessionID, 0, len(engine.sessions))
	for id, session := range engine.sessions {
		if session.player != nil && session.player.lifecycle == PlayerActive {
			sessions = append(sessions, id)
		}
	}
	// 使用稳定的排序，与 sim/drop.go 的同名实现对齐
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j] < sessions[i] {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}
	return sessions
}
