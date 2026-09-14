# Spec: authoritative-crafting

## ADDED Requirements

### Requirement: 护甲四配方进入固定配方表

系统 SHALL 在固定配方注册表末尾追加四条工作台 3×3 形状配方，产物各为一件满耐久的对应铁质护甲：

- `RecipeIronHelmet`：铁锭 5 件——顶排 3 件、次排左右各 1 件（`X X X / X . X`）；
- `RecipeIronChestplate`：铁锭 8 件——次排左右各 1 件、其余七格满（`X . X / X X X / X X X`）；
- `RecipeIronLeggings`：铁锭 7 件——顶排 3 件、其余两排左右各 1 件（`X X X / X . X / X . X`）；
- `RecipeIronBoots`：铁锭 4 件——上两排左右各 1 件（`X . X / X . X`）。

新配方 MUST 沿注册表哨兵纪律追加（循环上界常量随注册表自然延伸），MUST NOT 重排或复用任何既有 `RecipeID`；形状匹配、单格容量与产物取出语义 MUST 沿 `authoritative-crafting` 与 `authoritative-grid-crafting` 主规格既有条款。

#### Scenario: 逐配方产出满耐久护甲

- **GIVEN** 工作台 3×3 格按上述形状放入铁锭
- **WHEN** 取出产物
- **THEN** 分别得到满耐久的头盔/胸甲/护腿/靴子各一件，原料按既有原子更新纪律扣除

#### Scenario: 形状不匹配不出产物

- **GIVEN** 3×3 格的铁锭排布与四条护甲形状均不匹配
- **WHEN** 请求产物
- **THEN** 无产物可选，既有配方不受影响
