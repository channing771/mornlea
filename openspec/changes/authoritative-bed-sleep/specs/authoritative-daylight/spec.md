# authoritative-daylight Specification

## MODIFIED Requirements

### Requirement: 世界时间由服务端权威推进

服务端 SHALL 维护绝对 `WorldTimeTicks`，每个完成的权威 tick MUST 恰好增加 `1`，并以 `24000` tick 为一个显示昼夜周期。服务端 SHALL 另维护显示相位偏移 `DayPhaseOffset`（0..23999）：显示相位 MUST 等于 `(WorldTimeTicks + DayPhaseOffset) % 24000`，偏移 MUST 只影响显示相位，MUST NOT 影响绝对时间的推进或任何以绝对时间驱动的模拟。客户端 MUST 以最新有效权威玩家状态中的绝对时间与 `DayPhaseOffset` 决定昼夜相位，不得各自选择独立时间源。

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

### Requirement: 世界时间通过 metadata v3 持久化

世界 metadata v3 SHALL 保存绝对 `WorldTimeTicks` 与 `DayPhaseOffset`。既有 metadata v1/v2 世界 MUST 可迁移为 v3，迁移时世界时间保持原值、偏移取 `0`；自动保存 MUST 异步提交该保存边界观察到的最新权威时间与偏移，正常关服 MUST 持久化冻结后的最终权威时间与偏移，且 metadata I/O MUST NOT 阻塞权威 tick。

#### Scenario: v2 世界迁移偏移为零

- **GIVEN** 一个 CRC 有效的 metadata v2 世界
- **WHEN** 新程序首次打开该世界
- **THEN** 系统 MUST 读取既有种子、出生信息与世界时间，把 `DayPhaseOffset` 设为 `0`，并在下一次正常保存时写为 metadata v3

#### Scenario: 重启延续世界时间与偏移

- **GIVEN** 正常关服屏障已成功保存绝对时间与偏移
- **WHEN** 服务端重新打开同一世界并完成初始化
- **THEN** 首份有效权威状态 MUST 从保存值继续，显示相位 MUST 与关服前一致，而不是重置到客户端本地时间或默认相位

#### Scenario: 自动保存不阻塞 tick

- **GIVEN** metadata 保存底层 I/O 尚未完成
- **WHEN** 权威时钟继续产生 tick
- **THEN** simulation MUST 继续推进，且待保存时间与偏移 MUST 合并到最新值而不得形成无界保存队列
