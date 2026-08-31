package archcheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const companionAgentWorkflowFixture = `name: fixture
jobs:
  test:
    needs: [native-macos, integration]
    runs-on: ubuntu-latest
    steps:
      - name: summary
        run: |
          test "${{ needs.integration.result }}" = success
  native-macos:
    runs-on: macos-latest
    steps: []
  unrelated-job:
    runs-on: ubuntu-latest
    steps: []
  integration:
    needs: native-macos
    runs-on: macos-latest
    steps:
      - uses: actions/setup-python@v5
        with:
          python-version: '3.12'
      - uses: astral-sh/setup-uv@v6
        with:
          version: '0.12.5'
          enable-cache: true
          cache-dependency-glob: |
            services/companion-agent/pyproject.toml
            services/companion-agent/uv.lock
      - uses: actions/download-artifact@v4
        with:
          name: native-macos-${{ github.sha }}
          path: engine/target/release
      - name: verify artifact
        run: |
          test "$(cat engine/target/release/native-source-sha.txt)" = "$GITHUB_SHA"
          test "$(wc -l < engine/target/release/native-artifact-manifest.txt | tr -d ' ')" = 3
          test "$digest" = "$(shasum -a 256 "$path" | awk '{print $1}')"
      - name: check
        run: make companion-agent-check
      - name: integration
        run: make companion-agent-integration
`

func TestCompanionAgentCIGateMutations(t *testing.T) {
	if violations := companionAgentWorkflowViolations([]byte(companionAgentWorkflowFixture)); len(violations) != 0 {
		t.Fatalf("合法 job 重排的 fixture 被误拒绝: %v", violations)
	}

	mutations := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{"integration dependency", "needs: native-macos\n    runs-on: macos-latest", "needs: quality\n    runs-on: macos-latest", "needs"},
		{"macos runner", "runs-on: macos-latest\n    steps:\n      - uses: actions/setup-python", "runs-on: ubuntu-latest\n    steps:\n      - uses: actions/setup-python", "macos-latest"},
		{"python version", "python-version: '3.12'", "python-version: '3.13'", "Python 3.12"},
		{"python version type", "python-version: '3.12'", "python-version: 3.12", "string"},
		{"uv version", "version: '0.12.5'", "version: 'latest'", "uv 0.12.5"},
		{"uv cache type", "enable-cache: true", "enable-cache: 'true'", "bool"},
		{"pyproject cache dependency", "            services/companion-agent/pyproject.toml\n", "", "pyproject.toml"},
		{"lock cache dependency", "            services/companion-agent/uv.lock\n", "", "uv.lock"},
		{"same sha artifact", "name: native-macos-${{ github.sha }}", "name: native-macos-latest", "same-SHA"},
		{"source sha verification", "          test \"$(cat engine/target/release/native-source-sha.txt)\" = \"$GITHUB_SHA\"\n", "", "source SHA"},
		{"artifact manifest verification", "          test \"$digest\" = \"$(shasum -a 256 \"$path\" | awk '{print $1}')\"\n", "", "manifest"},
		{"python check", "        run: make companion-agent-check", "        run: make companion-agent-check-disabled", "companion-agent-check"},
		{"process integration", "        run: make companion-agent-integration", "        run: make companion-agent-integration-disabled", "companion-agent-integration"},
		{"summary dependency", "needs: [native-macos, integration]", "needs: [native-macos]", "test job needs integration"},
		{"summary assertion", "test \"${{ needs.integration.result }}\" = success", "test \"${{ needs.quality.result }}\" = success", "integration result"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := replaceCompanionFixtureOnce(t, companionAgentWorkflowFixture, mutation.old, mutation.new)
			assertCompanionViolationContains(t, companionAgentWorkflowViolations([]byte(mutated)), mutation.want)
		})
	}

	verify := `      - name: verify artifact
        run: |
          test "$(cat engine/target/release/native-source-sha.txt)" = "$GITHUB_SHA"
          test "$(wc -l < engine/target/release/native-artifact-manifest.txt | tr -d ' ')" = 3
          test "$digest" = "$(shasum -a 256 "$path" | awk '{print $1}')"
`
	check := `      - name: check
        run: make companion-agent-check
`
	mutatedOrder := replaceCompanionFixtureOnce(t, companionAgentWorkflowFixture, verify+check, check+verify)
	assertCompanionViolationContains(t, companionAgentWorkflowViolations([]byte(mutatedOrder)), "before")
}

func TestCompanionAgentQuickstartRegexSelectsRealTests(t *testing.T) {
	quickstart := readBaselineDoc(t, moduleRoot(t), filepath.Join("docs", "notes", "test-quickstart.md"))
	pattern := companionQuickstartGoTestPattern(t, quickstart)
	testNames := companionServerTopLevelTestNames(t, moduleRoot(t))
	want := []string{
		"TestCompanionAgentHTTPProcessIntegration",
		"TestMCPAgentCrossLanguageCancellationIntegration",
		"TestMCPAgentCrossLanguageIntegration",
	}
	if got := companionMatchingTestNames(t, pattern, testNames); !slices.Equal(got, want) {
		t.Fatalf("quickstart regexp %q 选中 %v，想要三个真实测试 %v", pattern, got, want)
	}

	oldEscapedPipe := strings.Replace(pattern, "|", `\|`, 1)
	if got := companionMatchingTestNames(t, oldEscapedPipe, testNames); len(got) != 0 {
		t.Fatalf("旧 escaped-pipe mutation 意外选中测试: %v", got)
	}
}

func TestCompanionGoProductionBoundaryMutations(t *testing.T) {
	roots := []string{
		"cmd/mornlea",
		"cmd/mornlea/app",
		"cmd/mornlea-server",
		"internal/server",
		"internal/companion",
	}
	contract := map[string][]string{
		"cmd/mornlea":             {"cmd/mornlea/benchmark", "internal/audio", "internal/client"},
		"cmd/mornlea/app":         {},
		"cmd/mornlea-server":      {},
		"internal/server":         {},
		"internal/companion":      {},
		"cmd/mornlea/benchmark":   {"os/exec"},
		"cmd/mornlea-agent-board": {"os/exec"},
		"internal/audio":          {"C"},
		"internal/client":         {"C"},
		"internal/nativeabi":      {"C"},
	}
	if violations := companionGoProductionBoundaryViolations(contract, roots); len(violations) != 0 {
		t.Fatalf("明确合法的 native bridge、benchmark 与闭包外 agent-board 被误拒绝: %v", violations)
	}

	for _, forbidden := range []string{"os/exec", "C", "runtime/cgo"} {
		t.Run(forbidden, func(t *testing.T) {
			mutated := cloneCompanionImportGraph(contract)
			mutated["internal/companion"] = append(mutated["internal/companion"], "internal/companion/pythonhelper")
			mutated["internal/companion/pythonhelper"] = []string{forbidden}
			violations := companionGoProductionBoundaryViolations(mutated, roots)
			assertCompanionViolationContains(t, violations, "pythonhelper")
			assertCompanionViolationContains(t, violations, forbidden)
		})
	}
}

func replaceCompanionFixtureOnce(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if strings.Count(source, old) != 1 {
		t.Fatalf("mutation sentinel %q 出现 %d 次，想要恰好一次", old, strings.Count(source, old))
	}
	return strings.Replace(source, old, replacement, 1)
}

func assertCompanionViolationContains(t *testing.T, violations []string, want string) {
	t.Helper()
	if !slices.ContainsFunc(violations, func(violation string) bool {
		return strings.Contains(violation, want)
	}) {
		t.Fatalf("violations=%v，想要包含 %q", violations, want)
	}
}

func cloneCompanionImportGraph(source map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(source))
	for name, imports := range source {
		cloned[name] = slices.Clone(imports)
	}
	return cloned
}

func companionQuickstartGoTestPattern(t *testing.T, markdown string) string {
	t.Helper()
	const prefix = "`go test ./internal/server -run '"
	for _, line := range strings.Split(markdown, "\n") {
		if !strings.Contains(line, "Go/Python Agent 合同") {
			continue
		}
		start := strings.Index(line, prefix)
		if start < 0 {
			t.Fatal("Go/Python Agent 合同 quickstart 缺少固定 go test 命令")
		}
		remainder := line[start+len(prefix):]
		end := strings.Index(remainder, "'")
		if end < 0 {
			t.Fatal("Go/Python Agent 合同 quickstart 的 -run regexp 未闭合")
		}
		return remainder[:end]
	}
	t.Fatal("test quickstart 缺少 Go/Python Agent 合同行")
	return ""
}

func companionServerTopLevelTestNames(t *testing.T, root string) []string {
	t.Helper()
	serverDir := filepath.Join(root, "internal", "server")
	entries, err := os.ReadDir(serverDir)
	if err != nil {
		t.Fatalf("读取 server 测试目录: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(serverDir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("解析 server 测试 %s: %v", entry.Name(), err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
				names = append(names, function.Name.Name)
			}
		}
	}
	slices.Sort(names)
	return names
}

func companionMatchingTestNames(t *testing.T, pattern string, testNames []string) []string {
	t.Helper()
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("编译 quickstart regexp %q: %v", pattern, err)
	}
	var matching []string
	for _, name := range testNames {
		if compiled.MatchString(name) {
			matching = append(matching, name)
		}
	}
	return matching
}
