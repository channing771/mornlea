package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定难度的构造注入契约：难度是实体状态的构造期快照、此后只读；
// 缺省构造（既有测试夹具路径）表达 normal，三档显式构造各就各位，非法值
// 在构造边界 panic 而不是把未定义档位带进权威 tick。分档行为（饥饿伤害、
// 回血门控、生成门控）由 difficulty_starvation_test.go、
// difficulty_regen_test.go 与 hostile_spawn_test.go 各自锁定。

// TestNewStateDefaultsToNormalDifficulty 覆盖缺省构造：不传难度尾参的既有
// 路径必须表达 normal——这是大量既有夹具零改动继续成立的语义基础。
func TestNewStateDefaultsToNormalDifficulty(t *testing.T) {
	state := NewState(0)
	if state.difficulty != core.DifficultyNormal {
		t.Fatalf("缺省构造难度=%v，想要 %v", state.difficulty, core.DifficultyNormal)
	}
}

// TestNewStateAcceptsAllThreeDifficulties 覆盖三档显式构造：快照必须原样
// 保存调用方给出的域值，不做任何归一之外的改写。
func TestNewStateAcceptsAllThreeDifficulties(t *testing.T) {
	for _, difficulty := range []core.Difficulty{
		core.DifficultyNormal,
		core.DifficultyPeaceful,
		core.DifficultyHard,
	} {
		state := NewState(0, difficulty)
		if state.difficulty != difficulty {
			t.Fatalf("构造难度 %v 读回 %v", difficulty, state.difficulty)
		}
	}
}

// TestNewStatePanicsOnInvalidDifficulty 覆盖非法难度的稳定失败：越界值必须
// panic 于构造期，而不是进入权威 tick 后靠未定义分支消化。
func TestNewStatePanicsOnInvalidDifficulty(t *testing.T) {
	for _, invalid := range []core.Difficulty{core.Difficulty(3), core.Difficulty(255)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("构造难度 %d 未 panic", uint8(invalid))
				}
			}()
			_ = NewState(0, invalid)
		}()
	}
}

// TestNewStatePanicsOnAmbiguousDifficulty 覆盖尾参歧义：难度至多传一个，
// 传多个是装配错误，必须立刻失败而不是静默取其中一个。
func TestNewStatePanicsOnAmbiguousDifficulty(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("传入两个难度值未 panic")
		}
	}()
	_ = NewState(0, core.DifficultyNormal, core.DifficultyHard)
}

// TestFixtureEngineForwardsDifficulty 钉住夹具引擎的难度透传：分档用例经
// 夹具 `NewEngine` 构造，若透传缺失，全部难度用例测的其实是 normal。
func TestFixtureEngineForwardsDifficulty(t *testing.T) {
	engine := NewEngine(0, 0, 0, core.DifficultyHard)
	if engine.difficulty != core.DifficultyHard {
		t.Fatalf("夹具引擎难度=%v，想要 %v", engine.difficulty, core.DifficultyHard)
	}
	if plain := NewEngine(0, 0, 0); plain.difficulty != core.DifficultyNormal {
		t.Fatalf("夹具缺省难度=%v，想要 %v", plain.difficulty, core.DifficultyNormal)
	}
}
