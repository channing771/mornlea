package archcheck_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const runtimeCorpusPathLiteral = "testdata/runtime-migration"

var runtimeCorpusFlagNameArgument = map[string]int{
	"Bool":        0,
	"BoolFunc":    0,
	"Duration":    0,
	"Float64":     0,
	"Func":        0,
	"Int":         0,
	"Int64":       0,
	"String":      0,
	"Uint":        0,
	"Uint64":      0,
	"BoolVar":     1,
	"DurationVar": 1,
	"Float64Var":  1,
	"IntVar":      1,
	"Int64Var":    1,
	"StringVar":   1,
	"TextVar":     1,
	"UintVar":     1,
	"Uint64Var":   1,
	"Var":         1,
}

var runtimeCorpusWriteActionTokens = []string{
	"update",
	"rewrite",
	"regen",
	"regenerate",
	"write",
}

type runtimeCorpusTestSource struct {
	path string
	data []byte
}

type runtimeCorpusWriterFlagViolation struct {
	path string
	name string
	line int
}

func TestCorpusTestFlagsCannotRewriteFrozenEvidence(t *testing.T) {
	violations, err := runtimeCorpusWriterFlagViolations(repositoryRoot(t))
	if err != nil {
		t.Fatalf("scan runtime-migration producer tests: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("runtime-migration producer tests expose tracked-evidence writer flags:\n%s", formatRuntimeCorpusWriterFlagViolations(violations))
	}
}

func TestCorpusWriterFlagGuardDetectsDrift(t *testing.T) {
	t.Run("constructor name arguments", func(t *testing.T) {
		sources := []runtimeCorpusTestSource{
			{
				path: "packages/tools/cmd/runtime-oracle/default_test.go",
				data: []byte(`package producer
import "flag"
func configure() {
	flag.Int("update", 0, "")
	flag.Bool("update-command-order-corpus", false, "")
}`),
			},
			{
				path: "packages/tools/cmd/runtime-oracle/alias_test.go",
				data: []byte(`package producer
import flags "flag"
func configure() {
	flags.BoolFunc("rewrite-bool-func", "", nil)
	flags.Duration("regen-duration", 0, "")
	flags.Float64("regenerate-float", 0, "")
	flags.Func("write-func", "", nil)
	flags.Int64("update-int64", 0, "")
	flags.String("rewrite-string", "", "")
	flags.Uint("regen-uint", 0, "")
	flags.Uint64("regenerate-uint64", 0, "")
	flags.BoolVar(nil, "write-bool-var", false, "")
	flags.DurationVar(nil, "update-duration-var", 0, "")
	flags.Float64Var(nil, "rewrite-float-var", 0, "")
	flags.IntVar(nil, "regen-int-var", 0, "")
	flags.Int64Var(nil, "regenerate-int64-var", 0, "")
	flags.StringVar(nil, "write-string-var", "", "")
	flags.TextVar(nil, "update-text-var", nil, "")
	flags.UintVar(nil, "rewrite-uint-var", 0, "")
	flags.Uint64Var(nil, "regen-uint64-var", 0, "")
	flags.Var(nil, "write-var", "")
}`),
			},
		}

		violations, err := runtimeCorpusWriterFlagViolationsInSources(sources)
		if err != nil {
			t.Fatalf("scan fixtures: %v", err)
		}
		want := []string{
			"regen-duration",
			"regen-int-var",
			"regen-uint",
			"regen-uint64-var",
			"regenerate-float",
			"regenerate-int64-var",
			"regenerate-uint64",
			"rewrite-bool-func",
			"rewrite-float-var",
			"rewrite-string",
			"rewrite-uint-var",
			"update",
			"update-command-order-corpus",
			"update-duration-var",
			"update-int64",
			"update-text-var",
			"write-bool-var",
			"write-func",
			"write-string-var",
			"write-var",
		}
		got := runtimeCorpusWriterFlagNames(violations)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("writer flags = %q, want %q", got, want)
		}
	})

	t.Run("dot import and FlagSet methods", func(t *testing.T) {
		sources := []runtimeCorpusTestSource{
			{
				path: "packages/tools/cmd/runtime-oracle/dot_test.go",
				data: []byte(`package producer
import . "flag"
func configure() { String("write-dot", "", "") }`),
			},
			{
				path: "packages/server/example/producer_test.go",
				data: []byte(`package producer
import "flag"
const corpus = "testdata/runtime-migration/cases/example"
func configure(set *flag.FlagSet) { set.Int("update-flag-set", 0, "") }`),
			},
			{
				path: "packages/server/example/shadowed_import_test.go",
				data: []byte(`package producer
import (
	"flag"
	set "strings"
)
const corpus = "testdata/runtime-migration/cases/example"
var _ = set.TrimSpace
func configure(set *flag.FlagSet) { set.Func("rewrite-shadowed-import", "", nil) }`),
			},
		}

		violations, err := runtimeCorpusWriterFlagViolationsInSources(sources)
		if err != nil {
			t.Fatalf("scan fixtures: %v", err)
		}
		want := []string{"rewrite-shadowed-import", "update-flag-set", "write-dot"}
		got := runtimeCorpusWriterFlagNames(violations)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("writer flags = %q, want %q", got, want)
		}
	})

	t.Run("safe filters and separate golden workflows", func(t *testing.T) {
		sources := []runtimeCorpusTestSource{
			{
				path: "packages/tools/cmd/runtime-oracle/filter_test.go",
				data: []byte(`package producer
import "flag"
var filter = flag.String("filter", "", "select cases")`),
			},
			{
				path: "packages/server/storage/chunk/chunk_codec_test.go",
				data: []byte(`package chunk
import "flag"
var update = flag.Bool("update-storage-golden", false, "")`),
			},
			{
				path: "packages/shared/network/codec/chunk_codec_test.go",
				data: []byte(`package codec
import "flag"
var update = flag.Bool("update-protocol-golden", false, "")`),
			},
		}

		violations, err := runtimeCorpusWriterFlagViolationsInSources(sources)
		if err != nil {
			t.Fatalf("scan fixtures: %v", err)
		}
		if len(violations) != 0 {
			t.Fatalf("safe or separately governed flags were rejected:\n%s", formatRuntimeCorpusWriterFlagViolations(violations))
		}
	})
}

func runtimeCorpusWriterFlagViolations(root string) ([]runtimeCorpusWriterFlagViolation, error) {
	packagesRoot := filepath.Join(root, "packages")
	var sources []runtimeCorpusTestSource
	err := filepath.WalkDir(packagesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
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
		sources = append(sources, runtimeCorpusTestSource{
			path: filepath.ToSlash(relative),
			data: data,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return runtimeCorpusWriterFlagViolationsInSources(sources)
}

func runtimeCorpusWriterFlagViolationsInSources(sources []runtimeCorpusTestSource) ([]runtimeCorpusWriterFlagViolation, error) {
	var violations []runtimeCorpusWriterFlagViolation
	for _, source := range sources {
		files := token.NewFileSet()
		parsed, err := parser.ParseFile(files, source.path, source.data, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", source.path, err)
		}
		if !runtimeCorpusProducerSource(source.path, parsed) {
			continue
		}

		flagAliases, dotImported := runtimeCorpusFlagImports(parsed)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			constructor, ok := runtimeCorpusFlagConstructor(call.Fun, flagAliases, dotImported)
			if !ok {
				return true
			}
			nameIndex := runtimeCorpusFlagNameArgument[constructor]
			if len(call.Args) <= nameIndex {
				return true
			}
			literal, ok := call.Args[nameIndex].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil || !runtimeCorpusWriterFlagName(name) {
				return true
			}
			violations = append(violations, runtimeCorpusWriterFlagViolation{
				path: source.path,
				name: name,
				line: files.Position(literal.Pos()).Line,
			})
			return true
		})
	}
	sort.Slice(violations, func(left, right int) bool {
		if violations[left].path != violations[right].path {
			return violations[left].path < violations[right].path
		}
		if violations[left].name != violations[right].name {
			return violations[left].name < violations[right].name
		}
		return violations[left].line < violations[right].line
	})
	return violations, nil
}

func runtimeCorpusProducerSource(path string, parsed *ast.File) bool {
	if strings.HasPrefix(filepath.ToSlash(path), "packages/tools/cmd/runtime-oracle/") {
		return true
	}
	classified := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil && strings.Contains(value, runtimeCorpusPathLiteral) {
			classified = true
			return false
		}
		return true
	})
	return classified
}

func runtimeCorpusFlagImports(parsed *ast.File) (map[string]bool, bool) {
	flagAliases := make(map[string]bool)
	dotImported := false
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		if path == "flag" {
			switch name {
			case ".":
				dotImported = true
			case "_":
			default:
				flagAliases[name] = true
			}
			continue
		}
	}
	return flagAliases, dotImported
}

func runtimeCorpusFlagConstructor(expression ast.Expr, flagAliases map[string]bool, dotImported bool) (string, bool) {
	switch expression := expression.(type) {
	case *ast.Ident:
		_, known := runtimeCorpusFlagNameArgument[expression.Name]
		return expression.Name, dotImported && known
	case *ast.SelectorExpr:
		constructor := expression.Sel.Name
		if _, known := runtimeCorpusFlagNameArgument[constructor]; !known {
			return "", false
		}
		if receiver, ok := expression.X.(*ast.Ident); ok {
			if flagAliases[receiver.Name] {
				return constructor, true
			}
		}
		// A selector on a locally constructed or otherwise unresolved receiver may
		// be a standard-library flag set; classified producer tests fail closed here.
		return constructor, true
	default:
		return "", false
	}
}

func runtimeCorpusWriterFlagName(name string) bool {
	lower := strings.ToLower(name)
	for _, action := range runtimeCorpusWriteActionTokens {
		if strings.Contains(lower, action) {
			return true
		}
	}
	return false
}

func runtimeCorpusWriterFlagNames(violations []runtimeCorpusWriterFlagViolation) []string {
	names := make([]string, 0, len(violations))
	for _, violation := range violations {
		names = append(names, violation.name)
	}
	sort.Strings(names)
	return names
}

func formatRuntimeCorpusWriterFlagViolations(violations []runtimeCorpusWriterFlagViolation) string {
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, fmt.Sprintf("%s:%d: flag %q", violation.path, violation.line, violation.name))
	}
	return strings.Join(lines, "\n")
}
