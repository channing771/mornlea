# 可放置火把

## 背景

第一夜生存批次（`docs/feature-backlog.md` A 节）的火把功能行。当前 main（`cc385d22`，协议 v27、engine ABI v7）上，夜间的唯一光源是客户端静态方块光与已放置的发光方块（`LightBlockID`，发光 15），但没有可合成、可放置、受支撑约束的五向火把；`core` 也没有任何方块发光/衰减的权威判定表——目前两套 switch 重复散落在 `internal/assets/blocks.go`（`Emission` 只识别发光方块、`LightAttenuation` 只识别流体），服务端后续的敌怪黑暗判定（夜行者功能行）没有可依赖的单一来源。A-01（PR #100）已合入格子合成系统（`authoritative-grid-crafting` 能力、recipe 表 1..13），火把配方可以在此基础上作为 `RecipeTorch`=14 追加。

## 目标

- 新增一个火把物品（`ItemTorch`=43，堆叠 64；`ItemIDMax` 43→44）与落地、四向墙面五个稳定方块形态（编号 62..66：62=落地、63..66=墙 +X/−X/+Z/−Z；`BlockIDMax` 62→67）。以上为最终锁定编号，无待集成裁决。
- 在 A-01 的格子合成系统上追加 `RecipeTorch`=14：煤炭位于木棍正上方的纵向两格形状，产出 4 个火把；形状可在 2×2 个人网格摆放，匹配走既有裁边与镜像规则；recipe 表 1..13 语义不变。
- 放置命中已加载、实心的合法支撑面：顶面命中生成落地形态，四个侧面生成对应墙面形态；底面、流体、非实心支撑与未就绪邻格拒绝，且拒绝不扣物品。
- 支撑被任何权威方块变化移除时，只复核本 tick 已变化位置的固定六邻居，在同一有界变化批次中把失去支撑的火把写成空气并生成一枚火把掉落，与原变化共享 revision/broadcast/存档。
- 火把零碰撞、非不透明、发光等级 14、非流体、不可放入水中。
- `core.BlockEmission` 与 `core.BlockLightAttenuation` 成为全仓唯一光源判定表；`internal/assets` 的两套 switch 收敛为转调。服务端生成判定（夜行者）与客户端 registry 都消费这一张表，不建第二套。
- mesh registry 条目追加 model 字段：entry 19 → 20 bytes（`model(u8)` 位于 offset 19，紧随 offset 18 的 `blockTopRaw`），条目上限 64 → 80（当前已注册 62 个方块加 5 个火把形态 = 67，必须越过 64）。Rust 以有限 model tag 为火把发出固定上界的原创窄柱/墙面倾斜 quad——quad 仍是 8 bytes、bit 63 仍空闲、走既有 terrain cutout pass；engine ABI v7 → v8。
- 火把外观用现有程序化 atlas 原创像素（窄木柄 + 暖色火芯），不引入任何外部图片。
- 新增 `torch-night` 无窗口 capture 场景并**自带 golden**：场景插入 `block-light-room` 之后、`materials-showcase` 之前（场景表内第 12 位），capture golden 从 19 张扩为 20 张（本变更显式更新基线并逐图人工复核；`workbench-crafting` 的 golden 缺口是 A-01 遗留的既有状态，不属本变更）。
- 伙伴防御清单保持拒绝：伙伴不得采掘火把相关目标之外的扩展、不得放置火把（本分支不扩 `mine`/`place` 语义）。

## 非目标

- 不做彩色光、手持光源、熄灭火把、火焰粒子、任意模型描述语言或光照缓存。
- 不改动 recipe 表 1..13 的既有语义，不注册木剑、石剑、铁剑、白床等其余批次物品/方块编号（recipe 15..18 与床方块归各自功能行，追加前查询这些 ID 保持稳定拒绝）。
- 不实现 bed model（model tag 6 保留给床，本分支 registry 出现该值即拒绝，床功能行实现）。
- 不补 `workbench-crafting` 的 golden PNG、不重生成其余 19 张既有 golden（除非火把登记改变了它们的可观察像素，且须逐图复核）。
- 不改协议 wire、区块 schema、世界 metadata、`companions.ai` schema、benchmark scenario、client ABI；不新增 GPU pass、instance pool 或动态资源；不新增协议命令（A-01 已锁 `MoveCraftingStack`/`TakeCraftingOutput`，放置沿用既有命令）。
- 不建立方块行为 interface / 层次化模型系统——两个真实消费者不足以支撑，见 design 的否决记录。
- 不建设服务端全场光照缓存（夜行者的黑暗判定将来用有界局部窗口，非本分支）。

## 影响

- `internal/core`：方块/物品枚举追加（`block.go`、`block_name.go`、`item.go`）、`BlockEmission`/`BlockLightAttenuation` 单一事实源、`PlaceableBlockAtFace`、`BlockDrop` 火把映射、`RecipeTorch`（`recipe.go`）。
- `internal/assets`：程序化火把纹理层、model tag 登记、发光/衰减 switch 转调 core。
- `internal/sim`：`executePlacement` 火把分支（`torchSupport`）、pending changes 上追加六邻居复核、火把掉落与既有 `drop` 通路、火把配方经既有网格匹配/取出路径生效。
- `internal/world`：支撑判定所需的 chunk 查询（`chunk.go`）。
- `internal/mesh` + `internal/nativeabi` + `engine/crates/mornlea_engine`：registry entry 20 bytes、80 entries、model offset 19、model dispatcher、`emit_torch`、ABI 版本 v8、C header（`engine/include/mornlea_engine.h`）与 Go 常量同步。
- `cmd/mornlea/capture.go` 及 capture 测试：`torch-night` 场景构造与场景表插入；golden 目录新增 `torch-night.png`。
- `AGENTS.md`/`CLAUDE.md`：只做「engine ABI v7 → v8」版本表述的最小同步（两份逐字节相同，`TestBaselineVersionsMatchCode` 转绿为准）。

## 兼容性与迁移

- 无存档格式、wire 或配置变化：协议 v27、chunk schema v9、世界 metadata v2、玩家 schema v7、`companions.ai` schema v4、client ABI v9、benchmark scenario v19 保持；火把方块编号只是既有方块数组的新取值，旧程序遇到未知方块编号按既有未来版本语义拒绝。
- engine ABI v7 → v8：registry entry 布局扩为 20 bytes 是唯一 ABI 语义变化；与二进制同一 release unit 的既有「同版本捆绑」纪律兜底，跨版本混装由 ABI 版本校验在握手层拒绝。
- 编号为 append-only 终值：火把方块 62..66 恒插在既有枚举末尾（`BlockIDMax` 哨兵之前），`ItemTorch`=43 同理；不重排任何既有编号。
- capture：`torch-night` 插入场景表中段，须按既有规则用显式基线更新重新生成受影响场景并逐图人工复核；其余既有 golden 逐字节不变（火把登记不改变其画面）。

## 延期与放弃

> 收尾时全文誊入未决项。
