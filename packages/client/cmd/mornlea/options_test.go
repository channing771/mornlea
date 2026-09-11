//go:build darwin

package main

import (
	"reflect"
	"strings"
	"testing"

	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/devcapture"
)

func TestParseMainOptionsRejectsRemoteLocalConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"--connect", "127.0.0.1:25565", "--world", "worlds/demo"},
		{"--connect", "127.0.0.1:25565", "--benchmark", "--perf-output", "x.json"},
		{"--benchmark", "--perf-output", "x.json", "--name", "Chen"},
	} {
		if _, err := parseMainOptions(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestParseMainOptionsAllowsRemoteWithDefaultWorld(t *testing.T) {
	options, err := parseMainOptions([]string{"--connect", "127.0.0.1:25565"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Application.Connect != "127.0.0.1:25565" || options.Application.WorldPath != "worlds/default" {
		t.Fatalf("options=%+v", options.Application)
	}
}

func TestParseMainOptionsRejectsDifficultyFlag(t *testing.T) {
	// 难度是世界身份，由服务端权威持有；图形客户端不提供难度覆盖入口。
	// 四条启动路径共用同一处 flag 解析，这里逐路径钉住 `--difficulty` 在
	// 解析阶段被拒绝：断言错误必须是「flag 未定义」而非某个取值校验——
	// 若未来有人注册了该旗标，即便随后另有拒绝逻辑，本测试也要失败，
	// 防止难度入口悄悄混入客户端。
	tests := []struct {
		name string
		args []string
	}{
		{"普通本地路径", []string{"--difficulty", "normal"}},
		{"远程联机路径", []string{"--connect", "127.0.0.1:25565", "--difficulty", "normal"}},
		{"benchmark 路径", []string{"--benchmark", "--perf-output", "x.json", "--difficulty", "normal"}},
		{"capture 路径", []string{"--capture", "/tmp/shots", "--difficulty", "normal"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMainOptions(tc.args)
			if err == nil {
				t.Fatalf("accepted %v", tc.args)
			}
			if !strings.Contains(err.Error(), "not defined") {
				t.Fatalf("错误 %q 想要 flag 未定义拒绝，实际来自其他校验", err.Error())
			}
		})
	}
}

func TestParseMainOptionsBenchmarkTransport(t *testing.T) {
	defaults, err := parseMainOptions([]string{"--benchmark", "--perf-output", "x.json"})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Application.BenchmarkTransport != "memory" {
		t.Fatalf("default benchmark transport=%q, want memory", defaults.Application.BenchmarkTransport)
	}
	tcp, err := parseMainOptions([]string{
		"--benchmark", "--benchmark-transport", "tcp", "--perf-output", "x.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tcp.Application.BenchmarkTransport != "tcp" {
		t.Fatalf("TCP benchmark transport=%q", tcp.Application.BenchmarkTransport)
	}
	for _, args := range [][]string{
		{"--benchmark-transport", "tcp"},
		{"--benchmark", "--benchmark-transport", "udp", "--perf-output", "x.json"},
	} {
		if _, err := parseMainOptions(args); err == nil {
			t.Fatalf("accepted invalid benchmark transport args %v", args)
		}
	}
}

func TestParseMainOptionsCaptureDir(t *testing.T) {
	opts, err := parseMainOptions([]string{"--capture", "/tmp/shots"})
	if err != nil {
		t.Fatalf("解析 --capture 失败: %v", err)
	}
	if opts.CaptureDir != "/tmp/shots" {
		t.Fatalf("CaptureDir = %q，想要 %q", opts.CaptureDir, "/tmp/shots")
	}
	if opts.Application.CaptureDir != "/tmp/shots" {
		t.Fatalf("Application.CaptureDir = %q，想要 %q", opts.Application.CaptureDir, "/tmp/shots")
	}
}

func TestParseMainOptionsCaptureRejectsConflicts(t *testing.T) {
	// --capture 与 --benchmark 都会独占无头渲染路径并各自驱动场景，
	// 同时开启的语义无法定义，必须直接拒绝而不是让某一方静默胜出。
	tests := []struct {
		name string
		args []string
	}{
		{"与 benchmark 互斥", []string{"--capture", "/tmp/shots", "--benchmark", "--perf-output", "/tmp/p.json"}},
		{"与 connect 互斥", []string{"--capture", "/tmp/shots", "--connect", "127.0.0.1:25565"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseMainOptions(tc.args); err == nil {
				t.Fatal("想要报错，实际通过")
			}
		})
	}
}

func TestParseMainOptionsUpdateGoldenRequiresCapture(t *testing.T) {
	if _, err := parseMainOptions([]string{"--update-golden"}); err == nil {
		t.Fatal("--update-golden 缺少 --capture 时想要报错，实际通过")
	}
}

func TestParseMainOptionsUpdateGoldenWithCapturePropagates(t *testing.T) {
	opts, err := parseMainOptions([]string{"--capture", "/tmp/shots", "--update-golden"})
	if err != nil {
		t.Fatalf("解析 --capture --update-golden 失败: %v", err)
	}
	if !opts.UpdateGolden {
		t.Fatal("UpdateGolden = false，想要 true")
	}
}

func TestParseMainOptionsWithoutUpdateGoldenDefaultsFalse(t *testing.T) {
	opts, err := parseMainOptions([]string{"--capture", "/tmp/shots"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.UpdateGolden {
		t.Fatal("UpdateGolden = true，想要默认 false")
	}
}

func TestParseMainOptionsWithoutCaptureLeavesDirEmpty(t *testing.T) {
	opts, err := parseMainOptions(nil)
	if err != nil {
		t.Fatalf("解析空参数失败: %v", err)
	}
	if opts.CaptureDir != "" {
		t.Fatalf("CaptureDir = %q，想要空", opts.CaptureDir)
	}
}

func TestParseMainOptionsCaptureScenesRequiresCapture(t *testing.T) {
	if _, err := parseMainOptions([]string{"--capture-scenes", "main-menu"}); err == nil {
		t.Fatal("--capture-scenes 缺少 --capture 时想要报错，实际通过")
	}
}

func TestParseMainOptionsCaptureScenesRejectsInvalidSelection(t *testing.T) {
	// 空项、重复与未知名都必须在启动前被点名拒绝：写错的子集若被静默
	// 忽略后照常跑全量，会把「我以为只跑了两景」的误读留给调用方。
	tests := []struct {
		name    string
		scenes  string
		wantErr string
	}{
		{"空项", "terrain-noon,,main-menu", "空项"},
		{"纯空白项", "terrain-noon,  ", "空项"},
		{"重复场景", "terrain-noon,terrain-noon", "terrain-noon"},
		{"未知场景", "no-such-scene", "no-such-scene"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMainOptions([]string{"--capture", "/tmp/shots", "--capture-scenes", tc.scenes})
			if err == nil {
				t.Fatalf("--capture-scenes %q 想要报错，实际通过", tc.scenes)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("错误信息 %q 未包含 %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestParseMainOptionsCaptureScenesParsesSubsetInInputOrder(t *testing.T) {
	// parse 层只切分与校验，不按场景表重排：main-menu 在场景表中晚于
	// terrain-noon，这里断言输出保持输入顺序，保序过滤由 capture 层完成。
	opts, err := parseMainOptions([]string{
		"--capture", "/tmp/shots",
		"--capture-scenes", " main-menu , terrain-noon ",
	})
	if err != nil {
		t.Fatalf("解析合法子集失败: %v", err)
	}
	want := []string{"main-menu", "terrain-noon"}
	if !reflect.DeepEqual(opts.CaptureScenes, want) {
		t.Fatalf("CaptureScenes = %v，想要 %v", opts.CaptureScenes, want)
	}
}

func TestParseMainOptionsCaptureScenesDefaultNil(t *testing.T) {
	opts, err := parseMainOptions([]string{"--capture", "/tmp/shots"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.CaptureScenes != nil {
		t.Fatalf("CaptureScenes = %v，想要 nil（缺省跑全部场景）", opts.CaptureScenes)
	}
}

func TestParseMainOptionsCaptureGifsRequiresCapture(t *testing.T) {
	if _, err := parseMainOptions([]string{"--capture-gifs"}); err == nil {
		t.Fatal("--capture-gifs 缺少 --capture 时想要报错，实际通过")
	}
}

func TestParseMainOptionsCaptureGifsPropagates(t *testing.T) {
	opts, err := parseMainOptions([]string{"--capture", "/tmp/shots", "--capture-gifs"})
	if err != nil {
		t.Fatalf("解析 --capture --capture-gifs 失败: %v", err)
	}
	if !opts.CaptureGIFs {
		t.Fatal("CaptureGIFs = false，想要 true")
	}
	// 与 --update-golden 组合合法但冗余（update 恒生成 GIF），不得拒绝。
	if _, err := parseMainOptions([]string{"--capture", "/tmp/shots", "--update-golden", "--capture-gifs"}); err != nil {
		t.Fatalf("--capture-gifs 与 --update-golden 组合想要合法: %v", err)
	}
}

func TestParseMainOptionsCaptureGifsDefaultOff(t *testing.T) {
	opts, err := parseMainOptions([]string{"--capture", "/tmp/shots"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.CaptureGIFs {
		t.Fatal("CaptureGIFs = true，想要默认 false（纯比对缺省不生成 GIF）")
	}
}

func TestParseOptionsDefaultsDevOff(t *testing.T) {
	options, err := parseMainOptions([]string{})
	if err != nil {
		t.Fatalf("parseMainOptions: %v", err)
	}
	if options.Dev {
		t.Fatal("--dev 默认必须关闭")
	}
}

func TestParseOptionsAcceptsDevAndConfig(t *testing.T) {
	options, err := parseMainOptions([]string{"--dev", "--config", "/tmp/x.json"})
	if err != nil {
		t.Fatalf("parseMainOptions: %v", err)
	}
	if !options.Dev {
		t.Fatal("--dev 必须被解析")
	}
	if options.ConfigPath != "/tmp/x.json" {
		t.Fatalf("ConfigPath = %q", options.ConfigPath)
	}
}

func TestParseMainOptionsDevCaptureDefaultsOff(t *testing.T) {
	opts, err := parseMainOptions(nil)
	if err != nil {
		t.Fatalf("解析空参数失败: %v", err)
	}
	if opts.DevCapture {
		t.Fatal("--dev-capture 默认必须关闭：捕获服务只在显式启用时监听端口并写发现文件")
	}
	if opts.DevCaptureAddr != devcapture.DefaultAddr {
		t.Fatalf("DevCaptureAddr = %q，想要默认 %q", opts.DevCaptureAddr, devcapture.DefaultAddr)
	}
}

func TestParseMainOptionsDevCaptureEnabled(t *testing.T) {
	opts, err := parseMainOptions([]string{"--dev-capture"})
	if err != nil {
		t.Fatalf("解析 --dev-capture 失败: %v", err)
	}
	if !opts.DevCapture {
		t.Fatal("DevCapture = false，想要 true")
	}
	if opts.DevCaptureAddr != devcapture.DefaultAddr {
		t.Fatalf("DevCaptureAddr = %q，想要默认 %q", opts.DevCaptureAddr, devcapture.DefaultAddr)
	}
}

func TestParseMainOptionsDevCaptureRejectsHeadlessPaths(t *testing.T) {
	// --dev-capture 消费交互窗口的合成画面；benchmark 与 capture 是无头路径，
	// 没有窗口可捕获，组合语义无法定义，必须在 parse 层直接拒绝而不是让
	// 某一方静默胜出。
	tests := []struct {
		name string
		args []string
	}{
		{"与 benchmark 互斥", []string{"--dev-capture", "--benchmark", "--perf-output", "/tmp/p.json"}},
		{"与 capture 互斥", []string{"--dev-capture", "--capture", "/tmp/shots"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseMainOptions(tc.args); err == nil {
				t.Fatalf("accepted %v", tc.args)
			}
		})
	}
}

func TestParseMainOptionsDevCaptureAddrPassesThrough(t *testing.T) {
	opts, err := parseMainOptions([]string{"--dev-capture", "--dev-capture-addr", "127.0.0.1:18790"})
	if err != nil {
		t.Fatalf("解析 --dev-capture --dev-capture-addr 失败: %v", err)
	}
	if opts.DevCaptureAddr != "127.0.0.1:18790" {
		t.Fatalf("DevCaptureAddr = %q，想要 127.0.0.1:18790", opts.DevCaptureAddr)
	}
}

func TestParseMainOptionsDevCaptureAddrAloneIsInert(t *testing.T) {
	// 独立 flag 语义（同 --perf-output）：--dev-capture-addr 单独给出不报错
	// 也不启用捕获服务，仅在 --dev-capture 存在时被消费。
	opts, err := parseMainOptions([]string{"--dev-capture-addr", "127.0.0.1:18790"})
	if err != nil {
		t.Fatalf("解析 --dev-capture-addr 失败: %v", err)
	}
	if opts.DevCapture {
		t.Fatal("--dev-capture-addr 不得单独启用捕获服务")
	}
}

func TestDevCaptureStatusSourceDegradesWithoutWindow(t *testing.T) {
	// 适配器对零值 Application（无窗口、未注入协调器）必须安全降级：不 panic，
	// 尺寸以非正值报告「未知」（StatusSource 契约），相位返回合法枚举串。
	source := devCaptureStatusSource{app: &application.Application{}}
	if width, height := source.WindowWidth(), source.WindowHeight(); width > 0 || height > 0 {
		t.Fatalf("无窗口时尺寸 (%d,%d)，想要非正值表示未知", width, height)
	}
	if phase := source.Phase(); phase == "" {
		t.Fatal("零值相位应返回合法枚举串（game），想要非空")
	}
}
