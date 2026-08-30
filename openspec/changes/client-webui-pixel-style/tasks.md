# Tasks: client-webui-pixel-style

## 1. OpenSpec change 产物

- [x] 1.1 创建 change 产物（proposal/design/tasks/ledger/delta spec）；
      验证：`openspec validate client-webui-pixel-style --strict
      --no-interactive` 与 `openspec validate --all --strict
      --no-interactive` 全绿。

## 2. pixel-retroui + Tailwind 构建链接入

- [x] 2.1 `pixel-retroui` 运行时依赖 + `tailwindcss@^3.4`/`postcss`/
      `autoprefixer` 开发依赖接入 `frontend`：tailwind.config（content 扫
      `src/**` 与 `node_modules/pixel-retroui/dist/**`，`corePlugins.preflight:
      false`）、postcss 配置、vite 联动；逐依赖核验零 postinstall 脚本；
      先写一条冒烟断言（Tailwind 工具类与 retroui 组件类进入产物 CSS）
      再接线；锁文件入库；
      Files：`frontend/{package.json,pnpm-lock.yaml,tailwind.config.*,
      postcss.config.*,src/ui/ui.css 或新增入口}`、`pnpm-workspace.yaml`
      （如需登记）；
      验证：`corepack pnpm install --frozen-lockfile`、
      `corepack pnpm typecheck && corepack pnpm test && corepack pnpm build`、
      `make frontend-check`（dist 字节一致）。

## 3. 缝合像素字体入库 + webview.rs 资产表扩展

- [x] 3.1 下载缝合像素字体（Fusion Pixel，OFL-1.1）woff2 单文件（版本与
      SHA 记 ledger）+ OFL 许可文本入库；`@font-face` + `tokens.css`
      字体栈改「像素字体优先、系统 CJK 兜底」（落地在 ui.css，见 ledger Task 3 偏差）；dist 新增字体资产 →
      `webview.rs` 资产表扩展条目（include_bytes + 表项）；`index.html`/
      `tokens.css`/`frontend/AGENTS.md` 的「零二进制资产」表述更新为
      「仅限 OFL 字体」白名单口径；
      Files：`frontend/src/`（字体与许可）、`frontend/src/ui/ui.css`、
      `frontend/index.html`、`frontend/dist/`（重建）、
      `engine/crates/mornlea_client/src/webview.rs`、
      `frontend/AGENTS.md`；
      验证：`make frontend-check`、`make rust && cargo test -p
      mornlea_client`、`go test ./internal/archcheck -count=1`。

## 4. pixel.tsx 主题桥接 + 四面板 retroui 化

- [x] 4.1 新建 `src/ui/pixel.tsx`（PixelButton/PixelCard/PixelInput/
      PixelDropdown 薄封装，颜色/几何全经 tokens.css 令牌）；四面板改造：
      主菜单按钮列、设置 Input/Dropdown、音量滑块自绘像素重绘（滑块形态
      不变）、暂停层、调试面板行内编辑；文案/事件/焦点语义零改动，
      `App.test.tsx` 行为断言保持绿，新增行为先写失败断言；
      Files：`frontend/src/ui/{pixel.tsx,ui.css,tokens.css,MainMenu.tsx,
      SettingsPanel.tsx,PauseMenu.tsx,DebugPanel.tsx,App.test.tsx}`、
      `dist/`（重建）；
      验证：`corepack pnpm typecheck && corepack pnpm test &&
      corepack pnpm build`、`make frontend-check`。

## 5. 视觉验收与文档同步

- [x] 5.1 `--dev-capture` 启动游戏：主菜单自动截图目检；设置/暂停/调试
      三屏人工导航逐屏截图核对（像素风格统一、中文渲染、无布局破损），
      结论与截图路径记 ledger；`frontend/AGENTS.md` 补像素组件桥接与
      字体资产纪律；
      Files：`frontend/AGENTS.md`、`build/devcapture-check/`（截图产物，
      gitignored）；
      验证：`make frontend-check`（文档改动后复查）、截图目检。

## 6. 收尾门禁

- [x] 6.1 全量门禁与 ledger 终局：`make rust && make rust-check`、
      `go test ./internal/archcheck -count=1`、`go test ./... -race`
      （前端改动不进 Go 闭包时以 archcheck 为准，race 按需）、
      `openspec validate --all --strict --no-interactive`，摘要入 ledger；
      验证：以上命令全绿。
