// Package realm 持有权威世界状态与区块变更事务。
package realm

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

var (
	ErrChunkNotReady   = errors.New("sim: chunk not ready")
	ErrBlockOutOfWorld = errors.New("sim: block outside world height")
)

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
	Chunk                *world.Chunk
	Revision             uint64
	PersistedRevision    uint64
	NeedsRewrite         bool
	Recovered            bool
	UnloadRequested      bool
	SaveInFlightRevision uint64
	Dirty                bool
	Err                  error
}

type ChunkRecord struct {
	State                ChunkState
	Chunk                *world.Chunk
	Revision             uint64
	PersistedRevision    uint64
	NeedsRewrite         bool
	Recovered            bool
	UnloadRequested      bool
	SaveInFlightRevision uint64
	Err                  error
	// stats 是该记录对维度持久化聚合的私有贡献缓存，由 refreshRecord 独占维护，
	// 不进入 Info() 快照。
	stats recordStats
}

// recordStats 缓存单条记录对维度持久化聚合（脏计数、估算字节、待卸载计数）的
// 当前贡献，配合 Dimension.refreshRecord 做增量记账，让 PersistenceStats 查询
// 与 PersistenceSnapshots 候选收集不再全量扫描记录。
//
// estimate/revision/chunk 三者构成 PayloadBytes 估算缓存：估算只依赖 24 区段
// palette（方块内容），而方块内容变更必经 Mutation/EnvironmentMutation 事务在
// Commit 推进 revision（环境推进的全部写入路径——EnvironmentMutation.SetBlock、
// fluidWorld.SetBlock、作物/火把/床结算——都先写方块再 Record 进同一事务），
// 整替记录必换 chunk 指针，因此 (revision, chunk) 命中时估算必然未变；箱子/
// 熔炉/掉落物等非方块槽位在 PayloadBytes 中是固定常量，无需进入键。
type recordStats struct {
	dirty         bool
	unloadWaiting bool
	estimate      int
	revision      uint64
	chunk         *world.Chunk
}

// recordAccessProbe 是持久化查询成本的记录访问计数缝：`ChunkRecord.Dirty()` 是
// 任何记录级过滤的必经谓词，注入回调后以查询前后的计数差度量该查询检视的记录数。
// 仅供包内测试使用，生产恒为 nil，热路径只承担一次可预测的 nil 判断。
var recordAccessProbe func()

func (record *ChunkRecord) Dirty() bool {
	if recordAccessProbe != nil {
		recordAccessProbe()
	}
	return record.Revision > record.PersistedRevision || record.NeedsRewrite
}

// Dimension 由 State 的单写者 tick 独占，不提供内部锁。
type Dimension struct {
	id      core.DimensionID
	records map[core.ChunkPos]*ChunkRecord
	// dirtyIndex 是脏区块索引：成员恰为 Dirty() 为真的记录位置，
	// PersistenceSnapshots 的候选收集只迭代该集合。
	dirtyIndex map[core.ChunkPos]struct{}
	// 以下聚合是该维度全部记录贡献缓存之和，由 refreshRecord/settleRecord 做差
	// 维护；PersistenceStats 只按维度求和，不触碰任何记录。
	dirtyChunks         int
	dirtyEstimatedBytes int64
	unloadWaiting       int
}

func NewDimension(id core.DimensionID) *Dimension {
	return &Dimension{
		id:         id,
		records:    make(map[core.ChunkPos]*ChunkRecord),
		dirtyIndex: make(map[core.ChunkPos]struct{}),
	}
}

// State 持有维度与持久化中的快照状态。
type State struct {
	dimensions    map[core.DimensionID]*Dimension
	inFlightSaves map[core.ChunkKey]persistenceInFlight
	environment   environmentState
	// inFlightEstimatedBytes 是全部在途快照 estimatedBytes 之和，由派发与结算
	// 路径成对维护；EstimatedBytes 查询 = 各维度脏估算之和 + 本值，从而保持
	// 「脏且在途」双计入的现行语义。
	inFlightEstimatedBytes int64
}

// NewState 构造初始世界：全部记账从零值起步（空记账）。
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

func (state *State) EnsureDimension(id core.DimensionID) *Dimension {
	dimension := state.dimensions[id]
	if dimension == nil {
		dimension = NewDimension(id)
		state.dimensions[id] = dimension
	}
	return dimension
}

func (state *State) SetDimension(dimension *Dimension) {
	// 整维替换：换入维度先全量重建记账再入表；换出维度的聚合随表项一并失效
	// （State 级查询是各维度聚合之和）。inFlightSaves 不在此清理，与既有语义
	// 一致——悬空在途条目仍由 persistenceInFlight 的一致性检查兜底。
	dimension.rebuildStats()
	state.dimensions[dimension.id] = dimension
}

// refreshRecord 是维度增量记账的唯一入口：重算一条记录当前对聚合的贡献，与其
// 缓存做差调整脏计数、估算字节与待卸载计数，并同步脏索引成员与估算缓存键。
// 任何改动记录持久化相关字段的迁移点在写后必须调用本方法；整替记录必须先
// settleRecord 扣清旧贡献再覆写（覆写会连缓存一起清零，漏结算会导致聚合漂移），
// 删除记录则直接以 settleRecord 收尾。
func (dimension *Dimension) refreshRecord(pos core.ChunkPos, record *ChunkRecord) {
	dirty := record.Dirty()
	counted := dirty && record.Chunk != nil
	wasCounted := record.stats.dirty
	previousEstimate := record.stats.estimate
	estimate := previousEstimate
	if counted && (record.stats.chunk != record.Chunk || record.stats.revision != record.Revision) {
		estimate = estimateChunkBytes(record.Chunk)
	}
	if counted != wasCounted {
		if counted {
			dimension.dirtyChunks++
		} else {
			dimension.dirtyChunks--
		}
	}
	if record.UnloadRequested != record.stats.unloadWaiting {
		if record.UnloadRequested {
			dimension.unloadWaiting++
		} else {
			dimension.unloadWaiting--
		}
	}
	bytesNow, bytesWas := 0, 0
	if counted {
		bytesNow = estimate
	}
	if wasCounted {
		bytesWas = previousEstimate
	}
	dimension.dirtyEstimatedBytes += int64(bytesNow - bytesWas)
	if dirty {
		if dimension.dirtyIndex == nil {
			dimension.dirtyIndex = make(map[core.ChunkPos]struct{})
		}
		dimension.dirtyIndex[pos] = struct{}{}
	} else {
		delete(dimension.dirtyIndex, pos)
	}
	record.stats = recordStats{
		dirty:         counted,
		unloadWaiting: record.UnloadRequested,
		estimate:      estimate,
		revision:      record.Revision,
		chunk:         record.Chunk,
	}
}

// settleRecord 在记录离开 records 前扣除其全部缓存贡献并清零缓存，同时移除脏
// 索引成员；此后不得再对该记录调用 refreshRecord。
func (dimension *Dimension) settleRecord(pos core.ChunkPos, record *ChunkRecord) {
	if record.stats.dirty {
		dimension.dirtyChunks--
		dimension.dirtyEstimatedBytes -= int64(record.stats.estimate)
	}
	if record.stats.unloadWaiting {
		dimension.unloadWaiting--
	}
	delete(dimension.dirtyIndex, pos)
	record.stats = recordStats{}
}

// rebuildStats 从零重建整个维度的记账，供整维替换收敛：换入维度即便携带外部
// 构造期间的记录，也能重建出与记录真值一致的聚合。对已一致的维度幂等。
func (dimension *Dimension) rebuildStats() {
	dimension.dirtyChunks = 0
	dimension.unloadWaiting = 0
	dimension.dirtyEstimatedBytes = 0
	clear(dimension.dirtyIndex)
	for pos, record := range dimension.records {
		record.stats = recordStats{}
		dimension.refreshRecord(pos, record)
	}
}

func (dimension *Dimension) ReadyChunk(pos core.ChunkPos) (*world.Chunk, bool) {
	record, ok := dimension.records[pos]
	if !ok || record.State != ChunkReady || record.Chunk == nil {
		return nil, false
	}
	return record.Chunk, true
}

func (dimension *Dimension) UpdateReadyChunk(pos core.ChunkPos, update func(*world.Chunk)) bool {
	chunk, ok := dimension.ReadyChunk(pos)
	if !ok {
		return false
	}
	update(chunk)
	return true
}

func (dimension *Dimension) Touch(pos core.ChunkPos) bool {
	record, ok := dimension.records[pos]
	if !ok || record.State != ChunkReady || record.Chunk == nil {
		return false
	}
	record.Revision++
	dimension.refreshRecord(pos, record)
	return true
}

func (dimension *Dimension) ReadyChunkPositions(dst []core.ChunkPos) []core.ChunkPos {
	for pos, record := range dimension.records {
		if record.State == ChunkReady && record.Chunk != nil {
			dst = append(dst, pos)
		}
	}
	return dst
}

// BeginLoading 把 Absent 或 Failed 区块转为 Loading。
func (dimension *Dimension) BeginLoading(pos core.ChunkPos) bool {
	record, exists := dimension.records[pos]
	if !exists {
		record = &ChunkRecord{State: ChunkLoading}
		dimension.records[pos] = record
		dimension.refreshRecord(pos, record)
		return true
	}
	switch record.State {
	case ChunkLoading, ChunkGenerating, ChunkReady, ChunkUnloading:
		return false
	case ChunkFailed:
		dimension.settleRecord(pos, record)
		*record = ChunkRecord{State: ChunkLoading}
		dimension.refreshRecord(pos, record)
		return true
	case ChunkAbsent:
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
	record := dimension.recordForTransition(pos, ChunkLoading, "Absent")
	dimension.settleRecord(pos, record)
	delete(dimension.records, pos)
}

func (dimension *Dimension) MarkGenerating(pos core.ChunkPos) bool {
	record, exists := dimension.records[pos]
	if !exists || record.State != ChunkLoading {
		return false
	}
	dimension.settleRecord(pos, record)
	*record = ChunkRecord{State: ChunkGenerating}
	dimension.refreshRecord(pos, record)
	return true
}

// BeginGeneration 把 Absent 或 Failed 区块转为 Generating。
func (dimension *Dimension) BeginGeneration(pos core.ChunkPos) bool {
	record, exists := dimension.records[pos]
	if !exists {
		record = &ChunkRecord{State: ChunkGenerating}
		dimension.records[pos] = record
		dimension.refreshRecord(pos, record)
		return true
	}
	switch record.State {
	case ChunkLoading, ChunkGenerating, ChunkReady, ChunkUnloading:
		return false
	case ChunkFailed:
		dimension.settleRecord(pos, record)
		*record = ChunkRecord{State: ChunkGenerating}
		dimension.refreshRecord(pos, record)
		return true
	case ChunkAbsent:
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
	record := dimension.recordForTransition(pos, ChunkGenerating, "Ready")
	if chunk == nil {
		return errors.New("sim: generated chunk is nil")
	}
	if chunk.Pos != pos {
		return fmt.Errorf("sim: generated chunk position %+v, want %+v", chunk.Pos, pos)
	}
	dimension.settleRecord(pos, record)
	*record = ChunkRecord{State: ChunkReady, Chunk: chunk, Revision: 1}
	dimension.refreshRecord(pos, record)
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
	record := dimension.recordForTransition(pos, ChunkLoading, "Ready")
	if chunk == nil {
		return errors.New("sim: loaded chunk is nil")
	}
	if chunk.Pos != pos {
		return fmt.Errorf("sim: loaded chunk position %+v, want %+v", chunk.Pos, pos)
	}
	if persistedRevision > revision {
		return fmt.Errorf("sim: persisted revision %d exceeds current revision %d at %+v", persistedRevision, revision, pos)
	}
	dimension.settleRecord(pos, record)
	*record = ChunkRecord{
		State:             ChunkReady,
		Chunk:             chunk,
		Revision:          revision,
		PersistedRevision: persistedRevision,
		NeedsRewrite:      needsRewrite || recovered,
		Recovered:         recovered,
	}
	dimension.refreshRecord(pos, record)
	return nil
}

// MarkFailed 把生成任务的失败结果记录在区块状态中。
func (dimension *Dimension) MarkFailed(pos core.ChunkPos, err error) {
	if err == nil {
		panic("sim: nil generation failure")
	}
	record := dimension.recordForTransition(pos, ChunkGenerating, "Failed")
	dimension.settleRecord(pos, record)
	*record = ChunkRecord{State: ChunkFailed, Err: err}
	dimension.refreshRecord(pos, record)
}

func (dimension *Dimension) MarkLoadFailed(pos core.ChunkPos, err error) {
	if err == nil {
		panic("sim: nil load failure")
	}
	record := dimension.recordForTransition(pos, ChunkLoading, "Failed")
	dimension.settleRecord(pos, record)
	*record = ChunkRecord{State: ChunkFailed, Err: err}
	dimension.refreshRecord(pos, record)
}

// RequestUnload 立即删除已干净的 Ready 区块；必须保存的区块保留为 Unloading。
func (dimension *Dimension) RequestUnload(pos core.ChunkPos) bool {
	record, exists := dimension.records[pos]
	if !exists || record.State != ChunkReady {
		return false
	}
	if !record.Dirty() && record.SaveInFlightRevision == 0 {
		dimension.settleRecord(pos, record)
		delete(dimension.records, pos)
		return true
	}
	record.State = ChunkUnloading
	record.UnloadRequested = true
	dimension.refreshRecord(pos, record)
	return false
}

func (dimension *Dimension) CancelUnload(pos core.ChunkPos) bool {
	record, exists := dimension.records[pos]
	if !exists || record.State != ChunkUnloading {
		return false
	}
	record.State = ChunkReady
	record.UnloadRequested = false
	dimension.refreshRecord(pos, record)
	return true
}

func (dimension *Dimension) deleteCleanUnloading(pos core.ChunkPos) {
	record := dimension.records[pos]
	if record != nil && record.State == ChunkUnloading && !record.Dirty() && record.SaveInFlightRevision == 0 {
		dimension.settleRecord(pos, record)
		delete(dimension.records, pos)
	}
}

func (dimension *Dimension) Info(pos core.ChunkPos) (ChunkInfo, bool) {
	record, ok := dimension.records[pos]
	if !ok {
		return ChunkInfo{}, false
	}
	return ChunkInfo{
		State:                record.State,
		Chunk:                record.Chunk,
		Revision:             record.Revision,
		PersistedRevision:    record.PersistedRevision,
		NeedsRewrite:         record.NeedsRewrite,
		Recovered:            record.Recovered,
		UnloadRequested:      record.UnloadRequested,
		SaveInFlightRevision: record.SaveInFlightRevision,
		Dirty:                record.Dirty(),
		Err:                  record.Err,
	}, true
}

func (dimension *Dimension) CloneReadyChunk(pos core.ChunkPos) (*world.Chunk, uint64, bool) {
	record, ok := dimension.records[pos]
	if !ok || record.State != ChunkReady {
		return nil, 0, false
	}
	return record.Chunk.Clone(), record.Revision, true
}

// BlockAt 返回方块与其所属区块是否 Ready。世界高度外恒为空气。
func (dimension *Dimension) BlockAt(position core.BlockPos) (core.BlockID, bool) {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return core.AirID, true
	}
	record, ok := dimension.records[position.Chunk()]
	if !ok || record.State != ChunkReady {
		return core.AirID, false
	}
	x, _, z := position.Local()
	return record.Chunk.BlockAt(x, position.Y, z), true
}

func (dimension *Dimension) SetBlock(position core.BlockPos, block core.BlockID) (old core.BlockID, changed bool, err error) {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return core.AirID, false, ErrBlockOutOfWorld
	}
	record, ok := dimension.records[position.Chunk()]
	if !ok || record.State != ChunkReady {
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

func (dimension *Dimension) recordForTransition(pos core.ChunkPos, want ChunkState, next string) *ChunkRecord {
	record, ok := dimension.records[pos]
	if !ok || record.State != want {
		state := ChunkAbsent
		if ok {
			state = record.State
		}
		panic(fmt.Sprintf("sim: illegal chunk transition %d -> %s at %+v", state, next, pos))
	}
	return record
}
