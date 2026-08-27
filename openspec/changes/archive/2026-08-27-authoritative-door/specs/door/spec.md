## Purpose

为单材质双格高木门提供权威闭环：双格原子放置、右键开合联动与破坏双清掉落，含碰撞/射线/流体与配方契约。

## ADDED Requirements

### Requirement: Door double-height atomic placement

系统 MUST 原子放置 `lower+upper` 双格，校验下实心上空，否则拒绝；成功时 `lower` 为方向对应的 `Closed` 态、`upper` 为 `DoorUpper` 且消耗 1 个 `ItemDoor`。

#### Scenario: 放置双格原子

- **GIVEN** 玩家持有 `ItemDoor` 且 yaw 为 South，目标 `lower` 为空气且正上方 `upper` 为空气且 `lower` 下方为实心（Opaque 或 Farmland）
- **WHEN** 玩家对 `lower` 执行 `PlaceBlock`
- **THEN** `lower` MUST 变为 `DoorLowerSouthClosed` 且 `upper` MUST 变为 `DoorUpper` 且物品 MUST 消耗 `1`
- **AND** 若 `upper` 被占用、下方非实心或 `upper` 所在 section 未就绪则 MUST 拒绝且零消耗零写入

### Requirement: Door interact toggle

系统 MUST 在玩家对门执行右键交互时切换 `Closed↔Open` 且上下联动，`upper` 逻辑关联 `lower` 方向不变。

#### Scenario: 右键开合联动

- **GIVEN** 世界中存在 `DoorLowerSouthClosed` 且其正上方为 `DoorUpper`
- **WHEN** 玩家对 `lower`（或 `Upper` 定位到 `y-1`）执行 `Interact`
- **THEN** `lower` MUST 变为 `DoorLowerSouthOpen` 且 `upper` 保持 `DoorUpper` 不变（逻辑关联）
- **AND** 再次交互 MUST 回到 `SouthClosed`，四向同理，无消耗

### Requirement: Door mining double-clear

系统 MUST 在破坏门的任意一半时原子双格置空并掉落 `ItemDoor 1`，`DoDrop=false` 时仍双清但零掉落。

#### Scenario: 破坏双清掉落

- **GIVEN** 世界中存在门双格（`DoorLower*+DoorUpper`），玩家对任意一半发起采掘
- **WHEN** `completeMining` 完成该半且 `DoDrop=true`
- **THEN** 双格 MUST 同时变为空气且 MUST 掉落 `ItemDoor 1`
- **AND** `DoDrop=false` 时 MUST 仍双格为空气且零掉落；命中 `Upper` 时定位 `lowerPos` 行为一致
