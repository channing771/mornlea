# multidimension-world Specification

## Purpose

让服务端同时权威运行主世界之外的第二个维度，并让玩家在维度之间传送往返，为后续多维度玩法提供地基。双维共享同一权威 tick 与存档管线，传送是服务端单 tick 原子事务，客户端只跟随镜像。

## Requirements

### Requirement: 双维度权威启动与隔离结算

系统 SHALL 在启动时同时装配主世界（维度 0）与 `Depths`（维度 1），并在每个权威 tick 内按固定顺序串行结算两个维度；任一维度的方块写入、revision 推进与发布批次 MUST NOT 污染另一维度。

#### Scenario: 启动后双维就绪

- **GIVEN** 服务端从有效存档启动
- **WHEN** 首个权威 tick 完成
- **THEN** 两个维度的出生区区块 MUST 均可被订阅加载
- **AND** 同坐标 `(x,z)` 在双维的方块内容 MUST 按各自维度种子独立确定

#### Scenario: 单维写入不跨维泄漏

- **GIVEN** 玩家位于 `Depths`
- **WHEN** 玩家放置一个方块并完成 tick
- **THEN** 主世界同坐标区块的 revision MUST 不变
- **AND** 主世界订阅者 MUST NOT 收到该变更

### Requirement: 传送事务原子性

系统 SHALL 提供聊天命令 `/warp depths|overworld` 触发的传送事务：同一 tick 内冻结输入、保存旧维位置、搬运维度归属、新维出生扫描，并下发 `PlayerState{Dimension,Reset:true}`；任一步骤失败时玩家 MUST 留在旧维且位置不变，并收到明确拒绝。

#### Scenario: 往返传送成功

- **GIVEN** 存活且已激活的玩家位于主世界
- **WHEN** 发送 `/warp depths` 并完成 tick，再发送 `/warp overworld` 并完成 tick
- **THEN** 两次 `PlayerState` 的 `Dimension` MUST 依次为 1、`Reset` 为 true，第二次为 0
- **AND** 背包、血量、饥饿值 MUST 与传送前一致
- **AND** 目标锚点区块尚未加载时 MUST 按登录冷启动语义暖起，而非拒绝

#### Scenario: 非法传送被拒绝且不动

- **GIVEN** 玩家处于死亡或待出生状态，或目标锚点区块已明确加载失败（`ChunkFailed`），或命令拼写非法
- **WHEN** 发送传送命令并完成 tick
- **THEN** 玩家维度与位置 MUST 保持不变
- **AND** 玩家 MUST 收到 `CommandRejected`

#### Scenario: 伙伴与敌怪不跟随传送

- **GIVEN** 玩家拥有跟随中的伙伴且附近存在敌怪
- **WHEN** 传送完成
- **THEN** 旧维 MUST 下发其 `Despawn`，新维 MUST NOT 自动生成其镜像
- **AND** 返回旧维后伙伴关系与任务状态 MUST 恢复

### Requirement: 协议维度值域与版本

系统 SHALL 将协议升至 v39：玩家与区块类消息（`ChunkSnapshot`、`BlockChanges`、`ForgetChunks`、`RequestChunkResync`、`PlayerState`、远端玩家系列）接受维度 0 与 1；伙伴、敌怪、被动生物类消息 MUST 继续拒绝非零维度；维度 ≥2 的任何消息 MUST 被拒绝；v38 及更早客户端 MUST 在握手阶段被明确拒绝且服务端 MUST NOT 加载或修改任何世界状态。

#### Scenario: v39 双维消息放行

- **GIVEN** v39 客户端已进入 Play
- **WHEN** 服务端下发 `Dimension=1` 的 `ChunkSnapshot` 与 `PlayerState`
- **THEN** 客户端 MUST 接受并按维度分别挂载镜像

#### Scenario: 越界维度被拒绝

- **GIVEN** v39 会话
- **WHEN** 任一方向出现 `Dimension=2` 的 Play 消息
- **THEN** 该消息 MUST 被拒绝且不改变任何权威状态

#### Scenario: 旧协议握手拒绝

- **GIVEN** 客户端声明协议 v38
- **WHEN** 发起握手
- **THEN** 服务端 MUST 以版本不匹配拒绝
- **AND** MUST NOT 加载或修改任何玩家与世界状态
