package archcheck_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
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
	"passives":   "passiveSet",
}

var requiredEntitySessionFields = map[string]string{
	"player":    "*playerState",
	"container": "core.ContainerRef",
}

func TestSimAuthorityStateOwnershipStaysExplicit(t *testing.T) {
	root := repositoryRoot(t)
	typeEnvironment, err := newSimAuthorityTypeEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	violations, err := simAuthorityViolationsFromTree(root, typeEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("权威模拟所有权漂移：%s", strings.Join(violations, "; "))
	}
}

func TestSimAuthorityGuardRejectsSyntheticDrift(t *testing.T) {
	typeEnvironment, err := newSimAuthorityTypeEnvironment(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	goodRuntime := `package runtime
import (
	"sort"
	"sync"
	"sync/atomic"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/server/sim/entity"
	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/tuning"
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
func (engine *Engine) StepWithTunables() {
	values := []int{2, 1}
	sort.SliceStable(values, func(i, j int) bool { return values[i] < values[j] })
	pending := engine.realm.NewMutation(); finishRealmMutation(pending)
}
func finishRealmMutation(pending *realm.Mutation) { pending.Commit() }
func mutateAsync(*realm.Mutation) {}
func mutateLater(*realm.Mutation) {}
func leakMutationBinding(**realm.Mutation) {}
`
	goodEntity := `package entity
import (
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
)
type SessionID uint64
type playerState struct{}
type companionState struct{}
type hostileSet struct{}
type passiveSet struct{}
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
	passives passiveSet
}
`

	if violations := simAuthorityViolationsFromSources(typeEnvironment, goodRuntime, goodEntity); len(violations) != 0 {
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
			name: "runtime 安全 imported value holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type H struct { Pos core.ChunkPos }\nvar h H\ntype Engine struct {", 1), entitySource
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
			wants: []string{"runtime package variable extraRealm 保存额外 realm.State owner"},
		},
		{
			name: "runtime type assertion 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraEntity = any(entity.NewState(0)).(*entity.State)\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraEntity 保存额外 entity.State owner"},
		},
		{
			name: "runtime slice expression 返回包级 mutable state",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type playerSnapshot struct{}\nvar hidden = make([]playerSnapshot, 1)[:]\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable hidden 保存递归 mutable state"},
		},
		{
			name: "runtime struct selector 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraEntity = struct { value *entity.State }{value: entity.NewState(0)}.value\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraEntity 保存额外 entity.State owner"},
		},
		{
			name: "runtime array index 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraEntity = [...]*entity.State{entity.NewState(0)}[0]\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraEntity 保存额外 entity.State owner"},
		},
		{
			name: "runtime dereference 返回包级 owner value",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "var extraEntity = *entity.NewState(0)\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraEntity 保存额外 entity.State owner"},
		},
		{
			name: "runtime generic identity 返回包级 owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "func identity[T any](value T) T { return value }\nvar extraEntity = identity(entity.NewState(0))\ntype Engine struct {", 1), entitySource
			},
			wants: []string{"runtime package variable extraEntity 保存额外 entity.State owner"},
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
			name: "runtime 固定值数组 holder",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "type Engine struct {", "type playerSnapshot struct{}\nvar hidden [2]playerSnapshot\ntype Engine struct {", 1), entitySource
			},
			clean: true,
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
					"\"github.com/channing771/mornlea/packages/server/sim/entity\"",
					"ent \"github.com/channing771/mornlea/packages/server/sim/entity\"",
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
			name: "entity 丢失 passive owner",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return runtimeSource, strings.Replace(entitySource, "\tpassives passiveSet\n", "", 1)
			},
			wants: []string{"entity.State 缺少字段 passives passiveSet"},
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
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); { var pending *realm.Mutation; pending.Commit() }; _ = pending",
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
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); mutationCh := make(chan *realm.Mutation); for pending := range mutationCh { pending.Commit(); break }; _ = pending",
					1,
				)
				return strings.Replace(runtimeSource, "func finishRealmMutation(pending *realm.Mutation) { pending.Commit() }", "func finishRealmMutation(pending *realm.Mutation) { _ = pending }", 1), entitySource
			},
			wants: []string{"局部 mutation \"pending\" 被声明或赋值 2 次"},
		},
		{
			name: "runtime 把 mutation 搬到不可达 helper",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				runtimeSource = strings.Replace(runtimeSource, "pending := engine.realm.NewMutation(); finishRealmMutation(pending)", "", 1)
				return strings.Replace(runtimeSource, "func finishRealmMutation", "func deadTick(engine *Engine) { pending := engine.realm.NewMutation(); finishRealmMutation(pending) }\nfunc finishRealmMutation", 1), entitySource
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
			name: "runtime IIFE 捕获 mutation local",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); func() { pending = pending }(); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables FuncLit 捕获 mutation local \"pending\""},
		},
		{
			name: "runtime mutation local 取址逃逸",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource,
					"pending := engine.realm.NewMutation(); finishRealmMutation(pending)",
					"pending := engine.realm.NewMutation(); leakMutationBinding(&pending); finishRealmMutation(pending)",
					1,
				), entitySource
			},
			wants: []string{"(*Engine).StepWithTunables 对 mutation local \"pending\" 取址逃逸"},
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
			name: "runtime helper IIFE 捕获 mutation 参数",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "func() { pending = pending }(); pending.Commit()", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" FuncLit 捕获 mutation 参数 \"pending\""},
		},
		{
			name: "runtime helper mutation 参数取址逃逸",
			mutate: func(runtimeSource, entitySource string) (string, string) {
				return strings.Replace(runtimeSource, "pending.Commit()", "leakMutationBinding(&pending); pending.Commit()", 1), entitySource
			},
			wants: []string{"helper \"finishRealmMutation\" 对 mutation 参数 \"pending\" 取址逃逸"},
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
			violations := simAuthorityViolationsFromSources(typeEnvironment, runtimeSource, entitySource)
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

type simAuthorityTypeEnvironment struct {
	files    *token.FileSet
	importer types.Importer
}

func newSimAuthorityTypeEnvironment(root string) (*simAuthorityTypeEnvironment, error) {
	// server 已是独立模块且不再被根模块 require：go list 直接落在
	// packages/server 目录，在 GOWORK=off 的单模块世界里以 server 自身的
	// require+replace 解析依赖闭包（根模块的 go.sum 不再涵盖 server 的第三方
	// 依赖）。
	command := exec.Command("go", "list", "-e", "-export", "-deps", "-json", "github.com/channing771/mornlea/packages/server/sim/runtime")
	command.Dir = filepath.Join(root, "packages", "server")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) != 0 {
			return nil, fmt.Errorf("go list 权威模拟类型导出数据: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("go list 权威模拟类型导出数据: %w", err)
	}
	type listedPackage struct {
		ImportPath string
		Export     string
		Error      *struct{ Err string }
	}
	exports := make(map[string]string)
	failures := make(map[string]string)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var listed listedPackage
		if err := decoder.Decode(&listed); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("解析权威模拟类型导出数据: %w", err)
		}
		if listed.Export != "" {
			exports[listed.ImportPath] = listed.Export
		}
		if listed.Error != nil {
			failures[listed.ImportPath] = listed.Error.Err
		}
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if failure := failures[path]; failure != "" {
			return nil, fmt.Errorf("解析 %s: %s", path, failure)
		}
		export := exports[path]
		if export == "" {
			return nil, fmt.Errorf("go list 未提供 %s 的类型导出数据", path)
		}
		file, err := os.Open(export)
		if err != nil {
			return nil, fmt.Errorf("打开 %s 的类型导出数据: %w", path, err)
		}
		return file, nil
	}
	files := token.NewFileSet()
	return &simAuthorityTypeEnvironment{
		files:    files,
		importer: importer.ForCompiler(files, "gc", lookup),
	}, nil
}

func simAuthorityViolationsFromTree(root string, typeEnvironment *simAuthorityTypeEnvironment) ([]string, error) {
	runtimeShape, err := readSimAuthorityPackage(
		filepath.Join(root, "packages", "server", "sim", "runtime"),
		"runtime",
		modulePath+"/packages/server/sim/runtime",
		typeEnvironment,
	)
	if err != nil {
		return nil, fmt.Errorf("读取 runtime 所有权结构：%w", err)
	}
	entityShape, err := readSimAuthorityPackage(
		filepath.Join(root, "packages", "server", "sim", "entity"),
		"entity",
		modulePath+"/packages/server/sim/entity",
		typeEnvironment,
	)
	if err != nil {
		return nil, fmt.Errorf("读取 entity 所有权结构：%w", err)
	}
	return simAuthorityViolations(runtimeShape, entityShape), nil
}

func simAuthorityViolationsFromSources(
	typeEnvironment *simAuthorityTypeEnvironment,
	runtimeSource, entitySource string,
) []string {
	runtimeShape, err := parseSimAuthoritySource(
		"runtime.go", runtimeSource, modulePath+"/packages/server/sim/runtime", typeEnvironment,
	)
	if err != nil {
		return []string{"解析 synthetic runtime：" + err.Error()}
	}
	entityShape, err := parseSimAuthoritySource(
		"entity.go", entitySource, modulePath+"/packages/server/sim/entity", typeEnvironment,
	)
	if err != nil {
		return []string{"解析 synthetic entity：" + err.Error()}
	}
	return simAuthorityViolations(runtimeShape, entityShape)
}

func readSimAuthorityPackage(
	directory, packageName, packagePath string,
	typeEnvironment *simAuthorityTypeEnvironment,
) (simAuthorityShape, error) {
	files := typeEnvironment.files
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
	fileNames := make([]string, 0, len(parsed.Files))
	for name := range parsed.Files {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	packageFiles := make([]*ast.File, 0, len(fileNames))
	for _, name := range fileNames {
		packageFiles = append(packageFiles, parsed.Files[name])
	}
	return collectSimAuthorityShape(files, packageFiles, packagePath, typeEnvironment)
}

func parseSimAuthoritySource(
	name, source, packagePath string,
	typeEnvironment *simAuthorityTypeEnvironment,
) (simAuthorityShape, error) {
	files := typeEnvironment.files
	parsed, err := parser.ParseFile(files, name, source, parser.SkipObjectResolution)
	if err != nil {
		return simAuthorityShape{}, err
	}
	return collectSimAuthorityShape(files, []*ast.File{parsed}, packagePath, typeEnvironment)
}

func collectSimAuthorityShape(
	files *token.FileSet,
	parsed []*ast.File,
	packagePath string,
	typeEnvironment *simAuthorityTypeEnvironment,
) (simAuthorityShape, error) {
	typeInfo := &types.Info{
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	configuration := types.Config{
		Importer:    typeEnvironment.importer,
		FakeImportC: true,
		GoVersion:   "go1.26",
	}
	if _, err := configuration.Check(packagePath, files, parsed, typeInfo); err != nil {
		return simAuthorityShape{}, fmt.Errorf("类型检查 %s: %w", packagePath, err)
	}
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
		shape.storageDrift = simAuthorityRuntimeStorageDrift(files, parsed, typeInfo)
		shape.mutationDrift = simAuthorityRuntimeMutationDrift(files, parsed, typeInfo)
	}
	return shape, nil
}

func formatSimAuthorityNode(files *token.FileSet, node ast.Node) string {
	var output bytes.Buffer
	if err := format.Node(&output, files, node); err != nil {
		return "<format error>"
	}
	return output.String()
}

type simAuthorityStorageTraits struct {
	owners  map[string]bool
	mutable bool
}

const (
	simAuthorityEntityPackage = modulePath + "/packages/server/sim/entity"
	simAuthorityRealmPackage  = modulePath + "/packages/server/sim/realm"
)

type simAuthorityImportedAlias struct {
	packagePath string
	name        string
}

var allowedRuntimeImportedAliases = map[string]simAuthorityImportedAlias{
	"ErrBlockOutOfWorld": {packagePath: simAuthorityRealmPackage, name: "ErrBlockOutOfWorld"},
	"ErrChunkNotReady":   {packagePath: simAuthorityRealmPackage, name: "ErrChunkNotReady"},
	"NewDimension":       {packagePath: simAuthorityRealmPackage, name: "NewDimension"},
}

func simAuthorityRuntimeStorageDrift(
	files *token.FileSet,
	parsed []*ast.File,
	typeInfo *types.Info,
) []string {
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
		for _, specification := range file.Imports {
			if specification.Name == nil || specification.Name.Name != "." {
				continue
			}
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err == nil {
				if owner := simAuthorityOwnerPackage(importPath); owner != "" {
					violations = append(violations, "runtime 不得以 dot import 引入 "+owner+" owner")
				}
			}
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
					object, ok := typeInfo.Defs[typeSpec.Name].(*types.TypeName)
					if !ok {
						continue
					}
					traits := simAuthorityStorageTraitsOf(object.Type(), make(map[types.Type]bool))
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
					if name.Name == "_" {
						continue
					}
					object, ok := typeInfo.Defs[name].(*types.Var)
					if !ok {
						continue
					}
					traits := simAuthorityStorageTraitsOf(object.Type(), make(map[types.Type]bool))
					location := "runtime package variable " + name.Name
					ownerViolations := simAuthorityOwnerTraitViolations(location, traits)
					violations = append(violations, ownerViolations...)
					if len(ownerViolations) == 0 && traits.mutable &&
						!simAuthorityAllowedImportedAlias(name.Name, value, valueIndex, typeInfo) {
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
			structureType, ok := types.Unalias(typeInfo.TypeOf(structure)).Underlying().(*types.Struct)
			if !ok {
				return true
			}
			location := namedStructs[structure.Pos()]
			if location == "" {
				location = "runtime anonymous holder at " + files.Position(structure.Pos()).String()
			}
			for fieldIndex := 0; fieldIndex < structureType.NumFields(); fieldIndex++ {
				field := structureType.Field(fieldIndex)
				fieldName := field.Name()
				if fieldName == "" {
					fieldName = "<embedded>"
				}
				traits := simAuthorityStorageTraitsOf(field.Type(), make(map[types.Type]bool))
				allowedOwner := simAuthorityAllowedEngineOwner(location, fieldName, field.Type())
				for owner := range traits.owners {
					if owner != allowedOwner {
						violations = append(violations, location+"."+fieldName+" 保存额外 "+owner+".State owner")
					}
				}
			}
			return true
		})
	}
	return violations
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

func simAuthorityStorageTraitsOf(current types.Type, seen map[types.Type]bool) simAuthorityStorageTraits {
	traits := simAuthorityStorageTraits{owners: make(map[string]bool)}
	if current == nil {
		traits.mutable = true
		return traits
	}
	current = types.Unalias(current)
	if owner := simAuthorityOwnerType(current); owner != "" {
		traits.owners[owner] = true
		traits.mutable = true
	}
	if seen[current] {
		return traits
	}
	seen[current] = true
	defer delete(seen, current)

	switch typed := current.(type) {
	case *types.Basic:
		if typed.Kind() == types.UnsafePointer {
			traits.mutable = true
		}
	case *types.Pointer:
		traits.mutable = true
		traits.merge(simAuthorityStorageTraitsOf(typed.Elem(), seen))
	case *types.Array:
		traits.merge(simAuthorityStorageTraitsOf(typed.Elem(), seen))
	case *types.Slice:
		traits.mutable = true
		traits.merge(simAuthorityStorageTraitsOf(typed.Elem(), seen))
	case *types.Map:
		traits.mutable = true
		traits.merge(simAuthorityStorageTraitsOf(typed.Key(), seen))
		traits.merge(simAuthorityStorageTraitsOf(typed.Elem(), seen))
	case *types.Chan:
		traits.mutable = true
		traits.merge(simAuthorityStorageTraitsOf(typed.Elem(), seen))
	case *types.Signature:
		traits.mutable = true
		if typed.Recv() != nil {
			traits.merge(simAuthorityStorageTraitsOf(typed.Recv().Type(), seen))
		}
		traits.merge(simAuthorityStorageTraitsOf(typed.Params(), seen))
		traits.merge(simAuthorityStorageTraitsOf(typed.Results(), seen))
	case *types.Interface:
		traits.mutable = true
		typed = typed.Complete()
		for methodIndex := 0; methodIndex < typed.NumMethods(); methodIndex++ {
			traits.merge(simAuthorityStorageTraitsOf(typed.Method(methodIndex).Type(), seen))
		}
		for embeddedIndex := 0; embeddedIndex < typed.NumEmbeddeds(); embeddedIndex++ {
			traits.merge(simAuthorityStorageTraitsOf(typed.EmbeddedType(embeddedIndex), seen))
		}
	case *types.Struct:
		for fieldIndex := 0; fieldIndex < typed.NumFields(); fieldIndex++ {
			traits.merge(simAuthorityStorageTraitsOf(typed.Field(fieldIndex).Type(), seen))
		}
	case *types.Tuple:
		for fieldIndex := 0; fieldIndex < typed.Len(); fieldIndex++ {
			traits.merge(simAuthorityStorageTraitsOf(typed.At(fieldIndex).Type(), seen))
		}
	case *types.Named:
		traits.merge(simAuthorityStorageTraitsOf(typed.Underlying(), seen))
	case *types.TypeParam:
		traits.mutable = true
		traits.merge(simAuthorityStorageTraitsOf(typed.Constraint(), seen))
	case *types.Union:
		traits.mutable = true
		for termIndex := 0; termIndex < typed.Len(); termIndex++ {
			traits.merge(simAuthorityStorageTraitsOf(typed.Term(termIndex).Type(), seen))
		}
	default:
		traits.mutable = true
	}
	return traits
}

func simAuthorityAllowedEngineOwner(location, fieldName string, fieldType types.Type) string {
	if location != "runtime.Engine" {
		return ""
	}
	pointer, ok := types.Unalias(fieldType).(*types.Pointer)
	if !ok {
		return ""
	}
	owner := simAuthorityOwnerType(types.Unalias(pointer.Elem()))
	if (fieldName == "realm" && owner == "realm") || (fieldName == "entities" && owner == "entity") {
		return owner
	}
	return ""
}

func simAuthorityOwnerType(current types.Type) string {
	named, ok := types.Unalias(current).(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil || named.Obj().Name() != "State" {
		return ""
	}
	return simAuthorityOwnerPackage(named.Obj().Pkg().Path())
}

func simAuthorityOwnerPackage(packagePath string) string {
	switch packagePath {
	case simAuthorityRealmPackage:
		return "realm"
	case simAuthorityEntityPackage:
		return "entity"
	default:
		return ""
	}
}

func simAuthorityAllowedImportedAlias(
	variableName string,
	value *ast.ValueSpec,
	valueIndex int,
	typeInfo *types.Info,
) bool {
	want, allowed := allowedRuntimeImportedAliases[variableName]
	if !allowed || len(value.Values) != len(value.Names) || valueIndex >= len(value.Values) {
		return false
	}
	expression := value.Values[valueIndex]
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	object := typeInfo.Uses[selector.Sel]
	return object != nil && object.Pkg() != nil && object.Pkg().Path() == want.packagePath && object.Name() == want.name
}

func simAuthorityRuntimeMutationDrift(files *token.FileSet, parsed []*ast.File, typeInfo *types.Info) []string {
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
	mutationLocal := typeInfo.Defs[simAuthorityDirectMutationLocal(step, receiver)]
	if mutationLocal == nil {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables 无法解析 mutation local %q 的类型对象",
			files.Position(step.Pos()), locals[0],
		)}
	}
	if violation := simAuthorityMutationBindingViolation(
		step.Body, mutationLocal, typeInfo,
		"(*Engine).StepWithTunables", "mutation local", locals[0],
	); violation != "" {
		return []string{files.Position(step.Pos()).String() + "：" + violation}
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
	parameterIdentifier := simAuthorityParameterAt(helpers[0], matchingArguments[0])
	if parameterIdentifier == nil {
		return []string{fmt.Sprintf(
			"%s：顶层 helper %q 缺少 mutation 参数位置 %d",
			files.Position(helpers[0].Pos()), helperName, matchingArguments[0],
		)}
	}
	parameter := parameterIdentifier.Name
	if writes := simAuthorityTrackedWrites(helpers[0].Body, parameter); writes != 0 {
		return []string{fmt.Sprintf(
			"%s：(*Engine).StepWithTunables helper %s 的 mutation 参数 %q 被重新声明或赋值 %d 次",
			files.Position(step.Pos()), helperName, parameter, writes,
		)}
	}
	parameterObject := typeInfo.Defs[parameterIdentifier]
	if parameterObject == nil {
		return []string{fmt.Sprintf(
			"%s：顶层 helper %q 无法解析 mutation 参数 %q 的类型对象",
			files.Position(helpers[0].Pos()), helperName, parameter,
		)}
	}
	if violation := simAuthorityMutationBindingViolation(
		helpers[0].Body, parameterObject, typeInfo,
		"helper \""+helperName+"\"", "mutation 参数", parameter,
	); violation != "" {
		return []string{files.Position(helpers[0].Pos()).String() + "：" + violation}
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

func simAuthorityDirectMutationLocal(function *ast.FuncDecl, receiver string) *ast.Ident {
	for _, statement := range function.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		name, nameOK := assignment.Lhs[0].(*ast.Ident)
		call, callOK := assignment.Rhs[0].(*ast.CallExpr)
		if nameOK && callOK && simAuthorityEngineNewMutation(call, receiver) {
			return name
		}
	}
	return nil
}

func simAuthorityMutationBindingViolation(
	body *ast.BlockStmt,
	binding types.Object,
	typeInfo *types.Info,
	location, bindingKind, bindingName string,
) string {
	violation := ""
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || violation != "" {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			captured := false
			ast.Inspect(literal.Body, func(nested ast.Node) bool {
				identifier, ok := nested.(*ast.Ident)
				if ok && typeInfo.Uses[identifier] == binding {
					captured = true
					return false
				}
				return !captured
			})
			if captured {
				violation = fmt.Sprintf("%s FuncLit 捕获 %s %q", location, bindingKind, bindingName)
			}
			return false
		}
		address, ok := node.(*ast.UnaryExpr)
		if !ok || address.Op != token.AND {
			return true
		}
		identifier, ok := ast.Unparen(address.X).(*ast.Ident)
		if ok && typeInfo.Uses[identifier] == binding {
			violation = fmt.Sprintf("%s 对 %s %q 取址逃逸", location, bindingKind, bindingName)
			return false
		}
		return true
	})
	return violation
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

func simAuthorityParameterAt(function *ast.FuncDecl, index int) *ast.Ident {
	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			if index == 0 {
				return name
			}
			index--
		}
		if len(field.Names) == 0 {
			if index == 0 {
				return nil
			}
			index--
		}
	}
	return nil
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
