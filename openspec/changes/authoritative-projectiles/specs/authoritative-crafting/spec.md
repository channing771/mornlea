# Spec: authoritative-crafting

## ADDED Requirements

### Requirement: 箭配方进入固定配方表

系统 SHALL 在固定配方表末尾追加 `RecipeArrow`（新末项配方 ID）：形状为上格砾石、下格木棍的纵向两格配方，产物为 2 支箭。该配方 MUST 可在背包 2×2 个人网格内合成，遵循既有固定配方的原料匹配、产物取出与原子更新语义；配方 ID MUST NOT 重排或复用任何既有配方编号，配方枚举的穷举上界 MUST 推进到新末项。

#### Scenario: 形状匹配并产出两支箭

- **GIVEN** 个人网格上格放 1 个砾石、下格放 1 根木棍
- **WHEN** 请求合成产物
- **THEN** 系统 MUST 产出恰好 2 支箭，两格原料各消耗 1 个

#### Scenario: 形状不匹配被拒绝

- **GIVEN** 个人网格上格放木棍、下格放砾石（上下颠倒），或任一格为其他物品
- **WHEN** 请求合成产物
- **THEN** 系统 MUST 拒绝合成且不消耗任何原料
