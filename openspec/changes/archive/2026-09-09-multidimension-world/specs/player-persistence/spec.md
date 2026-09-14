## ADDED Requirements

### Requirement: 玩家维度连续性

系统 SHALL 在传送时保存旧维位置并在新维恢复时携带维度标记；重启后玩家 MUST 回到下线时所在的维度与位置，重生点维度 MUST 同步保持；单维度旧档加载时维度 MUST 默认为主世界。

#### Scenario: 传送后重启仍在新维

- **GIVEN** 玩家传送到 `Depths` 后正常下线
- **WHEN** 服务端重启并加载该玩家
- **THEN** 玩家 MUST 出生在 `Depths` 下线位置
- **AND** 重生点维度 MUST 为玩家最后一次有效设置的维度
