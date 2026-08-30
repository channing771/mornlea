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
// internal/server → internal/physics：伙伴任务编排经 CompanionAction 提交移动输入，
// 该 API 的字段类型就是 physics.Input；发令者与伙伴交互几何从 tick 入口捕获的
// runtime.TickTunables 显式消费 physics 值，不在 server 内重读活动快照。该方向是
// 编排层依赖领域值类型，与 sim→physics 同向，不引入反向耦合。
//
// internal/sim/realm → internal/fluid：流动的调度
// 算法（待更新队列、全序排序、封闭盆地上的不动点收敛）被拆成独立包，使其能脱离权威
// 引擎被穷举性质测试；流动的数据所有权仍在 realm，fluid 只暴露
// Advance(now, FluidWorld, budget) 这样的无状态纯函数入口，不持有世界。
// internal/fluid → internal/nativeabi（变更 rust-engine-fluid）：Advance 的单格
// 流体规则求值经 FluidEvalBatch 批量送入 Rust engine kernel（eval_native.go 是
// 包内唯一调用点），Go 侧保留全部编排（调度、预算、冲突合并、排序提交），kernel
// 只做逐项无状态纯函数求值。fluid 除此之外只允许依赖 internal/core（当前实现未
// 依赖 internal/world，即便设计意图允许，也不预先登记未使用的边）；fluid MUST
// NOT 反向依赖 sim/network/render/storage，否则它会退化成 sim 的内部实现，丧失
// 独立测试的意义。
var allowed = map[string][]string{
	"internal/archcheck":        {},
	"internal/audio":            {},
	"internal/companion":        {"internal/core", "internal/pathfind"},
	"internal/core":             {"internal/nativeabi"},
	"internal/nativeabi":        {},
	"internal/config":           {"internal/companion", "internal/core", "internal/physics", "internal/sim/tuning", "internal/logging"},
	"internal/fluid":            {"internal/core", "internal/nativeabi"},
	"internal/physics":          {"internal/core", "internal/nativeabi"},
	"internal/pathfind":         {"internal/core"},
	"internal/logging":          {},
	"internal/network":          {"internal/core", "internal/network/codec", "internal/network/protocol"},
	"internal/network/codec":    {"internal/core", "internal/network/protocol"},
	"internal/network/protocol": {"internal/companion", "internal/core"},
	"internal/network/tcp":      {"internal/network"},
	"internal/profile":          {"internal/core"},
	"internal/sim/contract":     {"internal/companion", "internal/core", "internal/physics", "internal/world"},
	"internal/sim/entity":       {"internal/companion", "internal/core", "internal/physics", "internal/world", "internal/sim/contract", "internal/sim/realm", "internal/sim/tuning"},
	"internal/sim/realm":        {"internal/core", "internal/fluid", "internal/world"},
	"internal/sim/runtime":      {"internal/companion", "internal/core", "internal/physics", "internal/world", "internal/sim/contract", "internal/sim/entity", "internal/sim/realm", "internal/sim/tuning"},
	"internal/sim/tuning":       {"internal/core"},
	"internal/storage": {
		"internal/core",
		"internal/storage/chunk", "internal/storage/companion", "internal/storage/hostile",
		"internal/storage/player", "internal/storage/region",
		"internal/storage/storagedef", "internal/world",
	},
	"internal/storage/chunk":      {"internal/core", "internal/storage/region", "internal/storage/storagedef", "internal/world"},
	"internal/storage/player":     {"internal/core", "internal/storage/storagedef"},
	"internal/storage/companion":  {"internal/companion", "internal/core", "internal/storage/storagedef"},
	"internal/storage/hostile":    {"internal/core", "internal/storage/storagedef"},
	"internal/storage/region":     {"internal/core", "internal/storage/storagedef"},
	"internal/storage/storagedef": {},
	"internal/world":              {"internal/core"},
	"internal/worldgen":           {"internal/core", "internal/world", "internal/nativeabi"},
	"internal/mesh":               {"internal/core", "internal/world", "internal/nativeabi"},
	"internal/lod":                {"internal/core", "internal/nativeabi"},
	"internal/assets":             {"internal/core", "internal/world", "internal/mesh", "internal/worldgen"},
	"internal/render":             {"internal/core", "internal/world", "internal/mesh", "internal/assets"},
	"internal/render/hud":         {"internal/core", "internal/mesh", "internal/assets", "internal/render"},
	"internal/server":             {"internal/companion", "internal/core", "internal/network", "internal/pathfind", "internal/physics", "internal/world", "internal/worldgen", "internal/sim/contract", "internal/sim/runtime", "internal/storage", "internal/server/persistence"},
	"internal/server/persistence": {"internal/companion", "internal/core", "internal/physics", "internal/sim/contract", "internal/sim/runtime", "internal/storage"},
	"internal/client":             {"internal/companion", "internal/core", "internal/physics", "internal/network", "internal/world", "internal/mesh", "internal/assets", "internal/render"},
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

// TestServerPersistenceDoesNotDependOnServer 钉住持久化子包到根包的反向依赖禁令。
//
// 生产代码（未带 persistence_contract 构建约束的文件）不得导入 `internal/server`；
// 唯一的例外是被 //go:build persistence_contract 门控的契约测试文件，其为校验
// 根包兼容 re-export 与哨兵恒等而显式依赖 `internal/server`。
func TestServerPersistenceDoesNotDependOnServer(t *testing.T) {
	if slices.Contains(allowed["internal/server/persistence"], "internal/server") {
		t.Fatalf("internal/server/persistence 不允许依赖 internal/server：子包不得反向依赖父包")
	}
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "server", "persistence")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取 %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s: %v", path, err)
		}
		if strings.Contains(string(data), "persistence_contract") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("解析 %s: %v", path, err)
		}
		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			if localName(importPath) == "internal/server" {
				t.Errorf("持久化生产文件 %s 不允许导入 internal/server（仅 //go:build persistence_contract 的契约文件可导入）", entry.Name())
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

// simAllowedEdges 列出权威模拟子树 `internal/sim` 五个子包允许的内部依赖
// 边（本地 import path）。依赖方向契约为：`contract`/`tuning` 为叶子，
// `realm` 只依赖环境，`entity` 可依赖 `contract`/`tuning`/`realm`，
// `runtime` 是唯一允许同时编排其余四者的权威入口；任何反向边都会让
// 事务边界或快照所有权重新耦合，`go test` 的单包定点与事务收敛同时失效。
// 该表与全局 `allowed` 中对应五项保持一致，重复列出是为了让子树检查器
// 可在不依赖全局表的情况下独立校验方向与合成反向边。
var simAllowedEdges = map[string][]string{
	"internal/sim/contract": {"internal/companion", "internal/core", "internal/physics", "internal/world"},
	"internal/sim/tuning":   {"internal/core"},
	"internal/sim/realm":    {"internal/core", "internal/fluid", "internal/world"},
	"internal/sim/entity":   {"internal/companion", "internal/core", "internal/physics", "internal/world", "internal/sim/contract", "internal/sim/realm", "internal/sim/tuning"},
	"internal/sim/runtime":  {"internal/companion", "internal/core", "internal/physics", "internal/world", "internal/sim/contract", "internal/sim/entity", "internal/sim/realm", "internal/sim/tuning"},
}

// simRequiredEdges 是模拟子树必须真实存在的编排边：`runtime` 必须同时
// 装配四个下层子包，否则权威 tick 的阶段编排会悄悄绕过某层状态的所有权
// 边界，单 mutation 提交路径不再覆盖全部写入。
var simRequiredEdges = map[string][]string{
	"internal/sim/runtime": {"internal/sim/contract", "internal/sim/entity", "internal/sim/realm", "internal/sim/tuning"},
}

// TestSimAllowedEdgesMatchesGlobalAllowed 校验模拟子树的局部白名单
// 与全局 `allowed` 的对应项完全一致，防止双真相表静默漂移。
func TestSimAllowedEdgesMatchesGlobalAllowed(t *testing.T) {
	for pkg, simAllowed := range simAllowedEdges {
		globalAllowed, ok := allowed[pkg]
		if !ok {
			t.Fatalf("模拟子树包 %s 未在全局 allowed 中登记", pkg)
		}
		sortedSim := slices.Clone(simAllowed)
		slices.Sort(sortedSim)
		sortedGlobal := slices.Clone(globalAllowed)
		slices.Sort(sortedGlobal)
		if !slices.Equal(sortedSim, sortedGlobal) {
			t.Errorf("模拟子树包 %s 的局部白名单与全局 allowed 不一致：局部 %v，全局 %v", pkg, sortedSim, sortedGlobal)
		}
	}
	for pkg := range allowed {
		if isSimPackage(pkg) {
			if _, ok := simAllowedEdges[pkg]; !ok {
				t.Errorf("全局 allowed 中的模拟子树包 %s 未在 simAllowedEdges 中登记", pkg)
			}
		}
	}
}

// isSimPackage 报告本地 import path 是否落在权威模拟子树内。
func isSimPackage(localPath string) bool {
	return localPath == "internal/sim" || strings.HasPrefix(localPath, "internal/sim/")
}

// simDependencyViolations 对照模拟子树的允许边表与必需边表检查给定的
// 包依赖边，返回全部违规描述；空切片表示符合契约。输入是「本地包路径 →
// 生产 import 的本地路径有序列表」，与真实目录解耦，使「注入反向边」
// 的失败路径可以在纯内存中核对，不必改动源码树。
func simDependencyViolations(edges map[string][]string) []string {
	var violations []string
	for pkg, packageImports := range edges {
		allowed, registered := simAllowedEdges[pkg]
		if !registered {
			if isSimPackage(pkg) {
				violations = append(violations, fmt.Sprintf("模拟子树新增包 %s 未登记依赖白名单", pkg))
			}
			continue
		}
		allowSet := make(map[string]bool, len(allowed))
		for _, dependency := range allowed {
			allowSet[dependency] = true
		}
		for _, dependency := range packageImports {
			if allowSet[dependency] {
				continue
			}
			violations = append(violations, fmt.Sprintf(
				"模拟子包 %s 不允许依赖 %s：方向必须是 contract/tuning 互不依赖且不依赖 realm/entity/runtime、realm 不依赖 contract/tuning/entity/runtime、entity 不依赖 runtime、runtime 编排其余四者",
				pkg, dependency))
		}
	}
	// 逐包核对必需边；包整体被删除时由「必需边找不到宿主」统一暴露。
	for pkg, required := range simRequiredEdges {
		importSet := make(map[string]bool, len(edges[pkg]))
		for _, dependency := range edges[pkg] {
			importSet[dependency] = true
		}
		for _, dependency := range required {
			if !importSet[dependency] {
				violations = append(violations, fmt.Sprintf("模拟子包 %s 缺少必需依赖边 → %s", pkg, dependency))
			}
		}
	}
	slices.Sort(violations)
	return violations
}

// simImportEdges 扫描 `internal/sim` 子树内全部含生产 Go 源文件的目录，
// 返回「本地包路径 → 去重排序后的本地生产 import 列表」。子树内出现
// 未登记的新包目录时由检查器的白名单核对报错，新增子包不可能静默绕过。
func simImportEdges(t *testing.T) map[string][]string {
	t.Helper()
	root := moduleRoot(t)
	subtree := filepath.Join(root, "internal", "sim")
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

// TestSimSubpackageDependencyDirections 把真实源码树的模拟子树包依赖边
// 喂给 `simDependencyViolations`，钉住权威模拟的依赖方向契约。
//
// 边的来源是逐文件解析生产 .go（parser.ImportsOnly 模式，跳过 `_test.go`），
// 与 `TestClientCommandSubpackageDependencyDirections` 同理保持平台无关。
// 测试文件不计入——子包测试经同包或跨包测试装配复用是合法的，不应污染
// 生产依赖方向；全局 `TestInternalDependenciesAreOneWay` 另以 `go list`
// 覆盖全仓内部包的完整白名单，本测试聚焦模拟子树内部的方向与必需边。
func TestSimSubpackageDependencyDirections(t *testing.T) {
	edges := simImportEdges(t)
	if violations := simDependencyViolations(edges); len(violations) > 0 {
		t.Errorf("模拟子树依赖方向违反契约，%d 条违规：\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestSimDependencyViolationsDetectDrift 用合成边核对检查器对每类漂移
// 都真的报错。真实源码树处于契约内时，方向断言的负向路径没有天然的
// 失败信号；必须另有合成断言钉住检查器本身，否则检查逻辑被改坏后门禁
// 静默变松。
func TestSimDependencyViolationsDetectDrift(t *testing.T) {
	contractEdges := func() map[string][]string {
		return map[string][]string{
			"internal/sim/contract": {"internal/core", "internal/world", "internal/companion", "internal/physics"},
			"internal/sim/tuning":   {"internal/core"},
			"internal/sim/realm":    {"internal/core", "internal/fluid", "internal/world"},
			"internal/sim/entity":   {"internal/companion", "internal/core", "internal/physics", "internal/world", "internal/sim/contract", "internal/sim/realm", "internal/sim/tuning"},
			"internal/sim/runtime":  {"internal/companion", "internal/core", "internal/physics", "internal/world", "internal/sim/contract", "internal/sim/entity", "internal/sim/realm", "internal/sim/tuning"},
		}
	}
	if violations := simDependencyViolations(contractEdges()); len(violations) != 0 {
		t.Fatalf("契约内的合成边不应报违规: %v", violations)
	}

	for _, edge := range []struct {
		name string
		from string
		to   string
	}{
		{"contract反向依赖tuning", "internal/sim/contract", "internal/sim/tuning"},
		{"contract反向依赖realm", "internal/sim/contract", "internal/sim/realm"},
		{"contract反向依赖entity", "internal/sim/contract", "internal/sim/entity"},
		{"contract反向依赖runtime", "internal/sim/contract", "internal/sim/runtime"},
		{"tuning反向依赖contract", "internal/sim/tuning", "internal/sim/contract"},
		{"tuning反向依赖realm", "internal/sim/tuning", "internal/sim/realm"},
		{"tuning反向依赖entity", "internal/sim/tuning", "internal/sim/entity"},
		{"tuning反向依赖runtime", "internal/sim/tuning", "internal/sim/runtime"},
		{"realm反向依赖contract", "internal/sim/realm", "internal/sim/contract"},
		{"realm反向依赖tuning", "internal/sim/realm", "internal/sim/tuning"},
		{"realm反向依赖entity", "internal/sim/realm", "internal/sim/entity"},
		{"realm反向依赖runtime", "internal/sim/realm", "internal/sim/runtime"},
		{"entity反向依赖runtime", "internal/sim/entity", "internal/sim/runtime"},
	} {
		t.Run(edge.name, func(t *testing.T) {
			edges := contractEdges()
			edges[edge.from] = append(edges[edge.from], edge.to)
			violations := simDependencyViolations(edges)
			if len(violations) != 1 || !strings.Contains(violations[0], edge.from+" 不允许依赖 "+edge.to) {
				t.Fatalf("注入禁止边 %s → %s 未被拒绝: %v", edge.from, edge.to, violations)
			}
		})
	}

	t.Run("必需边缺失", func(t *testing.T) {
		edges := contractEdges()
		edges["internal/sim/runtime"] = []string{"internal/sim/contract", "internal/sim/realm", "internal/sim/tuning"}
		violations := simDependencyViolations(edges)
		found := false
		for _, violation := range violations {
			if strings.Contains(violation, "缺少必需依赖边") && strings.Contains(violation, "internal/sim/entity") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("runtime 卸掉 entity 装配应报缺失: %v", violations)
		}
	})

	t.Run("未登记新包", func(t *testing.T) {
		edges := contractEdges()
		edges["internal/sim/extra"] = []string{"internal/core"}
		violations := simDependencyViolations(edges)
		if len(violations) != 1 || !strings.Contains(violations[0], "未登记依赖白名单") {
			t.Fatalf("未登记子包未被拒绝: %v", violations)
		}
	})
}
