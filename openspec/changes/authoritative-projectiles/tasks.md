# 任务：权威投射物与远程敌怪

> 每任务组先写失败测试再实现（red → green → refactor）；测试与被测代码同目录；验证命令在任务组内全部通过并记入 ledger（按基线 SHA 复用）后才勾选。跨语言常量（frame tag、record 布局）在同一任务组内双侧改齐。

- [ ] 1. core 物品/配方与弹药消耗原语
  - 文件：`packages/shared/core/item.go`（append-only 追加 `ItemBow`=62、`ItemArrow`=63、`ItemBone`=64、`ItemBrokenBow`=65，哨兵 `ItemIDMax` 62→66；堆叠弓 1/箭 64/骨 64；`ItemMaxDurability` 弓 120；`ItemBrokenForm` 弓→损坏形态；显示名「弓/箭/骨头/损坏的弓」）、`packages/shared/core/recipe.go`（`RecipeArrow`=25：上砾石下木棍→2 箭，2×2 可合成；枚举循环上界推进到新末项）、`packages/shared/core/inventory.go`（`ConsumeItem(id) bool`：hotbar 0..8 → backpack 9..35 首个 count>0 栈减 1 的 copy-on-write 原子语义）、同包测试（哨兵穷举自动覆盖、配方形状匹配与不匹配、`ConsumeItem` 跨区消耗/不足/原子性）。
  - 验证：`go test ./packages/shared/core -race -count=1`。

- [ ] 2. 协议 v43：投射物三消息与 hostile kind 字节
  - 文件：`packages/shared/network/protocol/packet.go`（`ProtocolVersion = 43` + 版本史注记）、`registry.go`（S→C 29/30/31 `ProjectileSpawn`/`ProjectileState`/`ProjectileDespawn`；下一空闲推进）、`message_projectile.go`（新：三消息 DTO + `Validate`——计数 1..128、严格 ID 升序、有限性/值域、despawn 仅 ID 列表）、`message_hostile.go`（`HostileSpawnRecord`/`HostileStateRecord` 尾部 `Kind uint8` 值域 0..1）、对应 codec（`codec/codec_server.go` 编码/解码/长度上限三处 + 新投射物编解码文件）、wire frozen/越界拒绝/往返属性测试与 hostile record 定长测试重钉（spawn 30B、state 38B）。
  - 验证：`go test ./packages/shared/network -race -count=1`。

- [ ] 3. `hostile_mobs` schema v2
  - 文件：`packages/server/storage/hostile/hostile_codec.go`（record 72→73B 尾部 kind；`CurrentSchema=2`；decode 白名单 {1,2}，v1 记录迁移恒 0；`MaxFileLength=4704`；encode 侧 kind 值域校验）、`hostile_types.go`（`StoredHostileMob.Kind`）、`testdata`（新增 v2 fixture、保留 v1）、fuzz/golden/layout 测试重钉（头部/记录布局、v1 迁移、v2 往返逐位、未来版本拒绝）。
  - 验证：`go test ./packages/server/storage/... -race -count=1`。

- [ ] 4. 投射物引擎域：集合、弹道与命中结算
  - 文件：`packages/server/sim/entity/projectile.go`（新：`projectileSet`（cap 128、ID 升序、最旧先清）、弹道积分（`physics.FixedDelta` 同源步长、重力 18 格/秒²、短档 16/满档 30/骨刺 22 初速）、方块命中（`core.RaycastBlocks` 线段）、实体命中（线段×AABB：玩家/敌怪/被动，按弹种过滤目标与持有者）、命中结算（护甲减免+耐久+击退+`applyDamage`；`CombatHit` 追加；`hostileState.applyDamage`；`DamagePassive`）、寿命 100 tick、订阅区消失、出界消失）、`packages/server/sim/entity/tick.go`（阶段插入：`advanceHostileDistant` 之后、`settleHostileDeaths` 之前；阶段序守卫重钉）、`packages/server/sim/contract/contract.go`（投射物导出投影：ID/kind/位置列表，供发布）、同包测试（确定性重放逐位、命中/未命中/遮挡/寿命/上限挤占/同 tick 死亡结算、阶段序断言、和平档不受影响）。
  - 验证：`go test ./packages/server/sim/... -race -count=1`。

- [ ] 5. 掷骨者：kind 分支、生成与行为
  - 文件：`packages/server/sim/entity/hostile.go`（`hostileState.Kind` 字段、生成/快照/恢复携带、`shootCooldown` 瞬态计时）、`hostile_spawn.go`（候选 hash 2:1 分派 kind、近玩家上限 8/4 分别计数）、`combat.go`（掷骨者不产生近战意图）、掉落按 kind 分派（骨头 0..2 + 弓 1/8，容量原子语义）、`packages/server/sim/contract/contract.go`（`HostileAction` 新增 `RangedAttack` 判别载荷 + `HostileMob.Kind`）、`packages/server/server/hostile_manager.go`（ranged advisor：接近/保持/直线后退 + LOS 射击意图 + 散布基准方向）、快照/发布/ingress 携带 kind、同包与 server 包测试（分派比例与上限、三段行为边界、LOS 拒绝、散布确定性、灼烧/远离/掉落共享语义、Memory/TCP parity）。
  - 验证：`go test ./packages/server/sim/... ./packages/server/server -race -count=1`。

- [ ] 6. 玩家弓：拉弓状态机与发射
  - 文件：`packages/server/sim/entity/bow.go`（新：`bowState{slot,item,progressTicks}`、`advanceBowDraw`（进入条件含背包有箭、中断矩阵沿进食同形、松开=发射判定点、<6 不发射/6..19 短档/≥20 满档）、发射原子结算（`ConsumeItem(ItemArrow)`+弓耐久 −1+损坏形态+生成箭））、`packages/server/sim/entity/player.go`（`advanceActivePlayers` 接线、`applyDamage`/`beginReset` 清零点）、`mining.go`（入口守卫：手持弓清采掘状态）、`combat.go`（`playerCombatIntent` 跳过弓持有者）、同包测试（拉满/短档/过早松开、无箭不发射不耗耐久、弹药跨区消耗、切格/开箱/受伤/死亡中断矩阵、持弓不可采掘不近战、损坏弓不可拉弓）。
  - 验证：`go test ./packages/server/sim/... -race -count=1`。

- [ ] 7. server 接线、发布与 parity
  - 文件：`packages/server/server/hostile_publication.go`（hostile 发布携带 kind；投射物三消息按会话订阅发布，despawn→spawn→state 序）、`publication_delta.go`（如新增拒绝原因则映射——预计无）、`persistence/hostiles.go`（kind 字段映射）、同包 parity 与重启集成测试（Memory/TCP 投射物镜像一致、掷骨者重启保值且射击冷却重置、v1 存档迁移整链）。
  - 验证：`go test ./packages/server/server -race -count=1`。

- [ ] 8. 客户端镜像、tag 14 呈现与图标
  - 文件：`packages/client/client/hostiles.go`（镜像带 kind）、`packages/client/client/projectiles.go`（新：spawn/state/despawn 镜像 + 插值）、`packages/client/render/frame_streams.go` + `packages/client/render/avatar.go`（tag 14 编码器：96B 实例、mat4 沿速度、纯色哨兵；掷骨者 avatar 几何/配色分支）、`packages/client/cmd/mornlea/app`（消息路由、投射物呈现接线）、`packages/engine/crates/mornlea_client`（`FRAME_TAG_MAX` 14、tag 14 解码、新投射物 pass 与绘制序、双侧布局锁测试）、`packages/client/assets`（弓/箭/骨头/损坏的弓图标：`ItemIconLayer`/`originalItemTexture`/cutout 分类）、同层测试（tag 白名单、容量、镜像纪律、图标守护）。
  - 验证：`go test ./packages/client/... -race -count=1`；`cd packages/engine && cargo test --workspace`（或 `make rust` 内含）。

- [ ] 9. capture 场景、golden 与视觉验收
  - 文件：`packages/client/cmd/mornlea/capture`（新场景 `ranged-mob`：夜间夹具、掷骨者 + 目标玩家位姿、`PinVolatile` 钉弹道相位；插表位置与位置守卫测试）、golden `testdata/visual-golden/world/ranged-mob.png`、`testdata/visual-golden/README.md` 索引行、前端 `items-all` fixture 重生成（四件新物品）、`openspec/specs/visual-verification` 场景清单需求随 delta 重钉（30→31）。
  - 验证：`make visual-check`（31 景全绿）；`make frontend-visual-check`（更新 `items-all` 后全绿）。

- [ ] 10. 第二类敌怪边界评估落档与 audit 守卫
  - 文件：本 change `design.md` D3 评估结论定稿、`packages/audit`（如需：敌怪 kind 域守卫/协议 pure-append 断言更新）、`docs/feature-backlog.md` B-26 行回填评估结论（归档阶段执行）。
  - 验证：`go test ./packages/audit -count=1`。

- [ ] 11. 收尾门禁
  - 命令：`test -z "$(gofmt -l .)"`；`go vet ./packages/contracts/... ./packages/shared/... ./packages/server/... ./packages/client/... ./packages/tools/... ./packages/audit/...`；`make test-race`；`make rust`；`make dev-check`；`openspec validate --all --strict --no-interactive`；基线文档同步（根 `AGENTS.md` 版本矩阵、`openspec/config.yaml`、`docs/notes/progress.md`）。
  - 全部通过后按 ledger 记录证据，进入整分支终审与归档。
