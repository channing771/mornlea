## REMOVED Requirements

### Requirement: 自然回血受饥饿门控并消耗疲劳

### Requirement: 饥饿归零按固定间隔扣血但不致死

## ADDED Requirements

### Requirement: 自然回血按难度条件化门控并消耗疲劳

系统 SHALL 只在玩家饥饿值不低于回血门控阈值（`normal`/`hard`，默认 `18`）时进行自然回血；每回复一点生命值 MUST 累积固定的回血疲劳量，该疲劳规则三档难度一致。`peaceful` MUST 取消饥饿门控（阈值视为零），并仅在本次实际回复生命后把饥饿值恢复为 `MaxHunger`、饱和度恢复为该上限对应的最大值；未实际回血（满血或回血计时未到）时 MUST NOT 改变饥饿与饱和度状态。

#### Scenario: 饥饿值 18 以上可回血

- **GIVEN** `normal` 世界玩家生命值 10、饥饿值 18、已连续 100 tick 未受伤
- **WHEN** 系统再推进 40 tick
- **THEN** 生命值 MUST 变为 11,且疲劳值 MUST 增加固定的回血疲劳量

#### Scenario: 饥饿值 17 不回血

- **GIVEN** `normal` 世界玩家生命值 10、饥饿值 17、已连续 100 tick 未受伤
- **WHEN** 系统再推进任意 tick
- **THEN** 生命值 MUST 保持 10

#### Scenario: 回血消耗最终体现为饥饿下降

- **GIVEN** `normal` 世界玩家饥饿值 20、饱和度 0、生命值 10、已连续 100 tick 未受伤
- **WHEN** 系统持续回血直到疲劳累积达到阈值
- **THEN** 饥饿值 MUST 下降

#### Scenario: 和平难度无门控且回血后恢复完整饥饿

- **GIVEN** `peaceful` 世界玩家生命值 10、饥饿值 5、已连续 100 tick 未受伤
- **WHEN** 系统推进到一次实际回血完成
- **THEN** 生命值 MUST 增加，饥饿值 MUST 变为 `MaxHunger` 且饱和度 MUST 恢复为该上限对应的最大值，疲劳值 MUST 按同一回血疲劳规则累积

#### Scenario: 和平难度未实际回血不改饥饿状态

- **GIVEN** `peaceful` 世界玩家生命值 20（满血）、饥饿值 5
- **WHEN** 系统再推进任意 tick
- **THEN** 饥饿值与饱和度 MUST 保持 5 与原值不变

### Requirement: 饥饿归零按固定间隔扣血且致死性由难度决定

饥饿值为零时,系统 SHALL 每隔固定 tick 数经与其他伤害相同的结算入口扣除一点生命值并重置回血计时。`normal` 难度 MUST 保留「一点生命」硬地板:生命值不高于 `1` 时停止扣除,饥饿伤害 MUST NOT 使玩家死亡。`hard` 难度 MUST 取消该硬地板,饥饿伤害可进入既有死亡结算。`peaceful` 难度 MUST 完全跳过饥饿伤害,饥饿值为零时不扣血也不重置回血计时。

#### Scenario: 饥饿归零周期扣血

- **GIVEN** `normal` 世界玩家饥饿值 0、生命值 10
- **WHEN** 系统推进一个饥饿伤害间隔
- **THEN** 生命值 MUST 变为 9,且回血计时 MUST 被重置

#### Scenario: 饥饿伤害止于一点生命

- **GIVEN** `normal` 世界玩家饥饿值 0、生命值 1
- **WHEN** 系统推进任意数量的饥饿伤害间隔
- **THEN** 生命值 MUST 保持 1,玩家 MUST NOT 死亡

#### Scenario: 困难难度饥饿可致死

- **GIVEN** `hard` 世界玩家饥饿值 0、生命值 1
- **WHEN** 系统推进一个饥饿伤害间隔
- **THEN** 玩家 MUST 经既有死亡结算死亡,死亡反馈、快照与持久化 MUST 复用统一路径

#### Scenario: 和平难度饥饿归零不扣血

- **GIVEN** `peaceful` 世界玩家饥饿值 0、生命值 10
- **WHEN** 系统推进任意数量的饥饿伤害间隔
- **THEN** 生命值 MUST 保持 10,回血计时 MUST NOT 因饥饿伤害被重置
