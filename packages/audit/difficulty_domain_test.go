package archcheck_test

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// coreDifficultyPath 是 `core.Difficulty` 域值的唯一权威文件：三档枚举、合法性
// 判定、格式化与严格小写解析都定义在这一处。
const coreDifficultyPath = "packages/shared/core/difficulty.go"

// coreDifficultyDir 是允许声明名为 `Difficulty` 类型的唯一目录。
const coreDifficultyDir = "packages/shared/core"

// difficultyBlindRoots 是必须对难度零感知的生产子树：图形客户端不持有、不
// 预测难度，线上协议与跨语言 JSON 契约不新增难度字段——难度差异只能经服务端
// 行为间接可观察，这是权威难度规约的长期架构边界。
var difficultyBlindRoots = []string{
	"packages/client",
	"packages/contracts",
	"packages/shared/network",
}

// difficultyCanonicalTexts 是三档难度的全部规范文本。作为独立字符串字面量，
// 它们只允许出现在权威文件：其他位置出现同值字面量，通常意味着 CLI、
// metadata 或仿真侧重新发明了一张难度文本表，绕开了域值判定。
var difficultyCanonicalTexts = map[string]bool{
	`"normal"`:   true,
	`"peaceful"`: true,
	`"hard"`:     true,
}

// TestDifficultyStaysASingleCoreDomain 把世界难度的「单一事实源」钉进源码
// 门禁，守住三条边界：
//   - 名为 `Difficulty` 的类型只允许声明在 `core`，别处的同名声明即第二套
//     枚举；storage 只持有该域值，CLI 解析统一经 `core.ParseDifficulty`。
//   - 客户端、线上协议与跨语言契约的生产源码不出现任何难度标识符；要让
//     难度上线，必须先经 OpenSpec 变更「客户端与协议不感知难度」的裁决。
//   - 三档规范文本作为独立字符串字面量只存在于权威文件。
//
// 守卫只覆盖生产 Go 源码（跳过 `_test.go`）：测试夹具表达三档难度是正当的
// 消费行为，不是第二套规则。它防的是标识符与文本表层的侵蚀；一套不出现
// `Difficulty` 字样的影子难度逻辑仍需行为测试与评审发现，本守卫不替代。
func TestDifficultyStaysASingleCoreDomain(t *testing.T) {
	root := repositoryRoot(t)
	for _, module := range workspaceModules(t) {
		err := filepath.WalkDir(filepath.Join(root, module), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if scanDifficultyDomainTokens(t, path, relative, source) {
				assertSingleDifficultyTypeDeclaration(t, path, relative, source)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描 %s 生产源码: %v", module, err)
		}
	}
}

// scanDifficultyDomainTokens 对单个生产文件做 token 级检查，返回文件是否出现
// 难度标识符（供类型声明的 AST 精查复用）。注释不进入 token 流：文档注释里
// 讨论难度是正当的，不受本守卫约束。
func scanDifficultyDomainTokens(t *testing.T, path, relative string, source []byte) bool {
	t.Helper()
	fileSet := token.NewFileSet()
	var sourceScanner scanner.Scanner
	sourceScanner.Init(fileSet.AddFile(path, -1, len(source)), source, nil, 0)
	blind := false
	for _, blindRoot := range difficultyBlindRoots {
		if relative == blindRoot || strings.HasPrefix(relative, blindRoot+"/") {
			blind = true
			break
		}
	}
	mentionsDifficulty := false
	for {
		position, kind, literal := sourceScanner.Scan()
		if kind == token.EOF {
			break
		}
		switch kind {
		case token.IDENT:
			// 子串而非等值匹配：侵蚀通常以复合标识符形式出现（难度覆盖入口、
			// 协议字段名等），恰好等值反而是最罕见的形态。
			if !strings.Contains(strings.ToLower(literal), "difficulty") {
				continue
			}
			mentionsDifficulty = true
			if blind {
				t.Errorf(
					"%s: 难度零感知子树出现标识符 %s（客户端不持有难度，协议与契约不新增难度字段；需上线难度先经 OpenSpec 变更裁决）",
					fileSet.Position(position), literal,
				)
			}
		case token.STRING:
			if difficultyCanonicalTexts[literal] && relative != coreDifficultyPath {
				t.Errorf(
					"%s: 难度规范文本 %s 只允许出现在 %s，其他位置的独立字面量是一张第二套难度文本表的开端",
					fileSet.Position(position), literal, coreDifficultyPath,
				)
			}
			if blind {
				if unquoted, unquoteErr := strconv.Unquote(literal); unquoteErr == nil &&
					strings.Contains(strings.ToLower(unquoted), "difficulty") {
					t.Errorf(
						"%s: 难度零感知子树出现难度字符串 %s",
						fileSet.Position(position), literal,
					)
				}
			}
		}
	}
	return mentionsDifficulty
}

// assertSingleDifficultyTypeDeclaration 在文件提到难度标识符时做 AST 精查：
// 名为 `Difficulty` 的类型声明只允许出现在 `core` 目录。
func assertSingleDifficultyTypeDeclaration(t *testing.T, path, relative string, source []byte) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("解析 %s: %v", path, err)
	}
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Difficulty" {
				continue
			}
			if !strings.HasPrefix(relative, coreDifficultyDir+"/") {
				t.Errorf(
					"%s: 声明了第二套难度类型 Difficulty；难度域值只允许定义在 %s",
					path, coreDifficultyDir,
				)
			}
		}
	}
}
