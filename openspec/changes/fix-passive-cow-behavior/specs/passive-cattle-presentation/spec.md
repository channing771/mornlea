# passive-cattle-presentation Specification

## ADDED Requirements

### Requirement: 牛渲染朝向与权威移动方向一致

客户端装配层 SHALL 把被动牛的权威 yaw 映射到模型前进方向后再驱动渲染旋转：牛模型静止姿态面朝局部 +X，物理朝向约定为 `yaw=0` 面向 -Z，装配时 MUST 施加固定的 +π/2 旋转对齐，使牛头部指向权威朝向方向。映射 MUST 为纯装配层变换，MUST NOT 改变服务端权威 yaw 字段、协议 wire 值或其他实体（玩家/伙伴/夜行者）的朝向直传语义。

#### Scenario: 引诱跟随正面朝向玩家

- **GIVEN** 一头被持小麦玩家引诱、正朝玩家行进的牛
- **WHEN** 客户端渲染该牛
- **THEN** 牛头 MUST 指向行进方向（朝向玩家），MUST NOT 呈现身体侧对行进方向的横行姿态

#### Scenario: 漫游段内直线行进

- **GIVEN** 一头在某漫游段内沿稳定朝向直线行进的牛
- **WHEN** 客户端渲染该牛
- **THEN** 牛身长轴 MUST 与行进轨迹相切（头在前），腿摆动相位 MUST 与位移一致
