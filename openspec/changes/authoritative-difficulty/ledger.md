# authoritative-difficulty ledger

进度、评审结论与裁决记录。验证证据按 SHA 复用：同一 SHA 且工作区未再改动时，后续评审直接引用。

## 记录

- 2026-09-11 认领：B-11 由 zcode-control 认领（backlog claim commit `456e053f`，分支 `feat/B-11-difficulty`，worktree `.worktrees/B-11-difficulty`）。内容确认来源：生存闭环收尾战役设计（`docs/superpowers/specs/2026-09-11-survival-loop-completion-design.md`）经用户 2026-09-11 显式批准（「认可」批准战役、两次「继续」推进裁决与立项），等价 brainstorming 硬门禁的显式批准。
- 2026-09-11 Ruling: 孤儿分支只收编设计、不收编代码 — 其 81 文件全部位于单元化重构前的 `internal/` 路径且 metadata v3 槽位已被占用，rebase 不可行而设计论证完整可复用 — 此前规划者 2026-09-01 校对发现该分支但未裁决，悬置两周。
- 2026-09-11 Ruling: 范围补「和平档夜行者生成门控」 — 积压表 B-11 的「刷怪门控」半边在孤儿设计中被列为非目标，属当时范围收窄而非否决；难度建域固定且旧档迁移恒 normal，门控入口短路即可，无需 despawn — 若沿用孤儿非目标口径，`peaceful` 只剩饥饿语义差异，与积压表行文不符。
- 2026-09-11 立项：change 五件套（proposal/design/tasks/ledger/delta specs）就绪并推送；基线 SHA `1da2f345`；`openspec validate --all --strict --no-interactive` 107 passed / 0 failed。
- 2026-09-11 任务组 1 完成（基线 `d4cccb6c` → `bde98247`，4 提交：`916f47c4` core 域值 / `adcb46c0` codec v6 / `cd7ad3c3` fixture 清扫 / `bde98247` 基线钉值）。验证证据（SHA `bde98247`）：`go test ./packages/shared/core -race -count=1` ok（2.078s）、`./packages/server/storage -race -count=1` ok（11.204s）、`./packages/audit -count=1` ok（6.968s）；mornlea-server / app / benchmark 定点 ok；gofmt 与 `git diff --check` 干净。
  - Ruling: 接受跨包 `FormatVersion: 5→6` 清扫（64 文件） — encode 前置版本校验迫使全部 Create 构造点与 fixture 升 v6，沿 v4→v5 先例（`4649698a`）独立提交、评审逐行确认纯机械 — 若不清扫，专服与图形客户端新世界创建即失败。
  - Ruling: 接受 encode 侧难度校验与四个迁移测试改名 `ToV5`→`ToCurrent` — D1「非法值由编解码边界守住」双侧实现更稳；迁移目标随版本演进，原名误导。
  - 任务组 1 评审：PASS（SPEC 六项逐条通过、QUALITY 无 blocking/major；1 minor 为 ledger/勾选滞后已由本条清偿，1 nit CRC 锚点沿用 v5 惯例不改）。
  - 预存免责（非本组引入，基线 `d4cccb6c` 复现）：`packages/server/server` 的 `TestDualDimensionReloadAfterRestart`（cleanup 期 `flush passives: unsupported passive dimension 1`）与 `TestWarpParityMemoryVsTCP`（仅整包 `-short` 跑法）失败，属 F-11 在案 flake 族，移交 F-11 取证，本 change 不修。
