// Package companion 承载 companion 存档域：companions.ai 聚合文件（MCAI
// 信封、schema v1..v5）的编解码、任务区/FIFO/memory 载荷校验与伙伴存档值类型。
//
// 本包是纯 codec 域：依赖 packages/shared/companion 领域模型与 core 值类型，哨兵经
// storagedef 取用；不感知根包编排（DiskStore/MemoryStore 的 companions.ai
// 文件原子替换与路径编排），CompanionStore 接口属根包存储契约家族，定义
// 留在根包 types.go。
package companion

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
)

// ErrCompanionsNotFound 表示世界尚无伙伴聚合存档。根包以 var 绑定同一错误值
// 再导出，保持既有 storage.ErrCompanionsNotFound 引用与 errors.Is 身份不变。
var ErrCompanionsNotFound = errors.New("storage: companions not found")

// Body 是伙伴领域身体值的存档边界别名。根 storage 门面借此转发迁移入口，
// 无需越过 codec 子包直接依赖领域包。
type Body = companion.Body

// 任务区与 FIFO 的持久化上界。全部在编码/解码边界强制：磁盘文件与内存
// 占用都不随世界规模无界增长（推导见 `companion_codec.go` 的
// `MaxFileLength`）。
const (
	// MaxCompanionTaskCommandBytes 是任务区与 FIFO 每条指令的持久化字节
	// 上界，与网络聊天指令及 TaskCommand 的上界一致。
	MaxCompanionTaskCommandBytes = companion.MaxPlanCommandBytes
	// MaxCompanionPlanSteps 是单条任务持久化计划步骤数的防御性二进制
	// 上界：设计上不设步骤数上限而以 64 KiB 模型响应为天然界限（最密
	// go_to JSON step ≥30 bytes，实际 ≤ ~2,200 步），这里固定 5,000 以
	// 封顶单记录磁盘占用。
	MaxCompanionPlanSteps = 5000
	// MaxCompanionFIFOEntries 是单伙伴 FIFO 的持久化条数上界，与运行期
	// TaskQueue 的容量一致。
	MaxCompanionFIFOEntries = companion.MaxTaskQueueDepth
	// MaxCompanionSummaryBytes 是单条记录最近对话摘要的持久化字节上界，
	// 与 Dialogue 请求输入/终态响应的摘要上界同源（companion.
	// MaxDialogueSummaryBytes）：同一常量保证「能被写进请求的摘要」与
	// 「能被存档保留的摘要」两条边界不可能漂移。
	MaxCompanionSummaryBytes = companion.MaxDialogueSummaryBytes
)

// Identity 是 namespace、memory operation 与 tombstone 共用的二进制 UUIDv4
// 值。零值只允许表示 active canonical-zero memory 没有 operation；其余身份
// 必须通过 `Valid` 校验 version 与 RFC variant 位。
type Identity [16]byte

// Valid 报告 identity 是否是非零 canonical UUIDv4 二进制值。
func (identity Identity) Valid() bool {
	return identity != (Identity{}) && identity[6]&0xf0 == 0x40 && identity[8]&0xc0 == 0x80
}

// StoredCompanionLifecycle 是 v5 中与身体记录同 ID 关联的 lifecycle/memory
// 元数据。active 保存完整恢复 mirror，inactive 只保存 tombstone；两种形态
// 由严格 codec 校验为互斥集合。
type StoredCompanionLifecycle struct {
	ID                   companion.ID
	Active               bool
	MemoryEpoch          uint64
	MemoryRevision       uint64
	MemoryOperationID    Identity
	Summary              string
	TombstoneOperationID Identity
}

// StoredCompanions 是从聚合存档恢复的伙伴身体、lifecycle 与任务域载荷。
// `SourceSchema` 由 Decode 精确填入，供 bootstrap 区分 v5 no-op 与必须同步
// 重写的 legacy；纯新世界用零值表示尚无聚合存档。
type StoredCompanions struct {
	SourceSchema     uint32
	Revision         uint64
	AgentNamespaceID Identity
	Records          []companion.Body
	Lifecycles       []StoredCompanionLifecycle
	Queues           []StoredCompanionQueue
}

// CompanionSave 是一次 v5 伙伴身体、lifecycle 与任务域聚合保存请求。
// Lifecycles 必须与 Records 是同一 ID 集合；Queues 只允许关联 active 记录。
// 编码只读取载荷，绝不修改调用方切片。
type CompanionSave struct {
	Revision         uint64
	AgentNamespaceID Identity
	Records          []companion.Body
	Lifecycles       []StoredCompanionLifecycle
	Queues           []StoredCompanionQueue
}

// StoredCompanionTask 是存档中一条当前任务的持久化载荷。任务计划自带的
// 模型 summary 与 Generation 刻意不落盘：计划摘要是模型对计划的自由文本
// 描述，不属于任务事实（M5C/M5D 的任务事件不携带模型自由文本），世代只
// 用于丢弃过时 worker 结果，重启后没有在途请求可丢弃。注意与记录级的最
// 近对话摘要（StoredCompanionQueue.Summary，v4 起持久化）区分：后者是
// Dialogue 表达平面的近期记忆，不是任务事实。
type StoredCompanionTask struct {
	// Command 是玩家原始指令（不含 @伙伴名 前缀），≤MaxCompanionTaskCommandBytes。
	Command string
	// PlanSteps 是计划步骤（交付全集四 kind，编码按 kind 变长）；只有
	// Running 任务携带（模型计划只在 Validating 成功后落盘），
	// ≤MaxCompanionPlanSteps。
	PlanSteps []companion.PlanStep
	// StepIndex 是下一个待执行步骤的索引；仅 Running 任务可非零。
	StepIndex int
	// State 是保存时刻的六态任务状态。
	State companion.TaskState
	// StartTick 与 DeadlineTicks 使用持久化 WorldTimeTicks：关服期间世界
	// 时间不推进，恢复后不消耗执行时长；仅 Running 任务可非零。
	StartTick     uint64
	DeadlineTicks uint64
	// FailReason 仅与 TaskFailed 状态成对出现，其余状态必须为 TaskFailNone。
	FailReason companion.TaskFailReason
}

// StoredCompanionQueue 是一个伙伴任务域的持久化载荷：当前任务（若有）、
// 按接收顺序排列的 FIFO 指令，以及仅供 v4 Decode 迁移使用的裸摘要。v5
// Encode 要求 Summary 为空，恢复 mirror 只来自对应 lifecycle，避免旧 direct
// Dialogue 裸字符串伪造 revision/operation。
type StoredCompanionQueue struct {
	ID         companion.ID
	HasCurrent bool
	Current    StoredCompanionTask
	Pending    []string
	// Summary 只承接 schema v4 的 legacy 摘要。MergeV5 将其迁入 lifecycle
	// mirror 后清空；任何 v5 保存请求携带非空值都会被拒绝。
	Summary string
}

// validateStoredCompanionTask 校验单条任务载荷的全部不变量：状态与失败
// 原因的枚举与配对、指令文本边界、计划步骤按 schema 的结构合法性，以及
// "计划只在 Running 落盘"的字段耦合（非 Running 必须无步骤、无进度、
// 无计时）。编码与解码共用本函数，保证双向边界一致；schema 只影响步骤
// 集合（v2 只认 go_to、v3 认交付全集四 kind），其余不变量跨 schema 相同。
func validateStoredCompanionTask(task StoredCompanionTask, schema uint32) error {
	if task.State < companion.TaskQueued || task.State > companion.TaskStopped {
		return fmt.Errorf("%w: companion task state %d outside enum", storagedef.ErrCorrupt, task.State)
	}
	if task.State == companion.TaskFailed {
		if task.FailReason <= companion.TaskFailNone ||
			task.FailReason > companion.TaskFailInventoryFull {
			return fmt.Errorf("%w: companion task fail reason %d invalid", storagedef.ErrCorrupt, task.FailReason)
		}
	} else if task.FailReason != companion.TaskFailNone {
		return fmt.Errorf("%w: companion task fail reason %d without failed state", storagedef.ErrCorrupt, task.FailReason)
	}
	if err := companion.TaskCommand(task.Command).Validate(); err != nil {
		return fmt.Errorf("%w: companion task command: %v", storagedef.ErrCorrupt, err)
	}
	if task.State == companion.TaskRunning {
		// 步骤约束与 companion 侧 validPlanSteps 的结构校验保持一致
		//（summary 不落盘，故不能复用整份计划校验；place 方块的注册表
		// 值域校验依赖 companion 的私有注册表，由恢复路径 RestoreCurrent
		// → validPlanSteps 兜底，存档边界把 Block 当作有界不透明载荷）。
		if len(task.PlanSteps) == 0 {
			return fmt.Errorf("%w: running companion task has no plan steps", storagedef.ErrCorrupt)
		}
		if task.StepIndex < 0 || task.StepIndex >= len(task.PlanSteps) {
			return fmt.Errorf(
				"%w: companion task step index %d outside plan", storagedef.ErrCorrupt, task.StepIndex,
			)
		}
	} else if len(task.PlanSteps) != 0 || task.StepIndex != 0 ||
		task.StartTick != 0 || task.DeadlineTicks != 0 {
		return fmt.Errorf(
			"%w: companion task keeps plan progress outside running state", storagedef.ErrCorrupt,
		)
	}
	if len(task.PlanSteps) > MaxCompanionPlanSteps {
		return fmt.Errorf(
			"%w: companion task plan steps %d exceeds limit", storagedef.ErrCorrupt, len(task.PlanSteps),
		)
	}
	hasFollow := false
	for index, step := range task.PlanSteps {
		if err := validateStoredPlanStep(step, index, len(task.PlanSteps), schema); err != nil {
			return err
		}
		if step.Kind == companion.PlanStepFollow {
			hasFollow = true
		}
	}
	// 持续跟随不保存 deadline：DeadlineTicks 零值即运行期超时豁免（Task.
	// Expired 跳过零值），非零 deadline 的 follow 任务若被放行，恢复后将
	// 错误地重新挂上超时。v2 载荷不含 follow 步骤，本校验天然不影响 v2
	// 迁移；编码与解码共用同一道门。
	if hasFollow && task.DeadlineTicks != 0 {
		return fmt.Errorf(
			"%w: companion follow task keeps deadline %d", storagedef.ErrCorrupt, task.DeadlineTicks,
		)
	}
	return nil
}

// validateStoredPlanStep 校验单个计划步骤的结构约束。v2 只写过 go_to：
// 任何其他 kind 都是 v2 时代不可能出现的字节，按损坏拒绝（迁移读入后按
// v3 重写）。v3 按交付全集四 kind 校验：坐标步骤的 Y 必须在世界竖直边界
// 内、follow 的目标必须是有效 UUIDv4 且只能居末（follow 没有自然终点，
// 排在其后的步骤无从执行——与 companion.validPlanSteps 的结构约束一致，
// 存档边界提前拒绝，恢复路径无需再丢弃）。各 kind 未使用字段必须为零：
// 变长编码只写 kind 专属字段，非零的未用字段会在编码时静默丢失，零值
// 约束保证 round-trip 精确无损。
func validateStoredPlanStep(step companion.PlanStep, index, total int, schema uint32) error {
	if schema == companionSchemaV2 {
		if step.Kind != companion.PlanStepGoTo {
			return fmt.Errorf(
				"%w: companion task plan step %d kind %d is not go_to", storagedef.ErrCorrupt, index, step.Kind,
			)
		}
		if step.Y < core.MinY || step.Y >= core.MaxY {
			return fmt.Errorf(
				"%w: companion task plan step %d Y=%d outside world", storagedef.ErrCorrupt, index, step.Y,
			)
		}
		return nil
	}
	switch step.Kind {
	case companion.PlanStepGoTo, companion.PlanStepMine:
		if step.Block != 0 || step.PlayerID != (core.PlayerID{}) {
			return fmt.Errorf(
				"%w: companion task plan step %d keeps unused payload", storagedef.ErrCorrupt, index,
			)
		}
	case companion.PlanStepPlace:
		if step.PlayerID != (core.PlayerID{}) {
			return fmt.Errorf(
				"%w: companion task plan step %d keeps unused player payload", storagedef.ErrCorrupt, index,
			)
		}
	case companion.PlanStepFollow:
		if step.X != 0 || step.Y != 0 || step.Z != 0 || step.Block != 0 {
			return fmt.Errorf(
				"%w: companion task plan step %d keeps unused coordinate payload", storagedef.ErrCorrupt, index,
			)
		}
		if !step.PlayerID.Valid() {
			return fmt.Errorf(
				"%w: companion task plan step %d follow target invalid", storagedef.ErrCorrupt, index,
			)
		}
		if index != total-1 {
			return fmt.Errorf(
				"%w: companion task plan step %d follow is not last", storagedef.ErrCorrupt, index,
			)
		}
	default:
		return fmt.Errorf(
			"%w: companion task plan step %d kind %d is not delivered", storagedef.ErrCorrupt, index, step.Kind,
		)
	}
	if step.Kind != companion.PlanStepFollow && (step.Y < core.MinY || step.Y >= core.MaxY) {
		return fmt.Errorf(
			"%w: companion task plan step %d Y=%d outside world", storagedef.ErrCorrupt, index, step.Y,
		)
	}
	return nil
}

// validateStoredCompanionQueues 校验一组任务载荷的结构不变量：非空、ID
// 唯一、每条关联一条既有记录、当前任务与 FIFO 全部有界。records 是已按
// ID 升序排好的保存记录；本函数只读，不修改任何输入切片。schema 决定
// 步骤集合的校验口径——编码端恒为当前 schema（v4）。
func validateStoredCompanionQueues(
	queues []StoredCompanionQueue,
	records []companion.Body,
	schema uint32,
) error {
	known := make(map[companion.ID]struct{}, len(records))
	for _, body := range records {
		known[body.ID] = struct{}{}
	}
	seen := make(map[companion.ID]struct{}, len(queues))
	for index, queue := range queues {
		if err := validateStoredCompanionQueue(queue, schema); err != nil {
			return fmt.Errorf("companion queue %d: %w", index, err)
		}
		if _, duplicate := seen[queue.ID]; duplicate {
			return fmt.Errorf("%w: duplicate companion queue ID", storagedef.ErrCorrupt)
		}
		if _, exists := known[queue.ID]; !exists {
			return fmt.Errorf("%w: companion queue without body record", storagedef.ErrCorrupt)
		}
		seen[queue.ID] = struct{}{}
	}
	return nil
}

// validateStoredCompanionQueue 校验单条队列载荷：非空、ID 有效、当前任务
// 与 FIFO 每条指令的字节上界、最近对话摘要的文本边界。HasCurrent 为假时
// Current 不参与编码（任务区只随 flags bit0 落盘），因此必须整体为零值——
// 非零的 Current 无法在磁盘上表达，放行它会静默丢数据，一律拒绝。摘要按
// v4 语义校验（v3/v2/v1 迁移读入的载荷摘要恒为空，天然通过）。
func validateStoredCompanionQueue(queue StoredCompanionQueue, schema uint32) error {
	if !queue.HasCurrent && len(queue.Pending) == 0 && queue.Summary == "" {
		return fmt.Errorf("%w: empty companion queue", storagedef.ErrCorrupt)
	}
	if !queue.ID.Valid() {
		return fmt.Errorf("%w: invalid companion queue ID", storagedef.ErrCorrupt)
	}
	if queue.HasCurrent {
		if err := validateStoredCompanionTask(queue.Current, schema); err != nil {
			return err
		}
	} else if !storedCompanionTaskIsZero(queue.Current) {
		return fmt.Errorf("%w: companion queue keeps current task without HasCurrent", storagedef.ErrCorrupt)
	}
	if len(queue.Pending) > MaxCompanionFIFOEntries {
		return fmt.Errorf(
			"%w: companion FIFO depth %d exceeds limit", storagedef.ErrCorrupt, len(queue.Pending),
		)
	}
	for index, command := range queue.Pending {
		if err := companion.TaskCommand(command).Validate(); err != nil {
			return fmt.Errorf("companion FIFO entry %d: %w: %v", index, storagedef.ErrCorrupt, err)
		}
	}
	if err := validateStoredCompanionSummary(queue.Summary); err != nil {
		return err
	}
	return nil
}

// validateStoredCompanionSummary 校验最近对话摘要的持久化边界：不超过
// MaxCompanionSummaryBytes 字节、有效 UTF-8、不含 NUL。与 companion 侧
// validateDialogueSummary 同一规则（字节上界、编码与无 NUL），但存档边界
// 不设非空约束——空串等价于「无摘要」，是摘要被清空后的合法持久状态。
// 编码与解码共用本函数，保证双向边界一致。
func validateStoredCompanionSummary(summary string) error {
	if len(summary) > MaxCompanionSummaryBytes {
		return fmt.Errorf(
			"%w: companion summary %d bytes exceeds limit", storagedef.ErrCorrupt, len(summary),
		)
	}
	if !utf8.ValidString(summary) {
		return fmt.Errorf("%w: companion summary is not valid UTF-8", storagedef.ErrCorrupt)
	}
	if strings.ContainsRune(summary, 0) {
		return fmt.Errorf("%w: companion summary contains NUL", storagedef.ErrCorrupt)
	}
	return nil
}

// storedCompanionTaskIsZero 报告任务载荷是否为整体零值。载荷含切片字段
// 不可用 == 比较，这里逐字段判断；HasCurrent 为假的队列要求 Current 为
// 零值（磁盘形态无法表达它，非零即调用方缺陷）。
func storedCompanionTaskIsZero(task StoredCompanionTask) bool {
	return task.Command == "" &&
		task.PlanSteps == nil &&
		task.StepIndex == 0 &&
		task.State == 0 &&
		task.StartTick == 0 &&
		task.DeadlineTicks == 0 &&
		task.FailReason == companion.TaskFailNone
}
