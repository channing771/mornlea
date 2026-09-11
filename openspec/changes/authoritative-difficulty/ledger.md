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
- 2026-09-11 任务组 4 完成（基线 `27655b60` → `93c132b4`，单提交：`--difficulty` 旗标 + 监听前一致性校验 + 8 测试）。语义矩阵四格：省略×新世界=normal、省略×已有世界=metadata 生效不比对、显式×新世界=显式值、显式×已有世界=监听前比对（不一致 `errors.Join(明确错误, store.Close())`，错误指名两值）。listener 未创建以 `listenTCP` 注入布尔钉住（run 内唯一创建点）。验证证据（HEAD `93c132b4`）：`go test ./packages/server/cmd/mornlea-server -race -count=1` ok（4.012s）、storage Metadata 定点 ok、vet/gofmt 干净。
  - 任务组 4 评审：PASS（SPEC 四格全直证、`flag.Visit` 区分省略与显式 normal、非法值解析期拒绝不进世界打开路径、图形客户端零改动；三条 INFO：旗标校验顺序无 spec 约束、CLI 一致侧断言止于 metadata（Engine 注入由组 3 钉住分工合理）、勾选留控制会话——均不整改）。
- 2026-09-11 任务组 5 完成（基线 `19dd6e91` → `8d79437a`，单测试提交、生产零改动）。四条启动路径共用 `parseMainOptions`（`flag.ContinueOnError`，从未注册 `--difficulty`），未知旗标解析期即拒；测试两层断言（解析失败 + 错误含 `not defined`）钉住「无入口」，传合法值 `normal` 保鉴别力。评审独立复核：客户端子树唯一 flag 入口、生产零 difficulty 引用。验证证据（HEAD `8d79437a`）：`./packages/client/cmd/mornlea/... -count=1 -short` 五包 ok、race 定点 ok、gofmt 干净。
  - 任务组 5 评审：PASS（三条 INFO：`--motion-demo` 组合未列但结构上等同、勾选留控制会话、stdlib flag 错误文案依赖属公开可观察行为——均不整改）。
- 2026-09-11 任务组 6 完成（基线 `08f27ce9` → `f0077b5c`，2 提交：`1242f25d` audit 域守卫 / `f0077b5c` stdlib 裸词修正）。契约复读十条对照全部一致或等价；新增 `packages/audit/difficulty_domain_test.go`（`TestDifficultyStaysASingleCoreDomain`）钉三条边界：blind 子树零难度标识符/字符串、`Difficulty` 类型声明仅限 `packages/shared/core`、三档文本字面量仅权威文件（实现者注入式验证三种违规形态）。验证证据（HEAD `f0077b5c`）：openspec strict 107/107、audit ok（修复前 FAIL 即门禁真实起效）、六模块 vet 零输出、gofmt/`git diff --check` 干净。
  - Ruling: 采纳评审建议，组 4 遗留的 `` `flag.Visit` `` 反引号失真按「标准库名属裸词」既定约定修正、不开 exemption 先例 — exemption 只登记实际出现的非 Go 域名字，为标准库开先例会让 fmt/slog 等系统性涌入并掩盖真失真 — 该教训记档：任务组验证命令应含 `go test ./packages/audit`，组 4 的验证矩阵漏了它。
  - Ruling: 守卫缺常驻坏样本自检记为顺延项（沿 `TestCommentIdentifierScannerCatchesKnownBadSamples` 先例抽纯函数内核 + 内嵌样本），不阻塞本 change — 当前断言逻辑简单恒真风险低 — 顺延项誊入 proposal「延期与放弃」由归档收口。
  - 任务组 6 评审：PASS（守卫鉴别力经静态评估+grep 交叉验证三条边界独立 fail-closed；无误伤面（client 整树禁难度是 spec 成文化）；抽查 #5/#8 诚实；1 minor 即坏样本自检顺延、2 nit 留档、INFO 勾选本条清偿）。
- 2026-09-11 整分支终审：FINAL PASS（八项清单全过：范围冻结 102 文件全部可归因、契约六组锚点交叉复核、版本矩阵仅 metadata v6 一项且 network/engine/contracts 零 diff、架构边界单点装配、测试组织单主题同构、注释纪律零任务编号、ledger 完整、回退路径与实现一致）。1 MINOR（proposal Impact 枚举漏列 `packages/server/sim/runtime` 与 `packages/client`）由控制会话当场补齐；2 INFO 留档（候选预算半句结构保证、v5 golden 往返断言必要收窄）。全量门禁（gates.sh + make rust-check）由控制会话并行执行，结果并入本 ledger 后方可合入。
