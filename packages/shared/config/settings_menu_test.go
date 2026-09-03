package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/config"
)

func TestWindowSizeDefaultsTo1280x720(t *testing.T) {
	if got := config.Defaults().WindowSize; got != config.WindowSize1280x720 {
		t.Fatalf("WindowSize = %q，want %q", got, config.WindowSize1280x720)
	}

	loaded, err := config.Load(writeConfig(t, `{"version":1}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.WindowSize != config.WindowSize1280x720 {
		t.Fatalf("缺席 windowSize 时 WindowSize = %q，want %q", loaded.WindowSize, config.WindowSize1280x720)
	}
}

func TestWindowSizeLoadsAllPresetsAndMapsDimensions(t *testing.T) {
	tests := []struct {
		value         config.WindowSize
		width, height int
	}{
		{value: config.WindowSize640x360, width: 640, height: 360},
		{value: config.WindowSize960x540, width: 960, height: 540},
		{value: config.WindowSize1280x720, width: 1280, height: 720},
	}
	for _, test := range tests {
		t.Run(string(test.value), func(t *testing.T) {
			loaded, err := config.Load(writeConfig(t, fmt.Sprintf(`{"version":1,"windowSize":%q}`, test.value)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if loaded.WindowSize != test.value {
				t.Fatalf("WindowSize = %q，want %q", loaded.WindowSize, test.value)
			}
			width, height := loaded.WindowSize.Dimensions()
			if width != test.width || height != test.height {
				t.Fatalf("Dimensions = (%d,%d)，want (%d,%d)", width, height, test.width, test.height)
			}
		})
	}
}

func TestWindowSizeRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{`"1920x1080"`, `null`, `7`, `true`, `{}`} {
		t.Run(value, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, `{"version":1,"windowSize":`+value+`}`))
			if err == nil || !strings.Contains(err.Error(), "windowSize") {
				t.Fatalf("Load error = %v，want windowSize 字段上下文", err)
			}
		})
	}
}

func TestWindowSizeSaveLoadRoundTrip(t *testing.T) {
	for _, want := range []config.WindowSize{
		config.WindowSize640x360,
		config.WindowSize960x540,
		config.WindowSize1280x720,
	} {
		t.Run(string(want), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			cfg := config.Defaults()
			cfg.WindowSize = want
			if err := cfg.Save(path); err != nil {
				t.Fatalf("Save: %v", err)
			}

			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var written map[string]json.RawMessage
			if err := json.Unmarshal(body, &written); err != nil {
				t.Fatalf("保存产物不是合法 JSON: %v", err)
			}
			if got := string(written["windowSize"]); got != fmt.Sprintf("%q", want) {
				t.Fatalf("保存的 windowSize = %s，want %q", got, want)
			}

			loaded, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load after Save: %v", err)
			}
			if loaded.WindowSize != want {
				t.Fatalf("round-trip WindowSize = %q，want %q", loaded.WindowSize, want)
			}
		})
	}
}

func TestSettingsMenuTopLevelFieldsStayOutOfNumericFields(t *testing.T) {
	for _, field := range config.Fields() {
		if strings.EqualFold(field.Name, "windowSize") {
			t.Fatalf("windowSize 不得进入数值 Fields: %+v", field)
		}
	}
}

func TestTexturePackPathByteLimit(t *testing.T) {
	accepted := strings.Repeat("界", 341) + "a"
	rejected := accepted + "b"
	if len(accepted) != config.MaxTexturePackPathBytes || len(rejected) != config.MaxTexturePackPathBytes+1 {
		t.Fatalf("测试路径长度 = (%d,%d)，want (%d,%d)",
			len(accepted), len(rejected), config.MaxTexturePackPathBytes, config.MaxTexturePackPathBytes+1)
	}

	loaded, err := config.Load(writeConfig(t, fmt.Sprintf(`{"version":1,"texturePackPath":%q}`, accepted)))
	if err != nil {
		t.Fatalf("Load 接受 %d 字节路径: %v", config.MaxTexturePackPathBytes, err)
	}
	if loaded.TexturePackPath != accepted {
		t.Fatalf("TexturePackPath 长度 = %d，want %d", len(loaded.TexturePackPath), len(accepted))
	}

	_, err = config.Load(writeConfig(t, fmt.Sprintf(`{"version":1,"texturePackPath":%q}`, rejected)))
	if err == nil || !strings.Contains(err.Error(), "texturePackPath") {
		t.Fatalf("Load %d 字节路径 error = %v，want texturePackPath 字段错误",
			config.MaxTexturePackPathBytes+1, err)
	}
}

func TestTexturePackPathRejectsCRAndLFBeforeFilesystemUse(t *testing.T) {
	for _, test := range []struct {
		name      string
		separator string
	}{
		{name: "LF", separator: "\n"},
		{name: "CR", separator: "\r"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Darwin 文件系统允许目录名包含 CR/LF；即使目标真实存在，配置
			// 边界也必须拒绝原文，而不是把它解析成可使用路径。
			packPath := filepath.Join(t.TempDir(), "pack"+test.separator+"dir")
			if err := os.Mkdir(packPath, 0o700); err != nil {
				t.Fatalf("创建含 %s 的真实目录: %v", test.name, err)
			}
			_, err := config.Load(writeConfig(t,
				fmt.Sprintf(`{"version":1,"texturePackPath":%q}`, packPath)))
			if err == nil || !strings.Contains(err.Error(), "texturePackPath") {
				t.Fatalf("Load 含 %s 的真实目录 error=%v，want texturePackPath 字段错误", test.name, err)
			}
		})
	}
}

func TestTexturePackPathAcceptsOrdinarySingleLineValue(t *testing.T) {
	path := writeConfig(t, `{"version":1,"texturePackPath":"packs/local"}`)
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load 单行路径: %v", err)
	}
	if loaded.TexturePackPath != "packs/local" {
		t.Fatalf("TexturePackPath=%q，want packs/local", loaded.TexturePackPath)
	}
	want := filepath.Join(filepath.Dir(path), "packs/local")
	if loaded.ResolvedTexturePackPath != want {
		t.Fatalf("ResolvedTexturePackPath=%q，want %q", loaded.ResolvedTexturePackPath, want)
	}
}
