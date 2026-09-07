# authoritative-daylight Specification

## MODIFIED Requirements

### Requirement: 世界时间由服务端权威推进

服务端 SHALL 维护绝对 `WorldTimeTicks`，每个完成的权威 tick MUST 恰好增加 `1`，并以 `24000` tick 为一个显示昼夜周期。服务端 SHALL 另维护显示相位偏移 `DayPhaseOffset`（0..23999）：显示相位 MUST 等于 `(WorldTimeTicks + DayPhaseOffset) % 24000`，偏移 MUST 只影响显示相位，MUST NOT 影响绝对时间的推进或任何以绝对时间驱动的模拟。在显示相位之上，系统 SHALL 叠加季节 warp 得到季节化显示相位（昼弧比例按年相位连续伸缩，契约见 `seasonal-time-temperature`）；季节化相位 SHALL 由 `shared/core` 的唯一入口派生，入睡判定、夜行者生成窗口、白昼灼烧、昼间被动生成与客户端昼夜光照等全部判相位消费点 MUST 消费同一季节化入口，MUST NOT 自建相位或 warp 算式，MUST NOT 出现一处 warp 一处未 warp 的分叉。全员入睡跳夜 SHALL 只把季节化显示相位推进到当前季节下的早晨段，MUST NOT 回写绝对时间。客户端 MUST 以最新有效权威玩家状态中的绝对时间与 `DayPhaseOffset` 决定昼夜相位，不得各自选择独立时间源。

#### Scenario: 两名玩家观察同一相位

- **GIVEN** 两名 Ready 玩家连接同一服务端
- **WHEN** 服务端发布同一个权威 tick 的玩家状态
- **THEN** Memory 或 TCP 客户端观察到的 `WorldTimeTicks` 与 `DayPhaseOffset` MUST 分别相同

#### Scenario: 每个权威 tick 只推进一次

- **GIVEN** 服务端当前绝对世界时间为 `23999`
- **WHEN** 服务端完成下一个权威 tick
- **THEN** 绝对时间 MUST 为 `24000`，显示相位 MUST 回到周期起点

#### Scenario: 旧状态不回退时间

- **GIVEN** 客户端已经接受一份较新 `ServerTick` 的玩家状态
- **WHEN** 客户端随后收到一份较旧或重复 `ServerTick` 的状态
- **THEN** 客户端 MUST 忽略该状态且不得回退已确认的世界时间

#### Scenario: 偏移只影响显示相位

- **GIVEN** 服务端设置非零 `DayPhaseOffset`
- **WHEN** 服务端继续推进权威 tick
- **THEN** `WorldTimeTicks` 的推进节奏 MUST 保持每 tick 恰好 `1`，且作物、流体与掉落寿命等绝对时间消费者 MUST 与偏移为 0 时逐格一致

#### Scenario: warp 不影响绝对时间消费者

- **GIVEN** 同一 seed 下年相位处于冬至（昼弧最短）与春秋分点各运行一天
- **WHEN** 比较作物生长、流体推进与掉落寿命的结算
- **THEN** 两者 MUST 与 warp 无关、按绝对时间逐 tick 一致

#### Scenario: 判相位消费点共用季节化入口

- **GIVEN** 入睡判定与夜行者生成窗口在同一 tick 求显示相位
- **WHEN** 比较两处取值来源
- **THEN** 两者 MUST 消费同一 `shared/core` 季节化相位入口，取值 MUST 相同

#### Scenario: 跳夜落在当前季节的早晨

- **GIVEN** 冬季某夜全员入睡且跳夜条件满足
- **WHEN** 跳夜结算
- **THEN** 季节化显示相位 MUST 落在冬季昼弧的早晨段，绝对世界时间 MUST 保持每 tick +1 的自然推进
