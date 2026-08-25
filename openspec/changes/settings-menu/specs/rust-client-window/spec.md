## ADDED Requirements

### Requirement: 固定窗口预设决定初始与设置后尺寸

Darwin 图形客户端 SHALL 只接受 `640x360`、`960x540`、`1280x720` 三个逻辑内容尺寸预设，缺省为 `1280x720`。交互启动 SHALL 以已保存预设创建窗口；设置保存成功后 SHALL 立即请求新预设。创建与运行期调整 MUST 读取当前 NSWindow 所在 NSScreen 的 `visibleFrame`（逻辑 point），按当前窗口 style/chrome 从 outer frame 预算中扣除非内容区域后保持 16:9、只缩不放大；调整后的 outer frame MUST 重定位到 `visibleFrame` 内。物理帧缓冲仍 MUST 独立受 `2560x1440` 上限约束，不得把 Retina 物理像素与 AppKit 逻辑 point 混算。

#### Scenario: 已保存预设用于创建窗口

- **GIVEN** 配置的 `windowSize` 为 `960x540`
- **WHEN** 图形客户端创建交互窗口
- **THEN** 初始逻辑内容尺寸请求 MUST 为 `960x540`
- **AND** 物理帧缓冲超过上限或显示器工作区不足时 MUST 等比缩小而不放大

#### Scenario: 保存后运行期调整仍受上限约束

- **GIVEN** 当前窗口与保存后的新预设不同
- **WHEN** 设置保存成功并请求调整窗口内容尺寸
- **THEN** 窗口 MUST 请求新预设并刷新尺寸快照
- **AND** 最终尺寸 MUST 保持 16:9、位于显示器工作区内且物理帧缓冲不超过 `2560x1440`

#### Scenario: 标题栏和窗口位置计入工作区约束

- **GIVEN** 当前 NSScreen 的 `visibleFrame` 恰与请求 content 同为 16:9，但 NSWindow 具有标题栏，且窗口位于工作区右下边缘
- **WHEN** 创建窗口或运行期应用该预设
- **THEN** content MUST 为 outer-frame chrome 留出空间并等比缩小
- **AND** outer frame MUST 平移回真实 `visibleFrame`，包括该 frame 使用非零或负 origin 的多显示器布局

#### Scenario: 自动化路径不创建前台窗口

- **GIVEN** benchmark 或 capture 模式运行
- **WHEN** 配置或场景包含窗口预设
- **THEN** 自动化 MUST 继续使用既有离屏尺寸规则
- **AND** MUST NOT 创建、聚焦或调整前台窗口
