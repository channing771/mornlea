package core

import (
	"strings"
	"testing"
)

// difficulty_test.go：难度域值的完整契约——恰好三档且零值为 `DifficultyNormal`，
// `String` 与 `ParseDifficulty` 严格互逆，非法文本与越界值稳定失败，不进权威状态。

func TestDifficultyValueDomain(t *testing.T) {
	// 三个固定值钉死位模式：metadata v6 落盘字节与权威模拟构造边界都消费它们，
	// 重排或改值会直接破坏存档兼容。
	if DifficultyNormal != 0 || DifficultyPeaceful != 1 || DifficultyHard != 2 {
		t.Fatalf(
			"难度值域 = (%d, %d, %d)，想要 (0, 1, 2)",
			DifficultyNormal, DifficultyPeaceful, DifficultyHard,
		)
	}
	// 零值必须落在 normal：旧档迁移与未显式指定难度的构造都以零值表达默认档。
	var zero Difficulty
	if zero != DifficultyNormal {
		t.Fatalf("零值难度 = %d，想要 normal(0)", uint8(zero))
	}
	for _, difficulty := range []Difficulty{DifficultyNormal, DifficultyPeaceful, DifficultyHard} {
		if !difficulty.Valid() {
			t.Fatalf("合法难度 %d 被判非法", uint8(difficulty))
		}
	}
	for _, difficulty := range []Difficulty{3, 4, 42, 255} {
		if difficulty.Valid() {
			t.Fatalf("越界难度 %d 被判合法", uint8(difficulty))
		}
	}
}

func TestDifficultyStringParseRoundTrip(t *testing.T) {
	for _, difficulty := range []Difficulty{DifficultyNormal, DifficultyPeaceful, DifficultyHard} {
		text := difficulty.String()
		parsed, err := ParseDifficulty(text)
		if err != nil || parsed != difficulty {
			t.Fatalf("往返 %q = %d, err=%v，想要 %d", text, uint8(parsed), err, uint8(difficulty))
		}
	}
	// 解析严格小写：大写、前后空白、空串与未知文本一律失败，不承担任何宽容。
	for _, text := range []string{
		"Normal", "PEACEFUL", "Hard", "hard ", " hard", "", "nightmare", "标准", "normal\n",
	} {
		if _, err := ParseDifficulty(text); err == nil {
			t.Fatalf("接受了非法难度文本 %q", text)
		}
	}
}

func TestDifficultyStringInvalidIsDiagnostic(t *testing.T) {
	// 非法值的 `String` 只用于诊断输出：必须可区分且不与三档规范文本冲突，
	// 防止日志把越界值伪装成合法档位。
	got := Difficulty(3).String()
	if got == "normal" || got == "peaceful" || got == "hard" {
		t.Fatalf("非法难度 3 的文本 %q 与规范档位冲突", got)
	}
	if !strings.Contains(got, "3") {
		t.Fatalf("非法难度 3 的文本 %q 未携带数值，无法定位坏值来源", got)
	}
}
