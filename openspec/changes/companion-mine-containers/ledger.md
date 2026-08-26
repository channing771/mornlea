# Ledger: companion-mine-containers

执行进度、评审结论与裁决记录。格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。

## 2026-08-25 认领与内容确认

- 认领：C-01 伙伴采掘容器/多掉落方块，认领人 `zcode4-implementer @ feat/C-01-companion-mine-containers`，docs-only 提交 `dc0dbce9` 已推送 main；Discussion #71 状态评论 `DC_kwDOToJS8M4BFPRB` + 正文刷新。独占文件集见 backlog 行备注。
- 分类：bounded（复用 `PrepareDropBatch` 先例与 `TaskFailInventoryFull` 既有路径，扩展 `mine` 语义，无新子系统）。
- Ruling: 容器采掘容量语义取方案 A（全或无）— 需求方在「全或无」与「对齐玩家先例（部分结算掉世界）」间显式裁决为 A；玩家路径是产物全部掉落为世界掉落物，而伙伴既有框架是产物直入背包 + 容量失败稳定态，方案 A 是该框架从单件到批量的自然推广，且伙伴无世界拾取能力（C-02 未交付）使掉世界等于销毁内容物 — 无（裁决而非纠错）。
- Ruling: 多掉落目标集合显式枚举为 {箱子, 熔炉}，不引入编号层泛化判据 — 成熟小麦的第二份产物编号层面读不出，「巧合性安全不成立」既有论证成立；泛化需改 `core.BlockDrop` 形状并波及全部消费者（Ruling 5 已否决） — 无。
- Ruling: 农业十编号拒绝保持不变 — 种什么/何时收/成熟度判断属 C-11 未裁决语义，本 change 范围冻结排除 — 无。
- 短设计获需求方显式批准（2026-08-25，对话内 approval），结论已写入本 change 的 proposal/design 与各任务 brief。

## 任务执行

（按 Task 1..4 逐条追加 implementer 派发、评审结论、修复轮次与 Ruling。）

### Task 1（2026-08-25，implementer zcode4 @ feat/C-01-companion-mine-containers）

- **TDD red**：先新增 `internal/sim/companion_mining_container_test.go`（单主题：伙伴容器采掘批量全或无），实现前运行确认失败——`TestCompanionMineableBlockContainerTargets` 报 `companionMineableBlock(ChestID) = false`；`TestCompanionMiningChestBatchIsAtomic` / `TestCompanionMiningFurnaceBatchIsAtomic` 报完成 tick 方块仍为箱子/熔炉（11/9）、无容量场景进度被清零而非保持满格。
- **实现**（提交 `7a1d6cbc`）：`internal/sim/mining.go`——`companionMineableBlock` 删除容器拒绝分支（农业显式拒绝原样保留，GoDoc 改述为容器批量全或无语义）；`completeCompanionMining` 路由容器目标到新分叉 `completeCompanionContainerMining`（经 chunk record 的 `ChestAt`/`Chest`/`FurnaceAt`/`Furnace` 读取容器记录，对齐玩家路径；预演通过后同一权威 tick 内 `SetBlock(air)` + `DeactivateChest`/`DeactivateFurnace` + 背包提交副本 + `consumeToolDurability` + `recordChange`；区块失效/槽缺失/方块已移除时清零进度不结算）；导出纯函数 `CompanionMineContainerStaging`（D3：产物集合构造 + 本体在前/槽位序固定序 + 背包副本逐堆 `AddStack` 全或无预演，供 Task 3 Runner 复用）。
- **测试取舍**：`companion_mining_test.go` 的 `TestCompanionMiningRejectsContainerTargets` 锁定的是本 change 显式废除的旧行为（与 delta spec 场景 3/4/5 直接矛盾），随本任务删除并由新文件的三组用例取代；`go test -list` 集合语义变化为「删 1 增 3」，待 Task 4.2 整分支核对。
- **验证**（数值只记录）：`go test ./internal/sim -race -count=1` → ok（16.272s，复跑 15.405s）；`gofmt -l internal/sim` → 无输出；`go vet ./internal/sim` → 通过；附 `go test ./internal/archcheck -count=1` → ok（7.875s，注释标识符门禁通过）。侧证：`go test ./internal/companion -count=1` → ok（2.347s）；`go test ./internal/server -short -count=1` → ok（28.323s，此前一轮与基线各出现过 `TestAuthoritativeMiningMemoryLifecycle` 等 sandbox 计时型偶发失败，与本变更无关）。

### Task 2（2026-08-25，implementer zcode4 @ feat/C-01-companion-mine-containers）

- **TDD red**：先改测试后实现——新增 `internal/companion/plan_types_containers_test.go`（单主题：计划契约层容器目标放行，对齐 Task 1 的 sim 侧文件先例），并把 `planner_test.go` 锁定旧拒绝行为的断言替换为放行断言，实现前运行确认三处失败：`TestPlanMineableBlockAllowsContainerTargets` 报 `planMineableBlock(9) = false`（ChestID）；`TestPlanDecodeKindMatrix` 的「合法 mine 容器目标（箱子）」报「mine 目标方块不可采掘（容器或无单一掉落）」；`TestPlannerSystemPromptHeadBytesStable` 报提示词头段字节漂移（旧文含「不能是箱子或熔炉」）。
- **测试取舍**：`planner_test.go` 的 `TestPlanDecodeKindMatrix` 中「mine 目标是箱子」「mine 目标是熔炉」两条 invalid 用例锁定的是本 change 显式废除的旧行为（与 D2 两侧清单同步放开直接矛盾），随本任务删除并由合法段新增的「合法 mine 容器目标（箱子/熔炉）」两条 valid 用例取代（`go test -list` 集合语义不变，仅子用例增删）；`TestPlannerSystemPromptHeadBytesStable` 的期望文本同步为新文案。
- **实现**：`internal/companion/plan_types.go`——`planMineableBlock` 删除容器拒绝分支（`core.BlockDrop` 对 ChestID/FurnaceID 本有单一产物登记，删除分支即自然放行，与 sim 侧 `companionMineableBlock` 同构），GoDoc 改述为容器批量全或无语义、删除「超出单一产物直入背包的结算形状」过时理由，农业显式拒绝原样保留；`PlanStepMine` 步骤注释同步。`internal/companion/planner.go`——mine 约束提示词改为「箱子与熔炉也允许」，解码路径拒绝理由文案从「（容器或无单一掉落）」改为「（农业方块或无单一掉落）」（无测试锁定该文案，grep 全仓核实）。
- **测试形态**：新用例以全集枚举遍历 `core.BlockID(0)..core.BlockIDMax`，按 `core.IsCrop`/`core.IsFarmland` 命中断言仍拒绝（带 farmingSeen==10 计数守卫防谓词空转）、两容器编号断言放行，另点验 `core.BedrockID` 仍拒、`core.StoneID` 仍放行（单一 `BlockDrop` 判据不变）；农业回归同时由既有 `TestPlanMineableBlockRejectsFarmingBlocks` 保持锁定。
- **验证**（数值只记录）：`go test ./internal/companion -race -count=1` → ok（4.969s）；`gofmt -l internal/companion` → 无输出；`go vet ./internal/companion` → 通过；附 `go test ./internal/server -short -count=1` → ok（29.812s，计划校验消费方无意外连锁）；`go test ./internal/archcheck -count=1` → ok（5.238s，注释标识符门禁通过）。

### Task 3（2026-08-25，implementer zcode4 @ feat/C-01-companion-mine-containers）

- **TDD red**：先新增 `internal/server/companion_interact_container_test.go`（单主题：Runner 满格饱和分支对容器目标的批量容量判定与 Memory/TCP parity），实现前运行确认失败——`TestCompanionManagerMineContainerBatchOverflowFailsInventoryFull` 报事件序列 `[Accepted, TaskStarted]`（旧单件预演放行了「本体放得下、批量放不下」的形态，900 tick 内无终态）；`TestCompanionManagerContainerMineMemoryTCPParity` 两传输均报「箱子采掘任务未以 TaskFailed 终结」。`TestCompanionManagerMineContainerCompletesWithLoot`（可容纳完成锁）实现前即绿——单件预演在批量 staging 成立时本就不误报，属行为锁而非 red 项。
- **实现**：`internal/server/companion_interact.go`——满格饱和分支的外层条件从 `observed.Harvestable && 进度满格` 收敛为「进度满格」，容量判定整体移入新方法 `companionMineCapacityExceeded`：容器（ChestID/FurnaceID）经 `sim.CompanionMineContainerStaging` 做同一产物集合与固定序的批量预演（容器内容物经新 helper `containerContentsAt` 从与 `blockAt` 同源的 `CloneReadyChunk` tick 边界深拷贝读取——区块 record 的箱子/熔炉槽就是 sim 完成分叉读取的同一权威容器状态，无需从 sim 新增任何只读暴露）；其余方块回落既有单件 `core.Inventory.AddStack` 预演，普通方块行为逐字节不变（既有 `TestCompanionManagerMineInventoryFullKeepsBlock` 继续锁定）。容器分支不以 harvestable 为门槛：错误工具（harvestable=false）下内容物放不下时 sim 同样拒绝结算、进度同样饱和，Runner 对该形态给出同一 `TaskFailInventoryFull`；普通方块在 harvestable=false 时 sim 完成分叉不做容量前验、饱和不可达，default 分支维持既有门槛即与旧行为逐字节等价。
- **parity 测试几何取舍**：失败任务刻意排在脚本末尾——走近目标途中伙伴可能借碰撞爬上方块顶沿（实测 Y=2、AABB 与箱子重叠约 4.5cm），其站立格没有寻路支撑，任何后续移动任务都会以 PathUnreachable 失败；这是与传输无关、与容器语义无关的既有几何事实，把它留在受测窗口外（脚本：空箱子成功回收 → 装着燃料/产物的熔炉批量放不下以 InventoryFull 终结）。熔炉内容物取「输入空 + 煤×3 + 铁锭×1」：输入空使熔炉完全暂停（`canSmelt` 要求有效输入），内容物跨 tick 不变，parity 断言才稳定。
- **Task 1 评审遗留项补强**（按 brief 并入本 Task）：`internal/sim/companion_mining_container_test.go` 追加 `TestCompanionMiningContainerWrongToolSkipsBody`（空手 harvestable=false：完成 tick 方块仍被破坏、槽停用、内容物入包、本体不入包——对齐玩家路径 `completeMining` 的可收获判定）与 `TestCompanionMiningContainerEdgeClearsProgressWithoutSettlement`（完成 tick 容器槽缺失：清零进度不结算、哨兵容器槽原样保留；完成前方块被移除：进度清零、容器槽不动、内容物零泄漏）——两者锁定 Task 1 已交付行为，属覆盖补强而非 TDD 特性，实现前即绿。顺手清理 Task 1 评审 minor：`internal/sim/mining.go` 容器相关注释裸提标识符处补反引号（`completeMining`/`SetBlock`/`DeactivateChest`/`ChestAt`/`PrepareDropBatch` 等），`companionChestTicks` 更名 `companionContainerTicks`（新增 `companionContainerBareHandTicks` 常量供错误工具档 30 tick 用例）。
- **验证**（数值只记录）：`go test ./internal/server -race -count=1` → ok（192.712s）；`go test ./internal/sim -race -count=1` → ok（18.911s）；`gofmt -l internal/server internal/sim` → 无输出（全仓 `gofmt -l .` 亦无输出）；`go vet ./internal/server ./internal/sim` → 通过（附 `go vet ./...` 全仓通过）；`go test ./internal/archcheck -count=1` → ok（12.408s，注释标识符门禁通过）；侧证：`go test ./internal/companion -race -count=1` → ok（4.372s）。

### Task 4（2026-08-26，implementer zcode4 接续承载 @ feat/C-01-companion-mine-containers，前任超时中断、未提交改动经本轮复核后收尾）

- **Task 3 评审五条 minor 清理复核**（前任已完成文件修改，本轮逐项核对全部成立、零顺手改动）：
  1. `internal/sim/mining.go` 两处裸提补反引号（`completeCompanionMining`、`completeCompanionContainerMining`）——完成；
  2. `internal/server/companion_interact.go` 注释裸提 `companionMineCapacityExceeded` 补反引号——完成；
  3. parity 测试函数头注释脚本顺序改写——旧文把脚本写反（「先放不下的箱子失败、后空熔炉完成」），改写为与 `runContainerMineParity` 实际脚本及体内注释一致的「先空箱子完成回收、后装料熔炉 InventoryFull 终结」——完成；
  4. `_, _, staged` 更名 `stagedOK`——消除与 `sim.CompanionMineContainerStaging` 第二返回值 `staged`（预演背包）的撞名——完成；
  5. helper 去重走参数化路线——删除 `newContainerMineParityHost`，`newInteractionParityHost` 增加 `inventory core.Inventory` 参数；逐字节核对两 helper 旧主体（存档种子构造、`hostTestConfig` 六项覆盖、`mustNewHost`、Cleanup）全同，仅背包来源参数化，两个调用点分别传 `pickaxeInventory()` / `containerTightInventory()` 等价传参，既有 interaction parity 语义零变化。
- **全量门禁**（真实执行，数值只记录）：
  - `go vet ./...` → 通过；`gofmt -l .` → 无输出；`go test ./internal/archcheck -count=1` → ok（10.949s，注释标识符门禁通过）；`openspec validate --all --strict --no-interactive` → 65 passed / 0 failed（退出码 0）。
  - `go test ./... -race -count=1` 聚合跑：28 包 ok；`cmd/mornlea` 包级 600s 默认超时 FAIL（602.086s）、`internal/server` 仅 `TestCompanionDialogueTerminalCoversFourTerminalStates/Stopped` 60s 等待超时 FAIL（273.183s）。复跑取证定性为**并行会话 CPU 争用导致的环境 flake，非本分支引入**，证据链：①本分支文件集（`git diff dc0dbce9..HEAD`，9 文件）零涉及 dialogue 接线与 `cmd/mornlea`，`companion_dialogue_wiring_test.go` 与 merge-base 同源（最后触碰 `25f69af3`，先于基线）；②失败时段 `ps` 实测 2–4 个并发 `mornlea.test` race 二进制、load 15–24（主 worktree 并行会话同时在跑整分支门禁）；③隔离复跑逐一转绿——`-run 'TestCompanionDialogueTerminalCoversFourTerminalStates'` → ok（3.295s）、`./cmd/mornlea -race -short` → ok（23.090s）、`./cmd/mornlea -race -count=1` 单包完整 → ok（477.814s）、`./internal/server -race -count=1` 单包完整 → ok（200.651s）——全部 30 包各有绿色 race 运行，两条聚合失败在无同类争用时均不复现；两次聚合失败期间 `go vet`/`archcheck` 等 CPU 门禁均正常通过。
  - 定点（清理改动后）：`go test ./internal/server -race -count=1 -run 'Parity|Container'` → ok（19.499s）；`go test ./internal/sim -race -count=1 -run 'CompanionMining'` → ok（6.657s）；`gofmt -l internal/server internal/sim` → 无输出；`go vet ./internal/server ./internal/sim` → 通过。
- **Scenario↔测试对照**（delta spec `specs/companion-world-actions/spec.md` 七场景逐条锚定）：
  1. 完整采掘链三方原子成立 → sim 既有 `TestCompanionMiningCompletionIsThreeWayAtomic` + server 既有 `TestCompanionManagerMineCompletesWithEventsAndLoot`（M5C 交付、本 change 保持）；
  2. 背包无空位不破坏方块 → sim 既有 `TestCompanionMiningInventoryFullKeepsBlockAndSaturatesProgress` + server 既有 `TestCompanionManagerMineInventoryFullKeepsBlock`（普通方块路径不变锁定）；
  3. 采掘空箱子批量直入背包 → sim 新增 `TestCompanionMiningChestBatchIsAtomic`（子测试「空箱子同tick原子回收」）+ server 新增 `TestCompanionManagerContainerMineMemoryTCPParity` 脚本指令一；可容纳扩展形态另由 `TestCompanionManagerMineContainerCompletesWithLoot`（装料箱子）锚定；
  4. 容器内容物放不下时全或无 → sim 新增 `TestCompanionMiningChestBatchIsAtomic`（子测试「内容物超出背包余量时全或无」）+ `TestCompanionMiningFurnaceBatchIsAtomic`（子测试「不可容纳时全或无」）+ server 新增 `TestCompanionManagerMineContainerBatchOverflowFailsInventoryFull` 与 parity 脚本指令二（TaskFailInventoryFull 稳定失败、方块/内容物/背包/耐久不变）；
  5. 采掘熔炉连带三格内容物 → sim 新增 `TestCompanionMiningFurnaceBatchIsAtomic`（子测试「本体与三格内容物一并入包」）；
  6. 农业方块仍然被拒绝 → sim 新增 `TestCompanionMineableBlockContainerTargets`（农业十编号显式拒绝）+ sim 既有 `TestCompanionMineableBlockRejectsFarmingBlocks` + companion 新增 `TestPlanMineableBlockAllowsContainerTargets`（全集枚举 + farmingSeen 守卫）+ companion 既有 `TestPlanMineableBlockRejectsFarmingBlocks`——「计划校验与权威模拟双重拦截」两侧各有新旧双锚；
  7. 目标变化令进度失效 → sim 既有 `TestCompanionMiningTargetReplacedInvalidatesProgress` + sim 新增 `TestCompanionMiningContainerEdgeClearsProgressWithoutSettlement`（子测试「完成前方块被移除清零进度不结算」，容器形态）+ server 既有 `TestCompanionManagerMineTargetReplacedFailsWorldChanged`。
  - Requirement 级附加锚（无独立 Scenario）：harvestable 门控 → `TestCompanionMiningContainerWrongToolSkipsBody`；容器完成 tick 槽缺失边缘 → 同文件子测试「完成tick容器槽缺失清零进度不结算」；Planner 消费面 → `TestPlanDecodeKindMatrix`（子用例级 invalid→valid 替换）与 `TestPlannerSystemPromptHeadBytesStable`（提示词头段字节）。
- **`go test -list` 集合语义核对**：基线取 merge-base `dc0dbce9`（本地 `main` 已分叉含 B-05/B-14/C-08 等他 change，非本分支可比基线；经独立临时 worktree 真实运行，Rust 库以符号链接复用本分支 `engine/target`），两侧各跑 `go test -list '.*' ./internal/sim ./internal/companion ./internal/server`（过滤包状态行后 931 vs 923，净 +8）：**删 1** —— `TestCompanionMiningRejectsContainerTargets`（Task 1，锁定本 change 废除的旧行为）；**增 9** —— Task 1 三项（`TestCompanionMineableBlockContainerTargets`、`TestCompanionMiningChestBatchIsAtomic`、`TestCompanionMiningFurnaceBatchIsAtomic`）、Task 3 补强两项（`TestCompanionMiningContainerWrongToolSkipsBody`、`TestCompanionMiningContainerEdgeClearsProgressWithoutSettlement`）、Task 2 一项（`TestPlanMineableBlockAllowsContainerTargets`）、Task 3 三项（`TestCompanionManagerMineContainerCompletesWithLoot`、`TestCompanionManagerMineContainerBatchOverflowFailsInventoryFull`、`TestCompanionManagerContainerMineMemoryTCPParity`）；与 ledger 各任务记录精确吻合（Task 1「删 1 增 3」、Task 2/3 纯新增）。Task 4 清理只删 helper 函数（非测试函数），集合零扰动。
- tasks.md 4.1/4.2 勾选正当性：前任先行勾选，本段全量门禁记录即真实通过之追认（勾选时点超前、内容成立）。

## Task 1 评审（2026-08-25）

- SPEC 合规评审：**PASS**。逐项核对 delta spec sim 侧半边（谓词放开、农业回归、副本预演/固定序/全或无、四方不变+满格稳定、同 tick 五件套、`harvestable` 门控、`RejectNoTarget` 对齐、`core` 零变更、玩家路径不变、预演函数导出、旧测试删除正当）全部成立；独立复跑定点与全包 race、gofmt、vet、archcheck 均绿。两条非阻断观察：错误工具（`harvestable=false`）容器路径无直接用例；完成 tick 容器边缘（record 失效/槽缺失/`SetBlock` !changed）无容器专属用例。
- QUALITY 质量评审：**PASS**。设计取舍（路由分叉零侵入、纯函数签名最小、D7-4 两条分支不合并且注释留痕）、并发边界（值语义副本、单写者 tick、固定序无 map 序依赖、容器访问同源）、测试锋利度（逐堆落位/顺序断言、四方逐项核对、全复用既有 helper）全部通过。三条 minor findings：新注释裸提标识符未反引号；`companionChestTicks` 命名只提箱子实覆盖熔炉；staging 对畸形堆（`Count==0` 已注册物品）静默放行（生产不可达）。
- Ruling: 三条 minor 均不构成修复循环 — 反引号与常量名属风格债且门禁不拦（评审建议下次触碰顺手清）；畸形堆路径生产不可达（容器写入全经 `Valid()` 门禁），加固属镀金 — 修复循环只留给行为契约与合入质量阻断项，本任务零 blocker/major，进入 Task 2。
- Ruling: 两条覆盖观察并入 Task 3 — 错误工具容器用例与 Runner 接线同域（`Harvestable` 信号两侧共享）；完成 tick 容器边缘用例随 Task 3 补强一并落 — 覆盖缺口不阻断但须在收尾前闭合。

## Task 2 修复轮 R1（2026-08-25，implementer zcode4 @ feat/C-01-companion-mine-containers）

- 背景：Task 2 QUALITY 评审 **FAIL**，两条 findings 均为注释级（major 1 / minor 1）；本轮只改注释，不动代码逻辑与测试断言。执行记录，无 Ruling。
- Finding 1（major，`internal/companion/planner.go` 的 `validatePlanStepsAgainstSnapshot` GoDoc）：残留旧判据「方块必须满足单一掉落与非容器」，「非容器」与 `planMineableBlock` 已放行 `core.ChestID`/`core.FurnaceID` 直接矛盾。修复：该句改写为「方块必须满足 `planMineableBlock` 判据——非农业且具单一 `core.BlockDrop`，箱子与熔炉作为容器目标放行」，与 `internal/companion/plan_types.go` 现判据（农业显式拒绝 + `core.BlockDrop` 存在性，两容器经单一掉落登记自然放行）逐点核对一致，改动前后「恰好列入 ExposedBlocks」限定与其余子句零变化。
- Finding 2（minor，`internal/companion/planner_test.go` 容器目标用例前的注释）：裸提 `companionMineableBlock` 未反引号。修复：补反引号。
- 验证（数值只记录）：`go test ./internal/companion -race -count=1` → ok（5.383s）；`gofmt -l internal/companion` → 无输出；`go vet ./internal/companion` → 通过；`go test ./internal/archcheck -count=1` → ok（5.694s，注释标识符门禁通过）。

## Task 2 评审与修复（2026-08-25）

- SPEC 合规评审：**PASS**。两侧防御清单逐字同构（删除拒绝分支而非白名单，`core.BlockDrop` 存在性判据天然放行容器）、农业双保险（新全集枚举 + `farmingSeen==10` 计数守卫、既有具名回归）、提示词/解码文案/字节稳定测试三处消费面同步、invalid→valid 等价替换无门禁净放宽、文件集零溢出。
- QUALITY 质量评审：**FAIL**（1 major + 1 minor）。major：`planner.go` `validatePlanStepsAgainstSnapshot` GoDoc 残留「单一掉落与非容器」旧判据，与同函数已改的错误文案三行之隔自相矛盾；minor：`planner_test.go:758` 裸提 `companionMineableBlock` 无反引号（Task 1 minor 同类重复）。
- 修复轮 R1（原 implementer 已终结，由新 implementer 承载同一 brief）：两处注释改写闭合，提交 `d4f8a49a`。
- R1 复核：**PASS**。新 GoDoc 与实判据逐点一致、全包 grep 零「非容器」残留、零行为改动、复跑全绿。
- Ruling: Task 2 修复循环 1 轮收口 — major 是注释级失真而非行为缺陷，两行修复即达合入质量 — 教训：改语义时同一函数内错误文案与 GoDoc 必须同轮改写，注释失真机械门禁不覆盖、只能靠评审抓。

## Task 3 评审（2026-08-25）

- SPEC 合规评审：**PASS**。偏离 1（饱和分支外层条件收敛）经逐形态推演证明普通方块行为逐字节不变（harvestable=false 的普通方块饱和在 sim 侧不可达，default 分支 `!harvestable` 早退与旧外层短路等价）；D3「同一预演」结构性成立（Runner 与 sim 共用 `CompanionMineContainerStaging`，contents 装配逐字一致、`CloneReadyChunk` 深拷贝同源读取）；全或无失败链与真双传输 parity 断言到位；Task 1 遗留补强（错误工具/完成边缘）与玩家路径语义同构；文件集与依赖方向合规。1 条 minor：parity 测试函数头注释把脚本顺序写反（`companion_interact_container_test.go:369-372`）。
- QUALITY 质量评审：**PASS**（3 minor）。设计取舍与「同一预演」结构表达通过（零堆构造残留、深拷贝成本披露如实）；测试锋利（精确事件序列、六事件链、真双传输 `reflect.DeepEqual`）；回归面零溢出（纯新增 5 测试、更名零残留）。minor：`companion_interact.go:175` 裸提 `companionMineCapacityExceeded`（连续第三轮出现）；`mining.go:288-289、385-386` 两处容器相关裸提未在 4b 清偿范围内（清偿声明与事实不完全相符）；`newContainerMineParityHost` 与既有 helper 约 30 行重复。观察：`_, _, staged :=` 与第二返回值撞名。
- Ruling: 五条 minor 全部并入 Task 4 收尾清理，不构成修复循环 — 全部为注释债/命名噪点/测试构造重复，零行为影响且门禁不拦；裸标识符连续三轮出现，收尾清零避免带入整分支终审 — 修复循环只留行为契约阻断项。
- Ruling: helper 30 行重复的处置授权 Task 4 implementer 现场裁量 — 参数化推广若为纯机械改动（改参数签名+全引用点同步）则做，若牵动既有 parity 测试语义则不做并誊入「延期与放弃」— 复制重复是风格债，为去重在收尾期引入回归风险本末倒置。

## 整分支终审（2026-08-25）

- 裁决：**PASS（可合入）**。范围冻结（12 提交 15 文件全落独占集、基线文档/golden/版本号零触碰）、规格一致性（7 Scenario 锚定抽查属实、「延期与放弃」四条未越界）、正确性与上限（纯函数固定序、有界 ≤28 堆、无资源上限放宽）、并发/持久化错误路径（单写者 tick、深拷贝只读、容器槽 generation 语义无泄漏、storage 零变更）、wire 安全（`internal/network` 零变更、复用 v18 枚举）、无版权资源、`go test -list` 删 1 增 9 与记录精确吻合、8 条 Ruling 全在案。
- Findings：1 minor——proposal/design 把容器 UI 总格数 63 误当箱子存储格数（真实 `core.ChestSlots`=27、堆数上界 1+27=28）；行为边界比声明更紧、方向安全。已在归档前修正两文件共 5 处数字（63→27 语义、64→28 堆），delta spec 不含该数字故无主规格影响。
- 环境备注：Task 4 聚合 race 的两次超时有完整 flake 证据链（文件集无关、并发负载实测、隔离复跑 30 包全绿）；CI 全量门禁为最终兜底。

## 收尾门禁（2026-08-26）

- `scripts/agents/gates.sh` 全量六项 PASS：gofmt 无输出、`go vet ./...`、archcheck、OpenSpec strict 65/65、`make rust`（Rust 1.97.1）、全量 `go test ./... -race` 28 包 ok（`cmd/mornlea` 333.804s 通过）。
- 首轮 gates 的 race 失败（`cmd/mornlea` 包级超时 + overlay 单测失败）取证为并行会话同机跑全量 race 的负载 flake：失败时段实测另一 `go test ./... -race` 进程（load 15、`mornlea.test` 140% CPU），overlay 测试隔离 `-count=3` 全绿、GPU 测试安静机器 63.52s 通过（超时时为 10 分钟未完成）；本分支文件集零涉及 `cmd/mornlea`。CI 全量门禁为最终兜底。
