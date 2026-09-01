package capture

// capture_main_menu_test.go 钉住正式主菜单 capture 快照、按钮语义以及它与
// settings-menu、far-horizon 的相对位置。

import (
	"testing"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
)

// TestMainMenuCaptureScenePosition 钉住 main-menu 场景的存在与位置。
//
// main-menu 与 settings-menu 是视觉场景表中仅有的两个菜单相位场景，必须依次
// 排在 far-horizon 之前（far-horizon 仍为倒数第二、water-underwater 仍为最后，
// 另由 `TestFarHorizonCaptureSceneIsRegistered` 与
// `TestWaterUnderwaterCaptureSceneIsLast` 兜底）。断言写「位于 far-horizon 之前」
// 而不是「在表里」：后者是存在性断言，插到 far-horizon 之后照样通过，正是要挡
// 的那种改动。`Menu` 必须为真——菜单相位场景不进菜单相位就没有任何意义。
// 两张 golden 的内容是对应相位的世界全景底图：场景必须经 PinVolatile 钉住
// menu-vista 自转 tick（收敛帧数随机器速度波动，不钉住则相机姿态不可复现），
// 且两个场景必须钉在不同的自转角，让两张底图各自可辨。
func TestMainMenuCaptureScenePosition(t *testing.T) {
	scene := captureSceneByName(t, "main-menu")
	if !scene.Menu {
		t.Fatal("main-menu 场景必须声明菜单相位")
	}
	if scene.WarmupFrames != 8 || scene.Apply == nil {
		t.Fatalf("main-menu 场景不完整: %+v", scene)
	}
	if scene.PinVolatile == nil {
		t.Fatal("main-menu 场景必须钉住全景自转 tick")
	}
	if scene.Settings != nil {
		t.Fatalf("main-menu 场景不应携带设置夹具: %+v", scene)
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

// TestMenuCaptureScenesPinDistinctVistaTicks 钉住两个菜单场景钉住的全景
// 自转时刻：都在一个自转周期内且互不相同——同一全景世界因此呈现出两个
// 可区分的相机时刻，两张 golden 不是同一张图的两份拷贝。场景闭包与本测试
// 消费同一组常量，任何一侧单独漂移都会红。
func TestMenuCaptureScenesPinDistinctVistaTicks(t *testing.T) {
	if captureMenuVistaTickMainMenu >= application.MenuVistaYawPeriodTicks {
		t.Fatalf("main-menu 钉住的 tick %d 越出一个自转周期", captureMenuVistaTickMainMenu)
	}
	if captureMenuVistaTickSettingsMenu >= application.MenuVistaYawPeriodTicks {
		t.Fatalf("settings-menu 钉住的 tick %d 越出一个自转周期", captureMenuVistaTickSettingsMenu)
	}
	if captureMenuVistaTickMainMenu == captureMenuVistaTickSettingsMenu {
		t.Fatalf("两个菜单场景钉住了同一自转 tick %d：两张底图会不可区分",
			captureMenuVistaTickMainMenu)
	}
	for _, name := range []string{"main-menu", "settings-menu"} {
		scene := captureSceneByName(t, name)
		if scene.PinVolatile == nil {
			t.Fatalf("场景 %s 缺少全景自转 tick 钉住", name)
		}
	}
}
