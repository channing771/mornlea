// Package contract 定义模拟包之间传递的不可变值。
package contract

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
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
	CommandTillSoil
	CommandBoneMeal
	CommandMoveCraftingStack
	CommandTakeCraftingOutput
	CommandInteractDoor
	CommandInteractBed
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
}

type PlayerLocation struct {
	Dimension core.DimensionID
	Position  mgl32.Vec3
}

type PlayerRestore struct {
	Current          *PlayerLocation
	Safe             *PlayerLocation
	Yaw, Pitch       float32
	SpawnDimension   core.DimensionID
	SpawnAnchor      core.ChunkPos
	Inventory        core.Inventory
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
	Current          PlayerLocation
	Yaw, Pitch       float32
	Safe             *PlayerLocation
	Inventory        core.Inventory
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
