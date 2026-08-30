package entity

import (
	"errors"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

func (engine *Engine) executeInteractBed(command Command) (RejectReason, bool) {
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
	if !core.IsBed(block) {
		return 0, false
	}
	if !core.IsDisplayNightPhase(engine.displayDayPhase()) {
		return RejectInvalidBlock, true
	}
	foot, _, ok := bedHalfPositions(hit.Block, block)
	if !ok {
		return RejectInvalidBlock, true
	}
	player := session.player
	player.sleeping = true
	player.respawnPresent = true
	player.respawnPos = foot
	player.respawnDim = dimensionID
	return 0, false
}

func (engine *Engine) settleSleepThroughNight() {
	sessions := engine.sortedActiveSessions()
	if len(sessions) == 0 {
		return
	}
	for _, id := range sessions {
		if !engine.sessions[id].player.sleeping {
			return
		}
	}
	completed := engine.worldTime.Load() + 1
	offset := (core.DayLengthTicks - completed%core.DayLengthTicks) % core.DayLengthTicks
	engine.dayPhaseOffset.Store(uint64(offset))
	for _, session := range engine.sessions {
		if session.player != nil {
			session.player.sleeping = false
		}
	}
}

var bedStandHeight = physics.BlockCollisionBoxes(core.BedFootSouthID, true).Boxes[0].Max.Y()

func (engine *Engine) bedRespawnCandidate(player *playerState) *restoreCandidate {
	if !player.respawnPresent {
		return nil
	}
	dimension := engine.dimension(player.respawnDim)
	if dimension == nil {
		return nil
	}
	footBlock, footReady := dimension.BlockAt(player.respawnPos)
	if !footReady {
		return nil
	}
	if !core.IsBedFoot(footBlock) {
		player.respawnPresent = false
		return nil
	}
	dir := core.BedDir(footBlock)
	headPos := core.BedHeadNeighbor(player.respawnPos, dir)
	headBlock, headReady := dimension.BlockAt(headPos)
	if !headReady {
		return nil
	}
	if !core.IsBedHead(headBlock) || core.BedDir(headBlock) != dir {
		player.respawnPresent = false
		return nil
	}
	return &restoreCandidate{location: PlayerLocation{
		Dimension: player.respawnDim,
		Position: mgl32.Vec3{
			float32(player.respawnPos.X) + 0.5,
			float32(player.respawnPos.Y) + bedStandHeight,
			float32(player.respawnPos.Z) + 0.5,
		},
	}}
}
