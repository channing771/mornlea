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
