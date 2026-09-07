# camera-perspectives Specification

## Purpose

为玩家提供 F5 循环的三态视角（第一人称、第三人称背面、第三人称正面），使自身模型显隐与相机后拉在本地一致且跨世界保留。

## Requirements

### Requirement: F5 按固定顺序循环三态并本地持久化

客户端 SHALL 维护本地 `CameraMode`（0=第一人称、1=第三人称背面、2=第三人称正面），按 F5（或等价按键/触屏按钮）以第一人称→背面→正面→第一人称顺序循环；首次进入世界默认为第一人称，退出世界后 MUST 保存并在下次进入时恢复。视角切换 MUST 为纯本地呈现状态，不得新增服务端消息或权威字段。

#### Scenario: F5 循环顺序固定

- **GIVEN** 当前为第一人称
- **WHEN** 连续按三次 F5
- **THEN** 视角 MUST 依次为背面、正面、第一人称

#### Scenario: 跨世界保留

- **GIVEN** 玩家在背面视角下退出世界
- **WHEN** 再次进入同一或另一世界
- **THEN** 初始视角 MUST 为背面

### Requirement: 自身模型与双手按视角显隐

第一人称 MUST NOT 渲染自身身体模型且 MUST 显示 viewmodel 双手；第三人称（背面与正面）MUST 渲染自身身体模型且 MUST NOT 显示 viewmodel 双手。背包/菜单打开时的隐藏规则 MUST 与既有 HUD 同隐同现保持一致。

#### Scenario: 第一人称不渲染自己但有手

- **GIVEN** 第一人称游戏相位且已确认选中非空
- **WHEN** 渲染一帧
- **THEN** 画面 MUST 含 viewmodel 双手且 MUST NOT 含自身身体模型

#### Scenario: 第三人称渲染自己且无手

- **GIVEN** 第三人称背面游戏相位
- **WHEN** 渲染一帧
- **THEN** 画面 MUST 含自身身体模型且 MUST NOT 含 viewmodel 像素

### Requirement: 第三人称相机后拉并防穿墙

第三人称相机 SHALL 位于眼睛沿视线反方向（背面）或正方向（正面）固定距离（默认约 4 格），当该线段穿过完整不透明方块时 MUST 按最近阻挡点收缩（保留最小贴脸距离），视线被完全遮挡时 MUST 收至眼睛处而不得穿墙。瞄准、挖掘与攻击射线 MUST 仍以眼睛与朝向为准，不得改用相机位置。

#### Scenario: 开阔地保持固定距离

- **GIVEN** 第三人称背面且身后 4 格内无不透明方块
- **WHEN** 计算相机位置
- **THEN** 相机 MUST 位于眼睛后方固定距离处

#### Scenario: 贴墙收缩不穿墙

- **GIVEN** 第三人称背面且身后 1 格处有不透明墙
- **WHEN** 计算相机位置
- **THEN** 相机 MUST 收缩到墙前且不得进入墙内，瞄准射线 MUST 仍从眼睛发出

#### Scenario: 正面朝向一致

- **GIVEN** 第三人称正面
- **WHEN** 渲染自身模型
- **THEN** 模型 MUST 面向相机（可观察到脸部），移动方向 MUST 与第一人称一致
