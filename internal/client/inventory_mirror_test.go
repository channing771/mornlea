package client_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	networktcp "github.com/channing771/mornlea/packages/shared/network/tcp"
)

func stockedMirrorInventory() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Selected = 5
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 12}
	inventory.Backpack[2] = core.ItemStack{Item: core.ItemDirt, Count: 30}
	return inventory
}

func TestInventoryMirrorStartsUnconfirmed(t *testing.T) {
	var mirror client.InventoryMirror
	if hotbar, ok := mirror.State(); ok || hotbar != (core.Inventory{}) {
		t.Fatalf("初始镜像 = %+v, %v，想要空且未确认", hotbar, ok)
	}
}

func TestInventoryMirrorAppliesAuthoritativeState(t *testing.T) {
	var mirror client.InventoryMirror
	want := stockedMirrorInventory()
	if err := mirror.Apply(network.InventoryState{Inventory: want}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, ok := mirror.State()
	if !ok || got != want {
		t.Fatalf("镜像 = %+v, %v，想要 %+v, true", got, ok, want)
	}
}

func TestInventoryMirrorRejectsInvalidStateWithoutPartialApply(t *testing.T) {
	var mirror client.InventoryMirror
	confirmed := stockedMirrorInventory()
	if err := mirror.Apply(network.InventoryState{Inventory: confirmed}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	invalid := []core.Inventory{
		{Hotbar: core.Hotbar{Selected: core.HotbarSlots}},
		{Hotbar: core.Hotbar{Slots: [core.HotbarSlots]core.ItemStack{
			0: {Item: core.ItemID(4242), Count: 1},
		}}},
		{Backpack: [core.BackpackSlots]core.ItemStack{
			0: {Item: core.ItemDirt, Count: core.MaxStackCount + 1},
		}},
		{Backpack: [core.BackpackSlots]core.ItemStack{
			core.BackpackSlots - 1: {Item: core.ItemNone, Count: 2},
		}},
	}
	for _, inventory := range invalid {
		if err := mirror.Apply(network.InventoryState{Inventory: inventory}); err == nil {
			t.Fatalf("非法状态 %+v 被接受", inventory)
		}
		got, ok := mirror.State()
		if !ok || got != confirmed {
			t.Fatalf("非法状态改写了镜像: %+v, %v", got, ok)
		}
	}
}

func TestInventoryMirrorResetDropsPreviousSession(t *testing.T) {
	var mirror client.InventoryMirror
	if err := mirror.Apply(network.InventoryState{Inventory: stockedMirrorInventory()}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	mirror.Reset()
	if hotbar, ok := mirror.State(); ok || hotbar != (core.Inventory{}) {
		t.Fatalf("reset 后镜像 = %+v, %v，想要空且未确认", hotbar, ok)
	}
}

func stockedCraftingState() network.CraftingState {
	var state network.CraftingState
	state.Size = 3
	state.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	state.Slots[3] = core.ItemStack{Item: core.ItemStick, Count: 1}
	state.Output = core.ItemStack{Item: core.ItemStoneBrick, Count: 4}
	return state
}

func TestCraftingMirrorStartsUnconfirmed(t *testing.T) {
	var mirror client.CraftingMirror
	if state, ok := mirror.State(); ok || state != (network.CraftingState{}) {
		t.Fatalf("初始镜像 = %+v, %v，想要空且未确认", state, ok)
	}
}

// TestCraftingMirrorAppliesLatestWins 锁死 latest-wins：镜像只保留最后一份
// 合法完整网格状态，绝不预测、绝不增量合并。
func TestCraftingMirrorAppliesLatestWins(t *testing.T) {
	var mirror client.CraftingMirror
	first := stockedCraftingState()
	if err := mirror.Apply(first); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	second := network.CraftingState{Size: 2}
	second.Slots[1] = core.ItemStack{Item: core.ItemWheat, Count: 1}
	if err := mirror.Apply(second); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, ok := mirror.State()
	if !ok || got != second {
		t.Fatalf("镜像 = %+v, %v，想要 latest-wins 的 %+v", got, ok, second)
	}
}

// TestCraftingMirrorRejectsInvalidStateWithoutPartialApply 与背包镜像同形：
// 非法状态整包拒绝，已确认值原样保留。
func TestCraftingMirrorRejectsInvalidStateWithoutPartialApply(t *testing.T) {
	var mirror client.CraftingMirror
	confirmed := stockedCraftingState()
	if err := mirror.Apply(confirmed); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	invalid := []network.CraftingState{
		{Size: 1},
		{Size: 4},
		{Size: 2, Slots: [core.CraftingGridSlots]core.ItemStack{
			5: {Item: core.ItemStone, Count: 1},
		}},
		{Size: 3, Slots: [core.CraftingGridSlots]core.ItemStack{
			0: {Item: core.ItemID(9999), Count: 1},
		}},
		{Size: 3, Output: core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount + 1}},
	}
	for _, state := range invalid {
		if err := mirror.Apply(state); err == nil {
			t.Fatalf("非法状态 %+v 被接受", state)
		}
		got, ok := mirror.State()
		if !ok || got != confirmed {
			t.Fatalf("非法状态改写了镜像: %+v, %v", got, ok)
		}
	}
}

// TestCraftingMirrorResetDropsPreviousSession 覆盖「断线清空客户端镜像」：
// 重连后以服务端发布的状态为准，不得继承上一会话的网格。
func TestCraftingMirrorResetDropsPreviousSession(t *testing.T) {
	var mirror client.CraftingMirror
	if err := mirror.Apply(stockedCraftingState()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	mirror.Reset()
	if state, ok := mirror.State(); ok || state != (network.CraftingState{}) {
		t.Fatalf("reset 后镜像 = %+v, %v，想要空且未确认", state, ok)
	}
}

// gridMirrorTransportResult 是同一条网格命令序列经一种传输跑完后的可比结果：
// 末态网格镜像、产物镜像、背包镜像与全部拒绝。
type gridMirrorTransportResult struct {
	Grid       network.CraftingState
	Inventory  core.Inventory
	Rejections []network.CommandRejected
}

// TestCraftingGridMirrorsConvergeAcrossMemoryAndTCP 证明同一命令序列经 Memory
// 与 TCP 两种传输得到逐字段相同的 grid、output、inventory 与拒绝结果：
// 服务端侧由一个只依赖 core 导出 API（`MatchCraftingGrid`/`Recipe`/
// `ConsumeRecipe`/`Inventory.AddStack`）的确定性 mini-authority 回答，权威
// 语义本身由 sim 的网格测试锁定，这里锁定的是「客户端镜像 + 传输 + 编解码」
// 这半条链路在两种传输下不可分辨。
func TestCraftingGridMirrorsConvergeAcrossMemoryAndTCP(t *testing.T) {
	memory := runGridMirrorScript(t, "memory")
	tcp := runGridMirrorScript(t, "tcp")
	if !reflect.DeepEqual(tcp, memory) {
		t.Fatalf("网格镜像 Memory/TCP 未收敛\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	full, _ := core.ItemMaxDurability(core.ItemStoneHoe)
	wantHoe := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: full}
	if memory.Grid.Size != 2 || memory.Grid.Slots != ([core.CraftingGridSlots]core.ItemStack{}) {
		t.Fatalf("末态网格 = %+v，想要个人尺寸空网格", memory.Grid)
	}
	if got := memory.Inventory.Hotbar.Slots[0]; got != wantHoe {
		t.Fatalf("末态背包 0 号格 = %+v，想要 %+v", got, wantHoe)
	}
	wantRejections := []network.CommandRejected{
		{Sequence: 5, Reason: network.RejectInvalidInput},
		{Sequence: 7, Reason: network.RejectInvalidInput},
	}
	if !reflect.DeepEqual(memory.Rejections, wantRejections) {
		t.Fatalf("拒绝序列 = %+v，想要 %+v", memory.Rejections, wantRejections)
	}
}

// runGridMirrorScript 在一种传输上把命令序列发给 mini-authority 并把应答
// 逐条喂给客户端镜像，返回可比结果。命令序列与权威回应在两种传输上完全
// 相同，任何编码/传输差异都会在镜像末态或拒绝序列上现形。
func runGridMirrorScript(t *testing.T, transport string) gridMirrorTransportResult {
	t.Helper()
	var clientStream network.ClientPacketStream
	var serverStream network.ServerPacketStream
	if transport == "memory" {
		clientStream, serverStream = network.NewMemoryStreamPair(64)
	} else {
		listener, err := networktcp.ListenTCP("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		accepted := make(chan network.ServerPacketStream, 1)
		go func() {
			stream, acceptErr := listener.Accept(context.Background())
			accepted <- stream
			if acceptErr != nil {
				return
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		clientStream, err = networktcp.DialTCP(ctx, listener.Addr())
		cancel()
		if err != nil {
			_ = listener.Close()
			t.Fatal(err)
		}
		serverStream = <-accepted
		t.Cleanup(func() { _ = listener.Close() })
	}
	t.Cleanup(func() {
		_ = clientStream.Close()
		_ = serverStream.Close()
	})

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- serveGridAuthorityScript(t, serverStream)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	endpoint, err := network.LoginClient(ctx, clientStream, network.Identity{
		PlayerID: gridMirrorPlayerID(t), DisplayName: "GridMirror",
	})
	if err != nil {
		t.Fatalf("%s LoginClient: %v", transport, err)
	}

	var crafting client.CraftingMirror
	var inventory client.InventoryMirror
	result := gridMirrorTransportResult{}
	for _, command := range gridMirrorCommands() {
		if err := endpoint.Send(ctx, command); err != nil {
			t.Fatalf("%s Send(%T): %v", transport, command, err)
		}
		for {
			message, err := endpoint.Recv(ctx)
			if err != nil {
				t.Fatalf("%s Recv: %v", transport, err)
			}
			switch message := message.(type) {
			case network.CraftingState:
				if err := crafting.Apply(message); err != nil {
					t.Fatalf("%s CraftingMirror.Apply: %v", transport, err)
				}
			case network.InventoryState:
				if err := inventory.Apply(message); err != nil {
					t.Fatalf("%s InventoryMirror.Apply: %v", transport, err)
				}
			case network.CommandRejected:
				result.Rejections = append(result.Rejections, message)
			default:
				t.Fatalf("%s 意外业务消息 %T", transport, message)
			}
			// 每条命令恰好一条应答（网格状态或拒绝），物品状态只在背包变化时
			// 追加；因此收到网格状态或拒绝即为本条命令的边界。
			switch message.(type) {
			case network.CraftingState, network.CommandRejected:
				goto nextCommand
			}
		}
	nextCommand:
	}
	state, ok := crafting.State()
	if !ok {
		t.Fatalf("%s 镜像从未确认网格状态", transport)
	}
	result.Grid = state
	result.Inventory, _ = inventory.State()
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("%s 权威脚本: %v", transport, err)
	}
	// 断线后清空：镜像层契约在独立用例中锁定，这里只确认 Reset 可安全重放。
	crafting.Reset()
	if _, still := crafting.State(); still {
		t.Fatalf("%s Reset 后镜像仍持有状态", transport)
	}
	return result
}

// gridMirrorCommands 是驱动脚本的命令序列：摆石锄 → 空源拒绝 → 取出石锄 →
// 空网格取出拒绝 → 把产物搬回网格再搬走（网格回到个人尺寸空网格的末态）。
func gridMirrorCommands() []network.ClientMessage {
	return []network.ClientMessage{
		network.MoveCraftingStack{Sequence: 1, From: 9, To: 0},
		network.MoveCraftingStack{Sequence: 2, From: 10, To: 2},
		network.MoveCraftingStack{Sequence: 3, From: 11, To: 1},
		network.MoveCraftingStack{Sequence: 4, From: 12, To: 3},
		network.MoveCraftingStack{Sequence: 5, From: 13, To: 1},
		network.TakeCraftingOutput{Sequence: 6},
		network.TakeCraftingOutput{Sequence: 7},
		network.MoveCraftingStack{Sequence: 8, From: 9, To: 0},
		network.MoveCraftingStack{Sequence: 9, From: 0, To: 9},
	}
}

// serveGridAuthorityScript 用 core 导出 API 实现最小确定性权威，按收到的
// 命令逐条应答；语义（整堆移动、形状匹配、消费、产物入包）全部复用 core
// 的导出实现，不复制 sim 的私有逻辑。
func serveGridAuthorityScript(t *testing.T, stream network.ServerPacketStream) error {
	t.Helper()
	pending, err := network.BeginServerLogin(context.Background(), stream, 0)
	if err != nil {
		return err
	}
	var endpoint network.ServerEndpoint
	if err := pending.Accept(context.Background(), func(attached network.ServerEndpoint) error {
		endpoint = attached
		return nil
	}); err != nil {
		return err
	}
	authority := newGridAuthority()
	ctx := context.Background()
	for _, want := range gridMirrorCommands() {
		got, err := endpoint.Recv(ctx)
		if err != nil {
			return err
		}
		if got != want {
			t.Errorf("权威收到 %T=%+v，想要 %+v", got, got, want)
			return nil
		}
		if sendErr := authority.respond(ctx, endpoint, got); sendErr != nil {
			return sendErr
		}
	}
	return nil
}

// gridAuthority 是只依赖 core 导出 API 的最小权威网格状态机。
type gridAuthority struct {
	inventory core.Inventory
	size      uint8
	slots     [core.CraftingGridSlots]core.ItemStack
}

func newGridAuthority() *gridAuthority {
	authority := &gridAuthority{size: 2}
	// 石锄的四个独立原料栈：石头×2 在快捷栏 0/1，木棍×2 在快捷栏 2/3。
	authority.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	authority.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	authority.inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStick, Count: 1}
	authority.inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemStick, Count: 1}
	return authority
}

// viewStack 读取统一视图格（网格 0..8、物品栏 9..44）。
func (authority *gridAuthority) viewStack(slot uint8) (core.ItemStack, bool) {
	if slot >= core.CraftingGridSlots {
		return authority.inventory.Slot(slot - core.CraftingGridSlots)
	}
	return authority.slots[slot], true
}

// setViewStack 写入统一视图格；返回 false 表示索引非法。
func (authority *gridAuthority) setViewStack(slot uint8, stack core.ItemStack) bool {
	if slot >= core.CraftingGridSlots {
		next, ok := authority.inventory.SetSlot(slot-core.CraftingGridSlots, stack)
		if !ok {
			return false
		}
		authority.inventory = next
		return true
	}
	authority.slots[slot] = stack
	return true
}

// respond 按命令类型应答：成功动作发布完整网格状态（背包变化时先发布完整
// 物品状态，与服务端发布顺序一致），失败按既有拒绝路径回拒。
func (authority *gridAuthority) respond(
	ctx context.Context,
	endpoint network.ServerEndpoint,
	message network.ClientMessage,
) error {
	sequence := uint64(0)
	var gridDirty, inventoryDirty bool
	switch command := message.(type) {
	case network.MoveCraftingStack:
		sequence = command.Sequence
		gridDirty, inventoryDirty = authority.move(command.From, command.To)
	case network.TakeCraftingOutput:
		sequence = command.Sequence
		gridDirty, inventoryDirty = authority.take()
	default:
		return nil
	}
	if !gridDirty {
		return endpoint.Send(ctx, network.CommandRejected{
			Sequence: sequence, Reason: network.RejectInvalidInput,
		})
	}
	if inventoryDirty {
		if err := endpoint.Send(ctx, network.InventoryState{Inventory: authority.inventory}); err != nil {
			return err
		}
	}
	_, output, _ := core.MatchCraftingGrid(authority.size, authority.slots)
	return endpoint.Send(ctx, network.CraftingState{
		Size: authority.size, Slots: authority.slots, Output: output,
	})
}

// move 在网格与背包之间执行一次两次点击整堆移动；语义与 sim 的
// `applyMoveCraftingStack` 一致（空目标接收整堆、同类按上限合并、异类拒绝），
// 但只用 core 导出 API 表达，不依赖 sim。
func (authority *gridAuthority) move(from, to uint8) (gridDirty, inventoryDirty bool) {
	if from >= core.CraftingGridSlots+core.InventorySlots ||
		to >= core.CraftingGridSlots+core.InventorySlots {
		return false, false
	}
	if from == to || (from >= core.CraftingGridSlots && to >= core.CraftingGridSlots) {
		return false, false
	}
	if from < core.CraftingGridSlots && from >= authority.size*authority.size {
		return false, false
	}
	if to < core.CraftingGridSlots && to >= authority.size*authority.size {
		return false, false
	}
	source, _ := authority.viewStack(from)
	if source.Item == core.ItemNone {
		return false, false
	}
	target, _ := authority.viewStack(to)
	var nextSource, nextTarget core.ItemStack
	switch {
	case target.Item == core.ItemNone:
		nextSource, nextTarget = core.ItemStack{}, source
	case target.Item == source.Item:
		limit, hasLimit := core.ItemStackLimit(source.Item)
		if !hasLimit || target.Count >= limit {
			return false, false
		}
		moved := min(limit-target.Count, source.Count)
		nextTarget = target
		nextTarget.Count += moved
		if source.Count > moved {
			nextSource = core.ItemStack{Item: source.Item, Count: source.Count - moved}
		}
	default:
		return false, false
	}
	if !authority.setViewStack(from, nextSource) || !authority.setViewStack(to, nextTarget) {
		return false, false
	}
	return true, true
}

// take 执行一次产物取出：匹配、消费、产物经 AddStack 入包。
func (authority *gridAuthority) take() (gridDirty, inventoryDirty bool) {
	id, output, matched := core.MatchCraftingGrid(authority.size, authority.slots)
	if !matched {
		return false, false
	}
	pattern, registered := core.Recipe(id)
	if !registered {
		return false, false
	}
	consumed, ok := core.ConsumeRecipe(authority.size, authority.slots, pattern)
	if !ok {
		return false, false
	}
	next, leftover := authority.inventory.AddStack(output)
	if leftover.Count != 0 {
		return false, false
	}
	authority.inventory = next
	authority.slots = consumed
	return true, true
}

// gridMirrorPlayerID 返回一个固定的 UUIDv4 测试身份。
func gridMirrorPlayerID(t *testing.T) core.PlayerID {
	t.Helper()
	id, err := core.ParsePlayerID("00112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
