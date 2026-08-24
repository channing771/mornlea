# Task 4 报告 — Go：client ABI v8 绑定与菜单帧编码

## 状态
SUCCESS — 功能实现完成，Go 侧验证通过（archcheck 基线 v7→v8 除外，属 Task 7.0）。

## 实现清单

### internal/client/render.go
- 新增 `frameTagUI = 9`（TLV tag，注释说明与 Rust `FRAME_TAG_UI` 一致）。
- `RenderFrame` 新增 `UISegment []byte`。
- `hasPassSegments()` 计入 `len(frame.UISegment) > 0`。
- `EncodeRenderFrame` 在 water 段之后追加 `appendTLV(frameTagUI, frame.UISegment)`（空段由 appendTLV 自动缺席）。
- 新增 `type UIButton struct { ID uint32; Label string; Enabled bool }`。
- 新增 `type UIMenu struct { Visible bool; Title, Version, Error string; Buttons []UIButton }`。
- 新增 `EncodeUIMenu(menu UIMenu) []byte`：小端 layout v1（u32=1）、flags u32（bit0=visible）、u32 按钮数、每按钮 u32 id + u32 label_len + UTF-8 label + u32 enabled(0/1)、u32 title_len + title、u32 version_len + version、u32 error_len + error；越界 panic（编程错误）。
- 新增 UI 常量：`uiLayoutVersion=1`、`uiFlagVisible=1`、`maxUIButtons=8`、`maxUILabelBytes=64`、`maxUITitleBytes=128`、`maxUIVersionBytes=64`、`maxUIErrorBytes=256`、`maxUIEventsPerFrame=64`。
- 新增 `Renderer.UploadUIFont(font []byte)`：C 调用 `mornlea_client_render_upload_ui_font`，状态码非 OK 走 `r.check` panic。
- 新增 `Renderer.DrainUIEvents() []uint32`：C 调用 `mornlea_client_render_drain_ui_events`；返回数量 = 写入 u32 个数；缓冲容量 = 64 事件；每次调用排空（Rust 端语义）。
- cgo 序言对两个新出口补 `#cgo noescape/nocallback`（既有格式）。

### internal/client/window.go
- cgo 序言同样补两个新出口的 `#cgo noescape/nocallback` 声明（与 render.go 重复——Go cgo 容忍重复，测试与链接均通过）。

### internal/render/font_atlas.go
- 新增导出 `EmbeddedCJKFont() []byte`：返回 `go:embed` 的 Noto Sans CJK OTF 字节的**独立副本**（只读），供 client ABI 菜单字体上传，不改既有 glyph 生成路径。

### 测试
- `internal/client/ui_menu_test.go`（新文件）：`TestEncodeUIMenuCrossLanguageGolden`、`TestEncodeUIMenuFourButtonsWithErrorFieldPositions`、`TestEncodeUIMenuMinimal`、`TestEncodeUIMenuMaximalValid`、`TestEncodeUIMenuPanicsOutOfBounds`，及 helper `assertU32At`/`mustPanic`。
- `internal/client/render_test.go`：`TestHasPassSegmentsIncludesUISegment`、`TestEncodeRenderFrameUISegment`。
- `internal/client/window_test.go`：`TestClientABIVersionMatchesHeader` 期望 7→8（见「遗留担忧」）。
- `internal/render/font_atlas_test.go`：`TestEmbeddedCJKFontProvenance`（长度 + sha256 + 副本语义）。

## 字节布局（与 Rust decode_ui_frame 逐字节一致）
`EncodeUIMenu` 输出（小端）：
1. u32 layout = 1
2. u32 flags（bit0 = visible）
3. u32 button_count
4. 每按钮：u32 id + u32 label_len + UTF-8 label + u32 enabled(0/1)
5. u32 title_len + title 字节
6. u32 version_len + version 字节
7. u32 error_len + error 字节

上界（越界 panic）：按钮 ≤ 8、label ≤ 64 字节、title ≤ 128 字节、version ≤ 64 字节、error ≤ 256 字节；与 Rust `MAX_UI_*` 逐字一致。

## 跨语言交叉证据
Rust `four_button_frame()`（`ui.rs` 测试夹具）：
- 按钮 `[(1,"进入游戏",true),(2,"多人游戏",false),(3,"设置",false),(4,"退出游戏",true)]`
- title="Mornlea"、version="dev"、error=""

Go 测试 `TestEncodeUIMenuCrossLanguageGolden` 断言 `EncodeUIMenu` 对同一菜单产出与硬编码 golden hex 完全一致（124 字节）。我已用**独立的 Rust encode_frame 复刻**（JS 脚本）核对 golden hex 与之完全匹配；`TestEncodeUIMenuFourButtonsWithErrorFieldPositions` 再对每个 u32 字段偏移与值逐项断言（小端）。

golden hex（124 字节）：
\`010000000100000004000000010000000c000000e8bf9be585a5e6b8b8e6888f01000000020000000c000000e5a49ae4babae6b8b8e6888f000000000300000006000000e8aebee7bdae00000000040000000c000000e98080e587bae6b8b8e6888f01000000070000004d6f726e6c65610300000064657600000000\`

## 测试与命令输出

### go test ./internal/client ./internal/render -race -count=1
\`\`\`
ok  github.com/channing771/mornlea/internal/client  5.483s
ok  github.com/channing771/mornlea/internal/render  3.967s
\`\`\`
（注：默认 GOCACHE 位于 `~/Library/Caches/go-build`，在本会话沙箱下写入被拒；测试以 `GOCACHE=$PWD/.harness-gocache`（工作区临时目录）运行，临时目录已删除。）

### go vet ./internal/client ./internal/render
\`vet exit=0\`（无输出）

### gofmt -l internal/client internal/render
无输出（exit=0）

### go test ./internal/archcheck -count=1
- `TestCommentBacktickIdentifiersExist`：PASS（已处理 `render.go` 注释中 Rust 标识符 `decode_ui_frame` 的反引号包裹，避免被误判为 Go 标识符）。
- `TestBaselineVersionsMatchCode`：FAIL —— AGENTS.md/CLAUDE.md 仍写 client ABI v7、代码为 8。属 Task 7.0 基线同步，不在本任务范围。

## 提交 hash
\`0df4778915db2eae64f23511c3b24dbed932a3cc\`
（`feat: bind client ABI v8 ui font and event drain in Go`；7 files changed, 432 insertions(+), 5 deletions(-)，新建 internal/client/ui_menu_test.go）

## git status
\`\`\`
 M openspec/changes/add-egui-main-menu/ledger.md   <- 会话前既有改动，未触碰
?? docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md   <- 既有未跟踪文件，未触碰
\`\`\`
（我的 7 个文件已全部提交，无其他工作区残留。）

## SPEC 自评
- 字节布局与 Rust `decode_ui_frame` 逐字一致：cross-lock golden + 逐字段位置断言双重锁定。
- 越界 panic、`hasPassSegments` 计入、`frameTagUI=9` 追加在 water 之后——全部覆盖并有测试。
- `UploadUIFont`（非 OK panic 走 `r.check`）、`DrainUIEvents`（返回 u32 计数、排空、缓冲 64）符合 brief。
- `EmbeddedCJKFont` 长度与 sha256 匹配 provenance，返回只读副本。
- cgo 声明与头文件签名一致（render.go 与 window.go 两个序言都补）。
- 范围纪律：未触碰 cmd/mornlea；未改既有导出签名；未提交既有未跟踪文件（spec 文档）；只有 1 个追加 commit，无 amend。

## QUALITY 自评
- 注释中文、导出项有中文 GoDoc；标识符反引号合规（`TestCommentBacktickIdentifiersExist` PASS）。
- 错误处理：越界 panic 带上下文；`UploadUIFont` 走 `r.check`。
- 只读/不可变约定：`EmbeddedCJKFont` 返回副本，调用方无法改写共享嵌入字节。
- 无新依赖；未导入 GPU 绑定。

## 遗留担忧
1. **brief「最小 28 字节 header」与实际 24 字节不一致**：零按钮、空串字段的最小帧实际为 24 字节（layout+flags+button_count+三个长度字段共六个 u32），与 Rust `encode_frame`/`decode_ui_frame` 一致。brief 的「28」多算一个 u32；我按 Rust 端真值实现（24 字节），`TestEncodeUIMenuMinimal` 已锁定。
2. **`DrainUIEvents` 返回值是事件计数而非状态码**：无法区分「恰好写 1..7 个事件」与「句柄错误（状态码 WINDOW=3 等）」。绑定依赖渲染器句柄在合法程序中始终有效（与其余方法共用同一 `Renderer` 句柄），故不复用 `r.check`；若句柄失效会出现静默错误返回值。这是 Task 3 ABI 设计的固有取舍。
3. **archcheck 基线红**（`TestBaselineVersionsMatchCode`）：AGENTS.md/CLAUDE.md 的「client ABI v7」需在 Task 7.0 同步为 v8，且两份必须逐字节相同。
4. **越界修改了 `window_test.go`**：把 `TestClientABIVersionMatchesHeader` 期望 7→8。严格范围纪律是「只改 render.go/window.go/font_atlas.go」，但不更新它则 brief 自己要求的 `go test ./internal/client` 无法全绿（dylib 已是 v8）。这是必要的一行版本常量修正。
5. **`EmbeddedCJKFont` 每次调用分配 ~16 MiB 拷贝**：只读安全、仅启动时上传一次，可接受；若未来频繁调用再考虑复用缓冲。
6. **cgo noescape/nocallback 在两个序言重复声明**：Go cgo 容忍重复（测试与链接通过）；规范主位是 render.go（与既有「window.go 放链接标志、render.go 补 render 入口指令」模式相符），window.go 的重复是遵 brief「两个序言都要」。
