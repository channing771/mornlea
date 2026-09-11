# authoritative-difficulty ledger

进度、评审结论与裁决记录。验证证据按 SHA 复用：同一 SHA 且工作区未再改动时，后续评审直接引用。

## 记录

- 2026-09-11 认领：B-11 由 zcode-control 认领（backlog claim commit `456e053f`，分支 `feat/B-11-difficulty`，worktree `.worktrees/B-11-difficulty`）。内容确认来源：生存闭环收尾战役设计（`docs/superpowers/specs/2026-09-11-survival-loop-completion-design.md`）经用户 2026-09-11 显式批准（「认可」批准战役、两次「继续」推进裁决与立项），等价 brainstorming 硬门禁的显式批准。
- 2026-09-11 Ruling: 孤儿分支只收编设计、不收编代码 — 其 81 文件全部位于单元化重构前的 `internal/` 路径且 metadata v3 槽位已被占用，rebase 不可行而设计论证完整可复用 — 此前规划者 2026-09-01 校对发现该分支但未裁决，悬置两周。
- 2026-09-11 Ruling: 范围补「和平档夜行者生成门控」 — 积压表 B-11 的「刷怪门控」半边在孤儿设计中被列为非目标，属当时范围收窄而非否决；难度建域固定且旧档迁移恒 normal，门控入口短路即可，无需 despawn — 若沿用孤儿非目标口径，`peaceful` 只剩饥饿语义差异，与积压表行文不符。
- 2026-09-11 立项：change 五件套（proposal/design/tasks/ledger/delta specs）就绪并推送；基线 SHA `1da2f345`；`openspec validate --all --strict --no-interactive` 107 passed / 0 failed。
