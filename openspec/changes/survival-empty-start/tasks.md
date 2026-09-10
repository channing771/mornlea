# B-34 生存初始背包留空（tasks，如实勾选）

> 按 `subagent-driven-development` 执行：一任务一实现 + SPEC/QUALITY 双评审，任务 brief 是唯一需求来源；ledger 见工作区 `.superpowers/sdd/tasks/survival-empty-start.md`。测试与对应实现放在同一任务组，先写失败测试。

## Task 1: 缺失玩家初始背包为空

- [x] `packages/server/server/persistence/players_snapshot.go`：删除 `starterMaterialItems` 与 `starterMaterialInventory()`；`newMissingCachedPlayer` 不再设置 `Inventory`（零值即全部空槽），并以注释说明「空初始背包是契约，不是缺省」
- [x] `packages/server/server/persistence/player_persistence_lifecycle_test.go`：期望 helper 改为「36 格全空」，四个断言材料包的测试改名并改为全空断言（提交 `0792b00b`）
- [x] 同文件：`TestPlayerPersistenceExistingPlayerKeepsLegacyStarterSeeds` 的「升级前材料包」夹具改为就地写出的 14 组整栈字面量（提交 `0792b00b`）
- [x] 修复轮：注释锚点挪出复合字面量内部、去重断言、补 `Inventory.Valid()` 断言（提交 `cd8c4800`）
- [x] 验证：`go test ./packages/server/server/persistence -race -count=1` 全绿

## Task 2: 空背包下的登录就绪判定

- [x] `packages/server/server/farming_loop_e2e_test.go`：登录就绪改为「收到携带权威背包的玩家状态消息」本身（`inventoryPublished`），登录后断言改为 36 格全空扫描
- [x] `packages/server/server/hunger_loop_e2e_test.go`：同一处判定与阶段命名/注释同步更新
- [x] `packages/server/server/eating_parity_test.go`：仅更新失效注释（提交 `0135497c`）
- [x] 未放宽任何等待预算（`integrationLoginTickBudget` 仍为 3000），未新增 sleep
- [x] 验证：`go test ./packages/server/server -run 'TestFarmingLoopEndToEndMemory|TestHungerLoopEndToEndMemory|TestMemoryTCPEatingConvergence' -race -count=1`、`go test ./packages/server/server -run TestNaturalSeedFarmingMemoryTCPParity -race -count=1` 全绿（eating parity 的真实测试名是 `TestMemoryTCPEatingConvergence`）

## Task 3: 熔炉与石剑配方改用可徒手取得的石料

- [x] `packages/shared/core/recipe.go`：`RecipeFurnace` 与 `RecipeStoneSword` 的原料 `ItemCobblestone` → `ItemStone`，形状/数量/产物/耐久逐字节不变（提交 `3b738c4c`）
- [x] 相关既有形状与原料断言同步更新；未新增平行的「有自然来源」判据（该判据只由 Task 4 的守卫实现一次）
- [x] 无关的圆石夹具（采掘规则、容器回填、掉落渲染、伙伴白名单）刻意保持原样
- [x] 客户端核对：配方书与图标由配方表派生，原料变化无需改动（评审独立复核确认）
- [x] 修复轮：配方注释改为过去式并保留真实约束（提交 `b0d00f56`）
- [x] 验证：`go test ./packages/shared/core -race -count=1`、`go test ./packages/server/sim/... -race -count=1`、`go test ./packages/client/render ./packages/client/assets -count=1` 全绿

## Task 4: 价值闭包守卫

- [x] 新建 `packages/server/sim/entity/reachability_closure_test.go`：带溯源注释的自然来源方块常量 + 真实 `miningRule`（含工具门槛）+ `core.BlockDrop` + 作物/生物掉落 + 舀水命令 + 工具损坏映射 + `core.Recipe` + `core.SmeltingOutput`，求不动点（提交 `f9dcb702`）
- [x] 正向断言：工作台、火把、至少一种镐、面包或熟肉在闭包内；熔炉、铁锭、空桶也在闭包内；食物路径的前置农具必须一并纳入正向断言
- [x] 反向断言：闭包外的物品集合恰由单一显式清单声明，含五种装饰材料与**四项既有缺口（骨粉、马铃薯、胡萝卜、毒马铃薯）**——骨粉无任何生产者由本任务侦察发现，属既有延期（`bone-meal` 归档非目标 + backlog 骨粉获取路径行）
- [x] 测试自证非空洞：自然来源非空、闭包非空、配方枚举 > 0、清单内每项确实不在闭包内、集合差两个方向都断言
- [x] 验证：`go test ./packages/server/sim/entity -run TestReachability -race -count=1`、`go test ./packages/audit -count=1` 全绿；变异实验证明守卫能抓住配方原料回退与物品失去来源两类缺陷

## Task 5: 空背包起家端到端冒烟

- [x] 新建 `packages/server/server/survival_bootstrap_e2e_test.go`：真实缺省世界 + Memory transport，从空背包推进「原木 → 木板 → 工作台 → 放置开台 → 3×3 → 石镐」（提交 `630d5248`）
- [x] 全程真实玩法路径：无 `SetPlayerInventoryForTest`、无方块夹具、无 sleep；采掘前逐格核验冻结样本前提，worldgen 变化是硬失败而非 flaky
- [x] 评审用四个变异实验证伪（原木不掉落、工作台不开 3×3、石料不掉落、石镐原料变化）全部按预期变红
- [x] 验证：`go test ./packages/server/server -run TestSurvivalBootstrap -race -count=3` 全绿

## Task 6: 跨模块源码守卫改指新契约

- [x] `packages/shared/core/sapling_acquisition_test.go`：第三条腿改锚 `newMissingCachedPlayer`，断言其函数体不含物品标识符、不把 `Inventory` 填成非空、且仍自建快照字面量；文件级禁树苗标识符；锚点消失即失败（提交 `77ab14d6`）
- [x] 修复轮：删除不可达的点导入分支、失败消息补源码位置、两处失真措辞收敛、文档列出全部失败模式（提交 `6a9ba665`）
- [x] 验证：`go test ./packages/shared/core -run TestSaplingHasNoCraftingSmeltingOrStartingInventory -count=1`、`go test ./packages/audit -count=1` 全绿

## Task 7: 收尾（规格、文档、版本矩阵与全量门禁）

- [ ] 应用/核对三份 delta 与主规格一致：`common-block-materials`、`authoritative-crafting`、`oak-sapling-regrowth`（归档阶段执行 sync，本任务只核验 delta 与实现一致）
- [ ] `docs/notes/gameplay.md`、`docs/notes/limitations.md`、`README.md`：材料包描述改为「新玩家初始背包为空」，并补记四条关键能力由自然路径取得；`gameplay.md` 中熔炉与石剑的原料描述同步为石料；`limitations.md` 显式记名五种装饰材料与骨粉/马铃薯类四项既有缺口
- [ ] `docs/feature-backlog.md`：B-34 行状态与交付说明如实填写；为装饰材料自然获取路径登记后续行，并注明骨粉与马铃薯类的既有缺口行已存在（不重复登记）
- [ ] `docs/notes/progress.md`：追加本 change 的实现编年史条目
- [ ] 版本矩阵核验：确认协议 v40、玩家 schema v8、区块 schema v9、metadata v5、`companions.ai` v5、`hostile_mobs` v1、`passive_mobs` v1、engine ABI v11、client ABI v18、scenario v23 全部未变，并给出实际核验命令输出（`go test ./packages/audit -count=1`）
- [ ] 六模块 `gofmt`、`go vet`、全量 race 与严格校验：`make dev-check`、`make test-race`、`openspec validate --all --strict --no-interactive`
