package main

import (
	"testing"
)

// TestCountTaskCheckboxesBasic 锁定 mixed - [x] / - [ ] 计数。
func TestCountTaskCheckboxesBasic(t *testing.T) {
	lines := []string{
		"## 1. 固定形状配方",
		"- [x] 1.1 先写 RED",
		"- [x] 1.2 最小实现",
		"- [ ] 1.3 运行门禁",
		"- [ ] 1.4 收尾",
	}
	done, total := countTaskCheckboxes(lines)
	if done != 2 || total != 4 {
		t.Errorf("done=%d total=%d，想要 2/4", done, total)
	}
}

// TestCountTaskCheckboxesEmpty 锁定无勾选行（空文件/纯说明）返回 (0,0)。
func TestCountTaskCheckboxesEmpty(t *testing.T) {
	lines := []string{
		"",
		"# 说明",
		"普通段落",
		"```go",
		"- 一个没有勾选框的列表项",
		"```",
	}
	done, total := countTaskCheckboxes(lines)
	if done != 0 || total != 0 {
		t.Errorf("done=%d total=%d，想要 0/0", done, total)
	}
}

// TestCountTaskCheckboxesUppercase 锁定大写 [X] 视为已完成。
func TestCountTaskCheckboxesUppercase(t *testing.T) {
	lines := []string{"- [X] 大写完成项", "- [ ] 待办项"}
	done, total := countTaskCheckboxes(lines)
	if done != 1 || total != 2 {
		t.Errorf("done=%d total=%d，想要 1/2", done, total)
	}
}
