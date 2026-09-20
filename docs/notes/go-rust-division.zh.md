---
doc_id: go-rust-ownership
doc_revision: 2026-09-19.1
language: zh-CN
counterpart: go-rust-division.md
---
# Mornlea 语言职责与迁移规矩

本文档是新任务的简明归属指南。终局架构由 [`docs/architecture-target.md`](../architecture-target.md) 定义；[`docs/architecture.md`](../architecture.md) 描述当前实现。下面列出的 Go 职责，除非终局文档明确分配给 Go，否则都属于迁移期例外。

## 1. 终局职责总表

| 领域 | 终局归属 | 说明 |
|---|---|---|
| 权威玩法规则与 tick | Rust server/domain | Rust 负责状态转换和唯一世界写入路径。 |
| 协议与网络会话 | Rust protocol/server/client-core | wire contract 与 replay identity 在同一个 Rust 边界维护。 |
| 存档与 schema 迁移 | Rust storage/server | client 和 Godot host 不写权威存档。 |
| 物理、碰撞、raycast、世界生成、流体 | Rust kernel | 只有一套确定性生产实现；不在 Go/Python 中保留 fallback。 |
| Mesh、light、体素与批量变换 | Rust kernel/client-core | 在 Rust 侧准备批量数据，以批次跨 bridge。 |
| 客户端 mirror、预测、校正 | Rust client-core | Godot 接收语义快照，而不是协议记录。 |
| 窗口、场景、UI、输入、音频、动画 | Godot + 内嵌 Python | Python 是终局 Godot feature 语言，但工作必须有界且只负责表现。 |
| 伙伴规划、对话、记忆、AI 工具 | 独立 Python 服务 | 候选通过版本化服务契约，并由 Rust server 重新校验。 |
| 迁移 oracle 与转换工具 | 迁移期 Go | Go 离线比较 transcript、辅助数据迁移，但不是终局实时依赖。 |

## 2. 当前到终局的迁移

| 领域 | 当前生产或 pilot | 终局方向 | 新任务规矩 |
|---|---|---|---|
| Server | Go 权威 server | Rust 权威 server | 不新增 Go 权威逻辑；改为 Rust contract 或有时限 adapter。 |
| Godot client core | Rust/Godot adapter 后面的 Go runtime | 与 typed Godot bridge 直接连接的 Rust client-core | Python feature 可消费语义 view，但不能继续增长 Go wire/data 逻辑。 |
| Godot 脚本 | 内嵌 Python pilot + 纯 GDScript Bootstrap | 内嵌 Python feature host | 不新增生产 GDScript；Bootstrap 仅为迁移期机制。 |
| 数值引擎 | Go 调用 Rust engine | Rust server 与 client-core 直接消费 Rust domain/kernel | 不在 Go、Python 或 Godot 中复制数值实现。 |
| 渲染器 | Rust `mornlea_client` | Godot 表现层 + Rust 准备/bridge | 在正式 producer handoff 前保留旧渲染器作为基线。 |
| AI | 独立 Python Agent | 独立 Python Agent | 不能嵌入 client，也不能让 Agent 写世界状态。 |

Godot 内嵌 Python 与独立 Agent Python 是两个不同产品。二者拥有独立进程或 runtime 闭包、依赖锁、生命周期所有者和契约；语法相同不意味着可以共享 import 或状态。

## 3. 判定规则

1. 代码是否决定权威世界结果、持久化结果，或定义 protocol/schema？写入 Rust server/domain/protocol/storage 边界。
2. 是否对大量 cell、vertex、sample、entity 或 byte 做确定性数值运算？写入 Rust kernel 或 client-core。
3. 是否维护 client mirror、prediction、correction、网络会话或有界语义快照？写入 Rust client-core。
4. 是否组合 Godot scene、control、输入动作、音频 cue、动画或资源生命周期？通过 typed bridge 写入 Godot 内嵌 Python。
5. 是否在实时循环之外做规划、对话、记忆摘要或 AI 工具？写入独立 Python Agent。
6. 是否只是比较新旧行为、转换数据或运行迁移门禁？迁移期允许使用 Go，但结果必须可 replay，且不能定义新的终局 contract。
7. 如果同时命中多条规则，把状态和计算放在拥有更大有界操作的一侧；不要为了保留旧包边界而制造高频 FFI 搬运。

## 4. 不可违反的边界

- 在线只有一个权威。禁止 Go/Rust 双写或实时 shadow authority。
- Godot/Python 只消费 typed semantic value；不解析 packet、不读取存档、不调用裸 engine/client ABI，也不提交未经校验的世界动作。
- Python callback 必须有界且非阻塞，不得做 tick 关键数值循环、网络等待、磁盘 I/O、运行时安装或无界 scene 扫描。
- 人类输入、本地输入和 AI proposal 都必须由 Rust server 校验。
- Local Memory 与远程 TCP 共用同一 login、packet、validation 和 server-core 路径。
- 跨语言 buffer 必须是 null 或真实有效的 owned buffer，批次必须 failure-atomic；禁止逐 cell 调用和保留外部指针。
- 当前 Go/Python pilot seam 必须在对应 OpenSpec change 中命名，并记录移除或替代条件。

## 5. 测试归属

Rust 负责权威规则、协议、存档、数值、replay、property、fuzz 和 client-core 测试。Godot/Python 负责 typed bridge、生命周期、输入、UI、音频、动画和语义视觉测试。独立 Agent 负责 HTTP/MCP、候选校验、取消和记忆隔离测试。Go 测试只有在比较记录行为或验证转换/工具时才属于迁移证据；仅有 Go 的玩法测试不能证明 Go 应当保持终局所有权。

跨语言边界迁移必须通过 OpenSpec change，包含 replay corpus、兼容性方案、failure-atomicity 测试和回滚路径。性能数据用于决策，但不能为复制生产实现或无界 bridge 提供理由。
