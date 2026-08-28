# Ledger

## Baseline

- 隔离 worktree：`/Users/chen/work/mornlea/.worktrees/A-03-tiered-swords-combat`，分支 `feat/A-03-tiered-swords-combat`。
- Task 1 起始 HEAD：`6a992a5c5081d707423b2361ea5dc985375f19c8`；起始 `git status --short --branch` 退出 0，仅输出分支名，无 tracked/untracked 改动。
- 冻结检查对应的代码基线：`67fcc604bd3f3b5ce9326b5cd7498381163296d6`。Task 起始 HEAD 与该 SHA 的差异只承接控制器已完成的 admission/claim 规划事实；本 Task 不重复 admission、认领或 frozen tests。
- 基线版本矩阵：协议 v31、player schema v8、chunk schema v9、world metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、engine ABI v8、client ABI v10、benchmark scenario v19。
- 基线 append-only 与固定容量：`ItemBed=46`、`ItemIDMax=47`、`RecipeBed=16`、Play S→C registry 尾号 24、正式 capture 23 项、HUD 最大关闭/打开/容量 96/257/267。
- 已继承证据：`CARGO_TARGET_DIR="$PWD/engine/target" make rust`，exit 0，隔离 Rust release build 通过；tested SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`。
- 已继承证据：`go test ./internal/archcheck -run 'TestBaselineVersionsMatchCode|TestClientCommandSubpackageDependencyDirections' -count=1`，exit 0，版本矩阵与 client command 分包依赖方向通过；tested SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`。
- 已继承证据：`go test ./internal/core ./internal/network ./internal/render/hud ./cmd/mornlea/capture -run 'Test(ItemIDsAppendOnly|RegisteredRecipeCellsStayInsideShapeBounds|ProtocolVersionPinned|HostileMessageIDsAreFrozen|HotbarLayoutStaysWithinFixedCapacity|CaptureSceneOrderAndAICompanionDeterminism)$' -count=1`，exit 0，冻结 protocol v31、item 46/47、recipe 16、S→C tail 24、23 scenes 与 HUD 96/257/267；tested SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`。

## Rulings

- Ruling: 继承控制器已完成的 Task 1 Steps 1–4，不重复 promotion、claim、Discussion、worktree、Rust build 或 frozen tests — 这些前置已给出可复用的 SHA/exit 证据，重复执行既无新增事实又违反任务边界 — 原问题是 brief 的完整流程仍列出 admission 与基线命令，但当前 implementer 只负责 Steps 5–7 的 artifacts。
- Ruling: 不编辑 `docs/feature-backlog.md` — admission 与认领规划事实已经由控制器单独完成，本提交只包含 active change 文件 — 原问题是原始 Task 1 文件清单包含 backlog，但控制器明确裁定该 prerequisite 已完成且不得重复。
- Ruling: 同时记录 Task 起始 HEAD `6a992a5c5081d707423b2361ea5dc985375f19c8` 与 frozen code SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6` — 前者标识本次 artifact 工作的真实起点，后者标识可复用代码/测试证据的真实内容基线 — 原问题是单写任一 SHA 都会混淆 docs-only claim 提交与代码冻结证据。
- Ruling: `openspec/config.yaml` 保持不变并忽略其中过时的 v27/v7/v2/ABI prose — 仓库真相优先级要求使用代码、测试、根 `AGENTS.md` 和批准设计，Task 6 负责最终同步配置 — 原问题是 CLI instructions 会注入 stale context，若照抄会把 active change 降回错误版本矩阵。
- Ruling: 不读取、修改或消费 dirty sibling worktrees — 本任务只在已指定 isolation worktree 内工作，兄弟工作区与本 change 无关 — 原问题是并行分支可能包含未合入状态，读取它们会污染批准基线与文件所有权。
- Ruling: 不执行 GitHub Discussion 远程 bookkeeping — Discussion 暂时不可达且 admission/remote 同步由控制器持有，本 Task 只记录该边界 — 原问题是原计划要求 GraphQL 评论与正文 refresh，但当前网络状态和职责分工均禁止 implementer 重试。
- Ruling: 不派发 subagent 或独立 reviewer，本 Task 只执行 implementer self-review — 控制器对本次 fresh implementer 明确下达 no-dispatch/no-reviewer 指令，高于计划中的一般双评审流程 — 原问题是 brief 的 Step 7 通常要求 fresh SPEC/QUALITY 评审，但本次任务要求由控制器在 implementer 之外处理任何后续审阅。
- Ruling: `.openspec.yaml` 使用 CLI 生成的 `spec-driven` schema，并创建 proposal、11 份 delta、design、仅含 Tasks 2–5 的 tasks 与 ledger — 这是 `applyRequires=tasks` 的完整 transitive closure，同时排除 Task 1 admission 和 Task 6 integration runbook — 原问题是只创建 `tasks.md` 会让 file-existence status 虚假完成，也会把 archive/PR 工作错误暴露为 active implementation checkbox。
- Ruling: 协议唯一升版为 v32，S→C 追加 `CombatHit=25`，其余 player/chunk/world/companions/hostile/engine/client/benchmark 保持 8/9/3/4/1/8/10/19 — 批准设计已完成兼容选择且没有待定值 — 原问题是 `openspec/config.yaml` 的旧版本 prose 与当前代码事实冲突。
- Ruling: 统一战斗固定 72 actor/72 intent、hostile-first reservation、player 10 tick、hostile 20 tick、0.35 击退和整阶段 overflow fail closed — 这些是批准的玩法与安全边界，不能在实现期配置化或静默改变 — 原问题是保留双循环、截断 overflow 或统一 cooldown 都会破坏跨类型确定性和 A-04 兼容。
- Ruling: HUD 以代码事实 96/257/267 为基线，marker 后为 100/261，不沿用主规格中过时的 266 — 容量测试已在 frozen SHA 证明真实 opening maximum 为 257，新增 4 quad 无需扩容 — 原问题是旧 `container-ui-presentation` 数字会诱导扩大固定上传布局并造成 client ABI/benchmark 漂移。
- Ruling: 正式 capture 从 23 增为 24，固定相邻顺序为 `ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope` — 该顺序同时保持 `far-horizon` 倒数第二与 `water-underwater` 唯一末项 — 原问题是主规格残留 19/21/22 项历史口径且 hostile 原先直接紧随 companion。

## Task Reviews

- Task 1 implementer self-review round 1，SPEC PASS：proposal、11 份 delta、design 与 tasks 均逐项覆盖 brief/design 的 ItemID 47..53、recipe 17..19、2/4/5/6 damage、59/131/250 durability、72/72 failure boundary、mixed target 全序、10/20 tick、hostile-first、互击、0.35 击退、100 milli fatigue、private v32 `CombatHit=25`、PCM 参数/hash、4 quad/6 成功帧、100/261≤267 和 24-scene order；没有新增玩法值或开放问题。
- Task 1 implementer self-review round 1，QUALITY PASS：artifact 依赖按 `openspec status --json` 与 `openspec instructions ... --json` 的顺序创建；modified requirement 使用现有 requirement 精确标题；tasks 只有计划 Tasks 2–5，未包含 admission、sync/archive、PR、CI 或 cleanup；没有生产/测试代码或长期配置改动。
- Task 1 implementer self-review round 2，SPEC PASS：逐文件复核完整 16-file active change diff，并与 task brief、批准设计和实施计划 Tasks 2–5 对照；11 个 capability、固定值、failure/capacity boundary、兼容矩阵与 publication/client/capture 边界均完整，无遗漏、冲突或待定值。
- Task 1 implementer self-review round 2，QUALITY PASS：13 个 `MODIFIED` requirement 标题均与当前主规格精确匹配；所有 requirement 都有可判定 scenario；tasks 保持 2.1..2.9、3.1..3.11、4.1..4.8、5.1..5.13 且不包含 Task 1/6；全文未发现需要修复的 finding。
- Task 1 独立评审，SPEC PASS：评审报告 `.superpowers/sdd/2026-08-28-tiered-swords-unified-combat/task-1-review.md:1-11` 对提交区间 `6a992a5c..80c4affa` 的 16-file active change 完成核对；11 个 capability、冻结增量/不变矩阵、玩法数值、72/72 fail-closed、hostile-first、私有 `CombatHit`、HUD/capture 边界与 Tasks 2–5 镜像均符合批准设计，无法从 diff 验证的项目为 0。
- Task 1 独立评审，QUALITY PASS：同一报告 `:13-42` 给出 Task quality `Approved`，Critical 0、Important 0、Minor 0；没有 correctness、scope、maintainability 或 verification finding。
- Task 1 完成：起始提交 `6a992a5c` 到 proposal 提交 `80c4affa` 的 docs-only 区间已完成 implementer self-review、独立 SPEC/QUALITY 评审和既有 validation evidence，后续工作从 `tasks.md` 的 Task 2 开始。

## Validation Evidence

- 基线验证证据见 `Baseline`，均对应 frozen code SHA `67fcc604bd3f3b5ce9326b5cd7498381163296d6`，本 Task 未重复运行。
- `openspec status --change tiered-swords-combat`，exit 0，`4/4 artifacts complete`；验证时 HEAD `6a992a5c5081d707423b2361ea5dc985375f19c8`，工作区仅含本 change 的 16 个 intent-to-add 文件。
- `openspec validate --all --strict --no-interactive`，exit 0，77 passed、0 failed；验证时 HEAD 与工作区状态同上。
- `git diff --check`，exit 0，无输出；验证时 HEAD 与工作区状态同上。写入本证据后，提交前 MUST 对最终 active change bytes 重新运行同一组三项命令。

## Deferred And Abandoned

- 无未决玩法、协议、容量、兼容或视觉数值。
- OpenSpec sync/archive、长期版本文档、`openspec/config.yaml`、backlog/Discussion、PR/CI 与 worktree cleanup 明确属于计划 Task 6，不是本 active change checklist 的延期项。
- dirty sibling worktrees 与 `docs/notes/lan-server.md` 永久排除在本 change 文件范围之外。
