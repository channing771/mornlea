---
doc_id: architecture-target
doc_revision: 2026-09-20.1
language: zh-CN
counterpart: architecture-target.md
status: target-not-current
---
# Mornlea 终局架构

本文档是后续新架构决策的设计依据，描述迁移完成后的目标运行时，而不是当前已经存在的实现。当前实现见 [`architecture.md`](architecture.md)；OpenSpec 中明确批准的迁移期例外不能改变这份目标。

## 1. 终局决策

终局实时产品由一个 Rust 领域与运行时核心、一个由内嵌 Python 驱动的 Godot 表现层，以及一个独立的 Python 伙伴 AI 服务组成。Rust 负责权威状态、协议、存档契约、客户端会话状态、预测和数值内核；Godot/Python 负责场景组合和表现行为。Python 是 Godot 的脚本语言，但不因此拥有权威游戏循环。

终局产品不使用生产 GDScript 作为 feature 语言。迁移期可以保留纯 GDScript Bootstrap，用于打开未准备好的项目并报告缺失的 native/Python 产物；它只是过渡机制，不得加入玩法功能。

## 2. 终局拓扑

```text
                         表现层与设备
  +---------------------------------------------------------------+
  | Godot + 已验证的内嵌 Python                                  |
  | 场景 / UI / 输入 / 音频 / 动画 / 资源生命周期                 |
  +-----------------------------+---------------------------------+
                                | typed semantic bridge
                                v
  +---------------------------------------------------------------+
  | Rust client-core                                             |
  | 协议会话 / mirror / prediction / reconciliation              |
  | 客户端状态 / mesh 准备 / presentation snapshot               |
  +-----------------------------+---------------------------------+
                                | versioned protocol
                                v
  +---------------------------------------------------------------+
  | Rust authoritative server                                    |
  | tick / world / entities / rules / validation / persistence   |
  +-----------------------------+---------------------------------+
                                | 共享确定性 crate
                                v
  +---------------------------------------------------------------+
  | Rust domain / numerical kernel                               |
  | 物理 / 碰撞 / 世界生成 / 流体 / 寻路                         |
  | 体素变换 / 光照 / meshing                                    |
  +---------------------------------------------------------------+

  独立 Python Agent
  Planner / Dialogue / Memory / tools
             -- versioned service contract 的候选意图 -->
             Rust server -- tick 边界重新验证 --> world state

  迁移期 Go
  legacy runtime / replay oracle / differential tests / 转换工具
  （绝不能成为第二个在线权威）
```

Rust client-core 与 Rust server 共享领域和协议 crate，但不共享可变的权威状态。Godot 只接收语义快照并提交语义输入，不接触 packet 字节、存档记录或裸数值缓冲区。

## 3. 语言与边界职责

| 领域 | 终局归属 | 边界规则 |
|---|---|---|
| 权威 tick 与玩法结果 | Rust server/domain | server 是世界、玩家、实体、库存和伙伴结果的唯一写者。 |
| 确定性数值计算 | Rust kernel | 物理、碰撞、raycast、世界生成、流体、寻路、mesh、light 只有一套生产实现。 |
| 网络协议与兼容性 | Rust protocol | packet schema、codec、framing、版本协商和 replay identity 一起维护。 |
| 持久化与迁移 | Rust storage/server | 存档 schema 与迁移由 server 负责，client host 不实现存档。 |
| 客户端会话与状态 | Rust client-core | 登录、收包、mirror、预测、reconciliation、预算和语义 frame 组装在这里。 |
| 场景与表现 feature | Godot + 内嵌 Python | Python 在明确预算内把语义值映射为场景、控件、音频、动画和资源。 |
| Godot native adapter | Rust GDExtension | 一个 typed bridge 负责生命周期、转换、缓冲区校验和 client-core 连接；Python 不加载裸 C symbol。 |
| 伙伴 AI | 独立 Python 服务 | 只返回有界候选和摘要；所有世界效果都由 Rust server 校验。 |
| 迁移与离线工具 | 迁移期 Go，最终转 Rust/工具链 | Go 可离线比较新旧运行时，但不是终局实时产品依赖。 |

两个 Python 环境必须隔离。Godot 内嵌 Python 是带有固定、可复现分发闭包的产品依赖；伙伴 Agent 的 Python 环境是独立进程、独立 lockfile、独立契约和独立故障策略。二者不能互相 import。

## 4. Python 的含义与边界

使用 Python 作为 Godot 脚本语言，是表现层决策，不是把游戏模拟放进解释器。Godot Python 可以负责 feature 装配、场景生命周期、UI 状态映射、输入适配、音频路由、动画触发和其他有界的表现编排；不得负责权威规则、协议 codec、客户端预测、存档、物理积分、逐 cell 数值循环或直接写 server。

如果未来需要设计师编写行为，该行为的数据和执行模型仍必须确定性、可由 server 校验。Python callback 不能隐式成为权威；如需可扩展玩法脚本，必须另行设计沙箱或数据驱动变更，不能通过往 Godot feature 里增加 Python 代码来实现。

内嵌 Python runtime 必须通过 editor、headless、export、隔离、生命周期和重复 teardown 资格验证。当前 Py4Godot derivative 是迁移路径上的证据，不是无条件的终局依赖；若不能满足要求，必须验证 successor 或停止 Godot 产品切换，不能偷偷引入系统 Python、运行时安装或 GDScript 玩法 fallback。

## 5. 运行时数据流

```text
Godot input
  -> Python semantic InputBatch
  -> Rust client-core
  -> Rust protocol/session
  -> Rust authoritative server tick
  -> 已验证的 world mutation 与 persistence observation
  -> Rust protocol snapshot
  -> Rust client-core mirror/reconciliation/presentation snapshot
  -> Python typed feature views
  -> Godot 场景、UI、音频和渲染资源
```

server 是唯一权威。客户端预测可回滚，并由确认快照替换或校正。AI proposal 与人类输入走同一条已验证 command 路径。本地 Memory 与远程 TCP 只是传输选择，共用 login、packet、validation 和 server-core 路径；本地模式不得变成特权模拟实现。

跨语言调用必须是粗粒度且有界的。frame、input batch、world publication 或 entity set 以拥有明确所有权的语义值或不可变快照跨边界；禁止逐 cell FFI、保留裸指针，以及在 Python、Go、Rust 之间反复搬运字节。

## 6. Rust crate 方向

最终 Rust workspace 应收敛为单向依赖的、可独立测试的 crate，方向类似：

```text
mornlea-godot -> mornlea-client-core -> mornlea-protocol
                                      -> mornlea-domain
                                      -> mornlea-kernel

mornlea-server -> mornlea-storage
               -> mornlea-protocol
               -> mornlea-domain
               -> mornlea-kernel
```

名称只是实现指导，不是 ABI 承诺。关键是 server 与 client-core 使用共享 Rust contract 和 kernel，而 Godot/Python 只消费 typed presentation surface；新 crate 或 bridge 不能为了保留过渡语言边界而复制规则实现。

## 7. 测试归属

| 测试关注点 | 终局位置 | 必须具备的证据 |
|---|---|---|
| 规则、tick 顺序、权威性、存档 | Rust server/domain/storage | unit、property、fuzz、故障注入和确定性 replay |
| 物理、世界生成、流体、mesh、light | Rust kernel | 数值 oracle、property、benchmark 和跨平台确定性检查 |
| 协议与存档兼容 | Rust protocol/storage | wire/save golden、版本迁移、畸形输入和 replay corpus |
| client mirror、prediction、reconciliation | Rust client-core | transcript parity、修正/replay、bounded queue 和 race 测试 |
| Godot 表现 | Godot/Python harness | typed bridge、场景生命周期、输入/UI/音频行为和语义视觉证据 |
| AI 服务 | 独立 Python 服务 | HTTP/MCP 契约、候选校验、取消和存储隔离测试 |
| 迁移 parity | 迁移期 Go | 与 Rust reference 的离线 differential replay；Go-only 行为不能成为终局契约 |

测试跟随终局所有者。某个测试现在位于 Go，只说明当前实现位于 Go，不是继续保留 Go 玩法所有权的理由。

## 8. 迁移顺序

1. 冻结语言无关的 protocol、save、event、input 和 replay identity；发布本目标，并把当前 Go/Python seam 标记为明确的迁移期例外。
2. 在 Rust 中抽取或重写 domain、protocol、storage contract 和数值 kernel。Go 只作为离线 oracle/adapter，不允许两个在线写者。
3. 构建 Rust authoritative server，用记录的 Go replay 校验；只有确定性 replay、存档迁移和故障路径 parity 通过后才切换 server authority。
4. 构建 Rust client-core 与 typed Godot bridge。现有 Go client-core 可以继续供 pilot 使用，但任何新 feature 不得扩展它；按有界能力逐项转移所有权。
5. Python 保持为 Godot 的终局 feature 语言，同时把 Go client-core 和 Python wire/data 工作替换为 Rust client-core semantic view。待正式分发可以通过 native launcher 诊断缺失 runtime 后，再移除纯 GDScript Bootstrap。
6. 将 terrain、actors、UI、audio、本地模式、工具链和 release packaging 拆成独立变更。每个 feature 都消费 Rust semantic contract，并在 parity 证明前可禁用。
7. 只有 Rust server、Rust client-core、Godot/Python presentation、本地/远程路径和 release rollback package 经过多个发布周期后，才切换默认入口。之后从实时产品路径退休 Go，直至 Rust/tooling 替代品完成前只保留批准的工具用途。

每个阶段都必须有可逆边界。回滚选择上一版本或 replay/oracle 路径，不让两个权威同时运行，也不静默改写存档。

## 9. 明确禁止的方向

- 不能因为当前 server 仍是 Go，就继续把新的权威玩法逻辑写入 Go。
- 不能因为 Godot 支持 Python，就把新的实时玩法、协议、预测或存档逻辑写入 Python。
- 不能让 Godot Physics 或场景状态成为权威。
- 不能让 Godot/Python 调用裸 protocol、engine、client-core 或 storage ABI。
- 不能让 Go 与 Rust 作为两个在线权威并行写状态，也不能使用 shadow writer。
- 不能逐 cell 跨 FFI 搬运数据。
- 不能把独立 Agent 服务作为库嵌入游戏客户端。
- 不能把 Godot pilot 成功误解为可以跳过 Rust server/client-core 收敛阶段。

当新任务与本文档冲突时，必须重设计任务以符合目标，或者在其 OpenSpec change 中明确记录经过批准且有时限的迁移期例外。

## 10. Rust foundation stages、后续 feature 与 No-Go rollback

P7 只批准远程 TCP pilot 为 Go。该决定不授权切换默认客户端，也不授权新的 Go 实时所有权。后续工作必须等待独立提出的 Rust foundation stages：

| Stage | Owner | Prerequisite | Exit condition | Rollback |
|---|---|---|---|---|
| [F1](../openspec/changes/archive/2026-09-21-rust-runtime-foundation/proposal.md) | Rust domain、protocol、storage contract 与 numerical kernel | P7 Go | 与 Go 的 replay/oracle 一致；没有第二个在线写者 | 保留 Go 生产路径 |
| [F2](../openspec/changes/rust-authoritative-server/proposal.md) | Rust authoritative server | [F1](../openspec/changes/archive/2026-09-21-rust-runtime-foundation/proposal.md) | 确定性 replay、存档迁移、故障路径 parity、共享 Memory/TCP 语义 | 保留 Go 权威；禁止 dual-write |
| [F3](../openspec/changes/rust-client-core/proposal.md) | Rust client-core 与 typed Godot bridge | F1；F2 protocol | Transcript parity、correction/replay、有界 bridge、重复生命周期 | 保留 pilot Go core 且不再扩展 feature |
| [P8](../openspec/changes/godot-production-terrain/proposal.md) | 生产地形呈现 | [F3](../openspec/changes/rust-client-core/proposal.md) | 独立 world feature 消费 Rust semantic family | 禁用 catalog 项；旧客户端保持默认 |
| [P9](../openspec/changes/godot-complete-actors/proposal.md) | 完整实体与效果 | [F3](../openspec/changes/rust-client-core/proposal.md) | 可独立禁用的 actor feature | 按 catalog 禁用 |
| [P10](../openspec/changes/godot-ui-migration/proposal.md) | UI 迁移 | [F3](../openspec/changes/rust-client-core/proposal.md) | Godot Control 加 embedded Python；无生产 GDScript 或新 WebView 所有权 | 保留旧 UI 客户端 |
| [P11](../openspec/changes/godot-desktop-audio/proposal.md) | 音频与桌面设备 | [F3](../openspec/changes/rust-client-core/proposal.md) | Semantic cue；无头路径不触设备 | 禁用 adapter |
| [P12](../openspec/changes/godot-production-tooling/proposal.md) | 工具链 | 按需 F1–F3 | 离线 replay 与呈现测试；无双在线权威 | 继续使用旧工具链 |
| [P13](../openspec/changes/godot-desktop-packaging/proposal.md) | 本地游玩与桌面发行 | F2–F3 | 本地/远程共用一条 Rust 路径；仅桌面 closure | 回到仅远程或旧客户端 |
| [P14](../openspec/changes/godot-default-client-switch/proposal.md) | 默认切换与退役 | F1–P13 完成 | 两个发行周期和可用 rollback package | 恢复上一发行；禁止部分删除 |

这些链接指向当前规划变更，不表示实现已经完成。F1 冻结并验证共享契约；F2 建立 Rust 权威；F3 消费已经验收的 F2 协议/会话契约，并在自身验收前通过 Rust 服务端集成验证。每个前置阶段都必须在 ledger 中提供实现 SHA、语料覆盖、非空且实际执行的测试、故障路径及回滚证据。OpenSpec 产物状态或文本搜索不能证明阶段完成。

P12 先提供 P8–P11 所需的采集、身份和覆盖率基础设施，再在各 feature 的证据就绪后逐项验收生产者交接，从而避免工具与 feature 互相等待。一次交接只变更明确列出的语义场景；其余场景继续使用原生产者和回归检查。P13 验证桌面导出包证据；P14 在切换默认入口或退役迁移组件前，需要两个完整发行周期和可用回滚包。Bootstrap 退役不删除稳定项目根，且必须先由合格的原生诊断入口替代迁移 Bootstrap。

当前 pilot 的 No-Go rollback 仍是加法：删除 `apps/mornlea-godot/`、两个 GDExtension、捆绑 Python runtime、Go client-core ABI 以及可选的 `scripts/godot` 入口后，应恢复 pilot 之前的生产行为。pilot 失败不得改写存档或默认配置。P7 为 Go 之后，后续 feature 必须可独立回退，且不得删除稳定项目根。Rust 迁移使用 offline replay，而不是 dual online writer，也绝不运行两个在线权威。
