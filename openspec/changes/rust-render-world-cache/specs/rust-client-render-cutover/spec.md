## ADDED Requirements

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

### Requirement: client ABI v13 统一 surface 并早期拒绝混装

client C header、Rust 导出与 Go bridge 的 ABI 版本常数 MUST 同步为 13；无参数 identity export `mornlea_client_abi_version()` MUST 始终报告 13。集成 v13 动态库中每个接受 ABI version 参数的 export MUST 拒绝包括 12 在内的每一个其他版本。每个此类 v13 export MUST 在其其他适用 validation 或状态改变前检查 ABI version；现有 all-versioned-export ABI checks MUST 保留。版本错误 MUST 在 handle、pointer、UI/MRW1 内容或 renderer/RenderWorld 状态改变前返回 `ABI_VERSION`。v13 surface MUST 保留 main v12 的 `ui_push_state` 与版本化 JSON UI event drain，MUST NOT 恢复已退役的 `render_upload_ui_font`、frame TLV tag 9 或 UI layout v1–v4；系统 MUST NOT 提供 v12 兼容入口或 Go fallback。engine ABI MUST 保持 v8。

反向混装时，main v12 动态库与 v13 bridge 共有且接受 ABI version 的 exports MUST 在收到 13 时返回 `ABI_VERSION`，并且不得读取其他输入或改变状态。v13-only 的 `mornlea_client_render_apply_world_updates` symbol 在 main v12 动态库中不存在；尝试把要求该 symbol 的 v13 bridge 与 main v12 动态库组合 MUST 在 link、load 或 bind 阶段硬失败，MUST NOT 被描述为一次会返回 client status 的调用，MUST NOT 进入任何 FFI body、改变任何状态或使用兼容入口/fallback。

输入型 `mornlea_client_render_apply_world_updates` MUST 按以下顺序在改变 RenderWorld 前验证：ABI version、非零且不超过 MRW1 上限的 length、非空 pointer、address range 不溢出、已有 renderer handle、MRW1 layout 与容量。该入口没有输出 buffer，MUST NOT 要求 output capacity 或 overlap 检查；这些检查只适用于拥有输出的 export。任何 client ABI export MUST NOT 让 panic 穿过 FFI；panic MUST 映射为 `PANIC` 且不得留下部分 RenderWorld 状态。

#### Scenario: v13 动态库拒绝 ABI v12 调用方

- GIVEN 集成 client ABI v13 的动态库
- WHEN 调用方对每个接受 ABI version 的 export 传入 12
- THEN 每个调用 MUST 在其他 validation 或状态改变前返回 ABI_VERSION

#### Scenario: main v12 动态库的共有 exports 拒绝 ABI v13

- GIVEN main client ABI v12 的 WKWebView/UI 动态库与 v13 bridge
- WHEN v13 bridge 调用双方共有的 window、renderer 或 UI export 并传入 13
- THEN 该 v12 export MUST 在读取其他输入或改变 renderer、UI 状态前返回 ABI_VERSION

#### Scenario: main v12 动态库缺少 v13-only MRW1 symbol

- GIVEN main client ABI v12 动态库不导出 `mornlea_client_render_apply_world_updates`
- WHEN 尝试 link、load 或 bind 要求该 symbol 的 v13 bridge
- THEN 组合 MUST 在进入 FFI 调用前硬失败
- AND 系统 MUST NOT 声称返回 client status、改变 RenderWorld 或使用 fallback

#### Scenario: 错误 ABI 优先于输入内容检查

- GIVEN ABI version 不为 13 且 handle、pointer、UI JSON 或 MRW1 bytes 也无效的调用
- WHEN 调用任一接受 ABI version 参数的 client export
- THEN 调用返回 ABI_VERSION
- AND 不读取无效输入或改变 renderer 状态
- AND 无参数 `mornlea_client_abi_version()` 不接收该错误版本并始终报告 13

#### Scenario: v13 保留 main UI surface 且不复活旧 TLV

- GIVEN main v12 已提供 `ui_push_state` 与版本化 JSON UI events，并已退役字体上传出口和 frame TLV tag 9
- WHEN 合并后的 client ABI v13 surface 加入 MRW1 update 入口
- THEN `ui_push_state` 与版本化 JSON UI events MUST 保持可用
- AND `render_upload_ui_font`、frame TLV tag 9 与 UI layout v1–v4 MUST 保持不可用

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
