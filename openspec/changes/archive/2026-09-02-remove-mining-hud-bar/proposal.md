# remove-mining-hud-bar

## 背景

采掘的进度反馈现在有两处：HUD 屏幕空间采掘进度条（常显 HUD 迁 WebView 后由
前端 `ProgressTrack` 组件呈现，`hudState.mining` 桥分节三端钉值）与世界空间
方块裂纹 overlay（`mining-crack-presentation`，GPU 侧，按权威进度 10 阶段
递进）。裂纹已能独立传达"还差多久挖完"的直觉，屏幕进度条与裂纹并存反而
重复。

## 目标

- 移除 HUD 采掘进度条的**全部呈现与数据通道**：前端组件分支、桥
  `hudState.mining` 分节（schema.json / TS client / Go `UIHudMining` 三端
  同批删除）、样式与令牌。
- 采掘进度的唯一可观察反馈变为方块表面裂纹（`mining-crack-presentation`
  不变）。
- 进食进度条继续呈现于同一锚点（其轨道几何本就独立于采掘语义标记），与
  采掘的互斥裁决随采掘条一并移除。
- 不留死资源：`--hud-mining-*` 令牌、cap/notch 常量、`MiningOverlay` 中
 失去消费方的 `Harvestable` 字段一并删除。

## 非目标

- 不修改裂纹 overlay 的任何行为（数据源 `hud.MiningOverlay` 的
  Active/Target/HasTarget/ProgressTicks/RequiredTicks 全部保留）。
- 不修改服务端权威采掘、协议消息（`PlayerState.Mining*` 照旧下发，客户端
  只是不再把进度装进 UI 分节）。
- 不修改进食条的预测逻辑与锚点堆叠。

## 用户可观察结果

- 采掘时屏幕上不再出现采掘进度条；方块表面裂纹随进度加深是唯一反馈。
- 进食时进度条照常出现在状态栈上方。

## 受影响的包与文档

- `engine/crates/mornlea_client/frontend`：组件/桥/样式/令牌/测试/视觉
  基线/入库 dist。
- `internal/client`：`UIHudMining` 与 `UIHudState.Mining` 删除。
- `cmd/mornlea/app`：UI 分节组装去掉 mining；eating 门控重写。
- `internal/render/hud`：`MiningOverlay.Harvestable` 删除（无消费方）。
- `cmd/mornlea/capture`：夹具测试随字段调整（golden 无进度条像素，不动）。
- `openspec/specs/survival-hud-presentation`：本 change 的 delta。

## 兼容性影响

- 桥 `hudState.mining` 分节删除是前端构建产物与 Go 下行 JSON 的一致变更
  （同批构建、同批入库），无线上协议影响；`ui_push_state` 信封其余字段
  不变。
- client ABI、游戏协议、存档 schema 均不变。
- 裂纹 golden 不受影响（无头 capture 从未渲染 WebView 进度条）。
