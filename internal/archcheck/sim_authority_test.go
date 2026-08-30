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
func mutateAsync(*realm.Mutation) {}
func mutateLater(*realm.Mutation) {}
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
		clean  bool
	}{
		{
			name: "runtime 安全标量 holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type safeScalarHolder struct { player SessionID }\nvar safeScalar safeScalarHolder\ntype Engine struct {", 1), entitySource
			},
			clean: true,
		},
		{
			name: "runtime 无关嵌套绑定不遮蔽顶层 helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"{ finishRealmMutation := 1; _ = finishRealmMutation }; pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			clean: true,
		},
		{
			name: "runtime 增加包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraRealm *realm.State\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraRealm 保存额外 realm.State owner"},
		},
		{
			name: "runtime 经 helper 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "func newRealmOwner() *realm.State { return realm.NewState(core.Overworld) }\nvar extraRealm = newRealmOwner()\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraRealm 保存额外 realm.State owner"},
		},
		{
			name: "runtime 经 IIFE 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraRealm = func() *realm.State { return realm.NewState(core.Overworld) }()\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraRealm 保存额外 realm.State owner"},
		},
		{
			name: "runtime 经括号 IIFE 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraRealm = (func() *realm.State { return realm.NewState(core.Overworld) })()\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraRealm 保存额外 realm.State owner"},
		},
		{
			name: "runtime 未解析 method 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type ownerFactory struct{}\nfunc (ownerFactory) Make() *realm.State { return realm.NewState(core.Overworld) }\nvar extraRealm = ownerFactory{}.Make()\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraRealm 保存递归 mutable state"},
		},
		{
			name: "runtime 增加包级实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var sessions map[SessionID]int\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable sessions 保存递归 mutable state"},
		},
		{
			name: "runtime 中性字段嵌套包级实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type playerSnapshot struct{}\ntype hiddenMirror struct { nested struct { byID map[SessionID]playerSnapshot } }\nvar hidden hiddenMirror\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable hidden 保存递归 mutable state"},
		},
		{
			name: "runtime 固定数组包级实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type playerSnapshot struct{}\nvar hidden [2]playerSnapshot\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable hidden 保存递归 mutable state"},
		},
		{
			name: "runtime opaque 包级实体镜像",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var hidden sync.Map\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable hidden 保存递归 mutable state"},
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
				return strings.Replace(runtimeSource, "type Engine struct {", "type authorityMirror struct { companions map[uint64]int }\nvar mirror authorityMirror\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable mirror 保存递归 mutable state"},
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
				return strings.Replace(runtimeSource, "type Engine struct {", "type authorityHolder struct { nested struct { hostiles map[uint64]int } }\nvar mirror authorityHolder\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable mirror 保存递归 mutable state"},
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
			wants: []string{"runtime.subscriptionState 出现未评审字段 players"},
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
			wants: []string{"helper \"finishRealmMutation\" 对 mutation 参数 \"pending\" 的确定性 Commit 次数=2"},
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
			name: "runtime 局部函数遮蔽 mutation helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); finishRealmMutation := func(*realm.Mutation) {}; finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables 局部绑定遮蔽顶层 helper \"finishRealmMutation\""},
		},
		{
			name: "runtime mutation 创建后提前返回",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); if true { return }; finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables 在 mutation 创建或 helper 调用前存在提前退出"},
		},
		{
			name: "runtime mutation 创建前提前返回",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"if true { return }; pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables 在 mutation 创建或 helper 调用前存在提前退出"},
		},
		{
			name: "runtime mutation 创建后 goto 跳过提交",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); goto afterCommit; finishRealmMutation(pending); afterCommit: _ = pending",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables 在 mutation 创建或 helper 调用前存在提前退出"},
		},
		{
			name: "runtime mutation 被其他 goroutine 使用",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); go mutateAsync(pending); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables mutation 生命周期不得包含 go"},
		},
		{
			name: "runtime mutation alias 被其他 goroutine 使用",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); alias := pending; go mutateAsync(alias); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables mutation 生命周期不得包含 go"},
		},
		{
			name: "runtime mutation 被 defer 使用",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); defer mutateLater(pending); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables mutation 生命周期不得包含 defer"},
		},
		{
			name: "runtime 条件调用 mutation helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "finishRealmMutation(pending)", "if true { finishRealmMutation(pending) }", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 调用位于条件控制流"},
		},
		{
			name: "runtime 循环调用 mutation helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "finishRealmMutation(pending)", "for range 2 { finishRealmMutation(pending) }", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 调用位于循环"},
		},
		{
			name: "runtime go 调用 mutation helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "finishRealmMutation(pending)", "go finishRealmMutation(pending)", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 调用不得使用 go"},
		},
		{
			name: "runtime defer 调用 mutation helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "finishRealmMutation(pending)", "defer finishRealmMutation(pending)", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 调用不得使用 defer"},
		},
		{
			name: "runtime helper 提交前提前返回",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "if true { return }; pending.Commit()", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 在 Commit 前存在提前退出"},
		},
		{
			name: "runtime helper 条件提交 mutation",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "if true { pending.Commit() }", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 的 Commit 调用位于条件控制流"},
		},
		{
			name: "runtime helper 循环提交 mutation",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "for range 2 { pending.Commit() }", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 的 Commit 调用位于循环"},
		},
		{
			name: "runtime helper 异步提交 mutation",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "go pending.Commit()", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 的 Commit 调用不得使用 go"},
		},
		{
			name: "runtime helper 延迟提交 mutation",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "defer pending.Commit()", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 的 Commit 调用不得使用 defer"},
		},
		{
			name: "runtime helper 延迟旁路使用 mutation",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "defer mutateLater(pending); pending.Commit()", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" mutation 生命周期不得包含 defer"},
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
			wants: []string{"使用 mutation local \"pending\" 的顶层 helper 调用次数=0"},
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
			if testCase.clean {
				if joined != "" {
					t.Fatalf("安全形状被误拒绝：%s", joined)
				}
				return
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

type simAuthorityTypeDeclaration struct {
	expression ast.Expr
	aliases    map[string]string
}

type simAuthorityFunctionDeclaration struct {
	function *ast.FuncDecl
	aliases  map[string]string
}

type simAuthorityStorageIndex struct {
	types     map[string]simAuthorityTypeDeclaration
	functions map[string]simAuthorityFunctionDeclaration
}

type simAuthorityStorageTraits struct {
	owners  map[string]bool
	mutable bool
}

func simAuthorityRuntimeStorageDrift(files *token.FileSet, parsed []*ast.File) []string {
	index := simAuthorityBuildStorageIndex(parsed)
	namedStructs := make(map[token.Pos]string)
	for name, declaration := range index.types {
		if structure, ok := declaration.expression.(*ast.StructType); ok {
			namedStructs[structure.Pos()] = "runtime." + name
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
					if _, structure := typeSpec.Type.(*ast.StructType); structure {
						continue
					}
					traits := simAuthorityTypeStorageTraits(typeSpec.Type, ownerAliases, index, make(map[string]bool))
					violations = append(violations, simAuthorityOwnerTraitViolations(
						"runtime type "+typeSpec.Name.Name, traits,
					)...)
				}
				continue
			}
			if general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value := specification.(*ast.ValueSpec)
				for valueIndex, name := range value.Names {
					traits := simAuthorityStorageTraits{owners: make(map[string]bool)}
					if value.Type != nil {
						traits.merge(simAuthorityTypeStorageTraits(
							value.Type, ownerAliases, index, make(map[string]bool),
						))
					}
					if initializer := simAuthorityValueInitializer(value, valueIndex); initializer != nil {
						traits.merge(simAuthorityValueStorageTraits(
							initializer, ownerAliases, index, make(map[string]bool),
						))
					}
					location := "runtime package variable " + name.Name
					ownerViolations := simAuthorityOwnerTraitViolations(location, traits)
					violations = append(violations, ownerViolations...)
					if len(ownerViolations) == 0 && traits.mutable {
						violations = append(violations, location+" 保存递归 mutable state")
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
				fieldNames := field.Names
				if len(fieldNames) == 0 {
					fieldNames = []*ast.Ident{{Name: "<embedded>"}}
				}
				traits := simAuthorityTypeStorageTraits(field.Type, ownerAliases, index, make(map[string]bool))
				for _, name := range fieldNames {
					allowedOwner := simAuthorityAllowedEngineOwner(location, name.Name, field.Type, ownerAliases)
					for owner := range traits.owners {
						if owner != allowedOwner {
							violations = append(violations, location+"."+name.Name+" 保存额外 "+owner+".State owner")
						}
					}
				}
			}
			return true
		})
	}
	return violations
}

func simAuthorityBuildStorageIndex(parsed []*ast.File) simAuthorityStorageIndex {
	index := simAuthorityStorageIndex{
		types:     make(map[string]simAuthorityTypeDeclaration),
		functions: make(map[string]simAuthorityFunctionDeclaration),
	}
	for _, file := range parsed {
		aliases := simAuthorityOwnerAliases(file)
		for _, declaration := range file.Decls {
			switch current := declaration.(type) {
			case *ast.GenDecl:
				if current.Tok != token.TYPE {
					continue
				}
				for _, specification := range current.Specs {
					typeSpec := specification.(*ast.TypeSpec)
					index.types[typeSpec.Name.Name] = simAuthorityTypeDeclaration{
						expression: typeSpec.Type,
						aliases:    aliases,
					}
				}
			case *ast.FuncDecl:
				if current.Recv == nil {
					index.functions[current.Name.Name] = simAuthorityFunctionDeclaration{
						function: current,
						aliases:  aliases,
					}
				}
			}
		}
	}
	return index
}

func simAuthorityValueInitializer(value *ast.ValueSpec, index int) ast.Expr {
	if index < len(value.Values) {
		return value.Values[index]
	}
	if len(value.Values) == 1 {
		return value.Values[0]
	}
	return nil
}

func (traits *simAuthorityStorageTraits) merge(other simAuthorityStorageTraits) {
	if traits.owners == nil {
		traits.owners = make(map[string]bool)
	}
	for owner := range other.owners {
		traits.owners[owner] = true
	}
	traits.mutable = traits.mutable || other.mutable
}

func simAuthorityOwnerTraitViolations(location string, traits simAuthorityStorageTraits) []string {
	violations := make([]string, 0, len(traits.owners))
	for owner := range traits.owners {
		violations = append(violations, location+" 保存额外 "+owner+".State owner")
	}
	return violations
}

func simAuthorityTypeStorageTraits(
	expression ast.Expr,
	aliases map[string]string,
	index simAuthorityStorageIndex,
	seenTypes map[string]bool,
) simAuthorityStorageTraits {
	traits := simAuthorityStorageTraits{owners: make(map[string]bool)}
	switch current := expression.(type) {
	case *ast.ParenExpr:
		return simAuthorityTypeStorageTraits(current.X, aliases, index, seenTypes)
	case *ast.StarExpr:
		traits.mutable = true
		traits.merge(simAuthorityTypeStorageTraits(current.X, aliases, index, seenTypes))
	case *ast.ArrayType:
		traits.mutable = true
		traits.merge(simAuthorityTypeStorageTraits(current.Elt, aliases, index, seenTypes))
	case *ast.MapType:
		traits.mutable = true
		traits.merge(simAuthorityTypeStorageTraits(current.Key, aliases, index, seenTypes))
		traits.merge(simAuthorityTypeStorageTraits(current.Value, aliases, index, seenTypes))
	case *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		traits.mutable = true
	case *ast.StructType:
		for _, field := range current.Fields.List {
			traits.merge(simAuthorityTypeStorageTraits(field.Type, aliases, index, seenTypes))
		}
	case *ast.SelectorExpr:
		if packageName, ok := current.X.(*ast.Ident); ok && current.Sel.Name == "State" {
			if owner := aliases[packageName.Name]; owner != "" {
				traits.owners[owner] = true
				traits.mutable = true
				break
			}
		}
		traits.mutable = true
	case *ast.Ident:
		if current.Name == "any" || current.Name == "error" {
			traits.mutable = true
			break
		}
		declaration, ok := index.types[current.Name]
		if !ok || seenTypes[current.Name] {
			break
		}
		seenTypes[current.Name] = true
		traits.merge(simAuthorityTypeStorageTraits(declaration.expression, declaration.aliases, index, seenTypes))
		delete(seenTypes, current.Name)
	case *ast.IndexExpr:
		traits.merge(simAuthorityTypeStorageTraits(current.X, aliases, index, seenTypes))
		traits.merge(simAuthorityTypeStorageTraits(current.Index, aliases, index, seenTypes))
	case *ast.IndexListExpr:
		traits.merge(simAuthorityTypeStorageTraits(current.X, aliases, index, seenTypes))
		for _, argument := range current.Indices {
			traits.merge(simAuthorityTypeStorageTraits(argument, aliases, index, seenTypes))
		}
	}
	return traits
}

func simAuthorityValueStorageTraits(
	expression ast.Expr,
	aliases map[string]string,
	index simAuthorityStorageIndex,
	seenFunctions map[string]bool,
) simAuthorityStorageTraits {
	traits := simAuthorityStorageTraits{owners: make(map[string]bool)}
	switch current := expression.(type) {
	case *ast.ParenExpr:
		return simAuthorityValueStorageTraits(current.X, aliases, index, seenFunctions)
	case *ast.UnaryExpr:
		traits.merge(simAuthorityValueStorageTraits(current.X, aliases, index, seenFunctions))
		if current.Op == token.AND {
			traits.mutable = true
		}
	case *ast.CompositeLit:
		traits.merge(simAuthorityTypeStorageTraits(current.Type, aliases, index, make(map[string]bool)))
	case *ast.FuncLit:
		traits.mutable = true
	case *ast.CallExpr:
		functionExpression := simAuthorityUnparenExpression(current.Fun)
		identifier := simAuthorityCallableIdentifier(functionExpression)
		if literal, ok := functionExpression.(*ast.FuncLit); ok {
			if literal.Type.Results != nil {
				for _, result := range literal.Type.Results.List {
					traits.merge(simAuthorityTypeStorageTraits(
						result.Type, aliases, index, make(map[string]bool),
					))
				}
			}
			break
		}
		if identifier != nil && identifier.Name == "new" && len(current.Args) == 1 {
			traits.mutable = true
			traits.merge(simAuthorityTypeStorageTraits(current.Args[0], aliases, index, make(map[string]bool)))
			break
		}
		if identifier != nil && identifier.Name == "make" && len(current.Args) != 0 {
			traits.mutable = true
			traits.merge(simAuthorityTypeStorageTraits(current.Args[0], aliases, index, make(map[string]bool)))
			break
		}
		if selector, ok := functionExpression.(*ast.SelectorExpr); ok && selector.Sel.Name == "NewState" {
			if packageName, ok := selector.X.(*ast.Ident); ok {
				if owner := aliases[packageName.Name]; owner != "" {
					traits.owners[owner] = true
					traits.mutable = true
					break
				}
			}
		}
		resolved := false
		switch functionExpression.(type) {
		case *ast.StarExpr, *ast.ArrayType, *ast.MapType,
			*ast.ChanType, *ast.FuncType, *ast.InterfaceType, *ast.StructType:
			traits.merge(simAuthorityTypeStorageTraits(functionExpression, aliases, index, make(map[string]bool)))
			resolved = true
		}
		if identifier != nil {
			if declaration, ok := index.types[identifier.Name]; ok {
				traits.merge(simAuthorityTypeStorageTraits(
					declaration.expression, declaration.aliases, index, make(map[string]bool),
				))
				resolved = true
			}
			if simAuthorityBuiltinScalarType(identifier.Name) {
				resolved = true
			}
			if declaration, ok := index.functions[identifier.Name]; ok && !seenFunctions[identifier.Name] {
				seenFunctions[identifier.Name] = true
				if declaration.function.Type.Results != nil {
					for _, result := range declaration.function.Type.Results.List {
						traits.merge(simAuthorityTypeStorageTraits(
							result.Type, declaration.aliases, index, make(map[string]bool),
						))
					}
				}
				delete(seenFunctions, identifier.Name)
				resolved = true
			}
		}
		if !resolved {
			traits.mutable = true
		}
	}
	return traits
}

func simAuthorityUnparenExpression(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func simAuthorityBuiltinScalarType(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128", "float32", "float64", "int", "int8", "int16",
		"int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}

func simAuthorityCallableIdentifier(expression ast.Expr) *ast.Ident {
	switch current := expression.(type) {
	case *ast.Ident:
		return current
	case *ast.IndexExpr:
		return simAuthorityCallableIdentifier(current.X)
	case *ast.IndexListExpr:
		return simAuthorityCallableIdentifier(current.X)
	case *ast.ParenExpr:
		return simAuthorityCallableIdentifier(current.X)
	default:
		return nil
	}
}

func simAuthorityAllowedEngineOwner(location, fieldName string, expression ast.Expr, aliases map[string]string) string {
	if location != "runtime.Engine" {
		return ""
	}
	star, ok := expression.(*ast.StarExpr)
	if !ok {
		return ""
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "State" {
		return ""
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	owner := aliases[packageName.Name]
	if (fieldName == "realm" && owner == "realm") || (fieldName == "entities" && owner == "entity") {
		return owner
	}
	return ""
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
	steps := make([]*ast.FuncDecl, 0, 1)
	helpers := make([]*ast.FuncDecl, 0, 1)
	for _, file := range parsed {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if function.Recv == nil {
				if function.Name.Name == "finishRealmMutation" {
					helpers = append(helpers, function)
				}
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
	creationIndices := make([]int, 0, 1)
	for statementIndex, statement := range step.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		name, nameOK := assignment.Lhs[0].(*ast.Ident)
		call, callOK := assignment.Rhs[0].(*ast.CallExpr)
		if nameOK && callOK && simAuthorityEngineNewMutation(call, receiver) {
			locals = append(locals, name.Name)
			creationIndices = append(creationIndices, statementIndex)
		}
	}
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

	const helperName = "finishRealmMutation"
	if len(helpers) != 1 {
		return []string{fmt.Sprintf(
			"%s：runtime 顶层 helper %q 数量=%d，想要 1",
			files.Position(step.Pos()), helperName, len(helpers),
		)}
	}

	directCalls, placementViolations := simAuthorityTopLevelHelperCalls(step.Body, helperName)
	if len(placementViolations) != 0 {
		return placementViolations
	}
	helperStatementIndex := -1
	if len(directCalls) == 1 {
		helperStatementIndex = simAuthorityTopLevelCallStatementIndex(step.Body, directCalls[0])
		if simAuthorityFunctionBindsNameBefore(step, helperName, helperStatementIndex) {
			return []string{fmt.Sprintf(
				"%s：(*Engine).StepWithTunables 局部绑定遮蔽顶层 helper %q",
				files.Position(step.Pos()), helperName,
			)}
		}
	}
	matchingArguments := make([]int, 0, 1)
	for _, call := range directCalls {
		for argumentIndex, argument := range call.Args {
			identifier, ok := argument.(*ast.Ident)
			if ok && identifier.Name == locals[0] {
				matchingArguments = append(matchingArguments, argumentIndex)
			}
		}
	}
	if len(matchingArguments) != 1 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 使用 mutation local %q 的顶层 helper 调用次数=%d，想要 1",
			files.Position(step.Pos()), locals[0], len(matchingArguments),
		)}
	}
	if len(directCalls) != 1 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 顶层 helper %q 调用次数=%d，想要 1",
			files.Position(step.Pos()), helperName, len(directCalls),
		)}
	}
	if creationIndices[0] >= helperStatementIndex {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables mutation 创建必须先于顶层 helper 调用",
			files.Position(step.Pos()),
		)}
	}
	if simAuthorityStatementsContainExit(step.Body.List[:helperStatementIndex], true) ||
		simAuthorityStatementsContainExit(step.Body.List, false) {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 在 mutation 创建或 helper 调用前存在提前退出",
			files.Position(step.Pos()),
		)}
	}
	if kind := simAuthorityAsyncStatementKind(step.Body); kind != "" {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables mutation 生命周期不得包含 %s",
			files.Position(step.Pos()), kind,
		)}
	}
	parameter := simAuthorityParameterAt(helpers[0], matchingArguments[0])
	if parameter == "" {
		return []string{fmt.Sprintf(
			"%s：顶层 helper %q 缺少 mutation 参数位置 %d",
			files.Position(helpers[0].Pos()), helperName, matchingArguments[0],
		)}
	}
	if writes := simAuthorityTrackedWrites(helpers[0].Body, parameter); writes != 0 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables helper %s 的 mutation 参数 %q 被重新声明或赋值 %d 次",
			files.Position(step.Pos()), helperName, parameter, writes,
		)}
	}
	commits, commitViolations := simAuthorityDeterministicCommitCount(helpers[0].Body, parameter, helperName)
	if len(commitViolations) != 0 {
		return commitViolations
	}
	if commits != 1 {
		return []string{fmt.Sprintf(
			"%s：helper %q 对 mutation 参数 %q 的确定性 Commit 次数=%d，想要 1",
			files.Position(helpers[0].Pos()), helperName, parameter, commits,
		)}
	}
	commitStatementIndex := simAuthorityTopLevelCommitStatementIndex(helpers[0].Body, parameter)
	if commitStatementIndex < 0 {
		return []string{fmt.Sprintf(
			"%s：helper %q 缺少确定性顶层 Commit 语句",
			files.Position(helpers[0].Pos()), helperName,
		)}
	}
	if simAuthorityStatementsContainExit(helpers[0].Body.List[:commitStatementIndex], true) ||
		simAuthorityStatementsContainExit(helpers[0].Body.List, false) {
		return []string{fmt.Sprintf(
			"%s：helper %q 在 Commit 前存在提前退出",
			files.Position(helpers[0].Pos()), helperName,
		)}
	}
	if kind := simAuthorityAsyncStatementKind(helpers[0].Body); kind != "" {
		return []string{fmt.Sprintf(
			"%s：helper %q mutation 生命周期不得包含 %s",
			files.Position(helpers[0].Pos()), helperName, kind,
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

func simAuthorityFunctionBindsNameBefore(function *ast.FuncDecl, name string, statementIndex int) bool {
	for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params, function.Type.Results} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			for _, fieldName := range field.Names {
				if fieldName.Name == name {
					return true
				}
			}
		}
	}
	for index, statement := range function.Body.List {
		if index >= statementIndex {
			break
		}
		switch current := statement.(type) {
		case *ast.AssignStmt:
			for _, expression := range current.Lhs {
				if identifier, ok := expression.(*ast.Ident); ok && identifier.Name == name {
					return true
				}
			}
		case *ast.DeclStmt:
			general, ok := current.Decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range general.Specs {
				switch spec := specification.(type) {
				case *ast.ValueSpec:
					for _, declaredName := range spec.Names {
						if declaredName.Name == name {
							return true
						}
					}
				case *ast.TypeSpec:
					if spec.Name.Name == name {
						return true
					}
				}
			}
		}
	}
	return false
}

func simAuthorityTopLevelHelperCalls(body *ast.BlockStmt, helperName string) ([]*ast.CallExpr, []string) {
	calls := make([]*ast.CallExpr, 0, 1)
	violations := make([]string, 0)
	for _, statement := range body.List {
		switch current := statement.(type) {
		case *ast.ExprStmt:
			if call, ok := current.X.(*ast.CallExpr); ok && simAuthorityNamedCall(call, helperName) {
				calls = append(calls, call)
				continue
			}
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if simAuthorityNodeContainsNamedCall(statement, helperName) {
				violations = append(violations, fmt.Sprintf("helper %q 调用位于条件控制流", helperName))
				continue
			}
		case *ast.ForStmt, *ast.RangeStmt:
			if simAuthorityNodeContainsNamedCall(statement, helperName) {
				violations = append(violations, fmt.Sprintf("helper %q 调用位于循环", helperName))
				continue
			}
		case *ast.GoStmt:
			if simAuthorityNamedCall(current.Call, helperName) {
				violations = append(violations, fmt.Sprintf("helper %q 调用不得使用 go", helperName))
				continue
			}
		case *ast.DeferStmt:
			if simAuthorityNamedCall(current.Call, helperName) {
				violations = append(violations, fmt.Sprintf("helper %q 调用不得使用 defer", helperName))
				continue
			}
		}
		if simAuthorityNodeContainsNamedCall(statement, helperName) {
			violations = append(violations, fmt.Sprintf("helper %q 调用不是确定性顶层语句", helperName))
		}
	}
	return calls, violations
}

func simAuthorityNamedCall(call *ast.CallExpr, name string) bool {
	identifier, ok := call.Fun.(*ast.Ident)
	return ok && identifier.Name == name
}

func simAuthorityTopLevelCallStatementIndex(body *ast.BlockStmt, target *ast.CallExpr) int {
	for index, statement := range body.List {
		expression, ok := statement.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := expression.X.(*ast.CallExpr)
		if ok && call == target {
			return index
		}
	}
	return -1
}

func simAuthorityNodeContainsNamedCall(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		if _, nested := current.(*ast.FuncLit); nested {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if ok && simAuthorityNamedCall(call, name) {
			found = true
			return false
		}
		return true
	})
	return found
}

func simAuthorityDeterministicCommitCount(body *ast.BlockStmt, local, helperName string) (int, []string) {
	commits := 0
	violations := make([]string, 0)
	for _, statement := range body.List {
		switch current := statement.(type) {
		case *ast.ExprStmt:
			if call, ok := current.X.(*ast.CallExpr); ok && simAuthorityCommitCall(call, local) {
				commits++
				continue
			}
		case *ast.RangeStmt:
			if call, ok := current.X.(*ast.CallExpr); ok && simAuthorityCommitCall(call, local) {
				commits++
			}
			if simAuthorityNodeContainsCommit(current.Body, local) {
				violations = append(violations, fmt.Sprintf("helper %q 的 Commit 调用位于循环", helperName))
			}
			continue
		case *ast.ForStmt:
			if simAuthorityNodeContainsCommit(current, local) {
				violations = append(violations, fmt.Sprintf("helper %q 的 Commit 调用位于循环", helperName))
			}
			continue
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if simAuthorityNodeContainsCommit(current, local) {
				violations = append(violations, fmt.Sprintf("helper %q 的 Commit 调用位于条件控制流", helperName))
			}
			continue
		case *ast.GoStmt:
			if simAuthorityCommitCall(current.Call, local) {
				violations = append(violations, fmt.Sprintf("helper %q 的 Commit 调用不得使用 go", helperName))
			}
			continue
		case *ast.DeferStmt:
			if simAuthorityCommitCall(current.Call, local) {
				violations = append(violations, fmt.Sprintf("helper %q 的 Commit 调用不得使用 defer", helperName))
			}
			continue
		}
		if simAuthorityNodeContainsCommit(statement, local) {
			violations = append(violations, fmt.Sprintf("helper %q 的 Commit 调用不是确定性顶层语句", helperName))
		}
	}
	return commits, violations
}

func simAuthorityTopLevelCommitStatementIndex(body *ast.BlockStmt, local string) int {
	for index, statement := range body.List {
		switch current := statement.(type) {
		case *ast.ExprStmt:
			if call, ok := current.X.(*ast.CallExpr); ok && simAuthorityCommitCall(call, local) {
				return index
			}
		case *ast.RangeStmt:
			if call, ok := current.X.(*ast.CallExpr); ok && simAuthorityCommitCall(call, local) {
				return index
			}
		}
	}
	return -1
}

func simAuthorityCommitCall(call *ast.CallExpr, local string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Commit" || len(call.Args) != 0 {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == local
}

func simAuthorityNodeContainsCommit(node ast.Node, local string) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		if _, nested := current.(*ast.FuncLit); nested {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if ok && simAuthorityCommitCall(call, local) {
			found = true
			return false
		}
		return true
	})
	return found
}

func simAuthorityStatementsContainExit(statements []ast.Stmt, includeReturn bool) bool {
	for _, statement := range statements {
		found := false
		ast.Inspect(statement, func(node ast.Node) bool {
			if found {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			switch current := node.(type) {
			case *ast.ReturnStmt:
				if includeReturn {
					found = true
					return false
				}
			case *ast.BranchStmt:
				if current.Tok == token.GOTO {
					found = true
					return false
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func simAuthorityAsyncStatementKind(body *ast.BlockStmt) string {
	found := ""
	ast.Inspect(body, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		switch node.(type) {
		case *ast.GoStmt:
			found = "go"
			return false
		case *ast.DeferStmt:
			found = "defer"
			return false
		}
		return true
	})
	return found
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
