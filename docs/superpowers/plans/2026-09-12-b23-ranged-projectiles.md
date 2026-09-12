# 实现计划：权威投射物与远程敌怪（B-23）

> OpenSpec change：`openspec/changes/authoritative-projectiles/`（proposal/specs/design/tasks/ledger 为唯一契约来源，本计划是其可执行切片）。上游设计：`docs/superpowers/specs/2026-09-12-b23-ranged-projectiles-design.md`（已批准）。
> 工作分支：`feat/B-23-ranged-projectiles`（worktree `.worktrees/B-23-ranged-projectiles`）。

## Global Constraints（每个任务组都必须遵守）

- 版本纪律：本 change 只升协议 v42→v43（`packages/shared/network/protocol/packet.go` 的 `ProtocolVersion`）与 `hostile_mobs` schema v1→v2（`packages/server/storage/hostile/hostile_codec.go` 的 `CurrentSchema`）；engine ABI v11、client ABI v19、玩家 schema v9、区块 schema v9、世界 metadata v6、`companions.ai`/`passive_mobs` v5/v1、benchmark scenario v23 一律不动；frame TLV 投射物段是新 tag 14，不加任何新 FFI 导出面。
- 编号纪律：物品/配方/消息 ID 只允许 append-only；`ItemIDMax` 哨兵推进；配方穷举循环上界推进到新末项；S→C 投射物三消息用 ID 29/30/31；hostile record 尾部 pure-append `Kind u8`（值域 {0,1}，0=夜行者、1=掷骨者）。
- 确定性纪律：所有新数值判定（散布、掉落数量、弹道）必须是 `(worldSeed, tick, 维度, 坐标/ID)` 输入的纯整数 splitmix64 链或固定数值契约；无 `math/rand`、无 map 遍历、无全局 RNG；弹道积分复用 `physics.FixedDelta` 与 f32 语义；固定值：重力 18 格/秒²、骨刺初速 22、箭短档 16/满档 30、箭伤害短档 2/满档 5、骨刺伤害 3、拉弓 <6 不发射/6..19 短档/≥20 满档、骨刺射击冷却 40 tick、投射物寿命 100 tick、投射物全服上限 128（最旧 ID 先清）、掷骨者掉落骨头 0..2 + 弓 1/8、近玩家上限夜行者 8/掷骨者 4（48 格内）。
- 伤害纪律：一切对玩家/敌怪/被动的伤害必须经既有唯一入口（`playerState.applyDamage`/`hostileState.applyDamage`/`engine.DamagePassive`）；玩家目标先经 `core.ReducedDamage` 护甲减免并按既有规则耗耐久；击退沿弹速水平分量。
- 注释纪律：新代码注释一律中文；禁止任何任务编号（`B-23` 形如 `[A-F]-[0-9]{2}`）出现在生产或测试代码注释中。
- 测试纪律：每任务组 red → green → refactor；测试与被测代码同目录；一个测试文件一个主题；不跑全量 race（那是收尾门禁）。
- 验证命令按任务组执行并记入 change ledger（按基线 SHA）；Rust 侧改动同批跑 `cargo test --workspace -p mornlea_client` 级别的定点面。
- 独占文件集见 change proposal；不触碰 `docs/notes/lan-server.md`、伙伴域、被动牛行为、世界生成。

## Task 1: core 物品/配方与弹药消耗原语

文件：`packages/shared/core/item.go`（append-only 追加 `ItemBow=62`、`ItemArrow=63`、`ItemBone=64`、`ItemBrokenBow=65`，哨兵 `ItemIDMax` 62→66；`ItemStackLimit`：弓 1、箭 64、骨 64、损坏弓 1；`ItemMaxDurability`：弓 120；`ItemBrokenForm`：`ItemBow→ItemBrokenBow`；`item_name.go` 显示名「弓/箭/骨头/损坏的弓」；三者不进 `BlockDrop`、不进 `ItemPlacement`）、`packages/shared/core/recipe.go`（`RecipeArrow=25`：pattern 上格 `ItemGravel`、下格 `ItemStick`，产物 2×`ItemArrow`；穷举循环上界推进到 `RecipeArrow`）、`packages/shared/core/inventory.go`（`ConsumeItem(id ItemID) bool`：copy-on-write，hotbar 0..8 → backpack 9..35 首个 count>0 匹配栈减 1，任一失败不提交）。
测试：`item_test.go` 哨兵穷举自动覆盖 + 新物品属性断言；`recipe_test.go` 配方形状匹配/不匹配/2×2 可合成；`inventory_test.go` 跨区消耗、不足失败、原子性（失败不改动）、扫描序确定性。
验证：`go test ./packages/shared/core -race -count=1`。
注意：先确认 `ItemGravel`/`ItemStick` 的实际标识符名（实现期以代码为准，不得新增砾石/木棍物品）。

## Task 2: 协议 v43——投射物三消息与 hostile kind 字节

文件：`packages/shared/network/protocol/packet.go`（`ProtocolVersion=43` + 版本史追加一行）、`protocol/registry.go`（S→C 29/30/31 = `ProjectileSpawn`/`ProjectileState`/`ProjectileDespawn`；ServerPacketForID 更新；「下一空闲」注记推进到 32）、`protocol/message_projectile.go`（新：三 DTO + `Validate`——count 1..128、spawn/state 严格 ID 升序且非零、spawn 携带合法 dimension 与有限 pos/vel、despawn 仅 ID、record 定长常量）、`protocol/message_hostile.go`（`HostileSpawnRecord`/`HostileStateRecord` 尾部追加 `Kind uint8`，Validate 值域 {0,1}；wire 常量 spawn 29→30B、state 37→38B 同批）、`codec/codec_projectile.go`（新：三消息双向编解码，精确字节长校验）、`codec/codec_server.go`（分发、长度上限三处登记）、同批测试：wire frozen 布局测试（投射物 record 定长 + hostile 30/38B 重钉）、越界拒绝矩阵（逆序/零 ID/NaN/kind=2/count 越界/截断/尾随）、双传输往返属性测试。
验证：`go test ./packages/shared/network -race -count=1`。

## Task 3: hostile_mobs schema v2

文件：`packages/server/storage/hostile/hostile_codec.go`（record 72→73B：尾部 kind；`CurrentSchema`→2；decode schema 白名单 {1,2}，v1 记录迁移 kind 恒 0；`MaxFileLength`→4704；encode kind 值域校验）、`hostile_types.go`（`StoredHostileMob.Kind uint8`）、`testdata/hostile-mobs-v2.bin`（新 golden fixture）+ 保留 v1 fixture、测试：layout 测试重钉（头部/记录 73B）、v1 只读迁移断言、v2 往返逐位、非法 kind 拒绝、未来版本拒绝、fuzz 更新。
验证：`go test ./packages/server/storage/... -race -count=1`。

## Task 4: 投射物引擎域——集合、弹道与命中结算

文件：`packages/server/sim/entity/projectile.go`（新：`projectileSet`——cap 128、ID 升序 slice + 二分、超限最旧先清并产生 despawn 记录；`projectileState{kind, pos, vel, ageTicks, ownerSession?}`；`advanceProjectiles`——固定步长积分（重力 18）、`core.RaycastBlocks` 线段方块命中（`core.InteractionTarget` 谓词）、实体线段×AABB 命中（玩家 8/敌怪 64/被动 32 候选上界、按弹种过滤：骨刺只命中玩家、箭命中其他玩家/敌怪/被动、两类不命中发射者）、命中结算（玩家：`core.ReducedDamage`+`consumeArmorDurability`+击退+`applyDamage`+玩家所有的箭追加 `CombatHit`；敌怪：`applyDamage`+击退；被动：`DamagePassive`）、寿命 100/订阅区外/出界消失）、`packages/server/sim/entity/tick.go`（阶段插入：`advanceHostileDistant` 之后、`settleHostileDeaths` 之前调用 `engine.advanceProjectiles`；`TickContext` 相应扩展）、`packages/server/sim/entity/engine.go`（`State` 增 `projectiles projectileSet` 所有权字段——audit 断言同步）、`packages/server/sim/contract/contract.go`（投射物发布投影类型：ID/Kind/Dimension/Pos/Vel 与 despawn 列表，ID 升序）、同包测试：确定性重放（同种子同输入逐位）、命中/未命中/遮挡、寿命与出界与上限挤占、发射者免疫、骨刺不命中牛、弹击致死同 tick 结算（0 血不留到下 tick）、阶段序守卫测试重钉。
验证：`go test ./packages/server/sim/... -race -count=1`。

## Task 5: 掷骨者——kind 分支、生成与行为

文件：`packages/server/sim/entity/hostile.go`（`hostileState.Kind uint8`；生成/快照/恢复/校验携带 kind；`shootCooldown uint8` 瞬态字段不入存档）、`hostile_spawn.go`（候选通过校验后按候选哈希 `hash%3==0 → 掷骨者` 分派；`hostileNearLimitExceeded` 按 kind 分别计数 8/4）、`combat.go`（掷骨者不产生近战意图）、`hostile.go` 掉落分派（夜行者腐肉不变；掷骨者骨头 0..2 + 弓 1/8，`(worldSeed, tick, mobID)` 哈希，走 `PrepareDropBatch` 原子预演）、`packages/server/sim/contract/contract.go`（`HostileMob.Kind` 投影 + `HostileAction` 新增 `RangedAttack{VelocityX/Y/Z 判别载荷}`）、`packages/server/sim/entity/hostile_action.go`（`RangedAttack` 结算：冷却/存活校验 + 散布哈希 + 生成骨刺）、`packages/server/server/hostile_manager.go`（ranged advisor：>14 A* 接近、6..14 保持、<6 直线后退意图；射击决策=40 tick 冷却 + `core.InteractionTarget` LOS + 基准方向）、`packages/server/server/hostile_publication.go`/`persistence/hostiles.go`（kind 携带）、同包与 server 测试：分派确定性、上限按 kind、行为三带边界、LOS 拒绝、散布确定性、灼烧/远离/掉落共享、掷骨者无近战意图、Memory/TCP parity、重启保值（射击冷却重置为就绪）。
验证：`go test ./packages/server/sim/... ./packages/server/server -race -count=1`。

## Task 6: 玩家弓——拉弓状态机与发射

文件：`packages/server/sim/entity/bow.go`（新：`bowState{slot uint8, item core.ItemID, progressTicks uint16}`；`advanceBowDraw`——进入条件（完好弓+`miningHeld`+`ConsumeItem` 预检有箭+无容器+视野就绪+非 reset）、逐 tick 累加、`(slot,item)` 失配重置、松开发射判定（<6 不发射/6..19 短档/≥20 满档）、发射原子结算（`ConsumeItem(ItemArrow)` 失败整体不发生；成功扣弓耐久 1、归零换 `ItemBrokenBow`、生成箭））、`packages/server/sim/entity/player.go`（`bowState` 字段挂 `playerState`；`advanceActivePlayers` 在进食后接线；`applyDamage`/`beginReset` 清零）、`packages/server/sim/entity/mining.go`（入口守卫追加：手持任一形态弓则清采掘状态且本 tick 不推进）、`packages/server/sim/entity/combat.go`（`playerCombatIntent` 跳过弓持有者）、同包测试：拉满/短档/过早、中断矩阵（切格/开箱/受伤/死亡/视野）、无箭不开弓、损坏弓不可拉弓、弹药跨区消耗、耐久归零换形态、持弓不近战不采掘。
验证：`go test ./packages/server/sim/... -race -count=1`。

## Task 7: server 接线、发布与 parity

文件：`packages/server/server/hostile_publication.go`（hostile spawn/state 携带 kind；投射物三消息按会话订阅发布——复用敌怪可见性谓词与 despawn→spawn→state 每类一包纪律）、`packages/server/server/session_ingress.go`（如需：无新命令则零改动）、`packages/server/server/persistence/hostiles.go`（kind 字段映射 v2）、同包测试：投射物发布序列 parity（Memory/TCP 逐字段）、可见性边界、掷骨者+投射物重启集成、v1 存档迁移整链。
验证：`go test ./packages/server/server -race -count=1`。

## Task 8: 客户端镜像、tag 14 呈现与图标

文件：`packages/client/client/hostiles.go`（镜像带 Kind）、`packages/client/client/projectiles.go`（新：`MaxProjectiles=128`、spawn/state/despawn latest-wins + `remoteActor` 插值、未知 ID state 丢弃、升序呈现）、`packages/client/render/frame_streams.go`（tag 14 编码器：96B 实例、mat4 沿速度取向、纯色哨兵材质、骨刺骨白/箭深棕两色）、`packages/client/render/avatar.go`（掷骨者几何/配色分支：`EntityKind` 不增值，Hostile 内部按 Kind 分支——骨白躯干/深灰头/细臂，tag 1 通道不变）、`packages/client/cmd/mornlea/app/app_messages.go`+`app_render.go`（路由与接线）、`packages/engine/crates/mornlea_client/src/ffi.rs`（`FRAME_TAG_MAX` 13→14、tag 14 解码 case、对齐/重复拒绝同规）、`packages/engine/crates/mornlea_client/src/render/`（新投射物 pass——EntityPass outline 变体同构，绘制序 avatar 之后；mod.rs pass 表登记）、`packages/client/assets/item_icons.go`（弓/箭/骨头/损坏的弓四例：`ItemIconLayer`+`originalItemTexture`+`isCutoutLayer` 视需要）、双侧布局锁测试（tag 14 实例 96B、FRAME_TAG_MAX、镜像纪律、图标守护测试过）。
验证：`go test ./packages/client/... -race -count=1`；`cargo test --workspace`（engine crate 定点面）。
注意：frame 实例布局与 `avatar.go` 96B 三方锁测试同批更新；不得新增 client ABI 导出面。

## Task 9: capture 场景、golden 与视觉验收

文件：`packages/client/cmd/mornlea/capture/capture.go`（新场景 `ranged-mob`：夜间夹具 + 掷骨者与目标玩家位姿 + `PinVolatile` 钉弹道相位；插在 `hostile-mob` 相关序列之后、`water-surface-slope` 之前；位置守卫测试按需重钉）、golden `testdata/visual-golden/world/ranged-mob.png`（`make visual-update SCENES=ranged-mob` 后全量核对）、`testdata/visual-golden/README.md` 索引行、前端 `items-all` fixture 重生成（`make frontend-visual-update` 范围内）、`openspec/specs/visual-verification/spec.md` 场景清单需求按 delta 同步（随 change 归档 sync，任务组内只改 capture 代码与 golden）。
验证：`make visual-check`（31 景全绿）；`make frontend-check` 与 `make frontend-visual-check`。
注意：既有 30 张世界 golden 必须零差异（新增场景不影响既有场景——共享 application 状态按场景表顺序全部自设）。

## Task 10: 边界评估落档与 audit 守卫

文件：change `design.md` D3 评估结论定稿（kind 字节共享机制、advisor 分派、完整 ECS 被否、升级条件=第三族敌怪或跨族行为收敛需求）、`packages/audit`（敌怪域守卫按需：kind 值域/协议 pure-append 断言）、本计划验收清单核对。
验证：`go test ./packages/audit -count=1`。

## Task 11: 收尾门禁

命令（全部通过并记 ledger）：`test -z "$(gofmt -l .)"`；`go vet ./packages/contracts/... ./packages/shared/... ./packages/server/... ./packages/client/... ./packages/tools/... ./packages/audit/...`；`make test-race`；`make rust`；`make dev-check`；`openspec validate --all --strict --no-interactive`；基线文档同步（根 `AGENTS.md` 版本矩阵两处、`openspec/config.yaml`、`docs/notes/progress.md` 基线段）。

## 验收（对照战役 §7）

1. 夜间生成掷骨者并投掷骨刺，玩家可被命中（护甲减免生效）。
2. 玩家可掷骨者掉落获得弓、合成箭，远程命中敌怪/牛/玩家（CombatHit 确认）。
3. `hostile_mobs` v1 旧档迁移无感；重启保值。
4. 协议 v43 + hostile_mobs v2 唯二升版；engine ABI/client ABI/其余 schema/scenario 不变。
5. 31 景视觉全绿；第二类敌怪边界评估落档。
