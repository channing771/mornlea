# 设计 — 背包面板拖拽与拖出丢弃

## 分层与数据所有权

- **前端（拖拽呈现态唯一所有者）**：`GamePanels.tsx` 增加本地拖拽态（源槽引用与指针位置）；主键按下（非空槽、已确认）发起，window 级 pointermove 跟随，pointerup 命中槽位发 `dragMove`、命中页面背景发 `drop`、命中面板空白取消；Esc 或次键取消。被拖组用绝对定位浮层渲染，源槽位让位态纯 CSS。拖拽中权威刷新照常覆盖面板数据，浮层保持。
- **桥**：`schema.json` 的 `gameActionEvent` 新增 `dragMove`（fromArea/fromIndex/toArea/toIndex）与 `drop`（area/index）两个分支加 ajv 夹具；Go `ui_game_bridge.go` 扩展解码与严格校验。
- **Go app 翻译**：`dragMove` 复用 `handleGameAction` 既有 slot 路径的区域/视图/槽位映射，发送与两次点击完全相同的 Move* 消息；`drop` 映射为新的 `DropStack`。
- **服务端**：`contract.CommandDropStack{StackView, Slot}`；ingress 翻译；结算复用 `dropSelectedItem` 的投放出口（泛化为按视图槽位取整组）：取出整组 → `PrepareDrop/CommitDrop` 脚下投放 → 拒绝码复用既有注册表。背包/合成视图在命令相位内联结算，容器视图走 `tick.containerMoves` 同款延迟通道。

## 协议（v44 → v45）

- 新 C→S ID 21 `DropStack{Sequence u64, Container core.ContainerRef, View uint8, Slot u8}`，寻址面与 `MoveStackPartial` 完全一致（View ∈ {0,1,2}，Slot 按视图统一槽位域）；解码校验视图与槽位域。
- 版本矩阵同步：协议常量、`packages/audit` 基线、根 AGENTS.md 与 openspec/config.yaml 矩阵行。

## 并发与预算

- 拖拽呈现纯前端本地态，不进预测器；丢弃消息经既有命令相位单写者结算，无新共享状态。
- 拖拽事件不进 Rust 输入栈（cursorFree 参与模式下 DOM 原生接收指针事件），Rust 侧零改动、client ABI 不变。

## 受影响文件

- 前端：`frontend/src/ui/GamePanels.tsx`、`frontend/src/bridge/game.ts`、`frontend/src/bridge/schema.json` 及对应测试
- Go 客户端：`packages/client/client/ui_game_bridge.go`、`packages/client/cmd/mornlea/app/app_game_ui.go` 及测试
- 协议：`packages/shared/network/protocol/message_drop_stack.go`（新）、`registry.go`、`packet.go` 版本号及测试
- 服务端：`packages/server/server/session_ingress.go`、`packages/server/sim/contract/contract.go`、`packages/server/sim/entity/drop.go`、`packages/server/sim/entity/tick.go` 及测试
- 版本矩阵：`packages/audit`、根 `AGENTS.md`、`openspec/config.yaml`

## 取舍

- **拖放槽位复用既有 Move\* 消息而非新消息**：与两次点击语义完全等价是硬需求，复用消除第二套搬运结算与服务端新行为。
- **整组丢弃而非单件**：与经典语义一致；单件丢弃已有 Q 键路径，分批拖出留待后续。
- **拖拽态不进 Go**：`gameSource` 先例已确立「前端持语义引用、Go 只做翻译」的分工；拖拽只是更长的引用生命周期，同一分工。
- **合成格可拖出**：合成格是权威状态的一部分，丢弃寻址复用视图统一槽位域即可，不为合成格开特例。

## 验证

`make frontend-check`；`go test ./packages/shared/network/protocol ./packages/client/client ./packages/client/cmd/mornlea/app ./packages/server/sim/entity ./packages/server/server -race -count=1`；`go test ./packages/audit -count=1`。
