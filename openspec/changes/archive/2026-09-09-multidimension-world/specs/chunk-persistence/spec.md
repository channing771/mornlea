## ADDED Requirements

### Requirement: Depths 区块持久化与维度值域

系统 SHALL 将 `Depths`（维度 1）区块与主世界区块同等持久化到 `dimensions/1/regions/`，chunk 信封的维度字段值域 MUST 为 0 或 1；维度 ≥2 的存档记录 MUST 以损坏拒绝且 MUST NOT 覆盖既有数据。

#### Scenario: 双维保存与重载

- **GIVEN** 双维均有脏区块
- **WHEN** 保存批次提交并重启服务端
- **THEN** `dimensions/0/regions` 与 `dimensions/1/regions` MUST 均可加载
- **AND** 双维 revision MUST 单调连续

#### Scenario: 越界维度记录被拒绝

- **GIVEN** 一条维度为 2 的区块保存请求
- **WHEN** 提交保存批次
- **THEN** 该请求 MUST 被拒绝
- **AND** 磁盘既有数据 MUST 逐字节不变
