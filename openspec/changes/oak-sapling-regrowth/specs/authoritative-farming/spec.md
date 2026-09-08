## MODIFIED Requirements

### Requirement: 生长推进完全确定且成本与作物数量无关

生长推进 SHALL 在给定世界种子、tick 与方块状态下产生完全确定的结果。推进 MUST NOT 依赖哈希遍历顺序或任何进程级随机源。单个 tick 内被随机作物阶段考察的格数 MUST 只正比于 active Ready 范围内的区段数，MUST NOT 随世界中作物、耕地或树苗的数量增长。随机作物阶段 MUST NOT 扫描耕地的湿润邻域，其方块读取次数 MUST NOT 超过被考察格数的两倍；其中树苗生长分支 MUST 只在被考察格恰为 `SaplingID` 时读取树形几何，单次读取格数 MUST NOT 超过 `128`，且 MUST NOT 因世界中树苗数量增长而增加每 tick 的读取总量上界。独立的耕地湿度阶段每 tick 的方块读取次数 MUST NOT 超过 `65,536`。

#### Scenario: 相同输入重放结果一致

- **GIVEN** 相同的世界种子、相同的初始方块状态与相同的已加载区段集合
- **WHEN** 系统推进相同数量的 tick 两次
- **THEN** 两次的作物阶段与耕地干湿状态 MUST 逐格一致
- **AND** 两次的树苗生长结果 MUST 逐格一致

#### Scenario: 作物数量增加不改变单 tick 考察量

- **GIVEN** 两个只有作物数量不同的世界，active Ready 范围内的区段数相同
- **WHEN** 各推进一个 tick
- **THEN** 两者被随机作物阶段考察的格数 MUST 相同

#### Scenario: 密集耕地不放大随机作物阶段读取

- **GIVEN** 两个 active Ready 区段数相同的世界，一个没有耕地，另一个全部填充为耕地
- **WHEN** 各推进一个随机作物阶段
- **THEN** 两者的方块读取次数 MUST 分别不超过各自被考察格数的两倍
- **AND** 全耕地世界 MUST NOT 为随机样本执行湿润邻域扫描

#### Scenario: 树苗数量增加不放大单 tick 读取总量

- **GIVEN** 两个 active Ready 区段数相同的世界，一个没有树苗，另一个的每个被考察格都是树苗
- **WHEN** 各推进一个随机作物阶段
- **THEN** 树苗生长分支的方块读取次数 MUST 分别不超过「被考察格数 × `130`」（每格常数读取加至多 `128` 格树形几何）
- **AND** 该上界 MUST 只由区段数与固定记录上限决定
