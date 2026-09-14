# B-35 完整分堆与快捷搬运 — 短设计（bounded）

日期：2026-09-13
状态：待需求方批准（brainstorming 门禁）
分类：bounded（复用既有移动命令族/两击语义/容器视图域，无新子系统）
来源：`docs/feature-backlog.md` B-35 行（依赖 B-23 已完成）；认领提交 `d5a47382`。

## 1. 现状

- 三个移动命令全是整堆：`MoveInventoryStack`（0..35）、`MoveCraftingStack`（0..44 网格+背包）、`MoveContainerStack`（箱子 0..62 / 熔炉 0..38 统一视图 + ContainerRef）；无数量位，唯一「部分」是同类合并的容量剩余。
- 两次点击语义是客户端本地态（首击记 `gameSource`，次击发一条命令）；服务端无手持游标栈。
- 现役容器 UI 是 WebView React 面板（`GamePanels.tsx`）：槽位只有左键 `onClick`，无右键/修饰键；桥事件 schema 三端钉值（schema.json / game.ts / ui_game_bridge.go，`requireExactKeys`）。

## 2. 交互语义（裁决 D1）

保持「无服务端游标栈」与两次点击模型，新增两类输入：

| 输入 | 语义 |
|---|---|
| 左键两击（现状） | 整堆移动（不变） |
| 右键选源 + 右键点目标 | **半组移动**：移动 `ceil(n/2)` 个（n=源栈数量） |
| 右键选源 + Shift+右键点目标 | **单件移动**：移动恰好 1 个 |
| Shift+左键单击（任意槽） | **快捷搬运**：整堆移到对侧区域首个可容纳位置 |

- 前端槽位事件携带 `button: left|right` 与 `shift: bool`（React `onContextMenu` + `shiftKey`）；右键 `preventDefault`。
- 选中态沿用 `gameSource` 高亮；页脚提示行更新为「先选物品，再选目标位置；右键半组 / Shift+右键单件 / Shift+点击快速搬运」。
- 半组/单件移动的目标为**非同类非空格时拒绝**（部分移动不做交换——swap 语义在无游标态下不可定义，保持最小）。

## 3. 命令与协议（D2）

pure-append 新增两条 C→S 命令（**协议 v43→v44 唯一升版**；既有消息零 reshape）：

- C→S **19 `MoveStackPartial`** `{Sequence, Container ContainerRef, View uint8, From, To uint8, Single bool}`：View ∈ {0=inventory(0..35), 1=crafting(0..44), 2=container(箱子 0..62/熔炉 0..38)}；View≠2 时 Container 必须为零值。数量由服务端按源栈推导（half=`ceil(n/2)`、single=1）——**客户端不得传任意数量**（防刷）。
- C→S **20 `QuickMoveStack`** `{Sequence, Container ContainerRef, View uint8, From uint8}`：同 View 域；整堆移到对侧区域。

## 4. 服务端结算（D3，全部 copy-on-write 原子、复用既有约束）

- `core.Inventory.MoveStackAmount(from, to, amount) (Inventory, bool)`：目标空→放 amount；同类→合并 `min(amount, space)`；异类→false。沿 `MoveStack` 形状。
- 半组/单件三视图域：inventory 直用新原语；crafting 域复用 `craftingMoveCommandReasons` 边界 + `canRepackCrafting` 预演（异类拒绝）；container 域复用箱子/熔炉 view helper + 熔炉槽位物品约束（输入须 `SmeltingOutput` 表内、燃料仅煤、输出槽只可为源）+ repack 预演。
- 快捷搬运四方向（整堆，目标序固定确定性）：
  - 容器视图：容器区→背包按 `Inventory.AddStack` 既有 4 相位序；背包→箱子扫描 36..62 首个「空或同类未满」；背包→熔炉按「燃料→燃料槽，否则熔炼输入→输入槽」优先级（两类皆可时输入优先，沿 MC 炉子语义），无可容纳槽拒绝。
  - crafting 视图：网格→背包 AddStack；背包→网格首个「空或同类未满」网格格（容量后 repack 预演）。
  - inventory 视图（纯背包面板）：hotbar(0..8)↔backpack(9..35) 互移（AddStack 序）。
- 拒绝原因全复用 `RejectInvalidSlot` / `RejectInvalidInput` / `RejectPlayerNotReady`（零新增枚举）；结算相位沿用现状（inventory/crafting 内联、container 延迟至 FinishWorld 区块写相位）。

## 5. 客户端与视觉（D4）

- `GamePanels.tsx` 槽位 `onClick` 携带 `shiftKey`、新增 `onContextMenu`；桥 `game-action` 事件 `slot` 变体追加 `button`/`shift` 两字段——schema.json / game.ts / `ui_game_bridge.go` 三端钉值同批（`requireExactKeys` 拒绝旧字段集，需同步）。
- `handleGameAction`：Shift+左键直发 `QuickMoveStack`；右键两击按 Single 位发 `MoveStackPartial`。
- **client ABI 保持 v19**（上行事件 JSON 信封内容扩展、无新 FFI 导出面，B-24 armor 先例只对下行 hudState 结构扩展 bump）。
- 视觉：panel-* 前端部件基线随页脚提示文案重生成；世界 31 景 golden 零变化（容器面板不进世界 capture）。benchmark scenario v23、全部存档 schema、engine ABI 不变。

## 6. 非目标（沿 backlog 范围冻结）

不做拖拽铺放、自动整理、服务端游标栈/手上持有态、跨容器快捷搬运（箱子↔熔炉直移）、数字键快速放入、半组+交换混合语义。

## 7. 任务组草案（7 组）

T1 core `MoveStackAmount` 原语与目标序 helper；T2 协议 v44 两命令+codec+frozen；T3 sim 半组/单件三域；T4 sim 快捷搬运四方向；T5 ingress+parity；T6 前端事件/桥/handleGameAction/vitest；T7 fixtures 重生成+收尾门禁+基线文档。
