## Why

当前客户端虽然已经把 section mesh 与 light 的数值计算交给 Rust engine，但 Go 仍在渲染热路径中维护区块 mesh、可见性和上传数据平面，并为每次重建深拷贝和展开邻域。迁移的第一个可回退闭环需要先建立一个只保存已验证紧凑区块状态的 Rust `RenderWorld` 和稳定更新边界，为后续迁移 mesh/light/connectivity 准备状态所有权，同时不改变现有绘制结果。

## What Changes

- 新增独立于网络协议的 MRW1 v1 小端 render update batch；Go 从 `world.ContainerSnapshot` 编码 single、indexed 或 direct 紧凑 section、column height、tombstone 与 world reset。indexed section 的 packed words 固定为 4-bit 256 个或 8-bit 512 个，并且 payload 必须恰好消费完毕。
- 在 `mornlea_client` 内新增派生、可丢弃并可重建的 `RenderWorld` cache，以原子 batch 校验和应用 epoch、revision 与 tombstone 状态；失败不保留部分更新，也不保存 Go 指针。
- **BREAKING**：client ABI 从 v11 升到 v12，新增 `mornlea_client_render_apply_world_updates` 与 `Renderer.ApplyRenderWorldUpdates`。全部 client ABI export 均版本优先拒绝混装；新输入入口在改变缓存前按 ABI、长度、pointer/address、handle、MRW1 的顺序校验；engine ABI 保持 v8。
- 新入口只由 Rust/Go ABI 测试和 test-only driver 驱动，不接入 `cmd/mornlea/app` 实时消息路径。现有 Go CPU mesh 调度、geometry 上传、connectivity、visibility、`RenderFrame.Visible`、frame 编码与 draw 输出保持不变。
- 不创建共享 voxel kernel、mesh worker 或 GPU pool，不移动或重构 engine 的 input/light/quad/greedy，不改变流体状态、传播、tick、协议或专属 mesh 语义；这些工作属于后续 change。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `rust-client-render-cutover`：增加 RenderWorld 原子更新、client ABI v12 版本一致性与本阶段 frame/draw 字节不变契约。

## Impact

- 受影响实现：`engine/crates/mornlea_client`、`engine/include/mornlea_client.h` 与 `internal/client`；Task 实现与独立终审均已完成，whole-branch final review findings 的 fix wave 已实现；final Rust baseline 的 release/ABI/visual 验证已通过，随后只有 independently reviewed server parity test 变更，final HEAD 的 changed-race 与清缓存 full race 均通过。client ABI v12 当前事实已同步到 `rust-client-render-cutover` 主规格与 active OpenSpec context；change 保持 active，等待同一 final reviewer scoped re-review，不在本轮 archive。
- 所有权与并发：Go Mirror 继续是客户端逻辑真相来源；RenderWorld 仅由 renderer 持有派生渲染缓存。本 change 不新增 worker、goroutine、GPU 写入或生产热路径调用。
- 兼容性：client ABI v11/v12 不兼容并早期拒绝混装；不提供兼容入口或 Go fallback。engine ABI v8 不变。
- 协议与存档：协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` v4、`hostile_mobs` v1 与 benchmark scenario v20 均不变；MRW1 不复用已分配给 raycast cursor 的 `MRC1`，也不解析网络 wire packet。
- 性能与用户可观察结果：cache 使用紧凑 palette/bitpack 表示并限制 batch 总长 4 MiB、record 数 4096；由于不接入实时 app，也不切换 mesh/upload/draw/Visible，现有离屏 frame 字节、截图和用户可观察绘制结果 MUST 保持不变。
