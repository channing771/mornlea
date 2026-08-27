## MODIFIED Requirements

### Requirement: 客户端从权威方块镜像确定性派生静态方块光

客户端 SHALL 只从已接受的权威方块镜像派生静态方块光。`LightBlockID` MUST 发出等级 `15`，五种火把方块形态 MUST 发出等级 `14`，其余已知或未知方块 MUST 发出 `0`；光在六个轴向上 MUST 仅向 `AirID` 相邻格传播并每格衰减 `1`，任何其他方块即使未来被标记为透明也 MUST 阻断方块光，多个光源在同一格 MUST 取最大值。缺失邻区 MUST 按非空气且无发光处理，服务端 MUST NOT 计算、存储或传输光照数组。

#### Scenario: 单光源按距离衰减

- **GIVEN** 一个发光块周围有连续空气且没有其他光源
- **WHEN** 客户端从相同权威方块镜像派生方块光
- **THEN** 源格 MUST 为 `15`、相邻空气 MUST 为 `14`、距离 `14` 的空气 MUST 为 `1`，距离 `15` 的空气 MUST 为 `0`

#### Scenario: 火把按等级 14 参与同一传播

- **GIVEN** 一朵火把周围有连续空气且没有其他光源
- **WHEN** 客户端从相同权威方块镜像派生方块光
- **THEN** 火把格 MUST 为 `14`、相邻空气 MUST 为 `13`、距离 `13` 的空气 MUST 为 `1`，距离 `14` 的空气 MUST 为 `0`
- **AND** 传播、阻断与多光源取最大值的规则 MUST 与发光块完全同路径，不存在火把专属光照公式

#### Scenario: 非空气方块与缺失邻区阻断传播

- **GIVEN** 发光块与目标位置之间存在任一非 `AirID` 方块或尚未加载的邻区
- **WHEN** 客户端派生边界处的方块光
- **THEN** 光 MUST NOT 因该方块当前或未来的透明属性而穿过边界产生亮缝，邻区到达后 MUST 从最新镜像重新收敛

#### Scenario: 多光源结果确定且取最大值

- **GIVEN** 两个发光块可经不同距离照到同一空气格
- **WHEN** 客户端重复从同一权威方块镜像派生光照
- **THEN** 该格 MUST 取两条路径中的较高等级，且每次构建的 packed 光照 MUST 相同

## ADDED Requirements

### Requirement: 唯一发光判定表

服务端（含未来夜行者黑暗判定）与客户端注册表 MUST 都消费 `core.BlockEmission`，不得各自维护发光表。五种火把与发光方块的 emission 值 MUST 经同一张表进入 mesh registry 快照送过 ABI 边界；`internal/assets.Registry.Emission` MUST 完全转调 `core.BlockEmission`；任何新增发光方块 MUST 只改 core 一张表。

#### Scenario: assets 转调

- **GIVEN** `internal/assets.Registry.Emission` 被任意方块编号调用
- **WHEN** 与 `core.BlockEmission` 对照
- **THEN** 两个返回值 MUST 恒等
- **AND** assets 内 MUST 不存在独立的发光判定分支

#### Scenario: 火把与夜空

- **GIVEN** 夜晚、无天空光，`torch-night` 场景的封闭暗室
- **WHEN** 渲染完成
- **THEN** 火把附近 MUST 呈现受方块光照亮的表面
