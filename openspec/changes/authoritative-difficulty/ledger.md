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
- 2026-09-11 任务组 2 完成（基线 `bf0a4386` → `b08a7e4c`，3 提交：`f28b8034` 构造注入 / `b42d3725` 饥饿回血分档 / `b08a7e4c` 和平生成门控）。构造签名：`entity.NewState(seed, ...core.Difficulty)` 与 `runtime.NewEngine(viewRadius, worldTime, seed, ...core.Difficulty)`，缺省尾参=normal，非法/多尾参 panic 于构造期。验证证据（HEAD `b08a7e4c`）：`go test ./packages/server/sim/entity -race -count=1` ok（5.575s）、`./packages/server/sim/... -count=1` 四包 ok、audit ok、vet/gofmt 干净、`./packages/server/server -count=1` 仅剩两条 F-11 预存失败。
  - Ruling: 接受 peaceful 饥饿为零时 `starvationTicks` 冻结 — 该字段不进快照/恢复/哈希三条外露路径，包外不可观察，冻结与 normal 硬地板行为同构 — 规格只约束不扣血与不重置回血计时，无第三种可观察语义需要区分。
  - Ruling: 接受两个只读访问器（`State.Difficulty()` / `Engine.DifficultyForTest()`）— 沿 `SeedForTest` 先例、无写入口，供组 3 断言装配接线 — 无 accessor 则组 3 只能靠行为差异间接验证注入，证据更弱。
  - 任务组 2 评审：PASS（SPEC：normal 三处逐位一致有既有测试零改动+hard 首生成 tick 探针直证；QUALITY 无 blocking/major；1 minor 为 ledger/勾选滞后由本条清偿；1 nit `runtime/engine.go:96` doc 注释反引号内为含括号表达式 `store.Metadata().Difficulty`，audit 静默放行属绕过，并入组 3 一行搭车改为点路径写法）。
- 2026-09-11 任务组 3 完成（基线 `9ec45532` → `8695f850`，2 提交：`a4aea25b` 装配+三集成测试 / `8695f850` 组 2 nit 搭车清偿）。装配收口：`newWorld` 单点 `runtime.NewEngine(viewRadius, worldTime, seed, metadata.Difficulty)`，Memory/磁盘/benchmark 全形态汇入无分叉。三个测试：磁盘重启保值（重开不带 Create）、装配后改写 metadata 不影响快照（补足 spec 需求 2「权威 tick 不读磁盘」Scenario）、peaceful 生成探针跨 Memory/TCP parity（normal 控制腿运行时断言有鉴别力）。验证证据（HEAD `8695f850`）：`go test ./packages/server/server -race -count=1` ok（263.593s，两条 F-11 flake 本轮通过）、`./packages/server/sim/runtime -race -count=1` ok、audit 反引号门禁 PASS。
  - Ruling: 接受组 3 实现 bonus「tick 不读磁盘」测试 — delta spec 需求 2 明列该 Scenario，装配层必要补全 — tasks 3.2 字面未列，spec 驱动优先于字面。
  - Ruling: 实现者首版探针泄漏（依赖无 cleanup 的 `mustNewHost`，毒害三个进程级泄漏判定测试）经 stash 基线对照归因后彻底修复，所有 host/store 生命周期显式收口 — 教训记档：parity runner 必须显式 `Shutdown`，与既有纪律一致。
  - 任务组 3 评审：PASS（三条 INFO：探针 defer 次序与既有 runner 微异、首段失败路径 store 不关、defer 内 Fatalf 风格——均无害不整改，留档）。
