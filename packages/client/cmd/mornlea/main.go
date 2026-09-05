//go:build darwin

// Command mornlea 启动 M3B TCP 直连与持久世界客户端。
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"slices"

	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/benchmark"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/capture"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/devcapture"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/logging"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/profile"
)

func init() {
	runtime.LockOSThread()
}

type runDependencies struct {
	newApplication func(application.Options) (*application.Application, error)
	loadIdentity   func(*string) (network.Identity, error)
	runInteractive func(*application.Application) error
	runBenchmark   func(*application.Application, string) error
	runCapture     func(*application.Application, string, bool) error
	// `runMotionDemo` 只服务显式 motion 演示；普通 capture 与游戏不调用。
	runMotionDemo func(*application.Application, string) error
	// `runGoldenUpdateControl` 只服务显式 baseline update；普通 capture 与游戏不调用。
	runGoldenUpdateControl func(*application.Application, *application.Application, string) error
}

func run(args []string) error {
	return runWithDependencies(args, runDependencies{
		newApplication: application.New,
		loadIdentity:   loadApplicationIdentity,
		runInteractive: application.RunInteractive,
		runBenchmark:   benchmark.RunBenchmark,
		// capture 公开入口经其消费端接口 `SceneApplication` 表达；`*application.Application`
		// 隐式实现该接口，这里的适配只为对齐 `runDependencies` 字段的具体签名。
		runCapture: func(app *application.Application, dir string, updateGolden bool) error {
			return capture.RunCapture(app, dir, updateGolden)
		},
		runMotionDemo: func(app *application.Application, outPath string) error {
			return capture.RunBreakBurstMotion(app, outPath)
		},
		runGoldenUpdateControl: func(lodOn, lodOff *application.Application, dir string) error {
			return capture.RunGoldenUpdateControl(lodOn, lodOff, dir)
		},
	})
}

func runWithDependencies(args []string, dependencies runDependencies) error {
	options, err := parseMainOptions(args)
	if err != nil {
		return err
	}
	if !options.Application.Benchmark {
		identity, err := dependencies.loadIdentity(options.RequestedName)
		if err != nil {
			return fmt.Errorf("加载本机 profile: %w", err)
		}
		options.Application.Identity = &identity
	}

	effective, err := resolveConfig(options)
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}
	// 内层 handler 的 Level 固定为 LevelDebug：过滤全部交给 logging 包的包装器，
	// 内层不得二次过滤，否则模块放宽会失效。
	logging.Install(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}), effective.Logging)
	effective.Apply()
	if options.Application.Connect == "" && !options.Application.Benchmark && options.CaptureDir == "" &&
		options.MotionDemoPath == "" {
		// 交互本地客户端启动停留在主菜单，世界装配延迟到「进入游戏」之后。
		options.Application.StartAtMenu = true
		options.Application.Companions = slices.Clone(effective.CompanionDefinitions())
		// Agent 设置与已解析 credential 只注入普通本地模式；远程、benchmark、
		// capture 三条路径不携带本地 Agent 运行时。
		if effective.AI != nil {
			options.Application.AgentService = effective.AI.AgentService
			options.Application.AgentCredential = resolveAgentCredential(effective.AI.AgentService)
			options.Application.TaskTimeoutMinutes = effective.AI.TaskTimeout()
		}
	}
	if remoteTuningDiverges(options, effective) {
		slog.Warn("联机时本机配置改动了 physics/sim：这两组必须与服务端一致，"+
			"否则客户端预测会与权威模拟持续分歧（面板在联机时已锁这两组，配置文件不受该锁约束）",
			"connect", options.Application.Connect)
	}
	// benchmark 不构造面板渲染器：它的产出要与性能基线比对，面板既不该占用
	// GPU 资源，也不该让结果随 --dev 变化。
	//
	// 抓帧相反，必须无条件构造：debug-panel 场景要拍的就是面板本身，而基线
	// 重生成与 CI 调用 capture 时都不会带 --dev。面板默认隐藏，只有该场景的
	// Apply 会把它打开，因此其余场景的画面不受影响。
	options.Application.Dev = (options.Dev || options.CaptureDir != "") &&
		!options.Application.Benchmark
	options.Application.Render = effective.Render
	options.Application.AudioVolume = effective.AudioVolume
	options.Application.TexturePackPath = effective.TexturePackPath
	options.Application.ResolvedTexturePackPath = effective.ResolvedTexturePackPath
	options.Application.WindowSize = effective.WindowSize
	// 注水门控与用户配置的解耦由 resolveConfig 负责：benchmark 与抓帧两条路径
	// 都强制返回 config.Defaults()，因此这里的 effective.FluidEnabled 在这两条
	// 路径上是编译期常量，不会随谁的配置文件漂移。
	//
	// 在此之上再钉死 benchmark 为 false：benchmark 是固定工作负载，它的世界内容
	// 必须与「默认是否注水」这个产品决策解耦，否则翻默认值就会改变性能基线的
	// 被测世界。与 multiplayer_benchmark_server.go 里钉死的 false 是同一套策略。
	//
	// 抓帧刻意**不**钉死，而是跟随编译期默认值：golden 是"玩家默认看到什么"的
	// 视觉门禁，把水排除在外等于让门禁看不见默认开启的主要世界内容。翻默认值
	// 因此必须连带重新生成 golden——这正是期望行为。
	options.Application.FluidEnabled = effective.FluidEnabled && !options.Application.Benchmark
	// ConfigPath 是设置页的配置保存目标；调试面板 F5 保存已被移除
	// （面板不落盘配置），不再需要保存路径的只有不进交互循环的
	// benchmark 与抓帧路径。
	if !options.Application.Benchmark && options.CaptureDir == "" {
		configPath, err := resolveConfigPath(options)
		if err != nil {
			return fmt.Errorf("解析配置文件路径: %w", err)
		}
		options.Application.ConfigPath = configPath
	}

	if options.CaptureDir != "" && options.UpdateGolden {
		// update 必须先用两个 disposable application 比较同一当前材质下的
		// LOD on/off 帧。两者关闭后才创建 fresh application，避免 control
		// 场景留下的相机和镜像状态污染正式场景顺序。
		lodOnOptions := options.Application
		lodOnOptions.Render.LodEnabled = true
		lodOn, err := dependencies.newApplication(lodOnOptions)
		if err != nil {
			return fmt.Errorf("启动 LOD-on 视觉基线 control: %w", err)
		}
		lodOffOptions := lodOnOptions
		lodOffOptions.Render.LodEnabled = false
		lodOff, err := dependencies.newApplication(lodOffOptions)
		if err != nil {
			return errors.Join(
				fmt.Errorf("启动 LOD-off 视觉基线 control: %w", err),
				lodOn.Close(),
			)
		}
		controlErr := dependencies.runGoldenUpdateControl(lodOn, lodOff, options.CaptureDir)
		controlErr = errors.Join(controlErr, lodOff.Close(), lodOn.Close())
		if controlErr != nil {
			return fmt.Errorf("视觉基线近环 control: %w", controlErr)
		}
		fmt.Println("LOD on/off control applications 已关闭，开始写入视觉基线")

		app, err := dependencies.newApplication(lodOnOptions)
		if err != nil {
			return fmt.Errorf("启动正式视觉基线抓帧: %w", err)
		}
		return errors.Join(
			dependencies.runCapture(app, options.CaptureDir, true),
			app.Close(),
		)
	}

	if options.MotionDemoPath != "" {
		// motion 演示是独立的无头路径：单个 fresh application，不跑 LOD
		// control（那是 PNG 基线更新的专属门禁），演示场景不进场景表。
		motionOptions := options.Application
		motionOptions.MotionDemo = true
		motionApp, err := dependencies.newApplication(motionOptions)
		if err != nil {
			return fmt.Errorf("启动 motion 演示抓帧: %w", err)
		}
		return errors.Join(
			dependencies.runMotionDemo(motionApp, options.MotionDemoPath),
			motionApp.Close(),
		)
	}

	app, err := dependencies.newApplication(options.Application)
	if err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	if options.CaptureDir != "" {
		return errors.Join(
			dependencies.runCapture(app, options.CaptureDir, options.UpdateGolden),
			app.Close(),
		)
	}

	var runErr error
	if options.Application.Benchmark {
		if err := dependencies.runBenchmark(app, options.PerfOutput); err != nil {
			runErr = fmt.Errorf("性能记录失败: %w", err)
		}
	} else {
		// 开发捕获服务只挂在交互路径：与 benchmark/capture 的互斥已在
		// parse 层拒绝，无头路径不经过本分支，这是第二道双保险。
		var captureService *devcapture.Service
		if options.DevCapture {
			captureService = startDevCapture(app, options.DevCaptureAddr)
		}
		runErr = dependencies.runInteractive(app)
		if captureService != nil {
			// 先停服务再关 app：监听关闭后 /status|/screenshot 不再触达
			// 已开始释放的 app；端口发现文件由 `Stop` 清除，即使
			// runInteractive 已带错返回也要执行。
			if err := captureService.Stop(); err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("停止开发捕获服务: %w", err))
			}
		}
	}
	return errors.Join(runErr, app.Close())
}

// devCaptureStatusSource 把 `*application.Application` 的最小状态访问器适配成
// `devcapture.StatusSource`，供 `/status` 观察客户端相位与窗口尺寸。降级语义
// （无窗口、未注入协调器时的未知值）全部收敛在 app 侧访问器内，这里只做纯
// 转发、不持有状态。
type devCaptureStatusSource struct {
	app *application.Application
}

// devCaptureStatusSource 必须持续满足 StatusSource：签名漂移在编译期暴露。
var _ devcapture.StatusSource = devCaptureStatusSource{}

func (s devCaptureStatusSource) Phase() string     { return s.app.Phase() }
func (s devCaptureStatusSource) WindowWidth() int  { return s.app.WindowWidth() }
func (s devCaptureStatusSource) WindowHeight() int { return s.app.WindowHeight() }

// startDevCapture 装配并启动本地开发捕获服务：注入状态适配器、把服务登记为
// 帧循环的捕获协调器，然后绑定监听并写入端口发现文件（路径留空由 devcapture
// 包回落 ~/.mornlea 默认值）。实际绑定地址打印到 stdout——顺延绑定可能偏离
// 请求端口，stdout 与发现文件是两个独立的发现渠道。
//
// 启动失败（端口耗尽、发现文件不可写等）不让游戏崩溃：捕获是纯增益的调试
// 服务，这里告警并清除协调器，让帧循环回到零参与状态，游戏照常运行。
func startDevCapture(app *application.Application, addr string) *devcapture.Service {
	service := devcapture.New(devcapture.Options{
		Status: devCaptureStatusSource{app: app},
		Addr:   addr,
	})
	// 先注入再启动：`Start` 之后 HTTP handler 随时可能读取状态适配器并经
	// 协调器发起捕获，协调器必须在第一个请求可能到达前就位。
	app.SetCaptureCoordinator(service)
	listenAddr, err := service.Start()
	if err != nil {
		app.SetCaptureCoordinator(nil)
		slog.Warn("开发捕获服务启动失败，本次运行禁用画面捕获", "error", err)
		return service
	}
	fmt.Println("开发捕获服务已启动: http://" + listenAddr)
	return service
}

func loadApplicationIdentity(requestedName *string) (network.Identity, error) {
	loaded, err := profile.LoadOrCreate(profile.Options{RequestedName: requestedName})
	if err != nil {
		return network.Identity{}, err
	}
	return network.Identity{PlayerID: loaded.PlayerID, DisplayName: loaded.DisplayName}, nil
}

// resolveAgentCredential 按 settings.APIKeyEnv 指向的环境变量名解析密钥值。
//
// 环境变量名缺失、变量未设置或值为空均返回空串，并由 server.NewHost 的
// credential 边界拒绝启动。密钥值只流入内存中的 application.Options。
func resolveAgentCredential(settings companion.AgentServiceSettings) string {
	if settings.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(settings.APIKeyEnv)
}

// clientMemoryLimit 是客户端进程的 Go 堆软上限。
//
// 视距 32 下快速移动会造成密集的区块加载、卸载与网格化周转。Go 默认要等堆长到
// 活跃集的两倍才回收，实测这会让堆保留冲到 1635MiB，而其中约 400MiB 只是尚未
// 回收的空闲堆。设定软上限让 GC 在接近该值时更积极，实测把进程 RSS 峰值压低约
// 121MiB，活跃数据与帧时间分位数均不受影响。
//
// 取值需高于实测活跃堆峰值（约 1252MiB），否则 GC 会因长期贴近上限而频繁运行。
const clientMemoryLimit = 1500 << 20

func main() {
	debug.SetMemoryLimit(clientMemoryLimit)
	if err := run(os.Args[1:]); err != nil {
		slog.Error("mornlea 退出失败", "error", err)
		os.Exit(1)
	}
}
