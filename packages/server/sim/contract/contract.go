// Package contract 定义模拟包之间传递的不可变值。
package contract

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

type SessionID uint64

type CommandKind uint8

const (
	CommandTrustedObserverCenter CommandKind = iota
	CommandPlayerInput
	CommandPlaceBlock
	CommandResync
	CommandSelectHotbar
	CommandMoveInventoryStack
	CommandOpenFurnace
	CommandCloseFurnace
	CommandMoveFurnaceStack
	CommandDropSelectedItem
	CommandTillSoil
	CommandBoneMeal
	CommandMoveCraftingStack
	CommandTakeCraftingOutput
	CommandInteractDoor
	CommandInteractBed
	CommandCollectWater
	CommandPlaceWater
	// CommandEquipArmor 请求把权威选中快捷栏格中的护甲件穿到对应槽位；载荷
	// 只有序号，目标槽位由件类映射唯一确定。只读写玩家自身状态，不触碰区块。
	CommandEquipArmor
	// CommandMoveStackPartial 请求半组/单件部分数量移动：`Slot`/`ToSlot` 是
	// `StackView` 视图域的统一索引，`Single` 在半组（ceil）与单件（1）两档间
	// 选择。移动数量 MUST 由服务端在结算时按权威来源栈推导，命令载荷不存在
	// 客户端可声明的数量。背包/合成视图只读写玩家自身状态，命令阶段内联
	// 结算；容器视图携带 `Furnace` 容器引用并延迟到区块写相位结算（与
	// `CommandMoveFurnaceStack` 同路径）。
	CommandMoveStackPartial
)

// 分堆命令族（`CommandMoveStackPartial`）的视图域值域。值与协议侧
// `network.StackView*` 常量逐值相同（ingress 直接搬运 wire 的视图字节，
// 漂移由 contract 测试钉住）；sim 按它分派结算相位与值域上界。零值即背包
// 域，其它命令族不携带该字段，零值不会误入分堆路径。
const (
	// StackViewInventory 是背包视图域：统一索引 0..`core.InventorySlots`-1。
	StackViewInventory uint8 = 0
	// StackViewCrafting 是合成统一视图域：网格 0..8、背包 9..44。
	StackViewCrafting uint8 = 1
	// StackViewContainer 是容器统一视图域：箱子 0..62、熔炉 0..38，命令必须
	// 携带与查看关系一致的合法容器引用（`Command.Furnace`）。
	StackViewContainer uint8 = 2
)

type RejectReason uint8

const (
	RejectInvalidRay RejectReason = iota
	RejectNoTarget
	RejectChunkNotReady
	RejectProtectedBlock
	RejectInvalidBlock
	RejectOccupied
	RejectInvalidInput      RejectReason = 6
	RejectPlayerNotReady    RejectReason = 7
	RejectInvalidSlot       RejectReason = 8
	RejectHotbarFull        RejectReason = 9
	RejectDropCapacity      RejectReason = 10
	RejectContainerCapacity RejectReason = 11
	RejectNotFluidSource    RejectReason = 12
	RejectBucketMismatch    RejectReason = 13
	// RejectNotArmor 表示装备互换命令的权威选中快捷栏格未持有可穿戴的护甲件
	//（空格、非护甲物品或多件栈），权威状态零变化。显式取 14：按前值的
	// iota 重复语义会拿到与 `RejectBucketMismatch` 相同的字面量 13，让两个
	// 拒绝原因在 server 的映射 switch 里坍缩成同一个 case。
	RejectNotArmor RejectReason = 14
)

type Command struct {
	Session      SessionID
	Sequence     uint64
	Kind         CommandKind
	Dimension    core.DimensionID
	Center       core.ChunkPos
	Chunk        core.ChunkPos
	HaveRevision uint64
	Slot         uint8
	ToSlot       uint8
	Recipe       core.RecipeID
	Furnace      core.FurnaceRef
	MoveX        int8
	MoveZ        int8
	Jump         bool
	Yaw          float32
	Pitch        float32
	Mining       bool
	Eating       bool
	Sprinting    bool
	Sneaking     bool
	// StackView 是分堆命令族（`CommandMoveStackPartial`）的视图域，取值
	// `StackView*` 三常量之一；其它命令族恒为零值。
	StackView uint8
	// Single 是分堆命令族的数量档位：false = 半组（来源数量向上取整）、
	// true = 单件（1）。数量由服务端在结算时按权威来源栈推导。
	Single bool
}

type GeneratedChunk struct {
	Dimension core.DimensionID
	Pos       core.ChunkPos
	Chunk     *world.Chunk
	Err       error
}

type AcquiredChunk struct {
	Key               core.ChunkKey
	Chunk             *world.Chunk
	Revision          uint64
	PersistedRevision uint64
	NeedsRewrite      bool
	Recovered         bool
	Missing           bool
	Err               error
}

type BlockChange struct {
	Position core.BlockPos
	Block    core.BlockID
}

type ChunkChangeBatch struct {
	Dimension    core.DimensionID
	Chunk        core.ChunkPos
	BaseRevision uint64
	NewRevision  uint64
	Changes      []BlockChange
}

type Rejection struct {
	Session  SessionID
	Sequence uint64
	Reason   RejectReason
}

type PlacementSuccess struct {
	Session  SessionID
	Sequence uint64
}

type ResyncRequest struct {
	Session      SessionID
	Sequence     uint64
	Dimension    core.DimensionID
	Chunk        core.ChunkPos
	HaveRevision uint64
}

type CombatHit struct {
	Session    SessionID
	Damage     uint8
	TargetKind core.CombatTargetKind
}

type MiningUpdate struct {
	Active        bool
	Target        core.BlockPos
	ProgressTicks uint16
	RequiredTicks uint16
	Harvestable   bool
}

type PlayerUpdate struct {
	Session           SessionID
	Dimension         core.DimensionID
	ViewCenter        core.ChunkPos
	State             physics.State
	Yaw, Pitch        float32
	LastInputSequence uint64
	Ready             bool
	Reset             bool
	Mining            MiningUpdate
	Health            uint8
	Oxygen            uint16
	Hunger            uint8
	SaturationZero    bool
	DayPhaseOffset    uint16
	WorldTimeTicks    uint64
	// WeatherKind 是本 tick 结束时的权威天气（0=晴、1=雨、2=雷暴）：
	// 世界单值，同一 tick 发给所有玩家的更新里完全一致。
	WeatherKind core.WeatherKind
	// Season 与 SeasonProgress 是本 tick 结束时的季节派生单值（0..3 枚举与
	// 0..255 量化进度），由 runtime 按（绝对时间、seed 派生的季节偏移）在
	// Step 尾部求出后按人复制，与 WeatherKind 同批次；它们不是权威状态，
	// 接收方重放（时间+seed）必然重现。
	Season         core.Season
	SeasonProgress uint8
	// Temperature 是按本份更新里的玩家 Position.Y、当 tick 天气与季节相位
	// （`core.TemperatureAt`）求得的观察温度（℃）。就近取整（half away
	// from zero——.5 恰值远离零）后收窄为 int8，域由 core 在源头 clamp 到
	// [-40,45]，无二次裁剪。逐人求值、逐人不可变。
	Temperature int8
	// ArmorPoints 是四槽已装备护甲的完好件点数投影（`core.ArmorPoints`，
	// 0..`core.MaxArmorPoints`），每次发布时从权威装备区现算，损坏件计 0。
	ArmorPoints uint8
}

type CompanionUpdate struct {
	ID         companion.ID
	Dimension  core.DimensionID
	State      physics.State
	Yaw, Pitch float32
	Reset      bool
	Mining     MiningUpdate
}

type InventoryUpdate struct {
	Session   SessionID
	Inventory core.Inventory
}

type FurnaceUpdate struct {
	Session       SessionID
	Furnace       core.FurnaceRef
	Input         core.ItemStack
	Fuel          core.ItemStack
	Output        core.ItemStack
	ProgressTicks uint8
	BurnTicks     uint16
}

type FurnaceEnd struct {
	Session SessionID
	Furnace core.FurnaceRef
}

type ChestUpdate struct {
	Session SessionID
	Chest   core.ContainerRef
	Items   [core.ChestSlots]core.ItemStack
}

type CraftingUpdate struct {
	Session SessionID
	Size    uint8
	Slots   [core.CraftingGridSlots]core.ItemStack
	Output  core.ItemStack
}

type TickResult struct {
	Acquire            []core.ChunkKey
	Generate           []core.ChunkKey
	Forget             map[SessionID][]core.ChunkKey
	Ready              []core.ChunkKey
	Changes            []ChunkChangeBatch
	Rejected           []Rejection
	PlacementSuccesses []PlacementSuccess
	Resync             []ResyncRequest
	Players            []PlayerUpdate
	Companions         []CompanionUpdate
	Inventories        []InventoryUpdate
	Furnaces           []FurnaceUpdate
	FurnaceEnds        []FurnaceEnd
	Chests             []ChestUpdate
	Craftings          []CraftingUpdate
	CombatHits         []CombatHit
	Tick               uint64
	WorldTimeTicks     uint64
	// WeatherKind 是本 tick 结束时的权威天气：发布侧按人复制进每份
	// `PlayerUpdate`，实体结算不消费它。
	WeatherKind core.WeatherKind
	// Season 与 SeasonProgress 是本 tick 结束时的季节派生单值：与
	// `WeatherKind` 同为世界单值，由 runtime 在 Step 尾部从（绝对时间、
	// seed 派生偏移）确定性求出，发布侧按人复制；实体结算不消费它们。
	// 发送成功后连同整个 TickResult 视为不可变。
	Season         core.Season
	SeasonProgress uint8
}

type PlayerLocation struct {
	Dimension core.DimensionID
	Position  mgl32.Vec3
}

type PlayerRestore struct {
	Current        *PlayerLocation
	Safe           *PlayerLocation
	Yaw, Pitch     float32
	SpawnDimension core.DimensionID
	SpawnAnchor    core.ChunkPos
	// ViewDistance 是该会话在登录协商中声明的期望视距（v40 `LoginStart`
	// 域内值 2..64）；0 表示未声明（未经登录协商的注册路径）。它是会话
	// 协商事实而非存档状态：不持久化、不进入快照，仅在注册时被换算为
	// 订阅半径（声明 +1，按引擎视界上界钳制）后即完成使命。
	ViewDistance uint8
	Inventory    core.Inventory
	// Armor 是四槽已装备护甲（按 `core.ArmorSlot` 槽位顺序），随存档跨重启
	// 保留；缺失路径（新玩家、只给锚点的注册）为零值即全空。损坏形态以
	// 「数量 1、耐久 0」原地表达。
	Armor            [core.ArmorSlotCount]core.ItemStack
	Health           uint8
	Hunger           uint8
	SaturationMilli  uint16
	ExhaustionMilli  uint16
	HasHunger        bool
	RespawnPresent   bool
	RespawnPosition  [3]float32
	RespawnDimension core.DimensionID
}

type PlayerSnapshot struct {
	Current    PlayerLocation
	Yaw, Pitch float32
	Safe       *PlayerLocation
	Inventory  core.Inventory
	// Armor 是四槽已装备护甲（按 `core.ArmorSlot` 槽位顺序），持久化路径是
	// 它跨重启保留的唯一通道；漏进快照之外会在重登时静默落回空装备。
	Armor            [core.ArmorSlotCount]core.ItemStack
	Health           uint8
	Hunger           uint8
	SaturationMilli  uint16
	ExhaustionMilli  uint16
	RespawnPresent   bool
	RespawnPosition  [3]float32
	RespawnDimension core.DimensionID
}

type CompanionRestore struct {
	ID             companion.ID
	Body           *companion.Body
	SpawnDimension core.DimensionID
	SpawnAnchor    core.ChunkPos
}

type CompanionActionKind uint8

const (
	CompanionActionMove CompanionActionKind = iota + 1
	CompanionActionMineHold
	CompanionActionMineRelease
	CompanionActionPlace
)

type CompanionAction struct {
	ID     companion.ID
	Kind   CompanionActionKind
	Input  physics.Input
	Target core.BlockPos
	Block  core.BlockID
}

type DropSnapshot struct {
	ID         core.DropID
	BlockIndex uint32
	Item       core.ItemID
	Count      uint8
	Durability uint16
}

const MaxSessionDrops = core.MaxSessionDrops

type SaveMode uint8

const (
	SaveUrgent SaveMode = iota
	SaveAll
)

type ChunkSaveSnapshot struct {
	Key            core.ChunkKey
	Revision       uint64
	EstimatedBytes int
	Chunk          *world.Chunk
}

type PersistedChunk struct {
	Key      core.ChunkKey
	Revision uint64
}

type PersistenceStats struct {
	DirtyChunks    int
	EstimatedBytes int64
	InFlightChunks int
	UnloadWaiting  int
}

type ChunkState uint8

const (
	ChunkAbsent ChunkState = iota
	ChunkLoading
	ChunkGenerating
	ChunkReady
	ChunkFailed
	ChunkUnloading
)

type ChunkInfo struct {
	State                ChunkState
	Revision             uint64
	PersistedRevision    uint64
	SaveInFlightRevision uint64
	Err                  error
}

const HostileAttackRange = float32(1.8)

// ProjectileSnapshot 是一条在飞投射物的权威投影：非零稳定 ID、弹种（0=骨刺、
// 1=箭，与协议 v43 的 kind 字节同值）、所在维度、位置与速度。发布侧消费本投影
// 组装按会话订阅的 spawn/state 批次（despawn 由「镜像有而截面无」的差异判据
// 派生，与敌怪发布同形）。全部字段为值语义，跨 goroutine 发送成功后视为不可变。
type ProjectileSnapshot struct {
	ID        uint64
	Kind      uint8
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Velocity  mgl32.Vec3
}

// 敌怪 kind 值域（与协议 v43 hostile record 尾部 kind 字节、hostile_mobs v2
// 存档记录的 Kind 字段共用同一映射）：0 是夜行者（近战追击），1 是掷骨者
// （远程投掷骨刺）。新增敌怪类别必须同步扩展协议、存档与引擎三侧值域。
const (
	HostileKindNightwalker uint8 = 0
	HostileKindBoneThrower uint8 = 1
)

// HostileMob 是一只敌怪（任一 kind）的权威投影：非零稳定 ID、所在维度、
// 物理体、朝向、生命、三个 20-tick 周期冷却与追逐事实。`Kind` 随记录持久化
// 并随线上消息携带；`ShootCooldown` 是掷骨者射击冷却的瞬态投影（0 = 就绪），
// 仅供编排层的射击决策消费，永不落盘。全部字段为值语义，跨 goroutine 发送
// 成功后视为不可变。
type HostileMob struct {
	ID              uint64
	Dimension       core.DimensionID
	State           physics.State
	Yaw             float32
	Health          uint8
	AttackCooldown  uint8
	HurtCooldown    uint8
	BurnCooldown    uint8
	HasTarget       bool
	PlayerID        core.PlayerID
	NextRepathTicks uint64
	DistantTicks    uint16
	Kind            uint8
	ShootCooldown   uint8
}

// HostileAction 是服务端编排层在 tick 边界提交给一只敌怪的本 tick 意图。
// 移动以世界轴分量表达（与玩家/伙伴输入同界 [-1,1]），攻击意图携带目标
// 会话；`RangedAttack` 为真表示本意图是掷骨者的远程射击——`AimX/Y/Z` 是
// 归一化的瞄准基准方向（目标眼位 − 掷骨者眼位），散布由引擎侧确定性求值。
// 每个 ID 每 tick 取最早的一条合法意图，重复与非法载荷确定性丢弃。
type HostileAction struct {
	ID            uint64
	MoveX, MoveZ  float32
	Jump          bool
	AttackTarget  bool
	TargetSession SessionID
	// RangedAttack 是射击意图判别位：为真时本意图不携带移动语义，冷却/
	// 存活/kind 校验在引擎侧结算点统一执行。
	RangedAttack     bool
	AimX, AimY, AimZ float32
}

// PassiveMob 是一头被动牛的权威身体事实：稳定非零身份、所在维度、物理体、
// 朝向与生命。全部字段为值语义，跨 goroutine 发送成功后视为不可变；逃跑
// 计时、出生区块等运行时派生物不进本类型（见被动存档域的独立记录），重载后
// 由恢复入口重新锚定。
//
// `Grazing` 是吃草事件的瞬态呈现位：置位表示该牛正在低头，仅供服务端发布
// 与客户端位姿使用。它永不落盘（存档转换器不得拷贝它），永不进入生成与
// 恢复入口（恢复时恒为清位，重启后事件自然消失）。
type PassiveMob struct {
	ID        uint64
	Dimension core.DimensionID
	State     physics.State
	Yaw       float32
	Health    uint8
	Grazing   bool
}
