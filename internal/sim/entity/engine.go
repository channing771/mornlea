package entity

import (
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// sessionState 只保存玩家与玩法生命周期状态；命令序号、观察中心和区块订阅由
// runtime 单独持有，避免把传输编排混入实体 owner。
type sessionState struct {
	id            SessionID
	dimension     core.DimensionID
	player        *playerState
	container     core.ContainerRef
	viewContainer bool
}

// State 是玩家、伙伴、夜行者及其玩法结算状态的唯一 owner。
type State struct {
	seed                   int64
	sessions               map[SessionID]*sessionState
	companions             map[companion.ID]*companionState
	hostiles               hostileSet
	hostileLight           *blockLightScratch
	subscriptionsDirty     bool
	tramplePending         []tramplePendingCell
	dropKeySeen            map[core.ChunkKey]struct{}
	dropKeyScratch         []core.ChunkKey
	containerViewerScratch []SessionID
	dropSessionScratch     []SessionID
}

// engineContext 是一次调用或一个 tick 的只读编排上下文。它借用 runtime 持有的
// realm 与时钟/参数快照，不把这些值变成 entity 的第二份权威状态。
type engineContext struct {
	*State
	realm           *realm.State
	tick            localCounter
	worldTime       localCounter
	dayPhaseOffset  localCounter
	tunables        tuning.Tunables
	physicsTunables physics.Tunables
	views           ViewSnapshot
}

// localCounter 只服务一次串行 entity 调用或包内测试夹具。权威并发时钟仍由
// runtime 的原子字段持有；这里保留 `Load`/`Store`/`Add` 形状，避免短命 tick
// 上下文携带不可复制的原子值。
type localCounter uint64

func (counter *localCounter) Load() uint64 { return uint64(*counter) }

func (counter *localCounter) Store(value uint64) { *counter = localCounter(value) }

func (counter *localCounter) Add(delta uint64) uint64 {
	*counter += localCounter(delta)
	return uint64(*counter)
}

// SessionView 是 runtime 在调用 entity 阶段时提供的只读订阅快照。
type SessionView struct {
	Ready  bool
	Center core.ChunkPos
}

// TickSessionView 是 runtime 借给单个 tick 的会话视图值，不含可变订阅集合。
type TickSessionView struct {
	Session      SessionID
	View         SessionView
	Origin       core.ChunkKey
	OriginWanted bool
}

// ViewSnapshot 借用调用方在 tick 期间保持不变的视图 slice。内部只做线性只读
// 查询，不把 runtime 的 map、会话指针或回调带入 entity。
type ViewSnapshot struct {
	entries     []TickSessionView
	single      TickSessionView
	singleValid bool
}

// NewViewSnapshot 构造短命只读视图；调用方在 entity 阶段结束前不得修改 entries。
func NewViewSnapshot(entries []TickSessionView) ViewSnapshot {
	return ViewSnapshot{entries: entries}
}

func singleViewSnapshot(id SessionID, view SessionView) ViewSnapshot {
	return ViewSnapshot{
		single: TickSessionView{Session: id, View: view}, singleValid: true,
	}
}

func (views ViewSnapshot) sessionView(id SessionID) SessionView {
	if views.singleValid && views.single.Session == id {
		return views.single.View
	}
	for index := range views.entries {
		if views.entries[index].Session == id {
			return views.entries[index].View
		}
	}
	return SessionView{}
}

func (views ViewSnapshot) sessionWantsChunk(id SessionID, key core.ChunkKey) bool {
	if views.singleValid && views.single.Session == id {
		return views.single.OriginWanted && views.single.Origin == key
	}
	for index := range views.entries {
		entry := views.entries[index]
		if entry.Session == id {
			return entry.OriginWanted && entry.Origin == key
		}
	}
	return false
}

// NewState 创建唯一的实体状态 owner。
func NewState(seed int64) *State {
	return &State{
		seed:         seed,
		sessions:     make(map[SessionID]*sessionState),
		companions:   make(map[companion.ID]*companionState),
		hostiles:     newHostileSet(),
		hostileLight: newBlockLightScratch(),
	}
}

func (state *State) context(
	realmState *realm.State,
	tick uint64,
	worldTime uint64,
	dayPhaseOffset uint16,
	tunables tuning.Tunables,
	physicsTunables physics.Tunables,
	views ViewSnapshot,
) *engineContext {
	context := state.contextValue(
		realmState,
		tick,
		worldTime,
		dayPhaseOffset,
		tunables,
		physicsTunables,
		views,
	)
	return &context
}

func (state *State) contextValue(
	realmState *realm.State,
	tick uint64,
	worldTime uint64,
	dayPhaseOffset uint16,
	tunables tuning.Tunables,
	physicsTunables physics.Tunables,
	views ViewSnapshot,
) engineContext {
	context := engineContext{
		State:           state,
		realm:           realmState,
		tunables:        tunables,
		physicsTunables: physicsTunables,
		views:           views,
	}
	context.tick.Store(tick)
	context.worldTime.Store(worldTime)
	context.dayPhaseOffset.Store(uint64(dayPhaseOffset))
	return context
}

func (engine *engineContext) sessionView(session *sessionState) SessionView {
	if session == nil {
		return SessionView{}
	}
	return engine.views.sessionView(session.id)
}

func (engine *engineContext) sessionWantsChunk(
	session *sessionState,
	key core.ChunkKey,
) bool {
	return session != nil && engine.views.sessionWantsChunk(session.id, key)
}

func (engine *engineContext) dimension(id core.DimensionID) *Dimension {
	if engine.realm == nil {
		return nil
	}
	return engine.realm.Dimension(id)
}

func (engine *engineContext) DayPhaseOffset() uint16 {
	return uint16(engine.dayPhaseOffset.Load())
}

func (engine *engineContext) WorldTime() uint64 {
	return engine.worldTime.Load()
}

func (engine *engineContext) displayDayPhase() uint16 {
	return core.DisplayDayPhase(engine.worldTime.Load(), engine.DayPhaseOffset())
}

// TakeSubscriptionsDirty 返回实体生命周期是否改变订阅输入，并清除提示位。
func (state *State) TakeSubscriptionsDirty() bool {
	dirty := state.subscriptionsDirty
	state.subscriptionsDirty = false
	return dirty
}
