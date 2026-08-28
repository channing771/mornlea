package archcheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestProductionGoSourceScansSplitFiles(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"first.go":        "package sample\nfunc firstMarker() {}\n",
		"second.go":       "package sample\nfunc secondMarker() {}\n",
		"ignored_test.go": "package sample\nfunc ignoredMarker() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 子包目录同样必须纳入扫描：客户端命令拆出 app/capture/benchmark 子包后，
	// 生产源码不再全部平铺在顶层，扁平扫描会静默丢失子树覆盖。
	nested := filepath.Join(dir, "app")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "third.go"), []byte("package app\nfunc thirdMarker() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "third_test.go"), []byte("package app\nfunc thirdIgnoredMarker() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := productionGoSource(t, dir)
	for _, marker := range []string{"firstMarker", "secondMarker", "thirdMarker"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("production source missed marker %s: %q", marker, got)
		}
	}
	for _, ignored := range []string{"ignoredMarker", "thirdIgnoredMarker"} {
		if strings.Contains(got, ignored) {
			t.Fatalf("production source included test marker %s: %q", ignored, got)
		}
	}
}

func TestTopLevelDeclarationNamesInScansSplitFiles(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"session.go":              "package sample\nconst sessionMarker = 1\n",
		"session_reader.go":       "package sample\ntype sessionReaderMarker struct{}\n",
		"session_ignored_test.go": "package sample\nfunc sessionIgnoredMarker() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	names := topLevelDeclarationNamesIn(t, dir, "session*.go")
	for _, name := range []string{"sessionMarker", "sessionReaderMarker"} {
		if !names[name] {
			t.Errorf("split production files missed declaration %s", name)
		}
	}
	if names["sessionIgnoredMarker"] {
		t.Errorf("test file declaration must be ignored")
	}
}

// productionGoSource 拼接 directory 子树内全部生产 Go 源码（递归遍历，跳过
// `_test.go`）。必须递归：cmd/mornlea 的生产源码已按功能域拆入 app/capture/
// benchmark 子包，只扫顶层会让字符串级架构守卫静默丢失子包覆盖。
func productionGoSource(t *testing.T, directory string) string {
	t.Helper()
	var source strings.Builder
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source.Write(data)
		return nil
	})
	if err != nil {
		t.Fatalf("遍历 %s: %v", directory, err)
	}
	return source.String()
}

func topLevelDeclarationNamesIn(t *testing.T, directory, pattern string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("读取 %s: %v", directory, err)
	}
	slices.SortFunc(entries, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})
	names := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		matches, err := filepath.Match(pattern, name)
		if err != nil {
			t.Fatalf("匹配 %s: %v", pattern, err)
		}
		if entry.IsDir() || !matches || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(directory, name)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				names[declaration.Name.Name] = true
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						names[specification.Name.Name] = true
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							names[name.Name] = true
						}
					}
				}
			}
		}
	}
	return names
}

// isTunableDefaultName 判断标识符是否形如 defaultXxx（default 后紧跟大写字母）。
func isTunableDefaultName(name string) bool {
	const prefix = "default"
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok || rest == "" {
		return false
	}
	return unicode.IsUpper(rune(rest[0]))
}

func localName(importPath string) string {
	return strings.TrimPrefix(strings.TrimSpace(importPath), "github.com/channing771/mornlea/")
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("查找 go.mod 失败: %v", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}
