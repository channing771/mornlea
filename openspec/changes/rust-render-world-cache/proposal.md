## Why

当前客户端虽然已经把 section mesh 与 light 的数值计算交给 Rust engine，但 Go 仍在渲染热路径中维护区块 mesh、可见性和上传数据平面，并为每次重建深拷贝和展开邻域。迁移的第一个可回退闭环需要先建立一个只保存已验证紧凑区块状态的 Rust `RenderWorld` 和稳定更新边界，为后续迁移 mesh/light/connectivity 准备状态所有权，同时不改变现有绘制结果。

## What Changes

- 新增独立于网络协议的 MRW1 v1 小端 render update batch；Go 从 `world.ContainerSnapshot` 编码 single、indexed 或 direct 紧凑 section、column height、tombstone 与 world reset。indexed section 的 packed words 固定为 4-bit 256 个或 8-bit 512 个，并且 payload 必须恰好消费完毕。
- 在 `mornlea_client` 内新增派生、可丢弃并可重建的 `RenderWorld` cache，以原子 batch 校验和应用 epoch、revision 与 tombstone 状态；失败不保留部分更新，也不保存 Go 指针。
- **BREAKING**：以 main 已用于进程内 WKWebView/UI cutover 的 client ABI v12 为前代基线，把合并后的 client ABI 升到 v13，并新增 `mornlea_client_render_apply_world_updates` 与 `Renderer.ApplyRenderWorldUpdates`。v13 必须保留 main 的 `ui_push_state` 与版本化 JSON UI events，已经退役的 `render_upload_ui_font`、frame TLV tag 9 及 UI layout v1–v4 不得复活。全部 client ABI export 均版本优先拒绝混装；新输入入口在改变缓存前按 ABI、长度、pointer/address、handle、MRW1 的顺序校验；engine ABI 保持 v8。
- 新入口只由 Rust/Go ABI 测试和 test-only driver 驱动，不接入 `cmd/mornlea/app` 实时消息路径。现有 Go CPU mesh 调度、geometry 上传、connectivity、visibility、`RenderFrame.Visible`、frame 编码与 draw 输出保持不变。
- 不创建共享 voxel kernel、mesh worker 或 GPU pool，不移动或重构 engine 的 input/light/quad/greedy，不改变流体状态、传播、tick、协议或专属 mesh 语义；这些工作属于后续 change。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `rust-client-render-cutover`：在 main 的 v12 WKWebView/UI surface 上增加 RenderWorld 原子更新，形成 client ABI v13 的统一版本一致性与本阶段 frame/draw 字节不变契约。

## Impact

- 受影响实现：`engine/crates/mornlea_client`、`engine/include/mornlea_client.h` 与 `internal/client`。原 cache-only 实现与整分支终审已在独立的 feature 基线上完成，但 main 已另行把 client ABI v12 分配给 WKWebView/UI surface；change 现处于集成待办状态，必须先合入最新 main、把统一 surface 升到 v13、重新同步当前文档/规格并完成合并基线验证与独立评审，才能再次视为可交付。本结论不代表 archive 或 merge into main 授权。
- 所有权与并发：Go Mirror 继续是客户端逻辑真相来源；RenderWorld 仅由 renderer 持有派生渲染缓存。本 change 不新增 worker、goroutine、GPU 写入或生产热路径调用。
- 兼容性：集成后的 client ABI v13 与 main 的 v12 动态库不兼容并早期拒绝混装；不提供 v12 兼容入口或 Go fallback。main v12 已退役的 UI exports/TLV payload 保持退役，当前 JSON UI surface 保持可用。engine ABI v8 不变。
- 协议与存档：协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` v4、`hostile_mobs` v1 与 benchmark scenario v20 均不变；MRW1 不复用已分配给 raycast cursor 的 `MRC1`，也不解析网络 wire packet。
- 性能与用户可观察结果：cache 使用紧凑 palette/bitpack 表示并限制 batch 总长 4 MiB、record 数 4096；由于不接入实时 app，也不切换 mesh/upload/draw/Visible，现有离屏 frame 字节、截图和用户可观察绘制结果 MUST 保持不变。
