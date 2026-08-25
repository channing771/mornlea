# 实现者（Implementer）工作者卡

> 用途：从规划表认领任务并按 `docs/development-process.md` 闭环开发，开发完成后**自动收尾**（门禁 → 归档 → 基线同步 → 回填标记 → 合入）。每个任务应派发一个全新实现者会话；本卡同时供子代理实现者与集成者使用。

## 触发

- 手动：`make agent-implementer` 或 `scripts/agents/run-agent.sh implementer`（默认 `AGENT_MODE=pr`：创建 PR → 监听 CI 到全绿 → merge；`AGENT_MODE=merge` 才直接合并）。
- 自动：控制会话/规划者点名（brief 中附任务行 ID 与来源）。
- **接力循环**：`AGENT_LOOP=1` 启动后，每个实现者完成（自动收尾后）即 `relay.sh` 接力启动下一个实现者认领下一行，直到规划表无「未认领」任务自动终结。
- **故障恢复**：会话以退出码 1 结束且日志含 `You've hit your session limit`（claude 账号用量上限，消息里给出 reset 时间）或其它异常中断时：先查 worktree/分支与 `~/.claude/projects/...` 会话 jsonl 判断**当前行进度**，等 reset 后用 `AGENT_RESUME=<该行最近确认请求ID> AGENT_LOOP=1 scripts/agents/run-agent.sh implementer` 续跑**同一行**（不重新认领；未完成的 worktree 继续用）。

## 第 1 步：认领

0. **AGENT_LOOP=1（接力模式）**：守卫登记与防重入已由 `run-agent.sh` 统一完成——启动时以**真实会话 pid** 写入本链守卫（`WORKER_ID` 未设置 → `~/.mornlea/loop.guard`；设置 → `~/.mornlea/loop.guard.$WORKER_ID`），本链守卫已有**存活** pid 则直接退出。**不要再手动 `echo $$`**（旧写法写入的是临时 bash 工具 shell 的 pid，命令一返回进程即死，"存活 pid 检查"永不命中，防重入形同虚设）。不同 WORKER_ID 的链互不排斥，可并行认领不同行。
1. 读 `docs/feature-backlog.md`：选一行 `未认领` 且依赖行已满足；同一时间只认领一行。
2. 编辑该行：`状态` → `已认领`，`认领人` → `<agent 标识> @ <分支名>`，备注声明独占文件集；docs-only 提交。
3. 在 Discussion #71 追加一条【状态变更】评论（→ 已认领，模板见文末；「每条状态变化一条」含认领时点，不是收尾才补），并用 `python3 scripts/agents/refresh-discussion.py --update` 即时刷新正文列表。
4. 从 `main`（或批次共享 SHA）创建 isolation worktree/分支；确认工作区干净。
5. 读任务来源文档（该行「来源」列指向的 design「遗留与简化清单」条目或设计文档章节），把来源细节带进 brief。

## 第 2 步：加载 skill 上下文

按任务类型读取对应 skill（先 `using-superpowers` 再按需）：

- **内容确认（必做）**：`brainstorming`——任何实现动作前先跑它（见第 3 步）
- 规划类/契约类：`openspec-propose`、`openspec-apply-change`、`openspec-update-change`
- 执行类：`subagent-driven-development`、`writing-plans`（复杂任务先写计划）
- 分支类：`using-git-worktrees`、`finishing-a-development-branch`
- 质量类：`test-driven-development`、`verification-before-completion`、`requesting-code-review`

## 第 3 步：内容确认（brainstorming 硬门禁）

认领后、**任何实现动作之前**（建 change、写代码、派发子代理都算实现动作），必须以 `brainstorming` skill 对任务内容做确认：

1. **先分类**并说明路径：`spike`（可行性问题）/ `bounded`（仓库内既有流程的改动，对话内短设计）/ `architectural`（新子系统或重构，须写设计文档）；拿不准走重的那条，中途发现复杂度升级立即停下并重分类。
2. **探索上下文**：读任务来源、相关代码/测试、既有规格，把「内容确认」建立在对 repo 现状的核对之上。
3. **一次一个问题澄清（走「澄清提问」轮次）**：目的、边界、成功标准、约束（版本号互斥、资源上限、版权红线）——每个细节问题用 `confirm.sh ask --id X-xx --kind question --title "…澄清" --question "<一次一个问题>"` 推送（卡片标题「澄清提问」）；**带选项的问题用 `--option "..."` 逐项传**（可多次），卡片把选项渲染成**按钮**（点哪个答哪个，value 携带请求 ID；≤5 个按钮，更多时提示文本回复）：`--option "A. 只补白名单" --option "B. 白名单+兜底"`；你在飞书点按选项按钮、在卡片输入区手写后点「发送」、或「回复」该卡片消息（action=`answer`）→ 实现者继续分析/追问。同一任务澄清轮**最多 5 轮**，超限把未决假设作为 Ruling 写进短设计并转 approval 收口；`reject` 随时可终止。
4. **呈现设计并等待显式批准**：设计敲定后发 `--kind approval`（默认）确认请求——bounded 给几段短设计；architectural 按节呈现并写 `docs/superpowers/specs/` 设计文档。批准来源 = 用户或控制会话的显式确认（`approve`）。
5. **未经批准不得开工**：简单任务只是设计更短，不是免批准；收到 `edit`（修改意见）时修订设计后重新走 approval 轮。

### 确认通道：设备优先，回复即续跑

机制见 `docs/agents/confirmation-channel.md`；通道由 `AGENT_CONFIRM_CHANNEL` 决定（未设则 auto 探测：有飞书配置用 feishu，否则 none）。

1. **发起**：`confirm.sh ask --id X-xx --title "..." --category bounded|architectural --question "..." --design "..."` —— 配置了飞书时推送到你的设备；无飞书自动降级 discussion/none 并按提示处理。
2. **等待**：`confirm.sh wait --id X-xx --timeout-min 2`——短等即可。收到回复（approve/edit/reject + 文本）→ 把结论写入 OpenSpec change 的 proposal/design 与 brief → 从第 4 步继续；**没收到就正常退出**，续跑交给 listener（见下），不要长等（避免与原会话双开）。
3. **自动续跑**：`feishu-listener.js`（常驻）收到回复后写 `<id>.reply.json`，并以 `AGENT_RESUME=<id>` 在后台重跑 `run-agent.sh implementer`——本会话若见 `AGENT_RESUME` 环境变量，直接读对应 reply 文件继续（不重新认领）；这是 headless 下的**唯一**恢复入口。
4. **无通道/发送失败**（请求 channel 为 discussion|none）：才降级 Discussion 协议——结构化评论发到 Discussion #71 对应行、backlog 该行备注标「待确认」，**停在确认点**，不得静默开工；用户回复后 `confirm.sh reply --id X-xx --action ...` 或下次调度恢复。**注意**：飞书卡片已送达但用户暂未回复（wait 超时）**不是**无通道——只停确认点等 listener 续跑，**不要**往 Discussion 写卡片转录；讨论区只承载【状态变更】与规划者记录，不承载澄清提问/内容确认的正文。
5. **终端即时问答**（手动模式）：`AGENT_INTERACTIVE=1 scripts/agents/run-agent.sh implementer`（claude 交互模式，可直接对话确认后再开工）。

## 第 4 步：开发（子智能体驱动）

严格按 `docs/development-process.md` 阶段 2–4（前置的「阶段 1 内容确认门禁」已通过）：

1. 建 OpenSpec change（复杂功能必建；F 组小型修复走直接修改豁免）。
2. 每个 Task 派发**全新** implementer 子代理；brief 必须自包含：当前 Task、契约 SHA、change 产物路径、全局约束（AGENTS.md 关键条款）、精确验证命令；禁止子代理自我派生。
3. TDD；每 Task 后独立 SPEC + QUALITY 双评审；修复 ≤5 轮（R≤3 原实现者，R≥4 换新）；所有结论与 Ruling 写 `ledger.md`。
4. 全部 Task 完成后整分支终审；跑 `scripts/agents/gates.sh` 全量门禁；改动域涉及渲染/tick/存储/协议时补 benchmark（数值只记录）、fuzz/golden、`make visual` 或 `perfcheck`。

## 第 5 步：自动收尾（完成即执行）

```text
□ 全量门禁与整分支终审通过（gates.sh；改动域对应 benchmark/fuzz/golden/visual 已核）
□ openspec sync（delta 沉淀主规格）→ 逐 change openspec archive
□ AGENTS.md 与 CLAUDE.md 逐字节相同（cmp -s）且只写已验证事实
□ docs/notes/progress.md 追加基线段落
□ docs/feature-backlog.md 该行 → 已完成（认领人保留履历）；集成任务受影响时同步 A/I 行
□ GitHub Discussion #71 **追加状态评论**（每次状态变化都要，含 F 组）；
□ **同步正文**：python3 scripts/agents/refresh-discussion.py --update（正文列表随仓库状态**即时**刷新；脚本幂等，规划者每轮仍会全量对账）
   gh api graphql --input <(jq -n --rawfile b /tmp/disc-note.md --arg q 'mutation($b:String!){ addDiscussionComment(input:{discussionId:"D_kwDOToJS8M4Aou6G", body:$b}){ comment { id } } }' '{query:$q, variables:{b:$b}}')
□ 门禁证据归档：ledger 补最终验证输出摘要（数值记录，不改基线）
□ 推送分支：git push -u origin <branch>
□ 创建 PR：gh pr create --title "feat: <行 ID> <功能名>" --body "<OpenSpec change 链接 + 规划行 + 验证摘要>"
□ 启动 PR 收尾守护（detached，会话退出也不丢）：nohup scripts/agents/pr-finalize.sh <PR号> >> ~/Library/Logs/mornlea-pr-finalize.log 2>&1 &
   守护职责：监听 CI 至完成 → 失败自动 failed-only 重跑（最多 3 轮，flaky 免疫）→ 仍失败保持 OPEN 并在日志登记
   → 全绿后 gh pr merge --merge；实现者随后只需确认合并结果（gh pr view <PR> --json state）
   如需主动修复：gh run view <run-id> --log-failed（或 gh pr checks --json name,state,url）定位 → 本地修复并推送 → 守护会自动重跑
□ 确认合并后同步本地 main：git fetch origin && git checkout main && git pull --ff-only
□ 接力循环（AGENT_LOOP=1 时）：rm -f ~/.mornlea/loop.guard${WORKER_ID:+.${WORKER_ID}} && scripts/agents/relay.sh（清**本链**守卫；relay 按同一链身份接力，工具继承 AGENT_TOOL）
   —— 无未认领任务时 relay.sh 自动终结循环；手动单次运行（AGENT_LOOP=0）跳过
(AGENT_MODE=merge 时跳过 PR，直接本地合并并推送 main，仍需本地全绿)
```

## 状态变更评论模板（讨论 #71，每条状态变化一条）

```text
【状态变更】<ID> <功能名> → <状态：已认领|开发中|已完成|放弃>
- 时间：<UTC>（如 2026-08-24T16:50:02Z）
- 认领人：<agent> @ <分支名>
- 关键证据：PR #<n>（CI n/n 全绿 · merge <sha 前 7 位>）｜ commit <sha 前 7 位>｜ OpenSpec change <名称>
- 备注：<一句话结果/用途>（例如：独占文件集：<files>；flake 重跑一次后全绿）
（状态以仓库 `docs/feature-backlog.md` 为准；正文列表已由实现者同步，规划者每轮仍全量对账）
```

完成态示例（可直接套用）：

```text
【状态变更】F-03 「使用」键放置判定收敛 → 已完成
- 时间：2026-08-24T16:50:02Z
- 认领人：claude-implementer @ fix/F-03-use-key-placement
- 关键证据：PR #74（CI 8/8 全绿 · merge 939b35d3）｜ commit ca5794cd
- 备注：placeBlock 判定收敛至 core.ItemPlacement；首跑伙伴台词 flake 重跑后全绿
（状态以仓库 docs/feature-backlog.md 为准；正文列表由规划者每轮刷新）
```

## 收尾自查清单（提交前）

0. `brainstorming` 确认已获显式批准，且确认结论已写入 proposal/design 与 brief。
1. `go test -list` 集合语义一致；`gofmt -l .` 无输出；`go vet ./...` 干净。
2. `openspec validate --all --strict --no-interactive` 通过；全部 Task 已勾选并核对。
3. `AGENTS.md` 与 `CLAUDE.md` 逐字节相同（`TestBaselineDocsAreIdentical` 兜底）。
4. 无超范围改动：git diff 只含本行声明的独占文件集（+ 基线文档同步）。
5. 未决项已誊入「延期与放弃」；ledger 的最终裁决已写。
6. 本行 `已完成` 已在仓库与讨论两处同步。

## 红线

- 已认领行不得抢；跨行依赖未满足不得开工。
- 不得为通过测试放宽正确性、资源上限、报告完整性、真实 overflow 或数据丢失门禁；benchmark 数值不改变退出状态。
- 不得绕过或不修改 `.codex/hooks.json` / `.claude/settings.json` 共享的 `scripts/agent-hooks/guard.mjs` 及其豁免变量。
- PR 合并前确认整分支终审与 `gates.sh` 全量门禁通过、无未推送功能分支依赖本行产出；集成批次按计划固定合流顺序，不机械 ours/theirs；多条流水线并行时逐条监听，任一失败都先修复再合并。
- 自动测试不得启动或聚焦前台游戏窗口；视觉验收只在用户明确要求时人工跑。