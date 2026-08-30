## ADDED Requirements

### Requirement: 权威模拟具有五个单向子包

仓库 MUST 将权威模拟组织为 `internal/sim/contract`、`internal/sim/tuning`、
`internal/sim/realm`、`internal/sim/entity` 与 `internal/sim/runtime` 五个子包。
根 `internal/sim` 不得保留生产 Go package、类型别名、转发函数或兼容 facade。所有
内部调用方 MUST 直接导入其所消费值或行为的所属子包。

`contract` 与 `tuning` MUST 不依赖 `realm`、`entity` 或 `runtime`；`realm` MUST
不依赖 `entity` 或 `runtime`；`entity` MAY 依赖 `contract`、`tuning` 与 `realm`，但
`runtime` 是唯一允许同时编排其余四个子包的模拟包。所有直接依赖 MUST 由架构检查
登记并验证。

#### Scenario: 架构检查接受约定方向并拒绝反向边
- **GIVEN** 仓库包含五个权威模拟子包
- **WHEN** 架构依赖检查枚举全部内部生产包
- **THEN** 它 MUST 接受 `runtime` 对 `contract`、`tuning`、`realm` 与 `entity` 的编排依赖
- **AND** 它 MUST 拒绝 `contract` 或 `tuning` 对上层模拟包的依赖、`realm` 对 `entity` 或 `runtime` 的依赖，以及 `entity` 对 `runtime` 的依赖

#### Scenario: 迁移后的调用方不经过根包兼容层
- **GIVEN** 既有 server、config 与客户端装配调用方已迁移
- **WHEN** 仓库编译其内部生产包
- **THEN** 它们 MUST 直接解析 `runtime.Engine`、`contract` 值或 `tuning` 值
- **AND** 不得存在可编译的生产 `internal/sim` facade、类型别名或转发 API

### Requirement: 子包边界保持单一权威提交路径

子包整理 MUST 保持服务端为世界与玩家状态的唯一权威。每个权威 tick 的区块写入
MUST 经同一个 realm-owned 事务收敛，并在一次提交中维持既有 revision、持久化、流体
入队与发布批次语义。`runtime` MUST 保持既有串行 tick 阶段顺序、goroutine 所有权、
有界工作和快照边界。每个实际推进模拟的 server tick MUST 在处理聊天、伙伴任务与
runtime 阶段前捕获一个不可变参数束；该参数束 MUST 分别包含一次读取所得的 simulation
tunables 与 physics tunables，并在当前 tick 的 server manager、runtime、entity、realm
和 physics 路径中按值复用，不得在阶段中再次读取任一全局活动快照。两组独立活动快照的
捕获不构成跨参数组事务，也不得为此引入 `tuning` 对 `physics` 的依赖。

#### Scenario: 同一权威输入保留结算与发布结果
- **GIVEN** 固定世界、权威输入、异步区块结果与 tunable 快照
- **WHEN** 子包整理后的运行时完成一个或多个权威 tick
- **THEN** 它 MUST 产生与整理前相同的接受或拒绝结果、状态发布、区块 revision、方块变更顺序和持久化请求
- **AND** 同一 tick 的相关库存、容器、掉落物与方块变更 MUST 继续原子提交或原子拒绝

#### Scenario: 模拟边界不改变持久化或线上契约
- **GIVEN** 子包整理前后的相同世界与玩家数据
- **WHEN** 它们经 Memory 或 TCP 登录并执行既有模拟路径
- **THEN** wire bytes、协议状态机、存档编码、schema、engine ABI、client ABI、benchmark scenario 与视觉 golden MUST 保持不变

#### Scenario: 单个权威 tick 复用同一参数束
- **GIVEN** server tick 入口已经分别捕获 simulation tunables 与 physics tunables
- **AND** 任一全局活动快照在该 tick 的后续阶段中发生更新
- **WHEN** server manager、runtime、entity、realm 与 physics 完成当前 tick
- **THEN** 当前 tick 的交互距离、眼高、实体运动、浸没和环境推进 MUST 全部使用入口捕获值
- **AND** 下一次实际推进模拟的 tick MAY 观察更新后的活动快照
- **AND** 暂停早退不得无意义地捕获参数，shutdown 的最终模拟 tick MUST 对其 manager 与 runtime 阶段复用同一参数束

### Requirement: 子包迁移保持测试入口

迁移前持久化的 `internal/sim` Test、Benchmark、Fuzz 入口与 `t.Run` 标签 MUST 在迁移
后仍可从 `internal/sim/...` 子包集合中逐项取得。白盒测试 MUST 与其生产所有者一同
迁移，不得为访问旧私有状态而增加生产导出 API。

#### Scenario: 子包测试入口并集等于迁移前清单
- **GIVEN** 迁移前已保存 `internal/sim` 的 Test、Benchmark、Fuzz 入口和 `t.Run` 标签清单
- **WHEN** 迁移后枚举 `internal/sim/...` 的测试入口
- **THEN** 子包入口并集与每个 `t.Run` 标签 MUST 与迁移前清单完全一致
- **AND** 每个子包 MUST 能独立运行其 focused race 测试而不依赖已删除的根 `sim` package
