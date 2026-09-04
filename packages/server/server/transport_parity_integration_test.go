package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/sim/runtime"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	networktcp "github.com/channing771/mornlea/packages/shared/network/tcp"
	"github.com/channing771/mornlea/packages/shared/world"
)

type miningParityGenerator struct{}

func (miningParityGenerator) GenerateChunk(position core.ChunkPos) *world.Chunk {
	chunk := integrationChunk(position, core.StoneID)
	for _, target := range []core.BlockPos{{X: -1, Y: 1, Z: -6}, {X: 1, Y: 1, Z: -6}} {
		if target.Chunk() != position {
			continue
		}
		x, _, z := target.Local()
		chunk.SetBlock(x, target.Y, z, core.StoneID)
	}
	chunk.Compact()
	return chunk
}

// barrenParityGenerator 是业务 transcript parity 专用的无草平坦世界：地表
// y=0 为泥土而非 `integrationChunk` 的草。被动牛只在草上出生/吃草，而两侧
// 录像的绝对 tick 窗口本就不同（握手 tick 数随传输而异），草地会让 transcript
// 混入下标错位的背景泥土写入；地表材质对本脚本（移动/合成/放置/采掘/重同步）
// 不可见，parity 只要求两侧逐字段相同、不与历史基线比对。
// parity 世界按契约保持无背景生物写入（背景实体消息同样被比对过滤）。
type barrenParityGenerator struct{}

func (barrenParityGenerator) GenerateChunk(position core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(position)
	for z := 0; z < core.SectionSize; z++ {
		for x := 0; x < core.SectionSize; x++ {
			chunk.SetBlock(x, core.MinY, z, core.BedrockID)
			for y := int32(core.MinY + 1); y < 0; y++ {
				chunk.SetBlock(x, y, z, core.StoneID)
			}
			chunk.SetBlock(x, 0, z, core.DirtID)
		}
	}
	chunk.Compact()
	return chunk
}

func TestMemoryTCPParityBusinessTranscriptAndHashes(t *testing.T) {
	memory := runParityTranscript(t, "memory")
	tcp := runParityTranscript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		t.Fatalf("TCP parity result differs\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	snapshots := 0
	for _, entry := range memory.Transcript {
		if strings.HasPrefix(entry, "ChunkSnapshot:") {
			snapshots++
		}
	}
	if snapshots < 9 {
		t.Fatalf("parity readiness transcript has %d snapshots, want at least 9", snapshots)
	}
	if last := memory.Transcript[len(memory.Transcript)-1]; !strings.HasPrefix(last, "DisconnectTick:") {
		t.Fatalf("parity transcript ends with %q, want disconnect tick", last)
	}
}

func TestMemoryTCPMiningConvergence(t *testing.T) {
	memory := runMiningParityScript(t, "memory")
	tcp := runMiningParityScript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		t.Fatalf("采掘 Memory/TCP 未收敛\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
}

func TestMiningCompletionOraclesRejectOrderDuplicatesAndMirrorDivergence(t *testing.T) {
	target := core.BlockPos{X: 1, Y: 1, Z: -6}
	blockIndex, ok := world.ChunkBlockIndex(target)
	if !ok {
		t.Fatal("测试目标没有区块索引")
	}
	delta := network.BlockChanges{
		Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 1, NewRevision: 2,
		Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
	}
	upsert := network.ItemDropUpserts{ServerTick: 2, Drops: []network.ItemDrop{{
		ID:         core.DropID{Dimension: core.Overworld, Chunk: target.Chunk(), Generation: 1},
		BlockIndex: blockIndex, Item: core.ItemStone, Count: 1,
	}}}
	full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	var hotbar core.Hotbar
	hotbar.Selected = 1
	hotbar.Slots[1] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: full - 1}
	inventory := network.InventoryState{Inventory: core.Inventory{Hotbar: hotbar}}
	wrongDurability := inventory
	wrongDurability.Inventory.Hotbar.Slots[1].Durability--
	inactive := network.PlayerState{ServerTick: 2}
	valid := []network.ServerMessage{delta, upsert, inventory, inactive}
	tests := []struct {
		name     string
		messages []network.ServerMessage
		wantErr  bool
	}{
		{name: "规范完成帧", messages: valid},
		{name: "交换 BlockChanges 和 drop", messages: []network.ServerMessage{upsert, delta, inventory, inactive}, wantErr: true},
		{name: "重复 drop upsert", messages: []network.ServerMessage{delta, upsert, upsert, inventory, inactive}, wantErr: true},
		{name: "缺少 InventoryState", messages: []network.ServerMessage{delta, upsert, inactive}, wantErr: true},
		{name: "工具耐久错误", messages: []network.ServerMessage{delta, upsert, wrongDurability, inactive}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateMiningCompletionFrame(test.messages, target)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateMiningCompletionFrame error=%v wantErr=%t", err, test.wantErr)
			}
		})
	}
	var hash [32]byte
	hash[0] = 1
	other := hash
	other[0] = 2
	if err := validateMiningMirrorConvergence(hash, 2, other, 2, true); err == nil {
		t.Fatal("authority/mirror hash 偏离未被拒绝")
	}
}

// withoutPassiveMessages 剔除完成帧里的被动牛背景消息：完成帧契约只含
// 破坏 delta、掉落、背包与玩家状态四条，被动牛 spawn/state 与采掘正交，
// 由被动牛专用的发布测试覆盖，此处只保证完成帧断言不受背景消息干扰。
func withoutPassiveMessages(messages []network.ServerMessage) []network.ServerMessage {
	kept := messages[:0]
	for _, message := range messages {
		switch message.(type) {
		case network.PassiveSpawn, network.PassiveState, network.PassiveDespawn:
			continue
		}
		kept = append(kept, message)
	}
	return kept
}

func validateMiningCompletionFrame(messages []network.ServerMessage, target core.BlockPos) error {
	if len(messages) != 4 {
		return fmt.Errorf("采掘完成帧消息数=%d，想要 4: %+v", len(messages), messages)
	}
	delta, ok := messages[0].(network.BlockChanges)
	if !ok {
		return fmt.Errorf("采掘完成帧第一条=%T，想要 BlockChanges", messages[0])
	}
	if err := delta.Validate(); err != nil {
		return fmt.Errorf("采掘 BlockChanges 非法: %w", err)
	}
	if delta.Dimension != core.Overworld || delta.Chunk != target.Chunk() ||
		len(delta.Changes) != 1 || delta.Changes[0] != (network.BlockChange{Position: target, Block: core.AirID}) {
		return fmt.Errorf("采掘 BlockChanges 未精确破坏目标: %+v", delta)
	}
	upserts, ok := messages[1].(network.ItemDropUpserts)
	if !ok {
		return fmt.Errorf("采掘完成帧第二条=%T，想要 ItemDropUpserts", messages[1])
	}
	if err := upserts.Validate(); err != nil {
		return fmt.Errorf("采掘 ItemDropUpserts 非法: %w", err)
	}
	blockIndex, indexed := world.ChunkBlockIndex(target)
	if !indexed || len(upserts.Drops) != 1 || upserts.Drops[0].BlockIndex != blockIndex ||
		upserts.Drops[0].Item != core.ItemStone || upserts.Drops[0].Count != 1 {
		return fmt.Errorf("采掘掉落物不唯一或内容不匹配: %+v", upserts)
	}
	inventory, ok := messages[2].(network.InventoryState)
	if !ok {
		return fmt.Errorf("采掘完成帧第三条=%T，想要 InventoryState", messages[2])
	}
	if err := inventory.Validate(); err != nil {
		return fmt.Errorf("采掘 InventoryState 非法: %w", err)
	}
	full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	hotbar := inventory.Inventory.Hotbar
	if hotbar.Selected != 1 || hotbar.Slots[1] != (core.ItemStack{
		Item: core.ItemIronPickaxe, Count: 1, Durability: full - 1,
	}) {
		return fmt.Errorf("采掘 InventoryState 未精确扣减选中铁镐耐久: %+v", inventory)
	}
	state, ok := messages[3].(network.PlayerState)
	if !ok {
		return fmt.Errorf("采掘完成帧第四条=%T，想要 PlayerState", messages[3])
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("采掘 PlayerState 非法: %w", err)
	}
	if state.ServerTick != upserts.ServerTick || canonicalMiningState(state) != (miningTranscriptEntry{}) {
		return fmt.Errorf("采掘完成帧未以同 tick 规范非活动状态收尾: state=%+v upserts=%+v", state, upserts)
	}
	return nil
}

func validateMiningMirrorConvergence(authorityHash [32]byte, authorityRevision uint64, mirrorHash [32]byte, mirrorRevision uint64, loaded bool) error {
	if !loaded {
		return errors.New("采掘 mirror 未加载")
	}
	if mirrorHash != authorityHash || mirrorRevision != authorityRevision {
		return fmt.Errorf("mirror/authority 未收敛: mirror=%x/%d authority=%x/%d",
			mirrorHash, mirrorRevision, authorityHash, authorityRevision)
	}
	return nil
}

type miningTranscriptEntry struct {
	Active        bool
	Target        core.BlockPos
	ProgressTicks uint16
	RequiredTicks uint16
	Harvestable   bool
}

type miningParityResult struct {
	ChunkHash     [32]byte
	ChunkRevision uint64
	InventoryHash [32]byte
	DropHash      [32]byte
	Progress      []miningTranscriptEntry
	FinalInactive miningTranscriptEntry
	Disconnected  bool
}

func runMiningParityScript(t *testing.T, transport string) miningParityResult {
	t.Helper()
	identity := integrationIdentity(0x72, "MiningParity")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	stoneFull, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	ironFull, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: stoneFull}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: ironFull}
	inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: [3]float32{0.5, 1.001, 0.5}}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location, Inventory: inventory,
	})); err != nil {
		t.Fatal(err)
	}
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, miningParityGenerator{}, store)
	endpoint, acceptDone, closeTransport := openParityTransport(t, host, transport, identity)
	defer closeTransport()
	mirror := client.NewMirror()
	drops := client.NewItemDrops()
	inventoryConfirmed := false
	ready := false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s mining parity", transport),
		func() bool { return ready && inventoryConfirmed && parityViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v inventoryConfirmed=%v viewLoaded=%v", ready, inventoryConfirmed, parityViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
			for _, message := range messages {
				applyMiningParityMessage(t, drops, message)
				assertMiningParityHasNoRemotePlayers(t, message)
				switch message := message.(type) {
				case network.PlayerState:
					assertValidIntegrationPlayerState(t, message)
					ready = ready || message.Ready
				case network.InventoryState:
					inventoryConfirmed = message.Inventory == inventory
				}
			}
		},
	)

	step := func(command network.ClientMessage) (network.PlayerState, []network.ServerMessage) {
		if command != nil {
			sendIntegration(t, endpoint, command)
			waitIntegrationCondition(t, fmt.Sprintf("%s mining %T queued", transport, command), func() bool {
				return len(host.world.incoming) > 0
			})
		}
		_, messages := parityStep(t, host, endpoint, mirror)
		var state network.PlayerState
		for _, message := range messages {
			applyMiningParityMessage(t, drops, message)
			if current, ok := message.(network.PlayerState); ok {
				assertValidIntegrationPlayerState(t, current)
				state = current
			}
		}
		return state, messages
	}

	result := miningParityResult{Progress: make([]miningTranscriptEntry, 0, 45)}
	record := func(state network.PlayerState) {
		result.Progress = append(result.Progress, canonicalMiningState(state))
	}
	state, _ := step(network.PlayerInput{Sequence: 1, Pitch: -0.2, Mining: true})
	record(state)
	for range 3 {
		state, _ = step(nil)
		record(state)
	}
	state, _ = step(network.SelectHotbar{Sequence: 2, Slot: 1})
	record(state)
	if state.MiningProgressTicks != 1 || state.MiningRequiredTicks != 8 {
		t.Fatalf("%s 切换铁镐状态 = %+v", transport, state)
	}
	state, _ = step(network.PlayerInput{Sequence: 3, Pitch: -0.2})
	record(state)
	if state.MiningActive {
		t.Fatalf("%s 松键后仍活动: %+v", transport, state)
	}
	state, _ = step(network.SelectHotbar{Sequence: 4, Slot: 2})
	record(state)
	for tick := 1; tick <= 30; tick++ {
		var command network.ClientMessage
		if tick == 1 {
			command = network.PlayerInput{Sequence: 5, Pitch: -0.2, Mining: true}
		}
		var messages []network.ServerMessage
		state, messages = step(command)
		record(state)
		if tick < 30 && (!state.MiningActive || state.MiningProgressTicks != uint16(tick) ||
			state.MiningRequiredTicks != 30 || state.MiningHarvestable) {
			t.Fatalf("%s 错误工具 tick %d = %+v", transport, tick, state)
		}
		if tick == 30 {
			if state.MiningActive || miningParityHasDrop(messages) {
				t.Fatalf("%s 错误工具完成 = state=%+v messages=%+v", transport, state, messages)
			}
		}
	}
	state, _ = step(network.SelectHotbar{Sequence: 6, Slot: 1})
	record(state)
	const sideYaw = -0.16514868
	sideTarget := core.BlockPos{X: 1, Y: 1, Z: -6}
	var completionMessages []network.ServerMessage
	for tick := 1; tick <= 8; tick++ {
		var command network.ClientMessage
		if tick == 1 {
			command = network.PlayerInput{Sequence: 7, Yaw: sideYaw, Pitch: -0.2, Mining: true}
		}
		state, completionMessages = step(command)
		record(state)
		if tick < 8 && (!state.MiningActive || state.MiningProgressTicks != uint16(tick) ||
			state.MiningRequiredTicks != 8 || !state.MiningHarvestable) {
			t.Fatalf("%s 铁镐 tick %d = %+v", transport, tick, state)
		}
	}
	if err := validateMiningCompletionFrame(withoutPassiveMessages(completionMessages), sideTarget); err != nil {
		t.Fatalf("%s 铁镐完成帧: %v", transport, err)
	}
	if state.MiningActive || len(drops.Presentations()) != 1 {
		t.Fatalf("%s 铁镐完成 = state=%+v drops=%+v", transport, state, drops.Presentations())
	}
	state, _ = step(network.PlayerInput{Sequence: 8, Yaw: sideYaw, Pitch: -0.2})
	result.FinalInactive = canonicalMiningState(state)
	if result.FinalInactive != (miningTranscriptEntry{}) {
		t.Fatalf("%s 最终非活动状态不规范: %+v", transport, result.FinalInactive)
	}

	host.mu.Lock()
	active := *host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
	if !ok {
		t.Fatalf("%s 采掘 player snapshot 不可用", transport)
	}
	result.InventoryHash = inventoryDigest(snapshot.Inventory)
	result.DropHash = itemDropDigest(drops.Presentations())
	authorityHash, authorityRevision, ok := host.world.ChunkHash(core.Overworld, core.ChunkPos{})
	if !ok {
		t.Fatalf("%s 采掘 chunk hash 不可用", transport)
	}
	result.ChunkHash, result.ChunkRevision, ok = mirror.Hash(core.Overworld, core.ChunkPos{})
	if err := validateMiningMirrorConvergence(
		authorityHash, authorityRevision, result.ChunkHash, result.ChunkRevision, ok,
	); err != nil {
		t.Fatalf("%s 采掘镜像收敛: %v", transport, err)
	}
	state, _ = step(network.PlayerInput{Sequence: 9, Yaw: -sideYaw, Pitch: -0.2, Mining: true})
	if !state.MiningActive {
		t.Fatalf("%s 断线前采掘未活动: %+v", transport, state)
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	select {
	case err := <-acceptDone:
		if err != nil && !errors.Is(err, network.ErrClosed) {
			t.Fatalf("%s mining accept worker: %v", transport, err)
		}
	case <-ctx.Done():
		t.Fatalf("%s mining accept worker did not exit: %v", transport, ctx.Err())
	}
	host.world.StepForTest()
	_, present := host.world.PlayerSnapshotFor(active.Session)
	result.Disconnected = !present
	if present {
		t.Fatalf("%s 活动采掘玩家断线后仍存在", transport)
	}
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("%s mining Host.Shutdown: %v", transport, err)
	}
	stored, err := store.LoadPlayer(ctx, identity.PlayerID)
	if err != nil {
		t.Fatalf("%s mining LoadPlayer: %v", transport, err)
	}
	if inventoryDigest(stored.Inventory) != result.InventoryHash {
		t.Fatalf("%s 断线持久化背包与最终快照不一致", transport)
	}
	return result
}

// assertMiningParityHasNoRemotePlayers 锁定旧采掘脚本是单玩家场景：所有 primary
// action 都没有三格内玩家候选，因而必须完整走既有采掘状态机。
func assertMiningParityHasNoRemotePlayers(t *testing.T, message network.ServerMessage) {
	t.Helper()
	switch message.(type) {
	case network.RemotePlayerSpawn, network.RemotePlayerStates, network.RemotePlayerDespawn:
		t.Fatalf("单玩家采掘 parity 收到远端玩家消息 %T", message)
	}
}

func applyMiningParityMessage(t *testing.T, drops *client.ItemDrops, message network.ServerMessage) {
	t.Helper()
	switch message.(type) {
	case network.ItemDropUpserts, network.ItemDropRemoves:
		if err := drops.Apply(message); err != nil {
			t.Fatalf("采掘掉落镜像 Apply(%T): %v", message, err)
		}
	}
}

func assertValidIntegrationPlayerState(t *testing.T, state network.PlayerState) {
	t.Helper()
	if err := state.Validate(); err != nil {
		t.Fatalf("PlayerState tick=%d 非法: %v", state.ServerTick, err)
	}
}

func canonicalMiningState(state network.PlayerState) miningTranscriptEntry {
	return miningTranscriptEntry{
		Active: state.MiningActive, Target: state.MiningTarget,
		ProgressTicks: state.MiningProgressTicks, RequiredTicks: state.MiningRequiredTicks,
		Harvestable: state.MiningHarvestable,
	}
}

func miningParityHasDrop(messages []network.ServerMessage) bool {
	for _, message := range messages {
		if _, ok := message.(network.ItemDropUpserts); ok {
			return true
		}
	}
	return false
}

func inventoryDigest(inventory core.Inventory) [32]byte {
	var encoded bytes.Buffer
	encoded.WriteByte(inventory.Hotbar.Selected)
	for slot := range core.InventorySlots {
		stack, _ := inventory.Slot(uint8(slot))
		_ = binary.Write(&encoded, binary.LittleEndian, uint16(stack.Item))
		encoded.WriteByte(stack.Count)
	}
	return sha256.Sum256(encoded.Bytes())
}

func itemDropDigest(drops []client.ItemDropPresentation) [32]byte {
	var encoded bytes.Buffer
	for _, drop := range drops {
		_ = binary.Write(&encoded, binary.LittleEndian, int32(drop.ID.Dimension))
		_ = binary.Write(&encoded, binary.LittleEndian, drop.ID.Chunk.X)
		_ = binary.Write(&encoded, binary.LittleEndian, drop.ID.Chunk.Z)
		_ = binary.Write(&encoded, binary.LittleEndian, drop.ID.Slot)
		_ = binary.Write(&encoded, binary.LittleEndian, drop.ID.Generation)
		_ = binary.Write(&encoded, binary.LittleEndian, drop.BlockIndex)
		_ = binary.Write(&encoded, binary.LittleEndian, uint16(drop.Item))
		encoded.WriteByte(drop.Count)
	}
	return sha256.Sum256(encoded.Bytes())
}

type parityResult struct {
	Transcript      []string
	PlayerHash      [32]byte
	ChunkHash       [32]byte
	ChunkRevision   uint64
	MirrorHash      [32]byte
	MirrorRevision  uint64
	Inventory       core.Inventory
	StoredInventory core.Inventory
	DisconnectTick  bool
}

func runParityTranscript(t *testing.T, transport string) parityResult {
	t.Helper()
	identity := integrationIdentity(0x71, "Parity")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	var initialInventory core.Inventory
	initialInventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: [3]float32{0.5, 1.001, 0.5}}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location, Inventory: initialInventory,
	})); err != nil {
		t.Fatal(err)
	}
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, barrenParityGenerator{}, store)
	endpoint, acceptDone, closeTransport := openParityTransport(t, host, transport, identity)
	mirror := client.NewMirror()
	transcript := make([]string, 0, 64)
	readinessMessages := make([]network.ServerMessage, 0, 64)

	ready := false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s business parity", transport),
		func() bool { return ready && parityViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v viewLoaded=%v", ready, parityViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
			for _, message := range messages {
				readinessMessages = append(readinessMessages, message)
				if state, ok := message.(network.PlayerState); ok && state.Ready {
					ready = true
				}
			}
		},
	)
	transcript = append(transcript, parityReadinessTranscript(t, mirror, readinessMessages)...)

	commands := []network.ClientMessage{
		network.PlayerInput{Sequence: 1, MoveX: 1, Yaw: 0, Pitch: -0.2},
		network.MoveCraftingStack{Sequence: 2, From: 9, To: 0},
		network.MoveCraftingStack{Sequence: 3, From: 9, To: 1},
		network.PlaceBlock{Sequence: 4, Yaw: 0, Pitch: -0.2, Slot: 0},
	}
	for sequence := uint64(5); sequence < 35; sequence++ {
		commands = append(commands, network.PlayerInput{
			Sequence: sequence, Yaw: 0, Pitch: -0.2, Mining: true,
		})
	}
	commands = append(commands,
		network.RequestChunkResync{
			Sequence: 35, Dimension: core.Overworld,
			Chunk: (core.BlockPos{X: 0, Y: 1, Z: -5}).Chunk(), HaveRevision: 0,
		},
		network.PlayerInput{Sequence: 36, Yaw: 0, Pitch: -0.2},
	)
	for _, command := range commands {
		sendIntegration(t, endpoint, command)
		waitIntegrationCondition(t, fmt.Sprintf("%s %T queued", transport, command), func() bool {
			return len(host.world.incoming) > 0
		})
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			transcript = append(transcript, parityBusinessMessage(t, mirror, message)...)
		}
	}

	host.mu.Lock()
	active := *host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	playerHash, ok := host.world.engine.PlayerHash(active.Session)
	if !ok {
		t.Fatal("parity player hash unavailable")
	}
	snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
	if !ok {
		t.Fatal("parity player snapshot unavailable")
	}
	position := (core.BlockPos{X: 0, Y: 1, Z: -5}).Chunk()
	chunkHash, chunkRevision, ok := host.world.ChunkHash(core.Overworld, position)
	if !ok {
		t.Fatal("parity chunk hash unavailable before disconnect")
	}
	mirrorHash, mirrorRevision, ok := mirror.Hash(core.Overworld, position)
	if !ok {
		t.Fatal("parity mirror hash unavailable before disconnect")
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	select {
	case err := <-acceptDone:
		if err != nil && !errors.Is(err, network.ErrClosed) {
			t.Fatalf("parity accept worker: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("parity accept worker did not exit: %v", ctx.Err())
	}
	disconnect := host.world.StepForTest()
	_, playerPresent := host.world.PlayerSnapshotFor(active.Session)
	_, _, chunkPresent := host.world.ChunkHash(core.Overworld, position)
	transcript = append(transcript, fmt.Sprintf(
		"DisconnectTick:player-present=%t:chunk-present=%t", playerPresent, chunkPresent,
	))
	if playerPresent {
		t.Fatal("parity player remains after deterministic disconnect tick")
	}
	if chunkPresent {
		t.Fatal("parity chunk remains loaded after deterministic disconnect tick")
	}
	result := parityResult{
		Transcript: transcript, PlayerHash: playerHash,
		ChunkHash: chunkHash, ChunkRevision: chunkRevision,
		MirrorHash: mirrorHash, MirrorRevision: mirrorRevision,
		Inventory:      snapshot.Inventory,
		DisconnectTick: disconnect.Tick > 0,
	}
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("parity Host.Shutdown: %v", err)
	}
	stored, err := store.LoadPlayer(ctx, identity.PlayerID)
	if err != nil {
		t.Fatalf("parity LoadPlayer: %v", err)
	}
	result.StoredInventory = stored.Inventory
	closeTransport()
	return result
}

func parityReadinessTranscript(
	t *testing.T,
	mirror *client.Mirror,
	messages []network.ServerMessage,
) []string {
	t.Helper()
	var lastState network.PlayerState
	hasState := false
	for _, message := range messages {
		switch message := message.(type) {
		case network.PlayerState:
			lastState = message
			hasState = true
		case network.ChunkSnapshot, network.KeepAlive, network.InventoryState,
			network.ItemDropUpserts, network.ItemDropRemoves, network.CraftingState,
			network.PassiveSpawn, network.PassiveState, network.PassiveDespawn:
		default:
			t.Fatalf("unexpected parity readiness message %T", message)
		}
	}
	if !hasState || !lastState.Ready {
		t.Fatalf("parity readiness ended without ready PlayerState: %+v", lastState)
	}
	lastState.ServerTick = 0
	// 两次运行在脚本开始前的 tick 数不同，绝对世界时间不属于业务对等内容；
	// 同一服务端上多客户端的时间一致性由多人昼夜测试覆盖。
	lastState.WorldTimeTicks = 0
	transcript := []string{fmt.Sprintf("PlayerState:%+v", lastState)}
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			position := core.ChunkPos{X: x, Z: z}
			hash, revision, ok := mirror.Hash(core.Overworld, position)
			if !ok {
				t.Fatalf("parity readiness mirror missing %+v", position)
			}
			transcript = append(transcript, fmt.Sprintf(
				"ChunkSnapshot:%d:%d:%d:%x", x, z, revision, hash,
			))
		}
	}
	return transcript
}

func openParityTransport(
	t *testing.T,
	host *Host,
	transport string,
	identity network.Identity,
) (network.ClientEndpoint, <-chan error, func()) {
	t.Helper()
	var clientStream network.ClientPacketStream
	var serverStream network.ServerPacketStream
	closeTransport := func() {}
	switch transport {
	case "memory":
		clientStream, serverStream = network.NewMemoryStreamPair(256)
	case "tcp":
		listener, err := networktcp.ListenTCP("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		accepted := make(chan struct {
			stream network.ServerPacketStream
			err    error
		}, 1)
		go func() {
			stream, err := listener.Accept(context.Background())
			accepted <- struct {
				stream network.ServerPacketStream
				err    error
			}{stream: stream, err: err}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		clientStream, err = networktcp.DialTCP(ctx, listener.Addr())
		cancel()
		if err != nil {
			_ = listener.Close()
			t.Fatal(err)
		}
		serverResult := <-accepted
		if serverResult.err != nil {
			_ = clientStream.Close()
			_ = listener.Close()
			t.Fatal(serverResult.err)
		}
		serverStream = serverResult.stream
		closeTransport = func() { _ = listener.Close() }
	default:
		t.Fatalf("unknown parity transport %q", transport)
	}
	acceptDone := make(chan error, 1)
	go func() { acceptDone <- host.AcceptStream(context.Background(), serverStream) }()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	endpoint, err := network.LoginClient(ctx, clientStream, identity)
	cancel()
	if err != nil {
		closeTransport()
		t.Fatal(err)
	}
	return endpoint, acceptDone, closeTransport
}

func parityStep(
	t *testing.T,
	host *Host,
	endpoint network.ClientEndpoint,
	mirror *client.Mirror,
) (contract.TickResult, []network.ServerMessage) {
	t.Helper()
	result := host.world.StepForTest()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	messages := make([]network.ServerMessage, 0, 16)
	for {
		message, err := endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("parity tick %d Recv: %v", result.Tick, err)
		}
		applyIntegrationMessage(t, mirror, message)
		messages = append(messages, message)
		if state, ok := message.(network.PlayerState); ok {
			if state.ServerTick != result.Tick {
				t.Fatalf("parity PlayerState tick=%d, want %d", state.ServerTick, result.Tick)
			}
			return result, messages
		}
	}
}

func parityViewLoaded(mirror *client.Mirror) bool {
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			if _, ok := mirror.Chunk(core.Overworld, core.ChunkPos{X: x, Z: z}); !ok {
				return false
			}
		}
	}
	return true
}

func parityBusinessMessage(
	t *testing.T,
	mirror *client.Mirror,
	message network.ServerMessage,
) []string {
	t.Helper()
	switch message := message.(type) {
	case network.ChunkSnapshot:
		hash, revision, ok := mirror.Hash(message.Dimension, message.Chunk)
		if !ok {
			t.Fatalf("parity snapshot mirror missing %+v", message.Chunk)
		}
		return []string{fmt.Sprintf("ChunkSnapshot:%d:%d:%d:%x", message.Chunk.X, message.Chunk.Z, revision, hash)}
	case network.BlockChanges:
		return []string{fmt.Sprintf("BlockChanges:%+v", message)}
	case network.ForgetChunks:
		return []string{fmt.Sprintf("ForgetChunks:%+v", message)}
	case network.PlayerState:
		message.ServerTick = 0
		message.WorldTimeTicks = 0
		return []string{fmt.Sprintf("PlayerState:%+v", message)}
	case network.CommandRejected:
		return []string{fmt.Sprintf("CommandRejected:%+v", message)}
	case network.PlaceBlockSucceeded:
		return []string{fmt.Sprintf("PlaceBlockSucceeded:%+v", message)}
	case network.InventoryState:
		return []string{fmt.Sprintf("InventoryState:%+v", message.Inventory)}
	case network.CraftingState:
		return []string{fmt.Sprintf("CraftingState:%+v", message)}
	case network.KeepAlive, network.Disconnect:
		return nil
	case network.PassiveSpawn, network.PassiveState, network.PassiveDespawn:
		// 被动牛是与本域脚本正交的背景模拟：自发出生 ID 由绝对 tick 派生，
		// 两条传输登录阶段消耗的 tick 数不同，位置与 ID 天然不可比；发布
		// 序列的跨传输等价由被动牛专用的 transcript parity 覆盖，这里只保
		// 证本域脚本不受背景消息干扰（与 KeepAlive/Disconnect 同例）。
		return nil
	default:
		t.Fatalf("unexpected parity business message %T", message)
		return nil
	}
}

// craftingGridParityResult 是网格命令脚本跑完后的可比结果：业务转写（含网格
// 状态、物品状态与拒绝）、末态权威网格与服务端派生产物、末态权威背包。
type craftingGridParityResult struct {
	Transcript []string
	FinalGrid  runtime.CraftingGrid
	FinalOuput core.ItemStack
	Inventory  core.Inventory
}

// TestMemoryTCPCraftingGridConvergence 覆盖 spec「网格状态私有同步且有界」与
// authoritative-crafting「合成遵循命令顺序并私有确认」的传输一致性要求：
// 同一串「网格移动 → 值域内语义拒绝 → 过期序列重放 → 取出 → 空网格取出拒绝 →
// 打开工作台 → 关闭工作台」命令在 Memory 与 TCP 两种传输下必须产生逐字段
// 相同的转写与末态。脚本用石锄配方（2×2、关镜像位的工具形状）作驱动样本。
func TestMemoryTCPCraftingGridConvergence(t *testing.T) {
	memory := runCraftingGridParityScript(t, "memory")
	tcp := runCraftingGridParityScript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		t.Fatalf("网格 Memory/TCP 未收敛\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	// 夹具自证：转写必须真的记下了网格状态与两种拒绝，否则空切片也能"一致"。
	// 成功且改变状态的命令恰有 7 条：4 次移动 + 1 次取出 + 打开 + 关闭；
	// 两条被拒命令（空源移动、空网格取出）不发布网格状态。
	states, rejects, inventoryStates := 0, 0, 0
	for _, entry := range memory.Transcript {
		switch {
		case strings.HasPrefix(entry, "CraftingState:"):
			states++
		case strings.HasPrefix(entry, "CommandRejected:"):
			rejects++
		case strings.HasPrefix(entry, "InventoryState:"):
			inventoryStates++
		}
	}
	if states != 7 {
		t.Fatalf("网格状态转写有 %d 条，想要恰好 7 条", states)
	}
	if rejects != 2 {
		t.Fatalf("拒绝转写有 %d 条，想要恰好 2 条（空源移动 + 空网格取出）", rejects)
	}
	if inventoryStates != 6 {
		t.Fatalf("物品状态转写有 %d 条，想要恰好 6 条（4 次移动 + 1 次取出 + 关闭回收）", inventoryStates)
	}
	full, _ := core.ItemMaxDurability(core.ItemStoneHoe)
	wantHoe := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: full}
	if got := countItem(memory.Inventory, core.ItemStoneHoe); got != 1 ||
		memory.Inventory.Hotbar.Slots[0] != wantHoe {
		t.Fatalf("取出后背包 = %+v，0 号格想要满耐久石锄 %+v", memory.Inventory, wantHoe)
	}
	if countItem(memory.Inventory, core.ItemStone) != 0 || countItem(memory.Inventory, core.ItemStick) != 0 {
		t.Fatalf("取出后原料未清零: %+v", memory.Inventory)
	}
	if memory.FinalGrid != (runtime.CraftingGrid{Size: runtime.CraftingGridSizePersonal}) {
		t.Fatalf("末态网格 = %+v，想要个人尺寸空网格", memory.FinalGrid)
	}
}

// runCraftingGridParityScript 在一种传输上跑完整段网格命令脚本并返回可比结果。
// 脚本形状照 `runEatingParityScript`：登录就绪后逐条发命令并转写业务消息。
func runCraftingGridParityScript(t *testing.T, transport string) craftingGridParityResult {
	t.Helper()
	identity := integrationIdentity(0x95, "GridCrafter")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	// 石锄的四个原料各占一格：整堆移动语义不能拆堆，同一物品的多格形状必须
	// 由多个独立栈摆放（这正是两次点击整堆语义下的真实玩家操作形态）。
	var initial core.Inventory
	initial.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	initial.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	initial.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStick, Count: 1}
	initial.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemStick, Count: 1}
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: [3]float32{0.5, 1.001, 0.5}}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location, Inventory: initial,
	})); err != nil {
		t.Fatal(err)
	}
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, flatGenerator{}, store)
	endpoint, acceptDone, closeTransport := openParityTransport(t, host, transport, identity)
	defer closeTransport()
	mirror := client.NewMirror()

	ready := false
	inventoryConfirmed := false
	for !ready || !inventoryConfirmed || !parityViewLoaded(mirror) {
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			switch message := message.(type) {
			case network.PlayerState:
				assertValidIntegrationPlayerState(t, message)
				ready = ready || message.Ready
			case network.InventoryState:
				inventoryConfirmed = message.Inventory == initial
			}
		}
	}
	// 脚本阶段在工作台脚手架就位后才开始：把出生支撑块换成工作台，供后半段
	// 验证「打开把尺寸提到 3、关闭回收降回 2」的发布序列。
	host.world.SetBlockForTest(core.BlockPos{}, core.WorkbenchID)

	result := craftingGridParityResult{Transcript: make([]string, 0, 64)}
	var lastGrid network.CraftingState
	seenGrid := false
	step := func(command network.ClientMessage) {
		t.Helper()
		if command != nil {
			sendIntegration(t, endpoint, command)
			waitIntegrationCondition(
				t, fmt.Sprintf("%s grid crafting %T queued", transport, command),
				func() bool { return len(host.world.incoming) > 0 },
			)
		}
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			result.Transcript = append(result.Transcript, parityBusinessMessage(t, mirror, message)...)
			switch message := message.(type) {
			case network.CraftingState:
				lastGrid = message
				seenGrid = true
			case network.InventoryState:
				result.Inventory = message.Inventory
			}
		}
	}

	sequence := uint64(0)
	next := func() uint64 { sequence++; return sequence }

	// 石锄形状（2×2、镜像位关闭）：格 0/2 是石头纵列，格 1/3 是木棍纵列。
	step(network.MoveCraftingStack{Sequence: next(), From: 9, To: 0})
	step(network.MoveCraftingStack{Sequence: next(), From: 10, To: 2})
	// 值域内的语义拒绝：空源（0 号格已搬空）必须拒绝且状态不变。
	step(network.MoveCraftingStack{Sequence: next(), From: 9, To: 1})
	step(network.MoveCraftingStack{Sequence: next(), From: 11, To: 1})
	step(network.MoveCraftingStack{Sequence: next(), From: 12, To: 3})
	if !seenGrid || lastGrid.Output == (core.ItemStack{}) {
		t.Fatalf("%s 摆满石锄形状后产物格 = %+v（seenGrid=%v），想要非空产物", transport, lastGrid.Output, seenGrid)
	}
	if id, _, matched := core.MatchCraftingGrid(lastGrid.Size, lastGrid.Slots); !matched || id != core.RecipeStoneHoe {
		t.Fatalf("%s 摆满后网格匹配 = (%d, %v)，想要石锄配方", transport, id, matched)
	}
	// 过期序列重放：旧序号命令必须被静默丢弃（无新网格/物品发布、无拒绝）。
	stepsBefore := len(result.Transcript)
	step(network.MoveCraftingStack{Sequence: 1, From: 9, To: 0})
	for _, entry := range result.Transcript[stepsBefore:] {
		if strings.HasPrefix(entry, "CraftingState:") ||
			strings.HasPrefix(entry, "InventoryState:") ||
			strings.HasPrefix(entry, "CommandRejected:") {
			t.Fatalf("%s 过期序列产生了新业务消息: %s", transport, entry)
		}
	}
	// 取出：产物入包、网格按消费后剩余重派生（四个格各恰减 1 后全空）。
	step(network.TakeCraftingOutput{Sequence: next()})
	// 空网格再取出：必须稳定拒绝。
	step(network.TakeCraftingOutput{Sequence: next()})
	// 打开工作台：尺寸 3 的完整网格状态必须发布给本人。
	step(network.OpenContainer{Sequence: next(), Pitch: -float32(math.Pi)/2 + 0.01})
	if !seenGrid || lastGrid.Size != 3 {
		t.Fatalf("%s 打开工作台后网格尺寸 = %d（seenGrid=%v），想要 3", transport, lastGrid.Size, seenGrid)
	}
	// 关闭工作台：先回收（网格本就为空）再把尺寸降回 2 并发布。
	step(network.CloseContainer{Sequence: next()})
	if !seenGrid || lastGrid.Size != 2 {
		t.Fatalf("%s 关闭工作台后网格尺寸 = %d（seenGrid=%v），想要 2", transport, lastGrid.Size, seenGrid)
	}

	host.mu.Lock()
	active := *host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	// engine 的网格只能在 `stepMu` 下读取：紧随其后的 endpoint 关闭会经
	// `endpointReader`→`DetachSession`→`UnregisterSession` 异步执行断线回收的
	// 网格写入，无锁读与该写入构成数据竞争（与 companion 测试的 `stepMu`
	// 直读先例同锁）。
	host.world.stepMu.Lock()
	result.FinalGrid, result.FinalOuput, _ = host.world.engine.PlayerCrafting(active.Session)
	host.world.stepMu.Unlock()
	if result.Inventory == (core.Inventory{}) {
		t.Fatalf("%s 脚本从未在 wire 上确认物品状态", transport)
	}

	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	select {
	case err := <-acceptDone:
		if err != nil && !errors.Is(err, network.ErrClosed) {
			t.Fatalf("%s grid crafting accept worker: %v", transport, err)
		}
	case <-ctx.Done():
		t.Fatalf("%s grid crafting accept worker did not exit: %v", transport, ctx.Err())
	}
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("%s grid crafting Host.Shutdown: %v", transport, err)
	}
	return result
}

// TestNaturalSeedFarmingMemoryTCPParity 覆盖 natural-grass-seeds 的传输一致性
// 要求：同一份自然种子固定脚本（零种子登录 → 原地采除自然短草 → 权威掉落 →
// 9/10 拾取 → 未翻地拒绝 → 翻地润湿 → 种植归零）在 Memory 与真实 TCP listener
// 两种传输下必须得到逐字段相同的结果。两次运行都从同一固定 seed 的生产
// `worldgen.New` 新世界开始（真实 Rust worldgen，流体开启），共用 Host 登录、
// session、entity/realm tick 与 packet/codec，没有本地捷径。
//
// 断言分两层：runner 内部对关键事实就地 Fatal（掉落恰好一颗种子、拾取恰在第
// 10 步、拒绝语义、湿耕地、stage0 作物、种子归零），两层比较再钉住「两次运行
// 的全部可观察结果一致」——包括登录背包、锄头耐久与种子格位。
func TestNaturalSeedFarmingMemoryTCPParity(t *testing.T) {
	memory := runNaturalSeedFarmingScript(t, "memory", nil)
	tcp := runNaturalSeedFarmingScript(t, "tcp", nil)
	if !reflect.DeepEqual(tcp, memory) {
		t.Fatalf("自然种子闭环 Memory/TCP 未收敛\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	// 夹具自证：比较的确实是「走了完整闭环」的非空结果，而不是两边同空的
	// 假一致。关键事实在 runner 内已就地断言，这里只复核比较面覆盖了它们。
	if memory.DropItem != core.ItemWheatSeeds || memory.DropCount != 1 {
		t.Fatalf("掉落 = %d×%d，想要恰好一颗小麦种子", memory.DropItem, memory.DropCount)
	}
	if memory.PickupDelaySteps != naturalFarmingPickupDelaySteps {
		t.Fatalf("拾取延迟步数 = %d，想要 %d", memory.PickupDelaySteps, naturalFarmingPickupDelaySteps)
	}
	if memory.Farmland != core.FarmlandWetID {
		t.Fatalf("最终耕地 = %d，想要自然海水润湿的 %d", memory.Farmland, core.FarmlandWetID)
	}
	if memory.Crop != core.WheatStage0ID || memory.TargetAfterMine != core.AirID {
		t.Fatalf("作物 = %d / 采除后 = %d，想要 stage0 / 空气", memory.Crop, memory.TargetAfterMine)
	}
	if seeds := countItem(memory.Inventory, core.ItemWheatSeeds); seeds != 0 {
		t.Fatalf("种植后种子 = %d 颗，想要归零", seeds)
	}
	if memory.PlantRejection == (network.CommandRejected{}) {
		t.Fatal("比较结果里缺少「未翻地就种」的拒绝记录")
	}
}
