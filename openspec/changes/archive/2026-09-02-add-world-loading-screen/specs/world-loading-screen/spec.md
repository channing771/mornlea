# world-loading-screen Delta Spec

## Purpose

为主菜单「进入游戏」建立初始世界加载相位：装配成功后以不透明 WebView 加载屏覆盖渐进加载中的世界，进度数据仅来自权威区块列镜像，完成判据与无头加载路径共用单一定义，收敛后才捕获光标进入游戏相位——消除「画面透明、地形渐进浮现」的裸等待期。

## ADDED Requirements

### Requirement: 装配成功进入世界加载相位

装配成功后交互客户端 SHALL 进入加载相位：主菜单消失，WebView 以菜单族参与模式（可见、firstResponder）呈现不透明加载屏；光标 MUST NOT 被捕获；加载期间游戏键盘、指针与滚轮输入 MUST NOT 产生任何玩法上行，桥上行动作（含 Enter 默认按钮路径）MUST NOT 重复触发世界装配；常显 HUD MUST NOT 呈现。加载屏背景 MUST 完全不透明，其下渐进装配的半成品世界画面 MUST NOT 可见。窗口关闭请求在加载相位 MUST 正常退出客户端。

#### Scenario: 加载期输入与 HUD 抑制

- **GIVEN** 世界装配成功、加载屏呈现中
- **WHEN** 玩家按下 WASD/Enter/Esc 或点击、滚动指针
- **THEN** 不产生任何玩法上行，世界装配不重复执行，光标保持未捕获
- **AND** 加载屏不呈现快捷栏、生命、饥饿等任何常显 HUD 分节内容

#### Scenario: 加载屏遮挡半成品世界

- **GIVEN** 初始区块快照仅部分到达、地形仍在渐进网格化与上传
- **WHEN** 加载屏呈现
- **THEN** 屏幕上 MUST NOT 出现天空底色或部分地形；仅加载屏自身内容可见

#### Scenario: 二次装配重新加载

- **GIVEN** 从暂停页「退回主菜单」拆链后再次点击「进入游戏」且装配成功
- **WHEN** 加载相位开始
- **THEN** 加载屏重新呈现，进度从新会话的区块镜像重新推进，不携带上一台世界的任何加载状态

### Requirement: 加载进度以权威区块镜像驱动

加载屏 SHALL 呈现标题、进度条与区块计数，下行 `loading` 分节 MUST 仅含两个整数：`loaded`（当前已就绪区块列数，取自客户端区块列镜像的势）与 `total`（目标列数，取无头加载判据同源的 `LoadedChunkTarget` 公式 `(2*(ViewDistance+1)+1)^2`）。前端 MUST 以 `clamp(loaded/total, 0, 1)` 驱动进度条，MUST NOT 自行预测或平滑进度；文案与格式由前端常量呈现。分节 MUST 经既有整份 `uiState` 下行路径推送，`loaded` 未变化的帧零推送。

#### Scenario: 进度随快照推进

- **GIVEN** 加载相位中服务端按距离顺序持续下发区块快照
- **WHEN** 已就绪区块列数从 `k` 增至 `k+1`
- **THEN** 下一次下行 MUST 携带 `loaded = k+1`，进度条呈现比例相应前进
- **AND** 已就绪列数未变化的帧 MUST 零推送

#### Scenario: 进度比例钳制

- **GIVEN** `loading` 分节携带任意合法 `loaded`/`total` 组合（含 `loaded > total` 或 `total` 大于剩余快照量）
- **WHEN** 前端呈现进度条
- **THEN** 呈现比例 MUST 钳制在闭区间 `[0, 1]`，不出现越界宽度或除零错误

### Requirement: 加载完成判据与无头路径单一定义

交互加载的完成 SHALL 复用无头加载判据的同一组成：已加载区块列数等于 `LoadedChunkTarget`，且 mesher 排队/在飞/就绪/脏段计数与调度器待上传段数全部归零（`ApplicationLoadComplete`）。加载循环每帧的消息 drain 与 mesh 工作预算 MUST 与无头 `WaitUntilLoaded` 同源（`MessageDrainMax`），MUST NOT 另立第二套预算或完成定义。收敛后客户端 MUST 捕获光标、进入游戏相位，游戏相位首帧 MUST 呈现完整世界与已确认玩家相机。加载相位 MUST NOT 设超时或中途取消；进度摘要 MUST 至少每 5 秒记录一次日志。无头路径（benchmark/capture）MUST 不经过加载相位，既有 `WaitUntilLoaded` 行为零变化。

#### Scenario: 收敛后进入完整世界

- **GIVEN** 加载相位中区块快照到齐且 mesher 与上传队列归零
- **WHEN** 完成判据首次成立
- **THEN** 加载屏消失，光标被捕获，进入游戏相位
- **AND** 首帧呈现完整近环世界，相机位于已确认玩家呈现位，无错误机位闪烁

#### Scenario: 判据未满不进入世界

- **GIVEN** 区块快照已到齐但 mesher 仍有在飞任务或调度器仍有待上传段
- **WHEN** 加载循环推进
- **THEN** 客户端 MUST 停留在加载相位，进度条按 `loaded/total` 呈现
- **AND** 完成判据成立前 MUST NOT 捕获光标或呈现世界

#### Scenario: 无头路径零变化

- **GIVEN** benchmark 或 capture 无头启动
- **WHEN** 固定场景加载与抓帧
- **THEN** 加载判据、预算与行为 MUST 与本 change 之前逐项一致，不经过交互加载相位
