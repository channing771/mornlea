## Why

当前调试面板是 Go 程序化绘制的屏幕空间叠加层（`internal/render/debug_panel.go` + `cmd/mornlea/debug_panel.go`），与 D-01 建立的 egui 工具型 UI 基础并存但不在同一通道：Go 侧手工拼接 quad/glyph 序列上传到 GPU 呈现。自 D-01 起菜单与设置页已迁入 egui，调试面板成为唯一仍走旧自绘路线的工具型 UI——双渲染路径让输入捕获、字体上传、状态呈现与事件回传的处理割裂。D-03 把调试面板迁入 egui，复用 D-01 布局通道与结构化事件批，删除旧自绘渲染路径，保留面板的全部既有能力（F3 开关、行选中、值编辑与写回）。

## What Changes

- 删除 Go 程序化调试面板渲染路径，新增 egui debug 面板 layout v3（`UI_DEBUG_LAYOUT_VERSION=3`）：Go 侧保留面板数据源（顶部只读读数、参数行 `PanelRow`），改经既有 `FRAME_TAG_UI` 段编码下行；Rust `mornlea_client` 新增 v3 解码分支与 egui 面板控件绘制。
- 调试面板交互完整迁移：F3 切换显示、面板可见时键盘全捕获、方向键移动选中行、Enter 进入编辑、数字/字符串编辑值、Enter 确认写回、Esc 取消编辑；编辑值写回仍只影响本地进程内的 config/physics/sim tunables（联机对远端只读，同 D-01 既有语义）。
- UI 上行在 D-01 结构化事件批上新增调试面板事件类型（`UI_ACTION_DEBUG_*`），Go 侧 `applyPanelChange` 消费并按既有 `config.Fields()` 语义写回。
- 面板行上限 `maxPanelRows=64`、读数 `maxPanelReadoutRows=7`、每行 `maxPanelRunesPerSide=24` 的既有约束保留在 layout v3 段格式中；载荷不超既有 `MAX_UI_SEGMENT_BYTES=4096`。
- 无 wire 变更：面板为客户端内呈现与本地校验，不产生新网络消息；`remote()` 时只读。
- 不升 client ABI：ln v9 内扩 layout 常量（D-01 先例：v9 上加 settings layout=2）。

非目标：不做调试面板的任意值类型校验逻辑扩展（沿用既有 `config.Fields()` 的只读/编辑分类）、不做面板尺寸/位置拖拽、不做多面板或工具窗体系、不做面板值的远程写回；F3 之外的既有调试面板快捷键以 egui 文本编辑/Enter/Esc 语义取代——F5 保存、F6 重置、←/→ 步进与 Shift/Alt 粗细调移出（设计决策 6，配置保存完全由 D-01 设置页承担）；不触碰线上 wire、世界/玩家/伙伴存档、engine ABI、benchmark workload 或并行在途 A-01/E-12 的独占文件集。

## Capabilities

### New Capabilities

- `debug-panel`: 定义调试面板的显示开关、只读读数、参数行选中/值编辑/写回的可观察行为，以及远端连接的只读约束。

### Modified Capabilities

- `egui-tool-ui`: UI 下行在 layout v1 主菜单、v2 设置页之外新增 layout v3 调试面板状态段；UI 上行结构化事件批扩展调试面板事件类型；「无 UI 帧时 egui 零参与」与「菜单字体只经 ABI 上传一次」继续对调试面板生效。

## Impact

- Go：`internal/render/debug_panel.go`（删除程序化绘制，保留 `PanelRow`/`PanelReadout` 与 `config.Fields()`/`rows()` 数据源）、`cmd/mornlea/debug_panel.go`（layout v3 段编码与 `applyPanelChange`）、`cmd/mornlea/interactive.go`（F3 开关与键盘边沿状态接 egui 事件）、`internal/client/window.go`（编辑键枚举与事件出队）。
- Rust：`engine/crates/mornlea_client/src/ui.rs`（`UI_DEBUG_LAYOUT_VERSION=3` 常量、v3 段解码、egui 面板控件、调试事件编码）、`ffi.rs`（若需新增出口）。
- 兼容性：client ABI 保持 v9；线上协议 v26、engine ABI v7、区块 schema v9、玩家 schema v7、世界 metadata v2、`companions.ai` schema v4 与 benchmark scenario v19 均不变；debug 面板可用性仍由 `-dev` 选项门控（只门控面板可用性，不门控配置文件）。
- 并发与性能：面板交互只在客户端呈现路径发生，不进入权威 tick、网络或服务器热路径；benchmark 路径继续不上传字体、不生成 UI 段、不运行 egui；面板行段编码受既有 `MAX_UI_SEGMENT_BYTES` 有界。
