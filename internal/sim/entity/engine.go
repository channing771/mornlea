package entity

import (
	"sync/atomic"

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
	tick            atomic.Uint64
	worldTime       atomic.Uint64
	dayPhaseOffset  atomic.Uint64
	tunables        tuning.Tunables
	physicsTunables physics.Tunables
	views           SessionViews
}

// SessionView 是 runtime 在调用 entity 阶段时提供的只读订阅快照。
type SessionView struct {
	Ready  bool
	Center core.ChunkPos
}

// SessionViews 让 runtime 以只读查询提供当前订阅视图，不复制或导出可变会话。
type SessionViews interface {
	EntitySessionView(SessionID) SessionView
	EntitySessionWantsChunk(SessionID, core.ChunkKey) bool
}

type singleSessionView struct {
	id     SessionID
	view   SessionView
	wanted map[core.ChunkKey]struct{}
}

func (views singleSessionView) EntitySessionView(id SessionID) SessionView {
	if id == views.id {
		return views.view
	}
	return SessionView{}
}

func (views singleSessionView) EntitySessionWantsChunk(
	id SessionID,
	key core.ChunkKey,
) bool {
	if id != views.id {
		return false
	}
	_, wanted := views.wanted[key]
	return wanted
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
	views SessionViews,
) *engineContext {
	context := &engineContext{
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
	if session == nil || engine.views == nil {
		return SessionView{}
	}
	return engine.views.EntitySessionView(session.id)
}

func (engine *engineContext) sessionWantsChunk(
	session *sessionState,
	key core.ChunkKey,
) bool {
	return session != nil && engine.views != nil &&
		engine.views.EntitySessionWantsChunk(session.id, key)
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
