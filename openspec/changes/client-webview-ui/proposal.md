# 客户端菜单层迁移 WebView（Vite + TS + React）与世界全景背景

## Why

用户对 egui 菜单层的视觉裁决是「太丑」，并在架构选型中明确裁决：窗口型 UI 走**进程内 WebView**（真·现代前端），技术栈 **Vite + TypeScript + React**，F3 调试面板同迁、**egui 完全退役**，主菜单背景使用**世界全景**。egui（immediate-mode）无布局系统、无设计令牌级联、无动效模型，是菜单「调试工具感」的架构根因；现代前端栈给到组件化、声明式布局与设计令牌的完整开发体验。

## What Changes

- **新前端栈**：`engine/crates/mornlea_client/frontend/` 新增 Vite + TypeScript + React 应用（设计令牌 CSS、类型化桥 client、四屏组件），`npm ci + tsc + vitest + vite build`，构建产物 dist 提交入库并被 Rust 二进制内嵌；仓库新增 node 构建链（Makefile `frontend-check`、CI 步骤、frontend 局部指南）。
- **WKWebView 集成**：`objc2-web-kit` 手写 WKWebView 挂到既有 winit NSWindow contentView 上层；`drawsBackground=false` 透明覆盖露出 wgpu 画面；资产经 `WKURLSchemeHandler` 从内嵌字节供给（`mornlea://`，零磁盘写入、零网络）；菜单相位 WebView 接管输入，游戏相位 `hidden` 零参与。
- **桥协议**：Go 保持菜单状态权威——下行 `push_ui_state`（JSON、事件驱动）替代帧 TLV tag 9 的 layout v1–v4；上行 WKScriptMessageHandler → Rust 队列 → 结构化 JSON 事件批（保留 drain 形态）；桥 schema 单源文件 + Go/Rust/TS 三端钉值测试。
- **client ABI v11→v12**：`upload_ui_font` 退役（字体随 web 资源内嵌）、UI 帧段语义退役；协议/schema/存档/engine ABI/benchmark scenario 均不变。
- **世界全景背景**：主菜单与设置页相位以固定种子 worldgen 直供区块 + mesher + 天空光照渲染全景，固定脚本相机缓慢环绕；不打开世界存储、不启动本地权威服务端（启动语义红线不动）。
- **egui 完全退役**：`egui`/`egui-wgpu` 依赖删除，`ui.rs`/egui RawInput 翻译与 egui pass 全部移除，`egui-tool-ui` capability 退役并由新 capability `webview-menu-ui` 接替（全部行为语义平移）。
- **验证策略**：世界场景 22 张像素 golden 不变（无头 capture 无 WebView 参与）；`main-menu`/`settings-menu` 两张 golden 内容变为**全景底图**（纯 wgpu、确定性），WebView chrome 由 vitest 组件断言 + 桥协议三端测试覆盖（系统 WebKit 版本漂移不可钉死，菜单像素比对退役——确定性代价如实入账）。

## Impact

- **specs**：`egui-tool-ui` 全部 Requirements REMOVED（capability 退役）；新 capability `webview-menu-ui` ADDED（语义平移 + 技术边界）；`visual-verification` MODIFIED「主菜单与设置菜单无窗口 capture 场景」。
- **版本**：client ABI v11→v12 为唯一版本动量；协议 v31、玩家/区块 schema、world metadata、engine ABI、benchmark scenario v20、配置版本均不变。
- **行为不变**：启动停留主菜单、世界装配延迟、「多人游戏」禁用、设置三字段草稿/原子保存/生效时机、Esc 优先级栈、暂停门、退回主菜单、调试面板行为（只读/编辑/联机只读/64 行上限）、HUD 与容器交互。
- **新风险如实入账**：WKWebView 透明背景为半文档特性（集成任务组带 kill-criteria spike）；菜单 chrome 不再进像素 golden（改结构性断言）；仓库新增 node 供应链（lockfile 锁定 + dist 入库校验缓解）。
