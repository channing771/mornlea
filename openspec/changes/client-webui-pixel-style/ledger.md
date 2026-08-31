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
## Task 5: 视觉验收与文档同步

- Commits: `91296c84` `docs(frontend): add pixel component bridging
  discipline`。
- 视觉验收方式（环境适应）：验收时本机无可用显示器（`list_displays` 为
  空），游戏窗口无处合成、dev-capture 截图恒为空背板（三连哈希一致
  `00e678fe`，帧循环存活、dylib 为 worktree 新鲜构建，排除 UI 缺陷）；改走
  浏览器 harness——本地静态服务分发 worktree `dist/`，按 schema 构造四相位
  fixture 经 `window.mornlea.onState` 驱动，逐屏截图目检：
  - 主菜单：像素字体（标题/中文按钮/版本行全覆盖）、retroui 硬描边 + 硬
    阴影按钮、禁用态置灰、纵排几何符合规格；
  - 设置：像素滑块（方形琥珀拇指 25%）、PixelInput、窗口三预设按钮组
    （选中项琥珀高亮）、Card 面板描边；
  - 暂停：像素标题「已暂停」+ 双按钮；
  - 调试面板：模式头/readout/分组线/参数行，选中行琥珀左缘标记。
- 截图归档：IAB 会话产物（会话工件目录）；主菜单另有浏览器截图
  `build/devcapture-check/pixel-menu*.png`（主树 build，gitignored）。
- 遗留（记录不阻塞）：WKWebView 真机内嵌路径（mornlea:// scheme 同源字体
  加载）待显示器恢复后以 `--dev-capture` 复核一次；IAB 大视口截图管线在
  无显示环境下不稳定（与 UI 无关）。
- 文档：AGENTS.md 增补四条可判定像素组件纪律（桥接层强制、样式表引入与
  升级注意、DropdownMenu/16px 穿透两条已核实约束、通用类名前缀隔离）。

## Task 6: 收尾门禁

- 终局门禁（最终 HEAD `91296c84`，全部真实捕获）：
  - `make frontend-check`：冻结安装 + typecheck + 82 vitest + 构建 +
    dist 字节一致，EXIT=0；
  - `make rust-check`：mornlea_client 98 passed / mornlea_engine
    218 passed（webview.rs 资产表扩展后完整复验）；
  - `go test ./internal/archcheck -count=1`：ok；
  - `openspec validate --all --strict --no-interactive`：Totals: 80 passed,
    0 failed；
  - `go test ./... -race` 未跑：本 change 零 Go 行为改动（archcheck 为
    准，按 tasks.md 6.1 口径）。
- 分支终局：`feat/client-webui-pixel-style`，main 本地 HEAD（`c86b9c5a`，
  含未推送 dev-capture 18 提交）之上 14 笔提交，工作树干净，未 push。
- 遗留：合并/推送方式待用户指令；PixelDropdown 上游残留面首个消费方前须
  补映射；孤儿令牌（--radius-control/--slot-well-edge）后续顺手清理。
## Task 7: UI 部件视觉基线

- Commits: `2f27721e` `feat(frontend): add per widget visual baselines`、
  `a5722f75` `fix(frontend): harden visual capture close path`。
- 实现：`frontend/visual/`——`fixtures.tsx` 注册表（12 部件：四整屏以
  fixture props 原样渲染生产面板组件；button-default/disabled/pressed、
  input-text、preset-group、slider、debug-rows、error-line 走 pixel.tsx 与
  生产语义 class，零假货）、`fixture-names.ts` 单源（`as const` + 字面量
  联合，registry `Record<FixtureName, …>` 编译期互钉，双向 TS 实验：
  多键 TS2353/缺键 TS2741）、`visual.vite.config.ts` 独立构建到 gitignored
  的 `visual-dist/`（dist/ 与其字节门禁零触碰，复跑 frontend-check 绿）、
  `visual.mjs` 自包含管线（随机端口静态服务 + Chrome headless CLI +
  pngjs 双阈值比对，MAX_CHANNEL_DELTA=2 / MAX_DIFF_PIXEL_RATIO=0.0001 与
  `visual_compare.go` 同值同义；缺基线报错不自动创建；候选与 diff 图出
  `build/visual-ui/`）；Make 目标 `frontend-visual-check`/
  `frontend-visual-update`；12 张 1280×720 基线 PNG（208K）入库。
- 确定性与检出证据：连续两次 update 12 张 sha256 逐字节一致；人为 1px
  margin 漂移被 check 指认正确部件（通道差 245、差异 0.1286%）并出红标
  diff 图，复原后 12/12 绿。pngjs 7.0.0 零依赖零 install 钩子（独立复核）。
- 评审（独立评审子代理）：规格合规 PASS + 代码质量 PASS。修复轮 1：
  `a5722f75`——close 分支先消费截图再清 profile（原顺序对正常退出型
  Chrome 必失败）、`visual-typecheck` 接线为 check 前置、单源改 .ts 实现
  编译期互钉（JSON 宽化经实证无效）、`closeAllConnections()`、460px 字面量
  双向交叉引用注释。执行偏差一处：pnpm 不在脚本 PATH，visual-check 内联
  tsc 命令（仓库禁 npm 嵌套，合理）。
- 本机工具口径：不进 CI、零网络、Chrome 路径 env `CHROME_BIN` > 默认路径；
  Chrome 151 写完截图挂起为上游已知行为，管线以尺寸稳定轮询 + 进程组终止
  兜底并记入 AGENTS.md；exec 沙箱会挡 Chrome 写自身目录，管线须在正常
  终端执行。
