# Tasks: dev-capture

## 1. OpenSpec change 产物

- [x] 1.1 创建 change 五产物（proposal/design/tasks/ledger/delta spec）；
      验证：`openspec validate dev-capture --strict --no-interactive` 与
      `openspec validate --all --strict --no-interactive` 全绿。

## 2. Rust 捕获原语 + client ABI v13 + Go 桥接

- [x] 2.1 `mornlea_client` 新增捕获模块（`CGWindowListCreateImage` →
      `CGBitmapContext` → BGRA8 拷贝；校验顺序镜像 `render_readback`；两段式
      溢出回填 `out_required`；「不可用」独立状态），`ffi.rs` 导出
      `mornlea_client_window_capture`，header 升
      `MORNLEA_CLIENT_ABI_VERSION 13u`；`internal/client` 增 `Window.Capture`
      绑定（两段式重试、不可用→类型化错误、其余非 OK 稳定中文 panic），
      `window_test.go` 钉位 12→13，根 `AGENTS.md` 基线行 v12→v13；
      Files：`engine/include/mornlea_client.h`、
      `engine/crates/mornlea_client/src/{capture.rs,window.rs,ffi.rs,lib.rs}`、
      `internal/client/{window.go,window_test.go}`、`AGENTS.md`；
      验证：`make rust && make rust-check`、
      `go test ./internal/client -race -count=1`、`go build ./...`。

## 3. app 帧循环捕获泵

- [ ] 3.1 `cmd/mornlea/app` 新增 `dev_capture.go`（`CaptureRequest` /
      `CaptureOutcome` / `CaptureCoordinator` 接口 + 可空注入点），泵测试先行
      （fake 协调器计数：空闲零捕获调用、待办时每帧至多一次、交付非阻塞），
      `interactive.go` 菜单与游戏两处循环接入；
      Files：`cmd/mornlea/app/{dev_capture.go,dev_capture_test.go,
      interactive.go,app_dependencies.go}`；
      验证：`go test ./cmd/mornlea/app -race -count=1`、`go build ./...`。

## 4. devcapture 服务包

- [ ] 4.1 新建 `cmd/mornlea/devcapture`：`Service`（实现
      `app.CaptureCoordinator`，单 outstanding 请求通道）、HTTP mux
      （`/status`、`/screenshot`、`/record`）、录制采样编排（单帧推进 +
      帧间隔等待 + 丢帧计数）、zip/manifest/GIF 组装、BGRA8→NRGBA 转换、
      端口发现文件写清、参数上限校验（`0 < seconds ≤ 20`、`0 < fps ≤ 12`、
      `seconds×fps ≤ 240`、越界 400）；handler 测试以 fake 协调器走
      `httptest` 全覆盖三端点与失败路径；`internal/archcheck` 登记
      `devcapture → app` 边；
      Files：`cmd/mornlea/devcapture/*`、
      `internal/archcheck/dependency_test.go`；
      验证：`go test ./cmd/mornlea/devcapture ./internal/archcheck -race
      -count=1`、`go build ./...`。

## 5. options/main 接线

- [ ] 5.1 `--dev-capture`（默认关）与 `--dev-capture-addr`（默认
      `127.0.0.1:17790`）接入 `cmd/mornlea/options.go`（互斥矩阵追加
      `--benchmark`/`--capture`，options 测试先行），`main.go` 启动服务、
      注册优雅关闭与端口文件清理，与 `--connect` 组合可用；
      Files：`cmd/mornlea/{options.go,options_test.go,main.go}`；
      验证：`go test ./cmd/mornlea -race -count=1`、`go build ./...`。

## 6. 文档同步

- [ ] 6.1 新建 `docs/notes/dev-capture.md`（用法、端点契约、录制上限、
      屏幕录制授权与失败语义、端口发现文件），`docs/README.md` 索引挂链，
      `docs/agents/README.md` 增加代理使用小节（仿 agent-board 小节），
      `cmd/mornlea/AGENTS.md` 模式表与子包导览更新，新建
      `cmd/mornlea/devcapture/AGENTS.md`；
      Files：`docs/notes/dev-capture.md`、`docs/README.md`、
      `docs/agents/README.md`、`cmd/mornlea/AGENTS.md`、
      `cmd/mornlea/devcapture/AGENTS.md`；
      验证：`go test ./internal/archcheck -count=1`（文档链接与注释门禁），
      抽查链接目标存在。

## 7. 收尾门禁

- [ ] 7.1 全量门禁与 ledger 终局：`gofmt`、`go vet ./...`、
      `go test ./... -race`、`make rust && make rust-check`、
      `go test ./internal/archcheck -count=1`、
      `openspec validate --all --strict --no-interactive`，输出摘要记入
      ledger；
      验证：以上命令全绿。
- [ ] 7.2 人工验收一次真实链路：`--dev-capture` 启动 → `curl /status`、
      `/screenshot`、`/record`，画面内容逐层核对（世界 + HUD + 菜单层），
      结论记入 ledger（不进自动测试）。
