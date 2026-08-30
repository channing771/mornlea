package archcheck_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type simAuthorityShape struct {
	structs       map[string]map[string]string
	calls         map[string]int
	storageDrift  []string
	mutationDrift []string
}

var expectedRuntimeEngineFields = map[string]string{
	"viewRadius":         "int",
	"seed":               "int64",
	"subscriptions":      "map[SessionID]*subscriptionState",
	"wanted":             "map[core.ChunkKey]struct{}",
	"realm":              "*realm.State",
	"entities":           "*entity.State",
	"subscriptionsDirty": "bool",
	"entityViewScratch":  "[]entity.TickSessionView",
	"activeChunkScratch": "[]core.ChunkKey",
	"inboxMu":            "sync.Mutex",
	"commands":           "[]Command",
	"companionActions":   "[]CompanionAction",
	"hostileActions":     "[]HostileAction",
	"acquired":           "[]AcquiredChunk",
	"generated":          "[]GeneratedChunk",
	"tick":               "atomic.Uint64",
	"worldTime":          "atomic.Uint64",
	"dayPhaseOffset":     "atomic.Uint64",
	"stepPhaseObserver":  "func(stepPhase)",
	"tunables":           "tuning.Tunables",
	"physicsTunables":    "physics.Tunables",
}

var expectedRuntimeSubscriptionFields = map[string]string{
	"lastSequence":                "uint64",
	"lastTrustedObserverSequence": "uint64",
	"trustedObserver":             "bool",
	"hasView":                     "bool",
	"dimension":                   "core.DimensionID",
	"center":                      "core.ChunkPos",
	"wanted":                      "map[core.ChunkKey]struct{}",
}

var requiredEntityStateFields = map[string]string{
	"sessions":   "map[SessionID]*sessionState",
	"companions": "map[companion.ID]*companionState",
	"hostiles":   "hostileSet",
}

var requiredEntitySessionFields = map[string]string{
	"player":    "*playerState",
	"container": "core.ContainerRef",
}

var simAuthorityMirrorNames = map[string]bool{
	"sessions":               true,
	"player":                 true,
	"players":                true,
	"companions":             true,
	"hostiles":               true,
	"inventory":              true,
	"inventories":            true,
	"container":              true,
	"containers":             true,
	"combat":                 true,
	"drop":                   true,
	"drops":                  true,
	"lifecycle":              true,
	"lifecycles":             true,
	"viewcontainer":          true,
	"hostilelight":           true,
	"tramplepending":         true,
	"dropkeyseen":            true,
	"dropkeyscratch":         true,
	"containerviewerscratch": true,
	"dropsessionscratch":     true,
}

func TestSimAuthorityStateOwnershipStaysExplicit(t *testing.T) {
	violations, err := simAuthorityViolationsFromTree(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("权威模拟所有权漂移：%s", strings.Join(violations, "; "))
	}
}

func TestSimAuthorityGuardRejectsSyntheticDrift(t *testing.T) {
	goodRuntime := `package runtime
import (
	"sync"
	"sync/atomic"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/entity"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
)
type SessionID uint64
type Command struct{}
type CompanionAction struct{}
type HostileAction struct{}
type AcquiredChunk struct{}
type GeneratedChunk struct{}
type stepPhase uint8
var authorityDiagnostic = "realm.State"
type subscriptionState struct {
	lastSequence uint64
	lastTrustedObserverSequence uint64
	trustedObserver bool
	hasView bool
	dimension core.DimensionID
	center core.ChunkPos
	wanted map[core.ChunkKey]struct{}
}
type Engine struct {
	viewRadius int
	seed int64
	subscriptions map[SessionID]*subscriptionState
	wanted map[core.ChunkKey]struct{}
	realm *realm.State
	entities *entity.State
	subscriptionsDirty bool
	entityViewScratch []entity.TickSessionView
	activeChunkScratch []core.ChunkKey
	inboxMu sync.Mutex
	commands []Command
	companionActions []CompanionAction
	hostileActions []HostileAction
	acquired []AcquiredChunk
	generated []GeneratedChunk
	tick atomic.Uint64
	worldTime atomic.Uint64
	dayPhaseOffset atomic.Uint64
	stepPhaseObserver func(stepPhase)
	tunables tuning.Tunables
	physicsTunables physics.Tunables
}
func (engine *Engine) StepWithTunables() { pending := engine.realm.NewMutation(); finishRealmMutation(pending) }
func finishRealmMutation(pending *realm.Mutation) { pending.Commit() }
`
	goodEntity := `package entity
import (
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
)
type SessionID uint64
type playerState struct{}
type companionState struct{}
type hostileSet struct{}
type sessionState struct {
	id SessionID
	dimension core.DimensionID
	player *playerState
	container core.ContainerRef
	viewContainer bool
}
type State struct {
	sessions map[SessionID]*sessionState
	companions map[companion.ID]*companionState
	hostiles hostileSet
}
`

	if violations := simAuthorityViolationsFromSources(goodRuntime, goodEntity); len(violations) != 0 {
		t.Fatalf("合法窄所有权被误拒绝：%s", strings.Join(violations, "; "))
	}

	tests := []struct {
		name   string
		mutate func(string, string) (string, string)
		wants  []string
	}{
		{
			name: "runtime 增加包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraRealm *realm.State\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraRealm 保存额外 realm.State owner"},
		},
		{
			name: "runtime 增加包级实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var sessions map[SessionID]int\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable sessions 复制 entity 权威状态"},
		},
		{
			name: "runtime 增加独立命名 owner holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type authorityHolder struct { entities *entity.State }\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime.authorityHolder.entities 保存额外 entity.State owner"},
		},
		{
			name: "runtime 用 import alias 增加 owner holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				runtimeSource = strings.Replace(runtimeSource,
					"\"github.com/channing771/mornlea/internal/sim/entity\"",
					"ent \"github.com/channing771/mornlea/internal/sim/entity\"",
					1,
				)
				runtimeSource = strings.ReplaceAll(runtimeSource, "entity.", "ent.")
				return strings.Replace(runtimeSource, "type Engine struct {", "type aliasHolder struct { owner *ent.State }\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime.aliasHolder.owner 保存额外 entity.State owner"},
		},
		{
			name: "runtime 用类型别名隐藏 owner holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type entityOwner = *entity.State\ntype aliasHolder struct { owner entityOwner }\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime type entityOwner 保存额外 entity.State owner"},
		},
		{
			name: "runtime 增加独立命名实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type authorityMirror struct { companions map[uint64]int }\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime.authorityMirror.companions 复制 entity 权威状态"},
		},
		{
			name: "runtime 增加匿名包级 owner holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var authorityHolder = struct { entities *entity.State }{}\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable authorityHolder 保存额外 entity.State owner"},
		},
		{
			name: "runtime 增加嵌套匿名实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type authorityHolder struct { nested struct { hostiles map[uint64]int } }\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime anonymous holder", ".hostiles 复制 entity 权威状态"},
		},
		{
			name: "runtime 复制 realm owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "\trealm *realm.State", "\trealm *realm.State\n\tsecondRealm *realm.State", 1), entitySource
			},
			wants: []string{"runtime.Engine.secondRealm 保存额外 realm.State owner"},
		},
		{
			name: "subscription 混入玩家集合",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "\twanted map[core.ChunkKey]struct{}", "\twanted map[core.ChunkKey]struct{}\n\tplayers map[SessionID]int", 1), entitySource
			},
			wants: []string{"runtime.subscriptionState.players 复制 entity 权威状态"},
		},
		{
			name: "entity 丢失 hostile owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return runtimeSource, strings.Replace(entitySource, "\thostiles hostileSet\n", "", 1)
			},
			wants: []string{"entity.State 缺少字段 hostiles hostileSet"},
		},
		{
			name: "runtime 重复 mutation commit",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "pending.Commit(); pending.Commit()", 1), entitySource
			},
			wants: []string{"局部 mutation \"pending\" 的可达 Commit 次数=2"},
		},
		{
			name: "runtime 内层重绑 mutation 局部名",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				runtimeSource = strings.Replace(runtimeSource,
					"func (engine *Engine) StepWithTunables() { pending := engine.realm.NewMutation(); finishRealmMutation(pending) }",
					"func (engine *Engine) StepWithTunables() { pending := engine.realm.NewMutation(); { var pending *realm.Mutation; pending.Commit() }; _ = pending }",
					1,
				)
				return strings.Replace(runtimeSource, "func finishRealmMutation(pending *realm.Mutation) { pending.Commit() }", "func finishRealmMutation(pending *realm.Mutation) { _ = pending }", 1), entitySource
			},
			wants: []string{"局部 mutation \"pending\" 被声明或赋值 2 次"},
		},
		{
			name: "runtime range 重绑 mutation 局部名",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				runtimeSource = strings.Replace(runtimeSource,
					"func (engine *Engine) StepWithTunables() { pending := engine.realm.NewMutation(); finishRealmMutation(pending) }",
					"func (engine *Engine) StepWithTunables() { pending := engine.realm.NewMutation(); mutationCh := make(chan *realm.Mutation); for pending := range mutationCh { pending.Commit(); break }; _ = pending }",
					1,
				)
				return strings.Replace(runtimeSource, "func finishRealmMutation(pending *realm.Mutation) { pending.Commit() }", "func finishRealmMutation(pending *realm.Mutation) { _ = pending }", 1), entitySource
			},
			wants: []string{"局部 mutation \"pending\" 被声明或赋值 2 次"},
		},
		{
			name: "runtime 把 mutation 搬到不可达 helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"func (engine *Engine) StepWithTunables() { pending := engine.realm.NewMutation(); finishRealmMutation(pending) }",
					"func (engine *Engine) StepWithTunables() {}\nfunc deadTick(engine *Engine) { pending := engine.realm.NewMutation(); finishRealmMutation(pending) }",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables 直接创建 realm mutation 次数=0"},
		},
		{
			name: "runtime 提交不同 mutation 局部值",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); var other *realm.Mutation; finishRealmMutation(other); _ = pending",
					1,
				), entitySource
			},
			wants: []string{"局部 mutation \"pending\" 的可达 Commit 次数=0"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runtimeSource, entitySource := testCase.mutate(goodRuntime, goodEntity)
			violations := simAuthorityViolationsFromSources(runtimeSource, entitySource)
			joined := strings.Join(violations, "; ")
			if strings.Contains(joined, "解析 synthetic") {
				t.Fatalf("synthetic 无法解析：%s", joined)
			}
			for _, want := range testCase.wants {
				if !strings.Contains(joined, want) {
					t.Fatalf("未命中预期违规 %q：%s", want, joined)
				}
			}
		})
	}
}

func simAuthorityViolationsFromTree(root string) ([]string, error) {
	runtimeShape, err := readSimAuthorityPackage(filepath.Join(root, "internal", "sim", "runtime"), "runtime")
	if err != nil {
		return nil, fmt.Errorf("读取 runtime 所有权结构：%w", err)
	}
	entityShape, err := readSimAuthorityPackage(filepath.Join(root, "internal", "sim", "entity"), "entity")
	if err != nil {
		return nil, fmt.Errorf("读取 entity 所有权结构：%w", err)
	}
	return simAuthorityViolations(runtimeShape, entityShape), nil
}

func simAuthorityViolationsFromSources(runtimeSource, entitySource string) []string {
	runtimeShape, err := parseSimAuthoritySource("runtime.go", runtimeSource)
	if err != nil {
		return []string{"解析 synthetic runtime：" + err.Error()}
	}
	entityShape, err := parseSimAuthoritySource("entity.go", entitySource)
	if err != nil {
		return []string{"解析 synthetic entity：" + err.Error()}
	}
	return simAuthorityViolations(runtimeShape, entityShape)
}

func readSimAuthorityPackage(directory, packageName string) (simAuthorityShape, error) {
	files := token.NewFileSet()
	packages, err := parser.ParseDir(files, directory, func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		return simAuthorityShape{}, err
	}
	parsed, ok := packages[packageName]
	if !ok || len(parsed.Files) == 0 {
		return simAuthorityShape{}, fmt.Errorf("未找到 production package %s", packageName)
	}
	packageFiles := make([]*ast.File, 0, len(parsed.Files))
	for _, file := range parsed.Files {
		packageFiles = append(packageFiles, file)
	}
	return collectSimAuthorityShape(files, packageFiles), nil
}

func parseSimAuthoritySource(name, source string) (simAuthorityShape, error) {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, name, source, parser.SkipObjectResolution)
	if err != nil {
		return simAuthorityShape{}, err
	}
	return collectSimAuthorityShape(files, []*ast.File{parsed}), nil
}

func collectSimAuthorityShape(files *token.FileSet, parsed []*ast.File) simAuthorityShape {
	shape := simAuthorityShape{
		structs: make(map[string]map[string]string),
		calls:   make(map[string]int),
	}
	for _, file := range parsed {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				fields := make(map[string]string)
				for _, field := range structure.Fields.List {
					fieldType := formatSimAuthorityNode(files, field.Type)
					if len(field.Names) == 0 {
						fields["<embedded "+fieldType+">"] = fieldType
						continue
					}
					for _, name := range field.Names {
						fields[name.Name] = fieldType
					}
				}
				shape.structs[typeSpec.Name.Name] = fields
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch function := call.Fun.(type) {
			case *ast.Ident:
				if function.Name == "finishRealmMutation" {
					shape.calls[function.Name]++
				}
			case *ast.SelectorExpr:
				if function.Sel.Name == "NewMutation" || function.Sel.Name == "Commit" {
					shape.calls[function.Sel.Name]++
				}
			}
			return true
		})
	}
	if len(parsed) != 0 && parsed[0].Name.Name == "runtime" {
		shape.storageDrift = simAuthorityRuntimeStorageDrift(files, parsed)
		shape.mutationDrift = simAuthorityRuntimeMutationDrift(files, parsed)
	}
	return shape
}

func formatSimAuthorityNode(files *token.FileSet, node ast.Node) string {
	var output bytes.Buffer
	if err := format.Node(&output, files, node); err != nil {
		return "<format error>"
	}
	return output.String()
}

func simAuthorityRuntimeStorageDrift(files *token.FileSet, parsed []*ast.File) []string {
	namedStructs := make(map[token.Pos]string)
	for _, file := range parsed {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				if structure, ok := typeSpec.Type.(*ast.StructType); ok {
					namedStructs[structure.Pos()] = "runtime." + typeSpec.Name.Name
				}
			}
		}
	}

	violations := make([]string, 0)
	for _, file := range parsed {
		ownerAliases := simAuthorityOwnerAliases(file)
		if ownerAliases["."] != "" {
			violations = append(violations, "runtime 不得以 dot import 引入 "+ownerAliases["."]+" owner")
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			if general.Tok == token.TYPE {
				for _, specification := range general.Specs {
					typeSpec := specification.(*ast.TypeSpec)
					if _, structure := typeSpec.Type.(*ast.StructType); !structure {
						violations = append(violations, simAuthorityStoredOwnerDrift(
							"runtime type "+typeSpec.Name.Name,
							formatSimAuthorityNode(files, typeSpec.Type),
							ownerAliases,
						)...)
					}
				}
				continue
			}
			if general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value := specification.(*ast.ValueSpec)
				for index, name := range value.Names {
					location := "runtime package variable " + name.Name
					if simAuthorityMirrorNames[strings.ToLower(name.Name)] {
						violations = append(violations, location+" 复制 entity 权威状态")
					}
					if value.Type != nil {
						violations = append(violations, simAuthorityStoredOwnerDrift(
							location, formatSimAuthorityNode(files, value.Type), ownerAliases,
						)...)
					}
					if index < len(value.Values) {
						violations = append(violations, simAuthorityStoredValueOwnerDrift(
							location, value.Values[index], files, ownerAliases,
						)...)
					} else if len(value.Values) == 1 {
						violations = append(violations, simAuthorityStoredValueOwnerDrift(
							location, value.Values[0], files, ownerAliases,
						)...)
					}
				}
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			structure, ok := node.(*ast.StructType)
			if !ok {
				return true
			}
			location := namedStructs[structure.Pos()]
			if location == "" {
				location = "runtime anonymous holder at " + files.Position(structure.Pos()).String()
			}
			for _, field := range structure.Fields.List {
				fieldType := formatSimAuthorityNode(files, field.Type)
				fieldNames := field.Names
				if len(fieldNames) == 0 {
					fieldNames = []*ast.Ident{{Name: "<embedded>"}}
				}
				for _, name := range fieldNames {
					fieldLocation := location + "." + name.Name
					if simAuthorityMirrorNames[strings.ToLower(name.Name)] {
						violations = append(violations, fieldLocation+" 复制 entity 权威状态")
					}
					allowed := location == "runtime.Engine" &&
						((name.Name == "realm" && fieldType == "*realm.State") ||
							(name.Name == "entities" && fieldType == "*entity.State"))
					if !allowed {
						violations = append(violations, simAuthorityStoredOwnerDrift(fieldLocation, fieldType, ownerAliases)...)
					}
				}
			}
			return true
		})
	}
	return violations
}

func simAuthorityStoredOwnerDrift(location, source string, ownerAliases map[string]string) []string {
	violations := make([]string, 0, 2)
	for alias, owner := range ownerAliases {
		if alias != "." && strings.Contains(source, alias+".State") {
			violations = append(violations, location+" 保存额外 "+owner+".State owner")
		}
	}
	return violations
}

func simAuthorityStoredValueOwnerDrift(
	location string,
	expression ast.Expr,
	files *token.FileSet,
	ownerAliases map[string]string,
) []string {
	switch current := expression.(type) {
	case *ast.ParenExpr:
		return simAuthorityStoredValueOwnerDrift(location, current.X, files, ownerAliases)
	case *ast.UnaryExpr:
		return simAuthorityStoredValueOwnerDrift(location, current.X, files, ownerAliases)
	case *ast.CompositeLit:
		return simAuthorityStoredOwnerDrift(location, formatSimAuthorityNode(files, current.Type), ownerAliases)
	case *ast.CallExpr:
		if identifier, ok := current.Fun.(*ast.Ident); ok && identifier.Name == "new" && len(current.Args) == 1 {
			return simAuthorityStoredOwnerDrift(location, formatSimAuthorityNode(files, current.Args[0]), ownerAliases)
		}
		if selector, ok := current.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "NewState" {
			if packageName, ok := selector.X.(*ast.Ident); ok {
				if owner := ownerAliases[packageName.Name]; owner != "" {
					return []string{location + " 保存额外 " + owner + ".State owner"}
				}
			}
		}
		return simAuthorityStoredOwnerDrift(location, formatSimAuthorityNode(files, current.Fun), ownerAliases)
	}
	return nil
}

func simAuthorityOwnerAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		owner := ""
		switch importPath {
		case "github.com/channing771/mornlea/internal/sim/realm":
			owner = "realm"
		case "github.com/channing771/mornlea/internal/sim/entity":
			owner = "entity"
		}
		if owner == "" {
			continue
		}
		alias := filepath.Base(importPath)
		if specification.Name != nil {
			alias = specification.Name.Name
		}
		aliases[alias] = owner
	}
	return aliases
}

func simAuthorityRuntimeMutationDrift(files *token.FileSet, parsed []*ast.File) []string {
	functions := make(map[string]*ast.FuncDecl)
	steps := make([]*ast.FuncDecl, 0, 1)
	for _, file := range parsed {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if function.Recv == nil {
				functions[function.Name.Name] = function
				continue
			}
			if function.Name.Name == "StepWithTunables" && simAuthorityEngineReceiver(function) != "" {
				steps = append(steps, function)
			}
		}
	}
	if len(steps) != 1 {
		return []string{fmt.Sprintf("runtime 的 (*Engine).StepWithTunables 方法数量=%d，想要 1", len(steps))}
	}

	step := steps[0]
	receiver := simAuthorityEngineReceiver(step)
	locals := make([]string, 0, 1)
	simAuthorityInspectFunctionBody(step.Body, func(node ast.Node) {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return
		}
		name, nameOK := assignment.Lhs[0].(*ast.Ident)
		call, callOK := assignment.Rhs[0].(*ast.CallExpr)
		if nameOK && callOK && simAuthorityEngineNewMutation(call, receiver) {
			locals = append(locals, name.Name)
		}
	})
	if len(locals) != 1 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 直接创建 realm mutation 次数=%d，想要 1",
			files.Position(step.Pos()), len(locals),
		)}
	}
	if writes := simAuthorityTrackedWrites(step.Body, locals[0]); writes != 1 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 局部 mutation %q 被声明或赋值 %d 次，想要仅创建时 1 次",
			files.Position(step.Pos()), locals[0], writes,
		)}
	}

	commits := simAuthorityDirectCommitCount(step.Body, locals[0])
	rebound := ""
	simAuthorityInspectFunctionBody(step.Body, func(node ast.Node) {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || functions[name.Name] == nil {
			return
		}
		for index, argument := range call.Args {
			identifier, ok := argument.(*ast.Ident)
			if !ok || identifier.Name != locals[0] {
				continue
			}
			if parameter := simAuthorityParameterAt(functions[name.Name], index); parameter != "" {
				if writes := simAuthorityTrackedWrites(functions[name.Name].Body, parameter); writes != 0 {
					rebound = fmt.Sprintf("helper %s 的 mutation 参数 %q 被重新声明或赋值 %d 次", name.Name, parameter, writes)
					continue
				}
				commits += simAuthorityDirectCommitCount(functions[name.Name].Body, parameter)
			}
		}
	})
	if rebound != "" {
		return []string{fmt.Sprintf("%s：(*Engine).StepWithTunables %s", files.Position(step.Pos()), rebound)}
	}
	if commits != 1 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 局部 mutation %q 的可达 Commit 次数=%d，想要 1",
			files.Position(step.Pos()), locals[0], commits,
		)}
	}
	return nil
}

func simAuthorityEngineReceiver(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return ""
	}
	star, ok := function.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return ""
	}
	identifier, ok := star.X.(*ast.Ident)
	if !ok || identifier.Name != "Engine" {
		return ""
	}
	return function.Recv.List[0].Names[0].Name
}

func simAuthorityEngineNewMutation(call *ast.CallExpr, receiver string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "NewMutation" || len(call.Args) != 0 {
		return false
	}
	realm, ok := selector.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := realm.X.(*ast.Ident)
	return ok && realm.Sel.Name == "realm" && identifier.Name == receiver
}

func simAuthorityDirectCommitCount(body *ast.BlockStmt, local string) int {
	commits := 0
	simAuthorityInspectFunctionBody(body, func(node ast.Node) {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && selector.Sel.Name == "Commit" && identifier.Name == local {
			commits++
		}
	})
	return commits
}

func simAuthorityTrackedWrites(body *ast.BlockStmt, local string) int {
	writes := 0
	simAuthorityInspectFunctionBody(body, func(node ast.Node) {
		switch current := node.(type) {
		case *ast.AssignStmt:
			for _, expression := range current.Lhs {
				if identifier, ok := expression.(*ast.Ident); ok && identifier.Name == local {
					writes++
				}
			}
		case *ast.ValueSpec:
			for _, name := range current.Names {
				if name.Name == local {
					writes++
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{current.Key, current.Value} {
				if identifier, ok := expression.(*ast.Ident); ok && identifier.Name == local {
					writes++
				}
			}
		}
	})
	return writes
}

func simAuthorityParameterAt(function *ast.FuncDecl, index int) string {
	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			if index == 0 {
				return name.Name
			}
			index--
		}
		if len(field.Names) == 0 {
			if index == 0 {
				return ""
			}
			index--
		}
	}
	return ""
}

func simAuthorityInspectFunctionBody(body *ast.BlockStmt, visit func(ast.Node)) {
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		visit(node)
		return true
	})
}

func simAuthorityViolations(runtimeShape, entityShape simAuthorityShape) []string {
	violations := make([]string, 0)
	violations = append(violations, exactSimAuthorityFields(
		"runtime.Engine", runtimeShape.structs["Engine"], expectedRuntimeEngineFields,
	)...)
	violations = append(violations, exactSimAuthorityFields(
		"runtime.subscriptionState", runtimeShape.structs["subscriptionState"], expectedRuntimeSubscriptionFields,
	)...)
	violations = append(violations, requiredSimAuthorityFields(
		"entity.State", entityShape.structs["State"], requiredEntityStateFields,
	)...)
	violations = append(violations, requiredSimAuthorityFields(
		"entity.sessionState", entityShape.structs["sessionState"], requiredEntitySessionFields,
	)...)
	violations = append(violations, runtimeShape.storageDrift...)
	violations = append(violations, runtimeShape.mutationDrift...)

	owners := map[string]int{"*realm.State": 0, "*entity.State": 0}
	for _, fieldType := range runtimeShape.structs["Engine"] {
		if _, tracked := owners[fieldType]; tracked {
			owners[fieldType]++
		}
	}
	for ownerType, count := range owners {
		if count != 1 {
			violations = append(violations, fmt.Sprintf("runtime.Engine 的 %s owner 数量=%d，想要 1", ownerType, count))
		}
	}
	for call, want := range map[string]int{"NewMutation": 1, "Commit": 1} {
		if got := runtimeShape.calls[call]; got != want {
			violations = append(violations, fmt.Sprintf("runtime production 的 %s 调用数=%d，想要 %d", call, got, want))
		}
	}
	sort.Strings(violations)
	return violations
}

func exactSimAuthorityFields(owner string, got, want map[string]string) []string {
	violations := requiredSimAuthorityFields(owner, got, want)
	for name, fieldType := range got {
		if _, allowed := want[name]; !allowed {
			violations = append(violations, fmt.Sprintf("%s 出现未评审字段 %s %s", owner, name, fieldType))
		}
	}
	return violations
}

func requiredSimAuthorityFields(owner string, got, want map[string]string) []string {
	violations := make([]string, 0)
	if got == nil {
		return []string{owner + " 不存在"}
	}
	for name, fieldType := range want {
		actual, exists := got[name]
		switch {
		case !exists:
			violations = append(violations, fmt.Sprintf("%s 缺少字段 %s %s", owner, name, fieldType))
		case actual != fieldType:
			violations = append(violations, fmt.Sprintf("%s 字段 %s 类型=%s，想要 %s", owner, name, actual, fieldType))
		}
	}
	return violations
}
