package server

// warp_test.go：权威传送事务的端到端回归：聊天命令触发、tick 内原子搬运、
// 出生扫描落点、物品与生命状态保持、非法传送拒绝与伙伴不跟随。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// warpOverworldAnchor 与 warpDepthsAnchor 是传送夹具的双维出生锚点：刻意取
// 不同区块，落点断言因此能证明出生扫描用了目标维锚点而非复用旧维坐标。
var (
	warpOverworldAnchor = core.ChunkPos{X: 0, Z: 0}
	warpDepthsAnchor    = core.ChunkPos{X: 8, Z: -8}
)

type warpTestPlayer struct {
	session contract.SessionID
	client  network.ClientEndpoint
}

type warpTestHost struct {
	t        *testing.T
	running  *Server
	next     contract.SessionID
	players  []warpTestPlayer
	rejected map[contract.SessionID][]network.CommandRejected
	forgot   map[contract.SessionID][]core.ChunkKey
}

// newWarpTestHost 装配一个双维就绪的内存世界：平坦生成器在两维都提供可站立
// 地表，心跳门调高到小时级避免测试被保活超时干扰。
func newWarpTestHost(t *testing.T, seed int64) *warpTestHost {
	t.Helper()
	return newWarpTestHostWithGenerator(t, seed, playerTestGenerator{})
}

// newWarpTestHostWithGenerator 是 `newWarpTestHost` 的生成器可注入形态，供
// 目标维生成失败的拒绝用例构造确切的 `ChunkFailed` 状态。
func newWarpTestHostWithGenerator(t *testing.T, seed int64, generator Generator) *warpTestHost {
	t.Helper()
	return newWarpTestHostConfigured(t, seed, generator, 1)
}

// newWarpTestHostConfigured 是 `newWarpTestHostWithGenerator` 的视界可注入
// 形态，供声明视距与引擎上界可区分的订阅断言构造夹具。
func newWarpTestHostConfigured(
	t *testing.T,
	seed int64,
	generator Generator,
	viewRadius int,
) *warpTestHost {
	t.Helper()
	store := storage.NewMemory(storage.Metadata{
		FormatVersion:     6,
		Seed:              seed,
		SpawnDimension:    core.Overworld,
		SpawnAnchor:       warpOverworldAnchor,
		DepthsSpawnAnchor: warpDepthsAnchor,
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	config := DefaultConfig(seed)
	config.ViewRadius = viewRadius
	config.Workers = 1
	config.HeartbeatInterval = time.Hour
	config.HeartbeatTimeout = time.Hour
	running := NewWorld(config, generator, store)
	t.Cleanup(func() { shutdownServerForTest(t, running) })
	return &warpTestHost{t: t, running: running, rejected: make(map[contract.SessionID][]network.CommandRejected), forgot: make(map[contract.SessionID][]core.ChunkKey)}
}

func warpAnchorFor(dimension core.DimensionID) core.ChunkPos {
	if dimension == core.Depths {
		return warpDepthsAnchor
	}
	return warpOverworldAnchor
}

// SpawnPlayer 接入一名指定维度的玩家并推进到出生就绪。
func (host *warpTestHost) SpawnPlayer(t *testing.T, dimension core.DimensionID) warpTestPlayer {
	t.Helper()
	return host.SpawnPlayerWithRestore(t, contract.PlayerRestore{
		SpawnDimension: dimension,
		SpawnAnchor:    warpAnchorFor(dimension),
	})
}

// SpawnPlayerWithRestore 是 `SpawnPlayer` 的恢复载荷可注入形态，供登录协商
// 事实（如声明视距）随接入注入。
func (host *warpTestHost) SpawnPlayerWithRestore(
	t *testing.T,
	restore contract.PlayerRestore,
) warpTestPlayer {
	t.Helper()
	dimension := restore.SpawnDimension
	client, endpoint := network.NewMemoryPair(64)
	host.next++
	spec := registrySessionSpecWithRestore(host.next, 1, endpoint, restore)
	if _, err := host.running.AttachSession(spec); err != nil {
		t.Fatalf("接入维度 %d 会话: %v", dimension, err)
	}
	player := warpTestPlayer{session: host.next, client: client}
	host.players = append(host.players, player)
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		state := host.PlayerState(t, player)
		if state.Ready && state.Dimension == dimension {
			return player
		}
		host.Step(t)
	}
	t.Fatalf("维度 %d 玩家未在时限内出生就绪", dimension)
	return player
}

// SendChat 把聊天文本直投权威队列，绕开传输 reader 的异步投递。
func (host *warpTestHost) SendChat(t *testing.T, player warpTestPlayer, text string) {
	t.Helper()
	host.running.enqueueIncomingChat(context.Background(), incomingChat{
		sessionID:  player.session,
		generation: 1,
		command:    network.ChatCommand{Text: text},
	})
}

// Step 推进一个完整权威 tick 并返回其结果，同时排空全部测试客户端的
// 出盒：不断言的发布若不消费会撑满慢客户端队列导致会话被关闭。
func (host *warpTestHost) Step(t *testing.T) contract.TickResult {
	t.Helper()
	result := host.running.StepForTest()
	host.drain()
	return result
}

// drain 非阻塞地排空全部测试客户端的出盒：拒绝与遗忘消息暂存 backlog 供
// 断言消费，其余发布直接丢弃。
func (host *warpTestHost) drain() {
	for _, player := range host.players {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			message, err := player.client.Recv(ctx)
			cancel()
			if err != nil {
				break
			}
			switch message := message.(type) {
			case network.CommandRejected:
				host.rejected[player.session] = append(host.rejected[player.session], message)
			case network.ForgetChunks:
				host.forgot[player.session] = append(host.forgot[player.session], forgetKeys(message)...)
			}
		}
	}
}

func forgetKeys(message network.ForgetChunks) []core.ChunkKey {
	keys := make([]core.ChunkKey, 0, len(message.Chunks))
	for _, pos := range message.Chunks {
		keys = append(keys, core.ChunkKey{Dimension: message.Dimension, Pos: pos})
	}
	return keys
}

// ExpectForget 消费 backlog 并断言旧维遗忘：非空且全部属于旧维。
func (host *warpTestHost) ExpectForget(
	t *testing.T,
	player warpTestPlayer,
	oldDimension core.DimensionID,
) {
	t.Helper()
	keys := host.forgot[player.session]
	delete(host.forgot, player.session)
	if len(keys) == 0 {
		t.Fatal("传送后未收到旧维 ForgetChunks")
	}
	for _, key := range keys {
		if key.Dimension != oldDimension {
			t.Fatalf("误遗忘非旧维区块 %+v", key)
		}
	}
}

// PlayerState 读取指定会话的最新权威玩家更新。
func (host *warpTestHost) PlayerState(t *testing.T, player warpTestPlayer) contract.PlayerUpdate {
	t.Helper()
	host.running.stepMu.Lock()
	defer host.running.stepMu.Unlock()
	update, ok := host.running.engine.Player(player.session)
	if !ok {
		t.Fatalf("会话 %d 无权威玩家状态", player.session)
	}
	return update
}

// PlayerSnapshot 读取指定会话的最新权威玩家快照（含背包与生命状态）。
func (host *warpTestHost) PlayerSnapshot(
	t *testing.T,
	player warpTestPlayer,
) contract.PlayerSnapshot {
	t.Helper()
	snapshot, ok := host.running.PlayerSnapshotFor(player.session)
	if !ok {
		t.Fatalf("会话 %d 无权威玩家快照", player.session)
	}
	return snapshot
}

// SetInventory 原子改写指定会话玩家的权威背包。
func (host *warpTestHost) SetInventory(
	t *testing.T,
	player warpTestPlayer,
	mutate func(core.Inventory) core.Inventory,
) {
	t.Helper()
	host.running.stepMu.Lock()
	defer host.running.stepMu.Unlock()
	host.running.engine.SetPlayerInventoryForTest(player.session, mutate)
}

// StepUntilDimension 推进到玩家在目标维就绪，返回途中是否见过 `Reset` 置位。
func (host *warpTestHost) StepUntilDimension(
	t *testing.T,
	player warpTestPlayer,
	dimension core.DimensionID,
) bool {
	t.Helper()
	sawReset := false
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		result := host.Step(t)
		for _, update := range result.Players {
			if update.Session != player.session {
				continue
			}
			if update.Dimension == dimension && update.Reset {
				sawReset = true
			}
			if update.Dimension == dimension && update.Ready {
				return sawReset
			}
		}
	}
	state := host.PlayerState(t, player)
	t.Fatalf(
		"传送后未在时限内落到维度 %d（当前维度 %d 就绪 %v）",
		dimension, state.Dimension, state.Ready,
	)
	return false
}

// ExpectCommandRejected 在客户端出盒里等待一条拒绝并返回其原因：先消费
// `Step` 期排空时暂存的 backlog，再实时接收。
func (host *warpTestHost) ExpectCommandRejected(
	t *testing.T,
	player warpTestPlayer,
) network.RejectReason {
	t.Helper()
	if pending := host.rejected[player.session]; len(pending) != 0 {
		host.rejected[player.session] = pending[1:]
		return pending[0].Reason
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		message, err := player.client.Recv(ctx)
		if err != nil {
			t.Fatalf("等待拒绝时接收失败: %v", err)
		}
		if rejected, ok := message.(network.CommandRejected); ok {
			return rejected.Reason
		}
	}
}

func TestParseWarpCommand(t *testing.T) {
	cases := []struct {
		text      string
		dimension core.DimensionID
		ok        bool
	}{
		{text: "/warp depths", dimension: core.Depths, ok: true},
		{text: "/warp overworld", dimension: core.Overworld, ok: true},
		{text: "/warp Depths"},
		{text: "/warp DEPTHS"},
		{text: "/warp depths "},
		{text: "/warp  depths"},
		{text: "/warp"},
		{text: "/warp nether"},
		{text: "/warp depths extra"},
		{text: "warp depths"},
		{text: "/warpspeed"},
		{text: ""},
	}
	for _, test := range cases {
		dimension, ok := parseWarpCommand(test.text)
		if ok != test.ok || (ok && dimension != test.dimension) {
			t.Fatalf(
				"parseWarpCommand(%q) = (%d, %v)，想要 (%d, %v)",
				test.text, dimension, ok, test.dimension, test.ok,
			)
		}
	}
}

func TestWarpOverworldToDepthsRoundTrip(t *testing.T) {
	host := newWarpTestHost(t, 42)
	player := host.SpawnPlayer(t, core.Overworld)
	host.SendChat(t, player, "/warp depths")
	// 传送即下发旧维 `ForgetChunks`：新维订阅收敛与旧维遗忘同 tick 完成。
	host.Step(t)
	host.ExpectForget(t, player, core.Overworld)
	if !host.StepUntilDimension(t, player, core.Depths) {
		t.Fatal("首次传送未下发 Reset 置位的玩家状态")
	}
	state := host.PlayerState(t, player)
	if foot := publicationFootChunk(state.Dimension, state.State.Position); foot.Pos != warpDepthsAnchor {
		t.Fatalf("落点区块 = %+v，想要 depths 锚点 %+v", foot, warpDepthsAnchor)
	}
	host.ExpectDepthsSnapshot(t, player)
	host.SendChat(t, player, "/warp overworld")
	if !host.StepUntilDimension(t, player, core.Overworld) {
		t.Fatal("返回传送未下发 Reset 置位的玩家状态")
	}
	back := host.PlayerState(t, player)
	if foot := publicationFootChunk(back.Dimension, back.State.Position); foot.Pos != warpOverworldAnchor {
		t.Fatalf("返回落点区块 = %+v，想要主世界锚点 %+v", foot, warpOverworldAnchor)
	}
}

// ExpectDepthsSnapshot 断言新维锚点区块的快照已走完发布路径：`snapshotSent`
// 为真意味着 `ChunkSnapshot` 已进过该会话的出盒。
func (host *warpTestHost) ExpectDepthsSnapshot(t *testing.T, player warpTestPlayer) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		host.Step(t)
		// 判定整体在锁内完成：`publications` 映射与 `snapshotSent` 位都只在
		// `stepMu` 下变更，锁外读取会与 tick 发布产生竞态。
		host.running.stepMu.Lock()
		sent := false
		if session := host.running.sessions[player.session]; session != nil {
			if publication := session.publications[core.ChunkKey{
				Dimension: core.Depths,
				Pos:       warpDepthsAnchor,
			}]; publication != nil && publication.snapshotSent {
				sent = true
			}
		}
		host.running.stepMu.Unlock()
		if sent {
			return
		}
	}
	t.Fatal("新维锚点区块快照未在时限内发布")
}

func TestWarpRejectsIllegalSpell(t *testing.T) {
	host := newWarpTestHost(t, 42)
	player := host.SpawnPlayer(t, core.Overworld)
	before := host.PlayerState(t, player)
	host.SendChat(t, player, "/warp nether")
	host.Step(t)
	if reason := host.ExpectCommandRejected(t, player); reason != network.RejectInvalidInput {
		t.Fatalf("非法拼写拒绝原因 = %q，想要 invalid_input", reason)
	}
	after := host.PlayerState(t, player)
	if after.Dimension != before.Dimension || after.State.Position != before.State.Position {
		t.Fatalf("非法传送后玩家被移动: %+v -> %+v", before.State.Position, after.State.Position)
	}
}

func TestWarpRejectsSameDimension(t *testing.T) {
	host := newWarpTestHost(t, 42)
	player := host.SpawnPlayer(t, core.Overworld)
	before := host.PlayerState(t, player)
	host.SendChat(t, player, "/warp overworld")
	host.Step(t)
	if reason := host.ExpectCommandRejected(t, player); reason != network.RejectInvalidInput {
		t.Fatalf("同维传送拒绝原因 = %q，想要 invalid_input", reason)
	}
	after := host.PlayerState(t, player)
	if after.Dimension != before.Dimension || after.State.Position != before.State.Position {
		t.Fatalf("同维传送后玩家被移动: %+v -> %+v", before.State.Position, after.State.Position)
	}
}

func TestWarpRejectsWhilePendingSpawn(t *testing.T) {
	host := newWarpTestHost(t, 42)
	client, endpoint := network.NewMemoryPair(64)
	host.next++
	spec := registrySessionSpecWithRestore(
		host.next, 1, endpoint, contract.PlayerRestore{
			SpawnDimension: core.Overworld,
			SpawnAnchor:    warpOverworldAnchor,
		},
	)
	if _, err := host.running.AttachSession(spec); err != nil {
		t.Fatalf("接入会话: %v", err)
	}
	player := warpTestPlayer{session: host.next, client: client}
	host.players = append(host.players, player)
	// 出生完成前即发送传送：待出生玩家不得传送。
	host.SendChat(t, player, "/warp depths")
	host.Step(t)
	if reason := host.ExpectCommandRejected(t, player); reason != network.RejectPlayerNotReady {
		t.Fatalf("待出生传送拒绝原因 = %q，想要 player_not_ready", reason)
	}
}

func TestWarpPreservesBelongings(t *testing.T) {
	host := newWarpTestHost(t, 42)
	player := host.SpawnPlayer(t, core.Overworld)
	host.SetInventory(t, player, func(inventory core.Inventory) core.Inventory {
		inventory.Backpack[0] = core.ItemStack{Item: core.ItemOakLog, Count: 7}
		return inventory
	})
	host.Step(t)
	before := host.PlayerSnapshot(t, player)
	if before.Inventory.Backpack[0] != (core.ItemStack{Item: core.ItemOakLog, Count: 7}) {
		t.Fatalf("前置失败：标记物品未写入背包: %+v", before.Inventory.Backpack[0])
	}
	host.SendChat(t, player, "/warp depths")
	host.StepUntilDimension(t, player, core.Depths)
	after := host.PlayerSnapshot(t, player)
	if after.Inventory != before.Inventory {
		t.Fatal("传送前后背包不一致")
	}
	if after.Health != before.Health || after.Hunger != before.Hunger ||
		after.SaturationMilli != before.SaturationMilli ||
		after.ExhaustionMilli != before.ExhaustionMilli {
		t.Fatalf(
			"传送前后生命/饥饿不一致: (%d,%d) -> (%d,%d)",
			before.Health, before.Hunger, after.Health, after.Hunger,
		)
	}
	if after.RespawnPresent != before.RespawnPresent ||
		after.RespawnPosition != before.RespawnPosition ||
		after.RespawnDimension != before.RespawnDimension {
		t.Fatal("传送丢了个人重生点记录")
	}
}

// warpFailDepthsGenerator 只让 depths 生成失败：目标锚点走向 `ChunkFailed`。
type warpFailDepthsGenerator struct{}

func (warpFailDepthsGenerator) GenerateChunk(
	dimension core.DimensionID,
	position core.ChunkPos,
) *world.Chunk {
	if dimension == core.Depths {
		return nil
	}
	return playerTestGenerator{}.GenerateChunk(dimension, position)
}

func TestWarpRejectsFailedTarget(t *testing.T) {
	host := newWarpTestHostWithGenerator(t, 42, warpFailDepthsGenerator{})
	player := host.SpawnPlayer(t, core.Overworld)
	// 用一个临时 depths 会话把目标锚点驱动到加载失败，再 detach 使失败态稳定
	// 可观测（有订阅时失败会即刻重试回加载中，只有无人问津的失败才是确切的
	// 「目标不可用」）。
	client, endpoint := network.NewMemoryPair(64)
	host.next++
	spec := registrySessionSpecWithRestore(
		host.next, 1, endpoint, contract.PlayerRestore{
			SpawnDimension: core.Depths,
			SpawnAnchor:    warpDepthsAnchor,
		},
	)
	if _, err := host.running.AttachSession(spec); err != nil {
		t.Fatalf("接入临时会话: %v", err)
	}
	probe := warpTestPlayer{session: host.next, client: client}
	host.players = append(host.players, probe)
	// 临时会话把锚点驱动到生成中后 detach，再直注确切的生成失败：失败经
	// 正常 ingress 落盘为 `ChunkFailed`，且无人问津故稳定可观测。
	generating := false
	for range 40 {
		host.Step(t)
		if info, ok := host.running.ChunkInfo(core.Depths, warpDepthsAnchor); ok &&
			info.State == contract.ChunkGenerating {
			generating = true
			break
		}
	}
	if !generating {
		t.Fatal("前置失败：目标锚点未进入生成中")
	}
	host.running.DetachSession(probe.session, 1, nil)
	host.running.engine.SubmitGenerated(contract.GeneratedChunk{
		Dimension: core.Depths,
		Pos:       warpDepthsAnchor,
		Err:       errors.New("warp test injected depths generation failure"),
	})
	host.Step(t)
	host.Step(t)
	if info, ok := host.running.ChunkInfo(core.Depths, warpDepthsAnchor); !ok ||
		info.State != contract.ChunkFailed {
		t.Fatalf("前置失败：目标锚点状态 = (%v, %+v)，想要确切失败", ok, info)
	}
	before := host.PlayerState(t, player)
	host.SendChat(t, player, "/warp depths")
	host.Step(t)
	if reason := host.ExpectCommandRejected(t, player); reason != network.RejectChunkNotReady {
		t.Fatalf("失败目标拒绝原因 = %q，想要 chunk_not_ready", reason)
	}
	after := host.PlayerState(t, player)
	if after.Dimension != before.Dimension || after.State.Position != before.State.Position {
		t.Fatal("目标失败时玩家应留在旧维且位置不变")
	}
}

// TestWarpCompanionAndHostileStayBehind 覆盖多维规约「伙伴与敌怪不跟随传送」：
// 跟随中的伙伴与附近夜行者在传送完成时下发旧维的消失消息、新维不自动生成镜像；
// 跟随任务在异维期间保持 `companion.TaskRunning`（不失败、不寻路），返回旧维后
// 伙伴重现且跟随恢复。夜行者实体始终锚定旧维。
func TestWarpCompanionAndHostileStayBehind(t *testing.T) {
	definitions := []companion.Definition{{ID: chatTestCompanionID(1), Name: "阿木"}}
	model := newFakeCompanionModel(t)
	host := newCompanionManagerHost(t, definitions, model, nil)
	issuerIdentity := integrationIdentity(0x91, "发令者")
	issuer := openCompanionChatClient(t, host, "memory", issuerIdentity)
	clients := []network.ClientEndpoint{issuer}
	issuerLogin := activeLoginForPlayer(t, host, issuerIdentity.PlayerID)

	// 等待玩家就绪与伙伴出生送达（自建循环：现成的就绪 helper 会吞掉
	// `CompanionSpawn` 消息，消失断言需要先见过出生）。
	var body companion.Body
	sawCompanionSpawn := false
	bodyFound := false
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) && (!sawCompanionSpawn || !bodyFound) {
		result := host.world.StepForTest()
		ready := false
		for _, message := range receiveCompanionChatTick(t, issuer, result.Tick) {
			switch message := message.(type) {
			case network.PlayerState:
				ready = message.Ready
			case network.CompanionSpawn:
				if message.ID == definitions[0].ID {
					sawCompanionSpawn = true
				}
			}
		}
		if !ready {
			continue
		}
		for _, candidate := range host.world.engine.CompanionBodies() {
			if candidate.ID == definitions[0].ID {
				body = candidate
				bodyFound = true
			}
		}
	}
	if !sawCompanionSpawn || !bodyFound {
		t.Fatal("伙伴始终未在客户端可见")
	}

	// 夜晚避免日间灼烧干扰夜行者可见性；玩家与伙伴都在主世界出生锚点附近。
	host.world.engine.SetWorldTimeForTest(18000)
	mob := contract.HostileMob{
		ID:        7,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{body.Position[0] + 6, 1, body.Position[2]},
			OnGround: true,
		},
		Yaw:             0.25,
		Health:          13,
		BurnCooldown:    20,
		NextRepathTicks: ^uint64(0),
	}
	if err := host.world.engine.RestoreHostile(mob); err != nil {
		t.Fatalf("RestoreHostile: %v", err)
	}
	// 等待夜行者在客户端可见，再开始跟随任务。
	sawHostileSpawn := false
	deadline = time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) && !sawHostileSpawn {
		result := host.world.StepForTest()
		for _, message := range receiveCompanionChatTick(t, issuer, result.Tick) {
			if spawn, ok := message.(network.HostileSpawn); ok {
				for _, record := range spawn.Spawns {
					if record.ID == mob.ID {
						sawHostileSpawn = true
					}
				}
			}
		}
	}
	if !sawHostileSpawn {
		t.Fatal("夜行者始终未在客户端可见")
	}

	// 下发跟随任务并等它进入 Running。
	model.setPlanScript(followPlanContent(issuerIdentity.PlayerID))
	sendIntegration(t, issuer, network.ChatCommand{Text: "@阿木 跟着我"})
	waitForIncomingChatDepth(t, host.world, 1)
	started := false
	deadline = time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) && !started {
		result := host.world.StepForTest()
		for _, event := range companionChatEvents(receiveCompanionChatTick(t, issuer, result.Tick)) {
			if event.Kind == network.ChatEventTaskStarted && event.Command == "跟着我" {
				started = true
			}
		}
	}
	if !started {
		t.Fatal("跟随任务始终未进入 Running")
	}
	// 把玩家挪远迫使跟随进入寻路，再传送：跨维保持必须压住这次寻路，
	// 而不是让它以寻路失败终结任务。
	setPlayerPosition(t, host, issuerLogin.Session, [3]float32{body.Position[0] + 40, 1, body.Position[2]})
	for range 10 {
		result := host.world.StepForTest()
		receiveCompanionChatTick(t, issuer, result.Tick)
	}

	// 传送到 depths 并收集全程消息。
	sendIntegration(t, issuer, network.ChatCommand{Text: "/warp depths"})
	waitForIncomingChatDepth(t, host.world, 1)
	var (
		sawCompanionDespawn  bool
		sawHostileDespawn    bool
		depthsMirrorSpawn    bool
		followFailed         bool
		landed               bool
		companionPosAtWarp   [3]float32
		warpedCompanionFixed bool
	)
	deadline = time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) && !landed {
		result := host.world.StepForTest()
		for _, message := range receiveCompanionChatTick(t, issuer, result.Tick) {
			switch message := message.(type) {
			case network.CompanionDespawn:
				if message.ID == definitions[0].ID {
					sawCompanionDespawn = true
				}
			case network.CompanionSpawn:
				if message.Dimension == core.Depths {
					depthsMirrorSpawn = true
				}
			case network.HostileDespawn:
				for _, id := range message.IDs {
					if id == mob.ID {
						sawHostileDespawn = true
					}
				}
			case network.ChatEvent:
				if message.Kind == network.ChatEventTaskFailed && message.Command == "跟着我" {
					followFailed = true
				}
			}
		}
		if update, ok := host.world.engine.Player(issuerLogin.Session); ok &&
			update.Dimension == core.Depths && update.Ready {
			landed = true
			if !warpedCompanionFixed {
				companionPosAtWarp = currentCompanionBody(t, host, definitions[0].ID).Position
				warpedCompanionFixed = true
			}
		}
	}
	if !landed {
		t.Fatal("传送后未在时限内落到 depths")
	}
	if !sawCompanionDespawn {
		t.Fatal("传送完成时旧维未下发伙伴 Despawn")
	}
	if depthsMirrorSpawn {
		t.Fatal("新维自动生成了伙伴镜像")
	}
	if !sawHostileDespawn {
		t.Fatal("传送完成时旧维未下发夜行者 Despawn")
	}
	if followFailed {
		t.Fatal("异维期间跟随任务失败，返回后无法恢复")
	}
	// 逗留期间伙伴保持原地（不跟随）且任务仍在 Running。
	for range 30 {
		result := host.world.StepForTest()
		for _, event := range companionChatEvents(receiveCompanionChatTick(t, issuer, result.Tick)) {
			if event.Kind == network.ChatEventTaskFailed && event.Command == "跟着我" {
				t.Fatal("逗留期间跟随任务失败")
			}
		}
	}
	sojourn := currentCompanionBody(t, host, definitions[0].ID).Position
	dx, dz := sojourn[0]-companionPosAtWarp[0], sojourn[2]-companionPosAtWarp[2]
	if dx*dx+dz*dz > 1e-6 {
		t.Fatalf("异维期间伙伴移动：%v -> %v", companionPosAtWarp, sojourn)
	}
	slot := host.world.companionManager.slots[definitions[0].ID]
	if current, ok := slot.queue.Current(); !ok || current.State != companion.TaskRunning {
		t.Fatal("异维期间跟随任务不在 Running，返回后无法恢复")
	}
	for _, mob := range host.world.engine.HostileMobs() {
		if mob.ID == 7 && mob.Dimension != core.Overworld {
			t.Fatalf("夜行者被带到维度 %d", mob.Dimension)
		}
	}

	// 返回旧维：伙伴重现，跟随恢复。
	sendIntegration(t, issuer, network.ChatCommand{Text: "/warp overworld"})
	waitForIncomingChatDepth(t, host.world, 1)
	respawned := false
	returned := false
	deadline = time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) && !returned {
		result := host.world.StepForTest()
		for _, message := range receiveCompanionChatTick(t, issuer, result.Tick) {
			if spawn, ok := message.(network.CompanionSpawn); ok && spawn.ID == definitions[0].ID {
				respawned = true
			}
		}
		if update, ok := host.world.engine.Player(issuerLogin.Session); ok &&
			update.Dimension == core.Overworld && update.Ready {
			returned = true
		}
	}
	if !returned {
		t.Fatal("返回后未在时限内落到主世界")
	}
	if !respawned {
		t.Fatal("返回旧维后伙伴未重现，伙伴关系未恢复")
	}
	stopFollowTask(t, host, clients)
}

// TestWarpDropsSameTickInput 覆盖传送事务「同 tick 冻结输入」：与 `/warp`
// 同 tick 到达的旧维输入不得跨维生效。移动 drain 先于传送聊天 drain 进入
// `engine` inbox，而注销重建会把订阅的 `lastSequence` 清零——重建后若不垫高
// 到传送前水位，在途旧维序号会在新维通过序号过滤，以待出生身份被结算为
// `RejectPlayerNotReady` 的伪拒绝下发客户端。此处同 tick 先送达旧维移动与
// 瞄准输入再传送：客户端 MUST NOT 收到这些序号的拒绝，新维出生朝向 MUST
// 与传送前快照一致，且传送本身仍下发 `Reset`。
func TestWarpDropsSameTickInput(t *testing.T) {
	host := newWarpTestHost(t, 42)
	player := host.SpawnPlayer(t, core.Overworld)
	// 先以一笔普通输入把朝向固定到已知值：`0.5` 与 `-1.0` 都在 `normalizeYaw`
	// 的恒等区，前后比较不受归一化干扰。
	host.running.enqueueIncoming(context.Background(), incomingCommand{
		Session: player.session, Generation: 1,
		Command: contract.Command{
			Session: player.session, Sequence: 1,
			Kind: contract.CommandPlayerInput, Yaw: 0.5,
		},
	})
	host.Step(t)
	before := host.PlayerState(t, player)
	if before.Yaw != 0.5 {
		t.Fatalf("前置失败：传送前朝向 = %v，想要 0.5", before.Yaw)
	}
	// 同 tick：在途旧维移动与瞄准输入先入 inbox，随后传送聊天到达。
	host.running.enqueueIncoming(context.Background(), incomingCommand{
		Session: player.session, Generation: 1,
		Command: contract.Command{
			Session: player.session, Sequence: 2,
			Kind: contract.CommandPlayerInput, MoveX: 1, Yaw: 0.5,
		},
	})
	host.running.enqueueIncoming(context.Background(), incomingCommand{
		Session: player.session, Generation: 1,
		Command: contract.Command{
			Session: player.session, Sequence: 3,
			Kind: contract.CommandPlaceBlock, Yaw: -1.0, Slot: 0,
		},
	})
	host.SendChat(t, player, "/warp depths")
	result := host.Step(t)
	sawReset := false
	for _, update := range result.Players {
		if update.Session == player.session && update.Dimension == core.Depths && update.Reset {
			sawReset = true
		}
	}
	after := host.PlayerState(t, player)
	if after.Dimension != core.Depths {
		t.Fatalf("传送后维度 = %d，想要 %d", after.Dimension, core.Depths)
	}
	// 在途旧维输入不得下发伪拒绝：垫高水位后它们在序号过滤即被丢弃，
	// 既不进待出生结算、也不占 `TickResult.Rejected`（`Step` 已排空出盒，
	// backlog 即本 tick 的全部拒绝）。
	if pending := host.rejected[player.session]; len(pending) != 0 {
		t.Fatalf("同 tick 旧维输入被跨维结算：收到伪拒绝 %+v", pending)
	}
	if after.Yaw != before.Yaw {
		t.Fatalf("同 tick 旧维输入跨维生效：出生朝向 %v -> %v", before.Yaw, after.Yaw)
	}
	if rest := host.StepUntilDimension(t, player, core.Depths); !sawReset && !rest {
		t.Fatal("传送未下发 Reset 置位的玩家状态")
	}
	landed := host.PlayerState(t, player)
	if foot := publicationFootChunk(landed.Dimension, landed.State.Position); foot.Pos != warpDepthsAnchor {
		t.Fatalf("落点区块 = %+v，想要 depths 锚点 %+v", foot, warpDepthsAnchor)
	}
}

// TestWarpPreservesDeclaredViewDistance 钉住跨维传送的会话视距保持：传送以
// 注销重建订阅实现，重建必须携带原会话在登录协商中声明的视距——传送后
// 新维订阅仍是声明视距的方形（半径 3 < 引擎上界 5），不回落引擎缺省视界。
func TestWarpPreservesDeclaredViewDistance(t *testing.T) {
	host := newWarpTestHostConfigured(t, 42, playerTestGenerator{}, 5)
	player := host.SpawnPlayerWithRestore(t, contract.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    warpOverworldAnchor,
		ViewDistance:   2,
	})

	host.SendChat(t, player, "/warp depths")
	host.Step(t)
	host.ExpectForget(t, player, core.Overworld)
	host.StepUntilDimension(t, player, core.Depths)

	engine := host.running.engine
	inside := core.ChunkKey{Dimension: core.Depths, Pos: core.ChunkPos{
		X: warpDepthsAnchor.X + 3, Z: warpDepthsAnchor.Z + 3,
	}}
	outside := core.ChunkKey{Dimension: core.Depths, Pos: core.ChunkPos{
		X: warpDepthsAnchor.X + 4, Z: warpDepthsAnchor.Z,
	}}
	if !engine.SessionWantsChunk(player.session, inside) {
		t.Fatalf("传送后应仍订阅声明视距方形内区块 %+v", inside)
	}
	if engine.SessionWantsChunk(player.session, outside) {
		t.Fatalf("传送后不应订阅声明视距方形外区块 %+v", outside)
	}
}
