// 被动牛按会话订阅发布测试：脚底 chunk 快照送达前不发、进入视野发 spawn、
// 逐 tick 发 state、离开视野发 despawn、未订阅的 chunk 永不发送，以及
// Memory 与 TCP 两条传输对同一世界序列给出相同的发布 transcript（位置与
// 速度只在单传输内断言精确值，跨传输比对只含种类/ID/维度/生命——被动牛每
// tick 按绝对 tick 派生漫游朝向，两条传输登录阶段消耗的 tick 数不同，漫游
// 轨迹天然不可比；字段搬运的精确性由单传输单测与 codec 往返覆盖）。
package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// passivePublicationMob 返回一头字段各异的合法被动牛：生命取非零非满的中
// 间值、朝向非零，保证「字段根本没搬运」与零值不可分辨。
func passivePublicationMob(id uint64, position mgl32.Vec3) contract.PassiveMob {
	return contract.PassiveMob{
		ID:        id,
		Dimension: core.Overworld,
		State:     physics.State{Position: position, OnGround: true},
		Yaw:       0.5,
		Health:    9,
	}
}

func onlyPassiveMessages(messages []network.ServerMessage) []network.ServerMessage {
	result := make([]network.ServerMessage, 0, len(messages))
	for _, message := range messages {
		switch message.(type) {
		case network.PassiveSpawn, network.PassiveState, network.PassiveDespawn:
			result = append(result, message)
		}
	}
	return result
}

func TestPassivePublicationSpawnsAfterFootChunkSnapshotThenStates(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	mob := passivePublicationMob(5, mgl32.Vec3{0.5, 1, 0.5})
	if err := h.running.engine.RestorePassive(mob); err != nil {
		t.Fatalf("RestorePassive: %v", err)
	}

	// 脚底 chunk 的快照尚未送达：被动牛事实必须对客户端隐身。
	h.publish(contract.TickResult{Tick: 10})
	if messages := onlyPassiveMessages(h.drain(1)); len(messages) != 0 {
		t.Fatalf("snapshot 前收到被动牛消息：%#v", messages)
	}

	h.markSnapshotSent(1, core.ChunkPos{})
	h.publish(contract.TickResult{Tick: 11})
	messages := onlyPassiveMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("spawn tick 被动牛消息=%#v，想要恰好 1 条 spawn", messages)
	}
	spawn, ok := messages[0].(network.PassiveSpawn)
	if !ok {
		t.Fatalf("spawn tick 消息类型=%T，想要 PassiveSpawn", messages[0])
	}
	if err := spawn.Validate(); err != nil {
		t.Fatalf("PassiveSpawn.Validate: %v", err)
	}
	if spawn.ServerTick != 11 || len(spawn.Spawns) != 1 {
		t.Fatalf("PassiveSpawn=%+v，想要 tick 11 恰好 1 条记录", spawn)
	}
	if got, want := spawn.Spawns[0], (network.PassiveSpawnRecord{
		ID:        mob.ID,
		Dimension: mob.Dimension,
		Position:  mob.State.Position,
		Yaw:       mob.Yaw,
		Health:    mob.Health,
	}); got != want {
		t.Fatalf("spawn record=%+v，想要权威身体 %+v", got, want)
	}

	// 下一 tick 起只发 state：spawn 不重复，state 携带权威位置/速度/朝向/生命。
	h.publish(contract.TickResult{Tick: 12})
	messages = onlyPassiveMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("稳定 tick 被动牛消息=%#v，想要恰好 1 条 state", messages)
	}
	state, ok := messages[0].(network.PassiveState)
	if !ok {
		t.Fatalf("稳定 tick 消息类型=%T，想要 PassiveState", messages[0])
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("PassiveState.Validate: %v", err)
	}
	if state.ServerTick != 12 || len(state.States) != 1 {
		t.Fatalf("PassiveState=%+v，想要 tick 12 恰好 1 条记录", state)
	}
	if got, want := state.States[0], (network.PassiveStateRecord{
		ID:       mob.ID,
		Position: mob.State.Position,
		Velocity: mob.State.Velocity,
		Yaw:      mob.Yaw,
		Health:   mob.Health,
	}); got != want {
		t.Fatalf("state record=%+v，想要权威身体 %+v", got, want)
	}
	if _, reused := h.running.sessions[1].visiblePassives[mob.ID]; !reused {
		t.Fatal("spawn 后会话镜像未登记该被动牛")
	}
}

// TestPassivePublicationCarriesGrazingFlag 覆盖发布侧的放牧位投影：权威瞬态
// `Grazing` 经 `publishPassives` 逐头映射为 state record 尾部的 0/1 字节，
// 首 tick 的 spawn 不携带该位（出生身体无瞬态），次 tick 起的 state 才携带。
func TestPassivePublicationCarriesGrazingFlag(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	h.markSnapshotSent(1, core.ChunkPos{})
	mobs := []contract.PassiveMob{
		{ID: 5, Dimension: core.Overworld, State: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}, Yaw: 0.5, Health: 9, Grazing: true},
		{ID: 8, Dimension: core.Overworld, State: physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}, Yaw: -0.5, Health: 7},
	}
	current := h.running.sessions[1]
	if !h.running.publishPassives(current, 20, mobs, nil) {
		t.Fatal("首 tick publishPassives 失败")
	}
	if messages := onlyPassiveMessages(h.drain(1)); len(messages) != 1 {
		t.Fatalf("首 tick 被动牛消息=%#v，想要恰好 1 条 spawn", messages)
	}
	if !h.running.publishPassives(current, 21, mobs, nil) {
		t.Fatal("次 tick publishPassives 失败")
	}
	messages := onlyPassiveMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("次 tick 被动牛消息=%#v，想要恰好 1 条 state", messages)
	}
	state, ok := messages[0].(network.PassiveState)
	if !ok {
		t.Fatalf("次 tick 消息类型=%T，想要 PassiveState", messages[0])
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("PassiveState.Validate: %v", err)
	}
	if len(state.States) != 2 || state.States[0].Grazing != 1 || state.States[1].Grazing != 0 {
		t.Fatalf("state 放牧位=%+v，想要 [1 0]", state.States)
	}
}

// TestPassivePublicationProjectsDeathReason 覆盖发布侧的原因位投影：同 tick
// 死亡集合命中的 despawn 原因位置 1，未命中的恒为 0（出视野消失）。
func TestPassivePublicationProjectsDeathReason(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	h.markSnapshotSent(1, core.ChunkPos{})
	mobs := []contract.PassiveMob{
		passivePublicationMob(5, mgl32.Vec3{0.5, 1, 0.5}),
		passivePublicationMob(8, mgl32.Vec3{2.5, 1, 2.5}),
	}
	current := h.running.sessions[1]
	if !h.running.publishPassives(current, 20, mobs, nil) {
		t.Fatal("首 tick publishPassives 失败")
	}
	if messages := onlyPassiveMessages(h.drain(1)); len(messages) != 1 {
		t.Fatalf("首 tick 被动牛消息=%#v，想要恰好 1 条 spawn", messages)
	}
	// 死亡 tick：5 号死亡（快照无、死亡集合命中），8 号单纯出视野（快照无、
	// 死亡集合未命中）。
	if !h.running.publishPassives(current, 21, nil, []uint64{5}) {
		t.Fatal("死亡 tick publishPassives 失败")
	}
	messages := onlyPassiveMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("死亡 tick 被动牛消息=%#v，想要恰好 1 条 despawn", messages)
	}
	despawn, ok := messages[0].(network.PassiveDespawn)
	if !ok {
		t.Fatalf("死亡 tick 消息类型=%T，想要 PassiveDespawn", messages[0])
	}
	if err := despawn.Validate(); err != nil {
		t.Fatalf("PassiveDespawn.Validate: %v", err)
	}
	if len(despawn.Despawns) != 2 {
		t.Fatalf("PassiveDespawn=%+v，想要 2 条记录（ID 升序）", despawn)
	}
	if despawn.Despawns[0].ID != 5 || despawn.Despawns[0].Reason != network.PassiveDespawnDied {
		t.Fatalf("5 号原因位=%+v，想要死亡 %d", despawn.Despawns[0], network.PassiveDespawnDied)
	}
	if despawn.Despawns[1].ID != 8 || despawn.Despawns[1].Reason != network.PassiveDespawnVanished {
		t.Fatalf("8 号原因位=%+v，想要消失 %d", despawn.Despawns[1], network.PassiveDespawnVanished)
	}
}
func TestPassivePublicationUnsubscribedFootChunkNeverSends(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	h.markSnapshotSent(1, core.ChunkPos{})
	// 视距半径 0、兴趣中心 (0,0)：(80.5, z 80.5) 所在 chunk 不在订阅集合。
	if err := h.running.engine.RestorePassive(
		passivePublicationMob(6, mgl32.Vec3{80.5, 1, 80.5})); err != nil {
		t.Fatalf("RestorePassive: %v", err)
	}
	for tick := uint64(10); tick <= 11; tick++ {
		h.publish(contract.TickResult{Tick: tick})
		if messages := onlyPassiveMessages(h.drain(1)); len(messages) != 0 {
			t.Fatalf("未订阅 chunk 收到被动牛消息：%#v", messages)
		}
	}
	if got := len(h.running.sessions[1].visiblePassives); got != 0 {
		t.Fatalf("未订阅 chunk 的会话镜像=%d，想要空", got)
	}
}

func TestPassivePublicationDespawnsOnInterestExit(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	mob := passivePublicationMob(5, mgl32.Vec3{0.5, 1, 0.5})
	if err := h.running.engine.RestorePassive(mob); err != nil {
		t.Fatalf("RestorePassive: %v", err)
	}
	h.markSnapshotSent(1, core.ChunkPos{})
	h.publish(contract.TickResult{Tick: 1})
	if messages := onlyPassiveMessages(h.drain(1)); len(messages) != 1 {
		t.Fatalf("首个可见 tick 消息=%#v，想要 1 条 spawn", messages)
	}

	// 兴趣中心移往相邻 chunk：旧 chunk 退出订阅集合，下一 tick 必须 despawn，
	// 且此后不再收到该 ID 的 state。
	moved := h.moveInterest(1, core.ChunkPos{X: 1})
	h.publish(contract.TickResult{Tick: 2, Forget: moved.Forget})
	messages := onlyPassiveMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("离开视野 tick 消息=%#v，想要 1 条 despawn", messages)
	}
	despawn, ok := messages[0].(network.PassiveDespawn)
	if !ok {
		t.Fatalf("离开视野 tick 消息类型=%T，想要 PassiveDespawn", messages[0])
	}
	if err := despawn.Validate(); err != nil {
		t.Fatalf("PassiveDespawn.Validate: %v", err)
	}
	if despawn.ServerTick != 2 || len(despawn.Despawns) != 1 || despawn.Despawns[0].ID != mob.ID {
		t.Fatalf("PassiveDespawn=%+v，想要 tick 2 只携带 ID %d", despawn, mob.ID)
	}
	if despawn.Despawns[0].Reason != network.PassiveDespawnVanished {
		t.Fatalf("出视野原因位=%d，想要消失 %d", despawn.Despawns[0].Reason, network.PassiveDespawnVanished)
	}
	if _, reused := h.running.sessions[1].visiblePassives[mob.ID]; reused {
		t.Fatal("despawn 后会话镜像未清除该被动牛")
	}

	h.publish(contract.TickResult{Tick: 3})
	if messages := onlyPassiveMessages(h.drain(1)); len(messages) != 0 {
		t.Fatalf("despawn 后仍收到被动牛消息：%#v", messages)
	}
}

// passiveTranscriptRecord 是一次录制里逐 tick 的被动牛消息投影序列。录制从
// 「玩家就绪且视野载入」后恢复被动牛这一刻开始对齐，而不是从连接建立开始：
// 握手与首批区块快照所占的 tick 数随传输而异（TCP 走真实 socket，Memory 走
// 内存管道），绝对 `ServerTick` 因此不可比。除 tick 与漫游过程量（位置、
// 速度、朝向）外的全部 wire 字段（种类、ID、维度、生命）都进入比对，任何一
// 侧漏发、重排或改写一个字段都会让两份录像不相等。
type passiveTranscriptRecord struct {
	Ticks [][]string
}

// passiveParityTicks 是录制长度：首 tick 覆盖 spawn，其后每 tick 一条 state，
// 足以暴露任何一侧的漏发、重排或字段改写。
const passiveParityTicks = 24

// canonicalPassiveEntries 把一个 tick 的被动牛消息压成可比对的规范串列表：
// 消息种类与除漫游过程量外的 record 字段进入比对，`ServerTick` 与位置/速度/
// 朝向除外（见类型注释的对齐论证）。
func canonicalPassiveEntries(messages []network.ServerMessage) []string {
	entries := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message := message.(type) {
		case network.PassiveSpawn:
			for _, record := range message.Spawns {
				entries = append(entries, fmt.Sprintf("spawn id=%d dim=%d health=%d",
					record.ID, record.Dimension, record.Health))
			}
		case network.PassiveState:
			for _, record := range message.States {
				entries = append(entries, fmt.Sprintf("state id=%d health=%d",
					record.ID, record.Health))
			}
		case network.PassiveDespawn:
			for _, record := range message.Despawns {
				entries = append(entries, fmt.Sprintf("despawn id=%d reason=%d", record.ID, record.Reason))
			}
		}
	}
	return entries
}

// TestMemoryTCPPassivePublicationTranscriptParity 覆盖订阅发布契约的传输
// 一致性：同一世界序列（玩家就绪后恢复一头被动牛并逐 tick 推进）在 Memory
// 与 TCP 两条传输下，会话收到的被动牛消息序列逐字段相同（漫游过程量除外，
// 见 `canonicalPassiveEntries` 的论证）。
//
// 夹具把世界拨到夜间相位：被动牛只在白昼自发生成，夜间恢复的个体不受生成
// 判定干扰，身体除漫游外保持生命不变，两侧差异只能来自传输本身。
func TestMemoryTCPPassivePublicationTranscriptParity(t *testing.T) {
	memory := recordPassiveTranscript(t, "memory")
	tcp := recordPassiveTranscript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		for index, memoryTick := range memory.Ticks {
			if !reflect.DeepEqual(tcp.Ticks[index], memoryTick) {
				t.Fatalf("被动牛发布 Memory/TCP 在第 %d 个记录 tick 起不一致\nmemory=%#v\ntcp=%#v",
					index, memoryTick, tcp.Ticks[index])
			}
		}
		t.Fatalf("被动牛发布 Memory/TCP transcript 不一致\nmemory=%#v\ntcp=%#v", memory, tcp)
	}
	spawns, states := 0, 0
	for index, tick := range memory.Ticks {
		for _, entry := range tick {
			switch {
			case len(entry) > 5 && entry[:5] == "spawn":
				spawns++
			case len(entry) > 5 && entry[:5] == "state":
				states++
			default:
				t.Fatalf("第 %d 个记录 tick 出现意外条目 %q", index, entry)
			}
		}
	}
	// 夹具前提守卫：录像必须真的覆盖 spawn 与逐 tick state，否则比对空转。
	if spawns != 1 || states < passiveParityTicks-2 {
		t.Fatalf("录像含 spawn %d 条、state %d 条，想要 1 条与 ≥%d 条（夹具失效）",
			spawns, states, passiveParityTicks-2)
	}
}

func recordPassiveTranscript(t *testing.T, transport string) passiveTranscriptRecord {
	t.Helper()
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	config := hostTestConfig()
	config.ViewRadius = 1
	// 关掉自动存盘：本测试只观察发布路径，存档由被动存档域测试覆盖。
	config.AutosaveTicks = 1 << 30
	host := mustNewHost(t, config, flatTestGenerator{}, store)

	identity := integrationIdentity(0x7d, "PassiveParity")
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	defer func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	}()

	// 夜间相位抑制被动牛自发生成：录像里只有手工恢复的一头牛，spawn 必然
	// 唯一。相位值取夜行者重启用例的同源夜间常量区间内。
	host.world.engine.SetWorldTimeForTest(13000)

	mirror := client.NewMirror()
	record := passiveTranscriptRecord{Ticks: make([][]string, 0, passiveParityTicks)}
	ready := false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s passive parity", transport),
		func() bool { return ready && parityViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v viewLoaded=%v", ready, parityViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
			for _, message := range messages {
				if state, ok := message.(network.PlayerState); ok && state.Ready {
					ready = true
				}
			}
		},
	)

	// 视野对齐后恢复被动牛：世界时间仍在夜间相位（不触发自发生成）。两侧自
	// 此从同一相对时刻开始观察，spawn 必然落在第 0 个记录 tick。
	if err := host.world.engine.RestorePassive(passivePublicationMob(42, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestorePassive: %v", err)
	}

	for range passiveParityTicks {
		_, messages := parityStep(t, host, endpoint, mirror)
		record.Ticks = append(record.Ticks, canonicalPassiveEntries(onlyPassiveMessages(messages)))
	}
	return record
}
