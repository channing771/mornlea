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
	"strings"
	"testing"
)

type simAuthorityShape struct {
	structs map[string]map[string]string
	calls   map[string]int
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

	for name, mutate := range map[string]func(string, string) (string, string){
		"runtime 增加嵌套镜像 holder": func(runtimeSource, entitySource string) (string, string) {
			return strings.Replace(runtimeSource, "\tphysicsTunables physics.Tunables", "\tphysicsTunables physics.Tunables\n\tentityMirror struct{ sessions map[SessionID]int }", 1), entitySource
		},
		"runtime 复制 realm owner": func(runtimeSource, entitySource string) (string, string) {
			return strings.Replace(runtimeSource, "\trealm *realm.State", "\trealm *realm.State\n\tsecondRealm *realm.State", 1), entitySource
		},
		"subscription 混入玩家集合": func(runtimeSource, entitySource string) (string, string) {
			return strings.Replace(runtimeSource, "\twanted map[core.ChunkKey]struct{}", "\twanted map[core.ChunkKey]struct{}\n\tplayers map[SessionID]int", 1), entitySource
		},
		"entity 丢失 hostile owner": func(runtimeSource, entitySource string) (string, string) {
			return runtimeSource, strings.Replace(entitySource, "\thostiles hostileSet\n", "", 1)
		},
		"runtime 重复 mutation commit": func(runtimeSource, entitySource string) (string, string) {
			return strings.Replace(runtimeSource, "pending.Commit()", "pending.Commit(); pending.Commit()", 1), entitySource
		},
	} {
		t.Run(name, func(t *testing.T) {
			runtimeSource, entitySource := mutate(goodRuntime, goodEntity)
			if violations := simAuthorityViolationsFromSources(runtimeSource, entitySource); len(violations) == 0 {
				t.Fatal("所有权漂移未被拒绝")
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
	return shape
}

func formatSimAuthorityNode(files *token.FileSet, node ast.Node) string {
	var output bytes.Buffer
	if err := format.Node(&output, files, node); err != nil {
		return "<format error>"
	}
	return output.String()
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
	for call, want := range map[string]int{"NewMutation": 1, "finishRealmMutation": 1, "Commit": 1} {
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
