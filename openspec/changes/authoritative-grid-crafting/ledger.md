# Ledger — authoritative-grid-crafting（A-01）

## 2026-08-25 认领与规划

- 18:31 `zcode-implementer` 认领 `docs/feature-backlog.md` A-01 行（main 提交 `540bbe17`），分支 `feat/A-01-authoritative-grid-crafting`，worktree `.worktrees/A-01-grid-crafting` 自 `main`（`16ac2fe7`）创建。
- 阶段 0.5 内容确认：分类 architectural（跨 core/sim/network/server/client/HUD 的批次基础功能），设计蓝本为批次设计文档与 `docs/superpowers/plans/2026-08-23-authoritative-grid-crafting.md`；用户于 2026-08-25 显式批准开工（「确认」）。
- 本 change 规划产物（proposal / 5 份 delta specs / design / tasks）在分支内创建，独立执行模式（crafting 计划 Task 1 的「独立执行本计划时才在本分支创建 Task 1 产物」分支）。

## Rulings

- Ruling: A-01 在无共享契约的重置批次下按 append-only 段独立执行 — 三条功能线（A-01/A-02/A-04）已各自从 `main` fork，任何一线强制他人 rebase 新契约 SHA 都违反「已认领行不得抢」；批次计划已规定合并序锁定终值 — 原共享契约提交随机器迁移丢失是根因，重置裁决「从当前 main 重做」已接受该事实。
- Ruling: in-branch 消息临时编号 C→S `MoveCraftingStack=14`、`TakeCraftingOutput=15`、S→C `CraftingState=21`，协议版本保持 v26 — 批次终值 `MoveCraftingStack=7` 依赖 `network.CraftRecipe` 删除（A-06）与 v27 升版（A-07），均为集成分支独占；分支内同构建客户端/服务端一致 — 不这么做会在功能分支触碰版本基线（集成任务独占文件集）。
- Ruling: 本分支只登记 recipe `1..13` 与 `ItemStick=37`/`ItemWorkbench=38`/`WorkbenchID=45` — recipe `14..18` 的产物（火把/剑/床）物品归 A-02/A-03/A-05 的 append-only 段，A-03 认领行已注明「recipe 15..17 依赖 A-01 合流」；backlog A-01 行的「七条新配方」描述批次终态，跨线实现 — 若在本分支预登记他人物品会在合并时产生重复编号冲突。
- Ruling: `CraftRecipe` 保留线上注册与 codec、ingress 稳定拒绝 — 类型删除与编号 7 释放归 A-06（backlog A-06 行明示「删除 network.CraftRecipe 过渡类型」）；保留注册让 fuzz/round-trip 在过渡期继续全覆盖。
- Ruling: `container-ui-presentation` 的「最大打开态 266 quad」改为「布局边界测试锁定精确值且 ≤267」 — 十条配方行被网格+产物格替换后精确最坏组合必然变化，功能分支不能改 `AGENTS.md` 基线表述（A-07 独占），精确值必须仍由测试锁定而非漂移。

## 基线证据

- 2026-08-25 worktree `main`（`16ac2fe7`）基线：`make rust` 退出码 0（全量 cdylib 构建）；`internal/core`、`internal/assets`、`internal/physics` 于任务组 1 开工前处于已验证的干净绿态（任务组 1 的红→绿流程以此为前提）。focused Go 全组基线随任务组 2 的 sim 接入一并记录（任务组 1 删除 `Inventory.Craft` 前全仓编译依赖 sim/server/hud 未受影响）。

## 评审与执行记录

### 任务组 1：形状注册表与工作台方块

- Implementer：fresh 子代理，提交 `615818b6`（`feat: add shaped crafting recipes`，17 文件）。验证：`go test ./internal/core ./internal/assets ./internal/physics -race -count=1` 全绿；`gofmt -l` 无输出；`go vet` 三包通过。
- 红→绿证据：`Test(RecipePattern|MatchCraftingGrid)`、`TestRecipeShapeTableOneToThirteenIsFrozen`、5 个 `TestConsumeRecipe*`、`TestWorkbench*` 全部先红后绿。
- 实现者偏离记录（1：编号注册前置——工具/新配方形状引用 `ItemStick`/`ItemWorkbench`，不前置无法编译；2：新增白盒 `recipe_shape_internal_test.go`——注册表内无「不对称且开镜像」形状，镜像正例只能对私有 `matchesPattern` 直接证明；3：`MatchCraftingGrid`/`ConsumeRecipe` 带 `size` 参数——2×2 个人格在 3×3 行主序下是 L 形，匹配器必须知道有效尺寸；4：`pack_test.go` 追加三行绑定；5：非工具配方 `Mirror=true`、工具关镜像，锄头方向锁定「材料列在左、木棍列在右」）。
- 已知中间态（预期）：`Inventory.Craft`/`CraftingRecipe` 删除使 `internal/sim/engine_step.go:252`、`internal/render/hud/container.go:297,313`、`cmd/mornlea/app_input.go:161` 编译失败，任务组 2/3/4 接续修复；`internal/network`/`internal/mesh`/`internal/world`/`internal/storage` 保持绿。
- 评审：QUALITY **PASS**（独立评审者，2026-08-25）：七项清单全过——匹配器零分配固定循环、Consume 七条失败路径原子、GoDoc/编号纪律达标、测试锋利（冻结表完整结构体相等 + 配位次断言）、一文件一主题、无越界、gofmt/vet/-race 亲测复核通过。非阻塞建议 4 条：1) `inventory.go:213` 注释与匹配层 `Count==0` 归一化语义相反，会误导「Match 通过则 Consume 必通过」假设（消费层更严，应改写注释或补锁定用例）；2) `MatchCraftingGrid` 两个防御分支（非法 size、个人格 4..8 残留）缺直接测试；3) `ConsumeRecipe` 在 `Mirror=false` 时冗余重跑正向尝试（无害不对称）；4) 可补通用注册表不变量测试（Cells 不越 Width×Height 子矩形），未来追加 14..18 自动生效。
- 评审：SPEC **FAIL → 进入修复循环 R1**（独立评审者，2026-08-25）：形状表/匹配语义/原子性/编号/拒绝/方块属性/无放宽七项全 PASS；唯一必改项 N1——`Inventory.Craft` 删除未给 `internal/sim/engine_step.go:252`、`internal/render/hud/container.go:297,313`、`cmd/mornlea/app_input.go:161` 留可编译过渡，全仓 `go build ./...`、`go vet ./...`、`internal/archcheck` 三重门禁在本提交失效；「已知中间态」披露不等于合规，任务组 2 的红→绿也无法在不可编译包上执行。
- Ruling: N1 立即修复而非接受中间态 — 门禁失效违反「每个任务可单独验证」的子代理纪律，且修复方向（sim 合成命令转稳定拒绝）正是 design.md D6 的目标语义 — 修复循环 R1（≤3 轮续用原 implementer）授权临时越界：仅限上述三个调用点及其测试的最小编译过渡，不得回加任何 `Craft` 双路径；`hud` 侧只去掉对已删符号的咨询，配方列表 UI 的正式替换归任务组 4。
- Ruling: SPEC 建议 1 采纳——`authoritative-grid-crafting` delta 的「匹配 MUST 允许水平镜像」与 D3 按配方 `Mirror` 位存在文本张力（注册表内无可实例化「不对称且三列」形状），控制会话已把 MUST 改写为「镜像等价性由每条配方的镜像位声明」，避免归档时沉淀被冻结行为直接违反的主规格。SPEC 建议 2 采纳——`WorkbenchID` 采掘规则缺失登记进 tasks.md 2.4（木 tier 15 tick）。建议 3/4 与 QUALITY 建议 1/2 合并进 R1 修复（core 同包文件）；QUALITY 建议 4 一并进 R1。
- R1 执行（原 implementer，提交见 git log `fix: restore tree-wide gates after shaped recipe cutover`）：N1 三处过渡——`CommandCraftRecipe` 分支改为「未就绪照旧 `RejectPlayerNotReady`、就绪一律 `RejectInvalidInput`」的稳定拒绝（状态零变化），新建 `internal/sim/crafting_test.go` 以面包夹具锁定拒绝语义；hud `appendRecipeRows` 去掉聚合输入与可合成性咨询（输入图标改用形状代表材料，按钮恒中性色）；cmd 去掉本地 Craft 预检（只保留确认镜像门）。顺带修复全数落地：消费层「更严」注释与 GoDoc 补「形状空格残留栈」、`MatchCraftingGrid` 两个防御分支直接测试（`TestMatchCraftingGridRejectsInvalidSize`/`RejectsResidueBeyondEffectiveSize`）、`ConsumeRecipe` 失败表补「形状空洞被占」、注册表 Cells 不越子矩形不变量测试（`TestRegisteredRecipeCellsStayInsideShapeBounds`，附无空洞计数断言）、`ConsumeRecipe` 在 `Mirror=false` 时单次正向尝试、工作台三层「不在植物区间」断言。验证：`go build ./...`、`go vet ./...`、`go test ./internal/archcheck -count=1`（含 `TestMornleaCurrentIdentity`）、`go test ./internal/core ./internal/assets ./internal/physics ./internal/sim -race -count=1`、`go test ./internal/render/hud -race -count=1` 全绿；触碰包 `gofmt -l` 无输出。已知红（越界范围、归任务组 2/3）：`internal/server` 六个依赖合成命令执行的 e2e（eating/farming/hunger/material-processing/crafting-restart 的 Memory/TCP 循环）因 recipe-click 稳定拒绝而失败，网格路径接入后恢复；`TestAuthoritativePlayer*` 两个 goroutine 基线用例在并行重载下抖动、隔离重跑通过（与本变更无关）。测试过渡调整：sim 删除 `TestCraftPublishesOnceAndConsumesInputs`/`TestCraftRejectsWithoutChangingInventory`/`TestCraftBreadFromWheatViaCommand`（成功路径已不存在，拒绝语义由 crafting_test.go 接管），`TestCraftIgnoresStaleSequence` 改为拒绝下的序列去重；hud 输入列改代表材料（熔炉→圆石、箱子→木板）并以「按钮恒中性色」替换可合成性断言；cmd `TestUnavailableCraftRecipeClickDoesNothing` 改写为 `TestCraftRecipeClickSendsWithoutLocalPredict`（空/满背包同样发送）、音频用例期望 5→6 cue（空背包配方点击现为有效发送）。
- Ruling: R1 遗留的 `internal/server` 六个 e2e 红接受为**有主中间态**，不追加 R1 轮次 — 六个失败全部是「e2e 以 recipe-click 命令驱动合成执行」与 D6 稳定拒绝的正确冲突，其修复不是过渡补丁而是把测试驱动迁移到网格路径（`MoveCraftingStack`/`TakeCraftingOutput`），这正是任务组 3.2 的 parity 工作内容（`transport_parity_integration_test.go` 本就在任务组 3 文件集内）；在 R1 越界授权内顺手改它们会扩大越界范围。任务组 3 brief 须显式包含「迁移六个 e2e 到网格驱动并恢复绿」；任务组 5 终审以全仓 `-race` 全绿为门。
- R1 复核（原 SPEC 评审者定向复核，worktree HEAD `40d73807`）：**N1 已解除，任务组 1 SPEC 改判 PASS**。亲测复跑全部门禁命令全绿；三处过渡最小、无 `Craft` 双路径回加、hud 未越权预支任务组 4 的 UI 替换（266 quad 容量门原样通过）；顺带修复抽查 6 条全落地；镜像位措辞修订（`ddde0885`）张力消除；server 六 e2e 红接受为 N1 范围外独立债务（红测试≠门禁失效，根因确证为 recipe-click 驱动，归属任务组 3）。**任务组 1 关闭（双评审 PASS）**。
- 已知并行负载抖动清单（隔离重跑均秒级通过，与本变更无语义关联，任务组 5 终审留意）：`TestAuthoritativePlayerConvergesAfterThreeTickStateDelay`、`TestAuthoritativePlayer` 系 Replay 确定性用例、`TestPersistentShutdownReturnsAllGoroutinesWithinSharedDeadline`（后者由 SPEC 复核者补记）。

### 任务组 2：玩家瞬态网格与回收不变量

- Implementer：fresh 子代理，提交 `d0f292bb`（`feat: make crafting grid authoritative`，10 文件，全部 `internal/sim/**`）。验证：`go test ./internal/sim -race -count=1`、`go test ./internal/core ./internal/sim -race -count=1` 全绿；`go vet`/`gofmt -l`/`archcheck`/`go build ./...` 通过；server 8 红与既有记录一致（6 e2e 归任务组 3 + 2 并行抖动）。
- 红→绿证据：2.1 移动/取出 12 用例；2.2 回收不变量 5 用例（含 tick 末性质测试与独立 oracle 复算）+ 自引入缺陷修复（`repackCraftingSlots` 双重计数改与 `canRepackCrafting` 逐字同序原子提交）；2.3 生命周期 8 用例（关闭/走远/被挖同 tick、断线快照前回收、死亡先回收再清空、不可能回收 panic 内部错误路径）；2.4 工作台 4 用例（无容器引用、15 tick 采掘恰掉 1、双人独立）。
- 实现者偏离（待评审裁决）：1) 断线回收挂钩 `player.go` 的 `UnregisterSession`（`persistence.go` 只管区块持久化，取快照前回收即满足「断线持久化之前」）；2) 跨容器移动（箱/炉→背包）也加不变量守卫（spec 五入口未列，但 tick 末不变量是无条件 SHALL 且为真实可达破坏路径）；3) 采掘/作物掉落收口在拾取层（玩家采掘产物本就只生成世界掉落，入包唯一通道是拾取），初始材料包在 `RegisterPlayer` 时网格恒空无需运行时守卫；4) 「工作台被挖」用例自挖（双人同点位触发 v25 近战抑制采掘）；5) 2.3 断线用例归 `crafting_test.go`（一文件一主题）。
- 给任务组 3 的接口：`CommandMoveCraftingStack`（Slot=from/ToSlot=to）、`CommandTakeCraftingOutput`（只带 Sequence）、`TickResult.Craftings []CraftingUpdate`（`publishCraftings` 在 `publishInventories` 后、Active+dirty、latest-wins、发布即清、注册初始为真）、`Engine.PlayerCrafting` 只读访问器、`CraftingGrid{Size, Slots[9]}` 不入快照/存档/哈希。
- 评审：待双评审记录。
