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

## Task 1 完成

- Task 1: complete（commits 69534fa8..64ec093f — 9a8292f1 候选 + 64ec093f enabled 修复，review clean）。
- 评审（5919ff97）：SPEC ✅ / QUALITY Approved；无 Critical/Important；Minor 全部记入递延清单（终审复核）：
  - M1 标题用 FontId::proportional(32) 而非 RichText::heading——行为满足，仅措辞差异。
  - M2 错误行仅当 ≥1 按钮时绘制（主菜单恒有按钮，实际影响低）。
  - M3 install_font doc 幂等机制表述不准（set_fonts 整体替换表）。
  - M4 确定性测试只断言 shapes.len()（符合 brief 明确要求，弱于 design 措辞）。
  - M5 decode 不校验段内尾部多余字节（TLV 界定段长，风险低）。
  - M6 8px 间隙点子例未直接测（Ruling 2 已记录 egui 交互内边距行为）。
- ⚠️ 跨任务项（各落点在 Task 2/3/4/5/6，已纳入对应 brief）：Go EncodeUIMenu enabled、ABI v8 出口、Noto 上传一次、winit_to_ui_events、真实字形渲染（capture）、design.md GPU 半部与跨语言 golden。

## Task 2 执行与裁决

- 2026-08-23 implementer(34e37a4e)完成候选：1f2c38c2（feat: bridge winit events into egui raw input）。cargo test 90+160 全绿、clippy 0 警告、fmt 通过。实现：ui.rs 输入队列（VecDeque 1024 丢最旧，push/take/clear）+ key_from + winit_to_ui_events + window.rs 接线；既有游戏输入路径逐字节不变。
- 偏差记录（implementer 报告 §2）：(1) UiEvent::Key 无 repeat 字段、raw_input 固定 repeat:false——brief 要求按 winit repeat 填 egui Event::Key.repeat；(2) 未置快照 reserved overflow 位（tasks.md 提及、brief 未要求，且触碰 input.rs 超出 brief 范围）；(3) key_from 覆盖 KeyA..KeyZ（优于 brief 的 K..Z 措辞）；(4) winit KeyEvent 不可构造，键盘翻译拆私有辅助函数。
- 控制会话预裁决（等评审确认后执行 fix round 1）：偏差(1) 按 brief 是明确要求，虽然本竖切菜单无文本输入、影响为 minor，但修复成本低且为后续菜单（服务器地址输入、设置）打底——fix 将 UiEvent::Key 增加 repeat: bool，raw_input 填充 egui::Event::Key.repeat，并补测试。若错：enum 字段无历史承诺、成本为一处测试调整。
- 偏差(2)(3)(4) 记录为接受（(2) 超出 brief 范围且 tasks.md 措辞为「若改变既定布局则回退」，丢最旧已实现；(3) 更全集；(4) 实现选择）。

## Task 2 完成

- Task 2: complete（commits 64ec093f..1f2c38c2，review clean）。评审(da257db3)：SPEC ✅ / QUALITY Approved；无 Critical/Important。
- Minor 递延（终审复核）：
  - M1(UiEvent::Key 无 repeat；brief 要求、与 Task 1 固定枚举冲突)——**Ruling 4（修正早先预裁决）**：递延为 minor。理由：UiEvent 是 Rust 进程内枚举（非 wire），后续菜单加文本输入时补 repeat 字段成本为零；本竖切菜单无文本输入，run-time 影响近零。若错：未来加 repeat 需连带 raw_input 与测试，仍是一次小改动。
  - M2 push_ui_event while→if（风格）；M3 每事件空 Vec 分配（可忽略）；M4 Modifiers.command/mac_cmd 恒 false（信息级；菜单语义在 Go）；M5 key_from A..Z 超集（信息级）。
- ⚠️ W2（渲染侧 take_ui_events 的 drop 语义）→ Task 3 落点；W1/W3 已裁决接受/递延。

## Task 3 执行与裁决

- 2026-08-23 implementer(07d60249)完成候选：f3b93252（feat: egui wgpu pass and client ABI v8 exports，7 文件 +587/-20）。cargo test 160+95 全绿、clippy 0 警告、fmt 通过。新建 render/egui.rs（EguiPass），FrameInput.ui_segment + tag 9 校验、CLIENT_ABI_VERSION=8、两出口、header/lib.rs 同步。
- Ruling 5（load-bearing 提前裁决）：`TestBaselineVersionsMatchCode` 是机械门禁——client ABI 头文件已升 v8，AGENTS.md/CLAUDE.md 必须在本 change 内同步版本号为 v8（两份逐字节一致；能力描述按选型文档留到归档）。tasks.md Task 7 已增补 7.0。若错：门禁保持红、CI 无法合并。
- 范围外最小改动记录（同 crate、功能必需）：src/ui.rs + pub fn ctx()（tessellate 需要 egui::Context）；src/render/water_tests.rs 的 render-pass 计数守卫 5→6（egui pass 是新增合法 render pass）。
- 偏差记录：run_and_record/set_size/new 采用 brief 签名（ppp/尺寸存入 self.screen，语义等价）；raw_input 以 time:Option<f64>=None 调用；字体缺失 Invalid 在 debug pass 之后触发（不提交已录命令）。

## Task 3 完成

- Task 3: complete（commits 1f2c38c2..f3b93252，review clean）。评审(ba1fe12c)：SPEC ✅（1 Minor 偏差）/ QUALITY Approved；无 Critical/Important。
- Minor 递延（终审复核）：
  - _window 未改名 window（brief item 0；字段已被读取，_ 前缀误导）——Ruling 6：递延，可由终审 fix wave 一并处理（纯命名、零行为影响）。
  - callback_buffers 先于主 encoder 提交（无 callback 时无害）。
  - water_tests 断言消息措辞（「半透明」实为 egui screen-space pass）。
  - EguiPass font 字段冗余（双真相源+双份字节副本，≤2×32MiB）。
  - decode_ui_frame 每帧双调（<4KB，design 既定）。
  - 早退帧不 take 事件（正常帧满足契约，队列上限防积压）。
  - 范围微超（ui.rs/water_tests.rs 同 crate 必需最小改动）；report 漏列 M tasks.md（报告精度）。
- ⚠️ 评审环境无 cargo（未复跑 test/clippy——由 Task 7 终审复跑）；GPU 端到端 → Task 6；archcheck 红 → 本会话已同步 AGENTS/CLAUDE（ef691004，Ruling 5 落地），Task 4-6 验证恢复全绿。

## Task 4 执行与裁决

- 2026-08-23 implementer(7d34c0ae)完成候选：0df47789（feat: bind client ABI v8 ui font and event drain in Go，7 文件 +432/-5，另附报告提交 b442edb4/f328d803）。Go 验证全绿（client/render race ok、vet ok、gofmt 干净）；跨语言 golden（Rust four_button_frame 124 字节）双重锁定。
- Ruling 7（load-bearing，下达 fix round 1）：DrainUIEvents 首版签名的返回值即事件数，与状态码空间（WINDOW=3 等）冲突——「3 个事件」与「窗口句柄错误」无法区分。裁决：签名改为 `drain_ui_events(abi, handle, out, out_len, out_count) -> status`，计数经 out_count 输出，Go 绑定经 r.check 判定状态并读计数。若错：ABI 是本 change 新引入、无历史格式承诺，改动成本低（ffi.rs/header/render.go + 测试）。
- 其他记录：最小帧 24 字节（brief 28 之说以 Rust 真值为准并已锁定）；EmbeddedCJKFont 每次 16 MiB 副本（仅启动一次）；window_test.go 版本期望 7→8（必要一行）；cgo 序言重复声明（Go 容忍）。
- 基线文档二次修正：TestBaselineVersionsMatchCode 用 FindAllStringSubmatch（**全部**匹配必须等于代码值），AGENTS/CLAUDE 第 13 行的历史「client ABI v7」同样会让门禁红——已改为不含该数字的历史表述（「client ABI 其后经 egui 主菜单变更升到 v8」），两份逐字节一致（工作区未提交，随后续 docs commit）。

## Task 4 评审发现（Important）与 Ruling 8

- 评审(be3ae096)：SPEC ✅ / QUALITY Approved + 1 Important：**帧级 4 字节对齐缺口**——parse_frame 对所有 TLV 段强制 length%4==0，而 EncodeUIMenu 的 UTF-8 字段序列不保证 4 对齐（四按钮+错误行=142、单按钮=42 等），真实菜单内容会把帧拒成 INVALID_ARGUMENT → Go panic；cross-lock 只在段内容层、未在帧封装层锁定。
- Ruling 8（load-bearing，评审轮 R1 → Task 4 implementer）：**FRAME_TAG_UI 豁免 4 对齐**（TLV 长度以字段自身界定；其余 pass 段仍强制）。配套：Rust 单测（坏句柄 + 非 4 对齐 UI 段 → WINDOW 而非 INVALID_ARGUMENT，证明 parse 接受）+ Go 单测（真实菜单字节的 TLV 长度=实际载荷、不填充）；Task 5 brief 增补真实菜单跨语言回圆测试。否决的替代：EncodeUIMenu 内部补齐到 4 对齐（依赖 decode 的尾部宽容，把语义耦合到填充字节）、豁免改在 decode（治标不治本）。

## Task 4 完成

- Task 4: complete（commits f3b93252..f3c078c6，review clean）。评审(be3ae096)：SPEC ✅ / QUALITY Approved + 1 Important（Ruling 8 帧级 4 对齐缺口）；scoped re-review(d797042d)：ADDRESSED，无 open Critical/Important。
- Minor 递延（终审复核）：R1 新观察 x2（142B 夹具硬编码 1u32 常量建议用 UI_LAYOUT_VERSION/UI_FLAG_VISIBLE；42B 单按钮 case 未单独测试——tag 维度豁免覆盖充分）＋评审环境无 cargo 因素（Task 7 复跑）。
- Ruling 8 落点已写进 design.md（TLV 文本段豁免 4 对齐）、tasks.md（main-menu 夹具错误行「存档无法打开」= 非对齐回圆）与 Task 5/6 brief（真实菜单跨语言编码锁 + capture 错误行夹具）。
- 辅助记录：implementer 报告按仓库先例提交（.superpowers/sdd/add-egui-main-menu/task-4-report.md 已入仓）；Task 1-3 报告未入仓——终审收尾时按同一先例补交。review-gocache 目录（462MB，评审会话在仓根生成）收尾清理。

## Task 5 执行与裁决

- 2026-08-23 implementer(521fcf80)完成候选：d8a3f05a（feat: defer world boot behind main menu，只改 cmd/mornlea：app_menu.go/app_menu_test.go/app_startup.go/app.go/interactive.go/app_frame.go/main.go）。cmd/mornlea 全量 race 247.9s 通过（含 8 个新 menu 测试），archcheck 通过、vet 通过、gofmt 干净。
- **dylib 核实（纠正给评审的 ⚠️）**：engine/target/release/libmornlea_client.dylib 时间戳 22:13（Task 5 测试运行前），nm 确认含 upload_ui_font 符号（计数 2）——Task 5 的 go test 实际链接 v8 dylib，v8 绑定真实覆盖；同时解释了 capture golden 未变（旧画面无 UI 段）。
- Ruling 9（spec 措辞释读，接受实现）：「capture 路径仅在 main-menu 场景需要时上传字体并渲染菜单」读作「capture 上传字体的理由是 main-menu 场景需要它」；实现为 !Benchmark 时上传（交互+capture），egui 只在有 UI 段的帧运行。硬约束保持：benchmark 不上传/不渲染；非 main-menu 场景无 UI 段 → egui 零参与。若错：把上传挪到 main-menu 场景前置（Task 6 可做，成本小）。
- 其他记录：menuVersion (devel)→"dev"；startWorld 失败经 releaseWorldConnection 清理半装配（可重试）；菜单期 Escape 由 egui 抢占（满足输入不生效，不提供退出，设计未要求）。

## Task 5 完成

- Task 5: complete（commits f3c078c6..d8a3f05a，review clean）。评审(bd40c86e)：SPEC ✅ / QUALITY Approved；无 Critical/Important；Minor 记录：(devel)→dev（可辩护）、capture 字体上传释读（Ruling 9）、starting 守卫防御性、捕获幂等重复。
- ⚠️ 均已解决：dylib v8 已就绪（评审与控制器双向核实符号导出）；GPU 端到端 → Task 6；Rust 非对齐豁免 → Task 4 已测；全量 race → Task 7。
task 5 status recorded
