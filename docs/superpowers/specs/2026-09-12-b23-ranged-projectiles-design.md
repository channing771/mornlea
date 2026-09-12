# B-23 远程敌怪与投射物 — 实现设计

日期：2026-09-12
状态：待需求方批准（brainstorming 门禁）
路径分类：architectural（新实体族 + 协议族 + 存档 schema 升版）
来源：`docs/feature-backlog.md` B-23 行；`docs/superpowers/specs/2026-09-11-survival-loop-completion-design.md` §4 阶段一、§8；行认领提交 `c827996a`。

## 1. 缺口定位（对原版的差距分析）

对照《我的世界》原生玩法逐块盘点当前基线（协议 v42、B-24 护甲与 B-11 难度已归档）：

- **已成型**：采掘/放置/合成/熔炼、背包/容器、饥饿/进食/农业三作物、生命/摔落/溺水/回血/床与重生、昼夜/四季/天气/积雪、护甲与三档难度、潜行/疾跑、伙伴 AI、牛与夜行者。
- **最大缺口（战役内）**：**战斗与生物纵深**——敌怪只有夜行者一种且只会近战，没有任何远程威胁、投射物与远程武器；玩家战斗面止步于剑与护甲。次大缺口是地下世界（B-37 排队在斧铲行之后）与红石（用户已裁决排在生存闭环战役之后）。
- 本行按已批准的串行链队首补「战斗与生物纵深」：投射物权威域 + 一类远程敌怪 + 玩家弓，并随行完成战役 §8 硬性要求的「第二类敌怪 AI/ECS 共享边界」评估。

## 2. 交付面总览

| 域 | 交付 |
|---|---|
| 服务端 sim | 新投射物权威域（弹道/命中/结算/上限）、远程敌怪 kind 分支、玩家拉弓状态机 |
| 协议 | v42→v43：S→C 29/30/31 投射物三消息；hostile Spawn/State record 尾部追加 kind 字节（pure-append） |
| 存档 | `hostile_mobs` schema v1→v2：record 72→73 字节尾部 kind；v1 只读迁移恒夜行者 |
| 物品/配方 | 弓（62，耐久 120，损坏形态 65）、箭（63）、骨头（64）、`ItemIDMax`=66；`RecipeArrow`=25（砾石+木棍→2 箭） |
| 客户端 | hostile 镜像 kind、投射物镜像与呈现（frame TLV tag 14，零 client ABI 升版）、音效 2 cue、`ranged-mob` 场景 |
| 版本矩阵 | **协议 v43 与 `hostile_mobs` v2 唯二升版**；engine ABI v11、client ABI v19、玩家/区块 schema、metadata v6、scenario v23 全不变 |

## 3. 裁决（D1..D11）

**D1 投射物权威域与弹道归属**：新 `projectileSet`（ID 升序、全服上限 128、超限最旧 ID 先清）。弹道为 Go 侧固定步长积分（与玩家/敌怪同一 `physics.FixedDelta` 与 f32 语义），方块命中走既有 Rust DDA（`core.RaycastBlocks` 生产出口），实体命中为每 tick 线段×AABB 扫描（玩家 ≤8、敌怪 ≤64、投射物 ≤128，有界）。被否：Rust engine 新增弹道出口（engine ABI v12）——弹道是每 tick ≤128 条的轻量规则，非重数值热路径，A* 寻路留 Go 同判；也不引入第二条物理积分路径（复用步长常量即够）。

**D2 弹种与命中模型**：kind 0=骨刺（远程敌怪发射，伤害 3，只命中玩家），kind 1=箭（玩家弓发射，短档伤害 2 / 满档 5，命中其他玩家[ PvP]、敌怪、被动牛；不命中持有者本人）。命中玩家：经 `core.ReducedDamage` 护甲减免 + 受击全件耐久 −1 + 沿弹速水平分量的击退 + `applyDamage` 唯一入口；玩家射中实体追加既有 `CombatHit` 私有确认。命中方块：消失（不做插箭滞留态与拾取，非目标）。寿命 100 tick；离全部订阅区、出世界边界、超 cap 同样消失。**瞬态不持久化**：重启后在飞投射物消失，存档面零迁移；被否：持久化在飞投射物（vanilla 亦只在实体层短存，Mornlea 无实体存档通道，收益不抵 schema 面扩）。

**D3 远程敌怪「掷骨者」**：`hostileState` 追加 `kind` 字节分支（0=夜行者近战，1=掷骨者远程），共享 `hostileSet`（全服 64）、单候选/每 tick 生成预算（同 hash 以 2:1 比例分派 kind）、灼烧（白昼露天 1 点/20 tick）、远离 64 格消失、死亡掉落结算。近玩家 48 格上限：夜行者 8、掷骨者 4（分别计数）。行为：目标 >14 格接近（A*，沿夜行者先例）、6..14 格保持、<6 格直线后退意图（直走世界轴向量，可能被墙挡住，v1 接受）；射击 40 tick 冷却 + 视线无遮挡（`core.InteractionTarget` 同源射线）+ 确定性散布（`(mobID, tick)` hash）；白昼照常灼烧。死亡掉落：骨头 0..2（hash）+ 弓 1/8（确定性 hash，沿树苗掉落先例）。和平档零生成由既有 `advanceHostileSpawn` 入口短路自动覆盖。

**D4 `hostile_mobs` schema v2**：record 72→73 字节尾部追加 `kind u8`；`CurrentSchema=2`；decode 白名单 {1,2}，v1 记录迁移恒 kind=夜行者；`MaxFileLength` 32+64×73=4704；golden fixture 与 layout 测试重钉；persistence 编排字段映射扩展。被否：独立 `ranged_mobs.bin`（被动牛先例是独立**族**；掷骨者与夜行者共享生成/灼烧/消失/掉落生命周期，同文件才能保持单一生命周期真源）。

**D5 协议 v43**：S→C 29 `ProjectileSpawn`（id/kind/dimension/pos×3/vel×3）、30 `ProjectileState`（id/pos×3）、31 `ProjectileDespawn`（id 列表），形状沿 hostile 三消息（计数上限、严格 ID 升序、按会话视野发布、latest-wins 镜像）；hostile Spawn/State record 尾部追加 `kind u8`（pure-append 纪律）；客户端 hostile 镜像带 kind 并分支呈现。下一空闲 ID 29 已核实（registry.go:188 注记）。

**D6 玩家弓与弹药**：拉弓复用既有 `Mining` 主输入位——**持弓时该位的服务端语义=拉弓**（该位本就是「持续主动作、语义由服务端按手持物品裁决」，v25 注记同源；无新增输入位、无新命令）。`bowState{slot,item,progressTicks}` 挂 `playerState`（瞬态不持久化），中断矩阵沿进食同形：松开=发射判定点；切格(槽,物品)失配、开容器/视野未就绪、受伤、死亡、复位=清零不发射。拉弓 <6 tick 松开不发射；6..19 tick 短档（伤害 2、初速低）；≥20 tick 满档（伤害 5、初速高）。发射原子结算：新增 `Inventory.ConsumeItem`（全背包 0..35 扫描取 1 的 copy-on-write，沿 `ConsumeRecipe` 形状）扣 1 箭 + 弓耐久 −1（0 损坏形态）+ 生成箭投射物；无箭不开弓（起始 tick 校验）。**持弓排除近战意图**（`playerCombatIntent` 跳过弓持有者）且不可采掘（`advanceMining` 入口守卫追加手持弓让位）；被否：右键拉弓（`Use` 位与开箱/进食共用，冲突面大于收益）。

**D7 物品与配方编号**：append-only 追加 `ItemBow`=62、`ItemArrow`=63、`ItemBone`=64、`ItemBrokenBow`=65，`ItemIDMax` 推进 66；堆叠弓 1、箭/骨 64；弓耐久 120、损坏形态入 `ItemBrokenForm`；显示名「弓/箭/骨头/损坏的弓」。`RecipeArrow`=25（上砾石下木棍→2 箭，2×2 个人网格可合成），配方枚举循环上界推进。**弓不配方**：获取=掷骨者 1/8 掉落；「纤维类原料」不存在于当前材料表，弓合成另立后续行（记入非目标）。`ItemBone` 本行只有掉落来源，骨头→骨粉配方归 B-39（战役 §8 已协调）。

**D8 呈现与音频**：frame TLV **tag 14** = 投射物段（96B avatar 实例布局：mat4 沿速度取向 + 纯色哨兵材质），Rust 侧新小 EntityPass（outline/crack 变体同构），`FRAME_TAG_MAX` 13→14；**client ABI 保持 v19**（零新导出面，沿裂纹 overlay 先例）。掷骨者本体走既有 tag 1 avatar 通道（`EntityKind` 不增值，Hostile=4 内部按 kind 分支几何/配色：骨白色躯干 + 深灰头部 + 持骨姿势，原创程序化）。音效 `CueBowRelease`=7、`CueProjectileHit`=8（本地确认边界纪律）。capture 新场景 `ranged-mob`（30→31，PinVolatile 钉住弹道相位）。

**D9 tick 阶段与死亡结算**：投射物推进插入敌怪阶段内部——`advanceHostileDistant` 之后、`settleHostileDeaths`/`settleDeaths` 之前（阶段序守卫测试同步），保证弹击致死同 tick 完成掉落与重生结算；玩家弓发射结算挂 `advanceActivePlayers`（进食同位段），投射物入队次 tick 起步。

**D10 第二类敌怪 AI/ECS 共享边界评估（战役硬性交付）**：结论=以 kind 字节共享 `hostileSet`/生成预算/灼烧/远离消失/死亡掉落机制，行为差异收敛为管理器侧 advisor（chase vs ranged）+ `applyHostileActions` 分支；完整 ECS 抽象被否（当前两族敌怪共享面已显式且测试锁定，第三族出现前重构无消费者）。评估全文进 change design.md，归档时在 backlog B-26 行回填结论。

**D11 版本矩阵与门禁**：协议 v43、`hostile_mobs` v2 唯二升版；根 `AGENTS.md` 与 `openspec/config.yaml` 版本矩阵收尾同步；全量门禁 `make test-race`（六模块）+ `make rust` + gofmt + 六模块 vet + `openspec validate --all --strict` + `make visual-check`（31 景）+ `make frontend-check`（如 items-all fixture 重生成）。

## 4. 非目标

- 不做第二类之外的敌怪、不做按黑暗生成（「服务端不计算光照」禁令在案，战役 §8）。
- 不做弓合成配方（纤维来源另立后续行）、骨头→骨粉配方（B-39）。
- 不做箭矢插块滞留态/拾取、不做弹道拖尾粒子（D-14 域）。
- 不做难度伤害倍率（B-11 显式不含）。
- 不动伙伴/被动生物行为；掷骨者不攻击牛。

## 5. 风险

- 阶段序重排（settleHostileDeaths 后移到投射物之后）触及既有阶段守卫测试——以阶段序测试显式重钉并论证零观察差异（弹击致死同 tick 是新语义，原顺序下不存在该输入）。
- 协议 record 尾部追加字节改变 hostile 批次上限（spawn 29→30B、state 37→38B），wire frozen 测试与上限测试同批重钉。
- 掷骨者后退无 A*，被墙挡住属可观察行为，规格按「直线后退意图」措辞，不承诺绕障。
