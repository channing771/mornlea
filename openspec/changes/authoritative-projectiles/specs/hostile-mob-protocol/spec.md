# Spec: hostile-mob-protocol

## MODIFIED Requirements

### Requirement: 三类消息的值域与严格排序约束

系统 SHALL 提供三类 S→C 消息：`HostileSpawn`、`HostileState`、`HostileDespawn`。每类消息 MUST 携带 `ServerTick`（u64）与 `count`（u8，≥1）；每类 MUST 至多 64 条记录，record MUST 按 ID 严格升序且非零。spawn record 携带 ID、dimension、position、yaw、health 与敌怪 kind（值域 {0,1}：0=夜行者、1=掷骨者）；state record 携带 ID、position、velocity、yaw、health 与敌怪 kind；despawn record 只携带 ID。kind 字节 MUST 以 pure-append 方式位于 record 尾部，MUST NOT 重排或改义任何既有字段。解码 MUST 拒绝：重复或逆序或零 ID、position/velocity/yaw 含 NaN 或 Inf、health 为 0 或大于 20、非法 dimension、kind 越界、count 为 0 或大于 64、截断、尾随。

#### Scenario: 合法消息 round trip

- **GIVEN** 一个含 3 条升序记录（含合法 kind）的 spawn 消息与一个含 2 条升序记录的 state 消息
- **WHEN** 编码后经 Memory 与 TCP 两条传输解码
- **THEN** 两种传输 MUST 得到逐字段相同的值，且全部记录按 ID 升序、kind 逐条一致

#### Scenario: 逆序与零 ID 被拒绝

- **GIVEN** 消息记录按 ID 降序，或含 ID 0
- **WHEN** 服务端或客户端解码
- **THEN** 整份消息 MUST 被拒绝，MUST NOT 部分应用

#### Scenario: 越界与非法记录被拒绝

- **GIVEN** 某记录 health 为 0/21，或坐标含 NaN，或 dimension 非法，或 kind 为 2
- **WHEN** 解码
- **THEN** 整份消息 MUST 被拒绝

#### Scenario: count 越界与截断被拒绝

- **GIVEN** count 为 0 或 65，或 payload 截断/存在尾随
- **WHEN** 解码
- **THEN** 整份消息 MUST 被拒绝

#### Scenario: 客户端拒绝未知健壮性边界

- **GIVEN** 客户端收到一条含 64 个升序 ID 的合法消息
- **WHEN** 又收到一条含同 ID 的重复 spawn
- **THEN** 重复 spawn MUST 被按稳定规则处理（同 ID 已有身体时忽略）

### Requirement: 按会话订阅发布

服务端 SHALL 只向对相应 chunk 已订阅的会话发布敌怪事件（两类 kind 同规则）：敌怪进入视野发 spawn，每 tick 发 state，离开视野或死亡发 despawn，均按会话镜像；每类每 tick 至多发送一包（≤64 条、ID 升序）。Memory 与 TCP 对同一命令与同一世界序列 MUST 给出逐字段相同的会话发布序列。未订阅 chunk 内的敌怪 MUST NOT 发送。

#### Scenario: 进入视野发 spawn 与逐 tick state

- **GIVEN** 一只敌怪（任一 kind）进入某会话已订阅 chunk
- **WHEN** 服务端推进后续 tick
- **THEN** 该会话 MUST 收到该敌怪的 spawn（含 kind），随后每 tick 收到一条含其当前 ID 的 state

#### Scenario: 离开视野或死亡发 despawn

- **GIVEN** 敌怪离开该会话订阅范围或死亡
- **WHEN** 服务端完成该 tick
- **THEN** 该会话 MUST 收到一条含该 ID 的 despawn，后续 MUST 不再收到该 ID 的 state

#### Scenario: 未订阅不发送

- **GIVEN** 敌怪所在 chunk 不在某会话订阅集合
- **WHEN** 服务端推进整个观察窗口
- **THEN** 该会话 MUST NOT 收到任何与该敌怪相关的消息

#### Scenario: Memory/TCP 发布序列一致

- **GIVEN** 相同的世界状态、玩家会话与敌怪集合（两类 kind 混合）
- **WHEN** Memory 与 TCP 各运行一次并记录全部会话发布
- **THEN** 两边的消息序列 MUST 逐字段相同
