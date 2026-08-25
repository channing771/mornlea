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
