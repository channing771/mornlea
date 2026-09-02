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
	key                string
	label              string
	file               string
	body               *ast.BlockStmt
	callableParameters []ownershipCallableParameter
	direct             uint64
	edges              map[string]struct{}
}

type ownershipCallableParameter struct {
	position int
	variadic bool
}

type ownershipCallableSet map[string]struct{}

type ownershipCallableBindings map[string]ownershipCallableSet

type ownershipBoundType struct {
	expression ast.Expr
	bindings   ownershipTypeBindings
}

type ownershipTypeBindings map[string]ownershipBoundType

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
	stageSweepUnsupportedWildPlants
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
	stageSweepUnsupportedWildPlants |
	stageSweepUnsupportedTorches |
	stageSweepUnsupportedBeds |
	stageMutationCommit

var ownershipStageByCall = map[string]uint64{
	"ApplyPlayerCommands":        stageApplyPlayerCommands,
	"ApplyCompanionActions":      stageApplyCompanionActions,
	"AdvanceActors":              stageAdvanceActors,
	"AdvanceHostiles":            stageAdvanceHostiles,
	"SettleGameplay":             stageSettleGameplay,
	"SettleTramples":             stageSettleTramples,
	"FinishWorld":                stageFinishWorld,
	"Publish":                    stagePublish,
	"AdvanceFluids":              stageAdvanceFluids,
	"AdvanceFarmlandMoisture":    stageAdvanceFarmlandMoisture,
	"AdvanceCrops":               stageAdvanceCrops,
	"SweepUnsupportedWildPlants": stageSweepUnsupportedWildPlants,
	"SweepUnsupportedTorches":    stageSweepUnsupportedTorches,
	"SweepUnsupportedBeds":       stageSweepUnsupportedBeds,
	"Commit":                     stageMutationCommit,
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
	var visitType func(string, []ast.Expr, ownershipTypeBindings, string)
	var visitExpression func(ast.Expr, ownershipTypeBindings, string)

	visitType = func(
		name string,
		arguments []ast.Expr,
		outer ownershipTypeBindings,
		path string,
	) {
		declaration, ok := types[name]
		if !ok {
			return
		}
		bindings := ownershipBindTypeParameters(declaration.spec, arguments, outer)
		instance := ownershipTypeInstanceKey(name, declaration.spec, bindings)
		if _, seen := visited[instance]; seen {
			return
		}
		visited[instance] = struct{}{}
		visitExpression(declaration.spec.Type, bindings, path+" -> "+instance)
	}
	visitExpression = func(expression ast.Expr, bindings ownershipTypeBindings, path string) {
		switch value := expression.(type) {
		case *ast.StructType:
			for _, field := range value.Fields.List {
				fieldPath := path
				if len(field.Names) != 0 {
					fieldPath += "." + field.Names[0].Name
				}
				visitExpression(field.Type, bindings, fieldPath)
			}
		case *ast.Ident:
			if bound, ok := bindings[value.Name]; ok {
				visitExpression(bound.expression, bound.bindings, path)
				return
			}
			visitType(value.Name, nil, nil, path)
		case *ast.SelectorExpr:
			return
		case *ast.StarExpr:
			visitExpression(value.X, bindings, path)
		case *ast.ParenExpr:
			visitExpression(value.X, bindings, path)
		case *ast.ArrayType:
			if payload := ownershipPayloadType(value.Elt, types, bindings, nil); payload != "" {
				violations = append(violations,
					fmt.Sprintf("%s: fixture Engine 可达 %s inbox (%s)", engine.file, payload, path))
			}
			visitExpression(value.Elt, bindings, path)
		case *ast.ChanType:
			if payload := ownershipPayloadType(value.Value, types, bindings, nil); payload != "" {
				violations = append(violations,
					fmt.Sprintf("%s: fixture Engine 可达 %s inbox (%s)", engine.file, payload, path))
			}
			visitExpression(value.Value, bindings, path)
		case *ast.MapType:
			for _, contained := range []ast.Expr{value.Key, value.Value} {
				if payload := ownershipPayloadType(contained, types, bindings, nil); payload != "" {
					violations = append(violations,
						fmt.Sprintf("%s: fixture Engine 可达 %s inbox (%s)", engine.file, payload, path))
				}
				visitExpression(contained, bindings, path)
			}
		case *ast.IndexExpr:
			if name := ownershipNamedType(value.X); name != "" {
				visitType(name, []ast.Expr{value.Index}, bindings, path)
			}
		case *ast.IndexListExpr:
			if name := ownershipNamedType(value.X); name != "" {
				visitType(name, value.Indices, bindings, path)
			}
		}
	}
	visitType("Engine", nil, nil, "Engine")

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
	bindings ownershipTypeBindings,
	seen map[string]struct{},
) string {
	if seen == nil {
		seen = make(map[string]struct{})
	}
	switch value := expression.(type) {
	case *ast.Ident:
		if bound, ok := bindings[value.Name]; ok {
			return ownershipPayloadType(bound.expression, types, bound.bindings, seen)
		}
		if _, forbidden := forbiddenFixtureInboxTypes[value.Name]; forbidden {
			return value.Name
		}
		declaration, ok := types[value.Name]
		if !ok {
			return ""
		}
		instance := ownershipTypeInstanceKey(value.Name, declaration.spec, nil)
		if _, visited := seen[instance]; visited {
			return ""
		}
		seen[instance] = struct{}{}
		return ownershipPayloadType(declaration.spec.Type, types, nil, seen)
	case *ast.SelectorExpr:
		if _, forbidden := forbiddenFixtureInboxTypes[value.Sel.Name]; forbidden {
			return value.Sel.Name
		}
	case *ast.StarExpr:
		return ownershipPayloadType(value.X, types, bindings, seen)
	case *ast.ParenExpr:
		return ownershipPayloadType(value.X, types, bindings, seen)
	case *ast.ArrayType:
		return ownershipPayloadType(value.Elt, types, bindings, seen)
	case *ast.ChanType:
		return ownershipPayloadType(value.Value, types, bindings, seen)
	case *ast.MapType:
		if payload := ownershipPayloadType(value.Key, types, bindings, seen); payload != "" {
			return payload
		}
		return ownershipPayloadType(value.Value, types, bindings, seen)
	case *ast.IndexExpr:
		return ownershipPayloadInstantiation(
			value.X, []ast.Expr{value.Index}, types, bindings, seen,
		)
	case *ast.IndexListExpr:
		return ownershipPayloadInstantiation(value.X, value.Indices, types, bindings, seen)
	}
	return ""
}

func ownershipPayloadInstantiation(
	named ast.Expr,
	arguments []ast.Expr,
	types map[string]ownershipTypeDecl,
	outer ownershipTypeBindings,
	seen map[string]struct{},
) string {
	name := ownershipNamedType(named)
	declaration, ok := types[name]
	if !ok {
		return ""
	}
	bindings := ownershipBindTypeParameters(declaration.spec, arguments, outer)
	instance := ownershipTypeInstanceKey(name, declaration.spec, bindings)
	if _, visited := seen[instance]; visited {
		return ""
	}
	seen[instance] = struct{}{}
	return ownershipPayloadType(declaration.spec.Type, types, bindings, seen)
}

func ownershipBindTypeParameters(
	spec *ast.TypeSpec,
	arguments []ast.Expr,
	outer ownershipTypeBindings,
) ownershipTypeBindings {
	if spec.TypeParams == nil || len(arguments) == 0 {
		return nil
	}
	bindings := make(ownershipTypeBindings)
	argument := 0
	for _, field := range spec.TypeParams.List {
		for _, name := range field.Names {
			if argument >= len(arguments) {
				return bindings
			}
			bindings[name.Name] = ownershipBoundType{
				expression: arguments[argument],
				bindings:   outer,
			}
			argument++
		}
	}
	return bindings
}

func ownershipTypeInstanceKey(
	name string,
	spec *ast.TypeSpec,
	bindings ownershipTypeBindings,
) string {
	if spec.TypeParams == nil {
		return name
	}
	arguments := make([]string, 0)
	for _, field := range spec.TypeParams.List {
		for _, parameter := range field.Names {
			bound, ok := bindings[parameter.Name]
			if !ok {
				arguments = append(arguments, parameter.Name)
				continue
			}
			arguments = append(arguments,
				ownershipTypeExpressionKey(bound.expression, bound.bindings))
		}
	}
	return name + "[" + strings.Join(arguments, ",") + "]"
}

func ownershipTypeExpressionKey(
	expression ast.Expr,
	bindings ownershipTypeBindings,
) string {
	switch value := expression.(type) {
	case *ast.Ident:
		if bound, ok := bindings[value.Name]; ok {
			return ownershipTypeExpressionKey(bound.expression, bound.bindings)
		}
		return value.Name
	case *ast.SelectorExpr:
		return ownershipTypeExpressionKey(value.X, bindings) + "." + value.Sel.Name
	case *ast.StarExpr:
		return "*" + ownershipTypeExpressionKey(value.X, bindings)
	case *ast.ArrayType:
		return "[]" + ownershipTypeExpressionKey(value.Elt, bindings)
	case *ast.MapType:
		return "map[" + ownershipTypeExpressionKey(value.Key, bindings) + "]" +
			ownershipTypeExpressionKey(value.Value, bindings)
	case *ast.IndexExpr:
		return ownershipTypeExpressionKey(value.X, bindings) + "[" +
			ownershipTypeExpressionKey(value.Index, bindings) + "]"
	case *ast.IndexListExpr:
		parts := make([]string, 0, len(value.Indices))
		for _, index := range value.Indices {
			parts = append(parts, ownershipTypeExpressionKey(index, bindings))
		}
		return ownershipTypeExpressionKey(value.X, bindings) + "[" +
			strings.Join(parts, ",") + "]"
	}
	return fmt.Sprintf("%T", expression)
}

func ownershipNamedType(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.ParenExpr:
		return ownershipNamedType(value.X)
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
				body: function.Body, callableParameters: ownershipCallableParameters(function.Type),
				edges: make(map[string]struct{}),
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
					body: literal.Body, callableParameters: ownershipCallableParameters(literal.Type),
					edges: make(map[string]struct{}),
				}
				return true
			})
		}
	}

	for _, function := range functions {
		bindings := ownershipCallableBindingsForBody(
			function.body,
			literalKeys,
			freeFunctions,
			methods,
		)
		ast.Inspect(function.body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok {
				_, isRoot := literalKeys[literal]
				return !isRoot
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if target, ok := call.Fun.(*ast.SelectorExpr); ok {
				function.direct |= ownershipStageByCall[target.Sel.Name]
			}

			targets := ownershipCallableExpression(
				call.Fun,
				bindings,
				literalKeys,
				freeFunctions,
				methods,
			)
			for target := range targets {
				function.edges[target] = struct{}{}
				callee := functions[target]
				if callee == nil {
					continue
				}
				ownershipLinkCallableArguments(
					function.edges,
					call.Args,
					callee.callableParameters,
					bindings,
					literalKeys,
					freeFunctions,
					methods,
				)
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

func ownershipCallableBindingsForBody(
	body *ast.BlockStmt,
	literalKeys map[*ast.FuncLit]string,
	freeFunctions map[string]string,
	methods map[string][]string,
) ownershipCallableBindings {
	bindings := make(ownershipCallableBindings)
	bind := func(name string, expression ast.Expr) {
		if name == "_" {
			return
		}
		callables := ownershipCallableExpression(
			expression,
			bindings,
			literalKeys,
			freeFunctions,
			methods,
		)
		if len(callables) == 0 {
			return
		}
		if bindings[name] == nil {
			bindings[name] = make(ownershipCallableSet)
		}
		for callable := range callables {
			bindings[name][callable] = struct{}{}
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, expression := range value.Rhs {
				if index >= len(value.Lhs) {
					continue
				}
				name, ok := value.Lhs[index].(*ast.Ident)
				if ok {
					bind(name.Name, expression)
				}
			}
		case *ast.ValueSpec:
			for index, expression := range value.Values {
				if index >= len(value.Names) {
					continue
				}
				bind(value.Names[index].Name, expression)
			}
		}
		return true
	})
	return bindings
}

func ownershipCallableParameters(function *ast.FuncType) []ownershipCallableParameter {
	if function == nil || function.Params == nil {
		return nil
	}
	parameters := make([]ownershipCallableParameter, 0)
	position := 0
	for _, field := range function.Params.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		typeExpression := field.Type
		variadic := false
		if ellipsis, ok := typeExpression.(*ast.Ellipsis); ok {
			typeExpression = ellipsis.Elt
			variadic = true
		}
		_, callable := typeExpression.(*ast.FuncType)
		for index := 0; index < count; index++ {
			if callable {
				parameters = append(parameters, ownershipCallableParameter{
					position: position,
					variadic: variadic && index == count-1,
				})
			}
			position++
		}
	}
	return parameters
}

func ownershipCallableExpression(
	expression ast.Expr,
	bindings ownershipCallableBindings,
	literalKeys map[*ast.FuncLit]string,
	freeFunctions map[string]string,
	methods map[string][]string,
) ownershipCallableSet {
	callables := make(ownershipCallableSet)
	switch value := expression.(type) {
	case *ast.Ident:
		for callable := range bindings[value.Name] {
			callables[callable] = struct{}{}
		}
		if callable := freeFunctions[value.Name]; callable != "" {
			callables[callable] = struct{}{}
		}
	case *ast.SelectorExpr:
		for _, callable := range methods[value.Sel.Name] {
			callables[callable] = struct{}{}
		}
	case *ast.FuncLit:
		if callable := literalKeys[value]; callable != "" {
			callables[callable] = struct{}{}
		}
	case *ast.ParenExpr:
		return ownershipCallableExpression(
			value.X, bindings, literalKeys, freeFunctions, methods,
		)
	case *ast.IndexExpr:
		return ownershipCallableExpression(
			value.X, bindings, literalKeys, freeFunctions, methods,
		)
	case *ast.IndexListExpr:
		return ownershipCallableExpression(
			value.X, bindings, literalKeys, freeFunctions, methods,
		)
	}
	return callables
}

func ownershipLinkCallableArguments(
	edges map[string]struct{},
	arguments []ast.Expr,
	parameters []ownershipCallableParameter,
	bindings ownershipCallableBindings,
	literalKeys map[*ast.FuncLit]string,
	freeFunctions map[string]string,
	methods map[string][]string,
) {
	for _, parameter := range parameters {
		end := parameter.position + 1
		if parameter.variadic {
			end = len(arguments)
		}
		if parameter.position >= len(arguments) {
			continue
		}
		for index := parameter.position; index < end; index++ {
			callables := ownershipCallableExpression(
				arguments[index], bindings, literalKeys, freeFunctions, methods,
			)
			for callable := range callables {
				edges[callable] = struct{}{}
			}
		}
	}
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
