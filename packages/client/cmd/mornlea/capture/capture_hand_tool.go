package capture

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// 本文件是手持基线四景的共用装帧与工具手持场景：四个场景共用与战斗场景
// 相同的固定机位（双手落点四景可比），只换右手持物与动作状态。

// applyHandCaptureFraming 钉死手持基线共用的呈现帧：公共清场、正午、固定
// 机位与中心同步；背包由各场景自行确认（选中变化不触发弹条基线污染）。
func applyHandCaptureFraming(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	// 与战斗场景同一机位：双手落点在四景之间可比，差异只来自持物与动作。
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{5.5, 3.2, 9.5}, Yaw: 0, Pitch: -0.05,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)
	if app.Panel() != nil {
		app.Panel().SetVisible(false)
	}
	// 静态确认状态，不是选中变化；丢弃前序场景的选中基线，避免确认持物
	// 时触发弹条（与战斗场景同一理由）。
	app.ResetItemPopupBaseline()
	app.SetInventoryOpen(false)
	return nil
}

// confirmHandCaptureBackpack 确认手持场景的背包：2 号槽选中指定持物栈。
func confirmHandCaptureBackpack(app SceneApplication, stack core.ItemStack) error {
	inv := core.Inventory{}
	inv.Hotbar.Selected = 2
	inv.Hotbar.Slots[2] = stack
	if err := app.Inventory().Apply(network.InventoryState{Inventory: inv}); err != nil {
		return fmt.Errorf("装入手持场景背包: %w", err)
	}
	return nil
}

// applyHandToolCaptureState 装入工具手持基线：半耐久铁镐选中态、中立持握。
func applyHandToolCaptureState(app SceneApplication) error {
	if err := applyHandCaptureFraming(app); err != nil {
		return err
	}
	return confirmHandCaptureBackpack(app,
		core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 125})
}
