// 投射物三消息与敌怪 kind 携带的 Memory/TCP 传输 parity 测试：同一世界种子、
// 同一登录与掷骨者恢复序列、对齐后的同一绝对 tick 窗口内，两条传输逐字段
// 相同的会话发布录像。投射物 ID 与散布由 (seed, 权威 tick) 派生，录像因此以
// 绝对 tick 对齐——握手所占 tick 数随传输而异，先推进到固定起始 tick 再恢复
// 掷骨者，两侧的射击 tick、弹道与全部 wire 字段方可逐位比较。
package server

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// projectileParityStartTick 是录像起始的绝对权威 tick：必须大过任一传输从
// 建立连接到「就绪且视野载入」所需的 tick 数（Memory 传输的区块管线实测
// 需要一百多 tick 且随调度小幅浮动），同时小到整个用例仍在等待预算内推进
// 完成；就绪时刻越过本值的运行会在对齐段被显式拒绝而不是静默错拍。
const projectileParityStartTick = uint64(384)

// projectileParityTicks 是录像长度：覆盖 spawn、命中前的逐 tick state 与
// despawn（骨刺命中附近目标只需个位数 tick），额外余量吸收散布带来的路径差。
const projectileParityTicks = 24

type projectileTranscriptRecord struct {
	Ticks [][]string
}

// canonicalProjectileParityEntries 把一个 tick 的相关消息压成规范串列表：
// 投射物三类消息的全部 wire 字段与敌怪消息的 kind 携带进入比对，
// `ServerTick` 除外（录像已按绝对 tick 对齐，两侧必然相同）。
func canonicalProjectileParityEntries(messages []network.ServerMessage) []string {
	entries := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message := message.(type) {
		case network.ProjectileSpawn:
			for _, record := range message.Spawns {
				entries = append(entries, fmt.Sprintf(
					"projectile spawn id=%d kind=%d dim=%d pos=%v vel=%v",
					record.ID, record.Kind, record.Dimension, record.Position, record.Velocity))
			}
		case network.ProjectileState:
			for _, record := range message.States {
				entries = append(entries, fmt.Sprintf(
					"projectile state id=%d pos=%v", record.ID, record.Position))
			}
		case network.ProjectileDespawn:
			for _, id := range message.IDs {
				entries = append(entries, fmt.Sprintf("projectile despawn id=%d", id))
			}
		case network.HostileSpawn:
			for _, record := range message.Spawns {
				entries = append(entries, fmt.Sprintf(
					"hostile spawn id=%d kind=%d pos=%v health=%d",
					record.ID, record.Kind, record.Position, record.Health))
			}
		case network.HostileState:
			for _, record := range message.States {
				entries = append(entries, fmt.Sprintf(
					"hostile state id=%d kind=%d pos=%v health=%d",
					record.ID, record.Kind, record.Position, record.Health))
			}
		}
	}
	return entries
}

func TestMemoryTCPProjectilePublicationTranscriptParity(t *testing.T) {
	memory := recordProjectileTranscript(t, "memory")
	tcp := recordProjectileTranscript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		for index, memoryTick := range memory.Ticks {
			if !reflect.DeepEqual(tcp.Ticks[index], memoryTick) {
				t.Fatalf("投射物发布 Memory/TCP 在第 %d 个记录 tick 起不一致\nmemory=%#v\ntcp=%#v",
					index, memoryTick, tcp.Ticks[index])
			}
		}
		t.Fatalf("投射物发布 Memory/TCP transcript 不一致\nmemory=%#v\ntcp=%#v", memory, tcp)
	}
	spawns, states, despawns, hostileKindCarry := 0, 0, 0, 0
	for _, tick := range memory.Ticks {
		for _, entry := range tick {
			switch {
			case strings.HasPrefix(entry, "projectile spawn "):
				spawns++
			case strings.HasPrefix(entry, "projectile state "):
				states++
			case strings.HasPrefix(entry, "projectile despawn "):
				despawns++
			case strings.HasPrefix(entry, "hostile spawn ") ||
				strings.HasPrefix(entry, "hostile state "):
				hostileKindCarry++
			default:
				t.Fatalf("录像出现意外条目 %q", entry)
			}
		}
	}
	// 夹具前提守卫：录像必须真的覆盖投射物完整生命周期与敌怪 kind 携带，
	// 否则逐字段比对空转。
	if spawns != 1 || states < 1 || despawns != 1 {
		t.Fatalf("录像含投射物 spawn %d 条、state %d 条、despawn %d 条，想要 1、≥1、1（夹具失效）",
			spawns, states, despawns)
	}
	if hostileKindCarry < 2 {
		t.Fatalf("录像含敌怪 kind 携带条目 %d 条，想要 ≥2（spawn 与 state 各一，夹具失效）",
			hostileKindCarry)
	}
}

func recordProjectileTranscript(t *testing.T, transport string) projectileTranscriptRecord {
	t.Helper()
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 6, Seed: 42, SpawnDimension: core.Overworld,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1 << 30
	host := mustNewHost(t, config, flatTestGenerator{}, store)

	identity := integrationIdentity(0x7d, "ProjectileParity")
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	defer func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	}()

	mirror := client.NewMirror()
	record := projectileTranscriptRecord{Ticks: make([][]string, 0, projectileParityTicks)}
	ready := false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s projectile parity", transport),
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

	// 绝对 tick 对齐：就绪后继续推进到起始 tick 的前一步，两侧世界自同一
	// 种子起经历的 step 数由此相等，投射物 ID、散布与弹道全部可比。就绪若
	// 已越过起始 tick（区块管线异常缓慢），显式失败而不是错拍比对。
	for {
		result, messages := parityStep(t, host, endpoint, mirror)
		if result.Tick >= projectileParityStartTick-1 {
			if result.Tick != projectileParityStartTick-1 {
				t.Fatalf("就绪过晚：对齐推进到 tick %d 仍未停在 %d，起始常量过小",
					result.Tick, projectileParityStartTick-1)
			}
			for _, message := range messages {
				if state, ok := message.(network.PlayerState); ok &&
					state.ServerTick != result.Tick {
					t.Fatalf("对齐尾步消息 tick=%d，想要 %d", state.ServerTick, result.Tick)
				}
			}
			break
		}
	}

	// 掷骨者固定落在原点区块：射击决策与距离带无关（冷却就绪 + 视线通畅即
	// 射），首拍即发射；其 3×3 视线视图与玩家 3×3 订阅重合（两侧都已就绪）。
	// 射击时机、瞄准与散布只由 (seed, tick, 敌怪 ID) 与两侧相同的玩家位置决
	// 定；重规划 tick 推到远端，追逐编排不派发任何 A* 快照。
	hurler := hostilePublicationMob(42, mgl32.Vec3{2.5, 1, 2.5})
	hurler.Kind = contract.HostileKindBoneThrower
	hurler.HasTarget = true
	hurler.PlayerID = identity.PlayerID
	if err := host.world.engine.RestoreHostile(hurler); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}

	for range projectileParityTicks {
		result, messages := parityStep(t, host, endpoint, mirror)
		record.Ticks = append(record.Ticks, canonicalProjectileParityEntries(messages))
		if got := result.Tick; got < projectileParityStartTick ||
			got >= projectileParityStartTick+projectileParityTicks {
			t.Fatalf("录像 tick=%d 越出对齐窗口 [%d,%d)", got,
				projectileParityStartTick, projectileParityStartTick+projectileParityTicks)
		}
	}
	return record
}
