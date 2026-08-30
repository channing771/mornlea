# client-webui-pixel-style Ledger

## Setup

- OpenSpec change: `client-webui-pixel-style`。
- 需求来源：用户指定以 retroui（pixel-retroui）统一 WebView 四面板像素
  风格；字体取舍经用户拍板——不采用 retroui 内置「Minecraft font」（版权
  疑虑 + 不覆盖 CJK），另引入缝合像素字体（Fusion Pixel，OFL-1.1）。
- 执行基线：main 本地 HEAD = dev-capture 终局提交（`c86b9c5a`），工作树
  干净；前端无 Tailwind、无任何二进制 Web 资产（`webview.rs` 资产表仅
  index.html/index.js/index.css 三项）；桥 schema 三侧互钉（Go/Rust/TS）。
- 隔离 worktree（用户指定）：`/Users/chen/work/mornlea/.worktrees/
  client-webui-pixel-style`，分支 `feat/client-webui-pixel-style`
  （基于本地 main `c86b9c5a`，含 dev-capture 服务便于视觉验收）；全部
  实现任务在 worktree 内执行，主工作树不再触碰。
- 关键边界（已核实）：菜单 chrome 像素不参与无头 golden 比对
  （`openspec/specs/webview-menu-ui/spec.md`），视觉验收走 dev-capture
  `/screenshot`；`pnpm-workspace.yaml` `allowBuilds: esbuild: false`，
  新增依赖不得引入 postinstall。

## Task 1: OpenSpec change 产物

- 产物：proposal.md、design 待补（本 change 设计密度低，取舍记录在
  proposal 与 ledger：Minecraft font 否决原因、音量滑块保留原因、preflight
  关闭原因）、specs/webview-menu-ui/spec.md（2 条 ADDED Requirement、
  5 个 Scenario）、tasks.md、本 ledger。
