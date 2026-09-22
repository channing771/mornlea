//go:build darwin

package app

// app_menu_vista_test.go 钉住菜单全景（menu-vista）的三条契约：
//
//  1. 相机脚本是整数 tick 的纯函数：同 tick 同姿态、周期回绕、固定俯仰与
//     固定高度，两次构造逐位一致（spec webview-menu-ui「全景背景确定性」）。
//  2. 相位门控：只有主菜单与设置页相位构建并推进全景；游戏相位零参与，
//     世界装配（startWorld/discard）后全景被丢弃。
//  3. 启动红线：全景文件不出现任何世界存储、本地权威服务端或登录装配的
//     构造符号——区块经与权威发布同一份纯函数编码出口进入客户端镜像。

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/config"
)

// captureSettleTimeoutForVistaTest 与 capture 收敛判据同一量级的墙钟时限：
// 全景网格化在 native worker 上异步进行，帧数不是工作量的好度量。
const captureSettleTimeoutForVistaTest = 5 * time.Minute

// clientCameraForVistaTest 返回姿态测试共用的基础相机：俯仰继承自全景常量
// 的语义由 pose 保证，这里只固定投影参数。
func clientCameraForVistaTest() client.Camera {
	return client.Camera{
		Pos:    mgl32.Vec3{99, 99, 99},
		Pitch:  menuVistaPitch,
		FovY:   float32(math.Pi / 3),
		Aspect: 16.0 / 9.0,
		Near:   0.1,
		Far:    2000,
	}
}

// newMenuVistaForTest 用计数空出口构造一条真实 worldgen→镜像→网格化→远环
// 的全景管线（不触碰 GPU），并在测试结束时释放 worker。
func newMenuVistaForTest(t *testing.T) (*menuVista, error) {
	t.Helper()
	vista, err := newMenuVista(
		&menuVistaProbeSink{}, assets.NewDefaultRegistry(), config.Defaults().Render, false,
	)
	if err != nil {
		return nil, err
	}
	t.Cleanup(vista.release)
	return vista, nil
}

// menuVistaProbeSink 统计上传调用的空渲染出口，供无 GPU 的相机脚本测试构造
// 全景管线（不上传任何真实像素）。
type menuVistaProbeSink struct {
	sections int
	tiles    int
}

func (s *menuVistaProbeSink) UploadSection(x, y, z int32, opaque, water []byte) {
	s.sections++
}

func (s *menuVistaProbeSink) DropSection(x, y, z int32) {}

func (s *menuVistaProbeSink) UploadLodTile(x, z int32, quads []byte) { s.tiles++ }

func (s *menuVistaProbeSink) DropLodTile(x, z int32) {}

// TestMenuVistaCameraScriptTickToYaw 钉住自转角的 tick 映射：tick 0 起点为 0、
// 周期回绕、周期内单调且两两不同。角速度因此只由整数 tick 决定，与墙钟和
// 机器速度无关（无头 capture 与交互菜单同一轨迹）。
func TestMenuVistaCameraScriptTickToYaw(t *testing.T) {
	if got := menuVistaYawAt(0); got != 0 {
		t.Fatalf("tick 0 的自转角 = %v，想要 0", got)
	}
	period := uint64(MenuVistaYawPeriodTicks)
	if period < 2 {
		t.Fatalf("自转周期 %d 太小，无法表达缓慢环绕", period)
	}
	for _, tick := range []uint64{0, 1, 7, period - 1, period, period + 3, 5 * period} {
		if got, want := menuVistaYawAt(tick), menuVistaYawAt(tick%period); got != want {
			t.Fatalf("tick %d 的自转角 = %v，想要按周期回绕为 %v", tick, got, want)
		}
	}
	previous := float64(-1)
	for tick := uint64(1); tick < period; tick++ {
		current := menuVistaYawAt(tick)
		if current <= previous {
			t.Fatalf("tick %d 的自转角 %v 不大于前一 tick 的 %v：周期内应单调递增",
				tick, current, previous)
		}
		previous = current
	}
	if menuVistaYawAt(period-1) >= 2*math.Pi {
		t.Fatalf("周期末自转角 %v 越出 [0, 2π)", menuVistaYawAt(period-1))
	}
}

// TestMenuVistaCameraScriptPoseIsDeterministic 钉住相机姿态：位置与俯仰是
// 常量、偏航由 tick 唯一决定，且两次独立构造的 panorama 管线对同一 tick
// 产出逐位相同的姿态（同种子逐帧一致）。
func TestMenuVistaCameraScriptPoseIsDeterministic(t *testing.T) {
	base := clientCameraForVistaTest()
	first, err := newMenuVistaForTest(t)
	if err != nil {
		t.Fatalf("构造全景管线: %v", err)
	}
	second, err := newMenuVistaForTest(t)
	if err != nil {
		t.Fatalf("二次构造全景管线: %v", err)
	}
	if first.cameraPos != second.cameraPos {
		t.Fatalf("两次构造的相机位置不一致: %v vs %v", first.cameraPos, second.cameraPos)
	}
	if first.cameraPos[1] < menuVistaLowestGroundY+menuVistaCameraLift {
		t.Fatalf("相机高度 %v 低于固定抬升下界，可能陷入地形或海面", first.cameraPos)
	}
	for _, tick := range []uint64{0, 1, 360, MenuVistaYawPeriodTicks / 2} {
		first.tick, second.tick = tick, tick
		want, got := first.pose(base), second.pose(base)
		if got != want {
			t.Fatalf("tick %d 两次姿态不一致: %+v vs %+v", tick, want, got)
		}
		if got.Pos != first.cameraPos {
			t.Fatalf("tick %d 姿态位置 %v 随 tick 漂移，想要固定 %v", tick, got.Pos, first.cameraPos)
		}
		if got.Pitch != base.Pitch {
			t.Fatalf("tick %d 俯仰 %v 应继承基础相机的 %v", tick, got.Pitch, base.Pitch)
		}
		if got.Yaw != float32(menuVistaYawAt(tick)) {
			t.Fatalf("tick %d 偏航 %v 与脚本 %v 不一致", tick, got.Yaw, menuVistaYawAt(tick))
		}
	}
}

// TestMenuVistaPumpConvergesToZeroPending 驱动真实 worldgen→镜像→网格化→
// 远环管线直到收敛，钉住「全景装配工作量归零」这一 capture 收敛判据的
// 语义：区块预算逐帧清空队列，mesher 与远环调度器排空后 pending 归零。
// 网格化在 native worker 上异步进行，帧数不是工作量的好度量，这里按
// 与 capture 收敛同一量级的墙钟时限驱动。
func TestMenuVistaPumpConvergesToZeroPending(t *testing.T) {
	vista, err := newMenuVistaForTest(t)
	if err != nil {
		t.Fatalf("构造全景管线: %v", err)
	}
	if vista.pending() == 0 {
		t.Fatal("构造后全景应尚有未完成装配工作")
	}
	deadline := time.Now().Add(captureSettleTimeoutForVistaTest)
	for frame := 0; vista.pending() > 0; frame++ {
		if frame%1000 == 0 && time.Now().After(deadline) {
			t.Fatalf("全景在时限内未收敛：pending=%d（queue=%d mesher=%+v sched=%d lod=%d）",
				vista.pending(), len(vista.queue), vista.mesher.Stats(),
				vista.scheduler.PendingUploads(), vista.lodScheduler.Busy())
		}
		vista.pump(64)
	}
	if vista.lodScheduler.Busy() != 0 {
		t.Fatal("远环调度器未排空")
	}
}

// TestMenuVistaPhaseGating 钉住相位门控：游戏相位不构建、不推进全景；
// 菜单相位惰性构建并随渲染帧推进；相位重进把自转 tick 归零（确定性同
// 画面）；显式丢弃后 pending 恒为 0。需要 GPU 适配器，缺失时跳过。
// 全景装配由 TestMenuVistaPumpConvergesToZeroPending 穷举，这里只驱动
// 若干帧验证管线在真实渲染帧路径上运转（pending 有推进即足）。
func TestMenuVistaPhaseGating(t *testing.T) {
	app := NewOffscreenRenderApplicationForTest(
		t, &IntegrationGlyphSource{}, 64, 64, config.Defaults().Render,
	)
	if got := app.menuVistaForFrame(); got != nil {
		t.Fatal("游戏相位不得构建全景")
	}
	if app.MenuVistaPending() != 0 {
		t.Fatal("游戏相位的全景 pending 应恒为 0")
	}

	app.SetMenuPhase(MenuPhaseMenu)
	vista := app.menuVistaForFrame()
	if vista == nil {
		t.Fatal("菜单相位必须惰性构建全景")
	}
	if app.menuVistaForFrame() != vista {
		t.Fatal("同一菜单相位内全景必须复用，不得逐帧重建")
	}
	for frame := 0; frame < 16; frame++ {
		if _, err := app.RenderFrame(64); err != nil {
			t.Fatalf("渲染全景帧: %v", err)
		}
	}
	// The reveal gate assembles only 12 of 625 queued chunks per render frame, so
	// 16 frames cannot converge. Waiting frames use the sky-only fallback and keep
	// the rotation clock at zero, which lets the converged vista reveal from tick zero.
	if vista.tick != 0 {
		t.Fatalf("未收敛的 16 个渲染帧后自转 tick = %d，想要 0", vista.tick)
	}

	// 相位重进（主菜单 → 设置页）把自转 tick 归零：两次进入菜单相位的
	// The first-frame poses therefore match exactly. Seed a nonzero clock to model
	// several revealed frames so the reset on re-entry is observable.
	vista.tick = 5
	app.SetMenuPhase(MenuPhaseSettings)
	if got := app.menuVistaForFrame(); got == nil {
		t.Fatal("设置页相位必须继续渲染全景")
	}
	if vista.tick != 0 {
		t.Fatalf("相位重进后自转 tick = %d，想要 0", vista.tick)
	}

	// 游戏相位立即零参与；显式丢弃（世界装配路径）后不再有任何工作量。
	app.SetMenuPhase(MenuPhaseGame)
	if got := app.menuVistaForFrame(); got != nil {
		t.Fatal("回到游戏相位后全景必须停止参与")
	}
	app.discardMenuVista()
	if app.menuVista != nil || app.MenuVistaPending() != 0 {
		t.Fatal("丢弃后全景必须为 nil 且 pending 归零")
	}
	app.discardMenuVista() // 幂等
}

// `TestMenuVistaTickPinRendersPinnedPose` verifies capture tick pinning after
// convergence. The first frame after `SetMenuVistaTick` renders the pinned pose,
// independent of convergence duration. The reveal gate does not advance time
// before convergence, so the pinned frame is necessarily a revealed frame and
// matches capture's converge-then-pin sequence.
func TestMenuVistaTickPinRendersPinnedPose(t *testing.T) {
	app := NewOffscreenRenderApplicationForTest(
		t, &IntegrationGlyphSource{}, 64, 64, config.Defaults().Render,
	)
	app.SetMenuPhase(MenuPhaseMenu)
	vista := app.menuVistaForFrame()
	if vista == nil {
		t.Fatal("菜单相位必须惰性构建全景")
	}
	// Pump to convergence exactly as the real frame path does once per frame;
	// capture pins the tick only after convergence.
	deadline := time.Now().Add(captureSettleTimeoutForVistaTest)
	for frame := 0; vista.pending() > 0; frame++ {
		if frame%1000 == 0 && time.Now().After(deadline) {
			t.Fatalf("全景在时限内未收敛：pending=%d", vista.pending())
		}
		vista.pump(64)
	}
	pinned := uint64(MenuVistaYawPeriodTicks / 8)
	app.SetMenuVistaTick(pinned)
	if got := app.menuVista.tick; got != pinned {
		t.Fatalf("钉住后 tick = %d，想要 %d", got, pinned)
	}
	if _, err := app.RenderFrame(64); err != nil {
		t.Fatalf("渲染钉住帧: %v", err)
	}
	if got := app.menuVista.tick; got != pinned+1 {
		t.Fatalf("钉住帧渲染后 tick = %d，想要 %d（渲染 pose(N) 后再推进）", got, pinned+1)
	}
}

// `TestMenuVistaRevealGateWithholdsUnconvergedFrames` verifies the reveal gate
// without a GPU. While `pending` is nonzero, frame entry cannot return the vista,
// so rendering uses the build-failure sky-only fallback, submits no vista
// geometry, and freezes the rotation clock. The first converged frame returns
// the same vista at tick zero and subsequent frames keep it revealed, preserving
// the existing deterministic frame sequence.
func TestMenuVistaRevealGateWithholdsUnconvergedFrames(t *testing.T) {
	vista, err := newMenuVistaForTest(t)
	if err != nil {
		t.Fatalf("构造全景管线: %v", err)
	}
	// Inject an already-built vista into a minimal `Application`. Renderer and
	// material registry requirements apply only to construction, so this preserves
	// the real frame-entry lifecycle without requiring a GPU.
	app := &Application{menu: menuState{phase: MenuPhaseMenu}}
	app.menuVista = vista
	app.menuVistaPhase = MenuPhaseMenu

	// An unconverged frame returns nil for the sky-only path and keeps time at zero.
	for frame := 0; frame < 4; frame++ {
		if got := app.revealMenuVista(64); got != nil {
			t.Fatalf("未收敛的第 %d 帧不得揭示全景几何", frame)
		}
		if vista.tick != 0 {
			t.Fatalf("未收敛帧推进了自转时钟: tick=%d", vista.tick)
		}
	}
	// Four frames at 12 chunks each are far below 625 queued chunks, so convergence is impossible here.
	if vista.pending() == 0 {
		t.Fatal("4 帧装配后全景不应收敛（每帧区块预算远小于队列总量）")
	}

	// Pump through the wait to convergence, matching the real once-per-frame path
	// outside rendering, under the same order-of-magnitude timeout as capture.
	deadline := time.Now().Add(captureSettleTimeoutForVistaTest)
	for frame := 0; vista.pending() > 0; frame++ {
		if frame%1000 == 0 && time.Now().After(deadline) {
			t.Fatalf("全景在时限内未收敛：pending=%d", vista.pending())
		}
		vista.pump(64)
	}

	// The first converged frame reveals the same instance from tick zero, which
	// advances only after rendering, and later frames must not close the gate again.
	for frame := 0; frame < 3; frame++ {
		if got := app.revealMenuVista(64); got != vista {
			t.Fatalf("收敛后的第 %d 帧必须揭示同一全景实例", frame)
		}
		if vista.tick != 0 {
			t.Fatalf("揭示首帧应渲染 pose(0)，得到 tick=%d", vista.tick)
		}
	}
}

// `TestMenuVistaBuildFailureStillDegradesToSkyClear` verifies that the reveal gate
// preserves build-failure fallback. Without a renderer, each menu frame returns
// nil without building, revealing, or advancing; vista state remains nil and
// pending work stays zero, leaving the sky clear behind interactive menu chrome.
func TestMenuVistaBuildFailureStillDegradesToSkyClear(t *testing.T) {
	app := &Application{menu: menuState{phase: MenuPhaseMenu}}
	for frame := 0; frame < 4; frame++ {
		if got := app.revealMenuVista(64); got != nil {
			t.Fatalf("无渲染器的第 %d 帧不得构建或揭示全景", frame)
		}
	}
	if app.menuVista != nil || app.MenuVistaPending() != 0 {
		t.Fatal("构建失败降级后全景必须保持 nil 且 pending 归零")
	}
}

// TestMenuVistaDoesNotAssembleWorld 是启动红线的结构性守卫：全景源文件
// 只允许出现 worldgen 纯函数生成与权威发布共用的快照编码出口，不得出现
// 世界存储打开、本地权威服务端、登录或内存流装配的任何构造符号。
func TestMenuVistaDoesNotAssembleWorld(t *testing.T) {
	source, err := os.ReadFile("app_menu_vista.go")
	if err != nil {
		t.Fatalf("读取全景源文件: %v", err)
	}
	text := string(source)
	for _, forbidden := range []string{
		"storage.", "OpenStore", "server.New", "NewHost", "LoginClient",
		"NewMemoryStreamPair", "assembleLocalApplicationConnection",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("全景源文件出现装配红线符号 %q：菜单相位不得触碰世界存储、服务端或登录", forbidden)
		}
	}
	if !strings.Contains(text, "server.BuildChunkSnapshot") {
		t.Fatal("全景区块必须经与权威发布同一份快照编码出口进入客户端镜像")
	}
}
