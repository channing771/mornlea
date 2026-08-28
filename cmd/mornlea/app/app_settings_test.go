//go:build darwin

package app

// app_settings_test.go：设置页的 Go 草稿状态机、保存事务与运行时资源替换测试。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
)

type settingsTestWindow struct {
	fakeInteractiveWindow
	events                              *[]string
	contentWidth, contentHeight         int
	framebufferWidth, framebufferHeight int
	framebufferScale                    int
	setWidth, setHeight                 int
	setCalls, pollCalls                 int
}

func (window *settingsTestWindow) ContentSize() (int, int) {
	return window.contentWidth, window.contentHeight
}

func (window *settingsTestWindow) FramebufferSize() (int, int) {
	return window.framebufferWidth, window.framebufferHeight
}

func (window *settingsTestWindow) SetContentSize(width, height int) {
	window.setCalls++
	window.setWidth, window.setHeight = width, height
	window.contentWidth, window.contentHeight = width, height
	scale := max(1, window.framebufferScale)
	window.framebufferWidth, window.framebufferHeight = width*scale, height*scale
	if window.events != nil {
		*window.events = append(*window.events, fmt.Sprintf("window %dx%d", width, height))
	}
}

func (window *settingsTestWindow) Poll() {
	window.pollCalls++
	if window.events != nil {
		*window.events = append(*window.events, "poll")
	}
}

func newSettingsStateTestApplication(values SettingsValues) *Application {
	app := &Application{
		menu: menuState{phase: MenuPhaseMenu, title: "Mornlea", version: "dev"},
		settings: SettingsState{
			Committed: values,
			Draft:     values,
		},
		startupOptions: Options{
			ConfigPath:      "unused.json",
			AudioVolume:     values.AudioVolume,
			TexturePackPath: values.TexturePackPath,
			WindowSize:      values.WindowSize,
		},
		itemDrops: client.NewItemDrops(),
	}
	app.releaseResources = app.releaseOwnedResources
	return app
}

func TestSettingsNavigationDraftCancelAndBack(t *testing.T) {
	Committed := SettingsValues{
		AudioVolume: 0.25, TexturePackPath: "packs/local", WindowSize: config.WindowSize960x540,
	}
	app := newSettingsStateTestApplication(Committed)

	if quit := app.handleMenuEvent(menuActionSettings); quit {
		t.Fatal("进入设置不应退出")
	}
	if app.menu.phase != MenuPhaseSettings || app.settings.dirty() {
		t.Fatalf("进入设置 phase=%v dirty=%v", app.menu.phase, app.settings.dirty())
	}
	if app.settings.Draft != Committed || app.settings.Committed != Committed {
		t.Fatalf("设置初始化错误: %+v", app.settings)
	}

	changed := client.UISettingsValues{
		AudioVolume: 0.5, TexturePackPath: "packs/next", Window: client.UISettingsWindow640x360,
	}
	quit, disposition := app.handleMenuUIEvent(client.UIEvent{Kind: client.UIEventSettingsChanged, Settings: changed})
	if quit || disposition != menuUIEventHandled {
		t.Fatalf("typed change quit=%v disposition=%v", quit, disposition)
	}
	wantDraft := SettingsValues{AudioVolume: 0.5, TexturePackPath: "packs/next", WindowSize: config.WindowSize640x360}
	if app.settings.Draft != wantDraft || !app.settings.dirty() {
		t.Fatalf("Draft=%+v dirty=%v，want %+v/true", app.settings.Draft, app.settings.dirty(), wantDraft)
	}

	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != MenuPhaseSettings || app.settings.Draft != wantDraft || app.settings.status == "" {
		t.Fatalf("dirty 返回丢失草稿或未提示: phase=%v state=%+v", app.menu.phase, app.settings)
	}
	app.handleMenuEvent(menuActionSettingsCancel)
	if app.menu.phase != MenuPhaseSettings || app.settings.Draft != Committed || app.settings.status != "" || app.settings.error != "" {
		t.Fatalf("取消未恢复并留页: phase=%v state=%+v", app.menu.phase, app.settings)
	}
	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != MenuPhaseMenu {
		t.Fatalf("clean 返回 phase=%v，want menu", app.menu.phase)
	}
}

func TestSettingsEscapeUsesSameDirtyGuard(t *testing.T) {
	values := SettingsValues{AudioVolume: 0.7, WindowSize: config.WindowSize1280x720}
	app := newSettingsStateTestApplication(values)
	app.handleMenuEvent(menuActionSettings)
	app.settings.Draft.AudioVolume = 0.2

	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != MenuPhaseSettings || !app.settings.dirty() {
		t.Fatal("Escape/返回 action 不得丢弃 dirty 草稿")
	}
	app.settings.Draft = values
	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != MenuPhaseMenu {
		t.Fatal("clean Escape/返回 action 应回主菜单")
	}
}

func TestSettingsTypedChangeIsDefensiveAndPhaseScoped(t *testing.T) {
	values := SettingsValues{AudioVolume: 0.7, WindowSize: config.WindowSize1280x720}
	app := newSettingsStateTestApplication(values)
	valid := client.UIEvent{Kind: client.UIEventSettingsChanged, Settings: client.UISettingsValues{
		AudioVolume: 0.1, Window: client.UISettingsWindow640x360, TexturePackPath: "packs/local",
	}}

	_, disposition := app.handleMenuUIEvent(valid)
	if disposition != menuUIEventIgnored || app.settings.Draft != values {
		t.Fatalf("非设置相位接受了 change: disposition=%v Draft=%+v", disposition, app.settings.Draft)
	}
	app.handleMenuEvent(menuActionSettings)
	invalid := valid
	invalid.Settings.TexturePackPath = "bad\npath"
	_, disposition = app.handleMenuUIEvent(invalid)
	if disposition != menuUIEventIgnored || app.settings.Draft != values {
		t.Fatalf("非法 change 被接受: disposition=%v Draft=%+v", disposition, app.settings.Draft)
	}
	_, disposition = app.handleMenuUIEvent(client.UIEvent{Kind: client.UIEventKind(99), ActionID: menuActionSettingsSave})
	if disposition != menuUIEventIgnored || app.settings.Draft != values {
		t.Fatalf("未知 typed event 产生副作用: disposition=%v", disposition)
	}
}

func TestSettingsUISegmentReflectsDraftDirtyStatusAndError(t *testing.T) {
	app := newSettingsStateTestApplication(SettingsValues{
		AudioVolume: 0.25, TexturePackPath: "packs/local", WindowSize: config.WindowSize960x540,
	})
	app.handleMenuEvent(menuActionSettings)
	app.settings.Draft.AudioVolume = 0.5
	app.settings.status = "先保存或取消"
	app.settings.error = "校验失败"
	want := client.EncodeUISettings(client.UISettings{
		Visible: true, AudioVolume: 0.5, Window: client.UISettingsWindow960x540,
		TexturePackPath: "packs/local", Dirty: true, Status: "先保存或取消", Error: "校验失败",
	})
	if got := app.uiSegment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("settings layout v2 不符:\ngot=%v\nwant=%v", got, want)
	}
	app.menu.phase = MenuPhaseMenu
	if got := app.uiSegment(); len(got) == 0 || reflect.DeepEqual(got, want) {
		t.Fatal("主菜单应继续输出 layout v1")
	}
}

func TestSettingsUnsavedEditDoesNotCreateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	values := SettingsValues{AudioVolume: 0.7, WindowSize: config.WindowSize1280x720}
	app := newSettingsStateTestApplication(values)
	app.startupOptions.ConfigPath = path
	app.handleMenuEvent(menuActionSettings)
	app.settings.Draft.AudioVolume = 0.2
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("未保存编辑创建了配置: %v", err)
	}
}

func TestSettingsSaveDefaultCreatesConfigV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	defaults := config.Defaults()
	values := SettingsValues{
		AudioVolume: defaults.AudioVolume, WindowSize: defaults.WindowSize,
		TexturePackPath: defaults.TexturePackPath,
	}
	app := newSettingsStateTestApplication(values)
	app.startupOptions.ConfigPath = path
	app.startupDeps = defaultDependencies()

	if err := app.saveSettings(); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != config.CurrentVersion || loaded.AudioVolume != defaults.AudioVolume || loaded.WindowSize != defaults.WindowSize {
		t.Fatalf("保存的默认配置错误: %+v", loaded)
	}
}

func TestSettingsSaveProductionPatchRepairsTopLevelNull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}
	Committed := SettingsValues{
		AudioVolume: 0.7, TexturePackPath: "packs/old", WindowSize: config.WindowSize1280x720,
	}
	Draft := SettingsValues{
		AudioVolume: 0.25, TexturePackPath: "", WindowSize: config.WindowSize960x540,
	}
	app := newSettingsStateTestApplication(Committed)
	app.startupOptions.ConfigPath = path
	app.settings.Draft = Draft
	app.startupDeps = defaultDependencies()
	var events []string
	app.window = &settingsTestWindow{events: &events}
	app.closeAudio = func() { events = append(events, "close old audio") }
	app.startupDeps.NewAudioPlayer = func(volume float32) (func(audio.Cue), func()) {
		loaded, err := config.Load(path)
		if err != nil {
			t.Fatalf("音频应用前重载配置: %v", err)
		}
		if loaded.AudioVolume != Draft.AudioVolume || loaded.WindowSize != Draft.WindowSize {
			t.Fatalf("运行态先于磁盘提交: %+v", loaded)
		}
		events = append(events, fmt.Sprintf("audio %.2f", volume))
		return nil, nil
	}

	if err := app.saveSettings(); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != config.CurrentVersion || loaded.AudioVolume != Draft.AudioVolume ||
		loaded.TexturePackPath != Draft.TexturePackPath || loaded.WindowSize != Draft.WindowSize {
		t.Fatalf("保存后的 null 配置=%+v", loaded)
	}
	if app.settings.Committed != Draft || app.settings.dirty() {
		t.Fatalf("保存后的设置状态=%+v", app.settings)
	}
	wantEvents := []string{"audio 0.25", "close old audio", "window 960x540", "poll"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("null 保存副作用顺序=%v want=%v", events, wantEvents)
	}
}

func TestSettingsSavePreservesRawUnexposedConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := []byte(`{
  "version": 1,
  "audioVolume": 0.7,
  "texturePackPath": "packs/old",
  "windowSize": "1280x720",
  "futureTop": { "keep": [1, {"raw": true}] },
  "render": { "viewDistance": 999, "fovDegrees": 70, "mouseSensitivity": 1, "lodEnabled": true, "lodFarMultiplier": 3, "lodStep": 4, "futureNested": {"keep": "verbatim"} }
}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var before map[string]json.RawMessage
	if err := json.Unmarshal(body, &before); err != nil {
		t.Fatal(err)
	}

	app := newSettingsStateTestApplication(SettingsValues{
		AudioVolume: 0.7, TexturePackPath: "packs/old", WindowSize: config.WindowSize1280x720,
	})
	app.startupOptions.ConfigPath = path
	app.settings.Draft = SettingsValues{
		AudioVolume: 0.25, TexturePackPath: "packs/new", WindowSize: config.WindowSize960x540,
	}
	app.startupDeps = defaultDependencies()
	app.startupDeps.NewRegistry = func(string) (*assets.Registry, error) {
		return assets.NewDefaultRegistry(), nil
	}
	if err := app.saveSettings(); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}

	afterBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]json.RawMessage
	if err := json.Unmarshal(afterBody, &after); err != nil {
		t.Fatalf("保存结果不是合法 JSON: %v", err)
	}
	for _, key := range []string{"futureTop", "render"} {
		if !bytes.Equal(before[key], after[key]) {
			t.Fatalf("%s raw value changed:\nbefore=%s\nafter=%s", key, before[key], after[key])
		}
	}
	if got := string(after["audioVolume"]); got != "0.25" {
		t.Fatalf("AudioVolume=%s", got)
	}
	if got := string(after["texturePackPath"]); got != `"packs/new"` {
		t.Fatalf("TexturePackPath=%s", got)
	}
	if got := string(after["windowSize"]); got != `"960x540"` {
		t.Fatalf("WindowSize=%s", got)
	}
}

func TestSettingsSaveProductionPatchFailureKeepsDiskDraftAndRuntime(t *testing.T) {
	want := SettingsValues{AudioVolume: 0.7, WindowSize: config.WindowSize1280x720}
	for _, test := range []struct {
		name    string
		prepare func(*testing.T) (path string, verify func(*testing.T))
	}{
		{
			name: "damaged JSON",
			prepare: func(t *testing.T) (string, func(*testing.T)) {
				path := filepath.Join(t.TempDir(), "config.json")
				body := []byte(`{"version":1,"render":`)
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
				return path, func(t *testing.T) {
					got, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(got, body) {
						t.Fatalf("damaged config changed: body=%q err=%v", got, err)
					}
				}
			},
		},
		{
			name: "I/O failure",
			prepare: func(t *testing.T) (string, func(*testing.T)) {
				parent := filepath.Join(t.TempDir(), "not-a-directory")
				if err := os.WriteFile(parent, []byte("sentinel"), 0o600); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(parent, "config.json"), func(t *testing.T) {
					got, err := os.ReadFile(parent)
					if err != nil || string(got) != "sentinel" {
						t.Fatalf("I/O sentinel changed: body=%q err=%v", got, err)
					}
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, verify := test.prepare(t)
			app := newSettingsStateTestApplication(want)
			app.startupOptions.ConfigPath = path
			app.settings.Draft = SettingsValues{AudioVolume: 0.25, WindowSize: config.WindowSize640x360}
			window := &settingsTestWindow{}
			app.window = window
			audioCreates := 0
			app.startupDeps = defaultDependencies()
			app.startupDeps.NewAudioPlayer = func(float32) (func(audio.Cue), func()) {
				audioCreates++
				return nil, nil
			}
			if err := app.saveSettings(); err == nil {
				t.Fatal("saveSettings unexpectedly succeeded")
			}
			verify(t)
			if app.settings.Committed != want || !app.settings.dirty() || audioCreates != 0 || window.setCalls != 0 {
				t.Fatalf("failure changed state/runtime: state=%+v audio=%d window=%d",
					app.settings, audioCreates, window.setCalls)
			}
		})
	}
}

func TestSettingsSaveOrderPatchesLatestAndAppliesAfterDisk(t *testing.T) {
	var events []string
	Committed := SettingsValues{AudioVolume: 0.7, TexturePackPath: "packs/old", WindowSize: config.WindowSize1280x720}
	Draft := SettingsValues{AudioVolume: 0.25, TexturePackPath: "packs/new", WindowSize: config.WindowSize960x540}
	app := newSettingsStateTestApplication(Committed)
	configPath := filepath.Join(t.TempDir(), "config.json")
	app.startupOptions.ConfigPath = configPath
	app.settings.Draft = Draft
	window := &settingsTestWindow{
		events: &events, contentWidth: 1280, contentHeight: 720,
		framebufferWidth: 1280, framebufferHeight: 720,
	}
	app.window = window
	app.playCue = func(audio.Cue) {}
	oldPlay := app.playCue
	var newPlayed bool
	app.closeAudio = func() {
		if reflect.ValueOf(app.playCue).Pointer() == reflect.ValueOf(oldPlay).Pointer() {
			t.Fatal("旧播放器关闭前必须先安装新 play 闭包")
		}
		events = append(events, "close old audio")
	}

	var saved config.SettingsPatch
	app.startupDeps = Dependencies{
		NewRegistry: func(path string) (*assets.Registry, error) {
			events = append(events, "registry "+path)
			return assets.NewDefaultRegistry(), nil
		},
		PatchSettings: func(path string, patch config.SettingsPatch) (config.PersistenceResult, error) {
			events = append(events, "patch "+path)
			saved = patch
			return config.PersistenceResult{Committed: true}, nil
		},
		NewAudioPlayer: func(volume float32) (func(audio.Cue), func()) {
			events = append(events, fmt.Sprintf("audio %.2f", volume))
			return func(audio.Cue) { newPlayed = true }, func() { events = append(events, "close new audio") }
		},
	}

	if err := app.saveSettings(); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}
	wantCandidate, err := filepath.Abs(filepath.Join(filepath.Dir(configPath), Draft.TexturePackPath))
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"registry " + wantCandidate,
		"patch " + configPath,
		"audio 0.25",
		"close old audio",
		"window 960x540",
		"poll",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("保存顺序=%v\nwant=%v", events, wantEvents)
	}
	if saved.AudioVolume != Draft.AudioVolume || saved.TexturePackPath != Draft.TexturePackPath || saved.WindowSize != Draft.WindowSize {
		t.Fatalf("三项 patch 错误: %+v", saved)
	}
	if app.settings.Committed != Draft || app.settings.dirty() {
		t.Fatalf("成功后 Committed/Draft 错误: %+v", app.settings)
	}
	if app.settings.status == "" || !strings.Contains(app.settings.status, "下次启动") {
		t.Fatalf("材质生效提示=%q", app.settings.status)
	}
	app.playCue(1)
	if !newPlayed {
		t.Fatal("保存后 cue 未使用新播放器")
	}
	if window.setCalls != 1 || window.setWidth != 960 || window.setHeight != 540 || window.pollCalls != 1 {
		t.Fatalf("窗口应用 set=%d (%d,%d) poll=%d", window.setCalls, window.setWidth, window.setHeight, window.pollCalls)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	oldCloses, newCloses := 0, 0
	for _, event := range events {
		oldCloses += boolInt(event == "close old audio")
		newCloses += boolInt(event == "close new audio")
	}
	if oldCloses != 1 || newCloses != 1 {
		t.Fatalf("音频 close 次数 old/new=%d/%d，want 1/1；events=%v", oldCloses, newCloses, events)
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestSettingsCandidateValidationPathAndSkipRules(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.json")
	for _, test := range []struct {
		name      string
		Committed string
		Draft     string
		wantPath  string
		wantCalls int
	}{
		{name: "relative", Committed: "old", Draft: "packs/new", wantPath: filepath.Join(base, "packs/new"), wantCalls: 1},
		{name: "absolute", Committed: "old", Draft: filepath.Join(base, "absolute"), wantPath: filepath.Join(base, "absolute"), wantCalls: 1},
		{name: "empty", Committed: "old", Draft: "", wantCalls: 0},
		{name: "unchanged", Committed: "packs/same", Draft: "packs/same", wantCalls: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			values := SettingsValues{AudioVolume: 0.7, TexturePackPath: test.Committed, WindowSize: config.WindowSize1280x720}
			app := newSettingsStateTestApplication(values)
			app.settings.Draft.TexturePackPath = test.Draft
			app.startupOptions.ConfigPath = configPath
			app.startupDeps = Dependencies{
				NewRegistry: func(path string) (*assets.Registry, error) {
					calls++
					if path != test.wantPath {
						t.Fatalf("candidate path=%q want=%q", path, test.wantPath)
					}
					return assets.NewDefaultRegistry(), nil
				},
				PatchSettings: func(string, config.SettingsPatch) (config.PersistenceResult, error) {
					return config.PersistenceResult{Committed: true}, nil
				},
			}
			if err := app.saveSettings(); err != nil {
				t.Fatalf("saveSettings: %v", err)
			}
			if calls != test.wantCalls {
				t.Fatalf("candidate calls=%d want=%d", calls, test.wantCalls)
			}
		})
	}
}

func TestSettingsSaveFailuresHaveNoRuntimeSideEffectsAndKeepDraft(t *testing.T) {
	wantErr := errors.New(strings.Repeat("材质配置损坏🙂", 40))
	for _, stage := range []string{"candidate", "patch"} {
		t.Run(stage, func(t *testing.T) {
			Committed := SettingsValues{AudioVolume: 0.7, TexturePackPath: "old", WindowSize: config.WindowSize1280x720}
			Draft := SettingsValues{AudioVolume: 0.2, TexturePackPath: "new", WindowSize: config.WindowSize640x360}
			app := newSettingsStateTestApplication(Committed)
			app.handleMenuEvent(menuActionSettings)
			app.settings.Draft = Draft
			app.startupOptions.ConfigPath = filepath.Join(t.TempDir(), "config.json")
			window := &settingsTestWindow{}
			app.window = window
			audioCreates, audioCloses := 0, 0
			app.closeAudio = func() { audioCloses++ }
			app.startupDeps = Dependencies{
				NewRegistry: func(string) (*assets.Registry, error) {
					if stage == "candidate" {
						return nil, wantErr
					}
					return assets.NewDefaultRegistry(), nil
				},
				PatchSettings: func(string, config.SettingsPatch) (config.PersistenceResult, error) {
					if stage == "patch" {
						return config.PersistenceResult{}, wantErr
					}
					return config.PersistenceResult{Committed: true}, nil
				},
				NewAudioPlayer: func(float32) (func(audio.Cue), func()) {
					audioCreates++
					return nil, func() { audioCloses++ }
				},
			}

			app.handleMenuEvent(menuActionSettingsSave)
			if app.menu.phase != MenuPhaseSettings || app.settings.Committed != Committed || app.settings.Draft != Draft {
				t.Fatalf("失败修改状态: phase=%v state=%+v", app.menu.phase, app.settings)
			}
			if app.settings.error == "" || len(app.settings.error) > maxSettingsMessageBytes || !utf8.ValidString(app.settings.error) {
				t.Fatalf("UI error 非 UTF-8 有界值: len=%d value=%q", len(app.settings.error), app.settings.error)
			}
			if audioCreates != 0 || audioCloses != 0 || window.setCalls != 0 || window.pollCalls != 0 {
				t.Fatalf("失败产生运行时副作用 audio create/close=%d/%d window=%d/%d",
					audioCreates, audioCloses, window.setCalls, window.pollCalls)
			}
		})
	}
}

func TestSettingsSavePersistenceStagesMatchCommitResult(t *testing.T) {
	for _, test := range []struct {
		name      string
		Committed bool
	}{
		{name: "directory open"},
		{name: "rename"},
		{name: "directory sync", Committed: true},
		{name: "directory close", Committed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			Committed := SettingsValues{AudioVolume: 0.7, WindowSize: config.WindowSize1280x720}
			Draft := SettingsValues{AudioVolume: 0.25, WindowSize: config.WindowSize960x540}
			app := newSettingsStateTestApplication(Committed)
			app.handleMenuEvent(menuActionSettings)
			app.settings.Draft = Draft
			window := &settingsTestWindow{}
			app.window = window
			audioCreates := 0
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })
			wantErr := fmt.Errorf("%s failed", test.name)
			app.startupDeps = Dependencies{
				PatchSettings: func(string, config.SettingsPatch) (config.PersistenceResult, error) {
					return config.PersistenceResult{Committed: test.Committed}, wantErr
				},
				NewAudioPlayer: func(float32) (func(audio.Cue), func()) {
					audioCreates++
					return nil, nil
				},
			}

			app.handleMenuEvent(menuActionSettingsSave)
			if !strings.Contains(logs.String(), wantErr.Error()) {
				t.Fatalf("完整持久性错误未写日志: %s", logs.String())
			}
			if test.Committed {
				if app.settings.Committed != Draft || app.settings.dirty() {
					t.Fatalf("已提交警告未更新设置状态: %+v", app.settings)
				}
				if app.startupOptions.AudioVolume != Draft.AudioVolume || app.startupOptions.WindowSize != Draft.WindowSize {
					t.Fatalf("已提交警告未更新启动镜像: %+v", app.startupOptions)
				}
				if audioCreates != 1 || window.setCalls != 1 || window.pollCalls != 1 {
					t.Fatalf("已提交警告未应用运行态: audio=%d window=%d poll=%d", audioCreates, window.setCalls, window.pollCalls)
				}
				if app.settings.error != "" || !strings.Contains(app.settings.status, "已保存但持久性同步异常") {
					t.Fatalf("已提交警告 UI 文案错误: status=%q error=%q", app.settings.status, app.settings.error)
				}
				return
			}
			if app.settings.Committed != Committed || !app.settings.dirty() {
				t.Fatalf("提交前失败修改设置状态: %+v", app.settings)
			}
			if app.startupOptions.AudioVolume != Committed.AudioVolume || app.startupOptions.WindowSize != Committed.WindowSize {
				t.Fatalf("提交前失败修改启动镜像: %+v", app.startupOptions)
			}
			if audioCreates != 0 || window.setCalls != 0 || window.pollCalls != 0 {
				t.Fatalf("提交前失败产生运行态副作用: audio=%d window=%d poll=%d", audioCreates, window.setCalls, window.pollCalls)
			}
			if app.settings.error == "" || app.settings.status != "" {
				t.Fatalf("提交前失败 UI 文案错误: status=%q error=%q", app.settings.status, app.settings.error)
			}
		})
	}
}

func TestSettingsDraftValidationRejectsInvalidValues(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	for _, values := range []SettingsValues{
		{AudioVolume: -0.1, WindowSize: config.WindowSize1280x720},
		{AudioVolume: 1.1, WindowSize: config.WindowSize1280x720},
		{AudioVolume: float32(math.NaN()), WindowSize: config.WindowSize1280x720},
		{AudioVolume: 0.5, WindowSize: config.WindowSize("1920x1080")},
		{AudioVolume: 0.5, WindowSize: config.WindowSize1280x720, TexturePackPath: "a\nb"},
		{AudioVolume: 0.5, WindowSize: config.WindowSize1280x720, TexturePackPath: strings.Repeat("界", 342)},
		{AudioVolume: 0.5, WindowSize: config.WindowSize1280x720, TexturePackPath: invalidUTF8},
	} {
		if err := values.validate(); err == nil {
			t.Fatalf("validate 接受非法值: %+v", values)
		}
	}
}

func TestNewApplicationUsesConfiguredWindowPreset(t *testing.T) {
	renderer := requireMenuTestRenderer(t)
	t.Cleanup(func() { renderer.Close() })
	identity := connectionTestIdentity()
	var gotWidth, gotHeight int
	deps := newMenuWindowedTestDeps(t, renderer)
	deps.NewWindow = func(width, height int, _ string) (Window, error) {
		gotWidth, gotHeight = width, height
		return &settingsTestWindow{
			contentWidth: width, contentHeight: height,
			framebufferWidth: width, framebufferHeight: height,
		}, nil
	}
	app, err := NewWithDependencies(Options{
		Identity: &identity, Render: config.Defaults().Render, StartAtMenu: true,
		WindowSize: config.WindowSize640x360,
	}, deps)
	if err != nil {
		t.Fatalf("NewWithDependencies: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if gotWidth != 640 || gotHeight != 360 {
		t.Fatalf("NewWindow=(%d,%d)，want (640,360)", gotWidth, gotHeight)
	}
}

func TestNewApplicationRejectsInvalidSettingsBeforeResourceSideEffects(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*Options)
	}{
		{name: "LF path", configure: func(options *Options) { options.TexturePackPath = "pack\nname" }},
		{name: "CR path", configure: func(options *Options) { options.TexturePackPath = "pack\rname" }},
		{name: "oversized path", configure: func(options *Options) {
			options.TexturePackPath = strings.Repeat("a", config.MaxTexturePackPathBytes+1)
		}},
		{name: "non-finite audio", configure: func(options *Options) {
			options.AudioVolume = float32(math.NaN())
		}},
		{name: "unknown window", configure: func(options *Options) {
			options.WindowSize = config.WindowSize("1920x1080")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := Options{
				Render: config.Defaults().Render, StartAtMenu: true,
				WindowSize: config.WindowSize1280x720,
			}
			test.configure(&options)
			resourceCalls := 0
			_, err := NewWithDependencies(options, Dependencies{
				NewRegistry: func(string) (*assets.Registry, error) {
					resourceCalls++
					return nil, errors.New("不应访问 registry")
				},
				NewAudioPlayer: func(float32) (func(audio.Cue), func()) {
					resourceCalls++
					return nil, nil
				},
				NewWindow: func(int, int, string) (Window, error) {
					resourceCalls++
					return nil, errors.New("不应创建窗口")
				},
			})
			if err == nil || !strings.Contains(err.Error(), "设置") {
				t.Fatalf("NewWithDependencies error=%v，want 设置校验错误", err)
			}
			if resourceCalls != 0 {
				t.Fatalf("非法设置触发资源副作用 %d 次", resourceCalls)
			}
		})
	}
}

func TestLoadedConfigEntersSettingsAndEncodesLayoutV2(t *testing.T) {
	configPath := writeSettingsApplicationConfig(t, `{
		"version": 1,
		"audioVolume": 0.25,
		"windowSize": "960x540"
	}`)
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	renderer := requireMenuTestRenderer(t)
	t.Cleanup(func() { renderer.Close() })
	identity := connectionTestIdentity()
	deps := newMenuWindowedTestDeps(t, renderer)
	app, err := NewWithDependencies(Options{
		Identity: &identity, Render: loaded.Render, StartAtMenu: true,
		ConfigPath: configPath, AudioVolume: loaded.AudioVolume,
		TexturePackPath: loaded.TexturePackPath, ResolvedTexturePackPath: loaded.ResolvedTexturePackPath,
		WindowSize: loaded.WindowSize,
	}, deps)
	if err != nil {
		t.Fatalf("NewWithDependencies: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	app.handleMenuEvent(menuActionSettings)
	want := client.EncodeUISettings(client.UISettings{
		Visible: true, AudioVolume: 0.25, Window: client.UISettingsWindow960x540,
	})
	if got := app.uiSegment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("正常配置 settings layout v2 不符:\ngot=%v\nwant=%v", got, want)
	}
}

func writeSettingsApplicationConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写配置: %v", err)
	}
	return path
}

func TestSettingsRuntimeWindowResizeFitsPhysicalLimitAndKeeps16By9(t *testing.T) {
	window := &settingsTestWindow{
		contentWidth: 960, contentHeight: 540,
		framebufferWidth: 2880, framebufferHeight: 1620,
		framebufferScale: 3,
	}
	app := &Application{window: window}
	app.applyWindowSize(config.WindowSize1280x720)
	if window.setCalls != 2 {
		t.Fatalf("高 DPI resize setCalls=%d，want preset request + fit request", window.setCalls)
	}
	width, height := window.ContentSize()
	frameWidth, frameHeight := window.FramebufferSize()
	if width*9 != height*16 {
		t.Fatalf("最终逻辑尺寸 %dx%d 不是 16:9", width, height)
	}
	if frameWidth > interactiveFramebufferWidth || frameHeight > interactiveFramebufferHeight {
		t.Fatalf("最终物理尺寸 %dx%d 超过 %dx%d", frameWidth, frameHeight,
			interactiveFramebufferWidth, interactiveFramebufferHeight)
	}
}
