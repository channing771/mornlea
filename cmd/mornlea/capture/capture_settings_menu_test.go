package capture

// capture_settings_menu_test.go 钉住设置页 capture 使用确定性的非默认设置
// 快照(草稿与已保存值一致、不脏);下行呈现由桥状态组装承担。

import (
	"testing"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/config"
)

// TestSettingsMenuCaptureSceneUsesCleanFixture 钉住设置页视觉场景携带完整
// 非默认设置快照，而不是另画一套测试专用表单。
func TestSettingsMenuCaptureSceneUsesCleanFixture(t *testing.T) {
	scene := captureSceneByName(t, "settings-menu")
	if scene.Settings == nil {
		t.Fatal("settings-menu 场景必须携带 Settings 设置快照")
	}
	want := application.SettingsValues{
		AudioVolume:     0.25,
		TexturePackPath: "packs/local",
		WindowSize:      config.WindowSize960x540,
	}
	if scene.Settings.Committed != want || scene.Settings.Draft != want {
		t.Fatalf("settings-menu Settings=%+v，想要已保存与草稿均为 %+v", scene.Settings, want)
	}
	if scene.Settings.Draft != scene.Settings.Committed {
		t.Fatalf("settings-menu 快照不应为脏草稿: %+v", scene.Settings)
	}
	if scene.Menu || scene.WarmupFrames != 8 || scene.Apply == nil {
		t.Fatalf("settings-menu 场景不完整或混入主菜单相位: %+v", scene)
	}
	// 设置页底图同为 menu-vista 全景：必须钉住与主菜单不同的自转时刻。
	if scene.PinVolatile == nil {
		t.Fatal("settings-menu 场景必须钉住全景自转 tick")
	}
}
