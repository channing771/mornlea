# Change: client-webui-pixel-style

## Why

WebView 四面板（主菜单、设置、暂停、调试面板）目前是各写各的 class + 系统字体，
视觉语言靠 `tokens.css` 令牌约束但没有统一的组件质感；用户要求以
[pixel-retroui](https://retroui.io)（npm `pixel-retroui`，BSD-3-Clause）为组件
底座建立统一像素风格。调研确认三点约束：retroui 依赖 Tailwind（前端尚无）；
其内置「Minecraft font」不采用（版权红旗且不覆盖中文）；UI 文案全中文，
像素风格要成立必须另配覆盖 CJK 的开源像素字体（用户已拍板采用缝合像素字体
Fusion Pixel，OFL-1.1）。

## What Changes

- 前端新增依赖：`pixel-retroui`（运行时）、`tailwindcss@^3.4` +
  `postcss` + `autoprefixer`（开发）；tailwind `content` 扫描 `src/**` 与
  `pixel-retroui` 产物，**关闭 preflight** 防全局 reset 冲掉既有 CSS；
  逐依赖核验零 postinstall 脚本（`allowBuilds` 纪律）。
- 引入缝合像素字体 woff2 单文件 + OFL-1.1 许可文本入库：`@font-face` 后
  `ui.css` 字体栈改为「像素字体优先、系统 CJK 兜底」（经 @font-face）；`webview.rs`
  内嵌资产表扩展字体条目；`index.html`/`ui.css`/`frontend/AGENTS.md`
  的「零二进制资产」表述更新为「仅限 OFL 字体」白名单口径。
- 新建 `src/ui/pixel.tsx` 主题桥接层（PixelButton/PixelCard/PixelInput/
  PixelDropdown）：颜色与几何全部经 `tokens.css` 令牌传给 retroui，
  琥珀唯一强调色、危险红仅错误、`prefers-reduced-motion` 令牌归零语义保持。
- 四面板改用像素组件呈现：主菜单按钮列、设置表单（Input/Dropdown）、
  暂停层、调试面板行内编辑；音量滑块保留自绘控件按像素令牌重绘（既有规格
  钉死「以滑块呈现」而 retroui 无滑块组件）。文案、上行事件、焦点与键盘
  语义零改动。
- `frontend/AGENTS.md` 同步像素组件与字体资产的样式纪律。
- 新增 UI 部件视觉基线管线：每个 UI 部件（四整屏 + 各控件态）一张入库
  基线 PNG，无头浏览器截取、双阈值比对（口径同世界 golden 管线），
  check/update 两个 Make 入口；本机开发工具，不进 CI、不触 dist。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `webview-menu-ui`: 新增「面板统一像素组件风格」与「UI 字体资产」两条
  Requirement——四面板呈现 MUST 走统一像素组件/令牌，行为契约零改动；
  字体作为唯一白名单二进制资产经 `mornlea://` 内嵌供给。

## Impact

- 受影响文件：`frontend/{package.json,pnpm-lock.yaml,vite.config.ts,
  tailwind.config.*,postcss.config.*,src/tokens.css,src/ui/*}`、字体与
  OFL 许可文本、重建的 `dist/`、
  `engine/crates/mornlea_client/src/webview.rs`（资产表 +1 条目）、
  `frontend/AGENTS.md`、`openspec/changes/client-webui-pixel-style/*`。
- 兼容性：不触碰桥 `schema.json`/`client.ts`/文案/事件语义，Go 与 Rust
  行为零改动（`webview.rs` 仅追加静态资产条目，无 ABI 变化）；协议 v32、
  各 schema、engine/client ABI 全部零触碰。
- 性能：dist 体积因 Tailwind 工具类与字体文件增长（字体 MB 级，数值入
  ledger）；WebView 无网络加载，字体经内嵌 scheme 供给，无额外运行时成本。
- golden：菜单 chrome 像素本就不参与无头 golden 比对（`webview-menu-ui`
  既有条款），零影响。
- 并发/存档：不适用。
