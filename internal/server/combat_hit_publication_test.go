package server

import (
	"context"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestCombatHitPublication(t *testing.T) {
	t.Run("attacker receives inventory then playerstate then combatHit", func(t *testing.T) {
		h := newCombatHitHarness(t, 1, 2, 3)
		attacker := runtime.SessionID(1)
		victim := runtime.SessionID(2)
		bystander := runtime.SessionID(3)
		var inv core.Inventory
		inv.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
		tick := uint64(42)
		result := runtime.TickResult{
			Tick: tick,
			Players: []runtime.PlayerUpdate{
				h.playerUpdate(attacker, tick),
				h.playerUpdate(victim, tick),
				h.playerUpdate(bystander, tick),
			},
			Inventories: []runtime.InventoryUpdate{
				{Session: attacker, Inventory: inv},
			},
			CombatHits: []runtime.CombatHit{
				{Session: attacker, Damage: 6, TargetKind: core.CombatTargetHostile},
			},
		}
		h.publish(result)

		attackerMsgs := h.drain(attacker)
		if len(attackerMsgs) != 3 {
			t.Fatalf("attacker messages=%#v want 3 (InventoryState, PlayerState, CombatHit)", attackerMsgs)
		}
		if _, ok := attackerMsgs[0].(network.InventoryState); !ok {
			t.Fatalf("attacker first message=%T want InventoryState", attackerMsgs[0])
		}
		state, ok := attackerMsgs[1].(network.PlayerState)
		if !ok {
			t.Fatalf("attacker second message=%T want PlayerState", attackerMsgs[1])
		}
		if state.ServerTick != tick {
			t.Fatalf("PlayerState tick=%d want %d", state.ServerTick, tick)
		}
		hit, ok := attackerMsgs[2].(network.CombatHit)
		if !ok {
			t.Fatalf("attacker third message=%T want CombatHit", attackerMsgs[2])
		}
		if hit.ServerTick != tick || hit.Damage != 6 || hit.TargetKind != core.CombatTargetHostile {
			t.Fatalf("CombatHit=%+v want tick %d damage 6 hostile", hit, tick)
		}

		for _, id := range []runtime.SessionID{victim, bystander} {
			msgs := h.drain(id)
			for _, msg := range msgs {
				if _, isHit := msg.(network.CombatHit); isHit {
					t.Fatalf("session %d 意外收到 CombatHit: %+v", id, msg)
				}
			}
		}

		// trusted observer 不得收到
		h.addTrustedObserver(t)
		result2 := runtime.TickResult{
			Tick: tick + 1,
			Players: []runtime.PlayerUpdate{
				h.playerUpdate(attacker, tick+1),
			},
			CombatHits: []runtime.CombatHit{
				{Session: attacker, Damage: 6, TargetKind: core.CombatTargetHostile},
			},
		}
		h.publish(result2)
		trustedMsgs := h.drainTrusted()
		for _, msg := range trustedMsgs {
			if _, isHit := msg.(network.CombatHit); isHit {
				t.Fatalf("trusted observer 意外收到 CombatHit: %+v", msg)
			}
		}
	})

	t.Run("slow session backpressure", func(t *testing.T) {
		h := newCombatHitHarness(t, 1, 2)
		attackerSlow := runtime.SessionID(1)
		attackerHealthy := runtime.SessionID(2)
		h.setOutboxCapacity(attackerSlow, 2)
		h.setOutboxCapacity(attackerHealthy, 32)

		var inv1, inv2 core.Inventory
		inv1.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
		inv2.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
		tick := uint64(100)
		result := runtime.TickResult{
			Tick: tick,
			Players: []runtime.PlayerUpdate{
				h.playerUpdate(attackerSlow, tick),
				h.playerUpdate(attackerHealthy, tick),
			},
			Inventories: []runtime.InventoryUpdate{
				{Session: attackerSlow, Inventory: inv1},
				{Session: attackerHealthy, Inventory: inv2},
			},
			CombatHits: []runtime.CombatHit{
				{Session: attackerSlow, Damage: 6, TargetKind: core.CombatTargetPlayer},
				{Session: attackerHealthy, Damage: 5, TargetKind: core.CombatTargetHostile},
			},
		}
		h.publish(result)

		if h.hasSession(attackerSlow) {
			t.Fatal("slow attacker 应该在 CombatHit enqueue 失败后被 detach")
		}
		if !h.hasSession(attackerHealthy) {
			t.Fatal("healthy attacker 不应被 detach")
		}
		healthyMsgs := h.drain(attackerHealthy)
		foundHit := false
		for _, msg := range healthyMsgs {
			if hit, ok := msg.(network.CombatHit); ok {
				if hit.ServerTick != tick || hit.Damage != 5 || hit.TargetKind != core.CombatTargetHostile {
					t.Fatalf("healthy hit=%+v", hit)
				}
				foundHit = true
			}
		}
		if !foundHit {
			t.Fatalf("healthy attacker 未收到自己的 CombatHit: %#v", healthyMsgs)
		}
	})
}

func TestCombatHitPublicationFiltersBySession(t *testing.T) {
	h := newCombatHitHarness(t, 1, 2, 3)
	tick := uint64(55)
	result := runtime.TickResult{
		Tick: tick,
		Players: []runtime.PlayerUpdate{
			h.playerUpdate(1, tick),
			h.playerUpdate(2, tick),
			h.playerUpdate(3, tick),
		},
		CombatHits: []runtime.CombatHit{
			{Session: 2, Damage: 4, TargetKind: core.CombatTargetHostile},
		},
	}
	h.publish(result)
	for _, id := range []runtime.SessionID{1, 3} {
		for _, msg := range h.drain(id) {
			if _, ok := msg.(network.CombatHit); ok {
				t.Fatalf("session %d 收到不属于自己的 hit", id)
			}
		}
	}
	msgs := h.drain(2)
	found := false
	for _, msg := range msgs {
		if hit, ok := msg.(network.CombatHit); ok {
			if hit.Damage != 4 {
				t.Fatalf("hit damage=%d", hit.Damage)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("session 2 未收到自己的 hit: %#v", msgs)
	}
}

func TestCombatHitUsesTickFromResult(t *testing.T) {
	h := newCombatHitHarness(t, 1)
	tick := uint64(999)
	result := runtime.TickResult{
		Tick: tick,
		Players: []runtime.PlayerUpdate{
			h.playerUpdate(1, tick),
		},
		CombatHits: []runtime.CombatHit{
			{Session: 1, Damage: 6, TargetKind: core.CombatTargetPlayer},
		},
	}
	h.publish(result)
	msgs := h.drain(1)
	var hit network.CombatHit
	found := false
	for _, msg := range msgs {
		if v, ok := msg.(network.CombatHit); ok {
			hit = v
			found = true
		}
	}
	if !found {
		t.Fatalf("未收到 CombatHit")
	}
	if hit.ServerTick != tick {
		t.Fatalf("CombatHit ServerTick=%d want %d", hit.ServerTick, tick)
	}
}

// --- harness ---

type combatHitHarness struct {
	t       *testing.T
	running *Server
}

func newCombatHitHarness(t *testing.T, ids ...runtime.SessionID) *combatHitHarness {
	t.Helper()
	config := DefaultConfig(1)
	config.ViewRadius = 0
	config.SnapshotChunks = 64
	config.SnapshotBytes = 1 << 20
	config.OutboxCapacity = 32
	running := &Server{
		config:         config,
		engine:         runtime.NewEngine(0, 0, 0),
		sessions:       make(map[runtime.SessionID]*session),
		playerSessions: make(map[core.PlayerID]runtime.SessionID),
		lifecycle:      serverRunning,
	}
	h := &combatHitHarness{t: t, running: running}
	for _, id := range ids {
		playerID := publicationPlayerID(byte(id))
		running.engine.RegisterObserverSession(id)
		running.sessions[id] = h.newSession(id, playerID, 32)
		running.playerSessions[playerID] = id
	}
	running.engine.Step()
	t.Cleanup(func() {
		for _, s := range running.sessions {
			s.shutdown()
		}
		if running.trustedObserver != nil {
			running.trustedObserver.shutdown()
		}
	})
	return h
}

func (h *combatHitHarness) newSession(id runtime.SessionID, playerID core.PlayerID, capacity int) *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		id:               id,
		generation:       1,
		playerID:         playerID,
		displayName:      string(rune('A' + byte(id))),
		endpoint:         newBlockingServerEndpoint(),
		ctx:              ctx,
		cancel:           cancel,
		outbox:           make(chan network.ServerMessage, capacity),
		exit:             make(chan SessionExit, 1),
		publications:     make(map[core.ChunkKey]*publication),
		pendingSnapshots: make(map[core.ChunkKey]snapshotRequest),
	}
}

func (h *combatHitHarness) playerUpdate(id runtime.SessionID, tick uint64) runtime.PlayerUpdate {
	update := runtime.PlayerUpdate{
		Session:    id,
		Dimension:  core.Overworld,
		ViewCenter: core.ChunkPos{},
		Ready:      true,
	}
	update.State.Position = mgl32.Vec3{float32(id), 2, 0.5}
	update.Health = core.MaxHealth
	update.WorldTimeTicks = tick * 10
	return update
}

func (h *combatHitHarness) publish(result runtime.TickResult) {
	h.running.publish(result)
}

func (h *combatHitHarness) drain(id runtime.SessionID) []network.ServerMessage {
	current := h.running.sessions[id]
	if current == nil {
		return nil
	}
	msgs := make([]network.ServerMessage, 0, len(current.outbox))
	for len(current.outbox) > 0 {
		msgs = append(msgs, <-current.outbox)
	}
	return msgs
}

func (h *combatHitHarness) drainTrusted() []network.ServerMessage {
	if h.running.trustedObserver == nil {
		return nil
	}
	msgs := make([]network.ServerMessage, 0, len(h.running.trustedObserver.outbox))
	for len(h.running.trustedObserver.outbox) > 0 {
		msgs = append(msgs, <-h.running.trustedObserver.outbox)
	}
	return msgs
}

func (h *combatHitHarness) addTrustedObserver(t *testing.T) *session {
	t.Helper()
	id := trustedObserverSessionID
	playerID := publicationPlayerID(0xFF)
	ctx, cancel := context.WithCancel(context.Background())
	sess := &session{
		id:               id,
		generation:       1,
		playerID:         playerID,
		displayName:      "trusted",
		endpoint:         newBlockingServerEndpoint(),
		ctx:              ctx,
		cancel:           cancel,
		outbox:           make(chan network.ServerMessage, 32),
		exit:             make(chan SessionExit, 1),
		publications:     make(map[core.ChunkKey]*publication),
		pendingSnapshots: make(map[core.ChunkKey]snapshotRequest),
	}
	h.running.trustedObserver = sess
	return sess
}

func (h *combatHitHarness) setOutboxCapacity(id runtime.SessionID, cap int) {
	sess := h.running.sessions[id]
	if sess == nil {
		return
	}
	newOutbox := make(chan network.ServerMessage, cap)
	for len(sess.outbox) > 0 {
		select {
		case msg := <-sess.outbox:
			select {
			case newOutbox <- msg:
			default:
			}
		default:
		}
	}
	sess.outbox = newOutbox
}

func (h *combatHitHarness) hasSession(id runtime.SessionID) bool {
	_, ok := h.running.sessions[id]
	return ok
}
