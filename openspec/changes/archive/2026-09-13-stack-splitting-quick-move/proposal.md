# 完整分堆与快捷搬运

## Why

生存闭环收尾战役阶段二首行（积压表 B-35，依赖 B-23 已交付）。当前物品栏与容器的全部移动命令只有整堆粒度（`MoveInventoryStack`/`MoveCraftingStack`/`MoveContainerStack` 均无数量位），也没有任何「一键转移」路径：玩家想把半组煤放进熔炉燃料槽、往合成网格放单个材料、或把一箱战利品快速搬回背包，都只能反复整堆移动后再手动回移。本行在 A-01 两次点击整堆语义之外补齐半组/单件拆分与快捷搬运。

2026-09-13 用户经对话内与飞书确认通道双重批准短设计（`docs/superpowers/specs/2026-09-13-b35-stack-splitting-design.md`）。

## What Changes

- `packages/shared/network` 协议 v43→v44：pure-append 两条 C→S 新命令——ID 19 `MoveStackPartial{Sequence, Container, View, From, To, Single}`（半组 `ceil(n/2)` / 单件 1，数量由服务端按源栈推导，客户端不得传任意数量）与 ID 20 `QuickMoveStack{Sequence, Container, View, From}`（整堆移到对侧区域首个可容纳位置）；View ∈ {0=inventory 0..35, 1=crafting 0..44, 2=container 箱子 0..62/熔炉 0..38}，View≠2 时 Container 必须为零值。既有消息零 reshape。
- `packages/shared/core`：新原语 `Inventory.MoveStackAmount(from, to, amount)`（copy-on-write：目标空放 amount、同类合并 `min(amount, space)`、异类拒绝），沿既有 `MoveStack` 形状。
- `packages/server/sim`：半组/单件移动在三个视图域结算（inventory 直用新原语；crafting 域复用既有边界校验 + repack 预演 + 异类拒绝；container 域复用箱子/熔炉视图 helper + 熔炉槽位物品约束 + repack 预演 + 异类拒绝）；快捷搬运四个方向（容器区→背包按 `AddStack` 4 相位序、背包→箱子按 36..62 扫描首个可容纳、背包→熔炉按「燃料→燃料槽，否则熔炼输入→输入槽」优先级、crafting 网格↔背包、纯背包面板 hotbar↔backpack 互移）；全部 copy-on-write 原子、拒绝原因零新增（复用 `RejectInvalidSlot`/`RejectInvalidInput`/`RejectPlayerNotReady`）；结算相位沿用现状（inventory/crafting 内联、container 延迟至区块写相位）。
- `packages/server/server`：两条新命令 ingress 翻译与 Memory/TCP parity。
- 前端交互：`GamePanels.tsx` 槽位事件携带 `button: left|right` 与 `shift: bool`（`onContextMenu` + `shiftKey`，右键 `preventDefault`）；桥 `game-action` 事件 `slot` 变体三端钉值扩展（schema.json / game.ts / `ui_game_bridge.go`）；`handleGameAction` 语义分支——右键两击发 `MoveStackPartial`（Shift+右键第二击=单件）、Shift+左键单击发 `QuickMoveStack`、左键两击整堆不变；页脚提示行更新。
- client ABI 保持 v19（上行事件 JSON 信封内容扩展、无新 FFI 导出面）。

## 契约与版本影响

- 协议 v43→v44 唯一升版（两条新 C→S 命令 pure-append）；旧客户端以协议版本协商拒绝，语义不变。
- engine ABI v11、client ABI v19、玩家 schema v9、区块 schema v9、世界 metadata v6、`companions.ai` v5、`hostile_mobs` v2、`passive_mobs` v1、benchmark scenario v23 均不变。
- golden：世界 31 景零变化（容器面板不进世界 capture）；panel-* 前端部件基线随页脚提示文案重生成。

## 用户可观察结果

- 右键选中来源格再右键点目标格：恰好一半（向上取整）物品移动过去。
- 右键选中来源后 Shift+右键点目标：恰好 1 个物品移动过去。
- Shift+左键点击任意槽：整堆快速转移到对侧区域首个可容纳位置（箱子↔背包、熔炉↔背包按燃料/输入优先级、合成网格↔背包、背包面板内快捷栏↔背包）。
- 半组/单件移动到「非同类非空」目标被稳定拒绝且不改变任何栏位。

## 非目标

- 拖拽铺放与自动整理（backlog 范围冻结排除）。
- 服务端手持游标栈/手上持有态（两击模型保持客户端本地选择）。
- 跨容器快捷搬运（箱子↔熔炉直移）。
- 数字键快速放入、半组移动与交换的混合语义（无游标态下部分移动遇异类目标一律拒绝）。

## 受影响的包与文档

`packages/shared/core`、`packages/shared/network`、`packages/server/sim`（entity）、`packages/server/server`、`packages/client`（client 桥、cmd/mornlea app）、`packages/engine/crates/mornlea_client/frontend`（GamePanels/bridge schema/vitest/visual fixtures）、`packages/audit`（基线钉值随版本推进）、根 `AGENTS.md` 版本矩阵、`openspec/config.yaml`、`docs/notes/progress.md`、`docs/feature-backlog.md` B-35 行回填。

## 延期与放弃

（实现期登记，暂空）
