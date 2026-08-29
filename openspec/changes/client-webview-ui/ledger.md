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
