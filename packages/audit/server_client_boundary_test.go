package archcheck_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// server 与 client 两个单元在模块层互有合法 require 边（Go 的 require 无法按
// 测试或命令子树限定），语义边界因此下沉到源码层由本文件强制：
//
//   - packages/server 的生产文件（非 `_test.go`）MUST NOT import
//     `packages/client` 的任何包。唯一豁免是测试文件——server 的 Memory/TCP
//     集成测试以客户端镜像驱动会话（S5 落地、控制会话裁决正式化的测试专用边）。
//   - packages/client 的域库包（client/render/mesh/lod/audio/assets，即 cmd/
//     子树之外的全部包）的任何文件（含测试）MUST NOT import `packages/server`
//     的任何包。例外是 `packages/client/cmd/mornlea` 应用入口：普通本地与
//     benchmark 模式在进程内装配本地权威 Host（Memory transport 登录边界
//     不变），与 tools 消费 server 的方向同构；库侧一旦 import server 就会把
//     权威模拟反向拖进呈现域，客户端失去独立构建的意义。client 侧不设测试
//     豁免——镜像驱动方向固定为「server 测试消费 client 镜像」，反向的测试
//     import 只会让依赖边在库测试里悄悄复辟。
//
// 与 `TestServerPersistenceDoesNotDependOnServer` 的 persistence_contract 豁免
// 同风格：模块层放行、源码层钉死，且检查器自身有合成 drift 测试兜底——守卫
// 逻辑被改坏时门禁必须报错而不是静默变松。
const (
	serverClientUnitImportPrefix   = "github.com/channing771/mornlea/packages/"
	clientUnitCommandSubtreePrefix = "packages/client/cmd/mornlea" // 消费 server 的应用入口子树
)

// serverClientImportViolations 对照上述源码级边界检查「仓库相对路径 → 源文件
// 内容」映射，返回全部违规描述；空切片表示符合契约。输入与真实目录解耦，
// 使「注入违规 import」的失败路径可以在纯内存中核对，不必改动源码树。
// 只用 parser.ImportsOnly 解析 import 声明，不关心文件其余内容；解析失败的
// 文件直接计为违规（守卫面对无法解析的输入必须 fail-closed）。
func serverClientImportViolations(sources map[string]string) []string {
	var violations []string
	for _, path := range slices.Sorted(maps.Keys(sources)) {
		source := sources[path]
		isTestFile := strings.HasSuffix(path, "_test.go")
		parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s 解析失败：%v", path, err))
			continue
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			local, ok := strings.CutPrefix(importPath, serverClientUnitImportPrefix)
			if !ok {
				continue
			}
			switch {
			case strings.HasPrefix(path, "packages/server/"):
				// server 生产文件禁 import client；测试文件（客户端镜像驱动的
				// 集成测试）放行。
				if !isTestFile && strings.HasPrefix(local, "client/") {
					violations = append(violations, fmt.Sprintf(
						"server 生产文件 %s 不得导入 packages/client 的包（%s）；客户端镜像只能出现在 _test.go 驱动的集成测试中", path, importPath))
				}
			case strings.HasPrefix(path, "packages/client/"):
				// client 域库的任何文件（含测试）禁 import server；cmd/mornlea
				// 应用入口（装配本地权威 Host）放行。
				inCommandSubtree := strings.HasPrefix(path, clientUnitCommandSubtreePrefix+"/")
				if !inCommandSubtree && strings.HasPrefix(local, "server/") {
					violations = append(violations, fmt.Sprintf(
						"client 域库文件 %s 不得导入 packages/server 的包（%s）；服务端装配只属于 packages/client/cmd/mornlea 应用入口", path, importPath))
				}
			}
		}
	}
	return violations
}

// TestServerClientUnitProductionBoundaries 把真实源码树中 server 与 client 两
// 个单元的全部 Go 源文件喂给 `serverClientImportViolations`，钉住两条生产面
// 禁令。cmd 子树内部的方向契约另由 `clientCommandAllowedEdges` 负责，本测试
// 不重复其职责，只管跨单元边界。
func TestServerClientUnitProductionBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	sources := make(map[string]string)
	for _, unit := range []string{"packages/server", "packages/client"} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(unit)), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			sources[filepath.ToSlash(relative)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("扫描 %s: %v", unit, err)
		}
	}
	if violations := serverClientImportViolations(sources); len(violations) > 0 {
		t.Errorf("server/client 单元生产面边界违反契约，%d 条违规：\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestServerClientBoundaryGuardDetectsDrift 用合成源文件核对守卫对每类漂移都
// 真的报错、对每类豁免都真的放行。真实源码树处于契约内时，方向断言的负向
// 路径没有天然的失败信号；必须另有合成断言钉住检查器本身，否则检查逻辑被
// 改坏后门禁静默变松。
func TestServerClientBoundaryGuardDetectsDrift(t *testing.T) {
	// contractSources 是恰好满足契约的最小合成文件集：server 测试文件 import
	// client（镜像驱动）、client 应用入口生产文件 import server（装配本地
	// Host），两侧都合法；库与 server 生产文件不带跨单元 import。
	contractSources := func() map[string]string {
		return map[string]string{
			"packages/server/server/server.go":                   "package server\n",
			"packages/server/server/mirror_integration_test.go":  "package server_test\nimport _ \"github.com/channing771/mornlea/packages/client/client\"\n",
			"packages/client/client/mesher.go":                   "package client\n",
			"packages/client/render/render.go":                   "package render\n",
			"packages/client/cmd/mornlea/app/app_startup.go":     "package app\nimport _ \"github.com/channing771/mornlea/packages/server/server\"\n",
			"packages/client/cmd/mornlea/benchmark/benchmark.go": "package benchmark\nimport _ \"github.com/channing771/mornlea/packages/server/storage\"\n",
		}
	}
	if violations := serverClientImportViolations(contractSources()); len(violations) != 0 {
		t.Fatalf("契约内的合成源文件不应报违规: %v", violations)
	}

	t.Run("server生产文件导入client被拒", func(t *testing.T) {
		sources := contractSources()
		sources["packages/server/sim/runtime/engine.go"] = "package runtime\nimport _ \"github.com/channing771/mornlea/packages/client/client\"\n"
		violations := serverClientImportViolations(sources)
		if len(violations) != 1 || !strings.Contains(violations[0], "server 生产文件 packages/server/sim/runtime/engine.go 不得导入") {
			t.Fatalf("server 生产文件导入 client 未被拒绝: %v", violations)
		}
	})
	t.Run("server测试文件导入client放行", func(t *testing.T) {
		sources := contractSources()
		sources["packages/server/server/daylight_integration_test.go"] = "package server_test\nimport _ \"github.com/channing771/mornlea/packages/client/client\"\n"
		if violations := serverClientImportViolations(sources); len(violations) != 0 {
			t.Fatalf("server 测试文件的镜像驱动 import 不应报违规: %v", violations)
		}
	})
	t.Run("client域库生产文件导入server被拒", func(t *testing.T) {
		sources := contractSources()
		sources["packages/client/render/section_scheduler.go"] = "package render\nimport _ \"github.com/channing771/mornlea/packages/server/sim/runtime\"\n"
		violations := serverClientImportViolations(sources)
		if len(violations) != 1 || !strings.Contains(violations[0], "client 域库文件 packages/client/render/section_scheduler.go 不得导入") {
			t.Fatalf("client 域库生产文件导入 server 未被拒绝: %v", violations)
		}
	})
	t.Run("client域库测试文件导入server同样被拒", func(t *testing.T) {
		sources := contractSources()
		sources["packages/client/client/mesher_parity_test.go"] = "package client_test\nimport _ \"github.com/channing771/mornlea/packages/server/sim/runtime\"\n"
		violations := serverClientImportViolations(sources)
		if len(violations) != 1 || !strings.Contains(violations[0], "client 域库文件 packages/client/client/mesher_parity_test.go 不得导入") {
			t.Fatalf("client 域库测试文件导入 server 未被拒绝（client 侧无测试豁免）: %v", violations)
		}
	})
	t.Run("client命令子树导入server放行", func(t *testing.T) {
		sources := contractSources()
		sources["packages/client/cmd/mornlea/app/app_dependencies.go"] = "package app\nimport _ \"github.com/channing771/mornlea/packages/server/server\"\n"
		if violations := serverClientImportViolations(sources); len(violations) != 0 {
			t.Fatalf("客户端命令装配本地 Host 的 import 不应报违规: %v", violations)
		}
	})
}
