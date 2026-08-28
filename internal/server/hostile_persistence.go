// 本文件实现 server 侧夜行者聚合存档的异步持久化（hostilePersistence）：
// 观察权威快照、容量 1 的单飞行保存、revision 递增、失败退避重试、autosave
// 节奏与关服 `Flush`/`Close` 屏障。并发形状与 `companion_persistence.go`
// 完全同构（mu 保护观察状态、completionMu 串行化完成回收与关服等待、
// worker goroutine 只持有深拷贝载荷）——只有一个消费者，刻意不抽通用
// generic。
//
// 与伙伴持久化的语义差异：夜行者没有任务域与摘要，观察输入就是 sim 的
// 按 ID 排序值快照一份；脏判定因此是纯记录比较。加载与恢复的启动矩阵
// （missing/corrupt/future/read error）由 `NewHost` 在构造期完成，本类型
// 只负责构造后的保存闭环。
package server

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
)

type hostilePersistence struct {
	store        storage.HostileMobStore
	config       Config
	mu           sync.Mutex
	completionMu sync.Mutex
	// records 是最近一次 Observe 的权威值快照（按 ID 严格升序），也是构造
	// 期加载记录的初始载体：newWorld 在首 tick 前读取一次用于恢复接线。
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

func newHostilePersistence(
	store storage.HostileMobStore,
	loaded storage.StoredHostileMobs,
	config Config,
) *hostilePersistence {
	ctx, cancel := context.WithCancel(context.Background())
	persistence := &hostilePersistence{
		store:       store,
		config:      config,
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

// Observe 合并一份权威夜行者值快照：与已保存记录逐字段比较，任一差异即
// 标记存档 dirty。输入来自持有 stepMu 的 tick 路径（`Engine.HostileMobs`
// 的排序值快照），本方法冻结深拷贝，调用方后续的任何变化都不影响已冻结
// 的快照。容量守卫与权威侧 `maxHostiles` 同源，越界是不可达的防御路径。
func (p *hostilePersistence) Observe(active []contract.HostileMob) {
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
func (p *hostilePersistence) Poll(tick uint64) error {
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
	if p.dirty && tick%p.config.AutosaveTicks == 0 {
		p.dispatchLocked(p.latestJobLocked())
	}
	return result
}

// Flush 作为关服屏障把最新权威快照落盘：等待继承的 in-flight、重派失败的
// retry、补写剩余 dirty，全部收敛后才返回。ctx 取消只中断等待，worker 与
// 重试状态原样保留（调用方可换 ctx 重试）。
func (p *hostilePersistence) Flush(ctx context.Context) error {
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

func (p *hostilePersistence) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		p.cancel()
		p.waitGroup.Wait()
	})
}

func (p *hostilePersistence) worker() {
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

func (p *hostilePersistence) dispatchAndWait(ctx context.Context, retry bool) error {
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

func (p *hostilePersistence) waitForInflight(ctx context.Context) error {
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

func (p *hostilePersistence) dispatchLocked(job hostileSaveJob) bool {
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

func (p *hostilePersistence) applyCompletionLocked(
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
			retryDelay(p.config.RetryBaseTicks, p.config.RetryMaxTicks, attempt),
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

func (p *hostilePersistence) latestJobLocked() hostileSaveJob {
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
func hostileStorageRecord(mob contract.HostileMob) storage.StoredHostileMob {
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
func hostileRestoreRecord(record storage.StoredHostileMob) contract.HostileMob {
	return contract.HostileMob{
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
