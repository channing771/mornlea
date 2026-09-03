package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

type materialProcessingResult struct {
	Inventory       core.Inventory
	Furnace         world.FurnaceSlot
	ChunkRevision   uint64
	PersistenceHash [sha256.Size]byte
	Rejection       network.CommandRejected
}

func TestMaterialProcessingMemoryTCPParity(t *testing.T) {
	memory := runMaterialProcessingScript(t, "memory")
	tcp := runMaterialProcessingScript(t, "tcp")
	if memory != tcp {
		t.Fatalf("材料加工 Memory/TCP 未收敛\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
}

func runMaterialProcessingScript(t *testing.T, transport string) materialProcessingResult {
	t.Helper()
	identity := integrationIdentity(0x94, "MaterialProcessor")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	var initial core.Inventory
	initial.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakLog, Count: 1}
	initial.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemSand, Count: 2}
	initial.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemClay, Count: 1}
	initial.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemCoal, Count: 1}
	// 发光方块配方（2×2 玻璃）需要四个独立玻璃栈：整堆移动不能拆堆，夹具把
	// 4 块玻璃摊进 backpack[0..3] 各一块（统一视图格 18..21）。
	initial.Backpack[0] = core.ItemStack{Item: core.ItemGlass, Count: 1}
	initial.Backpack[1] = core.ItemStack{Item: core.ItemGlass, Count: 1}
	initial.Backpack[2] = core.ItemStack{Item: core.ItemGlass, Count: 1}
	initial.Backpack[3] = core.ItemStack{Item: core.ItemGlass, Count: 1}
	location := storage.PlayerLocation{
		Dimension: core.Overworld,
		Position:  [3]float32{0.5, 1.001, 0.5},
	}
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
	closed := false
	t.Cleanup(func() {
		if closed {
			return
		}
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	})

	mirror := client.NewMirror()
	step := func(command network.ClientMessage) []network.ServerMessage {
		t.Helper()
		if command != nil {
			sendIntegration(t, endpoint, command)
			waitIntegrationCondition(t, fmt.Sprintf("%s material processing %T queued", transport, command), func() bool {
				return len(host.world.incoming) > 0
			})
		}
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			if rejected, ok := message.(network.CommandRejected); ok {
				t.Fatalf("%s 材料加工命令被拒绝: %+v", transport, rejected)
			}
		}
		return messages
	}

	ready, inventoryReady := false, false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s material processing", transport),
		func() bool { return ready && inventoryReady && parityViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v inventoryReady=%v viewLoaded=%v", ready, inventoryReady, parityViewLoaded(mirror))
		},
		func() {
			messages := step(nil)
			for _, message := range messages {
				switch message := message.(type) {
				case network.PlayerState:
					ready = ready || message.Ready
				case network.InventoryState:
					inventoryReady = inventoryReady || message.Inventory == initial
				}
			}
		},
	)

	key := core.ChunkKey{Dimension: core.Overworld}
	index, ok := world.ChunkBlockIndex(core.BlockPos{})
	if !ok {
		t.Fatal("材料加工熔炉位置没有区块索引")
	}
	host.world.SetBlockForTest(core.BlockPos{}, core.FurnaceID)
	host.world.SetChunkFurnaceForTest(key, 0, world.FurnaceSlot{
		Generation: 1, Active: true, BlockIndex: index,
	})

	// 木板（recipe 7，1×1 原木）：把唯一的原木搬上网格再取出。
	step(network.MoveCraftingStack{Sequence: 1, From: 9, To: 0})
	craftedMessages := step(network.TakeCraftingOutput{Sequence: 2})
	wantCrafted := initial
	wantCrafted.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 4}
	if got, ok := materialProcessingInventory(craftedMessages); !ok || got != wantCrafted {
		t.Fatalf("%s 原木合成后的背包 = %+v, %v，想要 %+v", transport, got, ok, wantCrafted)
	}
	// 发光方块（recipe 8，2×2 玻璃）：四个独立玻璃栈分别搬进格 0..3 再取出；
	// 产物经 AddStack 落进搬空后仍被 0..3 占位的下一个空格——快捷栏 4。
	step(network.MoveCraftingStack{Sequence: 3, From: 18, To: 0})
	step(network.MoveCraftingStack{Sequence: 4, From: 19, To: 1})
	step(network.MoveCraftingStack{Sequence: 5, From: 20, To: 2})
	lightMessages := step(network.MoveCraftingStack{Sequence: 6, From: 21, To: 3})
	lightMessages = append(lightMessages, step(network.TakeCraftingOutput{Sequence: 7})...)
	wantLight := wantCrafted
	wantLight.Backpack[0] = core.ItemStack{}
	wantLight.Backpack[1] = core.ItemStack{}
	wantLight.Backpack[2] = core.ItemStack{}
	wantLight.Backpack[3] = core.ItemStack{}
	wantLight.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemLightBlock, Count: 4}
	if got, ok := materialProcessingInventory(lightMessages); !ok || got != wantLight {
		t.Fatalf("%s 发光方块合成后的背包 = %+v, %v，想要 %+v", transport, got, ok, wantLight)
	}
	// 网格已随取出清空：对空网格再取出必须稳定拒绝（对应旧脚本「原料不足」
	// 的拒绝路径）。
	sendIntegration(t, endpoint, network.TakeCraftingOutput{Sequence: 8})
	waitIntegrationCondition(t, fmt.Sprintf("%s 空网格取出拒绝 queued", transport), func() bool {
		return len(host.world.incoming) > 0
	})
	_, rejectedMessages := parityStep(t, host, endpoint, mirror)
	wantRejection := network.CommandRejected{Sequence: 8, Reason: network.RejectInvalidInput}
	var rejection network.CommandRejected
	foundRejection := false
	for _, message := range rejectedMessages {
		if candidate, ok := message.(network.CommandRejected); ok {
			rejection, foundRejection = candidate, true
		}
	}
	if !foundRejection || rejection != wantRejection {
		t.Fatalf("%s 发光方块原料不足拒绝 = %+v, %v，想要 %+v", transport, rejection, foundRejection, wantRejection)
	}
	host.mu.Lock()
	active := *host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
	if !ok || snapshot.Inventory != wantLight {
		t.Fatalf("%s 被拒绝后的权威背包 = %+v, %v，想要 %+v", transport, snapshot.Inventory, ok, wantLight)
	}

	openedMessages := step(network.OpenContainer{
		Sequence: 9, Pitch: -float32(math.Pi)/2 + 0.01,
	})
	opened := materialProcessingFurnaceState(t, transport, openedMessages)
	state := materialProcessingFurnaceState(t, transport, step(network.MoveContainerStack{
		Sequence: 10, Container: opened.Furnace, From: 1, To: core.FurnaceInputSlot,
	}))
	if state.Input != (core.ItemStack{Item: core.ItemSand, Count: 2}) {
		t.Fatalf("%s 沙子未进入熔炉: %+v", transport, state)
	}
	state = materialProcessingFurnaceState(t, transport, step(network.MoveContainerStack{
		Sequence: 11, Container: opened.Furnace, From: 3, To: core.FurnaceFuelSlot,
	}))
	if state.Fuel != (core.ItemStack{Item: core.ItemCoal, Count: 1}) {
		t.Fatalf("%s 煤炭未进入熔炉: %+v", transport, state)
	}

	for range core.FurnaceSmeltTicks {
		state = materialProcessingFurnaceState(t, transport, step(nil))
	}
	if state.Input != (core.ItemStack{Item: core.ItemSand, Count: 1}) ||
		state.Output != (core.ItemStack{Item: core.ItemGlass, Count: 1}) ||
		state.ProgressTicks != 0 || state.BurnTicks != 1400 {
		t.Fatalf("%s 沙子熔炼结果 = %+v", transport, state)
	}
	for range 17 {
		state = materialProcessingFurnaceState(t, transport, step(nil))
	}
	if state.ProgressTicks != 17 || state.BurnTicks != 1383 {
		t.Fatalf("%s 输入切换前计时 = %+v", transport, state)
	}

	state = materialProcessingFurnaceState(t, transport, step(network.MoveContainerStack{
		Sequence: 12, Container: opened.Furnace, From: 2, To: core.FurnaceInputSlot,
	}))
	if state.Input != (core.ItemStack{Item: core.ItemClay, Count: 1}) ||
		state.Output != (core.ItemStack{Item: core.ItemGlass, Count: 1}) ||
		state.ProgressTicks != 0 || state.BurnTicks != 1382 {
		t.Fatalf("%s 切换黏土块后的熔炉 = %+v", transport, state)
	}

	glassMessages := step(network.MoveContainerStack{
		Sequence: 13, Container: opened.Furnace, From: core.FurnaceOutputSlot, To: 5,
	})
	state = materialProcessingFurnaceState(t, transport, glassMessages)
	wantFinalInventory := wantLight
	wantFinalInventory.Hotbar.Slots[1] = core.ItemStack{}
	wantFinalInventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemSand, Count: 1}
	wantFinalInventory.Hotbar.Slots[3] = core.ItemStack{}
	wantFinalInventory.Hotbar.Slots[5] = core.ItemStack{Item: core.ItemGlass, Count: 1}
	gotInventory, ok := materialProcessingInventory(glassMessages)
	if !ok || gotInventory != wantFinalInventory {
		t.Fatalf("%s 取出玻璃后的背包 = %+v, %v，想要 %+v", transport, gotInventory, ok, wantFinalInventory)
	}
	if gotInventory.Hotbar.Slots[4] != (core.ItemStack{Item: core.ItemLightBlock, Count: 4}) ||
		gotInventory.Hotbar.Slots[5] != (core.ItemStack{Item: core.ItemGlass, Count: 1}) {
		t.Fatalf("%s 取出玻璃后的发光方块/玻璃栏位 = %+v / %+v", transport,
			gotInventory.Hotbar.Slots[4], gotInventory.Hotbar.Slots[5])
	}
	if state.Output != (core.ItemStack{}) || state.ProgressTicks != 0 || state.BurnTicks != 1382 {
		t.Fatalf("%s 取出玻璃后的熔炉 = %+v", transport, state)
	}

	for range core.FurnaceSmeltTicks {
		state = materialProcessingFurnaceState(t, transport, step(nil))
	}
	wantFurnace := world.FurnaceSlot{
		Generation: 1, Active: true, BlockIndex: index,
		Output:    core.ItemStack{Item: core.ItemBrick, Count: 1},
		BurnTicks: 1182,
	}
	if state.Input != wantFurnace.Input || state.Fuel != wantFurnace.Fuel ||
		state.Output != wantFurnace.Output || state.ProgressTicks != wantFurnace.ProgressTicks ||
		state.BurnTicks != wantFurnace.BurnTicks {
		t.Fatalf("%s 黏土块熔炼结果 = %+v", transport, state)
	}

	snapshot, ok = host.world.PlayerSnapshotFor(active.Session)
	if !ok || snapshot.Inventory != wantFinalInventory {
		t.Fatalf("%s 最终权威背包 = %+v, %v，想要 %+v", transport, snapshot.Inventory, ok, wantFinalInventory)
	}
	chunk, revision, ok := host.world.CloneReadyChunkForTest(key)
	if !ok {
		t.Fatalf("%s 最终权威熔炉区块未 Ready", transport)
	}
	authorityFurnace := chunk.Furnace(0)
	if authorityFurnace != wantFurnace {
		t.Fatalf("%s 最终权威熔炉 = %+v，想要 %+v", transport, authorityFurnace, wantFurnace)
	}

	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	select {
	case err := <-acceptDone:
		if err != nil && !errors.Is(err, network.ErrClosed) {
			t.Fatalf("%s 材料加工 accept worker: %v", transport, err)
		}
	case <-ctx.Done():
		t.Fatalf("%s 材料加工 accept worker 未退出: %v", transport, ctx.Err())
	}
	host.world.StepForTest()
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("%s 材料加工 Host.Shutdown: %v", transport, err)
	}
	storedPlayer, err := store.LoadPlayer(ctx, identity.PlayerID)
	if err != nil {
		t.Fatalf("%s 材料加工 LoadPlayer: %v", transport, err)
	}
	storedChunk, err := store.LoadChunk(ctx, key)
	if err != nil {
		t.Fatalf("%s 材料加工 LoadChunk: %v", transport, err)
	}
	storedFurnace := storedChunk.Chunk.Furnace(0)
	if storedPlayer.Inventory != wantFinalInventory || storedFurnace != wantFurnace ||
		storedChunk.Revision != revision {
		t.Fatalf("%s 持久化材料状态未收敛: inventory=%+v furnace=%+v revision=%d/%d",
			transport, storedPlayer.Inventory, storedFurnace, storedChunk.Revision, revision)
	}
	digest := sha256.Sum256(fmt.Appendf(
		nil, "%#v|%#v|%d", storedPlayer.Inventory, storedFurnace, storedChunk.Revision,
	))
	closeTransport()
	closed = true
	return materialProcessingResult{
		Inventory: snapshot.Inventory, Furnace: authorityFurnace,
		ChunkRevision: revision, PersistenceHash: digest, Rejection: rejection,
	}
}

func materialProcessingFurnaceState(
	t *testing.T,
	transport string,
	messages []network.ServerMessage,
) network.FurnaceState {
	t.Helper()
	for index := len(messages) - 1; index >= 0; index-- {
		if state, ok := messages[index].(network.FurnaceState); ok {
			return state
		}
	}
	t.Fatalf("%s 材料加工帧缺少 FurnaceState: %+v", transport, messages)
	return network.FurnaceState{}
}

func materialProcessingInventory(messages []network.ServerMessage) (core.Inventory, bool) {
	for index := len(messages) - 1; index >= 0; index-- {
		if state, ok := messages[index].(network.InventoryState); ok {
			return state.Inventory, true
		}
	}
	return core.Inventory{}, false
}
