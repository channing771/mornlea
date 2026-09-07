# authoritative-weather Specification

## Purpose

为世界提供服务端权威的晴雨雷暴轮转与持久化，并向客户端同步统一天气，使雨/雪粒子、天空变灰与亮度压暗在多人下一致呈现。

## Requirements

### Requirement: 天气由服务端权威推进与轮转

服务端 SHALL 维护权威 `WeatherKind`（0=晴、1=雨、2=雷暴）与剩余时长 `WeatherTicksRemaining`，每个完成的权威 tick MUST 恰好递减 1（到期则按固定分布掷骰进入下一段：晴 10-150 分钟、雨 3-13 分钟、雷暴为雨段内小概率升级）。客户端 MUST 以最新有效权威玩家状态中的天气为准，不得使用本地随机或墙钟自选天气。

#### Scenario: 两名玩家观察同一天气

- **GIVEN** 两名 Ready 玩家连接同一服务端
- **WHEN** 服务端发布同一个权威 tick 的玩家状态
- **THEN** 两客户端观察到的 `WeatherKind` MUST 相同

#### Scenario: 到期自动轮转

- **GIVEN** 服务端当前 `WeatherTicksRemaining` 为 1 且天气为晴
- **WHEN** 服务端完成下一个权威 tick
- **THEN** 天气 MUST 进入一段合法的新天气（晴/雨/雷暴之一）且剩余时长 MUST 落在对应分布区间内

#### Scenario: 旧状态不回退天气

- **GIVEN** 客户端已接受较新 `ServerTick` 的玩家状态
- **WHEN** 客户端随后收到较旧或重复 `ServerTick` 的状态
- **THEN** 客户端 MUST 忽略该状态且不得回退已确认的天气

### Requirement: 天气经协议同步与 metadata 持久化

`PlayerState` SHALL 在 `WorldTimeTicks` 之后追加 1 字节天气（协议 v36，0..2，越界拒绝）；世界 metadata v4 SHALL 保存天气与剩余时长，既有 v3 世界 MUST 可迁移（默认晴天、剩余时长取默认值），自动保存与正常关服 MUST 持久化最终权威值且 MUST NOT 阻塞权威 tick。

#### Scenario: v36 往返保值

- **GIVEN** 服务端构造含合法天气的 `PlayerState`
- **WHEN** 经 Memory 与 TCP 编解码各往返一次
- **THEN** 两次得到的 `WeatherKind` MUST 与发送值相同，非法值 MUST 被拒绝

#### Scenario: v3 世界迁移默认晴天

- **GIVEN** 一个 CRC 有效的 metadata v3 世界
- **WHEN** 新程序首次打开该世界
- **THEN** 系统 MUST 读取既有种子与时间并把天气设为晴天，在下一次正常保存时写为 metadata v4

#### Scenario: 重启延续天气

- **GIVEN** 正常关服屏障已成功保存天气与剩余时长
- **WHEN** 服务端重新打开同一世界并完成初始化
- **THEN** 首份有效权威状态的天气 MUST 与关服前一致

### Requirement: 客户端天气表现统一且有界

雨/雷暴时客户端 SHALL 显示降水粒子（相机前方有界数量、随权威 tick 下落、无堆积）、天空与云变灰、昼夜亮度按上限压暗（雨天内部天空光等效不超过 12/15，雷暴不超过 10/15）；降水形态 SHALL 由共享温度公式的局部温度确定（局部温度 ≤ 雪点 0℃ 为雪、以上为雨；公式契约见 `seasonal-time-temperature`，海拔项使高处天然更冷），客户端按粒子位置本地求值，不得新增降水形态的服务端状态或同步字段；雷暴 SHALL 叠加全屏闪光（固定频率与幅度上限，无伤害语义）。晴天 MUST 恢复既有天空与亮度。稳定天气帧 MUST NOT 触发地形重网格、无界队列或每帧资源创建。

#### Scenario: 雨天压暗但 HUD 不变

- **GIVEN** 天气为雨且显示相位为正午
- **WHEN** 客户端绘制露天面与快捷栏
- **THEN** 露天昼夜亮度 MUST 低于晴天正午值，HUD MUST 保持既有颜色

#### Scenario: 局部温度决定雨雪形态

- **GIVEN** 权威天气为雨，冬季低海拔与夏季高海拔各一处降水粒子
- **WHEN** 客户端按共享温度公式对两处粒子求局部温度并绘制
- **THEN** 冬季低海拔与夏季高海拔 MUST 显示雪粒子，同季反例位置 MUST 显示雨粒子，且各处权威 `WeatherKind` MUST 相同

#### Scenario: 雷暴闪光有界

- **GIVEN** 天气为雷暴
- **WHEN** 连续绘制多帧
- **THEN** 每帧亮度增量 MUST 在固定上限内，且 MUST NOT 改变任何权威状态

#### Scenario: 回晴恢复

- **GIVEN** 天气由雨变为晴
- **WHEN** 客户端接受新权威状态并绘制下一帧
- **THEN** 雨/雪粒子 MUST 停止，天空与亮度 MUST 回到晴天曲线
