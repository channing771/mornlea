## Context

第一夜生存批次原设计要求五条功能线先在共同基线上冻结 append-only 共享契约提交（编号 / 协议消息 / 有限模型 tag），功能分支从该 SHA 创建；该契约与五条功能分支已随开发机迁移丢失，2026-08-25 控制会话裁决 A-01..A-05 全部从当前 `main` 重做。当前 A-02（火把）与 A-04（夜行者）已各自从 `main` 认领并创建 worktree，均未重建共享契约；三线的认领备注已收敛到同一协调基准：**各线 append-only 追加自己的编号段，分支内临时编号不构成稳定契约，编号与消息终值由集成任务 A-06 按固定合并序（crafting → torches → swords → nightwalker → bed）锁定，版本基线与 golden 由 A-07 独占**。本设计在该基准下给出 A-01 的实现决策。

当前 `main` 注册表末端：物品 `0..36`（`ItemIDMax=37`）、方块 `0..44`（`BlockIDMax=45`）、配方 `1..11`（聚合单输入 `CraftingRecipe`）、Play C→S 编号 `0..13`（`TillSoil=13`）、S→C 编号 `0..20`（`PlaceBlockSucceeded=20`）、协议 v26。批次设计为全部编号预留的终值（Item `37..47`、Block `45..58`、Recipe `12..18`、C→S `14/15`、S→C `21..25`）与这些末端精确衔接——A-01 按批次序追加自己的段即可落在终值上。

## Goals / Non-Goals

**Goals:**

- 用服务端权威的个人 2×2 / 工作台 3×3 实物网格替换 recipe-click 自动合成，交付 recipe `1..13` 形状表与全部网格语义。
- 「网格随时可无损装回背包」成为被证不变量，关闭、断线、死亡三条生命周期路径零丢物。
- 与 A-02/A-04 的并行改动保持 append-only 可合并，不触碰版本号、golden 与基线文档。

**Non-Goals:**

- 见 `proposal.md` 的 Non-Goals；设计层面补充：不建通用 slot container 抽象、不建通用矩阵/形状匹配包、不引入鼠标手持栈状态、不重建批次共享契约。

## Decisions

### D1：无共享契约下的编号与消息策略

- 追加 `ItemStick=37`、`ItemWorkbench=38`（`ItemIDMax → 39`）、`WorkbenchID=45`（`BlockIDMax → 46`）、recipe `12`（木棍）、`13`（工作台），全部按批次终值落位，紧贴既有哨兵之前。
- 新消息 in-branch 临时编号：C→S `MoveCraftingStack=14`、`TakeCraftingOutput=15`，S→C `CraftingState=21`。批次终值是 `MoveCraftingStack=7`（复用 `CraftRecipe` 释放的编号）、`TakeCraftingOutput=14`、`CraftingState=21`；`MoveCraftingStack` 的终值化与 `network.CraftRecipe` 类型删除都归 A-06，本分支不重排任何既有编号。
- 协议版本号保持 v26。分支内客户端与服务端同构建，握手一致；v27 升版、v26 拒绝与 `19:20` scenario 迁移归 A-07。
- 被否决替代：由本线重建共享契约提交并要求 A-02/A-04 rebase——三线已各自 fork 且归属裁决未下，强制 rebase 违反「已认领行不得抢」的认领纪律；append-only 段 + 固定合并序已足以把冲突压缩到 registry/codec 的少量行。

### D2：数据形状与所有权

```go
const CraftingGridSlots = 9

type RecipePattern struct {
    Width, Height uint8
    Cells         [CraftingGridSlots]ItemID
    Output        ItemStack
    Mirror        bool
}

type CraftingGrid struct {
    Size  uint8 // 2 或 3
    Slots [CraftingGridSlots]ItemStack
}
```

- `CraftingGrid` 只存在于 `sim` 的 `playerState` 与网络镜像，MUST NOT 进入 `storage.StoredPlayer` 或任何存档；`RecipePattern` 只存在于 `core` 注册表与测试。
- 统一视图格：网格 `0..8`、背包 `9..44`，仅用于命令值域与 UI 命中；`sim` 内部仍以两个独立容器表达。
- `Recipe(id)` 返回 `RecipePattern`，聚合 `CraftingRecipe` 与 `Inventory.Craft` 删除（真实扣料只能来自网格消费），不留双重合成路径。
- 形状表（`X` 表示非空格，`.` 为空，裁边后形状）：

| ID | 形状 | 产物 |
|---|---|---|
| 1 | 石头 2×2 | 石砖 ×4 |
| 2 | 圆石 3×3 圆环（中空） | 熔炉 ×1 |
| 3 | 铁锭 3×3 | 铁块 ×1 |
| 4 | 石头顶排 + 木棍中列 | 石镐 ×1 |
| 5 | 铁锭顶排 + 木棍中列 | 铁镐 ×1 |
| 6 | 木板 3×3 圆环（中空） | 箱子 ×1 |
| 7 | 原木 1×1 | 木板 ×4 |
| 8 | 玻璃 2×2 | 发光方块 ×4 |
| 9 | 石头纵列 + 木棍纵列（2×2，两列各两格） | 石锄 ×1 |
| 10 | 铁锭纵列 + 木棍纵列（同上） | 铁锄 ×1 |
| 11 | 小麦横排 1×3 | 面包 ×1 |
| 12 | 木板纵向 1×2 | 木棍 ×4 |
| 13 | 木板 2×2 | 工作台 ×1 |

### D3：匹配器为固定循环，不分配

`MatchCraftingGrid(grid) (RecipeID, ItemStack, bool)` 只做裁边归一化 + 逐格比较 + 水平镜像一次重试；最多 13 条 × 18 格的固定循环，无 map/slice 分配，无通用矩阵包。镜像语义由每条配方的 `Mirror` 位开关（工具类配方关闭镜像避免左右手机双解，圆形/对称配方天然镜像不变）；垂直翻转与旋转一律不匹配。

### D4：回收不变量是中心不变量

- 私有 `canRepackCrafting`：按与 `Inventory.AddStack` 相同的稳定顺序试算「网格全部物品 + 待产出」装入 36 格背包。
- 私有 `tryAddPreservingCrafting` 统一收口全部背包增量入口：产物取出、掉落物拾取、采掘掉落、作物多掉落与初始材料包。入口在局部副本上预演，破坏不变量即拒绝该增量（拾取拒绝 → 掉落留世界；取出拒绝 → 逐格不变）。
- 生命周期挂钩：关闭工作台 / 离开距离 / 工作台被挖 → 先回收格 `4..8` 再降尺寸；断线持久化前与死亡清空前 → 回收全部 9 格。不变量保证回收不失败；回收失败按内部错误路径暴露（测试断言其不可达），绝不静默丢物。
- 每次权威 tick 结束时不变量成立（含 grid 变化当 tick），因此不需要后台校验或持久化修复。

### D5：工作台是普通方块，不是容器

`openContainer` 扩展识别 `WorkbenchID`：沿用既有权威 raycast、触及距离与 loaded chunk 校验，成功后只设置该玩家 `CraftingGrid.Size=3` 与命中位置；工作台 MUST NOT 占用 `world.ContainerRef`、区块槽位或任何持久化记录。玩家离开触及距离、方块被挖（含同 tick 变空气）或主动关闭时按 D4 回收并降回 2。每玩家网格独立，多人共用一台工作台天然成立。

### D6：`CraftRecipe` 过渡语义

线上注册与 codec 保留（fuzz/round-trip 继续覆盖），但 ingress 不再映射到任何 sim 命令：收到即按既有命令拒绝路径稳定回拒、状态不变。客户端删除 recipe-click 列表与发送路径。类型与编号 7 的释放归 A-06。

### D7：UI 最小扩展既有 container overlay

- `internal/render/hud/container.go` 的合成区以「grid size 参数 + 产物格」替换十条配方行，继续复用 `appendItemTile`、既有 atlas cell 与 quad/glyph pass；不新建 UI framework。
- 命中面从 `RecipeButtonAt` 换成统一视图格 + 产物格命中；产物格不是普通移动目标。
- 输入只发权威命令：格点击组 `MoveCraftingStack`，产物格点击发 `TakeCraftingOutput`；确认前不本地改写任何镜像。
- 资源契约风险见 R2。

### D8：capture 只加构造，不写 golden

`inventory-crafting` 场景更新为 2×2 实物配方 + 非空产物格；新增 `workbench-crafting` 场景构造（3×3、镜像不对称配方、合法产物），插在 `inventory-crafting` 之后、`chest-container` 之前。只交付场景与像素不变量测试；golden PNG 由 A-07 在 scenario 迁移时统一重生成。

## Risks / Trade-offs

- **[合并冲突]** A-02/A-04 同样追加 `internal/core`、`internal/network` 与 `internal/sim` 的不同段 → 冲突被 append-only 段约束在 registry switch/case 与 import 行；按批次计划由 A-06 以固定顺序合并裁决，不机械 ours/theirs。
- **[HUD 固定容量]** 合成区组成变化改变最大打开态精确 quad 数（旧值 266 由十条配方行贡献）→ 实现必须在布局边界测试中重新锁定精确值并断言 ≤267；`AGENTS.md`/`CLAUDE.md` 的 266 表述由 A-07 在基线同步时更新，功能分支不触碰基线文档。
- **[窄窗口 3×3]** 3×3 网格加产物格抬高合成区高度 → 沿用统一 HUD 缩放比，全部格在受支持的最窄 framebuffer 内可见且命中矩形与绘制矩形一致，由布局测试穷举边界样本。
- **[过渡期双语义]** v26 线上仍注册 `CraftRecipe` 但语义拒绝 → 旧客户端若连接本分支服务端会得到稳定拒绝而非静默无响应；跨版本兼容在批次集成时由 v27 握手拒绝收口（A-07）。
- **[拾取变严]** 回收不变量使部分满载拾取从成功变为留世界 → 这是规格要求的行为（见 delta spec 场景），由失败测试先行锁定，避免被当成回归"修复"。

## Migration Plan

- 存档：无迁移。编号 append-only 对既有读档透明；网格瞬态不落盘。玩家 schema v7、区块 schema v9 均不变。
- 回退：整支 revert 分支即可，无任何存档残留。
- 批次集成路径：A-06 依 crafting → torches → swords → nightwalker → bed 合并并锁定 `MoveCraftingStack=7`、删除 `network.CraftRecipe`；A-07 升 v27、迁移 scenario `19:20`、重生成 18 张 golden 并同步基线文档。

## Open Questions

- 无。`MoveCraftingStack` 的 in-branch 临时编号（14）与终值（7）的差异、以及 recipe `14..18` 的追加归属，均已由批次计划与认领备注裁决，不需要在实现期重新决策。
