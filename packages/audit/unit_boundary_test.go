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
// server → shared/contracts（另 MAY require client，仅测试文件消费——客户端
// 镜像驱动的 Memory/TCP 集成测试，生产禁令由源码守卫强制）、
// client → shared/server（server 边仅由 cmd/mornlea 应用入口消费——普通本地
// 与 benchmark 模式在进程内装配本地权威 Host，client 域库包的禁令同样由
// 源码守卫强制）、tools → shared/server/client/contracts（perfcheck 同时消费
// 服务端与客户端侧包），audit/contracts/shared 是叶子——audit 是跨模块的
// 枚举校验者，只以读文件的方式观察被审单元，住进任何依赖边都会失去审计
// 资格；contracts 只承载 embed 契约文件；shared 是最底层共享域。
//
// 为什么按 go.mod require 而不是 import 检查：require 边由工具链强制（未
// 登记的跨单元 import 在构建层直接拒收），边界在编译期先行生效且不依赖
// 源码扫描；「该不该 import」的包级语义方向仍由上方 allowed 表与
// server_client_boundary 的源码守卫负责，两层互补而非重复——Go 的 require
// 无法按测试/命令子树限定，那些「模块层放行、语义层禁止」的边必须下沉到
// 源码层才能守住。
//
// 切割分阶段落地：本表是目标状态而非现状清单，登记单元暂不存在不是违规
// （阶段前期真实树断言空转）；反向的漂移必须报错——packages/ 下出现带
// go.mod 却未登记的单元，说明新单元绕过边界悄悄立项。
var unitAllowedRequireEdges = map[string][]string{
	"server":    {"shared", "contracts", "client"},
	"client":    {"shared", "server"},
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
					"单元 packages/%s 不允许 require packages/%s：合法方向只有 server→shared/contracts（另 MAY 仅因测试 require client）、client→shared/server（server 边仅限客户端命令装配本地 Host）、tools→shared/server/client/contracts，audit/contracts/shared 不得依赖任何兄弟单元",
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
	// require 边都取允许上界，叶子单元不带边。server→client 与 client→server
	// 两条豁免边（测试驱动镜像、应用装配本地 Host）取上界时一并带上，钉住
	// 它们在 require 层合法；表被改窄时本断言立即翻红。它们在生产面的禁令由
	// server_client_boundary 的源码守卫负责，那里另有 drift 测试钉住。
	contractEdges := func() map[string][]string {
		return map[string][]string{
			"server":    {"shared", "contracts", "client"},
			"client":    {"shared", "server"},
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
		{"client反向require contracts", "client", "contracts"},
		{"client反向require tools", "client", "tools"},
		{"server反向require tools", "server", "tools"},
		{"shared反向require server", "shared", "server"},
		{"shared反向require client", "shared", "client"},
		{"contracts依赖shared", "contracts", "shared"},
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

// TestGoModSiblingRequiresSkipsIndirect 用合成 go.mod 文本钉住解析器只取
// 作者声明的直接 require：带 `// indirect` 注解的兄弟单元行是 go mod tidy 的
// 传递性记账（client 因 server 消费 contracts 而带上），不属于单元的依赖
// 声明，不得计入边界；解析器被改坏（indirect 也计入）时本测试翻红。
func TestGoModSiblingRequiresSkipsIndirect(t *testing.T) {
	source := `module github.com/channing771/mornlea/packages/client

go 1.26.0

require (
	github.com/channing771/mornlea/packages/contracts v0.0.0-00010101000000-000000000000 // indirect
	github.com/channing771/mornlea/packages/server v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/shared v0.0.0-00010101000000-000000000000
)

replace github.com/channing771/mornlea/packages/contracts => ../contracts
`
	if got := goModSiblingRequires(source); !slices.Equal(got, []string{"server", "shared"}) {
		t.Fatalf("indirect 兄弟 require 不应计入边界，实际得到 %v", got)
	}
	// 单行 require 与 replace 指令的正反两面：单行直接 require 计入，
	// replace 指向的兄弟目录不是依赖边。
	source = "module github.com/channing771/mornlea/packages/server\n" +
		"require github.com/channing771/mornlea/packages/client v0.0.0-00010101000000-000000000000\n" +
		"replace github.com/channing771/mornlea/packages/client => ../client\n"
	if got := goModSiblingRequires(source); !slices.Equal(got, []string{"client"}) {
		t.Fatalf("单行 require 应计入且 replace 不应计入，实际得到 %v", got)
	}
}

// TestPackageUnitRequireBoundaries 把真实源码树里 packages/ 下各 go.mod 的
// require 边喂给 unitRequireViolations，钉住单元间的编译层依赖方向。根模块
// 解散后本测试覆盖全部六个 Go 单元；非 Go 单元（engine、agent 族）没有
// go.mod，天然不参与 require 边。
func TestPackageUnitRequireBoundaries(t *testing.T) {
	root := repositoryRoot(t)
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

	// server 的兄弟 require 集门禁化为精确值 {client, contracts, shared}：client
	// 是测试专用豁免边（28 个客户端镜像驱动的 Memory/TCP 集成测试），Go 的
	// require 无法按测试限定，模块层只能放行；「生产文件禁 import client」由
	// 源码守卫兜底。这里把集合钉成精确值——多一条由 unitRequireViolations 报，
	// 少一条（例如有人以重构测试为由删掉 client require）由本断言报，豁免边
	// 既不能悄悄扩大也不能悄悄消失。
	if serverRequires, ok := requires["server"]; ok && len(serverRequires) > 0 {
		if want := []string{"client", "contracts", "shared"}; !slices.Equal(serverRequires, want) {
			t.Errorf("server 的兄弟 require 集必须是 %v，实际 %v（client 边是测试专用豁免，生产禁令由源码守卫强制）", want, serverRequires)
		}
	}
}

// TestWorkspaceUseSetMatchesUnitModules 钉住 go.work 与 packages/ 下 go.mod 集
// 的双向一致：use 列表多一项（例如解散后又冒出根模块的 `.`）或少一项（新单元
// 立项却忘记 use）都是模块拓扑漂移——枚举类检查以 use 列表为输入，漏 use 的
// 模块会在不知不觉中退出全部审计。非 Go 单元（engine、agent 族）没有 go.mod，
// 天然不参与比对。
func TestWorkspaceUseSetMatchesUnitModules(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "packages"))
	if err != nil {
		t.Fatalf("读取 packages/: %v", err)
	}
	unitModules := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "packages", entry.Name(), "go.mod")); err == nil {
			unitModules = append(unitModules, "packages/"+entry.Name())
		}
	}
	slices.Sort(unitModules)
	workspace := workspaceModules(t)
	if !slices.Equal(workspace, unitModules) {
		t.Errorf("go.work use 集与 packages/ 下 go.mod 集不一致：use=%v，单元模块=%v（新单元必须同时立项 go.mod 与 go.work use）", workspace, unitModules)
	}
}

// goModSiblingRequires 从 go.mod 文本中提取 require 的兄弟单元名集合（去重
// 排序）。只解析 require 指令（单行与块状两种形态），exclude/replace 等其它
// 指令里的模块路径不构成依赖边；行内注释剥落后只取模块路径，但带
// `// indirect` 注解的行跳过——间接 require 是 go mod tidy 的传递性记账而非
// 单元作者声明的依赖意图（例如 client 因 server 消费 contracts 而带一行
// indirect），上游停止消费后该行自动消失，边界门禁不应随这种记账漂移。
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
			comment := strings.TrimSpace(line[index:])
			if strings.HasSuffix(comment, "indirect") {
				continue
			}
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
