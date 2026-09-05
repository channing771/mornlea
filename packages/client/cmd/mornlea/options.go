//go:build darwin

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/devcapture"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

type mainOptions struct {
	Application   application.Options
	PerfOutput    string
	RequestedName *string
	CaptureDir    string
	// UpdateGolden 为真时，抓帧结果写入 golden 基线而不是与之比对。
	// 与 application.Options 无关：它只影响 runCapture 的行为，从
	// runWithDependencies 直接传给 dependencies.runCapture。
	UpdateGolden bool
	// MotionDemoPath 非空时走 motion 演示模式：无头装配与抓帧同源，只跑
	// capture 包的 motion 演示入口并把 GIF 写到该路径，不进场景表与比对。
	MotionDemoPath string
	MotionScene    string
	// ConfigPath 是调参配置文件路径；留空表示使用 config.DefaultPath()。
	ConfigPath string
	// Dev 为真时启用调试面板（F3 切换）。它只门控面板可用性，不门控配置文件
	// 是否生效——配置文件里调过的值无论 Dev 是否为真都同样生效。
	Dev bool
	// DevCapture 为真时在交互式客户端内启用本地开发捕获服务（`devcapture`
	// 包）：仅绑定回环地址，实际端口写入 ~/.mornlea/dev-capture.json。与
	// --benchmark/--capture 互斥——那两条路径无头运行，没有窗口可捕获。
	DevCapture bool
	// DevCaptureAddr 是开发捕获服务的监听地址，仅接受回环地址（服务内还有
	// 一道绑定前的回环防御闸）。独立 flag（同 --perf-output）：不带
	// --dev-capture 时允许给出但无任何效果。
	DevCaptureAddr string
}

func parseMainOptions(args []string) (mainOptions, error) {
	flags := flag.NewFlagSet("mornlea", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	benchmark := flags.Bool("benchmark", false, "运行固定 1440p 性能场景")
	benchmarkTransport := flags.String("benchmark-transport", "memory", "benchmark 业务传输: memory 或 tcp")
	perfOutput := flags.String("perf-output", "", "性能报告 JSON 输出路径")
	worldPath := flags.String("world", "worlds/default", "世界存档目录")
	connect := flags.String("connect", "", "远程 TCP 服务器地址")
	name := flags.String("name", "", "玩家显示名")
	capture := flags.String("capture", "", "视觉抓帧输出目录；非空时走无头抓帧模式")
	updateGolden := flags.Bool("update-golden", false, "把本次抓帧结果写入 golden 基线")
	motionScene := flags.String("motion-scene", "break-burst", "motion 场景：break-burst/avatar-walk/drop-scatter/drop-density")
	motionDemo := flags.String("motion-demo", "", "motion 演示 GIF 输出路径；非空时走无头 motion 演示模式")
	dev := flags.Bool("dev", false, "启用调试面板（F3 切换）")
	configPath := flags.String("config", "", "配置文件路径，留空使用默认路径")
	devCapture := flags.Bool("dev-capture", false, "启用本地开发捕获服务（仅绑定回环地址）")
	devCaptureAddr := flags.String("dev-capture-addr", devcapture.DefaultAddr, "开发捕获服务监听地址，仅接受回环地址")
	if err := flags.Parse(args); err != nil {
		return mainOptions{}, err
	}
	if flags.NArg() != 0 {
		return mainOptions{}, fmt.Errorf("未知位置参数: %v", flags.Args())
	}
	if *benchmark && *perfOutput == "" {
		return mainOptions{}, errors.New("--benchmark 必须同时提供 --perf-output")
	}
	if *capture != "" && *benchmark {
		return mainOptions{}, errors.New("--capture 不能与 --benchmark 同时使用")
	}
	if *capture != "" && *connect != "" {
		return mainOptions{}, errors.New("--capture 不能与 --connect 同时使用")
	}
	// motion 演示同样独占无头渲染路径并按自己的 tick 节奏驱动帧循环，
	// 与其余独占路径组合的语义无法定义，直接拒绝而不是让某一方静默胜出。
	if *motionScene != "break-burst" && *motionScene != "avatar-walk" && *motionScene != "drop-scatter" && *motionScene != "drop-density" {
		return mainOptions{}, errors.New("未知 --motion-scene")
	}
	if *motionDemo == "" && *motionScene != "break-burst" {
		return mainOptions{}, errors.New("--motion-scene 需要 --motion-demo")
	}
	if *motionDemo != "" && *benchmark {
		return mainOptions{}, errors.New("--motion-demo 不能与 --benchmark 同时使用")
	}
	if *motionDemo != "" && *connect != "" {
		return mainOptions{}, errors.New("--motion-demo 不能与 --connect 同时使用")
	}
	if *motionDemo != "" && *capture != "" {
		return mainOptions{}, errors.New("--motion-demo 不能与 --capture 同时使用")
	}
	// --dev-capture 消费交互窗口的合成画面；benchmark 与 capture 无头运行，
	// 没有窗口可捕获，组合语义无法定义，直接拒绝而不是让一方静默胜出。
	if *devCapture && *benchmark {
		return mainOptions{}, errors.New("--dev-capture 不能与 --benchmark 同时使用")
	}
	if *devCapture && *capture != "" {
		return mainOptions{}, errors.New("--dev-capture 不能与 --capture 同时使用")
	}
	if *devCapture && *motionDemo != "" {
		return mainOptions{}, errors.New("--dev-capture 不能与 --motion-demo 同时使用")
	}
	if *updateGolden && *capture == "" {
		return mainOptions{}, errors.New("--update-golden 只能与 --capture 同时使用")
	}
	var worldExplicit, nameExplicit, benchmarkTransportExplicit bool
	flags.Visit(func(flag *flag.Flag) {
		worldExplicit = worldExplicit || flag.Name == "world"
		nameExplicit = nameExplicit || flag.Name == "name"
		benchmarkTransportExplicit = benchmarkTransportExplicit || flag.Name == "benchmark-transport"
	})
	if *connect != "" && worldExplicit {
		return mainOptions{}, errors.New("--connect 不能与显式 --world 同时使用")
	}
	if *connect != "" && *benchmark {
		return mainOptions{}, errors.New("--connect 不能与 --benchmark 同时使用")
	}
	if *benchmark && nameExplicit {
		return mainOptions{}, errors.New("--name 不能与 --benchmark 同时使用")
	}
	if benchmarkTransportExplicit && !*benchmark {
		return mainOptions{}, errors.New("--benchmark-transport 只能与 --benchmark 同时使用")
	}
	if *benchmarkTransport != "memory" && *benchmarkTransport != "tcp" {
		return mainOptions{}, fmt.Errorf("无效 --benchmark-transport %q：只支持 memory 或 tcp", *benchmarkTransport)
	}
	seed := int64(42)
	if *benchmark {
		seed = application.BenchmarkSeed
	}
	return mainOptions{
		Application: application.Options{
			Seed:               seed,
			Benchmark:          *benchmark,
			BenchmarkTransport: *benchmarkTransport,
			WorldPath:          *worldPath,
			Connect:            *connect,
			CaptureDir:         *capture,
		},
		PerfOutput: *perfOutput,
		RequestedName: func() *string {
			if nameExplicit {
				return name
			}
			return nil
		}(),
		CaptureDir:   *capture,
		UpdateGolden: *updateGolden,
		// `--update-golden 只能与 --capture 同时使用` 的既有校验已顺带拒绝
		// `--motion-demo + --update-golden` 组合（此时 capture 为空），这里
		// 不再重复设限。
		MotionDemoPath: *motionDemo,
		MotionScene:    *motionScene,
		ConfigPath:     *configPath,
		Dev:            *dev,

		DevCapture:     *devCapture,
		DevCaptureAddr: *devCaptureAddr,
	}, nil
}

// resolveConfigPath 决定调参配置文件的实际路径：显式 --config 优先，
// 否则落回用户配置目录下的默认路径。
func resolveConfigPath(options mainOptions) (string, error) {
	if options.ConfigPath != "" {
		return options.ConfigPath, nil
	}
	return config.DefaultPath()
}

// resolveConfig 决定本次运行的生效配置。
//
// benchmark、抓帧与 motion 演示三条路径强制使用编译默认值：它们的产出会与基线比对
// 或作为演示产物入库，若读入本机配置，结论就取决于开发者本机的配置文件内容而非代码。
func resolveConfig(options mainOptions) (config.Config, error) {
	if options.Application.Benchmark || options.CaptureDir != "" || options.MotionDemoPath != "" {
		return config.Defaults(), nil
	}
	if options.ConfigPath != "" {
		return config.Load(options.ConfigPath)
	}
	return config.LoadDefault()
}

// remoteTuningDiverges 报告本次运行是否"连远端服务端 + 本机 physics/sim 偏离
// 编译默认值"。
//
// 设计 §3.2：physics/sim 同时被客户端预测与服务端权威模拟消费，两侧参数不同
// 会让位置持续回弹。调试面板已经用 fieldReadOnly 挡住了联机时的改写，但
// 配置文件是始终生效的（§3.1），它绕过面板那道锁。
//
// 这里只告警不回落默认值：README 明确把这份配置文件描述为 mornlea 与 mornlea-server 共用，
// 局域网下两端读同一份调过的文件恰恰是正确且一致的用法，强制客户端回落默认值
// 反而会在那个本来能用的场景里制造分歧。
func remoteTuningDiverges(options mainOptions, effective config.Config) bool {
	if options.Application.Connect == "" {
		return false
	}
	return effective.Physics != physics.DefaultTunables() ||
		effective.Sim != tuning.DefaultTunables()
}
