# 权威投射物与远程敌怪

## Why

生存闭环收尾战役阶段一的第三行（积压表 B-23，依赖护甲行已交付）。当前战斗面只有近战：夜行者只会贴身挥击，玩家只有剑与护甲，没有任何远程威胁、投射物与远程武器；「第二类敌怪出现后的 AI/ECS 共享边界评估」是战役 §8 的硬性随行要求。本行补齐「战斗与生物纵深」：投射物权威域、一类远程敌怪（掷骨者）与玩家弓。

对照原版《我的世界》的缺口定位与串行链裁决见 `docs/superpowers/specs/2026-09-12-b23-ranged-projectiles-design.md`（2026-09-12 经确认通道批准）。

## What Changes

- `packages/server/sim/entity` 新建投射物权威域：`projectileSet`（ID 升序、全服上限 128、超限最旧先清）、两类弹种（骨刺=远程敌怪发射、箭=玩家弓发射）、固定步长弹道积分（与玩家/敌怪同一 `physics.FixedDelta` 纪律）、方块命中走既有 Rust DDA（`core.RaycastBlocks`）、实体命中为线段×AABB、命中结算复用护甲减免/耐久消耗/击退/`applyDamage` 唯一入口/`CombatHit` 私有确认；在飞投射物瞬态不持久化。
- `packages/server/sim/entity` 敌怪族引入 kind 分支：`hostileState` 追加 `kind` 字节（0=夜行者、1=掷骨者），共享生成预算、灼烧、远离消失与死亡掉落结算；生成候选按确定性 hash 以 2:1 比例分派 kind，近玩家 48 格上限夜行者 8、掷骨者 4（全服共享 64 上限）；掷骨者行为：目标 >14 格 A* 接近、6..14 格保持、<6 格直线后退，40 tick 射击冷却 + 视线无遮挡 + 确定性散布发射骨刺；死亡掉落骨头 0..2 与弓（1/8 确定性）。
- `packages/server/server`：`hostileManager` 管理器侧 advisor 分派（chase vs ranged）、掷骨者射击意图（含 LOS 检查）经 `HostileAction` 入域、投射物三消息按会话视野发布。
- `packages/shared/network` 协议 v42→v43：S→C 29/30/31 `ProjectileSpawn`/`ProjectileState`/`ProjectileDespawn`（沿 hostile 三消息形状：计数上限、严格 ID 升序、按会话订阅发布）；hostile `Spawn`/`State` record 尾部追加 `kind u8`（pure-append 纪律）。
- `packages/server/storage/hostile` schema v1→v2：record 72→73 字节尾部追加 kind；`CurrentSchema=2`；decode 白名单 {1,2}，v1 只读迁移恒夜行者；golden fixture 重钉。
- `packages/shared/core`：物品 append-only 追加 `ItemBow`=62（耐久 120、损坏形态 65）、`ItemArrow`=63、`ItemBone`=64、`ItemBrokenBow`=65，`ItemIDMax` 62→66；`RecipeArrow`=25（上砾石下木棍→2 箭，2×2 可合成）；新增 `Inventory.ConsumeItem`（全背包原子取 1）。
- `packages/server/sim/entity` 玩家拉弓状态机：持弓时 `Mining` 主输入位语义=拉弓（无新增输入位与命令），拉弓 <6 tick 不发射、6..19 短档（伤害 2）、≥20 满档（伤害 5）；发射原子结算扣 1 箭 + 弓耐久 −1 并生成箭投射物；中断矩阵沿进食同形（松开=发射判定点；切格、开容器/视野未就绪、受伤、死亡清零不发射）；持弓排除近战意图且不可采掘。
- `packages/client`：hostile 镜像带 kind、投射物镜像（latest-wins + 插值）、frame TLV tag 14 投射物段（96B avatar 实例布局，mat4 沿速度取向，纯色哨兵材质）、Rust 新增投射物 pass（`FRAME_TAG_MAX` 13→14，零 client ABI 升版，沿裂纹 overlay 先例）、掷骨者 avatar 几何/配色分支（原创程序化）、弓/箭/骨头的物品图标。
- `packages/client/cmd/mornlea/capture`：新场景 `ranged-mob`（30→31 景）。
- `packages/audit`：随行第二类敌怪共享边界评估落档（kind 字节共享机制，完整 ECS 被否），并按需补域守卫。

## 契约与版本影响

- 协议 v42→v43（S→C 追加 29/30/31 + hostile record 尾部 1 字节；pure-append，旧版拒绝语义不变）。
- `hostile_mobs` schema v1→v2（record 尾部 1 字节；v1 只读迁移恒 kind=夜行者，首次保存升 v2）。
- engine ABI v11、client ABI v19、玩家 schema v9、区块 schema v9、世界 metadata v6、`companions.ai`/`passive_mobs` schema、benchmark scenario v23 均不变（frame TLV tag 14 为帧内追加，无新导出面）。
- golden：新场景 `ranged-mob` 1 张；`items-all` 前端部件基线随四件新物品重生成；其余 30 张世界场景与既有部件基线零差异。

## 用户可观察结果

- 夜间出现掷骨者：保持距离投掷骨刺，被击中有伤害红边与音效，护甲可减免。
- 玩家击杀掷骨者掉骨头与（概率性）弓；弓用于远距离射击敌怪与牛，命中出 CombatHit 音效与命中标记。
- 箭可用砾石+木棍合成；拉弓时间越长伤害与初速越高，松开发射。
- 和平难度下掷骨者不生成（既有生成入口短路自动覆盖）。

## 非目标

- 不做第二类之外的新敌怪；不做按黑暗生成（「服务端不计算光照」禁令在案）。
- 不做弓的合成配方（材料表无纤维类原料，弓获取=掷骨者掉落；弓合成另立后续行）。
- 不做骨头→骨粉配方（B-39）；`ItemBone` 本行只有掉落来源。
- 不做箭矢插在方块上的滞留态与拾取；不做弹道拖尾粒子（D-14 域）。
- 不做难度伤害倍率（B-11 显式不含）；掷骨者不攻击被动牛。
- 不做伙伴使用弓或投射物。

## 受影响的包与文档

`packages/shared/core`、`packages/shared/network`、`packages/server/sim`（entity）、`packages/server/server`、`packages/server/storage`（hostile）、`packages/client`（client/render/assets/audio/cmd app+capture）、`packages/engine/crates/mornlea_client`（frame 解码与投射物 pass）、`packages/audit`、根 `AGENTS.md` 版本矩阵、`openspec/config.yaml`、`docs/notes/progress.md`、`docs/feature-backlog.md` B-23/B-26 行回填。

## 延期与放弃

（实现期登记，暂空）
