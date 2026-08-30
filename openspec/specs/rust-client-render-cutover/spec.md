# rust-client-render-cutover Specification

## Purpose
把客户端生产渲染切换到 Rust 渲染器(窗口 surface 与无头离屏两条路径)并删除 Go 渲染栈的 GPU 半部,以 golden 字节不变锁定切换的行为零变化;Rust 成为呈现的唯一生产实现。
## Requirements
### Requirement: 生产渲染由 Rust 渲染器独占

darwin 客户端的窗口呈现与无头 capture MUST 由 `mornlea_client` 渲染器独占
生产;Go 生产路径 MUST 不包含 GPU pipeline/pass 实现与 WebGPU 绑定依赖,
`go.mod` MUST 不含 `oliverbestmann/webgpu`。CPU 准备(mesh 调度、可见性、
字形光栅化、布局、实体编码)保留在 Go。

#### Scenario: 客户端一帧经单次渲染 FFI 呈现

- GIVEN 运行中的 darwin 客户端
- WHEN 主循环渲染一帧
- THEN 全部 GPU pass 由一次 render FFI 调用执行并呈现到窗口 surface,
  帧内无逐 pass 或逐资源渲染调用

#### Scenario: 生产代码不再引用 WebGPU 绑定

- GIVEN 切换完成后的仓库
- WHEN 检查 Go module 依赖与生产源码
- THEN 不存在 `internal/gfx` 包与对 `oliverbestmann/webgpu` 的引用

### Requirement: golden 与既有视觉行为字节不变

切换后的无头 capture 输出 MUST 原样通过既有 golden 比对与
`diffThreshold` 门禁;golden 基线文件、阈值与场景 MUST NOT 修改。

#### Scenario: 既有 golden 零改动通过

- GIVEN 既有全部 capture 场景与 golden 基线
- WHEN 以 Rust 渲染器执行无头抓帧并比对
- THEN 全部场景在既有阈值内通过,golden 文件未修改

### Requirement: 窗口 surface 呈现语义

窗口模式渲染 MUST 在帧内获取 surface 纹理并于提交后呈现;窗口被遮挡、
最小化或 surface 过期时 MUST 跳过该帧且不报错;窗口尺寸变化后 MUST 重建
渲染目标与 HiZ 并以新尺寸继续渲染。

#### Scenario: 遮挡帧跳过不中断

- GIVEN 窗口模式渲染器
- WHEN surface 纹理获取失败(遮挡/过期)
- THEN 该帧被跳过,后续帧恢复正常呈现,进程不退出

#### Scenario: resize 后继续正确渲染

- GIVEN 窗口模式渲染器
- WHEN 调用 resize 并渲染后续帧
- THEN 渲染目标与 HiZ 以新尺寸重建,呈现无花屏或尺寸错位

### Requirement: 字形与 HUD 图集经 sink 独占供给

Go 字形光栅化与 HUD 图集构建 MUST 只经 client 渲染器上传入口供给 GPU;
迁移后 MUST 不存在第二份图集纹理路径。字形队列、tofu 兜底与上限语义
MUST 与迁移前一致。

#### Scenario: 字形上传语义保持

- GIVEN 聊天与名牌产生新字形请求
- WHEN 光栅化 worker 完成并按帧预算冲刷
- THEN 字形经 client 上传入口进入唯一图集,文本呈现与迁移前一致

### Requirement: RenderWorld 只接受已验证的原子更新批

客户端 SHALL 只接受 magic 为 `MRW1`、layout version 为 1、两个 batch reserved 字段均为零且 epoch 非零的小端 render update batch。batch header MUST 恰为 24 字节，每条 record header MUST 恰为 32 字节，batch 总长度 MUST 不超过 4 MiB，record count MUST 在 1..=4096；record tag MUST 分别只表示 section upsert(1)、column upsert(2)、section tombstone(3)、column tombstone(4) 或 world reset(5)，其 record reserved 字节 MUST 为零。section record 的 dimension、X、Z MUST 接受完整 signed `i32` 域，Y MUST 在 `0..core.SectionsPerChunk`；column record 的 Y、storage kind、bits MUST 均为零。任一 record 的长度、保留字节、坐标、palette、bitpack、epoch、revision 或 payload 违反契约时，客户端 MUST 返回 `INVALID_ARGUMENT`、拒绝整个 batch 且保持 RenderWorld 调用前状态。`MRC1` MUST NOT 用作 render update magic。

section upsert MUST 按 `ContainerSnapshot` 三态接收紧凑数据：single 对应 storage kind 0，bits、palette 与 packed words 均为零；indexed 对应 storage kind 1、bits 只允许 4 或 8 且每个 packed slot 小于 palette count，bits 为 4 时 packed word count MUST 恰为 256，bits 为 8 时 MUST 恰为 512；direct 对应 storage kind 2、bits 为 15、palette count 为零、包含 1024 个 packed words 且每个 word 高四位为零。各 section payload MUST 按其声明的字段恰好消费完毕，不得接受尾随 bytes。column upsert MUST 恰含 256 个小端 `i16` height；tombstone MUST 无 payload。world reset 只允许为 batch 第一条，且其坐标、revision、storage 元数据与 payload MUST 均为零。

#### Scenario: 非法 indexed section 不产生部分缓存

- GIVEN 已含 revision 7 section 的 RenderWorld
- WHEN 收到同一 batch 中先有合法 revision 8、后有 palette slot 越界的 section record
- THEN 调用返回 INVALID_ARGUMENT
- AND 原 revision 7 数据仍是唯一可见缓存状态

#### Scenario: MRW1 容量边界被严格执行

- GIVEN 一个总长超过 4 MiB 或 record count 超过 4096 的 MRW1 v1 batch
- WHEN 客户端提交该 batch
- THEN 调用返回 INVALID_ARGUMENT
- AND RenderWorld 保持调用前状态

#### Scenario: 三种紧凑 section 存储均可往返

- GIVEN 分别来自 single、4-bit 或 8-bit indexed、15-bit direct `ContainerSnapshot` 的合法 section update
- WHEN Go 编码后由 client ABI 应用到 RenderWorld
- THEN cache 保留等价的紧凑 palette/bitpack 状态
- AND 不展开为 4096 个 block ID

#### Scenario: indexed packed words 的边界或尾随 bytes 原子失败

- GIVEN 已含 revision 7 section 的 RenderWorld
- WHEN 收到 bits 为 4 但 packed word count 非 256、bits 为 8 但非 512，或 payload 留有尾随 bytes 的 indexed section batch
- THEN 调用返回 INVALID_ARGUMENT
- AND revision 7 仍是唯一可见缓存状态

### Requirement: RenderWorld 以 reset、epoch、revision 与 tombstone 决定状态

首次写入和重连恢复 MUST 先以 batch 第一条 world reset 建立 epoch 1；后续 world reset MUST 仍为 batch 第一条且 epoch 严格大于当前 epoch，并 MUST 原子清空旧 section 与 column 状态。非 reset record 的 batch epoch MUST 等于当前 epoch；同一 key 只有 revision 更大时 SHALL 替换已有状态，revision 相等或更小 SHALL 幂等忽略。section 与 column tombstone MUST 保留 revision，且只有 revision 更大的 upsert 可以恢复对应 key。

#### Scenario: 非首条 reset 原子失败

- GIVEN 已建立 epoch 1 且含有效 section 的 RenderWorld
- WHEN 一个 batch 在普通 update 之后包含 world reset
- THEN 调用返回 INVALID_ARGUMENT
- AND epoch 与全部缓存状态保持不变

#### Scenario: 新 epoch 清除旧状态

- GIVEN epoch 1 中含 section、column 与 tombstone 的 RenderWorld
- WHEN 收到以 world reset 为首条且 epoch 为 2 的合法 batch
- THEN epoch 1 的 section、column 与 tombstone 全部清除
- AND batch 中 reset 后的合法 epoch 2 records 原子生效

#### Scenario: 陈旧 upsert 不能复活 tombstone

- GIVEN epoch 1 中某 section 已由 revision 11 tombstone
- WHEN 收到同一 key 的 revision 10 或 11 upsert
- THEN update 被幂等忽略
- AND 该 section 仍为 tombstone

#### Scenario: 更大 revision 可恢复 tombstone

- GIVEN epoch 1 中某 section 已由 revision 11 tombstone
- WHEN 收到同一 key 的 revision 12 合法 upsert
- THEN revision 12 的紧凑 section 状态替换 tombstone

### Requirement: client ABI v12 混装早期拒绝

client C header、Rust 导出与 Go bridge 的 ABI 版本常数 MUST 同步为 12，新增的 render world update 入口及全部既有 client ABI 入口 MUST 拒绝其他版本。每个 export MUST 在其其他适用 validation 前检查 ABI version；现有 all-export ABI checks MUST 保留。版本错误 MUST 在 handle、pointer、batch 内容或 RenderWorld 状态改变前返回 `ABI_VERSION`；系统 MUST NOT 提供 v11 兼容入口或 Go fallback。engine ABI MUST 保持 v8。

输入型 `mornlea_client_render_apply_world_updates` MUST 按以下顺序在改变 RenderWorld 前验证：ABI version、非零且不超过 MRW1 上限的 length、非空 pointer、address range 不溢出、已有 renderer handle、MRW1 layout 与容量。该入口没有输出 buffer，MUST NOT 要求 output capacity 或 overlap 检查；这些检查只适用于拥有输出的 export。任何 client ABI export MUST NOT 让 panic 穿过 FFI；panic MUST 映射为 `PANIC` 且不得留下部分 RenderWorld 状态。

#### Scenario: v11 动态库不能与 v12 Go bridge 混用

- GIVEN client ABI v11 的动态库
- WHEN v12 Go bridge 创建 renderer 或提交 render update
- THEN 调用 MUST 在任何 RenderWorld 状态改变前返回 ABI_VERSION

#### Scenario: 错误 ABI 优先于输入内容检查

- GIVEN ABI version 不为 12 且 handle、pointer 或 MRW1 bytes 也无效的调用
- WHEN 调用任一 client ABI 入口
- THEN 调用返回 ABI_VERSION
- AND 不读取无效输入或改变 renderer 状态

#### Scenario: 新输入入口按 ABI 优先的输入矩阵拒绝

- GIVEN 一个错误 ABI，或 ABI 正确但依次存在零/超限 length、null pointer、address overflow、未知 handle 或非法 MRW1 的调用
- WHEN 调用 render world update 入口
- THEN 错误 ABI 返回 ABI_VERSION，其他情况按所列顺序在状态改变前返回对应错误
- AND 输入型入口不执行 output capacity 或 overlap 检查

#### Scenario: panic 不跨越 client ABI

- GIVEN 任一 client ABI export 在其 Rust 实现内发生 panic
- WHEN export 返回给调用方
- THEN 调用返回 PANIC
- AND RenderWorld 保持调用前状态

### Requirement: 本 change 不改变 draw 或 frame 可观察结果

RenderWorld update 入口 MUST 只更新尚未接管绘制的派生 cache，MUST NOT 接入实时 app 消息路径，MUST NOT 改变既有 `RenderFrame.Visible` 载荷、frame ABI 字节布局、Go CPU mesh/connectivity/visibility、逐 section upload 或 Rust draw 选择。应用合法 MRW1 batch 前后的现有离屏 frame 编码与输出 MUST 保持字节不变。

#### Scenario: 新缓存入口不改变现有 frame 编码

- GIVEN 相同 RenderFrame 输入和空 RenderWorld
- WHEN 分别在应用一个合法但未接管绘制的 MRW1 batch 前后编码 frame
- THEN EncodeRenderFrame 输出 MUST 逐字节一致

#### Scenario: 缓存更新不产生 frame 或上传调用

- GIVEN 一个已创建的离屏 renderer
- WHEN 只应用合法的 world reset 与 section update 而不调用 RenderFrame
- THEN frame call 与 section upload 统计保持不变
- AND 后续相同 frame 的 draw 输出与未应用该 batch 时逐字节一致
