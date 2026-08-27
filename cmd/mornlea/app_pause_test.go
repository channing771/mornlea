//go:build darwin

package main

// app_pause_test.go：暂停覆盖层的相位机、Esc 栈、防重入、会话拆链与远程形态
// 下行标志的行为契约测试。复用 app_menu_test.go 的装配夹具与 app_connection_test.go
// 的反依赖注入；宿主替身自带暂停门计数，验证「本地形态真实调用门、远程形态
// 零调用只发标志」这条核心分叉。

import (
	"context"
	"encoding/binary"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
)

// pauseCapableTestHost 在连接测试宿主之上补充暂停门计数：既满足 applicationHost，
// 又满足可选的 applicationPauseGate 能力接口，供断言「开层调一次 Pause、关层调
// 一次 Resume」的精确次数。暂停/恢复在替身上只计数，不触碰底层生命周期。
type pauseCapableTestHost struct {
	*connectionTestHost
	pauseCalls  atomic.Int32
	resumeCalls atomic.Int32
}

func (host *pauseCapableTestHost) Pause()  { host.pauseCalls.Add(1) }
func (host *pauseCapableTestHost) Resume() { host.resumeCalls.Add(1) }

// pauseWorldSuccessDeps 是 startWorldSuccessDeps 的暂停门升级版：newHost 返回带
// 暂停计数的宿主替身，工厂计数用于断言两次「进入游戏」各装配一台全新世界。
func pauseWorldSuccessDeps(t *testing.T) (applicationDependencies, func() []*pauseCapableTestHost) {
	t.Helper()
	var hosts []*pauseCapableTestHost
	deps := startWorldSuccessDeps(t)
	deps.newHost = func(_ context.Context, _ server.Config, _ server.Generator, store storage.WorldStore) (applicationHost, error) {
		host := &pauseCapableTestHost{connectionTestHost: newConnectionTestHost(store)}
		hosts = append(hosts, host)
		return host, nil
	}
	return deps, func() []*pauseCapableTestHost { return hosts }
}

// newPausedGameTestApp 构造一个已经过「进入游戏→打开暂停层」全程的真实装配态：
// 门经装配点类型断言捕获（本地形态），断言打开动作释放光标并恰好调用一次 Pause。
func newPausedGameTestApp(t *testing.T) (*application, func() []*pauseCapableTestHost) {
	t.Helper()
	deps, hosts := pauseWorldSuccessDeps(t)
	app := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = app.Close() })
	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("进入游戏不应请求退出")
	}
	if app.pauseGate == nil {
		t.Fatal("装配点应从宿主捕获暂停门（本地形态非空）")
	}
	app.openPauseOverlay()
	return app, hosts
}

// TestPauseTransitionMatrixEscapeOpensReleasesCursorAndPauses 锁定转换矩阵的开侧：
// 打开即释放光标、相位转暂停、嵌入服门恰好被置位一次，且 UI 段切到暂停布局。
func TestPauseTransitionMatrixEscapeOpensReleasesCursorAndPauses(t *testing.T) {
	app, hosts := newPausedGameTestApp(t)

	if app.menu.phase != menuPhasePaused {
		t.Fatalf("phase = %v，want paused", app.menu.phase)
	}
	if app.window.CursorCaptured() {
		t.Fatal("打开暂停层必须释放光标")
	}
	if got := hosts()[0].pauseCalls.Load(); got != 1 {
		t.Fatalf("本地形态打开应恰调用一次暂停门，实际 %d", got)
	}
	segment := app.uiSegment()
	if len(segment) != 12 {
		t.Fatalf("暂停相位 UI 段长度 = %d，want 12（版本+两布尔）", len(segment))
	}
}

// TestPauseTransitionMatrixEscOrButtonResumesOnce 锁定关侧防重入：Esc 边沿与
// 「返回游戏」动作同帧重复到达只生效一次——门只解一次、相位只翻一次。
func TestPauseTransitionMatrixEscOrButtonResumesOnce(t *testing.T) {
	app, hosts := newPausedGameTestApp(t)

	// 同一帧内三条恢复路径相继到达：Esc 边沿直呼 + 两条重复动作事件。
	app.closePauseOverlay()
	app.handleMenuEvent(menuActionPauseBack)
	app.handleMenuEvent(menuActionPauseBack)

	if app.menu.phase != menuPhaseGame {
		t.Fatalf("phase = %v，want game", app.menu.phase)
	}
	if !app.window.CursorCaptured() {
		t.Fatal("恢复后必须重新捕获光标")
	}
	if got := hosts()[0].resumeCalls.Load(); got != 1 {
		t.Fatalf("重复触发只应恢复一次，实际 %d 次", got)
	}

	// 开合周期可重复：重新打开后哨兵重新武装，再关仍恢复一次。
	app.openPauseOverlay()
	if got := hosts()[0].pauseCalls.Load(); got != 2 {
		t.Fatalf("第二次打开应再置位一次门，实际 %d", got)
	}
	app.handleMenuEvent(menuActionPauseBack)
	if app.menu.phase != menuPhaseGame || hosts()[0].resumeCalls.Load() != 2 {
		t.Fatalf("第二周期关闭失败: phase=%v resume=%d",
			app.menu.phase, hosts()[0].resumeCalls.Load())
	}
}

// TestPauseUnknownActionIgnored 锁定动作表面安全：暂停相位外的 8/9、以及暂停
// 相位内的未知 id 都不产生任何副作用。
func TestPauseUnknownActionIgnored(t *testing.T) {
	app := &application{
		menu:      menuState{phase: menuPhaseMenu},
		window:    &fakeInteractiveWindow{},
		itemDrops: client.NewItemDrops(),
	}
	for _, id := range []uint32{menuActionPauseBack, menuActionPauseQuitToMenu, 99} {
		if quit := app.handleMenuEvent(id); quit {
			t.Fatalf("菜单相位的暂停 id %d 不应请求退出", id)
		}
		if app.menu.phase != menuPhaseMenu {
			t.Fatalf("菜单相位收到 %d 改变了相位: %v", id, app.menu.phase)
		}
	}

	deps, _ := pauseWorldSuccessDeps(t)
	paused := newStartWorldTestApp(t, deps)
	t.Cleanup(func() { _ = paused.Close() })
	_ = paused.handleMenuEvent(menuActionStart)
	paused.openPauseOverlay()
	if quit := paused.handleMenuEvent(99); quit || paused.menu.phase != menuPhasePaused {
		t.Fatalf("暂停相位未知 id: quit=%v phase=%v", quit, paused.menu.phase)
	}
}

// TestPauseQuitToMenuTearsDownAndAllowsReassembly 锁定「退回主菜单」：
// 复用既有拆链语义安全关闭会话与世界连接后回主菜单，「进入游戏」可装配出
// 全新会话状态（镜像/预测器重建、门重新捕获），关服幂等不受二次会话影响。
func TestPauseQuitToMenuTearsDownAndAllowsReassembly(t *testing.T) {
	app, hosts := newPausedGameTestApp(t)
	firstMirror := app.mirror
	firstPredictor := app.predictor

	app.handleMenuEvent(menuActionPauseQuitToMenu)

	if app.menu.phase != menuPhaseMenu {
		t.Fatalf("拆链后 phase = %v，want menu", app.menu.phase)
	}
	finished := hosts()[0]
	if finished.resumeCalls.Load() != 1 {
		t.Fatalf("拆链前应先解除冻结，Resume=%d", finished.resumeCalls.Load())
	}
	if finished.shutdownCalls.Load() != 1 {
		t.Fatalf("世界连接未按既有序序 Shutdown（持久化收尾缺失），calls=%d",
			finished.shutdownCalls.Load())
	}
	if app.host != nil || app.receiver != nil || app.serverCancel != nil || app.serverDone != nil {
		t.Fatalf("拆链后连接字段未复位: host=%v receiver=%v cancel=%v done=%v",
			app.host != nil, app.receiver != nil, app.serverCancel != nil, app.serverDone != nil)
	}
	if app.remotePlayers.Presentations() != nil && len(app.remotePlayers.Presentations()) != 0 {
		t.Fatal("拆链后远端玩家镜像残留")
	}
	if app.inventoryOpen {
		t.Fatal("拆链后背包状态残留")
	}

	// 再点「进入游戏」能成功装配出全新会话状态，与首次进入同一构造口径。
	if quit := app.handleMenuEvent(menuActionStart); quit {
		t.Fatal("再进入不应请求退出")
	}
	if app.menu.phase != menuPhaseGame {
		t.Fatalf("二次装配后 phase = %v，want game", app.menu.phase)
	}
	if len(hosts()) != 2 {
		t.Fatalf("二次装配应新建世界宿主，实际 %d 台", len(hosts()))
	}
	if app.mirror == firstMirror || app.predictor == firstPredictor {
		t.Fatal("二次装配复用了上一会话的世界镜像/预测器，存在跨会话污染")
	}
	if app.clientSessionClosed {
		t.Fatal("新会话不得继承 clientSessionClosed（否则目标反馈永久失效）")
	}

	// 旧世界已析构、新世界归新宿主：最终 Close 只需安全收敛新连接。
	if err := app.Close(); err != nil {
		t.Fatalf("二次会话后 Close: %v", err)
	}
}

// TestPauseRemoteFormSkipsGateAndMarksSegment 锁定 TCP 远程形态：无嵌入宿主时
// 打开暂停层零调用任何本地服务端接口（nil 安全）、段内 remote 位为真；对照
// 本地形态同一位为假。
func TestPauseRemoteFormSkipsGateAndMarksSegment(t *testing.T) {
	remoteApp := &application{
		menu:            menuState{phase: menuPhaseGame},
		window:          &fakeInteractiveWindow{},
		remotePlayers:   client.NewRemotePlayers(),
		itemDrops:       client.NewItemDrops(),
		inventorySource: -1,
		render:          config.Defaults().Render,
		serverTick:      0,
		mirror:          client.NewMirror(),
		predictor:       client.NewPredictor(),
		startupOptions:  applicationOptions{Render: config.Defaults().Render},
	}
	remoteApp.releaseResources = remoteApp.releaseOwnedResources
	if !remoteApp.remote() {
		t.Fatal("夹具应为远程形态（host/server 双 nil）")
	}
	remoteApp.openPauseOverlay()
	if remoteApp.menu.phase != menuPhasePaused {
		t.Fatalf("远程形态也应呈现暂停相位，got %v", remoteApp.menu.phase)
	}
	if got := binary.LittleEndian.Uint32(remoteApp.uiSegment()[8:]); got != 1 {
		t.Fatalf("远程形态 remote 位 = %d，want 1", got)
	}

	localApp, _ := newPausedGameTestApp(t)
	if localApp.remote() {
		t.Fatal("本地夹具不应判为远程形态")
	}
	if got := binary.LittleEndian.Uint32(localApp.uiSegment()[8:]); got != 0 {
		t.Fatalf("本地形态 remote 位 = %d，want 0", got)
	}
}

// TestStartWorldRejectsRemoteConnectForm 防御性锁定：迟回主菜单的 -connect 形态
// 再点「进入游戏」必须以菜单错误行优雅拒绝，绝不带着 nil 存档跑进 Host 构造 panic。
func TestStartWorldRejectsRemoteConnectForm(t *testing.T) {
	identity := connectionTestIdentity()
	options := remoteConnectionOptions()
	options.StartAtMenu = true
	app := newStartWorldTestApp(t, applicationDependencies{})
	app.startupOptions = options
	app.startupOptions.Identity = &identity

	err := app.startWorld()
	if err == nil {
		t.Fatal("远程形态本地装配未被拒绝")
	}
	if !errors.Is(err, errRemoteStartWorldRejected) {
		t.Fatalf("拒绝错误不符: %v", err)
	}
	if app.menu.phase != menuPhaseMenu {
		t.Fatalf("拒绝后 phase = %v，want menu（保持主菜单）", app.menu.phase)
	}
	if app.menu.error == "" {
		t.Fatal("拒绝必须落错误行提示而非静默")
	}
}

// TestPauseSegmentAbsentOutsidePausedPhase 锁定下行契约的另一半：非暂停相位
// 绝不下发暂停段，capture 注入与设置/主菜单既有输出字节不变。
func TestPauseSegmentAbsentOutsidePausedPhase(t *testing.T) {
	// 设置相位的段编码要求合法窗口预设，夹具按构造期默认值填齐。
	preset := settingsValues{windowSize: config.WindowSize1280x720}
	blank := &application{
		settings: settingsState{committed: preset, draft: preset},
	}
	for _, phase := range []menuPhase{menuPhaseGame, menuPhaseMenu, menuPhaseSettings, menuPhaseStarting} {
		blank.menu.phase = phase
		segment := blank.uiSegment()
		if len(segment) == 12 && binary.LittleEndian.Uint32(segment[:4]) == uint32(uiPauseLayoutVersion) {
			t.Fatalf("相位 %v 不应下发暂停段", phase)
		}
	}
	game := &application{menu: menuState{phase: menuPhaseGame}}
	if segment := game.uiSegment(); segment != nil {
		t.Fatalf("裸游戏相位应保持无 UI 段契约，got %d 字节", len(segment))
	}
}
