package main

import "testing"

// TestIsGuardFile 锁定 loop.guard* 过滤器：识别主链/具名链守卫，排除以 .bak 结尾的备份，
// 与 confirm 扫描的排除策略保持一致。
func TestIsGuardFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"loop.guard", true},
		{"loop.guard.codex", true},
		{"loop.guard.foo", true},
		{"loop.guard.bak", false},
		{"loop.guard.codex.bak", false},
		{"loop.guardcodex", true}, // 前缀匹配（非点分隔）也接受
		{"foo.log", false},
		{"guard.codex", false},
	}
	for _, c := range cases {
		if got := isGuardFile(c.name); got != c.want {
			t.Errorf("isGuardFile(%q) = %v，想要 %v", c.name, got, c.want)
		}
	}
}
