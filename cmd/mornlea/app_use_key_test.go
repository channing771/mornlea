//go:build darwin

package main

// app_use_key_test.go：「使用」键与丢弃——翻地/进食分支只读已确认快捷栏，客户端不预测。

import (
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestInteractiveDropSendsOnlyWhenReadyAndAllowed(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	ready := network.PlayerState{
		ServerTick: 1,
		Dimension:  core.Overworld,
		Position:   mgl32.Vec3{0.5, 10, 0.5},
		OnGround:   true,
		Ready:      true,
		Reset:      true,
	}

	// 未 Ready：不得发送，也不得分配序号。
	app.applyInteractiveInput(0, client.Movement{}, client.Actions{Drop: true}, true)
	if app.sequence != 0 {
		t.Fatalf("未 Ready 时分配了 sequence=%d", app.sequence)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)

	sendInteractiveServerMessage(t, serverEndpoint, ready)
	app.drainServerMessages(1)

	// Ready 但操作被抑制：同样不得发送。
	app.applyInteractiveInput(0, client.Movement{}, client.Actions{Drop: true}, false)
	if app.sequence != 0 {
		t.Fatalf("allowActions=false 时分配了 sequence=%d", app.sequence)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)

	// Ready 且允许：恰好发送一条只携带序号的请求。
	beforeInventory, beforeHasInventory := app.inventory.State()
	beforeDrops := len(app.itemDrops.Presentations())
	app.applyInteractiveInput(0, client.Movement{}, client.Actions{Drop: true}, true)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	drop, ok := message.(network.DropSelectedItem)
	if !ok {
		t.Fatalf("上行消息 = %T，想要 DropSelectedItem", message)
	}
	if drop.Sequence != 1 {
		t.Fatalf("序号 = %d，想要 1", drop.Sequence)
	}
	// 客户端不预测：本地背包与掉落物镜像都不得改变。
	if got, has := app.inventory.State(); got != beforeInventory || has != beforeHasInventory {
		t.Fatalf("客户端预测了背包扣减：%+v", got)
	}
	if got := len(app.itemDrops.Presentations()); got != beforeDrops {
		t.Fatalf("客户端创建了本地掉落物：%d", got)
	}
}

// TestUseKeySendsTillSoilOnlyForHoeAgainstSoil 覆盖「使用」键的三路分流：手持
// 锄头对着泥土或草必须发翻地命令；手持可放置方块仍发放置；翻地条件不满足的
// 锄头、损坏锄头与镐都是不可放置物，一条命令也不发——服务端本来就会拒绝，
// 客户端不发只是不刷无谓的拒绝（判定与权威侧共用 `core.ItemPlacement`）。
//
// 表里三类行都有对照：只测翻地行的话，一个把所有「使用」都改发翻地的实现也会
// 全绿；只测「不发」行的话，一个把放置整个删掉的实现也会全绿。
func TestUseKeySendsTillSoilOnlyForHoeAgainstSoil(t *testing.T) {
	stoneFull, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	hoeFull, _ := core.ItemMaxDurability(core.ItemStoneHoe)
	for _, tc := range []struct {
		name   string
		target core.BlockID
		held   core.ItemStack
		// want 是「使用」键必须发出的命令；nil 表示一条命令也不发。
		want any
	}{
		{"锄头对草发翻地", core.GrassID,
			core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: hoeFull},
			network.TillSoil{Sequence: 1}},
		{"锄头对泥土发翻地", core.DirtID,
			core.ItemStack{Item: core.ItemIronHoe, Count: 1, Durability: 3},
			network.TillSoil{Sequence: 1}},
		{"锄头对石头不发命令", core.StoneID,
			core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: hoeFull}, nil},
		{"损坏锄头对草不发命令", core.GrassID,
			core.ItemStack{Item: core.ItemBrokenStoneHoe, Count: 1}, nil},
		{"镐对草不发命令", core.GrassID,
			core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: stoneFull}, nil},
		{"普通方块对草仍发放置", core.GrassID,
			core.ItemStack{Item: core.ItemDirt, Count: 1},
			network.PlaceBlock{Sequence: 1, Slot: 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, serverEndpoint := newInteractiveTestApplication(t)
			if err := app.predictor.Begin(network.PlayerState{
				ServerTick: 1, Dimension: core.Overworld,
				Position: mgl32.Vec3{0.5, 10, 3.5}, OnGround: true, Ready: true,
			}); err != nil {
				t.Fatal(err)
			}
			app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
			target := core.BlockPos{X: 0, Y: 10, Z: 0}
			loadInteractiveBlock(t, app, target, tc.target)
			var inventory core.Inventory
			inventory.Hotbar.Selected = 4
			inventory.Hotbar.Slots[4] = tc.held
			if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
				t.Fatal(err)
			}

			app.placeBlock()
			if tc.want != nil {
				if message := receiveInteractiveClientMessage(t, serverEndpoint); message != tc.want {
					t.Fatalf("请求 = %#v，想要 %#v", message, tc.want)
				}
			} else if app.sequence != 0 {
				t.Fatalf("不发命令的组合分配了序号：sequence=%d", app.sequence)
			}
			// 客户端绝不预测式改方块：目标格必须仍是服务端发来的那个编号。
			if got, loaded := app.mirror.BlockAt(core.Overworld, target); !loaded ||
				got != tc.target {
				t.Fatalf("本地镜像被预测式改写: %d, loaded=%v，想要保持 %d",
					got, loaded, tc.target)
			}
			assertNoInteractiveClientMessage(t, serverEndpoint)
		})
	}
}

// TestUseKeyHeldEatsOnlyWhileHoldingFood 钉死「使用」键**按住**的进食位语义：
// 手持食物才把 `PlayerInput.Eating` 置位，其余手持物一律不置。
//
// 表里既有「面包按住置位」，也有「小麦/锄头/方块都不置位」的对照，且锄头那行
// 同时断言仍然发出 `TillSoil`、方块那行断言仍然发出 `PlaceBlock`、小麦那行断言
// 一条命令也不发（小麦不可放置，放置判定收敛后上升沿不再发放置）——只断言
// 「没置进食位」的话，一个把食物分支条件写死为 false 的实现也会全绿。
func TestUseKeyHeldEatsOnlyWhileHoldingFood(t *testing.T) {
	hoeFull, _ := core.ItemMaxDurability(core.ItemStoneHoe)
	for _, tc := range []struct {
		name       string
		held       core.ItemStack
		wantEating bool
		// wantUse 是「使用」键上升沿必须发出的命令类型；nil 表示什么都不发。
		wantUse any
	}{
		{"手持面包按住即进食", core.ItemStack{Item: core.ItemBread, Count: 2}, true, nil},
		{"手持小麦不进食", core.ItemStack{Item: core.ItemWheat, Count: 3}, false, nil},
		{"手持锄头仍翻地", core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: hoeFull},
			false, network.TillSoil{Sequence: 1}},
		{"手持方块仍放置", core.ItemStack{Item: core.ItemDirt, Count: 9}, false,
			network.PlaceBlock{Sequence: 1, Slot: 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, serverEndpoint := newInteractiveTestApplication(t)
			if err := app.predictor.Begin(network.PlayerState{
				ServerTick: 1, Dimension: core.Overworld,
				Position: mgl32.Vec3{0.5, 10, 3.5}, OnGround: true, Ready: true,
			}); err != nil {
				t.Fatal(err)
			}
			app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
			loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.GrassID)
			var inventory core.Inventory
			inventory.Hotbar.Selected = 4
			inventory.Hotbar.Slots[4] = tc.held
			if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
				t.Fatal(err)
			}
			beforeInventory, _ := app.inventory.State()

			// 第一帧：使用键刚按下，同时给出上升沿与按住态。
			app.applyInteractiveInput(physics.FixedDelta, client.Movement{},
				client.Actions{Place: true, Use: true}, true)
			if tc.wantUse != nil {
				if got := receiveInteractiveClientMessage(t, serverEndpoint); got != tc.wantUse {
					t.Fatalf("使用键上升沿 = %#v，想要 %#v", got, tc.wantUse)
				}
			}
			input, ok := receiveInteractiveClientMessage(t, serverEndpoint).(network.PlayerInput)
			if !ok {
				t.Fatal("按住使用键没有上行玩家输入")
			}
			if input.Eating != tc.wantEating {
				t.Fatalf("按住首帧 Eating=%v，想要 %v", input.Eating, tc.wantEating)
			}

			// 第二帧：仍然按住（无上升沿），进食位必须保持。
			app.applyInteractiveInput(physics.FixedDelta, client.Movement{},
				client.Actions{Use: true}, true)
			held, ok := receiveInteractiveClientMessage(t, serverEndpoint).(network.PlayerInput)
			if !ok || held.Eating != tc.wantEating {
				t.Fatalf("持续按住 = %#v，想要 Eating=%v", held, tc.wantEating)
			}

			// 第三帧：松开使用键，进食位必须落回 false。
			app.applyInteractiveInput(physics.FixedDelta, client.Movement{},
				client.Actions{}, true)
			released, ok := receiveInteractiveClientMessage(t, serverEndpoint).(network.PlayerInput)
			if !ok || released.Eating {
				t.Fatalf("松开使用键 = %#v，想要 Eating=false", released)
			}

			// 客户端不做任何本地预测：手持食物长按也不得扣减本地背包镜像。
			if got, _ := app.inventory.State(); got != beforeInventory {
				t.Fatalf("客户端预测了进食扣料: %+v", got)
			}
			assertNoInteractiveClientMessage(t, serverEndpoint)
		})
	}
}

// TestUseKeyRisingEdgeSkipsPlaceForNonPlaceableItem 单独钉死「使用」键上升沿的
// 放置判定收敛：判定与服务端权威放置共用 `core.ItemPlacement` 这同一份事实源，
// 不可放置物（面包这类食物、小麦这类原料）一律不发放置命令也不分配序号——
// 服务端本来就会拒绝，客户端不发只是不刷无谓的拒绝。用未确认快捷栏做对照——
// 分支只读**已确认**的权威快捷栏。
func TestUseKeyRisingEdgeSkipsPlaceForNonPlaceableItem(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 3.5}, OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}

	// 快捷栏尚未确认：既不发放置也不置进食位。
	app.applyInteractiveInput(physics.FixedDelta, client.Movement{},
		client.Actions{Place: true, Use: true}, true)
	unconfirmed, ok := receiveInteractiveClientMessage(t, serverEndpoint).(network.PlayerInput)
	if !ok || unconfirmed.Eating {
		t.Fatalf("未确认快捷栏 = %#v，想要 Eating=false", unconfirmed)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)

	// 上面那帧 `PlayerInput` 已消耗序号 1；下面每次跳过的放置都不得再分配。
	for _, held := range []core.ItemStack{
		{Item: core.ItemBread, Count: 1},
		{Item: core.ItemWheat, Count: 3},
	} {
		var inventory core.Inventory
		inventory.Hotbar.Selected = 2
		inventory.Hotbar.Slots[2] = held
		if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
			t.Fatal(err)
		}
		app.placeBlock()
		assertNoInteractiveClientMessage(t, serverEndpoint)
		if app.sequence != 1 {
			t.Fatalf("手持物品 %d 的使用键上升沿分配了序号：sequence=%d", held.Item, app.sequence)
		}
	}
}
