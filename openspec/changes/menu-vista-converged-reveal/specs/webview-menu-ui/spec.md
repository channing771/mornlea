## ADDED Requirements

### Requirement: 全景在装配收敛前不揭示

主菜单与设置页的全景 SHALL 在装配收敛（待生成区块、待烘焙段、待上传与远环队列全部清空）之前不渲染部分装配的几何：等待期帧 MUST 只呈现天空清屏底色，WebView 菜单 chrome MUST 保持可见与可交互。收敛后的首帧 MUST 从相机脚本 tick 0 开始，收敛后的帧内容与相机轨迹 MUST 与既有「全景背景确定性」契约逐帧一致。装配泵速 MUST 保持确定性调度，MUST NOT 引入非确定顺序。

#### Scenario: 进入主菜单无逐块浮现

- **GIVEN** 交互客户端完成窗口与渲染器初始化并进入主菜单相位
- **WHEN** 全景装配尚未收敛
- **THEN** 已呈现的帧 MUST 不包含任何部分装配的全景几何
- **AND** 菜单按钮 MUST 可点击

#### Scenario: 收敛后从脚本起点揭示且轨迹不变

- **GIVEN** 全景装配收敛
- **WHEN** 后续帧渲染
- **THEN** 相机脚本 MUST 从 tick 0 开始推进
- **AND** 相同收敛状态下同一 tick 的帧 MUST 逐位一致（既有确定性场景继续成立）

#### Scenario: 全景构建失败仍降级

- **GIVEN** 全景管线构建失败
- **WHEN** 菜单相位渲染
- **THEN** MUST 维持既有天空底色降级，菜单 MUST 保持可用
