package runtime

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/entity"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/world"
)

const (
	productionTickInterval = 50 * time.Millisecond
	maxCatchUpSteps        = 5
)

type Clock interface {
	C() <-chan time.Time
	Stop()
}

type subscriptionState struct {
	lastSequence                uint64
	lastTrustedObserverSequence uint64
	trustedObserver             bool
	hasView                     bool
	dimension                   core.DimensionID
	center                      core.ChunkPos
	wanted                      map[core.ChunkKey]struct{}
}

type Engine struct {
	viewRadius         int
	seed               int64
	subscriptions      map[SessionID]*subscriptionState
	wanted             map[core.ChunkKey]struct{}
	realm              *realm.State
	entities           *entity.State
	subscriptionsDirty bool
	entityViewScratch  []entity.TickSessionView
	activeChunkScratch []core.ChunkKey

	inboxMu          sync.Mutex
	commands         []Command
	companionActions []CompanionAction
	hostileActions   []HostileAction
	acquired         []AcquiredChunk
	generated        []GeneratedChunk
	tick             atomic.Uint64
	// worldTime 是权威绝对世界时间，只由 simulation owner 在 Step 中推进。
	worldTime atomic.Uint64
	// dayPhaseOffset 是 `core.DisplayDayPhase` 的显示相位偏移（值域钳 0..23999），
	// 只进入显示相位计算，绝不影响 `worldTime` 的推进。唯一写者是全员入睡时的
	// 跳夜结算（sleep.go），单值原子读写让判夜读取点无需额外同步；持久化由世界
	// metadata 批次承接。夜行者的夜间生成窗口与白昼灼烧同样按「拨过偏移后的
	// 显示相位」判定——床睡眠行生产、夜行者行消费，两行只共享这一个窄契约。
	dayPhaseOffset atomic.Uint64

	// stepPhaseObserver 仅供包内测试观测 `Step` 的阶段进入顺序。阶段顺序是规格
	// 契约（完整清单见 `stepPhase` 的常量定义），但各阶段写互不相交的状态、无法
	// 从外部结果观察先后，只能用显式探针锁定；生产代码恒为 nil，nil 判断零开销。
	stepPhaseObserver func(stepPhase)

	// tunables 与 physicsTunables 在每次 Step 入口刷新一次，同一 tick 内全程使用，
	// 保证单个 tick 的所有判定基于同一份参数。
	tunables        tuning.Tunables
	physicsTunables physics.Tunables
}

// NewEngine 创建权威引擎。worldTime 是从 metadata 恢复的绝对世界时间，
// seed 是世界种子（与 worldgen.New 同值，见 Engine.seed 的说明）。
func NewEngine(viewRadius int, worldTime uint64, seed int64) *Engine {
	if viewRadius < 0 {
		panic("sim: negative view radius")
	}
	realmState := realm.NewState(core.Overworld)
	engine := &Engine{
		viewRadius:    viewRadius,
		seed:          seed,
		realm:         realmState,
		entities:      entity.NewState(seed),
		subscriptions: make(map[SessionID]*subscriptionState),
		wanted:        make(map[core.ChunkKey]struct{}),
	}
	engine.worldTime.Store(worldTime)
	// 初始化快照，使未经 Step 就被调用的方法（例如 RegisterPlayer 的出生扫描）
	// 也有可用的参数快照。
	engine.tunables = tuning.ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	return engine
}

func (engine *Engine) dimension(id core.DimensionID) *Dimension {
	return engine.realm.Dimension(id)
}

// SeedForTest 读出构造时传入的世界种子，仅供测试断言 host 接线是否把
// storage.Metadata.Seed 原样传给了 NewEngine（见 Engine.seed 的说明）。
func (engine *Engine) SeedForTest() int64 {
	return engine.seed
}

// WorldTime 返回最近一个完成 tick 的绝对世界时间。
func (engine *Engine) WorldTime() uint64 { return engine.worldTime.Load() }

// DayPhaseOffset 返回当前显示相位偏移（0..23999）。跳夜结算之外恒为构造初值 0。
func (engine *Engine) DayPhaseOffset() uint16 { return uint16(engine.dayPhaseOffset.Load()) }

// RestoreDayPhaseOffset 写入从世界 metadata 恢复的显示相位偏移。它只允许宿主
// 装配阶段在首个权威 tick 之前调用一次：`dayPhaseOffset` 的常规写者是跳夜结算
// （`settleSleepThroughNight`），恢复先于一切命令与 tick，因此不构成第二个
// 并发写者。
func (engine *Engine) RestoreDayPhaseOffset(offset uint16) {
	engine.dayPhaseOffset.Store(uint64(offset))
}

// displayDayPhase 返回当前权威视角下的显示相位：绝对时间与偏移都只经
// `core.DisplayDayPhase` 组合，判夜读取点不得自建算式。
func (engine *Engine) displayDayPhase() uint16 {
	return core.DisplayDayPhase(engine.worldTime.Load(), engine.DayPhaseOffset())
}

// SetWorldTimeForTest 直接写入权威绝对世界时间，仅供测试把世界拨到特定显示相位
// （例如夜间的入睡判定与跳夜结算）。生产路径的时间只能经 `advanceWorldTime`
// 每 tick 恰好 +1，不得有任何旁路写者。
func (engine *Engine) SetWorldTimeForTest(ticks uint64) { engine.worldTime.Store(ticks) }

// SetDayPhaseOffsetForTest 直接写入显示相位偏移，仅供上层包的测试构造「跳夜
// 已结算」的偏移状态来验证持久化接线。生产路径的偏移只能经宿主装配的
// `RestoreDayPhaseOffset`（恢复 metadata）或跳夜结算写入。
func (engine *Engine) SetDayPhaseOffsetForTest(offset uint16) {
	engine.dayPhaseOffset.Store(uint64(offset))
}

// advanceWorldTime 把绝对世界时间推进恰好一个 tick 并返回新值。
func (engine *Engine) advanceWorldTime() uint64 { return engine.worldTime.Add(1) }

// Enqueue 可由 endpoint reader 并发调用。
func (engine *Engine) Enqueue(command Command) {
	engine.inboxMu.Lock()
	engine.commands = append(engine.commands, command)
	engine.inboxMu.Unlock()
}

// SubmitGenerated 可由生成 worker 并发调用，并转移 Chunk 所有权。
func (engine *Engine) SubmitGenerated(result GeneratedChunk) {
	engine.inboxMu.Lock()
	engine.generated = append(engine.generated, result)
	engine.inboxMu.Unlock()
}

// SubmitAcquired 可由区块读取 worker 并发调用，并转移 Chunk 所有权。
func (engine *Engine) SubmitAcquired(result AcquiredChunk) {
	engine.inboxMu.Lock()
	engine.acquired = append(engine.acquired, result)
	engine.inboxMu.Unlock()
}

func (engine *Engine) TickCount() uint64 {
	return engine.tick.Load()
}

func (engine *Engine) CloneReadyChunk(
	key core.ChunkKey,
) (*world.Chunk, uint64, bool) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return nil, 0, false
	}
	return dimension.CloneReadyChunk(key.Pos)
}

func (engine *Engine) ChunkHash(
	key core.ChunkKey,
) ([32]byte, uint64, bool) {
	chunk, revision, ok := engine.CloneReadyChunk(key)
	if !ok {
		return [32]byte{}, 0, false
	}
	return chunk.Hash(), revision, true
}

func (engine *Engine) ChunkInfo(
	key core.ChunkKey,
) (ChunkInfo, bool) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return ChunkInfo{}, false
	}
	info, ok := dimension.Info(key.Pos)
	return ChunkInfo{
		State:                ChunkState(info.State),
		Revision:             info.Revision,
		PersistedRevision:    info.PersistedRevision,
		SaveInFlightRevision: info.SaveInFlightRevision,
		Err:                  info.Err,
	}, ok
}

func (engine *Engine) takeInbox() ([]Command, []AcquiredChunk, []GeneratedChunk) {
	engine.inboxMu.Lock()
	commands := append([]Command(nil), engine.commands...)
	acquired := append([]AcquiredChunk(nil), engine.acquired...)
	generated := append([]GeneratedChunk(nil), engine.generated...)
	engine.commands = engine.commands[:0]
	engine.acquired = engine.acquired[:0]
	engine.generated = engine.generated[:0]
	engine.inboxMu.Unlock()
	return commands, acquired, generated
}
