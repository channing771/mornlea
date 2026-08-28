// Package realm 持有权威世界状态与区块变更事务。
package realm

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/world"
)

var (
	ErrChunkNotReady   = errors.New("sim: chunk not ready")
	ErrBlockOutOfWorld = errors.New("sim: block outside world height")
)

type ChunkRecord struct {
	State                contract.ChunkState
	Chunk                *world.Chunk
	Revision             uint64
	PersistedRevision    uint64
	NeedsRewrite         bool
	Recovered            bool
	UnloadRequested      bool
	SaveInFlightRevision uint64
	Err                  error
}

func (record *ChunkRecord) Dirty() bool {
	return record.Revision > record.PersistedRevision || record.NeedsRewrite
}

// Dimension 由 State 的单写者 tick 独占，不提供内部锁。
type Dimension struct {
	id      core.DimensionID
	Records map[core.ChunkPos]*ChunkRecord
}

func NewDimension(id core.DimensionID) *Dimension {
	return &Dimension{
		id:      id,
		Records: make(map[core.ChunkPos]*ChunkRecord),
	}
}

// State 持有维度与持久化中的快照状态。
type State struct {
	dimensions    map[core.DimensionID]*Dimension
	inFlightSaves map[core.ChunkKey]persistenceInFlight
}

func NewState(ids ...core.DimensionID) *State {
	state := &State{dimensions: make(map[core.DimensionID]*Dimension)}
	for _, id := range ids {
		state.dimensions[id] = NewDimension(id)
	}
	return state
}

func (state *State) Dimension(id core.DimensionID) *Dimension {
	return state.dimensions[id]
}

// Dimensions 为仍在根 sim 的迁移期调用方提供同一份维度记录。
func (state *State) Dimensions() map[core.DimensionID]*Dimension {
	return state.dimensions
}

// BeginLoading 把 Absent 或 Failed 区块转为 Loading。
func (dimension *Dimension) BeginLoading(pos core.ChunkPos) bool {
	record, exists := dimension.Records[pos]
	if !exists {
		dimension.Records[pos] = &ChunkRecord{State: contract.ChunkLoading}
		return true
	}
	switch record.State {
	case contract.ChunkLoading, contract.ChunkGenerating, contract.ChunkReady, contract.ChunkUnloading:
		return false
	case contract.ChunkFailed:
		*record = ChunkRecord{State: contract.ChunkLoading}
		return true
	case contract.ChunkAbsent:
		panic(fmt.Sprintf(
			"sim: illegal chunk transition %d -> Loading at %+v",
			record.State,
			pos,
		))
	default:
		panic(fmt.Sprintf("sim: unknown chunk state %d at %+v", record.State, pos))
	}
}

func (dimension *Dimension) DropLoading(pos core.ChunkPos) {
	dimension.recordForTransition(pos, contract.ChunkLoading, "Absent")
	delete(dimension.Records, pos)
}

func (dimension *Dimension) MarkGenerating(pos core.ChunkPos) bool {
	record, exists := dimension.Records[pos]
	if !exists || record.State != contract.ChunkLoading {
		return false
	}
	*record = ChunkRecord{State: contract.ChunkGenerating}
	return true
}

// BeginGeneration 把 Absent 或 Failed 区块转为 Generating。
func (dimension *Dimension) BeginGeneration(pos core.ChunkPos) bool {
	record, exists := dimension.Records[pos]
	if !exists {
		dimension.Records[pos] = &ChunkRecord{State: contract.ChunkGenerating}
		return true
	}
	switch record.State {
	case contract.ChunkLoading, contract.ChunkGenerating, contract.ChunkReady, contract.ChunkUnloading:
		return false
	case contract.ChunkFailed:
		*record = ChunkRecord{State: contract.ChunkGenerating}
		return true
	case contract.ChunkAbsent:
		panic(fmt.Sprintf(
			"sim: illegal chunk transition %d -> Generating at %+v",
			record.State,
			pos,
		))
	default:
		panic(fmt.Sprintf("sim: unknown chunk state %d at %+v", record.State, pos))
	}
}

// ApplyGenerated 接管生成结果并以首个未持久修订进入 Ready。
func (dimension *Dimension) ApplyGenerated(pos core.ChunkPos, chunk *world.Chunk) error {
	record := dimension.recordForTransition(pos, contract.ChunkGenerating, "Ready")
	if chunk == nil {
		return errors.New("sim: generated chunk is nil")
	}
	if chunk.Pos != pos {
		return fmt.Errorf("sim: generated chunk position %+v, want %+v", chunk.Pos, pos)
	}
	*record = ChunkRecord{State: contract.ChunkReady, Chunk: chunk, Revision: 1}
	return nil
}

func (dimension *Dimension) ApplyLoaded(
	pos core.ChunkPos,
	chunk *world.Chunk,
	revision uint64,
	persistedRevision uint64,
	needsRewrite bool,
	recovered bool,
) error {
	record := dimension.recordForTransition(pos, contract.ChunkLoading, "Ready")
	if chunk == nil {
		return errors.New("sim: loaded chunk is nil")
	}
	if chunk.Pos != pos {
		return fmt.Errorf("sim: loaded chunk position %+v, want %+v", chunk.Pos, pos)
	}
	if persistedRevision > revision {
		return fmt.Errorf("sim: persisted revision %d exceeds current revision %d at %+v", persistedRevision, revision, pos)
	}
	*record = ChunkRecord{
		State:             contract.ChunkReady,
		Chunk:             chunk,
		Revision:          revision,
		PersistedRevision: persistedRevision,
		NeedsRewrite:      needsRewrite || recovered,
		Recovered:         recovered,
	}
	return nil
}

// MarkFailed 把生成任务的失败结果记录在区块状态中。
func (dimension *Dimension) MarkFailed(pos core.ChunkPos, err error) {
	if err == nil {
		panic("sim: nil generation failure")
	}
	record := dimension.recordForTransition(pos, contract.ChunkGenerating, "Failed")
	*record = ChunkRecord{State: contract.ChunkFailed, Err: err}
}

func (dimension *Dimension) MarkLoadFailed(pos core.ChunkPos, err error) {
	if err == nil {
		panic("sim: nil load failure")
	}
	record := dimension.recordForTransition(pos, contract.ChunkLoading, "Failed")
	*record = ChunkRecord{State: contract.ChunkFailed, Err: err}
}

// RequestUnload 立即删除已干净的 Ready 区块；必须保存的区块保留为 Unloading。
func (dimension *Dimension) RequestUnload(pos core.ChunkPos) bool {
	record, exists := dimension.Records[pos]
	if !exists || record.State != contract.ChunkReady {
		return false
	}
	if !record.Dirty() && record.SaveInFlightRevision == 0 {
		delete(dimension.Records, pos)
		return true
	}
	record.State = contract.ChunkUnloading
	record.UnloadRequested = true
	return false
}

func (dimension *Dimension) CancelUnload(pos core.ChunkPos) bool {
	record, exists := dimension.Records[pos]
	if !exists || record.State != contract.ChunkUnloading {
		return false
	}
	record.State = contract.ChunkReady
	record.UnloadRequested = false
	return true
}

func (dimension *Dimension) deleteCleanUnloading(pos core.ChunkPos) {
	record := dimension.Records[pos]
	if record != nil && record.State == contract.ChunkUnloading && !record.Dirty() && record.SaveInFlightRevision == 0 {
		delete(dimension.Records, pos)
	}
}

func (dimension *Dimension) Info(pos core.ChunkPos) (contract.ChunkInfo, bool) {
	record, ok := dimension.Records[pos]
	if !ok {
		return contract.ChunkInfo{}, false
	}
	return contract.ChunkInfo{State: record.State, Revision: record.Revision, Err: record.Err}, true
}

func (dimension *Dimension) CloneReadyChunk(pos core.ChunkPos) (*world.Chunk, uint64, bool) {
	record, ok := dimension.Records[pos]
	if !ok || record.State != contract.ChunkReady {
		return nil, 0, false
	}
	return record.Chunk.Clone(), record.Revision, true
}

// BlockAt 返回方块与其所属区块是否 Ready。世界高度外恒为空气。
func (dimension *Dimension) BlockAt(position core.BlockPos) (core.BlockID, bool) {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return core.AirID, true
	}
	record, ok := dimension.Records[position.Chunk()]
	if !ok || record.State != contract.ChunkReady {
		return core.AirID, false
	}
	x, _, z := position.Local()
	return record.Chunk.BlockAt(x, position.Y, z), true
}

func (dimension *Dimension) SetBlock(position core.BlockPos, block core.BlockID) (old core.BlockID, changed bool, err error) {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return core.AirID, false, ErrBlockOutOfWorld
	}
	record, ok := dimension.Records[position.Chunk()]
	if !ok || record.State != contract.ChunkReady {
		return core.AirID, false, ErrChunkNotReady
	}
	x, _, z := position.Local()
	old = record.Chunk.BlockAt(x, position.Y, z)
	if old == block {
		return old, false, nil
	}
	record.Chunk.SetBlock(x, position.Y, z, block)
	return old, true, nil
}

func (dimension *Dimension) recordForTransition(pos core.ChunkPos, want contract.ChunkState, next string) *ChunkRecord {
	record, ok := dimension.Records[pos]
	if !ok || record.State != want {
		state := contract.ChunkAbsent
		if ok {
			state = record.State
		}
		panic(fmt.Sprintf("sim: illegal chunk transition %d -> %s at %+v", state, next, pos))
	}
	return record
}
