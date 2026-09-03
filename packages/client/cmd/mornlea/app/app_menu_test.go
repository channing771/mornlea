//go:build darwin

package app

// app_menu_test.go：主菜单状态机与延迟世界装配的测试。复用 app_test_helpers_test.go
// 的假窗口与 app_connection_test.go 的反依赖注入，验证 StartAtMenu 构造停留菜单、
// 「进入游戏」延迟装配、装配失败可退出、starting 防重入、菜单相位输入不生效，以及
// 真实菜单内容的桥下行状态组装锁。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/network"
)

// newMenuWindowedTestDeps 构造 StartAtMenu 窗口化路径的依赖注入载体：NewWindow/
// NewWindowedRenderer 服务窗口与渲染器创建，OpenStore/NewHost/... 断言在构造阶段
// 不被调用（StartAtMenu 延迟装配），供后续 startWorld 使用。
func newMenuWindowedTestDeps(t *testing.T, renderer *client.Renderer) Dependencies {
	t.Helper()
	unexpected := func(name string) {
		t.Helper()
		t.Fatalf("StartAtMenu 构造不应调用 %s（世界装配应延迟）", name)
	}
	return Dependencies{
		OpenStore: func(context.Context, Options) (storage.WorldStore, error) {
			unexpected("OpenStore")
			return nil, nil
		},
		DialTCP: func(context.Context, string) (network.ClientPacketStream, error) {
			unexpected("DialTCP")
			return nil, nil
		},
		LoginClient: func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
			unexpected("LoginClient")
			return nil, 0, nil
		},
		NewHost: func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error) {
			unexpected("NewHost")
			return nil, nil
		},
		NewMemoryStreamPair: func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
			unexpected("NewMemoryStreamPair")
			return nil, nil, nil
		},
		NewWindow: func(int, int, string) (Window, error) {
			return &fakeInteractiveWindow{}, nil
		},
		NewWindowedRenderer: func(Window) (*client.Renderer, error) {
			return renderer, nil
		},
	}
}

// requireMenuTestRenderer 创建一个离屏渲染器用于窗口化 StartAtMenu 测试；无 GPU 时跳过。
func requireMenuTestRenderer(t *testing.T) *client.Renderer {
	t.Helper()
	renderer, err := client.NewRenderer(64, 64)
	if errors.Is(err, client.ErrNoGPUAdapter) {
		t.Skip("无 GPU 适配器")
	}
	if err != nil {
		t.Fatalf("创建离屏渲染器: %v", err)
	}
	return renderer
}

// TestStartAtMenuConstructsMenuWithoutWorld 验证 StartAtMenu 构造后停留在菜单且世界未装配：
// server/host 为 nil、相位为菜单、标题/版本/按钮正确，并断言装配依赖在构造阶段零调用。
func TestStartAtMenuConstructsMenuWithoutWorld(t *testing.T) {
	renderer := requireMenuTestRenderer(t)
	t.Cleanup(func() { renderer.Close() })

	identity := connectionTestIdentity()
	options := Options{
		Seed:        42,
		WorldPath:   t.TempDir(),
		Identity:    &identity,
		Render:      config.Defaults().Render,
		StartAtMenu: true,
	}
	app, err := NewWithDependencies(options, newMenuWindowedTestDeps(t, renderer))
	if err != nil {
		t.Fatalf("NewWithDependencies: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	if app.server != nil {
		t.Fatalf("StartAtMenu 构造不应启动本地权威服务端，server=%v", app.server)
	}
	if app.host != nil {
		t.Fatalf("StartAtMenu 构造不应创建 Host，host=%v", app.host)
	}
	if app.menu.phase != MenuPhaseMenu {
		t.Fatalf("menu.phase = %v，want menu", app.menu.phase)
	}
	if app.menu.starting {
		t.Fatal("构造后 starting 应为 false")
	}
	if app.menu.title != "Mornlea" {
		t.Fatalf("menu.title = %q（期望 Mornlea）", app.menu.title)
	}
	if app.menu.version != menuVersion() {
		t.Fatalf("menu.version = %q，want %q", app.menu.version, menuVersion())
	}
	if app.menu.version == "" {
		t.Fatal("menu.version 不应为空（dev 或构建信息版本）")
	}

	state := app.buildUIState()
	if state.Phase != "menu" {
		t.Fatalf("桥相位 = %q，want menu", state.Phase)
	}
	if state.Menu == nil {
		t.Fatal("菜单相位应携带 menu 分节")
	}
	if state.Menu.Title != "Mornlea" || state.Menu.Version != app.menu.version || state.Menu.Error != "" {
		t.Fatalf("menu 标题/版本/错误行不符: %+v", state.Menu)
	}
	wantButtons := []UIMenuButton{
		{ID: menuActionStart, Label: "进入游戏", Enabled: true},
		{ID: menuActionMultiplayer, Label: "多人游戏", Enabled: false},
		{ID: menuActionSettings, Label: "设置", Enabled: true},
		{ID: menuActionQuit, Label: "退出游戏", Enabled: true},
	}
	if !reflect.DeepEqual(state.Menu.Buttons, wantButtons) {
		t.Fatalf("菜单按钮不符: got=%+v want=%+v", state.Menu.Buttons, wantButtons)
	}
}

// TestHandleMenuUIEventRoutesAction 锁定 client ABI v9 action 仍路由到既有
// `handleMenuEvent`，而不是把 typed event 本身当按钮 id。
func TestHandleMenuUIEventRoutesAction(t *testing.T) {
	app := &Application{menu: menuState{phase: MenuPhaseMenu}}
	quit, disposition := app.handleMenuUIEvent(client.UIEvent{
		Kind:     client.UIEventAction,
		ActionID: menuActionQuit,
	})
	if !quit || disposition != menuUIEventHandled {
		t.Fatalf("quit=%v disposition=%v", quit, disposition)
	}
}

// TestHandleMenuUIEventIgnoresNonActionOutsideSettings 锁定错相位 settings change
// 与未知 kind 都被忽略；即使伪造退出 `ActionID` 也不得误路由或改变菜单状态。
func TestHandleMenuUIEventIgnoresNonActionOutsideSettings(t *testing.T) {
	app := &Application{menu: menuState{phase: MenuPhaseMenu, error: "保留"}}
	quit, disposition := app.handleMenuUIEvent(client.UIEvent{
		Kind:  client.UIEventSettingsChanged,
		Field: client.UISettingsFieldAudioVolume,
		Value: json.RawMessage("0.25"),
	})
	if quit || disposition != menuUIEventIgnored {
		t.Fatalf("settings quit=%v disposition=%v", quit, disposition)
	}
	if app.menu.phase != MenuPhaseMenu || app.menu.error != "保留" {
		t.Fatalf("settings change 修改了菜单: %+v", app.menu)
	}

	quit, disposition = app.handleMenuUIEvent(client.UIEvent{Kind: client.UIEventKind(99), ActionID: menuActionQuit})
	if quit || disposition != menuUIEventIgnored {
		t.Fatalf("unknown quit=%v disposition=%v", quit, disposition)
	}
}

// startWorldSuccessDeps 为 startWorld 提供可成功的连接依赖：内存 store、Host、内存流对与登录。
func startWorldSuccessDeps(t *testing.T) Dependencies {
	t.Helper()
	return Dependencies{
		OpenStore: func(context.Context, Options) (storage.WorldStore, error) {
			return NewConnectionTestStore(42), nil
		},
		NewHost: func(_ context.Context, _ server.Config, _ server.Generator, store storage.WorldStore) (Host, error) {
			return newConnectionTestHost(store), nil
		},
		NewMemoryStreamPair: func(capacity int) (network.ClientPacketStream, network.ServerPacketStream, error) {
			clientStream, serverStream := network.NewMemoryStreamPair(capacity)
			return clientStream, serverStream, nil
		},
		LoginClient: func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
			clientEndpoint, serverEndpoint := network.NewMemoryPair(1)
			t.Cleanup(func() { _ = serverEndpoint.Close() })
			return clientEndpoint, 42, nil
		},
	}
}

// newStartWorldTestApp 构造一个菜单相位、可直接调用 startWorld 的 Application（关闭 LOD 使
// attachLodScheduler 零参与，因此无需真实渲染器），并正确配置 Close 所需的资源字段。
func newStartWorldTestApp(t *testing.T, deps Dependencies) *Application {
	t.Helper()
	identity := connectionTestIdentity()
	render := config.Defaults().Render
	render.LodEnabled = false
	ticks, _ := newPerformanceRecorders(false)
	app := &Application{
		menu: menuState{phase: MenuPhaseMenu, title: "Mornlea", version: menuVersion()},
		startupOptions: Options{
			Seed: 42, WorldPath: "unused", Identity: &identity,
			Render: render, StartAtMenu: true,
		},
		startupDeps:   deps,
		ticks:         ticks,
		render:        render,
		window:        &fakeInteractiveWindow{},
		remotePlayers: client.NewRemotePlayers(),
		companions:    &client.Companions{},
		chatEvents:    &client.ChatEvents{},
		itemDrops:     client.NewItemDrops(),
	}
	app.releaseResources = app.releaseOwnedResources
	return app
}

// TestHandleMenuEventStartAssemblesWorld 验证「进入游戏」点击：startWorld 被调用、
// 成功置相位 loading（加载收敛后才进入游戏相位）、starting 复位且光标保持未捕获
// （捕获时机已迁移到加载收敛点）。
func TestHandleMenuEventStartAssemblesWorld(t *testing.T) {
	app := newStartWorldTestApp(t, startWorldSuccessDeps(t))
	t.Cleanup(func() { _ = app.Close() })

	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("进入游戏不应请求退出")
	}
	if app.menu.phase != MenuPhaseLoading {
		t.Fatalf("装配成功后 phase = %v，want loading", app.menu.phase)
	}
	if app.menu.starting {
		t.Fatal("装配成功后 starting 应为 false")
	}
	if app.menu.error != "" {
		t.Fatalf("装配成功不应有错误行: %q", app.menu.error)
	}
	if app.window.CursorCaptured() {
		t.Fatal("装配成功停留在加载相位，不应捕获光标（捕获迁移到加载收敛点）")
	}
	if app.host == nil {
		t.Fatal("装配成功应设置 Host")
	}
	if app.receiver == nil {
		t.Fatal("装配成功应设置 receiver")
	}
}

// TestHandleMenuEventStartFailureKeepsMenu 验证装配失败：相位保持菜单、错误行写入、
// starting 复位且「退出游戏」仍可用。
func TestHandleMenuEventStartFailureKeepsMenu(t *testing.T) {
	wantErr := errors.New("存档不可打开")
	deps := startWorldSuccessDeps(t)
	deps.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		return nil, wantErr
	}
	app := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = app.Close() })

	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("装配失败不应请求退出")
	}
	if app.menu.phase != MenuPhaseMenu {
		t.Fatalf("装配失败后 phase = %v，want menu", app.menu.phase)
	}
	if app.menu.starting {
		t.Fatal("装配失败后 starting 应为 false")
	}
	if app.menu.error == "" {
		t.Fatal("装配失败应写入错误行")
	}
	if quit := app.handleMenuEvent(menuActionQuit); !quit {
		t.Fatal("装配失败后「退出游戏」应仍可请求退出")
	}
}

// TestHandleMenuEventStartIgnoredWhileStarting 验证 starting 防重入：装配进行中（starting=true）
// 重复点击「进入游戏」被忽略，不再次调用 startWorld（OpenStore 计数保持 0）。
func TestHandleMenuEventStartIgnoredWhileStarting(t *testing.T) {
	var openStoreCalls atomic.Int32
	deps := startWorldSuccessDeps(t)
	deps.OpenStore = func(ctx context.Context, options Options) (storage.WorldStore, error) {
		openStoreCalls.Add(1)
		return NewConnectionTestStore(42), nil
	}
	app := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = app.Close() })

	// 模拟装配已在进行中：starting=true 时重复点击应被守卫拦下。
	app.menu.starting = true
	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("重复点击不应请求退出")
	}
	if got := openStoreCalls.Load(); got != 0 {
		t.Fatalf("starting 期间重复点击不应再次装配，OpenStore 调用 = %d", got)
	}
	if app.menu.phase != MenuPhaseMenu {
		t.Fatalf("被守卫拦下后相位不应改变，got %v", app.menu.phase)
	}

	// 守卫只拦「装配中」，正常点击仍装配一次。
	app.menu.starting = false
	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("正常进入游戏不应请求退出")
	}
	if got := openStoreCalls.Load(); got != 1 {
		t.Fatalf("正常进入游戏应恰好装配一次，OpenStore 调用 = %d", got)
	}
	if app.menu.phase != MenuPhaseLoading {
		t.Fatalf("正常装配后 phase = %v，want loading", app.menu.phase)
	}
}

// TestHandleMenuEventLoadingPhaseIgnoresAllActions 锁定加载相位的防御档：加载屏
// 没有合法上行动作，任何动作 id（含 Enter 默认按钮路径的「进入游戏」）都不得
// 重新装配世界、改变相位或请求退出。
func TestHandleMenuEventLoadingPhaseIgnoresAllActions(t *testing.T) {
	var openStoreCalls atomic.Int32
	deps := startWorldSuccessDeps(t)
	deps.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		openStoreCalls.Add(1)
		return NewConnectionTestStore(42), nil
	}
	app := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = app.Close() })
	app.menu.phase = MenuPhaseLoading

	for _, id := range []string{menuActionStart, menuActionQuit, menuActionSettings, menuActionPauseBack, "unknown-action"} {
		if quit := app.handleMenuEvent(id); quit {
			t.Fatalf("加载相位动作 %q 不应请求退出", id)
		}
	}
	if app.menu.phase != MenuPhaseLoading {
		t.Fatalf("加载相位收到动作不得改变相位，got %v", app.menu.phase)
	}
	if got := openStoreCalls.Load(); got != 0 {
		t.Fatalf("加载相位不得重新装配世界，OpenStore 调用 = %d", got)
	}
}

// TestHandleMenuEventQuitRequestsClose 验证「退出游戏」返回退出信号，且不改变菜单状态。
func TestHandleMenuEventQuitRequestsClose(t *testing.T) {
	app := newStartWorldTestApp(t, startWorldSuccessDeps(t))
	if quit := app.handleMenuEvent(menuActionQuit); !quit {
		t.Fatal("退出游戏应返回退出信号")
	}
	if app.menu.phase != MenuPhaseMenu {
		t.Fatalf("菜单期退出不应改变相位，got %v", app.menu.phase)
	}
}

// TestHandleMenuEventUnknownIDIgnored 验证未知/禁用按钮 id 被忽略，不改变菜单状态。
func TestHandleMenuEventUnknownIDIgnored(t *testing.T) {
	app := newStartWorldTestApp(t, startWorldSuccessDeps(t))
	for _, id := range []string{menuActionMultiplayer, "unknown-action"} {
		if quit := app.handleMenuEvent(id); quit {
			t.Fatalf("禁用/未知 id %q 不应请求退出", id)
		}
	}
	if app.menu.phase != MenuPhaseMenu {
		t.Fatalf("禁用/未知 id 不应改变相位，got %v", app.menu.phase)
	}
	if app.menu.error != "" {
		t.Fatalf("禁用/未知 id 不应写入错误行: %q", app.menu.error)
	}
}

// menuInputSpyWindow 记录菜单相位是否会读取游戏输入键/按钮或捕获光标；ShouldClose 在单次
// Poll 后为真，使菜单循环恰好运行一帧。
type menuInputSpyWindow struct {
	fakeInteractiveWindow
	polled             bool
	gameKeyQueries     atomic.Int32
	cursorCaptureCalls atomic.Int32
	focusCalls         atomic.Int32
}

func (window *menuInputSpyWindow) ShouldClose() bool { return window.polled }
func (window *menuInputSpyWindow) Poll()             { window.polled = true }
func (window *menuInputSpyWindow) Focus()            { window.focusCalls.Add(1) }
func (window *menuInputSpyWindow) KeyDown(client.Key) bool {
	window.gameKeyQueries.Add(1)
	return false
}
func (window *menuInputSpyWindow) PrimaryButtonDown() bool {
	window.gameKeyQueries.Add(1)
	return false
}
func (window *menuInputSpyWindow) SecondaryButtonDown() bool {
	window.gameKeyQueries.Add(1)
	return false
}
func (window *menuInputSpyWindow) DrainTextInput(dst []rune) ([]rune, bool) {
	window.gameKeyQueries.Add(1)
	return dst, false
}
func (window *menuInputSpyWindow) SetCursorCaptured(bool)      { window.cursorCaptureCalls.Add(1) }
func (window *menuInputSpyWindow) FramebufferSize() (int, int) { return 64, 64 }
func (window *menuInputSpyWindow) ContentSize() (int, int)     { return 64, 64 }

// TestMenuPhaseInputIsolation 验证主菜单与设置页都不读取游戏输入
// （WASD/点击/文本）、不捕获光标。用真实离屏渲染器各跑一帧菜单循环。
func TestMenuPhaseInputIsolation(t *testing.T) {
	for _, phase := range []MenuPhase{MenuPhaseMenu, MenuPhaseSettings} {
		t.Run(fmt.Sprintf("phase-%d", phase), func(t *testing.T) {
			renderer := requireMenuTestRenderer(t)
			t.Cleanup(func() { renderer.Close() })

			spy := &menuInputSpyWindow{}
			options := Options{
				Seed: 42, WorldPath: t.TempDir(), Identity: func() *network.Identity {
					id := connectionTestIdentity()
					return &id
				}(),
				Render: config.Defaults().Render, StartAtMenu: true,
			}
			app, err := NewWithDependencies(options, newMenuWindowedTestDepsWithWindow(t, renderer, spy))
			if err != nil {
				t.Fatalf("NewWithDependencies: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			app.menu.phase = phase

			if err := RunInteractive(app); err != nil {
				t.Fatalf("RunInteractive 菜单相位: %v", err)
			}
			if got := spy.gameKeyQueries.Load(); got != 0 {
				t.Fatalf("菜单相位不应读取游戏输入键/按钮/文本，查询 = %d", got)
			}
			if got := spy.cursorCaptureCalls.Load(); got != 0 {
				t.Fatalf("菜单相位（未点击进入游戏）不应捕获光标，捕获 = %d", got)
			}
			// 交互路径启动时窗口前置一次（后台启动体验项）。
			if got := spy.focusCalls.Load(); got != 1 {
				t.Fatalf("RunInteractive 应恰请求一次窗口前置，实际 %d", got)
			}
		})
	}
}

// newMenuWindowedTestDepsWithWindow 是 newMenuWindowedTestDeps 的窗口替身；新 Window 用给定间谍窗口。
func newMenuWindowedTestDepsWithWindow(t *testing.T, renderer *client.Renderer, window Window) Dependencies {
	deps := newMenuWindowedTestDeps(t, renderer)
	deps.NewWindow = func(int, int, string) (Window, error) { return window, nil }
	return deps
}

// TestBuildUIStateRealMenuContentLock 锁定真实菜单内容（中文标签、错误行、
// 版本行、四按钮语义)在桥下行状态中的形状:内容与交互主菜单逐字一致,
// 供三端钉值测试以同一份输入对照 schema。
func TestBuildUIStateRealMenuContentLock(t *testing.T) {
	app := &Application{menu: menuState{
		phase: MenuPhaseMenu, title: "Mornlea", version: "dev", error: "存档无法打开",
	}}
	state := app.buildUIState()
	if state.Menu == nil {
		t.Fatal("菜单相位应携带 menu 分节")
	}
	if state.Menu.Title != "Mornlea" || state.Menu.Version != "dev" ||
		state.Menu.Error != "存档无法打开" {
		t.Fatalf("menu 分节内容不符: %+v", state.Menu)
	}
	if len(state.Menu.Buttons) != 4 {
		t.Fatalf("按钮数=%d，想要 4", len(state.Menu.Buttons))
	}
	if state.Menu.Buttons[1].ID != menuActionMultiplayer || state.Menu.Buttons[1].Enabled {
		t.Fatalf("多人游戏按钮应保持禁用: %+v", state.Menu.Buttons[1])
	}
}

// TestBuildUIStateStartingDisablesEnterGame 锁定装配进行中的下行防重呈现：
// starting 相位的菜单分节把「进入游戏」按钮置为禁用，前端经下行状态置灰且
// 不再产生点击/默认按钮事件；其余按钮的可用性不变。
func TestBuildUIStateStartingDisablesEnterGame(t *testing.T) {
	app := &Application{menu: menuState{
		phase: MenuPhaseStarting, title: "Mornlea", version: "dev",
	}}
	state := app.buildUIState()
	if state.Menu == nil {
		t.Fatal("starting 相位应携带 menu 分节")
	}
	if state.Menu.Buttons[0].ID != menuActionStart || state.Menu.Buttons[0].Enabled {
		t.Fatalf("starting 相位进入游戏按钮应经下行禁用: %+v", state.Menu.Buttons[0])
	}
	// 回到菜单相位后按钮恢复可用。
	app.menu.phase = MenuPhaseMenu
	if buttons := app.buildUIState().Menu.Buttons; !buttons[0].Enabled {
		t.Fatalf("菜单相位进入游戏按钮应恢复可用: %+v", buttons[0])
	}
}
