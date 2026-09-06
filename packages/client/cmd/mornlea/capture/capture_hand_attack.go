package capture

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是打击基线场景：铁剑在手、命中标记武装在 6 帧窗内。沿用战斗场景
// 的标记语义（收敛后重武装）；挥动沿需 `CombatHit` 确认才开启（抓帧管线无
// 注入面），像素为中立持剑。画面里是纯第一人称持剑基线，不带受击远端玩家
// （带目标的战斗对照已由战斗场景覆盖）。

// applyHandAttackCaptureState 装入打击基线：半耐久铁剑选中态并武装标记。
func applyHandAttackCaptureState(app SceneApplication) error {
	if err := applyHandCaptureFraming(app); err != nil {
		return err
	}
	if err := confirmHandCaptureBackpack(app,
		core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 125}); err != nil {
		return err
	}
	app.ArmCombatMarker()
	return nil
}
