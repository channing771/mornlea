package capture

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是打击基线场景：铁剑在手、命中标记武装在 6 帧窗内，收敛后合成确
// 认沿使最终帧落在攻击窗第 1 帧（首帧即起挥）。确认沿经抓帧专用缝写入，
// 与线上 `CombatHit` 同语义。画面里是纯第一人称挥动基线，不带受击远端玩
// 家（带目标的战斗对照已由战斗场景覆盖）。

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

// handAttackCaptureTick 是打击基线的合成确认沿：公共清场已把编码器沿清零
// （`lastAttackTick = 0`），收敛期间无任何确认，任意非零值都严格递增；取
// 固定常量使最终帧逐次一致。`Apply` 侧刻意不合成沿，否则与钉死沿同值而不
// 被接受（严格递增），窗口反而打不开。
const handAttackCaptureTick = uint64(1)

// pinHandAttackVolatile 在收敛后重武装标记并合成确认沿：最终帧落在攻击窗
// 第 1 帧（首帧即起挥、挥动角非零），逐次一致。
func pinHandAttackVolatile(app SceneApplication) error {
	app.ArmCombatMarker()
	app.ObserveCombatHitForCapture(handAttackCaptureTick)
	return nil
}
