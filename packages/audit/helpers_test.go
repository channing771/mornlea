package archcheck_test

import (
	"encoding/json"
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
// `_test.go`）。必须递归：packages/client/cmd/mornlea 的生产源码已按功能域
// 拆入 app/capture/benchmark 子包，只扫顶层会让字符串级架构守卫静默丢失
// 子包覆盖。
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

// repositoryRoot 返回仓库根（go.work 所在目录）。根模块已解散，`go env GOMOD`
// 只会落到本单元的 go.mod，不能再当仓库根用；GOWORK 由工具链从 cwd 逐级向上
// 解析，恒指向仓库根的 go.work，是解散后唯一可靠的根定位方式。空值与字面量
// `off`（GOWORK=off）都必须 fail-closed：`filepath.Dir("off")` 会把根误解析为
// `.`，后续按根找 packages/ 的检查会因目录不存在而空转通过，掩盖真实原因。
func repositoryRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOWORK").Output()
	if err != nil {
		t.Fatalf("查找 go.work 失败: %v", err)
	}
	workspace := strings.TrimSpace(string(out))
	if workspace == "" || workspace == "off" {
		t.Fatal("GOWORK 为空或被 GOWORK=off 关闭：审计测试必须在 go.work 工作区内运行")
	}
	return filepath.Dir(workspace)
}

// workspaceModules 解析根 go.work 的 use 列表，返回各模块相对仓库根的斜杠
// 目录（如 packages/shared），排序去重。根模块解散后 use 列表就是全部 Go 单元
// 的唯一真相：枚举类检查以它为输入，新模块立项而忘记 use（或 use 指向已删除
// 目录）时在这里先红，而不是让某个 `go list` 静默漏一个模块。
func workspaceModules(t *testing.T) []string {
	t.Helper()
	root := repositoryRoot(t)
	command := exec.Command("go", "work", "edit", "-json")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("解析 go.work: %v", err)
	}
	var workspace struct {
		Use []struct {
			DiskPath string `json:"DiskPath"`
		} `json:"Use"`
	}
	if err := json.Unmarshal(output, &workspace); err != nil {
		t.Fatalf("解码 go.work JSON: %v", err)
	}
	if len(workspace.Use) == 0 {
		t.Fatal("go.work 的 use 列表为空：仓库必须直辖全部 Go 单元模块")
	}
	modules := make([]string, 0, len(workspace.Use))
	for _, use := range workspace.Use {
		// DiskPath 保持 go.work 里的书写形态（`./packages/shared`），相对仓库
		// 根归一；`.`（根模块复活）会保留为 "."，由 use 集一致性检查报红。
		diskPath := use.DiskPath
		if !filepath.IsAbs(diskPath) {
			diskPath = filepath.Join(root, diskPath)
		}
		relative, err := filepath.Rel(root, diskPath)
		if err != nil {
			t.Fatalf("计算模块 %s 的相对路径: %v", use.DiskPath, err)
		}
		modules = append(modules, filepath.ToSlash(relative))
	}
	slices.Sort(modules)
	// 排序后相邻去重：go.work 手工编辑可能重复 use 同一目录，折叠后与目录
	// 枚举得到的单元集做 `slices.Equal` 比较才有集合语义。
	modules = slices.Compact(modules)
	return modules
}

// listWorkspacePackages 逐模块运行 `go list` 并汇总输出行，format 是 -f 模板。
// 根模块解散后 `./...` 在仓库根不再可用（目录前缀不含任何 use 模块）且本就不
// 跨嵌套模块，必须按 workspaceModules 的模块目录分别执行。`-e` 的收益只在
// 直接点名目录的模式上兑现：这样命中的包即使整包被构建约束排除（Linux 下的
// 仅 darwin gfxspike）或交叉编译下无文件可建的 cgo 包，也会以 ImportPath
// 完好、imports 为空的带错形态列出，而不是让整个枚举非零退出；wildcard
// `./...` 与 `X/...` 在模式展开阶段就静默跳过这类包，`-e` 救不回来。因此
// 含整包平台约束的模块必须在 wildcard 之外经 perModule 追加点名目录的显式
// 模式（tools 模块的 `./gfxspike` 即为此）；perModule 另用于收窄范围——
// client 模块的命令子树由 clientCommandAllowedEdges 单独治理，不混入全局
// 枚举。
func listWorkspacePackages(t *testing.T, format string, perModule map[string][]string) []string {
	t.Helper()
	root := repositoryRoot(t)
	var lines []string
	for _, module := range workspaceModules(t) {
		patterns := []string{"./..."}
		if override := perModule[module]; len(override) > 0 {
			patterns = override
		}
		command := exec.Command("go", append([]string{"list", "-e", "-f", format}, patterns...)...)
		command.Dir = filepath.Join(root, module)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("go list %s %v: %v", module, patterns, err)
		}
		if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
			lines = append(lines, strings.Split(trimmed, "\n")...)
		}
	}
	return lines
}
