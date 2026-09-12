package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestArmorSurvivesDiskRestart 覆盖装备跨重启保值：玩家经真实 ingress 上行
// 穿上部分护甲（含一件非满耐久的磨损件）、挨一记被减免的近战消耗耐久、正常
// 断开落盘、关服、用同一磁盘世界重开重连之后，四槽内容、逐件耐久与线上点数
// 必须与断线时的权威快照逐位一致。
//
// 耐久消耗走真实近战结算而非直接改档：这样「减免仗改了装备区」到「落盘」的
// 全链路（结算 → 快照 → 判脏 → 保存 → 迁移读取 → 恢复）都被这条测试压住，
// 任何一环漏带装备区都会让重启后的比对变红。
func TestArmorSurvivesDiskRestart(t *testing.T) {
	root := t.TempDir()
	wearer := integrationIdentity(0xa3, "ArmorWearer")
	striker := integrationIdentity(0xa4, "ArmorStriker")

	// 播种：穿戴者快捷栏 0 格放磨损铁头盔、1 格放完好铁胸甲（穿戴后合计
	// 8 点）；攻击者持完好木剑。位置沿用既有近战 parity 的几何：攻击者在
	// +z 侧、俯仰为零的正前射线恰好命中目标。
	var wearerInv core.Inventory
	wearerInv.Hotbar.Selected = 0
	helmetFull, ok := core.ItemMaxDurability(core.ItemIronHelmet)
	if !ok {
		t.Fatal("夹具失效：铁头盔缺少耐久上限登记")
	}
	wornHelmet := core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: helmetFull - 5}
	wearerInv.Hotbar.Slots[0] = wornHelmet
	chestFull, ok := core.ItemMaxDurability(core.ItemIronChestplate)
	if !ok {
		t.Fatal("夹具失效：铁胸甲缺少耐久上限登记")
	}
	wearerInv.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: chestFull}
	var strikerInv core.Inventory
	strikerInv.Hotbar.Selected = 0
	strikerFull, ok := core.ItemMaxDurability(core.ItemWoodenSword)
	if !ok {
		t.Fatal("夹具失效：木剑缺少耐久上限登记")
	}
	strikerInv.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: strikerFull}
	seedArmorRestartPlayer(t, root, wearer, wearerInv, [3]float32{0.5, 1.001, 2.5})
	seedArmorRestartPlayer(t, root, striker, strikerInv, [3]float32{0.5, 1.001, 4.5})

	first := startDiskHost(t, root, "127.0.0.1:0", flatGenerator{})
	wearerClient := dialArmorRestartClient(t, first, wearer)
	strikerClient := dialArmorRestartClient(t, first, striker)
	clients := []*armorRestartClient{wearerClient, strikerClient}
	// 双客户端异步推进下任何等待都在排干两条消息流：心跳应答只发生在客户端
	// 读取消息时，漏排会让服务端在心跳超时后拆掉还在用的会话。
	waitArmorRestart(t, "wearer ready", clients, func() bool {
		return wearerClient.ready()
	})
	waitArmorRestart(t, "striker ready", clients, func() bool {
		return strikerClient.ready()
	})

	// 穿戴：全部经网络 ingress 上行。`SelectHotbar` 结算在 interactions 阶段、
	// 晚于命令阶段的装备结算，因此每件护甲都先等权威选中格生效再发装备命令，
	// 不能同 tick 连发。
	sendArmorRestart(t, wearerClient, network.EquipArmor{Sequence: 1})
	waitArmorRestart(t, "helmet equipped on authority", clients, func() bool {
		snapshot, ok := first.PlayerSnapshotFor(t, wearer.PlayerID)
		return ok && snapshot.Armor[core.ArmorSlotHead] == wornHelmet &&
			snapshot.Inventory.Hotbar.Slots[0] == (core.ItemStack{})
	})
	sendArmorRestart(t, wearerClient, network.SelectHotbar{Sequence: 2, Slot: 1})
	waitArmorRestart(t, "hotbar selection applied", clients, func() bool {
		snapshot, ok := first.PlayerSnapshotFor(t, wearer.PlayerID)
		return ok && snapshot.Inventory.Hotbar.Selected == 1
	})
	sendArmorRestart(t, wearerClient, network.EquipArmor{Sequence: 3})
	wantWorn := [core.ArmorSlotCount]core.ItemStack{
		core.ArmorSlotHead:  wornHelmet,
		core.ArmorSlotChest: {Item: core.ItemIronChestplate, Count: 1, Durability: chestFull},
	}
	waitArmorRestart(t, "armor equipped on authority", clients, func() bool {
		snapshot, ok := first.PlayerSnapshotFor(t, wearer.PlayerID)
		return ok && snapshot.Armor == wantWorn &&
			snapshot.Inventory.Hotbar.Slots[0] == (core.ItemStack{}) &&
			snapshot.Inventory.Hotbar.Slots[1] == (core.ItemStack{})
	})

	// 减免仗：攻击者按住近战，权威耐久开始下降即拔线，避免多打。
	sendArmorRestart(t, strikerClient, network.PlayerInput{Sequence: 1, Yaw: 0, Pitch: 0, Mining: true})
	waitArmorRestart(t, "armor durability consumed by reduction", clients, func() bool {
		snapshot, ok := first.PlayerSnapshotFor(t, wearer.PlayerID)
		return ok && snapshot.Armor[core.ArmorSlotHead].Durability < wornHelmet.Durability
	})
	closeArmorRestartClient(t, strikerClient)
	// 等攻击者的两个会话索引都清空再取基准快照：索引清空先于其会话 worker
	// 退出，但引擎 detach（战斗参与者移除）已在收集退出快照时完成，不会有
	// 残留的最后一击打在基准与重启恢复之间。
	waitForPlayerReleased(t, first.Host, striker.PlayerID)

	// 记下断线前的权威装备区与背包，作为重启比对的基准。
	final := first.PlayerSnapshot(t, wearer.PlayerID)
	if core.ArmorPoints(final.Armor) == 0 {
		t.Fatalf("减免仗后点数为 0，夹具失效：%+v", final.Armor)
	}
	closeArmorRestartClient(t, wearerClient)
	first.WaitPlayerSaved(t, wearer.PlayerID)
	first.Shutdown(t)

	// 同一磁盘世界重开重连：四槽、耐久与背包逐位一致，线上点数与权威
	// 装备区现算值一致。
	second := startDiskHost(t, root, "127.0.0.1:0", flatGenerator{})
	reconnected := dialArmorRestartClient(t, second, wearer)
	waitArmorRestart(t, "reconnect ready", []*armorRestartClient{reconnected}, func() bool {
		return reconnected.ready()
	})
	wantPoints := core.ArmorPoints(final.Armor)
	waitArmorRestart(t, "armor points on wire", []*armorRestartClient{reconnected}, func() bool {
		return reconnected.hasState && reconnected.state.ArmorPoints != 0
	})
	if got := reconnected.state.ArmorPoints; got != wantPoints {
		t.Fatalf("重启后线上点数 = %d，想要权威现算值 %d", got, wantPoints)
	}
	restored := second.PlayerSnapshot(t, wearer.PlayerID)
	if restored.Armor != final.Armor {
		t.Fatalf("重启后装备区 = %+v，想要断线前 %v", restored.Armor, final.Armor)
	}
	if restored.Inventory != final.Inventory {
		t.Fatalf("重启后背包 = %+v，想要断线前 %v", restored.Inventory, final.Inventory)
	}

	closeArmorRestartClient(t, reconnected)
	second.Shutdown(t)
}

// armorRestartClient 是异步推进世界里一个客户端的泵：后台读取线程持续应答
// 心跳并把消息收进有界收件箱，等待循环再逐条取出。
type armorRestartClient struct {
	endpoint   network.ClientEndpoint
	receiver   *client.Receiver
	mirror     *client.Mirror
	state      network.PlayerState
	hasState   bool
	rejections []network.CommandRejected
}

func dialArmorRestartClient(t *testing.T, host integrationHost, identity network.Identity) *armorRestartClient {
	t.Helper()
	connected := dialIntegrationClient(t, host.Addr, identity)
	return &armorRestartClient{
		endpoint: connected.Endpoint,
		receiver: client.NewReceiver(connected.Endpoint, 4096),
		mirror:   connected.Mirror,
	}
}

// drain 取空收件箱：区块承载消息喂给镜像，本人权威状态记成最新一份。返回
// 读取线程遇到的错误（含服务端拆线），交由测试 goroutine 统一判失败。
func (c *armorRestartClient) drain() error {
	for {
		message, ok := c.receiver.TryRecv()
		if !ok {
			return c.receiver.Err()
		}
		switch message := message.(type) {
		case network.ChunkSnapshot, network.BlockChanges, network.ForgetChunks:
			if _, err := c.mirror.Apply(message); err != nil {
				return fmt.Errorf("Mirror.Apply(%T): %w", message, err)
			}
		case network.PlayerState:
			c.state = message
			c.hasState = true
		case network.CommandRejected:
			c.rejections = append(c.rejections, message)
		}
	}
}

func (c *armorRestartClient) ready() bool {
	return c.hasState && c.state.Ready
}

// waitArmorRestart 在排干全部客户端消息流的前提下轮询一个条件。条件闭包只
// 读测试侧与权威侧的内存状态，不做任何阻塞调用。
func waitArmorRestart(t *testing.T, label string, clients []*armorRestartClient, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(longWaitDeadline)
	defer deadline.Stop()
	for {
		for _, connected := range clients {
			if err := connected.drain(); err != nil {
				t.Fatalf("等待 %s 期间消息流中断: %v", label, err)
			}
		}
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("等待 %s 超时；拒绝消息=%+v", label,
				append([]network.CommandRejected(nil), collectedRejections(clients)...))
		default:
			time.Sleep(integrationPollInterval)
		}
	}
}

// collectedRejections 汇集所有客户端收到的拒绝消息，供超时诊断。
func collectedRejections(clients []*armorRestartClient) []network.CommandRejected {
	var all []network.CommandRejected
	for _, connected := range clients {
		all = append(all, connected.rejections...)
	}
	return all
}

func sendArmorRestart(t *testing.T, connected *armorRestartClient, command network.ClientMessage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := connected.endpoint.Send(ctx, command); err != nil {
		t.Fatalf("Send(%T): %v", command, err)
	}
}

func closeArmorRestartClient(t *testing.T, connected *armorRestartClient) {
	t.Helper()
	if err := connected.receiver.Close(); err != nil {
		t.Fatalf("关闭客户端: %v", err)
	}
}

// seedArmorRestartPlayer 在磁盘世界为一名玩家播种指定背包与出生位置的存档。
func seedArmorRestartPlayer(
	t *testing.T,
	root string,
	identity network.Identity,
	inventory core.Inventory,
	position [3]float32,
) {
	t.Helper()
	store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{Create: storage.Metadata{
		FormatVersion: 6, Seed: 42, SpawnDimension: core.Overworld,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("关闭播种用磁盘存档: %v", err)
		}
	}()
	location := storage.PlayerLocation{Dimension: core.Overworld, Position: position}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location, Inventory: inventory,
	})); err != nil {
		t.Fatal(err)
	}
}
