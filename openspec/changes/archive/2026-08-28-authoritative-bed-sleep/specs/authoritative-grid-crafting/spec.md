# authoritative-grid-crafting Specification

## ADDED Requirements

### Requirement: 床配方由小麦与木板合成

配方表 SHALL 新增床配方：3×3 网格顶排 3 个小麦、下排 3 个橡木木板（中排空），产物为 1 个床物品。该形状与门配方（2×3 两列木板）MUST NOT 互相匹配；床形状的水平镜像与其自身等价，匹配结果 MUST 与既有裁边与镜像语义一致。

#### Scenario: 正确摆放产出床

- **GIVEN** 工作台 3×3 网格顶排 3 个小麦、下排 3 个橡木木板
- **WHEN** 玩家取出产物
- **THEN** 产物 MUST 为恰好 1 个床物品，原料按既有原子取出语义消耗

#### Scenario: 与门形状互不误配

- **GIVEN** 门配方的 2×3 两列木板摆放
- **WHEN** 匹配配方表
- **THEN** 结果 MUST 为门配方，MUST NOT 匹配床配方；床配方摆放 MUST NOT 匹配门配方
