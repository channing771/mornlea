# Mornlea 开发流程（唯一说明）

> 本文件是全仓开发流程的**唯一说明文档**：`docs/feature-backlog.md`、工作者角色卡（`docs/agents/`）、GitHub Discussion #71 与各任务 brief 只引用本文件，不再内嵌流程。基线职责见 `AGENTS.md`（与 `CLAUDE.md` 逐字节相同）与 `openspec/config.yaml`；与代码冲突时以代码、测试与 `openspec/specs/` 主规格为真相。

## 角色

| 角色 | 职责 | 不得做 |
|---|---|---|
| 控制会话 | 派发、协调、裁决（Ruling）；不实现 | 绕过子代理直接实现、代评审 |
| 规划者 | 每日固定时间扩展/校对规划（见 `docs/agents/planner.md`） | 认领任务、修改功能代码、合并他人分支 |
| 实现者 | 认领规划行，先 `brainstorming` 确认内容再闭环开发（见 `docs/agents/implementer.md`） | 超出认领范围、跳过内容确认或评审 |
| 评审者 | SPEC 合规 / QUALITY 质量双裁决 | 自己实现与评审同一 Task |
| 集成者 | 批次合流的实现者特例：独占版本基线、golden、`AGENTS.md`/`CLAUDE.md` | 在功能分支改上述文件 |

## 阶段 0：认领

1. 读 `docs/feature-backlog.md` 与 `openspec/config.yaml`；选择一行**未认领**且依赖已满足。
2. 编辑该行：`状态` → `已认领`，`认领人` → `<agent 标识> @ <分支名>`，备注写明独占文件集。
3. 提交（docs-only，不关联 OpenSpec change）；同一时间只认领一行；已认领行转移须控制会话裁决。

## 阶段 1：隔离分支与 OpenSpec change

> 认领后先做「阶段 0.5」的内容确认，再进入本阶段建 change——确认结论决定 change 的 proposal/design。

- 从 `main`（或批次指定共享 SHA）创建 isolation worktree/分支。
- 复杂功能 / 新模块 / 跨包重构 / 存档 / 协议 / 性能契约 **必须**先建 OpenSpec change：`proposal.md` + delta specs + `design.md` + `tasks.md` + `ledger.md`，并 `openspec validate --all --strict --no-interactive`。
- 小型修复（拼写、格式、一次性实验）可直接修改（见规划表 F 组「直接修改豁免」），仍须相称验证。
- 先读对应 skill：`openspec-propose`、`openspec-apply-change`、`openspec-archive-change`、`openspec-sync-specs`（仓库内 `.claude/skills` 与 `.codex/skills`）。

## 阶段 0.5：内容确认（brainstorming 硬门禁）

认领后、**任何实现动作之前**（建 change、写代码、派发子代理都算实现动作），实现者**必须**以 `brainstorming` skill 与需求方（用户或控制会话）确认任务内容：

1. **先分类并说明路径**：`spike`（可行性问题）/ `bounded`（仓库既有流程改动，对话内短设计）/ `architectural`（新子系统或重构，须写设计文档）；拿不准走重的那条，中途发现复杂度升级立即停下并重分类（路径只升不降）。
2. **探索上下文**：读任务来源文档、相关代码/测试、既有主规格，把确认建立在对 repo 现状的核对之上。
3. **一次一个问题澄清**：目的、边界、成功标准、约束（版本号互斥、资源上限、版权红线）；一次只发一个问题。
4. **呈现设计并等待显式批准**：bounded 在对话里给短设计；architectural 按节呈现并写 `docs/superpowers/specs/` 设计文档。批准来源 = 用户或控制会话的显式确认，点头/明确同意即可。
5. **批准是硬门禁，不随任务规模缩小**：简单任务只是设计更短，不是免批准。自动调度无人在线时，按实现者角色卡的「无人在线确认协议」执行——结构化确认请求发到该行对应的 GitHub Discussion #71 评论、该行备注标「待确认」、**停在确认点**，不得静默开工；收到批准后把结论写进 OpenSpec change 的 proposal/design 与 implementer brief，未经确认的内容不得在实现期悄悄变卦。

## 阶段 2：subagent-driven-development 执行

1. **每 Task 派发全新 implementer 子代理**；任务 brief 是唯一需求来源，必须包含：当前 Task、共享契约 SHA、对应计划、change 产物（proposal/spec/design/tasks）、全局约束、精确验证命令。implementer 不得自我派生子代理或评审者。
2. **TDD：red → green → refactor**；先写失败测试再实现。
3. 测试组织纪律：测试与被测代码同目录；一个测试文件只装一个主题/一条被证性质；共享 helper 每包只设一个中心（`*_helpers_test.go`）；命名不叠加前缀后缀；已有混装文件先做零行为变化拆分。
4. 跨语言（Rust）改动同步：引擎 crate 内 `#[cfg(test)]` 主题子模块 + helper 中心；两侧手工同步的常量（如 registry 上限）必须在同一 Task 内改齐。
5. 实现发现规格不成立或范围漂移时，**先更新 OpenSpec 产物**再继续编码；绝不只改代码。

## 阶段 3：独立评审与修复循环

- 每 Task 完成后做**双评审**（全新 reviewer）：SPEC 合规（行为契约、可判定场景、门禁覆盖）+ QUALITY（设计取舍、注释、命名、并发/资源边界、测试锋利度）。
- 修复循环单任务**最多 5 轮**：R≤3 续用原 implementer，R≥4 换新 implementer（更强模型），超限逐条裁决并记录。
- 一切进度、评审结论与裁决写入该 change 的 `ledger.md`，格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。未决项必须全文誊入 `proposal.md` 的「延期与放弃」节。

## 阶段 4：整分支终审与门禁

```bash
make rust                       # 固定 Rust 1.97.1 构建
go test ./... -race             # 全量 race（迭代期可 make test-race-short）
go vet ./...
test -z "$(gofmt -l .)"          # 无输出
openspec validate --all --strict --no-interactive
```

- 渲染 / tick / 存储 / 协议热路径变化另加：对应 benchmark（**数值只记录，不改变退出状态**）、fuzz/golden 测试、`cmd/perfcheck`；capture 场景变化必须 `make visual` 逐图验收，**禁止放宽阈值**（阈值调整须有实测数据依据）。
- 平台专属或性能变更补充相应门禁；报告完整性、身份、真实 overflow、数据丢失和 I/O 错误是硬门禁。
- 可直接运行 `scripts/agents/gates.sh` 汇总执行上述标准门禁。

## 阶段 5：归档收尾（实现者自动执行）

1. 若拆分过测试文件，确认 `go test -list` 前后集合一致；
2. `openspec sync` 把 delta 沉淀到主规格 → 逐 change `openspec archive`；
3. 同步 `AGENTS.md` 与 `CLAUDE.md`（**逐字节相同**，`internal/archcheck` 的 `TestBaselineDocsAreIdentical` 兜底）与 `docs/notes/progress.md` 基线段——只写已集成且验证过的事实，不写本批次非目标；
4. 回填 `docs/feature-backlog.md`：该行 `状态` → `已完成`（认领人保留履历）；若批次集成任务有变化，同步更新对应行与 GitHub Discussion #71；
5. 合入 `main`（默认，需本地全绿）推送 `origin/main`；与在途分支冲突或用户要求走 PR 时，改为创建 PR 并暂停；
6. 关闭遗留：未决项誊入「延期与放弃」，不静默丢弃。

## 并行与冲突规则

- **版本号互斥**：协议 / 存档 schema / engine ABI / client ABI / benchmark scenario 的升版行互斥——同一时间只能一个认领者持有；冲突按实际合入顺序重排（例：第一夜批次设计写 client ABI v8，已被 egui 主菜单占用，须重排为 v9）。
- **同批并行先共享契约**：第一步在共同基线上冻结 append-only 共享契约提交（追加编号 / 协议消息 / 有限模型 tag），功能分支从该 SHA 创建；capture golden、benchmark scenario 与 `AGENTS.md`/`CLAUDE.md` 由**集成任务独占**，功能分支不得触碰。
- **文件所有权**：认领时声明独占文件集；与其它已认领行重叠则换行或延迟。
- **范围冻结**：认领后不得扩大范围；实现发现规格不成立时，先改 OpenSpec 产物再继续。

## 快速参考

| 阶段 | 动作 | 关键产物 |
|---|---|---|
| 0 认领 | 改 backlog 行 + docs-only 提交 | 状态/认领人 |
| 1 契约 | worktree + OpenSpec change + strict validate | proposal/spec/design/tasks/ledger |
| 0.5 确认 | `brainstorming` 分类→澄清→短设计→显式批准 | 设计结论（进 proposal/design 与 brief） |
| 2 实现 | 每 Task fresh implementer + TDD | 提交 + 测试 |
| 3 评审 | SPEC + QUALITY 双评审，≤5 轮 | ledger 结论与 Ruling |
| 4 门禁 | gates.sh / 全量验证 + 视觉/基准 | 通过证据（记录数值） |
| 5 收尾 | sync → archive → 基线同步 → 合入 → 回填 | 归档 change + backlog 标记 |
