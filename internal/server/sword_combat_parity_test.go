package server

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
)

type swordCombatTranscript struct {
	Hits              []network.CombatHit
	SwordStates       []core.ItemStack
	HostileHealths    []uint8
	HostileVelocities [][3]uint32
	HealthByTick      []uint64
}

const (
	swordCombatVelocityTolerance = float32(1e-5)
	swordCombatHostileID         = uint64(7)
	swordCombatDeferredRepath    = ^uint64(0)
)

func TestSwordCombatParity(t *testing.T) {
	memory := runSwordCombatWireScript(t, "memory")
	tcp := runSwordCombatWireScript(t, "tcp")
	comparableMemory := memory
	comparableTCP := tcp
	memoryVelocities := comparableMemory.HostileVelocities
	tcpVelocities := comparableTCP.HostileVelocities
	comparableMemory.HostileVelocities = nil
	comparableTCP.HostileVelocities = nil
	if !reflect.DeepEqual(comparableMemory, comparableTCP) {
		t.Fatalf("剑-夜行者 Memory/TCP transcript 不一致\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	if len(memoryVelocities) != len(tcpVelocities) {
		t.Fatalf("剑-夜行者 velocity 样本数不一致: memory=%d tcp=%d", len(memoryVelocities), len(tcpVelocities))
	}
	// 两种传输的 tick 与业务状态必须一致；击退/受击物理中的浮点运算受 goroutine
	// 调度顺序影响，速度只允许吸收远小于一个物理 tick 的数值误差。
	for sample := range memoryVelocities {
		for axis := range 3 {
			got := math.Float32frombits(memoryVelocities[sample][axis])
			want := math.Float32frombits(tcpVelocities[sample][axis])
			if difference := float32(math.Abs(float64(got - want))); difference > swordCombatVelocityTolerance {
				t.Fatalf("velocity[%d][%d]=%g，TCP=%g，difference=%g tolerance=%g", sample, axis, got, want, difference, swordCombatVelocityTolerance)
			}
		}
	}
	for _, tr := range []swordCombatTranscript{memory, tcp} {
		for i := 1; i < len(tr.Hits); i++ {
			if tr.Hits[i].ServerTick <= tr.Hits[i-1].ServerTick {
				t.Fatalf("hit tick 非递增: %+v", tr.Hits)
			}
			if tr.Hits[i].ServerTick-tr.Hits[i-1].ServerTick != 10 {
				t.Fatalf("hit 间隔非 10: %+v", tr.Hits)
			}
			if tr.Hits[i].TargetKind != core.CombatTargetHostile {
				t.Fatalf("hostile hit kind=%d want hostile", tr.Hits[i].TargetKind)
			}
		}
	}
	if len(memory.Hits) < 2 {
		t.Fatalf("夜行者 hit 数不足: %+v", memory.Hits)
	}
	if len(memory.HostileHealths) == 0 {
		t.Fatalf("未采集到夜行者 health")
	}
	foundBroken := false
	for _, stack := range memory.SwordStates {
		if stack.Item == core.ItemBrokenIronSword {
			foundBroken = true
		}
	}
	if !foundBroken {
		t.Fatalf("未观察到铁剑损坏: %+v", memory.SwordStates)
	}
}

func runSwordCombatWireScript(t *testing.T, transport string) swordCombatTranscript {
	t.Helper()
	attacker := integrationIdentity(0x93, "SwordAttacker")
	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld})
	var attackerInv core.Inventory
	attackerInv.Hotbar.Selected = 0
	attackerInv.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 2}
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: [3]float32{0.5, 1.001, 4.5}}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: attacker.PlayerID, Revision: 1, DisplayName: attacker.DisplayName,
		Current: location, Safe: &location, Inventory: attackerInv,
	})); err != nil {
		t.Fatal(err)
	}
	config := hostTestConfig()
	config.MaxPlayers = 1
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, flatGenerator{}, store)
	mob := sim.HostileMob{
		ID:              swordCombatHostileID,
		Dimension:       core.Overworld,
		State:           physics.State{Position: mgl32.Vec3{0.5, 1, 2.5}, Velocity: mgl32.Vec3{0, 0, 0}, OnGround: true},
		Yaw:             0,
		Health:          20,
		BurnCooldown:    20,
		NextRepathTicks: swordCombatDeferredRepath,
	}
	endpoint, done, closeTransport := openParityTransport(t, host, transport, attacker)
	t.Cleanup(func() {
		_ = endpoint.Close()
		closeTransport()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, network.ErrClosed) {
				t.Errorf("%s sword combat accept worker: %v", transport, err)
			}
		case <-ctx.Done():
			t.Errorf("%s sword combat accept worker 未退出: %v", transport, ctx.Err())
		}
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("%s sword combat Host.Shutdown: %v", transport, err)
		}
	})

	ready := false
	for i := 0; i < 100 && !ready; i++ {
		tick := host.world.StepForTest().Tick
		msgs := drainAllForSwordCombatTick(t, endpoint, tick)
		for _, msg := range msgs {
			if state, ok := msg.(network.PlayerState); ok && state.Ready {
				ready = true
			}
		}
	}
	if !ready {
		t.Fatalf("%s sword combat 未就绪", transport)
	}
	if got := host.world.engine.HostileMobs(); len(got) != 0 {
		t.Fatalf("%s sword combat 安装 fixture 前已有 hostile: %+v", transport, got)
	}
	// 登录阶段不放置 hostile，避免异步 A* 完成时机在 fixture 安装前产生
	// transport 相关的追逐积分；最大重规划 tick 让本用例只验证剑战斗、
	// 受击物理与 wire。
	if err := host.world.engine.RestoreHostile(mob); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	spawnSeen := false
	for i := 0; i < 100 && !spawnSeen; i++ {
		tick := host.world.StepForTest().Tick
		for _, msg := range drainAllForSwordCombatTick(t, endpoint, tick) {
			spawn, ok := msg.(network.HostileSpawn)
			if !ok || spawn.ServerTick != tick {
				continue
			}
			for _, record := range spawn.Spawns {
				if record.ID == swordCombatHostileID {
					spawnSeen = true
				}
			}
		}
	}
	if !spawnSeen {
		t.Fatalf("%s sword combat 未收到 fixture spawn", transport)
	}
	for i := 0; i < 5; i++ {
		tick := host.world.StepForTest().Tick
		drainAllForSwordCombatTick(t, endpoint, tick)
	}
	mobs := host.world.engine.HostileMobs()
	if len(mobs) != 1 {
		t.Fatalf("%s sword combat hostile fixture count=%d", transport, len(mobs))
	}
	gotMob := mobs[0]
	if gotMob.ID != mob.ID || gotMob.Dimension != mob.Dimension || gotMob.State != mob.State ||
		gotMob.Health != mob.Health || gotMob.HasTarget ||
		gotMob.PlayerID != (core.PlayerID{}) || gotMob.NextRepathTicks != swordCombatDeferredRepath {
		t.Fatalf("%s sword combat authority fixture 漂移: got=%+v want=%+v", transport, gotMob, mob)
	}
	slot := host.world.hostileManager.slots[mob.ID]
	if slot == nil || slot.pathInFlight || slot.path != nil {
		t.Fatalf("%s sword combat authority fixture 含异步路径状态: %+v", transport, slot)
	}

	input := network.PlayerInput{Sequence: 1, Yaw: 0, Pitch: 0, Mining: true}
	inputCtx, cancelInput := context.WithTimeout(context.Background(), waitDeadline)
	err := host.RunAtInputBoundary(inputCtx, input.Sequence, 1, func() error {
		return endpoint.Send(inputCtx, input)
	})
	cancelInput()
	if err != nil {
		t.Fatalf("%s sword combat input boundary: %v", transport, err)
	}
	sampleBaseTick := host.world.TickCount()

	result := swordCombatTranscript{
		Hits:              make([]network.CombatHit, 0, 10),
		SwordStates:       make([]core.ItemStack, 0, 10),
		HostileHealths:    make([]uint8, 0, 10),
		HostileVelocities: make([][3]uint32, 0, 10),
		HealthByTick:      make([]uint64, 0, 10),
	}
	var lastSword core.ItemStack = core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 2}
	result.SwordStates = append(result.SwordStates, lastSword)
	var lastHealth uint8 = 20
	for range 120 {
		tick := host.world.StepForTest().Tick
		if tick <= sampleBaseTick {
			t.Fatalf("%s sword combat sample tick=%d base=%d", transport, tick, sampleBaseTick)
		}
		relativeTick := tick - sampleBaseTick
		msgs := drainAllForSwordCombatTick(t, endpoint, tick)
		var hit *network.CombatHit
		var inv *network.InventoryState
		var hostileHealth *uint8
		var hostileVel *mgl32.Vec3
		for _, msg := range msgs {
			switch m := msg.(type) {
			case network.CombatHit:
				if m.TargetKind == core.CombatTargetHostile && m.ServerTick == tick {
					copy := m
					copy.ServerTick = relativeTick
					hit = &copy
				}
			case network.InventoryState:
				copy := m
				inv = &copy
			case network.HostileState:
				if m.ServerTick != tick {
					t.Fatalf("%s sword combat hostile state tick=%d want %d", transport, m.ServerTick, tick)
				}
				for _, rec := range m.States {
					if rec.ID == swordCombatHostileID {
						h := rec.Health
						hostileHealth = &h
						v := rec.Velocity
						hostileVel = &v
					}
				}
			}
		}
		if inv != nil {
			stack := inv.Inventory.Hotbar.Slots[inv.Inventory.Hotbar.Selected]
			lastSword = stack
		}
		result.SwordStates = append(result.SwordStates, lastSword)
		if hostileHealth != nil && *hostileHealth != lastHealth {
			result.HostileHealths = append(result.HostileHealths, *hostileHealth)
			result.HealthByTick = append(result.HealthByTick, relativeTick)
			lastHealth = *hostileHealth
			if hostileVel != nil {
				result.HostileVelocities = append(result.HostileVelocities, [3]uint32{math.Float32bits((*hostileVel)[0]), math.Float32bits((*hostileVel)[1]), math.Float32bits((*hostileVel)[2])})
			}
		}
		if hit != nil {
			result.Hits = append(result.Hits, *hit)
		}
		if hit != nil && hostileHealth != nil && *hostileHealth == 0 {
			break
		}
		if len(result.Hits) >= 3 && lastSword.Item == core.ItemBrokenIronSword {
			if len(result.Hits) >= 6 {
				break
			}
		}
	}
	return result
}

func drainAllForSwordCombatTick(t *testing.T, endpoint network.ClientEndpoint, tick uint64) []network.ServerMessage {
	t.Helper()
	msgs := make([]network.ServerMessage, 0, 8)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	seenPlayerState := false
	for !seenPlayerState {
		msg, err := endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("sword combat tick %d Recv: %v", tick, err)
		}
		msgs = append(msgs, msg)
		if state, ok := msg.(network.PlayerState); ok && state.ServerTick == tick {
			seenPlayerState = true
		}
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer shortCancel()
	for {
		msg, err := endpoint.Recv(shortCtx)
		if err != nil {
			break
		}
		msgs = append(msgs, msg)
		if state, ok := msg.(network.PlayerState); ok && state.ServerTick != tick {
			t.Fatalf("sword combat tick %d 收到下一 tick %d 的 PlayerState", tick, state.ServerTick)
		}
		if hit, ok := msg.(network.CombatHit); ok && hit.ServerTick != tick {
			t.Fatalf("sword combat hit tick=%d want %d", hit.ServerTick, tick)
		}
	}
	return msgs
}
