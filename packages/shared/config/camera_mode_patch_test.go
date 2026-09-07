package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/packages/shared/config"
)

func TestPatchCameraModeRoundTripPreservesOtherMembers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := []byte(`{
  "version": 1,
  "audioVolume": 0.7,
  "cameraMode": 0,
  "futureTop": { "array": [1, {"enabled": true}] }
}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := decodeRawSettingsObject(t, body)

	result, err := config.PatchCameraMode(path, 1)
	if err != nil {
		t.Fatalf("PatchCameraMode: %v", err)
	}
	if !result.Committed {
		t.Fatal("PatchCameraMode 成功却未标记提交")
	}
	afterBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after := decodeRawSettingsObject(t, afterBody)
	for _, key := range []string{"futureTop", "version", "audioVolume"} {
		if !bytes.Equal(before[key], after[key]) {
			t.Fatalf("%s raw value changed:\nbefore=%s\nafter=%s", key, before[key], after[key])
		}
	}
	if string(after["cameraMode"]) != "1" {
		t.Fatalf("cameraMode not patched: %s", afterBody)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.CameraMode != 1 {
		t.Fatalf("Load CameraMode = %d，想要 1", loaded.CameraMode)
	}
}

func TestPatchCameraModeRejectsOutOfRangeWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	before := []byte(`{"version":1,"cameraMode":2}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []int{-1, 3, 99} {
		if _, err := config.PatchCameraMode(path, mode); err == nil {
			t.Fatalf("mode %d unexpectedly accepted", mode)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected patch changed target: before=%q after=%q", before, after)
	}
}

func TestPatchCameraModeMissingFileCreatesLoadableDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	result, err := config.PatchCameraMode(path, 2)
	if err != nil {
		t.Fatalf("PatchCameraMode: %v", err)
	}
	if !result.Committed {
		t.Fatal("PatchCameraMode 成功却未标记提交")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.CameraMode != 2 {
		t.Fatalf("Load CameraMode = %d，想要 2", loaded.CameraMode)
	}
	if loaded.Render != config.Defaults().Render {
		t.Fatalf("missing-file defaults lost: render=%+v", loaded.Render)
	}
}

func TestLoadCameraModeDefaultsAndClamps(t *testing.T) {
	write := func(t *testing.T, body string) config.Config {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		loaded, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return loaded
	}
	if got := write(t, `{"version":1}`); got.CameraMode != 0 {
		t.Fatalf("缺失 cameraMode = %d，想要第一人称 0", got.CameraMode)
	}
	if got := write(t, `{"version":1,"cameraMode":9}`); got.CameraMode != 0 {
		t.Fatalf("越界 cameraMode = %d，想要落回第一人称 0", got.CameraMode)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"cameraMode":"back"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("类型错误的 cameraMode 意外通过")
	}
}
