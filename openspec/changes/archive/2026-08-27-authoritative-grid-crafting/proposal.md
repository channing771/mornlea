## Why

现行合成是「点击 recipe ID → 服务端按聚合单输入扣料」的最小闭环（`authoritative-crafting`，m4d 交付）：玩家不能摆放原料形状，配方表也无法表达镐、剑、床这类多行多列结构。第一夜生存批次需要真实的 2×2/3×3 格子工作台作为火把、剑、床三条后续配方的共同基础；批次原共享契约提交与五条功能分支已随开发机迁移丢失，本 change 按 2026-08-25 控制会话重置裁决从当前 `main` 重做 A-01。

## What Changes

- `core` 配方表从聚合单输入改为固定形状 `RecipePattern`：既有 recipe `1..11` 改成熟悉的格子形状（镐/锄用木棍、熔炉为圆石圆环、箱子为木板圆环、面包为横排小麦、其余保持各自产品意图），追加 recipe `12`（木棍：纵向两木板 → 4）与 recipe `13`（工作台：2×2 木板 → 1）。火把、三把剑与白床配方（终表 `14..18`）由 A-02/A-03/A-05 在本 change 合流后追加，编号终值由集成任务锁定。
- 每玩家瞬态权威合成网格：个人 2×2、对工作台完成权威射线交互后 3×3，统一 9 格；移动与产物取出全部由服务端验证，客户端只显示权威 `CraftingState`、不预测。
- 回收不变量成为中心契约：任意权威 tick 结束时，36 格背包总能无损容纳网格全部物品；产物取出、掉落物拾取、采掘/作物掉落与初始材料包都必须保持该不变量；关闭、断线与死亡先无损回收网格。
- 新增工作台物品与方块：普通完整立方体、原创程序化纹理、可放置可挖回；打开后只把同一玩家网格有效尺寸从 2 变 3，不占用容器引用或区块槽位、不持久化；每玩家网格相互独立。
- 协议追加三条 Play 消息：`MoveCraftingStack`（C→S，临时编号 14）、`TakeCraftingOutput`（C→S，临时编号 15）与 `CraftingState`（S→C，编号 21）；`CraftRecipe` 保留线上注册但语义上稳定拒绝（过渡类型，类型删除归 A-06）。
- HUD 背包 overlay 的十条配方列表替换为 2×2/3×3 格子与产物格；capture 更新 `inventory-crafting` 场景为 2×2 实物配方并新增 `workbench-crafting` 场景构造（本分支不写 golden PNG）。
- **BREAKING**（批次集成时生效）：协议 v27 起 recipe-click 线上注册删除、旧 v26 一律握手拒绝；本分支内协议版本号保持 v26（升版与 `19:20` scenario 迁移归 A-07 独占）。

## Capabilities

### New Capabilities

- `authoritative-grid-crafting`: 每玩家瞬态权威合成网格的尺寸切换、移动语义、形状匹配、产物取出原子性、回收不变量、工作台方块与打开生命周期，以及网格状态的私有有界同步。

### Modified Capabilities

- `authoritative-crafting`: 固定配方从聚合单输入语义改为裁边形状匹配语义（`1..13`）；合成原子性以网格模式消费表达；命令与私有确认从 `CraftRecipe` 改为 `MoveCraftingStack`/`TakeCraftingOutput` 与 `CraftingState`。
- `authoritative-inventory`: 拾取路径必须保持网格回收不变量；图形背包界面从六条固定配方入口改为个人 2×2 格子与产物格，工作台打开时为 3×3。
- `container-ui-presentation`: 背包/合成 overlay 的配方列表像素区改为格子与产物格；统一栏位与命中语义扩展到网格格与产物格，熔炉与箱子语义不变。
- `visual-verification`: 正式场景清单追加 `workbench-crafting`（紧随 `inventory-crafting`）；`inventory-crafting` 内容更新为 2×2 实物配方；golden 重生成由批次集成任务统一执行。

## Impact

- 受影响代码：`internal/core`（`recipe.go`/`inventory.go`/`item.go` 及测试）、`internal/assets`（工作台纹理）、`internal/sim`（新建 `crafting.go` 与网格生命周期接入）、`internal/network`（三条新消息）、`internal/server`（ingress/publication）、`internal/client`（网格镜像）、`internal/render/hud`（容器 overlay）、`cmd/mornlea`（合成输入与 capture 场景构造）。
- 兼容性：物品/方块/配方编号 append-only 追加（`ItemStick=37`、`ItemWorkbench=38`、`WorkbenchID=45`、recipe `12..13`），无编号重排；合成网格不落盘，玩家/区块/世界/伙伴存档 schema、engine ABI 与 client ABI 均不变；HUD 固定上传容量（267 quad / 46912 bytes）不得突破。旧存档无迁移：编号追加对既有读档透明，回退为整支 revert。
- 并行协调：与 A-02/A-04 已认领行的 `internal/core`/`internal/network`/`internal/sim` 重叠按各自认领备注的 append-only 边界执行；分支内临时消息编号不构成稳定契约，合并序（crafting → torches → swords → nightwalker → bed）与编号终值锁定归集成任务 A-06，版本基线与 golden 归 A-07。

## Non-Goals

- 不实现 shift-click、拖拽分堆、配方书、任意尺寸配方或鼠标手持栈状态；移动只复用既有两次点击整堆语义。
- 不持久化工作台容器槽位或合成网格，也不为「打开的工作台被破坏」发明恢复界面（回收不变量保证无损装回背包）。
- 不登记火把、木/石/铁剑与白床的物品/方块/配方（recipe `14..18` 由 A-02/A-03/A-05 在本 change 合流后追加）。
- 不重建批次共享契约提交，不升协议版本号 v27、不迁移 benchmark scenario v20、不生成或更新任何 golden PNG（A-07 独占）。
- 不删除 `network.CraftRecipe` 过渡类型及其线上注册（A-06 独占）；本分支只删除客户端发送路径与服务端执行语义。
