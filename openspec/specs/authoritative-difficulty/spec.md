# authoritative-difficulty Specification

## Purpose
TBD - created by archiving change authoritative-difficulty. Update Purpose after archive.
## Requirements
### Requirement: 难度是 core 领域值且经 metadata v6 持久化

系统 SHALL 在 `core` 提供恰好三档难度 `normal`、`peaceful`、`hard`，编码为单个 `uint8` 域值并支持严格小写解析与格式化。世界 metadata v6 SHALL 在 v5 载荷末尾纯尾部追加 1 字节难度（载荷 62 字节）；既有 v1..v5 世界 MUST 可只读迁移（难度取 `normal`），打开旧档 MUST NOT 改写磁盘，下一次正常保存才写出 v6；声明高于 v6 的版本 MUST 以未来版本错误稳定拒绝，长度、CRC 或难度值非法的 v6 MUST 以损坏错误拒绝。新世界创建 MUST 以当前版本写入。

#### Scenario: v5 世界迁移默认普通难度

- **GIVEN** 一个 CRC 有效的 metadata v5 世界
- **WHEN** 新程序首次打开该世界
- **THEN** 系统 MUST 读取既有种子、出生信息、世界时间、偏移、天气与维度表并把难度设为 `normal`，MUST NOT 改写磁盘上的旧文件，并在下一次正常保存时写为 metadata v6

#### Scenario: v6 往返保值

- **GIVEN** 服务端以 `hard` 难度正常保存世界
- **WHEN** 服务端重新打开同一世界
- **THEN** 读取到的难度 MUST 为 `hard`，其余 metadata 字段 MUST 与保存值逐字段一致

#### Scenario: 非法难度字节以损坏拒绝

- **GIVEN** 一份 CRC 有效但难度字节为 3 的 metadata v6
- **WHEN** 系统读取该 metadata
- **THEN** 读取 MUST 以损坏错误失败，MUST NOT 产生部分填充的世界状态

#### Scenario: 未来版本稳定拒绝

- **GIVEN** 世界 metadata 声明高于 v6 的版本
- **WHEN** 系统读取该 metadata
- **THEN** 读取 MUST 以未来版本错误失败，且该错误 MUST NOT 被当作损坏或迁移处理

### Requirement: 难度由服务端在构造时注入且生命周期内不可变

权威模拟 SHALL 在 Engine 构造时接收难度并在生命周期内只读；权威 tick MUST NOT 读取磁盘或配置获取难度，Memory 与 TCP MUST 复用同一注入路径与同一结算结果；非法难度 MUST 使构造稳定失败而非在 tick 内出现未定义分支。客户端 MUST NOT 持有、预测或上线难度字段，难度差异只经服务端行为（饥饿伤害、回血门控、夜行者生成）间接可观察。

#### Scenario: 重启与跨传输同难度结果

- **GIVEN** 两个相同种子、相同难度的世界分别经 Memory 与 TCP 装配并推进相同输入序列
- **WHEN** 各自完成同一脚本
- **THEN** 两者的饥饿、回血与夜行者生成结果 MUST 逐字段一致，保存重启后 MUST 延续同一难度行为

#### Scenario: 权威 tick 不读磁盘

- **GIVEN** 一个以 `hard` 难度装配并已运行的权威模拟
- **WHEN** 推进任意数量 tick
- **THEN** 难度语义 MUST 与构造时快照一致，MUST NOT 因磁盘上 metadata 的并发保存而改变本周期行为

#### Scenario: 客户端与协议不感知难度

- **GIVEN** 任意难度的世界
- **WHEN** 客户端登录并接收全部服务端消息
- **THEN** 协议 MUST NOT 出现任何难度字段，客户端状态 MUST NOT 含难度值

### Requirement: 专服难度在监听前完成一致性校验

专用服务端 SHALL 提供可选 `--difficulty`，仅接受三个小写合法值；省略时新世界创建为 `normal`、已有世界完全使用 metadata，显式时新世界使用该值、已有世界在 listener 创建前与 metadata 比较，不一致 MUST 失败且 MUST 关闭已打开的存储，MUST NOT 短暂暴露端口。图形客户端 MUST NOT 提供难度覆盖入口，误传参数 MUST 在解析阶段失败。

#### Scenario: 显式难度与已有世界冲突在监听前失败

- **GIVEN** 一个 metadata 难度为 `normal` 的已有世界
- **WHEN** 专服以 `--difficulty hard` 启动
- **THEN** 启动 MUST 失败并返回明确错误，已打开的存储 MUST 被关闭，MUST NOT 创建 listener

#### Scenario: 省略时沿用世界事实

- **GIVEN** 一个 metadata 难度为 `hard` 的已有世界
- **WHEN** 专服不带 `--difficulty` 启动
- **THEN** 服务端 MUST 以 `hard` 装配权威模拟并正常监听

#### Scenario: 非法难度参数被解析拒绝

- **GIVEN** 命令行传入 `--difficulty Nightmare`
- **WHEN** 专服解析参数
- **THEN** 解析 MUST 失败，MUST NOT 进入世界打开路径

