package entity

import (
	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

// sessionState 只保存玩家与玩法生命周期状态；命令序号、观察中心和区块订阅由
// runtime 单独持有，避免把传输编排混入实体 owner。
type sessionState struct {
	id        SessionID
	dimension core.DimensionID
	// viewDistance 是该会话登录协商中声明的期望视距（0 表示未声明）：
	// 注册时随 `PlayerRestore` 进入、此后只读，经 `SessionSubscription`
	// 派生成订阅半径供 runtime 消费；不持久化、不进入快照。
	viewDistance  uint8
	player        *playerState
	container     core.ContainerRef
	viewContainer bool
}

// State 是玩家、伙伴、夜行者、被动牛及其玩法结算状态的唯一 owner。
type State struct {
	seed int64
	// difficulty 是构造期注入的世界难度快照，此后只读：饥饿伤害的致死性、
	// 自然回血的门控与夜行者的生成门控都消费这一份值，权威 tick 不读
	// storage/config。它与 `seed` 同为世界身份的一部分（难度由 metadata
	// 持久化、装配时单次注入），不存在运行时切换写者。
	difficulty core.Difficulty
	// seasonOffset 是季节起点偏移（0..core.YearTicks-1），装配期由
	// `core.SeasonOffsetFromSeed(seed)` 一次写死、此后只读——与 runtime 侧的
	// 同名纪律一致（seed 已持久化，偏移是派生量而非权威状态）。判相位消费点
	// （入睡判定、夜行者生成窗口、白昼灼烧、昼间被动生成）经它把线性显示相
	// 位换算成季节化相位。
	seasonOffset uint64
	sessions     map[SessionID]*sessionState
	companions   map[companion.ID]*companionState
	hostiles     hostileSet
	passives     passiveSet
	// passiveDeaths 是本 tick 死亡结算移除的被动牛 ID 集合（ID 升序、有界
	// ≤32）：发布侧同 tick 取一次投影 despawn 原因位，下次结算先清空，不跨
	// tick 累积。
	passiveDeaths          []uint64
	hostileLight           *blockLightScratch
	subscriptionsDirty     bool
	tramplePending         []tramplePendingCell
	snowFootprintPending   []snowFootprintCell
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

// NewState 创建唯一的实体状态 owner。difficulty 是可选尾参：缺省表达
// normal 档（既有测试夹具零改动），至多传一个；非法值 panic 于构造期——把
// 未定义档位放进权威 tick，比在 tick 内出现未定义分支更难诊断。
func NewState(seed int64, difficulty ...core.Difficulty) *State {
	return &State{
		seed:         seed,
		difficulty:   resolveDifficulty(difficulty),
		seasonOffset: core.SeasonOffsetFromSeed(seed),
		sessions:     make(map[SessionID]*sessionState),
		companions:   make(map[companion.ID]*companionState),
		hostiles:     newHostileSet(),
		passives:     newPassiveSet(),
		hostileLight: newBlockLightScratch(),
	}
}

// resolveDifficulty 把可选难度尾参归一为唯一构造快照：缺省取零值档
// normal（与旧档迁移、未显式指定难度的新世界同语义），多于一个是装配
// 错误，非法值经 `core.Difficulty.Valid` 拒绝。合法性判定只有 core 这一份
// 域值表，这里不定义第二套枚举。
func resolveDifficulty(difficulty []core.Difficulty) core.Difficulty {
	switch {
	case len(difficulty) == 0:
		return core.DifficultyNormal
	case len(difficulty) > 1:
		panic("sim: at most one difficulty may be provided")
	case !difficulty[0].Valid():
		panic("sim: invalid difficulty")
	}
	return difficulty[0]
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

// effectiveDayPhase 返回当前权威视角的季节化显示相位：绝对时间、显示偏移与
// State 持有的季节偏移经 `core.EffectiveDayPhaseAt` 组合——判夜/判昼消费点统
// 一经这里取相位，不得直呼未季节化的线性相位或自建 warp。
func (engine *engineContext) effectiveDayPhase() uint16 {
	return core.EffectiveDayPhaseAt(
		engine.worldTime.Load(), engine.DayPhaseOffset(), engine.seasonOffset,
	)
}

// TakeSubscriptionsDirty 返回实体生命周期是否改变订阅输入，并清除提示位。
func (state *State) TakeSubscriptionsDirty() bool {
	dirty := state.subscriptionsDirty
	state.subscriptionsDirty = false
	return dirty
}
