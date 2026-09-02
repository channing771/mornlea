//go:build darwin

package main

import (
	"testing"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/cmd/mornlea/devcapture"
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
