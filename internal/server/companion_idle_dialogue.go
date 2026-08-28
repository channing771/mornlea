package server

import (
	"encoding/binary"
	"hash/fnv"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/pathfind"
)

const (
	idleDialogueMinTicks uint64 = companion.TicksPerMinute
	idleDialogueMaxTicks uint64 = 2 * companion.TicksPerMinute
)

func idleDialogueInterval(id companion.ID, seed uint64) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write(id[:])
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], seed)
	_, _ = hash.Write(encoded[:])
	return idleDialogueMinTicks + hash.Sum64()%(idleDialogueMaxTicks-idleDialogueMinTicks+1)
}

func idleDialogueDue(now, deadline uint64) bool {
	return int64(now-deadline) >= 0
}

func withinIdleDialogueDistance(from, to [3]float32) bool {
	dx := from[0] - to[0]
	dz := from[2] - to[2]
	const radius = pathfind.PathWindowHorizontalRadius
	return dx*dx+dz*dz <= radius*radius
}

func (m *companionManager) idleDialogueAudience(
	issuer companionTaskIssuer,
	body companion.Body,
) bool {
	if !issuer.playerID.Valid() || issuer.restored {
		return false
	}
	target, online := m.followTarget(issuer.playerID)
	return online && withinIdleDialogueDistance(body.Position, target.Position)
}

// dispatchIdleDialogues 在任务规划之后消费已到期的空闲表达机会。下一期限先于
// 发言资格检查安排，使 inactive、离线、超距或并发受限都只跳过本次机会。
func (m *companionManager) dispatchIdleDialogues() {
	now := m.engine.TickCount()
	for _, id := range m.orderedIDs {
		slot := m.slots[id]
		_, hasCurrent := slot.queue.Current()
		if hasCurrent || slot.queue.Len() != 0 {
			slot.hasIdleDialogueAtTick = false
			continue
		}
		if !slot.currentIssuer.playerID.Valid() || slot.currentIssuer.restored {
			slot.hasIdleDialogueAtTick = false
			continue
		}
		if !slot.hasIdleDialogueAtTick {
			slot.idleDialogueAtTick = now + idleDialogueInterval(id, now)
			slot.hasIdleDialogueAtTick = true
			continue
		}
		if !idleDialogueDue(now, slot.idleDialogueAtTick) {
			continue
		}
		seed := slot.idleDialogueAtTick
		slot.idleDialogueAtTick = seed + idleDialogueInterval(id, seed)
		body, active := m.body(id)
		if !active || !m.idleDialogueAudience(slot.currentIssuer, body) {
			continue
		}
		m.requestDialogue(id, companion.DialogueNode{Kind: companion.DialogueNodeIdle})
	}
}
