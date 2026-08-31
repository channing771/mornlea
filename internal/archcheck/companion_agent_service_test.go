package archcheck_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompanionAgentRepositoryBoundary(t *testing.T) {
	root := moduleRoot(t)
	required := []string{
		filepath.Join("contracts", "companion-agent", "http-v1", "manifest.json"),
		filepath.Join("contracts", "companion-agent", "http-v1", "schema.json"),
		filepath.Join("contracts", "companion-agent", "mcp-v1", "manifest.json"),
		filepath.Join("contracts", "companion-agent", "mcp-v1", "schema.json"),
		filepath.Join("services", "companion-agent", "pyproject.toml"),
		filepath.Join("services", "companion-agent", "uv.lock"),
		filepath.Join("services", "companion-agent", "src", "mornlea_companion_agent", "app.py"),
		filepath.Join("services", "companion-agent", "src", "mornlea_companion_agent", "adapters", "mcp.py"),
		filepath.Join("services", "companion-agent", "src", "mornlea_companion_agent", "harness", "planner.py"),
		filepath.Join("services", "companion-agent", "src", "mornlea_companion_agent", "domain", "http_v1.py"),
		filepath.Join("services", "companion-agent", "src", "mornlea_companion_agent", "storage", "sqlite_memory.py"),
		filepath.Join("services", "companion-agent", "tests", "integration", "process.py"),
	}
	for _, relative := range required {
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil || info.IsDir() {
			t.Errorf("伙伴 Agent 发布边界缺少文件 %s: %v", relative, err)
		}
	}

	pyproject := readBaselineDoc(t, root, filepath.Join("services", "companion-agent", "pyproject.toml"))
	for _, marker := range []string{
		`"../../contracts/companion-agent/mcp-v1/manifest.json"`,
		`"../../contracts/companion-agent/mcp-v1/schema.json"`,
	} {
		if !strings.Contains(pyproject, marker) {
			t.Errorf("Python wheel 未携带共享 MCP contract %s", marker)
		}
	}

	for _, relative := range []string{
		filepath.Join("services", "companion-agent", "src", "mornlea_companion_agent"),
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
	workflow := readBaselineDoc(t, moduleRoot(t), filepath.Join(".github", "workflows", "ci.yml"))
	start := strings.Index(workflow, "\n  integration:\n")
	end := strings.Index(workflow, "\n  linux-server:\n")
	if start < 0 || end <= start {
		t.Fatal("CI 缺少既有 integration job 边界")
	}
	integration := workflow[start:end]
	for _, marker := range []string{
		"actions/setup-python@v5",
		"python-version: '3.12'",
		"astral-sh/setup-uv@v6",
		"version: '0.12.5'",
		"services/companion-agent/pyproject.toml",
		"services/companion-agent/uv.lock",
		"make companion-agent-check",
		"make companion-agent-integration",
	} {
		if !strings.Contains(integration, marker) {
			t.Errorf("CI integration job 缺少伙伴 Agent 门禁标记 %q", marker)
		}
	}
}

func TestCompanionGoProductionDoesNotEmbedPython(t *testing.T) {
	root := moduleRoot(t)
	for _, relative := range []string{filepath.Join("internal", "companion"), filepath.Join("internal", "server")} {
		err := filepath.WalkDir(filepath.Join(root, relative), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imported := range parsed.Imports {
				name := strings.Trim(imported.Path.Value, `"`)
				if name == "C" || name == "os/exec" || name == "runtime/cgo" {
					t.Errorf("伙伴 Go 生产路径 %s 不得通过 %s 调用 Python", path, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描伙伴 Go 生产路径失败: %v", err)
		}
	}
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
