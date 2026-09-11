package main

// difficulty_test.go：专服 `--difficulty` 的旗标语义与监听前一致性校验。
// 覆盖难度规约「专服难度在监听前完成一致性校验」：显式冲突在监听前失败、
// 关闭已打开存储且不创建 listener；省略时已有世界完全沿用存档事实；
// 非法参数在解析阶段拒绝，不进入世界打开路径。

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestParseOptionsDifficultyOmittedByDefault(t *testing.T) {
	got, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if got.DifficultySet {
		t.Fatal("省略 --difficulty 时 DifficultySet 必须为 false")
	}
	if got.Difficulty != core.DifficultyNormal {
		t.Fatalf("省略 --difficulty 时 Difficulty = %s，期望 normal 零值", got.Difficulty)
	}
}

func TestParseOptionsDifficultyExplicitValues(t *testing.T) {
	for _, test := range []struct {
		text string
		want core.Difficulty
	}{
		{text: "normal", want: core.DifficultyNormal},
		{text: "peaceful", want: core.DifficultyPeaceful},
		{text: "hard", want: core.DifficultyHard},
	} {
		// 显式 normal 必须与省略可区分：DifficultySet 为 true，
		// 后续 run 层仍要与已有世界 metadata 比对。
		got, err := parseOptions([]string{"--difficulty=" + test.text})
		if err != nil {
			t.Fatalf("parseOptions(--difficulty=%s): %v", test.text, err)
		}
		if !got.DifficultySet || got.Difficulty != test.want {
			t.Fatalf("--difficulty=%s 解析为 set=%v difficulty=%s", test.text, got.DifficultySet, got.Difficulty)
		}
	}
}

func TestParseOptionsDifficultyRejectsInvalidValues(t *testing.T) {
	for _, text := range []string{"Nightmare", "HARD", "easy", " normal", "normal ", ""} {
		if _, err := parseOptions([]string{"--difficulty=" + text}); err == nil {
			t.Fatalf("parseOptions 接受了非法 --difficulty=%q", text)
		}
	}
}

func TestRunInvalidDifficultyFailsBeforeWorldOpen(t *testing.T) {
	err := run(context.Background(), append([]string{"--difficulty", "Nightmare"}, absentConfigArgs(t)...), dependencies{
		openDisk: func(context.Context, string, storage.OpenOptions) (storage.WorldStore, error) {
			t.Fatal("非法难度在解析阶段失败，不得进入世界打开路径")
			return nil, nil
		},
		listenTCP: func(string) (network.Listener, error) {
			t.Fatal("非法难度不得创建 listener")
			return nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "difficulty") {
		t.Fatalf("run 错误 = %v，期望包含 difficulty 的解析错误", err)
	}
}

// TestRunCreateMetadataDifficulty 钉住新世界语义：省略时 `Create` 难度落在
// normal，显式时取该值。内存 store 直接镜像 `options.Create`，模拟新世界
// 打开后 metadata 即 Create 的事实，并证明该路径通过监听前校验。
func TestRunCreateMetadataDifficulty(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want core.Difficulty
	}{
		{name: "omitted creates normal", args: nil, want: core.DifficultyNormal},
		{name: "explicit peaceful creates peaceful", args: []string{"--difficulty", "peaceful"}, want: core.DifficultyPeaceful},
		{name: "explicit hard creates hard", args: []string{"--difficulty", "hard"}, want: core.DifficultyHard},
	} {
		t.Run(test.name, func(t *testing.T) {
			var created core.Difficulty
			want := errors.New("stop after assembly")
			listened := false
			err := run(context.Background(), append(test.args, absentConfigArgs(t)...), dependencies{
				openDisk: func(_ context.Context, _ string, options storage.OpenOptions) (storage.WorldStore, error) {
					created = options.Create.Difficulty
					return storage.NewMemory(options.Create), nil
				},
				listenTCP: func(string) (network.Listener, error) {
					listened = true
					return mornleaServerTestListener{}, nil
				},
				newHost: func(context.Context, server.Config, server.Generator, storage.WorldStore) (mornleaServerHost, error) {
					return &mornleaServerTestHost{runErr: want}, nil
				},
			})
			if !errors.Is(err, want) || created != test.want || !listened {
				t.Fatalf("run error=%v created=%s listened=%v，期望 %v、%s 与已监听", err, created, listened, want, test.want)
			}
		})
	}
}

func TestRunExplicitDifficultyConflictFailsBeforeListener(t *testing.T) {
	for _, test := range []struct {
		name        string
		flag        string
		worldFactor core.Difficulty
	}{
		{name: "hard flag on normal world", flag: "hard", worldFactor: core.DifficultyNormal},
		// 显式 normal 与省略语义不同：已有 peaceful 世界上显式 normal 同样冲突。
		{name: "normal flag on peaceful world", flag: "normal", worldFactor: core.DifficultyPeaceful},
		{name: "peaceful flag on hard world", flag: "peaceful", worldFactor: core.DifficultyHard},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &mornleaServerClosingStore{WorldStore: storage.NewMemory(storage.Metadata{
				FormatVersion:     6,
				Seed:              42,
				SpawnDimension:    core.Overworld,
				DepthsSpawnAnchor: core.ChunkPos{},
				DepthsSeedSalt:    core.DepthsSeedSalt,
				Difficulty:        test.worldFactor,
			})}
			listened := false
			err := run(context.Background(), append([]string{"--difficulty", test.flag}, absentConfigArgs(t)...), dependencies{
				openDisk: func(context.Context, string, storage.OpenOptions) (storage.WorldStore, error) {
					return store, nil
				},
				listenTCP: func(string) (network.Listener, error) {
					listened = true
					return mornleaServerTestListener{}, nil
				},
			})
			if err == nil {
				t.Fatal("显式难度与存档冲突后 run 必须失败")
			}
			for _, want := range []string{"--difficulty", test.flag, test.worldFactor.String()} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("冲突错误 %q 缺少 %q", err.Error(), want)
				}
			}
			if listened {
				t.Fatal("难度冲突路径创建了 listener")
			}
			if store.closes != 1 {
				t.Fatalf("store 关闭次数 = %d，期望 1", store.closes)
			}
		})
	}
}

// TestRunDiskWorldDifficultyConflictFailsBeforeListener 用真实磁盘世界走完整
// 打开路径：已有世界忽略 `Create`、以磁盘 metadata 为准，显式 hard 在监听前
// 失败并关闭 store。
func TestRunDiskWorldDifficultyConflictFailsBeforeListener(t *testing.T) {
	ctx := context.Background()
	worldPath := filepath.Join(t.TempDir(), "world")
	creator, err := storage.OpenDisk(ctx, worldPath, storage.OpenOptions{Create: storage.Metadata{
		FormatVersion:     6,
		Seed:              42,
		SpawnDimension:    core.Overworld,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    core.DepthsSeedSalt,
		Difficulty:        core.DifficultyNormal,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := creator.Close(); err != nil {
		t.Fatal(err)
	}

	store := &mornleaServerClosingStore{}
	listened := false
	err = run(ctx, append([]string{"--world", worldPath, "--difficulty", "hard"}, absentConfigArgs(t)...), dependencies{
		openDisk: func(_ context.Context, world string, options storage.OpenOptions) (storage.WorldStore, error) {
			opened, openErr := storage.OpenDisk(ctx, world, options)
			if openErr != nil {
				return nil, openErr
			}
			store.WorldStore = opened
			return store, nil
		},
		listenTCP: func(string) (network.Listener, error) {
			listened = true
			return mornleaServerTestListener{}, nil
		},
	})
	if err == nil {
		t.Fatal("磁盘世界难度冲突后 run 必须失败")
	}
	if !strings.Contains(err.Error(), "hard") || !strings.Contains(err.Error(), "normal") {
		t.Fatalf("冲突错误应同时指明显式与存档难度: %v", err)
	}
	if listened {
		t.Fatal("磁盘世界难度冲突路径创建了 listener")
	}
	if store.closes != 1 {
		t.Fatalf("store 关闭次数 = %d，期望 1", store.closes)
	}
}

// TestRunDifficultyAgreementReachesHost 钉住省略与显式一致两路的成功语义：
// 显式值与存档一致（含显式 normal 命中 normal 世界）、或省略参数时，
// 启动必须越过校验进入监听与 Host 装配，已有世界难度原样生效。
func TestRunDifficultyAgreementReachesHost(t *testing.T) {
	for _, test := range []struct {
		name        string
		args        []string
		worldFactor core.Difficulty
	}{
		{name: "explicit hard matches hard world", args: []string{"--difficulty", "hard"}, worldFactor: core.DifficultyHard},
		{name: "explicit normal matches normal world", args: []string{"--difficulty", "normal"}, worldFactor: core.DifficultyNormal},
		{name: "omitted uses hard world metadata", args: nil, worldFactor: core.DifficultyHard},
		{name: "omitted uses peaceful world metadata", args: nil, worldFactor: core.DifficultyPeaceful},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := storage.NewMemory(storage.Metadata{
				FormatVersion:     6,
				Seed:              42,
				SpawnDimension:    core.Overworld,
				DepthsSpawnAnchor: core.ChunkPos{},
				DepthsSeedSalt:    core.DepthsSeedSalt,
				Difficulty:        test.worldFactor,
			})
			want := errors.New("stop after assembly")
			listened := false
			var assembled core.Difficulty
			err := run(context.Background(), append(test.args, absentConfigArgs(t)...), dependencies{
				openDisk: func(context.Context, string, storage.OpenOptions) (storage.WorldStore, error) {
					return store, nil
				},
				listenTCP: func(string) (network.Listener, error) {
					listened = true
					return mornleaServerTestListener{}, nil
				},
				newHost: func(_ context.Context, _ server.Config, _ server.Generator, got storage.WorldStore) (mornleaServerHost, error) {
					assembled = got.Metadata().Difficulty
					return &mornleaServerTestHost{runErr: want}, nil
				},
			})
			if !errors.Is(err, want) || !listened || assembled != test.worldFactor {
				t.Fatalf("run error=%v listened=%v assembled=%s，期望 %v、已监听且世界难度 %s 生效",
					err, listened, assembled, want, test.worldFactor)
			}
		})
	}
}
