package realm

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// oraclePersistenceStats 是增量记账改造前的全量扫描参考实现：逐记录重算四项
// 统计，不读取任何增量记账状态。作为等价性 oracle，数值语义（含「脏且在途」
// 对 EstimatedBytes 的双计入）以本函数为准。
func oraclePersistenceStats(state *State) PersistenceStats {
	var stats PersistenceStats
	for dimensionID, dimension := range state.dimensions {
		for pos, record := range dimension.records {
			if record.Dirty() && record.Chunk != nil {
				stats.DirtyChunks++
				stats.EstimatedBytes += int64(estimateChunkBytes(record.Chunk))
			}
			key := core.ChunkKey{Dimension: dimensionID, Pos: pos}
			if inFlight, exists := state.persistenceInFlight(key, record); exists {
				stats.InFlightChunks++
				stats.EstimatedBytes += int64(inFlight.estimatedBytes)
			}
			if record.UnloadRequested {
				stats.UnloadWaiting++
			}
		}
	}
	return stats
}

// oracleSnapshot 是全量候选收集参考实现产出的单条快照计划，只含可比对字段。
type oracleSnapshot struct {
	key            core.ChunkKey
	revision       uint64
	estimatedBytes int
}

// oracleSnapshotPlan 复刻改造前的候选收集（全部记录迭代、原有过滤、排序与预算
// 截断），但不产生派发副作用；用于对拍 PersistenceSnapshots 的输出与顺序。
func oracleSnapshotPlan(state *State, maxChunks int, maxBytes int, mode SaveMode) []oracleSnapshot {
	candidates := make([]persistenceCandidate, 0)
	for dimensionID, dimension := range state.dimensions {
		for pos, record := range dimension.records {
			key := core.ChunkKey{Dimension: dimensionID, Pos: pos}
			_, inFlight := state.persistenceInFlight(key, record)
			if record.Chunk == nil || !record.Dirty() || inFlight || mode == SaveUrgent && !record.UnloadRequested {
				continue
			}
			candidates = append(candidates, persistenceCandidate{key: key, record: record})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.record.UnloadRequested != right.record.UnloadRequested {
			return left.record.UnloadRequested
		}
		return chunkKeyLess(left.key, right.key)
	})

	plan := make([]oracleSnapshot, 0, min(maxChunks, len(candidates)))
	estimatedBytes := 0
	for _, candidate := range candidates {
		estimate := estimateChunkBytes(candidate.record.Chunk)
		if len(plan) > 0 && (len(plan) >= maxChunks || estimatedBytes+estimate > maxBytes) {
			break
		}
		plan = append(plan, oracleSnapshot{
			key:            candidate.key,
			revision:       candidate.record.Revision,
			estimatedBytes: estimate,
		})
		estimatedBytes += estimate
	}
	return plan
}

// recordAccessCount 在注入记录访问探针的情况下执行 f，返回 f 检视的记录数。
func recordAccessCount(f func()) int {
	count := 0
	previous := recordAccessProbe
	recordAccessProbe = func() { count++ }
	defer func() { recordAccessProbe = previous }()
	f()
	return count
}

// assertIncrementalMatchesOracle 断言增量 PersistenceStats 与全量 oracle 一致，
// 并顺带校验脏索引成员不变量：成员集合恰为 Dirty() 为真的记录集合、无悬挂成员。
func assertIncrementalMatchesOracle(t *testing.T, state *State) {
	t.Helper()
	got, want := state.PersistenceStats(), oraclePersistenceStats(state)
	if got != want {
		t.Fatalf("PersistenceStats 增量=%+v, oracle=%+v", got, want)
	}
	for _, dimension := range state.dimensions {
		dirtyRecords := 0
		for pos, record := range dimension.records {
			_, inIndex := dimension.dirtyIndex[pos]
			if record.Dirty() {
				dirtyRecords++
				if !inIndex {
					t.Fatalf("脏记录 %+v 不在脏索引中", pos)
				}
			} else if inIndex {
				t.Fatalf("干净记录 %+v 残留在脏索引中", pos)
			}
		}
		if len(dimension.dirtyIndex) != dirtyRecords {
			t.Fatalf("脏索引大小=%d, 脏记录数=%d", len(dimension.dirtyIndex), dirtyRecords)
		}
	}
}

func requireReadyRecord(t *testing.T, dimension *Dimension, pos core.ChunkPos) *ChunkRecord {
	t.Helper()
	record := dimension.records[pos]
	if record == nil || record.State != ChunkReady || record.Chunk == nil {
		t.Fatalf("区块 %+v 未处于 Ready 状态", pos)
	}
	return record
}

// requireGeneratedChunk 经生成路径落一个 Ready 记录并返回其记录指针。
func requireGeneratedChunk(t *testing.T, dimension *Dimension, pos core.ChunkPos, chunk *world.Chunk) *ChunkRecord {
	t.Helper()
	if !dimension.BeginGeneration(pos) {
		t.Fatalf("BeginGeneration(%+v) = false", pos)
	}
	if err := dimension.ApplyGenerated(pos, chunk); err != nil {
		t.Fatalf("ApplyGenerated(%+v): %v", pos, err)
	}
	return requireReadyRecord(t, dimension, pos)
}

// requireLoadedChunk 经加载路径落一个显式 revision 的 Ready 记录并返回其记录指针。
func requireLoadedChunk(
	t *testing.T,
	dimension *Dimension,
	pos core.ChunkPos,
	chunk *world.Chunk,
	revision, persistedRevision uint64,
	needsRewrite bool,
) *ChunkRecord {
	t.Helper()
	if !dimension.BeginLoading(pos) {
		t.Fatalf("BeginLoading(%+v) = false", pos)
	}
	if err := dimension.ApplyLoaded(pos, chunk, revision, persistedRevision, needsRewrite, false); err != nil {
		t.Fatalf("ApplyLoaded(%+v): %v", pos, err)
	}
	return requireReadyRecord(t, dimension, pos)
}

func TestPersistenceStatsDoubleCountsDirtyInFlightEstimatedBytes(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	pos := core.ChunkPos{X: 3, Z: -2}
	chunk := world.NewChunk(pos)
	chunk.SetBlock(4, core.MinY+5, 6, core.StoneID)
	requireGeneratedChunk(t, dimension, pos, chunk)
	payload := estimateChunkBytes(chunk)

	// 生成的区块 Revision=1 > PersistedRevision=0：脏且仅有当前估算。
	dirty := PersistenceStats{DirtyChunks: 1, EstimatedBytes: int64(payload)}
	if got := state.PersistenceStats(); got != dirty {
		t.Fatalf("派发前统计=%+v, want %+v", got, dirty)
	}

	snapshots := state.PersistenceSnapshots(1, 1<<30, SaveAll)
	if len(snapshots) != 1 {
		t.Fatalf("快照数=%d, want 1", len(snapshots))
	}
	if snapshots[0].EstimatedBytes != payload {
		t.Fatalf("快照估算=%d, want %d", snapshots[0].EstimatedBytes, payload)
	}

	// 钉住现行双计入语义：快照在途期间，当前 PayloadBytes 估算与在途快照的
	// estimatedBytes 同时计入 EstimatedBytes，DirtyChunks 与 InFlightChunks 均含该区块。
	doubleCounted := PersistenceStats{
		DirtyChunks:    1,
		EstimatedBytes: int64(payload) + int64(snapshots[0].EstimatedBytes),
		InFlightChunks: 1,
	}
	if got := state.PersistenceStats(); got != doubleCounted {
		t.Fatalf("在途统计=%+v, want %+v（双计入）", got, doubleCounted)
	}
	if got := oraclePersistenceStats(state); got != doubleCounted {
		t.Fatalf("oracle=%+v, want %+v（双计入）", got, doubleCounted)
	}

	// 结算后双计入解除，统计归零。
	state.ApplyPersisted([]PersistedChunk{{Key: snapshots[0].Key, Revision: snapshots[0].Revision}})
	if got := state.PersistenceStats(); got != (PersistenceStats{}) {
		t.Fatalf("结算后统计=%+v, want 全零", got)
	}
}

// newUniformStatsWorld 构造 N 个同内容区块的世界：2 个脏（其一待卸载、其一在途）
// 加其余干净记录，使任意 N>=2 的两个世界处于数值等价的统计状态。
func newUniformStatsWorld(t *testing.T, count int) *State {
	t.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	for i := range count {
		pos := core.ChunkPos{X: int32(i % 64), Z: int32(i / 64)}
		chunk := world.NewChunk(pos)
		chunk.SetBlock(0, core.MinY, 0, core.StoneID)
		persisted := uint64(1)
		if i < 2 {
			persisted = 0
		}
		requireLoadedChunk(t, dimension, pos, chunk, 1, persisted, false)
	}
	first := core.ChunkPos{}
	if dimension.RequestUnload(first) {
		t.Fatal("脏区块在持久化前被直接卸载")
	}
	snapshots := state.PersistenceSnapshots(1, 1<<30, SaveAll)
	if len(snapshots) != 1 || snapshots[0].Key.Pos != first {
		t.Fatalf("在途快照=%+v, want 首个待卸载区块", snapshots)
	}
	return state
}

func TestPersistenceStatsRecordAccessIndependentOfLoadedCount(t *testing.T) {
	small := newUniformStatsWorld(t, 2)
	large := newUniformStatsWorld(t, 2000)

	var smallStats, largeStats PersistenceStats
	smallAccesses := recordAccessCount(func() { smallStats = small.PersistenceStats() })
	largeAccesses := recordAccessCount(func() { largeStats = large.PersistenceStats() })

	if smallStats != largeStats {
		t.Fatalf("等价状态统计不一致: 2 区块=%+v, 2000 区块=%+v", smallStats, largeStats)
	}
	if want := oraclePersistenceStats(small); smallStats != want {
		t.Fatalf("2 区块统计=%+v, oracle=%+v", smallStats, want)
	}
	if want := oraclePersistenceStats(large); largeStats != want {
		t.Fatalf("2000 区块统计=%+v, oracle=%+v", largeStats, want)
	}
	if smallAccesses != largeAccesses {
		t.Fatalf("记录访问数随区块数增长: 2 区块=%d, 2000 区块=%d", smallAccesses, largeAccesses)
	}
	if smallAccesses != 0 {
		t.Fatalf("PersistenceStats 检视了 %d 条记录, want 0（只读增量聚合）", smallAccesses)
	}
}

func TestPersistenceSnapshotsCandidateCollectionTouchesOnlyDirtyRecords(t *testing.T) {
	const (
		total = 2000
		dirty = 5
	)
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	for i := range total {
		pos := core.ChunkPos{X: int32(i % 64), Z: int32(i / 64)}
		chunk := world.NewChunk(pos)
		chunk.SetBlock(0, core.MinY, 0, core.StoneID)
		persisted := uint64(1)
		if i < dirty {
			persisted = 0
		}
		requireLoadedChunk(t, dimension, pos, chunk, 1, persisted, false)
	}

	plan := oracleSnapshotPlan(state, 10, 1<<40, SaveAll)
	if len(plan) != dirty {
		t.Fatalf("oracle 快照计划数=%d, want %d", len(plan), dirty)
	}
	var snapshots []ChunkSaveSnapshot
	accesses := recordAccessCount(func() { snapshots = state.PersistenceSnapshots(10, 1<<40, SaveAll) })
	if accesses != dirty {
		t.Fatalf("候选收集检视记录数=%d, want %d（只迭代脏索引成员）", accesses, dirty)
	}
	if len(snapshots) != len(plan) {
		t.Fatalf("快照数=%d, oracle=%d", len(snapshots), len(plan))
	}
	for i := range snapshots {
		got, want := snapshots[i], plan[i]
		if got.Key != want.key || got.Revision != want.revision || got.EstimatedBytes != want.estimatedBytes {
			t.Fatalf("快照[%d]=%+v, oracle=%+v", i, got, want)
		}
	}
}

func TestSetDimensionRebuildsIncrementalAccounting(t *testing.T) {
	state := NewState(core.Overworld)
	if got := state.PersistenceStats(); got != (PersistenceStats{}) {
		t.Fatalf("NewState 初始统计=%+v, want 全零", got)
	}
	dimension := state.Dimension(core.Overworld)

	// 原维度铺满各类记录形态：在途、脏待卸载、干净、生成失败、加载中。
	inFlightPos := core.ChunkPos{X: 0}
	requireGeneratedChunk(t, dimension, inFlightPos, world.NewChunk(inFlightPos))
	unloadingPos := core.ChunkPos{X: 1}
	requireGeneratedChunk(t, dimension, unloadingPos, world.NewChunk(unloadingPos))
	if dimension.RequestUnload(unloadingPos) {
		t.Fatal("脏区块在持久化前被直接卸载")
	}
	cleanPos := core.ChunkPos{X: 2}
	requireLoadedChunk(t, dimension, cleanPos, world.NewChunk(cleanPos), 4, 4, false)
	failedPos := core.ChunkPos{X: 3}
	if !dimension.BeginGeneration(failedPos) {
		t.Fatal("BeginGeneration(failedPos) = false")
	}
	dimension.MarkFailed(failedPos, errors.New("boom"))
	loadingPos := core.ChunkPos{X: 4}
	if !dimension.BeginLoading(loadingPos) {
		t.Fatal("BeginLoading(loadingPos) = false")
	}
	snapshots := state.PersistenceSnapshots(4, 1<<30, SaveAll)
	if len(snapshots) != 2 {
		t.Fatalf("快照数=%d, want 2（待卸载优先 + 在途）", len(snapshots))
	}
	if snapshots[0].Key.Pos != unloadingPos {
		t.Fatalf("首快照=%+v, want 待卸载区块", snapshots[0].Key)
	}
	assertIncrementalMatchesOracle(t, state)

	// 换入维度：换入前先结算在途，避免悬空在途条目与换入记录键冲突
	// （该悬空态在原全扫实现中同样触发 persistenceInFlight 的一致性 panic）。
	state.FailPersistence(snapshots)

	fresh := NewDimension(core.Overworld)
	freshDirtyA := core.ChunkPos{X: 10}
	requireGeneratedChunk(t, fresh, freshDirtyA, world.NewChunk(freshDirtyA))
	freshDirtyB := core.ChunkPos{X: 11}
	requireGeneratedChunk(t, fresh, freshDirtyB, world.NewChunk(freshDirtyB))
	if fresh.RequestUnload(freshDirtyB) {
		t.Fatal("脏区块在持久化前被直接卸载")
	}
	freshClean := core.ChunkPos{X: 12}
	requireLoadedChunk(t, fresh, freshClean, world.NewChunk(freshClean), 2, 2, false)

	state.SetDimension(fresh)
	assertIncrementalMatchesOracle(t, state)

	// 再次安装同一维度：重建必须幂等，聚合不重复累计。
	state.SetDimension(fresh)
	assertIncrementalMatchesOracle(t, state)

	// 安装空维度：聚合归零。
	state.SetDimension(NewDimension(core.Overworld))
	assertIncrementalMatchesOracle(t, state)
	if got := state.PersistenceStats(); got != (PersistenceStats{}) {
		t.Fatalf("空维度统计=%+v, want 全零", got)
	}
}

func TestEnvironmentMutationContentChangesRefreshEstimateCache(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	pos := core.ChunkPos{X: -7, Z: 9}
	chunk := world.NewChunk(pos)
	requireGeneratedChunk(t, dimension, pos, chunk)
	assertIncrementalMatchesOracle(t, state)
	emptyBytes := state.PersistenceStats().EstimatedBytes

	// 环境事务的方块写入经 dimension.SetBlock + Mutation.Record 汇入同一事务，
	// Commit 推进 revision：(revision, chunk) 估算缓存键随之失效并重算。
	// 这是估算缓存键精确性的根基——环境推进不存在绕过事务的方块内容写入。
	mutation := state.NewMutation()
	environment := state.NewEnvironmentMutation(mutation, 1, EnvironmentConfig{})
	position := core.BlockPos{X: pos.X<<core.SectionShift + 3, Y: core.MinY + 20, Z: pos.Z<<core.SectionShift + 5}
	if old, changed, err := environment.SetBlock(core.Overworld, position, core.WaterSourceID); err != nil || !changed || old != core.AirID {
		t.Fatalf("EnvironmentMutation.SetBlock() = (%d, %v, %v)", old, changed, err)
	}
	if len(mutation.ChangedBlocks()) != 1 {
		t.Fatalf("环境事务变更数=%d, want 1", len(mutation.ChangedBlocks()))
	}
	mutation.Commit()
	assertIncrementalMatchesOracle(t, state)
	if got := state.PersistenceStats().EstimatedBytes; got <= emptyBytes {
		t.Fatalf("内容变更后估算=%d, 未超过空区块估算 %d（缓存未失效重算）", got, emptyBytes)
	}

	// 掉落物等非方块槽位在 PayloadBytes 中是固定常量：不推进 revision 的直接
	// 覆写既不改变 oracle，也不得改变增量估算。
	beforeBytes := state.PersistenceStats().EstimatedBytes
	if !dimension.UpdateReadyChunk(pos, func(c *world.Chunk) { c.SetDrop(0, world.DropSlot{}) }) {
		t.Fatal("UpdateReadyChunk() = false")
	}
	assertIncrementalMatchesOracle(t, state)
	if got := state.PersistenceStats().EstimatedBytes; got != beforeBytes {
		t.Fatalf("非方块槽位覆写改变估算: %d -> %d", beforeBytes, got)
	}
}

// --- 随机操作序列属性测试 ---

var propertyBlockPool = []core.BlockID{
	core.StoneID, core.DirtID, core.GrassID, core.SandID, core.BedrockID, core.WaterSourceID,
}

var propertyPositions = []core.ChunkPos{
	{}, {X: 1}, {X: -1}, {Z: 1}, {Z: -2}, {X: 2, Z: 3}, {X: -3, Z: -4}, {X: 5, Z: -5},
}

var propertyDimensions = []core.DimensionID{core.Overworld, 1}

// propertyWorld 在固定槽位池上随机混合全部记录迁移操作，每步后对拍增量与 oracle。
type propertyWorld struct {
	state   *State
	rng     *rand.Rand
	pending []ChunkSaveSnapshot
}

func newPropertyWorld(seed uint64) *propertyWorld {
	return &propertyWorld{
		state: NewState(propertyDimensions...),
		rng:   rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
	}
}

func (w *propertyWorld) dimension() (core.DimensionID, *Dimension) {
	dimensionID := propertyDimensions[w.rng.IntN(len(propertyDimensions))]
	return dimensionID, w.state.dimensions[dimensionID]
}

func (w *propertyWorld) position() core.ChunkPos {
	return propertyPositions[w.rng.IntN(len(propertyPositions))]
}

// randomChunk 构造内容随机的区块：随机数量的随机方块写入让 24 区段 palette 呈现
// 单值/索引等多态，PayloadBytes 随之变化，覆盖估算缓存的命中与失效。
func (w *propertyWorld) randomChunk(pos core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(pos)
	for range w.rng.IntN(96) {
		x := w.rng.IntN(core.SectionSize)
		y := w.rng.IntN(core.SectionSize * core.SectionsPerChunk)
		z := w.rng.IntN(core.SectionSize)
		chunk.SetBlock(x, int32(core.MinY+y), z, propertyBlockPool[w.rng.IntN(len(propertyBlockPool))])
	}
	return chunk
}

// drainPending 随机结算（ack 或 fail）全部在途快照。换维等操作前调用，避免
// 悬空在途条目与换入记录键冲突——该悬空态在原实现中即触发一致性 panic。
func (w *propertyWorld) drainPending() {
	for _, snapshot := range w.pending {
		if w.rng.IntN(2) == 0 {
			w.state.ApplyPersisted([]PersistedChunk{{Key: snapshot.Key, Revision: snapshot.Revision}})
		} else {
			w.state.FailPersistence([]ChunkSaveSnapshot{snapshot})
		}
	}
	w.pending = nil
}

func (w *propertyWorld) step(t *testing.T) {
	t.Helper()
	switch n := w.rng.IntN(100); {
	case n < 8:
		_, dimension := w.dimension()
		pos := w.position()
		record := dimension.records[pos]
		if record != nil && record.State == ChunkReady && record.Chunk != nil {
			if !dimension.Touch(pos) {
				t.Fatalf("Touch(Ready) = false at %+v", pos)
			}
		}
	case n < 20:
		w.opCommitBlockChanges()
	case n < 26:
		w.opCommitTouchOnly()
	case n < 32:
		_, dimension := w.dimension()
		dimension.BeginLoading(w.position())
	case n < 36:
		_, dimension := w.dimension()
		pos := w.position()
		if record := dimension.records[pos]; record != nil && record.State == ChunkLoading {
			dimension.DropLoading(pos)
		}
	case n < 40:
		_, dimension := w.dimension()
		dimension.MarkGenerating(w.position())
	case n < 45:
		_, dimension := w.dimension()
		dimension.BeginGeneration(w.position())
	case n < 55:
		_, dimension := w.dimension()
		pos := w.position()
		if record := dimension.records[pos]; record != nil && record.State == ChunkGenerating {
			if err := dimension.ApplyGenerated(pos, w.randomChunk(pos)); err != nil {
				t.Fatalf("ApplyGenerated(%+v): %v", pos, err)
			}
		}
	case n < 65:
		_, dimension := w.dimension()
		pos := w.position()
		if record := dimension.records[pos]; record != nil && record.State == ChunkLoading {
			revision := uint64(1 + w.rng.IntN(8))
			persisted := uint64(w.rng.IntN(int(revision) + 1))
			err := dimension.ApplyLoaded(
				pos, w.randomChunk(pos), revision, persisted,
				w.rng.IntN(4) == 0, w.rng.IntN(8) == 0,
			)
			if err != nil {
				t.Fatalf("ApplyLoaded(%+v): %v", pos, err)
			}
		}
	case n < 68:
		_, dimension := w.dimension()
		pos := w.position()
		if record := dimension.records[pos]; record != nil && record.State == ChunkGenerating {
			dimension.MarkFailed(pos, errors.New("property generation failure"))
		}
	case n < 71:
		_, dimension := w.dimension()
		pos := w.position()
		if record := dimension.records[pos]; record != nil && record.State == ChunkLoading {
			dimension.MarkLoadFailed(pos, errors.New("property load failure"))
		}
	case n < 78:
		_, dimension := w.dimension()
		dimension.RequestUnload(w.position())
	case n < 83:
		_, dimension := w.dimension()
		dimension.CancelUnload(w.position())
	case n < 91:
		w.opSnapshots(t)
	case n < 96:
		w.opApplyPersisted()
	default:
		w.opFailPersistence()
	}
}

// opCommitBlockChanges 模拟权威 tick 的方块事务：写入随机方块并经同一事务提交，
// 覆盖 revision 推进与 section 压缩对估算缓存的失效。
func (w *propertyWorld) opCommitBlockChanges() {
	dimensionID, dimension := w.dimension()
	mutation := w.state.NewMutation()
	for _, pos := range propertyPositions {
		record := dimension.records[pos]
		if record == nil || record.State != ChunkReady || record.Chunk == nil || w.rng.IntN(3) != 0 {
			continue
		}
		for range 1 + w.rng.IntN(4) {
			x := w.rng.IntN(core.SectionSize)
			y := w.rng.IntN(core.SectionSize * core.SectionsPerChunk)
			z := w.rng.IntN(core.SectionSize)
			position := core.BlockPos{
				X: pos.X<<core.SectionShift + int32(x),
				Y: core.MinY + int32(y),
				Z: pos.Z<<core.SectionShift + int32(z),
			}
			block := propertyBlockPool[w.rng.IntN(len(propertyBlockPool))]
			if _, changed, err := dimension.SetBlock(position, block); err == nil && changed {
				mutation.Record(dimensionID, position, block)
			}
		}
	}
	mutation.Commit()
}

// opCommitTouchOnly 模拟不经方块写入的 revision barrier（事务 Touch）。
func (w *propertyWorld) opCommitTouchOnly() {
	dimensionID, dimension := w.dimension()
	pos := w.position()
	record := dimension.records[pos]
	if record == nil || record.State != ChunkReady || record.Chunk == nil {
		return
	}
	mutation := w.state.NewMutation()
	mutation.Touch(core.ChunkKey{Dimension: dimensionID, Pos: pos})
	mutation.Commit()
}

// opSnapshots 对拍快照派发：先以 oracle 计划（无副作用）预测输出，再执行真实
// PersistenceSnapshots，断言键序、revision 与估算逐项一致。
func (w *propertyWorld) opSnapshots(t *testing.T) {
	t.Helper()
	maxChunks := 1 + w.rng.IntN(4)
	maxBytes := 1 + w.rng.IntN(1<<22)
	mode := SaveAll
	if w.rng.IntN(2) == 0 {
		mode = SaveUrgent
	}
	plan := oracleSnapshotPlan(w.state, maxChunks, maxBytes, mode)
	snapshots := w.state.PersistenceSnapshots(maxChunks, maxBytes, mode)
	if len(snapshots) != len(plan) {
		t.Fatalf("快照数=%d, oracle=%d", len(snapshots), len(plan))
	}
	for i := range snapshots {
		got, want := snapshots[i], plan[i]
		if got.Key != want.key || got.Revision != want.revision || got.EstimatedBytes != want.estimatedBytes {
			t.Fatalf("快照[%d]=%+v, oracle=%+v", i, got, want)
		}
	}
	w.pending = append(w.pending, snapshots...)
}

func (w *propertyWorld) opApplyPersisted() {
	if len(w.pending) == 0 {
		return
	}
	count := 1 + w.rng.IntN(len(w.pending))
	chosen := w.pending[:count]
	acks := make([]PersistedChunk, 0, count)
	for _, snapshot := range chosen {
		acks = append(acks, PersistedChunk{Key: snapshot.Key, Revision: snapshot.Revision})
	}
	w.state.ApplyPersisted(acks)
	w.pending = w.pending[count:]
}

func (w *propertyWorld) opFailPersistence() {
	if len(w.pending) == 0 {
		return
	}
	count := 1 + w.rng.IntN(len(w.pending))
	chosen := w.pending[:count]
	w.state.FailPersistence(chosen)
	w.pending = w.pending[count:]
}

func TestPersistenceIncrementalStatsMatchOracleUnderRandomOps(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3, 4, 5, 6} {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			w := newPropertyWorld(seed)
			for range 600 {
				w.step(t)
				assertIncrementalMatchesOracle(t, w.state)
			}
			// 收尾前随机结算剩余在途，覆盖末态结算路径。
			w.drainPending()
			assertIncrementalMatchesOracle(t, w.state)
		})
	}
}

// TestPersistenceRandomOpsSetDimensionRebuild 在随机序列中途整维替换：换入维度
// 由公共迁移方法构造，重建后增量必须与 oracle 收敛一致。
func TestPersistenceRandomOpsSetDimensionRebuild(t *testing.T) {
	w := newPropertyWorld(42)
	for range 200 {
		w.step(t)
		assertIncrementalMatchesOracle(t, w.state)
	}
	w.drainPending()

	dimensionID := propertyDimensions[w.rng.IntN(len(propertyDimensions))]
	fresh := NewDimension(dimensionID)
	for _, pos := range propertyPositions[:3] {
		if !fresh.BeginGeneration(pos) {
			t.Fatalf("BeginGeneration(%+v) = false", pos)
		}
		if err := fresh.ApplyGenerated(pos, w.randomChunk(pos)); err != nil {
			t.Fatalf("ApplyGenerated(%+v): %v", pos, err)
		}
	}
	if fresh.RequestUnload(propertyPositions[1]) {
		t.Fatal("脏区块在持久化前被直接卸载")
	}
	w.state.SetDimension(fresh)
	assertIncrementalMatchesOracle(t, w.state)
}
