## MODIFIED Requirements

### Requirement: 三类消息的值域与严格排序约束

系统 SHALL 提供三类 S→C 消息：`PassiveSpawn`、`PassiveState`、`PassiveDespawn`。每类消息 MUST 携带 `ServerTick`（u64）与 `count`（u8，≥1）；每类 MUST 至多 64 条记录，record MUST 按 ID 严格升序且非零。spawn record 携带 ID、dimension、position、yaw、health；state record 携带 ID、position、velocity、yaw、health 与放牧标志（`grazing` u8，仅 `0/1` 合法）；despawn record 只携带 ID。解码 MUST 拒绝：重复或逆序或零 ID、position/velocity/yaw 含 NaN 或 Inf、health 为 0 或大于满值、放牧标志非 `0/1`、非法 dimension、count 为 0 或大于 64、截断、尾随。

#### Scenario: 合法消息 round trip

- **GIVEN** 一个含 3 条升序记录的 spawn 消息与一个含 2 条升序记录的 state 消息（含放牧标志 `0/1` 各一）
- **WHEN** 编码后经 Memory 与 TCP 两条传输解码
- **THEN** 两种传输 MUST 得到逐字段相同的值（含放牧标志），且全部记录按 ID 升序

#### Scenario: 逆序与零 ID 被拒绝

- **GIVEN** 消息记录按 ID 降序，或含 ID 0
- **WHEN** 服务端或客户端解码
- **THEN** 整份消息 MUST 被拒绝，MUST NOT 部分应用

#### Scenario: 越界与非法记录被拒绝

- **GIVEN** 某记录 health 为 0 或超满值，或坐标含 NaN，或 dimension 非法，或放牧标志为 2
- **WHEN** 解码
- **THEN** 整份消息 MUST 被拒绝

#### Scenario: 客户端重复 spawn 按稳定规则处理

- **GIVEN** 客户端收到一条含 64 个升序 ID 的合法消息
- **WHEN** 又收到一条含同 ID 的重复 spawn
- **THEN** 重复 spawn MUST 被按稳定规则处理（同 ID 已有身体时忽略）
