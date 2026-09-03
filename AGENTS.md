# Mornlea 项目指南

## 指南作用域

Agent 指南沿目录祖先链叠加生效；离目标文件最近的 `AGENTS.md` 补充父级指南。局部规则不得降低本文件规定的全局安全、正确性与验证门禁。

## 项目与契约

Mornlea 是使用 Go 1.26 编写的独立体素游戏，Go module 为 `github.com/channing771/mornlea`，包含自研客户端、权威服务端、世界存储、物理、Rust `mornlea_engine` 数值引擎和 Rust `mornlea_client` wgpu 渲染客户端；项目不兼容官方 Minecraft 协议、存档或版权资源。当前基线已经包含协议 v32；玩家 schema v8、区块 schema v9、世界 metadata v3、独立 `companions.ai` schema v5、独立 `hostile_mobs` schema v1、engine ABI v10、client ABI v14，benchmark scenario 为 v22。
## 真相优先级

发生冲突时，按以下顺序核实现状：代码与测试 -> `openspec/specs/` -> `docs/architecture.md` -> `docs/notes/progress.md` -> `docs/superpowers/`。历史文档只提供背景，不覆盖已验证的当前行为。

## 仓库与局部指南

- Go 内部包、依赖方向和权威模拟：`internal/AGENTS.md`
- 共享域 Go 模块（server/client 双侧共用的领域包）：`packages/shared/`（局部指南随包目录，如 `packages/shared/network/AGENTS.md`）
- Rust engine、client 与 C ABI：`packages/engine/AGENTS.md`
- 图形客户端与其 app/capture/benchmark 子包：`cmd/mornlea/AGENTS.md`（子包目录各有局部指南，依赖方向由 `internal/archcheck` 强制）
- 文档结构、长期说明和测试组织文档：`docs/AGENTS.md`
- 脚本、发布与自动化：`scripts/AGENTS.md`
- Python 伙伴 Agent 服务：`packages/agent/companion/AGENTS.md`
- OpenSpec 项目上下文与产物规则：`openspec/config.yaml`

## 开始工作前

1. 阅读 `openspec/config.yaml`。
2. 若任务属于某个 OpenSpec change，依次阅读该 change 的 `proposal.md`、delta specs、`design.md` 和 `tasks.md`。
3. 检查 `git status`，保留用户已有及与任务无关的改动。
4. clean checkout 上涉及 Rust 的任务先运行 `make rust`，再执行直接的 focused Go 命令。

## 全局架构边界

- 服务端是世界与玩家状态的唯一权威；客户端只持有镜像、预测和呈现状态。
- 单机 Memory 与远程 TCP 必须复用同一套登录、模拟和校验路径。
- 任何 Go 包都不得导入 WebGPU 绑定；GPU 渲染由 Rust client 独占。
- Go 只能经仓库既定的 ABI bridge 调用 Rust，不得增加生产 fallback 或旁路。
- Go 服务端只通过 loopback Agent HTTP 与 MCP 合同连接独立 Python 服务，不得 shell-out、FFI 或嵌入 Python；Python 只编排 Planner、Dialogue 与 compact memory，不得提交世界动作。
- 跨 goroutine 发送成功后的消息及其切片视为不可变。
- 权威 tick、渲染与网络热路径不得执行无界工作或阻塞 CPU、磁盘和网络操作。
- 仓库不得加入 Mojang 版权材质或其他未经授权的二进制美术资源。

## 工程纪律

- 修改保持最小且聚焦，优先复用既有抽象，不顺手重构无关代码。
- 新增或修改行为时先写失败测试，再完成最小实现和重构。
- 代码注释、GoDoc 和 Rust doc comment 使用中文；Go/Rust 标识符、wire magic、外部 API 名称和技术术语保留英文。
- 注释中提及 Go 标识符时用反引号包裹，并解释意图、边界与取舍，而非机械复述代码。
- 禁止在任何代码注释、GoDoc 或 Rust doc comment 中出现任务标识（形如 `A-01`、`B-23` 等 `[A-F]-[0-9]{2}` 编号）；需关联任务时改为描述功能或契约名称，溯源以 `docs/feature-backlog.md`、Discussion #71 与 OpenSpec change 为准——该编号仅允许出现在规划层产物（`docs/feature-backlog.md`、`docs/notes/`、`docs/agents/` 对规则本身的举例、OpenSpec 产物、`scripts/` 等）中，不得出现在生产或测试代码的注释里。
- 保护用户已有和无关改动；不得擅自回退、覆盖或清理它们。
- Git 提交信息（所有 agent 会话一律遵守）只写**单行英文**，格式 `<type>(<scope>): <subject>`：`type` 取 `feat`/`fix`/`docs`/`refactor`/`perf`/`test`/`chore`，`scope` 可省略；`subject` 用英文祈使句、小写开头、不以句号结尾，可含任务编号；不写正文、页脚与 `Co-Authored-By`（合并提交等工具自动生成信息除外）。
- Pull request（所有 agent 会话一律遵守）：标题沿用提交信息的单行英文格式；正文用英文，直接采用 `.github/PULL_REQUEST_TEMPLATE.md` 模板——`## Summary` 以要点列表概述改动内容与契约/版本影响，`## Validation` 逐行列出实际执行的验证命令与门禁结果（可附 OpenSpec change 链接）；不写中文段落、长篇叙事与 `Generated with` 等签名页脚。
- 禁止破坏性 Git 操作、强制推送、跳过 Hook 或用豁免变量绕过失败，除非用户明确授权。

## 开发流程

复杂功能、新模块、跨包重构、存档、协议或性能契约变更必须走 OpenSpec，代码与计划不一致时先更新 change 产物。OpenSpec change 执行、多步骤修复与重构必须按 `subagent-driven-development` skill 执行：一轮开发一轮审查，进度和裁决写入 ledger；控制会话不得直接实现，评审定义以 skill 内任务评审为准。小型拼写修复、纯格式修改和一次性实验可直接修改，但仍须完成相称验证。详细流程见 `docs/development-process.md`、`docs/openspec.md` 和 `docs/test-organization.md`。

## 验证

按风险递增选择入口：编辑循环与任务闭环默认停在最低相称层级（T0/T1），`dev-check`、`test-race-short`（T2）与全量门禁（T3）只在阶段边界（推送前、提交前或复现 CI 失败）运行；同一基线 SHA 下已记入 change ledger 的验证输出可直接引用，不重跑。定点命令与测试分层见 `docs/notes/test-quickstart.md`：

```bash
make rust
make companion-agent-check
make companion-agent-integration
go test ./path/to/affected/package -race -count=1
go test ./internal/archcheck -count=1
make dev-check
make test-race-changed
make test-race
openspec validate --all --strict --no-interactive
```

benchmark 与 `perfcheck` 的性能数值只记录，不改变退出状态；报告完整性、真实 overflow、数据丢失和 I/O 错误仍必须硬失败。自动测试不得启动或聚焦前台游戏窗口，除非用户明确要求人工验收。

## Hook

原先挂在 `.codex/hooks.json` 与 `.claude/settings.json` 上的 `scripts/agent-hooks/guard.mjs` 钩子门禁已下线：两处 hook 配置均已移除，`guard.mjs` 实现与 `node --test scripts/agent-hooks/guard.test.mjs` 仍保留在仓库与 CI。钩子不再自动拦截后，代理仍须自觉遵守上文全部门禁，不得以钩子下线为由放宽验证。
