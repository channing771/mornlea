## Purpose

定义全部 Avatar（玩家/伙伴/夜行者/牛）的 locomotion 呈现：移动时四肢按权威 tick 摆动，静止时回到中性位姿。摆动是纯呈现（tick 派生相位），不触碰任何权威状态。

## ADDED Requirements

### Requirement: 移动时四肢摆动且静止回中

Avatar 的手臂/腿 SHALL 在身体移动时绕肩/髋转轴正弦摆动（对侧反相，MC 同形），相位由已确认 server tick 与实体稳定 ID 派生（drop 旋转同先例，禁用墙钟）；速度由呈现位置差分估计，低于阈值 MUST 回中性位姿且 MUST NOT 原地踏步。摆动 MUST NOT 改变位置、朝向、包围盒或任何权威字段；死亡/低头等特殊位姿生效时摆动让路。

#### Scenario: 行走摆动可辨

- **GIVEN** 一头牛连续多 tick 位移
- **WHEN** 逐帧渲染
- **THEN** 其四腿 MUST 有肉眼可辨的交替摆动，且牛头朝向 MUST 与身体朝向一致

#### Scenario: 静止回中不停摆

- **GIVEN** 一具 40 tick 位置不变的 Avatar
- **WHEN** 逐帧渲染
- **THEN** 四肢 MUST 保持中性位姿，MUST NOT 踏步

#### Scenario: 摆动不碰权威

- **GIVEN** 行走中的任意 Avatar
- **WHEN** 比对其镜像位置/朝向/生命与服务端快照
- **THEN** MUST 逐字段一致，摆动 MUST 只存在于渲染变换中
