# B-33 树苗与橡树再生（proposal）

## 背景

`deterministic-tree-generation` 让世界生成阶段产出确定性橡树，`natural-grass-seeds` 让种子有了自然入口，B-02 让水可搬运，但木材仍只有一次性来源：橡树被采掘后不会再生，玩家无法在自家门口种树，「自给家园」的木材闭环仍然缺口。本 change 是发布列车第 8 行（依赖 B-02，已完成），只交付一种树苗与其有界确定性生长，不建设通用植被系统。

## 目标

- 新增树苗方块与物品（编号 append-only）：树苗是交叉斜面植物，无碰撞、可穿过、可瞄准、不完全遮光。
- 树叶按冻结 salt 以确定性 `1/8` 判定额外掉落 1 个树苗（树叶自身掉落语义不变）。
- 树苗可种在泥土或草方块上方；下方支撑消失或被流动水冲毁时树苗按物品掉落，容量不足原子拒绝。
- 树苗经既有随机 tick 阶段有界确定性生长为橡树：露天、空间足够且判定命中时，按新增 engine ABI 函数给出的有界方块列表写入，可跨区块且全有或全无。
- 树形几何由 Rust 单一真源计算（engine ABI v10→v11），Go 只做空间校验、原子写入与变更登记。

## 非目标

- 不做树叶衰减/腐烂、树苗生长阶段方块、骨粉催熟树苗、多种树、苹果掉落、伙伴植树、通用植被系统与跨区块通用事务抽象（B-21）。
- 不改变世界生成树形与任何既有 golden 字节，不扫描、迁移或回填已保存区块。
- 不新增协议消息或命令（种植复用既有 `PlaceBlock`），不改变 player/chunk/metadata schema、`companions.ai`/`hostile_mobs`/`passive_mobs` schema、client ABI 与 benchmark scenario。

## 用户可观察结果

- 打掉树叶偶尔（`1/8`）在树叶物品之外多掉一个树苗。
- 手持树苗对泥土或草方块上方使用可种下；树苗是贴地交叉斜面植物，玩家可穿过。
- 树苗露天、上方空间足够且下方仍是泥土/草时，过一段时间长成橡树（原木 + 树叶），树干底格就是原树苗格。
- 上方被遮挡或空间不足时树苗不生长；下方支撑被挖掉时树苗掉回物品；被流动水冲毁时也掉回物品。

## 受影响的包或文档

- `packages/shared/core`（方块/物品编号、谓词、名称、掉落表）、`packages/shared/worldgen`（新 ABI 桥）、`packages/shared/companion`（放置豁免）、`packages/shared/nativeabi`（新 ABI 包装）。
- `packages/server/sim/entity`（放置/采掘/耐久/伙伴防御）、`packages/server/sim/realm`（随机 tick 生长、支撑清理、流体冲毁）、`packages/server/fluid`（可替换表镜像）。
- `packages/engine`（worldgen 树形函数、`ffi.rs`、头文件、quad/fluid 常量镜像）、`packages/client/assets`（程序化纹理层、植物材质集合、图标回退）、`packages/client/mesh`（植物材质常量）、`packages/client/cmd/mornlea/capture`（新场景与 golden）、`packages/audit`（依赖边与版本基线）。
- 主规格同步：新建 `oak-sapling-regrowth`；MODIFIED `authoritative-mining`、`tool-durability`、`authoritative-fluid`、`authoritative-farming`、`rust-engine-worldgen`、`deterministic-tree-generation`、`visual-verification`。

## 兼容性

- 方块/物品编号 append-only；无协议、存档 schema 或命令变更，区块 palette 天然承载新编号，旧存档可直接加载。
- engine ABI v10→v11（新增 `mornlea_tree_blocks` 与版本常量）；`MGW1` 请求布局不变，世界生成输出逐格不变，既有 worldgen golden 字节不变。
- benchmark scenario 保持 v22（无世界生成变化）；视觉基线只新增 `sapling-growth` 场景，既有 golden 逐字节不变。
- 回退即 revert 本分支：编号与 ABI 版本号均为追加式，无数据迁移；写入过树苗/橡树的存档在旧程序里按未知方块既有语义处理。

## Capabilities

### New Capabilities

- `oak-sapling-regrowth`: 树苗的稳定编号、种植支撑、树叶掉落、随机 tick 有界确定性生长、跨区块原子写入、环境移除与植物呈现语义。

### Modified Capabilities

- `authoritative-mining`: 树叶在自身掉落之外按冻结判定额外掉落树苗，树苗 `1` tick 采掘并掉落自身，掉落容量语义与既有原子拒绝一致。
- `tool-durability`: 树苗加入第三类零耐久豁免的同类，豁免数量由三变四。
- `authoritative-fluid`: 树苗加入流动水的可替换目标，被冲毁时按物品掉落并沿用作物冲毁的容量原子拒绝与重试语义。
- `authoritative-farming`: 随机 tick 阶段新增树苗生长消费者，并给出该分支的每 tick 有界读取上界。
- `rust-engine-worldgen`: 新增运行时树形 ABI 函数与其输入校验契约（engine ABI v11），世界生成既有行为不变。
- `deterministic-tree-generation`: 明确运行时种植的橡树不属于世界生成，不得改变、迁移或回填世界生成结果。
- `visual-verification`: 新增 `sapling-growth` 无窗口场景，既有阈值与其它 golden 不变。

## 延期与放弃

以下事项经任务评审、整分支终审与 scoped 复审确认，均为 Minor、不可达路径或独立后续行，不阻塞本 change：

- **文档刷新独立成行**：`docs/architecture.md`、`README.md`/`README.en.md`、`docs/notes/go-rust-division.md`、`docs/notes/lan-server.md`、`docs/notes/visual-verification.md`、`packages/tools/perfcheck/compare.go` 等处的版本描述同时滞后于协议 v38、client ABI v18 等多个版本，属独立文档刷新任务；权威矩阵（根 `AGENTS.md`、`openspec/config.yaml`）已随本 change 更新。
- **主规格场景计数存量滞后**：`visual-verification` 主规格部分 Requirement 仍写「27/28 个场景」，早于本 change 前代码的 29 景即已失真；本 change 的 delta 只新增 `sapling-growth`（29→30），存量计数修正留待主规格维护。
- **呈现细化候选**：树苗掉落物沿用可放置方块的 1/4 mini-cube 薄片分支，物品图标经 `blockItemTexture` 回退（透明源像素为黑，与玻璃图标同路径）；如需扁平 cutout 图标或薄片掉落，属独立呈现裁决。
- **不可达路径**：`mornlea_tree_blocks` 对 X/Z 距 i32 边界 ±2 的根坐标硬拒并触发 Go 桥 panic，需 `|X|≈2^31` 的存档才可达；生长写入的回滚分支被 Y 范围与 ChunkReady 双重前置校验挡在公共缝之外，无直接测试（Rust 侧 `0xAA` canary 覆盖坏输入原子性）。
- **测试强度**：Rust `quad.rs` 的植物集合测试断言常量而非字面量 `164`，字面量由跨语言真实 mesher pin 兜底；`nativeabi` 成功路径「尾字节不变」断言因缓冲零初始化而空转（同性质由 Rust canary 承担）。
- **伙伴语义显式边界**：伙伴采掘树叶只产出既有 `ItemLeaves`（不触发树苗判定），伙伴植树在放置注册表中显式豁免——两者均已写入主规格契约，非缺陷。
- **注册表注释存量**：`packages/engine/crates/mornlea_engine/src/input.rs` 的「今天是 85 条」注释早于本 change 即已陈旧，未在本 change 清扫。
