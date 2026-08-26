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
| 1 Idle node contract | — | pending | pending | 0 | pending |
| 2 Schedule and dispatch | — | pending | pending | 0 | pending |
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

## Execution Notes

- 每个 Task 的任务 brief 是 implementer 唯一需求来源，必须附 proposal、delta spec、design、implementation plan、基线 SHA、文件边界和精确命令。
- 每个 Task 使用 fresh implementer；SPEC 与 QUALITY 使用彼此独立且未实现该 Task 的 fresh reviewer。
- 修复轮次 R1–R3 续用原 implementer，R4–R5 换 fresh implementer；超过 5 轮停止并逐条裁决。
- tasks checkbox 只在实现、focused verification、SPEC review 与 QUALITY review 全部通过后勾选。
