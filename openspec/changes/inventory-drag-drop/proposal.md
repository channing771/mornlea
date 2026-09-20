# 背包面板拖拽与拖出丢弃

## 背景

背包/工作台/箱子/熔炉面板目前只支持「先选后放」的两次点击搬运（含右键分堆与 Shift 快速搬运，见 stack-splitting）。用户期望经典拖拽：按住拖起一组物品、放到其它槽位；拖到面板外松手即把整组物品丢进世界。面板打开时 WebView 已全权接收 DOM 指针事件（cursorFree 参与模式），拖拽不需要新的输入管线；世界掉落物实体、Q 键丢弃与掉落同步链路均已存在。

## 目标

- 面板槽位支持指针拖拽搬运：拖起时源槽位呈让位呈现、被拖组跟随指针；松手到其它槽位时以与「先选后放」左键搬运完全相同的权威消息完成结算（复用既有 Move* 消息，服务端搬运语义零新增）。
- 松手在面板外（页面背景/世界上）时，客户端发送新的按视图槽位寻址的丢弃消息，服务端把该槽整组作为掉落物按既有 `authoritative-item-dropping` 契约投放在玩家脚下，经既有掉落同步呈现。
- 拖拽是纯前端呈现态加权威结算：拖拽中到达的权威背包刷新照常应用；松手后以服务端确认状态为准，不做客户端预测。

## 非目标

- 不做右键拖拽分批放置、多槽循环分发与自动整理。
- 不改既有两次点击/右键分堆/Shift 快速搬运语义与 Q 键单件丢弃。
- 不做拖拽的客户端预测（背包保持确认镜像）。

## 用户可观察结果

- 按住主键拖动物品到其它槽位完成搬运；拖出面板松手，整组物品出现在脚边并可捡回。
- 非法状态（空源槽、未确认状态）不发起任何协议消息。

## 受影响包

- `packages/engine/crates/mornlea_client/frontend`（拖拽交互、schema、测试）
- `packages/client/client`（桥解码）
- `packages/client/cmd/mornlea/app`（game-action 翻译）
- `packages/shared/network/protocol`（新 C→S 消息，协议 v44 → v45）
- `packages/server/server`（ingress 映射）
- `packages/server/sim/contract`、`packages/server/sim/entity`（命令与权威结算）
- `packages/audit`（版本矩阵）

## 兼容性影响

协议升 v45：新增 C→S ID 21 `DropStack`（Sequence、Container、View、Slot），寻址面与 `MoveStackPartial` 一致；旧客户端不发送即无影响。无存档 schema 变更（掉落物与背包均为既有形态）；client ABI 与 engine ABI 不变。
