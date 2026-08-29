## Why

scenario v12 的 GPU 完成探针当前把每个样本拆成 256 次 `render_frame` 提交，且未建立完成等待边界。连续样本会积压 Metal command buffer，最终阻塞在 cgo 渲染入口，令 benchmark 测试超时并违反既有的批量采样契约。

阶段门禁还暴露了一个既有的服务端启动竞态：`world.meta` 创建后、`NewHost` 完成前收到 SIGTERM 会取消 hostile loading，使已请求关闭的子进程以错误状态退出。

隔离 worktree 的共享 Cargo target 还会复用另一个 worktree 的同名 crate 产物；默认 `make rust` 因而可能复制缺少当前 client ABI 符号的旧 dylib，使 Go 链接门禁加载错误产物。

## What Changes

- client ABI 升至 v11，增加 benchmark 专用的批次准备和单次提交完成等待入口；普通 `render_frame` 语义不变。
- Rust renderer 在计时前将固定批次录入一个 command buffer，计时区间只执行一次 submit 和一次完成等待；待提交批次不可重入，提交后立即释放其所有权。
- Go benchmark 探针改用该两阶段 ABI，并移除将 Go GC 视作 GPU 回收边界的做法。
- 报告批次数字段的测试改为纯报告断言，不再因验证常量接线而执行真实 GPU 工作。
- 当 SIGTERM 在 `NewHost` 期间取消启动时，服务端仍关闭已打开的 listener 和 world store，并将该已请求关闭作为正常退出；其他启动失败保持错误语义。
- 默认 Rust target 改为 worktree 内的私有目录；`make rust` 只从该次构建复制 dylib，显式设置 `CARGO_TARGET_DIR` 仍可覆盖默认值。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

无。`bounded-benchmark-workload` 已规定同一 command buffer 的批量提交和一次完成等待；本变更只恢复该既有契约的实现。

## Impact

- 影响 `engine/crates/mornlea_client`、`engine/include/mornlea_client.h`、`internal/client`、`cmd/mornlea/benchmark`、`cmd/mornlea-server` 和根 `Makefile`。
- client ABI v10 与 v11 不可混装，必须同步重建 Rust cdylib 与 Go 消费端；不影响 engine ABI、网络协议、存档或世界数据。
- GPU 探针仍在无头离屏路径运行，性能数值继续只记录；固定批次的 CPU 编码和一次 GPU 完成等待取代未界定的提交积压。
- 服务端修复不改变网络协议、存档或正常启动失败的退出行为。
- worktree 首次 Rust 构建不再跨分支复用 crate 输出；不改变工具链、依赖版本或发布 dylib 路径。
