## ADDED Requirements

### Requirement: 运行时树形 ABI 函数

Rust engine SHALL 通过新增导出 `mornlea_tree_blocks` 提供运行时树形几何计算，且 engine ABI 版本 MUST 由 `10` 升为 `11`。输入 MUST 是固定布局：`MTB1` magic(4) + layout u32(4) + 世界种子 i64(8) + 根坐标 x/y/z i32(12)，共 `28` 字节；layout MUST 为 `1`，任何未知 magic、未知 layout、长度不符、越界坐标或 ABI 版本不符 MUST 被拒绝且 MUST NOT 改写输出缓冲。输出 MUST 是 `count u32` 加每条 `8` 字节的记录（`dx i8`、`dy i8`、`dz i8`、保留 u8、`block u16` 小端、保留 u16），记录数 MUST NOT 超过 `128`，输出缓冲不足时 MUST 返回显式错误且 MUST NOT 产生部分结果。同一 (世界种子, 根坐标) MUST 恒返回逐记录一致的几何；几何 MUST 限定为普通橡树家族（高度 `5..7`、水平半径 ≤ `2`、普通与蓬松两档固定树冠、无分杈、无珍异巨树），MUST NOT 依赖世界生成的 `8×8` 候选格网格、区块生成顺序或进程级随机源。Go 生产路径 MUST 只经既有 `packages/shared/nativeabi` 桥调用，MUST NOT 包含任何树形几何计算实现。

#### Scenario: 版本与布局校验拒绝坏输入

- **GIVEN** 调用方传入错误 ABI 版本、未知 magic、未知 layout、长度不符或越界坐标
- **WHEN** 调用 `mornlea_tree_blocks`
- **THEN** 函数 MUST 返回错误状态
- **AND** 输出缓冲 MUST 保持调用前内容不变

#### Scenario: 相同输入几何逐记录一致

- **GIVEN** 同一世界种子与同一根坐标
- **WHEN** 重复调用 `mornlea_tree_blocks`
- **THEN** 返回的记录数与每条记录 MUST 完全一致

#### Scenario: 记录数超界时显式失败

- **GIVEN** 调用方提供的输出缓冲小于几何所需
- **WHEN** 调用 `mornlea_tree_blocks`
- **THEN** 函数 MUST 返回显式错误状态且 MUST NOT 写出部分记录

#### Scenario: 世界生成入口不受影响

- **GIVEN** 同一世界种子与同一组区块坐标
- **WHEN** 分别调用 `mornlea_worldgen_chunk` 与 `mornlea_worldgen_probe`
- **THEN** 输出 MUST 与引入本函数前逐格一致
