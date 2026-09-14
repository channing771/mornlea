## ADDED Requirements

### Requirement: 运行时种植橡树不属于世界生成

玩家种植并生长的橡树 SHALL 是与世界生成相互独立的运行时世界修改：其几何只由运行时树形 ABI 函数按世界种子与树苗根坐标给出，MUST NOT 参与、改变或复用世界生成的 `8×8` 候选格判定。世界生成的橡树 MUST 逐格不变：`GenerateChunk`、`BaseBlockAt`、`HeightAt`、`TerrainBlockAt` 与既有 worldgen golden 字节 MUST 保持不变；系统 MUST NOT 因本能力扫描、迁移或回填任何已保存区块，也 MUST NOT 在新区块生成阶段写入运行时种植的橡树。

#### Scenario: 世界生成输出不因运行时种树改变

- **GIVEN** 同一世界种子与同一组区块坐标
- **WHEN** 在运行时种植并生长任意数量的橡树之后再次生成新区块
- **THEN** 新区块的 `GenerateChunk`、`BaseBlockAt`、`HeightAt`、`TerrainBlockAt` 输出 MUST 与种树前逐格一致

#### Scenario: 已保存区块不回填

- **GIVEN** 一份升级前已保存且不含树苗的有效 chunk schema v9 区块
- **WHEN** 新程序加载并再次保存该区块
- **THEN** 系统 MUST NOT 因本能力向其写入 `SaplingID`、原木或树叶
- **AND** 该区块的全部既有方块 MUST 保持不变
