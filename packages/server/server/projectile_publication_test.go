// 投射物按会话订阅发布的单元测试：脚底 chunk 快照送达前不发、进入视野发
// spawn（携带完整飞行事实）、下一 tick 起逐 tick 发 state、离开视野或投射物
// 从权威集合消失（命中/寿命/订阅区/上限挤占共用同一差异判据）发 despawn、
// 未订阅的会话永不收到，以及同一 tick 内 hostile 三包之后紧随投射物三包的
// 固定次序。客户端镜像语义不在本文件（见呈现层测试）。
package server

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// projectilePublicationHurler 构造一只可通过恢复校验的掷骨者：kind 置 1、
// 重规划 tick 推到持久化时间轴远端（追逐编排不派发任何快照）、冷却就绪。
func projectilePublicationHurler(id uint64, position mgl32.Vec3) contract.HostileMob {
	mob := hostilePublicationMob(id, position)
	mob.Kind = contract.HostileKindBoneThrower
	return mob
}

// projectileViewSessionID 是夹具注册的真实引擎会话：harness 的发布会话是
// trusted observer（不进入引擎视图），而引擎侧的「离开全部会话订阅区即消
// 失」判定只认真实视图——没有它，投射物会在生成当 tick 的推进阶段就被引擎
// 移除，发布测试无从观察生命周期。半径 0 中心 (0,0)：只有原点区块内的
// 投射物存活。
const projectileViewSessionID = contract.SessionID(9)

// newProjectilePublicationHarness 在既有发布 harness 上补一个真实引擎视图
// 会话并放行原点区块快照。
func newProjectilePublicationHarness(t *testing.T) *remotePublicationHarness {
	t.Helper()
	h := newRemotePublicationHarness(t, 1)
	h.running.engine.RegisterSession(projectileViewSessionID, core.Overworld, core.ChunkPos{})
	h.markSnapshotSent(1, core.ChunkPos{})
	return h
}

// hurlerShot 让已恢复的掷骨者沿 aim 方向发射一条骨刺并推进一个权威 tick：
// 射击意图经引擎公开 inbox 提交，结算点在敌怪阶段（生成 + 同 tick 首步推进）。
func hurlerShot(h *remotePublicationHarness, id uint64, aim mgl32.Vec3) {
	h.t.Helper()
	if !h.running.engine.EnqueueHostileAction(contract.HostileAction{
		ID:           id,
		RangedAttack: true,
		AimX:         aim.X(),
		AimY:         aim.Y(),
		AimZ:         aim.Z(),
	}) {
		h.t.Fatalf("射击意图入队失败：敌怪 %d", id)
	}
	h.running.engine.Step()
}

func onlyProjectileMessages(messages []network.ServerMessage) []network.ServerMessage {
	result := make([]network.ServerMessage, 0, len(messages))
	for _, message := range messages {
		switch message.(type) {
		case network.ProjectileSpawn, network.ProjectileState, network.ProjectileDespawn:
			result = append(result, message)
		}
	}
	return result
}

func TestProjectilePublicationSpawnsAfterFootChunkSnapshotThenStates(t *testing.T) {
	h := newProjectilePublicationHarness(t)
	if err := h.running.engine.RestoreHostile(
		projectilePublicationHurler(7, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}

	// 脚底 chunk 的快照尚未送达：先放行射击，再验证投射物事实对客户端隐身。
	hurlerShot(h, 7, mgl32.Vec3{1, 0, 0})
	h.running.sessions[1].publications[overworldChunk(core.ChunkPos{})] = &publication{}
	h.publish(contract.TickResult{Tick: 10})
	if messages := onlyProjectileMessages(h.drain(1)); len(messages) != 0 {
		t.Fatalf("snapshot 前收到投射物消息：%#v", messages)
	}

	h.markSnapshotSent(1, core.ChunkPos{})
	h.publish(contract.TickResult{Tick: 11})
	messages := onlyProjectileMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("首个可见 tick 投射物消息=%#v，想要恰好 1 条 spawn", messages)
	}
	spawn, ok := messages[0].(network.ProjectileSpawn)
	if !ok {
		t.Fatalf("首个可见 tick 消息类型=%T，想要 ProjectileSpawn", messages[0])
	}
	if err := spawn.Validate(); err != nil {
		t.Fatalf("ProjectileSpawn.Validate: %v", err)
	}
	projectile := h.running.engine.ProjectilesForTest()
	if len(projectile) != 1 {
		t.Fatalf("权威投射物=%d 条，想要 1 条（夹具失效）", len(projectile))
	}
	if spawn.ServerTick != 11 || len(spawn.Spawns) != 1 {
		t.Fatalf("ProjectileSpawn=%+v，想要 tick 11 恰好 1 条记录", spawn)
	}
	if got, want := spawn.Spawns[0], (network.ProjectileSpawnRecord{
		ID:        projectile[0].ID,
		Kind:      projectile[0].Kind,
		Dimension: projectile[0].Dimension,
		Position:  projectile[0].Position,
		Velocity:  projectile[0].Velocity,
	}); got != want {
		t.Fatalf("spawn record=%+v，想要权威飞行事实 %+v", got, want)
	}
	if got := spawn.Spawns[0].Kind; got != network.ProjectileKindShard {
		t.Fatalf("骨刺 spawn record kind=%d，想要 %d", got, network.ProjectileKindShard)
	}

	// 下一 tick 起只发 state：spawn 不重复，state 携带权威位置。
	h.running.engine.Step()
	h.publish(contract.TickResult{Tick: 12})
	messages = onlyProjectileMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("稳定 tick 投射物消息=%#v，想要恰好 1 条 state", messages)
	}
	state, ok := messages[0].(network.ProjectileState)
	if !ok {
		t.Fatalf("稳定 tick 消息类型=%T，想要 ProjectileState", messages[0])
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("ProjectileState.Validate: %v", err)
	}
	if state.ServerTick != 12 || len(state.States) != 1 {
		t.Fatalf("ProjectileState=%+v，想要 tick 12 恰好 1 条记录", state)
	}
	advanced := h.running.engine.ProjectilesForTest()
	if len(advanced) != 1 {
		t.Fatalf("推进后权威投射物=%d 条，想要 1 条", len(advanced))
	}
	if got, want := state.States[0], (network.ProjectileStateRecord{
		ID:       advanced[0].ID,
		Position: advanced[0].Position,
	}); got != want {
		t.Fatalf("state record=%+v，想要权威位置 %+v", got, want)
	}
	if _, known := h.running.sessions[1].visibleProjectiles[projectile[0].ID]; !known {
		t.Fatal("spawn 后会话镜像未登记该投射物")
	}
}

func TestProjectilePublicationUnsubscribedFootChunkNeverSends(t *testing.T) {
	h := newProjectilePublicationHarness(t)
	// 发布会话兴趣移往 (1,0) 并只放行该 chunk 快照：投射物留在原点区块且
	// 权威集合继续持有（真实视图会话仍订阅 (0,0)），但发布会话不再订阅其
	// 脚底 chunk——纯发布侧可见性门禁。
	moved := h.moveInterest(1, core.ChunkPos{X: 1})
	h.markSnapshotSent(1, core.ChunkPos{X: 1})
	if err := h.running.engine.RestoreHostile(
		projectilePublicationHurler(7, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	hurlerShot(h, 7, mgl32.Vec3{1, 0, 0})
	if got := len(h.running.engine.ProjectilesForTest()); got != 1 {
		t.Fatalf("权威投射物=%d 条，想要 1 条（夹具失效）", got)
	}
	for tick := uint64(10); tick <= 11; tick++ {
		h.publish(contract.TickResult{Tick: tick, Forget: moved.Forget})
		if messages := onlyProjectileMessages(h.drain(1)); len(messages) != 0 {
			t.Fatalf("未订阅 chunk 收到投射物消息：%#v", messages)
		}
	}
	if got := len(h.running.sessions[1].visibleProjectiles); got != 0 {
		t.Fatalf("未订阅 chunk 的会话镜像=%d，想要空", got)
	}
}

func TestProjectilePublicationDespawnsOnInterestExit(t *testing.T) {
	h := newProjectilePublicationHarness(t)
	if err := h.running.engine.RestoreHostile(
		projectilePublicationHurler(7, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	hurlerShot(h, 7, mgl32.Vec3{1, 0, 0})
	id := h.running.engine.ProjectilesForTest()[0].ID
	h.publish(contract.TickResult{Tick: 1})
	if messages := onlyProjectileMessages(h.drain(1)); len(messages) != 1 {
		t.Fatalf("首个可见 tick 消息=%#v，想要 1 条 spawn", messages)
	}

	// 兴趣中心移往相邻 chunk：旧 chunk 退出订阅集合，下一 tick 必须 despawn，
	// 且此后不再收到该 ID 的 state。
	moved := h.moveInterest(1, core.ChunkPos{X: 1})
	h.publish(contract.TickResult{Tick: 2, Forget: moved.Forget})
	messages := onlyProjectileMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("离开视野 tick 消息=%#v，想要 1 条 despawn", messages)
	}
	despawn, ok := messages[0].(network.ProjectileDespawn)
	if !ok {
		t.Fatalf("离开视野 tick 消息类型=%T，想要 ProjectileDespawn", messages[0])
	}
	if err := despawn.Validate(); err != nil {
		t.Fatalf("ProjectileDespawn.Validate: %v", err)
	}
	if despawn.ServerTick != 2 || len(despawn.IDs) != 1 || despawn.IDs[0] != id {
		t.Fatalf("ProjectileDespawn=%+v，想要 tick 2 只携带 ID %d", despawn, id)
	}
	if _, known := h.running.sessions[1].visibleProjectiles[id]; known {
		t.Fatal("despawn 后会话镜像未清除该投射物")
	}

	h.publish(contract.TickResult{Tick: 3})
	if messages := onlyProjectileMessages(h.drain(1)); len(messages) != 0 {
		t.Fatalf("despawn 后仍收到投射物消息：%#v", messages)
	}
}

// TestProjectilePublicationFullLifecycleUntilEngineRemoval 钉住「订阅会话收到
// 完整生命周期」：spawn → 逐 tick 恰一条 state → 投射物飞出全部订阅区被权威
// 集合移除后的 despawn。命中、寿命与上限挤占走同一条「镜像有而截面无」差异
// 判据（引擎侧不再区分移除原因），这里以订阅区消失为代表钉住整条链。
func TestProjectilePublicationFullLifecycleUntilEngineRemoval(t *testing.T) {
	h := newProjectilePublicationHarness(t)
	if err := h.running.engine.RestoreHostile(
		projectilePublicationHurler(7, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	// 沿 +X 全速飞行：约 12 个推进步后越出原点区块（半径 0 的唯一订阅区），
	// 引擎在越界当 tick 移除并随之发布 despawn。
	hurlerShot(h, 7, mgl32.Vec3{1, 0, 0})
	id := h.running.engine.ProjectilesForTest()[0].ID

	sawSpawn, sawDespawn, states := false, false, 0
	for tick := uint64(1); tick <= 40; tick++ {
		h.publish(contract.TickResult{Tick: tick})
		messages := onlyProjectileMessages(h.drain(1))
		if sawDespawn {
			if len(messages) != 0 {
				t.Fatalf("tick %d 在 despawn 后仍收到消息：%#v", tick, messages)
			}
			continue
		}
		for _, message := range messages {
			switch message := message.(type) {
			case network.ProjectileSpawn:
				if sawSpawn || states != 0 {
					t.Fatalf("tick %d 出现迟到的 spawn", tick)
				}
				if err := message.Validate(); err != nil {
					t.Fatalf("ProjectileSpawn.Validate: %v", err)
				}
				sawSpawn = true
			case network.ProjectileState:
				if !sawSpawn {
					t.Fatalf("tick %d 在 spawn 前收到 state", tick)
				}
				if err := message.Validate(); err != nil {
					t.Fatalf("ProjectileState.Validate: %v", err)
				}
				states++
			case network.ProjectileDespawn:
				if !sawSpawn {
					t.Fatalf("tick %d 在 spawn 前收到 despawn", tick)
				}
				if err := message.Validate(); err != nil {
					t.Fatalf("ProjectileDespawn.Validate: %v", err)
				}
				if len(message.IDs) != 1 || message.IDs[0] != id {
					t.Fatalf("despawn=%+v，想要只携带 ID %d", message, id)
				}
				sawDespawn = true
			}
		}
		if !sawDespawn {
			// 尚未消失：推进下一 tick。每 tick 至多一类一包已由上面逐消息
			// 断言隐含（多包会出现多条同型消息）。
			h.running.engine.Step()
		}
	}
	if !sawSpawn || !sawDespawn || states < 4 {
		t.Fatalf("生命周期不完整：spawn=%v state=%d despawn=%v", sawSpawn, states, sawDespawn)
	}
}

// TestProjectilePublicationOrderFollowsHostiles 钉住同一 tick 内两类实体的
// 固定包序：先 hostile 的 despawn→spawn→state，后投射物的 despawn→spawn→
// state。本例用「hostile 与投射物同 tick 首发」覆盖跨族 spawn 先后。
func TestProjectilePublicationOrderFollowsHostiles(t *testing.T) {
	h := newProjectilePublicationHarness(t)
	if err := h.running.engine.RestoreHostile(
		projectilePublicationHurler(7, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	hurlerShot(h, 7, mgl32.Vec3{1, 0, 0})
	h.publish(contract.TickResult{Tick: 1})
	messages := h.drain(1)
	var kinds []string
	for _, message := range messages {
		switch message.(type) {
		case network.HostileSpawn:
			kinds = append(kinds, "hostile-spawn")
		case network.ProjectileSpawn:
			kinds = append(kinds, "projectile-spawn")
		case network.HostileState:
			kinds = append(kinds, "hostile-state")
		case network.ProjectileState:
			kinds = append(kinds, "projectile-state")
		case network.HostileDespawn:
			kinds = append(kinds, "hostile-despawn")
		case network.ProjectileDespawn:
			kinds = append(kinds, "projectile-despawn")
		}
	}
	want := []string{"hostile-spawn", "projectile-spawn"}
	if len(kinds) != len(want) {
		t.Fatalf("实体消息=%v，想要 %v（完整消息=%#v）", kinds, want, messages)
	}
	for index := range want {
		if kinds[index] != want[index] {
			t.Fatalf("实体包序=%v，想要 %v", kinds, want)
		}
	}
}
