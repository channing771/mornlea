# rust-engine-mesh Specification

## Purpose
在保持现有游戏、协议、存档、视觉和资源上限不漂移的前提下，以可验证且可回退的 Rust 生产边界接管确定性区段网格和传播光照。
## Requirements
### Requirement: Rust mesh 保持现有输出契约
系统 MUST 让 Rust 生产实现对同一 neighborhood 与 registry 产生与冻结 Go oracle 数量、顺序和 packed bits 完全相同的 quads。

#### Scenario: 固定与随机输入逐位一致
- **GIVEN** 现有夹具和固定种子随机 neighborhood
- **WHEN** Go oracle 与 Rust 生产实现分别网格化
- **THEN** packed `uint64` 序列 MUST 完全一致

#### Scenario: 继承的视觉基线失败不得掩盖迁移漂移
- **GIVEN** 同一验收设备上的冻结 pre-M4P 提交与 M4P HEAD 使用同一 non-update visual-check 命令
- **WHEN** 两者因同一既有 golden 偏差返回非零
- **THEN** 全部 10 个场景 capture PNG MUST 逐字节一致
- **AND** 每个失败场景的摘要、actual PNG 与 diff PNG MUST 逐字节一致
- **AND** M4P MUST 不修改 golden、阈值或场景集合
- **AND** 任一新增失败、摘要差异或字节差异 MUST 阻断验收

### Requirement: native 边界具有单一所有权
系统 MUST 由 Go 持有 input、scratch 与 output，并禁止任一语言在调用结束后保留对方指针。

#### Scenario: 多 worker 并发
- **GIVEN** 每个 worker 拥有独立 scratch
- **WHEN** 并发网格化不同区段
- **THEN** 结果 MUST 确定且不得共享可变 native 状态

### Requirement: ABI 失败不得产生部分网格
系统 MUST 通过 `mornlea_engine.h` 中的 `MORNLEA_ENGINE_*`、`MORNLEA_STATUS_*`、`mornlea_engine_abi_version` 与 `mornlea_mesh_section` 提供唯一 native ABI，并对版本、结构长度、非空区段使用的 registry、emission、output overflow 与 Rust panic 返回可判定失败。该边界最初以 ABI version `1` 建立；当前统一 engine ABI version MUST 为 `9`，status 数值 MUST 保持 `0..9`。

#### Scenario: 非法输入被原子拒绝
- **WHEN** native 调用收到任一非法输入
- **THEN** output length MUST 为 0
- **AND** panic/unwind MUST NOT 穿过 C ABI

#### Scenario: 全空气区段不读取 registry 语义
- **GIVEN** native 输入的 magic、长度、bounded count、visibility 行宽、presence 位与借用范围结构合法
- **AND** center section 的所有方块都等于 header 声明的 AirID
- **WHEN** registry 排序、air/barrier identity、opacity、emission 或 required-ID 语义本会校验失败
- **THEN** native 调用 MUST 在 registry 语义与传播光照之前返回成功
- **AND** output length MUST 为 0
- **AND** light scratch MUST 保持不变

#### Scenario: 旧 native 身份不再导出
- **WHEN** 检查发布的 header 与 dylib symbols
- **THEN** MUST 只存在 Mornlea C ABI 身份且 MUST 不存在旧 `mcgo` C symbol

### Requirement: clean checkout 使用 Rust-first 构建
系统 MUST 通过 canonical Make、CI 与 Hook 从 `engine/` workspace root 使用固定的 Rust 1.97.1，在 Go 验证前执行 `cargo build --locked --release` 构建 pinned Rust `cdylib`；workspace MUST 仅含 `mornlea_engine`，并且该 crate 的 normal dependency MUST 只使用 `std`。

#### Scenario: 无预编译 artifact 的构建
- **GIVEN** clean checkout 不含 Cargo target 或 native library
- **WHEN** 运行 `make test-race`
- **THEN** 系统 MUST 先在 `engine/` 目录中以 Rust 1.97.1 执行 `cargo build --locked --release`，再执行 Go race tests
- **AND** `cd engine && rustup show active-toolchain` MUST 报告 `1.97.1` directory override
- **AND** `cargo metadata --no-deps --format-version 1 --manifest-path engine/Cargo.toml` MUST 只报告 workspace member `mornlea_engine`
- **AND** `cargo tree --manifest-path engine/Cargo.toml --workspace --edges normal` MUST 只含 workspace root，且不得报告第三方 dependency

#### Scenario: 本地客户端产物不依赖 Cargo target 位置
- **GIVEN** `make build` 已生成本地客户端产物
- **WHEN** 临时移开 `engine/target`
- **THEN** `bin/mornlea -h` MUST 从同目录 `libmornlea_engine.dylib` 进入 Go 参数解析
- **AND** MUST 以 exit 1 与 `flag: help requested` 证明解析路径已运行
- **AND** 输出 MUST 不含 `dyld` 或 `Library not loaded`
- **AND** 产物 MUST 不包含指向 Cargo 临时 `deps` 目录的 load path

### Requirement: Rust 客户端边界不污染无图形服务端
系统 MUST 允许 `mornlea-server` 经共享 physics/core 依赖固定 Rust engine 与 CGO，但 MUST 保持无客户端、无 WebGPU、无窗口、无 `gfx`、无 `render`。

#### Scenario: Linux amd64 原生 bundle
- **GIVEN** Ubuntu amd64 clean checkout
- **WHEN** 原生构建并移开 Cargo target
- **THEN** MUST 生成同目录 `mornlea-server` 与 `libmornlea_engine.so`
- **AND** server MUST 通过 `$ORIGIN` 加载相邻 `.so` 并进入 Go 参数解析
- **AND** 依赖闭包 MUST 不含 client、mesh、render、gfx、WebGPU、GLFW、字体或窗口包

### Requirement: registry entry 追加 model 字段

mesh registry entry MUST 从 19 字节扩展为 20 字节：既有布局 `id(u16) + opaque(u8) + emission(u8) + material[6](u16) + fluidHeight(u8) + lightAttenuation(u8) + blockTopRaw(u8)` 保持逐字节不变（`blockTopRaw` 仍在 offset 18），新增 `model(u8)` 位于 offset 19。registry 条目上限 MUST 从 64 提升到 80（集成重订后追加五种火把形态的已注册方块为 76，越过 64；80 同时为后续多形态方块留余量）。Go（`nativeRegistryEntryBytes`/`nativeMaxRegistryEntries`/`nativeMaxRegistryWords`/`maxNativeInputBytes`）与 Rust（`REGISTRY_ENTRY_BYTES`/`MAX_REGISTRY_ENTRIES` 等）多处硬编码 MUST 手工同步一致，两侧一致 MUST 由喂满一次跨 FFI 的容量夹具测试守护。未来/未知 model 值 MUST 被拒绝，不得静默回退。

#### Scenario: 条目布局逐字节锁定

- **GIVEN** 解析一条 registry entry 的 Go 解包与 Rust 编码互相对照
- **WHEN** 检查各字段偏移
- **THEN** id=0..1、opaque=2、emission=3、material=4..15、fluidHeight=16、lightAttenuation=17、blockTopRaw=18、model=19 MUST 全部一致

#### Scenario: 容量双端一致

- **GIVEN** Go 侧注册恰好 80 项的 registry 快照
- **WHEN** 作为一次跨 FFI mesh 调用喂给 Rust
- **THEN** 该调用 MUST 成功且不拒绝
- **AND** 第 81 项 MUST 在 Go 侧就被拒绝

#### Scenario: 未知 model 拒绝

- **GIVEN** registry 中某条目携带未定义 model 值
- **WHEN** Rust 处理该 mesh 邻域
- **THEN** 该次 mesh 调用 MUST 返回明确错误，不得产出部分几何

### Requirement: 限五向火把的有限模型 dispatcher

model 值 MUST 是有限的封闭集合：0=默认（无模型覆写，满格、短方块、流体与植物继续走既有判定，植物仍按 material 区间识别）、1..5=火把五种形态（1=落地、2..5=墙面 +X/−X/+Z/−Z，与火把方块编号 72..75 同序）、6=床（保留给床功能行）；流体 MUST 继续使用既有路径与 `fluidHeight`/`blockTopRaw` 输入，不增加重复枚举。Rust MUST 有一个最小 model dispatcher：0 走既有几何，1..5 走新的 `emit_torch` 发射；bed model 与任何未知值 MUST 返回相同的拒绝。MUST NOT 建立 trait/object 层次或任意模型描述语言。

#### Scenario: 默认模型输出逐位不变

- **GIVEN** 不含任何火把方块的既有 neighborhood 与 registry 快照（model 全为 0）
- **WHEN** 以扩展后的 20 字节布局网格化
- **THEN** 满格、短方块、流体与植物的 quad 序列 MUST 与扩展前逐位一致

#### Scenario: 双面 cutout quad 与边长

- **GIVEN** 全空气邻域中的一朵火把
- **WHEN** Rust 发射其几何
- **THEN** 每个形态 MUST 发出固定数量的双面 alpha-cutout quad
- **AND** quad 结构 MUST 仍为既有 8 字节格式、bit 63 MUST 为 0、坐标 MUST 全部落在本格范围

#### Scenario: 落地与墙面几何

- **GIVEN** 一朵落地火把与一朵墙面火把
- **WHEN** 对比发射几何
- **THEN** 落地 MUST 为竖直居中的窄柱；墙面 MUST 贴近对应支撑面且向远离支撑的方向倾斜
- **AND** material/light/AO MUST 来自 registry 与邻域既有规则

#### Scenario: 火把不参与合并

- **GIVEN** 两朵相邻火把与一段普通方块
- **WHEN** greedy 阶段执行
- **THEN** 火把 quad MUST 不并入任何 merge 单元，普通方块 MUST 按既有规则保留合并

### Requirement: engine ABI v8 引入 mesh registry 布局并由 v9 保留

registry entry 布局扩为 20 字节时，engine ABI 版本 MUST 从 `7` 升到 `8`；这是 mesh registry layout 的历史引入版本。当前 engine ABI v9 MUST 原样保留该 20 字节布局和 mesh 行为，并在同一统一 ABI surface 上增加独立的 fluid exports；统一 ABI 版本常数、C header、Rust identity 与 Go 侧常量 MUST 同步为 9。任一非当前版本的调用 MUST 被既有 ABI 校验拒绝。mesh 布局变更本身 MUST NOT 改变 client ABI、协议或存档 schema。

#### Scenario: 版本协商一致

- **GIVEN** Go 侧 `internal/nativeabi.ABIVersion` 与 Rust `mornlea_engine_abi_version()` 在加载时互检
- **WHEN** 双方均为 9
- **THEN** 调用 MUST 全部成功

#### Scenario: v8 布局引入事实保持可追溯

- **GIVEN** registry entry 从旧的 v7 布局扩为 20 字节
- **WHEN** 检查 mesh ABI 演进历史
- **THEN** 该布局引入 MUST 记为 engine ABI v8
- **AND** 当前 v9 MUST 保持相同布局与 mesh 结果

#### Scenario: 当前 v9 拒绝 v8 调用方

- **GIVEN** 调用方以 8 调起当前 v9 的任一导出
- **WHEN** ABI 校验
- **THEN** MUST 被拒绝且携带版本错误，不发布部分输出
