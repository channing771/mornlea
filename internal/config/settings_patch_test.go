package config_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/internal/config"
)

func TestPatchSettingsPreservesRawValuesOutsideOwnedMembers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := []byte(`{
  "version": 1,
  "audioVolume": 0.7,
  "texturePackPath": "old",
  "windowSize": "1280x720",
  "futureTop": { "array": [1, {"enabled": true}] },
  "render": { "viewDistance": 999, "fovDegrees": 70, "mouseSensitivity": 1, "lodEnabled": true, "lodFarMultiplier": 3, "lodStep": 4, "futureNested": {"raw": "keep"} }
}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := decodeRawSettingsObject(t, body)

	if err := config.PatchSettings(path, config.SettingsPatch{
		AudioVolume: 0.25, TexturePackPath: "next", WindowSize: config.WindowSize960x540,
	}); err != nil {
		t.Fatalf("PatchSettings: %v", err)
	}
	afterBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after := decodeRawSettingsObject(t, afterBody)
	for _, key := range []string{"futureTop", "render", "version"} {
		if !bytes.Equal(before[key], after[key]) {
			t.Fatalf("%s raw value changed:\nbefore=%s\nafter=%s", key, before[key], after[key])
		}
	}
	if string(after["audioVolume"]) != "0.25" || string(after["texturePackPath"]) != `"next"` ||
		string(after["windowSize"]) != `"960x540"` {
		t.Fatalf("owned fields not patched: %s", afterBody)
	}
}

func TestPatchSettingsMissingFileCreatesLoadableDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	patch := config.SettingsPatch{
		AudioVolume: 0.5, TexturePackPath: "packs/local", WindowSize: config.WindowSize640x360,
	}
	if err := config.PatchSettings(path, patch); err != nil {
		t.Fatalf("PatchSettings: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != config.CurrentVersion || loaded.AudioVolume != patch.AudioVolume ||
		loaded.TexturePackPath != patch.TexturePackPath || loaded.WindowSize != patch.WindowSize {
		t.Fatalf("created config=%+v", loaded)
	}
	if loaded.Render != config.Defaults().Render {
		t.Fatalf("missing-file defaults lost: render=%+v", loaded.Render)
	}
}

func TestPatchSettingsRejectsDamagedOrUnreadableTargetWithoutMutation(t *testing.T) {
	patch := config.SettingsPatch{AudioVolume: 0.5, WindowSize: config.WindowSize640x360}
	t.Run("damaged JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		before := []byte(`{"version":1,"render":`)
		if err := os.WriteFile(path, before, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := config.PatchSettings(path, patch); err == nil {
			t.Fatal("damaged config unexpectedly accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("damaged target changed: before=%q after=%q", before, after)
		}
	})
	t.Run("I/O failure", func(t *testing.T) {
		parentFile := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(parentFile, []byte("sentinel"), 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(parentFile, "config.json")
		if err := config.PatchSettings(path, patch); err == nil {
			t.Fatal("I/O failure unexpectedly accepted")
		}
		after, err := os.ReadFile(parentFile)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, []byte("sentinel")) {
			t.Fatalf("I/O failure changed sentinel: %q", after)
		}
		if _, err := os.Stat(path); err == nil {
			t.Fatal("I/O failure left target")
		}
	})
}

func decodeRawSettingsObject(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var values map[string]json.RawMessage
	if err := json.Unmarshal(body, &values); err != nil {
		t.Fatalf("decode raw object: %v", err)
	}
	return values
}
