package capture

// capture_settings_menu_test.go 钉住设置页 capture 使用正式 client ABI v9
// layout v2 及确定性的非默认设置快照。

import (
	"encoding/binary"
	"testing"

	"github.com/channing771/mornlea/internal/client"
)

// TestSettingsMenuCaptureSceneUsesLayoutV2Fixture 钉住设置页视觉场景使用正式
// layout v2 的完整非默认快照，而不是另画一套测试专用表单。
func TestSettingsMenuCaptureSceneUsesLayoutV2Fixture(t *testing.T) {
	scene := captureSceneByName(t, "settings-menu")
	if scene.Settings == nil {
		t.Fatal("settings-menu 场景必须携带 Settings 设置快照")
	}
	want := client.UISettings{
		Visible:         true,
		AudioVolume:     0.25,
		Window:          client.UISettingsWindow960x540,
		TexturePackPath: "packs/local",
		Dirty:           false,
	}
	if got := scene.Settings.UISettings(); got != want {
		t.Fatalf("settings-menu Settings=%+v，想要 %+v", got, want)
	}
	if scene.Menu != nil || scene.WarmupFrames != 8 || scene.Apply == nil {
		t.Fatalf("settings-menu 场景不完整或混入主菜单快照: %+v", scene)
	}
	encoded := client.EncodeUISettings(scene.Settings.UISettings())
	if len(encoded) < 4 || binary.LittleEndian.Uint32(encoded[:4]) != 2 {
		t.Fatalf("settings-menu 未编码为 layout v2: %v", encoded)
	}
}
