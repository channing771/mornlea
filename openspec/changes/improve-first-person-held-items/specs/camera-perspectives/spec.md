## MODIFIED Requirements

### Requirement: 自身模型与双手按视角显隐

第一人称 MUST NOT 渲染自身身体模型且 MUST 显示 viewmodel 右手并隐藏空闲左手；第三人称（背面与正面）MUST 渲染自身身体模型且 MUST NOT 显示 viewmodel 双手。背包/菜单打开时的隐藏规则 MUST 与既有 HUD 同隐同现保持一致。

#### Scenario: 第一人称不渲染自己但有手

- **GIVEN** 第一人称游戏相位且已确认选中非空
- **WHEN** 渲染一帧
- **THEN** 画面 MUST 含 viewmodel 右手且 MUST 隐藏空闲左手且 MUST NOT 含自身身体模型

#### Scenario: 第三人称渲染自己且无手

- **GIVEN** 第三人称背面游戏相位
- **WHEN** 渲染一帧
- **THEN** 画面 MUST 含自身身体模型且 MUST NOT 含 viewmodel 像素

