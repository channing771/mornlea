package capture

// capture_main_menu_test.go 钉住正式主菜单 capture 快照、按钮语义以及它与
// settings-menu、far-horizon 的相对位置。

import "testing"

// TestMainMenuCaptureScenePosition 钉住 main-menu 场景的存在与位置。
//
// main-menu 与 settings-menu 是视觉场景表中仅有的两个 egui UI 场景，必须依次
// 排在 far-horizon 之前（far-horizon 仍为倒数第二、water-underwater 仍为最后，
// 另由 `TestFarHorizonCaptureSceneIsRegistered` 与
// `TestWaterUnderwaterCaptureSceneIsLast` 兜底）。断言写「位于 far-horizon 之前」
// 而不是「在表里」：后者是存在性断言，插到 far-horizon 之后照样通过，正是要挡
// 的那种改动。`Menu` 字段必须非 nil——菜单场景不注入菜单快照就没有任何意义。
func TestMainMenuCaptureScenePosition(t *testing.T) {
	scene := captureSceneByName(t, "main-menu")
	if scene.Menu == nil {
		t.Fatal("main-menu 场景必须携带 Menu 菜单快照")
	}
	menu := scene.Menu
	if !menu.Visible || menu.Title != "Mornlea" || menu.Version != "dev" ||
		menu.Error != "存档无法打开" {
		t.Fatalf("main-menu Menu=%+v 与夹具不符", menu)
	}
	if len(menu.Buttons) != 4 {
		t.Fatalf("main-menu 按钮数=%d，想要 4", len(menu.Buttons))
	}
	// 进入/设置/退出可用、多人禁用：复用交互主菜单的按钮表语义。
	if menu.Buttons[0].Label != "进入游戏" || !menu.Buttons[0].Enabled ||
		menu.Buttons[1].Label != "多人游戏" || menu.Buttons[1].Enabled ||
		menu.Buttons[2].Label != "设置" || !menu.Buttons[2].Enabled ||
		menu.Buttons[3].Label != "退出游戏" || !menu.Buttons[3].Enabled {
		t.Fatalf("main-menu 按钮表与既有交互菜单不一致: %+v", menu.Buttons)
	}
	if scene.WarmupFrames != 8 || scene.Apply == nil {
		t.Fatalf("main-menu 场景不完整: %+v", scene)
	}
	indexOf := func(name string) int {
		for i, s := range captureScenes {
			if s.Name == name {
				return i
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	if indexOf("settings-menu") != indexOf("main-menu")+1 ||
		indexOf("settings-menu") >= indexOf("far-horizon") {
		t.Fatalf("main-menu/settings-menu 必须相邻并排在 far-horizon 之前: main-menu=%d settings-menu=%d far-horizon=%d",
			indexOf("main-menu"), indexOf("settings-menu"), indexOf("far-horizon"))
	}
}
