package runtime

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定权威引擎的难度注入边界：`NewEngine` 的可选尾参缺省表达
// normal（既有调用点零改动），生产装配显式传入 metadata 难度；非法值经
// entity 构造边界统一 panic，引擎自身不再定义第二套校验。

// TestNewEngineDefaultsToNormalDifficulty 覆盖缺省构造：不传难度尾参的既有
// 装配路径必须表达 normal。
func TestNewEngineDefaultsToNormalDifficulty(t *testing.T) {
	engine := NewEngine(2, 0, 1)
	if got := engine.DifficultyForTest(); got != core.DifficultyNormal {
		t.Fatalf("缺省构造难度=%v，想要 %v", got, core.DifficultyNormal)
	}
}

// TestNewEngineAcceptsAllThreeDifficulties 覆盖三档显式注入：快照原样保存。
func TestNewEngineAcceptsAllThreeDifficulties(t *testing.T) {
	for _, difficulty := range []core.Difficulty{
		core.DifficultyNormal,
		core.DifficultyPeaceful,
		core.DifficultyHard,
	} {
		engine := NewEngine(2, 0, 1, difficulty)
		if got := engine.DifficultyForTest(); got != difficulty {
			t.Fatalf("构造难度 %v 读回 %v", difficulty, got)
		}
	}
}

// TestNewEnginePanicsOnInvalidDifficulty 覆盖非法难度的稳定失败：拒绝发生在
// 构造期（由 entity 边界统一执行），不得带着未定义档位进入首个权威 tick。
func TestNewEnginePanicsOnInvalidDifficulty(t *testing.T) {
	for _, invalid := range []core.Difficulty{core.Difficulty(3), core.Difficulty(255)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("构造难度 %d 未 panic", uint8(invalid))
				}
			}()
			_ = NewEngine(2, 0, 1, invalid)
		}()
	}
}

// TestNewEnginePanicsOnAmbiguousDifficulty 覆盖尾参歧义：难度至多传一个。
func TestNewEnginePanicsOnAmbiguousDifficulty(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("传入两个难度值未 panic")
		}
	}()
	_ = NewEngine(2, 0, 1, core.DifficultyNormal, core.DifficultyPeaceful)
}
