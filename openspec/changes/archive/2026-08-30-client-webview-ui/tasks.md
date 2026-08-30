# Tasks: 客户端菜单层迁移 WebView 与世界全景背景

> 每组任务由 fresh implementer 实现，接受独立 SPEC 与 QUALITY 双评审；裁决与证据记入 `ledger.md`。控制会话不直接实现。

## 1. 前端基建与脚手架

- [ ] 1.1 `engine/crates/mornlea_client/frontend/`：Vite + React + TS(strict) 脚手架、`src/tokens.css` 设计令牌、`src/bridge/schema.json` 与类型化 `bridge/client.ts` 骨架、四屏组件空壳。
- [ ] 1.2 构建链：pnpm 锁定（`packageManager` 字段 + corepack、`pnpm-lock.yaml` 提交）、`make frontend-check`（pnpm install --frozen-lockfile + typecheck + vitest + build + dist 一致性）、CI 步骤、dist 首次入库；`.gitignore` 只忽略 node_modules。
- [ ] 1.3 vitest 首批组件冒烟测试（App 渲染四相位切换）；`docs/agents-md-style.md` 口径的 `frontend/AGENTS.md` 局部指南。
- 验证：`make frontend-check` exit 0；CI 本地等价命令绿。

## 2. WKWebView 集成 + 桥（含 kill-criteria spike）

- [ ] 2.1 **首日 spike**：objc2-web-kit 透明 WKWebView 覆盖在 winit NSWindow 上 + `mornlea://` scheme handler + 一条桥消息往返；透明/焦点任一不可行即触发 D9 降级裁决并回报 ledger。
- [ ] 2.2 Rust 侧：WebView 生命周期（相位 hidden/显示、firstResponder 路由）、`push_ui_state` JSON 下行、`WKScriptMessageHandler` → 事件队列 → `drain_ui_events` JSON 信封；未知/越界事件拒绝。
- [ ] 2.3 client ABI v12：版本三处 + Go 绑定；`upload_ui_font` 与帧 tag 9 段退役；Go 侧 `push_ui_state` 组装（相位/菜单/设置草稿/调试行）与上行消费；协议 schema 三端钉值测试。
- [ ] 2.4 无 UI 状态零参与语义（隐藏时无 evaluateJavaScript、无消息处理）。
- 验证：`cargo test -p mornlea_client`、`go test ./internal/client ./internal/nativeabi ./cmd/mornlea/... -race -count=1`、`make frontend-check`。

## 3. 四屏组件与行为语义平移

- [ ] 3.1 主菜单（全景之上的标题/按钮列/版本行/错误行；「多人游戏」禁用；进入游戏装配流与重复点击忽略；装配失败错误行）。
- [ ] 3.2 设置页（三字段草稿/取消/原子保存/生效时机/材质下次启动提示/脏草稿阻止返回/错误行——语义逐条对齐 `settings-menu` 既有规格）。
- [ ] 3.3 暂停层（两按钮、远程注明行、Esc 关闭语义经桥重组 Esc 优先级栈：聊天→容器→调试面板→暂停）。
- [ ] 3.4 F3 调试面板（D7 语义全平移；编辑事件上行）。
- [ ] 3.5 每屏 vitest 断言 + Rust/Go 桥级测试覆盖动作映射（旧 `UI_ACTION_*` 语义一一对应）。
- 验证：`make frontend-check`、`cargo test -p mornlea_client`、`go test ./cmd/mornlea/... -race -count=1`（app 级行为测试全绿）。

## 4. egui 完全退役

- [ ] 4.1 删除 `egui`/`egui-wgpu` 依赖、`ui.rs`、egui RawInput 翻译、egui pass、Go 侧 layout v1–v4 编码与字体上传链；winit 事件直服游戏输入。
- [ ] 4.2 Rust/Go 测试清理与重写（桥侧测试接替）；`cargo` 依赖树确认无 egui 残留；依赖版本线 spec 语义由新 capability 接替。
- 验证：`cargo test -p mornlea_client`、`cargo tree -p mornlea_client | grep -c egui`（为 0）、`make dev-check`。

## 5. 世界全景背景 + golden 替换

- [ ] 5.1 `menu-vista` 渲染路径：固定种子 worldgen 直供区块 → mesher → 地形/天空/光照 pass；固定脚本环绕相机；仅主菜单/设置相位；不触存储/服务端。
- [ ] 5.2 capture 接线：`main-menu`/`settings-menu` 场景产出全景底图（无头无 webview，确定性像素 golden）；visual-verification delta 与场景测试同步。
- [ ] 5.3 `make visual-update` 重生成两图并人工复核；世界场景 22 张零变化断言。
- 验证：`make visual-check`、`go test ./cmd/mornlea/capture -race -count=1`。

## 6. 基线文档同步 + 全量门禁 + 终审

- [ ] 6.1 文档：`docs/architecture.md`（菜单层架构）、`docs/notes/progress.md` chronicle、`docs/notes/compatibility.md`（client ABI v12）、README 版本矩阵、`docs/development-process.md` 如涉 CI 变更；frontend 指南在 1.3 已建。
- [ ] 6.2 门禁：`make rust`、`make frontend-check`、`make dev-check`、`make test-race-changed`、`openspec validate --all --strict --no-interactive`、`go test ./internal/archcheck`、`make visual-check`；重型门禁前后重建 Rust。
- [ ] 6.3 整分支终审：对照 D10 护栏逐条核对后开 PR。
