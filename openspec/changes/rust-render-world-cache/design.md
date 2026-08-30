# Rust RenderWorld Cache Design

## Context

本设计落实 `proposal.md` 与 `rust-client-render-cutover` delta 的第一个、可独立
回退的缓存闭环。Go Mirror 仍是客户端逻辑状态的真相来源；Rust `RenderWorld` 只保存
从已验证 Go 状态派生的渲染缓存，随 renderer 创建和销毁。缓存尚不接管 mesh、light、
connectivity、visibility、GPU upload 或 draw。

受影响文件限定为本 change 自有实现的
`engine/crates/mornlea_client/src/`、`engine/include/mornlea_client.h`、
`internal/client/` 与相应测试。`internal/nativeabi` 仍是唯一接触 engine ABI 的边界。
follow-up main sync 会把 main 已交付的 engine ABI v9 与 fluid 实现带入最终树，但
`engine/crates/mornlea_engine/`、`engine/include/mornlea_engine.h`、`internal/fluid/`、
`internal/nativeabi/` 和 `internal/sim/realm/` 都是受保护路径，必须相对所选 main 父提交
保持零 feature-side diff。

feature 基线曾独立把 MRW1 export 分配给 client ABI v12；与此同时，当前 main 已把
client ABI v12 分配给进程内 WKWebView/UI cutover：退役 `render_upload_ui_font` 与 frame
TLV tag 9（UI layout v1–v4），新增 `ui_push_state`，并把 `render_drain_ui_events` 的载荷改为
版本化 JSON 信封。两套不同 export surface 不能共享同一 ABI 版本，因此集成必须以 main
v12 为前代基线，把“main UI surface + MRW1”统一分配为 v13。

已评审 Task 6.1 的固定 main 父是 `8b8891a3`，该提交当时仍为 engine ABI v8；这是一条
不可改写的历史事实。Task 6.1 进行期间 local main 前进 22 commits 到 `a23833f9`：该范围
加入伙伴负责的 Rust/Go fluid eval/rescan、engine ABI v9 与四项测试稳定性修复，没有改变
client ABI v12 WKWebView surface，也没有加入 MRW1。最终集成因此需要一次新的 non-rewriting
main sync：继承 engine ABI v9/fluid，但不把它们纳入本 change 的设计或实现所有权。

Tasks 6.1–6.4 已在 feature HEAD `10f8e8ab` 完成，所选 main 父仍为 `a23833f9`。原
Task 6.5 whole-integration review 的行为规格裁决通过，但 quality 未通过：最终树继承了
selected main 的 25 处代码注释任务编号，并有两处 feature 邻近注释错误描述生产 GPU
所有权和 versioned export 数量。该 review 之后的任何 tracked 修改或 main sync 都会产生
新的实现 HEAD，因此 Task 6.4 的完整验证证据只属于 `10f8e8ab`，不得沿用到收尾后的树。

## Goals / Non-Goals

**Goals:**

- 建立集成 client ABI v13 的同步身份，在 main v12 UI surface 上加入 MRW1 v1 更新入口，
  以及原子、可重建的 `RenderWorld` cache。
- 以 non-rewriting merge 跟进执行时最新 local main，原样继承其 engine ABI v9 与伙伴 fluid
  实现，并以受保护路径审计证明本 change 没有 feature-side engine/fluid 修改。
- 在 Rust 拷贝或规范化 Go 同步调用期间提供的 bytes；之后不保存 Go 指针、slice 或
  对象，且按 epoch、revision、tombstone 决定派生缓存状态。
- 用单元、FFI、Go bridge、fuzz 与 test-only driver 锁定 wire 校验、原子失败和离屏
  frame 字节不变。
- 在不扩大 MRW1 行为范围的前提下，确保最终代码注释满足任务编号纪律，并准确描述 Rust
  client 的生产 GPU 所有权、Go 的 CPU 半部以及 identity/versioned export 边界。

**Non-Goals:**

- 不创建 mesh worker、goroutine、thread、GPU pool、GPU 写入或任何 worker/GPU 调度。
- 不接入 `cmd/mornlea/app` 实时消息路径，不替换 Go CPU mesh、connectivity、visibility、
  `RenderFrame.Visible`、逐 section upload 或 draw。
- 不移动、复制或重构任何 engine 的 fluid-aware input、light、quad 或 greedy 源码；
  不接管、改写或扩展伙伴交付的流体状态、传播、tick、协议或专属 mesh 语义。
- 不创建共享 voxel kernel，不改网络协议 v32、存档 schema、world metadata、benchmark
  scenario v20；engine ABI v9 仅作为所选 main 的既有身份原样继承。
- 不把 selected main 继承的 25 处无关注释清理直接扫入 feature-side diff，不 push，不 archive，
  也不把 feature merge into main；local main 只允许 fast-forward 到独立评审通过的 cleanup。

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
该失败不得进入 FFI body、改变状态或转入兼容入口/Go fallback。最终 engine ABI 为 v9，
因为 follow-up sync 原样继承所选 main 的既有 engine/fluid surface；本 change 不修改该 ABI、
数值 kernel 或 fluid-aware 源码。Task 6.1 固定父仍为 engine ABI v8 的历史证据，不得被
解释成最终版本目标。

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

### D6: follow-up main sync 固定执行时父提交并保护伙伴路径

新的 Task 6.2 必须由 fresh implementer 在 clean worktree 中执行。它在 merge 命令前立即
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

### D7: 终审修复先清理 main，再重新固定集成基线

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
- [follow-up sync 覆盖伙伴 fluid/engine 或 main 在执行前再次漂移] → merge 前即时固定并记录
  latest local main；若新增提交改变契约/范围则先修订 planning。merge 后以 actual
  selected-main-parent 对五组受保护路径执行 zero-diff 审计，并由独立 reviewer 复核。
- [继承的代码注释违规被误归入 feature，或注释修复后继续沿用旧验证] → 先在独立 main-side
  cleanup 中清零并评审任务编号，再 non-rewriting sync；任何新 tracked HEAD 都重跑完整
  18 门禁，最终 fresh reviewer 复核零匹配与同基线证据。
- [紧凑缓存提高常驻内存] → 4 MiB 单 batch和 4096 record 上限，按 tombstone/reset
  丢弃旧派生状态；本阶段不建立无界后台队列。

## Migration Plan

1. 保留已完成的 feature 基线与 Task 6.1 证据；Task 6.1 的 actual main 父固定为
   `8b8891a3`、engine ABI v8，不改写该 merge 或其已通过的独立评审。
2. fresh implementer 在 follow-up merge 前即时读取 latest local main。若它超过已审计的
   `a23833f9`，先记录新增 commits/paths；契约或范围变化必须先更新 planning，否则选定该
   latest main，并以 `git merge --no-commit --no-ff main` 做 non-rewriting sync。
3. 重新读取 merged guides，逐项解决真实 overlap；以 actual selected-main-parent 对
   `mornlea_engine`、engine header、`internal/fluid`、`internal/nativeabi` 与
   `internal/sim/realm` 做 byte-for-byte zero-diff 审计。独立 reviewer 接受后，最终树继承
   main 的 engine ABI v9/fluid，本 change 仍不拥有这些生产路径。
4. 把 client C header、Rust、Go、当前文档、active config、main specs 与 tests 同步为
   client ABI v13 / engine ABI v9；验证 v13/v12 双向共有 export 拒绝、v12 缺失 v13-only
   MRW1 symbol 的 bind hard failure，并保留 UI JSON 与 MRW1 cache-only 契约。
5. 在同一 merged baseline 重建 release dylib，运行 release/race/visual/OpenSpec 全部门禁，
   并以离屏 renderer 验证 MRW1 应用前后 frame encoding/readback 不变；Go
   mesh/visibility/upload、production app 与 draw 继续使用原路径。
6. 在独立 main-side cleanup 中修复并评审继承的代码注释任务编号；可选 archcheck 必须先有
   RED。只把 local main fast-forward 到 reviewed cleanup，不 push、不接入 feature 实现。
7. fresh feature implementer 即时固定该 latest main，审计漂移并做 non-rewriting merge；记录
   新 selected-main-parent、双亲与冲突，证明五组 protected paths 和 visual golden 零 diff，
   再最小修复生产 GPU/Go CPU 职责注释与 identity/28 versioned exports 注释，并独立评审。
8. 在新的 immutable final HEAD 重跑 Task 6.4 的完整 18 门禁，由独立 reviewer 核验报告；
   `10f8e8ab` 的完整 PASS 只保留为历史，不计入新 HEAD 的完成证据。
9. 由未参与上述 planning、cleanup、sync/fixes 或 validation 的 fresh reviewer 做 whole-
   integration review；只有 spec/quality 均通过且 0 open findings 才宣告 implementation complete。
10. 发布时 client ABI v13 与动态库同步替换；main v12 混装显式失败。若集成发现问题，只
   回退 MRW1/client-v13 增量到所选 main 的 client v12 WKWebView predecessor；必须保留
   main 的 engine ABI v9、伙伴 fluid 实现及其测试稳定性修复，不得恢复旧 egui/TLV surface。
   没有网络、存档或世界数据迁移。

## Verification

Task 6.6 必须在 post-cleanup/post-sync/post-comment-fix 的同一个 immutable final HEAD 完整
重跑以下门禁并记录逐命令输出、exit status、真实 wall time、Skip 与基线身份；不得继承
`10f8e8ab` 的 Task 6.4 PASS：

- `make rust`
- `make rust-check`
- `gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go internal/client/window.go internal/client/window_test.go internal/server/sword_combat_parity_test.go`
- `go vet ./...`
- `go test ./internal/client -race -count=1`
- `go test ./internal/mesh -race -count=1`
- `go test ./internal/archcheck -count=1`
- `make test-race-changed`
- `go clean -testcache`
- `go test ./... -race -count=1`
- `make visual-check`
- `openspec validate --all --strict --no-interactive`
- `git diff --exit-code <selected-main-parent>..HEAD -- engine/crates/mornlea_engine engine/include/mornlea_engine.h internal/fluid internal/nativeabi internal/sim/realm`
- `git diff --exit-code <selected-main-parent>...HEAD -- cmd/mornlea/capture/testdata/golden`
- current/selected-main client symbols、identity、双向 version-mix、v13-only bind hard failure、
  no fallback、retired UI、no production MRW1、Go test-binary RPATH 与 release dylib SHA 审计
- 代码注释中的 `[A-F]-[0-9]{2}` 任务编号零匹配，以及 `internal/client/render.go`、
  `engine/crates/mornlea_client/src/ffi.rs` 两处修正的定点审计
- `git diff --check`
- exact HEAD、tracked/staged/untracked clean status 与全部证据同基线证明

集成实现与验证保持 cache-only：main predecessor client ABI 为 v12，统一目标为 v13，
最终 engine ABI 为从 selected-main-parent 原样继承的 v9；MRW1 固定布局、坐标裁决、边界与
阶段范围均由 binding brief 确定。生产 app 的 MRW1 接线、Go mesh/visibility/upload/draw、
共享 kernel、伙伴 fluid/engine 实现、协议、schema、benchmark scenario 与 visual golden
均不得产生 feature-side 改动。本节列出的新 final-HEAD 验证尚未执行。
