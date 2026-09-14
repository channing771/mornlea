## ADDED Requirements

### Requirement: 世界 metadata v4 迁移到 v5

系统 SHALL 读取 v1..v5 的世界 metadata，写出只用 v5；v5 在 v4 载荷尾部追加维度表（维度数与 `Depths` 出生锚点、种子 salt）；v4 及更早文件缺失尾部时，`Depths` 出生锚点 MUST 默认取主世界锚点；高于 v5 的版本 MUST 以未来版本拒绝且 MUST NOT 修改任何文件。

#### Scenario: v4 旧档默认迁移

- **GIVEN** 世界目录持有合法 v4 `world.meta`
- **WHEN** 服务端启动加载
- **THEN** 世界 MUST 正常启动且 `Depths` 出生锚点 MUST 等于主世界锚点
- **AND** 首次保存 MUST 写出 v5 文件

#### Scenario: 未来版本拒绝且保旧

- **GIVEN** `world.meta` 声明版本高于 v5
- **WHEN** 服务端启动加载
- **THEN** 启动 MUST 以未来版本错误终止
- **AND** 磁盘文件 MUST 逐字节不变
