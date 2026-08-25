## 1. 配置契约

- [ ] 1.1 在 `internal/config` 以测试先行方式加入固定窗口尺寸预设与 `texturePackPath` 1,024-byte 上限：覆盖缺省值、三个合法值、非法值/类型/null、1,024/1,025-byte 边界、保存再加载及 `Fields()` 不泄露新增字段；运行 `go test ./internal/config -race -count=1`。

## 2. client ABI v9 与结构化事件

- [ ] 2.1 在 `internal/client` 与 `engine/crates/mornlea_client` 以测试先行方式实现设置页 layout v2、结构化 uplink 批次、64 条有界队列、整批容量门禁及 client ABI v9；覆盖非法布局、未知事件、非法 UTF-8、NaN/越界音量、非法窗口枚举、尾随字节、同帧 change-before-save 顺序与容量不足不消费；运行 `make rust`、`go test ./internal/client -race -count=1` 和相应 Rust 单元测试。

## 3. Rust egui 设置页呈现

- [ ] 3.1 在 Rust `mornlea_client` 的 egui 层以 headless 测试先行方式实现音量、材质包路径、窗口预设、保存/取消/返回、dirty 提示、状态/错误文案及 Escape 行为；验证 640×360 下可访问且无重叠、路径输入有界、同帧事件顺序稳定、未引入 `egui-winit`；运行 `make rust-check`。

## 4. Go 设置事务与运行时接线

- [ ] 4.1 在 `cmd/mornlea` 以测试先行方式实现 Go 所有的 committed/draft 状态机、最新配置 patch-save、材质候选预验证、错误保留、成功后音频替换与窗口 resize，并把设置入口接入普通本地主菜单；覆盖取消、clean/dirty 返回、保存顺序、失败原子性、相对路径、现存额外配置字段保留、`-connect`/benchmark/capture 隔离及启动窗口预设；运行 `go test ./cmd/mornlea -race -count=1`。

## 5. 视觉基线与长期文档

- [ ] 5.1 新增 `settings-menu` capture 并启用 `main-menu` 的设置按钮，保持 `far-horizon` 倒数第二、`water-underwater` 最后；更新 `main-menu.png`、新增 `settings-menu.png`，证明其余既有 golden 逐字节不变，人工检查两张图；运行 `make visual-update`、`make visual-check` 和相关 capture 测试。
- [ ] 5.2 更新 README、材质包/进度说明及 `AGENTS.md`/`CLAUDE.md` 的当前能力与 client ABI v9 基线，保持两份基线文档逐字节相同；运行 `cmp -s AGENTS.md CLAUDE.md` 与 `go test ./internal/archcheck -count=1`。

## 6. 整分支收尾

- [ ] 6.1 对照 proposal、delta specs、design 与 ledger 完成整分支独立终审，修复所有阻断项，并执行 `gofmt -l .`、`go vet ./...`、`make rust-check`、`go test ./... -race`、`make visual-check` 与 `openspec validate --all --strict --no-interactive`。

