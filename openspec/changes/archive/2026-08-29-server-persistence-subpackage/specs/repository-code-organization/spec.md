## ADDED Requirements

### Requirement: Server persistence has a one-way package boundary

仓库 MUST 将世界区块与 metadata、玩家、伙伴和夜行者的存档加载、观察、异步
保存、重试、flush 及 worker 生命周期置于 `internal/server/persistence`。根
`internal/server` MUST 保留 Host、Server、权威 tick、登录、会话、发布和关服
编排，并可以依赖该子包；`internal/server/persistence` MUST NOT 反向依赖根包。

#### Scenario: 架构守卫接受唯一允许的父子依赖

- **GIVEN** 仓库包含 `internal/server` 与 `internal/server/persistence`
- **WHEN** 架构依赖检查枚举全部内部生产包
- **THEN** MUST 接受 `internal/server -> internal/server/persistence`
- **AND** MUST 拒绝 `internal/server/persistence -> internal/server` 或任何未登记的内部依赖

#### Scenario: 持久化职责不留在根包

- **GIVEN** 世界、玩家、伙伴和夜行者都需要其既有存档生命周期
- **WHEN** 根 Server 或 Host 执行 tick、登录、登出或关服
- **THEN** 对应存档生命周期 MUST 由 `internal/server/persistence` 的单一所有者执行
- **AND** 根包 MUST NOT 保留第二套保存队列、重试状态或 worker 实现

### Requirement: Server persistence extraction preserves contracts

这次结构迁移 MUST 保持已有的根包调用面、存档载荷、时序和失败语义。它 MUST NOT
改变 autosave、retry、backpressure、channel 容量、worker 数量、flush/close 顺序、
协议、schema、Rust ABI 或 client ABI。

#### Scenario: 既有持久化工作流保持可观察行为

- **GIVEN** 玩家或世界、伙伴、夜行者存档处于保存、重试、背压或关服 flush 状态
- **WHEN** 迁移后的 Host 或 Server 运行相同工作流
- **THEN** MUST 产生与迁移前相同的成功、失败、重试、背压和关闭结果
- **AND** 已有 `server` 包公开 API 与错误哨兵 MUST 继续可由既有调用方使用

#### Scenario: 测试入口和子测试标签保持可寻址

- **GIVEN** 迁移前已保存 `internal/server` 的 Test、Benchmark、Fuzz 名称及被迁移测试的 `t.Run` 标签
- **WHEN** 迁移完成后分别枚举 `internal/server` 与 `internal/server/persistence`
- **THEN** 默认构建的两个包入口并集 MUST 等于不可变迁移前基线，加且仅加 `TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry`、`TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker`、`TestShutdownFlushSerializesPublicEngineReads` 和 `TestShutdownWorkerTimeoutDrainsReadySaveFailure`
- **AND** `TestPublicPersistenceContracts` MUST 排除在默认构建并集外，并以其单独的 `persistence_contract` 命令验证
- **AND** 被迁移测试的 `t.Run` 标签 MUST 逐项保持不变
