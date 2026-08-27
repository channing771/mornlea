package sim

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
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
	// CommandTillSoil 请求把视线内的泥土或草翻成耕地。只带朝向：目标格由权威
	// 射线决定，作用的锄头取权威选中的快捷栏格。CommandKind 只追加不重排。
	CommandTillSoil
	// CommandBoneMeal 请求用骨粉催熟视线内的作物。只带朝向：目标格由权威
	// 射线决定，作用的骨粉取权威选中的快捷栏格。CommandKind 只追加不重排。
	CommandBoneMeal
	// CommandMoveCraftingStack 在合成网格与背包之间执行一次两次点击整堆移动。
	// `Slot`/`ToSlot` 是统一视图格（网格 0..8、背包 9..44），语义见 crafting.go
	// 的 `applyMoveCraftingStack`。
	CommandMoveCraftingStack
	// CommandTakeCraftingOutput 请求取出当前网格匹配派生的完整产物；只携带
	// 序号，产物、扣料与容量全部由权威侧决定。
	CommandTakeCraftingOutput
)

// LookDirection 把玩家 look 角转换为单位方向；yaw=0、pitch=0 朝向 -Z。
func LookDirection(yaw, pitch float32) mgl32.Vec3 {
	cosPitch := float32(math.Cos(float64(pitch)))
	return mgl32.Vec3{
		-float32(math.Sin(float64(yaw))) * cosPitch,
		float32(math.Sin(float64(pitch))),
		-float32(math.Cos(float64(yaw))) * cosPitch,
	}
}

type RejectReason uint8

const (
	RejectInvalidRay RejectReason = iota
	RejectNoTarget
	RejectChunkNotReady
	RejectProtectedBlock
	RejectInvalidBlock
	RejectOccupied
	RejectInvalidInput   RejectReason = 6
	RejectPlayerNotReady RejectReason = 7
	RejectInvalidSlot    RejectReason = 8
	RejectHotbarFull     RejectReason = 9
	RejectDropCapacity   RejectReason = 10
	// RejectContainerCapacity 表示区块某类容器（熔炉或箱子）的固定槽位已经用尽。
	RejectContainerCapacity RejectReason = 11
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
	// Eating 是 CommandPlayerInput 携带的持续进食意图，对应协议 v24 的
	// `PlayerInput.Eating`；与 `Mining` 同形，只表达"按住了进食键"。
	Eating bool
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

// PlacementSuccess 标记某会话的玩家放置命令已原子完成世界
// 写入和恰减一件物品。它只是当 tick 的出口事实，不参与后续模拟。
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

type TickResult struct {
	Acquire  []core.ChunkKey
	Generate []core.ChunkKey
	Forget   map[SessionID][]core.ChunkKey
	Ready    []core.ChunkKey
	Changes  []ChunkChangeBatch
	Rejected []Rejection
	// PlacementSuccesses 按命令的确定性处理顺序列出本 tick 的
	// 玩家放置成功；数量受本 tick 已摄取命令数的既有上界限制。
	PlacementSuccesses []PlacementSuccess
	Resync             []ResyncRequest
	Players            []PlayerUpdate
	Companions         []CompanionUpdate
	Inventories        []InventoryUpdate
	Furnaces           []FurnaceUpdate
	FurnaceEnds        []FurnaceEnd
	// Chests 是本 tick 发给箱子查看者的完整权威状态；关闭通知与熔炉共用 FurnaceEnds，
	// 因为 ContainerRef 本身携带 Kind，一份关闭列表足以表达两种容器。
	Chests []ChestUpdate
	// Craftings 是本 tick 发给网格所属玩家的完整权威合成网格状态
	//（latest-wins、不广播，见 crafting.go 的 `publishCraftings`）。
	Craftings []CraftingUpdate
	Tick      uint64
	// WorldTimeTicks 是本 tick 结束时的权威绝对世界时间。
	WorldTimeTicks uint64
}

// FurnaceUpdate 是本 tick 发给某个查看者的完整权威熔炉状态。
type FurnaceUpdate struct {
	Session       SessionID
	Furnace       core.FurnaceRef
	Input         core.ItemStack
	Fuel          core.ItemStack
	Output        core.ItemStack
	ProgressTicks uint8
	BurnTicks     uint16
}

// FurnaceEnd 通知某个查看者其容器引用已经失效；熔炉与箱子共用同一份关闭通知，
// 因为 core.FurnaceRef 是 core.ContainerRef 的类型别名，本身携带 Kind。
type FurnaceEnd struct {
	Session SessionID
	Furnace core.FurnaceRef
}

// ChestUpdate 是本 tick 发给某个查看者的完整权威箱子状态。
type ChestUpdate struct {
	Session SessionID
	Chest   core.ContainerRef
	Items   [core.ChestSlots]core.ItemStack
}
