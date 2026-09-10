# B-34 生存初始背包留空（tasks，如实勾选）

> 按 `subagent-driven-development` 执行：一任务一实现 + SPEC/QUALITY 双评审，任务 brief 是唯一需求来源；ledger 见工作区 `.superpowers/sdd/tasks/survival-empty-start.md`。测试与对应实现放在同一任务组，先写失败测试。

## Task 1: 缺失玩家初始背包为空

- [ ] `packages/server/server/persistence/players_snapshot.go`：删除 `starterMaterialItems` 与 `starterMaterialInventory()`；`newMissingCachedPlayer` 不再设置 `Inventory`（零值即全部空槽），并以注释说明「空初始背包是契约，不是缺省」；`assets` 或任何其它文件的 `core.ItemID` 引用不受影响
- [ ] `packages/server/server/persistence/player_persistence_lifecycle_test.go`：重写 `wantStarterMaterialInventory()`（或其替代）为「全部空槽」的期望值；`TestPlayerPersistencePrepareMissingProvidesStarterMaterialInventory`、`TestPlayerPersistenceMissingStarterDoesNotPersistBeforeConfirm`、`TestPlayerPersistenceConfirmPersistsStarterMaterialInventoryOnce`、`TestPlayerPersistencePrepareMissingGrantsNoStarterWheatSeeds` 的断言改为 36 格全空 + 快捷栏为空，测试名与注释随语义更新
- [ ] 同文件：`TestPlayerPersistenceExistingPlayerKeepsLegacyStarterSeeds` 的「升级前材料包」夹具改为**就地写出的 14 组整栈字面量**，不得再从期望 helper 派生（自我指涉的夹具会在实现变空时静默失去覆盖）
- [ ] 验证：`go test ./packages/server/server/persistence -race -count=1`

## Task 2: 空背包下的登录就绪判定

- [ ] `packages/server/server/farming_loop_e2e_test.go`：登录就绪判定不再以 `message.Inventory != core.Inventory{}` 为信号，改为「收到携带权威背包的玩家状态消息」本身；相关注释与字段说明（`InventoryAfterLogin 是登录即得的完整权威背包：材料包 14 叠…`）随语义更新，登录后断言改为 36 格全空
- [ ] `packages/server/server/hunger_loop_e2e_test.go`：同一处判定与阶段命名/注释同步更新（该脚本第 1 步的语义从「拿到材料包」变为「拿到空背包与三层饥饿初值」）
- [ ] `packages/server/server/eating_parity_test.go`：仅更新已失效的注释（材料包概念消失），不改任何断言
- [ ] 不得放宽 `integrationLoginTickBudget` 或任何等待预算；脚本必须能以空背包正常闩锁
- [ ] 验证：`go test ./packages/server/server -run 'TestFarmingLoopEndToEndMemory|TestHungerLoopEndToEndMemory|TestEatingParity' -race -count=1`，并确认 TCP parity 共用 runner（`transport_parity_integration_test.go` 的 `TestNaturalSeedFarmingMemoryTCPParity`）在同一任务内一并跑通：`go test ./packages/server/server -run TestNaturalSeedFarmingMemoryTCPParity -race -count=1`

## Task 3: 熔炉与石剑配方改用可徒手取得的石料

- [ ] `packages/shared/core/recipe.go`：`RecipeFurnace` 的 3×3 圆环与 `RecipeStoneSword` 的纵向 2 格由 `ItemCobblestone` 改为 `ItemStone`；注释说明「原料必须具有自然来源，圆石不参与世界生成」
- [ ] `packages/shared/core` 配方测试：固定形状断言随原料更新；补一条断言「熔炉与石剑的每格原料都具有自然来源」或由 Task 4 的守卫承接（二者择一，不得两处重复实现同一判据）
- [ ] `packages/server/sim/entity`、`packages/server/sim/runtime` 与 `packages/server/server` 中凡是**用圆石摆出熔炉/石剑形状**的合成测试夹具改为石料；凡是**断言配方原料**的测试同步更新
- [ ] 客户端配方书与图标呈现：确认原料变化不需要新增材质层或图标（`packages/client/assets`、`packages/client/cmd/mornlea/app/app_game_ui.go` 的配方书数据）；若配方书硬编码了原料物品，同步为石料
- [ ] 验证：`go test ./packages/shared/core ./packages/server/sim/... -race -count=1`、`make dev-check`

## Task 4: 价值闭包守卫

- [ ] 新建 `packages/server/sim/entity` 包内测试（例如 `reachability_closure_test.go`）：以带溯源注释的自然来源方块常量为起点（逐项注明 worldgen 材料表 / 矿石规则 / 树木规则 / 短草规则），用真实 `miningRule` 建模工具门槛、`core.BlockDrop`、`core.Recipe`、`core.SmeltingOutput` 与工具损坏映射求不动点
- [ ] 正向断言：工作台、火把、至少一种镐、面包或熟肉在闭包内；熔炉、铁锭、空桶也在闭包内
- [ ] 反向断言：物品注册表中不在闭包内的物品集合**恰好**等于显式声明的已接受不可获得集合（圆石、平滑石、苔藓圆石、白色羊毛、红色瓦块、马铃薯、胡萝卜、毒马铃薯），集合变化必须由人显式确认
- [ ] 测试须自证非空洞：断言自然来源常量非空、闭包非空、且已接受集合中的每个物品确实不在闭包内
- [ ] 验证：`go test ./packages/server/sim/entity -run TestReachability -race -count=1`、`go test ./packages/audit -count=1`

## Task 5: 空背包起家端到端冒烟

- [ ] `packages/server/server` 新增脚本化冒烟（例如 `survival_bootstrap_e2e_test.go`）：真实缺省世界生成 + Memory transport，从空初始背包出发，只用手动采掘与合成推进「原木 → 木板 → 工作台 → 放置工作台 → 3×3 网格 → 石镐」，逐步断言权威背包与世界状态
- [ ] 脚本不得用 `SetPlayerInventoryForTest` 或任何直给物品的后门代替其中任何一步；为在有限预算内确定性推进，允许把玩家置于缺省种子的确定性坐标，但不得用夹具改写方块
- [ ] 验证：`go test ./packages/server/server -run TestSurvivalBootstrap -race -count=1`

## Task 6: 跨模块源码守卫改指新契约

- [ ] `packages/shared/core/sapling_acquisition_test.go`：第三条腿从解析 `starterMaterialItems` 改为断言缺失玩家初始背包的构造处不出现树苗标识符；守卫找不到目标时仍必须失败（不得静默失效），测试名与注释随语义更新
- [ ] 验证：`go test ./packages/shared/core -run TestSaplingHasNoCraftingSmeltingOrStarterKitSource -count=1`

## Task 7: 收尾（规格、文档、版本矩阵与全量门禁）

- [ ] 应用/核对三份 delta 与主规格一致：`common-block-materials`、`authoritative-crafting`、`oak-sapling-regrowth`（归档阶段执行 sync，本任务只核验 delta 与实现一致）
- [ ] `docs/notes/gameplay.md`、`docs/notes/limitations.md`、`README.md`：材料包描述改为「新玩家初始背包为空」，并补记四条关键能力由自然路径取得；`limitations.md` 显式记名五种装饰材料与马铃薯/胡萝卜不可自然获得
- [ ] `docs/feature-backlog.md`：B-34 行状态与交付说明如实填写；新增后续行（装饰材料自然获取路径；马铃薯/胡萝卜种子来源）并注明来源
- [ ] `docs/notes/progress.md`：追加本 change 的实现编年史条目
- [ ] 版本矩阵核验：确认协议 v40、玩家 schema v8、区块 schema v9、metadata v5、`companions.ai` v5、`hostile_mobs` v1、`passive_mobs` v1、engine ABI v11、client ABI v18、scenario v23 全部未变，并给出实际核验命令输出（`go test ./packages/audit -count=1`）
- [ ] 六模块 `gofmt`、`go vet`、全量 race 与严格校验：`make dev-check`、`make test-race`、`openspec validate --all --strict --no-interactive`
