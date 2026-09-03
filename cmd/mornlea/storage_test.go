//go:build darwin

package main

// storage_test.go：main 装配层（mainOptions 解析与 runWithDependencies 生命周期）
// 的测试。`openApplicationStore` 的存储选择主题测试随 app 域迁入
// `cmd/mornlea/app`；本文件只保留生产符号属于 main 包的入口。

import (
	"context"
	"errors"
	"testing"

	"github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestMainOptionsDefaultsAndWorldOverride(t *testing.T) {
	defaults, err := parseMainOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Application.Seed != 42 || defaults.Application.Benchmark ||
		defaults.Application.WorldPath != "worlds/default" {
		t.Fatalf("default options=%+v", defaults)
	}

	selected, err := parseMainOptions([]string{"--world", "testdata/my-world"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Application.WorldPath != "testdata/my-world" {
		t.Fatalf("selected world=%q", selected.Application.WorldPath)
	}

	benchmark, err := parseMainOptions([]string{
		"--benchmark", "--perf-output", "report.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !benchmark.Application.Benchmark || benchmark.Application.Seed != app.BenchmarkSeed {
		t.Fatalf("benchmark options=%+v", benchmark)
	}
}

func TestMainOptionsRejectMissingBenchmarkOutputBeforeConstruction(t *testing.T) {
	constructed := 0
	err := runWithDependencies([]string{"--benchmark"}, runDependencies{
		loadIdentity: testIdentityLoader,
		newApplication: func(app.Options) (*app.Application, error) {
			constructed++
			return nil, errors.New("不应构造应用")
		},
	})
	if err == nil {
		t.Fatal("run error=nil，想要缺少 --perf-output 的错误")
	}
	if constructed != 0 {
		t.Fatalf("缺少 --perf-output 时构造应用 %d 次", constructed)
	}
}

func TestRunWithDependenciesPropagatesLifecycleErrorsAndAlwaysCloses(t *testing.T) {
	constructionErr := errors.New("构造失败")
	benchmarkErr := errors.New("benchmark 失败")
	closeErr := errors.New("持久化关服失败")
	tests := []struct {
		name             string
		args             []string
		constructionErr  error
		benchmarkErr     error
		closeErr         error
		wantConstruction int
		wantInteractive  int
		wantBenchmark    int
		wantClose        int
		wantErrors       []error
	}{
		{
			name:             "construction error",
			constructionErr:  constructionErr,
			wantConstruction: 1,
			wantErrors:       []error{constructionErr},
		},
		{
			name:             "interactive success and close error",
			closeErr:         closeErr,
			wantConstruction: 1,
			wantInteractive:  1,
			wantClose:        1,
			wantErrors:       []error{closeErr},
		},
		{
			name:             "benchmark error and successful close",
			args:             []string{"--benchmark", "--perf-output", "report.json"},
			benchmarkErr:     benchmarkErr,
			wantConstruction: 1,
			wantBenchmark:    1,
			wantClose:        1,
			wantErrors:       []error{benchmarkErr},
		},
		{
			name:             "benchmark and close errors",
			args:             []string{"--benchmark", "--perf-output", "report.json"},
			benchmarkErr:     benchmarkErr,
			closeErr:         closeErr,
			wantConstruction: 1,
			wantBenchmark:    1,
			wantClose:        1,
			wantErrors:       []error{benchmarkErr, closeErr},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			constructionCalls := 0
			interactiveCalls := 0
			benchmarkCalls := 0
			closeCalls := 0
			gotErr := runWithDependencies(append(append([]string{}, test.args...), absentConfigArgs(t)...), runDependencies{
				loadIdentity: testIdentityLoader,
				newApplication: func(app.Options) (*app.Application, error) {
					constructionCalls++
					if test.constructionErr != nil {
						return nil, test.constructionErr
					}
					serverDone := make(chan error, 1)
					if test.closeErr == nil {
						serverDone <- context.Canceled
					} else {
						serverDone <- test.closeErr
					}
					return app.NewServerTeardownApplicationForTest(serverDone, func() {
						closeCalls++
					}), nil
				},
				runInteractive: func(*app.Application) error {
					interactiveCalls++
					return nil
				},
				runBenchmark: func(*app.Application, string) error {
					benchmarkCalls++
					return test.benchmarkErr
				},
			})

			if constructionCalls != test.wantConstruction ||
				interactiveCalls != test.wantInteractive ||
				benchmarkCalls != test.wantBenchmark || closeCalls != test.wantClose {
				t.Fatalf(
					"calls construction=%d interactive=%d benchmark=%d close=%d，想要 %d/%d/%d/%d",
					constructionCalls, interactiveCalls, benchmarkCalls, closeCalls,
					test.wantConstruction, test.wantInteractive, test.wantBenchmark,
					test.wantClose,
				)
			}
			for _, wantErr := range test.wantErrors {
				if !errors.Is(gotErr, wantErr) {
					t.Errorf("run error=%v，不包含 %v", gotErr, wantErr)
				}
			}
		})
	}
}

func testIdentityLoader(*string) (network.Identity, error) {
	return network.Identity{}, nil
}
