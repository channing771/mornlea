package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// plantingParityResult 是种植脚本跑完后的全部可观察结果：权威背包、客户端
// 镜像里的落脚格与作物格，以及「未翻地就种」被拒的完整拒绝消息。
type plantingParityResult struct {
	Inventory core.Inventory
	Ground    core.BlockID
	Crop      core.BlockID
	Rejection network.CommandRejected
}

// plantingParitySeeds 是脚本开局持有的种子数。它必须 ≥ 2：夹具若只放 1 颗，
// 「被拒时一颗不掉」与「被拒时扣光了」在读数上无法区分。
const plantingParitySeeds = uint8(4)

// TestPlantSeedsMemoryTCPParity 覆盖任务 5.3 的传输一致性要求：同一条种植
// 脚本在 Memory 与 TCP 两种传输下必须得到逐字段相同的结果。
//
// 脚本刻意同时走拒绝与成功两条路：先在**没翻过的草地**上种一次（必须被拒且
// 一颗种子都不掉），再翻地、再种一次（必须成功且恰好扣一颗）。只跑成功那一半
// 的话，一个把种子种到任何地面上的实现照样能让两种传输一致。
//
// 锄头**由脚本自己合成**而不是塞进初始背包：`authoritative-crafting` 主规格
// 要求「新增配方 …… 相同初始状态与命令序列经 Memory 和 TCP MUST 得到相同
// 结果」，而 recipe 9/10 此前只有 core 层的原子性单测，没有任何跨传输证据。
// 让本脚本以 recipe 9 起手，这条限定词就随种植一起被两种传输各跑一遍。
func TestPlantSeedsMemoryTCPParity(t *testing.T) {
	memory := runPlantingParityScript(t, "memory")
	tcp := runPlantingParityScript(t, "tcp")
	if memory != tcp {
		t.Fatalf("种植 Memory/TCP 未收敛\nmemory=%+v\ntcp=%+v", memory, tcp)
	}

	wantSeeds := core.ItemStack{Item: core.ItemWheatSeeds, Count: plantingParitySeeds - 1}
	if got := memory.Inventory.Hotbar.Slots[1]; got != wantSeeds {
		t.Fatalf("两次种植后的种子 = %+v，想要恰好扣一颗的 %+v（第一次被拒不得扣料）",
			got, wantSeeds)
	}
	if memory.Ground != core.FarmlandDryID {
		t.Fatalf("客户端镜像里的落脚格 = %d，想要干耕地 %d", memory.Ground, core.FarmlandDryID)
	}
	if memory.Crop != core.WheatStage0ID {
		t.Fatalf("客户端镜像里的作物格 = %d，想要第一阶段作物 %d",
			memory.Crop, core.WheatStage0ID)
	}
	full, _ := core.ItemMaxDurability(core.ItemStoneHoe)
	wantHoe := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: full - 1}
	if got := memory.Inventory.Hotbar.Slots[0]; got != wantHoe {
		t.Fatalf("合成并翻地一次后的选中格 = %+v，想要耐久恰好 −1 的石锄 %+v", got, wantHoe)
	}
	want := network.CommandRejected{Sequence: 6, Reason: network.RejectInvalidBlock}
	if memory.Rejection != want {
		t.Fatalf("未翻地就种的拒绝 = %+v，想要 %+v", memory.Rejection, want)
	}
}

func runPlantingParityScript(t *testing.T, transport string) plantingParityResult {
	t.Helper()
	identity := integrationIdentity(0x9d, "Planter")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
	})
	var initial core.Inventory
	// 石锄配方（recipe 9，2×2、镜像位关闭）需要石头纵列 + 木棍纵列共四个
	// 独立栈——整堆移动不能拆堆，同一物品的多格形状只能由多个独立栈摆放
	// （两次点击整堆语义下的真实操作形态）。木棍刻意也由夹具直给：本脚本
	// 验证的是种植行为在两种传输下一致，木棍的网格合成链由
	// transport_parity 的网格脚本与 farming_loop_e2e 覆盖。种子留在第 1 格
	// （放置命令自带栏位号，不需要选中），锄头产出去向见下面的断言。
	initial.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	initial.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStone, Count: 1}
	initial.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemStick, Count: 1}
	initial.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemStick, Count: 1}
	initial.Hotbar.Slots[1] = core.ItemStack{
		Item: core.ItemWheatSeeds, Count: plantingParitySeeds,
	}
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
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	t.Cleanup(func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	})

	mirror := client.NewMirror()
	var rejection network.CommandRejected
	step := func() {
		t.Helper()
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			if rejected, ok := message.(network.CommandRejected); ok {
				rejection = rejected
			}
		}
	}
	send := func(command network.ClientMessage) {
		t.Helper()
		sendIntegration(t, endpoint, command)
		waitIntegrationCondition(t, fmt.Sprintf("%s plant %T queued", transport, command), func() bool {
			return len(host.world.incoming) > 0
		})
		step()
	}

	ready, inventoryReady := false, false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s plant seeds", transport),
		func() bool { return ready && inventoryReady && parityViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v inventoryReady=%v viewLoaded=%v", ready, inventoryReady, parityViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
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

	// 合成石锄（网格驱动）：石头纵列摆进格 0/2、木棍纵列摆进格 1/3（个人
	// 2×2 网格，行主序），取出后四个源格全部搬空，满耐久石锄经 AddStack 落在
	// 最低的空快捷栏位——正是权威选中格 0（翻地只认选中格）。
	send(network.MoveCraftingStack{Sequence: 1, From: 9, To: 0})
	send(network.MoveCraftingStack{Sequence: 2, From: 11, To: 2})
	send(network.MoveCraftingStack{Sequence: 3, From: 12, To: 1})
	send(network.MoveCraftingStack{Sequence: 4, From: 13, To: 3})
	send(network.TakeCraftingOutput{Sequence: 5})
	for range 3 {
		step()
	}
	// 第一次种植：脚下还是草，必须被拒且一颗种子都不掉。
	send(network.PlaceBlock{Sequence: 6, Pitch: tillSoilLookDown, Slot: 1})
	for range 3 {
		step()
	}
	// 翻地：脚下那格草变成耕地。
	send(network.TillSoil{Sequence: 7, Pitch: tillSoilLookDown})
	for range 3 {
		step()
	}
	// 第二次种植：同一条命令，这次必须成功。
	send(network.PlaceBlock{Sequence: 8, Pitch: tillSoilLookDown, Slot: 1})
	for range 3 {
		step()
	}

	ground, groundLoaded := mirror.BlockAt(core.Overworld, core.BlockPos{})
	crop, cropLoaded := mirror.BlockAt(core.Overworld, core.BlockPos{Y: 1})
	if !groundLoaded || !cropLoaded {
		t.Fatalf("%s 目标格没有进入客户端镜像", transport)
	}
	host.mu.Lock()
	active := *host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
	if !ok {
		t.Fatalf("%s 没有权威玩家快照", transport)
	}
	return plantingParityResult{
		Inventory: snapshot.Inventory,
		Ground:    ground,
		Crop:      crop,
		Rejection: rejection,
	}
}
