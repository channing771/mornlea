package capture

// capture_main_menu_test.go 钉住正式主菜单 capture 快照、按钮语义以及它与
// settings-menu、far-horizon 的相对位置。

import "testing"

// TestMainMenuCaptureScenePosition 钉住 main-menu 场景的存在与位置。
//
// main-menu 与 settings-menu 是视觉场景表中仅有的两个菜单相位场景，必须依次
// 排在 far-horizon 之前（far-horizon 仍为倒数第二、water-underwater 仍为最后，
// 另由 `TestFarHorizonCaptureSceneIsRegistered` 与
// `TestWaterUnderwaterCaptureSceneIsLast` 兜底）。断言写「位于 far-horizon 之前」
// 而不是「在表里」：后者是存在性断言，插到 far-horizon 之后照样通过，正是要挡
// 的那种改动。`Menu` 必须为真——菜单相位场景不进菜单相位就没有任何意义。
// 菜单层已迁 WebView,本场景输出无 chrome 的世界底图,按钮语义由 app 包的
// 桥状态测试钉住。
func TestMainMenuCaptureScenePosition(t *testing.T) {
	scene := captureSceneByName(t, "main-menu")
	if !scene.Menu {
		t.Fatal("main-menu 场景必须声明菜单相位")
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
