## Purpose

把服务端大世界多玩家流式的可观察行为钉为主规格：每会话视距协商、订阅并集兴趣管理、磁盘存档 I/O 的并行边界与 region 句柄的有界性，以及 benchmark 对流式吞吐与内存的记录口径。

## ADDED Requirements

### Requirement: 每会话视距经登录协商并按并集加载

服务端 SHALL 在协议 v40 的登录消息中接受客户端声明的期望视距（闭区间 `2..64` 的整数），并按会话独立计算订阅范围：超出服务端上界的值 MUST 被钳制到服务端上界，非法值域外的值 MUST 使登录被拒绝。每个会话的订阅范围 MUST 是以其订阅中心为圆心的方形视距；服务端实际加载的区块集合 MUST 是全部会话订阅范围的并集；不再属于任何会话订阅范围的区块 MUST 按既有卸载语义卸载。同一服务端上不同会话 MUST 可以持有不同视距，且一个会话的视距 MUST NOT 使其他会话的订阅范围缩小。

#### Scenario: 两会话不同视距按并集加载

- **GIVEN** 两个已完成登录的会话，中心相邻，声明的视距分别为 `4` 与 `16`
- **WHEN** 订阅对账完成且区块加载收敛
- **THEN** 已加载区块集合 MUST 等于两个会话各自方形视距的并集
- **AND** 任一会话视距内的区块 MUST NOT 因另一会话视距更小而被卸载

#### Scenario: 超出服务端上界被钳制

- **GIVEN** 客户端在登录消息中声明视距 `64`，服务端配置的视距上界换算后为 `32`
- **WHEN** 登录完成
- **THEN** 该会话的生效视距 MUST 为 `32`
- **AND** 登录 MUST 成功

#### Scenario: 非法视距拒绝登录

- **GIVEN** 客户端在登录消息中声明视距 `0`、`65` 或 `255`
- **WHEN** 服务端校验登录消息
- **THEN** 登录 MUST 被拒绝
- **AND** 会话 MUST NOT 以任何默认视距静默建立

#### Scenario: 视距缩小后多余区块卸载

- **GIVEN** 单一会话以视距 `16` 完成订阅加载，随后该会话断开，另一视距 `4` 的会话仍在相邻中心
- **WHEN** 订阅对账完成
- **THEN** 不再属于任何会话订阅范围的区块 MUST 进入既有卸载路径（干净即卸，脏则保存后卸载）

### Requirement: 磁盘存档 I/O 按类别与 region 并行

服务端存档 I/O SHALL 允许不同 region 文件的操作并行执行，且不同存档类别的操作 MUST NOT 互相阻塞。同一 region 文件内的操作 MUST 保持串行，双 bank 崩溃恢复与原子替换语义 MUST NOT 因并行化而改变。并发交错下的保存与读取结果 MUST 与某种串行执行顺序的结果逐位一致。权威 tick 线程 MUST 保持零磁盘 I/O。

#### Scenario: 并行交错等价于串行结果

- **GIVEN** 一批针对多个 region 的保存与读取任务
- **WHEN** 以随机交错并发执行，并与一次串行执行分别完成
- **THEN** 磁盘上的全部 region 文件内容 MUST 逐位一致
- **AND** 每次读取 MUST 返回与该串行顺序一致的区块数据

#### Scenario: 同一 region 内保持串行

- **GIVEN** 同一 region 文件的两次保存同时就绪
- **WHEN** 两者并发提交
- **THEN** 文件 MUST 不出现交错损坏，解码 MUST 始终能选出一个完整合法的 bank
- **AND** 后完成的保存 MUST 基于先完成的保存结果之上（revision 单调不减）

#### Scenario: 类别间互不阻塞

- **GIVEN** 一次大区块批次保存正在进行
- **WHEN** 一个玩家存档保存任务派发
- **THEN** 玩家存档保存 MUST NOT 等待区块批次完成，两者 MUST 可同时执行

### Requirement: region 句柄有界且淘汰安全

存档层打开的 region 文件句柄数 MUST NOT 超过配置上限（默认 `256`）。句柄淘汰 MUST 只关闭没有在途 I/O 引用的句柄；有在途引用的句柄 MUST NOT 被关闭。被淘汰的 region 再次被访问时 MUST 重新打开并返回与未淘汰时一致的数据。

#### Scenario: 句柄数受上限约束

- **GIVEN** 配置的 region 句柄上限为 `N`，且存档涉及的 region 文件多于 `N` 个
- **WHEN** 依次对全部 region 执行保存或读取
- **THEN** 任一时刻保持打开的句柄数 MUST NOT 超过 `N`
- **AND** 全部保存与读取 MUST 成功且数据完整

#### Scenario: 在途引用不被淘汰关闭

- **GIVEN** 某个 region 的保存正在进行且持有引用
- **WHEN** 句柄治理试图淘汰该 region
- **THEN** 该次淘汰 MUST 被推迟到在途 I/O 完成之后
- **AND** 该次保存 MUST 正常完成

### Requirement: 流式指标族记录多玩家吞吐与内存

benchmark 报告 SHALL 包含 `streaming` 指标族：服务端探针在多会话分散视距场景下的已加载区块数、区块加载时延分位（p50/p95/p99/max）与进程峰值内存。该指标族的数值 MUST 只记录，MUST NOT 改变 producer、比较器或 CI 的退出状态；报告完整性、分位单调性与样本有效性仍 MUST 满足既有报告校验。

#### Scenario: 报告包含完整的 streaming 指标

- **WHEN** benchmark 在多会话分散视距探针场景下生成报告
- **THEN** 报告 MUST 包含已加载区块数、加载时延的 p50/p95/p99/max 与峰值内存
- **AND** 分位数值 MUST 为正且单调不减

#### Scenario: streaming 数值恶化不阻断

- **GIVEN** 一份 streaming 加载时延 p99 相对基线显著恶化的当前报告
- **WHEN** 比较器执行比较
- **THEN** 比较器 MUST 记录该恶化并返回成功
- **AND** 报告结构与样本完整性的校验 MUST 仍然生效
