# game-overlay-webview

## Purpose

为游戏相位建立 WebView 的 GameOverlay 参与模型与常显 HUD 的 CSS 呈现契约：WebView 在游戏相位可见、透明合成于 GPU 画面之上并承载常显 HUD 组件，但 MUST NOT 参与响应链；HUD 数据沿既有桥出口按权威 tick 合并下行，权威边界、颜色无关可辨性与资产离线纪律在 CSS 呈现中等效保留。

## ADDED Requirements

### Requirement: 游戏相位 WebView 以 GameOverlay 模式呈现且不参与响应链

交互式图形客户端在游戏相位（世界已装配、无菜单相位、无容器打开态的常显阶段）SHALL 以 GameOverlay 模式挂载 WebView：WebView MUST 可见并透明合成于 wgpu 画面之上，承载常显 HUD 组件；同时 MUST NOT 参与响应链——鼠标、键盘与滚轮事件 MUST 全部由既有 winit 采集路径处理，WebView MUST NOT 因指针悬停或点击产生任何上行事件、MUST NOT 改变光标捕获状态。WebView 参与模式 SHALL 恰有两态：`Menu`（菜单相位，全参与，既有行为）与 `GameOverlay`（游戏相位常显阶段，仅合成）；benchmark、capture 与 `-connect` 无头路径 MUST 保持零 WebView 参与。GameOverlay 模式的建立 MUST NOT 引入新的 C ABI 出口，client ABI 版本 MUST 保持不变。

#### Scenario: 游戏相位 HUD 可见且输入行为与迁移前一致

- **GIVEN** 游戏相位且 GameOverlay WebView 呈现常显 HUD
- **WHEN** 玩家执行 WASD 移动、采掘、放置、快捷栏切换、聊天与 Esc 暂停等既有输入序列
- **THEN** 全部输入行为 MUST 与迁移前逐项一致，输入不因 WebView 的存在被吞、被延迟或被重复
- **AND** WebView MUST NOT 产生任何上行桥事件，桥事件排空结果 MUST 为空

#### Scenario: 参与模式两态切换与无头路径零参与

- **GIVEN** 客户端在菜单相位、游戏相位与无头路径（benchmark/capture/`-connect`）之间切换
- **WHEN** 相位迁移发生
- **THEN** 菜单相位 MUST 保持既有 Menu 模式（WebView 全参与、菜单 chrome 可交互），游戏相位 MUST 进入 GameOverlay 模式（仅合成）
- **AND** 无头路径 MUST 全程不初始化、不挂载、不渲染任何 WebView，与现状逐项一致

#### Scenario: GameOverlay 不改变渲染热路径契约

- **GIVEN** GameOverlay WebView 已挂载并持续合成 HUD
- **WHEN** 系统连续准备与呈现渲染帧
- **THEN** wgpu 渲染路径的每帧资源纪律、pass 顺序与稳定态零动态分配契约 MUST 保持不变
- **AND** WebView 合成开销 MUST 经本 change 的 spike 实测钉入性能门禁，MUST NOT 为此放宽任何真实 overflow 或报告完整性门禁

### Requirement: 常显 HUD 以 WebView 组件呈现且语义逐项平移

常显 HUD（快捷栏九格与选中标识、数量与耐久、生命、饥饿、氧气、采掘进度、进食进度、物品名弹条、准星、聊天呈现、权威命中 marker 显示）SHALL 由 WebView HUD 组件呈现，并满足以下平移约束：呈现数据 MUST 仅来自已确认权威镜像（经桥下行状态），未确认值 MUST 与迁移前同口径隐藏；生命/饥饿锚定快捷栏左右边缘、氧气沿饥饿外缘堆叠、行避让随容器打开态翻转的构图关系 MUST 等效保留；快捷栏选中格 MUST 保持双层轮廓、采掘可采/不可采 MUST 保持颜色与形状双差异的颜色无关可辨性；组件样式 MUST 经前端令牌单源供给并遵守 Mornlea 双强调体系与 `prefers-reduced-motion` 降级；聊天输入 MUST 保持既有 winit 采集路径（Phase 1 不迁移输入）。

#### Scenario: 权威镜像驱动与未确认值隐藏

- **GIVEN** 已确认权威镜像中的任意快捷栏、生命、饥饿、氧气与采掘状态组合
- **WHEN** HUD 组件呈现该组合
- **THEN** 呈现内容 MUST 与迁移前同源同语义：快捷栏恰九格、数量仅对多于一件的堆叠显示、耐久仅对部分磨损工具显示、生命按钳制值解析空心/半心/满心、饥饿常驻十格、满氧与未确认值完全不呈现
- **AND** 未确认的选中变化 MUST NOT 触发物品名弹条

#### Scenario: 构图关系与颜色无关可辨性等效保留

- **GIVEN** 任意合法窗口尺寸下的常显 HUD
- **WHEN** 检查组件构图与状态标识
- **THEN** 生命起点 MUST 对齐快捷栏左缘、饥饿终点对齐右缘、耗损氧气沿饥饿外缘堆叠、容器打开态行栈向外避让，构图关系 MUST 与迁移前一致
- **AND** 选中格的双层轮廓、采掘可采末端标记与不可采固定缺口的形状差异 MUST 忽略颜色后仍可判定

#### Scenario: 窄窗口与超大窗口的协调缩放

- **GIVEN** 从极小到极大的合法窗口尺寸序列
- **WHEN** HUD 组件布局
- **THEN** 常显 HUD MUST 以单一比例协调缩放，全部元素 MUST 位于视口内且互不遮挡关键信息，MUST NOT 出现固定像素导致的溢出或失控放大
- **AND** 零尺寸或非法尺寸视口 MUST 安全降级为不呈现

### Requirement: HUD 状态按权威 tick 合并下行

游戏相位 HUD 状态 SHALL 经既有 JSON 状态下行出口推送：同一权威 tick 内的多次状态变化 MUST 合并为至多一次推送且携带终态；无变化 MUST 零推送；推送 MUST NOT 按渲染帧触发。桥 schema SHALL 以单源 JSON Schema 定义游戏相位状态族，Go/Rust/TS 三端 MUST 各有钉值一致性测试；未知状态类型、越界字段或非法 UTF-8 MUST 被拒绝且不触碰运行态。权威命中 marker 的窗口计时 MUST 保持既有「成功呈现帧计数、失败不消耗」语义，计时状态留在 Go 侧，仅以状态变化驱动呈现。

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
