package persistence

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
)

// Hostiles 持有夜行者聚合存档的异步持久化生命周期。
type Hostiles struct {
	store        storage.HostileMobStore
	options      Options
	mu           sync.Mutex
	completionMu sync.Mutex
	// records 是最近一次 Observe 的权威值快照（按 ID 严格升序），也是构造
	// 期加载记录的初始载体：Restore 在首 tick 前读取一次用于恢复接线。
	records     []storage.StoredHostileMob
	persisted   uint64
	dirty       bool
	inFlight    bool
	inFlightJob hostileSaveJob
	retry       *hostileSaveJob
	jobs        chan hostileSaveJob
	completions chan hostileSaveCompletion
	ctx         context.Context
	cancel      context.CancelFunc
	waitGroup   sync.WaitGroup
	closed      bool
	closeOnce   sync.Once
}

type hostileSaveJob struct {
	Save     storage.HostileMobsSave
	Attempt  uint32
	NextTick uint64
}

type hostileSaveCompletion struct {
	Job hostileSaveJob
	Err error
}

// NewHostiles 构造夜行者持久化协调器并启动单个持久化 worker。
func NewHostiles(
	store storage.HostileMobStore,
	loaded storage.StoredHostileMobs,
	options Options,
) *Hostiles {
	ctx, cancel := context.WithCancel(context.Background())
	persistence := &Hostiles{
		store:       store,
		options:     options,
		records:     cloneAndSortHostileRecords(loaded.Records),
		persisted:   loaded.Revision,
		jobs:        make(chan hostileSaveJob, 1),
		completions: make(chan hostileSaveCompletion, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
	persistence.waitGroup.Add(1)
	go persistence.worker()
	return persistence
}

// Restore 返回启动加载的权威夜行者值快照的深拷贝，调用方持有后可用于
// 管理器恢复接线。返回按 ID 严格升序。
func (p *Hostiles) Restore() []sim.HostileMob {
	p.mu.Lock()
	defer p.mu.Unlock()
	mobs := make([]sim.HostileMob, 0, len(p.records))
	for _, record := range p.records {
		mobs = append(mobs, hostileRestoreRecord(record))
	}
	return mobs
}

// Observe 合并一份权威夜行者值快照：与已保存记录逐字段比较，任一差异即
// 标记存档 dirty。输入来自持有 stepMu 的 tick 路径（`Engine.HostileMobs`
// 的排序值快照），本方法冻结深拷贝，调用方后续的任何变化都不影响已冻结
// 的快照。容量守卫与权威侧 `maxHostiles` 同源，越界是不可达的防御路径。
func (p *Hostiles) Observe(active []sim.HostileMob) {
	if len(active) > storage.MaxHostileMobs {
		panic("server: hostile persistence exceeds stored record limit")
	}
	records := make([]storage.StoredHostileMob, 0, len(active))
	for _, mob := range active {
		records = append(records, hostileStorageRecord(mob))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if slices.Equal(records, p.records) {
		return
	}
	p.records = records
	p.dirty = true
}

// Poll 在 tick 边界回收保存完成并按 autosave 节奏派发新保存：完成处理与
// 重试调度沿用伙伴持久化的既有语义——失败按指数退避排到 `NextTick`，in-flight
// 期间的观察变化保持 dirty，下一轮保存合并 latest。任何保存失败都经返回值
// 上报，由调用方记日志；旧正式文件由存储层原子写保证原样保留。
func (p *Hostiles) Poll(tick uint64) error {
	p.completionMu.Lock()
	defer p.completionMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()

	var result error
	for {
		select {
		case completion := <-p.completions:
			if err := p.applyCompletionLocked(completion, tick); err != nil {
				result = errors.Join(result, err)
			}
		default:
			goto drained
		}
	}

drained:
	if p.inFlight || p.closed {
		return result
	}
	if p.retry != nil {
		if p.retry.NextTick <= tick {
			job := cloneHostileSaveJob(*p.retry)
			if p.dispatchLocked(job) {
				p.retry = nil
			}
		}
		return result
	}
	if p.dirty && tick%p.options.AutosaveTicks == 0 {
		p.dispatchLocked(p.latestJobLocked())
	}
	return result
}

// Flush 作为关服屏障把最新权威快照落盘：等待继承的 in-flight、重派失败的
// retry、补写剩余 dirty，全部收敛后才返回。ctx 取消只中断等待，worker 与
// 重试状态原样保留（调用方可换 ctx 重试）。
func (p *Hostiles) Flush(ctx context.Context) error {
	if ctx == nil {
		panic("server: nil hostile flush context")
	}
	p.completionMu.Lock()
	defer p.completionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	inherited := p.inFlight
	hasRetry := !inherited && p.retry != nil
	hasDirty := p.dirty
	p.mu.Unlock()

	switch {
	case inherited:
		if err := p.waitForInflight(ctx); err != nil {
			return err
		}
	case hasRetry:
		if err := p.dispatchAndWait(ctx, true); err != nil {
			return err
		}
	case hasDirty:
		if err := p.dispatchAndWait(ctx, false); err != nil {
			return err
		}
	default:
		return nil
	}

	p.mu.Lock()
	dirty := p.dirty
	p.mu.Unlock()
	if !dirty {
		return nil
	}
	return p.dispatchAndWait(ctx, false)
}

func (p *Hostiles) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		p.cancel()
		p.waitGroup.Wait()
	})
}

func (p *Hostiles) worker() {
	defer p.waitGroup.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job := <-p.jobs:
			err := p.store.SaveHostileMobs(p.ctx, cloneHostileSave(job.Save))
			select {
			case p.completions <- hostileSaveCompletion{Job: job, Err: err}:
			case <-p.ctx.Done():
				return
			}
		}
	}
}

func (p *Hostiles) dispatchAndWait(ctx context.Context, retry bool) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return p.ctx.Err()
	}
	var job hostileSaveJob
	if retry {
		if p.retry == nil {
			p.mu.Unlock()
			return nil
		}
		job = cloneHostileSaveJob(*p.retry)
	} else {
		if !p.dirty {
			p.mu.Unlock()
			return nil
		}
		job = p.latestJobLocked()
	}
	if !p.dispatchLocked(job) {
		p.mu.Unlock()
		return nil
	}
	if retry {
		p.retry = nil
	}
	p.mu.Unlock()
	return p.waitForInflight(ctx)
}

func (p *Hostiles) waitForInflight(ctx context.Context) error {
	for {
		select {
		case completion := <-p.completions:
			p.mu.Lock()
			matched := p.inFlight &&
				p.inFlightJob.Save.Revision == completion.Job.Save.Revision
			err := p.applyCompletionLocked(completion, 0)
			p.mu.Unlock()
			if matched {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		}
	}
}

func (p *Hostiles) dispatchLocked(job hostileSaveJob) bool {
	if p.closed || p.inFlight {
		return false
	}
	queued := cloneHostileSaveJob(job)
	select {
	case p.jobs <- queued:
		p.inFlight = true
		p.inFlightJob = cloneHostileSaveJob(job)
		return true
	default:
		return false
	}
}

func (p *Hostiles) applyCompletionLocked(
	completion hostileSaveCompletion,
	tick uint64,
) error {
	if !p.inFlight || p.inFlightJob.Save.Revision != completion.Job.Save.Revision {
		return nil
	}
	p.inFlight = false
	p.inFlightJob = hostileSaveJob{}
	if completion.Err != nil {
		retry := cloneHostileSaveJob(completion.Job)
		attempt := retry.Attempt
		if attempt == 0 {
			attempt = 1
		}
		retry.NextTick = saturatingAddUint64(
			tick,
			retryDelay(p.options.RetryBaseTicks, p.options.RetryMaxTicks, attempt),
		)
		if attempt < ^uint32(0) {
			retry.Attempt = attempt + 1
		}
		p.retry = &retry
		p.dirty = true
		return completion.Err
	}
	p.persisted = completion.Job.Save.Revision
	p.retry = nil
	// dirty 重判取「保存期间是否又有新观察」口径：完成时刻的 records 已含
	// in-flight 期间合并的最新快照，与本次落盘载荷不同即仍脏。
	p.dirty = !slices.Equal(p.records, completion.Job.Save.Records)
	return nil
}

func (p *Hostiles) latestJobLocked() hostileSaveJob {
	return hostileSaveJob{
		Save: storage.HostileMobsSave{
			Revision: p.persisted + 1,
			Records:  slices.Clone(p.records),
		},
		Attempt: 1,
	}
}

func cloneHostileSaveJob(job hostileSaveJob) hostileSaveJob {
	job.Save = cloneHostileSave(job.Save)
	return job
}

func cloneHostileSave(save storage.HostileMobsSave) storage.HostileMobsSave {
	save.Records = slices.Clone(save.Records)
	return save
}

func cloneAndSortHostileRecords(records []storage.StoredHostileMob) []storage.StoredHostileMob {
	clone := slices.Clone(records)
	slices.SortFunc(clone, func(a, b storage.StoredHostileMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return clone
}

// hostileStorageRecord 把权威夜行者值快照转换为存档记录：字段面一一对应，
// 路径与 worker generation 等运行时派生物不在权威值内，天然不落盘。
func hostileStorageRecord(mob sim.HostileMob) storage.StoredHostileMob {
	return storage.StoredHostileMob{
		ID:              mob.ID,
		Dimension:       mob.Dimension,
		Position:        [3]float32(mob.State.Position),
		Velocity:        [3]float32(mob.State.Velocity),
		OnGround:        mob.State.OnGround,
		Yaw:             mob.Yaw,
		Health:          mob.Health,
		AttackCooldown:  mob.AttackCooldown,
		HurtCooldown:    mob.HurtCooldown,
		BurnCooldown:    mob.BurnCooldown,
		HasTarget:       mob.HasTarget,
		PlayerID:        mob.PlayerID,
		NextRepathTicks: mob.NextRepathTicks,
		DistantTicks:    mob.DistantTicks,
	}
}

// hostileRestoreRecord 把存档记录恢复为权威值快照：与 hostileStorageRecord
// 互为逆变换，供启动恢复接线使用。
func hostileRestoreRecord(record storage.StoredHostileMob) sim.HostileMob {
	return sim.HostileMob{
		ID:        record.ID,
		Dimension: record.Dimension,
		State: physics.State{
			Position: mgl32.Vec3(record.Position),
			Velocity: mgl32.Vec3(record.Velocity),
			OnGround: record.OnGround,
		},
		Yaw:             record.Yaw,
		Health:          record.Health,
		AttackCooldown:  record.AttackCooldown,
		HurtCooldown:    record.HurtCooldown,
		BurnCooldown:    record.BurnCooldown,
		HasTarget:       record.HasTarget,
		PlayerID:        record.PlayerID,
		NextRepathTicks: record.NextRepathTicks,
		DistantTicks:    record.DistantTicks,
	}
}
