# 设计：客户端菜单层迁移 WebView 与世界全景背景

## Context

PR #121 已交付 HUD/容器的原版对齐与混合风格（Go quad 管线 + `style.go` 令牌），菜单层 egui 换肤只是止血。用户裁决：窗口型 UI 迁进程内 WebView（Vite + TS + React），F3 同迁、egui 完全退役，主菜单用世界全景背景。本设计给出集成架构、桥协议、构建链与确定性策略。

品味纪律延续：单一琥珀强调色 + 深色半透明一族（与 HUD `style.go` 同族语义）、零 em-dash、颜色无关可辨（几何/文字层级不依赖颜色）、`prefers-reduced-motion` 尊重。

## D1 分层边界与不变式

- **窗口型 UI**（主菜单/设置/暂停/F3）→ WKWebView + React。
- **生存 HUD/容器/准星/弹条/tooltip** → Go quad 管线不动（像素 golden 契约、输入延迟、benchmark 容量口径全部不碰）。
- 菜单状态权威在 Go（相位机/配置/持久化），WebView 只是呈现层；前端不得持久化任何配置。

## D2 WKWebView 集成

- `objc2` + `objc2-web-kit`（维护良好的安全绑定）手写集成，不引 wry/tao（避免第二窗口栈）。
- WKWebView 以 `addSubview` 挂到 winit NSWindow contentView 之上，frame 跟随窗口（autoresizing），`drawsBackground=false` 透明露出 wgpu 画面（半文档特性——T2 任务组首日 spike 验证，kill-criteria 见 D9）。
- 资产供给：`WKURLSchemeHandler` 注册 `mornlea://` scheme，从 Rust 内嵌字节（`include_dir` 指向 `frontend/dist`）供给 index.html/JS/CSS/字体；零磁盘写入、零网络、零 CDN。
- 相位路由：游戏相位 `webView.hidden = true` 且不参与 responder 链（winit 独占输入，行为与「无 UI 帧时零参与」等价）；菜单相位 `hidden = false` 并把 WebView 设为 firstResponder 消费键盘/指针，游戏输入本就被相位抑制（Esc 栈语义经桥上行重组）。
- 非目标：Windows/Linux webview（仓库客户端本就 Darwin 独占）。

## D3 前端栈与构建链

- `engine/crates/mornlea_client/frontend/`：Vite + React + TypeScript（strict）。npm（package-lock 提交，`engines.node` 钉 LTS 主版本，CI 用同版本）。
- 结构：`src/ui/`（App、MainMenu、PauseMenu、SettingsPanel、DebugPanel 组件）、`src/bridge/`（类型化 client：`window.mornlea` 注入对象 + 事件订阅）、`src/tokens.css`（设计令牌：色板/间距/字号/圆角/阴影，与 HUD 令牌表同族并排记录）、vitest 组件断言（@testing-library/react）。
- 产物：`vite build` 输出 `frontend/dist/` **提交入库**（本仓库哲学：可复现、离线、无 CI 隐依赖）；`make frontend-check` = `npm ci && tsc --noEmit && vitest run && vite build && git diff --exit-code dist`（构建产物与入库版本一致性门禁）；CI 增加同步骤。
- 排版/材质：面板 = 深色半透明 + 1px 亮边 + 圆角 8 + 大间距尺度（设计令牌驱动）；按钮三态（静默/悬停/焦点琥珀描边）；`prefers-reduced-motion` 时动效全关。

## D4 桥协议

- 单源 schema：`frontend/src/bridge/schema.json`（JSON Schema 草案 2020-12）。
- **下行**（Go → Rust → JS）：事件驱动 `push_ui_state`——Go 组装 `{phase, menu:{title,buttons:[{id,label,enabled}]}, settings:{draft:{audioVolume,texturePackPath,windowSize}, saved:{...}}, debug:{rows:[...], mode}}` JSON，Rust 经 `evaluateJavaScript` 调用 `window.mornlea.onState(state)`；仅在状态变化时推送（无每帧流量）。
- **上行**（JS → Rust → Go）：`window.webkit.messageHandlers.mornlea.postMessage(event)` → WKScriptMessageHandler → Rust 队列 → 既有 `drain_ui_events` 出口改 JSON 事件批（保留版本化信封：`{v:1, events:[{type:"action",id:"enter-game"}|{type:"settings-change",field,value}|{type:"debug-edit",...}]}`）；Go 依序消费，动作语义与现常量一一对应（跨语言钉值测试平移）。
- 三端钉值：Go/Rust/TS 各自的 schema 一致性测试共同引用同一 `schema.json` fixture。

## D5 client ABI v11→v12

- `MORNLEA_CLIENT_ABI_VERSION` 12（三处 + Go 绑定 + 版本查询出口）。
- 退役出口：`mornlea_client_render_upload_ui_font`（字体随 dist 内嵌 @font-face）、帧 TLV tag 9 UI 段及其 layout v1–v4 编解码（Go `EncodeUIMenu` 等删除）。
- 新出口：`mornlea_client_ui_push_state`（JSON 字符串）、`drain_ui_events` 信封格式更新（版本字段 v1）。
- benchmark 路径不受影响（本就不上传字体/不运行 UI）；协议/schema/存档/benchmark scenario 不变。

## D6 世界全景背景

- 「装配」红线不动：不打开世界存储、不启动本地权威服务端、不登录。全景是**渲染层演示场景**：固定种子调用 worldgen 生成菜单相机周围有限区块 → 既有 mesher → 既有地形/天空/光照 pass。
- 相机：固定高度环绕脚本（缓慢自转 + 固定俯仰），正午固定世界时间；世界内容确定性（同种子逐格一致）。
- 生效相位：主菜单与设置页（无已装配世界的相位）；游戏/暂停相位不渲染全景（暂停遮罩下是世界本身）。
- 无头 capture：webview 不参与无头路径 → `main-menu`/`settings-menu` golden 变为**全景底图**（纯 wgpu，确定性像素比对继续成立）；WebView chrome 由 vitest 组件断言覆盖（系统 WebKit 像素不可钉死，如实入账）。

## D7 F3 调试面板迁移

- 行为语义从 `debug-panel` 平移（该 capability 行为规格不动，只换呈现）：`-dev` 门控、F3 边沿、只读读数区、参数分组、行选中/编辑/写回、联机 physics/sim 只读、64 行上限、面板可见时捕获输入。
- 桥映射：读数与参数行进下行状态 `debug.rows`；编辑事件（选中移动/进入编辑/值输入/确认/取消/关闭）走上行事件批。
- 字节上限（标签/值 ≤24）由 Go 侧组装时维持；前端展示层不设新限制。

## D8 egui 退役

- 删除：`egui`/`egui-wgpu` 依赖、`ui.rs`（2248 行）与 `src/ui/` 测试、egui RawInput 翻译、egui pass 与 `render/egui.rs`、Go 侧 `EncodeUIMenu`/layout v1–v4 编码/字体上传链。
- 保留：`UI_ACTION_*` 语义（桥事件 id 一一映射）、暂停门、Esc 栈相位机（Go 侧不动）。
- `egui-tool-ui` 主规格 REMOVED，行为语义逐条平移进新 capability `webview-menu-ui` 的 ADDED Requirements。

## D9 风险与 kill-criteria

1. **透明背景**：`drawsBackground=false` 为半文档特性。T2 首日 spike 验证；若失败，降级为 WebView 自绘不透明菜单背景（全景仅透过缝隙不可行时放弃全景并回退纯 CSS 背景），ledger 记录裁决。
2. **焦点路由**：webview firstResponder 与 winit 事件循环共存是集成核心风险；spike 验证键盘/指针双相切换与 Esc 语义。
3. **node 供应链**：lockfile 锁定 + `npm ci`（禁 install）+ dist 入库一致性门禁；不引入 postinstall 脚本依赖。
4. **桥类型安全**：schema 三端钉值 + 越界/未知事件拒绝（对应旧 ABI 拒绝语义）。

## D10 护栏

1. 协议 v31、玩家/区块 schema、world metadata、engine ABI、benchmark scenario v20、配置版本全部不变；唯一动量 client ABI v11→v12。
2. 世界场景 22 张像素 golden 与 HUD 契约（320/768/15616/52480）不动；HUD/容器/准星/弹条/tooltip 零改动。
3. 行为语义逐条平移（启动/装配/禁用/设置/Esc/暂停门/调试面板），由既有 app 级测试 + 新桥测试双重守护。
4. 无网络、无遥测、无 Mojang 资产；前端资产全部内嵌。
5. 注释纪律：Go/Rust/TS 注释中文、无任务编号；frontend 局部指南按 `docs/agents-md-style.md` 新增。
