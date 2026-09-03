## MODIFIED Requirements

### Requirement: 缺失玩家材料包
系统 SHALL 只在玩家存档明确不存在时构造一次初始材料包：固定 27 格背包的前 14 格 MUST 依稳定材料清单顺序各包含 `64` 个对应物品，其后一格及全部剩余格 MUST 为空，九格快捷栏 MUST 保持为空。材料包 MUST NOT 再包含小麦种子；第一颗种子改由自然探索入口提供。材料包 MUST 通过现有背包合法性规则，并仍由服务端权威确认与持久化。已有玩家的全部快捷栏与背包栏位（包括升级前材料包留下的 `64` 颗种子）MUST 逐槽保留，MUST NOT 删除、补发、重排或清零。

#### Scenario: 缺失玩家获得固定材料包
- **GIVEN** `LoadPlayer` 返回 `ErrPlayerNotFound`
- **WHEN** 准备玩家快照
- **THEN** 快捷栏 MUST 为空且背包前 14 格 MUST 按固定顺序各含 64 个材料
- **AND** 背包第 15 格及之后全部栏位 MUST 为空

#### Scenario: 已有玩家与未确认登录不被补发
- **GIVEN** 已有玩家或未确认登录
- **WHEN** 恢复、迁移或断开
- **THEN** 已有快捷栏与背包全部栏位 MUST 逐槽不变且未确认材料包 MUST 不持久化或累加
- **AND** 已有栏位中的小麦种子 MUST 不被删除、补发或重排

#### Scenario: 确认后的新玩家不会重复获得材料
- **GIVEN** 新玩家已确认登录并保存包含初始材料包的快照
- **WHEN** 该玩家再次登录
- **THEN** 系统 MUST 恢复已保存背包，且 MUST NOT 再次填充或累加材料

#### Scenario: 材料包含有起步种子

> 标题为匹配主规格的 MODIFIED 漂移守卫而保留；本变更后的断言明确取消起步种子。

- **GIVEN** `LoadPlayer` 返回 `ErrPlayerNotFound`
- **WHEN** 准备玩家快照
- **THEN** 背包与快捷栏 MUST 不包含任何小麦种子
- **AND** 系统 MUST NOT 以其他初始栏位替代被取消的种子赠送
