# authoritative-weather Specification

## MODIFIED Requirements

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
