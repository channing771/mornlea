## ADDED Requirements

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

### Requirement: engine ABI v8

registry entry 布局扩为 20 字节时，engine ABI 版本 MUST 从 `7` 升到 `8`：`mornlea_engine_abi_version()` MUST 返回 8，统一 ABI 版本常数、C header 与 Go 侧常量 MUST 同步为 8；低于 8 的调用 MUST 被既有 ABI 校验拒绝。本变更 MUST 只升 engine ABI（client ABI、协议、存档 schema 不变）。

#### Scenario: 版本协商一致

- **GIVEN** Go 侧 `internal/nativeabi.ABIVersion` 与 Rust `mornlea_engine_abi_version()` 在加载时互检
- **WHEN** 双方均为 8
- **THEN** 调用 MUST 全部成功

#### Scenario: 旧版拒绝

- **GIVEN** 调用方以 7 调起任一导出
- **WHEN** ABI 校验
- **THEN** MUST 被拒绝且携带版本错误
