//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/internal/worldgen"
)

// applicationReceiverCapacity 是客户端接收服务端消息的缓冲上限。登录成功后
// 服务端立即按 `server.Config.SnapshotChunks`(默认 64 条/ tick)推送初始快照,
// 视距 32 时快照约 4489 条 `ChunkSnapshot`,而窗口/渲染器初始化(冷启动还
// 含着色器编译)需要数十秒,启动期没有消费方——旧的 256 容量会在构造完成前
// 溢出,把「启动慢」误判成「consumer too slow」并关掉会话(表现为启动后闪
// 退)。容量按«全量快照 + 一分钟的每 tick 状态»留足余量;运行期 fail-fast 语义
// 不变:消费者仍不能落后太多,8192 条 ≈ 128 个权威 tick(约 2 秒)的积压。
const applicationReceiverCapacity = 8192

func openApplicationStore(
	ctx context.Context,
	options applicationOptions,
) (storage.WorldStore, error) {
	if options.Connect != "" {
		return nil, nil
	}
	metadata := storage.Metadata{
		FormatVersion:  2,
		Seed:           options.Seed,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
	}
	// benchmark 与 capture 都要求世界状态与本机磁盘上的真实存档隔离：
	// benchmark 为了性能测量不被磁盘 I/O 干扰，capture 为了抓帧结果不随
	// "这台机器碰巧玩到哪一步"漂移——两者都复用内存 store 达成确定性初始状态。
	if options.Benchmark || options.CaptureDir != "" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return storage.NewMemory(metadata), nil
	}
	return storage.OpenDisk(ctx, options.WorldPath, storage.OpenOptions{Create: metadata})
}

// buildApplicationServerConfig 依据 applicationOptions 构建本地权威服务端的
// server.Config。供 newApplicationWithDependencies（非菜单路径）与 startWorld
// （StartAtMenu 延迟装配）共用，保证两条路径产出同一份服务端配置（伙伴、
// 模型运行时、视距半径、性能观察器）。
func buildApplicationServerConfig(options applicationOptions, ticks *tickRecorder, saves *saveRecorder) server.Config {
	config := server.DefaultConfig(options.Seed)
	config.Companions = slices.Clone(options.Companions)
	// 模型运行时与已解析密钥原样转发：均为值拷贝，不与 options 共享引用；
	// NewHost 会在打开存档前对非空伙伴列表做完整性校验。
	config.AIModel = options.AIModel
	config.AIAPIKey = options.AIAPIKey
	config.ViewRadius = options.Render.ViewDistance + 1
	config.TrustedObserver = options.Benchmark
	config.TickObserver = ticks.add
	if saves != nil {
		config.SaveObserver = saves.add
	}
	return config
}

func newApplication(options applicationOptions) (*application, error) {
	return newApplicationWithDependencies(options, defaultApplicationDependencies())
}

func newApplicationWithDependencies(
	options applicationOptions,
	dependencies applicationDependencies,
) (*application, error) {
	// options.Render 的零值是一个静默退化的配置：ViewDistance=0 会让
	// DropOutside 在每帧把中心区块外的一切都丢弃。真实入口（cmd/mornlea/main.go）
	// 总是先经 resolveConfig 填好这个字段，这里只防漏填 Render 的调用方
	// （包括测试）静默跑在退化配置下而不报错。
	if options.Render.ViewDistance == 0 {
		options.Render = config.Defaults().Render
	}
	// `applicationOptions` 也可能由测试或其他包内装配点直接构造，不能假设
	// 一定经过 `config.Load`。空窗口值按缺省预设补齐；其余三项原始设置在
	// registry、音频、窗口与 UI 资源产生任何副作用前复用设置页同一校验。
	if options.WindowSize == "" {
		options.WindowSize = config.WindowSize1280x720
	}
	initialSettings := settingsValues{
		audioVolume:     options.AudioVolume,
		texturePackPath: options.TexturePackPath,
		windowSize:      options.WindowSize,
	}
	if err := initialSettings.validate(); err != nil {
		return nil, fmt.Errorf("校验客户端设置: %w", err)
	}
	if dependencies.newGlyphAtlas == nil {
		dependencies.newGlyphAtlas = render.NewGlyphAtlasWithSink
	}
	if dependencies.newWindowedRenderer == nil {
		dependencies.newWindowedRenderer = defaultApplicationDependencies().newWindowedRenderer
	}
	if dependencies.newOffscreenRenderer == nil {
		dependencies.newOffscreenRenderer = client.NewRenderer
	}
	if dependencies.newRegistry == nil {
		dependencies.newRegistry = defaultApplicationDependencies().newRegistry
	}
	if dependencies.loadConfig == nil {
		dependencies.loadConfig = config.Load
	}
	if dependencies.saveConfig == nil {
		dependencies.saveConfig = func(cfg config.Config, path string) error { return cfg.Save(path) }
	}
	reg, registryErr := dependencies.newRegistry(options.ResolvedTexturePackPath)
	if registryErr != nil {
		return nil, fmt.Errorf("加载材质包 %q: %w", options.ResolvedTexturePackPath, registryErr)
	}
	ctx := context.Background()
	var store storage.WorldStore
	var clientEndpoint network.ClientEndpoint
	var running *server.Server
	var host applicationHost
	var serverCancel context.CancelFunc
	var serverDone chan error
	var err error
	// worldSeed 是登录成功应答携带的权威世界种子(uint64 全值域,0 合法):
	// 单机与 TCP 远程都在登录装配点取得,benchmark 观察者不消费(不建远环
	// Scheduler)。它是运行时事实而非配置,只在装配链路内传递。
	var worldSeed uint64
	ticks, saves := newPerformanceRecorders(options.Benchmark)
	// StartAtMenu（交互本地）在此跳过世界装配：服务端配置、store 打开、Host
	// 与登录都延迟到「进入游戏」，由 startWorld 在菜单相位之后一次性完成。
	// 其余路径在此构建与引入菜单前逐字相同的服务端配置（buildApplicationServerConfig
	// 只是把既有装配常量收拢为一个函数，无行为变化）。
	menuMode := options.StartAtMenu
	var config server.Config
	if !menuMode {
		config = buildApplicationServerConfig(options, ticks, saves)
	}
	if options.Connect != "" {
		if options.Identity == nil {
			return nil, errors.New("远程连接缺少本机身份")
		}
		stream, err := dependencies.dialTCP(ctx, options.Connect)
		if err != nil {
			return nil, fmt.Errorf("连接远程服务器: %w", err)
		}
		clientEndpoint, worldSeed, err = dependencies.loginClient(ctx, stream, *options.Identity)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("远程登录: %w", err), stream.Close())
		}
	} else if !menuMode {
		store, err = dependencies.openStore(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("打开世界存储: %w", err)
		}
	}
	if options.Benchmark {
		running = server.NewWorld(config, worldgen.New(store.Metadata().Seed, options.FluidEnabled), store)
		clientEndpoint, err = assembleBenchmarkObserverConnection(
			ctx, running, options.BenchmarkTransport, uint64(store.Metadata().Seed),
			func(address string) (network.Listener, error) {
				return network.ListenTCP(address)
			},
			dependencies.dialTCP,
		)
		if err != nil {
			_ = store.Close()
			return nil, err
		}
		serverContext, cancel := context.WithCancel(context.Background())
		serverCancel = cancel
		serverDone = make(chan error, 1)
		go func() { serverDone <- running.Run(serverContext) }()
	} else if options.Connect == "" && !menuMode {
		if options.Identity == nil {
			_ = store.Close()
			return nil, errors.New("本地世界缺少本机身份")
		}
		var localSeed uint64
		clientEndpoint, host, serverCancel, serverDone, localSeed, err = assembleLocalApplicationConnection(
			ctx,
			config,
			worldgen.New(store.Metadata().Seed, options.FluidEnabled),
			store,
			*options.Identity,
			dependencies,
		)
		if err != nil {
			return nil, fmt.Errorf("连接本地 Host: %w", err)
		}
		// 单机登录走与远程相同的 LoginSuccess 状态机,种子由服务端从存档
		// metadata 填入;远环播种只认登录应答,不直接读 store,保证单机与
		// TCP 远程同一条种子链路。
		worldSeed = localSeed
	}
	var receiver *client.Receiver
	if !menuMode {
		receiver = client.NewReceiver(clientEndpoint, applicationReceiverCapacity)
	}

	var window applicationWindow
	var rustRenderer *client.Renderer
	// 交互渲染的目标物理帧缓冲(见 `fitFramebuffer` 注释);benchmark 离屏
	// 渲染沿用同一基准分辨率并在报告中固定钉住。
	width, height := interactiveFramebufferWidth, interactiveFramebufferHeight
	headless := options.Benchmark || options.CaptureDir != ""
	if options.CaptureDir != "" {
		width, height = captureWidth, captureHeight
	}
	var playCue func(audio.Cue)
	var closeAudio func()
	if !headless && dependencies.newAudioPlayer != nil {
		playCue, closeAudio = dependencies.newAudioPlayer(options.AudioVolume)
	}
	if headless {
		rustRenderer, err = dependencies.newOffscreenRenderer(width, height)
	} else {
		// 初始逻辑尺寸来自三个固定 16:9 预设；Rust 先按显示器工作区钳制，
		// 创建后 `fitFramebuffer` 再只缩不放大地守住物理帧缓冲上限。
		logicalWidth, logicalHeight := options.WindowSize.Dimensions()
		window, err = dependencies.newWindow(logicalWidth, logicalHeight, applicationWindowTitle)
		if err == nil {
			fitFramebuffer(window, interactiveFramebufferWidth, interactiveFramebufferHeight)
			width, height = window.FramebufferSize()
			rustRenderer, err = dependencies.newWindowedRenderer(window)
		}
	}
	if err != nil {
		if closeAudio != nil {
			closeAudio()
		}
		if rustRenderer != nil {
			rustRenderer.Close()
		}
		if window != nil {
			window.Close()
		}
		var connectionErr error
		if receiver != nil {
			connectionErr = receiver.Close()
		}
		if serverCancel != nil {
			serverCancel()
		}
		if serverDone != nil {
			connectionErr = errors.Join(connectionErr, <-serverDone)
		}
		return nil, errors.Join(err, connectionErr)
	}

	camera := client.Camera{
		Pos:    mgl32.Vec3{0, 110, 0},
		Pitch:  -0.25,
		FovY:   mgl32.DegToRad(options.Render.FovDegrees),
		Aspect: float32(width) / float32(height),
		Near:   0.1,
		Far:    2000,
	}
	// StartAtMenu 构造菜单相位，其余路径保持游戏相位（menuPhaseGame 为零值）。
	appMenuPhase := menuPhaseGame
	if menuMode {
		appMenuPhase = menuPhaseMenu
	}
	app := &application{
		window:          window,
		renderer:        rustRenderer,
		frameWidth:      width,
		frameHeight:     height,
		clientEndpoint:  clientEndpoint,
		receiver:        receiver,
		server:          running,
		host:            host,
		serverCancel:    serverCancel,
		serverDone:      serverDone,
		mirror:          client.NewMirror(),
		itemDrops:       client.NewItemDrops(),
		inventorySource: -1,
		predictor:       client.NewPredictor(),
		remotePlayers:   client.NewRemotePlayers(),
		companions:      &client.Companions{},
		chatEvents:      &client.ChatEvents{},
		remoteNameTags:  make([]render.NameTag, 0, maxFrameNameTags),
		camera:          camera,
		center:          cameraChunk(camera.Pos),
		loadedChunks:    make(map[core.ChunkPos]struct{}),
		ticks:           ticks,
		saves:           saves,
		render:          options.Render,
		benchmarkTransport: func() string {
			if options.BenchmarkTransport == "" {
				return "memory"
			}
			return options.BenchmarkTransport
		}(),
		playCue:        playCue,
		closeAudio:     closeAudio,
		startupOptions: options,
		startupDeps:    dependencies,
		menu: menuState{
			phase:   appMenuPhase,
			title:   "Mornlea",
			version: menuVersion(),
		},
		settings: settingsState{committed: initialSettings, draft: initialSettings},
	}
	app.releaseResources = app.releaseOwnedResources
	// 材质与 HUD 图集一次性上传;mesh 上传调度经 SectionScheduler 下沉。
	atlasLayers, atlasPixels := reg.AtlasPixels()
	rustRenderer.UploadAtlas(atlasLayers, atlasPixels)
	app.scheduler = render.NewSectionScheduler(rustRenderer, applicationUploadPerFrame)
	app.glyphAtlas, err = dependencies.newGlyphAtlas(rendererGlyphSink{renderer: rustRenderer})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("创建字形图集: %w", err), app.Close())
	}
	app.nameTagRenderer = render.NewNameTagLayouter(app.glyphAtlas)
	app.hotbarRenderer = hud.NewHotbarLayout(app.glyphAtlas, reg)
	hudWidth, hudHeight, hudPixels := app.hotbarRenderer.AtlasPixels()
	rustRenderer.UploadHUDAtlas(hudWidth, hudHeight, hudPixels)
	// 菜单字体一次性上传：交互与 capture 路径（未 benchmark）在渲染器创建后上传内嵌
	// Noto CJK，供 egui 主菜单渲染（spec「菜单字体只经 ABI 上传一次」）；benchmark 不参与
	// 菜单、零上传。上传发生在任何 UI 段被渲染之前，否则 Rust 侧会以编程错误拒绝 UI 帧。
	if !options.Benchmark {
		rustRenderer.UploadUIFont(render.EmbeddedCJKFont())
	}
	app.configPath = options.ConfigPath
	if options.Dev {
		app.debugPanelRenderer = render.NewDebugPanelLayouter(app.glyphAtlas)
		// 面板的初始生效值取当前已生效的 physics/sim 快照（main.go 在构造
		// application 之前已经调用过 config.Config.Apply）与调用方传入的
		// Render，三组合起来与启动时真正生效的参数保持一致，不需要额外
		// 传一份完整 config.Config 进 applicationOptions。
		app.panel = newPanelStateFromActive(options.Render)
	}
	app.mesher = client.NewMesher(reg, max(1, runtime.NumCPU()-2))
	// 远环 LOD 接线:登录种子→Scheduler 播种远环带→雾距离按配置推导下发。
	// 禁用(lodEnabled=false)与 benchmark 观察者路径在此零参与。
	if !menuMode {
		if err := app.attachLodScheduler(worldSeed, options.FluidEnabled, options.Benchmark); err != nil {
			return nil, errors.Join(fmt.Errorf("接线远环 LOD: %w", err), app.Close())
		}
	}
	if options.Benchmark {
		if err := app.requestTrustedObserverCenter(app.center); err != nil {
			cleanupErr := app.Close()
			return nil, errors.Join(
				fmt.Errorf("设置初始 trusted observer 中心: %w", err),
				cleanupErr,
			)
		}
	}
	return app, nil
}

// startWorld 在「进入游戏」点击后执行延迟的世界装配：打开世界存储、启动本地
// 权威服务端、完成登录并把远环 LOD 播种器接线到登录种子。复用既有
// openApplicationStore/assembleLocalApplicationConnection/attachLodScheduler 与既有
// 错误包装；成功设置相位 menuPhaseGame 与 starting=false，失败返回 error（相位仍
// 由调用方保持菜单并显示错误文本）。菜单构造阶段已存 startupOptions/startupDeps
// 快照，此处用同一份配置与注入载体，保证与既有路径产出相同的服务端状态。
func (a *application) startWorld() error {
	options := a.startupOptions
	dependencies := a.startupDeps
	ctx := context.Background()
	config := buildApplicationServerConfig(options, a.ticks, a.saves)

	store, err := dependencies.openStore(ctx, options)
	if err != nil {
		return fmt.Errorf("打开世界存储: %w", err)
	}
	if options.Identity == nil {
		_ = store.Close()
		return errors.New("本地世界缺少本机身份")
	}
	endpoint, host, serverCancel, serverDone, localSeed, err := assembleLocalApplicationConnection(
		ctx,
		config,
		worldgen.New(store.Metadata().Seed, options.FluidEnabled),
		store,
		*options.Identity,
		dependencies,
	)
	if err != nil {
		return fmt.Errorf("连接本地 Host: %w", err)
	}
	worldSeed := localSeed
	receiver := client.NewReceiver(endpoint, applicationReceiverCapacity)

	a.clientEndpoint = endpoint
	a.host = host
	a.serverCancel = serverCancel
	a.serverDone = serverDone
	a.receiver = receiver

	if err := a.attachLodScheduler(worldSeed, options.FluidEnabled, options.Benchmark); err != nil {
		cleanupErr := a.releaseWorldConnection(config.ShutdownTimeout)
		return errors.Join(fmt.Errorf("接线远环 LOD: %w", err), cleanupErr)
	}

	a.menu.phase = menuPhaseGame
	a.menu.starting = false
	return nil
}

// releaseWorldConnection 在 startWorld 中途失败时释放已装配的半成品连接资源：
// 关闭客户端接收器、取消服务端 Host、等待 hostDone 并 Shutdown host（host 拥有
// store），随后把连接字段复位为 nil，使菜单相位可重试且不占用 clientCloseOnce。
func (a *application) releaseWorldConnection(shutdownTimeout time.Duration) error {
	var err error
	if a.receiver != nil {
		err = errors.Join(err, a.receiver.Close())
	}
	if a.serverCancel != nil {
		a.serverCancel()
	}
	if a.serverDone != nil {
		err = errors.Join(err, ignoreApplicationStartupCloseError(<-a.serverDone))
	}
	if a.host != nil {
		err = errors.Join(err, shutdownApplicationHost(a.host, shutdownTimeout))
	}
	a.clientEndpoint = nil
	a.host = nil
	a.serverCancel = nil
	a.serverDone = nil
	a.receiver = nil
	return err
}

// applicationUploadPerFrame 是每帧 mesh 上传字节预算,与旧渲染器默认一致。
const applicationUploadPerFrame = 4 << 20

// rendererGlyphSink 把字形矩形写入适配到 Rust 渲染器上传入口。
type rendererGlyphSink struct{ renderer *client.Renderer }

// WriteGlyphRect 直传 client 渲染器的字形图集入口。
func (sink rendererGlyphSink) WriteGlyphRect(x, y, width, height uint32, pixels []byte) {
	sink.renderer.UploadGlyphRect(int(x), int(y), int(width), int(height), pixels)
}

// assembleBenchmarkObserverConnection 为 benchmark 观察者建立连接。内存
// 传输走 AttachTrustedObserver 旁路；TCP 传输复用与真实客户端相同的登录
// 状态机，worldSeed 与被观测世界同源，保证 LoginSuccess 与生产路径一致。
func assembleBenchmarkObserverConnection(
	ctx context.Context,
	running *server.Server,
	transport string,
	worldSeed uint64,
	listenTCP func(string) (network.Listener, error),
	dialTCP func(context.Context, string) (network.ClientPacketStream, error),
) (network.ClientEndpoint, error) {
	if transport == "" || transport == "memory" {
		clientEndpoint, serverEndpoint := network.NewMemoryPair(256)
		if err := running.AttachTrustedObserver(serverEndpoint); err != nil {
			_ = clientEndpoint.Close()
			return nil, err
		}
		return clientEndpoint, nil
	}
	if transport != "tcp" {
		return nil, fmt.Errorf("不支持 benchmark transport %q", transport)
	}
	listener, err := listenTCP("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	acceptContext, cancelAccept := context.WithCancel(ctx)
	defer cancelAccept()
	serverDone := make(chan error, 1)
	go func() {
		stream, acceptErr := listener.Accept(acceptContext)
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		pending, loginErr := network.BeginServerLogin(acceptContext, stream, worldSeed)
		if loginErr == nil {
			loginErr = pending.Accept(acceptContext, running.AttachTrustedObserver)
		}
		serverDone <- loginErr
	}()
	stream, err := dialTCP(ctx, listener.Addr())
	if err != nil {
		cancelAccept()
		closeErr := listener.Close()
		return nil, errors.Join(err, closeErr, <-serverDone)
	}
	identity := network.Identity{
		PlayerID:    core.PlayerID{0x2c, 0xad, 0xe1, 0x90, 0x9d, 0xb6, 0x43, 0x82, 0x8d, 0x31, 0xcb, 0x40, 0xe5, 0xbb, 0x52, 0x29},
		DisplayName: "Benchmark",
	}
	endpoint, loginErr := network.LoginClient(ctx, stream, identity)
	serverErr := <-serverDone
	if err := errors.Join(loginErr, serverErr); err != nil {
		_ = stream.Close()
		return nil, err
	}
	return endpoint, nil
}

type applicationLoginResult struct {
	endpoint  network.ClientEndpoint
	worldSeed uint64
	err       error
}

// assembleLocalApplicationConnection 建立单机模式的全套连接:内嵌 Host、
// 内存流对与登录状态机。返回值含登录成功应答的 WorldSeed(远环 LOD 的
// 播种输入),与 TCP 远程路径同一来源。
func assembleLocalApplicationConnection(
	ctx context.Context,
	config server.Config,
	generator server.Generator,
	store storage.WorldStore,
	identity network.Identity,
	dependencies applicationDependencies,
) (
	network.ClientEndpoint,
	applicationHost,
	context.CancelFunc,
	chan error,
	uint64,
	error,
) {
	host, err := dependencies.newHost(ctx, config, generator, store)
	if err != nil {
		return nil, nil, nil, nil, 0, errors.Join(err, store.Close())
	}
	hostContext, cancel := context.WithCancel(ctx)
	hostDone := make(chan error, 1)
	go func() { hostDone <- host.Run(hostContext, nil) }()

	clientStream, serverStream, err := dependencies.newMemoryStreamPair(256)
	if err != nil {
		cleanupErr := cleanupLocalApplicationStartup(
			host, cancel, hostDone, nil, nil, nil, config.ShutdownTimeout,
		)
		return nil, nil, nil, nil, 0, errors.Join(err, cleanupErr)
	}
	acceptDone := make(chan error, 1)
	go func() { acceptDone <- host.AcceptStream(hostContext, serverStream) }()
	loginDone := make(chan applicationLoginResult, 1)
	go func() {
		endpoint, worldSeed, loginErr := dependencies.loginClient(hostContext, clientStream, identity)
		loginDone <- applicationLoginResult{endpoint: endpoint, worldSeed: worldSeed, err: loginErr}
	}()

	select {
	case result := <-loginDone:
		if result.err == nil {
			return result.endpoint, host, cancel, hostDone, result.worldSeed, nil
		}
		cleanupErr := cleanupLocalApplicationStartup(
			host,
			cancel,
			hostDone,
			acceptDone,
			clientStream,
			serverStream,
			config.ShutdownTimeout,
		)
		return nil, nil, nil, nil, 0, errors.Join(result.err, cleanupErr)
	case hostErr := <-hostDone:
		_ = clientStream.Close()
		_ = serverStream.Close()
		cancel()
		result := <-loginDone
		acceptErr := <-acceptDone
		shutdownErr := shutdownApplicationHost(host, config.ShutdownTimeout)
		return nil, nil, nil, nil, 0, errors.Join(
			hostErr,
			result.err,
			ignoreApplicationStartupCloseError(acceptErr),
			shutdownErr,
		)
	case acceptErr := <-acceptDone:
		_ = clientStream.Close()
		_ = serverStream.Close()
		cancel()
		result := <-loginDone
		hostErr := <-hostDone
		shutdownErr := shutdownApplicationHost(host, config.ShutdownTimeout)
		return nil, nil, nil, nil, 0, errors.Join(
			acceptErr,
			result.err,
			ignoreApplicationStartupCloseError(hostErr),
			shutdownErr,
		)
	}
}

func cleanupLocalApplicationStartup(
	host applicationHost,
	cancel context.CancelFunc,
	hostDone <-chan error,
	acceptDone <-chan error,
	clientStream network.ClientPacketStream,
	serverStream network.ServerPacketStream,
	shutdownTimeout time.Duration,
) error {
	var cleanupErr error
	if clientStream != nil {
		cleanupErr = errors.Join(cleanupErr, clientStream.Close())
	}
	if serverStream != nil {
		cleanupErr = errors.Join(cleanupErr, serverStream.Close())
	}
	cancel()
	if acceptDone != nil {
		cleanupErr = errors.Join(
			cleanupErr,
			ignoreApplicationStartupCloseError(<-acceptDone),
		)
	}
	cleanupErr = errors.Join(
		cleanupErr,
		ignoreApplicationStartupCloseError(<-hostDone),
		shutdownApplicationHost(host, shutdownTimeout),
	)
	return cleanupErr
}

func shutdownApplicationHost(host applicationHost, timeout time.Duration) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return host.Shutdown(shutdownContext)
}

func ignoreApplicationStartupCloseError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, network.ErrClosed) {
		return nil
	}
	return err
}

// interactiveFramebufferWidth/Height 是交互渲染的目标物理帧缓冲分辨率:
// `fitFramebuffer` 只在帧缓冲超过它时收缩,绝不放大。
const (
	interactiveFramebufferWidth  = 2560
	interactiveFramebufferHeight = 1440
)

// applicationWindowTitle 是窗口中显示的产品名,不带内部里程碑代号。
const applicationWindowTitle = "Mornlea"

// fitFramebuffer 把窗口内容尺寸换算为使物理帧缓冲不多于目标分辨率,但只缩不
// 放大:屏幕可用区域不足(1x 屏/小屏)时保持创建尺寸,绝不把窗口撑出屏幕。
// 两轴都不变时同样跳过,不做无意义的 Resize 与 `Poll`。
func fitFramebuffer(window applicationWindow, targetWidth, targetHeight int) {
	contentWidth, contentHeight := window.ContentSize()
	framebufferWidth, framebufferHeight := window.FramebufferSize()
	if framebufferWidth <= 0 || framebufferHeight <= 0 {
		return
	}
	wantWidth := max(1, int(math.Round(float64(targetWidth*contentWidth)/float64(framebufferWidth))))
	wantHeight := max(1, int(math.Round(float64(targetHeight*contentHeight)/float64(framebufferHeight))))
	// 任一轴需要放大(目标分辨率在屏内放不下)就整体跳过,避免混合轴缩放
	// 把窗口推出屏幕。
	if wantWidth >= contentWidth || wantHeight >= contentHeight {
		return
	}
	// D-01 的交互窗口只允许 16:9 预设。高缩放显示器上的独立四舍五入可能
	// 产生 853×480 一类一像素漂移，因此向下吸附到最大的 16×9 整数倍；
	// 只会再缩小，不会突破上面的物理像素上限或把窗口推出工作区。
	units := min(wantWidth/16, wantHeight/9)
	if units == 0 {
		return
	}
	wantWidth, wantHeight = units*16, units*9
	window.SetContentSize(wantWidth, wantHeight)
	window.Poll()
}
