// 夜行者按会话订阅发布测试：脚底 chunk 快照送达前不发、进入视野发 spawn、
// 逐 tick 发 state、离开视野发 despawn、未订阅的 chunk 永不发送，以及
// Memory 与 TCP 两条传输对同一世界序列给出逐字段相同的发布 transcript。
// 镜像与插值的客户端语义见 `internal/client` 的 presentation 转换测试。
package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// hostilePublicationMob 返回一只字段各异的合法夜行者：生命取非零非满的
// 中间值、朝向非零，保证「字段根本没搬运」与零值不可分辨。追逐事实保持
// 无目标且重规划 tick 推到持久化时间轴的远端，发布测试与传输 parity 因此
// 不受追逐编排派发时序的影响，只观察身体事实的网络呈现。
func hostilePublicationMob(id uint64, position mgl32.Vec3) contract.HostileMob {
	return contract.HostileMob{
		ID:              id,
		Dimension:       core.Overworld,
		State:           physics.State{Position: position, OnGround: true},
		Yaw:             0.25,
		Health:          13,
		BurnCooldown:    20,
		NextRepathTicks: ^uint64(0),
	}
}

func onlyHostileMessages(messages []network.ServerMessage) []network.ServerMessage {
	result := make([]network.ServerMessage, 0, len(messages))
	for _, message := range messages {
		switch message.(type) {
		case network.HostileSpawn, network.HostileState, network.HostileDespawn:
			result = append(result, message)
		}
	}
	return result
}

func TestHostilePublicationSpawnsAfterFootChunkSnapshotThenStates(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	mob := hostilePublicationMob(7, mgl32.Vec3{0.5, 1, 0.5})
	if err := h.running.engine.RestoreHostile(mob); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}

	// 脚底 chunk 的快照尚未送达：夜行者事实必须对客户端隐身。
	h.publish(contract.TickResult{Tick: 10})
	if messages := onlyHostileMessages(h.drain(1)); len(messages) != 0 {
		t.Fatalf("snapshot 前收到夜行者消息：%#v", messages)
	}

	h.markSnapshotSent(1, core.ChunkPos{})
	h.publish(contract.TickResult{Tick: 11})
	messages := onlyHostileMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("spawn tick 夜行者消息=%#v，想要恰好 1 条 spawn", messages)
	}
	spawn, ok := messages[0].(network.HostileSpawn)
	if !ok {
		t.Fatalf("spawn tick 消息类型=%T，想要 HostileSpawn", messages[0])
	}
	if err := spawn.Validate(); err != nil {
		t.Fatalf("HostileSpawn.Validate: %v", err)
	}
	if spawn.ServerTick != 11 || len(spawn.Spawns) != 1 {
		t.Fatalf("HostileSpawn=%+v，想要 tick 11 恰好 1 条记录", spawn)
	}
	if got, want := spawn.Spawns[0], (network.HostileSpawnRecord{
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
	messages = onlyHostileMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("稳定 tick 夜行者消息=%#v，想要恰好 1 条 state", messages)
	}
	state, ok := messages[0].(network.HostileState)
	if !ok {
		t.Fatalf("稳定 tick 消息类型=%T，想要 HostileState", messages[0])
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("HostileState.Validate: %v", err)
	}
	if state.ServerTick != 12 || len(state.States) != 1 {
		t.Fatalf("HostileState=%+v，想要 tick 12 恰好 1 条记录", state)
	}
	if got, want := state.States[0], (network.HostileStateRecord{
		ID:       mob.ID,
		Position: mob.State.Position,
		Velocity: mob.State.Velocity,
		Yaw:      mob.Yaw,
		Health:   mob.Health,
	}); got != want {
		t.Fatalf("state record=%+v，想要权威身体 %+v", got, want)
	}
	if _, reused := h.running.sessions[1].visibleHostiles[mob.ID]; !reused {
		t.Fatal("spawn 后会话镜像未登记该夜行者")
	}
}

func TestHostilePublicationUnsubscribedFootChunkNeverSends(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	h.markSnapshotSent(1, core.ChunkPos{})
	// 视距半径 0、兴趣中心 (0,0)：(80.5, z 80.5) 所在 chunk 不在订阅集合。
	if err := h.running.engine.RestoreHostile(
		hostilePublicationMob(9, mgl32.Vec3{80.5, 1, 80.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	for tick := uint64(10); tick <= 11; tick++ {
		h.publish(contract.TickResult{Tick: tick})
		if messages := onlyHostileMessages(h.drain(1)); len(messages) != 0 {
			t.Fatalf("未订阅 chunk 收到夜行者消息：%#v", messages)
		}
	}
	if got := len(h.running.sessions[1].visibleHostiles); got != 0 {
		t.Fatalf("未订阅 chunk 的会话镜像=%d，想要空", got)
	}
}

func TestHostilePublicationDespawnsOnInterestExit(t *testing.T) {
	h := newRemotePublicationHarness(t, 1)
	mob := hostilePublicationMob(7, mgl32.Vec3{0.5, 1, 0.5})
	if err := h.running.engine.RestoreHostile(mob); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	h.markSnapshotSent(1, core.ChunkPos{})
	h.publish(contract.TickResult{Tick: 1})
	if messages := onlyHostileMessages(h.drain(1)); len(messages) != 1 {
		t.Fatalf("首个可见 tick 消息=%#v，想要 1 条 spawn", messages)
	}

	// 兴趣中心移往相邻 chunk：旧 chunk 退出订阅集合，下一 tick 必须 despawn，
	// 且此后不再收到该 ID 的 state。
	moved := h.moveInterest(1, core.ChunkPos{X: 1})
	h.publish(contract.TickResult{Tick: 2, Forget: moved.Forget})
	messages := onlyHostileMessages(h.drain(1))
	if len(messages) != 1 {
		t.Fatalf("离开视野 tick 消息=%#v，想要 1 条 despawn", messages)
	}
	despawn, ok := messages[0].(network.HostileDespawn)
	if !ok {
		t.Fatalf("离开视野 tick 消息类型=%T，想要 HostileDespawn", messages[0])
	}
	if err := despawn.Validate(); err != nil {
		t.Fatalf("HostileDespawn.Validate: %v", err)
	}
	if despawn.ServerTick != 2 || len(despawn.IDs) != 1 || despawn.IDs[0] != mob.ID {
		t.Fatalf("HostileDespawn=%+v，想要 tick 2 只携带 ID %d", despawn, mob.ID)
	}
	if _, reused := h.running.sessions[1].visibleHostiles[mob.ID]; reused {
		t.Fatal("despawn 后会话镜像未清除该夜行者")
	}

	h.publish(contract.TickResult{Tick: 3})
	if messages := onlyHostileMessages(h.drain(1)); len(messages) != 0 {
		t.Fatalf("despawn 后仍收到夜行者消息：%#v", messages)
	}
}

// hostileTranscriptRecord 是一次录制里逐 tick 的夜行者消息规范序列。录制从
// 「玩家就绪且视野载入」后恢复夜行者这一刻开始对齐，而不是从连接建立开始：
// 握手与首批区块快照所占的 tick 数随传输而异（TCP 走真实 socket，Memory 走
// 内存管道），绝对 `ServerTick` 因此不可比。除 tick 外的全部 wire 字段（种类、
// ID、维度、位置、速度、朝向、生命）都进入比对，任何一侧漏发、重排或改写
// 一个字段都会让两份录像不相等。
type hostileTranscriptRecord struct {
	Ticks [][]string
}

// hostileParityTicks 是录制长度：首 tick 覆盖 spawn，其后每 tick 一条 state，
// 足以暴露任何一侧的漏发、重排或字段改写。
const hostileParityTicks = 24

// canonicalHostileEntries 把一个 tick 的夜行者消息压成可比对的规范串列表：
// 消息种类与 record 的全部字段进入比对，`ServerTick` 除外（见类型注释的对齐
// 论证）。
func canonicalHostileEntries(messages []network.ServerMessage) []string {
	entries := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message := message.(type) {
		case network.HostileSpawn:
			for _, record := range message.Spawns {
				entries = append(entries, fmt.Sprintf("spawn id=%d dim=%d pos=%v yaw=%v health=%d",
					record.ID, record.Dimension, record.Position, record.Yaw, record.Health))
			}
		case network.HostileState:
			for _, record := range message.States {
				entries = append(entries, fmt.Sprintf("state id=%d pos=%v vel=%v yaw=%v health=%d",
					record.ID, record.Position, record.Velocity, record.Yaw, record.Health))
			}
		case network.HostileDespawn:
			for _, id := range message.IDs {
				entries = append(entries, fmt.Sprintf("despawn id=%d", id))
			}
		}
	}
	return entries
}

// TestMemoryTCPHostilePublicationTranscriptParity 覆盖订阅发布契约的传输
// 一致性：同一世界序列（玩家就绪后恢复一只静止夜行者并逐 tick 推进）在
// Memory 与 TCP 两条传输下，会话收到的夜行者消息序列逐字段相同。
//
// 夹具把夜行者的重规划 tick 推到持久化时间轴的远端：追逐编排对它不做任何
// 派发，路径 worker 的应用时序这一非确定源不进入录像，两侧差异只能来自
// 传输本身。
func TestMemoryTCPHostilePublicationTranscriptParity(t *testing.T) {
	memory := recordHostileTranscript(t, "memory")
	tcp := recordHostileTranscript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		for index, memoryTick := range memory.Ticks {
			if !reflect.DeepEqual(tcp.Ticks[index], memoryTick) {
				t.Fatalf("夜行者发布 Memory/TCP 在第 %d 个记录 tick 起不一致\nmemory=%#v\ntcp=%#v",
					index, memoryTick, tcp.Ticks[index])
			}
		}
		t.Fatalf("夜行者发布 Memory/TCP transcript 不一致\nmemory=%#v\ntcp=%#v", memory, tcp)
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
	if spawns != 1 || states < hostileParityTicks-2 {
		t.Fatalf("录像含 spawn %d 条、state %d 条，想要 1 条与 ≥%d 条（夹具失效）",
			spawns, states, hostileParityTicks-2)
	}
}

func recordHostileTranscript(t *testing.T, transport string) hostileTranscriptRecord {
	t.Helper()
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	config := hostTestConfig()
	config.ViewRadius = 1
	// 关掉自动存盘：本测试只观察发布路径，存档由任务组 6 覆盖。
	config.AutosaveTicks = 1 << 30
	host := mustNewHost(t, config, flatTestGenerator{}, store)

	identity := integrationIdentity(0x7c, "HostileParity")
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	defer func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	}()

	mirror := client.NewMirror()
	record := hostileTranscriptRecord{Ticks: make([][]string, 0, hostileParityTicks)}
	ready := false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s hostile parity", transport),
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

	// 视野对齐后恢复夜行者：世界时间仍在夜间相位（不触发自发生成），重规
	// 划 tick 推到远端使追逐编排不派发任何快照，身体除物理沉降外保持静止。
	// 两侧自此从同一相对时刻开始观察，spawn 必然落在第 0 个记录 tick。
	if err := host.world.engine.RestoreHostile(hostilePublicationMob(42, mgl32.Vec3{2.5, 1, 2.5})); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}

	for range hostileParityTicks {
		_, messages := parityStep(t, host, endpoint, mirror)
		record.Ticks = append(record.Ticks, canonicalHostileEntries(onlyHostileMessages(messages)))
	}
	return record
}
