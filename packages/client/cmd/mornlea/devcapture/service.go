//go:build darwin

// Package devcapture 实现运行中交互式客户端的本地开发捕获服务：仅绑定回环
// 地址的 `http.Server` 暴露 `/status`、`/screenshot` 与 `/record` 三个端点，
// 把「观察游戏画面」变成一次 HTTP 请求。
//
// 与帧循环的衔接走 `app.CaptureCoordinator` 消费端契约：本包的 `Service`
// 持有容量 1 的请求通道（单 outstanding），帧循环每帧非阻塞检查待办、按需
// 捕获并立即交付；PNG/zip/GIF 编码全部发生在本包的服务 goroutine，帧循环
// 永不编码、永不被消费速度反压。生产接线由 main 完成（注入协调器、拉起
// 监听、注册优雅关闭），本包不感知窗口与渲染细节。
package devcapture

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
)

// 服务面的固定时限。全部取值面向「本地调试工具」：等待上限足够覆盖一次
// 窗口合成拷贝的慢分位，又保证帧循环停止后请求必然有界收敛。
const (
	// DefaultAddr 是默认监听地址：只绑回环，服务永不对局域网暴露。
	DefaultAddr = "127.0.0.1:17790"

	// defaultFrameWait 是单帧捕获从入队到交付的等待上限（截图整体等待与
	// 录制逐帧等待共用）：帧循环正常运行时一帧捕获约 10-30ms，10s 只在
	// 帧循环已停止或被长时间阻塞时才会触达。
	defaultFrameWait = 10 * time.Second

	// recordDeadlineMargin 是录制总截止的固定余量：与帧数无关的部分（节奏
	// 等待的调度抖动、请求编排开销）落在这里。随帧数线性扩展的捕获与编码
	// 成本由 `recordPerFrameBudget` 覆盖，二者相加构成可判定的总上限。
	recordDeadlineMargin = 30 * time.Second

	// recordPerFrameBudget 是录制总截止为每帧预留的预算（捕获 + 编码 +
	// 量化全链路）。按两组真实窗口实测反推（Retina 全分辨率 2784×1728）：
	// PNG 路径 BestSpeed 编码后约 0.42s/帧，GIF 路径经 Floyd-Steinberg
	// 调色板量化后约 2.2s/帧——预算取最慢可选路径放大到 2.5s/帧，覆盖
	// GIF 慢分位；最坏 240 帧的截止（600s + 30s + 名义时长）仍是有界常量。
	recordPerFrameBudget = 2500 * time.Millisecond

	// maxBindAttempts 是默认端口被占时的顺延尝试次数：顺延足够越过偶发
	// 占用，又保证地址耗尽时快速失败。
	maxBindAttempts = 10

	// readHeaderTimeout 限制读取请求头的时间，防止慢速连接长期占用（与
	// agent-board 服务同一口径）。
	readHeaderTimeout = 5 * time.Second

	// shutdownTimeout 是优雅关闭的等待上限：本地工具的请求都有毫秒级
	// 上限，5s 足够让在途请求排空。
	shutdownTimeout = 5 * time.Second
)

// 协调器通道两侧的运行期失败。它们是服务内部的流控信号，对外统一映射为
// 503 与稳定中文文案（见 http.go 的 `frameWaitErrorText`）。
var (
	// ErrCaptureBusy 表示容量 1 的请求通道已有 outstanding 请求：并发调用
	// 方（或并发端点）撞车时立即失败，绝不排队放大延迟。
	ErrCaptureBusy = errors.New("devcapture: 已有捕获请求在执行")

	// ErrFrameWaitTimeout 表示在等待上限内未见交付：帧循环已停止或长期
	// 阻塞，请求按有界失败收敛。
	ErrFrameWaitTimeout = errors.New("devcapture: 等待捕获交付超时")
)

// StatusSource 是 `/status` 读取客户端侧状态的最小注入面。接口在本包声明、
// 由 main 装配时以小适配器实现——`Application` 的窗口尺寸可经其导出访问器
// 取得（`Window().ContentSize()`）；相位当前没有现成的并发安全访问器，实现
// 可以返回空串（`/status` 显示 unknown），待 app 侧补齐后再如实接线。
//
// 并发注意：app 状态由帧循环 goroutine 写入，实现方必须经同步访问器或原子
// 快照返回，不得裸读无同步字段——本包不会为读数引入跨 goroutine 数据竞争。
type StatusSource interface {
	// Phase 返回当前阶段的人类可读描述（如 menu/game）；空串表示未知。
	Phase() string
	// WindowWidth 返回当前窗口内容宽度；非正值表示未知。
	WindowWidth() int
	// WindowHeight 返回当前窗口内容高度；非正值表示未知。
	WindowHeight() int
}

// Options 是 `New` 的装配参数。零值即可用（默认地址、真实时钟、不注入状态
// 源、端口文件走默认路径）。
type Options struct {
	// Status 供 `/status` 读取相位与窗口尺寸；可为 nil（phase 显示 unknown）。
	Status StatusSource

	// Addr 是监听地址，空值取 `DefaultAddr`。只应配置回环地址；端口被占时
	// 自动顺延（见 `Start`）。
	Addr string

	// PortFilePath 是端口发现文件路径，空值取 `~/.mornlea/dev-capture.json`。
	// 测试注入临时目录路径，绝不触碰真实用户目录。
	PortFilePath string

	// Now 返回当前时间；nil 用 `time.Now`。录制时间戳与截止检查都经它，
	// 测试注入假时钟获得确定性。
	Now func() time.Time

	// Sleep 挂起当前 goroutine；nil 用 `time.Sleep`。录制帧间隔等待经它，
	// 测试注入以避免真实等待。
	Sleep func(time.Duration)
}

// captureSummary 是最近一次捕获交付的摘要（`/status` 的 last_capture 来源）。
// 零值的 `at` 表示从未发生捕获。
type captureSummary struct {
	at      time.Time
	width   int
	height  int
	errText string
}

// Service 是本地捕获服务：实现 `app.CaptureCoordinator` 供 main 注入 app，
// 同时持有 HTTP 面与端口发现文件的生命周期。零值不可用，经 `New` 构造。
type Service struct {
	// status 为注入的 app 状态只读面，可为 nil。
	status StatusSource

	// addr 是请求的监听地址（已应用默认值）。
	addr string

	// requests 是泵与 HTTP 侧的交接通道。容量 1 即单 outstanding 语义：
	// 帧循环侧契约最简（每帧至多一次捕获、永不排队），并发请求方以
	// `ErrCaptureBusy` 立即失败而不是排队。
	requests chan app.CaptureRequest

	// frameWait 是单帧交付的等待上限，测试可收短以加速超时路径。
	frameWait time.Duration

	// now 与 sleep 抽出时间源，测试注入假时钟避免真实等待。
	now   func() time.Time
	sleep func(time.Duration)

	// portFilePath 是 Options 注入的发现文件路径；空表示用默认路径
	// （延迟到 `Start` 解析，构造期不做 I/O）。resolvedPortFile 是 `Start`
	// 实际写入的路径，`Stop` 按它清理。
	portFilePath     string
	resolvedPortFile string

	mu sync.Mutex
	// lastCapture 是最近一次交付的摘要；`at` 为零值表示从未捕获。
	lastCapture captureSummary
	// recording 标记 `/record` 采样进行中（互斥防止并发录制）。
	recording bool
	// dropped 计数交付缓冲满而丢弃的 outcome：这只在消费者已超时离开等
	// 异常路径出现，正常交接中交付缓冲必有空位。
	dropped uint64
	// port 是 `Start` 绑定的实际端口；未运行为 0。
	port int
	// server 与 listener 在 `Start` 后非 nil、`Stop` 后归位 nil。
	server   *http.Server
	listener net.Listener
}

// `Service` 必须持续满足 app 侧的消费端契约：接口漂移在编译期暴露。
var _ app.CaptureCoordinator = (*Service)(nil)

// New 按装配参数构造 `Service`。只做内存初始化，不监听、不写文件；生命周期
// 由 `Start`/`Stop` 显式驱动。
func New(opts Options) *Service {
	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Service{
		status:       opts.Status,
		addr:         addr,
		requests:     make(chan app.CaptureRequest, 1),
		frameWait:    defaultFrameWait,
		now:          now,
		sleep:        sleep,
		portFilePath: opts.PortFilePath,
	}
}

// PendingCapture 返回一个待执行捕获请求。必须非阻塞（`app.CaptureCoordinator`
// 契约）：帧循环每帧至多调用一次，无待办时立即返回零值与 false，不做任何
// 等待——这是「空闲帧零开销」的全部成本所在。
func (s *Service) PendingCapture() (app.CaptureRequest, bool) {
	select {
	case req := <-s.requests:
		return req, true
	default:
		return app.CaptureRequest{}, false
	}
}

// CompleteCapture 交付一次捕获的结果。必须非阻塞（`app.CaptureCoordinator`
// 契约）：交付通道容量 1，投递走非阻塞 send——正常路径中 HTTP handler 正在
// 接收、缓冲必有空位；缓冲已满说明消费者已超时离开（或请求为零值），丢弃并
// 计数，绝不以任何形式的等待反压帧循环。像素所有权自本调用起移交本包
// goroutine（发送成功即不可变），编码从此处开始、全部离开帧循环线程。
//
// 无论投递还是丢弃，结果都进入最近捕获摘要：`/status` 的观察者靠它发现
// 帧循环侧的异常（如捕获不可用）。
func (s *Service) CompleteCapture(req app.CaptureRequest, pixels []byte, width, height int, err error) {
	s.recordLastCapture(width, height, err)
	outcome := app.CaptureOutcome{Pixels: pixels, Width: width, Height: height, Err: err}
	select {
	case req.Done <- outcome:
	default:
		s.mu.Lock()
		s.dropped++
		s.mu.Unlock()
	}
}

// requestFrame 由 HTTP handler 侧发起一帧捕获并等待交付。通道满以
// `ErrCaptureBusy` 立即失败（单 outstanding 的流控语义）；等待上限
// `frameWait` 或客户端断开都会终止等待，帧循环停止时请求因此有界收敛。
func (s *Service) requestFrame(ctx context.Context, wait time.Duration) (app.CaptureOutcome, error) {
	done := make(chan app.CaptureOutcome, 1)
	select {
	case s.requests <- app.CaptureRequest{Done: done}:
	default:
		return app.CaptureOutcome{}, ErrCaptureBusy
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome, nil
	case <-timer.C:
		return app.CaptureOutcome{}, ErrFrameWaitTimeout
	case <-ctx.Done():
		return app.CaptureOutcome{}, fmt.Errorf("devcapture: 请求已取消: %w", ctx.Err())
	}
}

// recordLastCapture 在互斥锁内更新最近捕获摘要。时间取注入时钟，测试可
// 断言；互斥锁是固定开销，不构成对帧循环的反压。
func (s *Service) recordLastCapture(width, height int, err error) {
	summary := captureSummary{at: s.now(), width: width, height: height}
	if err != nil {
		summary.errText = captureErrorText(err)
	}
	s.mu.Lock()
	s.lastCapture = summary
	s.mu.Unlock()
}

// lastCaptureSnapshot 返回最近捕获摘要的拷贝（`/status` 使用）。
func (s *Service) lastCaptureSnapshot() captureSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCapture
}

// droppedOutcomes 返回满缓冲丢弃计数（测试断言用）。
func (s *Service) droppedOutcomes() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

// recordingActive 报告录制是否进行中（`/status` 与测试使用）。
func (s *Service) recordingActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recording
}

// tryBeginRecording 占用录制互斥标志：已有录制时返回 false，并发录制请求
// 以 503 失败而不是交叠采样。
func (s *Service) tryBeginRecording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recording {
		return false
	}
	s.recording = true
	return true
}

// endRecording 释放录制互斥标志。
func (s *Service) endRecording() {
	s.mu.Lock()
	s.recording = false
	s.mu.Unlock()
}

// Port 返回 `Start` 绑定的实际端口；未运行时为 0（`/status` 使用）。
func (s *Service) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Start 绑定监听、写入端口发现文件并在后台开始服务，返回实际监听地址。
//
// 幂等防线先行：重复 Start 在任何副作用（顺延绑定、写端口文件）发生之前
// 失败——否则第二个监听会以顺延后的新端口覆写发现文件，留下指向死端口的
// 条目且无任何失败信号。
//
// 端口策略：自请求端口起逐个 +1 顺延（至多 `maxBindAttempts` 次），既容忍
// 偶发占用，也让发现文件的消费者拿到的端口始终可预测；仅「地址被占」参与
// 顺延，其余绑定错误立即失败。请求端口为 0（内核随机分配）时无顺延语义。
// 发现文件写入失败视为启动失败：发现机制半失效比拒绝启动更难排查。
func (s *Service) Start() (string, error) {
	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		return "", errors.New("devcapture: 服务已启动")
	}
	s.mu.Unlock()
	listener, err := s.listen()
	if err != nil {
		return "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	path := s.portFilePath
	if path == "" {
		path, err = DefaultPortFilePath()
		if err != nil {
			_ = listener.Close()
			return "", err
		}
	}
	startedAt := s.now()
	if err := writePortFile(path, portFileData{PID: os.Getpid(), Port: port, StartedAt: startedAt}); err != nil {
		_ = listener.Close()
		return "", err
	}
	server := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	s.mu.Lock()
	s.port = port
	s.resolvedPortFile = path
	s.server = server
	s.listener = listener
	s.mu.Unlock()

	go func() { _ = server.Serve(listener) }()
	return listener.Addr().String(), nil
}

// listen 按顺延策略绑定回环监听（见 `Start`）。回环约束是 spec 的 MUST：
// 默认值只落在 `127.0.0.1`，但 options 层传入的任何地址都过这道防御闸——
// 非回环主机在绑定之前即以明确错误拒绝，绝不让调试服务暴露到回环之外。
func (s *Service) listen() (net.Listener, error) {
	host, portRaw, err := net.SplitHostPort(s.addr)
	if err != nil {
		return nil, fmt.Errorf("devcapture: 解析监听地址 %q 失败: %w", s.addr, err)
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
	default:
		return nil, fmt.Errorf("devcapture: 监听地址 %q 的主机 %q 不是回环地址：捕获服务只允许绑定回环（127.0.0.1、::1 或 localhost）", s.addr, host)
	}
	base, err := strconv.Atoi(portRaw)
	if err != nil || base < 0 {
		return nil, fmt.Errorf("devcapture: 监听地址 %q 的端口无效", s.addr)
	}
	attempts := maxBindAttempts
	if base == 0 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		candidate := net.JoinHostPort(host, strconv.Itoa(base+i))
		listener, listenErr := net.Listen("tcp", candidate)
		if listenErr == nil {
			return listener, nil
		}
		lastErr = listenErr
		if !errors.Is(listenErr, syscall.EADDRINUSE) {
			break
		}
	}
	return nil, fmt.Errorf("devcapture: 绑定 %q 失败（含顺延重试）: %w", s.addr, lastErr)
}

// Stop 优雅关闭服务并清除端口发现文件。幂等：未启动、重复调用都返回 nil。
// 关闭带 `shutdownTimeout` 上限：超时即返回，未排空的在途连接不会被强制
// 断开，兜底是进程退出（本地工具的请求都有毫秒级上限，正常到不了这一步）。
func (s *Service) Stop() error {
	s.mu.Lock()
	server, listener, path := s.server, s.listener, s.resolvedPortFile
	s.server, s.listener, s.resolvedPortFile = nil, nil, ""
	s.port = 0
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	err := server.Shutdown(ctx)
	if listener != nil {
		// `Shutdown` 已关闭监听；这里的 Close 兜底超时路径，幂等。
		_ = listener.Close()
	}
	removePortFile(path)
	return err
}

// captureErrorText 把捕获终态错误映射为稳定中文文案。`client.ErrCaptureUnavailable`
// 是运行期预期条件（授权缺失、窗口不可捕获），给观察者可操作的指引；其余
// 错误保留原始文本，不吞不洗。
func captureErrorText(err error) string {
	if errors.Is(err, client.ErrCaptureUnavailable) {
		return "窗口合成捕获不可用：请检查系统设置的屏幕录制授权后重试"
	}
	return "捕获失败：" + err.Error()
}
