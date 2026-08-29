## ADDED Requirements

### Requirement: 网络按域拆分子包

仓库 MUST 将 `internal/network` 组织为编排根包加 `internal/network/protocol`
、`internal/network/codec` 两个子包；`internal/network/tcp` MUST 保持现状
不动。根包 MUST 保留会话与传输编排（共享 packet stream 接口、登录状态机、
Memory transport、`ClientEndpoint`/`ServerEndpoint` 与 `ErrClosed`）；
`protocol` MUST 承载密封 `ClientPacket`/`ServerPacket` 接口、全部协议消息
DTO 与 `Validate`、冻结包 ID 表 `registry` 与区块 wire DTO `snapshot`，并
承载协议级校验 `ValidateDecodedClientWirePacket`（含 Handshake/Login 放行
路径）；`codec` MUST 承载 `Codec` 门面与双向分发、编码原语与帧封装。

#### Scenario: 依赖图接受网络子包布局并拒绝反向边

- **GIVEN** 仓库包含 `internal/network` 的全部子包
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** MUST 接受根包到 protocol 与 codec 的依赖边及既有
  internal/core 边
- **AND** MUST 接受 codec → protocol、protocol → internal/companion、
  tcp → network 等已登记的消费边
- **AND** MUST 拒绝任何子包指向根包的反向依赖、protocol → codec 边与根包
  到 tcp 的依赖

#### Scenario: 密封接口与冻结包 ID 表同处 protocol 包

- **GIVEN** `ClientPacket`/`ServerPacket` 以 unexported marker 密封且全部
  message DTO 与 snapshot 实现 marker，`registry` 与
  `CommandRejected.Validate` 双向引用包 ID 映射
- **WHEN** 针对拆分后的仓库编译
- **THEN** 密封接口、全部 message DTO、`ChunkSnapshot` wire DTO 与冻结包
  ID 表 MUST 位于同一 `protocol` 包
- **AND** `ValidateDecodedClientWirePacket` MUST 位于 `protocol` 包且
  Handshake/Login 放行路径原样保留
- **AND** 根包 MUST 继续持有登录状态机、Memory transport 与共享 stream
  接口

### Requirement: 网络子包依赖方向单向

网络子包的依赖方向 MUST 单向且由架构依赖检查登记：根包 → {protocol,
codec, internal/core}；protocol → {internal/core, internal/companion}；
codec → {protocol, internal/core}；tcp → 根包（既有边不动）。根包的
internal/companion 边 MUST 随 companion 消息文件移交 protocol 后移除；
protocol 与 codec 之间 MUST 仅存在 codec → protocol 单向边；子包 MUST
NOT 依赖根包。

#### Scenario: archcheck 增量登记并强制网络依赖边

- **GIVEN** `internal/archcheck` 的依赖白名单已按任务增量登记网络子包允许
  边
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** 实际存在的包间导入边 MUST 全部落在允许边集合内
- **AND** 向任何子包注入未登记依赖边（如 protocol → codec、codec → 根包）
  MUST 被拒绝

#### Scenario: companion 协议域沿用既有伙伴领域边

- **GIVEN** companion 消息文件的协议域代码引用伙伴领域类型
- **WHEN** 拆分完成并枚举依赖边
- **THEN** `internal/network/protocol` MUST 仅经既有的
  `internal/companion` 边访问伙伴领域类型
- **AND** 根包 MUST NOT 再保留 internal/companion 边，该边方向与拆分前
  保持一致

### Requirement: 网络别名再导出保持消费面与协议语义

拆分 MUST 保持消费方生产代码零改动：根包 MUST 以类型别名、常量、错误与
var 函数别名再导出全部迁出符号，既有 `network.X` 引用 MUST 继续以同一
名称与类型/错误/常量身份解析；类型别名 MUST 保持方法集（`msg.Validate`
继续可用）；留守根包的会话与传输 API MUST 保持原生定义。拆分 MUST NOT
改变 wire 字节、包 ID、协议语义、校验规则、错误语义、Memory/TCP 行为或
`ProtocolVersion`；`chunk-snapshot-v1.bin` fixture MUST 逐字节随 codec 包
迁移。Memory 与 TCP transport MUST 继续使用相同的登录状态机和 packet
校验契约。

#### Scenario: 消费方符号继续可寻址

- **GIVEN** `internal/server`、`internal/client`、`internal/sim`、
  `cmd/mornlea`、`cmd/mornlea/app`、`cmd/mornlea/benchmark`、
  `cmd/mornlea/capture`、`cmd/mornlea-server` 与 `internal/network/tcp` 以
  `network.X` 引用迁出符号
- **WHEN** 拆分完成并编译全仓
- **THEN** 全部既有 `network.X` 符号 MUST 继续以同一名称与类型/错误/常量
  身份解析
- **AND** 消费方生产代码与 tcp 生产代码 MUST 零改动

#### Scenario: 协议语义与 wire 字节不变

- **GIVEN** 既有 golden、fuzz 与 `chunk-snapshot-v1.bin` fixture 测试断言
  wire 字节、包 ID 与错误语义
- **WHEN** 拆分完成并运行网络测试
- **THEN** wire 字节、包 ID、校验规则与错误语义 MUST 逐项不变
- **AND** fixture MUST 逐字节不变且继续被 codec 包测试加载
- **AND** Memory 与 TCP transport 的既有 handshake、登录与 Play packet
  流行为 MUST 保持不变

### Requirement: 网络分包保持测试入口集合

拆分 MUST 保持全部既有测试入口可寻址：根包与 protocol/codec/tcp 各包
`go test -list` 入口并集 MUST 等于拆分前根包与 tcp 集合（根包 164 =
151 Test + 7 Benchmark + 6 Fuzz，tcp 33 Test），测试函数名与 `t.Run`
标签逐一不变。拆分后 MUST 能对单个子包定点运行测试而不编译执行其他子包
的测试。

#### Scenario: 入口并集与基线一致

- **GIVEN** 拆分前已持久化 `go test ./internal/network/... -list '.*'`
  全量快照（根包 164 + tcp 33 = 197 项）
- **WHEN** 拆分完成并对 `./internal/network/...` 各包取 `-list` 并集
- **THEN** 剥离快照的 `#` 分节行与空行后，Test、Benchmark、Fuzz 入口集合
  MUST 与快照逐名一致
- **AND** 子测试标签 MUST 逐一不变

#### Scenario: 单域迭代不为其他域付费

- **GIVEN** 开发者修改 protocol 或 codec 子包的协议消息或编解码逻辑
- **WHEN** 运行 `go test ./internal/network/<子包> -race`
- **THEN** MUST NOT 编译或执行其他子包的测试
- **AND** 每个子包 MUST 可单独定点运行
