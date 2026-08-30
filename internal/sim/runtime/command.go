package runtime

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/sim/contract"
)

// 根包在拆分期间仅供尚未迁走的内部实现使用这些别名。
type (
	SessionID           = contract.SessionID
	CommandKind         = contract.CommandKind
	RejectReason        = contract.RejectReason
	Command             = contract.Command
	GeneratedChunk      = contract.GeneratedChunk
	AcquiredChunk       = contract.AcquiredChunk
	BlockChange         = contract.BlockChange
	ChunkChangeBatch    = contract.ChunkChangeBatch
	Rejection           = contract.Rejection
	PlacementSuccess    = contract.PlacementSuccess
	ResyncRequest       = contract.ResyncRequest
	CombatHit           = contract.CombatHit
	TickResult          = contract.TickResult
	FurnaceUpdate       = contract.FurnaceUpdate
	FurnaceEnd          = contract.FurnaceEnd
	ChestUpdate         = contract.ChestUpdate
	MiningUpdate        = contract.MiningUpdate
	PlayerUpdate        = contract.PlayerUpdate
	PlayerLocation      = contract.PlayerLocation
	PlayerRestore       = contract.PlayerRestore
	PlayerSnapshot      = contract.PlayerSnapshot
	InventoryUpdate     = contract.InventoryUpdate
	CompanionRestore    = contract.CompanionRestore
	CompanionUpdate     = contract.CompanionUpdate
	CompanionActionKind = contract.CompanionActionKind
	CompanionAction     = contract.CompanionAction
	CraftingUpdate      = contract.CraftingUpdate
	DropSnapshot        = contract.DropSnapshot
	SaveMode            = contract.SaveMode
	ChunkSaveSnapshot   = contract.ChunkSaveSnapshot
	PersistedChunk      = contract.PersistedChunk
	PersistenceStats    = contract.PersistenceStats
	ChunkState          = contract.ChunkState
	ChunkInfo           = contract.ChunkInfo
	HostileMob          = contract.HostileMob
	HostileAction       = contract.HostileAction
)

const (
	CommandTrustedObserverCenter = contract.CommandTrustedObserverCenter
	CommandPlayerInput           = contract.CommandPlayerInput
	CommandPlaceBlock            = contract.CommandPlaceBlock
	CommandResync                = contract.CommandResync
	CommandSelectHotbar          = contract.CommandSelectHotbar
	CommandMoveInventoryStack    = contract.CommandMoveInventoryStack
	CommandOpenFurnace           = contract.CommandOpenFurnace
	CommandCloseFurnace          = contract.CommandCloseFurnace
	CommandMoveFurnaceStack      = contract.CommandMoveFurnaceStack
	CommandDropSelectedItem      = contract.CommandDropSelectedItem
	CommandTillSoil              = contract.CommandTillSoil
	CommandBoneMeal              = contract.CommandBoneMeal
	CommandMoveCraftingStack     = contract.CommandMoveCraftingStack
	CommandTakeCraftingOutput    = contract.CommandTakeCraftingOutput
	CommandInteractDoor          = contract.CommandInteractDoor
	CommandInteractBed           = contract.CommandInteractBed
	RejectInvalidRay             = contract.RejectInvalidRay
	RejectNoTarget               = contract.RejectNoTarget
	RejectChunkNotReady          = contract.RejectChunkNotReady
	RejectProtectedBlock         = contract.RejectProtectedBlock
	RejectInvalidBlock           = contract.RejectInvalidBlock
	RejectOccupied               = contract.RejectOccupied
	RejectInvalidInput           = contract.RejectInvalidInput
	RejectPlayerNotReady         = contract.RejectPlayerNotReady
	RejectInvalidSlot            = contract.RejectInvalidSlot
	RejectHotbarFull             = contract.RejectHotbarFull
	RejectDropCapacity           = contract.RejectDropCapacity
	RejectContainerCapacity      = contract.RejectContainerCapacity
	CompanionActionMove          = contract.CompanionActionMove
	CompanionActionMineHold      = contract.CompanionActionMineHold
	CompanionActionMineRelease   = contract.CompanionActionMineRelease
	CompanionActionPlace         = contract.CompanionActionPlace
	MaxSessionDrops              = contract.MaxSessionDrops
	SaveUrgent                   = contract.SaveUrgent
	SaveAll                      = contract.SaveAll
	ChunkAbsent                  = contract.ChunkAbsent
	ChunkLoading                 = contract.ChunkLoading
	ChunkGenerating              = contract.ChunkGenerating
	ChunkReady                   = contract.ChunkReady
	ChunkFailed                  = contract.ChunkFailed
	ChunkUnloading               = contract.ChunkUnloading
	HostileAttackRange           = contract.HostileAttackRange
)

func LookDirection(yaw, pitch float32) mgl32.Vec3 {
	cosPitch := float32(math.Cos(float64(pitch)))
	return mgl32.Vec3{
		-float32(math.Sin(float64(yaw))) * cosPitch,
		float32(math.Sin(float64(pitch))),
		-float32(math.Cos(float64(yaw))) * cosPitch,
	}
}
