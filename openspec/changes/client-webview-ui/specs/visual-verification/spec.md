# visual-verification Delta

## MODIFIED Requirements

### Requirement: 主菜单与设置菜单无窗口 capture 场景

视觉场景表 SHALL 包含 `main-menu` 与紧随其后的 `settings-menu`：两者均以既有 `640x360` 离屏渲染路径运行，回读像素并与 golden 按既有双阈值比对。菜单 chrome 由 WebView 呈现且 MUST NOT 参与无头 capture——两张 golden 的内容 SHALL 为对应相位的世界全景底图（纯 wgpu 渲染、确定性像素），WebView 层的结构与视觉 MUST 由前端组件断言测试覆盖而非像素 golden。两场景 MUST 依次排在 `far-horizon` 之前，`far-horizon` 仍为倒数第二、`water-underwater` 仍为最后。

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
- **AND** 比对对象 MUST 为纯 wgpu 全景底图，无头路径 MUST NOT 初始化 WebView、产生任何菜单 chrome 像素或网络请求
