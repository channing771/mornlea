package entity

import (
	"math"

	"github.com/channing771/mornlea/packages/shared/core"
)

// SessionSubscription 是 runtime 维护订阅时读取的实体派生值。
type SessionSubscription struct {
	Dimension core.DimensionID
	Center    core.ChunkPos
	Pending   []core.ChunkKey
	Active    bool
}

// SessionSubscription 返回玩家当前中心和待出生阶段必须保留的区块。
func (state *State) SessionSubscription(id SessionID) (SessionSubscription, bool) {
	session := state.sessions[id]
	if session == nil || session.player == nil {
		return SessionSubscription{}, false
	}
	player := session.player
	center := player.anchor
	if player.lifecycle == PlayerActive {
		center = (core.BlockPos{
			X: int32(math.Floor(float64(player.state.Position.X()))),
			Z: int32(math.Floor(float64(player.state.Position.Z()))),
		}).Chunk()
	}
	value := SessionSubscription{
		Dimension: session.dimension,
		Center:    center,
		Active:    player.lifecycle == PlayerActive,
	}
	if player.lifecycle == PlayerPendingSpawn {
		value.Pending = make([]core.ChunkKey, 0, len(player.restoreWanted)+len(player.spawnWanted))
		for key := range player.restoreWanted {
			value.Pending = append(value.Pending, key)
		}
		for chunk := range player.spawnWanted {
			value.Pending = append(value.Pending, core.ChunkKey{
				Dimension: session.dimension,
				Pos:       chunk,
			})
		}
		sortChunkKeys(value.Pending)
	}
	return value, true
}

// AddCompanionWanted 把伙伴生命周期需要的区块加入 runtime 的订阅并集。
func (state *State) AddCompanionWanted(wanted map[core.ChunkKey]struct{}) {
	for _, entry := range state.companions {
		if entry.active {
			center := companionChunk(entry.state.Position)
			for dz := -companionInterestRadius; dz <= companionInterestRadius; dz++ {
				for dx := -companionInterestRadius; dx <= companionInterestRadius; dx++ {
					wanted[core.ChunkKey{
						Dimension: entry.dimension,
						Pos: core.ChunkPos{
							X: center.X + int32(dx),
							Z: center.Z + int32(dz),
						},
					}] = struct{}{}
				}
			}
			continue
		}
		for key := range entry.restoreWanted {
			wanted[key] = struct{}{}
		}
		for chunk := range entry.spawnWanted {
			wanted[core.ChunkKey{Dimension: entry.dimension, Pos: chunk}] = struct{}{}
		}
	}
}

// CompanionSubscriptionDistanceSquared 返回伙伴对区块的最近订阅距离。
func (state *State) CompanionSubscriptionDistanceSquared(
	key core.ChunkKey,
) (int64, bool) {
	distance := int64(math.MaxInt64)
	relevant := false
	for _, entry := range state.companions {
		candidate, ok := companionSubscriptionDistanceSquared(entry, key)
		if ok && candidate < distance {
			distance = candidate
			relevant = true
		}
	}
	return distance, relevant
}

// CompanionWantsChunk 报告任一伙伴是否仍需要该区块。
func (state *State) CompanionWantsChunk(key core.ChunkKey) bool {
	_, wanted := state.CompanionSubscriptionDistanceSquared(key)
	return wanted
}

func companionSubscriptionDistanceSquared(
	state *companionState,
	key core.ChunkKey,
) (int64, bool) {
	if state.active {
		if key.Dimension != state.dimension {
			return 0, false
		}
		center := companionChunk(state.state.Position)
		if absChunkDelta(key.Pos.X, center.X) > companionInterestRadius ||
			absChunkDelta(key.Pos.Z, center.Z) > companionInterestRadius {
			return 0, false
		}
		return chunkDistanceSquared(key.Pos, center), true
	}
	distance := int64(math.MaxInt64)
	relevant := false
	if _, wanted := state.restoreWanted[key]; wanted {
		for _, candidate := range state.restoreCandidates {
			if candidate.location.Dimension != key.Dimension {
				continue
			}
			distance = min(distance, chunkDistanceSquared(
				key.Pos,
				companionChunk(candidate.location.Position),
			))
			relevant = true
		}
	}
	if key.Dimension == state.dimension {
		if _, wanted := state.spawnWanted[key.Pos]; wanted && len(state.spawnCandidates) != 0 {
			anchor := (core.BlockPos{
				X: state.spawnCandidates[0].X,
				Z: state.spawnCandidates[0].Z,
			}).Chunk()
			distance = min(distance, chunkDistanceSquared(key.Pos, anchor))
			relevant = true
		}
	}
	return distance, relevant
}

func chunkDistanceSquared(left, right core.ChunkPos) int64 {
	dx := int64(left.X - right.X)
	dz := int64(left.Z - right.Z)
	return dx*dx + dz*dz
}

func absChunkDelta(left, right int32) int64 {
	delta := int64(left) - int64(right)
	if delta < 0 {
		return -delta
	}
	return delta
}
