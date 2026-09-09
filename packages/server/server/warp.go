package server

import (
	"strings"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// parseWarpCommand 解析传送聊天命令：精确等于 `/warp depths` 或
// `/warp overworld`（大小写敏感、无多余参数）才接受，其余一律拒绝。
// 命名空间归属见 `isWarpCommand`：凡是传送命名空间的文本都不进入伙伴寻址。
func parseWarpCommand(text string) (core.DimensionID, bool) {
	switch text {
	case "/warp depths":
		return core.Depths, true
	case "/warp overworld":
		return core.Overworld, true
	default:
		return 0, false
	}
}

// isWarpCommand 报告聊天文本是否属于传送命名空间：精确的 `/warp` 或
// `/warp ` 前缀。命名空间内的拼写错误走传送拒绝而不是伙伴寻址拒绝；
// `/warpspeed` 这类仅有前缀重合的文本仍按普通聊天处理。
func isWarpCommand(text string) bool {
	return text == "/warp" || strings.HasPrefix(text, "/warp ")
}

// handleWarpChat 在持有 `stepMu` 的 tick 边界处理一条传送聊天：解析成功则
// 执行 `warpPlayer`，拼写非法则直发 `CommandRejected`。成功与失败都不产生
// `ChatEvent`，不消耗聊天事件编号。
func (server *Server) handleWarpChat(chat incomingChat) {
	current := server.sessions[chat.sessionID]
	if current == nil || current.closed() {
		return
	}
	target, ok := parseWarpCommand(chat.command.Text)
	if !ok {
		current.enqueue(network.CommandRejected{Reason: network.RejectInvalidInput})
		return
	}
	server.warpPlayer(chat.sessionID, target)
}

// warpPlayer 在持有 `stepMu` 的 tick 边界执行传送事务：校验存活与激活、
// 目标异维、快照旧维状态、注销旧维会话并以同一会话号在目标维重建（复用既有
// 出生扫描、订阅收敛与发布路径）。成功返回 true，落点与 `Reset` 由随后同
// tick 的权威推进与发布下发；任何失败都向会话直发 `CommandRejected` 并返回
// false，玩家保持不动。
//
// 原子性与取舍：
//   - 校验全部发生在提交之前：待出生/死亡（`Ready` 为假）、同维目标、快照
//     缺失一律拒绝且零状态变化；目标锚点区块已明确加载失败（`ChunkFailed`）
//     同样拒绝——有订阅时失败会即刻重试，能稳定观测到的失败即无人问津的
//     确切不可用，重试不可能成功。
//   - 目标锚点区块尚未加载（缺席/加载中/生成中）不拒绝：重建后的待出生保留
//     经既有 `spawnWanted` 把目标维区块暖起来，与登录冷启动同语义；登录从
//     不因冷区块拒绝，传送亦然。
//   - 重建只携带背包、生命、三层饥饿、朝向与个人重生点，不携带旧维当前位置
//     与安全点：旧维候选若被复用会把玩家拉回旧维，传送必须做全新按维扫描。
//     输入状态随重建清零（与登录一致），同 tick 在途输入不再跨维生效。
//   - 伙伴与敌怪不跟随：它们仍锚定旧维，旧维的 `CompanionDespawn` 与
//     `HostileDespawn`、新维不自动镜像都由既有订阅差分自然完成；跟随任务
//     的跨维保持见伙伴编排。
func (server *Server) warpPlayer(session contract.SessionID, target core.DimensionID) bool {
	current := server.sessions[session]
	if current == nil || current.closed() {
		return false
	}
	reject := func(reason network.RejectReason) bool {
		current.enqueue(network.CommandRejected{Reason: reason})
		return false
	}
	update, ok := server.engine.Player(session)
	if !ok || !update.Ready {
		return reject(network.RejectPlayerNotReady)
	}
	if update.Dimension == target {
		return reject(network.RejectInvalidInput)
	}
	oldDimension := update.Dimension
	snapshot, ok := server.engine.PlayerSnapshot(session)
	if !ok {
		return reject(network.RejectPlayerNotReady)
	}
	metadata := server.store.Metadata()
	anchor := metadata.SpawnAnchor
	if target == core.Depths {
		anchor = metadata.DepthsSpawnAnchor
	}
	if info, ok := server.engine.ChunkInfo(core.ChunkKey{
		Dimension: target,
		Pos:       anchor,
	}); ok && info.State == contract.ChunkFailed {
		return reject(network.RejectChunkNotReady)
	}
	if _, ok := server.engine.UnregisterSession(session); !ok {
		return reject(network.RejectPlayerNotReady)
	}
	server.engine.RegisterPlayer(session, contract.PlayerRestore{
		SpawnDimension:   target,
		SpawnAnchor:      anchor,
		Yaw:              snapshot.Yaw,
		Pitch:            snapshot.Pitch,
		Inventory:        snapshot.Inventory,
		Health:           snapshot.Health,
		Hunger:           snapshot.Hunger,
		SaturationMilli:  snapshot.SaturationMilli,
		ExhaustionMilli:  snapshot.ExhaustionMilli,
		HasHunger:        true,
		RespawnPresent:   snapshot.RespawnPresent,
		RespawnPosition:  snapshot.RespawnPosition,
		RespawnDimension: snapshot.RespawnDimension,
	})
	// 注销重建会换掉整份订阅记录，旧维兴趣集合随之消失、收敛时无从差分；
	// 这里按会话已发布镜像显式遗忘旧维区块（同时丢弃其排队中的快照），
	// 与收敛产生的 `Forget` 语义一致。失败只发生在慢客户端关闭路径，
	// 传送本身已提交，沿既有关闭流程处理。
	oldKeys := make([]core.ChunkKey, 0, len(current.publications))
	for key := range current.publications {
		if key.Dimension == oldDimension {
			oldKeys = append(oldKeys, key)
		}
	}
	for _, message := range current.applyForget(oldKeys) {
		if !current.enqueue(message) {
			break
		}
	}
	return true
}
