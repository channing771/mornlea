//go:build darwin

package main

// app_settings_test.go：设置页的 Go 草稿状态机、保存事务与运行时资源替换测试。

import (
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
	"github.com/channing771/mornlea/internal/logging"
	"github.com/channing771/mornlea/internal/network"
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

func newSettingsStateTestApplication(values settingsValues) *application {
	app := &application{
		menu: menuState{phase: menuPhaseMenu, title: "Mornlea", version: "dev"},
		settings: settingsState{
			committed: values,
			draft:     values,
		},
		startupOptions: applicationOptions{
			ConfigPath:      "unused.json",
			AudioVolume:     values.audioVolume,
			TexturePackPath: values.texturePackPath,
			WindowSize:      values.windowSize,
		},
		itemDrops: client.NewItemDrops(),
	}
	app.releaseResources = app.releaseOwnedResources
	return app
}

func TestSettingsNavigationDraftCancelAndBack(t *testing.T) {
	committed := settingsValues{
		audioVolume: 0.25, texturePackPath: "packs/local", windowSize: config.WindowSize960x540,
	}
	app := newSettingsStateTestApplication(committed)

	if quit := app.handleMenuEvent(menuActionSettings); quit {
		t.Fatal("进入设置不应退出")
	}
	if app.menu.phase != menuPhaseSettings || app.settings.dirty() {
		t.Fatalf("进入设置 phase=%v dirty=%v", app.menu.phase, app.settings.dirty())
	}
	if app.settings.draft != committed || app.settings.committed != committed {
		t.Fatalf("设置初始化错误: %+v", app.settings)
	}

	changed := client.UISettingsValues{
		AudioVolume: 0.5, TexturePackPath: "packs/next", Window: client.UISettingsWindow640x360,
	}
	quit, disposition := app.handleMenuUIEvent(client.UIEvent{Kind: client.UIEventSettingsChanged, Settings: changed})
	if quit || disposition != menuUIEventHandled {
		t.Fatalf("typed change quit=%v disposition=%v", quit, disposition)
	}
	wantDraft := settingsValues{audioVolume: 0.5, texturePackPath: "packs/next", windowSize: config.WindowSize640x360}
	if app.settings.draft != wantDraft || !app.settings.dirty() {
		t.Fatalf("draft=%+v dirty=%v，want %+v/true", app.settings.draft, app.settings.dirty(), wantDraft)
	}

	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != menuPhaseSettings || app.settings.draft != wantDraft || app.settings.status == "" {
		t.Fatalf("dirty 返回丢失草稿或未提示: phase=%v state=%+v", app.menu.phase, app.settings)
	}
	app.handleMenuEvent(menuActionSettingsCancel)
	if app.menu.phase != menuPhaseSettings || app.settings.draft != committed || app.settings.status != "" || app.settings.error != "" {
		t.Fatalf("取消未恢复并留页: phase=%v state=%+v", app.menu.phase, app.settings)
	}
	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != menuPhaseMenu {
		t.Fatalf("clean 返回 phase=%v，want menu", app.menu.phase)
	}
}

func TestSettingsEscapeUsesSameDirtyGuard(t *testing.T) {
	values := settingsValues{audioVolume: 0.7, windowSize: config.WindowSize1280x720}
	app := newSettingsStateTestApplication(values)
	app.handleMenuEvent(menuActionSettings)
	app.settings.draft.audioVolume = 0.2

	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != menuPhaseSettings || !app.settings.dirty() {
		t.Fatal("Escape/返回 action 不得丢弃 dirty 草稿")
	}
	app.settings.draft = values
	app.handleMenuEvent(menuActionSettingsBack)
	if app.menu.phase != menuPhaseMenu {
		t.Fatal("clean Escape/返回 action 应回主菜单")
	}
}

func TestSettingsTypedChangeIsDefensiveAndPhaseScoped(t *testing.T) {
	values := settingsValues{audioVolume: 0.7, windowSize: config.WindowSize1280x720}
	app := newSettingsStateTestApplication(values)
	valid := client.UIEvent{Kind: client.UIEventSettingsChanged, Settings: client.UISettingsValues{
		AudioVolume: 0.1, Window: client.UISettingsWindow640x360, TexturePackPath: "packs/local",
	}}

	_, disposition := app.handleMenuUIEvent(valid)
	if disposition != menuUIEventIgnored || app.settings.draft != values {
		t.Fatalf("非设置相位接受了 change: disposition=%v draft=%+v", disposition, app.settings.draft)
	}
	app.handleMenuEvent(menuActionSettings)
	invalid := valid
	invalid.Settings.TexturePackPath = "bad\npath"
	_, disposition = app.handleMenuUIEvent(invalid)
	if disposition != menuUIEventIgnored || app.settings.draft != values {
		t.Fatalf("非法 change 被接受: disposition=%v draft=%+v", disposition, app.settings.draft)
	}
	_, disposition = app.handleMenuUIEvent(client.UIEvent{Kind: client.UIEventKind(99), ActionID: menuActionSettingsSave})
	if disposition != menuUIEventIgnored || app.settings.draft != values {
		t.Fatalf("未知 typed event 产生副作用: disposition=%v", disposition)
	}
}

func TestSettingsUISegmentReflectsDraftDirtyStatusAndError(t *testing.T) {
	app := newSettingsStateTestApplication(settingsValues{
		audioVolume: 0.25, texturePackPath: "packs/local", windowSize: config.WindowSize960x540,
	})
	app.handleMenuEvent(menuActionSettings)
	app.settings.draft.audioVolume = 0.5
	app.settings.status = "先保存或取消"
	app.settings.error = "校验失败"
	want := client.EncodeUISettings(client.UISettings{
		Visible: true, AudioVolume: 0.5, Window: client.UISettingsWindow960x540,
		TexturePackPath: "packs/local", Dirty: true, Status: "先保存或取消", Error: "校验失败",
	})
	if got := app.uiSegment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("settings layout v2 不符:\ngot=%v\nwant=%v", got, want)
	}
	app.menu.phase = menuPhaseMenu
	if got := app.uiSegment(); len(got) == 0 || reflect.DeepEqual(got, want) {
		t.Fatal("主菜单应继续输出 layout v1")
	}
}

func TestSettingsUnsavedEditDoesNotCreateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	values := settingsValues{audioVolume: 0.7, windowSize: config.WindowSize1280x720}
	app := newSettingsStateTestApplication(values)
	app.startupOptions.ConfigPath = path
	app.handleMenuEvent(menuActionSettings)
	app.settings.draft.audioVolume = 0.2
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("未保存编辑创建了配置: %v", err)
	}
}

func TestSettingsSaveDefaultCreatesConfigV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	defaults := config.Defaults()
	values := settingsValues{
		audioVolume: defaults.AudioVolume, windowSize: defaults.WindowSize,
		texturePackPath: defaults.TexturePackPath,
	}
	app := newSettingsStateTestApplication(values)
	app.startupOptions.ConfigPath = path
	app.startupDeps = defaultApplicationDependencies()

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

func TestSettingsSaveOrderPatchesLatestAndAppliesAfterDisk(t *testing.T) {
	var events []string
	committed := settingsValues{audioVolume: 0.7, texturePackPath: "packs/old", windowSize: config.WindowSize1280x720}
	draft := settingsValues{audioVolume: 0.25, texturePackPath: "packs/new", windowSize: config.WindowSize960x540}
	app := newSettingsStateTestApplication(committed)
	configPath := filepath.Join(t.TempDir(), "config.json")
	app.startupOptions.ConfigPath = configPath
	app.settings.draft = draft
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

	latest := config.Defaults()
	latest.Render.ViewDistance = 19
	latest.FluidEnabled = false
	latest.Logging = logging.Config{Default: slog.LevelWarn, Modules: map[string]slog.Level{"sim": slog.LevelError}}
	var saved config.Config
	app.startupDeps = applicationDependencies{
		newRegistry: func(path string) (*assets.Registry, error) {
			events = append(events, "registry "+path)
			return assets.NewDefaultRegistry(), nil
		},
		loadConfig: func(path string) (config.Config, error) {
			events = append(events, "load "+path)
			return latest, nil
		},
		saveConfig: func(cfg config.Config, path string) error {
			events = append(events, "save "+path)
			saved = cfg
			return nil
		},
		newAudioPlayer: func(volume float32) (func(audio.Cue), func()) {
			events = append(events, fmt.Sprintf("audio %.2f", volume))
			return func(audio.Cue) { newPlayed = true }, func() { events = append(events, "close new audio") }
		},
	}

	if err := app.saveSettings(); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}
	wantCandidate, err := filepath.Abs(filepath.Join(filepath.Dir(configPath), draft.texturePackPath))
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"registry " + wantCandidate,
		"load " + configPath,
		"save " + configPath,
		"audio 0.25",
		"close old audio",
		"window 960x540",
		"poll",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("保存顺序=%v\nwant=%v", events, wantEvents)
	}
	if saved.AudioVolume != draft.audioVolume || saved.TexturePackPath != draft.texturePackPath || saved.WindowSize != draft.windowSize {
		t.Fatalf("三项 patch 错误: %+v", saved)
	}
	if saved.Render.ViewDistance != latest.Render.ViewDistance || saved.FluidEnabled != latest.FluidEnabled || !reflect.DeepEqual(saved.Logging, latest.Logging) {
		t.Fatalf("保存覆盖了最新磁盘其他字段: saved=%+v latest=%+v", saved, latest)
	}
	if app.settings.committed != draft || app.settings.dirty() {
		t.Fatalf("成功后 committed/draft 错误: %+v", app.settings)
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
		committed string
		draft     string
		wantPath  string
		wantCalls int
	}{
		{name: "relative", committed: "old", draft: "packs/new", wantPath: filepath.Join(base, "packs/new"), wantCalls: 1},
		{name: "absolute", committed: "old", draft: filepath.Join(base, "absolute"), wantPath: filepath.Join(base, "absolute"), wantCalls: 1},
		{name: "empty", committed: "old", draft: "", wantCalls: 0},
		{name: "unchanged", committed: "packs/same", draft: "packs/same", wantCalls: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			values := settingsValues{audioVolume: 0.7, texturePackPath: test.committed, windowSize: config.WindowSize1280x720}
			app := newSettingsStateTestApplication(values)
			app.settings.draft.texturePackPath = test.draft
			app.startupOptions.ConfigPath = configPath
			app.startupDeps = applicationDependencies{
				newRegistry: func(path string) (*assets.Registry, error) {
					calls++
					if path != test.wantPath {
						t.Fatalf("candidate path=%q want=%q", path, test.wantPath)
					}
					return assets.NewDefaultRegistry(), nil
				},
				loadConfig: func(string) (config.Config, error) { return config.Defaults(), nil },
				saveConfig: func(config.Config, string) error { return nil },
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
	for _, stage := range []string{"candidate", "load", "save"} {
		t.Run(stage, func(t *testing.T) {
			committed := settingsValues{audioVolume: 0.7, texturePackPath: "old", windowSize: config.WindowSize1280x720}
			draft := settingsValues{audioVolume: 0.2, texturePackPath: "new", windowSize: config.WindowSize640x360}
			app := newSettingsStateTestApplication(committed)
			app.handleMenuEvent(menuActionSettings)
			app.settings.draft = draft
			app.startupOptions.ConfigPath = filepath.Join(t.TempDir(), "config.json")
			window := &settingsTestWindow{}
			app.window = window
			audioCreates, audioCloses := 0, 0
			app.closeAudio = func() { audioCloses++ }
			app.startupDeps = applicationDependencies{
				newRegistry: func(string) (*assets.Registry, error) {
					if stage == "candidate" {
						return nil, wantErr
					}
					return assets.NewDefaultRegistry(), nil
				},
				loadConfig: func(string) (config.Config, error) {
					if stage == "load" {
						return config.Config{}, wantErr
					}
					return config.Defaults(), nil
				},
				saveConfig: func(config.Config, string) error {
					if stage == "save" {
						return wantErr
					}
					return nil
				},
				newAudioPlayer: func(float32) (func(audio.Cue), func()) {
					audioCreates++
					return nil, func() { audioCloses++ }
				},
			}

			app.handleMenuEvent(menuActionSettingsSave)
			if app.menu.phase != menuPhaseSettings || app.settings.committed != committed || app.settings.draft != draft {
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

func TestSettingsDraftValidationRejectsInvalidValues(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	for _, values := range []settingsValues{
		{audioVolume: -0.1, windowSize: config.WindowSize1280x720},
		{audioVolume: 1.1, windowSize: config.WindowSize1280x720},
		{audioVolume: float32(math.NaN()), windowSize: config.WindowSize1280x720},
		{audioVolume: 0.5, windowSize: config.WindowSize("1920x1080")},
		{audioVolume: 0.5, windowSize: config.WindowSize1280x720, texturePackPath: "a\nb"},
		{audioVolume: 0.5, windowSize: config.WindowSize1280x720, texturePackPath: strings.Repeat("界", 342)},
		{audioVolume: 0.5, windowSize: config.WindowSize1280x720, texturePackPath: invalidUTF8},
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
	deps.newWindow = func(width, height int, _ string) (applicationWindow, error) {
		gotWidth, gotHeight = width, height
		return &settingsTestWindow{
			contentWidth: width, contentHeight: height,
			framebufferWidth: width, framebufferHeight: height,
		}, nil
	}
	app, err := newApplicationWithDependencies(applicationOptions{
		Identity: &identity, Render: config.Defaults().Render, StartAtMenu: true,
		WindowSize: config.WindowSize640x360,
	}, deps)
	if err != nil {
		t.Fatalf("newApplicationWithDependencies: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if gotWidth != 640 || gotHeight != 360 {
		t.Fatalf("newWindow=(%d,%d)，want (640,360)", gotWidth, gotHeight)
	}
}

func TestSettingsRuntimeWindowResizeFitsPhysicalLimitAndKeeps16By9(t *testing.T) {
	window := &settingsTestWindow{
		contentWidth: 960, contentHeight: 540,
		framebufferWidth: 2880, framebufferHeight: 1620,
		framebufferScale: 3,
	}
	app := &application{window: window}
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

func TestRunPassesRawResolvedAndWindowSettingsWithAutomationIsolation(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.json")
	cfg := config.Defaults()
	cfg.TexturePackPath = "packs/local"
	cfg.WindowSize = config.WindowSize960x540
	cfg.AudioVolume = 0.25
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		args            []string
		wantRaw         string
		wantResolved    string
		wantWindow      config.WindowSize
		wantStartAtMenu bool
	}{
		{name: "local", args: []string{"--config", configPath}, wantRaw: "packs/local", wantResolved: filepath.Join(configDir, "packs/local"), wantWindow: config.WindowSize960x540, wantStartAtMenu: true},
		{name: "connect", args: []string{"--config", configPath, "--connect", "127.0.0.1:25565"}, wantRaw: "packs/local", wantResolved: filepath.Join(configDir, "packs/local"), wantWindow: config.WindowSize960x540},
		{name: "benchmark", args: []string{"--config", configPath, "--benchmark", "--perf-output", filepath.Join(t.TempDir(), "perf.json")}, wantWindow: config.WindowSize1280x720},
		{name: "capture", args: []string{"--config", configPath, "--capture", t.TempDir()}, wantWindow: config.WindowSize1280x720},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got applicationOptions
			err := runWithDependencies(test.args, runDependencies{
				loadIdentity: func(*string) (network.Identity, error) { return network.Identity{}, nil },
				newApplication: func(options applicationOptions) (*application, error) {
					got = options
					return nil, errors.New("stop before window")
				},
			})
			if err == nil {
				t.Fatal("want injected construction error")
			}
			if got.TexturePackPath != test.wantRaw || got.ResolvedTexturePackPath != test.wantResolved || got.WindowSize != test.wantWindow || got.StartAtMenu != test.wantStartAtMenu {
				t.Fatalf("settings options=%+v", got)
			}
		})
	}
}
