# avatar-locomotion Specification

## Purpose

定义全部 Avatar（玩家/伙伴/夜行者/牛）的 locomotion 呈现：移动时四肢按权威 tick 摆动，静止时回到中性位姿。摆动是纯呈现（tick 派生相位），不触碰任何权威状态。
## Requirements
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

### Requirement: 摆动步频与地面位移协调

摆动相位 MUST 按行进距离推进（按腿长和角幅校准的固定步幅，减轻滑步），而非按固定时间频率；速度估计由呈现位置的水平 XZ 差分驱动，静止 MUST 回中。同一速度下不同实体 MUST 同步调，不同速度 MUST 有可辨的步频差。同 tick 插值 MUST 完整累计水平路程而不重复累计相同位置；纯垂直位移、时间回退与传送 MUST 不推进步态。

#### Scenario: 步频随速度变化

- **GIVEN** 同一 Avatar 分别以慢速与快速移动相同距离
- **WHEN** 对比两者摆动周期数
- **THEN** 周期数 MUST 基本相同（步幅锁定），且慢速序列的单周期 MUST 更长

#### Scenario: 插值与垂直移动
- **GIVEN** 同距离序列分别分成多个同 tick 插值帧，以及只有 Y 变化的序列
- **WHEN** 估算步态
- **THEN** 水平同距 MUST 得到相同周期数，垂直序列 MUST 保持中性；时间回退与传送后 MUST 重置

#### Scenario: 权威 tick 速度门控与显式重置
- **GIVEN** 同 tick 内分段插值后到达下一个权威 tick，或会话显式 Reset
- **WHEN** 更新呈现历史
- **THEN** 同 tick MUST 只推进水平路程并沿用速度，下一个 tick MUST 纳入待估速路程；静止 tick MUST 回中，Reset MUST 清空距离和速度

#### Scenario: 人体尺度与其他种类隔离
- **GIVEN** 人类腿长 0.7 格、最大摆角 0.65 弧度的六件人体，以及牛和夜行者
- **WHEN** 编码身体与摆动
- **THEN** 人类周期路程 MUST 为 `4*0.7*sin(0.65)` 约 1.694522 格，牛与夜行者 MUST 保留各自 0.35 弧度/2 格周期；任意种类单帧位移超过 8 格 MUST 重置呈现步态

