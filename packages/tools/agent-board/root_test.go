package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 根 go.mod 已随单元化解散，仓库根以 go.work 工作区清单标识。
	workspace := "go 1.26.0\n\nuse (\n\t./packages/tools\n)\n"
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(workspace), 0o644); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	got, ok := findRepoRoot(filepath.Join(root, "a", "b"))
	if !ok {
		t.Fatalf("从深层子目录应找到仓库根")
	}
	if got != root {
		t.Errorf("root = %q，想要 %q", got, root)
	}
	// 仓库根自身也应当能找到。
	if got, ok := findRepoRoot(root); !ok || got != root {
		t.Errorf("从仓库根自身查找应命中，got=%q ok=%v", got, ok)
	}
	// 无 go.work 的空目录应当返回 false。
	if _, ok := findRepoRoot(t.TempDir()); ok {
		t.Errorf("无 go.work 的目录不应命中")
	}
	// 只含单元 go.mod（不含 go.work）的目录不应命中：go.work 只出现在仓库根。
	wrong := filepath.Join(t.TempDir(), "unitrepo")
	if err := os.MkdirAll(wrong, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrong, "go.mod"), []byte("module github.com/channing771/mornlea/packages/tools\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := findRepoRoot(wrong); ok {
		t.Errorf("单元模块目录不应命中仓库根判定")
	}
}
