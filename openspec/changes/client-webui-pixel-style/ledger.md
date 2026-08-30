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
- 验证：`openspec validate client-webui-pixel-style --strict` valid；
  `openspec validate --all --strict` Totals: 80 passed, 0 failed。
- Commits: `331d0609` `docs: add client-webui-pixel-style change products`。

## Task 2: pixel-retroui + Tailwind 构建链接入

- Commits: `1a780213` `feat(frontend): add pixel-retroui and tailwind build
  chain`。
- 实现：`pixel-retroui ^2.1.0`（运行时，peer 官方含 React 19）+
  `tailwindcss ^3.4.19`/`postcss ^8.5.26`/`autoprefixer ^10.5.4`（开发）；
  `tailwind.config.js`（ESM，content 扫 index.html + src + retroui dist，
  preflight=false）、`postcss.config.js`、ui.css 头部三指令；锁文件 +110 包
  全部零 preinstall/install/postinstall（脚本遍历 node_modules/.pnpm 核验，
  评审独立复核）；`retroui_smoke.test.tsx` 冒烟（React 19 渲染真 Button，
  RED→GREEN）；dist 仅 index.css 7261→10818 B（+3.5KB，js/html 字节不变），
  两次构建 diff 空。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。三项 Minor：
  冒烟断言重解释（tasks.md 原文「retroui 组件类进产物 CSS」在 retroui
  v2.1 CSS Modules 架构下字面不可满足——组件样式在 `dist/index.css`
  预编译文件里由任务 4 显式引入，工具类进产物已由 dist 字节门禁钉住；
  评审裁决为任务文本架构误设，记录在案）；通用工具类（.container/.hidden
  等）进入全局命名空间（现有类名全前缀隔离零冲突，任务 4/5 避免裸用）；
  ui.css 注释一处理由不精确（无害）。Important 一项为 ledger 补记，本节
  即补记。
- 任务 4 关键输入（控制会话核实）：retroui 组件样式是预编译 CSS Modules
  （`pixel-retroui/dist/index.css`，exports 暴露），主题经 CSS 变量回退链
  （`var(--button-custom-bg, var(--bg-button, #f0f0f0))`）——桥接层只需在
  tokens.css 定义 `--bg-button`/`--border-button`/`--shadow-button` 等变量
  映射既有令牌，并显式引入该样式表。
