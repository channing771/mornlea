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

## Goals / Non-Goals

**Goals:**

- 建立 client ABI v12 的同步身份、MRW1 v1 更新入口，以及原子、可重建的
  `RenderWorld` cache。
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
2=direct。single 要求 bits/palette/packed 均为零；indexed 只允许 bits 4 或 8，
并校验所有 packed slot 小于 palette_count；direct 要求 bits 15、palette_count 为零、
packed_word_count 为 1024，且每个 word 的高四位为零。column upsert 的 payload 恰为
256 个 i16 LE height；其 section_y、storage_kind、bits 均为零。tombstone 的 payload
长度为零。world reset 的 record 仅允许作为 batch 第一条，所有坐标、revision、storage
元数据和 payload 均为零。

section record 的 X/Z/dimension 使用完整 signed i32 域，section Y 必须在
`0..core.SectionsPerChunk`；column record 的 section Y 必须为 0。所有长度计算使用
checked arithmetic，并在解引用 input 前完成 pointer、长度、容量、对齐和重叠检查。

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

### D4: ABI v12 必须整套同步并 fail fast

client C header、Rust 常数和导出、Go bridge、动态库身份检查与跨语言测试一起升级为
v12。每个 client ABI 入口先验证版本；错误版本在读取 handle、pointer 或 MRW1 bytes
前返回 `ABI_VERSION`。不存在 v11 兼容入口或 Go fallback。engine ABI 保持 v8，因为
本 change 未改变无状态 engine 数值 ABI，也不移动其 fluid-aware 源码。

否决“保持 v11 并只添加可选符号”：这会允许 header、dylib 与 Go binding 混装，不能
在改变 cache 前可靠失败。

### D5: 现有 frame/draw 是可验证的不变量

新入口只由 Rust/Go ABI 测试和 test-only driver 驱动。既有 Go mesh/scheduler、
connectivity/visibility、geometry upload、`RenderFrame.Visible` payload、frame ABI 编码
和 Rust draw 选择不读取 `RenderWorld`。因此同一离屏 frame 在应用合法 MRW1 前后其
编码与 readback 字节必须相同；这既是第一阶段的回退点，也是后续 change 的基线。

否决“在 cache 建立后立即切换一个 draw 分支”：它会混合 cache 输入与 mesh/draw
迁移，无法独立审查或归因视觉回归。

## Risks / Trade-offs

- [错误的长度、乘法或 packed 索引导致越界或部分应用] → 先做全量 checked 预检，
  以 atomic invalid-batch、边界与 fuzz 测试锁定失败前状态。
- [v11/v12 header、dylib 和 Go binding 混装] → 全入口 ABI 优先检查、版本常数测试，
  并在 clean Rust build 后运行 Go bridge race 测试。
- [cache 误接入既有 draw] → test-only driver、frame 编码及 readback 字节相等测试；
  不修改 app、mesher、scheduler 或 fluid-aware 源码。
- [紧凑缓存提高常驻内存] → 4 MiB 单 batch和 4096 record 上限，按 tombstone/reset
  丢弃旧派生状态；本阶段不建立无界后台队列。

## Migration Plan

1. 先以失败测试固定 MRW1 解析、原子预检、epoch/revision/tombstone 与 v12 fail-fast；
   再实现最小 `RenderWorld` 与 C/Go ABI 接线。
2. 由 Go test-only encoder 从 `ContainerSnapshot` 生成 MRW1，覆盖 single、indexed、
   direct、column、tombstone、reset 和坐标边界；不把入口接入实时 app。
3. 以离屏 renderer 对应用前后完全相同的既有 frame input 做编码与 readback 比较，
   并保留 Go mesh/visibility/upload 原路径。
4. 发布时 client ABI v12 与动态库同步替换；v11 混装显式失败。若发现 cache 输入或
   ABI 问题，回退本 change 即恢复 v11 与原始绘制路径；没有网络、存档或世界数据迁移。

## Verification

- `make rust`
- `cd engine && cargo test -p mornlea_client --locked`
- `go test ./internal/client -race -count=1`
- `go test ./internal/archcheck -count=1`
- `gofmt -l .`
- `go vet ./...`
- `go test ./... -race -count=1`
- `openspec validate --all --strict --no-interactive`

没有未决问题；MRW1 固定布局、坐标裁决、边界与阶段范围均由 binding brief 确定。
