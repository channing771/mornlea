## MODIFIED Requirements

### Requirement: 夜间在暗处确定性生成

系统 SHALL 仅当全部条件同时成立时才生成夜行者：世界难度为非 `peaceful`；显示相位在 `13000..23000`（含端点）；候选来自锚点玩家（从已排序 active session 按 `WorldTimeTicks % 会话数` 选锚点）且水平距离在 `24..48`（含）；候选格局部区块光 MUST ≤7；候选为双格空气（高度 2 的竖向空间）；候选下方支撑格为 solid；候选格与支撑格均非流体；候选所在 chunk 完整加载。生成判定 MUST 从世界种子与 `WorldTimeTicks` 导出，MUST NOT 读取全局随机数或遍历 map；每 tick MUST 至多验证一个生成候选。任一条件不成立时，该候选 MUST 被拒绝且本 tick MUST NOT 生成任何夜行者。

#### Scenario: 夜间暗处生成

- **GIVEN** `normal` 世界显示相位 13000、锚点玩家安全、候选水平距离 36、局部区块光 4、双格空气且下方 solid
- **WHEN** 系统推进该候选验证
- **THEN** 该候选 MUST 被生成，初始生命 MUST 为 20、攻击冷却 MUST 为 0、路径 MUST 为空

#### Scenario: 白日不生成

- **GIVEN** 显示相位 2400（白昼）
- **WHEN** 系统推进任意 tick
- **THEN** MUST NOT 生成任何夜行者

#### Scenario: 和平难度不生成

- **GIVEN** `peaceful` 世界的任意显示相位与候选
- **WHEN** 系统推进任意 tick
- **THEN** MUST NOT 生成任何夜行者，生成验证 MUST NOT 消耗候选预算

#### Scenario: 过亮候选被拒绝

- **GIVEN** 候选局部区块光为 8
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成夜行者

#### Scenario: 距离窗口外被拒绝

- **GIVEN** 候选水平距锚点玩家 23 或 49
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成夜行者

#### Scenario: 流体或支撑不足被拒绝

- **GIVEN** 候选格为流体，或候选下方支撑格为空气/流体，或候选仅一格竖向空气
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成夜行者

#### Scenario: 未加载 chunk 不生成

- **GIVEN** 候选落在尚未完整加载的 chunk
- **WHEN** 系统推进该候选验证
- **THEN** MUST NOT 生成夜行者，MUST NOT 为生成而触发同步加载

#### Scenario: 每 tick 至多验证一个候选

- **GIVEN** 一枚候选的派生与验证预算
- **WHEN** 一个 tick 内完成该候选但条件不满足
- **THEN** 本 tick 结果 MUST NOT 再消耗其它候选，下一次候选验证 MUST 在下一 tick
