// 本文件实现 server 侧被动牛的按会话订阅发布：被动牛进入已订阅 chunk 发
// spawn、逐 tick 发 state、离开视野或死亡发 despawn，每类每 tick 至多一包
// 且 record 按 ID 严格升序。可见性判定（脚底 chunk 已订阅且快照已送达）与
// 每会话镜像跟随复用夜行者发布的既有形状；本文件不触碰被动牛的权威事实，
// 只把 `Engine.PassiveMobs` 的 tick 末值快照投影到各会话。
package server

import (
	"slices"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/network"
)

// publishPassives 是 publishSession 的被动牛段。passives 是 tick 末的全量
// 值快照（Engine 集合秩序，即 ID 升序），由 `publishWithChats` 每 tick 取一
// 次并供全部会话共享。固定次序为先 despawn、再 spawn、后 state：镜像容量先
// 释放后占用，新可见个体只在 spawn tick 携带完整身体（state 从下一 tick 开
// 始，与夜行者发布的「新 spawn 跳过当 tick state」语义一致）。任一包校验失
// 败或入队失败都关闭该会话并中止本段，绝不留下半更新的会话镜像。
func (server *Server) publishPassives(
	current *session,
	tick uint64,
	passives []contract.PassiveMob,
) bool {
	if len(passives) == 0 && len(current.visiblePassives) == 0 {
		return true
	}
	if current.visiblePassives == nil {
		// 与 `visibleHostiles` 同款惰性初始化：直接构造 session 的调用方
		// （测试 harness、observer 会话）不必预建全部镜像集合。
		current.visiblePassives = make(map[uint64]struct{})
	}
	// 可见性截面：保持快照的 ID 升序（Engine 秩序），despawn/spawn/state
	// 三个批次的排序因此免于再排。
	visible := make([]contract.PassiveMob, 0, len(passives))
	for _, mob := range passives {
		if server.passiveCandidateVisible(current, mob) {
			visible = append(visible, mob)
		}
	}

	// 1) despawn：镜像里登记、当前不可见的个体。死亡与远离消失的个体已从
	// Engine 集合移除，走同一条「镜像有而可见截面无」的判据，不需要单独通道。
	despawned := make([]uint64, 0, len(current.visiblePassives))
	for id := range current.visiblePassives {
		if passiveIndexOf(visible, id) < 0 {
			despawned = append(despawned, id)
		}
	}
	slices.Sort(despawned)
	if len(despawned) != 0 {
		despawn := network.PassiveDespawn{ServerTick: tick, IDs: despawned}
		if err := despawn.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(despawn) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
		for _, id := range despawned {
			delete(current.visiblePassives, id)
		}
	}

	// 2) 与 3) 同一遍截面扫描：镜像未登记的可见个体进 spawn 批次（携带完整
	// 出生身体），已登记的进 state 批次；刚 spawn 的个体下一 tick 才进入
	// state（spawn 已携带当 tick 完整身体）。两个批次都继承截面的升序。
	spawns := make([]network.PassiveSpawnRecord, 0, len(visible))
	states := make([]network.PassiveStateRecord, 0, len(visible))
	for _, mob := range visible {
		if _, known := current.visiblePassives[mob.ID]; known {
			// 吃草瞬态只进 state：`Grazing` 是布尔呈现位，wire 上按 0/1 字节
			// 搬运；spawn 携带的是出生身体，不含任何瞬态。
			var grazing uint8
			if mob.Grazing {
				grazing = 1
			}
			states = append(states, network.PassiveStateRecord{
				ID:       mob.ID,
				Position: mob.State.Position,
				Velocity: mob.State.Velocity,
				Yaw:      mob.Yaw,
				Health:   mob.Health,
				Grazing:  grazing,
			})
			continue
		}
		spawns = append(spawns, network.PassiveSpawnRecord{
			ID:        mob.ID,
			Dimension: mob.Dimension,
			Position:  mob.State.Position,
			Yaw:       mob.Yaw,
			Health:    mob.Health,
		})
	}
	if len(spawns) != 0 {
		spawn := network.PassiveSpawn{ServerTick: tick, Spawns: spawns}
		if err := spawn.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(spawn) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
		for _, record := range spawns {
			current.visiblePassives[record.ID] = struct{}{}
		}
	}
	if len(states) != 0 {
		state := network.PassiveState{ServerTick: tick, States: states}
		if err := state.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(state) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
	}
	return true
}

// passiveCandidateVisible 是被动牛的会话可见性判定：脚底 chunk 必须仍在会
// 话订阅集合内，且该会话已收到过这份快照（客户端先有世界再有实体，与夜行
// 者的判定逐语义一致）。
func (server *Server) passiveCandidateVisible(current *session, mob contract.PassiveMob) bool {
	foot := publicationFootChunk(mob.Dimension, [3]float32(mob.State.Position))
	if !server.engine.SessionWantsChunk(current.id, foot) {
		return false
	}
	publication := current.publications[foot]
	return publication != nil && publication.snapshotSent
}

// passiveIndexOf 返回 ID 在升序截面中的下标；未命中返回 -1。线性扫描即可：
// 截面至多 32 个个体，二分开销不值得。
func passiveIndexOf(mobs []contract.PassiveMob, id uint64) int {
	for index := range mobs {
		if mobs[index].ID == id {
			return index
		}
	}
	return -1
}
