# passive-mob-protocol Specification

## Purpose

定义被动牛在网络上的可观察同步语义：三类 S→C 消息的值域与排序约束、按会话订阅的发布规则，以及客户端 latest-wins 镜像与插值行为，与夜行者协议纪律对齐但消息独立。

## Requirements

### Requirement: 三类消息的值域与严格排序约束

系统 SHALL 提供三类 S→C 消息：`PassiveSpawn`、`PassiveState`、`PassiveDespawn`。每类消息 MUST 携带 `ServerTick`（u64）与 `count`（u8，≥1）；每类 MUST 至多 64 条记录，record MUST 按 ID 严格升序且非零。spawn record 携带 ID、dimension、position、yaw、health；state record 携带 ID、position、velocity、yaw、health 与放牧标志（`grazing` u8，仅 `0/1` 合法）；despawn record 携带 ID 与原因位（`reason` u8，仅 `0/1` 合法，0=消失/出视野，1=死亡）。解码 MUST 拒绝：重复或逆序或零 ID、position/velocity/yaw 含 NaN 或 Inf、health 为 0 或大于满值、放牧标志非 `0/1`、原因位非 `0/1`、非法 dimension、count 为 0 或大于 64、截断、尾随。

#### Scenario: 合法消息 round trip

- **GIVEN** 一个含 3 条升序记录的 spawn 消息、一个含 2 条升序记录的 state 消息（含放牧标志 `0/1` 各一）与一个含 2 条升序记录的 despawn 消息（含原因位 `0/1` 各一）
- **WHEN** 编码后经 Memory 与 TCP 两条传输解码
- **THEN** 两种传输 MUST 得到逐字段相同的值（含放牧标志与原因位），且全部记录按 ID 升序

#### Scenario: 逆序与零 ID 被拒绝

- **GIVEN** 消息记录按 ID 降序，或含 ID 0
- **WHEN** 服务端或客户端解码
- **THEN** 整份消息 MUST 被拒绝，MUST NOT 部分应用

#### Scenario: 越界与非法记录被拒绝

- **GIVEN** 某记录 health 为 0 或超满值，或坐标含 NaN，或 dimension 非法，或放牧标志为 2，或原因位为 2
- **WHEN** 解码
- **THEN** 整份消息 MUST 被拒绝

#### Scenario: 客户端重复 spawn 按稳定规则处理

- **GIVEN** 客户端收到一条含 64 个升序 ID 的合法消息
- **WHEN** 又收到一条含同 ID 的重复 spawn
- **THEN** 重复 spawn MUST 被按稳定规则处理（同 ID 已有身体时忽略）

### Requirement: 按会话订阅发布

服务端 SHALL 只向对相应 chunk 已订阅的会话发布牛事件：牛进入视野发 spawn，每 tick 发 state，离开视野或死亡发 despawn（携带原因位：死亡为 1，其余为 0），均按会话镜像；每类每 tick 至多发送一包（≤64 条、ID 升序）。Memory 与 TCP 对同一命令与同一世界序列 MUST 给出逐字段相同的会话发布序列。未订阅 chunk 内的牛 MUST NOT 发送。

#### Scenario: 进入视野发 spawn 与逐 tick state

- **GIVEN** 一头牛进入某会话已订阅 chunk
- **WHEN** 服务端推进后续 tick
- **THEN** 该会话 MUST 收到该牛的 spawn，随后每 tick 收到一条含其当前 ID 的 state

#### Scenario: 离开视野或死亡发 despawn

- **GIVEN** 牛离开该会话订阅范围或死亡
- **WHEN** 服务端完成该 tick
- **THEN** 该会话 MUST 收到一条含该 ID 的 despawn（死亡原因位为 1，单纯离开为 0），后续 MUST 不再收到该 ID 的 state

#### Scenario: 未订阅不发送

- **GIVEN** 牛所在 chunk 不在某会话订阅集合
- **WHEN** 服务端推进整个观察窗口
- **THEN** 该会话 MUST NOT 收到任何与该牛相关的消息

#### Scenario: Memory/TCP 发布序列一致

- **GIVEN** 相同的世界状态、玩家会话与牛集合
- **WHEN** Memory 与 TCP 各运行一次并记录全部会话发布
- **THEN** 两边的消息序列 MUST 逐字段相同（含原因位）

### Requirement: 客户端 latest-wins 镜像且不预测

客户端 SHALL 保存牛的权威镜像：spawn 建立身体，state 只接受 `ServerTick` 更新的记录，despawn 按原因位移除（消失立即移除，死亡进入 20 tick 保留后移除，见 `passive-death-presentation`）；未知 ID 的 state（未 spawn）MUST 被丢弃且不隐式造实体。移动呈现 MUST 复用与远端玩家/伙伴/夜行者相同的时间边界插值。客户端 MUST NOT 预测牛的生命、伤害或出生位置。

#### Scenario: 过期 state 被丢弃

- **GIVEN** 客户端镜像 `ServerTick` 为 100
- **WHEN** 收到 `ServerTick` 为 99 的 state 消息
- **THEN** 该消息 MUST 被丢弃，镜像 MUST 保持 tick 100 的值

#### Scenario: 未 spawn 的 state 被丢弃

- **GIVEN** 客户端从未收到某 ID 的 spawn
- **WHEN** 收到含该 ID 的 state
- **THEN** 该记录 MUST 被丢弃且不创建实体

#### Scenario: despawn 按原因移除镜像

- **GIVEN** 客户端已有某 ID 的 spawn 镜像
- **WHEN** 收到含该 ID 的 despawn
- **THEN** 消失原因的身体 MUST 被立即移除；死亡原因的身体 MUST 进入 20 tick 保留，保留期后 MUST 被移除且后续渲染 MUST 不再出现

