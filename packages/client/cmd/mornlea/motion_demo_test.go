//go:build darwin

package main

// motion_demo_test.go：motion 演示模式（`--motion-demo`）的装配回归：
// flag 解析与互斥、单 application 路由、无头装配开关与配置隔离。

import (
	"errors"
	"path/filepath"
	"testing"

	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestParseMainOptionsMotionDemo(t *testing.T) {
	opts, err := parseMainOptions([]string{"--motion-demo", "testdata/visual-golden/motion/break-burst.gif"})
	if err != nil {
		t.Fatalf("解析 --motion-demo 失败: %v", err)
	}
	if opts.MotionDemoPath != "testdata/visual-golden/motion/break-burst.gif" {
		t.Fatalf("MotionDemoPath = %q", opts.MotionDemoPath)
	}
}

func TestParseMainOptionsMotionDemoDefaultsEmpty(t *testing.T) {
	opts, err := parseMainOptions(nil)
	if err != nil {
		t.Fatalf("解析空参数失败: %v", err)
	}
	if opts.MotionDemoPath != "" {
		t.Fatalf("MotionDemoPath = %q，想要空", opts.MotionDemoPath)
	}
}

func TestParseMainOptionsMotionDemoRejectsConflicts(t *testing.T) {
	// motion 演示独占无头渲染路径并按自己的 tick 节奏驱动，与其余独占
	// 路径组合的语义无法定义，必须直接拒绝而不是让某一方静默胜出。
	tests := []struct {
		name string
		args []string
	}{
		{"与 benchmark 互斥", []string{"--motion-demo", "x.gif", "--benchmark", "--perf-output", "/tmp/p.json"}},
		{"与 connect 互斥", []string{"--motion-demo", "x.gif", "--connect", "127.0.0.1:25565"}},
		{"与 capture 互斥", []string{"--motion-demo", "x.gif", "--capture", "/tmp/shots"}},
		{"与 dev-capture 互斥", []string{"--motion-demo", "x.gif", "--dev-capture"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseMainOptions(tc.args); err == nil {
				t.Fatalf("accepted %v", tc.args)
			}
		})
	}
}

func TestRunMotionDemoUsesOneApplicationWithoutCapture(t *testing.T) {
	constructed, closed, demonstrated := 0, 0, 0
	var gotPath string
	err := runWithDependencies(
		append([]string{"--motion-demo", "testdata/visual-golden/motion/break-burst.gif"}, absentConfigArgs(t)...),
		runDependencies{
			loadIdentity: func(*string) (network.Identity, error) { return network.Identity{}, nil },
			newApplication: func(options application.Options) (*application.Application, error) {
				constructed++
				if !options.MotionDemo {
					t.Fatal("motion 演示必须置位无头装配开关 options.MotionDemo")
				}
				if options.CaptureDir != "" {
					t.Fatalf("motion 演示不得冒充抓帧模式 options.CaptureDir=%q", options.CaptureDir)
				}
				return application.NewCloseTrackedApplicationForTest(func() { closed++ }), nil
			},
			runMotionDemo: func(_ *application.Application, path, _ string) error {
				demonstrated++
				gotPath = path
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("runWithDependencies: %v", err)
	}
	if constructed != 1 || demonstrated != 1 || closed != 1 {
		t.Fatalf("constructed=%d demonstrated=%d closed=%d，want 1/1/1", constructed, demonstrated, closed)
	}
	if gotPath != "testdata/visual-golden/motion/break-burst.gif" {
		t.Fatalf("motion 输出路径=%q", gotPath)
	}
}

func TestMotionDemoIgnoresUserConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	custom := config.Defaults()
	custom.Physics.Gravity = 1
	if err := custom.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	t.Cleanup(func() { config.Defaults().Apply() })

	effective, err := resolveConfig(mainOptions{ConfigPath: path, MotionDemoPath: "x.gif"})
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if effective.Physics.Gravity != config.Defaults().Physics.Gravity {
		t.Fatal("motion 演示路径必须使用编译默认值，不得读用户配置")
	}
}

func TestRunMotionDemoPropagatesError(t *testing.T) {
	want := errors.New("motion failed")
	err := runWithDependencies(
		append([]string{"--motion-demo", "x.gif"}, absentConfigArgs(t)...),
		runDependencies{
			loadIdentity: func(*string) (network.Identity, error) { return network.Identity{}, nil },
			newApplication: func(application.Options) (*application.Application, error) {
				return application.NewCloseTrackedApplicationForTest(func() {}), nil
			},
			runMotionDemo: func(*application.Application, string, string) error { return want },
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v，want %v", err, want)
	}
}

func TestMotionSceneSelector(t *testing.T) {
	for _, scene := range []string{"break-burst", "avatar-walk", "drop-scatter", "drop-density"} {
		opts, err := parseMainOptions([]string{"--motion-demo", "x.gif", "--motion-scene", scene})
		if err != nil || opts.MotionScene != scene {
			t.Fatalf("scene=%s opts=%+v err=%v", scene, opts, err)
		}
	}
	for _, args := range [][]string{{"--motion-scene", "avatar-walk"}, {"--motion-demo", "x.gif", "--motion-scene", "unknown"}} {
		if _, err := parseMainOptions(args); err == nil {
			t.Fatalf("接受非法选择 %v", args)
		}
	}
}
