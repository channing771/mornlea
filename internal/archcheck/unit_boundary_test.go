package archcheck_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// unitAllowedRequireEdges 列出 packages/<name> 单元的 go.mod 允许 require 的
// 兄弟单元集合，键与值都取 packages/ 下的目录名。合法方向严格向下单向：
// server → shared/contracts、client → shared、tools → shared/server/client/contracts
// （perfcheck 同时消费服务端与客户端侧包），audit/contracts/shared 是叶子——
// audit 是跨模块的枚举校验者，只以读文件的方式观察被审单元，住进任何依赖边
// 都会失去审计资格；contracts 只承载 embed 契约文件；shared 是最底层共享域。
//
// 为什么按 go.mod require 而不是 import 检查：require 边由工具链强制（未
// 登记的跨单元 import 在构建层直接拒收），边界在编译期先行生效且不依赖
// 源码扫描；「该不该 import」的包级语义方向仍由上方 allowed 表负责，两层
// 互补而非重复。
//
// 切割分阶段落地：本表是目标状态而非现状清单，登记单元暂不存在不是违规
// （阶段前期真实树断言空转）；反向的漂移必须报错——packages/ 下出现带
// go.mod 却未登记的单元，说明新单元绕过边界悄悄立项。
var unitAllowedRequireEdges = map[string][]string{
	"server":    {"shared", "contracts"},
	"client":    {"shared"},
	"tools":     {"shared", "server", "client", "contracts"},
	"audit":     {},
	"contracts": {},
	"shared":    {},
}

// unitRequireViolations 对照单元允许 require 边表检查给定的「单元名 → go.mod
// require 的兄弟单元集合」，返回全部违规描述；空切片表示符合契约。输入与
// 真实目录解耦，使「注入反向 require」的失败路径可以在纯内存中核对，不必
// 改动源码树。
func unitRequireViolations(requires map[string][]string) []string {
	var violations []string
	for unit, dependencies := range requires {
		allowed, registered := unitAllowedRequireEdges[unit]
		if !registered {
			violations = append(violations, fmt.Sprintf(
				"未登记单元 packages/%s：packages/ 下出现 go.mod 却不在 unitAllowedRequireEdges，新单元必须先声明允许依赖再立项", unit))
			continue
		}
		allowSet := make(map[string]bool, len(allowed))
		for _, dependency := range allowed {
			allowSet[dependency] = true
		}
		for _, dependency := range dependencies {
			if !allowSet[dependency] {
				violations = append(violations, fmt.Sprintf(
					"单元 packages/%s 不允许 require packages/%s：合法方向只有 server→shared/contracts、client→shared、tools→shared/server/client/contracts，audit/contracts/shared 不得依赖任何兄弟单元",
					unit, dependency))
			}
		}
	}
	slices.Sort(violations)
	return violations
}

// TestUnitRequireViolationsDetectDrift 用合成边核对检查器对每类漂移都真的
// 报错。真实源码树处于契约内（或尚未切割）时，方向断言的负向路径没有天然
// 的失败信号；必须另有合成断言钉住检查器本身，否则检查逻辑被改坏后门禁
// 静默变松。
func TestUnitRequireViolationsDetectDrift(t *testing.T) {
	// contractEdges 是恰好满足契约的最小合成 require 集：每个登记单元的
	// require 边都取允许上界，叶子单元不带边。
	contractEdges := func() map[string][]string {
		return map[string][]string{
			"server":    {"shared", "contracts"},
			"client":    {"shared"},
			"tools":     {"shared", "server", "client", "contracts"},
			"audit":     {},
			"contracts": {},
			"shared":    {},
		}
	}
	if violations := unitRequireViolations(contractEdges()); len(violations) != 0 {
		t.Fatalf("契约内的合成 require 边不应报违规: %v", violations)
	}

	for _, edge := range []struct {
		name string
		from string
		to   string
	}{
		{"client反向require server", "client", "server"},
		{"server反向require client", "server", "client"},
		{"shared反向require server", "shared", "server"},
		{"tools依赖audit", "tools", "audit"},
		{"audit依赖shared", "audit", "shared"},
	} {
		t.Run(edge.name, func(t *testing.T) {
			edges := contractEdges()
			edges[edge.from] = append(edges[edge.from], edge.to)
			violations := unitRequireViolations(edges)
			forbidden := "packages/" + edge.from + " 不允许 require packages/" + edge.to
			if len(violations) != 1 || !strings.Contains(violations[0], forbidden) {
				t.Fatalf("注入禁止边 %s 未被拒绝: %v", forbidden, violations)
			}
		})
	}

	t.Run("未登记单元", func(t *testing.T) {
		edges := contractEdges()
		edges["widget"] = []string{"shared"}
		violations := unitRequireViolations(edges)
		if len(violations) != 1 || !strings.Contains(violations[0], "未登记单元 packages/widget") {
			t.Fatalf("未登记单元未被拒绝: %v", violations)
		}
	})
}

// TestPackageUnitRequireBoundaries 把真实源码树里 packages/ 下各 go.mod 的
// require 边喂给 unitRequireViolations，钉住单元间的编译层依赖方向。
// packages/ 尚未立项时断言空转直接通过（切割分阶段进行，本测试随每个
// 模块落地自动收紧，全程无中间红）；非 Go 单元（engine、agent 族）没有
// go.mod，天然不参与 require 边。
func TestPackageUnitRequireBoundaries(t *testing.T) {
	root := moduleRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "packages"))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("读取 packages/: %v", err)
	}
	requires := make(map[string][]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		source, err := os.ReadFile(filepath.Join(root, "packages", entry.Name(), "go.mod"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("读取 packages/%s/go.mod: %v", entry.Name(), err)
		}
		requires[entry.Name()] = goModSiblingRequires(string(source))
	}
	if violations := unitRequireViolations(requires); len(violations) > 0 {
		t.Errorf("packages 单元 require 边违反契约，%d 条违规：\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// goModSiblingRequires 从 go.mod 文本中提取 require 的兄弟单元名集合（去重
// 排序）。只解析 require 指令（单行与块状两种形态），exclude/replace 等其它
// 指令里的模块路径不构成依赖边；行内注释与版本字段剥落后只取模块路径。
// 不用 golang.org/x/mod 是为了让 archcheck 保持零新增依赖，go.mod 的 require
// 语法足够稳定，手写解析的维护成本可接受。
func goModSiblingRequires(source string) []string {
	const modulePrefix = "github.com/channing771/mornlea/packages/"
	siblings := make(map[string]bool)
	inRequireBlock := false
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
		} else {
			rest, isRequire := strings.CutPrefix(line, "require")
			if !isRequire {
				continue
			}
			line = strings.TrimSpace(rest)
			if line == "(" {
				inRequireBlock = true
				continue
			}
		}
		if index := strings.Index(line, "//"); index >= 0 {
			line = strings.TrimSpace(line[:index])
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// 兄弟单元的 require 形如 <modulePrefix><unit> <version>；名字里
		// 再含斜杠说明指向的是深层路径而非单元根，属异常写法，直接忽略。
		if name, ok := strings.CutPrefix(fields[0], modulePrefix); ok && name != "" && !strings.Contains(name, "/") {
			siblings[name] = true
		}
	}
	return slices.Sorted(maps.Keys(siblings))
}
