package archcheck_test

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

var taskIDInCodeComment = regexp.MustCompile(`[A-F]-[0-9]{2}`)

type codeCommentSource struct {
	path string
	data []byte
}

type codeCommentSpan struct {
	offset int
	text   []byte
}

type codeCommentTaskIDFinding struct {
	path string
	line int
	ids  []string
}

// TestCodeCommentsExcludeTaskIDs 守住任务编号只属于规划产物的仓库纪律。
// 扫描器按语言词法边界提取注释，字符串字面量中的工具数据不属于检查对象。
func TestCodeCommentsExcludeTaskIDs(t *testing.T) {
	sources, err := repositoryCodeCommentSources(moduleRoot(t))
	if err != nil {
		t.Fatalf("收集 Go/Rust 源码: %v", err)
	}
	requireCodeCommentScanCoverage(t, sources)

	findings, err := scanCodeCommentTaskIDs(sources)
	if err != nil {
		t.Fatalf("扫描 Go/Rust 注释: %v", err)
	}
	if len(findings) == 0 {
		return
	}
	for _, finding := range findings {
		t.Errorf("%s:%d: 代码注释含任务编号 %s", finding.path, finding.line, strings.Join(finding.ids, ", "))
	}
	t.Fatalf("代码注释任务编号命中 %d 处", len(findings))
}

func TestCodeCommentTaskIDScanner(t *testing.T) {
	t.Run("已知坏注释会失败", func(t *testing.T) {
		findings, err := scanCodeCommentTaskIDs([]codeCommentSource{
			{path: "bad.go", data: []byte("package sample\n// 等待 A-01 完成\n")},
			{path: "bad.rs", data: []byte("// 等待 B-02 完成\nfn main() {}\n")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 2 {
			t.Fatalf("坏注释命中数 = %d，想要 2：%+v", len(findings), findings)
		}
	})

	t.Run("同形字符串不会失败", func(t *testing.T) {
		findings, err := scanCodeCommentTaskIDs([]codeCommentSource{
			{path: "strings.go", data: []byte("package sample\nconst task = `A-01 // B-02`\n")},
			{path: "strings.rs", data: []byte("const A: &str = \"C-03 // D-04\";\nconst B: &str = r#\"E-05 /* F-06 */\"#;\n")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 0 {
			t.Fatalf("字符串字面量被误报：%+v", findings)
		}
	})

	t.Run("普通注释不会失败", func(t *testing.T) {
		findings, err := scanCodeCommentTaskIDs([]codeCommentSource{
			{path: "ordinary.go", data: []byte("package sample\n// 用 SHA-256 表示内容摘要。\n")},
			{path: "ordinary.rs", data: []byte("/* 用 CRC-32C 校验持久化记录。 */\nfn main() {}\n")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 0 {
			t.Fatalf("普通注释被误报：%+v", findings)
		}
	})
}

func scanCodeCommentTaskIDs(sources []codeCommentSource) ([]codeCommentTaskIDFinding, error) {
	sources = slices.Clone(sources)
	slices.SortFunc(sources, func(left, right codeCommentSource) int {
		return strings.Compare(left.path, right.path)
	})

	var findings []codeCommentTaskIDFinding
	for _, source := range sources {
		var (
			comments []codeCommentSpan
			err      error
		)
		switch filepath.Ext(source.path) {
		case ".go":
			comments, err = goCodeComments(source)
		case ".rs":
			comments, err = rustCodeComments(source.data)
		default:
			return nil, fmt.Errorf("不支持的源码类型 %s", source.path)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source.path, err)
		}
		for _, comment := range comments {
			ids := taskIDsInCodeComment(comment.text)
			if len(ids) == 0 {
				continue
			}
			findings = append(findings, codeCommentTaskIDFinding{
				path: source.path,
				line: bytes.Count(source.data[:comment.offset], []byte{'\n'}) + 1,
				ids:  ids,
			})
		}
	}
	return findings, nil
}

func taskIDsInCodeComment(comment []byte) []string {
	var ids []string
	for _, match := range taskIDInCodeComment.FindAllIndex(comment, -1) {
		if match[0] > 0 && isASCIIAlphaNumeric(comment[match[0]-1]) {
			continue
		}
		if match[1] < len(comment) && isASCIIAlphaNumeric(comment[match[1]]) {
			continue
		}
		ids = append(ids, string(comment[match[0]:match[1]]))
	}
	return ids
}

func isASCIIAlphaNumeric(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func goCodeComments(source codeCommentSource) ([]codeCommentSpan, error) {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, source.path, source.data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var comments []codeCommentSpan
	for _, group := range parsed.Comments {
		for _, comment := range group.List {
			position := files.Position(comment.Pos())
			comments = append(comments, codeCommentSpan{offset: position.Offset, text: []byte(comment.Text)})
		}
	}
	return comments, nil
}

// rustCodeComments 只返回 Rust 行注释与可嵌套块注释；普通、byte、C 与 raw
// 字符串均先完整跳过，避免其中的注释形状被当成源码注释。
func rustCodeComments(source []byte) ([]codeCommentSpan, error) {
	var comments []codeCommentSpan
	for index := 0; index < len(source); {
		if end, ok, err := rustRawStringEnd(source, index); ok {
			if err != nil {
				return nil, err
			}
			index = end
			continue
		}
		if source[index] == 'b' || source[index] == 'c' {
			if index+1 < len(source) && source[index+1] == '"' {
				end, err := rustQuotedStringEnd(source, index+1)
				if err != nil {
					return nil, err
				}
				index = end
				continue
			}
			if source[index] == 'b' && index+1 < len(source) && source[index+1] == '\'' {
				if end, ok := rustCharLiteralEnd(source, index+1); ok {
					index = end
					continue
				}
			}
		}
		switch source[index] {
		case '"':
			end, err := rustQuotedStringEnd(source, index)
			if err != nil {
				return nil, err
			}
			index = end
		case '\'':
			if end, ok := rustCharLiteralEnd(source, index); ok {
				index = end
			} else {
				index++
			}
		case '/':
			if index+1 >= len(source) {
				index++
				continue
			}
			switch source[index+1] {
			case '/':
				end := bytes.IndexByte(source[index+2:], '\n')
				if end < 0 {
					end = len(source)
				} else {
					end += index + 2
				}
				comments = append(comments, codeCommentSpan{offset: index, text: source[index:end]})
				index = end
			case '*':
				end, err := rustBlockCommentEnd(source, index)
				if err != nil {
					return nil, err
				}
				comments = append(comments, codeCommentSpan{offset: index, text: source[index:end]})
				index = end
			default:
				index++
			}
		default:
			index++
		}
	}
	return comments, nil
}

func rustQuotedStringEnd(source []byte, quote int) (int, error) {
	for index := quote + 1; index < len(source); index++ {
		switch source[index] {
		case '\\':
			index++
		case '"':
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("offset %d 的字符串未闭合", quote)
}

func rustRawStringEnd(source []byte, start int) (end int, ok bool, err error) {
	index := start
	if index < len(source) && (source[index] == 'b' || source[index] == 'c') {
		index++
	}
	if index >= len(source) || source[index] != 'r' {
		return 0, false, nil
	}
	index++
	hashes := 0
	for index < len(source) && source[index] == '#' {
		hashes++
		index++
	}
	if index >= len(source) || source[index] != '"' {
		return 0, false, nil
	}
	for index++; index < len(source); index++ {
		if source[index] != '"' || index+hashes >= len(source) {
			continue
		}
		matched := true
		for hash := 0; hash < hashes; hash++ {
			if source[index+1+hash] != '#' {
				matched = false
				break
			}
		}
		if matched {
			return index + 1 + hashes, true, nil
		}
	}
	return 0, true, fmt.Errorf("offset %d 的 raw 字符串未闭合", start)
}

func rustCharLiteralEnd(source []byte, quote int) (int, bool) {
	index := quote + 1
	if index >= len(source) {
		return 0, false
	}
	if source[index] == '\\' {
		index++
		if index >= len(source) {
			return 0, false
		}
		switch source[index] {
		case 'x':
			index += 3
		case 'u':
			index += 2
			for index < len(source) && source[index] != '}' {
				index++
			}
			index++
		default:
			index++
		}
	} else {
		_, size := utf8.DecodeRune(source[index:])
		index += size
	}
	if index < len(source) && source[index] == '\'' {
		return index + 1, true
	}
	return 0, false
}

func rustBlockCommentEnd(source []byte, start int) (int, error) {
	depth := 1
	for index := start + 2; index+1 < len(source); index++ {
		switch string(source[index : index+2]) {
		case "/*":
			depth++
			index++
		case "*/":
			depth--
			index++
			if depth == 0 {
				return index + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("offset %d 的块注释未闭合", start)
}

func repositoryCodeCommentSources(root string) ([]codeCommentSource, error) {
	var sources []codeCommentSource
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".worktrees", "vendor", "node_modules", "target":
				return fs.SkipDir
			}
			return nil
		}
		extension := filepath.Ext(path)
		if extension != ".go" && extension != ".rs" {
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
		sources = append(sources, codeCommentSource{path: filepath.ToSlash(relative), data: data})
		return nil
	})
	return sources, err
}

func requireCodeCommentScanCoverage(t *testing.T, sources []codeCommentSource) {
	t.Helper()
	counts := map[string]int{".go": 0, ".rs": 0}
	paths := make(map[string]bool, len(sources))
	for _, source := range sources {
		counts[filepath.Ext(source.path)]++
		paths[source.path] = true
		for _, component := range strings.Split(source.path, "/") {
			switch component {
			case ".git", ".worktrees", "vendor", "node_modules", "target":
				t.Fatalf("扫描包含应跳过的源码副本 %s", source.path)
			}
		}
	}
	if counts[".go"] < 500 || counts[".rs"] < 20 {
		t.Fatalf("源码覆盖异常：Go=%d Rust=%d", counts[".go"], counts[".rs"])
	}
	for _, required := range []string{
		"internal/archcheck/code_comment_task_id_test.go",
		"engine/crates/mornlea_client/src/window.rs",
	} {
		if !paths[required] {
			t.Fatalf("源码扫描根遗漏 %s", required)
		}
	}
}
