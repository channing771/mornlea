# Rust RenderWorld Cache Design

## Context

本设计落实 `proposal.md` 与 `rust-client-render-cutover` delta 的第一个、可独立
回退的缓存闭环。Go Mirror 仍是客户端逻辑状态的真相来源；Rust `RenderWorld` 只保存
从已验证 Go 状态派生的渲染缓存，随 renderer 创建和销毁。缓存尚不接管 mesh、light、
connectivity、visibility、GPU upload 或 draw。

受影响文件限定为本 change 自有实现的
`engine/crates/mornlea_client/src/`、`engine/include/mornlea_client.h`、
`internal/client/` 与相应测试；selected-main 已有 app capture pump 的继承审计另显式覆盖
`cmd/mornlea/app/app_dependencies.go`、`cmd/mornlea/app/dev_capture.go`、
`cmd/mornlea/app/dev_capture_test.go` 与 `cmd/mornlea/app/interactive.go`，但本 change 不在
这些路径上新增功能。`internal/nativeabi` 仍是唯一接触
engine ABI 的边界。
follow-up main sync 会把 main 已交付的 engine ABI v9 与 fluid 实现带入最终树，但
`engine/crates/mornlea_engine/`、`engine/include/mornlea_engine.h`、`internal/fluid/`、
`internal/nativeabi/` 和 `internal/sim/realm/` 都是受保护路径，必须相对所选 main 父提交
保持零 feature-side diff。

feature 基线曾独立把 MRW1 export 分配给 client ABI v12，而当时的 main 已把 v12 分配给
进程内 WKWebView/UI cutover。Tasks 1–6.6 随后完成了 non-rewriting main sync、client ABI
v13 统一、main-side 注释清理、独立 review 与 immutable-HEAD 18 门禁；这些 completed 历史
及其当时选择的 main 不得改写。旧 selected main `e1e2e287` 上的 Task 6.6 曾完整通过；之后
exact feature HEAD `5e5243a7` 的 revalidation 在 Gates 1–17 全部通过，但 Gate 18 发现 local
main 已前进到 `bcc053e8`，因此最终是 17/18 binding failure、0 product failures、0 Skip。
feature 后续只 non-rewriting 同步了 `bcc053e8` 上 5 个无关 `dev-capture` planning files，
形成 merge `65ac703a` 与 ledger commit `eccdca39`，没有改变 runtime/client contract。

以 `eccdca39` 为候选的新 revalidation 在 Gate 1 前再次执行 preflight 时，local main 已从
`bcc053e8` 前进到 `be5ff22bf3b5c35199884177e8e0a595d5713c30`，因此 0/18 gates started、
0 product failures 并取消。`bcc053e8..be5ff22b` 恰有 1 个提交
`feat(client): add window composite capture with client abi v13`，修改 8 个路径：根
`AGENTS.md`、新增 `engine/crates/mornlea_client/src/capture.rs`，以及 client `ffi.rs`、
`lib.rs`、`window.rs`、C header、Go `window.go` 与 `window_test.go`。这不是可忽略的 planning
漂移：latest main 已把 capture surface 分配给 client ABI v13。

planning 修改期间 local main 先前进到 capture code-fix
`4c553f3b3b34f575ebb5304d67984af3425c7209`。`be5ff22b..4c553f3b` 恰有 1 个提交
`fix(client): align window capture cg option bits with sdk headers`，只修改
`engine/crates/mornlea_client/src/capture.rs`，38 insertions / 11 deletions。完整 patch 审计确认
它保持同一 client ABI v13/export/header/Go surface，只修正 capture 的 SDK 对齐：
`CGWindowListOption`/`CGWindowImageOption` FFI 参数由错误的 `u64` 改为 SDK `u32`，
IncludingWindow 从错误的 `1<<0` 改为 `1<<3`，BestResolution 从错误的 `1<<8` 改为
`1<<3`，并新增 SDK option/BGRA bitmap bits 防回归测试。`be5ff22b` 与 `4c553f3b` 的
FFI export 名单 SHA-256 相同，header/FFI/lib/Go diff 为零，因此这是一项必须继承的 capture
实现正确性修复，不改变 v14 union 裁决。

随后 local main 又前进到当时 observed latest
`a83192b7d9a95cb622fc29035b199a8a6de5645c`。`4c553f3b..a83192b7` 恰有 1 个提交
`docs: record dev-capture task 2 review rounds`，只修改
`openspec/changes/dev-capture/{design.md,ledger.md,tasks.md}`，43 insertions / 4 deletions。
完整 patch 与更新后三文件审计确认它只记录 Task 2 implementer/review/fix rounds、勾选 2.1，
并把 design 的 validation order 文字校正为已实现的 ABI version → output pointer/zero-capacity
consistency → handle → capacity；production/test code、header/FFI/lib/window/capture/Go、client
ABI v13、export/identity 数量与公共 contract 均零变化。这项 docs-only 漂移可由 Task 6.8
non-rewriting sync 继承，不改变 v14 planning；`a83192b7` 曾取代 `4c553f3b` 成为
merge-time audit anchor，后续又成为当前锚点之前的历史中间点。

独立 planning review 期间 local main 又前进到
`522c7d6a795fd4b4baf7b88fd1c0bc1a4949040f`。`a83192b7..522c7d6a` 恰有 1 个提交
`feat(app): add frame loop capture pump`，只修改
`cmd/mornlea/app/app_dependencies.go`、新增 `dev_capture.go`、新增 `dev_capture_test.go` 与修改
`interactive.go`，共 4 paths、356 insertions / 0 deletions。它对 client header、Rust client
source、`internal/client`、ABI constants、28+1 export 集合、五组 protected paths 与 visual
golden 均零 diff，因此不改变 v14 的 29+1 union 数量；但它把 capture bridge 接入生产 app，
新增 `CaptureCoordinator`/`SetCaptureCoordinator`、菜单与游戏两处 `pumpDevCapture`、非阻塞
single-outstanding 与 pixels/error 交付语义，以及 7 个 app-pump tests，属于必须先重规划并
原样继承的 contract-affecting production integration。

planning-fix 交接前 local main 又前进到当前 observed latest
`9bb84c6841b59a18b030256d5952ed60acc215da`。`522c7d6a..9bb84c68` 恰有 1 个提交
`docs: record dev-capture task 3 review verdict`，只修改
`openspec/changes/dev-capture/ledger.md` 与 `tasks.md`，21 insertions / 1 deletion。完整 patch
审计确认它只记录 app pump Task 3 的实现、7 tests、验证与独立 spec/quality 双 PASS，并勾选
3.1；app source/tests、client header/Rust/Go、ABI/export/identity、protected/golden、current docs、
main specs 与 config 均零 diff。因此它是同一 dev-capture 范围的 docs-only bookkeeping，不改变
app pump 或 v14 union contract。`bcc053e8..9bb84c68` 精确为 5 commits、15 unique paths、
1192 insertions / 30 deletions，当前 merge-time audit anchor 更新为 `9bb84c68`。

Task 6.8 随后已完成 non-rewriting merge
`6f622407b1078d264707d8643f7fec41c553a48e`，其双亲依次为 feature
`9dc22f1b9a8106f71a5f6496ac2bd708c31c5584` 与 selected-main
`9bb84c6841b59a18b030256d5952ed60acc215da`。用户在实现暂停点明确裁决“不用管 main，把自己的
实现”，因此该 second parent 现冻结为本 change 的唯一 selected-main provenance；本分支不再
合入、审计 adoption 或要求 parity 于 `8646c313` 或任何更晚 main。Task 6.8 的其余 ABI v14
实现与独立 reviews 仍未完成，任务保持 unchecked。

精确 source/header 审计显示，`bcc053e8` 是 client ABI v12，包含 27 个 versioned exports
与 1 个 identity export；observed latest main `9bb84c68` 是 client ABI v13，包含 28 个 versioned
exports 与 1 个 identity export，相对前者恰好新增 `mornlea_client_window_capture`。当前
feature `eccdca39` 也占用 v13，包含另一路 28 个 versioned exports 与 1 个 identity export，
但它相对 `bcc053e8` 新增的是 `mornlea_client_render_apply_world_updates` 而不是 capture。
因此两套 v13 surface 不能合并后继续共用 v13；最终 union 必须完整保留 selected-main v13
的 28 个 versioned exports，再叠加 MRW1，统一升级为 client ABI v14：29 个 versioned
exports、1 个 identity export、总计 30 个 exports。

## Goals / Non-Goals

**Goals:**

- 建立集成 client ABI v14 的同步身份，在 exact `9bb84c68` selected-main v13 capture/WKWebView/UI
  surface 上只叠加 MRW1 v1 更新入口与原子、可重建的 `RenderWorld` cache。
- 完整保留 selected-main v13 的 28 个 versioned exports、window composite capture
  status/两段式容量/top-down BGRA8、Go bridge 与 Rust/Go tests，形成精确的 29+1 v14 union。
- 原样保留 selected-main 的 `CaptureCoordinator`/`SetCaptureCoordinator`、菜单与游戏两处
  `pumpDevCapture`、非阻塞 single-outstanding/pixel ownership/error delivery 语义和 7 个
  app-pump tests，同时证明 MRW1 仍无 production caller。
- 保留已完成的 non-rewriting merge `6f622407`，原样继承冻结父 `9bb84c68` 的 engine ABI v9
  与伙伴 fluid 实现，并以受保护路径审计证明本 change 没有 feature-side engine/fluid 修改。
- 在 Rust 拷贝或规范化 Go 同步调用期间提供的 bytes；之后不保存 Go 指针、slice 或
  对象，且按 epoch、revision、tombstone 决定派生缓存状态。
- 用单元、FFI、Go bridge、fuzz 与 test-only driver 锁定 wire 校验、原子失败和离屏
  frame 字节不变。
- 在不扩大 MRW1 行为范围的前提下，确保最终代码注释满足任务编号纪律，并准确描述 Rust
  client 的生产 GPU 所有权、Go 的 CPU 半部以及 identity/versioned export 边界。

**Non-Goals:**

- 不创建 mesh worker、goroutine、thread、GPU pool、GPU 写入或任何 worker/GPU 调度。
- 不把 MRW1 接入 `cmd/mornlea/app` 实时消息路径，不替换 selected-main 已有 capture pump，
  也不替换 Go CPU mesh、connectivity、visibility、`RenderFrame.Visible`、逐 section upload
  或 draw。
- 不实现、接管或扩展 selected-main app pump 之外的 `dev-capture` service、HTTP、options、
  recording 或 docs 工作。
- 不移动、复制或重构任何 engine 的 fluid-aware input、light、quad 或 greedy 源码；
  不接管、改写或扩展伙伴交付的流体状态、传播、tick、协议或专属 mesh 语义。
- 不创建共享 voxel kernel，不改网络协议 v32、存档 schema、world metadata、benchmark
  scenario v20；engine ABI v9 仅作为所选 main 的既有身份原样继承。
- 不改写 Tasks 1–6.6 的历史，不修改 main，不 push、不 archive、不 rebase，也不把 feature
  merge into main；本轮先只修订 OpenSpec planning，等待独立 planning review。
- 不合入、审计 adoption 或要求 parity 于 `8646c313` 或任何更晚 main；若 later-main 工作需要
  集成，必须作为本 change 之外的独立后续工作处理。

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

### D4: 以 selected-main v13 capture 为前代，把最终 union 分配为 ABI v14

冲突解决与后续验证必须以 exact `9bb84c6841b59a18b030256d5952ed60acc215da` 的 v13 header、
Rust exports、Go bridge 与 app capture pump 为 predecessor。该冻结父的 surface 包含 28 个 versioned exports 与 1 个 identity
export；相对 `bcc053e8` 恰好新增 `mornlea_client_window_capture`。最终实现必须保留全部
28 个 versioned exports，包括 capture 的 `CAPTURE_OVERFLOW`/`CAPTURE_UNAVAILABLE` status、
两段式容量查询、`NSWindow windowNumber` 到 `CGWindowListCreateImage`/`CGBitmapContext` 的
完整窗口合成、紧凑 top-down BGRA8 bytes、Go `Window.Capture`/typed unavailable error、
未知 handle 稳定 panic、`4c553f3b` 已校正的 CoreGraphics `u32` option FFI width、两个
`1<<3` option bits、ABI → output pointers/zero-capacity consistency → handle → capacity 的
capture validation order 以及现有 Rust/Go tests；还要保留 WKWebView、`ui_push_state` 与版本化
JSON UI event drain，并保持旧字体 upload、frame TLV tag 9 与 UI layout v1–v4 退役。

同一 predecessor 的 production app pump 也必须原样继承：`CaptureCoordinator` 与
`SetCaptureCoordinator` 继续构成启动前依赖注入，菜单和游戏 loop 继续在 `Window.Poll` 后、
render 前分别调用 `pumpDevCapture`。nil coordinator 或无 pending request 必须非阻塞返回；
每帧至多检查/完成一次请求，single outstanding 由 coordinator 维持而不在帧循环排队；成功
发送后紧凑 top-down BGRA8 pixels 归 service goroutine，pump 不再持有或修改；capture error
包括 typed `client.ErrCaptureUnavailable` 必须原样交付，不吞掉、不重试、不伪造。selected-main
已有 7 个 app-pump tests 必须完整保留。本 change 不实现余下 `dev-capture` service、HTTP、
options、recording 或 docs，也不把 pump 误作 MRW1 production caller。

feature 只向该 predecessor 叠加 `mornlea_client_render_apply_world_updates`、RenderWorld 与
MRW1 tests。最终 C header/Rust/Go/current identity docs 必须统一为 client ABI v14，形成
29 个 versioned exports 与 1 个 identity export、总计 30。无参数 identity export
`mornlea_client_abi_version()` 始终报告 14；全部 29 个接受 ABI version 参数的 v14 exports
必须在其他 validation 或状态改变前拒绝包括 13 在内的任何非 14 版本。最终 all-versioned
ABI-first test 必须精确枚举这 29 个 exports，同时覆盖 capture 与 MRW1，不得用推断数量替代。

反向混装同样分两层验证。exact selected-main v13 dylib 的 28 个 versioned exports 收到
v14 bridge 传入的 14 时必须全部在读取其他输入或改变 window/renderer/capture/UI 状态前
返回 `ABI_VERSION`。它不导出 v14-only `mornlea_client_render_apply_world_updates`；要求该
symbol 的 v14 bridge 与 v13 dylib 组合必须在 link/load/bind 阶段硬失败，不能声称不存在的
FFI 调用会返回 status。不得进入 FFI body、改变状态、动态加载可选 symbol 或转入 Go fallback。

MRW1 entry 仍是 input-only `u8` entry，严格按 ABI version、非零且受上限约束的 length、
non-null pointer、无 overflow 的 address range、existing renderer handle、MRW1
layout/capacity 顺序验证，随后才允许预检并原子改变 `RenderWorld`。`u8` pointer 没有额外
alignment 检查；入口没有输出 buffer，因此不执行 output-capacity 或 overlap 检查。全部
client ABI exports 都必须在 panic catcher 内，panic 映射为 `PANIC` 且不得留下部分状态。

最终 engine ABI 为从 exact `9bb84c68` 原样继承的 v9；本 change 不修改 engine 数值
kernel 或 fluid-aware 源码。回退只允许移除 MRW1/client-v14 增量并回到 exact `9bb84c68` v13
capture predecessor，必须保留 capture、app frame-loop pump、engine ABI v9 与伙伴 fluid，
不得回到 v12。

否决“把 main capture 与 feature MRW1 两套不同 surface 继续共用 v13”：header、dylib 与 Go
binding 对 v13 的含义会产生歧义。也否决“用 feature v13 覆盖 main”或“把 capture 作为可选
动态 symbol”：前者会删除已交付的 capture，后者会引入不允许的兼容路径并掩盖错误绑定。

### D5: 现有 frame/draw 是可验证的不变量

新入口只由 Rust/Go ABI 测试和 test-only driver 驱动。既有 Go mesh/scheduler、
connectivity/visibility、geometry upload、`RenderFrame.Visible` payload、frame ABI 编码
和 Rust draw 选择不读取 `RenderWorld`。因此同一离屏 frame 在应用合法 MRW1 前后其
编码与 readback 字节必须相同；这既是第一阶段的回退点，也是后续 change 的基线。

否决“在 cache 建立后立即切换一个 draw 分支”：它会混合 cache 输入与 mesh/draw
迁移，无法独立审查或归因视觉回归。

### D6: 历史 follow-up main sync 固定执行时父提交并保护伙伴路径

历史 Task 6.2 由 fresh implementer 在 clean worktree 中执行。它在 merge 命令前立即
读取 local `main`；若 `main` 已超过已审计的 `a23833f9`，先记录新增 commits/paths 并判断
是否改变版本、ABI、client surface、MRW1、fluid 所有权或排除项。只有新提交不改变契约与
范围时才可直接选为 main 父；若改变，则先更新本 change planning 并完成相应独立 planning
review。随后使用 `git merge --no-commit --no-ff main`，不 rebase、不改写已评审 Task 6.1。

merge 开始后以实际 `MERGE_HEAD` 为不可变 selected-main-parent，重新读取 merged root 与目标
目录 `AGENTS.md`，逐项解决真实 overlap。`engine/crates/mornlea_engine/`、
`engine/include/mornlea_engine.h`、`internal/fluid/`、`internal/nativeabi/` 和
`internal/sim/realm/` 必须与 selected-main-parent 逐字一致；根版本身份与 current docs/main
specs 只允许产生 client ABI v13 与 cache-only 事实所需的 feature-side 差异。fresh reviewer
必须独立核验 merge parent、全部 conflict resolution 和 protected path zero-diff 后，Task 6.2
才可完成。

否决“为了排除流体而跳过新 main”：这会让最终树停留在已过时的 engine ABI v8。也否决
“把伙伴 fluid 复制进 feature 再手工重放”：这会模糊所有权、破坏路径同一性并扩大冲突面。

### D7: 历史终审修复先清理 main，再重新固定集成基线

原 Task 6.5 的 Important finding 属于 selected main：25 处代码注释包含
`[A-F]-[0-9]{2}` 形态的任务编号，feature-added lines 为零，且不位于五组 protected paths。
因此先由独立 main-side implementer 在 latest local main 上做 comment-only cleanup；若增加
防回归 archcheck，必须先以现有违规注释证明 RED，再完成 GREEN。独立 reviewer 同时给出
spec-compliance 与 quality verdict 后，local main 才可用 fast-forward 前进到该 reviewed
cleanup；不得 push，也不得把 feature 合回 main。

随后由 fresh feature implementer 在 merge 命令前即时读取新的 local main。若 reviewed
cleanup 后 main 又前进，必须先审计新增 commits/paths；任何版本、ABI、client/MRW1 surface、
fluid 所有权或排除项变化都先回到 planning。契约不变时，以
`git merge --no-commit --no-ff main` 做 non-rewriting sync，并以实际 `MERGE_HEAD` 重新记录
selected-main-parent、merge 双亲、冲突和逐项裁决。最终树相对新父的五组 protected paths 与
visual golden 必须零 diff。

feature 只最小修正两处终审 Minor finding：`internal/client/render.go` 必须说明 Rust client
独占生产 GPU 渲染、Go 保留 CPU mesh/visibility/frame input，而 v13 RenderWorld cache 才是
仅测试驱动；`engine/crates/mornlea_client/src/ffi.rs` 必须区分无参数 identity export 与其余
28 个接受 `abi_version` 的 versioned exports。该 sync/fix 取得独立 spec/quality review 后，
才能冻结新的 final HEAD。

新的 validation implementer 必须在该 immutable final HEAD 重跑 Task 6.4 的完整 18 门禁；
不得把 `10f8e8ab` 的 PASS 继承为新 HEAD 证据。最后一名未参与 planning、main cleanup、
feature sync/fixes 或 validation 的 fresh reviewer 必须同时核验行为规格、代码质量、代码注释
任务编号零匹配、两处注释修复、新 selected-main protected/golden zero-diff 与完整同基线证据。
只有 0 open findings 才可完成 change implementation。

否决“直接在 feature 清扫 25 处继承注释”：这会把无关 main hygiene 伪装为 feature 改动并
破坏 selected-main 归属。也否决“只跑定点测试或复用旧完整验证”：main merge 和 tracked
comment fixes 已改变最终 commit identity，旧 HEAD 的 release/race/visual 证据不能证明新树。

该路径最终完成了 Tasks 6.1–6.6；其 selected main、client ABI v13 与完整验证证据都是
不可改写历史。它不授权在后续 main 漂移后继续沿用旧 binding，也不覆盖下面针对
`be5ff22b` contract-affecting capture surface 及其 `4c553f3b` SDK option fix 的新裁决。

### D8: 保留已完成 merge 并冻结 selected-main parent

Task 6.7 只允许由 fresh planning implementer 修改本 change 的 5 个 planning artifacts，写入
ignored planning report，并接受独立 planning review；不得修改生产/测试代码、main specs、
current docs、config 或 main，也不得 merge/rebase/archive/push。planning review 前 Task 6.7
保持 pending。commit `6d69db84` 的首轮 independent review 因遗漏 selected-main app pump
给出 1 个 Important、spec/quality Needs fixes，明确禁止 Task 6.8 开始；本修复轮必须把该
finding、修订证据与复审裁决 append 到 ledger，不得改写前轮或 Tasks 2–6.6 历史。

Task 6.8 已完成 non-rewriting merge `6f622407b1078d264707d8643f7fec41c553a48e`，其双亲必须
保持为 feature `9dc22f1b9a8106f71a5f6496ac2bd708c31c5584` 与 selected-main
`9bb84c6841b59a18b030256d5952ed60acc215da`。用户明确裁决冻结该 selected-main parent，并在
这一基线上继续本分支自己的 combined client ABI v14 实现。Task 6.8 不得再执行第二次 main
merge，不得审计 adoption 或要求 parity 于 `8646c313` 或任何更晚 main，也不得因此改写历史、
修改 main、删除已继承的 v13 capture/app-pump contract、触碰伙伴 fluid 或弱化门禁。若该裁决
遗漏 later-main 的无关或兼容工作，代价是未来另开一次独立集成；本 change 的 v14 正确性仍只
相对冻结父判定。

已完成 merge 的结果继续以 `engine/include/mornlea_client.h`、client
`ffi.rs`/`lib.rs`/`window.rs`、Go `window.go`/`window_test.go`、current identity docs，以及 app pump 的
`cmd/mornlea/app/app_dependencies.go`、`cmd/mornlea/app/dev_capture.go`、
`cmd/mornlea/app/dev_capture_test.go`、`cmd/mornlea/app/interactive.go` 为继承基线。四个 app
paths 的最终内容必须相对 exact `9bb84c68` 精确零 diff，并纳入 `gofmt` 零差异与 7 个
app-pump focused tests。暂停的最小实现继续统一 client ABI v14，保留冻结父的 capture
exports/behavior/tests、生产 bridge 与 app pump，
只叠加 MRW1 cache-only/no-production-caller 增量；不得实现或接管其余 `dev-capture` service、
HTTP、options、recording 或 docs。五组 protected paths
`engine/crates/mornlea_engine`、`engine/include/mornlea_engine.h`、`internal/fluid`、
`internal/nativeabi`、`internal/sim/realm` 与 visual golden 都必须相对 exact `9bb84c68`
零 diff。完成实现后必须分别接受 fresh spec-compliance review 与 quality review。

Task 6.9 由另一 fresh validation implementer 在 immutable final HEAD 运行完整 18 门禁；任何
tracked post-validation fix 都会产生新 HEAD，必须从 Gate 1 完整重跑。原 moving-ref /
exact-current-main gate 改为 frozen-parent provenance gate：`6f622407` 必须是 final HEAD 的祖先
并保持上述 exact 双亲，所有 app/protected/golden 比较使用 exact `9bb84c68`，且 `6f622407`
之后不得再通过额外 main merge 引入该父之后的 main commits；exact implementation HEAD 与 clean
status 仍是硬门禁。Task 6.10 再由未参与 planning、sync/implementation、reviews 或 validation
的 fresh reviewer 按同一冻结 provenance 做 whole-integration 终审；local `main` 指向其他提交
本身不得导致失败。只有 0 open findings 才可宣告 implementation complete。

## Risks / Trade-offs

- [错误的长度、乘法、indexed word count、尾随 bytes 或 packed 索引导致越界或部分应用]
  → 先做全量 checked 预检，固定 4-bit=256、8-bit=512、direct=1024 word 计数，并以
  atomic invalid-batch、边界与 fuzz 测试锁定失败前状态。
- [selected-main v13 与集成 v14 header、dylib 和 Go binding 混装，或 input ABI 检查顺序
  不安全] → 双向分层验证：v14 动态库的全部 29 个 versioned exports 对 ABI 13 先返回
  `ABI_VERSION`；exact selected-main v13 动态库的全部 28 个 versioned exports 对 ABI 14
  先返回 `ABI_VERSION`；v13 缺失的 MRW1 symbol 必须在 link/load/bind 阶段硬失败且无动态
  加载/Go fallback。新 input-only `u8` 入口只锁定 version、bounded nonzero length、
  pointer/address、handle、MRW1 次序，不增加 alignment/output/overlap 检查。
- [冲突解决删除 capture、复活已退役 UI export/TLV 或丢失 main JSON bridge] → 以 exact
  `9bb84c68` v13 header、FFI/window/capture 与 Rust/Go tests 为基线，只叠加 MRW1；对
  29+1 export union、capture status/容量/BGRA8/Go bridge、SDK `u32` option FFI width 与两个
  `1<<3` option bits、frame tag 拒绝、`ui_push_state` 与 JSON event envelope 做定点审计和
  回归测试。
- [已完成的 non-rewriting merge 静默删除、移动或改变 selected-main app capture pump] → 把四个 app
  paths 相对 exact `9bb84c68` 做精确 zero-diff，再由独立 reviewer 核验 `CaptureCoordinator` 注入、菜单/游戏两处
  poll 后 render 前 pump、非阻塞 single-outstanding、pixels ownership、error delivery 与 7 个
  focused tests。MRW1 仍无 production caller，且不接管其余 dev-capture 工作。
- [cache 误接入既有 draw 或 inherited app pump] → test-only driver、no-production-caller、frame
  编码及 readback 字节相等测试；不改变 selected-main app pump、mesher、scheduler 或
  fluid-aware 源码。
- [验证误把移动的 local `main` 重新绑定为 predecessor，或额外 main merge 覆盖伙伴
  fluid/engine] → 把 `9bb84c68` 固定为唯一 provenance；核验 `6f622407` 的 exact 双亲，禁止
  之后再引入 later-main 的 merge，并只以 `9bb84c68` 对五组受保护路径与 golden 执行 zero-diff
  审计。local `main` 指向 `8646c313` 或任何更晚提交不改变本 change 的结果。
- [继承的代码注释违规被误归入 feature，或注释修复后继续沿用旧验证] → 先在独立 main-side
  cleanup 中清零并评审任务编号，再 non-rewriting sync；任何新 tracked HEAD 都重跑完整
  18 门禁，最终 fresh reviewer 复核零匹配与同基线证据。
- [紧凑缓存提高常驻内存] → 4 MiB 单 batch和 4096 record 上限，按 tombstone/reset
  丢弃旧派生状态；本阶段不建立无界后台队列。

## Migration Plan

1. 保留 Tasks 1–6.6 的 completed 历史、旧 selected main `e1e2e287` 完整 PASS、
   `5e5243a7` 的 17/18 binding failure、`bcc053e8` 无关 planning sync 与 `eccdca39` 上因
   `be5ff22b` 漂移产生的 0/18 preflight cancellation；不得重写旧 merge/review/validation。
2. Task 6.7 已只更新 proposal、唯一 delta spec、design、tasks 与 ledger，记录 planning
   implementer identity 和 ignored report；同时记录 `6d69db84` review 的 app-pump Important
   finding 与本修复轮证据，并已由独立 reviewer 复审通过后勾选。
3. Task 6.8 已以 non-rewriting merge `6f622407` 固定 feature parent `9dc22f1b` 与 selected-main
   parent `9bb84c68`。保留该 merge，不执行第二次 main merge，不审计或吸收 `8646c313` 及
   later-main；本 planning amendment 必须先接受独立 planning review，暂停实现方才可恢复。
4. 恢复 Task 6.8 后，以 exact `9bb84c68` 审计 client header、`ffi.rs`、`lib.rs`、`window.rs`、
   Go `window.go`/`window_test.go`、identity docs 与四个 app pump paths。四个 app paths 必须
   exact zero-diff，并运行 `gofmt` 与 7 个 focused tests；五组 protected engine/fluid paths
   与 visual golden 同样必须 byte-for-byte zero-diff。
5. 继续暂停中的最小实现，把 C header、Rust、Go、current docs、active config、main specs 与 tests 同步为
   client ABI v14 / inherited engine ABI v9。保留 selected-main v13 的全部 28 个 versioned
   exports 与 capture/WKWebView/UI behavior/tests，并原样保留 app pump 的 coordinator 注入、
   两个 loop call sites、非阻塞 single-outstanding、pixels/error 交付与 7 tests；只叠加
   v14-only MRW1，形成 29+1 union。MRW1 继续 cache-only、no production caller；其余
   dev-capture service/HTTP/options/recording/docs 保持本 change 范围外。
6. 在同一实现 HEAD 验证 v14 全部 29 个 versioned exports 对 ABI 13 的 ABI-first 拒绝、
   exact `9bb84c68` v13 全部 28 个 versioned exports 对 ABI 14 的 ABI-first 拒绝，以及 v13
   缺少 v14-only MRW1 symbol 的 bind hard failure；不得增加动态加载或 Go fallback。随后由
   fresh spec reviewer 与 fresh quality reviewer 分别审查 Task 6.8。
7. Task 6.9 在独立 immutable final HEAD 重建 release dylib并运行完整 18 门禁，覆盖 capture
   symbol/status/两段式容量/BGRA8/Go bridge/tests、app pump 四路径 zero-diff/gofmt/7 focused
   tests、MRW1 原子 cache-only/no production caller/frame 不变、RPATH/SHA、protected/golden
   zero-diff 与全部身份。任何 tracked fix 都必须从 Gate 1 重跑。
8. Task 6.10 由未参与 planning、implementation、Task 6.8 reviews 或 validation 的 fresh
   reviewer 做 whole-integration final review；只有 spec/quality 均通过且 0 open findings
   才宣告 implementation complete。
9. 发布时 client ABI v14 header、Go bridge 与 dylib 同步替换，v13/v14 混装显式失败。若
   集成发现问题，只回退 MRW1/client-v14 增量到 exact `9bb84c68` v13 capture/app-pump predecessor；
   必须保留 capture、app frame-loop pump、engine ABI v9、伙伴 fluid 与其测试，不得回到 v12
   或恢复旧 egui/TLV surface。没有网络、存档或世界数据迁移。

## Verification

Task 6.9 必须在 Task 6.8 implementation 及其 spec/quality reviews 通过后的同一个 immutable
final HEAD 完整运行以下 18 门禁，逐项记录命令、exit status、真实 wall time、Skip、exact
frozen parent `9bb84c6841b59a18b030256d5952ed60acc215da`、final HEAD 和 artifact SHA。旧 Task 6.6 PASS、17/18 revalidation 与
0/18 cancellation 只保留为历史，不能计入本轮。任何 tracked post-validation fix 或 sync
都会改变 HEAD，必须从 Gate 1 重新完整运行，不能只补跑失败项：

1. `make rust`
2. `make rust-check`
3. `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go internal/server/sword_combat_parity_test.go cmd/mornlea/app/app_dependencies.go cmd/mornlea/app/dev_capture.go cmd/mornlea/app/dev_capture_test.go cmd/mornlea/app/interactive.go`，随后证明零 diff
4. `go vet ./...`
5. `go test ./internal/client -race -count=1`
6. `go test ./internal/mesh -race -count=1`
7. `go test ./internal/archcheck -count=1`
8. `make test-race-changed`
9. `go clean -testcache`
10. `go test ./... -race -count=1`
11. `make visual-check`
12. `openspec validate --all --strict --no-interactive`；当前 planning 基线为 80/80 changes，
    若实现时仓库总数变化则记录并以当时实际全量计数为准
13. `git diff --exit-code 9bb84c6841b59a18b030256d5952ed60acc215da..HEAD -- engine/crates/mornlea_engine engine/include/mornlea_engine.h internal/fluid internal/nativeabi internal/sim/realm`
14. `git diff --exit-code 9bb84c6841b59a18b030256d5952ed60acc215da...HEAD -- cmd/mornlea/capture/testdata/golden`
15. client contract/artifact 审计：final v14 必须精确为 29 个 versioned exports + 1 identity
    export（总计 30），全部 29 个对 ABI 13 ABI-first；exact selected-main v13 必须精确为
    28 个 versioned exports + 1 identity export（总计 29），全部 28 个对 ABI 14 ABI-first；
    v13 dylib 必须保留 `mornlea_client_window_capture` 而缺少 v14-only MRW1 symbol，后者以
    link/load/bind hard failure 证明且无动态加载/Go fallback。还要核验 capture status、两段式
    capacity、top-down compact BGRA8、Rust/Go capture tests、Go `Window.Capture` production
    surface、WKWebView/JSON UI 保留与 retired UI 不复活；核验 MRW1 原子/cache-only、无
    production caller、frame encoding/readback 不变，以及 Go test-binary RPATH 指向同一
    release dylib 且记录 dylib SHA；capture audit 还必须钉死 SDK `u32` option FFI width、
    IncludingWindow=`1<<3`、BestResolution=`1<<3` 与对应防回归测试。app pump 还必须以
    `git diff --exit-code 9bb84c6841b59a18b030256d5952ed60acc215da..HEAD -- cmd/mornlea/app/app_dependencies.go cmd/mornlea/app/dev_capture.go cmd/mornlea/app/dev_capture_test.go cmd/mornlea/app/interactive.go`
    证明四路径 exact selected-main zero-diff，并运行
    `go test ./cmd/mornlea/app -race -run 'Test(PumpDevCapture|RunInteractive(Game|Menu)LoopPumpsPendingCaptureOnce)' -count=1`，
    证明 7 个 focused tests 保留；同时审计两处 poll 后 render 前调用、nil/idle 非阻塞、single
    outstanding、pixels ownership 与 error 原样交付
16. 身份/范围审计：protocol 32、player 8、chunk 9、world 3、companions 4、hostile 1、
    engine 9、client 14、benchmark 20；代码注释任务编号零匹配；除原样继承且已在 Gate 15
    证明 exact selected-main zero-diff 的 app pump 外，不新增 production app caller；Go
    mesh/visibility/upload/draw、共享 kernel、伙伴 fluid/engine 与 visual golden 无 feature-side
    改动；其余 dev-capture service/HTTP/options/recording/docs 不属于本 change
17. `git diff --check`
18. frozen-parent provenance：`6f622407b1078d264707d8643f7fec41c553a48e` 必须是 exact final
    HEAD 的祖先且双亲仍精确为 `9dc22f1b` / `9bb84c68`；`6f622407` 之后不得存在把
    `9bb84c68` 之后 main commits 引入本分支的额外 main merge；同时证明 exact implementation
    HEAD、tracked/staged/untracked clean status 与全部证据同基线。local `main` ref 的当前位置
    不参与 PASS/FAIL。

集成实现与验证保持 cache-only：predecessor 是 exact `9bb84c68` client ABI v13 capture/app-pump，
统一目标是 client ABI v14，engine ABI 为从该冻结父原样继承的 v9。回退只回到该 v13
capture/app-pump predecessor，不能删除 capture/fluid 或回到 v12。本节列出的新 final-HEAD 18 门禁
尚未执行。
