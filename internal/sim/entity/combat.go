package entity

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

const (
	playerMeleeReach         = float32(3)
	playerMeleeCooldownTicks = uint8(10)
	hostileMeleeDamage       = int32(3)
	maxCombatActors          = 72
	maxCombatIntents         = 72
)

var hostileAttackRangeSquared = HostileAttackRange * HostileAttackRange

type combatActor struct {
	kind core.CombatTargetKind
	id   uint64
}

type combatActorSnapshot struct {
	actor          combatActor
	dimension      core.DimensionID
	position       mgl32.Vec3
	yaw            float32
	pitch          float32
	health         uint8
	attackCooldown uint8
	hurtCooldown   uint8
	attacking      bool
	targetSession  SessionID
	selectedSlot   uint8
	selectedItem   core.ItemID
	selectedCount  uint8
}

type combatIntent struct {
	attacker         combatActor
	target           combatActor
	dimension        core.DimensionID
	damage           int32
	distance         float32
	attackerPosition mgl32.Vec3
	targetPosition   mgl32.Vec3
	attackerYaw      float32
	selectedSlot     uint8
	selectedItem     core.ItemID
	selectedCount    uint8
}

func (engine *engineContext) advanceCombat(result *TickResult) {
	engine.advanceCombatWithLimits(result, maxCombatActors, maxCombatIntents)
}

// advanceCombatWithLimits 先在固定数组中完成快照、冷却预演与全部 intent 追加；
// 任一追加越界都在提交前失败，只有 tick-local 采掘抑制会在入口清零。
func (engine *engineContext) advanceCombatWithLimits(
	result *TickResult,
	actorLimit, intentLimit int,
) bool {
	var playerIDs [maxCombatActors]SessionID
	playerCount := 0
	for id, session := range engine.sessions {
		if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
			continue
		}
		session.player.meleeSuppressedMining = false
		if playerCount == len(playerIDs) {
			return false
		}
		index := playerCount
		for index > 0 && playerIDs[index-1] > id {
			playerIDs[index] = playerIDs[index-1]
			index--
		}
		playerIDs[index] = id
		playerCount++
	}

	var snapshots [maxCombatActors]combatActorSnapshot
	snapshotCount := 0
	for index := range engine.hostiles.entries {
		if snapshotCount >= actorLimit || snapshotCount == len(snapshots) {
			return false
		}
		hostile := &engine.hostiles.entries[index]
		snapshots[snapshotCount] = combatActorSnapshot{
			actor:          combatActor{kind: core.CombatTargetHostile, id: hostile.id},
			dimension:      hostile.dimension,
			position:       hostile.state.Position,
			yaw:            hostile.yaw,
			health:         hostile.health,
			attackCooldown: hostile.attackCooldown,
			hurtCooldown:   hostile.hurtCooldown,
			attacking:      hostile.attackIntent,
			targetSession:  hostile.attackTargetSession,
		}
		snapshotCount++
	}
	for _, id := range playerIDs[:playerCount] {
		if snapshotCount >= actorLimit || snapshotCount == len(snapshots) {
			return false
		}
		session := engine.sessions[id]
		player := session.player
		selectedSlot := player.inventory.Hotbar.Selected
		selectedItem := core.ItemNone
		selectedCount := uint8(0)
		if selectedSlot < core.HotbarSlots {
			stack := player.inventory.Hotbar.Slots[selectedSlot]
			selectedItem, selectedCount = stack.Item, stack.Count
		}
		snapshots[snapshotCount] = combatActorSnapshot{
			actor:          combatActor{kind: core.CombatTargetPlayer, id: uint64(id)},
			dimension:      session.dimension,
			position:       player.state.Position,
			yaw:            player.yaw,
			pitch:          player.pitch,
			health:         player.health,
			attackCooldown: player.attackCooldownTicks,
			hurtCooldown:   player.hurtCooldownTicks,
			attacking:      player.miningHeld,
			selectedSlot:   selectedSlot,
			selectedItem:   selectedItem,
			selectedCount:  selectedCount,
		}
		snapshotCount++
	}
	for index := range snapshotCount {
		if snapshots[index].attackCooldown > 0 {
			snapshots[index].attackCooldown--
		}
		if snapshots[index].hurtCooldown > 0 {
			snapshots[index].hurtCooldown--
		}
	}

	var intents [maxCombatIntents]combatIntent
	intentCount := 0
	for index := range engine.hostiles.entries {
		attacker := combatSnapshotForActor(
			snapshots[:snapshotCount],
			combatActor{kind: core.CombatTargetHostile, id: engine.hostiles.entries[index].id},
		)
		if attacker == nil || !attacker.attacking || attacker.health == 0 || attacker.attackCooldown != 0 {
			continue
		}
		target := combatSnapshotForActor(
			snapshots[:snapshotCount],
			combatActor{kind: core.CombatTargetPlayer, id: uint64(attacker.targetSession)},
		)
		if target == nil || target.health == 0 || target.hurtCooldown != 0 ||
			target.dimension != attacker.dimension ||
			horizontalDistanceSq(attacker.position, target.position) > hostileAttackRangeSquared {
			continue
		}
		if intentCount >= intentLimit || intentCount == len(intents) {
			return false
		}
		intents[intentCount] = combatIntent{
			attacker: attacker.actor, target: target.actor, damage: hostileMeleeDamage,
			dimension:        attacker.dimension,
			distance:         float32(math.Sqrt(float64(horizontalDistanceSq(attacker.position, target.position)))),
			attackerPosition: attacker.position, targetPosition: target.position,
			attackerYaw: attacker.yaw,
		}
		intentCount++
	}
	for _, id := range playerIDs[:playerCount] {
		attacker := combatSnapshotForActor(
			snapshots[:snapshotCount], combatActor{kind: core.CombatTargetPlayer, id: uint64(id)},
		)
		intent, ok := engine.playerCombatIntent(attacker, snapshots[:snapshotCount])
		if !ok {
			continue
		}
		if intentCount >= intentLimit || intentCount == len(intents) {
			return false
		}
		intents[intentCount] = intent
		intentCount++
	}

	for index := range snapshotCount {
		snapshot := &snapshots[index]
		switch snapshot.actor.kind {
		case core.CombatTargetPlayer:
			session := engine.sessions[SessionID(snapshot.actor.id)]
			if session != nil && session.player != nil {
				session.player.attackCooldownTicks = snapshot.attackCooldown
				session.player.hurtCooldownTicks = snapshot.hurtCooldown
			}
		case core.CombatTargetHostile:
			if hostileIndex := engine.hostiles.findIndex(snapshot.actor.id); hostileIndex >= 0 {
				hostile := &engine.hostiles.entries[hostileIndex]
				hostile.attackCooldown = snapshot.attackCooldown
				hostile.hurtCooldown = snapshot.hurtCooldown
			}
		}
	}
	var reservedVictims [maxCombatIntents]combatActor
	reservedCount := 0
	for index := range intentCount {
		intent := intents[index]
		reserved := false
		for reservedIndex := range reservedCount {
			if reservedVictims[reservedIndex] == intent.target {
				reserved = true
				break
			}
		}
		if reserved {
			continue
		}
		reservedVictims[reservedCount] = intent.target
		reservedCount++
		engine.settleCombatIntent(result, intent)
	}
	return true
}

func combatSnapshotForActor(
	snapshots []combatActorSnapshot,
	actor combatActor,
) *combatActorSnapshot {
	for index := range snapshots {
		if snapshots[index].actor == actor {
			return &snapshots[index]
		}
	}
	return nil
}

func (engine *engineContext) playerCombatIntent(
	attacker *combatActorSnapshot,
	snapshots []combatActorSnapshot,
) (combatIntent, bool) {
	if attacker == nil || !attacker.attacking || attacker.health == 0 || attacker.attackCooldown != 0 {
		return combatIntent{}, false
	}
	dimension := engine.dimension(attacker.dimension)
	if dimension == nil {
		return combatIntent{}, false
	}
	eye := attacker.position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(attacker.yaw, attacker.pitch)
	var target *combatActorSnapshot
	targetDistance := playerMeleeReach
	for index := range snapshots {
		candidate := &snapshots[index]
		if !candidate.actor.kind.Valid() || candidate.actor == attacker.actor ||
			candidate.dimension != attacker.dimension || candidate.health == 0 {
			continue
		}
		distance, hit := rayAABBDistance(eye, direction, physics.PlayerBounds(candidate.position))
		if !hit || distance > targetDistance || distance == targetDistance && target != nil &&
			(candidate.actor.kind > target.actor.kind ||
				candidate.actor.kind == target.actor.kind && candidate.actor.id > target.actor.id) {
			continue
		}
		target, targetDistance = candidate, distance
	}
	if target == nil || target.hurtCooldown != 0 {
		return combatIntent{}, false
	}
	hit, blocked, err := core.RaycastBlocks(
		eye, direction, playerMeleeReach, blockRaycastSampler(dimension),
	)
	if err != nil || blocked && hit.Distance < targetDistance {
		return combatIntent{}, false
	}
	return combatIntent{
		attacker: attacker.actor, target: target.actor, dimension: attacker.dimension,
		damage:   core.WeaponDamage(attacker.selectedItem),
		distance: targetDistance, attackerPosition: attacker.position, targetPosition: target.position,
		attackerYaw: attacker.yaw, selectedSlot: attacker.selectedSlot, selectedItem: attacker.selectedItem,
		selectedCount: attacker.selectedCount,
	}, true
}

func combatKnockback(from, to mgl32.Vec3, yaw float32) mgl32.Vec3 {
	delta := mgl32.Vec3{to.X() - from.X(), 0, to.Z() - from.Z()}
	if delta.LenSqr() == 0 {
		look := LookDirection(yaw, 0)
		delta = mgl32.Vec3{look.X(), 0, look.Z()}
	}
	return delta.Normalize().Mul(0.35)
}

// settleCombatIntent 在任何状态写入前解析全部 live 身份与冻结栏位；验证成功后
// 按固定顺序一次提交该 intent，失败不会留下部分副作用。
func (engine *engineContext) settleCombatIntent(result *TickResult, intent combatIntent) bool {
	var attackerPlayer *playerState
	var attackerHostile *hostileState
	var attackerDimension core.DimensionID
	switch intent.attacker.kind {
	case core.CombatTargetPlayer:
		session := engine.sessions[SessionID(intent.attacker.id)]
		if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
			return false
		}
		attackerPlayer = session.player
		attackerDimension = session.dimension
		if intent.selectedSlot >= core.HotbarSlots ||
			attackerPlayer.inventory.Hotbar.Selected != intent.selectedSlot {
			return false
		}
		stack := attackerPlayer.inventory.Hotbar.Slots[intent.selectedSlot]
		if stack.Item != intent.selectedItem || stack.Count != intent.selectedCount {
			return false
		}
	case core.CombatTargetHostile:
		index := engine.hostiles.findIndex(intent.attacker.id)
		if index < 0 {
			return false
		}
		attackerHostile = &engine.hostiles.entries[index]
		attackerDimension = attackerHostile.dimension
	default:
		return false
	}
	if attackerDimension != intent.dimension || intent.damage <= 0 {
		return false
	}

	var targetPlayer *playerState
	var targetHostile *hostileState
	var targetDimension core.DimensionID
	switch intent.target.kind {
	case core.CombatTargetPlayer:
		session := engine.sessions[SessionID(intent.target.id)]
		if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
			return false
		}
		targetPlayer = session.player
		targetDimension = session.dimension
	case core.CombatTargetHostile:
		index := engine.hostiles.findIndex(intent.target.id)
		if index < 0 {
			return false
		}
		targetHostile = &engine.hostiles.entries[index]
		targetDimension = targetHostile.dimension
	default:
		return false
	}
	if targetDimension != intent.dimension || attackerHostile != nil && targetPlayer == nil {
		return false
	}

	if targetPlayer != nil {
		targetPlayer.applyDamage(intent.damage)
		targetPlayer.state.Velocity = targetPlayer.state.Velocity.Add(combatKnockback(
			intent.attackerPosition, intent.targetPosition, intent.attackerYaw,
		))
	} else {
		targetHostile.applyDamage(intent.damage)
		targetHostile.state.Velocity = targetHostile.state.Velocity.Add(combatKnockback(
			intent.attackerPosition, intent.targetPosition, intent.attackerYaw,
		))
	}

	if attackerPlayer != nil {
		attackerPlayer.attackCooldownTicks = playerMeleeCooldownTicks
		if targetPlayer != nil {
			targetPlayer.hurtCooldownTicks = playerMeleeCooldownTicks
		} else {
			targetHostile.hurtCooldown = playerMeleeCooldownTicks
		}
		attackerPlayer.applyExhaustion(exhaustionMeleeMilli, engine.tunables.ExhaustionThresholdMilli)
		attackerPlayer.meleeSuppressedMining = true
		if core.IsIntactSword(intent.selectedItem) &&
			consumeToolDurabilityAt(&attackerPlayer.actorState, intent.selectedSlot, intent.selectedItem) {
			attackerPlayer.inventoryDirty = true
		}
		result.CombatHits = append(result.CombatHits, CombatHit{
			Session: SessionID(intent.attacker.id), Damage: uint8(intent.damage), TargetKind: intent.target.kind,
		})
	} else {
		attackerHostile.attackCooldown = hostileCooldownPeriodTicks
		targetPlayer.hurtCooldownTicks = hostileCooldownPeriodTicks
	}
	return true
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
