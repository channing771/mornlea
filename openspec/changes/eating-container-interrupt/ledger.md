# Ledger: eating-container-interrupt

## 2026-08-25 认领与内容确认

- **Ruling: 认领 B-31 并批准与 A-01 的最小受控重叠 — 全部未认领行均与在途认领（A-01/A-02/A-04/A-05/B-07/B-13/E-04/E-12）冲突或依赖未满足，B-31 是唯一冲突面只剩一行调用点的行；按「重叠即换行」的例外须控制会话裁决 — 排查期一度考虑 B-14/D-06，两者分别撞上 A-01+A-04 的 `cmd/mornlea/app*.go` 双重独占与 A-02 的 `internal/assets` 独占。**
- 认领提交 `5dd018d5`（main，docs-only）；Discussion #71 已发【状态变更】评论（`DC_kwDOToJS8M4BFPO7`）并 `refresh-discussion.py --update` 刷新正文（76 行）。
- worktree `.worktrees/B-31-eating-container-interrupt`，分支 `feat/B-31-eating-container-interrupt`，基于 `main@5dd018d5`。
- **Ruling: 阶段 1 短设计获批（bounded）— 用户显式批准 — 中断判据为 `session.viewContainer || !session.hasView`，与 `mining.go:212` 形态对齐；`player.go` 重叠压到一行；客户端输入抑制列为非目标。** 结论已写入 proposal.md 与 design.md。
