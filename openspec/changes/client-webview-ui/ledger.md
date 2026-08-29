# Ledger: client-webview-ui

## 2026-08-29 控制会话：change 设立

- 背景：D-10（PR #121）egui 换肤合并当日，用户实测裁决菜单「太丑」并完成架构选型三轮裁决：①UI 架构 = 进程内 WebView（否决自研 retained/框架换血路线）；②主菜单背景 = 世界全景；③前端栈 = Vite + TypeScript + React（否决零构建 vanilla）；④F3 同迁、egui 完全退役。
- 版本占用核查：PR #122（fix-gpu-benchmark-batch）已占 client ABI v11 → 本 change 唯一版本动量为 **client ABI v11→v12**。
- 关键取舍如实入账：菜单 chrome 退出像素 golden（系统 WebKit 不可钉死），替换为「全景底图像素 golden + vitest 组件断言 + 桥 schema 三端钉值」；WKWebView 透明背景为半文档特性，T2 首日 spike 带 kill-criteria（D9）。
- 产物：proposal.md、design.md（D1–D10）、tasks.md（6 组）、deltas（egui-tool-ui REMOVED / webview-menu-ui ADDED / visual-verification MODIFIED）。

## 评审与裁决记录

### 控制会话补充裁决（change 文档修订）

- 包管理工具用户裁决：**pnpm**（corepack `packageManager` 钉版、`pnpm-lock.yaml` 提交、`--frozen-lockfile` 唯一安装姿势）；design D3 / tasks 1.2 / proposal 已同步。本机 corepack 0.35.0 可用、pnpm 未全局安装（由 corepack 按钉版自动供给）。
- T1 首次派发被用户取消（未执行），按 pnpm 口径重发。

### T1 前端基建与脚手架 — PASS（零修复轮）

- Implementer 报告：Vite+React+TS(strict) 脚手架（pnpm 11.24.0 经 corepack `packageManager` 钉版、`pnpm-workspace.yaml allowBuilds: esbuild: false` 零 postinstall）；`tokens.css` 令牌与 HUD `style.go`/`ui/style.rs` 同族（linear×255 口径逐条注释来源）；`bridge/schema.json` 单源 + `client.ts` 类型化桥（64 vitest：schema 30/client 21/App 13，非法面含未知事件/动作/op/越界/交叉不变量）；四屏组件空壳文案与 `ui.rs` 逐字对齐；`make frontend-check` + CI frontend job 五道门禁等价；两次构建逐字节一致（sha256 前后相同）。
- SPEC PASS：动作映射表 ↔ Go `app_menu.go` ↔ `ui.rs` 常量逐值核对（1..9 与 DEBUG 1..7）；令牌四项复算一致；字段增补（version/error/dirty/status/pause.remote/debug.visible）均有 tasks 3.x 依据且与 ui.rs 字节常量同值。
- QUALITY PASS：client 违约不冲状态/迟到订阅重放/退订语义实读核验；零快照测试、零 any、零网络调用（grep 兜底门禁实测成立）；dist 无源码路径/机器信息泄漏；CI needs 图正确。
- 控制会话随裁决收口：design/spec 三处 `npm ci` 残留措辞 → pnpm 口径；dist 随本组提交入库（一致性门禁开始咬合）。非阻塞移交：copy.ts windowSize 字面量重复、schema enum ↔ TS 常量钉值测试建议（T3 可收）。

### T2 WKWebView 集成 + 桥 + client ABI v12 — PASS（R1 一轮修复；中途 agent 超时一次由控制会话接续重启）

- Spike 三假设全部成立，无 kill-criteria：①透明性——macOS 26 未公开 `drawsBackground` 选择器，实现公开 setter → wry 同款 KVC（exception::catch 包裹）→ 不透明降级三级守卫，冒烟日志证明 KVC 写入成功；视觉穿透确认留 T5（前端 body 暂不透明）。②scheme handler 内嵌字节零网络零取舍。③相位路由成立（菜单 firstResponder / 游戏隐藏归还 winit；首次需要可见才惰性挂载，`-connect`/benchmark/capture 永不创建 WebView）。
- Spike 中修复两真问题：`didFinishNavigation` 早于 ES 模块执行 → 自投递脚本（16ms×10s 页面内重试）；缺失选择器 NSException 穿透 abort → 三级守卫。
- ABI v12：删 `upload_ui_font` 全链与帧 tag 9 段（白名单收窄 1..=8）；增 `ui_push_state`（窗口句柄域、惰性挂载、同状态幂等）；drain 签名不变、字节格式改 `{v:1,events[≤64]}` JSON 信封（空队列 0 字节）。三处版本常量 + Go 钉值测试。
- Go：`ui_bridge.go` 深校验（25 拒绝路径）、`app_ui_state.go` 事件驱动推送（game 相位常量快路径、调试行 24 码点截断）、动作数字化→字符串 id、组装 JSON 对真实 schema.json 自实现校验器全量校验（mutant 验证会咬）。行为语义测试平移后全绿（app 61.5s）。
- 冒烟实证（控制会话亲核截图）：WebView 呈现 React 主菜单（标题/四按钮/多人禁用/版本行），挂在 wgpu 暗色天空上；一次设置页截图溯源为光标 click-through，反向实证按钮命中→上行→相位切换→下行完整回路。
- SPEC PASS（范围纪律：egui 仅停用未删，符合 T4 边界）。QUALITY R0 FAIL（cargo fmt 18 处、clippy 4 条、SAFETY 注释失实、任务编号泄漏三处、注释损伤两处）→ R1 全修：fmt/clippy 归零；治本删除多余 `HostShared.webview` 弱引用字段（navigation delegate 消息自带 &WKWebView）；删无谓 `unsafe impl Send`；线程模型注释如实化；句柄缺失分支改降级语义；编号/措辞/注释全清。QUALITY(RE) PASS。
- 移交 T3/T4：DrainUIEvents 对页面枚举漂移是 panic 口径（三端钉值 CI 拦截，T3 评估）；WebContent 崩溃无自动 reload（`webViewWebContentProcessDidTerminate` 兜底）；`pushedUIStates` 夹具无消费测试（补「变化才推送」断言）；F3 场景每帧推送 ~60Hz（与旧 egui 同量级，措辞勿理解为降频）；编辑播种精度与面板可见时 F3/Esc 经桥重组（T3 核心项）；后台启动窗口偶发不前置（启动时 `window.Focus()` 体验项）。

### T3 四屏行为语义平移 — PASS（零修复轮；工作树含超时会话遗留改动，implementer 审查整合并修复一处损坏）

- Implementer 报告：键盘路由集中 `App.tsx routeKeyDown`（Esc 优先级栈：调试面板→暂停→设置；menu Enter 默认按钮；编辑态忽略方向键/Enter 不 preventDefault 保光标）；settings Esc=返回脏草稿由 Go 裁决；暂停层 Esc 关层经 React→`pause-back`，与 winit 开层互斥无回声（arm/takeResume 哨兵）；F3 面板编辑播种以下行 `editValue`（schema 唯一新增字段三端同步：TS 解析守卫/Go 组装 omitempty 只读行不携带/校验器全量验证），全精度不变量两端互钉；移交项全收口（`TestPushUIStateOnlyOnChange`、`webViewWebContentProcessDidTerminate` reload 自愈、启动 Focus 恰一次、DrainUIEvents panic 口径评估维持现状）。
- 冒烟：AXPress 点击注入成功，12 张截图全链路（主菜单→设置→脏草稿 Esc 阻止提示→取消→装配隐藏 WebView→暂停层→F3 面板→退出 exit 0）。
- SPEC PASS：settings-menu/debug-panel/webview-menu-ui 三 spec 行为逐条有测试；路由表逐行核验；三端同步证据齐（81 vitest/go race/cargo 188 全绿亲自复跑）。
- QUALITY PASS：遗留改动整合连贯无死代码；editValue 64 vs 24 截断口径核实（播种值不截断、构造性上界成立）；reload 不自激循环。
- 控制会话顺手收口：app_menu.go Escape 注释陈旧更新、debug_panel.go 引用不存在的测试名改正。
- **预存缺陷发现（记 backlog）**：暂停门置位时 `server.step` 整体跳过（含 KeepAlive 处理）而心跳走墙钟——暂停层停留 ~15-20s 必心跳超时拆链 exit 1；egui 时代已存在，与本地单机暂停语义无关远端时暴露，需独立 change（心跳循环与暂停门解耦）。
