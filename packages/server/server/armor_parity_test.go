package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// armorParitySampleCount 是减免仗采样的受击次数：三次受击足以让「每击恰好
// 减免到同一点数」与「四件耐久同频消耗」在两条传输上各出现三拍，又不至于
// 让击退把目标推出近战距离。
const armorParitySampleCount = 3

// armorParityTranscript 是装备互换与减免战斗脚本在一种传输下的全部可观察
// 投影：拒绝、穿戴后的背包与权威装备区、线上点数与受击生命序列。全部字段
// 都不含绝对 tick，Memory 与 TCP 的录像可直接逐位比对。
type armorParityTranscript struct {
	Rejection   network.CommandRejected
	Inventory   core.Inventory
	Armor       [core.ArmorSlotCount]core.ItemStack
	ArmorPoints uint8
	Healths     []uint8
}

// armorParityFullWorn 构造一套四槽全穿的完好铁甲夹具：点数与耐久一律从 core
// 护甲域现读，测试不复制任何表值。
func armorParityFullWorn() [core.ArmorSlotCount]core.ItemStack {
	var worn [core.ArmorSlotCount]core.ItemStack
	pieces := [core.ArmorSlotCount]core.ItemID{
		core.ArmorSlotHead:  core.ItemIronHelmet,
		core.ArmorSlotChest: core.ItemIronChestplate,
		core.ArmorSlotLegs:  core.ItemIronLeggings,
		core.ArmorSlotFeet:  core.ItemIronBoots,
	}
	for slot, item := range pieces {
		full, ok := core.ItemMaxDurability(item)
		if !ok {
			panic("parity fixture: 护甲件缺少耐久上限登记")
		}
		worn[slot] = core.ItemStack{Item: item, Count: 1, Durability: full}
	}
	return worn
}

// TestArmorEquipMemoryTCPParity 覆盖装备命令的服务端接线契约：同一条脚本在
// Memory 与 TCP 两种传输下必须得到逐位相同的投影。
//
// 脚本刻意同时走三条路：非护甲手持的装备命令被 `not_armor` 拒绝且零状态变
// 更；四件铁甲经权威选中格互换全部穿上、线上点数随之反映；随后一次按住近战
// 的减免仗把受击伤害压到公式值并同频消耗四件耐久。只跑成功路的话，拒绝映射
// 或耐久接线坏了照样两传输一致。
func TestArmorEquipMemoryTCPParity(t *testing.T) {
	memory := runArmorParityScript(t, "memory")
	tcp := runArmorParityScript(t, "tcp")
	if !reflect.DeepEqual(memory, tcp) {
		t.Fatalf("装备 Memory/TCP transcript 不一致\nmemory=%+v\ntcp=%+v", memory, tcp)
	}

	wantRejection := network.CommandRejected{Sequence: 1, Reason: network.RejectNotArmor}
	if memory.Rejection != wantRejection {
		t.Fatalf("非护甲手持的拒绝 = %+v，想要 %+v", memory.Rejection, wantRejection)
	}

	worn := armorParityFullWorn()
	// 减免仗之后权威装备区应仍是原四件，唯耐久各恰好消耗采样次数。
	wantWorn := worn
	for slot := range wantWorn {
		wantWorn[slot].Durability -= armorParitySampleCount
	}
	if got := memory.Armor; got != wantWorn {
		t.Fatalf("权威装备区 = %+v，想要耐久各耗 %d 的 %v", got, armorParitySampleCount, wantWorn)
	}
	wantPoints := core.ArmorPoints(worn)
	if memory.ArmorPoints != wantPoints {
		t.Fatalf("线上护甲点数 = %d，想要 %d", memory.ArmorPoints, wantPoints)
	}
	for slot := range 4 {
		if got := memory.Inventory.Hotbar.Slots[slot]; got != (core.ItemStack{}) {
			t.Fatalf("穿戴后快捷栏 %d 格 = %+v，想要已换空", slot, got)
		}
	}
	// 每次产生减免的受击四件完好护甲各恰好消耗 1 点耐久；点数与有效伤害
	// 都按消耗前的完好状态计算，因此三拍生命序列逐位等于公式值。
	wantEffective := core.ReducedDamage(core.WeaponDamage(core.ItemWoodenSword), wantPoints)
	last := core.MaxHealth
	for index, health := range memory.Healths {
		if got := int32(last - health); got != wantEffective {
			t.Fatalf("第 %d 次受击扣血 %d，想要公式值 %d（序列 %v）", index, got, wantEffective, memory.Healths)
		}
		last = health
	}
	if len(memory.Healths) != armorParitySampleCount {
		t.Fatalf("受击采样 %d 次，想要 %d（夹具失效）", len(memory.Healths), armorParitySampleCount)
	}
}

// armorParityRecorder 是一个会话在逐 tick 推进中的消息收集器：排干本 tick 的
// 消息流，顺手镜像区块并保留最新的拒绝、背包与本人权威状态。
type armorParityRecorder struct {
	endpoint  network.ClientEndpoint
	mirror    *client.Mirror
	rejection network.CommandRejected
	inventory core.Inventory
	hasInv    bool
	state     network.PlayerState
	hasState  bool
}

func (recorder *armorParityRecorder) drainToTick(t *testing.T, tick uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	for {
		message, err := recorder.endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("armor parity tick %d Recv: %v", tick, err)
		}
		recorder.applyToMirror(t, message)
		switch message := message.(type) {
		case network.PlayerState:
			if message.ServerTick != tick {
				continue
			}
			recorder.state = message
			recorder.hasState = true
			return
		case network.CommandRejected:
			recorder.rejection = message
		case network.InventoryState:
			recorder.inventory = message.Inventory
			recorder.hasInv = true
		}
	}
}

// applyToMirror 只把区块承载消息喂给镜像：拒绝原因是否被客户端镜像认领属
// 客户端接线范围，本测试的拒绝断言直接落在收集到的 wire 消息上。
func (recorder *armorParityRecorder) applyToMirror(t *testing.T, message network.ServerMessage) {
	t.Helper()
	switch message.(type) {
	case network.ChunkSnapshot, network.BlockChanges, network.ForgetChunks:
		if _, err := recorder.mirror.Apply(message); err != nil {
			t.Fatalf("Mirror.Apply(%T): %v", message, err)
		}
	}
}

func runArmorParityScript(t *testing.T, transport string) armorParityTranscript {
	t.Helper()
	striker := integrationIdentity(0xa1, "ArmorStriker")
	wearer := integrationIdentity(0xa2, "ArmorWearer")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 6, Seed: 42, SpawnDimension: core.Overworld,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	worn := armorParityFullWorn()
	var strikerInv core.Inventory
	strikerInv.Hotbar.Selected = 0
	strikerFull, ok := core.ItemMaxDurability(core.ItemWoodenSword)
	if !ok {
		t.Fatal("夹具失效：木剑缺少耐久上限登记")
	}
	strikerInv.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: strikerFull}
	var wearerInv core.Inventory
	for slot := range 4 {
		wearerInv.Hotbar.Slots[slot] = worn[slot]
	}
	seedArmorParityPlayer(t, store, striker, strikerInv, [3]float32{0.5, 1.001, 4.5})
	seedArmorParityPlayer(t, store, wearer, wearerInv, [3]float32{0.5, 1.001, 2.5})

	config := hostTestConfig()
	config.MaxPlayers = 2
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, flatGenerator{}, store)
	strikerRecorder := &armorParityRecorder{mirror: client.NewMirror()}
	wearerRecorder := &armorParityRecorder{mirror: client.NewMirror()}
	var closeStriker, closeWearer func()
	strikerRecorder.endpoint, _, closeStriker = openParityTransport(t, host, transport, striker)
	wearerRecorder.endpoint, _, closeWearer = openParityTransport(t, host, transport, wearer)
	t.Cleanup(func() {
		_ = strikerRecorder.endpoint.Close()
		_ = wearerRecorder.endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeStriker()
		closeWearer()
	})
	recorders := []*armorParityRecorder{strikerRecorder, wearerRecorder}

	tick := func() uint64 {
		t.Helper()
		result := host.world.StepForTest()
		for _, recorder := range recorders {
			recorder.drainToTick(t, result.Tick)
		}
		return result.Tick
	}
	// 单命令单 tick 纪律：上一条命令已入队并随本 tick 结算完毕，再发下一条，
	// 让等待「入队非空」的判断永远指向刚发送的那条命令。
	send := func(recorder *armorParityRecorder, command network.ClientMessage) {
		t.Helper()
		sendIntegration(t, recorder.endpoint, command)
		waitIntegrationCondition(t, "armor parity command queued", func() bool {
			return len(host.world.incoming) > 0
		})
		tick()
	}

	ready := func() bool {
		for _, recorder := range recorders {
			if !recorder.hasState || !recorder.state.Ready {
				return false
			}
			if !recorder.hasInv {
				return false
			}
			if !parityViewLoaded(recorder.mirror) {
				return false
			}
		}
		return true
	}
	waitIntegrationLoginReady(t, transport+" armor parity", ready,
		func() string {
			return "striker=" + armorParityRecorderState(strikerRecorder) +
				" wearer=" + armorParityRecorderState(wearerRecorder)
		},
		func() { tick() },
	)

	// 非护甲手持：木剑格发装备命令必须被 not_armor 拒绝且零状态变更。
	send(strikerRecorder, network.EquipArmor{Sequence: 1})

	// 四件全穿：每次先选中持有护甲的快捷栏格，再发装备命令经权威互换上台。
	sequence := uint64(10)
	for slot := range 4 {
		sequence++
		send(wearerRecorder, network.SelectHotbar{Sequence: sequence, Slot: uint8(slot)})
		sequence++
		send(wearerRecorder, network.EquipArmor{Sequence: sequence})
	}
	if !wearerRecorder.hasInv || wearerRecorder.inventory.Hotbar.Slots[0] != (core.ItemStack{}) {
		t.Fatalf("%s 穿戴未生效：背包 %+v", transport, wearerRecorder.inventory)
	}
	if !wearerRecorder.hasState || wearerRecorder.state.ArmorPoints != core.ArmorPoints(worn) {
		t.Fatalf("%s 穿戴后线上点数未反映：state=%+v", transport, wearerRecorder.state)
	}

	// 减免仗：攻击者按住近战，采到固定次数的受击生命迁移为止。
	send(strikerRecorder, network.PlayerInput{Sequence: 2, Yaw: 0, Pitch: 0, Mining: true})
	healths := make([]uint8, 0, armorParitySampleCount)
	last := core.MaxHealth
	for range 80 {
		tick()
		if !wearerRecorder.hasState {
			continue
		}
		if health := wearerRecorder.state.Health; health < last {
			healths = append(healths, health)
			last = health
			if len(healths) == armorParitySampleCount {
				break
			}
		}
	}

	host.mu.Lock()
	active := *host.activeByPlayer[wearer.PlayerID]
	host.mu.Unlock()
	snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
	if !ok {
		t.Fatalf("%s 没有权威玩家快照", transport)
	}
	return armorParityTranscript{
		Rejection:   strikerRecorder.rejection,
		Inventory:   snapshot.Inventory,
		Armor:       snapshot.Armor,
		ArmorPoints: wearerRecorder.state.ArmorPoints,
		Healths:     healths,
	}
}

func seedArmorParityPlayer(
	t *testing.T,
	store *storage.MemoryStore,
	identity network.Identity,
	inventory core.Inventory,
	position [3]float32,
) {
	t.Helper()
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: position}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location, Inventory: inventory,
	})); err != nil {
		t.Fatal(err)
	}
}

func armorParityRecorderState(recorder *armorParityRecorder) string {
	if !recorder.hasState {
		return "no-state"
	}
	return fmt.Sprintf("%+v", recorder.state)
}
