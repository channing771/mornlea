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
}

type HostileAction struct {
	ID            uint64
	MoveX, MoveZ  float32
	Jump          bool
	AttackTarget  bool
	TargetSession SessionID
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
