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
## Task 3: 缝合像素字体入库 + webview.rs 资产表扩展

- Commits: `342e8e27` `feat(frontend): embed fusion pixel cjk font asset`。
- 字体溯源：TakWolf/fusion-pixel-font release `2026.08.11`（tag 无 v 前缀），
  12px proportional zh_hans woff2，914,144 B，SHA256
  `4e2e29830c8b4e0f19bf7b6f01f1fa7347c48dab9418824a2a289919d4a8c748`；
  OFL.txt（4,418 B）随库入库；评审经 GitHub API 独立重下载比对，字体与
  许可均与上游逐字节一致；内部家族名 `Fusion Pixel 12px Prop zh_hans`
  （fontTools 读 name 表），cmap 36,521 码点覆盖全部 UI 汉字。
- dist 体积：字体 914,144 B + index.css 11,001 B（+183 B 的 @font-face），
  index.js 204,227 B 不变；两次构建字节一致；增量编译面：字体文件变更会
  重编 webview.rs 所在编译单元（include_bytes!）。
- 实现：@font-face（font-display: swap）与像素优先字体栈落 `ui.css`
  （偏差已核实：tokens.css 从无字体栈与「零二进制」表述，tasks.md/proposal
  措辞随本节一并订正）；webview.rs 资产表 +1 条目（EMBEDDED_PIXEL_FONT，
  路径与产物逐字一致），无 ABI 变化；index.html/AGENTS.md「零二进制」
  口径改「仅限 OFL 字体白名单」。
- 验证：make frontend-check 绿、cargo test -p mornlea_client 98 passed、
  go test ./internal/archcheck ok。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。字体级呈现留
  任务 5.1 人工验收（实现未伪造自动断言）；评审提示：WKWebView 对
  mornlea:// scheme 的字体请求走 CORS 判同源应放行，若被拒则按回退链
  优雅降级，5.1 目检时专门确认。
## Task 4: pixel.tsx 主题桥接 + 四面板 retroui 化

- Commits: `8abd9b8a` `feat(frontend): unify panels on pixel retroui
  components`、`2aeb1392` `fix(frontend): pin debug edit input font size to
  token`。
- 实现：`src/ui/pixel.tsx` 薄桥接（PixelButton/PixelInput/PixelCard/
  PixelDropdown，原生属性全透传、主题全经变量）；retroui 组件样式表
  `pixel-retroui/dist/index.css` 显式引入；tokens.css 新增像素组件变量段
  （`:root:root` 双写压过 retroui 产物自带 `:root` 亮色兜底，评审实测
  编译产物规则序证实必要）；四面板改造 + 音量滑块纯 CSS 像素重绘（滑块
  形态与键盘/ARIA 零改动）；`--pixel-*` 几何令牌与琥珀强调映射（选中/
  悬停/焦点环/滑块拇指），危险红仍专属错误。
- 偏差裁决（评审独立核实成立）：窗口三预设弃 retroui DropdownMenu——上游
  实现为点击弹出菜单件（菜单项普通 div、零键盘处理、事件不透传、选中不
  收起），采用必违反「焦点/键盘语义逐项一致」MUST 并打破 App.test 钉值
  断言；保持 PixelButton + `aria-pressed`。
- 测试：App.test.tsx 零改动 82/82 全绿；dist index.css 31.89 kB /
  index.js 208,202 B，两次构建字节一致。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。修复轮 1：
  `2aeb1392` 钉 `.debug-edit-input` 字号到 `--font-message`（上游
  `.pixelContainer` 硬编码 16px 穿透链 + 16px 高内容框裁切风险）。Minor
  记录：PixelDropdown 上游残留面（未渲染路径，首个消费方前须补映射与
  reduced-motion）、孤儿令牌保留、格式 nit。
