# SDD ledger — change: openspec/changes/add-egui-main-menu（egui 主菜单）

计划：`proposal.md`（What/Why）、`specs/egui-tool-ui/spec.md`、`specs/visual-verification/spec.md`、`design.md`、`tasks.md`（7 个 Task）。
控制会话：本会话（DeepSeek Agent）。实现以 subagent-driven-development 执行：每 Task 独立 implementer → 独立 task reviewer（SPEC+QUALITY 双裁决）→ fix 循环（≤5 轮）→ 整分支终审。

版本裁决（写实现前）：选型文档假定 egui 0.36.1，crates.io 实测 egui-wgpu 0.36.1 依赖 wgpu ^30.0、0.35.x 依赖 wgpu ^29.0；集成约束「绝不单侧升级 wgpu」→ 采用 egui 0.35.0 + egui-wgpu 0.35.0（egui rust-version 1.92 ≤ 工具链 1.97.1）。代价：egui 0.36 的 API 差异留待后续升级（届时整体评估 wgpu 30 迁移）。

基线 commit（分支 add-egui-main-menu 起点）：056dec987be030da71a438b06e9c8b7e56a3af39

## 预检（pre-flight scan）

Task 间文件/接口冲突检查：
- Task 1（ui.rs）+ Task 2（window.rs 桥 + ui.rs raw_input 组装）：Task 1 定义 `UiEvent` 与 `raw_input()`，Task 2 消费之——先做 Task 1，Task 2 只补 winit→UiEvent 翻译与队列写入；`UiEvent` 形状在 design.md 固定，无歧义。
- Task 3（render/egui.rs + ffi + header）依赖 Task 1 的 `UiState`/`decode_ui_frame` 与 Task 2 的 `UI_EVENTS`；task 3 内 ffi 需要 `FrameInput.ui_segment`——与 Task 1 无冲突（不修改既有 frame 编码语义）。
- Task 4（Go 绑定）与 Task 1/3 的字节布局必须逐字段一致：golden hex 双向锁定（Go 断言 + Rust 夹具），消除序歧义。
- Task 5 依赖 Task 4 的 `UIMenu`/`EncodeUIMenu`/`DrainUIEvents`/`UploadUIFont`；Task 6 依赖 Task 5 的 `menuOverride` 决策（在 Task 5 实现，Task 6 只做 capture 接线）——Task 5 的 brief 已含该字段。
- 风险点：Task 5 改动 `newApplicationWithDependencies` 的启动语义，需保持非 StartAtMenu 路径行为逐字节不变（cmd/mornlea 现有启动测试为护栏）。
- 无 task 之间互相矛盾的文本；egui-winit 禁止、wgpu 29 不动两约束贯穿全部 Rust task。

## 执行进度

（任务完成后按 Task 追加：Task N: complete（commits x..y, review clean）/ fix round 记录 / parked & Ruling 记录。）
