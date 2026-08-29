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
- 动作 id 语义与 Go `cmd/mornlea/app` 的 `menuAction*` 常量、Rust 既有
  `UI_ACTION_*` 清单逐值互钉，映射写在 schema 的 `menuAction` 描述里；任何
  一侧不得单方面改动数字。
- `src/` 内禁止出现 `localStorage`/`sessionStorage`/`indexedDB` 与
  `fetch`/`XMLHttpRequest`/`WebSocket`：前端不持久化任何配置、零网络
  （资产全部经 `mornlea://` 内嵌供给）。强制点：评审 grep 兜底
  （`grep -rnE 'localStorage|fetch\(' src/` 应为空）。

## 设计令牌与样式（`src/tokens.css`、`src/ui/ui.css`）

- 颜色、字号、圆角、几何与动效时长只允许以 `src/tokens.css` 的 CSS 自定义
  属性定义；`src/ui/ui.css` 消费令牌，不得出现裸色值、裸字号、裸时长。
  换算口径与取值来源见 `src/tokens.css` 头注（与
  `internal/render/hud/style.go` 及 `engine/crates/mornlea_client/src/ui/style.rs`
  同族并排）。强制点：评审兜底（无自动 lint）。
- 琥珀是唯一强调色相，只用于选中、进度、焦点/编辑态；错误行专用危险红。
  `prefers-reduced-motion` 下动效时长令牌归零，组件样式不得绕开令牌另设
  transition。

## 固定文案（`src/ui/copy.ts`）

- 设置页控件、暂停层标题/按钮/注明行的文案与 Rust
  `engine/crates/mornlea_client/src/ui.rs` 的绘制常量逐字对齐；主菜单标题、
  版本行、错误行与按钮表由 Go 下行驱动，前端不得内置。改文案必须同步
  `ui.rs` 对应常量。强制点：`src/ui/App.test.tsx` 的呈现断言 + 评审。

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

## 测试组织（`src/bridge/schema.test.ts`、`src/bridge/client.test.ts`、`src/ui/App.test.tsx`）

- `schema.test.ts` 是协议钉值中心：合法/非法夹具共用同一份 schema.json，
  非法用例必须包含未知事件类型、未知动作与越界值。
- `client.test.ts` 钉纯函数行为（`parseState`/`createEnvelope`/订阅重放/
  sink 封装）；`App.test.tsx` 钉四屏相位切换与上行事件（menu/starting/
  settings/paused/game 与 `debug.visible` 叠加），不接真 WKWebView，事件由
  注入的 sink/spy 捕获。
- 新增行为先写失败断言再实现（vitest + @testing-library/react）；组件新增
  交互时同步补上行事件断言。

## Focused Verification

```bash
make frontend-check          # 冻结安装 + typecheck + vitest + 构建 + dist 一致性
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
