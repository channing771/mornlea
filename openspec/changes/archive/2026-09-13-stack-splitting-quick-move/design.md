# 设计：完整分堆与快捷搬运

> 批准记录与交互矩阵见 `docs/superpowers/specs/2026-09-13-b35-stack-splitting-design.md`（2026-09-13 用户批准）。本文记录实现级数据所有权、命令布局与文件落点。

## D1 命令布局与数量推导

- 两条 pure-append C→S 命令，View 枚举收敛三视图域到同一命令族（避免每域一对命令的六命令膨胀）：
  - ID 19 `MoveStackPartial{Sequence uint64; Container core.ContainerRef; View uint8; From, To uint8; Single bool}`——载荷 8+8+1+1+1+1 = 20B + Sequence。
  - ID 20 `QuickMoveStack{Sequence uint64; Container core.ContainerRef; View uint8; From uint8}`。
- **数量由服务端推导**：半组 = `ceil(source.Count/2)`、单件 = 1。被否：客户端携带数量字段——任意数量请求扩大攻击面（每次 tick 可任意切分），且 UI 语义只需要两档。
- View 值域 {0,1,2}；View=2 时 Container 必须是合法 `ContainerRef`（沿 `MoveContainerStack` 的 Validate 矩阵），View∈{0,1} 时 Container 必须为零值。域内索引上界按 View 分派（35/44/62 或 38，熔炉/箱子由 Container.Kind 决定），`From != To`（Partial）。
- 结算相位沿用现状：View 0/1 内联 `ApplyPlayerCommands`；View 2 延迟进 `tick.containerMoves` 在 `FinishWorld` 区块写相位结算（与既有 `MoveContainerStack` 同路径）。

## D2 半组/单件的域规则（全部 copy-on-write 原子）

- **inventory 域（0..35）**：`core.Inventory.MoveStackAmount(from, to, amount)`——目标空接收 amount；同类合并 `min(amount, space)`（余量留源）；**异类非空目标拒绝**（被否：swap——无游标态下「半堆换半堆」语义不可定义）。
- **crafting 域（0..44 统一视图）**：复用 `craftingMoveCommandReasons` 边界（网格尺寸、双背包端拒绝）+ amount 版 `applyMoveCraftingStack`（异类拒绝沿网格域既有规则）+ 成功后 `canRepackCrafting` 预演不变量。
- **container 域（箱子 0..62 / 熔炉 0..38）**：复用 `chestViewSlot`/`furnaceViewSlot` helper 与 `mergeStacks` 的 amount 变体；熔炉槽位约束逐条复用（输入仅 `SmeltingOutput` 表内物品且换物品重置 `ProgressTicks`、燃料仅煤、输出槽只可为源）；跨区增量后 repack 预演。
- 拒绝原因零新增：`RejectPlayerNotReady`/`RejectInvalidSlot`/`RejectInvalidInput` 覆盖全部路径；数量退化为 0（源空）按 `RejectInvalidInput`。

## D3 快捷搬运的固定目标序（确定性契约）

- **container 域**：From 在容器区（≥36）→ 玩家背包按 `Inventory.AddStack` 既有 4 相位序整堆并入（余量留源）；From 在背包区（0..35）→ 箱子：统一视图 36..62 升序首个「空或同类未满」格（整堆或合并，余量留源）；熔炉：物品为熔炼输入→输入槽（合并或空），否则为煤→燃料槽，两类皆可时输入优先（沿参考实现炉子语义），均无可容纳→拒绝。
- **crafting 域**：网格格（0..8）→ 背包 `AddStack`；背包格（9..44）→ 网格 0..8 升序首个「空或同类未满」格，容量后 repack 预演，无可容纳→拒绝。
- **inventory 域（纯背包面板）**：快捷栏（0..8）↔ 背包（9..35）互移——源在对侧区域内 `AddStack` 序并入（限定对侧区域，不跨区自选）。
- 被否：目标选择可配置/智能填充——固定序是确定性契约且可测；「智能」属自动整理（非目标）。

## D4 前端与桥

- `GamePanels.tsx` 槽位 `slotButton`：`onClick` 事件携带 `shiftKey`；新增 `onContextMenu`（`preventDefault`，等价 `button:"right"` + `shiftKey`）；hotbar 选择不参与本行。
- 桥 `game-action` 的 `slot` 变体追加 `button`（"left"|"right"）与 `shift` 两字段：`frontend/src/bridge/schema.json`（$defs/gameActionEvent 的 slot 变体 required 扩容）、`frontend/src/bridge/game.ts` 校验、`packages/client/client/ui_game_bridge.go` `requireExactKeys` 同步——三端缺一即拒（旧字段集事件被新 schema 拒绝属预期，前端与 Go 同批发布）。
- `handleGameAction` 语义分支（`app_game_ui.go`）：
  - `shift && button==left` → 忽略现有 `gameSource`（清空）直发 `QuickMoveStack`；
  - `button==right` 且无源 → 记 `gameSource`（半组选中态，与整堆同高亮）；
  - `button==right` 且有源 → 发 `MoveStackPartial{Single: shift}`，清源；
  - `button==left` 无 shift → 现状整堆两击不变（混合左右键：第二击左键=整堆、第二击右键=半组/单件，均以第二击类型定档）。
- 页脚提示行更新为「先选物品，再选目标位置；右键半组 / Shift+右键单件 / Shift+点击快速搬运」；`panel-*` 部件基线重生成（文案进 golden），vitest 组件断言扩容（右键/shift 事件发射矩阵）。

## D5 版本与门禁

- 协议 v43→v44 唯一升版；根 `AGENTS.md`、`openspec/config.yaml`、`docs/notes/progress.md` 基线同步；`packages/audit` 版本矩阵钉值随推进；全仓 `grep ProtocolVersion` 扫测试钉值（B-23/B-24 教训成文化为收尾检查项）。
- 门禁：`make test-race`、`make rust`/`rust-check`（前端在 client crate 内）、`make frontend-check` + `frontend-visual-check`（panel-* 重生成）、`make visual-check`（世界 31 景零差异）、`openspec validate --all --strict`。

## D6 被否方案汇总

| 方案 | 否决理由 |
|---|---|
| 客户端携带任意数量 | 扩大攻击面；UI 只需半组/单件两档 |
| 服务端游标栈（MC 原版手持态） | 新权威状态 + 关容器放置语义 + 持久化边界，超范围 |
| 每视图域独立命令对 | 六命令膨胀；View 枚举收敛已够 |
| 半组移动 + 异类 swap | 无游标态语义不可定义 |
| 快捷搬运智能填充 | 非目标（自动整理）；固定序是确定性契约 |
| 既有 move 命令加字段 | reshape wire 违 pure-append 纪律 |
