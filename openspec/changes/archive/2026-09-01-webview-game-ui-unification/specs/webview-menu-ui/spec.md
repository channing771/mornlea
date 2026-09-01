## MODIFIED Requirements

### Requirement: WebView 集成技术边界

`mornlea_client` SHALL 通过 `objc2-web-kit` 将一个透明 WKWebView 挂载到既有 winit NSWindow 的 contentView 之上，MUST NOT 引入 wry、tao 或第二套窗口栈；webview 资产 MUST 经 `WKURLSchemeHandler` 从二进制内嵌字节以 `mornlea://` 供给，MUST NOT 访问网络、CDN 或磁盘临时文件；`egui` 与 `egui-wgpu` 依赖 MUST 全部移除；`wgpu` 主版本 MUST NOT 单侧升级；任何 Go 包 MUST NOT 引入 GUI 绑定。WebView 参与模式 SHALL 恰有两态——菜单相位 `Menu`（全参与）与游戏相位 `GameOverlay`（可见合成、不参与响应链，见 `game-overlay-webview` capability）：两态 MUST 由同一 WKWebView 实例经命中测试分级（子类化 hitTest）实现，MUST NOT 依赖窗口叠层顺序或第二实例切换；GameOverlay 态的建立 MUST NOT 引入新的 C ABI 出口。

#### Scenario: 透明覆盖与资产离线供给

- **GIVEN** 菜单相位的客户端窗口
- **WHEN** WebView 呈现菜单 chrome
- **THEN** WebView 背景 MUST 为透明，wgpu 渲染的世界全景在其下可见
- **AND** 全部 HTML/JS/CSS/字体请求 MUST 由内嵌 scheme handler 供给，无任何网络请求

#### Scenario: 游戏相位零参与

- **GIVEN** 游戏进行中（GameOverlay 模式）
- **WHEN** 系统渲染帧
- **THEN** GameOverlay WebView MUST 保持可见合成且 MUST NOT 进入响应链（指针、键盘、滚轮事件全部由 winit 采集），wgpu 呈现路径与迁移前一致
- **AND** 桥事件排空 MUST 为空，游戏输入行为与无 WebView 路径一致

#### Scenario: 依赖版本线替换

- **GIVEN** `engine/crates/mornlea_client/Cargo.toml` 与依赖树
- **WHEN** 检查依赖
- **THEN** `egui`、`egui-wgpu` 及其传递依赖 MUST 不存在
- **AND** `objc2`、`objc2-web-kit` MUST 在锁文件中钉版本

### Requirement: 菜单状态与事件桥

菜单状态权威 SHALL 在 Go：Go 侧在状态变化时经 client ABI 以 JSON 字符串推送菜单/设置/调试状态下行，Rust 侧转发为 WebView 内 `window.mornlea.onState` 调用；上行 SHALL 由 WebView 脚本消息进入 Rust 队列并以版本化 JSON 事件批经既有排空出口交付 Go 依序消费。桥 schema SHALL 以单源 JSON Schema 文件为准，Go/Rust/TS 三端 MUST 各有钉值一致性测试；未知事件类型、schema 越界或非法 UTF-8 MUST 被拒绝且不触碰运行态。游戏相位 SHALL 经同一 JSON 下行出口推送常显 HUD 状态族（`game-overlay-webview` capability）：状态族按权威 tick 合并推送、禁止每帧重复推送；schema 的游戏相位状态族与既有菜单状态族共用单源文件与三端钉值纪律，client ABI 版本 MUST 保持不变。

#### Scenario: 状态下行事件驱动

- **GIVEN** 菜单相位下 Go 侧菜单/设置/调试状态发生一次变化
- **WHEN** 系统处理该变化
- **THEN** MUST 恰好推送一份包含变化后完整状态的 JSON，且 MUST NOT 存在每帧重复推送

#### Scenario: 游戏相位 HUD 状态 tick 合并下行

- **GIVEN** 游戏相位下同一权威 tick 内多类 HUD 状态变化
- **WHEN** 桥下行运行
- **THEN** MUST 恰好推送一份合并终态的 JSON；无变化的 tick MUST 零推送

#### Scenario: 上行事件保序

- **GIVEN** 同一交互序列先后产生设置值编辑与按钮点击
- **WHEN** Go 排空事件批
- **THEN** 事件 MUST 按 WebView 产生顺序出现，值编辑事件先于点击事件

#### Scenario: 非法桥载荷被拒绝

- **GIVEN** 桥载荷含未知事件类型、schema 越界字段或非法 UTF-8
- **WHEN** Rust 或 Go 处理该载荷
- **THEN** MUST 返回错误且不触碰菜单状态与渲染器状态
