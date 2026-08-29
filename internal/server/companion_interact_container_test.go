// mine 步骤满格饱和分支对容器目标（箱子/熔炉）的批量容量判定测试（change
// companion-mine-containers Task 3）：Runner 与 sim 用同一 `runtime.CompanionMineContainerStaging`
// 批量预演——内容物可容纳时任务正常完成（不误报容量失败，产物入伙伴背包）；
// 单件本体能放下而批量放不下时以既有 TaskFailInventoryFull 稳定失败且方块与
// 容器内容物不变；Memory 与 TCP 两条传输的 transcript 与世界结果完全一致。
// 全部使用 httptest 假模型，绝不打开前台窗口或访问真实模型服务。
package server

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/world"
)

// setInteractionChest 在目标格放置箱子方块并激活对应的区块箱子槽（装入指定
// 内容物），返回写入的完整槽值（供前后比对）。generation 固定取 5：完成结算
// 停用槽后应恰好读回 `world.ChestSlot{Generation: 5}`。
func setInteractionChest(
	t *testing.T,
	host *Host,
	target core.BlockPos,
	items [core.ChestSlots]core.ItemStack,
) world.ChestSlot {
	t.Helper()
	setInteractionBlock(t, host, target, core.ChestID)
	index, ok := world.ChunkBlockIndex(target)
	if !ok {
		t.Fatalf("箱子目标 %+v 没有区块索引", target)
	}
	slot := world.ChestSlot{Generation: 5, Active: true, BlockIndex: index, Items: items}
	host.world.engine.SetChunkChestForTest(
		core.ChunkKey{Dimension: core.Overworld, Pos: target.Chunk()}, 0, slot)
	return slot
}

// setInteractionFurnace 在目标格放置熔炉方块并激活对应的区块熔炉槽（输入/
// 燃料/输出三格装入指定内容物），返回写入的完整槽值（供前后比对）。
func setInteractionFurnace(
	t *testing.T,
	host *Host,
	target core.BlockPos,
	input, fuel, output core.ItemStack,
) world.FurnaceSlot {
	t.Helper()
	setInteractionBlock(t, host, target, core.FurnaceID)
	index, ok := world.ChunkBlockIndex(target)
	if !ok {
		t.Fatalf("熔炉目标 %+v 没有区块索引", target)
	}
	slot := world.FurnaceSlot{
		Generation: 7, Active: true, BlockIndex: index,
		Input: input, Fuel: fuel, Output: output,
	}
	host.world.engine.SetChunkFurnaceForTest(
		core.ChunkKey{Dimension: core.Overworld, Pos: target.Chunk()}, 0, slot)
	return slot
}

// interactionChestAt 读取目标格所在区块箱子槽 0 的当前权威值（tick 边界只读
// 拷贝）。本文件的构造固定写入槽 0，读取也固定按槽位直读——完成结算停用槽后
// `ChestAt` 不再报告活动槽，直读才能断言「停用且保留 generation」的终值。
func interactionChestAt(t *testing.T, host *Host, target core.BlockPos) world.ChestSlot {
	t.Helper()
	chunk, _, ready := host.world.engine.CloneReadyChunk(core.ChunkKey{
		Dimension: core.Overworld,
		Pos:       target.Chunk(),
	})
	if !ready {
		t.Fatalf("目标 %v 所在区块未 ready", target)
	}
	return chunk.Chest(0)
}

// interactionFurnaceAt 读取目标格所在区块熔炉槽 0 的当前权威值（tick 边界只读
// 拷贝）；直读而非按活动槽查找的理由见 `interactionChestAt`。
func interactionFurnaceAt(t *testing.T, host *Host, target core.BlockPos) world.FurnaceSlot {
	t.Helper()
	chunk, _, ready := host.world.engine.CloneReadyChunk(core.ChunkKey{
		Dimension: core.Overworld,
		Pos:       target.Chunk(),
	})
	if !ready {
		t.Fatalf("目标 %v 所在区块未 ready", target)
	}
	return chunk.Furnace(0)
}

// containerTightInventory 构造「手握完好石镐且全背包只剩快捷栏栏位 1 一个空格」
// 的背包：单件箱子本体能放进该空格（旧单件预演的放行形态），而「本体 + 两堆
// 互异内容物」的批量必然放不下——批量判定与单件判定分岔的确定性构造。
func containerTightInventory() core.Inventory {
	inventory := fullStoneInventory()
	durability, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	inventory.Hotbar.Slots[0] = core.ItemStack{
		Item: core.ItemStonePickaxe, Count: 1, Durability: durability,
	}
	inventory.Hotbar.Slots[1] = core.ItemStack{}
	return inventory
}

// containerTightStoneCount 是 containerTightInventory 的石料总数：36 格中快捷栏
// 栏位 0 换石镐、栏位 1 留空，其余 34 格各 64 件。
const containerTightStoneCount = 34 * core.MaxStackCount

// TestCompanionManagerMineContainerCompletesWithLoot 验证容器采掘的正常完成：
// 箱子内容物可容纳时完成 tick 直接批量结算，Runner 不误报容量失败——任务以
// Accepted→TaskStarted→TaskCompleted 完成，方块变空气、箱子槽停用（保留
// generation）、本体与全部内容物入包、石镐耐久扣减一件。
func TestCompanionManagerMineContainerCompletesWithLoot(t *testing.T) {
	id := chatTestCompanionID(1)
	model := newFakeCompanionModel(t)
	host, client := newInteractionHost(t, id, model, pickaxeInventory())
	target := core.BlockPos{X: 8, Y: 1, Z: 4}
	// 槽位刻意稀疏（0/2/5）：锁定空槽跳过与「本体在前、槽位序」的产物展开。
	var items [core.ChestSlots]core.ItemStack
	items[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	items[2] = core.ItemStack{Item: core.ItemGlass, Count: 2}
	items[5] = core.ItemStack{Item: core.ItemOakPlanks, Count: 7}
	setInteractionChest(t, host, target, items)
	model.setPlanScript(minePlanJSON(target))

	events := collectInteractionEvents(t, host, client, "@阿木 挖那口箱子", 900,
		func(events []network.ChatEvent) bool {
			return len(eventsWithKind(events, network.ChatEventTaskCompleted)) > 0 ||
				len(eventsWithKind(events, network.ChatEventTaskFailed)) > 0
		})
	taskEvents := interactionEventsOf(events, "挖那口箱子")
	assertInteractionEventSequence(t, taskEvents,
		network.ChatEventAccepted, network.ChatEventTaskStarted, network.ChatEventTaskCompleted)

	if got := interactionBlockAt(t, host, target); got != core.AirID {
		t.Fatalf("完成后目标方块=%d，想要空气", got)
	}
	if got := interactionChestAt(t, host, target); got != (world.ChestSlot{Generation: 5}) {
		t.Fatalf("完成后箱子槽=%+v，想要停用且保留 generation 5", got)
	}
	body := currentCompanionBody(t, host, id)
	if count := interactionInventoryCount(body.Inventory, core.ItemChest); count != 1 {
		t.Fatalf("箱子本体=%d，想要 1（背包=%+v）", count, body.Inventory)
	}
	if count := interactionInventoryCount(body.Inventory, core.ItemStone); count != 3 {
		t.Fatalf("内容物石头=%d，想要 3", count)
	}
	if count := interactionInventoryCount(body.Inventory, core.ItemGlass); count != 2 {
		t.Fatalf("内容物玻璃=%d，想要 2", count)
	}
	if count := interactionInventoryCount(body.Inventory, core.ItemOakPlanks); count != 7 {
		t.Fatalf("内容物木板=%d，想要 7", count)
	}
	pickaxe := body.Inventory.Hotbar.Slots[0]
	if pickaxe.Item != core.ItemStonePickaxe || pickaxe.Durability != 130 {
		t.Fatalf("石镐=%+v，想要耐久 131→130", pickaxe)
	}
}

// TestCompanionManagerMineContainerBatchOverflowFailsInventoryFull 验证满格饱和
// 分支的批量容量判定：单件箱子本体能放进唯一空格（旧单件预演的形态），而
// 「本体 + 两堆互异内容物」的批量放不下——Runner 必须用与 sim 同一的批量预演
// 观察到该形态并以 TaskFailInventoryFull 稳定失败；方块、箱子内容物、背包与
// 耐久全部不变（全或无：sim 侧本就未结算）。
func TestCompanionManagerMineContainerBatchOverflowFailsInventoryFull(t *testing.T) {
	id := chatTestCompanionID(1)
	model := newFakeCompanionModel(t)
	host, client := newInteractionHost(t, id, model, containerTightInventory())
	target := core.BlockPos{X: 8, Y: 1, Z: 4}
	var items [core.ChestSlots]core.ItemStack
	items[0] = core.ItemStack{Item: core.ItemGlass, Count: 2}
	items[1] = core.ItemStack{Item: core.ItemOakPlanks, Count: 3}
	chest := setInteractionChest(t, host, target, items)
	model.setPlanScript(minePlanJSON(target))

	events := collectInteractionEvents(t, host, client, "@阿木 挖那口箱子", 900,
		func(events []network.ChatEvent) bool {
			return len(eventsWithKind(events, network.ChatEventTaskFailed)) > 0 ||
				len(eventsWithKind(events, network.ChatEventTaskCompleted)) > 0
		})
	taskEvents := interactionEventsOf(events, "挖那口箱子")
	assertInteractionEventSequence(t, taskEvents,
		network.ChatEventAccepted, network.ChatEventTaskStarted, network.ChatEventTaskFailed)
	if reason := interactionFailureReason(t, taskEvents); reason != network.TaskFailInventoryFull {
		t.Fatalf("失败原因=%d，想要 TaskFailInventoryFull", reason)
	}
	if got := interactionBlockAt(t, host, target); got != core.ChestID {
		t.Fatalf("失败后目标方块=%d，想要保持箱子", got)
	}
	if got := interactionChestAt(t, host, target); got != chest || !got.Active {
		t.Fatalf("失败后箱子槽被改动: %+v，想要原样保留 %+v", got, chest)
	}
	body := currentCompanionBody(t, host, id)
	if count := interactionInventoryCount(body.Inventory, core.ItemStone); count != containerTightStoneCount {
		t.Fatalf("背包石料=%d，想要保持 %d（背包 MUST 不变）", count, containerTightStoneCount)
	}
	if count := interactionInventoryCount(body.Inventory, core.ItemChest); count != 0 {
		t.Fatalf("背包出现箱子 %d 件，想要 0", count)
	}
	if count := interactionInventoryCount(body.Inventory, core.ItemGlass); count != 0 {
		t.Fatalf("背包出现玻璃 %d 件，想要 0", count)
	}
	if count := interactionInventoryCount(body.Inventory, core.ItemOakPlanks); count != 0 {
		t.Fatalf("背包出现木板 %d 件，想要 0", count)
	}
	pickaxe := body.Inventory.Hotbar.Slots[0]
	if pickaxe.Item != core.ItemStonePickaxe || pickaxe.Durability != 131 {
		t.Fatalf("石镐=%+v，想要耐久保持 131（未结算不扣耐久）", pickaxe)
	}
}

// containerMineParityResult 是一次传输运行收集的全部可比事实：事件 transcript、
// 伙伴最终背包与位置、两个容器目标的世界结果（方块值与容器槽）与假模型请求
// 计数（指令与规划请求的一对一事实）。
type containerMineParityResult struct {
	Transcript     []network.ChatEvent
	FinalInventory core.Inventory
	FinalPosition  [3]float32
	ChestBlock     core.BlockID
	ChestSlot      world.ChestSlot
	FurnaceTarget  core.BlockPos
	FurnaceBlock   core.BlockID
	FurnaceSlot    world.FurnaceSlot
	ModelRequests  int
}

// runContainerMineParity 在指定传输上执行容器采掘容量脚本并返回全部可比事实：
// 指令一「挖空箱子」的批量（仅本体 1 堆）放得下、必须完成回收；指令二「挖
// 装着燃料与产物的熔炉」的批量（本体 + 燃料 + 产物 3 堆互异）放不下，必须以
// TaskFailInventoryFull 终结（方块与内容物原样保留）。失败任务排在脚本末尾：
// 走近目标途中伙伴可能借碰撞爬上方块顶沿，其站立格因此没有寻路支撑，任何
// 后续移动任务都会以 PathUnreachable 失败——这是与传输无关的既有几何事实，
// 把它留在脚本外，受测窗口内只剩容器容量语义。指令发送严格串行（先等进入
// 服务端 ingress 队列再发下一条）。
func runContainerMineParity(t *testing.T, transport string) containerMineParityResult {
	t.Helper()
	id := chatTestCompanionID(1)
	model := newFakeCompanionModel(t)
	host := newInteractionParityHost(t, id, model, containerTightInventory())
	// 台词平面在本 parity 场景保持静默：为 dialogue 客户端接入持续 5xx 的独立
	// 假台词模型，成功台词的 CompanionSpeech 事件到达 tick 取决于 HTTP 时序，
	// 会破坏 transcript 的跨传输可比性（沿用 interaction parity 的先例）。
	silentDialogue := newFakeDialogueModel(t)
	silentDialogue.setStatus(500)
	host.world.companionManager.replaceDialogueForTest(t, silentDialogue)
	client := openCompanionChatClient(t, host, transport, integrationIdentity(0xc1, "指挥者"))
	clients := []network.ClientEndpoint{client}
	body := stepUntilCompanionManagerReady(t, host, clients, id)

	// 空箱子在出生点 -X 方向两格（批量仅本体，放进唯一空格恰好成立）；熔炉在
	// +X 方向四格（燃料 3 煤 + 产物 1 铁锭、输入空——输入空使熔炉完全暂停，
	// 内容物跨 tick 不变；本体 + 两堆互异内容物放不进唯一空格）——两个目标
	// 都在伙伴出生的中心区块内。
	baseX, baseZ := int32(body.Position[0]), int32(body.Position[2])
	chestTarget := core.BlockPos{X: baseX - 2, Y: 1, Z: baseZ}
	furnaceTarget := core.BlockPos{X: baseX + 4, Y: 1, Z: baseZ}
	setInteractionChest(t, host, chestTarget, [core.ChestSlots]core.ItemStack{})
	setInteractionFurnace(t, host, furnaceTarget,
		core.ItemStack{},
		core.ItemStack{Item: core.ItemCoal, Count: 3},
		core.ItemStack{Item: core.ItemIronIngot, Count: 1})
	model.setPlanScript(minePlanJSON(chestTarget), minePlanJSON(furnaceTarget))

	var transcript []network.ChatEvent
	// stepParityTick 推进一个权威 tick 并排空本 tick 的全部客户端消息（保持
	// 流同步），把 ChatEvent 追加进 transcript。
	stepParityTick := func() contract.TickResult {
		tickResult := host.world.StepForTest()
		transcript = append(transcript,
			companionChatEvents(receiveCompanionChatTick(t, client, tickResult.Tick))...)
		return tickResult
	}
	// stepUntilTerminal 推进至匹配事件命中或步数耗尽；先排空本 tick 全部事件
	// 再判定，同一批次里的其余事件绝不丢失。
	stepUntilTerminal := func(maxTicks int, kind network.ChatEventKind, command string) bool {
		for range maxTicks {
			tickResult := host.world.StepForTest()
			hit := false
			for _, event := range companionChatEvents(receiveCompanionChatTick(t, client, tickResult.Tick)) {
				transcript = append(transcript, event)
				if event.Kind == kind && event.Command == command {
					hit = true
				}
			}
			if hit {
				return true
			}
		}
		return false
	}
	// 预热寻路窗口（与传输无关的异步区块就绪事实移出受测窗口，两传输的位移
	// 才能从同一事件锚点起步）。
	warmInteractionParityPathWindow(t, host, id, stepParityTick)

	sendIntegration(t, client, network.ChatCommand{Text: "@阿木 挖那口箱子"})
	waitForIncomingChatDepth(t, host.world, 1)
	if !stepUntilTerminal(900, network.ChatEventTaskCompleted, "挖那口箱子") {
		t.Fatalf("箱子采掘任务未完成（事件=%v）", chatEventKinds(transcript))
	}
	sendIntegration(t, client, network.ChatCommand{Text: "@阿木 挖那座熔炉"})
	waitForIncomingChatDepth(t, host.world, 1)
	if !stepUntilTerminal(900, network.ChatEventTaskFailed, "挖那座熔炉") {
		t.Fatalf("熔炉采掘任务未以 TaskFailed 终结（事件=%v）", chatEventKinds(transcript))
	}
	for range 3 {
		stepParityTick()
	}

	final := currentCompanionBody(t, host, id)
	result := containerMineParityResult{
		Transcript:     transcript,
		FinalInventory: final.Inventory,
		FinalPosition:  final.Position,
		ChestBlock:     interactionBlockAt(t, host, chestTarget),
		ChestSlot:      interactionChestAt(t, host, chestTarget),
		FurnaceTarget:  furnaceTarget,
		FurnaceBlock:   interactionBlockAt(t, host, furnaceTarget),
		FurnaceSlot:    interactionFurnaceAt(t, host, furnaceTarget),
	}
	result.ModelRequests, _, _, _ = model.snapshotCounts()
	assertStrictlyIncreasingEventIDs(t, transcript)
	return result
}

// TestCompanionManagerContainerMineMemoryTCPParity 验证容器采掘容量判定的
// Memory/TCP 传输一致性：同一脚本（批量放得下的空箱子 → 完成回收入包；随后
// 批量放不下的装料熔炉 → TaskFailInventoryFull 且方块连同内容物原样保留）在
// 两条传输上产出逐字节一致的 ChatEvent transcript 与完全一致的世界结果。
func TestCompanionManagerContainerMineMemoryTCPParity(t *testing.T) {
	results := make(map[string]containerMineParityResult, 2)
	for _, transport := range []string{"memory", "tcp"} {
		transport := transport
		t.Run(transport, func(t *testing.T) {
			results[transport] = runContainerMineParity(t, transport)
		})
	}
	memory, tcpResult := results["memory"], results["tcp"]

	if !reflect.DeepEqual(memory.Transcript, tcpResult.Transcript) {
		t.Fatalf("Memory/TCP 容器采掘 transcript 不一致\nMemory=%+v\nTCP=%+v",
			memory.Transcript, tcpResult.Transcript)
	}
	if !reflect.DeepEqual(memory.FinalInventory, tcpResult.FinalInventory) {
		t.Fatalf("Memory/TCP 伙伴背包不一致\nMemory=%+v\nTCP=%+v",
			memory.FinalInventory, tcpResult.FinalInventory)
	}
	if !reflect.DeepEqual(memory.ChestSlot, tcpResult.ChestSlot) ||
		memory.ChestBlock != tcpResult.ChestBlock {
		t.Fatalf("Memory/TCP 箱子世界结果不一致 memory=(%d,%+v) tcp=(%d,%+v)",
			memory.ChestBlock, memory.ChestSlot, tcpResult.ChestBlock, tcpResult.ChestSlot)
	}
	if !reflect.DeepEqual(memory.FurnaceSlot, tcpResult.FurnaceSlot) ||
		memory.FurnaceBlock != tcpResult.FurnaceBlock {
		t.Fatalf("Memory/TCP 熔炉世界结果不一致 memory=(%d,%+v) tcp=(%d,%+v)",
			memory.FurnaceBlock, memory.FurnaceSlot, tcpResult.FurnaceBlock, tcpResult.FurnaceSlot)
	}
	assertInteractionParityPosition(t, "最终", memory.FinalPosition, tcpResult.FinalPosition)
	if memory.ModelRequests != tcpResult.ModelRequests || memory.ModelRequests != 2 {
		t.Fatalf("模型请求 memory=%d tcp=%d，想要两传输各恰好 2 次",
			memory.ModelRequests, tcpResult.ModelRequests)
	}

	// 单传输锁定（两传输已证一致，断言其一即覆盖两者）：事件序列恰为成功后
	// 失败的六事件链，失败原因是 TaskFailInventoryFull。
	kinds := chatEventKinds(memory.Transcript)
	want := []network.ChatEventKind{
		network.ChatEventAccepted, network.ChatEventTaskStarted, network.ChatEventTaskCompleted,
		network.ChatEventAccepted, network.ChatEventTaskStarted, network.ChatEventTaskFailed,
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("事件序列=%v，想要 %v", kinds, want)
	}
	if reason := interactionFailureReason(t, memory.Transcript); reason != network.TaskFailInventoryFull {
		t.Fatalf("失败原因=%d，想要 TaskFailInventoryFull", reason)
	}
	// 世界结果：空箱子被回收（方块空气、槽停用保留 generation 5、本体入包）；
	// 熔炉连同内容物原样保留（槽仍活动、输入/燃料/产物逐字段不变）；石镐
	// 耐久只有箱子结算扣一件，熔炉未结算不扣。
	if memory.ChestBlock != core.AirID || memory.ChestSlot != (world.ChestSlot{Generation: 5}) {
		t.Fatalf("完成后箱子=%d 槽=%+v，想要空气与停用槽", memory.ChestBlock, memory.ChestSlot)
	}
	furnaceIndex, indexOK := world.ChunkBlockIndex(memory.FurnaceTarget)
	if !indexOK {
		t.Fatalf("熔炉目标 %+v 没有区块索引", memory.FurnaceTarget)
	}
	wantFurnace := world.FurnaceSlot{
		Generation: 7, Active: true, BlockIndex: furnaceIndex,
		Fuel:   core.ItemStack{Item: core.ItemCoal, Count: 3},
		Output: core.ItemStack{Item: core.ItemIronIngot, Count: 1},
	}
	if memory.FurnaceBlock != core.FurnaceID || memory.FurnaceSlot != wantFurnace {
		t.Fatalf("失败后熔炉=%d 槽=%+v，想要原样保留 %+v",
			memory.FurnaceBlock, memory.FurnaceSlot, wantFurnace)
	}
	if count := interactionInventoryCount(memory.FinalInventory, core.ItemChest); count != 1 {
		t.Fatalf("箱子本体=%d，想要 1（背包=%+v）", count, memory.FinalInventory)
	}
	if count := interactionInventoryCount(memory.FinalInventory, core.ItemCoal); count != 0 {
		t.Fatalf("背包出现熔炉内容物煤 %d 件，想要 0", count)
	}
	if count := interactionInventoryCount(memory.FinalInventory, core.ItemIronIngot); count != 0 {
		t.Fatalf("背包出现熔炉内容物铁锭 %d 件，想要 0", count)
	}
	if count := interactionInventoryCount(memory.FinalInventory, core.ItemStone); count != containerTightStoneCount {
		t.Fatalf("背包石料=%d，想要保持 %d", count, containerTightStoneCount)
	}
	pickaxe := memory.FinalInventory.Hotbar.Slots[0]
	if pickaxe.Item != core.ItemStonePickaxe || pickaxe.Durability != 130 {
		t.Fatalf("石镐=%+v，想要耐久 131→130（箱子结算扣一件、熔炉未结算不扣）", pickaxe)
	}
}
