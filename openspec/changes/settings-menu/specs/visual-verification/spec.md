## MODIFIED Requirements

### Requirement: 主菜单与设置菜单无窗口 capture 场景

视觉场景表 SHALL 包含 `main-menu` 与紧随其后的 `settings-menu`：两者均以既有 `640x360` 离屏渲染路径运行，回读像素并与 golden 按既有双阈值比对。`main-menu` MUST 显示启用的「设置」按钮；`settings-menu` MUST 以确定性的非默认音量、短材质路径和窗口预设覆盖全部首版控件。两场景 MUST 依次排在 `far-horizon` 之前，`far-horizon` 仍为倒数第二、`water-underwater` 仍为最后。

#### Scenario: 场景表顺序与两张 UI 图产出

- **GIVEN** `captureScenes` 场景表
- **WHEN** 检查 UI 场景与尾序
- **THEN** `main-menu` 与 `settings-menu` MUST 依次相邻并位于 `far-horizon` 之前
- **AND** `far-horizon` MUST 仍为倒数第二、`water-underwater` MUST 仍为最后
- **AND** 抓帧目录 MUST 含 `main-menu.png` 与 `settings-menu.png`

#### Scenario: 两个 UI 场景参与 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `main-menu` 与 `settings-menu`
- **THEN** 两张 PNG MUST 分别与对应 golden 逐像素比对
- **AND** MUST 继续使用既有阈值与差异图产出规则

### Requirement: 未受影响场景 golden 逐字节不变

本变更 MAY 只更新因「设置」启用而改变的 `main-menu.png` 并新增 `settings-menu.png`；所有不携带设置 UI 的既有正式场景 golden SHALL 保持逐字节不变。

#### Scenario: 非设置场景不受变更影响

- **GIVEN** 全部既有正式场景（不含 `main-menu`）
- **WHEN** 运行 capture 并与变更前 golden 比对
- **THEN** 每个场景的 PNG 字节 MUST 保持不变
- **AND** 除更新 `main-menu.png` 与新增 `settings-menu.png` 外 MUST NOT 产生其他 golden 变更

## RENAMED Requirements

- FROM: `### Requirement: 主菜单无窗口 capture 场景`
- TO: `### Requirement: 主菜单与设置菜单无窗口 capture 场景`
- FROM: `### Requirement: 既有场景 golden 逐字节不变`
- TO: `### Requirement: 未受影响场景 golden 逐字节不变`

