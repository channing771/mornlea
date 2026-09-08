# B-02 可搬运水源（proposal）

## 背景

`authoritative-fluid` 交付时显式不做水桶与无限水源规则（`archive/2026-08-19-authoritative-fluid/proposal.md` 非目标），`authoritative-farming` 因此忠实带有约束「无水桶 + 干耕地不生长 ⇒ 农业只能在天然水体水平切比雪夫 4 格内进行」。本 change 解除该约束的前半段：玩家可舀取、搬运、倒出水源。

## 目标

- 空桶/水桶双物品：`ItemEmptyBucket=55`、`ItemWaterBucket=56`，限 1 堆叠、无耐久、非工具/非食物、`ItemPlacement` 永不映射流体。
- 空桶配方 `RecipeBucket=20`：左中/右中/底中 3 铁锭 → 1 空桶（裁边后 3×2，镜像自等价）。
- 双显式命令 `CollectWater`/`PlaceWater`（C→S ID 16/17，协议 v37→v38）：只取源、只放源，同 tick 原子完成世界写与背包原格互换，任一拒绝零副作用。
- 经典无限水：空气/流动格水平四邻 ≥2 源时升源，优先级低于垂直流动、高于等级扩散，复用既有预算队列与确定性全序。
- 伙伴对流体显式拒绝（采掘与放置双侧 `IsFluid` 守卫），破除对「流体无掉落」的巧合性依赖。

## 非目标

- 不做喝水、熔岩/流动水收取、发射器、伙伴取放、新方块、Rust ABI 升版。
- 不改变耕地干湿判定规则本身（4 格/同层或上一层口径不变）；可搬运水只是另一种流体来源。
- 不改变 HUD 上传布局与 benchmark scenario（图集加列只动层号，不动配额）。

## 用户可观察结果

- 手持空桶对源水执行取水：源变空气，空桶变水桶；手持水桶对空位执行放水：落点变源，水桶变空桶。
- 两源夹一空格（水平四邻）时空格变源，可持续取水。
- 远离天然水体的耕地可用桶装水灌溉至湿并种植。

## 受影响的包或文档

- `packages/shared/core`（物品/配方）、`packages/shared/network`（protocol/codec，协议 v38）、`packages/server/fluid` + `mornlea_engine::fluid_eval`（无限水，双侧同步）、`packages/server/sim/entity` + `server`（取/放事务与接线）、`packages/client`（图标/cue/`bucket-pond` 场景）。
- 主规格同步：`authoritative-fluid`（ADDED 无限水）、`authoritative-crafting`（MODIFIED 二十条配方）、`authoritative-grid-crafting`（ADDED 空桶配方）、`companion-world-actions`（MODIFIED 流体显式拒绝）。
- 协议 v37→v38；存档 schema、engine/client ABI、benchmark scenario 均不变。

## 兼容性

- 物品/配方/命令 ID 均为 append-only；旧客户端登录被版本拒绝（既有语义）。
- 无限水改变水平双源夹缝的平衡态：旧存档加载后按重扫收敛到新平衡态（重扫不动点论证随本 change 更新，见 design）。
