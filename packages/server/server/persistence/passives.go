package persistence

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// Passives 持有被动牛聚合存档的异步持久化生命周期。
type Passives struct {
	store        storage.PassiveMobStore
	options      Options
	mu           sync.Mutex
	completionMu sync.Mutex
	// records 是最近一次 Observe 的权威值快照（按 ID 严格升序），也是构造
	// 期加载记录的初始载体：Restore 在首 tick 前读取一次用于恢复接线。
	records     []storage.StoredPassiveMob
	persisted   uint64
	dirty       bool
	inFlight    bool
	inFlightJob passiveSaveJob
	retry       *passiveSaveJob
	jobs        chan passiveSaveJob
	completions chan passiveSaveCompletion
	ctx         context.Context
	cancel      context.CancelFunc
	waitGroup   sync.WaitGroup
	closed      bool
	closeOnce   sync.Once
}

type passiveSaveJob struct {
	Save     storage.PassiveMobsSave
	Attempt  uint32
	NextTick uint64
}

type passiveSaveCompletion struct {
	Job passiveSaveJob
	Err error
}

// NewPassives 构造被动牛持久化协调器并启动单个持久化 worker。
func NewPassives(
	store storage.PassiveMobStore,
	loaded storage.StoredPassiveMobs,
	options Options,
) *Passives {
	ctx, cancel := context.WithCancel(context.Background())
	persistence := &Passives{
		store:       store,
		options:     options,
		records:     cloneAndSortPassiveRecords(loaded.Records),
		persisted:   loaded.Revision,
		jobs:        make(chan passiveSaveJob, 1),
		completions: make(chan passiveSaveCompletion, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
	persistence.waitGroup.Add(1)
	go persistence.worker()
	return persistence
}

// Restore 返回启动加载的权威被动牛值快照的深拷贝，调用方持有后可用于
// 管理器恢复接线。返回按 ID 严格升序。
func (p *Passives) Restore() []contract.PassiveMob {
	p.mu.Lock()
	defer p.mu.Unlock()
	mobs := make([]contract.PassiveMob, 0, len(p.records))
	for _, record := range p.records {
		mobs = append(mobs, passiveRestoreRecord(record))
	}
	return mobs
}

// Observe 合并一份权威被动牛值快照：与已保存记录逐字段比较，任一差异即
// 标记存档 dirty。输入来自持有 stepMu 的 tick 路径，本方法冻结深拷贝，
// 调用方后续的任何变化都不影响已冻结的快照。容量守卫与权威侧 `maxPassives`
// 同源，越界是不可达的防御路径。
func (p *Passives) Observe(active []contract.PassiveMob) {
	if len(active) > storage.MaxPassiveMobs {
		panic("server: passive persistence exceeds stored record limit")
	}
	records := make([]storage.StoredPassiveMob, 0, len(active))
	for _, mob := range active {
		records = append(records, passiveStorageRecord(mob))
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
// 重试调度沿用夜行者持久化的既有语义——失败按指数退避排到 `NextTick`，in-flight
// 期间的观察变化保持 dirty，下一轮保存合并 latest。任何保存失败都经返回值
// 上报，由调用方记日志；旧正式文件由存储层原子写保证原样保留。
func (p *Passives) Poll(tick uint64) error {
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
			job := clonePassiveSaveJob(*p.retry)
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
func (p *Passives) Flush(ctx context.Context) error {
	if ctx == nil {
		panic("server: nil passive flush context")
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

func (p *Passives) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		p.cancel()
		p.waitGroup.Wait()
	})
}

func (p *Passives) worker() {
	defer p.waitGroup.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job := <-p.jobs:
			err := p.store.SavePassiveMobs(p.ctx, clonePassiveSave(job.Save))
			select {
			case p.completions <- passiveSaveCompletion{Job: job, Err: err}:
			case <-p.ctx.Done():
				return
			}
		}
	}
}

func (p *Passives) dispatchAndWait(ctx context.Context, retry bool) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return p.ctx.Err()
	}
	var job passiveSaveJob
	if retry {
		if p.retry == nil {
			p.mu.Unlock()
			return nil
		}
		job = clonePassiveSaveJob(*p.retry)
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

func (p *Passives) waitForInflight(ctx context.Context) error {
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

func (p *Passives) dispatchLocked(job passiveSaveJob) bool {
	if p.closed || p.inFlight {
		return false
	}
	queued := clonePassiveSaveJob(job)
	select {
	case p.jobs <- queued:
		p.inFlight = true
		p.inFlightJob = clonePassiveSaveJob(job)
		return true
	default:
		return false
	}
}

func (p *Passives) applyCompletionLocked(
	completion passiveSaveCompletion,
	tick uint64,
) error {
	if !p.inFlight || p.inFlightJob.Save.Revision != completion.Job.Save.Revision {
		return nil
	}
	p.inFlight = false
	p.inFlightJob = passiveSaveJob{}
	if completion.Err != nil {
		retry := clonePassiveSaveJob(completion.Job)
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

func (p *Passives) latestJobLocked() passiveSaveJob {
	return passiveSaveJob{
		Save: storage.PassiveMobsSave{
			Revision: p.persisted + 1,
			Records:  slices.Clone(p.records),
		},
		Attempt: 1,
	}
}

func clonePassiveSaveJob(job passiveSaveJob) passiveSaveJob {
	job.Save = clonePassiveSave(job.Save)
	return job
}

func clonePassiveSave(save storage.PassiveMobsSave) storage.PassiveMobsSave {
	save.Records = slices.Clone(save.Records)
	return save
}

func cloneAndSortPassiveRecords(records []storage.StoredPassiveMob) []storage.StoredPassiveMob {
	clone := slices.Clone(records)
	slices.SortFunc(clone, func(a, b storage.StoredPassiveMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return clone
}

// passiveStorageRecord 把权威被动牛值快照转换为存档记录：字段面一一对应，
// 逃跑计时与出生区块等运行时派生物不在权威值内，天然不落盘。
func passiveStorageRecord(mob contract.PassiveMob) storage.StoredPassiveMob {
	return storage.StoredPassiveMob{
		ID:        mob.ID,
		Dimension: mob.Dimension,
		Position:  [3]float32(mob.State.Position),
		Velocity:  [3]float32(mob.State.Velocity),
		OnGround:  mob.State.OnGround,
		Yaw:       mob.Yaw,
		Health:    mob.Health,
	}
}

// passiveRestoreRecord 把存档记录恢复为权威值快照：与 passiveStorageRecord
// 互为逆变换，供启动恢复接线使用。
func passiveRestoreRecord(record storage.StoredPassiveMob) contract.PassiveMob {
	return contract.PassiveMob{
		ID:        record.ID,
		Dimension: record.Dimension,
		State: physics.State{
			Position: mgl32.Vec3(record.Position),
			Velocity: mgl32.Vec3(record.Velocity),
			OnGround: record.OnGround,
		},
		Yaw:    record.Yaw,
		Health: record.Health,
	}
}
