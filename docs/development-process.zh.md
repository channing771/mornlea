---
doc_id: development-process
doc_revision: 2026-09-16.3
language: zh-CN
counterpart: development-process.md
---
# Mornlea 开发流程

本文件是当前唯一流程说明；`docs/feature-backlog.md`、`docs/agents/` 中的角色卡、GitHub Discussion #71 与任务 brief 都引用本文件。代码、测试和 `openspec/specs/` 是权威来源。

## 流程政策

OpenAI 原生编排采用隔离优先策略。经验证的 OpenAI ChatGPT/Codex 控制器拥有选择直接、委派或混合执行的授权。对于有边界的仓库探索、多文件推理、专项评审，或会让主上下文长期保留大量过程信息的任务，若主上下文保留成本高于交接成本，应优先使用全新代理；只有微小、与控制器当前编辑紧密耦合，或完成成本低于描述和集成成本的工作才留在控制器中。不得仅为并行速度或填满空闲槽位而委派，最多同时运行两个子代理。每个代理只接收简洁任务 brief 和全新或最小上下文。每次新委派前使用项目自有的 `adaptive-model-router` 发现实时主机能力；从任务难度、错误后果、上下文范围、工具调用跨度、验证强度、代理启动成本和用户偏好等多个角度评估，再选择可信够用的最低成本模型和 effort。代码质量与 token 效率同等重要：既不能让常规任务使用过高等级，也不能为了省 token 让高风险任务使用能力不足的等级。非 OpenAI 或未知提供方必须严格使用 `subagent-driven-development`，包括独立实现和评审。所有模式都必须保留范围、所有权、测试优先、验证和授权边界。

路由器可以使用原生子代理，也可以使用内置于 `adaptive-model-router/scripts/` 的外部 Z Code `GLM-5.3` 桥接。选择 Z Code 前先解析当前技能根目录并运行其中的 `scripts/zcode-agent.mjs probe`，通过 `run` 启动全新隔离会话，再用返回的 session ID 和 `send` 继续对话。该桥接不是 Codex 原生模型注册。编辑会话必须使用隔离 worktree 或独占文件，最终集成和验证仍由控制器负责。

每轮实现结束时，仅将稳定、可跨任务复用的架构约定提升到 `mornlea-architecture`；否则记录 `Architecture skill: no change`。同时复盘是否存在过度路由、能力不足、重试或升级、上下文传递成本、验证质量和 token 使用问题。只有经过验证、可复用的改进才同步更新两份项目 `adaptive-model-router`；否则记录 `Model router: no change`。

## 阶段

### 角色与认领纪律

控制会话负责协调和裁决；按提供方政策也可直接实现，但不得绕过必要评审。规划者维护规划材料，不认领任务或修改功能代码。评审者独立检查任务改动。只能认领一行 `ready`，改为 `claimed`，记录 `<agent> @ <branch>` 和独占文件集；转移认领须有控制会话裁决，`queued` 与 `design candidate` 不得认领。

0. 阅读 `docs/feature-backlog.md` 与 `openspec/config.yaml`，只认领 `ready` 行，记录所有权并保留无关脏改动。
1. 实现前将工作分类为 `spike`、`bounded` 或 `architectural`，核对代码/测试/历史和任务来源；一次只问一个澄清问题，说明范围、成功标准和约束，并先取得显式批准。确认通道设备优先（`confirm.sh ask` → 飞书回复 → `feishu-listener.js`/`AGENT_RESUME`）；不可用或超时时改用结构化 GitHub Discussion，并停在确认点。变化时先更新 OpenSpec 产物。
2. 较大工作使用隔离 worktree/分支；复杂功能、跨包重构、存档/协议、并发或性能契约变化必须有 `proposal.md`、delta specs、`design.md`、`tasks.md`、`ledger.md`，并运行 `openspec validate --all --strict --no-interactive`。
3. 遵循 `tasks.md` 和 red → green → refactor；测试与代码同目录，一个测试文件一个主题，每包一个共享 helper 中心，跨语言常量在同一任务同步。委派时只提供简洁 brief，包含任务、必要证据和路径、基线 SHA、相关 change 产物、约束、所有权、集成点和精确验证命令，不复制整个控制会话。OpenAI ChatGPT/Codex 控制器自行判断是否需要单独评审者，不强制“一轮实现、一轮评审”；非 OpenAI 或未知提供方仍采用新的实现者和独立评审。在 `ledger.md` 记录进度、实际执行的评审、证据和 `Ruling: <决定> — <理由> — <修正的错误>`；未决项写入 proposal.md 的“延期与放弃”。仅在工作区未变化时按 SHA 复用验证证据；聚焦评审检查改动行为，全量 race 留给门禁。
4. 运行下列命令，并按需增加 benchmark、fuzz/golden、视觉及平台门禁。自动 Hooks 已移除，只维护 `scripts/agent-hooks/guard.mjs` 及其测试。

```bash
make rust
make test-race
go vet ./packages/contracts/... ./packages/shared/... ./packages/server/... ./packages/client/... ./packages/tools/... ./packages/audit/...
test -z "$(gofmt -l .)"
openspec validate --all --strict --no-interactive
```
5. 拆分测试文件时确认 `go test -list` 集合；同步/归档 OpenSpec，依据已验证事实更新文档。行为变更走 PR/CI（`gh pr create`、`gh pr checks --watch`，修复后重复直到绿色，再 `gh pr merge --merge`）；纯同步/归档文档可在本地门禁全绿后直接合并。保留历史证据和未决项。

协议、存档 schema、engine/client ABI 和 benchmark scenario 升版互斥；版本化核心玩法串行，只有所有权和版本影响不重叠才可并行。
