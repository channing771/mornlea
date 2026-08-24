//go:build darwin

package main

// app_menu_test.go：主菜单状态机与延迟世界装配的测试。复用 app_test_helpers_test.go
// 的假窗口与 app_connection_test.go 的反依赖注入，验证 StartAtMenu 构造停留菜单、
// 「进入游戏」延迟装配、装配失败可退出、starting 防重入、菜单相位输入不生效，以及
// 真实菜单内容的跨语言段编码（Ruling 8 非 4 对齐）锁。

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
)

// newMenuWindowedTestDeps 构造 StartAtMenu 窗口化路径的依赖注入载体：newWindow/
// newWindowedRenderer 服务窗口与渲染器创建，openStore/newHost/... 断言在构造阶段
// 不被调用（StartAtMenu 延迟装配），供后续 startWorld 使用。
func newMenuWindowedTestDeps(t *testing.T, renderer *client.Renderer) applicationDependencies {
	t.Helper()
	unexpected := func(name string) {
		t.Helper()
		t.Fatalf("StartAtMenu 构造不应调用 %s（世界装配应延迟）", name)
	}
	return applicationDependencies{
		openStore: func(context.Context, applicationOptions) (storage.WorldStore, error) {
			unexpected("openStore")
			return nil, nil
		},
		dialTCP: func(context.Context, string) (network.ClientPacketStream, error) {
			unexpected("dialTCP")
			return nil, nil
		},
		loginClient: func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
			unexpected("loginClient")
			return nil, 0, nil
		},
		newHost: func(context.Context, server.Config, server.Generator, storage.WorldStore) (applicationHost, error) {
			unexpected("newHost")
			return nil, nil
		},
		newMemoryStreamPair: func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
			unexpected("newMemoryStreamPair")
			return nil, nil, nil
		},
		newWindow: func(int, int, string) (applicationWindow, error) {
			return &fakeInteractiveWindow{}, nil
		},
		newWindowedRenderer: func(applicationWindow) (*client.Renderer, error) {
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
	options := applicationOptions{
		Seed:        42,
		WorldPath:   t.TempDir(),
		Identity:    &identity,
		Render:      config.Defaults().Render,
		StartAtMenu: true,
	}
	app, err := newApplicationWithDependencies(options, newMenuWindowedTestDeps(t, renderer))
	if err != nil {
		t.Fatalf("newApplicationWithDependencies: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	if app.server != nil {
		t.Fatalf("StartAtMenu 构造不应启动本地权威服务端，server=%v", app.server)
	}
	if app.host != nil {
		t.Fatalf("StartAtMenu 构造不应创建 Host，host=%v", app.host)
	}
	if app.menu.phase != menuPhaseMenu {
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

	menu := app.menu.uiMenu()
	if !menu.Visible {
		t.Fatal("menu.Visible 应为 true（菜单相位可见）")
	}
	if menu.Title != "Mornlea" || menu.Version != app.menu.version || menu.Error != "" {
		t.Fatalf("menu 标题/版本/错误行不符: %+v", menu)
	}
	wantButtons := []client.UIButton{
		{ID: menuActionStart, Label: "进入游戏", Enabled: true},
		{ID: menuActionMultiplayer, Label: "多人游戏", Enabled: false},
		{ID: menuActionSettings, Label: "设置", Enabled: false},
		{ID: menuActionQuit, Label: "退出游戏", Enabled: true},
	}
	if !reflect.DeepEqual(menu.Buttons, wantButtons) {
		t.Fatalf("菜单按钮不符: got=%+v want=%+v", menu.Buttons, wantButtons)
	}

	if segment := app.uiSegment(); len(segment) == 0 {
		t.Fatal("菜单相位 uiSegment() 应产出非空 UI 段")
	}
}

// startWorldSuccessDeps 为 startWorld 提供可成功的连接依赖：内存 store、Host、内存流对与登录。
func startWorldSuccessDeps(t *testing.T) applicationDependencies {
	t.Helper()
	return applicationDependencies{
		openStore: func(context.Context, applicationOptions) (storage.WorldStore, error) {
			return newConnectionTestStore(42), nil
		},
		newHost: func(_ context.Context, _ server.Config, _ server.Generator, store storage.WorldStore) (applicationHost, error) {
			return newConnectionTestHost(store), nil
		},
		newMemoryStreamPair: func(capacity int) (network.ClientPacketStream, network.ServerPacketStream, error) {
			clientStream, serverStream := network.NewMemoryStreamPair(capacity)
			return clientStream, serverStream, nil
		},
		loginClient: func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
			clientEndpoint, serverEndpoint := network.NewMemoryPair(1)
			t.Cleanup(func() { _ = serverEndpoint.Close() })
			return clientEndpoint, 42, nil
		},
	}
}

// newStartWorldTestApp 构造一个菜单相位、可直接调用 startWorld 的 application（关闭 LOD 使
// attachLodScheduler 零参与，因此无需真实渲染器），并正确配置 Close 所需的资源字段。
func newStartWorldTestApp(t *testing.T, deps applicationDependencies) *application {
	t.Helper()
	identity := connectionTestIdentity()
	render := config.Defaults().Render
	render.LodEnabled = false
	ticks, _ := newPerformanceRecorders(false)
	app := &application{
		menu: menuState{phase: menuPhaseMenu, title: "Mornlea", version: menuVersion()},
		startupOptions: applicationOptions{
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

// TestHandleMenuEventStartAssemblesWorld 验证「进入游戏」点击：startWorld 被调用、成功置相位
// game、starting 复位并捕获光标。
func TestHandleMenuEventStartAssemblesWorld(t *testing.T) {
	app := newStartWorldTestApp(t, startWorldSuccessDeps(t))
	t.Cleanup(func() { _ = app.Close() })

	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("进入游戏不应请求退出")
	}
	if app.menu.phase != menuPhaseGame {
		t.Fatalf("装配成功后 phase = %v，want game", app.menu.phase)
	}
	if app.menu.starting {
		t.Fatal("装配成功后 starting 应为 false")
	}
	if app.menu.error != "" {
		t.Fatalf("装配成功不应有错误行: %q", app.menu.error)
	}
	if !app.window.CursorCaptured() {
		t.Fatal("装配成功应捕获光标")
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
	deps.openStore = func(context.Context, applicationOptions) (storage.WorldStore, error) {
		return nil, wantErr
	}
	app := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = app.Close() })

	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("装配失败不应请求退出")
	}
	if app.menu.phase != menuPhaseMenu {
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
// 重复点击「进入游戏」被忽略，不再次调用 startWorld（openStore 计数保持 0）。
func TestHandleMenuEventStartIgnoredWhileStarting(t *testing.T) {
	var openStoreCalls atomic.Int32
	deps := startWorldSuccessDeps(t)
	deps.openStore = func(ctx context.Context, options applicationOptions) (storage.WorldStore, error) {
		openStoreCalls.Add(1)
		return newConnectionTestStore(42), nil
	}
	app := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = app.Close() })

	// 模拟装配已在进行中：starting=true 时重复点击应被守卫拦下。
	app.menu.starting = true
	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("重复点击不应请求退出")
	}
	if got := openStoreCalls.Load(); got != 0 {
		t.Fatalf("starting 期间重复点击不应再次装配，openStore 调用 = %d", got)
	}
	if app.menu.phase != menuPhaseMenu {
		t.Fatalf("被守卫拦下后相位不应改变，got %v", app.menu.phase)
	}

	// 守卫只拦「装配中」，正常点击仍装配一次。
	app.menu.starting = false
	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("正常进入游戏不应请求退出")
	}
	if got := openStoreCalls.Load(); got != 1 {
		t.Fatalf("正常进入游戏应恰好装配一次，openStore 调用 = %d", got)
	}
	if app.menu.phase != menuPhaseGame {
		t.Fatalf("正常装配后 phase = %v，want game", app.menu.phase)
	}
}

// TestHandleMenuEventQuitRequestsClose 验证「退出游戏」返回退出信号，且不改变菜单状态。
func TestHandleMenuEventQuitRequestsClose(t *testing.T) {
	app := newStartWorldTestApp(t, startWorldSuccessDeps(t))
	if quit := app.handleMenuEvent(menuActionQuit); !quit {
		t.Fatal("退出游戏应返回退出信号")
	}
	if app.menu.phase != menuPhaseMenu {
		t.Fatalf("菜单期退出不应改变相位，got %v", app.menu.phase)
	}
}

// TestHandleMenuEventUnknownIDIgnored 验证未知/禁用按钮 id 被忽略，不改变菜单状态。
func TestHandleMenuEventUnknownIDIgnored(t *testing.T) {
	app := newStartWorldTestApp(t, startWorldSuccessDeps(t))
	for _, id := range []uint32{menuActionMultiplayer, menuActionSettings, 99} {
		if quit := app.handleMenuEvent(id); quit {
			t.Fatalf("禁用/未知 id %d 不应请求退出", id)
		}
	}
	if app.menu.phase != menuPhaseMenu {
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
}

func (window *menuInputSpyWindow) ShouldClose() bool { return window.polled }
func (window *menuInputSpyWindow) Poll()             { window.polled = true }
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

// TestMenuPhaseInputIsolation 验证菜单相位不读取游戏输入（WASD/点击/文本）、不捕获光标，
// 从而规格「菜单期间游戏输入不生效」。用真实离屏渲染器跑一帧菜单循环。
func TestMenuPhaseInputIsolation(t *testing.T) {
	renderer := requireMenuTestRenderer(t)
	t.Cleanup(func() { renderer.Close() })

	spy := &menuInputSpyWindow{}
	options := applicationOptions{
		Seed: 42, WorldPath: t.TempDir(), Identity: func() *network.Identity {
			id := connectionTestIdentity()
			return &id
		}(),
		Render: config.Defaults().Render, StartAtMenu: true,
	}
	app, err := newApplicationWithDependencies(options, newMenuWindowedTestDepsWithWindow(t, renderer, spy))
	if err != nil {
		t.Fatalf("newApplicationWithDependencies: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	if err := runInteractive(app); err != nil {
		t.Fatalf("runInteractive 菜单相位: %v", err)
	}
	if got := spy.gameKeyQueries.Load(); got != 0 {
		t.Fatalf("菜单相位不应读取游戏输入键/按钮/文本，查询 = %d", got)
	}
	if got := spy.cursorCaptureCalls.Load(); got != 0 {
		t.Fatalf("菜单相位（未点击进入游戏）不应捕获光标，捕获 = %d", got)
	}
}

// newMenuWindowedTestDepsWithWindow 是 newMenuWindowedTestDeps 的窗口替身；新 Window 用给定间谍窗口。
func newMenuWindowedTestDepsWithWindow(t *testing.T, renderer *client.Renderer, window applicationWindow) applicationDependencies {
	deps := newMenuWindowedTestDeps(t, renderer)
	deps.newWindow = func(int, int, string) (applicationWindow, error) { return window, nil }
	return deps
}

// TestUISegmentRealMenuEncodingLock 锁定真实菜单内容（中文标签、错误行）的跨语言段编码：
// 段字节长度与 EncodeUIMenu 算术预期一致，且非 4 对齐（Ruling 8：TLV 文本段豁免 4 对齐）。
func TestUISegmentRealMenuEncodingLock(t *testing.T) {
	menu := client.UIMenu{
		Visible: true,
		Title:   "Mornlea",
		Version: "dev",
		Error:   "存档无法打开",
		Buttons: menuButtons(),
	}
	app := &application{menuOverride: &menu}

	segment := app.uiSegment()
	expected := uiSegmentArithmeticLength(menu)
	if len(segment) != expected {
		t.Fatalf("真实菜单段长度 = %d，算术预期 %d", len(segment), expected)
	}
	if len(segment)%4 == 0 {
		t.Fatalf("真实菜单段长度 %d 不应 4 对齐（Ruling 8 非对齐 TLV 文本段）", len(segment))
	}
	if len(segment) == 0 {
		t.Fatal("菜单段不应为空")
	}
}

// uiSegmentArithmeticLength 按 EncodeUIMenu 的字段布局算术计算段字节长度：
// 固定 4 个 u32（layout/flags/buttonCount）+ 每按钮 [u32 id + u32 label_len + label + u32 enabled]
// + 三个字符串字段 [u32 len + bytes]。
func uiSegmentArithmeticLength(menu client.UIMenu) int {
	length := 3 * 4
	for _, button := range menu.Buttons {
		length += 4 + 4 + len([]byte(button.Label)) + 4
	}
	length += 4 + len([]byte(menu.Title))
	length += 4 + len([]byte(menu.Version))
	length += 4 + len([]byte(menu.Error))
	return length
}
