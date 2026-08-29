# Rust 客户端渲染数据平面迁移设计

- 日期：2026-08-29
- 状态：设计已逐节确认，等待书面评审
- 范围：客户端区块 mesh、光照、连通性、可见性与 GPU 上传的数据平面迁移
- 排除：流体状态、传播、专属调度及其协议；它们由独立的 Rust Engine 流体迁移设计负责

## 背景与目标

当前客户端已经把 section mesh 与 light 的数值内核放入 Rust engine，但客户端的高频数据平面仍主要在 Go 中运行：

- internal/client.Mesher 在 Go 维护 dirty、排队、worker、generation 与结果回收；
- 每个任务由 cloneNeighborhood 深拷贝 3×3×3 邻域；
- internal/mesh 的 native 输入编码器把邻域展开为 MGM1 字节流，随后经 engine ABI 调用 Rust；
- Go 计算 section connectivity，并在每帧用 VisibleSectionsInto 做 BFS 与 frustum 筛选；
- internal/render.SectionScheduler 保留 quad、排序、打包并逐 section 调 client ABI 上传；
- Go 将可见 section 列表重新编码到 client frame ABI。

这使真正的 Rust mesh 内核前后仍有大量 Go 分配、复制、队列操作和 ABI 往返。一次 Apple M2 定点基线中，BenchmarkRemeshBoundaryEdit 为约 981 µs/op、452 KiB/op、5,607 allocs/op；这不是稳定门槛，而是迁移前的 record-only 证据。

目标是把客户端区块渲染数据平面完整收敛到 Rust，同时保持 Go 的协议校验、客户端逻辑镜像、预测和游戏规则边界。迁移后的生产帧路径不得再在 Go 中深拷贝 3×3×3 邻域、编码密集 MGM1 输入、构造 mesh quad、维护上传队列，或计算可见 section 集合。

## 已确认边界

### Go 保留的职责

- 网络协议解析、长度/内容校验和客户端 Mirror 的逻辑状态更新；
- 预测、交互、规则、相机和 HUD/entity 等渲染语义；
- 将已验证的 section 与 column 状态编码为一次性 render update；
- 帧预算配置，例如最大接收消息数、最大 mesh 工作量和最大上传字节数；
- 对 client ABI 错误的现有上层错误传播。

Go Mirror 仍是客户端逻辑状态的真相来源。它不把 Rust 缓存当作查询、预测或协议判定的来源。

### Rust 持有的职责

- 一个仅供渲染、可丢弃并可重建的 RenderWorld；
- section/column 缓存、revision、generation、dirty 去重、工作选择和结果回收；
- palette/bitpack 的一次性规范化、邻域快照、mesh、light、connectivity 与可见性遍历；
- GPU section pool 的上传、回收和重建；
- 绘制所需的可见 section 集合和帧统计。

Rust 不解析网络包、不持有权威世界、不决定游戏规则，也不回调 Go 逐格查询。

### 流体的明确排除

本设计不改变流体 block 的状态、传播、tick、重扫、队列、协议或专属网格策略。现有水材质若作为通用 voxel mesh 的既有输出出现，继续按现有 material 语义消费；本 change 不定义任何流体行为或优化契约。

## 目标架构

~~~text
网络消息
  -> Go 校验并更新 Mirror
  -> Go RenderUpdateBatch
  -> Rust client RenderWorld
       -> dirty / 邻域 / mesh / light / connectivity
       -> 有界结果队列
       -> GPU section pool
       -> visibility / draw
~~~

### 单一 Rust 算法实现

新增 workspace 内部 crate “mornlea_voxel_kernel”。它只包含无状态、无窗口、无 GPU 的 voxel 数值实现：mesh、light、connectivity 与其共享数据结构。

mornlea_engine 继续作为 engine ABI v8 的 C ABI 封装，并依赖该 crate；mornlea_client 直接在 Rust 进程内依赖同一 crate。算法源代码迁移而非复制，因此不存在第二份生产 mesh/light/connectivity 实现，也不需要让 Rust client 经 C ABI 再调用 Rust engine。

### RenderWorld

RenderWorld 是 mornlea_client 内的状态对象，随 renderer 创建与销毁。它以 section key 为索引保存：

- 规范化的连续 block-id 数据；
- height map、section/column revision 与 world epoch；
- generation、dirty 状态、connectivity 和最近一次 GPU slot；
- 供 worker 借用的不可变 section 数据。

Go 发送 palette/bitpack 存储而不是 4096 格的展开数组。Rust 在 update 到达时只解码一次，写入紧凑连续 section 数据；后续 mesh 直接访问 Rust 内存。为此，Go world 层只新增一个不泄漏内部指针的只读存储序列化视图，不把 Go slice 或对象长期交给 FFI。

## 更新 ABI 与状态机

client ABI 从 v11 升级到 v12。新增批量入口的语义为 “apply render updates”，所有现有 C header、Rust FFI、Go binding、动态库身份检查和跨语言版本测试必须同步更新。engine ABI 保持 v8；网络协议、存档 schema 和 benchmark scenario v20 不变。

更新数据使用独立于网络协议的版本化小端 envelope “MRC1”。它只表达已经由 Go 校验过的渲染状态，包含 world epoch 和下列记录：

1. section upsert：dimension、section position、单调 revision、palette/bitpack 存储；
2. column metadata upsert：height map 与其 revision；
3. unload/tombstone：section 或 column 的位置、epoch 与 revision；
4. world reset：重连、dimension 切换或全量同步时清空派生缓存。

Rust 在改变任何状态前校验整个 batch 的 magic、版本、长度、坐标范围、palette、bit 宽度、索引和整数溢出。任一记录非法时拒绝整个 batch，不保留部分更新。

同一 epoch 内，较新 revision 替换较旧 revision；较旧 upsert、已 tombstone section 的延迟 upsert，以及 worker 的旧 generation 结果都静默丢弃。一个 section 变化会让本体和受影响邻域重新 dirty。world reset 提升 epoch 并使所有旧任务不可提交。

Go 仅在 Mirror 成功更新后生成 batch。输入内存只在同步 ABI 调用期间有效；Rust 会复制或规范化为自己的缓存，绝不保存 Go 指针。

## 有界调度与并发

RenderWorld 的可变状态和 GPU 资源只由渲染线程拥有。mesh worker 数量在 renderer 创建时固定并显式配置；不为单个更新创建 goroutine 或 thread。

调度流程为：

1. update 应用后，RenderWorld 在有界 dirty 集合中合并重复 section；
2. 按现有相机中心和稳定位置全序选择工作，直到本帧 mesh 预算耗尽；
3. 调度时只克隆 3×3×3 个 Arc 引用及 revision/generation，不深拷贝 block 数据；
4. worker 使用其专属 scratch 调用共享 kernel，并将 packed geometry、connectivity 与 generation 放入有界完成队列；
5. 渲染线程在上传字节预算内回收结果，验证 generation，写入 GPU pool；
6. 结果过期、队列暂满或预算耗尽时保留最新 dirty 状态，后续帧继续收敛。

GPU 写入永远在渲染线程进行。worker 不触碰 wgpu 资源，也不持有 renderer 全局锁。mesh 结果不再变成 Go 的 []mesh.Quad，因此 Go 不再逐 section 调 UploadSection。

## 可见性与帧输入

RenderWorld 保存最新 connectivity；Rust client 在 render frame 内用相机 origin、frustum 和现有固定 radius 运行可见性 BFS。可见 section 列表直接驱动 GPU section slot 绘制，绝不回传 Go。

Go 仍传递相机、昼夜、实体、UI、文字与其他渲染语义输入；frame ABI v12 删除由 Go 编码的 Visible 区段载荷。renderer 从 RenderWorld 提取可见 section，完成 mesh 结果回收、上传、visibility 与 draw，再返回有界帧统计。

迁移完成时，下列 Go 生产职责被删除或收敛为 test-only oracle：

- internal/client.Mesher 与 cloneNeighborhood；
- internal/render.SectionScheduler；
- internal/mesh.ComputeConnectivity 的客户端生产调用；
- internal/mesh.VisibleSectionsInto 的客户端生产调用；
- app 中的 rustVisible 复制和逐 section geometry 上传。

## 失败、恢复与可观测性

| 情形 | Rust 行为 | Go 行为 |
|---|---|---|
| ABI、长度、palette 或 bitpack 非法 | 原子拒绝 batch，不 panic、不改 RenderWorld | 走现有 renderer 输入错误路径 |
| 旧 revision、tombstone 或旧 generation | 静默丢弃 | 不重试 |
| dirty/完成队列满 | 合并为最新 dirty，后续帧收敛 | 不再创建重复 mesh 任务 |
| GPU pool 暂不能上传 | 不绘制该 revision 的旧 geometry，保留 dirty 并重试 | 消费统计，不接收 geometry |
| device 重建 | 保留 CPU RenderWorld，受预算限制重建 GPU slot | 不重新发送全世界 |
| ABI 版本不匹配 | fail fast，禁止混装 v11/v12 | 不提供兼容或 Go fallback |

extern “C” 边界必须捕获 panic，禁止 unwind 越过 ABI。所有长度、指针、容量、对齐与重叠约束按现有 client ABI 纪律验证。Rust 的可观测输出只包含有限帧统计、错误码和需要诊断的计数，不输出无界 geometry 或缓存内容。

生产构建不保留 Go mesh/scheduler/visibility fallback。测试可以保留旧路径作为 oracle；发布前删除旧生产分支，确保错误不会被静默掩盖。

## 分阶段 OpenSpec 计划

这是一个迁移项目而不是一个单次 change。每一步独立 proposal、design、tasks、ledger 和评审，按顺序执行：

### 1. rust-render-world-cache

- 建立 mornlea_voxel_kernel 并让 mornlea_engine 保持 ABI v8 行为；
- 建立 client ABI v12、MRC1 编解码、RenderWorld、epoch/revision/tombstone 状态机；
- 用单元、FFI、fuzz 与 test-only driver 验证 cache 输入和重建；
- 不切换用户可见的 mesh 或 draw 路径。

### 2. rust-render-mesh-pipeline

- 建立固定 worker、Arc 邻域、Rust mesh/light/connectivity 和 GPU 上传队列；
- 将 Go Mesher 与 SectionScheduler 从生产路径移除；
- 迁移期间 Go 可以临时保留 connectivity/visibility oracle 以供现有 frame 输入工作，但不再传递 geometry；
- 以旧 Go 路径和现有 engine 入口进行逐位或结构化差分。

### 3. rust-render-visibility-frame

- 把 connectivity 消费、BFS/frustum 和 visible section 选择收敛到 RenderWorld；
- 收缩 frame ABI，移除 Go Visible、rustVisible 与客户端生产可见性路径；
- 删除已完成职责的 Go 代码，只保留明确标注的 test-only oracle；
- 完成视觉、capture、benchmark 与架构门禁。

每个 change 的执行遵守项目 OpenSpec 与 subagent-driven-development 流程：fresh implementer、独立规格评审与质量评审、ledger 记录进度与验证。不得把三个 change 合并成一个不可审查的大型实现。

## 验证策略

### 正确性

- Rust 单元与性质测试：palette/bitpack 解码、边界邻域、light、mesh、connectivity、revision、epoch、tombstone、overflow 和 atomic batch；
- ABI 测试：版本、magic、长度、指针、容量、重叠、错误码与 panic 隔离；
- 跨实现 oracle：相同 neighborhood 的 packed mesh、connectivity、可见集合与关键帧统计一致；
- 并发测试：更新替换、worker 旧结果、queue 饱和、device 重建和 renderer 销毁；
- 现有 Go race、archcheck、capture 和视觉 golden 继续运行。

### 性能与资源

- BenchmarkRemeshBoundaryEdit 以迁移前约 452 KiB/op、5,607 allocs/op 为记录基线；最终同口径场景的 Go 侧分配目标为至少下降 90%；
- benchmark scenario v20 记录帧时、队列深度、candidate section、上传字节、可见 section 和完整性状态；
- 性能数字按仓库规则仅记录，不作为环境敏感的退出阈值；真实 overflow、数据丢失、ABI 版本错误、报告不完整和 I/O 错误仍硬失败；
- 通过架构检查或定点测试证明生产客户端不再调用 cloneNeighborhood、MGM1 客户端编码、Go connectivity/visibility 或逐 section UploadSection 路径。

### 分层验证入口

每个 change 在编辑循环优先运行受影响 Rust/Go 单测与 ABI 测试；阶段边界再运行 make rust、目标 Go race、internal/archcheck、headless capture、benchmark scenario v20、OpenSpec strict 和相称的仓库门禁。干净 checkout 上涉及 Rust 的验证先执行 make rust，防止陈旧 dylib 造成假失败。

## 风险与取舍

- Rust 渲染缓存占用更多常驻内存：以加载/卸载生命周期回收，使用紧凑 block-id 表示，并记录 cache、worker、GPU pool 峰值；
- update 顺序与陈旧 worker 结果容易造成视觉错误：epoch、revision、generation 和 tombstone 全部进入状态机测试；
- GPU 资源失败不能显示旧 revision：宁可暂时隐藏 section，也不显示已知过时 geometry；
- ABI 升级容易出现混装：v12 身份检查、C header/Rust/Go 同步测试和 make rust 纪律共同约束；
- 共享 kernel 重构可能影响既有 engine 行为：engine ABI v8 的现有输出保留为 oracle，并在首个 change 中锁定；
- Rust 直接解析网络包会侵蚀语言边界：MRC1 只接受 Go 已验证的语义化存储，不复用网络 wire format。

## 被否决的方案

1. 让 mornlea_engine 维护状态化渲染缓存：这违反 engine 的无状态数值内核边界，也增加跨 ABI 生命周期复杂度。
2. 继续让 Go 生成每个 mesh 的 dense 3×3×3 快照：仍保留主要复制与编码成本，不能实现彻底迁移。
3. 让 Rust 成为客户端逻辑 Mirror 的唯一所有者：会使预测、交互与现有 Go 逻辑产生细粒度 FFI 查询，破坏当前职责边界。
4. 将 Go 网络包原样交给 Rust 解析：会让 Rust 绑定协议 wire 语义，削弱 Go 的协议校验与演进责任。

## 完成条件

- Rust RenderWorld 是客户端区块渲染数据平面的唯一生产所有者；
- Go 只发送已验证 render update 与帧语义，不传递邻域、quad 或 visible section；
- client ABI v12 的 header、Rust、Go、版本检查和测试一致，engine ABI 仍为 v8；
- mesh/light/connectivity/visibility 与现有视觉和功能语义一致，流体系统未被本项目修改；
- Go 生产帧路径消除 3×3×3 深拷贝、MGM1 客户端编码、Go visibility 与逐 section upload；
- 正确性、并发、ABI、capture、架构和 OpenSpec 门禁通过，性能证据完整记录；
- 三个 OpenSpec change 均经独立实现与评审完成后，旧 Go 生产路径被移除。
