## MODIFIED Requirements

### Requirement: 游戏相位 WebView 以 GameOverlay 模式呈现且不参与响应链

交互式图形客户端在游戏相位（世界已装配、无菜单相位、无容器打开态的常显阶段）SHALL 以 GameOverlay 模式挂载 WebView：WebView MUST 可见并透明合成于 wgpu 画面之上，承载常显 HUD 组件；光标被捕获时 MUST 不参与响应链，游戏输入全部交给 winit；容器打开或用户进入自由光标状态时 MUST 接收前端交互，并经严格桥事件请求操作或恢复游戏。WebView 参与模式 SHALL 恰有两态：`Menu`（菜单或游戏交互面，全参与）与 `GameOverlay`（游戏相位常显阶段，仅合成）；benchmark 与 capture 无头路径 MUST 保持零 WebView 参与；`-connect` 交互窗口 MUST 支持相同前端面板。GameOverlay 模式的建立 MUST NOT 引入新的 C ABI 出口，client ABI 版本 MUST 保持不变。

#### Scenario: 游戏相位 HUD 可见且输入行为与迁移前一致

- **GIVEN** 游戏相位、光标被捕获且 GameOverlay WebView 呈现常显 HUD
- **WHEN** 玩家执行 WASD 移动、采掘、放置、快捷栏切换、聊天与 Esc 暂停等既有输入序列
- **THEN** 全部输入行为 MUST 与迁移前逐项一致，输入不因 WebView 的存在被吞、被延迟或被重复
- **AND** WebView MUST NOT 产生任何上行桥事件，桥事件排空结果 MUST 为空

#### Scenario: 参与模式两态切换与无头路径零参与

- **GIVEN** 客户端在菜单相位、游戏相位与无头路径（benchmark/capture）之间切换
- **WHEN** 相位迁移发生
- **THEN** 菜单相位 MUST 保持既有 Menu 模式（WebView 全参与、菜单 chrome 可交互），游戏相位光标捕获时 MUST 进入 GameOverlay 模式，自由光标或容器交互时 MUST 进入全参与模式
- **AND** 无头路径 MUST 全程不初始化、不挂载、不渲染任何 WebView，与现状逐项一致

#### Scenario: GameOverlay 不改变渲染热路径契约

- **GIVEN** GameOverlay WebView 已挂载并持续合成 HUD
- **WHEN** 系统连续准备与呈现渲染帧
- **THEN** wgpu 渲染路径的每帧资源纪律、pass 顺序与稳定态零动态分配契约 MUST 保持不变
- **AND** WebView 合成开销 MUST 经本 change 的 spike 实测钉入性能门禁，MUST NOT 为此放宽任何真实 overflow 或报告完整性门禁


#### Scenario: 交互面切换不留下游戏输入
- **GIVEN** 游戏中按下移动或鼠标按钮
- **WHEN** 打开面板、释放光标或关闭面板恢复捕获
- **THEN** 交互期间 MUST 中和游戏输入，恢复时 MUST 无残留按钮与视角突跳；关闭与暂停快捷键 MUST 可用

### Requirement: HUD 状态按权威 tick 合并下行

游戏相位 HUD 状态 SHALL 经既有 JSON 状态下行出口推送：同一权威 tick 内的多次状态变化 MUST 合并为至多一次推送且携带终态；无变化 MUST 零推送；推送 MUST NOT 按渲染帧触发。面板开关、来源选择与光标参与模式属于本地交互状态，MUST 由实际状态变化及时下行，不受暂停的权威 tick 阻塞；相同交互状态不得空推。桥 schema SHALL 以单源 JSON Schema 定义游戏相位状态族，Go/Rust/TS 三端 MUST 各有钉值一致性测试；未知状态类型、越界字段或非法 UTF-8 MUST 被拒绝且不触碰运行态。权威命中 marker 的窗口计时 MUST 保持既有「成功呈现帧计数、失败不消耗」语义，计时状态留在 Go 侧，仅以状态变化驱动呈现。

#### Scenario: tick 合并与零空推

- **GIVEN** 同一权威 tick 内快捷栏镜像与生命值先后变化，随后连续多个 tick 无变化
- **WHEN** 桥下行运行
- **THEN** 该 tick MUST 恰好产生一次携带两者终态的推送
- **AND** 无变化的 tick MUST 产生零推送，推送频率 MUST NOT 与渲染帧率耦合

#### Scenario: 非法载荷被拒绝

- **GIVEN** 下行载荷含未知状态类型、越界字段或非法 UTF-8
- **WHEN** Rust 或 TS 侧处理该载荷
- **THEN** MUST 返回错误且不触碰 WebView 呈现状态与渲染器状态

#### Scenario: 三端 schema 钉值一致性

- **GIVEN** 桥 schema 单源文件发生任何字段演进
- **WHEN** 运行三端钉值一致性测试
- **THEN** Go、Rust、TS 的类型与守卫 MUST 与 schema 逐值一致，任一侧漂移 MUST 使门禁失败

