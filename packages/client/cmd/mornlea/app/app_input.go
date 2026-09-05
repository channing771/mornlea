//go:build darwin

package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// dropSelectedItem 请求把权威选中栏位中的一个物品丢到脚下。
// 客户端不预测：不读也不改本地背包镜像，不创建本地掉落物。
func (a *Application) dropSelectedItem() {
	if _, ready := a.predictor.State(); !ready {
		return
	}
	if err := a.send(network.DropSelectedItem{Sequence: a.nextSequence()}); err != nil {
		slog.Warn("发送主动丢弃请求失败", "error", err)
	}
}

func (a *Application) placeBlock() {
	if _, ready := a.predictor.State(); !ready {
		return
	}
	hit, found, err := core.RaycastBlocks(
		a.camera.Pos,
		a.camera.Forward(),
		6,
		func(position core.BlockPos) (bool, error) {
			block, loaded := a.mirror.BlockAt(core.Overworld, position)
			return loaded && core.InteractionTarget(block), nil
		},
	)
	if err != nil {
		slog.Warn("本地容器射线失败", "error", err)
	} else if found {
		block, loaded := a.mirror.BlockAt(core.Overworld, hit.Block)
		// 工作台与熔炉/箱子共用这条既有打开判定路径：服务端才是权威射线，
		// 这里只按本地镜像的方块类型决定发哪种请求——工作台打开的是 3×3 合成
		// 网格而不是容器槽位，具体语义由服务端重新判定，客户端不做任何预测。
		if loaded && (block == core.FurnaceID || block == core.ChestID || block == core.WorkbenchID) {
			if err := a.send(network.OpenContainer{
				Sequence: a.nextSequence(), Yaw: a.camera.Yaw, Pitch: a.camera.Pitch,
			}); err != nil {
				slog.Warn("发送打开容器请求失败", "error", err)
			}
			return
		}
		// 手持锄头对着泥土或草时，「使用」键发翻地命令而不是放置。判定只读
		// 本地只读镜像与已确认的快捷栏，且与权威侧共用 core 里的同一对谓词；
		// 服务端仍会用权威射线重新判定目标，客户端**不做任何预测式改方块**，
		// 耕地要等权威广播回来才出现。其余手持物与目标组合行为完全不变。
		if hotbar, confirmed := a.inventory.Hotbar(); loaded && confirmed &&
			core.TillableBlock(block) &&
			core.TillingTool(hotbar.Slots[hotbar.Selected].Item) {
			if err := a.send(network.TillSoil{
				Sequence: a.nextSequence(), Yaw: a.camera.Yaw, Pitch: a.camera.Pitch,
			}); err != nil {
				slog.Warn("发送翻地命令失败", "error", err)
			}
			return
		}
		// 手持骨粉对着作物时，「使用」键发催熟命令。与翻地同形：只读本地镜像
		// 与已确认快捷栏，目标是否为作物与权威侧共用 core.IsCrop。
		if hotbar, confirmed := a.inventory.Hotbar(); loaded && confirmed &&
			core.IsCrop(block) &&
			hotbar.Slots[hotbar.Selected].Item == core.ItemBoneMeal {
			if err := a.send(network.BoneMeal{
				Sequence: a.nextSequence(), Yaw: a.camera.Yaw, Pitch: a.camera.Pitch,
			}); err != nil {
				slog.Warn("发送骨粉命令失败", "error", err)
			}
			return
		}
	}
	// 放置引用最后一个已确认的选中栏位；尚未确认时不发送。
	hotbar, confirmed := a.inventory.Hotbar()
	if !confirmed {
		return
	}
	// 放置判定与服务端权威放置共用 `core.ItemPlacement` 这同一份事实源：不可
	// 放置的物品（食物、小麦这类原料、各种工具）一律不发 `PlaceBlock`——服务端
	// 必然拒绝这些命令，客户端不发只是为了不刷无谓的拒绝。手持食物时「使用」键
	// 的语义是进食，由 `client.Control` 的进食位逐 tick 上行（见
	// `applyInteractiveInput`），与这条上升沿无关。
	//
	// 直接判上面那份 `hotbar` 而不是再取一次快捷栏：两次取值之间镜像可能被网络
	// goroutine 换掉，于是「拿来判放置的栏位」与「写进 `PlaceBlock.Slot` 的栏位」
	// 来自不同的快照。
	if _, placeable := core.ItemPlacement(hotbar.Slots[hotbar.Selected].Item); !placeable {
		return
	}
	if err := a.send(network.PlaceBlock{
		Sequence: a.nextSequence(),
		Yaw:      a.camera.Yaw,
		Pitch:    a.camera.Pitch,
		Slot:     hotbar.Selected,
	}); err != nil {
		slog.Warn("发送放置命令失败", "error", err)
	}
}

// holdingFood 报告已确认的选中快捷栏位里是不是食物。
//
// 与翻地分支同形：只读**已确认**的权威快捷栏，并与权威侧共用 `core.FoodValue`
// 这同一份食物表。尚未确认时一律返回 false——客户端绝不据猜测的手持物上行意图。
func (a *Application) holdingFood() bool {
	hotbar, confirmed := a.inventory.Hotbar()
	if !confirmed {
		return false
	}
	_, _, ok := core.FoodValue(hotbar.Slots[hotbar.Selected].Item)
	return ok
}

// containerOpen 报告是否有已确认的容器镜像（熔炉、箱子或工作台 3×3 网格）
// 正在驱动当前界面。三个镜像互斥：Apply 一个的同时会 Reset 其余，因此至多
// 一个返回 true。工作台以网格镜像确认尺寸 3 为准——它不是容器（design.md
// D5），但界面语义与熔炉/箱子一致（显式关闭要发 CloseContainer 让服务端
// 回收格 4..8）。
func (a *Application) containerOpen() bool {
	if _, opened := a.furnace.State(); opened {
		return true
	}
	if _, opened := a.chest.State(); opened {
		return true
	}
	if state, opened := a.crafting.State(); opened && state.Size == 3 {
		return true
	}
	return false
}

// setInventoryOpen 切换容器界面：显式关闭时立即清理并通知服务端。
func (a *Application) setInventoryOpen(open bool) {
	a.invalidateGameView()
	a.gameCursorFree = false
	a.gameCharacter = false
	if !open && a.containerOpen() {
		a.clearContainerUI()
		if err := a.send(network.CloseContainer{Sequence: a.nextSequence()}); err != nil {
			slog.Warn("发送关闭容器请求失败", "error", err)
		}
		// 本地翻转也是 hud 分节变化源：containerOpen 布局位不经权威消息就变了，
		// 必须显式置脏，否则要等下一条权威 PlayerState 才下行（纯静默会话里该位
		// 永不下行）。会话关闭时置脏无害——下行窗口由 `syncHUDPushWindow` 拦截。
		a.hudPush.Mark()
		return
	}
	a.inventoryOpen = open
	// 同上：本地开容器同样即时置脏，不依赖下一次权威状态。
	a.hudPush.Mark()
	if a.window != nil {
		a.window.SetCursorCaptured(!open)
	}
	if open {
		// 立即发送一次中性输入，清除服务端保留的上一帧移动。
		a.applyInteractiveInput(0, client.Movement{}, client.Actions{}, false)
	}
}

// clearContainerUI 丢弃当前熔炉、箱子与工作台镜像并关闭容器界面，不发送协议
// 消息。个人 2×2 网格镜像保留：它不是容器，其权威内容在会话内持续有效。
func (a *Application) clearContainerUI() {
	a.invalidateGameView()
	a.furnace.Reset()
	a.chest.Reset()
	if state, opened := a.crafting.State(); opened && state.Size == 3 {
		a.crafting.Reset()
	}
	a.inventoryOpen = false
	if a.window != nil {
		a.window.SetCursorCaptured(true)
	}
}

// selectHotbarSlot 只发送选择请求，不本地改写快捷栏镜像。
func (a *Application) selectHotbarSlot(slot uint8) {
	if _, ready := a.predictor.State(); !ready {
		return
	}
	if err := a.send(network.SelectHotbar{
		Sequence: a.nextSequence(),
		Slot:     slot,
	}); err != nil {
		slog.Warn("发送快捷栏选择失败", "error", err)
	}
}

func (a *Application) send(message network.ClientMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return a.clientEndpoint.Send(ctx, message)
}
