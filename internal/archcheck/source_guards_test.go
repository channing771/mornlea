package archcheck_test

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	physicsImportPath = "github.com/channing771/mornlea/packages/shared/physics"
	runtimeImportPath = "github.com/channing771/mornlea/packages/server/sim/runtime"
	tuningImportPath  = "github.com/channing771/mornlea/packages/shared/tuning"
)

type authorityFunction struct {
	name       string
	body       *ast.BlockStmt
	capture    token.Pos
	captureVar string
	gates      map[string]token.Pos
	calls      map[string][]authorityCall
}

type authorityCall struct {
	position token.Pos
	argument string
}

// TestAuthorityTickTunablesStayExplicit 守住权威 tick 的参数边界：entity 与
// server 不得重读全局物理参数或调用隐式 wrapper，runtime 只能在唯一捕获函数
// 内各读一次活动快照，server 的正常与关服 tick 必须把同一局部值传到各阶段。
func TestAuthorityTickTunablesStayExplicit(t *testing.T) {
	root := moduleRoot(t)
	assertNoStoredTickTunables(t, filepath.Join(root, "packages", "server", "server"))
	scopes := []struct {
		name      string
		directory string
	}{
		{name: "entity", directory: filepath.Join(root, "packages", "server", "sim", "entity")},
		{name: "runtime", directory: filepath.Join(root, "packages", "server", "sim", "runtime")},
		{name: "server", directory: filepath.Join(root, "packages", "server", "server")},
	}
	runtimeReads := map[string]int{physicsImportPath: 0, tuningImportPath: 0}
	serverCaptures := make(map[string]*authorityFunction)
	serverActiveCaptures := 0

	for _, scope := range scopes {
		files := 0
		err := filepath.WalkDir(scope.directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files++
			fileSet := token.NewFileSet()
			parsed, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				t.Fatalf("解析 %s：%v", path, err)
			}
			imports := authorityImports(t, path, parsed)
			functions := authorityFunctions(parsed)
			serverEngineAliases := make(map[*authorityFunction]map[string]bool)
			if scope.name == "server" {
				for _, function := range functions {
					serverEngineAliases[function] = authorityServerEngineAliases(function)
				}
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, selectorOK := node.(*ast.SelectorExpr)
				if selectorOK {
					function := authorityFunctionAt(functions, selector.Pos())
					functionName := "<package initializer>"
					if function != nil {
						functionName = function.name
					}
					importPath, referenceName, imported := authorityImportedSelector(selector, imports)
					if imported {
						if authorityForbiddenImportedReference(scope.name, importPath, referenceName) {
							t.Errorf("%s：%s 权威路径引用了隐式 %s.%s", fileSet.Position(selector.Pos()), scope.name, importPath, referenceName)
						}
						if scope.name == "runtime" && authorityActiveTunablesReference(importPath, referenceName) {
							runtimeReads[importPath]++
							if functionName != "ActiveTickTunables" {
								t.Errorf("%s：活动快照只能由 runtime.ActiveTickTunables 捕获", fileSet.Position(selector.Pos()))
							}
						}
						if scope.name == "server" && importPath == runtimeImportPath && referenceName == "ActiveTickTunables" {
							serverActiveCaptures++
							if function == nil || (function.name != "step" && function.name != "Shutdown") {
								t.Errorf("%s：server 参数束只能在 step 或 Shutdown 捕获", fileSet.Position(selector.Pos()))
							} else {
								serverCaptures[function.name] = function
							}
						}
					}
				}
				call, callOK := node.(*ast.CallExpr)
				if !callOK {
					return true
				}
				function := authorityFunctionAt(functions, call.Pos())
				if scope.name == "server" && authorityServerEngineStep(call, serverEngineAliases[function]) {
					t.Errorf("%s：server 权威路径必须调用 engine.StepWithTunables", fileSet.Position(call.Pos()))
				}
				return true
			})
			if scope.name == "server" {
				for _, function := range functions {
					collectAuthorityServerTrace(function, imports)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描 %s：%v", scope.directory, err)
		}
		if files == 0 {
			t.Fatalf("%s 未扫描到 production Go 文件", scope.directory)
		}
	}

	for importPath, count := range runtimeReads {
		if count != 1 {
			t.Errorf("runtime.ActiveTickTunables 对 %s 的读取次数=%d，想要 1", importPath, count)
		}
	}
	if serverActiveCaptures != 2 {
		t.Errorf("server 参数束捕获次数=%d，想要正常 tick 与关服 tick 各 1 次", serverActiveCaptures)
	}
	assertAuthorityServerTrace(t, serverCaptures["step"], true)
	assertAuthorityServerTrace(t, serverCaptures["Shutdown"], false)
}

func TestAuthorityTickTunablesGuardRejectsIndirectReferences(t *testing.T) {
	t.Run("imported function values", func(t *testing.T) {
		bad := authoritySyntheticSource(t, "bad_selectors.go", `package entity
import p "github.com/channing771/mornlea/packages/shared/physics"
import s "github.com/channing771/mornlea/packages/shared/tuning"
func bad() {
	step := (p.Step)
	submersion := p.SubmersionFlags
	physicsActive := p.ActiveTunables
	simulationActive := s.ActiveTunables
	_, _, _, _ = step, submersion, physicsActive, simulationActive
}`)
		if got := authorityForbiddenReferenceCount("entity", bad); got != 4 {
			t.Fatalf("间接禁用 selector 命中数=%d，想要 4", got)
		}

		good := authoritySyntheticSource(t, "good_selectors.go", `package entity
import p "github.com/channing771/mornlea/packages/shared/physics"
import s "github.com/channing771/mornlea/packages/shared/tuning"
func good() {
	step := p.StepWithTunables
	submersion := p.SubmersionFlagsWithTunables
	_, _, _ = step, submersion, s.Tunables{}
}`)
		if got := authorityForbiddenReferenceCount("entity", good); got != 0 {
			t.Fatalf("显式 selector 被误报 %d 次", got)
		}
	})

	t.Run("local engine receiver", func(t *testing.T) {
		bad := authoritySyntheticSource(t, "bad_engine.go", `package server
func (server *Server) bad() {
	engine := (server.engine)
	alias := engine
	alias.Step()
}`)
		if got := authorityImplicitServerStepCount(bad.file); got != 1 {
			t.Fatalf("局部 engine receiver 命中数=%d，想要 1", got)
		}

		good := authoritySyntheticSource(t, "good_engine.go", `package server
func (server *Server) good(bundle TickTunables) {
	engine := server.engine
	engine.StepWithTunables(bundle)
}`)
		if got := authorityImplicitServerStepCount(good.file); got != 0 {
			t.Fatalf("显式 engine receiver 被误报 %d 次", got)
		}
	})
}

func TestAuthorityTickTunablesStorageGuardExpandsNamedTypes(t *testing.T) {
	aliases := authoritySyntheticSource(t, "aliases.go", `package server
import rt "github.com/channing771/mornlea/packages/server/sim/runtime"
type TickAlias = rt.TickTunables
type TickNamed rt.TickTunables
var direct rt.TickTunables
var inferred = rt.TickTunables{}
`)
	holders := authoritySyntheticSource(t, "holders.go", `package server
type holder struct {
	alias TickAlias
	named TickNamed
}
var packageAlias TickAlias
var packageNamed TickNamed
`)
	if got := len(authorityStoredTickUses([]authoritySourceFile{aliases, holders})); got != 6 {
		t.Fatalf("跨文件 alias/named 存储命中数=%d，想要 6", got)
	}

	good := authoritySyntheticSource(t, "good_storage.go", `package server
import rt "github.com/channing771/mornlea/packages/server/sim/runtime"
type factory func() rt.TickTunables
type holder struct { make factory }
var packageFactory factory
`)
	if got := len(authorityStoredTickUses([]authoritySourceFile{good})); got != 0 {
		t.Fatalf("只在函数签名传递 bundle 被误报 %d 次", got)
	}
}

func authoritySyntheticSource(t *testing.T, name, source string) authoritySourceFile {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, name, source, 0)
	if err != nil {
		t.Fatalf("解析 synthetic source %s：%v", name, err)
	}
	return authoritySourceFile{
		path: name, fileSet: fileSet, file: parsed, imports: authorityImports(t, name, parsed),
	}
}

func authorityForbiddenReferenceCount(scope string, source authoritySourceFile) int {
	count := 0
	ast.Inspect(source.file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		importPath, name, imported := authorityImportedSelector(selector, source.imports)
		if imported && authorityForbiddenImportedReference(scope, importPath, name) {
			count++
		}
		return true
	})
	return count
}

func authorityImplicitServerStepCount(file *ast.File) int {
	count := 0
	for _, function := range authorityFunctions(file) {
		aliases := authorityServerEngineAliases(function)
		ast.Inspect(function.body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && authorityServerEngineStep(call, aliases) {
				count++
			}
			return true
		})
	}
	return count
}

func authorityImports(t *testing.T, path string, file *ast.File) map[string]string {
	t.Helper()
	imports := make(map[string]string, len(file.Imports))
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			t.Fatalf("解析 %s import：%v", path, err)
		}
		name := filepath.Base(importPath)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name == "." && (importPath == physicsImportPath || importPath == runtimeImportPath || importPath == tuningImportPath) {
			t.Errorf("%s：权威参数相关包不得 dot import %s", path, importPath)
			continue
		}
		if name != "_" {
			imports[name] = importPath
		}
	}
	return imports
}

func authorityFunctions(file *ast.File) []*authorityFunction {
	functions := make([]*authorityFunction, 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		functions = append(functions, &authorityFunction{
			name: function.Name.Name, body: function.Body,
			gates: make(map[string]token.Pos), calls: make(map[string][]authorityCall),
		})
	}
	return functions
}

func authorityFunctionAt(functions []*authorityFunction, position token.Pos) *authorityFunction {
	for _, function := range functions {
		if function.body.Pos() <= position && position <= function.body.End() {
			return function
		}
	}
	return nil
}

func authorityImportedCall(call *ast.CallExpr, imports map[string]string) (string, string, bool) {
	selector, ok := authorityUnparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	return authorityImportedSelector(selector, imports)
}

func authorityImportedSelector(selector *ast.SelectorExpr, imports map[string]string) (string, string, bool) {
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	importPath, ok := imports[identifier.Name]
	return importPath, selector.Sel.Name, ok
}

func authorityImplicitPhysicsCall(importPath, name string) bool {
	return importPath == physicsImportPath && (name == "Step" || name == "SubmersionFlags")
}

func authorityActiveTunablesReference(importPath, name string) bool {
	return (importPath == physicsImportPath || importPath == tuningImportPath) && name == "ActiveTunables"
}

func authorityForbiddenImportedReference(scope, importPath, name string) bool {
	if authorityImplicitPhysicsCall(importPath, name) {
		return true
	}
	return scope != "runtime" && authorityActiveTunablesReference(importPath, name)
}

func authorityUnparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func authorityServerEngineStep(call *ast.CallExpr, aliases map[string]bool) bool {
	selector, ok := authorityUnparen(call.Fun).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Step" {
		return false
	}
	return authorityServerEngineExpression(selector.X, aliases)
}

func authorityServerEngineAliases(function *authorityFunction) map[string]bool {
	aliases := make(map[string]bool)
	if function == nil {
		return aliases
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(function.body, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.AssignStmt:
				for index := 0; index < len(current.Lhs) && index < len(current.Rhs); index++ {
					identifier, ok := current.Lhs[index].(*ast.Ident)
					if ok && !aliases[identifier.Name] && authorityServerEngineExpression(current.Rhs[index], aliases) {
						aliases[identifier.Name] = true
						changed = true
					}
				}
			case *ast.DeclStmt:
				general, ok := current.Decl.(*ast.GenDecl)
				if !ok || general.Tok != token.VAR {
					return true
				}
				for _, specification := range general.Specs {
					value := specification.(*ast.ValueSpec)
					for index := 0; index < len(value.Names) && index < len(value.Values); index++ {
						name := value.Names[index].Name
						if !aliases[name] && authorityServerEngineExpression(value.Values[index], aliases) {
							aliases[name] = true
							changed = true
						}
					}
				}
			}
			return true
		})
	}
	return aliases
}

func authorityServerEngineExpression(expression ast.Expr, aliases map[string]bool) bool {
	switch current := authorityUnparen(expression).(type) {
	case *ast.Ident:
		return aliases[current.Name]
	case *ast.SelectorExpr:
		receiver, ok := authorityUnparen(current.X).(*ast.Ident)
		return ok && receiver.Name == "server" && current.Sel.Name == "engine"
	default:
		return false
	}
}

type authoritySourceFile struct {
	path    string
	fileSet *token.FileSet
	file    *ast.File
	imports map[string]string
}

type authorityStoredTickUse struct {
	position token.Position
	owner    string
}

func assertNoStoredTickTunables(t *testing.T, directory string) {
	t.Helper()
	groups := make(map[string][]authoritySourceFile)
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return err
		}
		key := filepath.Dir(path) + "\x00" + parsed.Name.Name
		groups[key] = append(groups[key], authoritySourceFile{
			path: path, fileSet: fileSet, file: parsed, imports: authorityImports(t, path, parsed),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 server tick 参数存储：%v", err)
	}
	if len(groups) == 0 {
		t.Fatal("server tick 参数存储守卫未扫描到 production Go package")
	}
	for _, files := range groups {
		for _, use := range authorityStoredTickUses(files) {
			t.Errorf("%s：server %s 不得保存 tick 参数束", use.position, use.owner)
		}
	}
}

func authorityStoredTickUses(files []authoritySourceFile) []authorityStoredTickUse {
	named := make(map[string]bool)
	for changed := true; changed; {
		changed = false
		for _, source := range files {
			for _, declaration := range source.file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, specification := range general.Specs {
					typeSpec := specification.(*ast.TypeSpec)
					if !named[typeSpec.Name.Name] && authorityTypeStoresTick(typeSpec.Type, source.imports, named) {
						named[typeSpec.Name.Name] = true
						changed = true
					}
				}
			}
		}
	}

	uses := make([]authorityStoredTickUse, 0)
	for _, source := range files {
		for _, declaration := range source.file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			switch general.Tok {
			case token.TYPE:
				for _, specification := range general.Specs {
					typeSpec := specification.(*ast.TypeSpec)
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range structure.Fields.List {
						if authorityTypeStoresTick(field.Type, source.imports, named) {
							uses = append(uses, authorityStoredTickUse{
								position: source.fileSet.Position(field.Pos()), owner: "struct " + typeSpec.Name.Name,
							})
						}
					}
				}
			case token.VAR:
				for _, specification := range general.Specs {
					value := specification.(*ast.ValueSpec)
					stored := value.Type != nil && authorityTypeStoresTick(value.Type, source.imports, named)
					for _, expression := range value.Values {
						stored = stored || authorityValueStoresTick(expression, source.imports, named)
					}
					if stored {
						for _, name := range value.Names {
							uses = append(uses, authorityStoredTickUse{
								position: source.fileSet.Position(name.Pos()), owner: "package variable " + name.Name,
							})
						}
					}
				}
			}
		}
	}
	return uses
}

func authorityTypeStoresTick(expression ast.Expr, imports map[string]string, named map[string]bool) bool {
	switch current := authorityUnparen(expression).(type) {
	case *ast.Ident:
		return named[current.Name]
	case *ast.SelectorExpr:
		identifier, ok := current.X.(*ast.Ident)
		return ok && imports[identifier.Name] == runtimeImportPath && current.Sel.Name == "TickTunables"
	case *ast.StarExpr:
		return authorityTypeStoresTick(current.X, imports, named)
	case *ast.ArrayType:
		return authorityTypeStoresTick(current.Elt, imports, named)
	case *ast.MapType:
		return authorityTypeStoresTick(current.Key, imports, named) || authorityTypeStoresTick(current.Value, imports, named)
	case *ast.ChanType:
		return authorityTypeStoresTick(current.Value, imports, named)
	case *ast.StructType:
		for _, field := range current.Fields.List {
			if authorityTypeStoresTick(field.Type, imports, named) {
				return true
			}
		}
	}
	return false
}

func authorityValueStoresTick(expression ast.Expr, imports map[string]string, named map[string]bool) bool {
	stored := false
	ast.Inspect(expression, func(node ast.Node) bool {
		current, ok := node.(ast.Expr)
		if ok && authorityTypeStoresTick(current, imports, named) {
			stored = true
			return false
		}
		return !stored
	})
	return stored
}

func collectAuthorityServerTrace(function *authorityFunction, imports map[string]string) {
	if function.name != "step" && function.name != "Shutdown" {
		return
	}
	ast.Inspect(function.body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.AssignStmt:
			if len(current.Lhs) != 1 || len(current.Rhs) != 1 {
				return true
			}
			call, ok := current.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			importPath, name, imported := authorityImportedCall(call, imports)
			identifier, identifierOK := current.Lhs[0].(*ast.Ident)
			if imported && importPath == runtimeImportPath && name == "ActiveTickTunables" && identifierOK {
				function.capture = call.Pos()
				function.captureVar = identifier.Name
			}
		case *ast.SelectorExpr:
			identifier, ok := current.X.(*ast.Ident)
			if function.name == "step" && ok && identifier.Name == "server" && current.Sel.Name == "lifecycle" {
				function.gates["lifecycle"] = earliestPosition(function.gates["lifecycle"], current.Pos())
			}
		case *ast.CallExpr:
			selector, ok := current.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if function.name == "step" && selector.Sel.Name == "Load" {
				if paused, ok := selector.X.(*ast.SelectorExpr); ok && paused.Sel.Name == "paused" {
					function.gates["paused"] = earliestPosition(function.gates["paused"], current.Pos())
				}
			}
			switch selector.Sel.Name {
			case "drainIncomingChats", "advanceCompanionTasks", "StepWithTunables":
				argument := ""
				if len(current.Args) == 1 {
					if identifier, ok := current.Args[0].(*ast.Ident); ok {
						argument = identifier.Name
					}
				}
				function.calls[selector.Sel.Name] = append(function.calls[selector.Sel.Name], authorityCall{
					position: current.Pos(), argument: argument,
				})
			}
		}
		return true
	})
}

func earliestPosition(current, candidate token.Pos) token.Pos {
	if current == token.NoPos || candidate < current {
		return candidate
	}
	return current
}

func assertAuthorityServerTrace(t *testing.T, function *authorityFunction, normalTick bool) {
	t.Helper()
	if function == nil || function.capture == token.NoPos || function.captureVar == "" {
		t.Fatalf("server %v tick 缺少可审计的参数束捕获", map[bool]string{true: "normal", false: "shutdown"}[normalTick])
	}
	if normalTick {
		for _, gate := range []string{"lifecycle", "paused"} {
			if position := function.gates[gate]; position == token.NoPos || position >= function.capture {
				t.Errorf("server.step 参数束捕获未位于 %s 早退之后", gate)
			}
		}
	}
	wantedCalls := []string{"drainIncomingChats", "StepWithTunables"}
	if normalTick {
		wantedCalls = append(wantedCalls, "advanceCompanionTasks")
	}
	for _, name := range wantedCalls {
		calls := function.calls[name]
		if len(calls) != 1 {
			t.Errorf("server.%s 中 %s 调用次数=%d，想要 1", function.name, name, len(calls))
			continue
		}
		if calls[0].position <= function.capture || calls[0].argument != function.captureVar {
			t.Errorf("server.%s 的 %s 未复用入口捕获参数束 %q", function.name, name, function.captureVar)
		}
	}
}

func TestTunableSourceGuardsCoverTuningPackage(t *testing.T) {
	packageDirectory := filepath.Join("packages", "shared", "tuning")
	if _, ok := tunableSourceGuardPackages()[packageDirectory]; !ok {
		t.Fatalf("可调参数源码守卫遗漏 %s", packageDirectory)
	}
}

func tunableSourceGuardPackages() map[string][]string {
	simTunables := []string{
		"RegenDelayTicks", "RegenIntervalTicks", "DropPickupDelayTicks",
		"PlayerDropPickupDelayTicks", "DropLifetimeTicks",
		"InteractionReach", "DropPickupRange", "SpawnRadius",
		"RandomTicksPerSection", "CropGrowthChancePercent",
		"StarvationDamageIntervalTicks", "ExhaustionThresholdMilli",
		"RegenHungerThreshold", "EatingTicks",
	}
	return map[string][]string{
		filepath.Join("packages", "shared", "physics"): {
			"EyeHeight", "StepHeight", "WalkSpeed", "GroundAcceleration",
			"GroundDeceleration", "AirAcceleration", "JumpSpeed", "Gravity",
			"TerminalFallSpeed", "FluidGravity", "FluidSinkSpeed",
			"FluidAscendSpeed", "FluidHorizontalDrag",
		},
		filepath.Join("packages", "shared", "tuning"): simTunables,
	}
}

// TestTunableConstantsAreNotExported 守住"可调参数只能经快照读取"这条不变量。
//
// 若某个可调参数同时以导出常量存在，任何一处漏改都会让编译期值与快照值并存：
// 例如相机读到编译期 EyeHeight、服务端射线读到快照值，玩家瞄准的方块与服务端
// 判定的方块就不是同一个，而且不会有任何报错。
func TestTunableConstantsAreNotExported(t *testing.T) {
	forbidden := tunableSourceGuardPackages()
	root := moduleRoot(t)
	for packageDirectory, names := range forbidden {
		files, err := filepath.Glob(filepath.Join(root, packageDirectory, "*.go"))
		if err != nil {
			t.Fatalf("枚举 %s: %v", packageDirectory, err)
		}
		// filepath.Glob 对不存在的目录返回 (nil, nil)：包一旦改名或移动，
		// 这条守卫就会静默变成空循环并永远通过，因此必须显式要求扫到文件。
		if len(files) == 0 {
			t.Fatalf("%s 下没有 Go 源文件：包被改名或移动后本守卫会静默失效", packageDirectory)
		}
		banned := make(map[string]bool, len(names))
		for _, name := range names {
			banned[name] = true
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("解析 %s: %v", path, err)
			}
			for _, declaration := range parsed.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) {
					continue
				}
				for _, specification := range general.Specs {
					value, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range value.Names {
						if banned[name.Name] {
							t.Errorf("%s: 可调参数 %s 仍以导出常量暴露，唯一入口必须是 Tunables 快照", path, name.Name)
						}
					}
				}
			}
		}
	}
}

// TestTunableDefaultsAreOnlyReadInTunablesFile 守住"可调参数的唯一读取入口是
// Tunables 快照"这条不变量的另一半。
//
// TestTunableConstantsAreNotExported 只看声明是否导出，对"某个生产读取点直接
// 读了未导出的 defaultXxx"完全失明——评审实测过：把 sim 调参包（现
// packages/shared/tuning）的 InteractionReach、DropLifetimeTicks、SpawnRadius、
// RegenDelay/IntervalTicks 与 packages/shared/physics 的 StepHeight 读取点逐一
// 改回 defaultXxx，既有测试全绿。
// 那种状态下配置文件与调试面板改不动这些参数，而且不会有任何报错，正是设计
// §3.4 要防的静默错位。
//
// 因此这里额外要求：除去常量声明本身与各包的 tunables.go（DefaultTunables 在
// 那里组装默认快照），任何非测试文件都不得再出现 defaultXxx 标识符。
func TestTunableDefaultsAreOnlyReadInTunablesFile(t *testing.T) {
	root := moduleRoot(t)
	for packageDirectory := range tunableSourceGuardPackages() {
		files, err := filepath.Glob(filepath.Join(root, packageDirectory, "*.go"))
		if err != nil {
			t.Fatalf("枚举 %s: %v", packageDirectory, err)
		}
		if len(files) == 0 {
			t.Fatalf("%s 下没有 Go 源文件：包被改名或移动后本守卫会静默失效", packageDirectory)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "tunables.go" {
				continue
			}
			fileSet := token.NewFileSet()
			parsed, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				t.Fatalf("解析 %s: %v", path, err)
			}
			declarationNames := make(map[*ast.Ident]bool)
			for _, declaration := range parsed.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, specification := range general.Specs {
					if value, ok := specification.(*ast.ValueSpec); ok {
						for _, name := range value.Names {
							declarationNames[name] = true
						}
					}
				}
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok || declarationNames[identifier] || !isTunableDefaultName(identifier.Name) {
					return true
				}
				t.Errorf("%s: 读取了编译期默认值 %s。可调参数的唯一读取入口必须是 Tunables 快照"+
					"（physics 用 Step 入口的快照，sim 用 engine.tunables）；直接读 default* 会让"+
					"该处永远停在编译期默认值，配置文件与调试面板改不动它，而且不会有任何报错。"+
					"default* 只允许出现在自己的声明处与 tunables.go 的 DefaultTunables 中。",
					fileSet.Position(identifier.Pos()), identifier.Name)
				return true
			})
		}
	}
}

// TestOnlyCommandsImportConfig 守住"自动化验证不读用户配置"这条不变量。
func TestOnlyCommandsImportConfig(t *testing.T) {
	// config 已迁入 packages/shared 模块：`./...` 不跨嵌套模块，须显式列出。
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \" \"}} {{join .TestImports \" \"}} {{join .XTestImports \" \"}}", "./internal/...", "./packages/shared/...")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		// config 的外部测试包（package config_test）导入自身，需要整体跳过，
		// 否则会自触发；这条豁免只对 config 包本身生效。
		if len(parts) != 2 || parts[0] == "github.com/channing771/mornlea/packages/shared/config" {
			continue
		}
		for _, imported := range strings.Fields(parts[1]) {
			if imported == "github.com/channing771/mornlea/packages/shared/config" {
				t.Errorf("%s 导入了 packages/shared/config；只有 cmd 可以导入它，否则本机配置会污染性能基线与抓帧 golden", parts[0])
			}
		}
	}
}

func TestOnlyTCPImplementationImportsNet(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "packages", "shared", "network")
	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatalf("枚举 network Go 文件: %v", err)
	}
	// 同 TestTunableConstantsAreNotExported：Glob 对不存在的目录静默返回空。
	if len(files) == 0 {
		t.Fatalf("%s 下没有 Go 源文件：包被改名或移动后本守卫会静默失效", root)
	}
	for _, path := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("解析 %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			if imported.Path.Value == `"net"` && filepath.Base(path) != "tcp.go" {
				t.Errorf("%s imports net; only tcp.go may import net", path)
			}
		}
	}
}

func TestLegacyPlayerAuthorityMessagesAreGone(t *testing.T) {
	root := moduleRoot(t)
	forbidden := map[string]struct{}{
		"SetViewCenter": {}, "BreakRay": {}, "PlaceRay": {}, "CommandBreakRay": {}, "CommandPlaceRay": {}, "localSessionID": {},
	}
	// packages/shared 是独立模块，legacy 标识符守卫须与其同侧扫描。
	for _, sourceRoot := range []string{"cmd", "internal", "packages/shared"} {
		err := filepath.WalkDir(filepath.Join(root, sourceRoot), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || path == filepath.Join(root, "internal", "archcheck", "source_guards_test.go") {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files := token.NewFileSet()
			var sourceScanner scanner.Scanner
			sourceScanner.Init(files.AddFile(path, -1, len(source)), source, nil, 0)
			for {
				position, kind, literal := sourceScanner.Scan()
				if kind == token.EOF {
					break
				}
				if kind == token.IDENT {
					if _, retired := forbidden[literal]; retired {
						t.Errorf("%s: legacy player authority identifier %s", files.Position(position), literal)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描 %s Go 源码: %v", sourceRoot, err)
		}
	}
}

func TestMornleaUsesLoginStreamsInsteadOfAttachedServerEndpoints(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "cmd", "mornlea")
	source := productionGoSource(t, path)
	for _, legacy := range []string{"server.NewEmbedded(", "server.NewEmbeddedMemory(", "server.New("} {
		if strings.Contains(source, legacy) {
			t.Errorf("%s retains legacy attached-server constructor %s", path, legacy)
		}
	}
	if !strings.Contains(source, "network.NewMemoryStreamPair") || !strings.Contains(source, "network.LoginClient") {
		t.Errorf("%s must assemble local connections through a stream login", path)
	}
}

func TestMornleaBenchmarkTCPPathUsesTheSharedLoginStateMachine(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "cmd", "mornlea")
	source := productionGoSource(t, path)
	for _, required := range []string{"networktcp.ListenTCP(", "network.BeginServerLogin(", "network.LoginClient(", "running.AttachTrustedObserver"} {
		if !strings.Contains(source, required) {
			t.Errorf("%s benchmark TCP path must contain %s", path, required)
		}
	}
}

func TestServerProductionDoesNotDeclareLegacyAttachedWorldWrappers(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "packages", "server", "server")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("读取 %s: %v", root, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(root, name)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			switch function.Name.Name {
			case "New", "NewMemory", "NewEmbedded", "NewEmbeddedMemory":
				t.Errorf("%s retains legacy server constructor %s", path, function.Name.Name)
			case "PlayerState":
				if function.Recv != nil && function.Type.Params.NumFields() == 0 {
					t.Errorf("%s retains legacy no-argument PlayerState", path)
				}
			}
		}
	}
}

func TestSessionLifecycleResponsibilitiesStayInSessionFiles(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "packages", "server", "server")
	sessionDeclarations := topLevelDeclarationNamesIn(t, root, "session*.go")
	serverDeclarations := topLevelDeclarationNamesIn(t, root, "server.go")
	wantSessionFile := []string{
		"inputCapacity", "trustedObserverSessionID", "SessionSpec", "SessionExit", "incomingCommand", "trustedObserverCenter", "appliedTrustedObserverCenter",
		"attachSessionLocked", "detachSessionLocked", "attachTrustedObserverLocked", "detachTrustedObserverLocked", "setTrustedObserverCenterLocked", "appliedTrustedObserverCenterLocked",
		"endpointReader", "translateClientMessage", "enqueueIncoming", "drainIncoming", "drainTrustedObserverCenter", "sortedSessionIDsLocked",
	}
	for _, name := range wantSessionFile {
		if !sessionDeclarations[name] {
			t.Errorf("session*.go must own session lifecycle declaration %s", name)
		}
		if serverDeclarations[name] {
			t.Errorf("server.go must not own session lifecycle declaration %s", name)
		}
	}
}

// TestCompanionPlannerProductionUsesAgentServiceOnly 锁定生产规划入口只经过 Agent
// service：旧 direct-model Planner 的类型、构造器与方法不可重新进入生产源码，
// Host 也不得重新装配 direct Planner 或旧 Dialogue client。
func TestCompanionPlannerProductionUsesAgentServiceOnly(t *testing.T) {
	root := moduleRoot(t)
	companionSource := productionGoSource(t, filepath.Join(root, "packages", "shared", "companion"))
	for _, forbidden := range []string{
		"type PlannerClient struct",
		"func NewPlannerClient(",
		"func (p *PlannerClient) Plan(",
		"type DialogueClient struct",
		"func NewDialogueClient(",
		"func (d *DialogueClient) Do(",
		"type DialogueRequest struct",
		"func NewDialogueRequest(",
		"func DecodeDialogueResponse(",
	} {
		if strings.Contains(companionSource, forbidden) {
			t.Errorf("packages/shared/companion production retains direct planner symbol %q", forbidden)
		}
	}

	serverSource := productionGoSource(t, filepath.Join(root, "packages", "server", "server"))
	for _, forbidden := range []string{
		"companion.NewPlannerClient(",
		"companion.NewDialogueClient(",
		"CompanionSummary",
		"slot.summary",
		"companionManagerSummaries(",
	} {
		if strings.Contains(serverSource, forbidden) {
			t.Errorf("packages/server/server production retains direct model construction %q", forbidden)
		}
	}
}
