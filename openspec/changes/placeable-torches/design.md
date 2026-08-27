# 设计

## 数据所有权与时序

- `internal/core` 是方块/物品属性和放置映射的**唯一事实源**：`BlockEmission`（发光判定）、`BlockLightAttenuation`（天空光额外衰减）、`PlaceableBlockAtFace`（物品×命中面 → 方块形态）、`BlockDrop`（火把掉落回一个火把）都落在 core；`internal/assets` 的 `Registry.Emission`/`LightAttenuation` 只做转调（本次删除 assets 内两处重复 switch，见 `internal/assets/blocks.go` 现 `Emission`/`LightAttenuation` 实现）。
- `internal/sim` 是放置与支撑失效的唯一执行者：`executePlacement`（`internal/sim/engine_placement.go`）对火把物品先经 `torchSupport(block, pos) (supportPos, bool)` 校验支撑，再走既有世界写入 + `recordChange` + 扣物品原子路径；不建立 block behavior interface（两个消费者不足以支撑抽象，见否决记录）。
- 支撑失效复核挂在**本 tick 已变化位置**上：`finishChanges`（`internal/sim/engine_changes.go`）之前对 pending block changes 的位置排序去重，逐个检查精确六邻居；邻居是火把且 `torchSupport` 指回该变化格 → `recordChange` 写成空气 + 既有掉落 append。`recordChange` 是权威 tick 内方块变化的唯一汇聚点（流体入队也挂在这里），在它之后、`finishChanges` 之前复核一次即可覆盖全部写者。火把零碰撞、不可能成为支撑源，新移除的火把不会被循环传播 → 不需要递归队列，边界严格 = 六邻居 × 单级。
- 掉落槽容量不足的取舍：移除火把前先 `PrepareDrop` 预检，容量不足时**整体保留火把**（不写空气、不掉落、无半结算），该格下一次权威变化重新触发复核自愈。与采掘完成路径的 `RejectDropCapacity` 同构——宁可让火把多停留一拍，也不产生「方块消失而物品无声丢失」的半结算；静默丢物品在任何路径都是硬失败语义，悬空火把只是可自愈的瞬态。邻居所在区块未加载时本轮跳过：火把所在区块必然已就绪才会被放置，未就绪意味着整列已随区块卸载，没有可复核的权威状态。
- 灯光能量（发光 14）经 mesh registry 快照送 Rust，与 15 级发光方块同路径；服务端将来夜行者的黑暗判定读同一张 `core.BlockEmission`，不再建服务端光源表。
- 火把配方完全复用 A-01 的格子合成闭环：core 的 recipe 表追加 `RecipeTorch`=14 后，网格匹配（裁边 + 镜像位）、产物取出、回收不变量、`CraftingState` 同步全部走既有路径；sim 不为火把写任何合成专用分支。

## 方向映射（冻结）

| 命中面 | 形态 | 支撑格（相对火把位置 `pos`） |
|---|---|---|
| `BlockFacePosY`（顶面） | standing | 下方一格 |
| `BlockFaceNegX` | wall −X | `pos + (1,0,0)`（`face.Opposite()`） |
| `BlockFacePosX` | wall +X | `pos + (-1,0,0)`（`face.Opposite()`） |
| `BlockFaceNegZ` | wall −Z | `pos + (0,0,+1)`（`face.Opposite()`） |
| `BlockFacePosZ` | wall +Z | `pos + (0,0,-1)`（`face.Opposite()`） |
| `BlockFaceNegY`（底面） | 拒绝 | — |

墙面火把的**形态名 = 命中面名**（火把贴在支撑块的哪个侧面），而火把的支撑格位于 `face.Opposite()` 方向（`BlockFace.Opposite()` 已存在于 `internal/core/block.go`）：命中 +X 面 → 火把在支撑块 +X 侧、支撑在火把的 −X 侧。墙外观向远离支撑的方向倾斜（Rust 几何侧），碰撞恒空。

## 编号契约（最终锁定）

append-only 契约以当前 main（`cc385d22`）的枚举末尾为准，无集成期重排：

- 方块（`BlockIDMax` 62 → 67）：既有 0..61 不动；火把落地=62、墙+X=63、墙−X=64、墙+Z=65、墙−Z=66。
- 物品（`ItemIDMax` 43 → 44）：`ItemTorch`=43（`ItemPoisonousPotato`=42 之后）；原料 `ItemStick`=37、`ItemCoal`=5 为既有编号。
- 配方：`RecipeTorch`=14（recipe 表 1..13 既有不变；15..18 留给剑与床功能行，追加前查询稳定拒绝）。
- 「0..`BlockIDMax`-1 全注册」是既有不变量（mesh snapshot 烘焙全部已注册方块），五个火把形态各自登记完整 registry 条目。

## mesh registry wire（v8）

19 → 20 字节（`blockTopRaw` 之前的布局不变，model 追加在末尾）：

```
id(u16) opaque(u8) emission(u8) material[6](u16) fluidHeight(u8) lightAttenuation(u8) blockTopRaw(u8) model(u8)
└─0..1──┘ └─2────┘ └─3────────┘ └─4..15────────────┘ └─16──────────┘ └─17──────────────┘ └─18─────────┘ └─19───┘
```

- 基线校准：当前 main 的 entry 已是 19 bytes（`blockTopRaw` 占 offset 18，ABI v7 由短方块顶面扩展占用）；条目上限已在更早提交的 v7 期内提前升至 80（62 个既有注册方块 + 5 个火把形态 = 67 越过 64 所迫，80 同时给床等多形态方块留余量），不属本次升版记账。本变更只做两件事：加第 20 字节 `model`（offset 19），并把 engine ABI 升到 v8。
- model 封闭集合：0=默认（无模型覆写：cube/短方块/流体/植物仍走既有判定，植物继续按 material 区间识别）、1=火把落地、2..5=火把墙 +X/−X/+Z/−Z（与方块编号 63..66 同序）、6=床（保留，出现即拒绝）、其余值未知拒绝。火把是 model tag 的第一个消费者，故 0 语义取「默认」而非「cube」，避免为既有四条几何路径重复造 tag。
- Go：`nativeRegistryEntryBytes = 2 + 1 + 1 + 6*2 + 1 + 1 + 1 + 1`；`nativeMaxRegistryEntries` 64 → 80；`nativeMaxRegistryWords` 随上限变为 2 words/行、`maxNativeInputBytes` 随之更新（文档注释同步）。
- Rust：`engine/crates/mornlea_engine/src/input.rs` 的 `REGISTRY_ENTRY_BYTES`、`MAX_REGISTRY_ENTRIES`；`greedy` 增加 model dispatcher；`encode` 端与 Go 解包各按 20 字节布局。
- `mornlea_engine_abi_version()`（`engine/crates/mornlea_engine/src/ffi.rs`）、C header（`engine/include/mornlea_engine.h`）与 Go `internal/nativeabi` 常量 → 8。
- 容量守护：既有 `TestNativeAcceptsRegistryAtGoCapacity`（喂满上限跨 FFI）与 Go 侧「上限 + 1 项拒绝」测试改写为 80；`TestRegistryCapacityCoversEveryRegisteredBlock` 继续守住「每个已注册方块都有条目」。
- 吞吐影响：registry 输入字节随 20-byte 条目与 80 项上限增长；benchmark 数值只记录，不改变退出状态。

## Rust 几何

- standing：竖直居中窄柱（两片双面 quad 或四片薄片，按固定上界）；wall：贴近对应支撑面、向远离支撑方向倾斜的窄柱。坐标全部限制在本格内；双面（正背各一片，惯用 plant 的 face 编组）；alpha cutout 走既有 terrain pass；不参与 greedy merge（与短方块、植物同一豁免路径）。
- light/AO/材质均来自 registry 与邻域既有规则（无火把专属光照公式）；quad 仍是 8 字节、bit 63 空闲。

## 纹理

程序化 material builder 直接画：竖向窄木柄（棕）＋顶部暖色火芯（黄/橙）；16×16、alpha 0/255、非空、与既有层不同。五种形态共用同一层。禁止引入外部 PNG。

## 放置拒绝路径（不扣物品）

1. 目标格未加载（既有「未就绪」语义）→ 拒绝。
2. 命中面为 `BlockFaceNegY` → 拒绝。
3. 支撑格为空 / 非实心 / 为流体 → 拒绝。
4. 目标格为流体或不可替换 → 拒绝。
5. 目标格玩家占位 → 拒绝。
6. 伙伴路径根本不进 fire 分支（防御清单保持拒绝）。

## 伙伴

本分支不扩伙伴 scope：`internal/companion/plan_types.go`、`internal/sim/companion_placement.go` 的防御清单保持拒绝火把；mine 的容器/多掉落限制不变。

## 兼容性与迁移

- chunk schema v9 结构不变（火把只是既有方块数组里的新取值）；协议 v27、metadata v2、玩家 schema v7、`companions.ai` v4、client ABI v9、benchmark scenario v19 全部不变；不新增协议命令（A-01 已锁 `MoveCraftingStack`/`TakeCraftingOutput`，火把放置沿用既有放置命令）。
- engine ABI v7 → v8：只有 mesh registry entry 布局与条目上限变化；release unit 纪律（`libmornlea_engine` 与二进制同版本捆绑）不变。主规格 `short-block-presentation` 的「engine ABI 升版」条目仍描述 v6→v7 那次历史升版，其版本句与本次 v8 的关系在归档收尾时按仓库惯例裁决（见 ledger 待办）。
- `AGENTS.md`/`CLAUDE.md`：只机械同步「engine ABI v7 → v8」的版本表述（以 `TestBaselineVersionsMatchCode` 转绿的最小集合为准），两份逐字节相同；其余基线内容不变。
- capture：场景表插入 `torch-night`（`block-light-room` 之后、`materials-showcase` 之前，表内第 12 位，总项数 20 → 21）；本变更经显式基线更新写入 `torch-night.png` 一张 golden（19 → 20 张），逐图人工复核；`workbench-crafting` 的 golden 是 A-01 遗留缺口（原批次集成任务已取消），本变更不补。

## 验证

- focused：`go test ./internal/core ./internal/assets ./internal/mesh ./internal/nativeabi ./internal/sim -race -count=1`、`go test ./internal/world -race -count=1`、`make rust`、Rust `cargo test --workspace --locked`。
- 全量：`scripts/agents/gates.sh`（全仓 race、vet、gofmt、archcheck、OpenSpec strict）；benchmark/golden 数值只记录，真实 I/O 与报告完整性错误仍硬失败。
- capture 场景：`torch-night` 经既有无窗口 capture 运行（不创建前台窗口），场景内像素断言（近亮远暗、透明边缘）+ golden 双阈值比对。
- 版本基线：`internal/archcheck` 的 `TestBaselineVersionsMatchCode` 必须转绿。

## 被否决的替代方案

- **block behavior interface / 层次化模型**：只有火把一个多形方块的消费者，抽象成本高于价值；火把的形态→支撑映射收敛到单一 `torchSupport`，后续床由床功能行自行实现（若统一抽象出现第二个并发消费者再评估）。
- **infinite model tag / 数据驱动模型**：model 是固定 tag，不是数据驱动格式；只有多个新增形态无法由有限 tag 清晰表达时才升级。
- **复用 offset 18 的 `blockTopRaw` 表达火把高度**：火把需要的是「形态 + 贴面倾斜」而非「顶面下沉」，短方块路径改不出墙面形态，且两条语义塞进一个字节会让域校验失效。
- **服务端全场光照缓存**：唯一消费方是夜行者黑暗判定，批次设计已定「半径 14 固定局部窗口 + 预分配 scratch」；全场缓存是本变更的显式非目标。
- **火把级联移除递归**：火把零碰撞、不能支撑自己/他人（支撑格必须实心），递归只会放大一次爆炸而不改变结果；六邻居单级复核就是闭包。

## 裁决记录

历史裁决（旧基线 `b2115a64`，产物已留档重写，仅作背景）：

- **Q1（2026-08-25，用户选 A）**：先重建共享契约预留提交；A-02 完成后推分支开 PR 但不合并，由 A-06 统一合流。——已被 2026-08-27 裁决取代：A-06/A-07 取消，职责回收各功能行，A-02 从当前 main 直接重做并自带全部职责。
- **Q2（2026-08-25，用户选 A）**：本分支负责 engine ABI 实际升版并对 `AGENTS.md`/`CLAUDE.md` 做仅版本表述的最小同步。——精神保留，升版数值改为 v7 → v8。
- **A-02-approval（2026-08-25T10:39:28Z，用户批准）**：bounded 短设计获准，进入 subagent-driven 开发。——针对旧基线；新基线的执行以本修订后的产物为准。

当前裁决（2026-08-27，用户）：

- **重做基线**：原 change 产物写于 A-01 合流前的旧基线（协议 v26、engine ABI v6→v7、批次预分配编号、配方归 A-01、golden 归 A-07），全部 stash 留档；从当前 main（`cc385d22`，协议 v27、engine ABI v7）重写，编号为最终锁定值。
- **golden 与配方自带**：A-06/A-07 已取消；`torch-night` 场景与 golden（19 → 20 张）由 A-02 自带；火把配方由 A-02 在 A-01 格子合成系统上追加（`RecipeTorch`=14）。
