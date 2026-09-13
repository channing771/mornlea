// 本文件实现 server 侧投射物的按会话订阅发布：投射物进入已订阅 chunk 发
// spawn（携带完整飞行事实）、逐 tick 发 state、离开视野或从权威集合消失（命中
// 方块/实体、寿命耗尽、越界、上限挤占——引擎统一从投影移除）发 despawn，每类
// 每 tick 至多一包且 record 按 ID 严格升序。可见性判定与发布节律复用夜行者
// 发布（`hostile_publication.go`）的既有形状；本文件不触碰投射物的权威事实，
// 只把 `Engine.Projectiles` 的 tick 末值快照投影到各会话。客户端镜像不预测：
// 弹道演进只由本文件下发的消息驱动。
package server

import (
	"slices"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/network"
)

// publishProjectiles 是 publishSession 的投射物段，固定排在夜行者段之后。
// projectiles 是 tick 末的全量值快照（Engine 集合秩序，即 ID 升序），由
// `publishWithChats` 每 tick 取一次并供全部会话共享。固定次序为先 despawn、
// 再 spawn、后 state：镜像容量先释放后占用，新可见投射物只在 spawn tick 携带
// 完整飞行事实（state 从下一 tick 开始，与夜行者的「新 spawn 跳过当 tick
// state」语义一致）。任一包校验失败或入队失败都关闭该会话并中止本段，绝不
// 留下半更新的会话镜像。
func (server *Server) publishProjectiles(
	current *session,
	tick uint64,
	projectiles []contract.ProjectileSnapshot,
) bool {
	if len(projectiles) == 0 && len(current.visibleProjectiles) == 0 {
		return true
	}
	if current.visibleProjectiles == nil {
		// 与 `visibleHostiles` 同款惰性初始化：直接构造 session 的调用方
		//（测试 harness、observer 会话）不必预建全部镜像集合。
		current.visibleProjectiles = make(map[uint64]struct{})
	}
	// 可见性截面：保持快照的 ID 升序（Engine 秩序），despawn/spawn/state
	// 三个批次的排序因此免于再排。
	visible := make([]contract.ProjectileSnapshot, 0, len(projectiles))
	for _, projectile := range projectiles {
		if server.projectileCandidateVisible(current, projectile) {
			visible = append(visible, projectile)
		}
	}

	// 1) despawn：镜像里登记、当前不可见的个体。命中、寿命耗尽、越出世界、
	// 离开全部订阅区与上限挤占都已由引擎从投影集合移除，走同一条「镜像有而
	// 可见截面无」的判据，不需要单独通道，也不携带移除原因。
	despawned := make([]uint64, 0, len(current.visibleProjectiles))
	for id := range current.visibleProjectiles {
		if projectileIndexOf(visible, id) < 0 {
			despawned = append(despawned, id)
		}
	}
	slices.Sort(despawned)
	if len(despawned) != 0 {
		despawn := network.ProjectileDespawn{ServerTick: tick, IDs: despawned}
		if err := despawn.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(despawn) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
		for _, id := range despawned {
			delete(current.visibleProjectiles, id)
		}
	}

	// 2) 与 3) 同一遍截面扫描：镜像未登记的可见个体进 spawn 批次（携带完整
	// 出生飞行事实：弹种、维度、位置与速度），已登记的进 state 批次（弹种与
	// 维度终身不变，只逐 tick 搬运位置）；刚 spawn 的个体下一 tick 才进入
	// state。两个批次都继承截面的升序。
	spawns := make([]network.ProjectileSpawnRecord, 0, len(visible))
	states := make([]network.ProjectileStateRecord, 0, len(visible))
	for _, projectile := range visible {
		if _, known := current.visibleProjectiles[projectile.ID]; known {
			states = append(states, network.ProjectileStateRecord{
				ID:       projectile.ID,
				Position: projectile.Position,
			})
			continue
		}
		spawns = append(spawns, network.ProjectileSpawnRecord{
			ID:        projectile.ID,
			Kind:      projectile.Kind,
			Dimension: projectile.Dimension,
			Position:  projectile.Position,
			Velocity:  projectile.Velocity,
		})
	}
	if len(spawns) != 0 {
		spawn := network.ProjectileSpawn{ServerTick: tick, Spawns: spawns}
		if err := spawn.Validate(); err != nil {
			server.closePublicationSessionLocked(current, err)
			return false
		}
		if !current.enqueue(spawn) {
			server.closePublicationSessionLocked(current, errSessionOutboxFull)
			return false
		}
		for _, record := range spawns {
			current.visibleProjectiles[record.ID] = struct{}{}
		}
	}
	if len(states) != 0 {
		state := network.ProjectileState{ServerTick: tick, States: states}
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

// projectileCandidateVisible 是投射物的会话可见性判定：所在 chunk 必须仍在
// 会话订阅集合内，且该会话已收到过这份快照（客户端先有世界再有实体，与夜行
// 者/被动牛的判定逐语义一致）。
func (server *Server) projectileCandidateVisible(
	current *session,
	projectile contract.ProjectileSnapshot,
) bool {
	foot := publicationFootChunk(projectile.Dimension, [3]float32(projectile.Position))
	if !server.engine.SessionWantsChunk(current.id, foot) {
		return false
	}
	publication := current.publications[foot]
	return publication != nil && publication.snapshotSent
}

// projectileIndexOf 返回 ID 在升序截面中的下标；未命中返回 -1。线性扫描即可：
// 截面至多 128 条（全服上限），与夜行者侧同形的常数级开销。
func projectileIndexOf(projectiles []contract.ProjectileSnapshot, id uint64) int {
	for index := range projectiles {
		if projectiles[index].ID == id {
			return index
		}
	}
	return -1
}
