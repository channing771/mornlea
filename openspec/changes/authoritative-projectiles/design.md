# 设计：权威投射物与远程敌怪

> 背景与缺口定位、批准记录见 `docs/superpowers/specs/2026-09-12-b23-ranged-projectiles-design.md`（本 change 的上游设计）。本文记录实现级数据所有权、并发边界与文件落点。

## D1 数据所有权与依赖方向

- **投射物状态**：`packages/server/sim/entity/projectile.go`（新）持有 `projectileSet`（容量固定 slice、ID 严格升序，镜像 `hostileSet` 形态：容量 128、二分定位、超限拒插并按「最旧 ID 先清」腾位）。投射物是引擎域内部状态，**不持久化**（D2）；`TickInput` 不新增字段，投射物集合挂在 `entity.State` 上（沿 `hostiles hostileSet` 的所有权先例，audit `sim_authority_test.go` 的显式所有权断言同步登记）。
- **弹道数值**：Go 侧固定步长积分。每权威 tick 一步：`vel.y -= gravity*dt`，`pos += vel*dt`（`physics.FixedDelta` 同源常量，f32 与玩家/敌怪同一积分语义）。方块命中经 `core.RaycastBlocks`（Rust DDA 生产出口）对「上一位置→新位置」线段求首个 `core.InteractionTarget` 命中；实体命中为同线段对候选 AABB 的参数化扫描，取最小 t。被否：Rust engine 新增弹道出口（engine ABI v12）——每 tick ≤128 条轻量规则非重数值热路径，A* 寻路留 Go 同判。
- **命中结算**：命中玩家走 `core.ReducedDamage`（冻结命中时点护甲点数）→ `consumeArmorDurability`（仅实际减免时）→ `combatKnockback` 同族击退（沿弹速水平分量）→ `applyDamage` 唯一入口；玩家所有的箭命中实体时向持有者会话追加既有 `CombatHit` 私有确认。命中敌怪走 `hostileState.applyDamage` + 击退；命中被动牛走 `engine.DamagePassive`。
- **弹种**：`ProjectileKindShard`（骨刺，敌怪发射，伤害 3，只命中玩家）与 `ProjectileKindArrow`（箭，玩家发射，短档伤害 2 / 满档 5，命中其他玩家、敌怪、被动牛；不命中持有者本人）。

## D2 生命周期与瞬态语义

- 生成：敌怪骨刺在 `applyHostileActions` 结算射击意图时生成；玩家箭在拉弓发射结算时生成。两者都直接入 `projectileSet` 并在同一权威 tick 的投射物阶段起步一次。
- 消失：命中方块（消失，不掉落）、命中实体（结算后消失）、寿命 100 tick、离开全部会话订阅区（沿 hostile 可见性谓词）、出世界边界、全服 128 上限被新投射物挤占（最旧 ID 先清）。**不持久化**：`hostile_mobs` 之外零新存档面，重启后在飞投射物消失——被否的持久化方案（实体存档通道）收益不抵 schema 面扩，vanilla 参考实现亦仅作短生命周期实体处理。
- 决定性：投射物 ID 由 `sampler.SplitMix64` 派生（沿敌怪 ID 链），无进程级随机源；散布与掉落 hash 输入为 `(worldSeed, tick, 维度, 坐标/ID)`。

## D3 敌怪 kind 与共享机制（第二类敌怪边界评估的结论）

- `hostileState` 追加 `kind uint8`（0=夜行者、1=掷骨者），随 `hostile_mobs` v2 持久化。共享机制不改：生成预算（每 tick 恰一个候选）、灼烧（白昼露天 1 点/20 tick）、远离 64 格消失（600 tick 累计）、死亡掉落结算、`hurtCooldown`/`burnCooldown` 计时、`combatKnockback`。
- **kind 分派**：生成候选通过全部既有校验后以候选 hash 按 2:1 分派（`hash%3 != 0` → 夜行者、`==0` → 掷骨者）；近玩家 48 格上限按 kind 分别计数（夜行者 8、掷骨者 4），全服 64 共享。
- **行为差异收敛为两处 advisor**：
  1. 管理器侧（`packages/server/server/hostile_manager.go`）：目标选择后按 kind 分派——夜行者走既有 chase（A* 接近 + 近战意图冻结）；掷骨者走 ranged advisor（>14 格 A* 接近、6..14 格保持、<6 格直线后退意图——直走世界轴向量，可能被墙挡住，规格按「直线后退意图」措辞；40 tick 射击冷却 + `core.InteractionTarget` 同源 LOS + 确定性散布方向经新 `HostileAction` kind 入域）。
  2. 引擎侧（`applyHostileActions`）：按 kind 结算——近战意图沿用既有冻结分叉；`RangedAttack` 意图校验冷却与存活后在射点生成骨刺（散布 hash 在引擎内求值，管理器只给基准方向，保证同输入重放一致）。
- **射击冷却**：掷骨者 `shootCooldown` 为引擎内瞬态计时（40 tick 周期），**不入存档**——重启后冷却归零，最坏提前一拍开火，属接受的恢复语义（v2 record 只追加 kind 字节，不再扩记录）。
- **掉落**：`dropHostileLoot` 按 kind 分派：夜行者腐肉 1（不变）；掷骨者骨头 0..2（hash）+ 弓 1/8（确定性 hash，沿树苗掉落先例；走与夜行者共享的既有掉落批原子语义——容量不足确定性省略、死亡照常完成，任务组 5 核实更正「待重试」措辞）。
- **被否的完整 ECS/共享抽象重构**：当前两族敌怪共享面已显式且测试锁定；被动族（牛）已走独立机制；出现第三族敌怪或跨族行为收敛需求前，重构无第二消费者。评估全文随本 change 归档，`docs/feature-backlog.md` B-26 行回填结论。

## D4 tick 阶段与死亡结算

投射物阶段插入敌怪阶段内部（`TickContext.AdvanceHostiles`，`packages/server/sim/entity/tick.go:420`）：

```
advanceHostiles（生成/意图/移动）
→ advanceCombat（近战结算）
→ advanceHostileBurn
→ advanceHostileDistant
→ advanceProjectiles   ← 新增（弹道推进 + 命中结算）
→ settleHostileDeaths
→ settleDeaths
```

理由：投射物命中致死必须与近战致死同 tick 完成掉落与重生结算，且任何 0 血实体 MUST NOT 存活到下一 tick；把两个死亡结算统一放在投射物之后保证单一结算点。既有 `settleHostileDeaths` 与 `advanceHostileDistant` 的相对顺序对调（distant 先于结算）不产生观察差异——distant 只改集合成员与计数，结算只处理 0 血成员；阶段序守卫测试重钉。

## D5 玩家拉弓状态机

- `bowState{slot uint8, item core.ItemID, progressTicks uint16}` 挂 `playerState`（瞬态、不持久化，沿 `eatingState` 先例；`beginReset`/`applyDamage` 清零点同进食）。
- 触发：`advanceActivePlayers` 内、进食之后按序推进 `advanceBowDraw`。进入条件：手持物品为弓（`ItemBow` 或 `ItemBrokenBow`？——损坏弓不可拉弓，仅完好弓）、`miningHeld` 为真、背包存在箭（新增 `core.Inventory.ConsumeItem(id) bool`：hotbar 0..8 → backpack 9..35 首个 count>0 栈减 1 的 copy-on-write，沿 `ConsumeRecipe` 形状）、无 `viewContainer`、视野 Ready、非 `reset`。任一不满足则状态清零。
- 进度：逐 tick 累加；`(slot,item)` 失配重置为 1（防「拉 A 弓射 B 弓」）。
- 松开（`miningHeld` 变假）= 发射判定点：`progressTicks < 6` 不发射；`6..19` 短档（伤害 2、初速低档）；`≥20` 满档（伤害 5、初速高档，更长按住保持满档）。发射原子结算：`ConsumeItem(ItemArrow)` 失败则不发射不耗耐久；成功则扣弓耐久 1（0 → `ItemBrokenBow` 换形态）并从眼位沿视线生成箭。
- **占用裁决**：持弓时主输入位完全归拉弓域——`advanceMining` 入口守卫追加「手持弓则清采掘状态」；`playerCombatIntent` 跳过弓持有者（弓不产生近战意图）。被否：右键（`Use` 位）拉弓——该位与开箱/进食共用，冲突面大于收益。
- 弹道初速：短档 16 格/秒、满档 30 格/秒（f32，沿视线方向）；骨刺初速 22 格/秒朝目标眼位 + 确定性散布；重力同为 18 格/秒²（三值在本 change 内为固定数值契约，写明注释与测试钉）。

## D6 协议 v43 与存储 v2

- S→C 29/30/31：`ProjectileSpawn{ID,Kind,Dimension,Pos×3,Vel×3}`、`ProjectileState{ID,Pos×3}`、`ProjectileDespawn{IDs}`。计数上限 128、spawn/state 严格 ID 升序、`Validate` 有限性/值域检查、按会话订阅发布（沿 `hostileCandidateVisible` 谓词与「despawn → spawn → state」每 tick 每类一包的发布序）。record 定长：spawn 37B、state 20B、despawn 8B（u64 id + kind 1B + 维度 4B + 6×f32 = 37；u64 id + 3×f32 = 20；实现期以 wire frozen 测试钉死，规格只钉「定长 + 上限 + 升序」；2026-09-12 任务组 2 核实更正，初稿 42B/18B 为算术笔误）。
- hostile `Spawn`/`State` record 尾部追加 `kind u8`（spawn 29→30B、state 37→38B，pure-append）。
- `hostile_mobs` v2：record 72→73B（尾部 kind）；`CurrentSchema=2`；decode 白名单 {1,2}（v1 记录迁移恒 0=夜行者）；`MaxFileLength = 32+64×73 = 4704`；CRC、逐项校验、损坏整拒语义逐条不变；golden fixture 新增 v2、保留 v1 供迁移断言。

## D7 客户端呈现

- hostile 镜像带 kind 分支：掷骨者为原创程序化 biped 变体（骨白躯干 + 深灰头部 + 细臂持骨姿势，`EntityKind` 不增值，Hostile=4 内部分支几何与配色，tag 1 通道与 96B 实例布局不变）。
- 投射物镜像：`packages/client/client/projectiles.go`（新，`MaxProjectiles=128`，latest-wins + `remoteActor` 插值，沿 hostiles 先例；未知 ID 的 state 丢弃、重复 spawn 忽略）。
- frame TLV **tag 14**：96B avatar 实例布局（mat4 沿速度取向 + 纯色哨兵材质），Go `frame_streams.go` 新编码器、Rust `FRAME_TAG_MAX` 13→14 + 新小 EntityPass（outline/crack 变体同构，绘制序 avatar 之后）。**零新 FFI 导出面 → client ABI 保持 v19**（裂纹 overlay 先例：tag 10 入帧未动 v14）。
- 物品图标：`ItemIconLayer` + `originalItemTexture` 补弓/箭/骨头/损坏的弓四例（缺失即 panic 的守护测试强制），`items-all` 前端部件基线重生成。
- 音效零新增：箭命中实体走既有 `CueCombatHit`，本地玩家被骨刺命中走既有 `CueDamage`（确认边界纪律内已有覆盖面，设计阶段原拟的 2 个新 cue 被否）。

## D8 并发与热路径边界

- 投射物推进全在权威 tick 单线程内，无新锁；`projectileSet` 与 `hostileSet` 同为 tick 内独占集合。
- 每 tick 成本上界：≤128 条弹道 ×（1 次 DDA 线段 + ≤8 玩家 + ≤64 敌怪 + ≤32 被动的 AABB 扫描），全部 O(常数)；发布每会话每类至多一包。无 map 遍历、无阻塞 I/O。
- 发布与镜像沿用 hostile 三消息的订阅/去重纪律；投射物不进入 benchmark 固定工作负载（scenario v23 不变）。

## D9 被否方案汇总

| 方案 | 否决理由 |
|---|---|
| Rust engine 弹道出口（ABI v12） | 轻量规则非热路径；A* 留 Go 同判；避免无谓 ABI 升版 |
| 持久化在飞投射物 | 无实体存档通道；收益不抵 schema 面扩；瞬态语义简单可验证 |
| 独立 `ranged_mobs.bin` | 掷骨者与夜行者共享生命周期，同文件保单一真源（被动牛是独立**族**才分文件） |
| 右键（Use 位）拉弓 | 与开箱/进食共用位，冲突面大 |
| 新增 `PlayerInput` 拉弓位 | `Mining` 位本就是「持续主动作、语义由服务端按手持物品裁决」，复用零协议面 |
| 新增弓/骨刺音效 cue | 既有 `CueCombatHit`/`CueDamage` 已覆盖确认边界 |
| 完整 ECS/AI 抽象重构 | 无第三消费者；kind 分支已把差异收敛到两处 advisor |
| settleHostileDeaths 保持在投射物之前 | 0 血敌怪会存活到下一 tick，违反「死亡同 tick 结算」既有契约 |

## D10 风险与回退

- 阶段序重排触及既有阶段守卫测试：以测试显式重钉并论证两处对调零观察差异（D4）；回退路径为整 change revert（协议 v43 与 schema v2 均为 pure-append，回退只需拒绝新版本）。
- 掷骨者直线后退可能卡墙：规格按「直线后退意图」措辞，不做绕障承诺；后续行可升级 A* 后退。
- 骨刺散布数值（角度上界）为固定数值契约：实现期以确定性 hash 向量测试钉住，不做 tunable。
