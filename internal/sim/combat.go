package sim

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

const (
	playerMeleeReach         = float32(3)
	playerMeleeDamage        = int32(2)
	playerMeleeCooldownTicks = uint8(10)
)

type meleeIntent struct {
	attacker SessionID
	target   SessionID
}

// advancePlayerMelee 在同一份已排序 active 会话快照上先收集全部有效近战意图，
// 再统一结算伤害。收集与应用分开，确保同 tick 被打到零血的攻击者仍会提交已冻结
// 的反击意图；死亡交给紧随其后的 `settleDeaths` 结算。
func (engine *Engine) advancePlayerMelee() {
	sessions := engine.sortedActiveSessions()
	for _, id := range sessions {
		player := engine.sessions[id].player
		player.meleeSuppressedMining = false
		if player.meleeCooldownTicks > 0 {
			player.meleeCooldownTicks--
		}
	}

	var intents [8]meleeIntent
	count := 0
	for _, id := range sessions {
		attacker := engine.sessions[id].player
		if !attacker.miningHeld {
			continue
		}
		targetID, ok := engine.playerMeleeTarget(id, sessions)
		if !ok {
			continue
		}
		target := engine.sessions[targetID].player
		if target.meleeCooldownTicks != 0 {
			continue
		}
		attacker.meleeSuppressedMining = true
		target.meleeCooldownTicks = playerMeleeCooldownTicks
		intents[count] = meleeIntent{attacker: id, target: targetID}
		count++
	}
	for _, intent := range intents[:count] {
		engine.sessions[intent.target].player.applyDamage(playerMeleeDamage)
	}
}

// rayAABBDistance 返回单位方向射线最早进入 bounds 的非负距离。近战只接受三格内
// 命中；平行于某轴时，起点已落在该轴盒外就不可能命中。
func rayAABBDistance(origin, direction mgl32.Vec3, bounds core.AABB) (float32, bool) {
	near, far := float32(-math.MaxFloat32), float32(math.MaxFloat32)
	for axis := range 3 {
		if math.Abs(float64(direction[axis])) < 1e-6 {
			if origin[axis] < bounds.Min[axis] || origin[axis] > bounds.Max[axis] {
				return 0, false
			}
			continue
		}
		entry := (bounds.Min[axis] - origin[axis]) / direction[axis]
		exit := (bounds.Max[axis] - origin[axis]) / direction[axis]
		if entry > exit {
			entry, exit = exit, entry
		}
		near = max(near, entry)
		far = min(far, exit)
		if near > far {
			return 0, false
		}
	}
	if far < 0 {
		return 0, false
	}
	near = max(near, 0)
	if near > playerMeleeReach {
		return 0, false
	}
	return near, true
}

// playerMeleeTarget 从本 tick 的 active 会话快照中选择攻击者射线最先命中的同维
// 活着玩家。方块只在玩家表面前方才阻挡，因而与玩家表面同距的方块不改写命中。
func (engine *Engine) playerMeleeTarget(
	attackerID SessionID,
	sessions []SessionID,
) (SessionID, bool) {
	attacker := engine.sessions[attackerID]
	if attacker == nil || attacker.player == nil ||
		attacker.player.lifecycle != PlayerActive || attacker.player.health == 0 {
		return 0, false
	}
	if !sessionInSnapshot(attackerID, sessions) {
		return 0, false
	}
	dimension := engine.dimensions[attacker.dimension]
	if dimension == nil {
		return 0, false
	}
	player := attacker.player
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(player.yaw, player.pitch)
	var target SessionID
	targetDistance := playerMeleeReach
	for _, id := range sessions {
		candidate := engine.sessions[id]
		if id == attackerID || candidate == nil || candidate.player == nil ||
			candidate.player.lifecycle != PlayerActive || candidate.player.health == 0 ||
			candidate.dimension != attacker.dimension {
			continue
		}
		distance, hit := rayAABBDistance(
			eye,
			direction,
			physics.PlayerBounds(candidate.player.state.Position),
		)
		if !hit || distance > targetDistance ||
			distance == targetDistance && target != 0 && id > target {
			continue
		}
		target, targetDistance = id, distance
	}
	if target == 0 {
		return 0, false
	}
	hit, blocked, err := core.RaycastBlocks(
		eye,
		direction,
		playerMeleeReach,
		blockRaycastSampler(dimension),
	)
	if err != nil || blocked && hit.Distance < targetDistance {
		return 0, false
	}
	return target, true
}

func sessionInSnapshot(id SessionID, sessions []SessionID) bool {
	for _, candidate := range sessions {
		if candidate == id {
			return true
		}
	}
	return false
}
