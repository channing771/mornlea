package persistence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/storage"
)

// Companions 持有伙伴聚合存档的异步持久化生命周期。
type Companions struct {
	store        storage.CompanionStore
	options      Options
	mu           sync.Mutex
	completionMu sync.Mutex
	records      []companion.Body
	namespace    storage.CompanionIdentity
	lifecycles   []storage.StoredCompanionLifecycle
	// tasks 是最近一次 Observe 的任务域观察输入；任务状态变化即令存档
	// dirty。latestJobLocked 在投递保存前把它转换为 storage 载荷（含
	// Planning/Validating→Queued 的保存侧归一）。
	tasks []companion.TaskQueueState
	// loadedQueues 是启动加载时存档携带的任务域载荷，构造后不变；Restore
	// 在构造后调用一次用于恢复接线。
	loadedQueues []storage.StoredCompanionQueue
	persisted    uint64
	dirty        bool
	inFlight     bool
	inFlightJob  companionSaveJob
	retry        *companionSaveJob
	jobs         chan companionSaveJob
	completions  chan companionSaveCompletion
	ctx          context.Context
	cancel       context.CancelFunc
	waitGroup    sync.WaitGroup
	closed       bool
	closeOnce    sync.Once
}

type companionSaveJob struct {
	Save     storage.CompanionSave
	Attempt  uint32
	NextTick uint64
}

type companionSaveCompletion struct {
	Job companionSaveJob
	Err error
}

// NewCompanions 构造伙伴持久化协调器并启动单个持久化 worker。
func NewCompanions(
	store storage.CompanionStore,
	loaded storage.StoredCompanions,
	options Options,
) *Companions {
	ctx, cancel := context.WithCancel(context.Background())
	persistence := &Companions{
		store:        store,
		options:      options,
		records:      cloneAndSortCompanionBodies(loaded.Records),
		namespace:    loaded.AgentNamespaceID,
		lifecycles:   slices.Clone(loaded.Lifecycles),
		loadedQueues: cloneStoredQueues(loaded.Queues),
		persisted:    loaded.Revision,
		jobs:         make(chan companionSaveJob, 1),
		completions:  make(chan companionSaveCompletion, 1),
		ctx:          ctx,
		cancel:       cancel,
	}
	persistence.waitGroup.Add(1)
	go persistence.worker()
	return persistence
}

// Restore 返回启动加载的身体快照与任务域载荷的深拷贝，调用方持有后可用于
// 管理器恢复接线。
func (p *Companions) Restore() ([]companion.Body, []storage.StoredCompanionQueue) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.records), cloneStoredQueues(p.loadedQueues)
}

// AgentNamespaceID 返回启动期已完成迁移并持久化的 namespace 身份副本。
// 返回值是固定数组，不暴露协调器内部可变状态。
func (p *Companions) AgentNamespaceID() storage.CompanionIdentity {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.namespace
}

// MemoryLifecycle 返回指定伙伴当前 v5 lifecycle/memory 元数据的值副本。
// 调用方只能把它用于 Agent reconcile/commit 的权威关联，不能原地修改。
func (p *Companions) MemoryLifecycle(id companion.ID) (storage.StoredCompanionLifecycle, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, lifecycle := range p.lifecycles {
		if lifecycle.ID == id {
			return lifecycle, true
		}
	}
	return storage.StoredCompanionLifecycle{}, false
}

// MemoryLifecycles 返回全部当前 v5 lifecycle/memory 元数据的深拷贝。
func (p *Companions) MemoryLifecycles() []storage.StoredCompanionLifecycle {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.lifecycles)
}

// ReplaceActiveMemory 以 epoch 与旧 revision 作为 CAS 围栏，整份替换当前
// Agent memory mirror。成功只标记聚合存档为 dirty，磁盘 I/O 仍由 Poll 或
// Flush 在既有单 worker 通道执行。
func (p *Companions) ReplaceActiveMemory(
	id companion.ID,
	epoch uint64,
	expectedRevision uint64,
	nextRevision uint64,
	operationID storage.CompanionIdentity,
	summary string,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return context.Canceled
	}
	if _, err := p.nextAggregateRevisionLocked(); err != nil {
		return err
	}
	if epoch == 0 || nextRevision == 0 || nextRevision <= expectedRevision ||
		!operationID.Valid() || len(summary) > companion.MaxDialogueSummaryBytes ||
		!utf8.ValidString(summary) || strings.ContainsRune(summary, '\x00') {
		return fmt.Errorf("%w: invalid active companion memory replacement", storage.ErrCorrupt)
	}
	for index, lifecycle := range p.lifecycles {
		if lifecycle.ID != id {
			continue
		}
		if lifecycle.Active && lifecycle.MemoryEpoch == epoch &&
			lifecycle.MemoryRevision == nextRevision &&
			lifecycle.MemoryOperationID == operationID && lifecycle.Summary == summary {
			return nil
		}
		if !lifecycle.Active || lifecycle.MemoryEpoch != epoch ||
			lifecycle.MemoryRevision != expectedRevision {
			return fmt.Errorf("companion memory CAS conflict")
		}
		next := slices.Clone(p.lifecycles)
		next[index].MemoryRevision = nextRevision
		next[index].MemoryOperationID = operationID
		next[index].Summary = summary
		next[index].TombstoneOperationID = storage.CompanionIdentity{}
		p.lifecycles = next
		p.dirty = true
		return nil
	}
	return fmt.Errorf("companion memory lifecycle not found")
}

// Observe 合并权威身体与任务域观察输入。Agent memory 只经 lifecycle CAS
// 更新，任务观察不能推导或改写 mirror。
func (p *Companions) Observe(
	active []companion.Body,
	tasks []companion.TaskQueueState,
) {
	p.mu.Lock()
	defer p.mu.Unlock()
	byID := make(map[companion.ID]companion.Body, len(p.records)+len(active))
	for _, body := range p.records {
		byID[body.ID] = body
	}
	for _, body := range active {
		byID[body.ID] = body
	}
	if len(byID) > companion.MaxStored {
		panic("server: companion persistence exceeds stored record limit")
	}
	records := make([]companion.Body, 0, len(byID))
	for _, body := range byID {
		records = append(records, body)
	}
	sortCompanionBodies(records)
	tasksChanged := !equalTaskQueueStates(tasks, p.tasks)
	if slices.Equal(records, p.records) && !tasksChanged {
		return
	}
	p.records = records
	p.tasks = cloneTaskQueueStates(tasks)
	p.dirty = true
}

// equalTaskQueueStates 比较两份任务域观察输入是否逐字段相等（含计划步骤）。
func equalTaskQueueStates(left, right []companion.TaskQueueState) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !equalTaskQueueState(left[index], right[index]) {
			return false
		}
	}
	return true
}

func equalTaskQueueState(left, right companion.TaskQueueState) bool {
	if left.ID != right.ID || left.HasCurrent != right.HasCurrent ||
		!slices.Equal(left.Pending, right.Pending) {
		return false
	}
	if !left.HasCurrent {
		return true
	}
	return equalTask(left.Current, right.Current)
}

func equalTask(left, right companion.Task) bool {
	return left.Generation == right.Generation &&
		left.Command == right.Command &&
		left.StepIndex == right.StepIndex &&
		left.State == right.State &&
		left.StartTick == right.StartTick &&
		left.DeadlineTicks == right.DeadlineTicks &&
		left.FailReason == right.FailReason &&
		equalPlan(left.Plan, right.Plan)
}

func equalPlan(left, right companion.Plan) bool {
	if left.Summary != right.Summary || len(left.Steps) != len(right.Steps) {
		return false
	}
	for index := range left.Steps {
		if left.Steps[index] != right.Steps[index] {
			return false
		}
	}
	return true
}

// cloneTaskQueueStates 深拷贝任务域观察输入：Pending 切片与当前任务的计划
// 步骤都独立于调用方，Observe 之后的任何调用方修改都不影响已冻结的快照。
func cloneTaskQueueStates(states []companion.TaskQueueState) []companion.TaskQueueState {
	cloned := make([]companion.TaskQueueState, len(states))
	for index := range states {
		cloned[index] = states[index]
		cloned[index].Pending = slices.Clone(states[index].Pending)
		cloned[index].Current.Plan.Steps = slices.Clone(states[index].Current.Plan.Steps)
	}
	return cloned
}

func (p *Companions) Poll(tick uint64) error {
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
			job := cloneCompanionSaveJob(*p.retry)
			if p.dispatchLocked(job) {
				p.retry = nil
			}
		}
		return result
	}
	if p.dirty && tick%p.options.AutosaveTicks == 0 {
		job, err := p.latestJobLocked()
		if err != nil {
			return errors.Join(result, err)
		}
		p.dispatchLocked(job)
	}
	return result
}

func (p *Companions) Flush(ctx context.Context) error {
	if ctx == nil {
		panic("server: nil companion flush context")
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

func (p *Companions) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		p.cancel()
		p.waitGroup.Wait()
	})
}

func (p *Companions) worker() {
	defer p.waitGroup.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job := <-p.jobs:
			err := p.store.SaveCompanions(p.ctx, cloneCompanionSave(job.Save))
			select {
			case p.completions <- companionSaveCompletion{Job: job, Err: err}:
			case <-p.ctx.Done():
				return
			}
		}
	}
}

func (p *Companions) dispatchAndWait(ctx context.Context, retry bool) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return p.ctx.Err()
	}
	var job companionSaveJob
	if retry {
		if p.retry == nil {
			p.mu.Unlock()
			return nil
		}
		job = cloneCompanionSaveJob(*p.retry)
	} else {
		if !p.dirty {
			p.mu.Unlock()
			return nil
		}
		var err error
		job, err = p.latestJobLocked()
		if err != nil {
			p.mu.Unlock()
			return err
		}
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

func (p *Companions) waitForInflight(ctx context.Context) error {
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

func (p *Companions) dispatchLocked(job companionSaveJob) bool {
	if p.closed || p.inFlight {
		return false
	}
	queued := cloneCompanionSaveJob(job)
	select {
	case p.jobs <- queued:
		p.inFlight = true
		p.inFlightJob = cloneCompanionSaveJob(job)
		return true
	default:
		return false
	}
}

func (p *Companions) applyCompletionLocked(
	completion companionSaveCompletion,
	tick uint64,
) error {
	if !p.inFlight || p.inFlightJob.Save.Revision != completion.Job.Save.Revision {
		return nil
	}
	p.inFlight = false
	p.inFlightJob = companionSaveJob{}
	if completion.Err != nil {
		retry := cloneCompanionSaveJob(completion.Job)
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
	// 任务域的 dirty 重判使用「未激活丢弃」口径：身体记录尚未出现的队列
	// 无法落盘（编码要求队列关联 active 记录），保持 dirty 让激活后的首次
	// 保存补上完整任务载荷。
	currentQueues, droppedPending := companionQueuesForSave(
		p.tasks, activeCompanionBodies(p.records, p.lifecycles),
	)
	p.dirty = !slices.Equal(p.records, completion.Job.Save.Records) ||
		p.namespace != completion.Job.Save.AgentNamespaceID ||
		!slices.Equal(p.lifecycles, completion.Job.Save.Lifecycles) ||
		droppedPending ||
		!equalStoredQueues(currentQueues, completion.Job.Save.Queues)
	return nil
}

func (p *Companions) latestJobLocked() (companionSaveJob, error) {
	nextRevision, err := p.nextAggregateRevisionLocked()
	if err != nil {
		return companionSaveJob{}, err
	}
	queues, _ := companionQueuesForSave(
		p.tasks, activeCompanionBodies(p.records, p.lifecycles),
	)
	return companionSaveJob{
		Save: storage.CompanionSave{
			Revision:         nextRevision,
			AgentNamespaceID: p.namespace,
			Records:          slices.Clone(p.records),
			Lifecycles:       slices.Clone(p.lifecycles),
			Queues:           queues,
		},
		Attempt: 1,
	}, nil
}

func (p *Companions) nextAggregateRevisionLocked() (uint64, error) {
	highest := p.persisted
	if p.inFlight && p.inFlightJob.Save.Revision > highest {
		highest = p.inFlightJob.Save.Revision
	}
	if p.retry != nil && p.retry.Save.Revision > highest {
		highest = p.retry.Save.Revision
	}
	if highest == math.MaxUint64 {
		return 0, fmt.Errorf("%w: companion aggregate revision overflow", storage.ErrCorrupt)
	}
	return highest + 1, nil
}

func cloneCompanionSaveJob(job companionSaveJob) companionSaveJob {
	job.Save = cloneCompanionSave(job.Save)
	return job
}

func cloneCompanionSave(save storage.CompanionSave) storage.CompanionSave {
	save.Records = slices.Clone(save.Records)
	save.Lifecycles = slices.Clone(save.Lifecycles)
	save.Queues = cloneStoredQueues(save.Queues)
	return save
}

// cloneStoredQueues 深拷贝任务域载荷（计划步骤与 FIFO），返回值与输入切片
// 完全独立。
func cloneStoredQueues(queues []storage.StoredCompanionQueue) []storage.StoredCompanionQueue {
	if queues == nil {
		return nil
	}
	cloned := make([]storage.StoredCompanionQueue, len(queues))
	for index := range queues {
		cloned[index] = queues[index]
		cloned[index].Current.PlanSteps = slices.Clone(queues[index].Current.PlanSteps)
		cloned[index].Pending = slices.Clone(queues[index].Pending)
	}
	return cloned
}

// companionQueuesForSave 把任务域观察输入转换为存档载荷并执行保存侧归一：
// Planning/Validating 尚未通过验证，按 Queued + 原始指令落盘（spec：模型
// 计划只在 Validating 成功后落盘）；Running 精确保留计划、步骤索引与
// deadline；终态快照（防御路径，正常快照不会出现）不落当前任务。records
// 是当前已知身体记录：队列必须关联记录才能编码，身体尚未激活（出生扫描
// 在途）的伙伴的队列被丢弃并经 dropped 报告——调用方保持 dirty，激活后的
// 首次保存补上完整载荷。返回值深拷贝自输入，与调用方切片完全独立。
func companionQueuesForSave(
	states []companion.TaskQueueState,
	records []companion.Body,
) (queues []storage.StoredCompanionQueue, dropped bool) {
	known := make(map[companion.ID]struct{}, len(records))
	for _, body := range records {
		known[body.ID] = struct{}{}
	}
	queues = make([]storage.StoredCompanionQueue, 0, len(states))
	for _, state := range states {
		if _, exists := known[state.ID]; !exists {
			dropped = true
			continue
		}
		queue := storage.StoredCompanionQueue{ID: state.ID}
		if state.HasCurrent {
			current := state.Current
			switch {
			case current.State.Terminal():
				// 终态任务在快照瞬间已被清出当前槽；这里只是防御，
				// 不落任何任务事实。
			case current.State == companion.TaskPlanning ||
				current.State == companion.TaskValidating:
				queue.HasCurrent = true
				queue.Current = storage.StoredCompanionTask{
					Command: string(current.Command),
					State:   companion.TaskQueued,
				}
			default:
				queue.HasCurrent = true
				queue.Current = storage.StoredCompanionTask{
					Command:       string(current.Command),
					PlanSteps:     slices.Clone(current.Plan.Steps),
					StepIndex:     current.StepIndex,
					State:         current.State,
					StartTick:     current.StartTick,
					DeadlineTicks: current.DeadlineTicks,
				}
			}
		}
		if len(state.Pending) != 0 {
			queue.Pending = make([]string, len(state.Pending))
			for index, command := range state.Pending {
				queue.Pending[index] = string(command)
			}
		}
		if queue.HasCurrent || len(queue.Pending) != 0 {
			queues = append(queues, queue)
		}
	}
	return queues, dropped
}

// equalStoredQueues 逐字段比较两份存档载荷（含计划步骤、FIFO 顺序与最近
// 对话摘要——摘要差异不得被判 clean，否则摘要变化后的保存会被跳过），供
// 保存完成后的 dirty 重判使用。
func equalStoredQueues(left, right []storage.StoredCompanionQueue) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.ID != b.ID || a.HasCurrent != b.HasCurrent || a.Summary != b.Summary ||
			!slices.Equal(a.Pending, b.Pending) {
			return false
		}
		if a.HasCurrent && !equalStoredCompanionTask(a.Current, b.Current) {
			return false
		}
	}
	return true
}

func equalStoredCompanionTask(a, b storage.StoredCompanionTask) bool {
	return a.Command == b.Command &&
		a.StepIndex == b.StepIndex &&
		a.State == b.State &&
		a.StartTick == b.StartTick &&
		a.DeadlineTicks == b.DeadlineTicks &&
		a.FailReason == b.FailReason &&
		slices.Equal(a.PlanSteps, b.PlanSteps)
}

func cloneAndSortCompanionBodies(records []companion.Body) []companion.Body {
	clone := slices.Clone(records)
	sortCompanionBodies(clone)
	return clone
}

func activeCompanionBodies(
	records []companion.Body,
	lifecycles []storage.StoredCompanionLifecycle,
) []companion.Body {
	if len(lifecycles) == 0 {
		return records
	}
	active := make(map[companion.ID]struct{}, len(lifecycles))
	for _, lifecycle := range lifecycles {
		if lifecycle.Active {
			active[lifecycle.ID] = struct{}{}
		}
	}
	result := make([]companion.Body, 0, len(active))
	for _, body := range records {
		if _, ok := active[body.ID]; ok {
			result = append(result, body)
		}
	}
	return result
}

// sortCompanionBodies 就地把身体记录按伙伴 ID 字节序升序排列，与
// companionManager.orderedIDs 使用同一确定性次序；ID 唯一，无并列元素，
// 排序稳定性不参与结果。
func sortCompanionBodies(records []companion.Body) {
	slices.SortFunc(records, func(left, right companion.Body) int {
		return bytes.Compare(left.ID[:], right.ID[:])
	})
}

// RecordsAndRevision 返回当前记录与持久化版本的深拷贝快照，供根包集成测试观测。
func (p *Companions) RecordsAndRevision() ([]companion.Body, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.records), p.persisted
}

// IsClosed 报告持久化是否已关闭。
func (p *Companions) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// Context 返回内部 worker 的 context，供测试观测取消状态。
func (p *Companions) Context() context.Context { return p.ctx }

// HasPendingCompletion 报告完成通道是否有待回收的完成。
func (p *Companions) HasPendingCompletion() bool { return len(p.completions) != 0 }

// WaitGroup 返回内部等待组，供测试等待 worker 退出。
func (p *Companions) WaitGroup() *sync.WaitGroup { return &p.waitGroup }
