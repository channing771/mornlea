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

## Task 1（egui 依赖 + 无 GPU UI 模型）

- 2026-08-23 控制会话：change 产物已提交（69534fa8）；Task 1 implementer 已派发（agent 1c74d3bc-9255-497e-b00e-f920046b85b4，brief: .superpowers/sdd/add-egui-main-menu/task-1-brief.md，report: task-1-report.md）；BASE = 69534fa8。
- Ruling（预检）：design.md 与选型文档的 egui 版本差异——选型文档假定 0.36.1，实测 crates.io egui-wgpu 0.36.1 = wgpu ^30、0.35.x = wgpu ^29；按「绝不单侧升级 wgpu」采用 0.35.0。若错：后续 egui 升级需要带 wgpu 30 迁移一起做。
- Ruling（预检）：egui default_fonts 关闭后单测无内嵌字体——先试空 FontDefinitions 布局；若 egui 无字体 panic，用 <=4KB 许可明确的小 TTF（include_bytes! 自 src/ui/testdata/）并注明来源。若错：测试字体依赖资产许可风险与体积增加。

## Task 1 执行与裁决

- 2026-08-23 implementer(1c74d3bc)完成：DONE_WITH_CONCERNS，候选 commit 9a8292f1（feat: add egui deps and headless menu ui model）。cargo test/clippy/fmt 全绿；cargo tree：wgpu 29.0.4、egui/egui-wgpu 0.35.0、无 egui-winit。实现：src/ui.rs（877 行 + 19 单测）、egui 依赖（default_fonts 关）、src/ui/testdata/demo.ttf（400B，取自 ttf-parser 0.25.1 测试夹具，MIT OR Apache-2.0，报告注明来源）。
- Ruling 1（load-bearing，下达 fix round 1）：ABI 布局 v1 必须编码逐按钮 enabled u32（0/1；否则 Err）——「禁用按钮不产生事件」是规范 MUST，wire 不携带禁用态则该 MUST 不可达成。design.md 已更新（每按钮 id+label_len+label+enabled；Go EncodeUIMenu 同步）。若错：wire 兼容面多一个字段（两端正开发期，无历史格式承诺，代价低）。
- Ruling 2（minor 记录，不修）：egui 0.35 按钮交互内边距使 8px 间隙点会命中相邻按钮；规范只约束「可见矩形内点击 → 恰好一次该按钮」与「同点最多命中一个」，间隙点击行为未规定。实现采用中心命中恰一次+远处不命中+互不重叠几何断言。若真需求变化：调 interact 矩形或间距。
- Ruling 3（minor 记录，接受）：demo.ttf 是 ttf-parser 测试夹具（MIT OR Apache-2.0），400 字节、仅 'A' 字形；真实渲染由 Task 6 capture golden 以 ABI 上传的 Noto CJK 验收。若错：无头单测无法测真实字体渲染（已规划到 capture 层）。
- API 校正（implementer 实测 0.35，控制会话已同步进 design.md）：Context::run_ui + ROOT 视口 native_pixels_per_point；Context::default()；egui-wgpu 的 wgpu/default 特性非法已去（wgpu 29 default 特性由直接依赖统一携带）；egui 点击需「Move→Press→Release」三帧序列。
- Task 1 fix round 1/5：进行中（resume 原 implementer，Ruling 1 的 enabled 位与 wire 路径测试）。
