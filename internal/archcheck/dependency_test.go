package archcheck_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// allowed 列出每个内部包允许直接依赖的内部包。
//
// internal/server → internal/physics：M5B 任务编排（m5b-companion-planning-fifo）
// 经 sim.CompanionAction 提交伙伴移动输入，该 API 的字段类型就是 physics.Input；
// server 只消费物理的值类型与 ActiveTunables 快照（发令者射线的眼高），方向是
// 编排层依赖领域值类型，与 sim→physics 同向，不引入反向耦合。
//
// internal/sim → internal/fluid（变更 authoritative-fluid，design D2）：流动的调度
// 算法（待更新队列、全序排序、封闭盆地上的不动点收敛）被拆成独立包，使其能脱离权威
// 引擎被穷举性质测试；流动的数据所有权仍在权威模拟的 sim，fluid 只暴露
// Advance(now, FluidWorld, budget) 这样的无状态纯函数入口，不持有世界。
// fluid 只允许依赖 internal/core（当前实现未依赖 internal/world，即便设计意图允许，
// 也不预先登记未使用的边）；fluid MUST NOT 反向依赖 sim/network/render/storage，
// 否则它会退化成 sim 的内部实现，丧失独立测试的意义。
var allowed = map[string][]string{
	"internal/archcheck":   {},
	"internal/audio":       {},
	"internal/companion":   {"internal/core", "internal/pathfind"},
	"internal/core":        {"internal/nativeabi"},
	"internal/nativeabi":   {},
	"internal/config":      {"internal/companion", "internal/core", "internal/physics", "internal/sim", "internal/logging"},
	"internal/fluid":       {"internal/core"},
	"internal/physics":     {"internal/core", "internal/nativeabi"},
	"internal/pathfind":    {"internal/core"},
	"internal/logging":     {},
	"internal/network":     {"internal/companion", "internal/core"},
	"internal/network/tcp": {"internal/network"},
	"internal/profile":     {"internal/core"},
	"internal/sim":         {"internal/companion", "internal/core", "internal/fluid", "internal/physics", "internal/world"},
	"internal/storage":     {"internal/companion", "internal/core", "internal/storage/storagedef", "internal/world"},
	// internal/storage/storagedef 是世界存储的哨兵错误叶子（ErrCorrupt/
	// ErrFutureVersion 的公共下沉）：region 与四个实体域子包都经它取哨兵，
	// 自身不得依赖任何 internal 包，否则叶子就失去了斩断 root↔子包循环的作用。
	"internal/storage/storagedef": {},
	"internal/world":              {"internal/core"},
	"internal/worldgen":           {"internal/core", "internal/world", "internal/nativeabi"},
	"internal/mesh":               {"internal/core", "internal/world", "internal/nativeabi"},
	// internal/lod 只做远环壳的请求编码、quad 解码与编排(依赖方向镜像
	// worldgen/mesh:core 提供 ChunkPos 等领域类型,nativeabi 是唯一 engine
	// ABI 入口);按 design 裁决不得依赖 render/sim/network。
	"internal/lod":        {"internal/core", "internal/nativeabi"},
	"internal/assets":     {"internal/core", "internal/world", "internal/mesh", "internal/worldgen"},
	"internal/render":     {"internal/core", "internal/world", "internal/mesh", "internal/assets"},
	"internal/render/hud": {"internal/core", "internal/mesh", "internal/assets", "internal/render"},
	"internal/server":     {"internal/companion", "internal/core", "internal/network", "internal/pathfind", "internal/physics", "internal/world", "internal/worldgen", "internal/sim", "internal/storage"},
	"internal/client":     {"internal/companion", "internal/core", "internal/physics", "internal/network", "internal/world", "internal/mesh", "internal/assets", "internal/render"},
}

func TestInternalDependenciesAreOneWay(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \" \"}}", "./internal/...")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("枚举 internal 包失败: %v", err)
	}

	actual := make(map[string]bool)
	imports := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		pkg := localName(parts[0])
		actual[pkg] = true
		if _, ok := allowed[pkg]; !ok {
			t.Errorf("新增内部包 %s 未登记依赖白名单", pkg)
			continue
		}
		if len(parts) == 2 {
			imports[pkg] = strings.Fields(parts[1])
		}
	}
	for pkg := range allowed {
		if !actual[pkg] {
			t.Errorf("依赖白名单中的包 %s 不存在", pkg)
		}
	}

	for pkg, packageImports := range imports {
		allowSet := make(map[string]bool, len(allowed[pkg]))
		for _, dependency := range allowed[pkg] {
			allowSet[dependency] = true
		}
		for _, importPath := range packageImports {
			dependency := localName(importPath)
			if dependency == importPath {
				continue
			}
			if !allowSet[dependency] {
				t.Errorf("%s 不允许直接依赖 %s", pkg, dependency)
			}
		}
	}
}

// clientCommandAllowedEdges 列出客户端命令子树 `cmd/mornlea` 允许的包间依赖
// 边（本地 import path）。依赖方向契约为：薄 main 装配三个功能域子包；
// capture 与 benchmark 各自依赖 app；app 不反向依赖 capture/benchmark，
// capture 与 benchmark 互不依赖——否则两侧的重型测试会经由包图重新耦合，
// `go test` 的单包定点与测试分层同时失效。main 是 `package main`，语言层面
// 不可被子包导入，但允许边表仍显式留空该方向，让契约可读而非依赖编译器兜底。
var clientCommandAllowedEdges = map[string][]string{
	"cmd/mornlea":           {"cmd/mornlea/app", "cmd/mornlea/benchmark", "cmd/mornlea/capture"},
	"cmd/mornlea/app":       {},
	"cmd/mornlea/benchmark": {"cmd/mornlea/app"},
	"cmd/mornlea/capture":   {"cmd/mornlea/app"},
}

// clientCommandRequiredEdges 是必须真实存在的装配边：main 必须装配全部三个
// 子包，capture 与 benchmark 必须经 app 访问宿主状态。只查「禁止边」守不住
// 悄悄改走旁路的装配（例如 main 绕过 capture 的公开入口直取其内部符号），
// 必需边消失同样是方向契约的漂移。
var clientCommandRequiredEdges = map[string][]string{
	"cmd/mornlea":           {"cmd/mornlea/app", "cmd/mornlea/benchmark", "cmd/mornlea/capture"},
	"cmd/mornlea/benchmark": {"cmd/mornlea/app"},
	"cmd/mornlea/capture":   {"cmd/mornlea/app"},
}

// isClientCommandPackage 报告本地 import path 是否落在客户端命令子树内。
func isClientCommandPackage(localPath string) bool {
	return localPath == "cmd/mornlea" || strings.HasPrefix(localPath, "cmd/mornlea/")
}

// clientCommandDependencyViolations 对照允许边表与必需边表检查给定的包依赖
// 边，返回全部违规描述；空切片表示符合契约。输入是「本地包路径 → 生产
// import 的本地路径有序列表」，与真实目录解耦，使「注入反向边」的失败路径
// 可以在纯内存中核对，不必改动源码树。
func clientCommandDependencyViolations(edges map[string][]string) []string {
	var violations []string
	for pkg, packageImports := range edges {
		allowed, registered := clientCommandAllowedEdges[pkg]
		if !registered {
			violations = append(violations, fmt.Sprintf("客户端命令子树新增包 %s 未登记依赖白名单", pkg))
			continue
		}
		allowSet := make(map[string]bool, len(allowed))
		for _, dependency := range allowed {
			allowSet[dependency] = true
		}
		for _, dependency := range packageImports {
			if !isClientCommandPackage(dependency) || allowSet[dependency] {
				continue
			}
			violations = append(violations, fmt.Sprintf(
				"客户端命令包 %s 不允许依赖 %s：方向必须是 main → app/capture/benchmark、capture/benchmark → app，且 capture 与 benchmark 互不依赖",
				pkg, dependency))
		}
	}
	// 逐包核对必需边；包整体被删除时由「必需边找不到宿主」统一暴露。
	for pkg, required := range clientCommandRequiredEdges {
		importSet := make(map[string]bool, len(edges[pkg]))
		for _, dependency := range edges[pkg] {
			importSet[dependency] = true
		}
		for _, dependency := range required {
			if !importSet[dependency] {
				violations = append(violations, fmt.Sprintf("客户端命令包 %s 缺少必需依赖边 → %s", pkg, dependency))
			}
		}
	}
	slices.Sort(violations)
	return violations
}

// TestClientCommandSubpackageDependencyDirections 把真实源码树的包依赖边喂给
// `clientCommandDependencyViolations`，钉住客户端命令的依赖方向契约。
//
// 边的来源是逐文件解析生产 .go（parser.ImportsOnly 模式，跳过 `_test.go`），
// 而不是 `go list`：`go list` 的导入边随 GOOS 与构建约束变化——带
// `//go:build darwin` 的包（main/app/benchmark）在 Linux 下整个加载失败
// （报 build constraints exclude all Go files，`.Imports` 为空），不带 tag 的
// capture 反而照常报出完整导入边；建立在 `go list` 上的断言因此随运行平台
// 翻转。源码级 AST 解析与平台无关，两个 CI 平台看到同一份边集。测试文件不
// 计入——子包测试经 app 包导出的测试装配入口（`testkit.go`）复用是合法的，
// 不应污染生产依赖方向。
func TestClientCommandSubpackageDependencyDirections(t *testing.T) {
	edges := clientCommandImportEdges(t)
	if violations := clientCommandDependencyViolations(edges); len(violations) > 0 {
		t.Errorf("客户端命令依赖方向违反契约，%d 条违规：\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// clientCommandImportEdges 扫描 `cmd/mornlea` 子树内全部含生产 Go 源文件的
// 目录，返回「本地包路径 → 去重排序后的本地生产 import 列表」。子树内出现
// 未登记的新包目录时由检查器的白名单核对报错，新增子包不可能静默绕过。
func clientCommandImportEdges(t *testing.T) map[string][]string {
	t.Helper()
	root := moduleRoot(t)
	subtree := filepath.Join(root, "cmd", "mornlea")
	edges := make(map[string][]string)
	err := filepath.WalkDir(subtree, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		imports := make(map[string]bool)
		for _, file := range entries {
			name := file.Name()
			if file.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(path, name), nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imported := range parsed.Imports {
				importPath := strings.Trim(imported.Path.Value, `"`)
				if local := localName(importPath); local != importPath {
					imports[local] = true
				}
			}
		}
		if !hasProductionGoFile(entries) {
			// testdata 等纯资产目录不是包，跳过；含生产源文件的包即使没有
			// 任何本地 import 也要登记（如纯叶子包），否则白名单核对漏检。
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		edges[filepath.ToSlash(relative)] = slices.Sorted(maps.Keys(imports))
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 %s: %v", subtree, err)
	}
	return edges
}

// hasProductionGoFile 报告目录项中是否存在非 `_test.go` 的 Go 源文件。
func hasProductionGoFile(entries []os.DirEntry) bool {
	for _, file := range entries {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".go") && !strings.HasSuffix(file.Name(), "_test.go") {
			return true
		}
	}
	return false
}

// TestClientCommandDependencyViolationsDetectDrift 用合成边核对检查器对每类
// 漂移都真的报错。真实源码树处于契约内时，方向断言的负向路径没有天然的失败
// 信号；必须另有合成断言钉住检查器本身，否则检查逻辑被改坏后门禁静默变松。
func TestClientCommandDependencyViolationsDetectDrift(t *testing.T) {
	// contractEdges 是恰好满足契约的最小合成边集。
	contractEdges := func() map[string][]string {
		return map[string][]string{
			"cmd/mornlea":           {"cmd/mornlea/app", "cmd/mornlea/benchmark", "cmd/mornlea/capture"},
			"cmd/mornlea/app":       {},
			"cmd/mornlea/benchmark": {"cmd/mornlea/app"},
			"cmd/mornlea/capture":   {"cmd/mornlea/app"},
		}
	}
	if violations := clientCommandDependencyViolations(contractEdges()); len(violations) != 0 {
		t.Fatalf("契约内的合成边不应报违规: %v", violations)
	}

	for _, edge := range []struct {
		name string
		from string
		to   string
	}{
		{"app反向依赖capture", "cmd/mornlea/app", "cmd/mornlea/capture"},
		{"app反向依赖benchmark", "cmd/mornlea/app", "cmd/mornlea/benchmark"},
		{"capture依赖benchmark", "cmd/mornlea/capture", "cmd/mornlea/benchmark"},
		{"benchmark依赖capture", "cmd/mornlea/benchmark", "cmd/mornlea/capture"},
	} {
		t.Run(edge.name, func(t *testing.T) {
			edges := contractEdges()
			edges[edge.from] = append(edges[edge.from], edge.to)
			violations := clientCommandDependencyViolations(edges)
			if len(violations) != 1 || !strings.Contains(violations[0], edge.from+" 不允许依赖 "+edge.to) {
				t.Fatalf("注入禁止边 %s → %s 未被拒绝: %v", edge.from, edge.to, violations)
			}
		})
	}

	t.Run("必需边缺失", func(t *testing.T) {
		edges := contractEdges()
		edges["cmd/mornlea"] = nil
		violations := clientCommandDependencyViolations(edges)
		if len(violations) != 3 {
			t.Fatalf("main 卸掉三个子包的装配应报 3 条缺失: %v", violations)
		}
	})

	t.Run("未登记新包", func(t *testing.T) {
		edges := contractEdges()
		edges["cmd/mornlea/widgets"] = []string{"cmd/mornlea/app"}
		violations := clientCommandDependencyViolations(edges)
		if len(violations) != 1 || !strings.Contains(violations[0], "未登记依赖白名单") {
			t.Fatalf("未登记子包未被拒绝: %v", violations)
		}
	})
}
