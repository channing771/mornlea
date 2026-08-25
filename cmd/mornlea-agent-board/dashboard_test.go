package main

// 本文件主题：内嵌首页 dashboard.html 的完整性。
//
// 背景：renderLogs 曾把 '\\n' 写成真实换行，导致内嵌 <script> 整段语法
// 错误、浏览器一个数据都不渲染（后端 /api/status 一切正常）。本文件锁定
// 三类可静态断言的性质：标题与六分区存在、零外部资源、内嵌脚本无跨行字符串
// （本页 JS 全部单引号字符串都应在同一行闭合，出现奇数个未转义单引号即失败）。

import (
	"os"
	"strings"
	"testing"
)

// TestEmbeddedPageCoreSections 断言内嵌页包含标题与六个数据分区锚点，且不引用
// 任何外部资源（无 http(s):// 与 <link>），保证页面离线可渲染。
func TestEmbeddedPageCoreSections(t *testing.T) {
	raw, err := os.ReadFile("dashboard.html")
	if err != nil {
		t.Fatalf("读取内嵌页面失败: %v", err)
	}
	page := string(raw)
	for _, want := range []string{
		"Mornlea Agent 执行看板",
		`id="agents"`, `id="chains"`, `id="tasks"`,
		`id="confirm"`, `id="prs"`, `id="logs"`,
		`setInterval(refresh, 5000)`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("内嵌页面缺少 %q", want)
		}
	}
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Errorf("内嵌页面引用了外部资源（http:// 或 https://），违反离线单页约束")
	}
}

// TestInlineScriptNoUnclosedString 提取 <script> 段，逐行统计未转义单引号：
// 本页 JS 无合法跨行字符串，任一行出现奇数个单引号即意味着字符串未闭合
// （历史上 lines.join(' 后跟真实换行导致的整页白屏即此类缺陷）。
func TestInlineScriptNoUnclosedString(t *testing.T) {
	raw, err := os.ReadFile("dashboard.html")
	if err != nil {
		t.Fatalf("读取内嵌页面失败: %v", err)
	}
	script := extractScript(string(raw))
	if script == "" {
		t.Fatal("未找到内嵌 <script> 段")
	}
	for i, line := range strings.Split(script, "\n") {
		n := countUnescapedQuotes(line)
		if n%2 != 0 {
			t.Errorf("第 %d 行单引号数量为奇数（字符串未闭合或跨行）: %q", i+1, line)
		}
	}
}

// extractScript 返回 dashboard.html 中首对 <script> 与 </script> 之间的内容。
func extractScript(page string) string {
	start := strings.Index(page, "<script>")
	if start < 0 {
		return ""
	}
	start += len("<script>")
	end := strings.Index(page[start:], "</script>")
	if end < 0 {
		return ""
	}
	return page[start : start+end]
}

// countUnescapedQuotes 统计一行中未被反斜杠转义的单引号数量。
func countUnescapedQuotes(line string) int {
	n := 0
	escaped := false
	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' {
			n++
		}
	}
	return n
}
