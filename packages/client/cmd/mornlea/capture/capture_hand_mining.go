package capture

import (
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是挖掘基线场景：铁镐在手、世界空间浅裂纹在目标砖上同框。机位与夹
// 具复用裂纹场景（裂纹必须贴在真实方块表面），不另起一套摆拍。

// applyHandMiningCaptureState 装入挖掘基线：裂纹场景的固定环境、铁镐选中
// 态、采掘镜像钉在浅阶段（6/30，阶段 2）。
func applyHandMiningCaptureState(app SceneApplication) error {
	if err := applyMiningCrackCaptureState(app); err != nil {
		return err
	}
	// 静态确认状态，不是选中变化；丢弃裂纹清场留下的选中基线（与战斗场景
	// 同一理由），否则确认铁镐时触发弹条。
	app.ResetItemPopupBaseline()
	if err := confirmHandCaptureBackpack(app,
		core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 125}); err != nil {
		return err
	}
	// 采掘镜像经 SetMiningOverlay 直装：Target/HasTarget/进度二元组驱动世
	// 界裂纹（与裂纹场景同一语义）。
	app.SetMiningOverlay(hud.MiningOverlay{
		Active: true, HasTarget: true,
		Target:        captureMiningCrackTarget,
		ProgressTicks: 6, RequiredTicks: 30,
	})
	return nil
}

// pinHandMiningVolatile 在收敛后丢弃双手编码器的挥动边沿：挖掘挥动相位随
// 收敛期间的权威 tick 推进（取决于机器速度），不钉死则最终帧的右手相位逐
// 次不同；清零后最终帧是挖掘上升沿（锚即本 tick、挥动角为零），裂纹仍由采
// 掘镜像呈现，画面逐次一致。
func pinHandMiningVolatile(app SceneApplication) error {
	app.ResetViewmodel()
	return nil
}
