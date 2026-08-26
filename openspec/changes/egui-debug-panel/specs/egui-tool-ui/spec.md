## MODIFIED Requirements

### Requirement: client ABI v9 结构化设置事件扩展

`mornlea_client` ABI SHALL 提升到 v9：保留字体上传与帧 TLV tag 9，UI 下行保留 layout v1 主菜单并新增 layout v2 设置页；UI 上行 SHALL 由裸按钮 ID 序列升级为有版本、有类型、有长度且顺序稳定的结构化事件批。ABI 版本不匹配 MUST 在所有出口被拒绝，v8 与 v9 二进制不可混装。

#### Scenario: 版本号三处一致

- **GIVEN** Rust client、C header 与 Go 绑定
- **WHEN** 检查 ABI 版本常量和查询出口
- **THEN** 三处 client ABI MUST 均为 `9`
- **AND** ABI 查询出口 MUST 返回 `9`

#### Scenario: 设置 UI 非法下行被拒绝

- **GIVEN** UI 段携带未知 layout、非法 UTF-8、NaN/Inf 音量、越界路径、未知窗口预设、非法布尔值或尾随字节
- **WHEN** 渲染该帧
- **THEN** MUST 返回 `INVALID_ARGUMENT` 且不触碰渲染器 UI 状态

#### Scenario: 结构化事件保持同帧顺序

- **GIVEN** 同一 egui 帧先完成文本编辑再点击保存
- **WHEN** Go 排空结构化事件批
- **THEN** 设置变化事件 MUST 排在保存 action 之前
- **AND** Go MUST 能用该批中的最终草稿执行保存

#### Scenario: 缓冲不足不排空事件

- **GIVEN** 待排空结构化事件的完整编码长度大于调用方缓冲
- **WHEN** 调用事件排空出口
- **THEN** MUST 返回 `CAPACITY`
- **AND** 调用方缓冲、输出计数与待排空队列 MUST 保持不变

### Requirement: 调试面板 layout v3 段

UI 下行 SHALL 除 layout v1 主菜单与 v2 设置页外接受 layout v3 调试面板状态段，编码为定宽行记录：段头（layout=3 + 可见 flags + 模式名 + 行计数）后接每行定宽记录（标签 ≤24 字节、值 ≤24 字节、每行 flags 编码只读/选中/可编辑/编辑态）。未知或非法 layout MUST 被拒绝。调试面板行数上限 `64`、读数 `7`、每行标签/值 `24` 的既有约束 MUST 保留；段长度 MUST 受既有 `MAX_UI_SEGMENT_BYTES` 上界约束。

#### Scenario: 合法 layout v3 段被接受

- **GIVEN** 布局合法、flags 合法、行数与各类长度合法的 layout v3 段
- **WHEN** 渲染该帧
- **THEN** MUST 成功，面板状态按段呈现

#### Scenario: 非法 layout v3 段被拒绝

- **GIVEN** 未携带 UI 或 UI 段携带未知 layout、空模式名、行数超上界、标签/值超 `24` 字节或截断字节
- **WHEN** 渲染该帧
- **THEN** MUST 返回 `INVALID_ARGUMENT` 且不触碰渲染器 UI 状态

### Requirement: 调试面板事件上行

UI 上行结构化事件批 SHALL新增调试面板动作类型：选中行移动、进入编辑、编辑值输入、确认写回、取消编辑、关闭面板。每帧事件批 SHALL 按 egui 产生顺序依次传递，Go 侧 MUST 依序消费。

#### Scenario: 编辑序列事件顺序稳定

- **GIVEN** 同一 egui 帧用户先移动选中行、再进入编辑、再输入值、再确认
- **WHEN** Go 排空结构化事件批
- **THEN** 移行、编辑、值输入、确认事件 MUST 按顺序出现

#### Scenario: 未知调试动作被拒

- **GIVEN** 上行事件批含未知调试面板动作类型
- **WHEN** 渲染该帧
- **THEN** 事件批 MUST 被拒绝
