# AGENTS.md — engine/crates/mornlea_client/frontend

本目录是菜单层 WebView 前端（Vite + TypeScript strict + React）：把 Go 组装的
菜单状态呈现为四屏组件，并把用户交互回传为上行事件。行为规格住 openspec
capability `webview-menu-ui`（当前 delta 见 `openspec/changes/client-webview-ui/specs/webview-menu-ui/spec.md`）；
菜单语义的权威在 Go（相位机、设置草稿、装配裁决），本目录只做呈现与回传。
Rust 侧只消费本目录的构建产物 `dist/`（经 `mornlea://` scheme 内嵌供给），
不导入本目录源码；本目录也不导入任何 Go/Rust 符号——该边界由评审与
`make frontend-check` 兜底。

## 桥协议单源（`src/bridge/schema.json`、`src/bridge/client.ts`）

- 下行状态与上行事件信封的形状只在 `src/bridge/schema.json` 定义一处（JSON
  Schema 草案 2020-12）；`src/bridge/client.ts` 的 TS 类型与守卫函数按 schema
  手写钉值，不引入 ajv 运行时进产物。改协议先改 schema，再同步类型与守卫。
- 未知相位、未知动作 id、未知事件类型、越界值与未知属性必须被拒绝：
  下行在 `parseState` 抛 `BridgeProtocolError`，上行信封由 schema 的
  `oneOf`/`additionalProperties: false` 拒绝。强制点：
  `src/bridge/schema.test.ts`（ajv 合法/非法夹具，含未知事件类型用例）与
  `src/bridge/client.test.ts`（纯函数拒绝用例）。
- 动作 id 语义与 Go `internal/client` 的 `UIAction*` 字符串常量、
  `cmd/mornlea/app` 的 `menuAction*` 别名逐值互钉，映射写在 schema 的
  `menuAction` 描述里；任何一侧不得单方面改动映射。
- `src/` 内禁止出现 `localStorage`/`sessionStorage`/`indexedDB` 与
  `fetch`/`XMLHttpRequest`/`WebSocket`：前端不持久化任何配置、零网络
  （资产全部经 `mornlea://` 内嵌供给）。强制点：评审 grep 兜底
  （`grep -rnE 'localStorage|fetch\(' src/` 应为空）。

## 设计令牌与样式（`src/tokens.css`、`src/ui/ui.css`、`src/ui/pixel.tsx`）

- 颜色、字号、圆角、几何与动效时长只允许以 `src/tokens.css` 的 CSS 自定义
  属性定义；`src/ui/ui.css` 消费令牌，不得出现裸色值、裸字号、裸时长。
  换算口径与取值来源见 `src/tokens.css` 头注（HUD 族取自
  `internal/render/hud/style.go`；egui 退役后，原 egui 皮肤族的次级取值
  以 `src/tokens.css` 为唯一权威）。强制点：评审兜底（无自动 lint）。
- 琥珀是唯一强调色相，只用于选中、进度、焦点/编辑态；错误行专用危险红。
  `prefers-reduced-motion` 下动效时长令牌归零，组件样式不得绕开令牌另设
  transition。
- 四面板按钮与表单控件必须经 `src/ui/pixel.tsx` 桥接层
  （`PixelButton`/`PixelInput`/`PixelCard`/`PixelDropdown`）或按
  `tokens.css` 令牌重绘的自绘控件呈现（音量滑块为后者的既定先例）；
  面板不得直接 import `pixel-retroui`，组件内不得裸写颜色。组件主题只经
  `tokens.css` 的「像素组件换肤段」变量映射供给（retroui 组件类按
  `var(--xx-custom-*, var(--bg-button, #亮色兜底))` 回退链取色）。该段
  选择器双写 `:root:root` 是有意为之：`ui.css` 顶部 @import 的上游产物
  自带 `:root` 亮色兜底且打包顺序在后，单写 `:root` 会被其覆盖。强制点：
  评审兜底（无自动 lint）。
- retroui 组件样式表 `pixel-retroui/dist/index.css` 是预编译 CSS Modules，
  由 `src/ui/ui.css` 顶部 `@import` 显式引入（`@import` 必须先于其他规则；
  不引入则 retroui 组件无样式）。升级 `pixel-retroui` 时须核对其产物
  `:root` 是否新增兜底变量，新增即补进换肤段映射，避免亮色兜底穿透。
  强制点：评审兜底 + dist 字节门禁。
- retroui 上游的两条已核实约束，触碰组件层时必须绕开：
  `DropdownMenu` 是无键盘语义的弹出菜单件（选项为普通 div、不参与 label
  关联、选中后不收起），不得用于表单语义控件——窗口预设等选择场景用
  `PixelButton` + `aria-pressed` 保持按钮组语义；`Input` 包装层
  `.pixelContainer` 硬编码 `font-size: 16px`，会经内部 input 的
  `font: inherit` 穿透，包装层 class 必须钉回字号令牌（先例：
  `.debug-edit-input` 的 `var(--font-message)`）。
- Tailwind `content` 扫描 `node_modules/pixel-retroui/dist/**`，retroui
  产物引用的工具类与其自带样式表会把 `.hidden`/`.shadow`/`.container`
  等通用名带进全局命名空间：前端类名保持既有 `menu-*`/`settings-*`/
  `pause-*`/`debug-*`/`pixel-*` 前缀隔离，不得裸用通用工具类名。

## 固定文案（`src/ui/copy.ts`）

- 设置页控件、暂停层标题/按钮/注明行的文案由 `src/ui/copy.ts` 单源固定
  （egui 退役后原 Rust 绘制常量已删除，本文件是这批文案的唯一权威）；
  主菜单标题、版本行、错误行与按钮表由 Go 下行驱动，前端不得内置。
  强制点：`src/ui/App.test.tsx` 的呈现断言 + 评审。

## 构建链与 dist 入库（`package.json`、`pnpm-workspace.yaml`、`vite.config.ts`、`dist/`）

- 包管理 pnpm：版本由 `package.json` 的 `packageManager` 字段经 corepack 钉版，
  `corepack pnpm install --frozen-lockfile` 是唯一安装姿势（锁文件
  `pnpm-lock.yaml` 提交入库）；不使用 npm/yarn 命令。
- 不执行依赖的构建脚本：`pnpm-workspace.yaml` 显式声明
  `allowBuilds: esbuild: false`（esbuild 平台二进制经 optionalDependencies
  供给）；新增依赖不得引入 postinstall 脚本。
- `vite.config.ts` 固定产物文件名（无内容 hash）保证两次构建逐字节一致；
  `dist/` 提交入库供 Rust 内嵌，禁止手工编辑 `dist/` 内文件。强制点：
  `make frontend-check` 末行的 `git diff --exit-code -- …/dist`（dist 入库后
  任何漂移即红）与两次连续构建实测（2026-08 实测一致）。
- 二进制 Web 资产仅限 `src/ui/fonts/` 的字体白名单：缝合像素字体
  （Fusion Pixel，OFL-1.1，许可文本与字体同目录入库），经 `@font-face`
  进产物、由 Rust 侧 `webview.rs` 资产表内嵌供给，除此之外不得新增任何
  二进制资产；零网络纪律不变（资产全部经 `mornlea://` 内嵌供给）。

## 测试组织（`src/bridge/schema.test.ts`、`src/bridge/client.test.ts`、`src/ui/App.test.tsx`）

- `schema.test.ts` 是协议钉值中心：合法/非法夹具共用同一份 schema.json，
  非法用例必须包含未知事件类型、未知动作与越界值。
- `client.test.ts` 钉纯函数行为（`parseState`/`createEnvelope`/订阅重放/
  sink 封装）；`App.test.tsx` 钉四屏相位切换与上行事件（menu/starting/
  settings/paused/game 与 `debug.visible` 叠加），不接真 WKWebView，事件由
  注入的 sink/spy 捕获。
- 新增行为先写失败断言再实现（vitest + @testing-library/react）；组件新增
  交互时同步补上行事件断言。

## UI 部件视觉基线（`visual/`、`visual-dist/`、`build/visual-ui/`）

- 管线构成：`visual/fixtures.tsx` 注册表（每个 UI 部件一个命名 fixture，经
  `?fixture=<name>` 选择渲染；四整屏直接以 fixture props 渲染生产面板组件，
  上行事件接空收集器，不挂 App/桥桩）+ `visual/visual.vite.config.ts` 独立
  构建到 `frontend/visual-dist/`（gitignored，绝不写 `dist/`、不碰其字节门禁；
  tailwind/postcss 与生产同配置，字体随 ui.css 进 harness 产物）+
  `visual/visual.mjs` 自包含脚本：自起 127.0.0.1 随机端口静态服务、逐 fixture
  调 Chrome headless 截图（1280x720、1x、sRGB）、pngjs 双阈值比对。
- 比对口径与世界 golden 管线对齐（`cmd/mornlea/capture/visual_compare.go`）：
  任一像素任一通道差上限 2，且差异像素（任一通道差 ≥ 1）占比上限 0.0001，
  超限即判漂移；漂移时把红标差异图写入 `build/visual-ui/<fixture>-diff.png`
  并以非零退出指认部件。
- 两个入口：`corepack pnpm visual-check` / `make frontend-visual-check`
  （比对；缺基线只报错列出、绝不自动创建）；`corepack pnpm visual-update` /
  `make frontend-visual-update`（截图覆盖 `visual/golden/*.png`）。实测 Chrome
  151（macOS）截完图不退出，脚本按「截图文件连续多次轮询尺寸稳定 → 结束
  进程组」处理，截图完整性另经 pngjs 解码校验。
- 本机开发工具：不进 CI 门禁、零网络（只访问本机临时静态服务）；Chrome 路径
  解析为 env `CHROME_BIN` > macOS 默认安装路径，缺失即报中文错误。
- 基线更新纪律：`visual/golden/*.png` 只允许在人工目检确认呈现正确后经显式
  update 入口覆盖；漂移先看差异图定位，再决定修代码还是更新基线。该基线是
  测试夹具二进制（与世界 golden PNG 同一入库先例），不属于 `dist/`「零二进制
  Web 资产」白名单约束范畴——那只约束生产 dist 资产。
- fixture 名称清单在 `visual/fixture-names.json` 单源（`fixtures.tsx` 与
  `visual.mjs` 共读，注册表在模块加载时与清单互钉）；新增部件先加清单与
  注册表，再跑 update 入库基线。harness 的 TS 自检用 `visual/tsconfig.json`
  （`tsc --noEmit -p visual/tsconfig.json`），不并入生产 `tsconfig.json`。

## Focused Verification

```bash
make frontend-check          # 冻结安装 + typecheck + vitest + 构建 + dist 一致性
make frontend-visual-check   # UI 部件视觉基线比对(本机工具,不进 CI)
```

定点命令（在 `engine/crates/mornlea_client/frontend/` 内执行）：

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build
```

无 Rust/Go 代码改动时不需要 `make rust`；本目录改动不进 Go 测试闭包，
`go test ./internal/archcheck -count=1` 仅在同时触碰 Go 侧时需要。
