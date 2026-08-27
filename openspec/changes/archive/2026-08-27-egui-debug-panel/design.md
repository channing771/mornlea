## Context

- 既有调试面板是 Go 程序化渲染（`internal/render/debug_panel.go` 构建 quad/glyph 序列，`cmd/mornlea/debug_panel.go` 提供面板帧输入与变更应用），经 frame stream 上传 GPU；D-01 已建立 egui 工具型 UI 通道（`FRAME_TAG_UI` → `decode_ui_frame` → egui 绘制，Go 侧 `cmd/mornlea/ui_*.go` 编码 layout v1/v2 段）。
- F3 触发与键盘边沿在 `cmd/mornlea/interactive.go`，编辑键（方向键/Enter/Esc）在 Rust `input.rs`已有但映射未用于面板。
- 既有约束：面板行 `maxPanelRows=64`、读数 `maxPanelReadoutRows=7`、每行 `maxPanelRunesPerSide=24`；载荷受 `MAX_UI_SEGMENT_BYTES=4096`；远程 `remote()` 只读；`-dev` 门控面板可用性。

## Goals / Non-Goals

**Goals**：调试面板完全迁入 egui，删除旧程序化绘制路径；保持既有行选中/值编辑/写回能力；无 wire 变更、不升 client ABI。

**Non-Goals**：不做任意值类型校验器扩展（沿用 `config.Fields()` 分类）；不做面板尺寸/位置拖拽；不做多面板体系；不做远程写回；不改变 F3 之外快捷键。

## Decisions

### 1. 完全迁移 vs 并存

**决定**：完全迁移——删掉 `internal/render/debug_panel.go` 的程序化绘制（保留 `PanelRow`/`PanelReadout` 类型与 `config.Fields()`/`rows()` 数据源）。egui 面板与 D-01 设置页同构。

**理由**：并存保留双渲染路径，维护两套输入捕获/字体/状态呈现的成本高；D-01 已布局通道，迁移是既有框架的直接延伸。

**被否方案**：egui 与旧面板并存——保留旧路径，但两种 UI 状态机并存、`-dev` 下切换不干净。

### 2. layout v3 段编码：定宽行记录

**决定**：layout v3 段 = 段头（layout=3 + flags[visible] + **结构化读数区** + mode 名 + rows 计数）+ 每行定宽记录（`label[24]` + `value[24]` + flags[readonly/selected/editable/editing] + 值编辑原文[编辑时]+编辑光标）。标签/值固定 24 字节、UTF-8 截断后零填充；段尾允许 ≤3 字节零填充（4 对齐）；段长度受 `MAX_UI_SEGMENT_BYTES=4096` 上界。字节级契约（小端，Task 1 解码器与 Task 2 编码器逐字节对应）：

```
u32 layout = 3
u32 flags                         bit0=visible；未知位拒绝
f64 frame_millis                  有限且 ≥0
f32 position x/y/z               均有限
f32 yaw, pitch（度）             均有限
u64 tick
u64 world_time
u32 loaded_chunks
string mode                       u32 长度 + UTF-8，≤64 字节、非空、单行
u32 row_count                     ≤64
每行记录:
  label[24] value[24]             零填充定宽，首个 NUL 后必须全零，整体合法 UTF-8
  u32 row flags                   bit0=readonly、bit1=selected、bit2=editable、
                                  bit3=editing；未知位拒绝；editing→editable、
                                  selected→!readonly 必须成立
  [编辑态] string edit_value      u32 长度 + UTF-8，≤64 字节、单行
             u32 edit_cursor      字节偏移，≤len 且落在字符边界
```

**读数区归属**：顶部只读读数（帧时、位置、朝向、tick、时刻、区块数、模式）是结构化段头字段，Rust 侧用固定标签呈现（「模式」行的值即段头 `mode`）；`rows` 只装参数行（分组段头行 + `config.Fields()` 行），行数上限 64 只约束参数行——这是对 spec「行数截断：config.Fields() 多于 64 行仅前 64 行可见」的落实。

**理由**：定宽记录与现有 v1/v2 布局风格一致，解码器（Rust `decode_ui_frame`）可在迭代器中直接按偏移解析；行级 TLV 更灵活但无既有先例、解码路径更复杂。读数区若放进 rows，Rust 无法确定哪些行是读数、哪些是参数，也就无法实现「顶部只读读数区 + 参数行列表」的两段式面板。

**被否方案**：行级 TLV——自描述但增加边界检查与解析路径，违背 v1/v2 既有编码惯例；读数区以只读行塞进 rows——行边界无法结构性区分。

### 3. 编辑态存储：Go 拥有本地草稿

**决定**：Go 侧仍然拥有面板状态（`panelState`），Rust 只呈现 layout v3 传来的每帧状态并回传动作。编辑中的文本留在 Rust 文本框；写入确认（Enter）时事件批带上新值字符串，Go `applyPanelChange` 校验并写回。Esc 取消时 Go 恢复原值。编辑会话期 `edit_value`/`edit_cursor` 是下行播种值（进入编辑的初始文本与光标），Rust 草稿以段内值初始化后不再被下行覆盖，直至 Go 翻转 `editing` 结束会话。

**理由**：面板值写回涉及 `config.Fields()` 的既有校验与本地 physics/sim tunables 更新——这些必须在 Go 侧（无权 Rust 侧），且避免 Rust 直接接触 Go 配置结构。D-01 设置页同构。

### 4. 键盘捕获：F3 边沿仍在 Go

**决定**：F3 边沿检测仍在 Go（`interactive.go` 的 `panelToggleWasDown`），面板可见时全捕获保持——面板打开期间游戏键一律不产生上行。方向键/Enter/Esc/文本输入由 Rust egui 处理并回传动作事件。

**理由**：F3 切换是 Go 侧 `window.KeyDown(client.KeyF3)` 边沿门控；egui 需要焦点才工作，Go 仍控制「面板是否可见且应捕获」总开关。

### 5. 不升 client ABI

**决定**：client ABI 维持 v9；在 ln v9 内新增 `UI_DEBUG_LAYOUT_VERSION=3` 常量与解码分支，事件批沿用 v9 结构化格式。调试动作是新事件类型 `UI_EVENT_KIND_DEBUG_ACTION=3`，payload = `u32 action` + `u32 value_len` + UTF-8 value（仅 EDIT_VALUE/CONFIRM 携带值，其余空）；`action` 取 `DEBUG_PANEL_ACTION_*`（SELECT_NEXT=1、SELECT_PREV=2、ENTER_EDIT=3、EDIT_VALUE=4、CONFIRM=5、CANCEL=6、CLOSE=7），未知 action 与超界/多行 value 由队列拒绝（Invalid）。固定生成顺序：选中移动 → 进入编辑 → 编辑值 → 确认 → 取消 → 关闭（同帧多键按此排列）。

**理由**：D-01 先在 v9 上加 layout v2 设置页未升 ABI；layout 是 UI 段内部字段，非 ABI 版本边界。升 ABI 则与其它功能行互斥且无必要。

### 6. 快捷键裁减与 F5 保存移除

**决定**：F3 之外的既有调试面板快捷键不逐一保留——旧自绘面板的方向键行选中、←/→ 数值步进、Shift/Alt 粗细调、F5 保存配置文件、F6 全量重置均随程序化路径删除；选中移动改由 egui 方向键处理，数值调整被 egui 文本框编辑 + Enter 确认 / Esc 取消取代（spec「行选中与值编辑」，编辑值写回仍是 design §3 的 Go 校验路径）；F5 保存删除——调试面板不再落盘配置，配置保存职责由设置页（D-01）承担。本决策对 proposal 非目标里「不改变 F3 之外的任何既有快捷键」的撤回：迁移到 egui 文本编辑后，旧步进/粗细调键的语义已被输入框 + Enter/Esc 覆盖，保留它们只会维持一套永不被新 UI 使用的键枚举。

**理由**：F5/F6/步进键的服务对象是旧自绘渲染器；egui TextEdit 提供全键盘文本编辑后，保留会形成互斥的两套数值编辑语义（步进 vs 文本输入），且 F5 保存与 D-01 设置页的保存状态机并存成双写入路径。

**被否方案**：保留旧 F5 保存——面板 F5 落盘与设置页保存形成双路径配置持久化，两套保存逻辑必须协调配置解析、写入与失败处理；面板配置会跨过"配置文件由设置页管理"的 D-01 边界。保留 ←/→ 步进与 Shift/Alt 粗细调——与 egui TextEdit 的文本编辑语义冲突，egui 获得键盘焦点后这两组分键的仲裁复杂且无收益。

## Risks / Trade-offs

- **[性能] egui 面板每帧绘制行数上限 64，若面板常开可能增加 GPU/UI 开销** → 面板默认隐藏，仅 F3 显示；段仅在面板可见时编码；`MAX_UI_SEGMENT_BYTES` 上限保证有界载荷；benchmark 路径不加 UI 段不运行 egui。
- **[兼容性] D-01 之后 ABI 事件批格式变化，旧 D-01 版本动态库不可混装** → 该 ABI 边界已存在于 D-01（v9 结构化事件），本次只是 v9 内扩展；三处一致性测试（Rust test + C header 常量 + Go `TestBaselineVersionsMatchCode`）保障正确加载。
- **[并发] 面板值写回改 physics/sim tunables 与 A 批次冲突** → 写回仅本地进程；A 批次在途独占文件集（`internal/sim` engine 文件）本行不触碰——只动 `config.Fields()` 兼容路径与 `cmd/mornlea/debug_panel.go`。
- **[拒绝路径] 非法段导致 `INVALID_ARGUMENT`** → 沿用 v2 的非法拒绝单测模式，加 v3 合法性单测（未知 layout、越界行/标签、尾随字节）。
- **[输入退降] 面板可见时若断线/失焦，编辑态悬挂** → 面板可见性变化（F3、remote 变化）时清空编辑态（Go 复位）。

## Migration Plan

1. 分支 `feat/D-03-debug-panel-egui`（已建，基准 `main` 干净）。
2. 实现顺序：Rust `ui.rs` v3 解码 + egui 控件 → Go 段编码 → 事件回传接线 → 删旧程序化路径。
3. 回滚：功能在功能分支合入前独立；合入后回滚需一个新的小 change，或 revert（`internal/render/debug_panel.go` 恢复）。
4. 无存档/协议/schema 迁移；`-dev` 门控与 F3 语义保持。

## Open Questions

- 无。
