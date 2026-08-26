# short-block-presentation 规格增量

## ADDED Requirements

### Requirement: 非满格方块按 registry 高度呈现几何

系统 SHALL 让方块 registry 携带每方块的 4-bit 顶面高度原值 `block_top_raw`：`0` 表示满格方块（哨兵），`1..=14` 表示该方块所有可见面的上缘按 `(block_top_raw+1)/16` 下沉，`15` MUST 被输入校验拒绝。携带非零高度的方块（首个消费者为干耕地与湿耕地，填 `14`，即呈现高度 15/16）的顶面与四个侧面的上缘 MUST 呈现在该高度处，与其权威碰撞体一致；其下缘与未下沉方向 MUST 保持整格边界。此类方块 MUST NOT 参与贪心合并，quad 实例 MUST 保持 `u64` / 8 字节。满格方块、流体与植物的既有呈现 MUST 逐位不变。

#### Scenario: 耕地几何与碰撞体一致

- **GIVEN** 一格干耕地或湿耕地、上方为空气
- **WHEN** 该区段被网格化并渲染
- **THEN** 顶面 quad 与四个侧面 quad 的上缘 MUST 呈现在 y = 15/16 处
- **AND** 玩家站立其上的脚部位置 MUST 与可见顶面齐平（碰撞体本就是 15/16）

#### Scenario: 下沉方块不贪心合并

- **GIVEN** 相邻两格同材质耕地
- **WHEN** 网格化完成
- **THEN** 每格 MUST 独立出 quad（1×1），MUST NOT 合并为跨格 quad

#### Scenario: 非法高度原值被原子拒绝

- **WHEN** registry 条目的 `block_top_raw` 字节为 `15`
- **WHEN** 输入解析执行
- **THEN** native 调用 MUST 返回可判定的校验失败且不产出部分网格

#### Scenario: 满格与流体呈现逐位不变

- **GIVEN** 不含任何非零 `block_top_raw` 方块的 neighborhood
- **WHEN** 以 v7 输入网格化
- **THEN** 产出的 packed quads MUST 与 v6 在同一输入下逐位一致

### Requirement: engine ABI 升版承载 registry 布局扩展

registry 条目布局从 18 字节扩为 19 字节时，engine ABI 版本 SHALL 从 `6` 升到 `7`：`mornlea_engine_abi_version` MUST 返回 `7`，Go 侧经 header 常量同源读取并与 dylib 握手。版本不匹配的 dylib 与二进制组合 MUST 被既有握手拒绝。

#### Scenario: 新旧组件不可混装

- **GIVEN** v6 dylib 搭配期望 v7 的 Go 二进制（或反之）
- **WHEN** 任一 native 入口被调用
- **THEN** MUST 返回 ABI 版本不匹配状态且不执行任何网格化

#### Scenario: 双侧版本常量同源

- **GIVEN** Rust `ffi.rs` 的 `ABI_VERSION` 与 Go 经 cgo 读到的 `nativeabi.ABIVersion`
- **WHEN** 构建完成后分别查询
- **THEN** 两值 MUST 相等且等于 `7`
