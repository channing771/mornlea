# Rust RenderWorld Cache Design

## Context

本设计落实 `proposal.md` 与 `rust-client-render-cutover` delta 的第一个、可独立
回退的缓存闭环。Go Mirror 仍是客户端逻辑状态的真相来源；Rust `RenderWorld` 只保存
从已验证 Go 状态派生的渲染缓存，随 renderer 创建和销毁。缓存尚不接管 mesh、light、
connectivity、visibility、GPU upload 或 draw。

受影响文件限定为未来实现时的
`engine/crates/mornlea_client/src/`、`engine/include/mornlea_client.h`、
`internal/client/` 与相应测试。`internal/nativeabi` 仍是唯一接触 engine ABI 的边界；
本 change 不改变 engine ABI v8。

feature 基线曾独立把 MRW1 export 分配给 client ABI v12；与此同时，当前 main 已把
client ABI v12 分配给进程内 WKWebView/UI cutover：退役 `render_upload_ui_font` 与 frame
TLV tag 9（UI layout v1–v4），新增 `ui_push_state`，并把 `render_drain_ui_events` 的载荷改为
版本化 JSON 信封。两套不同 export surface 不能共享同一 ABI 版本，因此集成必须以 main
v12 为前代基线，把“main UI surface + MRW1”统一分配为 v13。

## Goals / Non-Goals

**Goals:**

- 建立集成 client ABI v13 的同步身份，在 main v12 UI surface 上加入 MRW1 v1 更新入口，
  以及原子、可重建的 `RenderWorld` cache。
- 在 Rust 拷贝或规范化 Go 同步调用期间提供的 bytes；之后不保存 Go 指针、slice 或
  对象，且按 epoch、revision、tombstone 决定派生缓存状态。
- 用单元、FFI、Go bridge、fuzz 与 test-only driver 锁定 wire 校验、原子失败和离屏
  frame 字节不变。

**Non-Goals:**

- 不创建 mesh worker、goroutine、thread、GPU pool、GPU 写入或任何 worker/GPU 调度。
- 不接入 `cmd/mornlea/app` 实时消息路径，不替换 Go CPU mesh、connectivity、visibility、
  `RenderFrame.Visible`、逐 section upload 或 draw。
- 不移动、复制或重构任何 engine 的 fluid-aware input、light、quad 或 greedy 源码；
  不改变流体状态、传播、tick、协议或专属 mesh 语义。
- 不创建共享 voxel kernel，不改网络协议 v32、存档 schema、world metadata、benchmark
  scenario v20 或 engine ABI v8。

## Decisions

### D1: MRW1 是独立、一次性的受限小端输入

MRW1 不复用网络 wire packet；Go 只在 Mirror 成功更新后，从既有
`world.ContainerSnapshot` 生成 bytes。Rust 在更改任何 `RenderWorld` 状态前完整预检
batch，成功后才在自己的内存中复制或规范化。这样 ABI 调用返回后 Go 内存可以失效，
而失败 batch 不会留下部分缓存。

MRW1 batch header 为 24 字节小端布局：

```text
0..4   magic = "MRW1"
4..6   layout_version = 1 (u16)
6..8   reserved = 0 (u16)
8..16  epoch (u64, non-zero)
16..20 record_count (u32, 1..=4096)
20..24 reserved = 0 (u32)
```

每条 record 是 32 字节头加 payload：

```text
0      tag (u8)
1      storage_kind (u8)
2      bits (u8)
3      reserved = 0 (u8)
4..8   dimension (i32)
8..12  section_x (i32)
12..16 section_y (i32)
16..20 section_z (i32)
20..28 revision (u64)
28..32 payload_len (u32)
32..   payload
```

tag 的固定含义为：1=section upsert、2=column upsert、3=section tombstone、
4=column tombstone、5=world reset。batch 总长度最多 4 MiB，记录总数最多 4096；
超过任一上限返回 client `INVALID_ARGUMENT`。

section upsert payload 是 8 字节 meta 后接 palette 与 packed words：

```text
0..2   single block id (u16)
2..4   palette_count (u16)
4..6   packed_word_count (u16)
6..8   reserved = 0 (u16)
8..    palette_count × u16 LE
...    packed_word_count × u64 LE
```

`storage_kind` 与 `world.ContainerSnapshot` 一一对应：0=single、1=indexed、
2=direct。single 要求 bits/palette/packed 均为零；indexed 只允许 bits 4 或 8，并校验
所有 packed slot 小于 palette_count；bits 为 4 时 packed_word_count 必须恰为 256，bits
为 8 时必须恰为 512。direct 要求 bits 15、palette_count 为零、packed_word_count 为
1024，且每个 word 的高四位为零。section payload 解析后必须恰好耗尽 payload_len，任何
尾随 bytes 都使整个 batch 失败。column upsert 的 payload 恰为 256 个 i16 LE height；
其 section_y、storage_kind、bits 均为零。tombstone 的 payload 长度为零。world reset 的
record 仅允许作为 batch 第一条，所有坐标、revision、storage 元数据和 payload 均为零。

section record 的 X/Z/dimension 使用完整 signed i32 域，section Y 必须在
`0..core.SectionsPerChunk`；column record 的 section Y 必须为 0。所有长度计算使用
checked arithmetic。FFI 在解引用 input 前只按固定顺序检查：ABI version、非零且不超过
4 MiB 的 length、non-null `u8` pointer、address range 不溢出、existing renderer handle；
通过后才把输入作为 bytes 校验 MRW1 layout/capacity。`u8` input 没有额外 alignment 要求；
该入口是 input-only，因此没有 output capacity 或 overlap 检查。

否决“将网络包原样交给 Rust”：它会把协议解析责任越过 Go 的既有边界，也会使 MRW1
与网络版本演进耦合。

### D2: reset 是 epoch 建立和清空的唯一入口

首次写入和重连恢复必须先发送 world reset，epoch 从 1 开始单调递增。非 reset record
的 epoch 必须等于当前 `RenderWorld` epoch；同一 key 只有 revision 更大才替换，等于
或更小均为幂等忽略。tombstone 保留 revision，因此只有 revision 更大的 upsert 可以
恢复该 key。world reset 必须为首 record，且合法 reset 原子清空旧 section、column 和
tombstone，再应用同 batch 的新 epoch records。

否决“以隐式空 cache 接受第一条普通 update”：它无法辨别首次连接、重连和延迟旧更新，
会削弱恢复与原子性的测试边界。

### D3: RenderWorld 是 renderer 独占的派生所有权

`RenderWorld` 由 `mornlea_client` renderer 持有并在其销毁时一并丢弃。它以 section 或
column key 保存已验证的紧凑 palette/bitpack、height、epoch、revision 与 tombstone；
它不是 Go Mirror 的查询来源，也不持有权威状态。所有可变 cache 状态由已有渲染线程
串行访问；此阶段没有 worker，因此没有跨线程借用、锁、队列或 GPU 资源所有权变化。

否决“让 Go 长期持有 Rust cache 的借用或每格回调查询”：两者都会破坏 FFI 生命周期与
语言职责边界，且会在后续数据平面中重新引入高频跨语言访问。

### D4: 以 main v12 为前代，把统一 surface 分配为 ABI v13

冲突解决必须以 main 的 v12 header、Rust exports 与 Go bridge 为基线：保留
`mornlea_client_ui_push_state`、版本化 JSON `mornlea_client_render_drain_ui_events` 及其
WKWebView 路径；保持 `mornlea_client_render_upload_ui_font`、frame TLV tag 9 与 UI layout
v1–v4 退役；只把 feature 的 `mornlea_client_render_apply_world_updates`、RenderWorld 与
MRW1 tests 加入该 surface。随后 client C header、Rust 常数和导出、Go bridge、动态库身份
检查、当前文档/配置/主规格与跨语言测试一起升级为 v13。

集成 v13 动态库的无参数 identity export `mornlea_client_abi_version()` 始终报告 13；其他
每个接受 ABI version 参数的 client export 保留既有 all-versioned-export ABI 检查，并在其
任何其他适用 validation 或状态改变前检查版本。它们
收到包括 12 在内的任一非 v13 版本时，必须在读取 handle、pointer、UI JSON 或 MRW1 bytes
前返回 `ABI_VERSION`。

反向混装需要区分“共有 export”与“v13-only symbol”。main v12 动态库的共有 versioned
exports 收到 v13 bridge 传入的 13 时，在读取其他输入或改变状态前返回 `ABI_VERSION`；但
main v12 根本不导出 `mornlea_client_render_apply_world_updates`，因此要求该 symbol 的 v13
bridge 必须在 link/load/bind 阶段硬失败，不能声称一次不存在的 FFI 调用会返回 status。
该失败不得进入 FFI body、改变状态或转入兼容入口/Go fallback。engine ABI 保持 v8，因为本
change 未改变无状态 engine 数值 ABI，也不移动其 fluid-aware 源码。

新 `mornlea_client_render_apply_world_updates` 是 input-only `u8` entry，且严格按 ABI
version、非零且受 MRW1 上限约束的 length、non-null pointer、无 overflow 的 address
range、existing renderer handle、MRW1 layout/capacity 的顺序验证，随后才允许预检并原子
改变 `RenderWorld`。`u8` pointer 没有额外 alignment 检查；入口没有输出 buffer，因此不执行
output-capacity 或 overlap 检查，这些约束仅由带输出的既有 entry 按适用性承担。所有 client
ABI export 均在 panic catcher 内，
panic 映射为 `PANIC`，并且不得 unwind 穿过 FFI 或留下部分 RenderWorld 状态。

否决“让两套不同 surface 继续共用 v12”或“在 main v12 上只添加可选 MRW1 符号”：这会
让 header、dylib 与 Go binding 对 v12 的含义产生歧义，不能在改变 UI/cache 状态前可靠失败。
也否决“用 feature 的 v12 header 覆盖 main”：这会复活已退役的 egui 字体/TLV surface，
并丢失当前 WKWebView/JSON UI 契约。

### D5: 现有 frame/draw 是可验证的不变量

新入口只由 Rust/Go ABI 测试和 test-only driver 驱动。既有 Go mesh/scheduler、
connectivity/visibility、geometry upload、`RenderFrame.Visible` payload、frame ABI 编码
和 Rust draw 选择不读取 `RenderWorld`。因此同一离屏 frame 在应用合法 MRW1 前后其
编码与 readback 字节必须相同；这既是第一阶段的回退点，也是后续 change 的基线。

否决“在 cache 建立后立即切换一个 draw 分支”：它会混合 cache 输入与 mesh/draw
迁移，无法独立审查或归因视觉回归。

## Risks / Trade-offs

- [错误的长度、乘法、indexed word count、尾随 bytes 或 packed 索引导致越界或部分应用]
  → 先做全量 checked 预检，固定 4-bit=256、8-bit=512、direct=1024 word 计数，并以
  atomic invalid-batch、边界与 fuzz 测试锁定失败前状态。
- [main v12 与集成 v13 header、dylib 和 Go binding 混装，或 input ABI 检查顺序不安全]
  → 双向分层验证：v13 动态库的全部 versioned exports 对 ABI 12 返回 `ABI_VERSION`；main
  v12 动态库的共有 exports 对 ABI 13 返回 `ABI_VERSION`；v12 缺失的 MRW1 symbol 必须在
  link/load/bind 阶段硬失败且无 fallback。新 input-only `u8` 入口只锁定 version、bounded
  nonzero length、pointer/address、handle、MRW1 次序，不增加 alignment/output/overlap 检查。
- [冲突解决复活已退役 UI export/TLV 或丢失 main JSON bridge] → 以 main v12 header/UI tests
  为基线，只叠加 MRW1；对 export 列表、frame tag 拒绝、`ui_push_state` 与 JSON event
  envelope 做定点审计和回归测试。
- [cache 误接入既有 draw] → test-only driver、frame 编码及 readback 字节相等测试；
  不修改 app、mesher、scheduler 或 fluid-aware 源码。
- [紧凑缓存提高常驻内存] → 4 MiB 单 batch和 4096 record 上限，按 tombstone/reset
  丢弃旧派生状态；本阶段不建立无界后台队列。

## Migration Plan

1. 保留已完成的 feature 基线测试与实现证据；集成前以 `--no-commit --no-ff` 合入最新
   main，重新读取 merged `AGENTS.md`，并以 main v12 WKWebView/UI surface 解决冲突。
2. 由 Go test-only encoder 从 `ContainerSnapshot` 生成 MRW1，覆盖 single、indexed、
   direct、column、tombstone、reset 和坐标边界；冲突解决只将该入口叠加到 main surface，
   不把入口接入实时 app。
3. 把 C header、Rust、Go、当前文档、active config、main specs 与 tests 同步为 v13；验证
   v13 动态库对全部 versioned exports 的 ABI 12 调用返回 `ABI_VERSION`，main v12 动态库的
   共有 exports 对 ABI 13 返回 `ABI_VERSION`，而 v13-only MRW1 symbol 对 v12 动态库在
   link/load/bind 阶段硬失败、没有 status 或 fallback；同时保留 UI JSON 与 MRW1 契约。
4. 在 merged baseline 重建 release dylib，再以离屏 renderer 对应用前后完全相同的既有
   frame input 做编码与 readback/visual 比较，并保留 Go mesh/visibility/upload 原路径。
5. 发布时 client ABI v13 与动态库同步替换；main v12 混装显式失败。若集成发现问题，
   回退 MRW1/v13 增量即返回 main v12 WKWebView/UI predecessor，而不是恢复 feature 的旧
   egui/TLV surface；没有网络、存档或世界数据迁移。

## Verification

- `make rust`
- `make rust-check`
- `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go`
- `go vet ./...`
- `go test ./internal/client -race -count=1`
- `go test ./internal/mesh -race -count=1`
- `go test ./internal/archcheck -count=1`
- `make test-race-changed`
- `go clean -testcache`
- `go test ./... -race`
- `make visual-check`
- `openspec validate --all --strict --no-interactive`
- `git diff --check`

集成实现与验证保持 cache-only：main predecessor client ABI 为 v12，统一目标为 v13，
engine ABI 保持 v8；MRW1 固定布局、坐标裁决、边界与阶段范围均由 binding brief 确定。
生产 app 的 MRW1 接线、Go mesh/visibility/upload/draw、共享 kernel、流体、协议、schema、
benchmark scenario 与 visual golden 均不得改变。本节列出的 merged-baseline 验证尚未执行。
