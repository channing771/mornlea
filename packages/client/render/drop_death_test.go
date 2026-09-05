package render

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// dropPartScale 从部件变换的第一列模长提取均匀缩放（与旋转/平移解耦）。
func dropPartScale(part avatarPart) float32 {
	x, y, z := part.transform[0], part.transform[1], part.transform[2]
	return float32(math.Sqrt(float64(x*x + y*y + z*z)))
}

// TestDeathLinkedDropHidesBeforeHalfAndScalesInWithFlash 锁定死亡关联滞后：
// 关联掉落在死亡相位 50% 前不渲染，50% 起 scale-in 渐显并叠一次白色闪光；
// 非关联掉落不受影响，拾取（权威侧）更不受影响（本包只管呈现）。
func TestDeathLinkedDropHidesBeforeHalfAndScalesInWithFlash(t *testing.T) {
	linked := ItemDrop{
		ID:        core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1},
		Block:     core.BlockPos{X: 0, Y: 1, Z: -1},
		Item:      core.ItemStone,
		DeathTick: 100,
	}
	// 50% 之前（T+9）不可见。
	if got := buildItemDropParts(nil, 109, []ItemDrop{linked}); len(got) != 0 {
		t.Fatalf("T+9 关联掉落实例=%d，想要 0（隐藏）", len(got))
	}
	// 50% 起（T+10）渐显：小尺寸 + 白闪。
	half := buildItemDropParts(nil, 110, []ItemDrop{linked})
	if len(half) != 1 {
		t.Fatalf("T+10 关联掉落实例=%d，想要 1", len(half))
	}
	if scale := dropPartScale(half[0]); scale >= dropCubeSize {
		t.Fatalf("T+10 缩放=%v，想要小于全尺寸 %v（scale-in）", scale, dropCubeSize)
	}
	if half[0].color != [4]float32{2, 2, 2, 1} {
		t.Fatalf("T+10 颜色=%v，想要白色闪光", half[0].color)
	}
	// 保留末（T+20）长满：全尺寸 + 正常白。
	full := buildItemDropParts(nil, 120, []ItemDrop{linked})
	if len(full) != 1 {
		t.Fatalf("T+20 关联掉落实例=%d，想要 1", len(full))
	}
	if scale := dropPartScale(full[0]); scale != dropCubeSize {
		t.Fatalf("T+20 缩放=%v，想要全尺寸 %v", scale, dropCubeSize)
	}
	if full[0].color != [4]float32{1, 1, 1, 1} {
		t.Fatalf("T+20 颜色=%v，想要正常白", full[0].color)
	}
	// 渐显单调：T+15 尺寸介于两者之间。
	mid := buildItemDropParts(nil, 115, []ItemDrop{linked})
	if len(mid) != 1 {
		t.Fatalf("T+15 关联掉落实例=%d，想要 1", len(mid))
	}
	if small, medium, large := dropPartScale(half[0]), dropPartScale(mid[0]), dropPartScale(full[0]); !(small < medium && medium < large) {
		t.Fatalf("缩放非单调：%v/%v/%v，想要渐显", small, medium, large)
	}
	// 非关联对照：同 tick 全尺寸正常白。
	plain := ItemDrop{
		ID:    core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1},
		Block: core.BlockPos{X: 0, Y: 1, Z: -1},
		Item:  core.ItemStone,
	}
	control := buildItemDropParts(nil, 109, []ItemDrop{plain})
	if len(control) != 1 {
		t.Fatalf("非关联掉落实例=%d，想要 1（不受滞后影响）", len(control))
	}
	if scale := dropPartScale(control[0]); scale != dropCubeSize {
		t.Fatalf("非关联缩放=%v，想要全尺寸", scale)
	}
	// 同 tick 重放确定。
	repeat := buildItemDropParts(nil, 115, []ItemDrop{linked})
	if len(repeat) != 1 || repeat[0] != mid[0] {
		t.Fatal("同 tick 关联掉落重放不一致，想要确定")
	}
}
