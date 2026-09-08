# authoritative-grid-crafting Delta

## ADDED Requirements

### Requirement: 空桶配方由铁锭合成

配方表 SHALL 新增空桶配方：裁边后 3×2、左中/右中/底中 3 个铁锭，产物为 1 个空桶物品。该形状的水平镜像与其自身等价，匹配结果 MUST 与既有裁边与镜像语义一致；水桶 MUST NOT 可合成。

#### Scenario: 正确摆放产出空桶

- **GIVEN** 工作台 3×3 网格左中/右中/底中各 1 个铁锭
- **WHEN** 玩家取出产物
- **THEN** 产物 MUST 为恰好 1 个空桶物品，原料按既有原子取出语义消耗
