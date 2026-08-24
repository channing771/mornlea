## Why

当前图形客户端在构造时立即启动本地权威世界并直接进入游戏：窗口出现的世界已经装配完毕，玩家已经站在出生点。没有「启动 → 菜单 → 进入游戏」的入口分层，也没有可复用的窗口型 UI 基础设施——每加一个窗口型界面都要在 Go CPU 半部手写布局、命中测试、焦点与文本输入。

仓库已于 2026-08-23 在 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md` 裁决 egui 为工具型 UI（进入游戏后的菜单、设置等窗口型界面；常驻 HUD 与容器格子继续走既有程序化 quad 管线）的唯一技术栈，并给出集成约束（wgpu 29 线、client ABI v7→v8、capture/benchmark 不受影响、风格边界、测试纪律）。本变更交付该选择的第一个竖切：egui 集成基础 + 参考经典标题画面的主菜单（大标题、纵排按钮列、版本行），并把「进入游戏」做成真正的门禁——世界装配从启动时延迟到用户点击之后。

## What Changes

- `mornlea_client`（Rust）引入 `egui` 0.35.x 与 `egui-wgpu` 0.35.x（crates.io 核实：egui-wgpu 0.36.1 声明 wgpu ^30，仅 0.35.x 与仓库 wgpu 29 线兼容；egui 0.35 rust-version 1.92 ≤ 固定工具链 1.97.1），**不引入 `egui-winit`**：菜单输入由现有 winit 事件翻译手工构造 `egui::RawInput`。
- egui 以一条附加 wgpu render pass 挂进既有渲染器（窗口模式与离屏模式同构），排在既有 HUD/debug pass 之后；帧不携带 UI 段时 egui 完全零参与（不运行上下文、不提交 pass、丢弃积压菜单事件）。
- 菜单语义状态留在 Go：`cmd/mornlea` 持有菜单相位（菜单/装配中/游戏）与菜单内容（标题、版本行、按钮表、禁用态、错误行），经 client ABI 下行；Rust 侧不产生游戏语义。
- client ABI v7 → v8：新增 `render_upload_ui_font`（一次性上传菜单字体）与 `render_drain_ui_events`（回读菜单点击事件 id）两个出口；帧输入（`render_frame` TLV）新增 UI 段（tag 9）。既有入口签名不变，ABI 版本不匹配照旧全入口拒绝。
- 菜单字体复用仓库既有、已有 provenance 的 `internal/render/assets/NotoSansCJKsc-Regular.otf`（OFL-1.1，notofonts/noto-cjk f8d1575），关闭 egui `default_fonts` feature；不新增任何二进制资产。
- 交互式客户端（非 benchmark/capture、未指定 `-connect`）启动后停留在主菜单：打开存档、启动本地权威服务端、登录、远环播种全部延迟到点击「进入游戏」之后；`-connect` 与 benchmark/capture 路径行为不变（跳过菜单）。
- 主菜单参考经典标题画面：大标题「Mornlea」、中心纵排按钮列（进入游戏 / 多人游戏(禁用) / 设置(禁用) / 退出游戏）、底部版本行；菜单期间光标不捕获、游戏输入（移动/采掘/面板键等）不生效；「进入游戏」成功后捕获光标进入游戏，「退出游戏」正常关闭客户端；装配失败在菜单内显示错误行且进程不崩溃。
- capture 新增无窗口场景 `main-menu`（标题+按钮+版本行离屏渲染回读比对），插在 `far-horizon` 之前；`far-horizon` 仍倒数第二、`water-underwater` 仍最后；其余既有场景 golden 逐字节不变。
- benchmark scenario 保持 v19 不变（无固定上传布局/图集变化；benchmark 帧不携带 UI 段，egui pass 零参与）。

非目标：不做世界选择列表、多人游戏服务器连接表单、设置项菜单、暂停菜单或游戏内 egui 面板（各为后续独立 change）；不迁移既有程序化 HUD、容器 overlay、debug 面板与聊天呈现；不增加主题/配置系统；不引入 Mojang 版权素材或任何未授权二进制资产；不改变协议、存档、engine ABI 与权威/预测边界。

## Capabilities

### New Capabilities

- `egui-tool-ui`: 规定 egui 集成的技术边界（wgpu 29 线、无 egui-winit、pass 排序、ABI v8、字体来源与上传、无 UI 时零工作）与主菜单的可观察行为（启动相位、延迟世界装配、菜单内容、按钮语义、输入分流、失败路径、退出）。

### Modified Capabilities

- `visual-verification`: 场景表新增 `main-menu` 无窗口场景及其排序约束，并锁定其余既有场景 golden 逐字节不变。

## Impact

- 受影响仓库面：`engine/crates/mornlea_client`（Cargo.toml、`src/ui.rs` 新模块、`input.rs`/`window.rs` 事件桥、`render/mod.rs` 与 `render/egui.rs` 新 pass、`ffi.rs`、`engine/include/mornlea_client.h`）、`internal/client`（render.go/window.go 绑定）、`cmd/mornlea`（启动路径、交互循环、capture 场景与 golden）、`internal/render`（暴露字体字节的只读访问器）。
- 无新 Go 包：菜单状态机留在 `cmd/mornlea`（package main），ABI 绑定扩展 `internal/client`，不触碰 `internal/archcheck` 依赖白名单。
- client ABI v7 → v8（`mornlea_client.h` 与 Rust `CLIENT_ABI_VERSION` 同步，abiversion 测试钉住）；engine ABI v6、协议 v26、区块 schema v9、世界 metadata v2、玩家 schema v7、`companions.ai` schema v4、配置格式与 benchmark scenario v19 全部不变；TCP 无认证/加密的既有边界不变。
- 自动测试不启动前台窗口：egui 的布局/事件以纯 Rust 单测覆盖（无 GPU 的 `egui::Context` 驱动），GPU 呈现经无窗口 capture 场景 + golden 验收；交互路径经依赖注入的 Go 单测。
- 归档时同步 `AGENTS.md`/`CLAUDE.md` 基线（client ABI v8、egui 工具型 UI 已交付）与 `docs/notes/progress.md`；本提案不修改 OpenSpec 主规格。
