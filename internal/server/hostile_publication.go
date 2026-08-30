// 本文件实现 server 侧夜行者的按会话订阅发布：夜行者进入已订阅 chunk 发
// spawn、逐 tick 发 state、离开视野或死亡发 despawn，每类每 tick 至多一包
// 且 record 按 ID 严格升序。可见性判定（脚底 chunk 已订阅且快照已送达）与
// 每会话镜像跟随复用 `companion_publication.go` 的既有形状；本文件不触碰
// 夜行者的权威事实，只把 `Engine.HostileMobs` 的 tick 末值快照投影到各会话。
package server

import (
	"slices"

	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim/contract"
)

// publishHostiles 是 publishSession 的夜行者段。hostiles 是 tick 末的全量
// 值快照（Engine 集合秩序，即 ID 升序），由 `publishWithChats` 每 tick 取一
// 次并供全部会话共享。固定次序为先 despawn、再 spawn、后 state：镜像容量先
// 释放后占用，新可见个体只在 spawn tick 携带完整身体（state 从下一 tick 开
// 始，与伙伴发布的「新 spawn 跳过当 tick state」语义一致）。任一包校验失败
// 或入队失败都关闭该会话并中止本段，绝不留下半更新的会话镜像。
func (server *Server) publishHostiles(
	current *session,
	tick uint64,
	hostiles []contract.HostileMob,
) bool {
	if len(hostiles) == 0 && len(current.visibleHostiles) == 0 {
		return true
	}
	if current.visibleHostiles == nil {
		// 与 `visibleCompanions` 同款惰性初始化：直接构造 session 的调用方
		// （测试 harness、observer 会话）不必预建全部镜像集合。
		current.visibleHostiles = make(map[uint64]struct{})
	}
	// 可见性截面：保持快照的 ID 升序（Engine 秩序），despawn/spawn/state
	// 三个批次的排序因此免于再排。
	visible := make([]contract.HostileMob, 0, len(hostiles))
	for _, mob := range hostiles {
		if server.hostileCandidateVisible(current, mob) {
			visible = append(visible, mob)
		}
	}

	// 1) despawn：镜像里登记、当前不可见的个体。死亡与远离消失的个体已从
	// Engine 集合移除，走同一条「镜像有而可见截面无」的判据，不需要单独通道。
	despawned := make([]uint64, 0, len(current.visibleHostiles))
	for id := range current.visibleHostiles {
		if hostileIndexOf(visible, id) < 0 {
			despawned = append(despawned, id)
		}
	}
	slices.Sort(despawned)
	if len(despawned) != 0 {
		despawn := network.HostileDespawn{ServerTick: tick, IDs: despawned}
		if err := despawn.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(despawn) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
		for _, id := range despawned {
			delete(current.visibleHostiles, id)
		}
	}

	// 2) 与 3) 同一遍截面扫描：镜像未登记的可见个体进 spawn 批次（携带完整
	// 出生身体），已登记的进 state 批次；刚 spawn 的个体下一 tick 才进入
	// state（spawn 已携带当 tick 完整身体）。两个批次都继承截面的升序。
	spawns := make([]network.HostileSpawnRecord, 0, len(visible))
	states := make([]network.HostileStateRecord, 0, len(visible))
	for _, mob := range visible {
		if _, known := current.visibleHostiles[mob.ID]; known {
			states = append(states, network.HostileStateRecord{
				ID:       mob.ID,
				Position: mob.State.Position,
				Velocity: mob.State.Velocity,
				Yaw:      mob.Yaw,
				Health:   mob.Health,
			})
			continue
		}
		spawns = append(spawns, network.HostileSpawnRecord{
			ID:        mob.ID,
			Dimension: mob.Dimension,
			Position:  mob.State.Position,
			Yaw:       mob.Yaw,
			Health:    mob.Health,
		})
	}
	if len(spawns) != 0 {
		spawn := network.HostileSpawn{ServerTick: tick, Spawns: spawns}
		if err := spawn.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(spawn) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
		for _, record := range spawns {
			current.visibleHostiles[record.ID] = struct{}{}
		}
	}
	if len(states) != 0 {
		state := network.HostileState{ServerTick: tick, States: states}
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

// hostileCandidateVisible 是夜行者的会话可见性判定：脚底 chunk 必须仍在会
// 话订阅集合内，且该会话已收到过这份快照（客户端先有世界再有实体，与伙伴
// 的判定逐语义一致）。
func (server *Server) hostileCandidateVisible(current *session, mob contract.HostileMob) bool {
	foot := publicationFootChunk(mob.Dimension, [3]float32(mob.State.Position))
	if !server.engine.SessionWantsChunk(current.id, foot) {
		return false
	}
	publication := current.publications[foot]
	return publication != nil && publication.snapshotSent
}

// hostileIndexOf 返回 ID 在升序截面中的下标；未命中返回 -1。线性扫描即可：
// 截面至多 64 个个体，二分开销不值得。
func hostileIndexOf(mobs []contract.HostileMob, id uint64) int {
	for index := range mobs {
		if mobs[index].ID == id {
			return index
		}
	}
	return -1
}
