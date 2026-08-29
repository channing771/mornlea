package capture

// capture_item_popup_test.go 钉住物品名弹条场景的位置、触发夹具与恢复语义。

import (
	"testing"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

// TestItemPopupCaptureScenePosition 钉住 hud-item-name-popup 的表内位置：
// 位于 hud-survival-feedback 之后、avatar-nametag 之前（spec delta
// visual-verification「场景表顺序与导出」）。断言写成相邻与前驱关系而不是
// 「在表里」：后者是存在性断言，插到表尾照样通过，正是要挡的那种改动。
func TestItemPopupCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	popup := indexOf("hud-item-name-popup")
	survival := indexOf("hud-survival-feedback")
	nametag := indexOf("avatar-nametag")
	if popup != survival+1 {
		t.Fatalf("hud-item-name-popup=%d 必须紧随 hud-survival-feedback=%d", popup, survival)
	}
	if popup >= nametag {
		t.Fatalf("hud-item-name-popup=%d 必须在 avatar-nametag=%d 之前", popup, nametag)
	}
}

// TestItemPopupCaptureSceneFixture 钉住触发夹具的三条确定性来源：选中格指向
// 含已注册中文显示名的物品；确认选中与前序 HUD 场景钉住的格 2 不同（变化是
// 弹条唯一的触发条件）；HUD 满生命、满氧气、满饥饿。
func TestItemPopupCaptureSceneFixture(t *testing.T) {
	scene := captureSceneByName(t, "hud-item-name-popup")
	if scene.WarmupFrames != 8 || scene.Apply == nil {
		t.Fatalf("hud-item-name-popup 场景不完整: %+v", scene)
	}
	if scene.Popup == nil {
		t.Fatal("hud-item-name-popup 缺少弹条触发夹具")
	}
	hotbar := scene.Popup.Inventory.Hotbar
	stack := hotbar.Slots[hotbar.Selected]
	name, ok := core.ItemDisplayName(stack.Item)
	if !ok || name == "" {
		t.Fatalf("选中格 %d 物品 %d 没有已注册显示名，弹条无法触发", hotbar.Selected, stack.Item)
	}
	if hotbar.Selected == captureHUDHotbarInventory().Hotbar.Selected {
		t.Fatalf("触发选中 %d 与前序 HUD 场景钉住的确认选中相同，首个收敛帧不会检测到变化",
			hotbar.Selected)
	}
	if scene.HUD == nil || scene.HUD.Health != core.MaxHealth ||
		scene.HUD.Oxygen != core.MaxOxygenTicks || scene.HUD.Hunger != core.MaxHunger {
		t.Fatalf("hud-item-name-popup HUD=%+v，想要满生命、满氧气与满饥饿", scene.HUD)
	}
}

// TestCapturePopupFixtureRestoresInventoryAndBaseline 钉住恢复语义：装入夹具
// 换上触发物品栏，恢复后镜像回到装入前快照，重复恢复幂等；未确认镜像时
// 恢复不写入任何物品栏。
func TestCapturePopupFixtureRestoresInventoryAndBaseline(t *testing.T) {
	original := core.Inventory{}
	original.Hotbar.Selected = 2
	original.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 40}
	app := &application.Application{}
	if err := app.Inventory().Apply(network.InventoryState{Inventory: original}); err != nil {
		t.Fatal(err)
	}

	restore, err := applyCapturePopupFixture(app, &capturePopupFixture{
		Inventory: capturePopupTriggerInventory(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, confirmed := app.Inventory().State()
	if !confirmed || got.Hotbar.Selected != capturePopupTriggerInventory().Hotbar.Selected {
		t.Fatalf("装入后选中=%d confirmed=%v，想要触发选中", got.Hotbar.Selected, confirmed)
	}

	restore()
	got, confirmed = app.Inventory().State()
	if !confirmed || got != original {
		t.Fatalf("恢复后物品栏=%+v confirmed=%v，想要快照 %+v/true", got, confirmed, original)
	}
	restore()
	got, _ = app.Inventory().State()
	if got != original {
		t.Fatalf("重复恢复后物品栏=%+v，想要保持快照 %+v", got, original)
	}

	// 未确认镜像（无快照可还原）时恢复不得写入任何物品栏。
	fresh := &application.Application{}
	emptyRestore, err := applyCapturePopupFixture(fresh, &capturePopupFixture{
		Inventory: capturePopupTriggerInventory(),
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyRestore()
	if got, confirmed := fresh.Inventory().State(); !confirmed || got.Hotbar.Selected != capturePopupTriggerInventory().Hotbar.Selected {
		t.Fatalf("未确认快照的恢复不应改写镜像，但选中=%d confirmed=%v", got.Hotbar.Selected, confirmed)
	}
}
