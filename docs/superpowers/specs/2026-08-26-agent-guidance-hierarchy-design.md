# Mornlea Agent 指南与文档分层设计

日期：2026-08-26

状态：设计已在对话中确认，等待书面复核

## 1. 背景

当前根 `AGENTS.md` 与 `CLAUDE.md` 逐字节相同，各自同时承担项目身份、完整能力编年史、架构边界、开发流程、测试组织、OpenSpec 纪律和 Hook 说明。两份文件虽然有一致性测试保护，但任何任务都会加载与自身无关的大量功能细节，且归档变更频繁争用同一段巨型文本。

参考项目 deer-flow 采用不同组织：根 `AGENTS.md` 只负责全仓方向，模块与高风险子系统通过局部 `AGENTS.md` 补充祖先规则，`CLAUDE.md` 仅导入同目录的 `AGENTS.md`。该项目并非给每个目录机械创建指南，而是按局部复杂度和风险放置。

Mornlea 已有 OpenSpec 主规格、进度编年史、专题说明和历史设计记录。本次改造复用这些现有权威来源，不再让根 Agent 指南兼任第二套功能规格。

## 2. 目标

1. 让 `AGENTS.md` 成为 Agent 规则的唯一内容源，`CLAUDE.md` 只承担 Claude Code 导入适配。
2. 把根指南缩减为所有任务都必须知道的方向层。
3. 在核心、高风险作用域放置局部指南，使规则随目录祖先链逐层补充。
4. 建立当前文档索引和当前架构入口，明确主规格、进度记录和历史资料的权威关系。
5. 修复本次审计确认的现行文档漂移，但不改写历史资料。
6. 只为薄导入迁移做一处必要的 archcheck 适配，不扩展为通用文档检查系统。

## 3. 非目标

- 不给每个 Go 包、Rust 模块或文档子目录机械生成 `AGENTS.md`。
- 不复制 deer-flow 的 Python 检查器、文档站或 24 文件规模。
- 不新增或修改游戏行为、协议、schema、ABI、benchmark scenario 或可执行程序。
- 不修改 Hook、gate、构建、发布或 CI 行为。
- 不批量修订 `docs/superpowers/`、归档 OpenSpec 和其他历史证据中的旧路径、旧命令或旧架构。
- 不增加指南字节预算、祖先链预算、路径 manifest 或通用 Markdown 链接检查。
- 不运行全仓 Go/Rust 测试、`make dev-check`、视觉验证或图形客户端。

## 4. 指令继承模型

### 4.1 唯一内容源

`AGENTS.md` 是 Agent 规则的唯一内容源。根和主要模块的 `CLAUDE.md` 使用 deer-flow 式薄导入：

```markdown
# CLAUDE.md

本作用域的 Agent 指南位于 [AGENTS.md](AGENTS.md)，供 Claude Code、Codex、OpenCode 及其他编码代理共享。Claude Code 在下方导入该指南。

@AGENTS.md
```

薄导入文件不重复版本号、能力描述或工作流规则。

设置薄导入的作用域为：

- `/CLAUDE.md`
- `/internal/CLAUDE.md`
- `/engine/CLAUDE.md`
- `/cmd/mornlea/CLAUDE.md`

深层局部指南不配套机械生成 `CLAUDE.md`。它们由父级指南的作用域路由表明确要求读取，与 deer-flow 的模块层加深层局部说明模式一致。

### 4.2 祖先链规则

修改文件时，根指南始终生效；路径祖先上的局部 `AGENTS.md` 依次补充根规则；离目标文件最近的指南提供最具体的约束。

局部指南不得取消以下根级要求：

- 服务端权威和单机/远程共路；
- 依赖与 ABI 边界；
- 数据不丢失、真实 overflow 和报告完整性门禁；
- 用户改动保护和非破坏性 Git 操作；
- OpenSpec 与 subagent-driven-development 入口；
- 中文注释、版权资产和自动测试不得打开前台窗口等全局要求。

发生表述冲突时，以更近指南对本作用域的具体说明为准，但更近指南不能降低上述全局安全与正确性要求。

## 5. 文件布局

```text
AGENTS.md
CLAUDE.md

internal/
├── AGENTS.md
├── CLAUDE.md
├── client/AGENTS.md
├── nativeabi/AGENTS.md
├── network/AGENTS.md
├── sim/AGENTS.md
└── storage/AGENTS.md

engine/
├── AGENTS.md
├── CLAUDE.md
└── crates/
    ├── mornlea_client/AGENTS.md
    └── mornlea_engine/AGENTS.md

cmd/mornlea/
├── AGENTS.md
└── CLAUDE.md

docs/
├── AGENTS.md
├── README.md
└── architecture.md

scripts/
└── AGENTS.md
```

`openspec/` 不新增局部指南，因为 `openspec/config.yaml` 已定义 artifact 和 operation 规则。`cmd/mornlea-server/`、`internal/render/`、`web/` 等目录首轮沿用最近祖先规则；只有后续出现持续的局部上下文压力时再单独拆分。

## 6. 根指南职责

根 `AGENTS.md` 只保留：

1. Mornlea 项目身份和不兼容官方 Minecraft 协议、存档、资源的边界。
2. 当前契约版本矩阵：协议、玩家/区块/世界/伙伴 schema、engine/client ABI 和 benchmark scenario。
3. 真相优先级：代码与测试、OpenSpec 主规格、当前架构文档、进度编年史、历史设计。
4. 仓库地图和局部指南路由表。
5. 开始工作前的最短清单：读 `openspec/config.yaml`、匹配 active change、检查 `git status`。
6. 服务端权威、Memory/TCP 共路、Go/Rust 所有权、依赖方向、消息不可变和热路径等全局架构红线。
7. OpenSpec、开发流程、测试组织和快速验证文档的入口链接，不复制完整流程。
8. 聚焦修改、测试先行、中文注释、资产版权、前台窗口和 Hook 等全局工程纪律。

当前“项目定位”中的逐里程碑功能编年史不搬到其他 Agent 指南。已有事实分别由 `openspec/specs/`、`docs/notes/progress.md` 和 `docs/architecture.md` 承接。

## 7. 局部指南职责

### 7.1 Go 内部包

`internal/AGENTS.md` 记录 Go 内部包共同规则：依赖方向以 archcheck 为准、包级数据所有权、同目录测试组织、Go 格式和 focused package 验证。它只给出包地图，不复制依赖白名单。

`internal/sim/AGENTS.md` 记录服务端权威、`Engine.Step` 阶段组合、状态变更原子性、有界 tick 工作、变更广播汇聚以及模拟热路径验证要求。

`internal/network/AGENTS.md` 记录协议输入校验、长度与数量上限、发送后消息不可变、Memory/TCP 行为一致、协议升级和 fuzz/golden 同步面。

`internal/storage/AGENTS.md` 记录 schema 版本、只读迁移、容量上限、原子写入、故障注入、重启和零数据丢失要求。

`internal/nativeabi/AGENTS.md` 记录其作为 engine C ABI 唯一 Go 入口的职责、无生产 fallback、unsafe/指针生命周期、固定输入上限和跨语言容量验证。

`internal/client/AGENTS.md` 记录客户端只读镜像、预测边界、输入与呈现职责、client ABI 桥接和资源生命周期要求。服务端玩法规则不得在客户端复制成第二套权威逻辑。

### 7.2 Rust workspace

`engine/AGENTS.md` 记录 Rust 1.97.1 固定工具链、workspace 公共 FFI 规则、中文 doc comment、panic/指针边界和 Rust 定点命令。

`engine/crates/mornlea_engine/AGENTS.md` 记录 mesh/light、collision、raycast、physics 和 worldgen 的唯一生产实现边界，以及 engine ABI 的同步面。

`engine/crates/mornlea_client/AGENTS.md` 记录窗口、事件、GPU 渲染、egui、音频平台适配和 client ABI 的所有权，以及 headless 路径不得依赖该 crate 的要求。

### 7.3 应用装配、文档和脚本

`cmd/mornlea/AGENTS.md` 记录图形客户端装配边界、普通交互启动、`-connect`、capture、benchmark、用户配置与音频设备隔离。场景数量、顺序和固定容量不复制进指南，改为引用代码和测试。

`docs/AGENTS.md` 记录当前文档、OpenSpec、进度编年史和历史资料的权威层级；禁止把 change 非目标或逐功能规格重新写入根指南。

`scripts/AGENTS.md` 记录 Hook、gate、发布与自动化脚本的失败语义。指南描述实际行为，不承诺脚本当前没有执行的门禁。

## 8. 文档信息架构

新增 `docs/README.md` 作为文档地图，不复述正文。它按受众和权威性路由：

| 位置 | 职责 |
|---|---|
| `README.md` / `README.en.md` | 用户入口、运行、配置和发布说明 |
| `docs/architecture.md` | 当前系统架构、所有权和组件边界 |
| `openspec/specs/` | 已归档的可观察行为主规格 |
| `openspec/changes/` | 当前 change 的 proposal/spec/design/tasks/ledger |
| `docs/development-process.md` | 当前开发与子代理评审流程 |
| `docs/test-organization.md` | 当前 Go/Rust 测试组织规则 |
| `docs/notes/test-quickstart.md` | 当前验证命令和测试分层 |
| `docs/notes/progress.md` | 交付编年史，不作为当前行为规格 |
| `docs/notes/` | 当前工程、运维和专题说明 |
| `docs/superpowers/` | 历史设计背景，不保证描述当前实现 |
| `openspec/changes/archive/` | 已完成 change 的历史证据 |

新增 `docs/architecture.md` 只描述稳定的当前架构：

- Go、`mornlea_engine` 和 `mornlea_client` 的职责；
- 权威服务端、客户端镜像与预测；
- Memory/TCP 共用模拟和校验路径；
- 两条 ABI 与 release-unit 边界；
- Go 内部包依赖方向；
- 无图形专用服务端；
- 行为规格、实现细节和历史设计的下钻入口。

它不列举全部玩法、协议字段、HUD quad、capture 场景或里程碑。

## 9. 现行文档漂移清偿

本次只修订当前维护的文档和配置说明：

1. `openspec/config.yaml`：把旧的协议 v19、玩家 schema v6、区块 schema v8、client ABI v4 和 benchmark scenario v16 摘要收敛为当前版本与当前 Go/Rust 所有权；归档操作不再要求同步扩写两份巨型基线文档。
2. `README.md` 与 `README.en.md`：engine ABI v6 改为 v7；“整体架构”入口从历史 `docs/superpowers/specs/2026-07-26-minecraft-go-design.md` 改到 `docs/architecture.md`，历史设计仍可从文档索引访问。
3. `docs/development-process.md` 与 `docs/agents/implementer.md`：不存在的 `make visual` 改为仓库真实存在且符合语境的 `make visual-check` 或 `make visual-update`。
4. `docs/agents/README.md` 与 `docs/agents/confirmation-channel.md`：旧绝对路径 `/Users/chen/chenwork/minecraft-go` 改为不绑定本机目录的仓库根定位方式。
5. `docs/notes/go-rust-division.md`：删除对已移除 `internal/gfx` 的当前式表述，改为当前 `internal/nativeabi` 与 `internal/client` 两条桥接边界。
6. 根指南、`docs/openspec.md` 和相关当前说明：准确写明 Codex 配置拥有 `Stop` Hook，而 Claude 配置目前只有 `PreToolUse` 与 `PostToolUse`；本次不修改 Hook 配置。
7. 涉及 `scripts/agents/gates.sh` 的当前说明：准确描述脚本实际运行 `make rust` 而非 `make rust-check`；本次不修改脚本。

历史设计、历史计划和归档 change 中的旧命令、旧路径与旧版本保持原样，因为它们记录的是当时状态。

## 10. 最小 archcheck 适配

`internal/archcheck/baseline_test.go` 与 `internal/archcheck/identity_test.go` 是仅有的两个允许修改的非文档文件。

### 10.1 版本门禁

`TestBaselineVersionsMatchCode` 只读取根 `AGENTS.md`。它继续从代码权威常量提取版本并与文档比对，不把具体版本号复制进测试。

### 10.2 薄导入门禁

删除“根 `AGENTS.md` 与 `CLAUDE.md` 逐字节相同”的假设，将 `TestBaselineDocsAreIdentical` 替换为 `TestClaudeImportsAgentGuidance`。新测试逐一读取以下文件：

- `CLAUDE.md`
- `internal/CLAUDE.md`
- `engine/CLAUDE.md`
- `cmd/mornlea/CLAUDE.md`

测试要求每个文件：

1. 非空；
2. 包含指向同目录 `AGENTS.md` 的 Markdown 链接；
3. 包含独立一行 `@AGENTS.md`；
4. 不包含版本矩阵或完整 Agent 规则。

实现保持在现有 `baseline_test.go` 内，不新增检查包、通用解析器或测试文件。

### 10.3 原生引擎身份门禁

`TestNativeEngineLibraryIdentity` 保留 crate、产物、Makefile、cgo、README、OpenSpec 配置和进度文档的全部身份检查，以及对 `libmornlea_mesh` 的拒绝。当前指南身份改为跟随根 `AGENTS.md`；根 `CLAUDE.md` 只负责导入同目录指南，不再承担原生引擎身份正文。

## 11. 验证边界

按用户确认，本变更不运行全仓测试。只执行薄导入迁移直接依赖的定点测试：

```bash
go test ./internal/archcheck \
  -run 'TestNativeEngineLibraryIdentity|TestBaselineVersionsMatchCode|TestClaudeImportsAgentGuidance' \
  -count=1
```

另执行非测试检查：

```bash
git diff --check
```

不运行 `make dev-check`、`go test ./...`、Rust 测试、benchmark、capture 或图形客户端。

## 12. 兼容性与风险

### Claude Code 导入

风险是 `CLAUDE.md` 从完整内容改成导入后，遗漏 `@AGENTS.md` 会让 Claude Code 失去规则。定点 archcheck 直接锁定该行和同目录链接。

### 局部指南发现

风险是深层局部指南没有对应 `CLAUDE.md`。父级指南必须列出下级路由并要求进入相应目录前读取最近指南；OpenCode/Codex 的 `AGENTS.md` 祖先链继续直接生效。若实际工具验证显示某平台不能遵循该路由，后续可在具体深层目录增加薄 shim，而不是预先批量生成。

### 文档再次重复

局部指南只保存稳定约束和同步面，不保存动态版本、功能清单和行为规格。功能演进写入 OpenSpec 与进度记录，避免重新形成多个“当前能力大全”。

### 当前工作区

当前本地 `main` 与 `origin/main` 已分叉。实施不得重置、rebase 或合并本地 `main`，也不得修改与本任务无关的已有工作区内容。

## 13. 实施原则

实施按以下顺序进行：

1. 先建立文档索引和当前架构文档；
2. 再缩短根指南并创建局部指南；
3. 将 `CLAUDE.md` 改为薄导入并适配唯一必要的 archcheck 测试；
4. 修订当前文档引用和已确认漂移；
5. 运行约定的定点验证。

每个局部指南应短于其父级，只写该作用域特有内容。发现新的历史漂移时，除非它会让本次新增入口直接失真，否则记录而不顺手扩大范围。
