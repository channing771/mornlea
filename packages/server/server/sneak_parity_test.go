package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 潜行全链 parity：潜行前进后服务端权威位置与客户端预测位置必须收敛，
// 悬崖边潜行钳制与非潜行坠落必须在同一世界里对照成立，`Sneaking` 置位的
// `PlayerInput` 在 Memory 与 TCP 双路下必须逐位一致。
//
// 两例测试都只用平地/悬崖两种静态世界：没有草就没有被动牛背景写入，
// 位置轨迹只归因于输入与物理，不掺杂邻居 tick 的背景模拟。
const (
	// sneakParityCliffTicks 是悬崖场景的推进 tick 数：潜行 0.3x 自起点
	// 约 1 米外出发绰绰有余走到边并被钳住，行走对照组则早已坠落。
	sneakParityCliffTicks = 40
	// sneakParityWalkTicks 是双路往返脚本的推进 tick 数。
	sneakParityWalkTicks = 20
	// sneakParityPredictionTolerance 是权威位置与预测位置的容差（米）：
	// 两侧是同一套物理与同一份镜像世界，正常逐位一致，容差只吸收
	// wire float32 的表示误差。
	sneakParityPredictionTolerance = float32(1e-3)
)

// sneakCliffGenerator 是单边悬崖世界：世界 X<2 的柱子是石上覆土的平台
// （顶面 y=1，与出生脚底同高），X>=2 的柱子只有基岩底（其余空气）。
// 悬崖边放在 x=2 而不是 x=0：parity 的视野就绪判据固定检查原点周围
// 3x3 区块，玩家必须出生在 0 号区块内才能收敛。
// 顶面刻意用泥土而不用草：被动牛只在草上出生，无草就没有背景写入
// 污染位置轨迹。
type sneakCliffGenerator struct{}

func (sneakCliffGenerator) GenerateChunk(_ core.DimensionID, position core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(position)
	for z := 0; z < core.SectionSize; z++ {
		for x := 0; x < core.SectionSize; x++ {
			chunk.SetBlock(x, core.MinY, z, core.BedrockID)
			if int(position.X)*core.SectionSize+x < 2 {
				for y := int32(core.MinY + 1); y < 0; y++ {
					chunk.SetBlock(x, y, z, core.StoneID)
				}
				chunk.SetBlock(x, 0, z, core.DirtID)
			}
		}
	}
	chunk.Compact()
	return chunk
}

// sneakParitySession 是 parity 脚本的一次建链结果：登录就绪、镜像加载、
// 出生沉降完成，`start` 是沉降后静止的权威起点。
type sneakParitySession struct {
	host           *Host
	endpoint       network.ClientEndpoint
	acceptDone     <-chan error
	closeTransport func()
	mirror         *client.Mirror
	start          network.PlayerState
}

// setupSneakParitySession 在指定传输上建链并跑完登录就绪与出生沉降。
func setupSneakParitySession(
	t *testing.T,
	label string,
	generator Generator,
	identity network.Identity,
	spawn [3]float32,
	transport string,
) *sneakParitySession {
	t.Helper()
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 6, Seed: 42, SpawnDimension: core.Overworld,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: spawn}
	// 生命取满是承重条件：非满血玩家会自然回血并累积疲劳扣饥饿，
	// 位置轨迹就不纯粹归因于输入与物理。
	save := storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location,
		Health: core.MaxHealth,
	}
	save = wellFedPlayerSave(save)
	if _, err := store.SavePlayer(context.Background(), save); err != nil {
		t.Fatal(err)
	}
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, generator, store)
	endpoint, acceptDone, closeTransport := openParityTransport(t, host, transport, identity)
	session := &sneakParitySession{
		host: host, endpoint: endpoint, acceptDone: acceptDone,
		closeTransport: closeTransport, mirror: client.NewMirror(),
	}
	ready := false
	waitIntegrationLoginReady(
		t,
		label,
		func() bool { return ready && parityViewLoaded(session.mirror) },
		func() string {
			return fmt.Sprintf("ready=%v viewLoaded=%v", ready, parityViewLoaded(session.mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, session.mirror)
			for _, message := range messages {
				if state, ok := message.(network.PlayerState); ok {
					assertValidIntegrationPlayerState(t, state)
					session.start = state
					ready = ready || state.Ready
				}
			}
		},
	)
	// 沉降：就绪循环已把玩家带到地面，再推进 5 个空 tick 让初速归零，
	// 脚本起点是静止站立而不是出生下落途中。
	for range 5 {
		_, messages := parityStep(t, host, endpoint, session.mirror)
		for _, message := range messages {
			if state, ok := message.(network.PlayerState); ok {
				assertValidIntegrationPlayerState(t, state)
				session.start = state
			}
		}
	}
	return session
}

// closeSneakParitySession 按进食 parity 的体例关链：先关 endpoint 等
// accept 退出，再停 Host，最后关传输。
func closeSneakParitySession(t *testing.T, session *sneakParitySession) {
	t.Helper()
	if err := session.endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	select {
	case err := <-session.acceptDone:
		if err != nil && !errors.Is(err, network.ErrClosed) {
			t.Fatalf("sneak parity accept worker: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("sneak parity accept worker did not exit: %v", ctx.Err())
	}
	if err := session.host.Shutdown(ctx); err != nil {
		t.Fatalf("sneak parity Host.Shutdown: %v", err)
	}
	session.closeTransport()
}

// sneakCliffOutcome 是一次悬崖脚本的可比结果：逐 tick 权威脚底位置与
// （潜行组）同输入驱动的客户端预测器终点。
type sneakCliffOutcome struct {
	Start     [3]float32
	Trace     [][3]float32
	Predicted [3]float32
}

// runSneakCliffScript 在悬崖世界朝 +X（崖外）推进：潜行组经真实预测器
// 逐 tick 上行（与线上客户端同路径），对照组只发一条锁存输入后空步。
func runSneakCliffScript(t *testing.T, sneaking bool) sneakCliffOutcome {
	t.Helper()
	name := "SneakCliff"
	last := byte(0xA0)
	if !sneaking {
		name = "WalkCliff"
		last = 0xA1
	}
	session := setupSneakParitySession(t, name, sneakCliffGenerator{},
		integrationIdentity(last, name), [3]float32{1.0, 1.001, 0.5}, "memory")
	defer closeSneakParitySession(t, session)
	outcome := sneakCliffOutcome{Start: [3]float32(session.start.Position)}
	outcome.Trace = make([][3]float32, 0, sneakParityCliffTicks)

	control := client.Control{MoveX: 1, Sneaking: sneaking}
	source := client.MirrorCollisionSource{Mirror: session.mirror, Dimension: core.Overworld}
	var predictor *client.Predictor
	var sequence uint64
	if sneaking {
		predictor = client.NewPredictor()
		if err := predictor.Begin(session.start); err != nil {
			t.Fatalf("Begin sneak predictor: %v", err)
		}
	} else {
		sequence++
		sendIntegration(t, session.endpoint, network.PlayerInput{Sequence: sequence, MoveX: 1})
		waitIntegrationCondition(t, "walk cliff input queued", func() bool {
			return len(session.host.world.incoming) > 0
		})
	}
	for range sneakParityCliffTicks {
		if sneaking {
			// 预测器即客户端：输入经它的 send 回调上行，服务端消费的是
			// 与预测同源的那一条，不存在第二套输入。
			if err := predictor.Advance(physics.FixedDelta, control, source,
				func() uint64 { sequence++; return sequence },
				func(input network.PlayerInput) error {
					sendIntegration(t, session.endpoint, input)
					return nil
				},
			); err != nil {
				t.Fatal(err)
			}
			waitIntegrationCondition(t, "sneak cliff input queued", func() bool {
				return len(session.host.world.incoming) > 0
			})
		}
		_, messages := parityStep(t, session.host, session.endpoint, session.mirror)
		for _, message := range messages {
			if state, ok := message.(network.PlayerState); ok {
				assertValidIntegrationPlayerState(t, state)
				outcome.Trace = append(outcome.Trace, [3]float32(state.Position))
			}
		}
	}
	if sneaking {
		predicted, ok := predictor.State()
		if !ok {
			t.Fatal("潜行预测器在脚本后未就绪")
		}
		outcome.Predicted = [3]float32(predicted.Position)
	}
	return outcome
}

// TestSneakParityPredictionMatchesAuthority 覆盖潜行全链 parity：同一串潜行
// 前进输入下，客户端预测终点与服务端权威终点在容差内；悬崖边潜行 40 tick
// 被钳住不坠落，而不行潜行的对照组坠落。
func TestSneakParityPredictionMatchesAuthority(t *testing.T) {
	sneak := runSneakCliffScript(t, true)
	if len(sneak.Trace) != sneakParityCliffTicks {
		t.Fatalf("潜行轨迹 %d 条，想要 %d", len(sneak.Trace), sneakParityCliffTicks)
	}
	// 夹具自证：起点必须站在平台顶面（y=1），否则"没坠落" vacuous。
	if sneak.Start[1] < 0.9 || sneak.Start[1] > 1.1 {
		t.Fatalf("潜行起点 y=%v，想要站在平台顶面 y=1 附近", sneak.Start[1])
	}
	final := sneak.Trace[len(sneak.Trace)-1]
	for tick, position := range sneak.Trace {
		if position[1] < sneak.Start[1]-0.05 {
			t.Fatalf("潜行第 %d tick y=%v，比起点掉了超过 0.05", tick, position[1])
		}
	}
	if final[0] <= sneak.Start[0]+0.3 {
		t.Fatalf("潜行终点 x=%v，相对起点 %v 前进了不足 0.3 米", final[0], sneak.Start[0])
	}
	if final[0] >= 2.5 {
		t.Fatalf("潜行终点 x=%v，已越过悬崖边（x=2）", final[0])
	}
	drift := sub3(sneak.Predicted, final)
	if drift > sneakParityPredictionTolerance {
		t.Fatalf("预测终点 %v 与权威终点 %v 差 %v 米，超过容差 %v",
			sneak.Predicted, final, drift, sneakParityPredictionTolerance)
	}
	t.Logf("潜行 40 tick：起点 %v 终点 %v，预测/权威差 %v 米", sneak.Start, final, drift)

	walk := runSneakCliffScript(t, false)
	walkFinal := walk.Trace[len(walk.Trace)-1]
	if walkFinal[1] >= walk.Start[1]-1.0 {
		t.Fatalf("对照组终点 y=%v，相对起点 %v 下落不足 1 米（没有坠落）",
			walkFinal[1], walk.Start[1])
	}
	if walkFinal[0] <= 2 {
		t.Fatalf("对照组终点 x=%v，没有越过悬崖边", walkFinal[0])
	}
	t.Logf("对照组 40 tick：起点 %v 终点 %v（坠落）", walk.Start, walkFinal)
}

func sub3(a, b [3]float32) float32 {
	dx := float64(a[0] - b[0])
	dy := float64(a[1] - b[1])
	dz := float64(a[2] - b[2])
	return float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
}

// runSneakWalkScript 在无草平地上发一条锁存的 `PlayerInput` 后推进固定
// tick，返回逐 tick 的权威脚底位置。输入只发一次：权威侧意图在下一条
// `PlayerInput` 到达前保持不变，与进食脚本同形。
func runSneakWalkScript(t *testing.T, transport string, sneaking bool) [][3]float32 {
	t.Helper()
	last := byte(0xB0)
	name := "SneakWalkMemory"
	if transport == "tcp" {
		last = 0xB1
		name = "SneakWalkTCP"
	}
	if !sneaking {
		last = 0xB2
		name = "WalkRefMemory"
	}
	session := setupSneakParitySession(t, name, barrenParityGenerator{},
		integrationIdentity(last, name), [3]float32{0.5, 1.001, 0.5}, transport)
	defer closeSneakParitySession(t, session)
	sendIntegration(t, session.endpoint, network.PlayerInput{
		Sequence: 1, MoveX: 1, Sneaking: sneaking,
	})
	waitIntegrationCondition(t, fmt.Sprintf("%s sneak input queued", transport), func() bool {
		return len(session.host.world.incoming) > 0
	})
	trace := make([][3]float32, 0, sneakParityWalkTicks)
	for range sneakParityWalkTicks {
		_, messages := parityStep(t, session.host, session.endpoint, session.mirror)
		for _, message := range messages {
			if state, ok := message.(network.PlayerState); ok {
				assertValidIntegrationPlayerState(t, state)
				trace = append(trace, [3]float32(state.Position))
			}
		}
	}
	return trace
}

// TestPlayerInputSneakMemoryTCPParity 覆盖潜行输入的传输一致性：`Sneaking`
// 置位的 `PlayerInput` 在 Memory 与 TCP 双路下产生逐位一致的位置轨迹，
// 且潜行位移必须真实生效（大于零、小于同条件行走）。
func TestPlayerInputSneakMemoryTCPParity(t *testing.T) {
	memorySneak := runSneakWalkScript(t, "memory", true)
	tcpSneak := runSneakWalkScript(t, "tcp", true)
	if len(memorySneak) != sneakParityWalkTicks || len(tcpSneak) != sneakParityWalkTicks {
		t.Fatalf("轨迹长度 memory=%d tcp=%d，想要都是 %d",
			len(memorySneak), len(tcpSneak), sneakParityWalkTicks)
	}
	if !reflect.DeepEqual(tcpSneak, memorySneak) {
		t.Fatalf("潜行 Memory/TCP 轨迹未收敛\nmemory=%v\ntcp=%v", memorySneak, tcpSneak)
	}
	sneakDisplacement := memorySneak[len(memorySneak)-1][0] - memorySneak[0][0]
	if sneakDisplacement <= 0 {
		t.Fatalf("潜行位移 %v 米没有前进，比较的是静止轨迹", sneakDisplacement)
	}
	memoryWalk := runSneakWalkScript(t, "memory", false)
	walkDisplacement := memoryWalk[len(memoryWalk)-1][0] - memoryWalk[0][0]
	if sneakDisplacement >= walkDisplacement {
		t.Fatalf("潜行位移 %v 米不小于行走位移 %v 米，潜行位没有生效",
			sneakDisplacement, walkDisplacement)
	}
	t.Logf("20 tick 位移：潜行 %v 米，行走 %v 米，Memory/TCP 逐位一致",
		sneakDisplacement, walkDisplacement)
}
