package archcheck_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCompanionAgentRepositoryBoundary(t *testing.T) {
	root := moduleRoot(t)
	companion := filepath.Join("packages", "agent", "companion")
	required := []string{
		filepath.Join("packages", "contracts", "companion-agent", "http-v1", "manifest.json"),
		filepath.Join("packages", "contracts", "companion-agent", "http-v1", "schema.json"),
		filepath.Join("packages", "contracts", "companion-agent", "mcp-v1", "manifest.json"),
		filepath.Join("packages", "contracts", "companion-agent", "mcp-v1", "schema.json"),
		filepath.Join(companion, "pyproject.toml"),
		filepath.Join(companion, "uv.lock"),
		filepath.Join(companion, "src", "mornlea_companion_agent", "app.py"),
		filepath.Join(companion, "src", "mornlea_companion_agent", "adapters", "mcp.py"),
		filepath.Join(companion, "src", "mornlea_companion_agent", "harness", "planner.py"),
		filepath.Join(companion, "src", "mornlea_companion_agent", "domain", "http_v1.py"),
		filepath.Join(companion, "src", "mornlea_companion_agent", "storage", "sqlite_memory.py"),
		filepath.Join(companion, "tests", "integration", "process.py"),
	}
	for _, relative := range required {
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil || info.IsDir() {
			t.Errorf("伙伴 Agent 发布边界缺少文件 %s: %v", relative, err)
		}
	}

	pyproject := readBaselineDoc(t, root, filepath.Join(companion, "pyproject.toml"))
	for _, marker := range []string{
		`"../../../packages/contracts/companion-agent/mcp-v1/manifest.json"`,
		`"../../../packages/contracts/companion-agent/mcp-v1/schema.json"`,
	} {
		if !strings.Contains(pyproject, marker) {
			t.Errorf("Python wheel 未携带共享 MCP contract %s", marker)
		}
	}

	for _, relative := range []string{
		filepath.Join(companion, "src", "mornlea_companion_agent"),
	} {
		err := filepath.WalkDir(filepath.Join(root, relative), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
				return nil
			}
			name := strings.ToLower(entry.Name())
			if strings.Contains(name, "fake") || strings.Contains(name, "fixture") {
				t.Errorf("测试 fake/fixture 不得进入 Python 发布路径: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描 Python 发布路径失败: %v", err)
		}
	}
}

func TestCompanionAgentMakeTargets(t *testing.T) {
	makefile := readBaselineDoc(t, moduleRoot(t), "Makefile")
	for _, target := range []string{"companion-agent-check", "companion-agent-integration"} {
		if !makeTargetIsPhony(makefile, target) {
			t.Errorf("Makefile .PHONY 缺少 %s", target)
		}
		if !strings.Contains(makefile, "make "+target) {
			t.Errorf("make help 缺少 %s", target)
		}
	}

	check := makeTargetRecipe(t, makefile, "companion-agent-check")
	for _, command := range []string{
		"uv sync --locked",
		"uv run ruff format --check .",
		"uv run ruff check .",
		"uv run mypy src",
		"uv run pytest -q",
	} {
		if !strings.Contains(check, command) {
			t.Errorf("companion-agent-check 缺少命令 %q", command)
		}
	}

	integration := makeTargetRecipe(t, makefile, "companion-agent-integration")
	for _, command := range []string{"uv sync --locked", "tests/integration", "-race", "CrossLanguage"} {
		if !strings.Contains(integration, command) {
			t.Errorf("companion-agent-integration 缺少真进程门禁标记 %q", command)
		}
	}
}

func TestCompanionAgentCIGates(t *testing.T) {
	workflow := []byte(readBaselineDoc(t, moduleRoot(t), filepath.Join(".github", "workflows", "ci.yml")))
	if violations := companionAgentWorkflowViolations(workflow); len(violations) > 0 {
		t.Errorf("伙伴 Agent CI 合同有 %d 条违规：\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestVerifyNativeArtifactScript 钉住 scripts/ci/verify-native-artifact.sh 的
// 承重语句。校验逻辑从 ci.yml 的内联块收敛进脚本后，workflow 侧只钉「恰好
// 调用一次且位于 make 门禁之前」的编排位置；脚本本体若被删改（丢掉行数、
// SHA 或 sha256 任一环）在这里暴露，否则三个 job 共享的信任基准静默变松。
func TestVerifyNativeArtifactScript(t *testing.T) {
	script := readBaselineDoc(t, moduleRoot(t), filepath.Join("scripts", "ci", "verify-native-artifact.sh"))
	for _, required := range []string{
		"set -euo pipefail",
		"ENGINE_DYLIB=packages/engine/target/release/libmornlea_engine.dylib",
		"CLIENT_DYLIB=packages/engine/target/release/libmornlea_client.dylib",
		`test "$(cat packages/engine/target/release/native-source-sha.txt)" = "$GITHUB_SHA"`,
		`test "$(wc -l < "$MANIFEST" | tr -d ' ')" = 3`,
		`IFS=' ' read -r kind sha extra`,
		`test "$kind" = sha`,
		`test "$sha" = "$GITHUB_SHA"`,
		`test -z "$extra"`,
		`expected_path=$1`,
		`IFS=' ' read -r path size digest extra`,
		`test "$path" = "$expected_path"`,
		`test -z "$extra"`,
		`case "$size" in ''|*[!0-9]*) exit 1 ;; esac`,
		`test "$size" = "$(stat -f '%z' "$path")"`,
		`test "$digest" = "$(shasum -a 256 "$path" | awk '{print $1}')"`,
		`validate_artifact "$ENGINE_DYLIB"`,
		`validate_artifact "$CLIENT_DYLIB"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("verify-native-artifact.sh 缺少承重语句 %s", required)
		}
	}
}

func TestCompanionGoProductionDoesNotEmbedPython(t *testing.T) {
	graph, err := loadCompanionProductionImportGraph(moduleRoot(t))
	if err != nil {
		t.Fatalf("加载伙伴生产装配依赖闭包: %v", err)
	}
	if violations := companionGoProductionBoundaryViolations(graph, companionGoProductionRoots); len(violations) > 0 {
		t.Errorf("伙伴 Go 生产装配边界有 %d 条违规：\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

type companionWorkflow struct {
	Jobs map[string]companionWorkflowJob `yaml:"jobs"`
}

type companionWorkflowJob struct {
	If     companionYAMLString     `yaml:"if"`
	Needs  companionWorkflowNeeds  `yaml:"needs"`
	RunsOn companionYAMLString     `yaml:"runs-on"`
	Steps  []companionWorkflowStep `yaml:"steps"`
}

type companionWorkflowStep struct {
	Name string                       `yaml:"name"`
	Uses string                       `yaml:"uses"`
	Run  string                       `yaml:"run"`
	With map[string]companionYAMLNode `yaml:"with"`
}

type companionYAMLNode struct {
	Node *yaml.Node
}

func (value *companionYAMLNode) UnmarshalYAML(node *yaml.Node) error {
	value.Node = node
	return nil
}

type companionYAMLString struct {
	Value string
}

func (value *companionYAMLString) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("需要 YAML string，实际 kind=%d tag=%s", node.Kind, node.Tag)
	}
	value.Value = node.Value
	return nil
}

type companionWorkflowNeeds []string

func (needs *companionWorkflowNeeds) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return fmt.Errorf("needs scalar 必须是 string，实际 tag=%s", node.Tag)
		}
		*needs = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		values := make([]string, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode || child.Tag != "!!str" {
				return fmt.Errorf("needs sequence 元素必须是 string，实际 kind=%d tag=%s", child.Kind, child.Tag)
			}
			values = append(values, child.Value)
		}
		*needs = values
		return nil
	default:
		return fmt.Errorf("needs 必须是 string 或 sequence，实际 kind=%d", node.Kind)
	}
}

func companionAgentWorkflowViolations(source []byte) []string {
	var workflow companionWorkflow
	if err := yaml.Unmarshal(source, &workflow); err != nil {
		return []string{fmt.Sprintf("CI YAML 结构或 scalar 类型非法: %v", err)}
	}
	var violations []string
	integration, ok := workflow.Jobs["integration"]
	if !ok {
		return []string{"CI 缺少 integration job"}
	}
	if !slices.Equal([]string(integration.Needs), []string{"native-macos"}) {
		violations = append(violations, fmt.Sprintf("integration needs=%v，必须精确依赖 native-macos", integration.Needs))
	}
	if integration.RunsOn.Value != "macos-latest" {
		violations = append(violations, fmt.Sprintf("integration runs-on=%q，必须是 macos-latest", integration.RunsOn.Value))
	}

	pythonIndexes := workflowUsesStepIndexes(integration.Steps, "actions/setup-python@v5")
	if len(pythonIndexes) != 1 {
		violations = append(violations, fmt.Sprintf("integration setup-python 步骤数=%d，想要 1", len(pythonIndexes)))
	} else if value, err := workflowStringWith(integration.Steps[pythonIndexes[0]], "python-version"); err != nil {
		violations = append(violations, "Python 3.12 必须使用 YAML string: "+err.Error())
	} else if value != "3.12" {
		violations = append(violations, fmt.Sprintf("integration Python 3.12 被改为 %q", value))
	}

	uvIndexes := workflowUsesStepIndexes(integration.Steps, "astral-sh/setup-uv@v6")
	if len(uvIndexes) != 1 {
		violations = append(violations, fmt.Sprintf("integration setup-uv 步骤数=%d，想要 1", len(uvIndexes)))
	} else {
		step := integration.Steps[uvIndexes[0]]
		if value, err := workflowStringWith(step, "version"); err != nil {
			violations = append(violations, "uv 0.12.5 必须使用 YAML string: "+err.Error())
		} else if value != "0.12.5" {
			violations = append(violations, fmt.Sprintf("integration uv 0.12.5 被改为 %q", value))
		}
		if value, err := workflowBoolWith(step, "enable-cache"); err != nil {
			violations = append(violations, "uv enable-cache 必须使用 YAML bool: "+err.Error())
		} else if !value {
			violations = append(violations, "uv enable-cache 必须为 true")
		}
		if value, err := workflowStringWith(step, "cache-dependency-glob"); err != nil {
			violations = append(violations, "uv cache-dependency-glob 必须使用 YAML string: "+err.Error())
		} else {
			cachePaths := nonEmptyTrimmedLines(value)
			for _, required := range []string{
				"packages/agent/companion/pyproject.toml",
				"packages/agent/companion/uv.lock",
			} {
				if !slices.Contains(cachePaths, required) {
					violations = append(violations, "uv cache 缺少 "+required)
				}
			}
		}
	}

	downloadIndexes := workflowUsesStepIndexes(integration.Steps, "actions/download-artifact@v4")
	downloadIndex := uniqueWorkflowStepIndex(&violations, "same-SHA native artifact download", downloadIndexes)
	if downloadIndex >= 0 {
		step := integration.Steps[downloadIndex]
		if value, err := workflowStringWith(step, "name"); err != nil || value != "native-macos-${{ github.sha }}" {
			violations = append(violations, "native artifact download 必须使用 same-SHA 名称 native-macos-${{ github.sha }}")
		}
		if value, err := workflowStringWith(step, "path"); err != nil || value != "packages/engine/target/release" {
			violations = append(violations, "native artifact download path 必须是 packages/engine/target/release")
		}
	}

	verifyIndex := uniqueWorkflowStepIndex(&violations, "native artifact 校验脚本调用",
		workflowExactCommandStepIndexes(integration.Steps, "scripts/ci/verify-native-artifact.sh"))
	checkIndex := uniqueWorkflowStepIndex(&violations, "make companion-agent-check", workflowExactCommandStepIndexes(integration.Steps, "make companion-agent-check"))
	processIndex := uniqueWorkflowStepIndex(&violations, "make companion-agent-integration", workflowExactCommandStepIndexes(integration.Steps, "make companion-agent-integration"))
	if downloadIndex >= 0 && verifyIndex >= 0 && checkIndex >= 0 && processIndex >= 0 &&
		!(downloadIndex < verifyIndex && verifyIndex < checkIndex && verifyIndex < processIndex) {
		violations = append(violations, "same-SHA artifact download 与校验脚本必须在两条 Agent make 门禁 before 执行")
	}
	for _, setup := range append(slices.Clone(pythonIndexes), uvIndexes...) {
		if (checkIndex >= 0 && setup >= checkIndex) || (processIndex >= 0 && setup >= processIndex) {
			violations = append(violations, "Python/uv setup 必须先于两条 Agent make 门禁")
		}
	}

	summary, ok := workflow.Jobs["test"]
	if !ok {
		violations = append(violations, "CI 缺少最终 test job")
	} else {
		if summary.If.Value != "${{ always() }}" {
			violations = append(violations, "最终 test job if 必须精确为 ${{ always() }}")
		}
		if !slices.Contains([]string(summary.Needs), "integration") {
			violations = append(violations, "最终 test job needs integration")
		}
		if len(workflowStatementStepIndexes(summary.Steps, `test "${{ needs.integration.result }}" = success`)) != 1 {
			violations = append(violations, "最终 test job 缺少精确的 integration result success 断言")
		}
	}
	slices.Sort(violations)
	return violations
}

func workflowUsesStepIndexes(steps []companionWorkflowStep, action string) []int {
	var indexes []int
	for index, step := range steps {
		if step.Uses == action {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func workflowExactCommandStepIndexes(steps []companionWorkflowStep, command string) []int {
	var indexes []int
	for index, step := range steps {
		statements := workflowShellStatements(step.Run)
		if len(statements) == 1 && statements[0] == command {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func workflowStatementStepIndexes(steps []companionWorkflowStep, statement string) []int {
	var indexes []int
	for index, step := range steps {
		if slices.Contains(workflowShellStatements(step.Run), statement) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func workflowShellStatements(script string) []string {
	var statements []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			statements = append(statements, line)
		}
	}
	return statements
}

func workflowStringWith(step companionWorkflowStep, name string) (string, error) {
	value, ok := step.With[name]
	if !ok || value.Node == nil {
		return "", fmt.Errorf("缺少 with.%s", name)
	}
	if value.Node.Kind != yaml.ScalarNode || value.Node.Tag != "!!str" {
		return "", fmt.Errorf("with.%s 需要 YAML string，实际 kind=%d tag=%s", name, value.Node.Kind, value.Node.Tag)
	}
	return value.Node.Value, nil
}

func workflowBoolWith(step companionWorkflowStep, name string) (bool, error) {
	value, ok := step.With[name]
	if !ok || value.Node == nil {
		return false, fmt.Errorf("缺少 with.%s", name)
	}
	if value.Node.Kind != yaml.ScalarNode || value.Node.Tag != "!!bool" {
		return false, fmt.Errorf("with.%s 需要 YAML bool，实际 kind=%d tag=%s", name, value.Node.Kind, value.Node.Tag)
	}
	var decoded bool
	if err := value.Node.Decode(&decoded); err != nil {
		return false, fmt.Errorf("解码 with.%s: %w", name, err)
	}
	return decoded, nil
}

func uniqueWorkflowStepIndex(violations *[]string, name string, indexes []int) int {
	if len(indexes) != 1 {
		*violations = append(*violations, fmt.Sprintf("integration %s 步骤数=%d，想要 1", name, len(indexes)))
		return -1
	}
	return indexes[0]
}

func nonEmptyTrimmedLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

var companionGoProductionRoots = []string{
	"packages/client/cmd/mornlea",
	"packages/client/cmd/mornlea/app",
	"packages/server/cmd/mornlea-server",
	"packages/server/server",
	"packages/shared/companion",
}

var companionGoForbiddenImportAllowlist = map[string]map[string]string{
	"packages/client/cmd/mornlea/benchmark": {
		"os/exec": "benchmark 模式以独立服务端子进程测量真实传输，不用于伙伴 Agent 装配",
	},
	"packages/client/audio": {
		"C": "Darwin 音频桥，不用于伙伴 Agent 装配",
	},
	"packages/client/client": {
		"C": "Rust client ABI bridge，不用于伙伴 Agent 装配",
	},
	"packages/shared/nativeabi": {
		"C": "Rust engine ABI bridge，不用于伙伴 Agent 装配",
	},
}

func loadCompanionProductionImportGraph(root string) (map[string][]string, error) {
	graph := make(map[string][]string)
	// companion 已迁入 packages/shared 模块、服务端域已迁入 packages/server
	// 模块、客户端域已迁入 packages/client 模块；图必须覆盖全部模块所在子树，
	// 否则闭包会在跨模块边上断链。
	for _, top := range []string{"cmd", "internal", "packages/contracts", "packages/shared", "packages/server", "packages/client"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, entry fs.DirEntry, walkErr error) error {
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
			found := false
			for _, file := range entries {
				name := file.Name()
				if file.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
					continue
				}
				found = true
				parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(path, name), nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imported := range parsed.Imports {
					importPath := strings.Trim(imported.Path.Value, `"`)
					if strings.HasPrefix(importPath, "github.com/channing771/mornlea/") {
						importPath = strings.TrimPrefix(importPath, "github.com/channing771/mornlea/")
					}
					imports[importPath] = true
				}
			}
			if !found {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			importNames := mapsKeys(imports)
			slices.Sort(importNames)
			graph[filepath.ToSlash(relative)] = importNames
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("扫描 %s: %w", top, err)
		}
	}
	return graph, nil
}

func companionGoProductionBoundaryViolations(graph map[string][]string, roots []string) []string {
	const modulePrefix = "github.com/channing771/mornlea/"
	forbidden := map[string]bool{"C": true, "os/exec": true, "runtime/cgo": true}
	visited := make(map[string]bool)
	queue := slices.Clone(roots)
	var violations []string
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if visited[pkg] {
			continue
		}
		visited[pkg] = true
		imports, ok := graph[pkg]
		if !ok {
			violations = append(violations, "伙伴生产依赖闭包缺少源码包 "+pkg)
			continue
		}
		for _, imported := range imports {
			if strings.HasPrefix(imported, modulePrefix) {
				imported = strings.TrimPrefix(imported, modulePrefix)
			}
			if forbidden[imported] {
				if _, allowed := companionGoForbiddenImportAllowlist[pkg][imported]; !allowed {
					violations = append(violations, fmt.Sprintf("伙伴生产依赖闭包包 %s 导入禁止的 %s", pkg, imported))
				}
				continue
			}
			if _, local := graph[imported]; local {
				queue = append(queue, imported)
			}
		}
	}
	slices.Sort(violations)
	return violations
}

func mapsKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func makeTargetIsPhony(makefile, target string) bool {
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(line, ".PHONY:") && strings.Contains(" "+strings.TrimPrefix(line, ".PHONY:")+" ", " "+target+" ") {
			return true
		}
	}
	return false
}

func makeTargetRecipe(t *testing.T, makefile, target string) string {
	t.Helper()
	lines := strings.Split(makefile, "\n")
	for index, line := range lines {
		if line != target+":" {
			continue
		}
		var recipe []string
		for _, candidate := range lines[index+1:] {
			if strings.HasPrefix(candidate, "\t") {
				recipe = append(recipe, candidate)
				continue
			}
			if candidate == "" || strings.HasPrefix(candidate, "#") {
				continue
			}
			break
		}
		return strings.Join(recipe, "\n")
	}
	t.Fatalf("Makefile 缺少 target %s", target)
	return ""
}
