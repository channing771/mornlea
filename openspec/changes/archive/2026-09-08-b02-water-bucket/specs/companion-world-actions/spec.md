# companion-world-actions Delta

## MODIFIED Requirements

### Requirement: 伙伴采掘复用玩家计时规则且原子结算

普通方块的采掘目标 MUST 是具有单一 `BlockDrop` 的非容器、非农业、非流体方块。流体方块（源与七档流动水）MUST 被 Planner 契约与权威模拟双重显式拒绝——即使未来流体登记了掉落物，该显式拒绝 MUST 仍然成立，MUST NOT 因巧合性条件消失。其余三方原子、容量全或无与目标变化语义不变。

#### Scenario: 流体被显式拒绝

- **GIVEN** 交互距离内有一个源水方块
- **WHEN** 计划包含以其为目标的 `mine` 步骤
- **THEN** 该步骤 MUST 被拒绝（计划校验与权威模拟双重拦截），方块 MUST NOT 被破坏

### Requirement: 伙伴放置原子扣料写入

`place` 步骤的目标方块 MUST NOT 是流体方块；以流体为目标的放置 MUST 被拒绝，背包 MUST 保持不变。其余校验与原子语义不变。

#### Scenario: 向流体放置被拒绝

- **GIVEN** 计划包含以流体格为目标的 `place` 步骤
- **WHEN** 任务尝试执行放置
- **THEN** 放置 MUST 被拒绝，背包 MUST 保持不变
