package server

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

// stackSplitParityTranscript 是分堆双命令脚本在一种传输下的全部可比投影：
// 逐 tick 的业务消息转写（物品/网格/箱子/熔炉状态与拒绝）、按序收集的拒绝
// 应答、末次容器与网格状态以及末态权威背包。全部字段都不含绝对 tick，
// Memory 与 TCP 的录像可直接逐位比对。
type stackSplitParityTranscript struct {
	Events       []string
	Rejections   []network.CommandRejected
	LastCrafting network.CraftingState
	LastChest    network.ChestState
	LastFurnace  network.FurnaceState
	Inventory    core.Inventory
}

// stackSplitParityInventory 构造脚本初始背包：石头驱动背包/合成域的半组与
// 单件拆分，泥土驱动箱子域的双向部分移动，煤与粗铁驱动熔炉域的燃料约束与
// 输入优先快捷搬运。
func stackSplitParityInventory() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 12}
	inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemCoal, Count: 3}
	inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemRawIron, Count: 5}
	return inventory
}

// stackSplitParityFinalInventory 是按各命令的权威语义逐条推导的末态背包：
// 半组按 ceil、单件按 1、快捷搬运按对侧区域固定序；网格与箱子/熔炉的
// 余量不在背包内，另行由末次状态消息断言。
func stackSplitParityFinalInventory() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 8}
	inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemCoal, Count: 2}
	inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemRawIron, Count: 5}
	inventory.Backpack[10-9] = core.ItemStack{Item: core.ItemStone, Count: 1}
	inventory.Backpack[20-9] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	inventory.Backpack[21-9] = core.ItemStack{Item: core.ItemDirt, Count: 3}
	return inventory
}

// stackSplitParityEvent 把一条业务消息转写为可比字符串；绝对 tick 类消息
// （PlayerState/区块/环境实体）不属于分堆命令的对等观察面，直接跳过。
func stackSplitParityEvent(message network.ServerMessage) []string {
	switch message := message.(type) {
	case network.InventoryState:
		return []string{fmt.Sprintf("InventoryState:%+v", message.Inventory)}
	case network.CraftingState:
		return []string{fmt.Sprintf("CraftingState:%+v", message)}
	case network.ChestState:
		return []string{fmt.Sprintf("ChestState:%+v", message)}
	case network.FurnaceState:
		return []string{fmt.Sprintf("FurnaceState:%+v", message)}
	case network.CommandRejected:
		return []string{fmt.Sprintf("CommandRejected:%+v", message)}
	default:
		return nil
	}
}

// TestStackSplitCommandsMemoryTCPParity 覆盖分堆双命令的服务端接线契约：
// 同一串「背包域半组/单件/快捷 → 合成域半组/单件/快捷 → 箱子域部分移动与
// 快捷搬运 → 熔炉域输入优先快捷搬运与燃料约束」脚本在 Memory 与 TCP 两种
// 传输下必须得到逐字段相同的投影（含六条值域内语义拒绝）。
//
// 脚本刻意混入三类拒绝：空源（invalid_input）、个人网格扩展格（invalid_slot）
// 与熔炉燃料槽非煤（invalid_input）——只比对成功路的话，拒绝映射或视图域
// 翻译坏了照样两传输一致。熔炉阶段让输入与燃料永不同时非空：一旦点燃，
// 进度字段随登录阶段错位的绝对 tick 漂移，两条传输天然不可比。
func TestStackSplitCommandsMemoryTCPParity(t *testing.T) {
	memory := runStackSplitParityScript(t, "memory")
	tcp := runStackSplitParityScript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		t.Fatalf("分堆 Memory/TCP transcript 不一致\nmemory=%+v\ntcp=%+v", memory, tcp)
	}

	// 夹具自证：比较的确实是「半组/单件/快捷三组命令都真实生效」的非空
	// 结果，而不是两边同空的假一致。逐类计数与末态语义断言共同钉住。
	counts := map[string]int{}
	for _, event := range memory.Events {
		if index := strings.IndexByte(event, ':'); index > 0 {
			counts[event[:index]]++
		}
	}
	// 容器域拒绝命令同样向查看者回显一条内容不变的容器状态（既有发布
	// 语义），因此箱子/熔炉转写各含一条（箱子）与两条（熔炉）拒绝回显。
	wantCounts := map[string]int{
		"InventoryState":  15,
		"CraftingState":   3,
		"ChestState":      6,
		"FurnaceState":    8,
		"CommandRejected": 6,
	}
	for name, want := range wantCounts {
		if got := counts[name]; got != want {
			t.Fatalf("%s 转写有 %d 条，想要恰好 %d 条（实际计数 %v）", name, got, want, counts)
		}
	}

	// 六条拒绝按脚本顺序：空源、空源、个人扩展格、空箱格、非煤入燃料、
	// 非熔炼物快捷搬运——值域层已由协议校验拦截，这里全是在值域内的
	// 权威语义拒绝，必须走同一条 CommandRejected 投影。
	wantRejections := []network.CommandRejected{
		{Sequence: 3, Reason: network.RejectInvalidInput},
		{Sequence: 5, Reason: network.RejectInvalidInput},
		{Sequence: 8, Reason: network.RejectInvalidSlot},
		{Sequence: 14, Reason: network.RejectInvalidInput},
		{Sequence: 20, Reason: network.RejectInvalidInput},
		{Sequence: 21, Reason: network.RejectInvalidInput},
	}
	if !reflect.DeepEqual(memory.Rejections, wantRejections) {
		t.Fatalf("拒绝序列 = %+v，想要 %+v", memory.Rejections, wantRejections)
	}

	if got := memory.Inventory; got != stackSplitParityFinalInventory() {
		t.Fatalf("末态权威背包 = %+v，想要 %+v", got, stackSplitParityFinalInventory())
	}
	if memory.LastCrafting.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("末次网格 0 号格 = %+v，想要 3 个石头", memory.LastCrafting.Slots[0])
	}
	// 箱子末态：0 号格被快捷搬空，4 号格保留背包半组移入的 6 个泥土。
	if memory.LastChest.Items[0] != (core.ItemStack{}) ||
		memory.LastChest.Items[4] != (core.ItemStack{Item: core.ItemDirt, Count: 6}) {
		t.Fatalf("末次箱子状态异常: %+v", memory.LastChest.Items)
	}
	// 熔炉末态：输入被搬空、燃料恰好 1 个煤——半组/单件与输入优先快捷
	// 搬运全部经过同一条 FurnaceState 投影。
	if memory.LastFurnace.Input != (core.ItemStack{}) ||
		memory.LastFurnace.Fuel != (core.ItemStack{Item: core.ItemCoal, Count: 1}) {
		t.Fatalf("末次熔炉状态异常: input=%+v fuel=%+v",
			memory.LastFurnace.Input, memory.LastFurnace.Fuel)
	}
}

// runStackSplitParityScript 在一种传输上跑完整段分堆脚本并返回可比结果。
// 脚本形状照 `runCraftingGridParityScript`：登录就绪后在脚下方块放置容器，
// 单命令单 tick 逐条推进并转写业务消息；箱子阶段结束后把同一方块换成
// 熔炉，复用同一俯视射线打开第二段容器视图。
func runStackSplitParityScript(t *testing.T, transport string) stackSplitParityTranscript {
	t.Helper()
	identity := integrationIdentity(0xa7, "StackSplitter")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 6, Seed: 42, SpawnDimension: core.Overworld,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	initial := stackSplitParityInventory()
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
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	t.Cleanup(func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	})
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

	// 容器夹具：脚下方块换成箱子并预置 6 个泥土，供箱子域部分移动与
	// 快捷搬运双向使用（与箱子纵向测试同一俯视射线）。
	key := core.ChunkKey{Dimension: core.Overworld}
	blockIndex, ok := world.ChunkBlockIndex(core.BlockPos{})
	if !ok {
		t.Fatal("分堆 parity 容器位置没有区块索引")
	}
	var chestItems [core.ChestSlots]core.ItemStack
	chestItems[0] = core.ItemStack{Item: core.ItemDirt, Count: 6}
	host.world.SetBlockForTest(core.BlockPos{}, core.ChestID)
	host.world.SetChunkChestForTest(key, 0, world.ChestSlot{
		Generation: 1, Active: true, BlockIndex: blockIndex, Items: chestItems,
	})

	result := stackSplitParityTranscript{Events: make([]string, 0, 64)}
	var chestRef core.ContainerRef
	var furnaceRef core.FurnaceRef
	step := func(command network.ClientMessage) {
		t.Helper()
		sendIntegration(t, endpoint, command)
		waitIntegrationCondition(
			t, fmt.Sprintf("%s stack split %T queued", transport, command),
			func() bool { return len(host.world.incoming) > 0 },
		)
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			result.Events = append(result.Events, stackSplitParityEvent(message)...)
			switch message := message.(type) {
			case network.PlayerState:
				assertValidIntegrationPlayerState(t, message)
			case network.CommandRejected:
				result.Rejections = append(result.Rejections, message)
			case network.CraftingState:
				result.LastCrafting = message
			case network.ChestState:
				result.LastChest = message
				if chestRef == (core.ContainerRef{}) {
					chestRef = message.Chest
				}
			case network.FurnaceState:
				result.LastFurnace = message
				if furnaceRef == (core.FurnaceRef{}) {
					furnaceRef = message.Furnace
				}
			}
		}
	}

	sequence := uint64(0)
	next := func() uint64 { sequence++; return sequence }

	// 背包域：7 个石头按半组（ceil=4）与单件（1）拆进背包，快捷搬运再把
	// 背包格整堆并回快捷栏；空源的两条命令必须被拒绝且零状态变更。
	step(network.MoveStackPartial{
		Sequence: next(), View: network.StackViewInventory, From: 0, To: 9,
	})
	step(network.MoveStackPartial{
		Sequence: next(), View: network.StackViewInventory, From: 0, To: 10, Single: true,
	})
	step(network.MoveStackPartial{
		Sequence: next(), View: network.StackViewInventory, From: 5, To: 11,
	})
	step(network.QuickMoveStack{
		Sequence: next(), View: network.StackViewInventory, From: 9,
	})
	step(network.QuickMoveStack{
		Sequence: next(), View: network.StackViewInventory, From: 9,
	})

	// 合成域：个人 2×2 网格内半组/单件入网格、网格格快捷搬回背包；个人
	// 扩展格（4..8）是值域内非法来源，必须以槽位越界拒绝。
	step(network.MoveStackPartial{
		Sequence: next(), View: network.StackViewCrafting, From: 9, To: 0,
	})
	step(network.MoveStackPartial{
		Sequence: next(), View: network.StackViewCrafting, From: 9, To: 1, Single: true,
	})
	step(network.MoveStackPartial{
		Sequence: next(), View: network.StackViewCrafting, From: 4, To: 0,
	})
	step(network.QuickMoveStack{
		Sequence: next(), View: network.StackViewCrafting, From: 1,
	})

	// 箱子域：打开后双向部分移动（箱子→背包单件/半组、背包→箱子半组），
	// 空箱格快捷搬运整单拒绝，非空箱格快捷搬运按拾取序并回背包。
	step(network.OpenContainer{Sequence: next(), Pitch: chestScriptLookDown})
	if chestRef == (core.ContainerRef{}) {
		t.Fatalf("%s 打开箱子后未捕获容器引用", transport)
	}
	step(network.MoveStackPartial{
		Sequence: next(), Container: chestRef, View: network.StackViewContainer,
		From: core.ChestFirstSlot, To: 20, Single: true,
	})
	step(network.MoveStackPartial{
		Sequence: next(), Container: chestRef, View: network.StackViewContainer,
		From: core.ChestFirstSlot, To: 21,
	})
	step(network.MoveStackPartial{
		Sequence: next(), Container: chestRef, View: network.StackViewContainer,
		From: 1, To: core.ChestFirstSlot + 4,
	})
	step(network.QuickMoveStack{
		Sequence: next(), Container: chestRef, View: network.StackViewContainer,
		From: core.ChestFirstSlot + 1,
	})
	step(network.QuickMoveStack{
		Sequence: next(), Container: chestRef, View: network.StackViewContainer,
		From: core.ChestFirstSlot,
	})
	step(network.CloseContainer{Sequence: next()})

	// 熔炉域：同一方块换成熔炉（同一俯视射线）。箱子槽同步按真实破坏路径
	// 停用（仅保留 Generation），否则存档编码会因活动箱子槽不再指向箱子
	// 方块而拒绝区块保存。输入优先级由「粗铁快捷搬运先试输入槽」体现；
	// 石头入燃料与泥土快捷搬运都是值域内语义拒绝。
	// 输入与燃料永不同时非空，避免点燃后的绝对 tick 漂移（见测试注释）。
	host.world.SetBlockForTest(core.BlockPos{}, core.FurnaceID)
	host.world.SetChunkChestForTest(key, 0, world.ChestSlot{Generation: 1})
	host.world.SetChunkFurnaceForTest(key, 0, world.FurnaceSlot{
		Generation: 1, Active: true, BlockIndex: blockIndex,
	})
	step(network.OpenContainer{Sequence: next(), Pitch: chestScriptLookDown})
	if furnaceRef == (core.FurnaceRef{}) {
		t.Fatalf("%s 打开熔炉后未捕获容器引用", transport)
	}
	step(network.MoveStackPartial{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: 3, To: core.FurnaceInputSlot, Single: true,
	})
	step(network.MoveStackPartial{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: 3, To: core.FurnaceInputSlot,
	})
	step(network.MoveStackPartial{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: 0, To: core.FurnaceFuelSlot,
	})
	step(network.QuickMoveStack{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: 1,
	})
	step(network.QuickMoveStack{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: 3,
	})
	step(network.QuickMoveStack{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: core.FurnaceInputSlot,
	})
	step(network.MoveStackPartial{
		Sequence: next(), Container: furnaceRef, View: network.StackViewContainer,
		From: 2, To: core.FurnaceFuelSlot, Single: true,
	})
	step(network.CloseContainer{Sequence: next()})

	host.mu.Lock()
	active := *host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
	if !ok {
		t.Fatalf("%s 没有分堆 parity 权威玩家快照", transport)
	}
	result.Inventory = snapshot.Inventory
	if result.LastChest == (network.ChestState{}) || result.LastFurnace == (network.FurnaceState{}) {
		t.Fatalf("%s 脚本未在 wire 上确认箱子/熔炉状态", transport)
	}
	return result
}
