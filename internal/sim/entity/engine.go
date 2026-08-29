package entity

import (
	"sync"
	"sync/atomic"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/world"
)

type stepPhase uint8

type Clock interface {
	C() <-chan struct{}
	Stop()
}

type sessionState struct {
	lastSequence                uint64
	lastTrustedObserverSequence uint64
	trustedObserver             bool
	hasView                     bool
	dimension                   core.DimensionID
	center                      core.ChunkPos
	wanted                      map[core.ChunkKey]struct{}
	player                      *playerState
	container                   core.ContainerRef
	viewContainer               bool
}

type Engine struct {
	viewRadius int
	seed       int64
	sessions   map[SessionID]*sessionState
	companions map[companion.ID]*companionState
	hostiles   hostileSet
	hostileLight *blockLightScratch
	wanted             map[core.ChunkKey]struct{}
	realm              *realm.State
	subscriptionsDirty bool
	inboxMu          sync.Mutex
	commands         []Command
	companionActions []CompanionAction
	hostileActions   []HostileAction
	acquired         []AcquiredChunk
	generated        []GeneratedChunk
	tick             atomic.Uint64
	worldTime        atomic.Uint64
	dayPhaseOffset   atomic.Uint64
	stepPhaseObserver func(stepPhase)
	tunables        tuning.Tunables
	physicsTunables physics.Tunables
}

func NewEngine(viewRadius int, worldTime uint64, seed int64) *Engine {
	if viewRadius < 0 {
		panic("entity: negative view radius")
	}
	realmState := realm.NewState(core.Overworld)
	engine := &Engine{
		viewRadius:   viewRadius,
		seed:         seed,
		realm:        realmState,
		sessions:     make(map[SessionID]*sessionState),
		companions:   make(map[companion.ID]*companionState),
		hostiles:     newHostileSet(),
		hostileLight: newBlockLightScratch(),
		wanted:       make(map[core.ChunkKey]struct{}),
	}
	engine.worldTime.Store(worldTime)
	engine.tunables = tuning.ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	return engine
}

func (engine *Engine) dimension(id core.DimensionID) *Dimension {
	return engine.realm.Dimension(id)
}

func (engine *Engine) SeedForTest() int64 { return engine.seed }
func (engine *Engine) WorldTime() uint64 { return engine.worldTime.Load() }
func (engine *Engine) DayPhaseOffset() uint16 { return uint16(engine.dayPhaseOffset.Load()) }
func (engine *Engine) RestoreDayPhaseOffset(offset uint16) { engine.dayPhaseOffset.Store(uint64(offset)) }
func (engine *Engine) displayDayPhase() uint16 {
	return core.DisplayDayPhase(engine.worldTime.Load(), engine.DayPhaseOffset())
}
func (engine *Engine) SetWorldTimeForTest(ticks uint64) { engine.worldTime.Store(ticks) }
func (engine *Engine) SetDayPhaseOffsetForTest(offset uint16) { engine.dayPhaseOffset.Store(uint64(offset)) }
func (engine *Engine) advanceWorldTime() uint64 { return engine.worldTime.Add(1) }
func (engine *Engine) Enqueue(command Command) {
	engine.inboxMu.Lock()
	engine.commands = append(engine.commands, command)
	engine.inboxMu.Unlock()
}
func (engine *Engine) SubmitGenerated(result GeneratedChunk) {
	engine.inboxMu.Lock()
	engine.generated = append(engine.generated, result)
	engine.inboxMu.Unlock()
}
func (engine *Engine) SubmitAcquired(result AcquiredChunk) {
	engine.inboxMu.Lock()
	engine.acquired = append(engine.acquired, result)
	engine.inboxMu.Unlock()
}
func (engine *Engine) TickCount() uint64 { return engine.tick.Load() }
func (engine *Engine) CloneReadyChunk(key core.ChunkKey) (*world.Chunk, uint64, bool) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return nil, 0, false
	}
	return dimension.CloneReadyChunk(key.Pos)
}
func (engine *Engine) ChunkHash(key core.ChunkKey) ([32]byte, uint64, bool) {
	chunk, revision, ok := engine.CloneReadyChunk(key)
	if !ok {
		return [32]byte{}, 0, false
	}
	return chunk.Hash(), revision, true
}
func (engine *Engine) ChunkInfo(key core.ChunkKey) (ChunkInfo, bool) {
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
