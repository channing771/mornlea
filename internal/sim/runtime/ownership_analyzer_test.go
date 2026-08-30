package runtime

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ownershipSource struct {
	name     string
	contents string
}

type parsedOwnershipSource struct {
	ownershipSource
	file   *ast.File
	isTest bool
}

type ownershipTypeDecl struct {
	spec   *ast.TypeSpec
	file   string
	isTest bool
}

type ownershipFunction struct {
	key    string
	label  string
	file   string
	body   *ast.BlockStmt
	direct uint64
	edges  map[string]struct{}
}

const (
	stageApplyPlayerCommands uint64 = 1 << iota
	stageApplyCompanionActions
	stageAdvanceActors
	stageAdvanceHostiles
	stageSettleGameplay
	stageSettleTramples
	stageFinishWorld
	stagePublish
	stageAdvanceFluids
	stageAdvanceFarmlandMoisture
	stageAdvanceCrops
	stageSweepUnsupportedTorches
	stageSweepUnsupportedBeds
	stageMutationCommit
)

const completeOwnershipStageMask = stageApplyPlayerCommands |
	stageApplyCompanionActions |
	stageAdvanceActors |
	stageAdvanceHostiles |
	stageSettleGameplay |
	stageSettleTramples |
	stageFinishWorld |
	stagePublish |
	stageAdvanceFluids |
	stageAdvanceFarmlandMoisture |
	stageAdvanceCrops |
	stageSweepUnsupportedTorches |
	stageSweepUnsupportedBeds |
	stageMutationCommit

var ownershipStageByCall = map[string]uint64{
	"ApplyPlayerCommands":     stageApplyPlayerCommands,
	"ApplyCompanionActions":   stageApplyCompanionActions,
	"AdvanceActors":           stageAdvanceActors,
	"AdvanceHostiles":         stageAdvanceHostiles,
	"SettleGameplay":          stageSettleGameplay,
	"SettleTramples":          stageSettleTramples,
	"FinishWorld":             stageFinishWorld,
	"Publish":                 stagePublish,
	"AdvanceFluids":           stageAdvanceFluids,
	"AdvanceFarmlandMoisture": stageAdvanceFarmlandMoisture,
	"AdvanceCrops":            stageAdvanceCrops,
	"SweepUnsupportedTorches": stageSweepUnsupportedTorches,
	"SweepUnsupportedBeds":    stageSweepUnsupportedBeds,
	"Commit":                  stageMutationCommit,
}

var forbiddenFixtureInboxTypes = map[string]struct{}{
	"Command":         {},
	"CompanionAction": {},
	"HostileAction":   {},
	"AcquiredChunk":   {},
	"GeneratedChunk":  {},
}

// analyzeEntityOwnershipDirectory 加载 entity 全部 Go 源码；生产边界与测试
// 夹具由同一个分析器检查，避免真实树与合成用例规则分叉。
func analyzeEntityOwnershipDirectory(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	sources := make([]ownershipSource, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		sources = append(sources, ownershipSource{name: entry.Name(), contents: string(contents)})
	}
	return analyzeOwnershipSources(sources), nil
}

func analyzeOwnershipSources(sources []ownershipSource) []string {
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })
	parsed := make([]parsedOwnershipSource, 0, len(sources))
	violations := make([]string, 0)
	fileSet := token.NewFileSet()
	for _, source := range sources {
		file, err := parser.ParseFile(fileSet, source.name, source.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: 解析失败: %v", source.name, err))
			continue
		}
		parsed = append(parsed, parsedOwnershipSource{
			ownershipSource: source,
			file:            file,
			isTest:          strings.HasSuffix(source.name, "_test.go"),
		})
	}

	types := collectOwnershipTypes(parsed)
	violations = append(violations, inspectProductionOwnership(parsed)...)
	violations = append(violations, inspectFixtureOwnership(parsed, types)...)
	violations = append(violations, inspectCopiedTickClosures(parsed)...)
	sort.Strings(violations)
	return deduplicateOwnershipViolations(violations)
}

func collectOwnershipTypes(files []parsedOwnershipSource) map[string]ownershipTypeDecl {
	types := make(map[string]ownershipTypeDecl)
	for _, source := range files {
		for _, declaration := range source.file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, item := range general.Specs {
				typeSpec, ok := item.(*ast.TypeSpec)
				if !ok {
					continue
				}
				types[typeSpec.Name.Name] = ownershipTypeDecl{
					spec: typeSpec, file: source.name, isTest: source.isTest,
				}
			}
		}
	}
	return types
}

func inspectProductionOwnership(files []parsedOwnershipSource) []string {
	violations := make([]string, 0)
	for _, source := range files {
		if source.isTest {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.TypeSpec:
				if value.Name.Name == "StepHooks" {
					violations = append(violations,
						fmt.Sprintf("%s: production StepHooks 反向回调表", source.name))
				}
			case *ast.FuncDecl:
				if value.Name.Name == "Step" && ownershipReceiverName(value.Recv) == "State" {
					violations = append(violations,
						fmt.Sprintf("%s: production State.Step 总调度入口", source.name))
				}
			}
			return true
		})
	}
	return violations
}

func inspectFixtureOwnership(
	files []parsedOwnershipSource,
	types map[string]ownershipTypeDecl,
) []string {
	engine, exists := types["Engine"]
	if !exists || !engine.isTest {
		return nil
	}
	violations := make([]string, 0)
	visited := make(map[string]struct{})
	var visitType func(string, string)
	var visitExpression func(ast.Expr, string)

	visitType = func(name, path string) {
		if _, seen := visited[name]; seen {
			return
		}
		declaration, ok := types[name]
		if !ok {
			return
		}
		visited[name] = struct{}{}
		visitExpression(declaration.spec.Type, path+" -> "+name)
	}
	visitExpression = func(expression ast.Expr, path string) {
		switch value := expression.(type) {
		case *ast.StructType:
			for _, field := range value.Fields.List {
				fieldPath := path
				if len(field.Names) != 0 {
					fieldPath += "." + field.Names[0].Name
				}
				visitExpression(field.Type, fieldPath)
			}
		case *ast.Ident:
			visitType(value.Name, path)
		case *ast.SelectorExpr:
			return
		case *ast.StarExpr:
			visitExpression(value.X, path)
		case *ast.ParenExpr:
			visitExpression(value.X, path)
		case *ast.ArrayType:
			if payload := ownershipPayloadType(value.Elt, types, nil); payload != "" {
				violations = append(violations,
					fmt.Sprintf("%s: fixture Engine 可达 %s inbox (%s)", engine.file, payload, path))
			}
			visitExpression(value.Elt, path)
		case *ast.ChanType:
			if payload := ownershipPayloadType(value.Value, types, nil); payload != "" {
				violations = append(violations,
					fmt.Sprintf("%s: fixture Engine 可达 %s inbox (%s)", engine.file, payload, path))
			}
			visitExpression(value.Value, path)
		case *ast.MapType:
			for _, contained := range []ast.Expr{value.Key, value.Value} {
				if payload := ownershipPayloadType(contained, types, nil); payload != "" {
					violations = append(violations,
						fmt.Sprintf("%s: fixture Engine 可达 %s inbox (%s)", engine.file, payload, path))
				}
				visitExpression(contained, path)
			}
		case *ast.IndexExpr:
			visitExpression(value.X, path)
			visitExpression(value.Index, path)
		case *ast.IndexListExpr:
			visitExpression(value.X, path)
			for _, index := range value.Indices {
				visitExpression(index, path)
			}
		}
	}
	visitType("Engine", "Engine")

	for _, source := range files {
		if !source.isTest {
			continue
		}
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || ownershipReceiverName(function.Recv) != "Engine" {
				continue
			}
			name := function.Name.Name
			if name == "Step" || strings.HasPrefix(name, "Enqueue") || strings.HasPrefix(name, "Submit") {
				violations = append(violations,
					fmt.Sprintf("%s: fixture Engine.%s 复制 runtime 入口", source.name, name))
			}
		}
	}
	return violations
}

func ownershipPayloadType(
	expression ast.Expr,
	types map[string]ownershipTypeDecl,
	seen map[string]struct{},
) string {
	if seen == nil {
		seen = make(map[string]struct{})
	}
	switch value := expression.(type) {
	case *ast.Ident:
		if _, forbidden := forbiddenFixtureInboxTypes[value.Name]; forbidden {
			return value.Name
		}
		if _, visited := seen[value.Name]; visited {
			return ""
		}
		declaration, ok := types[value.Name]
		if !ok {
			return ""
		}
		seen[value.Name] = struct{}{}
		return ownershipPayloadType(declaration.spec.Type, types, seen)
	case *ast.SelectorExpr:
		if _, forbidden := forbiddenFixtureInboxTypes[value.Sel.Name]; forbidden {
			return value.Sel.Name
		}
	case *ast.StarExpr:
		return ownershipPayloadType(value.X, types, seen)
	case *ast.ParenExpr:
		return ownershipPayloadType(value.X, types, seen)
	}
	return ""
}

func inspectCopiedTickClosures(files []parsedOwnershipSource) []string {
	functions := make(map[string]*ownershipFunction)
	freeFunctions := make(map[string]string)
	methods := make(map[string][]string)
	literalKeys := make(map[*ast.FuncLit]string)

	for _, source := range files {
		if !source.isTest {
			continue
		}
		literalIndex := 0
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			receiver := ownershipReceiverName(function.Recv)
			key := function.Name.Name
			label := key
			if receiver != "" {
				key = receiver + "." + function.Name.Name
				label = key
				methods[function.Name.Name] = append(methods[function.Name.Name], key)
			} else {
				freeFunctions[function.Name.Name] = key
			}
			functions[key] = &ownershipFunction{
				key: key, label: label, file: source.name,
				body: function.Body, edges: make(map[string]struct{}),
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.FuncLit)
				if !ok {
					return true
				}
				literalIndex++
				literalKey := fmt.Sprintf("%s#literal%d", key, literalIndex)
				literalKeys[literal] = literalKey
				functions[literalKey] = &ownershipFunction{
					key: literalKey, label: literalKey, file: source.name,
					body: literal.Body, edges: make(map[string]struct{}),
				}
				return true
			})
		}
	}

	for _, function := range functions {
		bindings := ownershipLiteralBindings(function.body, literalKeys)
		ast.Inspect(function.body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok {
				_, isRoot := literalKeys[literal]
				return !isRoot
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				if literalKey := bindings[target.Name]; literalKey != "" {
					function.edges[literalKey] = struct{}{}
				} else if key := freeFunctions[target.Name]; key != "" {
					function.edges[key] = struct{}{}
				}
			case *ast.SelectorExpr:
				function.direct |= ownershipStageByCall[target.Sel.Name]
				for _, key := range methods[target.Sel.Name] {
					function.edges[key] = struct{}{}
				}
			case *ast.FuncLit:
				if key := literalKeys[target]; key != "" {
					function.edges[key] = struct{}{}
				}
			}
			return true
		})
	}

	closure := make(map[string]uint64, len(functions))
	for key, function := range functions {
		closure[key] = function.direct
	}
	changed := true
	for changed {
		changed = false
		for key, function := range functions {
			stages := closure[key]
			for edge := range function.edges {
				stages |= closure[edge]
			}
			if stages != closure[key] {
				closure[key] = stages
				changed = true
			}
		}
	}

	violations := make([]string, 0)
	for key, function := range functions {
		if closure[key]&completeOwnershipStageMask == completeOwnershipStageMask {
			violations = append(violations, fmt.Sprintf(
				"%s: %s 的 helper 传递闭包复制完整 runtime tick 编排",
				function.file,
				function.label,
			))
		}
	}
	return violations
}

func ownershipLiteralBindings(
	body *ast.BlockStmt,
	literalKeys map[*ast.FuncLit]string,
) map[string]string {
	bindings := make(map[string]string)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, expression := range value.Rhs {
				literal, ok := expression.(*ast.FuncLit)
				if !ok || index >= len(value.Lhs) {
					continue
				}
				name, ok := value.Lhs[index].(*ast.Ident)
				if ok {
					bindings[name.Name] = literalKeys[literal]
				}
			}
		case *ast.ValueSpec:
			for index, expression := range value.Values {
				literal, ok := expression.(*ast.FuncLit)
				if !ok || index >= len(value.Names) {
					continue
				}
				bindings[value.Names[index].Name] = literalKeys[literal]
			}
		}
		return true
	})
	return bindings
}

func ownershipReceiverName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) != 1 {
		return ""
	}
	expression := receiver.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	if named, ok := expression.(*ast.Ident); ok {
		return named.Name
	}
	return ""
}

func deduplicateOwnershipViolations(violations []string) []string {
	if len(violations) < 2 {
		return violations
	}
	write := 1
	for read := 1; read < len(violations); read++ {
		if violations[read] == violations[write-1] {
			continue
		}
		violations[write] = violations[read]
		write++
	}
	return violations[:write]
}
