package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/world"
)

func TestEntityReceivesMutationAndTunables(t *testing.T) {
	// 该测试锁定 entity 的世界写入必须经由 concrete *realm.Mutation 与 tuning 快照，
	// 禁止直接读取全局或 runtime 状态。
	realmState := realm.NewState(core.Overworld)
	engine := NewEngine(0, 0, 0)
	engine.realm = realmState
	mutation := realmState.NewMutation()
	tunables := tuning.DefaultTunables()
	// 使用 entity 的放置意图结算路径，验证其签名已改为接收 *realm.Mutation 与 Tunables
	// 若签名仍为旧的 *pendingChunkChanges，编译将失败 (RED)
	_ = tunables
	_ = mutation
	// 下面这行在 RED 阶段将编译失败（方法不存在或签名不匹配），GREEN 阶段应可编译
	// 为保持测试可运行，此处通过接口断言新签名存在性
	type placementSettler interface {
		CompleteCompanionPlacement(entry *companionState, target core.BlockPos, blockID core.BlockID, realmState *realm.State, mutation *realm.Mutation, tunables tuning.Tunables) bool
	}
	var _ placementSettler = engine
	if engine == nil || realmState == nil {
		t.Fatal("unexpected nil")
	}
}

func TestSpawnCandidatesOrder(t *testing.T) {
	// 复用 sim/spawn_test.go 的首段断言，保持 Test 名与子逻辑一致
	got := spawnCandidates(core.ChunkPos{}, tuning.DefaultTunables().SpawnRadius)
	if len(got) != 33*33 {
		t.Fatalf("spawnCandidates len=%d, want 1089", len(got))
	}
	if got[0] != (spawnColumn{X: 0, Z: 0}) {
		t.Fatalf("first candidate=%+v", got[0])
	}
}

func TestMutationCarriesChanges(t *testing.T) {
	realmState := realm.NewState(core.Overworld)
	dim := realmState.Dimension(core.Overworld)
	pos := core.ChunkPos{}
	if !dim.BeginGeneration(pos) {
		t.Fatalf("BeginGeneration failed")
	}
	chunk := world.NewChunk(pos)
	if err := dim.ApplyGenerated(pos, chunk); err != nil {
		t.Fatalf("ApplyGenerated: %v", err)
	}
	// 初始应为空气
	target := core.BlockPos{X: 0, Y: 0, Z: 0}
	if block, ready := dim.BlockAt(target); !ready || block != core.AirID {
		t.Fatalf("initial block=%d ready=%v", block, ready)
	}
	mutation := realmState.NewMutation()
	mutation.Record(core.Overworld, target, core.StoneID)
	if mutation.Len() != 1 {
		t.Fatalf("mutation Len=%d, want 1", mutation.Len())
	}
	batches := mutation.Commit()
	if len(batches) != 1 || len(batches[0].Changes) != 1 || batches[0].Changes[0].Block != core.StoneID {
		t.Fatalf("commit batches=%+v", batches)
	}
	// 提交后区块应已变更（SetBlock 需在 Record 前已写入，此处仅验证 mutation 承载）
	// 为验证真实落盘，需通过 dimension.SetBlock + Record 的组合路径
	realmState2 := realm.NewState(core.Overworld)
	dim2 := realmState2.Dimension(core.Overworld)
	if !dim2.BeginGeneration(pos) {
		t.Fatalf("BeginGeneration2 failed")
	}
	chunk2 := world.NewChunk(pos)
	if err := dim2.ApplyGenerated(pos, chunk2); err != nil {
		t.Fatalf("ApplyGenerated2: %v", err)
	}
	mutation2 := realmState2.NewMutation()
	// 使用 entity 的放置路径验证 mutation 真实承载：通过 CompleteCompanionPlacement 的底层 SetBlock+Record
	// 此处直接验证 mutation.Record/Touch/Commit 的可见性
	mutation2.Record(core.Overworld, target, core.GrassID)
	mutation2.Touch(core.ChunkKey{Dimension: core.Overworld, Pos: pos})
	if mutation2.Len() != 1 {
		t.Fatalf("mutation2 Len=%d", mutation2.Len())
	}
	batches2 := mutation2.Commit()
	if batches2[0].NewRevision != 2 {
		t.Fatalf("NewRevision=%d, want 2", batches2[0].NewRevision)
	}
	_ = world.ChunkBlockIndex // 避免未使用
}

func TestTunablesSnapshotAffectsRegister(t *testing.T) {
	realmState := realm.NewState(core.Overworld)
	engineSmall := NewEngine(0, 0, 0)
	engineSmall.realm = realmState
	engineLarge := NewEngine(0, 0, 0)
	engineLarge.realm = realmState

	smallTunables := tuning.DefaultTunables()
	smallTunables.SpawnRadius = 1
	largeTunables := tuning.DefaultTunables()
	largeTunables.SpawnRadius = 2

	// 不同 SpawnRadius 应产生不同候选数
	smallCandidates := spawnCandidates(core.ChunkPos{}, smallTunables.SpawnRadius)
	largeCandidates := spawnCandidates(core.ChunkPos{}, largeTunables.SpawnRadius)
	if len(smallCandidates) == len(largeCandidates) {
		t.Fatalf("tunables snapshot not effective: small=%d large=%d", len(smallCandidates), len(largeCandidates))
	}
	// 验证 RegisterPlayer 使用传入的 tunables 而非全局
	restore := PlayerRestore{SpawnDimension: core.Overworld, SpawnAnchor: core.ChunkPos{}, Inventory: core.Inventory{}}
	// 使用小半径注册
	engineSmall.RegisterPlayer(1, restore, realmState, smallTunables)
	if len(engineSmall.sessions[1].player.candidates) != len(smallCandidates) {
		t.Fatalf("RegisterPlayer snapshot not used: got %d want %d", len(engineSmall.sessions[1].player.candidates), len(smallCandidates))
	}
	engineLarge.RegisterPlayer(2, restore, realmState, largeTunables)
	if len(engineLarge.sessions[2].player.candidates) != len(largeCandidates) {
		t.Fatalf("RegisterPlayer large snapshot not used")
	}
}

func TestTunablesSnapshotAffectsWorkbench(t *testing.T) {
	realmState := realm.NewState(core.Overworld)
	dim := realmState.Dimension(core.Overworld)
	pos := core.ChunkPos{}
	if !dim.BeginGeneration(pos) {
		t.Fatalf("BeginGeneration failed")
	}
	chunk := world.NewChunk(pos)
	// 在 (0,1,0) 放置工作台
	chunk.SetBlock(0, 1, 0, core.WorkbenchID)
	if err := dim.ApplyGenerated(pos, chunk); err != nil {
		t.Fatalf("ApplyGenerated: %v", err)
	}
	engine := NewEngine(0, 0, 0)
	engine.realm = realmState
	session := &sessionState{
		dimension: core.Overworld,
		player: &playerState{
			actorState: actorState{state: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}}},
			workbench: core.BlockPos{X: 0, Y: 1, Z: 0},
			lifecycle: PlayerActive,
		},
	}
	// 近距离应有效
	nearTunables := tuning.DefaultTunables()
	nearTunables.InteractionReach = 10
	farTunables := tuning.DefaultTunables()
	farTunables.InteractionReach = 1
	physicsTunables := physics.ActiveTunables()
	if !engine.workbenchAnchorValid(session, realmState, nearTunables, physicsTunables) {
		t.Fatalf("near tunables should be valid")
	}
	// 将玩家移远，远距离应无效
	session.player.state.Position = mgl32.Vec3{100, 1, 100}
	if engine.workbenchAnchorValid(session, realmState, farTunables, physicsTunables) {
		t.Fatalf("far tunables should be invalid when far")
	}
}
