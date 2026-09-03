package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

type meleeWireTranscript struct {
	Healths       []uint8
	DeathReset    bool
	RemoteSamples [][3]uint32
	Hits          []network.CombatHit
	SwordStates   []core.ItemStack
	HealthByTick  []uint64
}

func TestPlayerMeleeMemoryTCPWireParity(t *testing.T) {
	memory := runPlayerMeleeWireScript(t, "memory")
	tcp := runPlayerMeleeWireScript(t, "tcp")
	// 去掉绝对 tick 后比较业务 transcript
	normalize := func(tr meleeWireTranscript) meleeWireTranscript {
		if len(tr.Hits) > 0 {
			base := tr.Hits[0].ServerTick
			norm := make([]network.CombatHit, len(tr.Hits))
			for i, hit := range tr.Hits {
				norm[i] = network.CombatHit{ServerTick: hit.ServerTick - base, Damage: hit.Damage, TargetKind: hit.TargetKind}
			}
			tr.Hits = norm
		}
		// HealthByTick 也做相对化以剔除绝对 origin
		if len(tr.HealthByTick) > 0 {
			base := tr.HealthByTick[0]
			for i := range tr.HealthByTick {
				tr.HealthByTick[i] -= base
			}
		}
		// RemoteSamples 不含 tick，直接比较
		return tr
	}
	if !reflect.DeepEqual(normalize(memory), normalize(tcp)) {
		t.Fatalf("近战 Memory/TCP wire transcript 不一致\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	// 校验 hit tick 严格递增且 10-tick 间隔
	for _, tr := range []meleeWireTranscript{memory, tcp} {
		for i := 1; i < len(tr.Hits); i++ {
			if tr.Hits[i].ServerTick <= tr.Hits[i-1].ServerTick {
				t.Fatalf("hit tick 非严格递增: %+v", tr.Hits)
			}
			interval := tr.Hits[i].ServerTick - tr.Hits[i-1].ServerTick
			if interval != 10 {
				t.Fatalf("相邻 hit 间隔=%d want 10, hits=%+v", interval, tr.Hits)
			}
		}
		// 冷却期无确认：已通过严格 10 间隔隐式验证
	}
	if !memory.DeathReset || len(memory.Healths) < 6 {
		t.Fatalf("近战 transcript 未观察到足够伤害和死亡 reset: %+v", memory)
	}
	// 校验剑耐久 progression：durability 2 ->1 -> broken
	if len(memory.SwordStates) < 3 {
		t.Fatalf("剑状态采样不足: %+v", memory.SwordStates)
	}
	// 初始应为铁剑 durability 2
	if memory.SwordStates[0].Item != core.ItemIronSword || memory.SwordStates[0].Durability != 2 {
		t.Fatalf("初始剑状态=%+v want IronSword durability 2", memory.SwordStates[0])
	}
	foundBroken := false
	for _, stack := range memory.SwordStates {
		if stack.Item == core.ItemBrokenIronSword {
			foundBroken = true
			if stack.Durability != 0 || stack.Count != 1 {
				t.Fatalf("损坏剑状态=%+v want Durability 0 Count 1", stack)
			}
		}
	}
	if !foundBroken {
		t.Fatalf("未观察到损坏形态: %+v", memory.SwordStates)
	}
}

func TestEightPlayersSameTickPrimaryInputKeepsSessionOrder(t *testing.T) {
	const seed int64 = 160017
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	memory := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: seed, SpawnDimension: core.Overworld})
	if _, err := memory.SaveBatch(context.Background(), []storage.ChunkSave{{
		Key: key, Revision: 1, Chunk: multiplayerManualGenerator{}.GenerateChunk(key.Pos),
	}}); err != nil {
		t.Fatal(err)
	}
	tracked := &trackedMemoryStore{MemoryStore: memory}
	config := hostTestConfig()
	config.Seed = seed
	config.MaxPlayers = multiplayerClientCount
	config.ViewRadius = 0
	config.OutboxCapacity = 4096
	running := NewWorld(config, multiplayerManualGenerator{}, tracked)
	clients := make([]*multiplayerTCPClient, multiplayerClientCount)
	t.Cleanup(func() {
		for _, connected := range clients {
			if connected != nil {
				_ = closeTask16Client(connected)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), longWaitDeadline)
		defer cancel()
		if err := running.Shutdown(ctx); err != nil {
			t.Errorf("关闭八人同 tick 近战服务端: %v", err)
		}
	})
	// Session 2 先连接，若近战错误地按插入顺序而不是 `SessionID` 处理，下面的
	// wire 采掘分流断言会恰好反转。
	for _, index := range [...]int{1, 0, 2, 3, 4, 5, 6, 7} {
		identity := multiplayerIdentity(byte(0xa0+index), multiplayerNames[index])
		clientEndpoint, serverEndpoint := network.NewMemoryPair(4096)
		if _, err := running.AttachSession(eightMeleeSessionSpec(index, identity, serverEndpoint)); err != nil {
			_ = clientEndpoint.Close()
			_ = serverEndpoint.Close()
			t.Fatalf("AttachSession player %d: %v", index, err)
		}
		clients[index] = &multiplayerTCPClient{
			identity: identity, endpoint: clientEndpoint, receiver: client.NewReceiver(clientEndpoint, 4096),
			mirror: client.NewMirror(), drops: client.NewItemDrops(), remotes: client.NewRemotePlayers(),
		}
	}
	warmCtx, cancelWarm := context.WithTimeout(context.Background(), longWaitDeadline)
	defer cancelWarm()
	for !manualMultiplayerStable(running, tracked, clients, key) {
		result := running.StepForTest()
		drainMultiplayerClientsToTick(t, warmCtx, "eight-melee", clients, result.Tick)
		if err := warmCtx.Err(); err != nil {
			t.Fatalf("eight-melee warm-up: %v", err)
		}
	}
	for index, connected := range clients {
		// 前两个会话共同瞄准第三个；第二个在目标冷却后必须转入既有采掘分支。
		input := network.PlayerInput{Sequence: 1, Yaw: math.Pi, Pitch: -0.2, Mining: true}
		if index%2 == 1 {
			input.Yaw = 0
		}
		if index == 1 {
			input.Yaw = float32(math.Pi - math.Atan2(0.7, 2))
		}
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		err := connected.endpoint.Send(ctx, input)
		cancel()
		if err != nil {
			t.Fatalf("player %d primary input: %v", index, err)
		}
	}
	waitIntegrationCondition(t, "eight melee inputs queued", func() bool { return len(running.incoming) == multiplayerClientCount })
	result := running.StepForTest()
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), waitDeadline)
	drainMultiplayerClientsToTick(t, drainCtx, "eight-melee", clients, result.Tick)
	cancelDrain()
	running.stepMu.Lock()
	sessionCount := len(running.sessions)
	running.stepMu.Unlock()
	if sessionCount != multiplayerClientCount {
		t.Fatalf("同 tick primary action 后 session=%d，想要 %d", sessionCount, multiplayerClientCount)
	}
	for index, connected := range clients {
		if connected.local.LastInputSequence != 1 {
			t.Fatalf("session %d sequence=%d，想要 1", index+1, connected.local.LastInputSequence)
		}
		switch index {
		case 0:
			if connected.local.Health != core.MaxHealth || connected.local.MiningActive {
				t.Fatalf("较小 SessionID 攻击者 state=%+v，想要未受伤且采掘被抑制", connected.local)
			}
		case 1:
			if connected.local.Health != core.MaxHealth || !connected.local.MiningActive ||
				connected.local.MiningTarget != multiplayerManualTarget || connected.local.MiningProgressTicks != 1 {
				t.Fatalf("较大 SessionID 攻击者 state=%+v，想要未受伤且继续采掘", connected.local)
			}
		case 2:
			if connected.local.Health != core.MaxHealth-2 {
				t.Fatalf("共享目标 state=%+v，想要只扣 2 血", connected.local)
			}
		default:
			if connected.local.Health != core.MaxHealth {
				t.Fatalf("容量玩家 %d state=%+v，想要满血", index+1, connected.local)
			}
		}
	}
}

func eightMeleeSessionSpec(index int, identity network.Identity, endpoint network.ServerEndpoint) SessionSpec {
	var position mgl32.Vec3
	if index < 3 {
		// 两名攻击者在第三名玩家前方并列，第二条斜射线在目标之后继续命中固定采掘墙。
		position = mgl32.Vec3{1.5, 1.001, 0.5}
		switch index {
		case 1:
			position[0] = 2.2
		case 2:
			position[2] = 2.5
		}
	} else {
		pair := index / 2
		position = mgl32.Vec3{float32(pair*4) + 0.5, 1.001, 4.5}
		if index%2 == 1 {
			position[2] = 2.5
		}
	}
	location := contract.PlayerLocation{Dimension: core.Overworld, Position: position}
	return SessionSpec{
		ID: contract.SessionID(index + 1), Generation: 1,
		PlayerID: identity.PlayerID, DisplayName: identity.DisplayName, Endpoint: endpoint,
		Restore: contract.PlayerRestore{Current: &location, Safe: &location, SpawnDimension: core.Overworld},
	}
}

func runPlayerMeleeWireScript(t *testing.T, transport string) meleeWireTranscript {
	t.Helper()
	attacker := integrationIdentity(0x91, "MeleeAttacker")
	target := integrationIdentity(0x92, "MeleeTarget")
	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld})
	seedMeleePlayer := func(identity network.Identity, position mgl32.Vec3, inventory core.Inventory) {
		location := storage.PlayerLocation{Dimension: core.Overworld, Position: [3]float32(position)}
		if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
			PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
			Current: location, Safe: &location, Inventory: inventory,
		})); err != nil {
			t.Fatal(err)
		}
	}
	var attackerInv core.Inventory
	attackerInv.Hotbar.Selected = 0
	attackerInv.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 2}
	seedMeleePlayer(attacker, mgl32.Vec3{0.5, 1.001, 4.5}, attackerInv)
	seedMeleePlayer(target, mgl32.Vec3{0.5, 1.001, 2.5}, core.Inventory{})
	config := hostTestConfig()
	config.MaxPlayers = 2
	config.ViewRadius = 0
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, flatGenerator{}, store)
	attackerEndpoint, attackerDone, closeAttackerTransport := openParityTransport(t, host, transport, attacker)
	targetEndpoint, targetDone, closeTargetTransport := openParityTransport(t, host, transport, target)
	t.Cleanup(func() {
		_ = attackerEndpoint.Close()
		_ = targetEndpoint.Close()
		closeAttackerTransport()
		closeTargetTransport()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		for _, done := range []<-chan error{attackerDone, targetDone} {
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, network.ErrClosed) {
					t.Errorf("%s melee accept worker: %v", transport, err)
				}
			case <-ctx.Done():
				t.Errorf("%s melee accept worker 未退出: %v", transport, ctx.Err())
			}
		}
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("%s melee Host.Shutdown: %v", transport, err)
		}
	})

	clients := [2]network.ClientEndpoint{attackerEndpoint, targetEndpoint}
	identities := [2]core.PlayerID{attacker.PlayerID, target.PlayerID}
	ready := [2]bool{}
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s melee wire", transport),
		func() bool { return ready[0] && ready[1] },
		func() string { return fmt.Sprintf("ready=%v", ready) },
		func() {
			states, _ := meleeWireTick(t, host, clients, identities)
			for index, state := range states {
				ready[index] = ready[index] || state.Ready
			}
		},
	)
	remoteReady := false
	for range 10 {
		_, remotes := meleeWireTick(t, host, clients, identities)
		if remotes[0].PlayerID == target.PlayerID && remotes[1].PlayerID == attacker.PlayerID {
			remoteReady = true
			break
		}
	}
	if !remoteReady {
		t.Fatal("近战脚本未从 wire 收到双方远端位置")
	}

	sendIntegration(t, attackerEndpoint, network.PlayerInput{Sequence: 1, Yaw: 0, Pitch: 0, Mining: true})
	waitIntegrationCondition(t, fmt.Sprintf("%s melee input queued", transport), func() bool {
		return len(host.world.incoming) > 0
	})
	result := meleeWireTranscript{Healths: make([]uint8, 0, 10), RemoteSamples: make([][3]uint32, 0, 100), Hits: make([]network.CombatHit, 0, 10), SwordStates: make([]core.ItemStack, 0, 10), HealthByTick: make([]uint64, 0, 100)}
	lastHealth := core.MaxHealth
	// 首个 inventory 是满耐久 2 的铁剑，未磨损前持续采样
	var lastSword core.ItemStack = core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 2}
	result.SwordStates = append(result.SwordStates, lastSword)
	for range 120 {
		tickData := meleeSwordTick(t, host, attackerEndpoint, targetEndpoint, identities)
		state := tickData.targetState
		if state.Health != lastHealth {
			result.Healths = append(result.Healths, state.Health)
			result.HealthByTick = append(result.HealthByTick, tickData.tick)
			lastHealth = state.Health
		}
		// 收集攻击者剑状态：若本 tick 有 InventoryState 更新则记录，否则沿用上一状态并按 tick 采样
		if tickData.attackerInventory != nil {
			inv := tickData.attackerInventory.Inventory
			stack := inv.Hotbar.Slots[inv.Hotbar.Selected]
			lastSword = stack
		}
		result.SwordStates = append(result.SwordStates, lastSword)
		// 收集攻击者 wire 上的 player-kind hit；drain 不能在 PlayerState 停止，必须继续到预期 hit
		if tickData.attackerHit != nil && tickData.attackerHit.TargetKind == core.CombatTargetPlayer {
			result.Hits = append(result.Hits, *tickData.attackerHit)
		} else if tickData.attackerHit != nil {
			t.Fatalf("attacker 收到非 player kind hit: %+v", tickData.attackerHit)
		}
		if tickData.remote.PlayerID == attacker.PlayerID {
			result.RemoteSamples = append(result.RemoteSamples, remotePositionBits(tickData.remote.Position))
		}
		if state.Reset && state.Health == core.MaxHealth {
			result.DeathReset = true
			break
		}
	}
	if !result.DeathReset || len(result.RemoteSamples) == 0 {
		t.Fatalf("%s 近战未在限制 tick 内观察到目标死亡 reset: %+v", transport, result)
	}
	// 校验线上节奏：相邻 hit 间隔 10，冷却期无确认已由严格间隔隐式覆盖
	for i := 1; i < len(result.Hits); i++ {
		if result.Hits[i].ServerTick != result.Hits[i-1].ServerTick+10 {
			t.Fatalf("%s hit 间隔错误: hits=%+v", transport, result.Hits)
		}
		// 冷却期 9 tick 内不应有 hit，间隔 10 已保证
	}
	return result
}

func meleeWireTick(
	t *testing.T,
	host *Host,
	clients [2]network.ClientEndpoint,
	identities [2]core.PlayerID,
) ([2]network.PlayerState, [2]network.RemotePlayerState) {
	t.Helper()
	result := host.world.StepForTest()
	var states [2]network.PlayerState
	var remotes [2]network.RemotePlayerState
	for index, endpoint := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		for {
			message, err := endpoint.Recv(ctx)
			if err != nil {
				cancel()
				t.Fatalf("近战 tick %d player %d Recv: %v", result.Tick, index, err)
			}
			switch message := message.(type) {
			case network.RemotePlayerStates:
				for _, remote := range message.Players {
					if remote.PlayerID == identities[1-index] {
						remotes[index] = remote
					}
				}
			case network.PlayerState:
				cancel()
				if message.ServerTick != result.Tick {
					t.Fatalf("近战 player %d ServerTick=%d，想要 %d", index, message.ServerTick, result.Tick)
				}
				assertValidIntegrationPlayerState(t, message)
				states[index] = message
				goto next
			}
		}
	next:
	}
	return states, remotes
}

type meleeSwordTickData struct {
	tick              uint64
	targetState       network.PlayerState
	remote            network.RemotePlayerState
	attackerInventory *network.InventoryState
	attackerHit       *network.CombatHit
}

func meleeSwordTick(
	t *testing.T,
	host *Host,
	attackerEndpoint, targetEndpoint network.ClientEndpoint,
	identities [2]core.PlayerID,
) meleeSwordTickData {
	t.Helper()
	result := host.world.StepForTest()
	tick := result.Tick
	// 目标端：读取自身 PlayerState（health/velocity）和远端位置
	targetState, targetRemote := drainMeleeTarget(t, targetEndpoint, tick, identities[0])
	// 攻击者端：读取 inventory mirror（selected sword）、自身 PlayerState、以及 player-kind hit
	attackerInv, attackerHit, attackerRemote := drainMeleeAttacker(t, attackerEndpoint, tick, identities[1])
	// attackerRemote 对应第二个远端样本（target 位置），与 targetRemote 应一致，取其一
	_ = attackerRemote
	return meleeSwordTickData{
		tick:              tick,
		targetState:       targetState,
		remote:            targetRemote,
		attackerInventory: attackerInv,
		attackerHit:       attackerHit,
	}
}

func drainMeleeTarget(t *testing.T, endpoint network.ClientEndpoint, tick uint64, attackerID core.PlayerID) (network.PlayerState, network.RemotePlayerState) {
	t.Helper()
	var state network.PlayerState
	var remote network.RemotePlayerState
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	for {
		msg, err := endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("近战 target tick %d Recv: %v", tick, err)
		}
		switch m := msg.(type) {
		case network.RemotePlayerStates:
			for _, r := range m.Players {
				if r.PlayerID == attackerID {
					remote = r
				}
			}
		case network.PlayerState:
			if m.ServerTick != tick {
				continue
			}
			assertValidIntegrationPlayerState(t, m)
			state = m
			return state, remote
		}
	}
}

func drainMeleeAttacker(t *testing.T, endpoint network.ClientEndpoint, tick uint64, targetID core.PlayerID) (*network.InventoryState, *network.CombatHit, network.RemotePlayerState) {
	t.Helper()
	var inv *network.InventoryState
	var hit *network.CombatHit
	var remote network.RemotePlayerState
	var playerStateSeen bool
	// 首阶段：直到 PlayerState 出现，期间收集 InventoryState 和 Remote
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	for !playerStateSeen {
		msg, err := endpoint.Recv(ctx)
		if err != nil {
			cancel()
			t.Fatalf("近战 attacker tick %d Recv before PlayerState: %v", tick, err)
		}
		switch m := msg.(type) {
		case network.InventoryState:
			copy := m
			inv = &copy
		case network.RemotePlayerStates:
			for _, r := range m.Players {
				if r.PlayerID == targetID {
					remote = r
				}
			}
		case network.PlayerState:
			if m.ServerTick != tick {
				continue
			}
			assertValidIntegrationPlayerState(t, m)
			playerStateSeen = true
			// 不立即结束，需继续检查紧随其后的 CombatHit（同一 tick 私发，排在 PlayerState 之后）
		case network.CombatHit:
			// 若 hit 抢在 PlayerState 之前出现（不应发生，但稳妥处理）
			if m.ServerTick == tick {
				copy := m
				hit = &copy
			}
		}
		if playerStateSeen {
			break
		}
	}
	cancel()
	// 第二阶段：drain 不能在 PlayerState 停止，必须继续到预期 hit 数。
	// 同 tick 的 CombatHit 紧随 PlayerState 之后，已就绪的消息会立即返回；无 hit 时短超时后结束。
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer shortCancel()
	for {
		msg, err := endpoint.Recv(shortCtx)
		if err != nil {
			break
		}
		switch m := msg.(type) {
		case network.CombatHit:
			if m.ServerTick == tick {
				copy := m
				hit = &copy
			} else {
				t.Fatalf("attacker hit tick=%d want %d", m.ServerTick, tick)
			}
			// 同 tick 最多一条，捕获后继续短轮询以确认无多余 hit
		case network.InventoryState:
			// 理论上 inventory 在 PlayerState 之前，但若因排序延迟落在之后也捕获
			copy := m
			inv = &copy
		case network.RemotePlayerStates:
			for _, r := range m.Players {
				if r.PlayerID == targetID {
					remote = r
				}
			}
		default:
			// 其他消息不属于本 tick 的 sword 关注面，忽略
		}
		// 若已捕获 hit，继续短轮询一次确认无重复 hit 后由超时结束
		if hit != nil {
			// 再尝试一次短轮询看是否有重复 hit（不应有）
			peekCtx, peekCancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			peekMsg, peekErr := endpoint.Recv(peekCtx)
			peekCancel()
			if peekErr == nil {
				if _, isHit := peekMsg.(network.CombatHit); isHit {
					t.Fatalf("同 tick 收到多条 CombatHit: first %+v second %+v", hit, peekMsg)
				}
			}
			break
		}
		// 若本次短轮询拿到的是非 hit，继续循环等待可能的 hit 直到超时
	}
	return inv, hit, remote
}

func remotePositionBits(position mgl32.Vec3) [3]uint32 {
	return [3]uint32{math.Float32bits(position[0]), math.Float32bits(position[1]), math.Float32bits(position[2])}
}
