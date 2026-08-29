## Context

见 `proposal.md`。`mornlea_client_render_frame` 当前在每次调用中创建、编码并提交一个 command buffer；benchmark 在一个样本内连续调用 256 次，Go 的 `runtime.GC()` 不能等待或回收 Rust/wgpu 的在途提交。既有 `bounded-benchmark-workload` 规格要求固定批次录入同一 command buffer，并只测量一次提交到一次完成等待。

`mornlea_client` 是唯一的 GPU 所有者。Go 只能经 `internal/client` 的 client ABI bridge 请求离屏渲染，benchmark 已在无窗口路径运行。

根 `Makefile` 曾将所有隔离 worktree 指向同一 `CARGO_TARGET_DIR`。Cargo 的同名 crate fingerprint 可以把另一 worktree 更新更晚的旧输出视为可复用，令当前源码的 ABI export 与复制给 cgo 的 dylib 不一致。

## Goals / Non-Goals

**Goals:**

- 每个 v12 GPU 样本在 Rust 中保存一个已完成编码的固定批次，并在计时区间内恰好提交和等待一次。
- 让 renderer 显式拥有待提交 command buffer，拒绝重入准备，避免任何未界定的提交积压。
- 在 client ABI v11 中同步 Rust 导出、C header、Go bridge、版本门禁和长期基线文档。
- 让报告字段测试不依赖真实 GPU；保留真实 benchmark 测试来验证采样路径。
- 在 `NewHost` 期间收到 SIGTERM 时正常关闭已取得的资源，避免启动竞态令已请求关闭误报为启动失败。
- 保证未显式覆盖时，`make rust` 产出的 client dylib 只来自当前 worktree 源码。

**Non-Goals:**

- 不改变普通帧渲染、窗口 surface、capture、网络协议、存档或 benchmark scenario 版本。
- 不改变性能数值只记录的比较器语义，不添加超时重试、Go GPU fallback 或新的 wgpu 依赖。
- 不把通用多帧批处理暴露给游戏渲染热路径；入口仅服务固定离屏 benchmark。
- 不将普通启动错误、资源关闭错误或已进入 `Host.Run` 后的关服语义改写为成功。
- 不升级 Rust 工具链、Cargo lock 或依赖，不改变 `engine/target/release` 的 cgo dylib 消费路径。

## Decisions

### 两阶段 benchmark ABI

新增 `mornlea_client_render_prepare_benchmark_batch` 与
`mornlea_client_render_submit_benchmark_batch`，并将 client ABI 从 v10 升至 v11。

- prepare 接收一个已编码帧和 `1..=256` 的重复次数。`256` 与 scenario v12 的固定批次数一致；Rust 在离屏 renderer 内完成输入校验并把重复绘制录入一个 `wgpu::CommandBuffer`，但不提交。
- submit 仅在存在 prepared batch 时消费该 command buffer，调用一次 `queue.submit` 和一次 `device.poll(wgpu::PollType::wait_indefinitely)`，然后清除 renderer 的 prepared 状态。
- prepare 的参数为空、非法、次数超过 `256`、非离屏 renderer，或 renderer 已有 prepared batch 时，稳定返回既有错误状态且不覆盖先前 batch。submit 没有 prepared batch 时同样拒绝。FFI 测试以两个天空色不同的合法帧验证：重复 prepare 被拒绝后，submit/readback 仍呈现首个 batch，而不是第二个 batch。
- `OffscreenRenderer` 持有 `Option<wgpu::CommandBuffer>`；它不跨线程转移，继续受既有 renderer handle/mutex 边界保护。关闭 renderer 会连同未提交 batch 一起释放。
- prepared batch 存在期间，所有会改写其 command buffer 依赖 GPU 资源的 renderer 操作稳定拒绝且不写入，包括普通 frame、resize、section/atlas/LOD/HUD/glyph/UI font 的上传或删除及 LOD fog 设置；只允许 submit、只读查询和关闭。测试以不同天空色的 frame 在 prepare 后尝试 frame、resize 和上传，再经 submit/readback 证明首批保持不变。

现有 `render_frame` 保持一帧一次提交。它和 prepare 复用同一帧校验与 pass 编码路径，避免两套渲染语义漂移；窗口模式的 surface acquire/present 仍只属于普通 `render_frame`。

一次式 `render_frame_batch` 被否决：它会把 Rust command encoding 纳入 Go 的计时区间，违反既有采样边界。对每次 `render_frame` 后补充 poll 也被否决：虽然可降低积压，却会重新把固定轮询开销放大到每次绘制。

### Go benchmark 只测 submit 到 completion

`internal/client.Renderer` 增加与两个 ABI 入口一一对应的 benchmark 方法。`cmd/mornlea/benchmark` 在 `probe.now()` 前完成 `RenderFrame` 编码和 `PrepareBenchmarkBatch`；起始读钟后调用 submit-and-wait，结束读钟后将总时长按固定批次数摊薄。

Go 不再在采样循环中调用 `runtime.GC()` 作为 GPU 回收手段。每个样本的 GPU 生命周期由 Rust submit-and-wait 闭合；Go 仍可由正常运行时自行回收编码切片。

报告字段测试直接构造采样器并验证 `Summary().RemoteGPUCompleteBatch`，不装配 renderer。真实采样测试验证固定样本数、摊薄时钟、批次调用和一次准备/提交边界。

### ABI 与基线同步

同一任务同步更新 Rust `CLIENT_ABI_VERSION`、C header、Go 编译期与运行时版本断言及 ABI 测试。根 `AGENTS.md` 的 client ABI 基线更新为 v11，并更新引用当前版本的长期入口和进度记录；`internal/archcheck` 继续从 header 与根指南读取版本，不新增平行版本常量。

### 启动期间的 SIGTERM

`runSignal` 将 SIGTERM 映射为传入 `run` 的取消 context。`run` 已先后取得 world store 和 listener，再把该 context 传给 `NewHost`；因此 `world.meta` 已存在不代表 Host 已完成构造。若 `NewHost` 因该已取消 context 返回 `context.Canceled`，`run` 仍关闭 listener 和 store，并只返回关闭错误，将请求的 SIGTERM 视为正常退出。

该分支只要求 `ctx.Err() != nil` 且构造错误匹配 `context.Canceled`；未取消 context 下的构造错误仍保留原始上下文，关闭错误也不得被丢弃。单元测试锁定构造期间取消会成功释放资源，子进程 SIGTERM 回归测试重复运行以覆盖 metadata 与 Host 就绪之间的窗口。

### Worktree Rust target

默认 `CARGO_TARGET_DIR` 设为当前 worktree 的 `engine/target/cargo`，而 cgo 继续加载 `engine/target/release`。`make rust` 完成当前 worktree 构建后，将 engine 和 client dylib 从私有 cargo target 复制到既有发布路径；显式环境变量 `CARGO_TARGET_DIR` 保留为 CI 或开发者的覆盖入口。

不再共享同一源码指纹缓存会增加新 worktree 的首次 Rust 编译时间，但消除不同分支 client ABI 混装。回归检查从新 worktree 的默认 `make rust` 后检查两个 benchmark ABI 导出，并运行依赖它们的 Go race 测试。

## Risks / Trade-offs

- [单一 batch 录入的内存和编码工作增加] → 次数固定并受 ABI 上界约束；现有 pass budget 测试在实现前按实际 pass 数重算，超出预算时显式失败而非拆成隐式多提交。
- [`wait_indefinitely` 等待实际 GPU 完成] → 等待只发生在无头 benchmark 的明确测量边界，且 batch 已提交数量恒为一；普通渲染热路径不增加等待。
- [ABI v10/v11 动态库混装] → 全部 client ABI 入口保持版本检查；`make rust` 与 Go bridge 同批重建，现有 ABI 测试在启动前发现不匹配。
- [refactor 令普通帧与 batch 帧编码分叉] → 抽取共享校验和 command recording，Rust 单元测试覆盖普通帧与 prepared batch 的相同非法输入和状态转换。
- [SIGTERM 与其他构造错误并发] → 仅在 context 已取消且错误匹配 `context.Canceled` 时归并为正常关闭；资源关闭失败仍返回，未取消启动保持原始错误。
- [每 worktree 独立 target 增加冷编译] → 保留显式 `CARGO_TARGET_DIR` 覆盖；默认优先保证 ABI 正确，不依赖跨分支输出的新旧程度。

## Migration Plan

1. 先加入失败测试，锁定 v11 版本、prepare/submit 状态机、固定批次的单一 completion 等待及报告字段无 GPU 调用。
2. 同步实现 Rust renderer/FFI、header 与 Go bridge，再迁移 benchmark producer。
3. 运行 Rust crate、Go bridge/benchmark race、archcheck、OpenSpec 严格校验与阶段门禁；性能数值只记录，结构不完整或 ABI 错误仍失败。
4. 发布时将 Go binary 和 `libmornlea_client` 作为同一 release unit 重建。没有持久化或线上迁移。

回退只需回退该 change 并重新构建两侧；不得将 v10 Go binary 与 v11 cdylib 混用。
