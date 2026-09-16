package runtime

import (
	"errors"
	"fmt"
	"slices"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// `MirrorChanges` identifies which host-neutral session mirrors were touched by a message.
// Presentation hosts use these facts to invalidate their own views without putting UI, cursor,
// audio-device, renderer, or platform work inside `runtime`.
type MirrorChanges uint32

const (
	MirrorChangeWorld MirrorChanges = 1 << iota
	MirrorChangeInventory
	MirrorChangeCrafting
	MirrorChangeChest
	MirrorChangeFurnace
	MirrorChangeChat
	MirrorChangeItemDrops
	MirrorChangeRemotePlayers
	MirrorChangeCompanions
	MirrorChangeHostiles
	MirrorChangePassives
	MirrorChangeProjectiles
	// `MirrorChangeBlockChangesApplied` distinguishes an accepted block delta from a stale,
	// missing-base, or already-desynced delta for host-side confirmation feedback.
	MirrorChangeBlockChangesApplied
)

// `Has` reports whether all requested change flags are present.
func (changes MirrorChanges) Has(requested MirrorChanges) bool {
	return changes&requested == requested
}

// `ContainerTransition` describes only the confirmed container-view lifecycle implied by server
// messages. The host decides how that transition affects cursor capture or concrete UI.
type ContainerTransition uint8

const (
	ContainerTransitionNone ContainerTransition = iota
	ContainerTransitionOpenFurnace
	ContainerTransitionOpenChest
	ContainerTransitionOpenCrafting
	ContainerTransitionClose
)

// `MessageOutcome` is the deterministic host-neutral result of one dequeued server message.
// `Message` remains available when `Handled` is false so staged extraction cannot discard a
// message class that a later runtime task still owns.
type MessageOutcome struct {
	Message           network.ServerMessage
	Handled           bool
	Changes           MirrorChanges
	Container         ContainerTransition
	World             client.MirrorUpdate
	PredictionChanged bool
	Reconcile         ReconcileResult
	Prediction        PredictionSnapshot
}

// `MirrorState` is a copied, read-only view of every non-world authoritative mirror owned by the
// session. Slice fields never alias runtime-owned buffers, so hosts may retain the value across a
// later message drain or session reset.
type MirrorState struct {
	Inventory          core.Inventory
	InventoryConfirmed bool
	Crafting           network.CraftingState
	CraftingConfirmed  bool
	Chest              network.ChestState
	ChestOpen          bool
	Furnace            network.FurnaceState
	FurnaceOpen        bool
	Chat               []network.ChatEvent
	ItemDrops          []client.ItemDropPresentation
	RemotePlayers      []client.RemotePresentation
	Companions         []client.CompanionPresentation
	Hostiles           []client.HostilePresentation
	Passives           []client.PassivePresentation
	Projectiles        []client.ProjectilePresentation
}

// `WorldChunkState` is the immutable host-visible identity of one loaded mirror chunk. Chunk block
// storage remains runtime-owned and is converted to bounded world batches by later runtime work.
type WorldChunkState struct {
	Revision uint64
	Desynced bool
}

// `sessionMirrors` is the complete authoritative mirror set owned by one runtime session.
// It is intentionally private: hosts consume later immutable presentation snapshots rather than
// mutating these mirrors around the runtime's validation and reset discipline.
type sessionMirrors struct {
	world         *client.Mirror
	inventory     client.InventoryMirror
	crafting      client.CraftingMirror
	chest         client.ChestMirror
	furnace       client.FurnaceMirror
	chat          client.ChatEvents
	itemDrops     *client.ItemDrops
	remotePlayers *client.RemotePlayers
	companions    client.Companions
	hostiles      client.Hostiles
	passives      client.Passives
	projectiles   client.Projectiles
}

func newSessionMirrors() *sessionMirrors {
	return &sessionMirrors{
		world:         client.NewMirror(),
		itemDrops:     client.NewItemDrops(),
		remotePlayers: client.NewRemotePlayers(),
	}
}

// `DrainMessages` performs at most `budget` non-blocking receiver polls and appends one ordered
// outcome per dequeued message. Remaining receiver work is deliberately left for a later frame.
func (runtime *Runtime) DrainMessages(dst []MessageOutcome, budget int) ([]MessageOutcome, error) {
	if runtime == nil || runtime.receiver == nil || budget <= 0 {
		return dst, nil
	}
	for range budget {
		message, ok := runtime.receiver.TryRecv()
		if !ok {
			return dst, nil
		}
		outcome, err := runtime.ApplyMessage(message)
		if err != nil {
			return dst, fmt.Errorf("runtime: apply server message %T: %w", message, err)
		}
		dst = append(dst, outcome)
	}
	return dst, nil
}

// `ApplyMessage` validates and applies one platform-independent non-player-state message route.
// Unknown routes are preserved as unhandled outcomes for later staged extraction.
func (runtime *Runtime) ApplyMessage(message network.ServerMessage) (MessageOutcome, error) {
	outcome := MessageOutcome{Message: message}
	if runtime == nil {
		return outcome, errors.New("runtime: nil runtime")
	}
	if message == nil {
		return outcome, errors.New("runtime: nil server message")
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return outcome, errors.New("runtime: client session is closed")
	}
	if runtime.mirrors == nil {
		runtime.mirrors = newSessionMirrors()
	}
	return runtime.applyMessageLocked(message)
}

func (runtime *Runtime) applyMessageLocked(message network.ServerMessage) (MessageOutcome, error) {
	outcome := MessageOutcome{Message: message, Handled: true}
	mirrors := runtime.mirrors
	switch message := message.(type) {
	case network.PlayerState:
		return runtime.applyPlayerStateLocked(outcome, message)
	case *network.PlayerState:
		if message == nil {
			return outcome, errors.New("runtime: nil player state")
		}
		return runtime.applyPlayerStateLocked(outcome, *message)
	case network.InventoryState:
		if err := mirrors.inventory.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeInventory
		return outcome, nil
	case network.CraftingState:
		previous, confirmed := mirrors.crafting.State()
		if err := mirrors.crafting.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeCrafting
		switch {
		case message.Size == 3 && (!confirmed || previous.Size != 3):
			if _, opened := mirrors.furnace.State(); opened {
				outcome.Changes |= MirrorChangeFurnace
			}
			if _, opened := mirrors.chest.State(); opened {
				outcome.Changes |= MirrorChangeChest
			}
			mirrors.furnace.Reset()
			mirrors.chest.Reset()
			outcome.Container = ContainerTransitionOpenCrafting
		case confirmed && previous.Size == 3 && message.Size != 3:
			outcome.Container = ContainerTransitionClose
		}
		return outcome, nil
	case network.FurnaceState:
		if err := mirrors.furnace.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeFurnace
		if _, opened := mirrors.chest.State(); opened {
			outcome.Changes |= MirrorChangeChest
		}
		mirrors.chest.Reset()
		outcome.Container = ContainerTransitionOpenFurnace
		return outcome, nil
	case network.ChestState:
		if err := mirrors.chest.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeChest
		if _, opened := mirrors.furnace.State(); opened {
			outcome.Changes |= MirrorChangeFurnace
		}
		mirrors.furnace.Reset()
		outcome.Container = ContainerTransitionOpenChest
		return outcome, nil
	case network.ContainerClosed:
		furnace, furnaceOpened := mirrors.furnace.Ref()
		chest, chestOpened := mirrors.chest.Ref()
		if err := mirrors.furnace.Close(message); err != nil {
			return outcome, err
		}
		if err := mirrors.chest.Close(message); err != nil {
			return outcome, err
		}
		if furnaceOpened && furnace == message.Container {
			outcome.Changes |= MirrorChangeFurnace
			outcome.Container = ContainerTransitionClose
		}
		if chestOpened && chest == message.Container {
			outcome.Changes |= MirrorChangeChest
			outcome.Container = ContainerTransitionClose
		}
		return outcome, nil
	case network.ChatEvent:
		if err := mirrors.chat.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeChat
		return outcome, nil
	case network.ItemDropUpserts, network.ItemDropRemoves:
		if err := mirrors.itemDrops.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeItemDrops
		return outcome, nil
	case network.RemotePlayerSpawn, *network.RemotePlayerSpawn,
		network.RemotePlayerDespawn, *network.RemotePlayerDespawn,
		network.RemotePlayerStates, *network.RemotePlayerStates:
		if err := mirrors.remotePlayers.Apply(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeRemotePlayers
		return outcome, nil
	case network.CompanionSpawn:
		if err := mirrors.companions.ApplySpawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeCompanions
		return outcome, nil
	case network.CompanionStates:
		if err := mirrors.companions.ApplyStates(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeCompanions
		return outcome, nil
	case network.CompanionDespawn:
		if err := mirrors.companions.ApplyDespawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeCompanions
		return outcome, nil
	case network.HostileSpawn:
		if err := mirrors.hostiles.ApplySpawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeHostiles
		return outcome, nil
	case network.HostileState:
		if err := mirrors.hostiles.ApplyStates(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeHostiles
		return outcome, nil
	case network.HostileDespawn:
		if err := mirrors.hostiles.ApplyDespawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeHostiles
		return outcome, nil
	case network.PassiveSpawn:
		if err := mirrors.passives.ApplySpawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangePassives
		return outcome, nil
	case network.PassiveState:
		if err := mirrors.passives.ApplyStates(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangePassives
		return outcome, nil
	case network.PassiveDespawn:
		if err := mirrors.passives.ApplyDespawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangePassives
		return outcome, nil
	case network.ProjectileSpawn:
		if err := mirrors.projectiles.ApplySpawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeProjectiles
		return outcome, nil
	case network.ProjectileState:
		if err := mirrors.projectiles.ApplyStates(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeProjectiles
		return outcome, nil
	case network.ProjectileDespawn:
		if err := mirrors.projectiles.ApplyDespawn(message); err != nil {
			return outcome, err
		}
		outcome.Changes = MirrorChangeProjectiles
		return outcome, nil
	}

	if isWorldMirrorMessage(message) {
		flags := MirrorChangeWorld
		if blockChanges, ok := blockChangesValue(message); ok {
			chunk, loaded := mirrors.world.Chunk(blockChanges.Dimension, blockChanges.Chunk)
			if loaded && !chunk.Desynced && chunk.Revision == blockChanges.BaseRevision && blockChanges.NewRevision > chunk.Revision {
				flags |= MirrorChangeBlockChangesApplied
			}
		}
		update, err := mirrors.world.Apply(message)
		if err != nil {
			return outcome, err
		}
		if update.Resync != nil {
			// Resync and player input share one session-wide sequence. The host may send this
			// already-numbered immutable request but never allocates protocol identity itself.
			update.Resync.Sequence = runtime.nextSequenceLocked()
		}
		outcome.Changes = flags
		outcome.World = update
		return outcome, nil
	}

	outcome.Handled = false
	return outcome, nil
}

func isWorldMirrorMessage(message network.ServerMessage) bool {
	switch message.(type) {
	case network.ChunkSnapshot, *network.ChunkSnapshot,
		network.BlockChanges, *network.BlockChanges,
		network.ForgetChunks, *network.ForgetChunks,
		network.CommandRejected, *network.CommandRejected:
		return true
	default:
		return false
	}
}

func blockChangesValue(message network.ServerMessage) (network.BlockChanges, bool) {
	switch message := message.(type) {
	case network.BlockChanges:
		return message, true
	case *network.BlockChanges:
		if message != nil {
			return *message, true
		}
	}
	return network.BlockChanges{}, false
}

// `ResetMirrors` establishes a fresh session epoch without retaining confirmed mirrors, prediction,
// input intent, or sequence state. Host-owned presentation and device state remains outside this
// reset boundary.
func (runtime *Runtime) ResetMirrors() {
	if runtime == nil {
		return
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	runtime.resetSessionLocked()
}

func (runtime *Runtime) resetSessionLocked() {
	// Replace session-owned objects rather than mutating published copies. Hosts may safely retain
	// earlier `MirrorState` and `PredictionSnapshot` values across an epoch transition.
	runtime.mirrors = newSessionMirrors()
	runtime.predictor = client.NewPredictor()
	runtime.semanticInput = SemanticInput{}
	runtime.sequence = 0
	runtime.playerTick = 0
	// Reset removes the derived eye pose and target inputs. Keep mode and its edge latch because
	// they are local presentation preference state, matching the existing client across worlds.
	runtime.cameraYaw = 0
	runtime.cameraPitch = 0
	runtime.cameraTargetReset = false
}

// `MirrorState` returns copied confirmed mirror values in deterministic entity order.
func (runtime *Runtime) MirrorState() MirrorState {
	if runtime == nil {
		return MirrorState{}
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.mirrors == nil {
		return MirrorState{}
	}
	mirrors := runtime.mirrors
	state := MirrorState{}
	state.Inventory, state.InventoryConfirmed = mirrors.inventory.State()
	state.Crafting, state.CraftingConfirmed = mirrors.crafting.State()
	state.Chest, state.ChestOpen = mirrors.chest.State()
	state.Furnace, state.FurnaceOpen = mirrors.furnace.State()
	state.Chat = mirrors.chat.Events(nil)
	state.ItemDrops = slices.Clone(mirrors.itemDrops.Presentations())
	state.RemotePlayers = mirrors.remotePlayers.AppendPresentations(nil)
	state.Companions = mirrors.companions.AppendPresentations(nil)
	state.Hostiles = mirrors.hostiles.AppendPresentations(nil)
	state.Passives = mirrors.passives.AppendPresentations(nil)
	state.Projectiles = mirrors.projectiles.AppendPresentations(nil)
	return state
}

// `WorldChunkState` returns a copied revision/desync view without exposing mutable chunk storage.
func (runtime *Runtime) WorldChunkState(dimension core.DimensionID, position core.ChunkPos) (WorldChunkState, bool) {
	if runtime == nil {
		return WorldChunkState{}, false
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.mirrors == nil || runtime.mirrors.world == nil {
		return WorldChunkState{}, false
	}
	chunk, loaded := runtime.mirrors.world.Chunk(dimension, position)
	if !loaded {
		return WorldChunkState{}, false
	}
	return WorldChunkState{Revision: chunk.Revision, Desynced: chunk.Desynced}, true
}
