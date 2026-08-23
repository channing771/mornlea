## ADDED Requirements

### Requirement: 本地 cue 只在确认边界播放

图形客户端 SHALL 只在成功采掘完成、成功放置完成、成功进食完成和收到伤害确认这四种已确认边界各播放对应的一次本地 cue。预测、拒绝、失败和同一确认的重复镜像 MUST NOT 播放 cue。

#### Scenario: 成功事件播放一次

- **GIVEN** 客户端收到任一四类事件的首次成功确认
- **WHEN** 客户端处理该确认
- **THEN** MUST 播放恰好一个对应 cue

#### Scenario: 非确认路径无声

- **GIVEN** 预测、拒绝、失败或重复确认
- **WHEN** 客户端处理该状态
- **THEN** MUST NOT 播放 cue

### Requirement: 总音量严格配置

系统 SHALL 将顶层 `audioVolume` 作为本地 cue 总音量。字段缺席时 MUST 为 `0.7`；`0`、`0.25` 与 `1` MUST 原样加载并保存往返；小于 `0`、大于 `1`、`null` 和非数值 MUST 使配置加载失败。该字段 MUST NOT 被报告为未知字段，也 MUST NOT 出现在数值 `Fields()` 或调试面板中，且配置版本 MUST 保持 `1`。

#### Scenario: 合法音量往返

- **GIVEN** 含任一合法 `audioVolume` 值的配置
- **WHEN** 系统加载后保存并再次加载
- **THEN** 音量 MUST 保持相同值

#### Scenario: 非法音量失败

- **GIVEN** `audioVolume` 为 `-0.01`、`1.01`、`null` 或字符串
- **WHEN** 系统加载配置
- **THEN** MUST 返回带 `audioVolume` 上下文的错误

### Requirement: 不可用音频无声降级

系统 SHALL 在无头、非 Darwin 或 Darwin 设备初始化失败时构造无声播放路径。该路径 MUST 零初始化、不得创建音频设备，并且 MUST NOT 阻止客户端启动、模拟、网络或渲染。

#### Scenario: 平台或设备不可用

- **GIVEN** 无头、非 Darwin 平台或不可用的 Darwin 音频设备
- **WHEN** 客户端初始化本地音频
- **THEN** 系统 MUST 使用无声路径并继续启动
