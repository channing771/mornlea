package entity

import (
	"crypto/sha256"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

func TestInvalidLatestInputIsAckedAndNeutral(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	moving := onlyMovementPlayer(t, advancePlayerMovementTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1, Mining: true,
	}}))
	if !engine.sessions[session].player.miningHeld {
		t.Fatal("有效输入未保存持续采掘意图")
	}
	onlyMovementPlayer(t, advancePlayerMovementTick(engine, []Command{{
		Session: session, Sequence: 3, Kind: CommandPlayerInput,
	}}))
	if engine.sessions[session].player.miningHeld {
		t.Fatal("中性输入未清空持续采掘意图")
	}
	moving = onlyMovementPlayer(t, advancePlayerMovementTick(engine, []Command{{
		Session: session, Sequence: 4, Kind: CommandPlayerInput, Mining: true,
	}}))

	result := advancePlayerMovementTick(engine, []Command{{
		Session: session, Sequence: 5, Kind: CommandPlayerInput,
		MoveZ: 2, Yaw: float32(math.NaN()), Mining: true,
	}})
	after := onlyMovementPlayer(t, result)
	if after.LastInputSequence != 5 || len(result.Rejected) != 1 ||
		result.Rejected[0] != (Rejection{Session: session, Sequence: 5, Reason: RejectInvalidInput}) {
		t.Fatalf("result=%+v", result)
	}
	if after.State.Position != moving.State.Position || after.State.Velocity != (mgl32.Vec3{}) {
		t.Fatalf("非法最新输入没有改用中立状态: moving=%+v after=%+v", moving.State, after.State)
	}
	if engine.sessions[session].player.miningHeld {
		t.Fatal("非法最新输入未清空持续采掘意图")
	}

	held := onlyMovementPlayer(t, advancePlayerMovementTick(engine, nil))
	if held.State != after.State || held.LastInputSequence != 5 {
		t.Fatalf("非法输入后的 held 状态不是中立: after=%+v held=%+v", after, held)
	}
}

func TestEngineReusesHeldPlayerInputWithoutNewCommand(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	first := onlyMovementPlayer(t, advancePlayerMovementTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1,
	}}))
	second := onlyMovementPlayer(t, advancePlayerMovementTick(engine, nil))

	if second.LastInputSequence != 2 {
		t.Fatalf("没有新输入时 ack=%d，想要 2", second.LastInputSequence)
	}
	if second.State.Position.X() <= first.State.Position.X() {
		t.Fatalf("没有沿用 MoveX held state: first=%+v second=%+v", first.State, second.State)
	}
}

func TestPlayerInputValidationBoundaries(t *testing.T) {
	maxPitch := float32(math.Pi/2 - 0.01)
	invalid := []struct {
		name  string
		input Command
	}{
		{name: "MoveX below", input: Command{MoveX: -2}},
		{name: "MoveX above", input: Command{MoveX: 2}},
		{name: "MoveZ below", input: Command{MoveZ: -2}},
		{name: "MoveZ above", input: Command{MoveZ: 2}},
		{name: "yaw NaN", input: Command{Yaw: float32(math.NaN())}},
		{name: "yaw positive infinity", input: Command{Yaw: float32(math.Inf(1))}},
		{name: "pitch NaN", input: Command{Pitch: float32(math.NaN())}},
		{name: "pitch negative infinity", input: Command{Pitch: float32(math.Inf(-1))}},
		{name: "pitch below", input: Command{Pitch: math.Nextafter32(-maxPitch, float32(math.Inf(-1)))}},
		{name: "pitch above", input: Command{Pitch: math.Nextafter32(maxPitch, float32(math.Inf(1)))}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := readyMovementPlayer(t)
			command := tc.input
			command.Session = session
			command.Sequence = 2
			command.Kind = CommandPlayerInput
			result := advancePlayerMovementTick(engine, []Command{command})
			player := onlyMovementPlayer(t, result)
			if player.LastInputSequence != 2 || len(result.Rejected) != 1 ||
				result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("player=%+v rejected=%+v", player, result.Rejected)
			}
		})
	}

	for _, pitch := range []float32{-maxPitch, maxPitch} {
		engine, session := readyMovementPlayer(t)
		result := advancePlayerMovementTick(engine, []Command{{
			Session: session, Sequence: 2, Kind: CommandPlayerInput,
			MoveX: -1, MoveZ: 1, Yaw: float32(3 * math.Pi), Pitch: pitch,
		}})
		player := onlyMovementPlayer(t, result)
		if len(result.Rejected) != 0 || player.Pitch != pitch ||
			player.Yaw < -float32(math.Pi) || player.Yaw >= float32(math.Pi) ||
			math.Abs(float64(player.Yaw+float32(math.Pi))) > 1e-5 {
			t.Fatalf("合法边界未接受或 yaw 未归一化: player=%+v rejected=%+v", player, result.Rejected)
		}
	}
}

func TestPlayerRecoveryResetsFallAndNonFiniteState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*playerState)
	}{
		{
			name: "feet below recovery floor",
			mutate: func(player *playerState) {
				player.state.Position[1] = float32(core.MinY - 17)
			},
		},
		{
			name: "non-finite velocity",
			mutate: func(player *playerState) {
				player.state.Velocity[0] = float32(math.NaN())
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := readyMovementPlayer(t)
			tc.mutate(engine.sessions[session].player)

			player := onlyMovementPlayer(t, advanceActorsTick(engine))
			wantPosition := mgl32.Vec3{0.5, core.MaxY + 1, 0.5}
			if player.Ready || player.Reset || player.State.Position != wantPosition ||
				!physics.ValidState(player.State) {
				t.Fatalf("不安全状态没有进入有限 PendingSpawn: %+v", player)
			}
		})
	}
}

func TestPlayerRecoveryKeepsInputAckAcrossRespawn(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	if player := onlyMovementPlayer(t, advancePlayerMovementTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1,
	}})); player.LastInputSequence != 2 {
		t.Fatalf("移动输入 ack=%d，想要 2", player.LastInputSequence)
	}
	engine.sessions[session].player.state.Position[1] = float32(core.MinY - 17)
	if pending := onlyMovementPlayer(t, advanceActorsTick(engine)); pending.Ready || pending.LastInputSequence != 2 {
		t.Fatalf("恢复 PendingSpawn 丢失 ack: %+v", pending)
	}
	if respawned := onlyMovementPlayer(t, advanceActorsTick(engine)); !respawned.Ready ||
		respawned.LastInputSequence != 2 {
		t.Fatalf("重新出生后 ack 回退: %+v", respawned)
	}
}

func TestPlayerRecoveryUsesFirstFreeSixteenth(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.state.Position[1] = 15.0 / 16.0
	before := player.state.Position.Y()
	if !engine.tryUnstick(player, engine.dimension(core.Overworld)) {
		t.Fatal("可向上解除的卡入被拒绝")
	}
	if player.state.Position.Y()-before != 1.0/16.0 {
		t.Fatalf("没有采用第一个无碰撞 1/16 位置: before=%v after=%v", before, player.state.Position.Y())
	}

	after := onlyMovementPlayer(t, advanceActorsTick(engine))
	if !after.Ready || after.Reset || after.State.Position.Y() != 1 {
		t.Fatalf("解除卡入后没有保持 Active: %+v", after)
	}
}

func TestPlayerRecoveryResetsUnresolvedOrUnknownOverlap(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*playerState)
	}{
		{
			name: "still intersecting after one block",
			mutate: func(player *playerState) {
				player.state.Position[1] = -1.0 / 16.0
			},
		},
		{
			name: "unknown neighbor",
			mutate: func(player *playerState) {
				player.state.Position[0] = 15.75
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := readyMovementPlayer(t)
			tc.mutate(engine.sessions[session].player)

			player := onlyMovementPlayer(t, advanceActorsTick(engine))
			if player.Ready || player.Reset ||
				player.State.Position != (mgl32.Vec3{0.5, core.MaxY + 1, 0.5}) {
				t.Fatalf("无法解析的卡入没有重置: %+v", player)
			}
		})
	}
}

func TestPlayerHashCoversEveryAuthoritativeField(t *testing.T) {
	engine, sessionID := readyMovementPlayer(t)
	session := engine.sessions[sessionID]
	originalPlayer := *session.player
	originalDimension := session.dimension
	baseline, ok := engine.PlayerHash(sessionID)
	if !ok {
		t.Fatal("权威玩家 hash 不可用")
	}
	if _, ok := engine.PlayerHash(999); ok {
		t.Fatal("未知 session 返回了玩家 hash")
	}

	tests := []struct {
		name   string
		mutate func(*sessionState, *playerState)
	}{
		{name: "dimension", mutate: func(s *sessionState, _ *playerState) { s.dimension = 7 }},
		{name: "lifecycle", mutate: func(_ *sessionState, p *playerState) { p.lifecycle = PlayerPendingSpawn }},
		{name: "position x", mutate: func(_ *sessionState, p *playerState) { p.state.Position[0] += 0.25 }},
		{name: "position y", mutate: func(_ *sessionState, p *playerState) { p.state.Position[1] += 0.25 }},
		{name: "position z", mutate: func(_ *sessionState, p *playerState) { p.state.Position[2] += 0.25 }},
		{name: "velocity x", mutate: func(_ *sessionState, p *playerState) { p.state.Velocity[0] = 1 }},
		{name: "velocity y", mutate: func(_ *sessionState, p *playerState) { p.state.Velocity[1] = 1 }},
		{name: "velocity z", mutate: func(_ *sessionState, p *playerState) { p.state.Velocity[2] = 1 }},
		{name: "on ground", mutate: func(_ *sessionState, p *playerState) { p.state.OnGround = false }},
		{name: "yaw", mutate: func(_ *sessionState, p *playerState) { p.yaw = 1 }},
		{name: "pitch", mutate: func(_ *sessionState, p *playerState) { p.pitch = 1 }},
		{name: "health", mutate: func(_ *sessionState, p *playerState) { p.health = 1 }},
		{name: "input x", mutate: func(_ *sessionState, p *playerState) { p.input.MoveX = 1 }},
		{name: "input z", mutate: func(_ *sessionState, p *playerState) { p.input.MoveZ = 1 }},
		{name: "input jump", mutate: func(_ *sessionState, p *playerState) { p.input.Jump = true }},
		{name: "input yaw", mutate: func(_ *sessionState, p *playerState) { p.input.Yaw = 1 }},
		{name: "last input sequence", mutate: func(_ *sessionState, p *playerState) { p.lastInputSequence = 1 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			player := originalPlayer
			session.player = &player
			session.dimension = originalDimension
			tc.mutate(session, &player)
			got, ok := engine.PlayerHash(sessionID)
			if !ok || got == baseline {
				t.Fatalf("字段变化没有改变 hash: ok=%v hash=%x", ok, got)
			}
		})
	}
	session.player = &originalPlayer
	session.dimension = originalDimension
}

func TestPlayerHashGoldenLittleEndianLayout(t *testing.T) {
	fixture := []byte{
		0x0f, 0x0e, 0x0d, 0x0c, // dimension: 0x0c0d0e0f
		0x01,                   // lifecycle: PlayerActive
		0x0a,                   // health: 10
		0x00, 0x00, 0xa0, 0x3f, // position X: 1.25
		0x00, 0x00, 0x20, 0xc0, // position Y: -2.5
		0x00, 0x00, 0x70, 0x40, // position Z: 3.75
		0x00, 0x00, 0x90, 0xc0, // velocity X: -4.5
		0x00, 0x00, 0xa8, 0x40, // velocity Y: 5.25
		0x00, 0x00, 0xd8, 0xc0, // velocity Z: -6.75
		0x00, 0x00, 0xf0, 0x40, // yaw: 7.5
		0x00, 0x00, 0x04, 0xc1, // pitch: -8.25
		0x01,                   // OnGround: true
		0xf7,                   // input MoveX: -9
		0x0a,                   // input MoveZ: 10
		0x01,                   // input Jump: true
		0x00, 0x00, 0x38, 0x41, // input Yaw: 11.5
		0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, // last input sequence
		0x06,             // hotbar selected: 6
		0x01, 0x00, 0x05, // slot 0: 5 个石头
		0x00, 0x00, 0x00, // slot 1: 空
		0x00, 0x00, 0x00, // slot 2: 空
		0x00, 0x00, 0x00, // slot 3: 空
		0x03, 0x00, 0x40, // slot 4: 64 个草
		0x00, 0x00, 0x00, // slot 5: 空
		0x00, 0x00, 0x00, // slot 6: 空
		0x00, 0x00, 0x00, // slot 7: 空
		0x00, 0x00, 0x00, // slot 8: 空
	}
	// schema v3：27 格空背包直接追加在快捷栏之后。
	fixture = append(fixture, make([]byte, core.BackpackSlots*3)...)
	want := [32]byte{
		0x3c, 0xd7, 0xf2, 0xa1, 0xce, 0x58, 0x6b, 0x07,
		0x0a, 0x7d, 0x18, 0x99, 0x7f, 0xa3, 0xee, 0x19,
		0xd1, 0x76, 0x96, 0x8d, 0x4d, 0xaa, 0x87, 0x3b,
		0x34, 0xab, 0xdd, 0x4a, 0x32, 0x15, 0x36, 0xb7,
	}
	if want := 82 + core.BackpackSlots*3; len(fixture) != want {
		t.Fatalf("fixture 长度=%d，想要 %d", len(fixture), want)
	}
	if digest := sha256.Sum256(fixture); digest != want {
		t.Fatalf("独立 fixture digest=%x，想要 %x", digest, want)
	}

	const sessionID = SessionID(73)
	engine := NewEngine(0, 0, 0)
	engine.sessions[sessionID] = &sessionState{
		dimension: core.DimensionID(0x0c0d0e0f),
		player: &playerState{
			lifecycle: PlayerActive,
			health:    10,
			actorState: actorState{
				state: physics.State{
					Position: mgl32.Vec3{1.25, -2.5, 3.75},
					Velocity: mgl32.Vec3{-4.5, 5.25, -6.75},
					OnGround: true,
				},
				yaw:   7.5,
				pitch: -8.25,
				input: physics.Input{
					MoveX: -9,
					MoveZ: 10,
					Jump:  true,
					Yaw:   11.5,
				},
				inventory: core.Inventory{
					Hotbar: core.Hotbar{
						Selected: 6,
						Slots: [core.HotbarSlots]core.ItemStack{
							0: {Item: core.ItemStone, Count: 5},
							4: {Item: core.ItemGrass, Count: core.MaxStackCount},
						},
					},
				},
			},
			lastInputSequence: 0x1122334455667788,
		},
	}
	got, ok := engine.PlayerHash(sessionID)
	if !ok || got != want {
		t.Fatalf("PlayerHash ok=%v digest=%x，想要 %x", ok, got, want)
	}
}

func readyMovementPlayer(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	return readyMovementPlayerForBench(t)
}

// advancePlayerMovementTick 只把已经按 production 契约筛选的玩家输入交给
// 命令阶段，再推进 actor 物理；它不运行 hostile、环境或世界结算。
func advancePlayerMovementTick(engine *Engine, commands []Command) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands(commands, &tick.result)
	tick.context.AdvanceActors()
	return publishFixture(engine, &tick)
}

// readyMovementPlayerForBench 与 readyMovementPlayer 相同，但接受 testing.TB 以供 benchmark 使用。
func readyMovementPlayerForBench(t testing.TB) (*Engine, SessionID) {
	t.Helper()
	engine := NewEngine(0, 0, 0)
	session := SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	loadMovementChunk(t, engine.dimension(core.Overworld), movementFlatChunk(core.ChunkPos{}))
	spawned := onlyMovementPlayer(t, advanceActorsTick(engine))
	if !spawned.Ready || spawned.State.Position != (mgl32.Vec3{0.5, 1, 0.5}) {
		t.Fatalf("玩家没有在 flat world 激活: %+v", spawned)
	}
	return engine, session
}

func onlyMovementPlayer(t testing.TB, result TickResult) PlayerUpdate {
	t.Helper()
	if len(result.Players) != 1 {
		t.Fatalf("Players=%+v，想要恰好一个", result.Players)
	}
	return result.Players[0]
}

func movementFlatChunk(position core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(position)
	for x := 0; x < core.SectionSize; x++ {
		for z := 0; z < core.SectionSize; z++ {
			chunk.SetBlock(x, 0, z, core.GrassID)
		}
	}
	return chunk
}

func loadMovementChunk(t testing.TB, dimension *Dimension, chunk *world.Chunk) {
	t.Helper()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatalf("区块 %+v 未开始生成", chunk.Pos)
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}
}
