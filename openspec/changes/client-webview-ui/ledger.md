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
