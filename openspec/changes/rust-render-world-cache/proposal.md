## Why

当前客户端虽然已经把 section mesh 与 light 的数值计算交给 Rust engine，但 Go 仍在渲染热路径中维护区块 mesh、可见性和上传数据平面，并为每次重建深拷贝和展开邻域。迁移的第一个可回退闭环需要先建立一个只保存已验证紧凑区块状态的 Rust `RenderWorld` 和稳定更新边界，为后续迁移 mesh/light/connectivity 准备状态所有权，同时不改变现有绘制结果。

## What Changes

- 新增独立于网络协议的 MRW1 v1 小端 render update batch；Go 从 `world.ContainerSnapshot` 编码 single、indexed 或 direct 紧凑 section、column height、tombstone 与 world reset。indexed section 的 packed words 固定为 4-bit 256 个或 8-bit 512 个，并且 payload 必须恰好消费完毕。
- 在 `mornlea_client` 内新增派生、可丢弃并可重建的 `RenderWorld` cache，以原子 batch 校验和应用 epoch、revision 与 tombstone 状态；失败不保留部分更新，也不保存 Go 指针。
- **BREAKING**：planning 时 observed latest local main `a83192b7` 已把窗口完整合成捕获分配给 client ABI v13，并包含 `4c553f3b` 的 capture code fix；本 change 不再占用 v13，而以该 capture surface 为 predecessor，把最终统一 surface 升到 client ABI v14。selected-main v13 的 28 个 versioned exports（其中新增 capture export 恰为 `mornlea_client_window_capture`）及其 SDK-aligned CoreGraphics `u32` option bits、BGRA8 两段式容量、错误状态、Go bridge 与 Rust/Go tests 必须完整保留，只叠加 v14-only `mornlea_client_render_apply_world_updates` 与 `Renderer.ApplyRenderWorldUpdates`，形成 29 个 versioned exports；另有 1 个无参数 identity export，总导出数 30。无参数 `mornlea_client_abi_version()` 始终报告 14；全部 29 个接受 ABI version 参数的 v14 exports 均版本优先拒绝 13。新 MRW1 输入入口仍在改变缓存前按 ABI、长度、pointer/address、handle、MRW1 的顺序校验；最终树继承 main 的 engine ABI v9，且本 change 不修改 engine/fluid 生产路径。
- 新入口只由 Rust/Go ABI 测试和 test-only driver 驱动，不接入 `cmd/mornlea/app` 实时消息路径。现有 Go CPU mesh 调度、geometry 上传、connectivity、visibility、`RenderFrame.Visible`、frame 编码与 draw 输出保持不变。
- 不创建共享 voxel kernel、mesh worker 或 GPU pool，不移动或重构 engine 的 input/light/quad/greedy；main 已交付的 engine ABI v9 与 fluid eval/rescan 由伙伴负责，本 change 只原样继承，不设计、修改或扩展流体状态、传播、tick、协议或专属 mesh 语义。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `rust-client-render-cutover`：在 latest main 的 v13 WKWebView/UI + window composite capture surface 上增加 RenderWorld 原子更新，形成 client ABI v14 的统一版本一致性、capture 保留与本阶段 frame/draw 字节不变契约。

## Impact

- 受影响实现：`engine/crates/mornlea_client`、`engine/include/mornlea_client.h` 与 `internal/client`；最终身份同步还覆盖根版本说明、当前 docs、active config、main specs 与 identity tests。Tasks 1–6.6 的 completed 历史保持不可改写：旧 selected main `e1e2e287` 上的完整门禁曾通过；随后 exact `5e5243a7` revalidation 因 local main 漂移到 `bcc053e8` 在 Gate 18 得到 17/18 binding failure，feature 又只同步了 5 个无关 `dev-capture` planning files；下一轮在 `eccdca39` 上则因 local main 前进到 contract-affecting `be5ff22b` 而于 Gate 1 前取消，0/18 gates started。之后 `be5ff22b..4c553f3b` 又恰有 1 个同 surface 修复提交，只修改 `capture.rs`（38+/11-）：把两个 CoreGraphics option typedef/FFI 参数从 `u64` 校正为 SDK `u32`，把 IncludingWindow 与 BestResolution 位值分别从错误的 `1<<0`/`1<<8` 校正为 `1<<3`/`1<<3`，并加入 SDK 位值防回归测试；export/header/Go surface 与 28+1 计数均未变化。随后 `4c553f3b..a83192b7` 又恰有 1 个 docs-only commit，只修改 `dev-capture` 的 design/ledger/tasks（43+/4-），记录 Task 2 review/fix rounds、勾选其 2.1，并把 validation-order 文字校正为已实现的 ABI→出参 pointer/空容一致性→handle→capacity；生产/测试代码、v13 export/identity/header/Go surface 与 contract 均零变化。新的 pending 收尾必须先完成本轮 v14 planning 与独立 review，再由 fresh implementer 即时固定 latest main（当前 observed main 为 `a83192b7`）、做 non-rewriting sync、保留已修正的 main capture surface 并完成 combined v14，随后在 immutable final HEAD 重跑完整 18 门禁并接受 fresh whole-integration review。本结论不代表 archive、push 或 merge feature into main 授权。
- 所有权与并发：Go Mirror 继续是客户端逻辑真相来源；RenderWorld 仅由 renderer 持有派生渲染缓存。本 change 不新增 worker、goroutine、GPU 写入或生产热路径调用。
- 兼容性：集成后的 client ABI v14 与 selected-main v13 动态库不兼容并早期拒绝混装；final v14 动态库的 29 个 versioned exports 收到 13 时均先返回 `ABI_VERSION`，selected-main v13 动态库的 28 个共有 versioned exports 收到 14 时同样先拒绝，而 v14-only MRW1 symbol 在 v13 dylib 上于 link/load/bind 阶段硬失败。不得增加动态加载兼容层或 Go fallback。main v13 的 capture（含 `4c553f3b` 的 SDK-aligned option bits/test）、WKWebView/JSON UI 与退役 UI 状态必须完整保留。最终 engine ABI 为从 main 原样继承的 v9；回退 MRW1/client-v14 增量时只能回到 selected-main v13 capture predecessor，仍保留 main 的 engine ABI v9、伙伴 fluid 与 capture，不得回到 v12。
- 协议与存档：协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` v4、`hostile_mobs` v1 与 benchmark scenario v20 均不变；MRW1 不复用已分配给 raycast cursor 的 `MRC1`，也不解析网络 wire packet。
- 性能与用户可观察结果：cache 使用紧凑 palette/bitpack 表示并限制 batch 总长 4 MiB、record 数 4096；由于不接入实时 app，也不切换 mesh/upload/draw/Visible，现有离屏 frame 字节、截图和用户可观察绘制结果 MUST 保持不变。
