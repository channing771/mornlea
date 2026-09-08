# B-02 水桶设计（2026-09-08）

状态：brainstorming 已确认（A 双命令），待写 OpenSpec change 前的输入。

## 1. 背景与目标

- 发布列车队首：`B-04` 已完成，`B-02` 解锁 `B-33`。当前农业只能在天然水体水平切比雪夫 4 格内进行。
- 目标：空桶/水桶双物品，只取源、只放源，经典 MC 无限水（水平四邻 ≥2 源则升级为源），全链路权威确定。
- 非目标：喝水、熔岩/流动水收取、发射器、伙伴取放、新方块、Rust ABI 变更。

## 2. 架构与数据所有权

- Go 持有全部权威；Rust 零改（engine/client ABI 不动）。
- `core` 只加物品编号与谓词；`fluid` 加无限水纯函数；`sim` 做原子事务；`network` 加 2 kind。
- `ItemPlacement` 永不映射流体；取水用新 `CollectTarget`（仅源），放水复用穿水命中+贴面落点。
- 双桶限 1 堆叠、无耐久、非工具/非食物，原格互换，不做分堆拆分。

## 3. 组件与触碰面

- `packages/shared/core/item.go`：`ItemEmptyBucket=55`、`ItemWaterBucket=56`，`ItemIDMax` 55→57。
- 配方：空桶 1 条（3 铁锭 V 形：3x3 左中/右中/底中，`RecipeBucket` 取下一空闲号），水桶不可合成。
- `packages/shared/network/protocol|codec`：`CollectWater`/`PlaceWater`（序号+Yaw/Pitch，目标权威射线反推），`RejectReason` 追加非源/落点被占/桶态错，协议 v37→v38。
- `packages/server/fluid` + `sim/realm|entity`：取/放事务各一文件，复用 `recordChange` + 湿度同 tick 重判；无限水复用现有预算队列。
- `packages/client`：HUD 程序化图标 2 格、复用 B-32 splash cue、`bucket-pond` 一景；伙伴防御清单加桶拒绝。

## 4. 数据流

- 取：选中空桶 + 命中源 + 在距内 + 就绪 → 同 tick（源→空气 + 空桶→水桶）→ 邻流体入队 + 湿度重判。
- 放：选中水桶 + 贴面落点为空气/流动水（源格拒绝）+ 就绪 → 同 tick（落点→源 + 水桶→空桶）→ 入队 + 湿度重判。
- 无限水：流体阶段后，空气/流动格水平四邻 ≥2 源则升源（不对角），走 `recordChange` 并链式触发湿度；超预算按 `(dueTick,ChunkKey,y,z,x)` 全序顺延。

## 5. 错误处理

- 任何拒绝零写入零扣料；覆盖流动水合法，覆盖后下游失撑按既有规则消失。
- 成功序号递增才发 cue；桶命令与采掘互斥（当 tick 抑制采掘）；伙伴直接拒绝。

## 6. 测试与门禁

- 单测：编号/堆叠/三表否定、配方、取/放矩阵、无限水四邻/对角、预算顺延确定性。
- 集成：Memory/TCP parity、湿度联动、重启重扫收敛。
- 门禁：受影响包 `-race`、audit、`visual-check`、`openspec validate --all --strict`。

## 7. 被否决方案

- B 单 `UseBucket`：省 kind 但分叉多、拒绝模糊。
- C 复用 `PlaceBlock`：混入固体校验，取水仍需新通道，破坏现有守护意图。

## 8. 补齐需求清单（给 OpenSpec proposal 用）

1. 物品编号与堆叠语义；2. 空桶配方形状与编号；3. 双命令 wire 与拒绝表；4. 取/放射线谓词与触及距离；5. 无限水邻域定义与预算；6. 湿度链式触发；7. HUD 图标与 cue；8. capture 场景；9. 伙伴拒绝；10. 版本矩阵（协议 v38，其余不动）。
