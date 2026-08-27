# companion-idle-dialogue 执行账本

## Baseline

| Item | Evidence |
|---|---|
| backlog promotion | `929d2da5 docs(backlog): promote C-08 idle dialogue` |
| backlog claim | `87928315 docs(backlog): claim C-08 companion idle dialogue` |
| approved design | `3f91fc31 docs: design C-08 companion idle dialogue` |
| implementation plan | `c60e8f69 docs: plan C-08 companion idle dialogue` |
| worktree | `.worktrees/feat/C-08-companion-idle-dialogue` on `feat/C-08-companion-idle-dialogue` |
| `make rust` baseline | pass；Rust 1.97.1 release build，`Finished ... in 48.58s` |
| companion race baseline | pass；`ok github.com/channing771/mornlea/internal/companion 4.102s` |
| server race baseline | pass；`ok github.com/channing771/mornlea/internal/server 200.021s` |
| OpenSpec baseline | proposal/spec/design creation期 `openspec validate companion-idle-dialogue --strict --no-interactive` pass |

首次组合运行 `go test ./internal/companion ./internal/server -race -count=1` 在工具 120 秒上限被终止：companion 已 pass，server 无失败输出；随后以 600 秒预算单独重跑 server 并在 200.021 秒通过。该事件是执行器预算不足，不裁决为测试失败，也未修改测试超时。

## Approval Evidence

| Decision | Result |
|---|---|
| 候选选择 | 用户批准从无 `就绪` 状态下由控制会话晋升 C-08 |
| 核心语义 | 用户批准最近真实发令者在线且水平 16 格内、每伙伴确定性 60–120 秒 |
| 架构 | 用户批准权威 tick 期限与既有 Dialogue 管道复用 |
| 并发 | 用户批准无抢占；idle 在途时任务台词按单在途规则跳过 |
| 文件与验证 | 用户批准最小文件集、TDD、Memory/TCP parity 与全仓门禁 |
| written design | 用户批准 `docs/superpowers/specs/2026-08-27-companion-idle-dialogue-design.md` |

## Task Status

| Task | Implementer | Spec Review | Quality Review | Repair Rounds | Status |
|---|---|---|---|---:|---|
| OpenSpec planning | control session | approved (`ses_fc0f2b5d2ffe47hJ7tMMWCV64G`) | approved (same review) | 2 | complete |
| 1 Idle node contract | `ses_fc0dbc81dffe0u3tj65YfW1cXN` | approved (`ses_fc0d74a4affeN0kkoo0FPgXyDJ`) | approved (`ses_fc0d74a37ffeZe7a5DYuWS0gxb`) | 0 | complete |
| 2 Schedule and dispatch | `ses_fc0d26fa8ffeUK5NbFSQdKC4z9` | approved (`ses_fc0bf6f63ffeYfIPRx7r9rFHJg`) | approved (`ses_fc0bf6f59ffe4jfnuZOUEyubUX`) | 1 | complete |
| 3 Outcome and parity | — | pending | pending | 0 | pending |
| 4 Whole-branch gate | control session | pending | pending | 0 | pending |

## Rulings

| ID | Task | Finding | Decision | Evidence |
|---|---|---|---|---|
| C08-D-R1 | design | 初稿把任务空闲与身体/在线/距离资格混为同一状态，无法判定离线或超距是否重置期限 | 分离“queue 完全空闲计时”与“期限到达时发言资格”；只有 current/pending 清期限，资格失败消费机会并排下一期 | 独立设计复审 `ses_fc111dd1effeIFcOFFAtvViyNX`，修订后 PASS |
| C08-D-R2 | design | 恢复任务会留下合法但合成的 `restoredIssuerIdentity`，仅按 UUID/在线性无法证明是真实最近发令者 | 在 server-only `companionTaskIssuer` 内增加 `restored` 标记；恢复身份永不取得 idle 资格，真实任务整体替换 issuer 值 | `companion_manager.go` 恢复路径核对；同一复审 PASS |
| C08-D-R3 | design | 初稿把溢出 deadline 饱和到 `MaxUint64`，在极限 tick 会缩短 1200 tick 下限 | 改用 `uint64` 模加法与 `int64(now-deadline) >= 0` 半区间比较，并增加跨回绕测试 | 同一复审 PASS；最大间隔 2400 远小于半个 `uint64` 空间 |
| C08-D-R4 | design | “结果可重放”混淆确定机会与异步模型输出 | 只承诺机会期限序列确定；parity 在受控 fake 模型下比较业务事件投影，不比较绝对 tick/跨传输 EventID | 既有 `dialogueParityProjection` 纪律与同一复审 PASS |
| C08-P-R1 | planning | 派发测试手工预设期限且只断言下一期限不同，无法抓住从未首排或从观测 tick 漂移递推的实现 | 增加无期限首排、晚到时从旧期限精确递推和任务清空后重排三项 RED 契约 | 独立 artifact review `ses_fc0f2b5d2ffe47hJ7tMMWCV64G` Important 1，accepted |
| C08-P-R2 | planning | 收尾 `gofmt -w` 无参数不可执行，工作树 `git diff --check` 不覆盖已提交 Task，OpenSpec validation 也不能证明版本/capture/golden 未改 | 列出 9 个 Go 文件；同时检查 `c60e8f69...HEAD` 与工作树；审核完整 changed-file allowlist，把版本等不变性归到 diff 证据 | 同一 review Important 2，accepted |
| C08-P-R3 | planning | generation stale 用启动任务制造了第二个拒绝条件；模型错误未证明 next deadline 不被重复安排 | generation outcome 只改 outcome generation 并保持 queue 空；error 精确断言已安排 deadline 不变 | 同一 review Minor 1，accepted |
| C08-P-R4 | planning | 实现计划要求 `tasks.md` 在 archive 前包含 archive 本身，与仓库要求 tasks 先全完成再 archive 冲突 | `tasks.md` 只负责 archive readiness；sync/archive/backlog/Discussion 保留为 checkbox 全完成后的外部集成门禁 | 同一 review Minor 2，accepted |
| C08-P-R5 | planning | range changed-file audit 不包含 staged、unstaged 与 untracked 文件，单独的工作树 whitespace 检查不能证明路径范围 | 增加 cached whitespace 检查并要求 audit 前 worktree 完全 clean；随后 range path list 覆盖全部 change 文件 | 同一 review repair round 1 Important 1，accepted |
| C08-T2-R1 | 2 | 初版测试全部直接调用 `dispatchIdleDialogues`，删除/调换 authority tick 接线或放弃 `orderedIDs` 扫描仍可假绿 | 增加真实 `StepForTest` 的 Planner 先占最后共享槽测试，以及非排序输入下单槽 canonical dispatch 测试；不修改生产行为 | phase swap mutation 在 line 432 失败；reverse scan mutation 在 line 514 失败；SPEC/QUALITY 复审均 approved |

## Verification

| Command | Result | Evidence |
|---|---|---|
| `make rust` | pass | baseline release build completed |
| `go test ./internal/companion -race -count=1` | pass | `ok .../internal/companion 4.102s` |
| `go test ./internal/server -race -count=1` | pass | `ok .../internal/server 200.021s` |
| `openspec validate companion-idle-dialogue --strict --no-interactive` | pass | change valid during artifact creation |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 67 passed, 0 failed (67 items)` before planning review |
| OpenSpec planning independent review | pass | repair round 2 后 reviewer 明确 `PASS`；无 Critical、Important 或 Minor finding |
| backlog `开发中` transition | pass | `refresh_discussion_test.py` 5/5、`relay_test.py` 3/3；Discussion #71 正文刷新为 82 行，状态评论 `DC_kwDOToJS8M4BFS_p` |
| Task 1 RED focused | expected fail | `DialogueNodeIdle` 未定义导致 5 处 build error |
| Task 1 GREEN focused | pass | `go test ./internal/companion -run 'TestDialogueNodeValidateMatrix|TestDialogueClientIdleNodePayload' -count=1` → `ok .../internal/companion 0.674s`；reviewer 重跑 `0.593s` |
| Task 1 companion race | pass | `go test ./internal/companion -race -count=1` → implementer `4.093s`；SPEC reviewer 重跑 `4.011s` |
| Task 1 diff and scope | pass | `git diff --check` 无输出；`09354e4a..a20b06e8` 仅含 4 个计划内 `internal/companion` 文件 |
| Task 1 independent reviews | approved | SPEC 与 QUALITY 均无 Critical、Important 或 Minor finding |
| Task 2 RED focused | expected fail | scheduler helper、audience 与 dispatch 尚不存在导致 build error |
| Task 2 GREEN focused | pass | 初版 `0.756s`；repair 后 `1.000s`；SPEC reviewer race 10 次 `4.117s`；QUALITY reviewer ordered/phase 分别 10/20 次通过 |
| Task 2 server race | pass | 初版 `209.752s`；repair 后 fresh rerun `210.855s` |
| Task 2 archcheck | pass | 初版 `4.802s`；repair 后 `4.907s`；reviewer 重跑通过 |
| Task 2 diff and scope | pass | cumulative `bea17388..dee6be55` 只含 4 个计划内 server 文件；repair 只改 idle 测试；diff/status clean |
| Task 2 independent reviews | approved | repair round 1 后 SPEC 与 QUALITY 均无 Critical、Important 或 Minor finding |
| Task 3 RED focused | expected fail | `TestIdleDialogueOutcomeValidBroadcastsWithoutSummary` facts 为空、双广播 0/0、parity 无 idle speech；3 个既有语义锁定用例按预期 PASS |
| Task 3 GREEN focused | pass | 初版 `1.016s`；repair 后控制会话 `-count=5` `2.110s` 复跑通过 |
| Task 3 companion+server race | pass | `internal/companion 3.802s`；`internal/server 209.030s` |
| Task 3 archcheck | pass | `ok 5.069s` |
| Task 3 diff and scope | pass | `40db7160` 只含 3 个计划内文件（439+/1-）；repair `10ee96c1` 只改 2 个测试文件 |
| Task 3 independent reviews | approved | SPEC 与 QUALITY 均 PASS、无 Critical/Important；各 2-4 条 Minor，其中 2 条经 repair round 1（`10ee96c1`）修复后 focused `-count=5` 全绿 |

## Task 3 Implementation Evidence

- 实现 commit：`40db7160cc8964906ca91b2b54fa30928d11bd0a feat(server): publish valid idle companion speech`。
- 评审修复 commit：`10ee96c1cfa97fff698e06618c24d0641ffd048c test(server): sharpen idle dialogue test synchronization`。
- `applyDialogueOutcome` node switch 追加 `DialogueNodeIdle` 分支，按 D7 重验空队列、真实同一 issuer（playerID+name）、非 restored、active 身体与在线+水平 16 格受众；generation 预检仍在 switch 前、err 处理在其后，任务节点分支未动。
- 有效 idle 复用 `applyDialogueEffect` 广播 speech；summary 仅 terminal 节点写入，idle 天然不摘要。模型失败不补发、不重排期。
- 测试覆盖：有效广播、七类 stale、`ErrDialogueUnavailable` 期限精确保持、idle 在途时任务启动不抢占（requests=1/inFlight=1/cancels=0）、双玩家广播逐字段一致、Memory/TCP parity 投影精确比较。
- 实现者两处编排偏离均经 SPEC reviewer 裁决为必要/加强：idle 请求须先于任务指令派发（否则 dispatch 清期限）；任务计划用六步保持 Running 避免断言窗口混入任务台词。

## Task 1 Implementation Evidence

- Commit：`a20b06e8a367da5b50411173b97196fbb981fc89 feat(companion): add idle dialogue node`。
- `DialogueNodeIdle` 追加在既有枚举末尾；零 payload 合法，三个任务 payload 字段任一非零均拒绝，HTTP kind 固定为 `"idle"`。
- 未修改系统提示、响应 schema、terminal 判定、server、协议或既有节点数值；Task 2–3 才接入调度与结果应用。

## Task 2 Implementation Evidence

- 实现 commit：`9b4a48d265269c662c2f3b068c43ad2f7c15b4b8 feat(server): schedule idle companion dialogue`。
- 评审修复 commit：`dee6be5527d0b3181f0de556d296f6cd829dbe44 test(server): cover idle dispatch tick ordering`。
- 期限由 `companion.ID || little-endian seed` 的 FNV-1a 64 确定导出，模加法与半区间比较覆盖 tick 回绕；首排、旧期限递推、任务重置和所有 due skip 均有精确期限断言。
- `restored` 只存在 server issuer 值；idle 在 Planning 之后按 `orderedIDs` 有界扫描，复用共享四槽与单在途纪律，不检查或递增任务八次预算。
- Task 2 保持旧 outcome switch 对 idle 的安全丢弃；结果重验与广播留给 Task 3。

## Execution Notes

- 每个 Task 的任务 brief 是 implementer 唯一需求来源，必须附 proposal、delta spec、design、implementation plan、基线 SHA、文件边界和精确命令。
- 每个 Task 使用 fresh implementer；SPEC 与 QUALITY 使用彼此独立且未实现该 Task 的 fresh reviewer。
- 修复轮次 R1–R3 续用原 implementer，R4–R5 换 fresh implementer；超过 5 轮停止并逐条裁决。
- tasks checkbox 只在实现、focused verification、SPEC review 与 QUALITY review 全部通过后勾选。
