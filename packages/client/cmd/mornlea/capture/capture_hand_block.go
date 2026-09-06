package capture

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是方块手持基线场景：泥土微缩立方在右手上方，中立持握。
//
// 泥土立方的顶面/侧面材质与世界同源（与掉落物同一代表层），本场景即该契
// 约的像素基线。

// applyHandBlockCaptureState 装入方块手持基线：泥土选中态、中立持握。
func applyHandBlockCaptureState(app SceneApplication) error {
	if err := applyHandCaptureFraming(app); err != nil {
		return err
	}
	return confirmHandCaptureBackpack(app, core.ItemStack{Item: core.ItemDirt, Count: 1})
}
