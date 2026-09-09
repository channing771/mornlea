package runtime

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/packages/server/sim/entity"
	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
	"github.com/channing771/mornlea/packages/shared/world"
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
	// radius 是该会话的生效订阅半径（方形视距循环边界）：注册时由声明
	// 视距换算并按引擎视界钳制，对账时随实体派生值刷新。trusted observer
	// 与未声明路径恒为引擎视界（缺省即上界）。
	radius int
	wanted map[core.ChunkKey]struct{}
}

type Engine struct {
	// viewRadius 是引擎视界：会话订阅半径的缺省值与钳制上界。声明视距的
	// 会话按 min(声明+1, viewRadius) 生效；trusted observer 与未声明路径
	// 直接沿用它（含 ViewRadius=0 的探针服务端——两类路径同得 0）。
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
	// weatherKind/weatherRemaining 是权威天气时钟（种类与剩余时长）：唯一写者是
	// `StepWithTunables` 尾部的天气推进（与 `advanceWorldTime` 同位置同模式，
	// 见 `advanceWeatherClock`），读者只在 tick 串行路径（发布装配与 `Player`
	// 查询）上串行访问——与无锁的实体状态同纪律，不设原子或互斥。
	weatherKind      core.WeatherKind
	weatherRemaining uint32
	// seasonOffset 是季节起点偏移（0..core.YearTicks-1）：`NewEngine` 装配期
	// 由 `core.SeasonOffsetFromSeed(seed)` 一次写死，此后只读——与
	// weatherKind 同纪律，不设原子或互斥。世界 seed 已由 metadata 持久化，
	// 同 seed 重启偏移必然一致，因此它不落任何持久化字段（派生量而非权威
	// 状态）；季节/温度每 tick 由它加绝对时间确定性重现。
	seasonOffset uint64
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
	realmState.EnsureDimension(core.Depths)
	engine := &Engine{
		viewRadius:    viewRadius,
		seed:          seed,
		seasonOffset:  core.SeasonOffsetFromSeed(seed),
		realm:         realmState,
		entities:      entity.NewState(seed),
		subscriptions: make(map[SessionID]*subscriptionState),
		wanted:        make(map[core.ChunkKey]struct{}),
	}
	engine.worldTime.Store(worldTime)
	// 天气从晴天起步：初始剩余时长按晴段分布掷骰（种子派生、tick 取 0，
	// 与任何完成 tick 号都不碰撞），重放确定。
	engine.weatherKind = core.WeatherClear
	engine.weatherRemaining = rollWeatherDuration(seed, 0, core.WeatherClear)
	// 初始化快照，使未经 Step 就被调用的方法（例如 RegisterPlayer 的出生扫描）
	// 也有可用的参数快照。
	initialTunables := ActiveTickTunables()
	engine.tunables = initialTunables.Simulation
	engine.physicsTunables = initialTunables.Physics
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

// WeatherKind 返回当前权威天气种类。天气时钟只在 tick 串行路径上推进，
// 读者（发布装配、非 tick 查询与持久化快照）同样只在 tick 串行路径或
// 引擎锁保护下读取，与无锁的实体状态同纪律。
func (engine *Engine) WeatherKind() core.WeatherKind { return engine.weatherKind }

// WeatherTicksRemaining 返回当前天气段的剩余权威 tick 数，读纪律与
// `WeatherKind` 相同。
func (engine *Engine) WeatherTicksRemaining() uint32 { return engine.weatherRemaining }

// RestoreWeather 写入从世界 metadata 恢复的权威天气。它只允许宿主装配阶段
// 在首个权威 tick 之前调用一次：`weatherKind`/`weatherRemaining` 的常规写者
// 是 tick 尾部的天气推进，恢复先于一切命令与 tick，因此不构成第二个并发写者。
//
// 越界种类归一为晴天（非 tick 查询路径直接读引擎值下发，不经过推进归一）；
// 剩余时长为零表示旧版本存档未记录天气，按该种类的分布掷骰补一段默认时长
// （种子派生、tick 取 0，与 `NewEngine` 初值同源），使迁移世界与同种子
// 新世界行为一致——零值分支只补时长，不改动已归一的种类。v4 存档写出的
// 剩余时长恒大于零，正常恢复不受此分支影响。
func (engine *Engine) RestoreWeather(kind core.WeatherKind, remaining uint32) {
	if kind > core.WeatherThunder {
		kind = core.WeatherClear
	}
	if remaining == 0 {
		remaining = rollWeatherDuration(engine.seed, 0, kind)
	}
	engine.weatherKind = kind
	engine.weatherRemaining = remaining
}

// DayPhaseOffset 返回当前显示相位偏移（0..23999）。跳夜结算之外恒为构造初值 0。
func (engine *Engine) DayPhaseOffset() uint16 { return uint16(engine.dayPhaseOffset.Load()) }

// RestoreDayPhaseOffset 写入从世界 metadata 恢复的显示相位偏移。它只允许宿主
// 装配阶段在首个权威 tick 之前调用一次：`dayPhaseOffset` 的常规写者是跳夜结算
// （`settleSleepThroughNight`），恢复先于一切命令与 tick，因此不构成第二个
// 并发写者。
func (engine *Engine) RestoreDayPhaseOffset(offset uint16) {
	engine.dayPhaseOffset.Store(uint64(offset))
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

// SetWeatherForTest 直接写入权威天气种类与剩余时长，仅供上层包的测试构造
// 「已推进」的天气状态来验证持久化接线。生产路径的天气只能经宿主装配的
// `RestoreWeather`（恢复 metadata）或 tick 尾部的天气推进写入。
func (engine *Engine) SetWeatherForTest(kind core.WeatherKind, remaining uint32) {
	engine.weatherKind = kind
	engine.weatherRemaining = remaining
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
