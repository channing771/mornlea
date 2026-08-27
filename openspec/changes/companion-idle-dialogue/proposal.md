## Why

伙伴目前只在任务生命周期节点生成 Dialogue 台词，任务结束后会无限期静默。C-08 用既有 Dialogue 与 `CompanionSpeech` 管道增加低频、确定性且严格有界的空闲表达，让玩家完成任务后仍能感知伙伴人设，同时不引入新的任务自主权或持久状态。

## What Changes

- 完全空闲（无 current、无 pending）的伙伴在拥有真实最近任务发令者后，按每伙伴确定性的 1200–2400 tick 间隔获得一次空闲台词机会。
- 期限到达时，只有最近发令者仍在线且位于伙伴水平 16 格内才发起 Dialogue；inactive、离线、超距、模型槽满或已有请求在途均跳过本次并安排下一期限。
- Dialogue 增加非终态、零载荷的 `idle` 事实节点；模型仍只返回既有受限 `line`，结果仍以 `CompanionSpeech` 广播给全部在线玩家。
- idle 请求复用 Planner/Dialogue 全服 4 槽与每伙伴单在途纪律，但不占每任务 8 次预算、不排队、不补发、不重试、不抢占。
- idle 结果只在权威 tick 边界应用，并重验 queue generation、空队列、真实最近发令者、在线性和 16 格距离；过时或失败结果只丢弃。

## Non-Goals

- 不做伙伴自主任务、移动、多伙伴聊天、玩家聊天理解或长期人格演化。
- 不更新最近对话摘要，不持久化空闲期限、最近发令者或逐条台词。
- 不新增配置项、wire 消息、协议/schema/ABI/scenario 版本、客户端 UI、capture 或 golden。
- 不扩展 Planner 输入，不建设通用调度器、优先队列、请求取消或缓存。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `companion-dialogue`: 扩展 Dialogue 触发节点、低频机会、资格、并发跳过和结果过时契约，使完全空闲伙伴可安全地产生非任务台词。

## Impact

- 生产代码：`internal/companion` 的 Dialogue 节点与稳定请求序列化；`internal/server` 的 Companion Manager tick 编排、运行期期限和结果重验。
- 测试：`internal/companion` 节点/请求定点测试，`internal/server` 调度、并发、恢复身份、失败、广播及 Memory/TCP parity 测试。
- 兼容性：协议 v26、玩家 schema v7、区块 schema v9、metadata v2、`companions.ai` schema v4、engine ABI v7、client ABI v9、benchmark scenario v19 均不变，无迁移。
- 并发与性能：继续共享既有 4 个模型槽和有界结果 channel；每 tick 最多扫描 4 个伙伴，环境摘要只在低频且资格满足的期限到达时构造。
- 回退：删除 idle 节点、两项 slot 期限字段和一个 tick 调用即可；既有任务 Dialogue、存档与客户端无需迁移。
