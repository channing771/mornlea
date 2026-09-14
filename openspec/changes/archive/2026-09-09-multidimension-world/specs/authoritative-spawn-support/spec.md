## ADDED Requirements

### Requirement: 按维独立出生扫描与床重生同维约束

系统 SHALL 对每个维度独立执行出生扫描（高度图与支撑语义与既有规则一致，仅数据源按维度切换）；床重生仅当床尾与床头在玩家死亡时所处维度完整可用才生效，否则 MUST 回落到该维度的出生锚点。

#### Scenario: 新维出生脚底合法

- **GIVEN** 玩家传送到从未加载过的 `Depths` 区块
- **WHEN** 出生扫描完成
- **THEN** 落点 MUST 为该维真实碰撞支撑顶面
- **AND** MUST NOT 复用主世界同坐标高度

#### Scenario: 跨维床失效回落本维锚点

- **GIVEN** 玩家重生点床位于主世界而死亡时位于 `Depths`
- **WHEN** 重生结算
- **THEN** 玩家 MUST 在 `Depths` 出生锚点重生
- **AND** 主世界床记录 MUST 保留到玩家返回后仍可用
