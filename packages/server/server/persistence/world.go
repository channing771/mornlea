package persistence

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/sim/runtime"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// Options 是已由根配置校验后的世界存档运行参数。
type Options struct {
	SaveWorkers    int
	SaveChunks     int
	SaveBytes      int
	AutosaveTicks  uint64
	RetryBaseTicks uint64
	RetryMaxTicks  uint64
	UnsavedBytes   int64
	SaveObserver   func(time.Duration)
	EngineLocker   sync.Locker
}

// Status 汇总权威区块的存档积压与最近一次存档结果。
type Status struct {
	DirtyChunks     int
	EstimatedBytes  int64
	InFlightChunks  int
	Backpressured   bool
	LastSuccess     time.Time
	LastError       string
	LastErrorAt     time.Time
	AutosaveDrained bool
	// 世界 metadata 保存有独立的固定调度状态，不进入按 region 分组的区块重试。
	MetadataPending   bool
	MetadataInFlight  bool
	MetadataLastError string
}

type saveKind uint8

const (
	saveKindChunks saveKind = iota
	saveKindMetadata
)

type saveJob struct {
	Kind      saveKind
	Region    storage.RegionKey
	Snapshots []contract.ChunkSaveSnapshot
	Attempt   uint32
	Retry     bool
	RetryID   uint64
	// Metadata 只在 Kind 为 saveKindMetadata 时有效，是一份不可变的世界快照。
	Metadata storage.Metadata
}

// metadataSaveState 是世界时间保存的固定调度状态：
// 最新权威时间、待提交边界、最多一个 in-flight、失败次数与下一重试 tick。
type metadataSaveState struct {
	latest        uint64
	committed     uint64
	pending       bool
	inFlight      bool
	attempts      uint32
	nextRetryTick uint64
	lastError     string
	lastErrorAt   time.Time
}

type saveCompletion struct {
	Job    saveJob
	Result storage.SaveResult
	Err    error
}

type retrySave struct {
	Job       saveJob
	Attempts  uint32
	NextTick  uint64
	LastError error
}

// World 持有世界区块和 metadata 的异步存档生命周期。
type World struct {
	store   storage.Store
	engine  *runtime.Engine
	options Options

	engineLocker sync.Locker
	engineMu     sync.Mutex

	saveCtx     context.Context
	cancelSaves context.CancelFunc
	saveWorkers sync.WaitGroup
	saveDone    chan struct{}

	mu              sync.Mutex
	saveJobs        chan saveJob
	saveCompletions chan saveCompletion
	autosaveActive  bool
	metadataSave    metadataSaveState
	retry           map[storage.RegionKey][]retrySave
	retryInFlight   map[uint64]retrySave
	nextRetryID     uint64
	backpressured   bool
	lastSaveSuccess time.Time
	lastSaveError   string
	lastSaveErrorAt time.Time
}

// NewWorld 构造并启动固定数量的世界存档 worker。
func NewWorld(store storage.Store, engine *runtime.Engine, options Options) *World {
	saveCtx, cancelSaves := context.WithCancel(context.Background())
	world := &World{
		store:           store,
		engine:          engine,
		options:         options,
		saveCtx:         saveCtx,
		cancelSaves:     cancelSaves,
		saveDone:        make(chan struct{}),
		saveJobs:        make(chan saveJob, options.SaveWorkers*2),
		saveCompletions: make(chan saveCompletion, options.SaveWorkers*2),
		retry:           make(map[storage.RegionKey][]retrySave),
		retryInFlight:   make(map[uint64]retrySave),
	}
	if options.EngineLocker != nil {
		world.engineLocker = options.EngineLocker
	} else {
		world.engineLocker = &world.engineMu
	}
	world.saveWorkers.Add(options.SaveWorkers)
	for range options.SaveWorkers {
		go world.saveWorker()
	}
	go func() {
		world.saveWorkers.Wait()
		close(world.saveDone)
	}()
	return world
}

func (world *World) lockEngine() sync.Locker {
	locker := world.engineLocker
	if locker == nil {
		locker = &world.engineMu
	}
	locker.Lock()
	return locker
}

// Drain 在根 tick 的既有锁保护下结算已完成的异步保存。
func (world *World) Drain() error {
	world.mu.Lock()
	defer world.mu.Unlock()
	return world.drainSaveCompletionsLocked()
}

// Observe 在根 tick 的既有锁保护下执行保存调度和背压更新。
func (world *World) Observe(tick, worldTime uint64) {
	world.mu.Lock()
	defer world.mu.Unlock()
	world.schedulePersistenceLocked(tick)
	world.scheduleMetadataSaveLocked(tick, worldTime)
	world.updatePersistenceBackpressureLocked()
}

// Flush 在根冻结权威后完成区块和 metadata 的最终保存屏障。
func (world *World) Flush(ctx context.Context) error {
	if err := world.flushFrozen(ctx); err != nil {
		return err
	}
	return world.flushMetadata(ctx)
}

// Close 停止存档 worker；调用方必须先完成或放弃 Flush。
func (world *World) Close() {
	world.cancelSaves()
	<-world.saveDone
	world.mu.Lock()
	world.autosaveActive = false
	world.mu.Unlock()
}

// Status 返回当前存档积压、背压和最近完成状态的值副本。
func (world *World) Status() Status {
	world.mu.Lock()
	defer world.mu.Unlock()
	stats := world.engine.PersistenceStats()
	return Status{
		DirtyChunks:    stats.DirtyChunks,
		EstimatedBytes: stats.EstimatedBytes,
		InFlightChunks: stats.InFlightChunks,
		Backpressured:  world.backpressured,
		LastSuccess:    world.lastSaveSuccess,
		LastError:      world.lastSaveError,
		LastErrorAt:    world.lastSaveErrorAt,
		AutosaveDrained: !world.autosaveActive && stats.DirtyChunks == 0 &&
			stats.InFlightChunks == 0 && len(world.retry) == 0 &&
			len(world.retryInFlight) == 0,
		MetadataPending:   world.metadataSave.pending,
		MetadataInFlight:  world.metadataSave.inFlight,
		MetadataLastError: world.metadataSave.lastError,
	}
}

func (world *World) saveWorker() {
	defer world.saveWorkers.Done()
	for {
		select {
		case <-world.saveCtx.Done():
			return
		case job := <-world.saveJobs:
			if job.Kind == saveKindMetadata {
				err := world.store.SaveMetadata(world.saveCtx, job.Metadata)
				select {
				case world.saveCompletions <- saveCompletion{Job: job, Err: err}:
				case <-world.saveCtx.Done():
					return
				}
				continue
			}
			saves := make([]storage.ChunkSave, len(job.Snapshots))
			for index, snapshot := range job.Snapshots {
				saves[index] = storage.ChunkSave{
					Key:      snapshot.Key,
					Revision: snapshot.Revision,
					Chunk:    snapshot.Chunk,
				}
			}
			started := time.Now()
			result, err := world.store.SaveBatch(world.saveCtx, saves)
			if world.options.SaveObserver != nil {
				world.options.SaveObserver(time.Since(started))
			}
			select {
			case world.saveCompletions <- saveCompletion{Job: job, Result: result, Err: err}:
			case <-world.saveCtx.Done():
				return
			}
		}
	}
}

func (world *World) drainSaveCompletionsLocked() error {
	var result error
	for {
		select {
		case completion := <-world.saveCompletions:
			result = errors.Join(result, world.applySaveCompletionLocked(completion))
		default:
			return result
		}
	}
}

func (world *World) applySaveCompletionLocked(completion saveCompletion) error {
	if completion.Job.Kind == saveKindMetadata {
		return world.applyMetadataCompletionLocked(completion)
	}
	uncommitted := make([]contract.ChunkSaveSnapshot, 0, len(completion.Job.Snapshots))
	for _, snapshot := range completion.Job.Snapshots {
		if revision, ok := completion.Result.Committed[snapshot.Key]; ok {
			world.applyCommittedSnapshot(snapshot, revision)
		} else {
			uncommitted = append(uncommitted, snapshot)
		}
	}
	err := completion.Err
	if err == nil && len(uncommitted) != 0 {
		err = errors.New("save result omitted submitted chunks")
	}
	if err != nil {
		world.retainFailedSaveLocked(completion.Job, uncommitted, err)
		return fmt.Errorf("save region %+v: %w", completion.Job.Region, err)
	}
	world.lastSaveSuccess = time.Now()
	if completion.Job.Retry {
		world.finishRetryDispatchLocked(completion.Job)
	}
	return nil
}

func (world *World) applyCommittedSnapshot(
	snapshot contract.ChunkSaveSnapshot,
	committedRevision uint64,
) {
	info, exists := world.engine.ChunkInfo(snapshot.Key)
	if !exists || committedRevision < snapshot.Revision ||
		committedRevision > info.Revision {
		world.engine.FailPersistence([]contract.ChunkSaveSnapshot{snapshot})
		return
	}
	if committedRevision > snapshot.Revision {
		world.engine.FailPersistence([]contract.ChunkSaveSnapshot{snapshot})
		if committedRevision >= info.Revision {
			return
		}
	}
	world.engine.ApplyPersisted([]contract.PersistedChunk{{
		Key: snapshot.Key, Revision: committedRevision,
	}})
}

func (world *World) schedulePersistenceLocked(tick uint64) {
	world.dispatchDueRetriesLocked(tick)
	world.dispatchPersistenceLocked(world.engine.PersistenceSnapshots(
		world.options.SaveChunks,
		world.options.SaveBytes,
		contract.SaveUrgent,
	))
	if tick%world.options.AutosaveTicks == 0 {
		world.autosaveActive = true
	}
	if !world.autosaveActive {
		return
	}
	world.dispatchPersistenceLocked(world.engine.PersistenceSnapshots(
		world.options.SaveChunks,
		world.options.SaveBytes,
		contract.SaveAll,
	))
	stats := world.engine.PersistenceStats()
	if stats.DirtyChunks == 0 && stats.InFlightChunks == 0 {
		world.autosaveActive = false
	}
}

func (world *World) dispatchPersistenceLocked(snapshots []contract.ChunkSaveSnapshot) {
	for _, job := range groupSaveJobs(snapshots) {
		job.Attempt = 1
		select {
		case world.saveJobs <- job:
		default:
			world.engine.FailPersistence(job.Snapshots)
		}
	}
}

func groupSaveJobs(snapshots []contract.ChunkSaveSnapshot) []saveJob {
	grouped := make(map[storage.RegionKey][]contract.ChunkSaveSnapshot)
	for _, snapshot := range snapshots {
		region, _ := storage.RegionFor(snapshot.Key)
		grouped[region] = append(grouped[region], snapshot)
	}
	regions := make([]storage.RegionKey, 0, len(grouped))
	for region := range grouped {
		regions = append(regions, region)
	}
	sort.Slice(regions, func(i, j int) bool {
		return regionKeyLess(regions[i], regions[j])
	})
	jobs := make([]saveJob, 0, len(regions))
	for _, region := range regions {
		group := grouped[region]
		sort.Slice(group, func(i, j int) bool {
			return chunkKeyLessForSave(group[i].Key, group[j].Key)
		})
		jobs = append(jobs, saveJob{Region: region, Snapshots: group})
	}
	return jobs
}

func regionKeyLess(left, right storage.RegionKey) bool {
	if left.Dimension != right.Dimension {
		return left.Dimension < right.Dimension
	}
	if left.X != right.X {
		return left.X < right.X
	}
	return left.Z < right.Z
}

func chunkKeyLessForSave(left, right core.ChunkKey) bool {
	if left.Dimension != right.Dimension {
		return left.Dimension < right.Dimension
	}
	if left.Pos.X != right.Pos.X {
		return left.Pos.X < right.Pos.X
	}
	return left.Pos.Z < right.Pos.Z
}

func (world *World) retainFailedSaveLocked(
	job saveJob,
	uncommitted []contract.ChunkSaveSnapshot,
	err error,
) {
	if job.Retry {
		world.finishRetryDispatchLocked(job)
	}
	if len(uncommitted) == 0 {
		world.recordSaveFailureLocked(job.Region, max(job.Attempt, 1), 0, err)
		return
	}
	attempt := job.Attempt
	if attempt == 0 {
		attempt = 1
	}
	nextTick := saturatingAddUint64(
		world.engine.TickCount(),
		retryDelay(world.options.RetryBaseTicks, world.options.RetryMaxTicks, attempt),
	)
	retryID := job.RetryID
	if retryID == 0 {
		retryID = world.allocateRetryIDLocked()
	}
	world.enqueueRetryCohortLocked(retrySave{
		Job: saveJob{
			Region:    job.Region,
			Snapshots: mergeRetrySnapshots(nil, uncommitted),
			Retry:     true,
			RetryID:   retryID,
		},
		Attempts:  attempt,
		NextTick:  nextTick,
		LastError: err,
	})
	world.recordSaveFailureLocked(job.Region, attempt, nextTick, err)
}

func (world *World) finishRetryDispatchLocked(job saveJob) {
	if retained, ok := world.retryInFlight[job.RetryID]; ok &&
		retained.Job.Attempt == job.Attempt {
		delete(world.retryInFlight, job.RetryID)
	}
}

func (world *World) allocateRetryIDLocked() uint64 {
	for {
		world.nextRetryID++
		if world.nextRetryID == 0 {
			world.nextRetryID++
		}
		if _, exists := world.retryInFlight[world.nextRetryID]; exists {
			continue
		}
		found := false
		for _, cohorts := range world.retry {
			for _, cohort := range cohorts {
				if cohort.Job.RetryID == world.nextRetryID {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return world.nextRetryID
		}
	}
}

func (world *World) enqueueRetryCohortLocked(incoming retrySave) {
	unique := make([]contract.ChunkSaveSnapshot, 0, len(incoming.Job.Snapshots))
	for _, snapshot := range incoming.Job.Snapshots {
		if !world.ownsRetrySnapshotLocked(snapshot) {
			unique = append(unique, snapshot)
		}
	}
	incoming.Job.Snapshots = unique
	if len(unique) == 0 {
		return
	}
	cohorts := world.retry[incoming.Job.Region]
	for index := range cohorts {
		if cohorts[index].Attempts != incoming.Attempts ||
			cohorts[index].NextTick != incoming.NextTick {
			continue
		}
		cohorts[index].Job.Snapshots = mergeRetrySnapshots(
			cohorts[index].Job.Snapshots,
			incoming.Job.Snapshots,
		)
		cohorts[index].LastError = incoming.LastError
		world.retry[incoming.Job.Region] = cohorts
		return
	}
	world.retry[incoming.Job.Region] = append(cohorts, incoming)
}

func (world *World) ownsRetrySnapshotLocked(snapshot contract.ChunkSaveSnapshot) bool {
	for _, cohorts := range world.retry {
		for _, cohort := range cohorts {
			for _, owned := range cohort.Job.Snapshots {
				if owned.Key == snapshot.Key && owned.Revision == snapshot.Revision {
					return true
				}
			}
		}
	}
	for _, cohort := range world.retryInFlight {
		for _, owned := range cohort.Job.Snapshots {
			if owned.Key == snapshot.Key && owned.Revision == snapshot.Revision {
				return true
			}
		}
	}
	return false
}

func (world *World) dispatchDueRetriesLocked(tick uint64) {
	type dueRetry struct {
		region storage.RegionKey
		cohort retrySave
	}
	due := make([]dueRetry, 0)
	for region, cohorts := range world.retry {
		for _, cohort := range cohorts {
			if cohort.NextTick <= tick {
				due = append(due, dueRetry{region: region, cohort: cohort})
			}
		}
	}
	sort.Slice(due, func(i, j int) bool {
		left, right := due[i], due[j]
		if left.cohort.NextTick != right.cohort.NextTick {
			return left.cohort.NextTick < right.cohort.NextTick
		}
		if left.region != right.region {
			return regionKeyLess(left.region, right.region)
		}
		return left.cohort.Job.RetryID < right.cohort.Job.RetryID
	})
	for _, candidate := range due {
		retained, exists := world.pendingRetryCohortLocked(
			candidate.region,
			candidate.cohort.Job.RetryID,
		)
		if !exists {
			continue
		}
		attempt := retained.Attempts
		if attempt < ^uint32(0) {
			attempt++
		}
		job := saveJob{
			Region:    candidate.region,
			Snapshots: append([]contract.ChunkSaveSnapshot(nil), retained.Job.Snapshots...),
			Attempt:   attempt,
			Retry:     true,
			RetryID:   retained.Job.RetryID,
		}
		select {
		case world.saveJobs <- job:
			world.removePendingRetryCohortLocked(candidate.region, job.RetryID)
			retained.Job = job
			world.retryInFlight[job.RetryID] = retained
		default:
			return
		}
	}
}

func (world *World) pendingRetryCohortLocked(
	region storage.RegionKey,
	retryID uint64,
) (retrySave, bool) {
	for _, cohort := range world.retry[region] {
		if cohort.Job.RetryID == retryID {
			return cohort, true
		}
	}
	return retrySave{}, false
}

func (world *World) removePendingRetryCohortLocked(region storage.RegionKey, retryID uint64) {
	cohorts := world.retry[region]
	kept := make([]retrySave, 0, len(cohorts))
	for _, cohort := range cohorts {
		if cohort.Job.RetryID != retryID {
			kept = append(kept, cohort)
		}
	}
	if len(kept) == 0 {
		delete(world.retry, region)
		return
	}
	world.retry[region] = kept
}

func mergeRetrySnapshots(
	existing []contract.ChunkSaveSnapshot,
	incoming []contract.ChunkSaveSnapshot,
) []contract.ChunkSaveSnapshot {
	byKey := make(map[core.ChunkKey]contract.ChunkSaveSnapshot, len(existing)+len(incoming))
	for _, snapshot := range existing {
		byKey[snapshot.Key] = snapshot
	}
	for _, snapshot := range incoming {
		current, exists := byKey[snapshot.Key]
		if !exists || snapshot.Revision > current.Revision {
			byKey[snapshot.Key] = snapshot
		}
	}
	merged := make([]contract.ChunkSaveSnapshot, 0, len(byKey))
	for _, snapshot := range byKey {
		merged = append(merged, snapshot)
	}
	sort.Slice(merged, func(i, j int) bool {
		return chunkKeyLessForSave(merged[i].Key, merged[j].Key)
	})
	return merged
}

func retryDelay(base, maximum uint64, attempts uint32) uint64 {
	delay := base
	for i := uint32(1); i < attempts && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	return min(delay, maximum)
}

func saturatingAddUint64(left, right uint64) uint64 {
	if left > ^uint64(0)-right {
		return ^uint64(0)
	}
	return left + right
}

func (world *World) applyMetadataCompletionLocked(completion saveCompletion) error {
	world.metadataSave.inFlight = false
	if completion.Err == nil {
		world.metadataSave.attempts = 0
		world.metadataSave.nextRetryTick = 0
		world.metadataSave.lastError = ""
		world.metadataSave.committed = max(
			world.metadataSave.committed, completion.Job.Metadata.WorldTimeTicks,
		)
		world.lastSaveSuccess = time.Now()
		return nil
	}
	world.metadataSave.attempts++
	world.metadataSave.pending = true
	world.metadataSave.nextRetryTick = saturatingAddUint64(
		world.engine.TickCount(),
		retryDelay(
			world.options.RetryBaseTicks,
			world.options.RetryMaxTicks,
			world.metadataSave.attempts,
		),
	)
	world.metadataSave.lastError = completion.Err.Error()
	world.metadataSave.lastErrorAt = time.Now()
	slog.Error(
		"世界 metadata 保存失败，将按 tick 退避重试",
		"attempt", world.metadataSave.attempts,
		"next_tick", world.metadataSave.nextRetryTick,
		"error", completion.Err,
	)
	return fmt.Errorf("save world metadata: %w", completion.Err)
}

func (world *World) scheduleMetadataSaveLocked(tick, worldTime uint64) {
	world.metadataSave.latest = worldTime
	if tick%world.options.AutosaveTicks == 0 {
		world.metadataSave.pending = true
	}
	if world.metadataSave.attempts != 0 && tick >= world.metadataSave.nextRetryTick {
		world.metadataSave.pending = true
	}
	if !world.metadataSave.pending || world.metadataSave.inFlight {
		return
	}
	metadata := world.store.Metadata()
	metadata.WorldTimeTicks = world.metadataSave.latest
	// 偏移在派发时刻现取：待保存批次合并到的总是最新权威值（自动保存语义
	// 与世界时间一致），不阻塞 tick 也不形成无界队列。
	metadata.DayPhaseOffset = uint64(world.engine.DayPhaseOffset())
	select {
	case world.saveJobs <- saveJob{
		Kind:     saveKindMetadata,
		Metadata: metadata,
		Attempt:  world.metadataSave.attempts + 1,
	}:
		world.metadataSave.pending = false
		world.metadataSave.inFlight = true
	default:
	}
}

func (world *World) recordSaveFailureLocked(
	region storage.RegionKey,
	attempt uint32,
	nextTick uint64,
	err error,
) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	wrapped := fmt.Errorf("save region %+v: %w", region, err)
	world.lastSaveError = wrapped.Error()
	world.lastSaveErrorAt = time.Now()
	attributes := []any{
		"operation", "save",
		"region", region,
		"attempt", attempt,
		"next_tick", nextTick,
		"error", wrapped,
	}
	if store, ok := world.store.(interface{ WorldPath() string }); ok {
		if path := store.WorldPath(); path != "" {
			attributes = append(attributes, "world_path", path)
		}
	}
	slog.Error("区块存档失败，将按 tick 退避重试", attributes...)
}

func (world *World) updatePersistenceBackpressureLocked() {
	stats := world.engine.PersistenceStats()
	world.backpressured = nextPersistenceBackpressure(
		world.backpressured,
		stats.EstimatedBytes,
		world.options.UnsavedBytes,
	)
}

func nextPersistenceBackpressure(current bool, estimated, limit int64) bool {
	if !current {
		return estimated >= limit
	}
	remainder := limit % 10
	threshold := limit/10*9 + (remainder*9+9)/10
	return estimated >= threshold
}

func (world *World) flushFrozen(ctx context.Context) error {
	var pending []saveJob
	for {
		engineLocker := world.lockEngine()
		world.mu.Lock()
		if err := world.drainSaveCompletionsLocked(); err != nil {
			world.releasePendingShutdownJobsLocked(pending)
			world.mu.Unlock()
			engineLocker.Unlock()
			return world.persistenceErrorWithContext(err, ctx)
		}

		for world.dispatchShutdownRetryLocked() {
		}
		if len(pending) == 0 && len(world.retry) == 0 {
			snapshots := world.engine.PersistenceSnapshots(
				world.options.SaveChunks,
				world.options.SaveBytes,
				contract.SaveAll,
			)
			if len(snapshots) != 0 {
				pending = groupSaveJobs(snapshots)
				for index := range pending {
					pending[index].Attempt = 1
				}
			}
		}
	dispatchPending:
		for len(pending) != 0 {
			select {
			case world.saveJobs <- pending[0]:
				pending = pending[1:]
			default:
				break dispatchPending
			}
		}
		stats := world.engine.PersistenceStats()
		drained := len(pending) == 0 && stats.DirtyChunks == 0 &&
			stats.InFlightChunks == 0 && len(world.retry) == 0 &&
			len(world.retryInFlight) == 0
		world.mu.Unlock()
		engineLocker.Unlock()
		if drained {
			return nil
		}

		if len(pending) != 0 {
			select {
			case world.saveJobs <- pending[0]:
				pending = pending[1:]
			case completion := <-world.saveCompletions:
				engineLocker := world.lockEngine()
				world.mu.Lock()
				err := world.applySaveCompletionLocked(completion)
				if err != nil {
					world.releasePendingShutdownJobsLocked(pending)
					pending = nil
				}
				world.mu.Unlock()
				engineLocker.Unlock()
				if err != nil {
					return world.persistenceErrorWithContext(err, ctx)
				}
			case <-ctx.Done():
				return world.shutdownOwnerContextError(ctx.Err(), nil, pending)
			}
			continue
		}

		select {
		case completion := <-world.saveCompletions:
			engineLocker := world.lockEngine()
			world.mu.Lock()
			err := world.applySaveCompletionLocked(completion)
			world.mu.Unlock()
			engineLocker.Unlock()
			if err != nil {
				return world.persistenceErrorWithContext(err, ctx)
			}
		case <-ctx.Done():
			return world.shutdownOwnerContextError(ctx.Err(), nil, nil)
		}
	}
}

func (world *World) flushMetadata(ctx context.Context) error {
	engineLocker := world.lockEngine()
	world.mu.Lock()
	target := world.engine.WorldTime()
	offset := world.engine.DayPhaseOffset()
	world.metadataSave.latest = target
	world.mu.Unlock()
	engineLocker.Unlock()

	for {
		engineLocker := world.lockEngine()
		world.mu.Lock()
		if err := world.drainSaveCompletionsLocked(); err != nil {
			world.mu.Unlock()
			engineLocker.Unlock()
			return world.persistenceErrorWithContext(err, ctx)
		}
		inFlight := world.metadataSave.inFlight
		done := !inFlight && world.metadataSave.committed >= target
		metadata := world.store.Metadata()
		metadata.WorldTimeTicks = target
		metadata.DayPhaseOffset = uint64(offset)
		world.mu.Unlock()
		engineLocker.Unlock()
		if done {
			return nil
		}

		if !inFlight {
			// 冻结后没有其他调度者，阻塞投递只等待 worker 取走这份最终快照。
			select {
			case world.saveJobs <- saveJob{
				Kind:     saveKindMetadata,
				Metadata: metadata,
				Attempt:  1,
			}:
				world.mu.Lock()
				world.metadataSave.pending = false
				world.metadataSave.inFlight = true
				world.mu.Unlock()
			case <-ctx.Done():
				return world.shutdownOwnerContextError(ctx.Err(), nil, nil)
			}
		}

		select {
		case completion := <-world.saveCompletions:
			engineLocker := world.lockEngine()
			world.mu.Lock()
			err := world.applySaveCompletionLocked(completion)
			world.mu.Unlock()
			engineLocker.Unlock()
			if err != nil {
				return world.persistenceErrorWithContext(err, ctx)
			}
		case <-ctx.Done():
			return world.shutdownOwnerContextError(ctx.Err(), nil, nil)
		}
	}
}

func (world *World) persistenceErrorWithContext(err error, ctx context.Context) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return world.shutdownOwnerContextError(ctxErr, err, nil)
	}
	return err
}

func (world *World) releasePendingShutdownJobsLocked(jobs []saveJob) {
	for _, job := range jobs {
		world.engine.FailPersistence(job.Snapshots)
	}
}

func (world *World) dispatchShutdownRetryLocked() bool {
	type candidate struct {
		region storage.RegionKey
		retry  retrySave
	}
	candidates := make([]candidate, 0)
	for region, cohorts := range world.retry {
		for _, cohort := range cohorts {
			candidates = append(candidates, candidate{region: region, retry: cohort})
		}
	}
	if len(candidates) == 0 {
		return false
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.retry.NextTick != right.retry.NextTick {
			return left.retry.NextTick < right.retry.NextTick
		}
		if left.region != right.region {
			return regionKeyLess(left.region, right.region)
		}
		return left.retry.Job.RetryID < right.retry.Job.RetryID
	})
	selected := candidates[0]
	attempt := selected.retry.Attempts
	if attempt < ^uint32(0) {
		attempt++
	}
	job := saveJob{
		Region:    selected.region,
		Snapshots: append([]contract.ChunkSaveSnapshot(nil), selected.retry.Job.Snapshots...),
		Attempt:   attempt,
		Retry:     true,
		RetryID:   selected.retry.Job.RetryID,
	}
	select {
	case world.saveJobs <- job:
		world.removePendingRetryCohortLocked(selected.region, job.RetryID)
		selected.retry.Job = job
		world.retryInFlight[job.RetryID] = selected.retry
		return true
	default:
		return false
	}
}

func (world *World) shutdownOwnerContextError(
	ctxErr error,
	persistenceErr error,
	pending []saveJob,
) error {
	engineLocker := world.lockEngine()
	world.mu.Lock()
	world.releasePendingShutdownJobsLocked(pending)
	readyErr := world.drainSaveCompletionsLocked()
	unresolvedErr := world.unresolvedSaveErrorLocked()
	world.mu.Unlock()
	engineLocker.Unlock()
	return errors.Join(persistenceErr, readyErr, unresolvedErr, ctxErr)
}

// ShutdownContextError 汇总关服等待超时前已就绪和仍待重试的存档失败。
func (world *World) ShutdownContextError(ctxErr, persistenceErr error) error {
	return world.shutdownOwnerContextError(ctxErr, persistenceErr, nil)
}

func (world *World) unresolvedSaveErrorLocked() error {
	type unresolved struct {
		region  storage.RegionKey
		retryID uint64
		err     error
	}
	entries := make([]unresolved, 0, len(world.retry)+len(world.retryInFlight))
	for region, cohorts := range world.retry {
		for _, cohort := range cohorts {
			if cohort.LastError != nil {
				entries = append(entries, unresolved{
					region: region, retryID: cohort.Job.RetryID, err: cohort.LastError,
				})
			}
		}
	}
	for retryID, cohort := range world.retryInFlight {
		if cohort.LastError != nil {
			entries = append(entries, unresolved{
				region: cohort.Job.Region, retryID: retryID, err: cohort.LastError,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].region != entries[j].region {
			return regionKeyLess(entries[i].region, entries[j].region)
		}
		return entries[i].retryID < entries[j].retryID
	})
	errs := make([]error, 0, len(entries))
	for _, entry := range entries {
		errs = append(errs, fmt.Errorf(
			"unresolved save region %+v retry %d: %w",
			entry.region,
			entry.retryID,
			entry.err,
		))
	}
	return errors.Join(errs...)
}
