// 本文件实现 server 侧 Dialogue worker：台词请求在权威 tick 边界派发、在
// worker goroutine 上执行、结果只在 tick 边界应用。与 Planner worker 同一
// 扇入模式，但并发策略刻意不同——台词是尽力而为的表达平面输出：
//   - 共享模型槽：复用既有 m.semaphore（cap=MaxActive=4，与 Planner 共用）；
//     Planner 在 tick 边界 try-acquire 失败则同 tick 以 PlannerUnavailable 终结，
//     Dialogue try-acquire 失败立即跳过该节点——不排队、不重试，
//     迟到台词在错误语境出现比少一句台词更糟（design.md 否决「排队等槽」）；
//   - 每伙伴最多一个在途请求：slot.dialogueInFlight 在 tick 边界置位/清除，
//     在途期间新节点直接跳过，绝不取消或替换在途请求；
//   - 失败只跳过台词：任何传输/解码失败都只记 debug 级结构化日志，绝不改变
//     任务状态、FIFO 或任何世界事实。
package server

import (
	"context"
	"log/slog"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/storage"
)

// companionDialogue 是 Agent Dialogue 的最小依赖面。生产实现只调用 Agent
// HTTP v1；测试可注入不接触生产 direct-model 配置的受控 adapter。
type companionDialogue interface {
	Dialogue(context.Context, companionDialogueRequest) (companionDialogueResult, error)
	CommitMemory(context.Context, companionMemoryCommitRequest) (companionMemoryCommitResult, error)
}

type companionMemoryReconciler interface {
	currentMemoryFence() (uint64, bool)
	ReconcileMemory(context.Context, storage.StoredCompanionLifecycle) (companionMemoryReconcileResult, error)
}

type companionDialogueReservation struct {
	operationID    string
	memoryEpoch    uint64
	baseRevision   uint64
	summary        string
	line           string
	issuer         companionTaskIssuer
	commitInFlight bool
}

// dialogueOutcome 是一次台词请求的结果，携带伙伴 ID、任务世代、节点身份、
// 发令者身份快照与台词文本供过期判定与广播组装：世代或任务状态不符即过时
// 丢弃（spec：companion-dialogue「并发受限且失败只跳过台词」）。发令者身份
// 在派发时冻结——结果应用时槽位的 currentIssuer 理论上仍属同一任务纪元
// （世代一致未被 BeginHead 提升），快照消除对槽位残留状态的任何依赖。
type dialogueOutcome struct {
	id            companion.ID
	generation    uint64
	attempt       uint64
	taskStepIndex int
	node          companion.DialogueNode
	issuer        companionTaskIssuer
	memoryEpoch   uint64
	result        companionDialogueResult
	err           error
}

type memoryCommitOutcome struct {
	id                companion.ID
	memoryEpoch       uint64
	operationID       string
	committedRevision uint64
	err               error
}

type memoryReconcileOutcome struct {
	fence   uint64
	results []memoryReconcileCompanionOutcome
}

type memoryReconcileCompanionOutcome struct {
	id        companion.ID
	lifecycle storage.StoredCompanionLifecycle
	err       error
}

// requestDialogue 是台词派发在 tick 边界的唯一入口（调用方必须持有 stepMu）。
// 守卫顺序：未知槽位/非 idle 任务预算耗尽/每伙伴在途/inactive 伙伴/共享槽；
// 任一失败都直接跳过（不取消、不排队、不重试）。成功则置在途标记，非 idle
// 节点计入每任务预算并 spawn worker。terminal 标志由 node.Kind 派生（结构性
// 事实，不接受调用方另行声明——D5 评审 Minor-1 的冗余自由度在此收紧为零）。
//
// 触发时机契约（D6 接线，companion_manager.go 的 dispatchDialogueNode 与
// advanceFollowRunner）：任务进入 Running、被选中的计划步骤完成、持续跟随
// 首次到达跟随距离、任务进入四种终态之一。时序约束：终止节点的派发必须
// 发生在终态迁移的同一 tick、且在 FIFO 提升（BeginHead）之前——FIFO 在同一
// tick 的 dispatchPlanning 阶段才推进，本方法在迁移点同步调用即天然满足；
// 迟于 BeginHead 的派发会携带新世代，结果按过时丢弃（D5 评审 Minor-2）。
//
// m.cancel 的调用契约：只在关服序列（beginShutdown/close）被调用，tick 路径
// 绝不调用——dialogueInFlight 的清除只能来自 tick 边界的结果应用
// （applyDialogueOutcome）或进程退出，关服 cancel 令在途 worker 经 ctx.Done
// 放弃结果（共享槽已在模型调用返回后、结果发送前释放，见 `dialogueWorker`）
// （D5 评审 Minor-5 的显式化）。
func (m *companionManager) requestDialogue(id companion.ID, node companion.DialogueNode) {
	slot := m.slots[id]
	if slot == nil {
		// 与 enqueueCommand/stopCompanion 的防御一致：配置缺陷按跳过处理并
		// 保留可诊断日志，绝不伪装成派发成功。
		slog.Error("台词派发找不到伙伴槽位", "companion", id)
		return
	}
	if m.dialogue == nil {
		return
	}
	if !slot.memoryReady {
		return
	}
	taskNode := node.Kind != companion.DialogueNodeIdle
	if taskNode && slot.dialogueRequests >= companion.MaxDialogueRequestsPerTask {
		// 每任务预算（本进程计数，不持久化——design.md 裁决）：结构上
		// 1+≤6+1 封顶，计数只防御未来接线缺陷，不参与正常路径。
		return
	}
	if slot.planningInFlight || slot.dialogueInFlight || slot.dialogueReservation != nil {
		// 每伙伴最多一个在途台词请求：新节点到来时仍有在途即跳过，不取消、
		// 不替换在途请求（spec：「在途请求存在时新节点被跳过」）。跳过即
		// 放弃该节点，绝不补发。
		return
	}
	body, active := m.body(id)
	if !active {
		// 伙伴未激活（出生扫描在途）：跳过该节点，等下一个触发节点。
		return
	}
	select {
	case m.semaphore <- struct{}{}:
	default:
		// 全服四个共享模型槽已满：立即跳过该节点；Planner 在相同条件下会
		// 先进入 Planning，再于同 tick 以 PlannerUnavailable 终结。
		return
	}
	lifecycle, ok := m.companions.MemoryLifecycle(id)
	if !ok || !lifecycle.Active || lifecycle.MemoryEpoch == 0 {
		<-m.semaphore
		return
	}
	fact, ok := agentDialogueFact(node)
	if !ok {
		<-m.semaphore
		return
	}
	digest := m.buildDialogueEnvDigest(body)
	environment := companion.AgentDialogueEnvironment{
		ExposedBlocks: make([]companion.AgentVisibleBlock, len(digest.ExposedBlocks)),
		Heights:       make([]companion.AgentHeight, len(digest.Heights)),
	}
	for index, block := range digest.ExposedBlocks {
		environment.ExposedBlocks[index] = companion.AgentVisibleBlock{
			Position: companion.AgentBlockPosition{X: block.Pos.X, Y: block.Pos.Y, Z: block.Pos.Z},
			BlockID:  uint16(block.Block),
		}
	}
	for index, height := range digest.Heights {
		environment.Heights[index] = companion.AgentHeight{X: height.X, Z: height.Z, Height: int16(height.Height)}
	}
	request := companionDialogueRequest{
		CompanionID: id, Generation: slot.queue.Generation(), MemoryEpoch: lifecycle.MemoryEpoch,
		Persona: slot.definition.ResolvedPersona, Fact: fact, Environment: environment,
		Terminal: node.Kind == companion.DialogueNodeTerminal,
	}
	if slot.dialogueAttempt == ^uint64(0) {
		<-m.semaphore
		return
	}
	slot.dialogueAttempt++
	slot.dialogueInFlight = true
	if taskNode {
		slot.dialogueRequests++
	}
	taskStepIndex := -1
	if current, hasCurrent := slot.queue.Current(); hasCurrent {
		taskStepIndex = current.StepIndex
	}
	m.waitGroup.Add(1)
	go m.dialogueWorker(
		id, slot.queue.Generation(), slot.dialogueAttempt, taskStepIndex,
		node, slot.currentIssuer, request,
	)
}

func agentDialogueFact(node companion.DialogueNode) (companion.AgentDialogueFact, bool) {
	switch node.Kind {
	case companion.DialogueNodeStart:
		return companion.AgentDialogueFact{Kind: "start"}, true
	case companion.DialogueNodeProgress:
		stepKind, ok := agentDialogueStepKind(node.StepKind)
		if !ok {
			return companion.AgentDialogueFact{}, false
		}
		return companion.AgentDialogueFact{Kind: "progress", StepKind: stepKind}, true
	case companion.DialogueNodeFirstArrival:
		return companion.AgentDialogueFact{Kind: "first_arrival"}, true
	case companion.DialogueNodeIdle:
		return companion.AgentDialogueFact{Kind: "idle"}, true
	case companion.DialogueNodeTerminal:
		state, reason, ok := agentDialogueTerminalFact(node.State, node.Reason)
		if !ok {
			return companion.AgentDialogueFact{}, false
		}
		return companion.AgentDialogueFact{Kind: "terminal", State: state, Reason: reason}, true
	default:
		return companion.AgentDialogueFact{}, false
	}
}

func agentDialogueStepKind(kind companion.PlanStepKind) (string, bool) {
	switch kind {
	case companion.PlanStepGoTo:
		return "go_to", true
	case companion.PlanStepMine:
		return "mine", true
	case companion.PlanStepPlace:
		return "place", true
	default:
		return "", false
	}
}

func agentDialogueTerminalFact(state companion.TaskState, reason companion.TaskFailReason) (string, string, bool) {
	stateText := ""
	switch state {
	case companion.TaskCompleted:
		stateText = "completed"
	case companion.TaskFailed:
		stateText = "failed"
	case companion.TaskTimedOut:
		stateText = "timed_out"
	case companion.TaskStopped:
		stateText = "stopped"
	default:
		return "", "", false
	}
	reasonText := "none"
	if state == companion.TaskFailed {
		switch reason {
		case companion.TaskFailPlannerUnavailable:
			reasonText = "planner_unavailable"
		case companion.TaskFailInvalidPlan:
			reasonText = "invalid_plan"
		case companion.TaskFailPathUnreachable:
			reasonText = "path_unreachable"
		case companion.TaskFailWorldChanged:
			reasonText = "world_changed"
		case companion.TaskFailInventoryFull:
			reasonText = "inventory_full"
		default:
			return "", "", false
		}
	}
	return stateText, reasonText, true
}

// dialogueWorker 在 worker goroutine 上调用模型：只读不可变请求值，结果经
// 有界 channel 回 tick 边界。ctx 取消（仅关服序列调用 m.cancel）时放弃结果；
// 共享槽在模型调用返回后、结果发送前释放（时序论证见函数内注释）——HTTP
// 调用直接使用 m.ctx，beginShutdown 的 cancel 同时取消在途模型请求；tick
// 路径没有任何取消路径（见 requestDialogue 的契约注释）。
func (m *companionManager) dialogueWorker(
	id companion.ID,
	generation uint64,
	attempt uint64,
	taskStepIndex int,
	node companion.DialogueNode,
	issuer companionTaskIssuer,
	request companionDialogueRequest,
) {
	defer m.waitGroup.Done()
	result, err := m.dialogue.Dialogue(m.ctx, request)
	// 释放先于发送：`m.semaphore` 约束的是在途模型调用数，`m.dialogue.Do` 返回
	// 即调用结束、名额自此可复用，结果投递只是队列簿记。若先发送再经 defer
	// 释放，两者之间没有屏障，tick 线程 try-acquire 的成败便依赖 goroutine
	// 调度（ns 级残余窗口，M5E 递延 8 记录的成因）；前移后「任何观察者看到
	// 结果之前名额已归还」成为严格事实，ctx 取消路径行为不变（取消时同样
	// 先释放、发送走 `<-m.ctx.Done()` 分支放弃结果）。
	<-m.semaphore
	outcome := dialogueOutcome{
		id: id, generation: generation, attempt: attempt, taskStepIndex: taskStepIndex,
		node: node, issuer: issuer,
		memoryEpoch: request.MemoryEpoch, result: result, err: err,
	}
	select {
	case m.dialogueResults <- outcome:
	case <-m.ctx.Done():
	}
}

// applyDialogueOutcomes 在 tick 边界非阻塞排空台词结果并应用（对齐
// applyPlannerOutcomes 模式）：世代或任务状态不符的结果直接丢弃。
func (m *companionManager) applyDialogueOutcomes() {
	for {
		select {
		case outcome := <-m.dialogueResults:
			m.applyDialogueOutcome(outcome)
		default:
			return
		}
	}
}

// applyDialogueOutcome 应用单条台词结果：先清在途标记（无论结果新旧，该次
// 请求的槽位生命周期已结束），再做两级过时判定——世代不匹配（任务已被替换
// 或队首已提升）直接丢弃；开始/进展/首次到达节点的任务必须仍在 Running
// （任务已终态即过时，防止「我出发了」出现在任务结束之后）。终态节点在任务
// 离开当前槽位时触发，世代一致即同一任务纪元，无需再断言当前槽位状态（清槽
// 是终态的既有序列）。idle 节点执行专用重验：队列完全空、真实同一发令者、
// 身体激活且受众资格仍成立（见 switch 内 D7 分支）。失败结果只记 debug 级
// 结构化日志并跳过该台词。
func (m *companionManager) applyDialogueOutcome(outcome dialogueOutcome) {
	slot := m.slots[outcome.id]
	if slot == nil || !slot.dialogueInFlight || slot.dialogueAttempt != outcome.attempt {
		return
	}
	slot.dialogueInFlight = false
	if slot.queue.Generation() != outcome.generation {
		if outcome.err != nil {
			// 「过时 + 失败」组合的显式记录（D5 评审 Minor-3）：失败结果本身
			// 只跳过台词，叠加过时判定后连日志都不留会让「为什么既没有台词
			// 也没有错误」不可归因。仍然只记 debug 级，不改变任何行为。
			slog.Debug("过时的失败台词结果一并丢弃",
				"companion", outcome.id, "node", uint8(outcome.node.Kind), "error", outcome.err)
		}
		return
	}
	switch outcome.node.Kind {
	case companion.DialogueNodeStart, companion.DialogueNodeProgress, companion.DialogueNodeFirstArrival:
		current, ok := slot.queue.Current()
		if !ok || current.State != companion.TaskRunning || current.StepIndex != outcome.taskStepIndex ||
			!m.taskDialogueAudience(slot, outcome.issuer) {
			return
		}
	case companion.DialogueNodeTerminal:
		if !m.taskDialogueAudience(slot, outcome.issuer) {
			return
		}
	case companion.DialogueNodeIdle:
		// idle 结果的专用重验（D7）：请求在途期间世界事实可能已变化，只有
		// 队列仍完全空、发令者仍是发起请求的同一真实玩家（非恢复合成身份）、
		// 伙伴仍激活且玩家仍在线并在水平 16 格内时，结果才仍属于发起它的
		// 语境；任一不符静默丢弃——不广播、不改摘要、不补发。失败结果的
		// 去向与任务节点一致（switch 之后的 err 分支）。
		if _, hasCurrent := slot.queue.Current(); hasCurrent || slot.queue.Len() != 0 {
			return
		}
		if slot.currentIssuer.restored ||
			slot.currentIssuer.playerID != outcome.issuer.playerID ||
			slot.currentIssuer.name != outcome.issuer.name {
			return
		}
		body, active := m.body(outcome.id)
		if !active || !m.idleDialogueAudience(slot.currentIssuer, body) {
			return
		}
	default:
		return
	}
	if outcome.err != nil {
		// 失败只跳过台词：错误来自客户端的三类哨兵（传输层/请求构造/输出
		// 解码，请求与输出各有独立哨兵），客户端已保证错误文本
		// 不含密钥与响应正文原文。
		slog.Debug("台词请求失败，跳过该台词",
			"companion", outcome.id, "node", uint8(outcome.node.Kind), "error", outcome.err)
		return
	}
	if outcome.result.Generation != outcome.generation || outcome.result.MemoryEpoch != outcome.memoryEpoch {
		return
	}
	if outcome.node.Kind == companion.DialogueNodeTerminal {
		proposal := outcome.result.Proposal
		lifecycle, ok := m.companions.MemoryLifecycle(outcome.id)
		if proposal == nil || !ok || !lifecycle.Active || lifecycle.MemoryEpoch != outcome.memoryEpoch ||
			proposal.BaseRevision != lifecycle.MemoryRevision {
			return
		}
		slot.dialogueReservation = &companionDialogueReservation{
			operationID: proposal.OperationID, memoryEpoch: outcome.memoryEpoch,
			baseRevision: proposal.BaseRevision, summary: proposal.Summary,
			line: outcome.result.Line, issuer: outcome.issuer,
		}
		m.dispatchMemoryCommit(outcome.id)
		return
	}
	if outcome.result.Proposal != nil {
		return
	}
	m.applyDialogueEffect(outcome.id, outcome.issuer, outcome.result.Line)
}

func (m *companionManager) taskDialogueAudience(
	slot *companionTaskSlot,
	issuer companionTaskIssuer,
) bool {
	if issuer.restored || slot.currentIssuer.restored ||
		slot.currentIssuer.playerID != issuer.playerID || slot.currentIssuer.name != issuer.name {
		return false
	}
	if m.onlinePlayers == nil {
		return true
	}
	for _, player := range m.onlinePlayers() {
		if player.ID == issuer.playerID {
			return true
		}
	}
	return false
}

func (m *companionManager) memoryCommitWorker(
	id companion.ID,
	reservation companionDialogueReservation,
) {
	defer m.waitGroup.Done()
	result, err := m.dialogue.CommitMemory(m.ctx, companionMemoryCommitRequest{
		CompanionID: id, MemoryEpoch: reservation.memoryEpoch,
		BaseRevision: reservation.baseRevision, OperationID: reservation.operationID,
		Summary: reservation.summary,
	})
	outcome := memoryCommitOutcome{
		id: id, memoryEpoch: result.MemoryEpoch, operationID: result.OperationID,
		committedRevision: result.CommittedRevision, err: err,
	}
	select {
	case m.memoryCommitResults <- outcome:
	default:
		// 每伙伴最多一条 accepted reservation，结果通道容量覆盖全部伙伴；
		// 关服取消后仍须发布已经结束的 commit，供静止点 drain 决定是否更新
		// mirror。default 只防御不变量被破坏时阻塞 worker。
	}
}

func (m *companionManager) dispatchMemoryCommit(id companion.ID) {
	slot := m.slots[id]
	if slot == nil || slot.dialogueReservation == nil ||
		slot.dialogueReservation.commitInFlight || m.dialogue == nil {
		return
	}
	slot.dialogueReservation.commitInFlight = true
	reservation := *slot.dialogueReservation
	m.waitGroup.Add(1)
	go m.memoryCommitWorker(id, reservation)
}

func (m *companionManager) applyMemoryCommitOutcomes() {
	for {
		select {
		case outcome := <-m.memoryCommitResults:
			m.applyMemoryCommitOutcome(outcome)
		default:
			return
		}
	}
}

func (m *companionManager) applyMemoryCommitOutcome(outcome memoryCommitOutcome) {
	slot := m.slots[outcome.id]
	if slot == nil || slot.dialogueReservation == nil {
		return
	}
	slot.dialogueReservation.commitInFlight = false
	if outcome.err != nil {
		m.requestMemoryReconcile(outcome.id)
		return
	}
	reservation := slot.dialogueReservation
	if outcome.memoryEpoch != reservation.memoryEpoch ||
		outcome.operationID != reservation.operationID ||
		reservation.baseRevision == ^uint64(0) ||
		outcome.committedRevision != reservation.baseRevision+1 {
		return
	}
	operation, err := companion.ParseID(outcome.operationID)
	if err != nil {
		return
	}
	if err := m.companions.ReplaceActiveMemory(
		outcome.id, reservation.memoryEpoch, reservation.baseRevision,
		outcome.committedRevision, storage.CompanionIdentity(operation), reservation.summary,
	); err != nil {
		m.requestMemoryReconcile(outcome.id)
		return
	}
	m.applyDialogueEffect(outcome.id, reservation.issuer, reservation.line)
	slot.dialogueReservation = nil
}

func (m *companionManager) requestMemoryReconcile(id companion.ID) {
	m.memoryReconcileRequested = true
	m.memoryReconcileRequestID[id] = struct{}{}
}

func (m *companionManager) dispatchMemoryReconcile() {
	reconciler, ok := m.dialogue.(companionMemoryReconciler)
	if !ok {
		return
	}
	fence, current := reconciler.currentMemoryFence()
	if !current {
		for _, slot := range m.slots {
			slot.memoryReady = false
		}
		return
	}
	if fence != m.memoryReconcileTarget {
		m.memoryReconcileTarget = fence
		clear(m.memoryReconcilePending)
		for id, slot := range m.slots {
			m.memoryReconcilePending[id] = struct{}{}
			slot.memoryReady = false
		}
		m.memoryReconcileRetryWait = 0
		m.memoryReconcileAttempts = 0
		clear(m.memoryReconcileRequestID)
		m.memoryReconcileRequested = false
	}
	if m.memoryReconcileRequested {
		if len(m.memoryReconcileRequestID) == 0 {
			for id := range m.slots {
				m.memoryReconcilePending[id] = struct{}{}
			}
		} else {
			for id := range m.memoryReconcileRequestID {
				m.memoryReconcilePending[id] = struct{}{}
			}
		}
		m.memoryReconcileRequested = false
		clear(m.memoryReconcileRequestID)
	}
	if m.memoryReconcileInFlight {
		return
	}
	if len(m.memoryReconcilePending) == 0 {
		m.memoryReconcileFence = fence
		return
	}
	if m.memoryReconcileRetryWait != 0 {
		m.memoryReconcileRetryWait--
		return
	}
	m.memoryReconcileInFlight = true
	lifecycles := m.companions.MemoryLifecycles()
	pending := lifecycles[:0]
	for _, lifecycle := range lifecycles {
		if _, ok := m.memoryReconcilePending[lifecycle.ID]; ok {
			pending = append(pending, lifecycle)
		}
	}
	m.waitGroup.Add(1)
	go m.memoryReconcileWorker(reconciler, fence, pending)
}

func (m *companionManager) memoryReconcileWorker(
	reconciler companionMemoryReconciler,
	fence uint64,
	lifecycles []storage.StoredCompanionLifecycle,
) {
	defer m.waitGroup.Done()
	results := make([]memoryReconcileCompanionOutcome, 0, len(lifecycles))
	for _, lifecycle := range lifecycles {
		result, err := reconciler.ReconcileMemory(m.ctx, lifecycle)
		results = append(results, memoryReconcileCompanionOutcome{
			id: lifecycle.ID, lifecycle: result.Lifecycle, err: err,
		})
	}
	select {
	case m.memoryReconcileResults <- memoryReconcileOutcome{fence: fence, results: results}:
	case <-m.ctx.Done():
	}
}

func (m *companionManager) applyMemoryReconcileOutcomes() {
	for {
		select {
		case outcome := <-m.memoryReconcileResults:
			m.applyMemoryReconcileBatch(outcome)
		default:
			return
		}
	}
}

func (m *companionManager) applyMemoryReconcileBatch(outcome memoryReconcileOutcome) {
	m.memoryReconcileInFlight = false
	reconciler, ok := m.dialogue.(companionMemoryReconciler)
	if !ok {
		return
	}
	fence, current := reconciler.currentMemoryFence()
	if !current || fence != outcome.fence || m.memoryReconcileTarget != outcome.fence {
		m.memoryReconcileTarget = 0
		clear(m.memoryReconcilePending)
		clear(m.memoryReconcileRequestID)
		m.memoryReconcileRequested = true
		for _, slot := range m.slots {
			slot.memoryReady = false
		}
		return
	}
	m.applyMemoryReconcileOutcome(outcome)
}

// drainShutdownOutcomes 在 worker 静止点排空所有已发布结果。返回 true 表示
// 本轮消费过结果；应用终态 Dialogue 可能派生 memory commit，调用方必须再次
// 等待 worker 并重复排空，直到一整轮没有结果。
func (m *companionManager) drainShutdownOutcomes() bool {
	drained := false
	for {
		progressed := false
		select {
		case outcome := <-m.dialogueResults:
			m.applyDialogueOutcome(outcome)
			progressed = true
		default:
		}
		select {
		case outcome := <-m.memoryCommitResults:
			m.applyMemoryCommitOutcome(outcome)
			progressed = true
		default:
		}
		select {
		case outcome := <-m.memoryReconcileResults:
			m.applyMemoryReconcileBatch(outcome)
			progressed = true
		default:
		}
		if !progressed {
			return drained
		}
		drained = true
	}
}

func (m *companionManager) applyMemoryReconcileOutcome(outcome memoryReconcileOutcome) {
	hadError := false
	for _, result := range outcome.results {
		slot := m.slots[result.id]
		if result.err != nil {
			hadError = true
			if slot != nil {
				slot.memoryReady = false
			}
			continue
		}
		delete(m.memoryReconcilePending, result.id)
		remote := result.lifecycle
		local, ok := m.companions.MemoryLifecycle(result.id)
		if remote.ID != result.id {
			if slot != nil {
				slot.memoryReady = false
			}
			continue
		}
		if slot == nil || !ok || local.Active != remote.Active ||
			local.MemoryEpoch != remote.MemoryEpoch {
			if slot != nil {
				slot.memoryReady = false
			}
			continue
		}
		if !local.Active {
			slot.memoryReady = local.TombstoneOperationID == remote.TombstoneOperationID
			continue
		}
		if remote.MemoryRevision < local.MemoryRevision {
			slot.memoryReady = false
			continue
		}
		if remote.MemoryRevision == local.MemoryRevision {
			slot.memoryReady = remote.MemoryOperationID == local.MemoryOperationID &&
				remote.Summary == local.Summary
			if slot.memoryReady {
				m.resolveMemoryReservationAfterReconcile(remote)
			}
			continue
		}
		if err := m.companions.ReplaceActiveMemory(
			remote.ID, remote.MemoryEpoch, local.MemoryRevision,
			remote.MemoryRevision, remote.MemoryOperationID, remote.Summary,
		); err != nil {
			slot.memoryReady = false
			continue
		}
		slot.memoryReady = true
		m.resolveMemoryReservationAfterReconcile(remote)
	}
	if hadError {
		if m.memoryReconcileAttempts < 6 {
			m.memoryReconcileAttempts++
		}
		m.memoryReconcileRetryWait = uint8(1) << (m.memoryReconcileAttempts - 1)
		return
	}
	if len(m.memoryReconcilePending) == 0 {
		m.memoryReconcileFence = outcome.fence
		m.memoryReconcileAttempts = 0
		m.memoryReconcileRetryWait = 0
	}
}

func (m *companionManager) resolveMemoryReservationAfterReconcile(
	remote storage.StoredCompanionLifecycle,
) {
	slot := m.slots[remote.ID]
	if slot == nil || slot.dialogueReservation == nil {
		return
	}
	reservation := slot.dialogueReservation
	operation, err := companion.ParseID(reservation.operationID)
	if err != nil || reservation.memoryEpoch != remote.MemoryEpoch ||
		reservation.baseRevision == ^uint64(0) {
		return
	}
	if remote.MemoryRevision == reservation.baseRevision+1 &&
		remote.MemoryOperationID == storage.CompanionIdentity(operation) &&
		remote.Summary == reservation.summary {
		m.applyDialogueEffect(remote.ID, reservation.issuer, reservation.line)
		slot.dialogueReservation = nil
		return
	}
	if remote.MemoryRevision == reservation.baseRevision {
		m.dispatchMemoryCommit(remote.ID)
	}
}

// applyDialogueEffect 把一条有效台词结果落到可观察行为（D6 真身）：
//   - 构造 CompanionSpeech 广播事实（kind ChatEventCompanionSpeech、伙伴身份
//     与台词、reason None），经本 tick 的 takeEventFacts → taskEventDeliveries
//     管道广播给全部在线玩家，EventID 沿全服聊天计数器严格递增；
//   - 终态 memory 只由 commit/reconcile 成功路径整体替换 v5 mirror；普通台词
//     与本方法都不持有或改写裸摘要。
//
// 表达平面纪律：本方法绝不改变任务状态、FIFO、路径或任何世界事实；摘要只
// 作为后续 Dialogue 请求输入，绝不进入 Planner（dialogueEffects 计数是 D5
// 遗留的测试观察哨兵，保留供既有测试断言）。
func (m *companionManager) applyDialogueEffect(
	id companion.ID,
	issuer companionTaskIssuer,
	line string,
) {
	m.dialogueEffects++
	slot := m.slots[id]
	if slot == nil {
		// 结果在途期间槽位被移除只可能来自配置热变更——当前版本配置只在
		// 启动时加载，这里只是防御。
		return
	}
	m.events = append(m.events, taskEventFact{
		issuer:     issuer,
		definition: slot.definition,
		speech:     line,
	})
}

// buildDialogueEnvDigest 在 tick 边界构造一次台词请求的环境摘要：复用规划
// 观察的同一有界扫描（scanEnvObservation）与 BoundExposedBlocks 归一，保证
// Dialogue 与 Planner 看到的环境半部同构（DialogueEnvDigest 契约见
// dialogue_types.go）。返回切片是本次构造的独立副本，worker 在途期间只读。
func (m *companionManager) buildDialogueEnvDigest(body companion.Body) companion.DialogueEnvDigest {
	_, exposed, heights := m.scanEnvObservation(body)
	return companion.DialogueEnvDigest{
		ExposedBlocks: companion.BoundExposedBlocks(exposed),
		Heights:       heights,
	}
}
