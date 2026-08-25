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
	gomod := "module github.com/channing771/mornlea\n\ngo 1.26.0\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
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
	// 无 go.mod 的空目录应当返回 false。
	if _, ok := findRepoRoot(t.TempDir()); ok {
		t.Errorf("无 go.mod 的目录不应命中")
	}
	// 含非目标 module 的 go.mod 不应命中。
	wrong := filepath.Join(t.TempDir(), "wrongrepo")
	if err := os.MkdirAll(wrong, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrong, "go.mod"), []byte("module example.com/other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := findRepoRoot(wrong); ok {
		t.Errorf("非本 module 的 go.mod 不应命中")
	}
}
