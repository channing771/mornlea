## ADDED Requirements

### Requirement: 仓库按独立单元收纳于 packages

仓库 MUST 把全部独立功能单元收纳在 `packages/` 之下：Go 单元
`packages/shared`、`packages/server`、`packages/client`、`packages/tools`、
`packages/audit`、`packages/contracts`，Rust workspace `packages/engine`，
以及 Agent 服务族 `packages/agent`（现有 `packages/agent/companion` Python
服务）。顶层 MUST 只保留全局内容：`go.work`、`Makefile`、`.github/`、
`docs/`、`openspec/`、`scripts/`、`testdata/` 与根级说明、许可和 ignore
文件；顶层 MUST NOT 再出现 `internal/`、`cmd/`、`services/`、`web/`、
`contracts/` 或 `engine/`。每个单元 MUST 可在其目录内独立构建与测试。

#### Scenario: 顶层仅含全局目录

- **GIVEN** 单元化重组完成后的仓库根目录
- **WHEN** 枚举顶层目录
- **THEN** MUST 只出现全局目录与文件（`packages/`、`go.work`、`Makefile`、
  `.github/`、`docs/`、`openspec/`、`scripts/`、`testdata/`、根级
  README/LICENSE/AGENTS.md/CLAUDE.md/.gitignore 等）
- **AND** MUST NOT 出现 `internal/`、`cmd/`、`services/`、`web/`、
  `contracts/`、`engine/` 等单元目录

#### Scenario: 单元可独立构建与测试

- **GIVEN** 工作区已按 `go.work` 解析本地模块、Rust cdylib 与 Python venv
  就绪
- **WHEN** 分别在 `packages/server`、`packages/client`、`packages/tools`
  运行 `go build ./...` 与 `go test ./...`，在 `packages/engine` 运行
  `cargo build`/`cargo test`，在 `packages/agent/companion` 运行
  `uv run pytest`
- **THEN** 每个单元 MUST 在自身目录内完成构建与测试，MUST NOT 要求先进入
  其他单元目录执行手工步骤

#### Scenario: Agent 服务族可扩展

- **GIVEN** `packages/agent/companion` 是现有唯一 Agent 服务
- **WHEN** 新增另一个独立 Agent 服务
- **THEN** 它 MUST 以 `packages/agent/<名称>` 并列入驻，MUST NOT 挪动
  companion 或其他单元

### Requirement: Go 单元模块边界双层强制

仓库 MUST 以根 `go.work` 直辖六个 Go 模块（shared、server、client、tools、
audit、contracts），根目录 MUST NOT 保留 `go.mod`。跨单元引用 MUST 经
go.mod require 声明，且仅允许以下方向：`server → {shared, contracts}`（生产
代码）、`client → {shared}`、`tools → {shared, server, client, contracts}`、
`audit` 与 `contracts` MUST NOT require 任何兄弟单元。唯一例外：`server`
MAY 仅因测试文件（客户端镜像驱动的 Memory/TCP 集成测试）require `client`，
其生产代码 MUST NOT import `packages/client` 的任何包，该禁令由架构检查以
源码级守卫强制。
`packages/audit` 的架构检查 MUST 枚举各单元真实 import 边并强制同一方向
契约：`client` 的任何包 MUST NOT 依赖 `server` 或 `tools`，`server` 的生产
包 MUST NOT 依赖 `client`、`tools`，`shared` MUST NOT 依赖 server、client、
tools 的任何包。

#### Scenario: 编译器拒绝未声明的跨单元导入

- **GIVEN** `packages/client` 的某包源码导入 `packages/server` 的包
- **WHEN** 在 workspace 下构建 `packages/client`
- **THEN** 构建 MUST 因未声明 require 而失败
- **AND** 即使补上 go.mod require，`packages/audit` 的单元边界检查 MUST
  因 `client → server` 违反方向契约而失败

#### Scenario: server 生产代码不得导入 client

- **GIVEN** `packages/server` 的某个非 `_test.go` 文件导入
  `packages/client` 的包
- **WHEN** 架构检查扫描 server 生产源码
- **THEN** 该导入 MUST 被拒绝
- **AND** `_test.go` 文件对 `packages/client` 的导入 MUST 被放行（客户端
  镜像驱动的集成测试）

#### Scenario: 架构检查跨模块枚举真实依赖边

- **GIVEN** `packages/audit` 的依赖白名单已按单元路径前缀登记
- **WHEN** 架构检查枚举 `packages/` 下各模块的生产 import
- **THEN** 实际存在的内部依赖边 MUST 全部落在允许边集合内
- **AND** 向任何单元注入未登记依赖边 MUST 被拒绝，且该拒绝 MUST 有合成
  drift 测试证明检查器本身有效

#### Scenario: 审计单元不反向耦合被审单元

- **GIVEN** `packages/audit` 只通过 `go list` 与源码 AST 观察被审单元
- **WHEN** 枚举 `packages/audit` 自身的导入
- **THEN** 它 MUST NOT 导入 shared、server、client、tools、contracts 的
  任何生产包

### Requirement: 单元迁移保持测试入口与产物集合

单元化迁移 MUST 保持全部既有测试入口可寻址：各单元 `go test -list` 的
Test、Benchmark、Fuzz 入口并集与迁移前全仓集合一致，`t.Run` 标签逐一
不变；迁移 MUST 以 `git mv` 保历史，golden PNG、版本化 bin fixture 与
dist 产物 MUST 逐字节不变；wire bytes、协议版本、存档 schema、engine
ABI、client ABI 与 benchmark scenario MUST 不变。

#### Scenario: 测试入口并集与迁移前一致

- **GIVEN** 迁移前已持久化全仓 `go test ./... -list '.*'` 快照
- **WHEN** 迁移后对各 Go 单元分别 `go test -list` 取并集
- **THEN** Test、Benchmark、Fuzz 入口集合 MUST 与快照一致
- **AND** `t.Run` 标签 MUST 逐一不变

#### Scenario: 视觉 golden 与 fixture 字节不变

- **GIVEN** `testdata/visual-golden` 的世界与 UI 基线、各包版本化 bin
  fixture 已随单元迁移
- **WHEN** 运行 `make visual-check` 与受影响包测试
- **THEN** 全部场景 MUST 通过 tracked golden 且退出 0
- **AND** MUST NOT 产生 `*-actual.png`/`*-diff.png`，MUST NOT 修改、放宽
  或重新生成任何基线

## MODIFIED Requirements

### Requirement: TCP transport 具有单向包边界

仓库 MUST 将 TCP listener、dial、stream、deadline 和 socket 生命周期实现置于
`packages/shared/network/tcp`。`packages/shared/network/tcp` MUST 依赖
`packages/shared/network`，而 `packages/shared/network` MUST NOT 依赖
`packages/shared/network/tcp`。根包 MUST 保留登录和应用装配使用的共享
packet stream 接口。

#### Scenario: 依赖图接受 TCP 子包
- **GIVEN** 仓库包含 `packages/shared/network/tcp`
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** 它 MUST 接受 `packages/shared/network/tcp -> packages/shared/network`
- **AND** 它 MUST 拒绝从 `packages/shared/network` 到 `packages/shared/network/tcp` 的任何反向依赖

#### Scenario: TCP 构造器由子包消费
- **GIVEN** 应用或测试需要创建 TCP listener 或 dial TCP stream
- **WHEN** 它针对整理后的仓库编译
- **THEN** 它 MUST 经由 `packages/shared/network/tcp` 解析构造器
- **AND** 返回值 MUST 满足共享根包的 listener 或 packet stream 接口

### Requirement: 客户端命令按功能域分包

仓库 MUST 将 `packages/client/cmd/mornlea` 组织为薄 `package main`（CLI
解析与装配入口）加 `app`、`capture`、`benchmark`、`devcapture` 四个
功能域子包。依赖方向 MUST 为：main → app/capture/benchmark/devcapture；
capture、benchmark 与 devcapture → app。app MUST NOT 导入 capture、
benchmark 或 devcapture；capture、benchmark 与 devcapture MUST NOT 相互
导入。

#### Scenario: 依赖图接受客户端子包并拒绝反向边

- **GIVEN** 仓库包含 `packages/client/cmd/mornlea/app`、
  `packages/client/cmd/mornlea/capture`、
  `packages/client/cmd/mornlea/benchmark` 与
  `packages/client/cmd/mornlea/devcapture`
- **WHEN** 架构依赖检查枚举全部包
- **THEN** MUST 接受 main → app、main → capture、main → benchmark、
  main → devcapture 与 capture/benchmark/devcapture → app
- **AND** MUST 拒绝 app → capture、app → benchmark、app → devcapture 与
  capture/benchmark/devcapture 之间任何方向的依赖边

#### Scenario: 跨包访问经由消费端接口

- **GIVEN** capture 或 benchmark 需要读写 app 域状态（菜单、设置、面板、
  相机、渲染器等）
- **WHEN** 其编译 against 迁移后的仓库
- **THEN** 所需能力 MUST 通过各自包内定义的接口表达，由 `app.Application`
  实现
- **AND** app 包 MUST NOT 为 capture/benchmark 导出全量内部字段

### Requirement: golden 基线统一于仓库根 testdata

视觉基线 MUST 统一位于仓库根 `testdata/visual-golden/`（`world/` 与
`ui/` 两个子目录），Go 世界场景基线由
`packages/client/cmd/mornlea/capture` 的 golden 目录常量引用，前端 UI
部件基线由 `packages/engine/crates/mornlea_client/frontend` 的视觉
harness 引用。基线 PNG 内容 MUST 逐字节不变，capture 子包 MUST NOT 再
持有自有 golden 目录。

#### Scenario: 视觉门禁在迁移后保持全绿

- **GIVEN** golden PNG 位于 `testdata/visual-golden` 且两侧引用路径已同步
- **WHEN** 运行 `make visual-check` 与 `make frontend-visual-check`
- **THEN** 全部场景 MUST 通过 tracked golden 且退出 0
- **AND** MUST NOT 产生 `*-actual.png` 或 `*-diff.png`
- **AND** MUST NOT 修改、放宽或重新生成任何 golden 图像

### Requirement: 架构守卫覆盖客户端子包子树

针对 `packages/client/cmd/mornlea` 生产源码的字符串级架构守卫（登录路径
守卫、benchmark TCP 路径守卫等）MUST 扫描该完整子树（含子包），不得因
源码迁入子包而丢失覆盖。

#### Scenario: 源码守卫随子包继续生效

- **GIVEN** 登录装配与 benchmark TCP 路径的生产源码已迁入子包
- **WHEN** 运行对应源码守卫
- **THEN** 守卫 MUST 继续要求 `network.NewMemoryStreamPair`、
  `network.LoginClient`、`networktcp.ListenTCP(` 等既有模式
- **AND** 守卫 MUST 继续拒绝 `server.NewEmbedded(`、`server.New(` 等违规模式

### Requirement: 存储世界按域拆分子包

仓库 MUST 将 `packages/server/storage` 组织为编排根包加
`storagedef`、`region`、`chunk`、`player`、`companion`、`hostile` 六个
子包。根包 MUST 保留 `Store`/`WorldStore` 等接口与 disk/memory/world_files/
backup/metadata/chunk_keys 编排；`storagedef` MUST 只承载
`ErrCorrupt`/`ErrFutureVersion` 跨域哨兵；`region` MUST 承载 region 格式
原语（superblock/bank 编解码、扇区空间分配）与 `RegionKey`/`RegionFor`；
`chunk` MUST 承载 chunk 信封编解码、迁移、chunk 值类型与 region 记录层
容器（现 `*region` 及其读写、压缩、崩溃恢复）；`player`/`companion`/
`hostile` MUST 各自承载对应实体的 codec、迁移、类型、域测试与版本化
bin fixture。

#### Scenario: 依赖图接受存储子包布局并拒绝反向边

- **GIVEN** 仓库包含 `packages/server/storage` 的全部六个子包
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** MUST 接受根包到 storagedef 叶子与五个域子包的依赖边
- **AND** MUST 接受 chunk → region、实体域 → storagedef、region → storagedef
  等已登记的消费边
- **AND** MUST 拒绝任何子包指向根包的反向依赖与子包之间未登记的相互依赖
  （chunk → region 除外）

#### Scenario: region 记录层随 chunk 包且 region 包只收格式原语

- **GIVEN** `region.go` 的读写路径直接调用 chunk 信封编解码并经手 chunk 值
  类型，属于 chunk 的记录层容器
- **WHEN** 针对拆分后的仓库编译
- **THEN** 记录层容器（现 `*region` 及其 open/load/save/sync/close/compact
  入口与文件注入钩子）MUST 位于 `chunk` 包，并由 chunk 包导出容器类型供根包
  缓存编排
- **AND** `region` 包 MUST 只承载格式原语与 `RegionKey`/`RegionFor`，MUST NOT
  承载 chunk 信封编解码或容器文件编排
- **AND** 根包 MUST 继续持有容器缓存与 `ChunkKeys` 编排

### Requirement: 存储子包依赖方向单向

存储子包的依赖方向 MUST 单向且由架构依赖检查登记：根包 →
region/chunk/player/companion/hostile 五个域子包与 storagedef 叶子（经错误
别名消费哨兵）；chunk → {region, storagedef, shared/core, shared/world}；
player/companion/hostile → {storagedef, shared/core, shared/world}；
region → {storagedef, shared/core}。companion 存储域 MUST 经既有
`shared/companion` 边访问伙伴领域类型，不得新增其他子包间依赖。

#### Scenario: archcheck 登记并强制存储依赖边

- **GIVEN** `packages/audit` 的依赖白名单已登记存储子包允许边
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** 实际存在的包间导入边 MUST 全部落在允许边集合内
- **AND** 向任何子包注入未登记依赖边（如 player → chunk、region → chunk）
  MUST 被拒绝

#### Scenario: companion 存储域沿用既有伙伴领域边

- **GIVEN** companion 存储域的 codec 引用伙伴领域类型
- **WHEN** 拆分完成并枚举依赖边
- **THEN** `packages/server/storage/companion` MUST 仅经既有的
  `packages/shared/companion` 边访问伙伴领域类型
- **AND** 该边方向与拆分前保持一致

### Requirement: 存储别名再导出保持消费面与格式语义

拆分 MUST 保持消费方生产代码零改动：根包 MUST 以类型别名与错误值别名再导出
全部迁出符号，既有 `storage.X` 引用 MUST 继续以同一名称与类型身份解析；
`ErrCorrupt`/`ErrFutureVersion` 别名 MUST 与 `storagedef` 定义为同一错误值；
消费方测试因版本化 bin fixture 随所属域包迁移所需的 fixture 相对路径更新
MUST NOT 视为消费方改动。拆分 MUST NOT 改变 schema 号、迁移表、错误消息、
原子替换路径或格式字节；版本化 bin fixture MUST 逐字节随所属域包迁移。
单元化搬迁时，消费方（`packages/server/server`、
`packages/client/cmd/mornlea` 的 app/benchmark、
`packages/server/cmd/mornlea-server` 等）MUST 仅发生 import path 改写，
不发生符号或逻辑改动。

#### Scenario: 消费方符号继续可寻址

- **GIVEN** 消费方以 `storage.X` 引用迁出符号
- **WHEN** 拆分与单元化搬迁完成并编译全仓
- **THEN** 全部既有 `storage.X` 符号 MUST 继续以同一名称与类型身份解析
- **AND** 消费方生产代码 MUST 仅出现 import path 改写
- **AND** 消费方测试仅因 fixture 随域包迁移更新的 fixture 相对路径 MUST NOT
  视为消费方改动

#### Scenario: 错误哨兵身份与 fixture 字节不变

- **GIVEN** 既有测试以 `errors.Is` 匹配 `ErrCorrupt`/`ErrFutureVersion` 并断言
  错误消息文本
- **WHEN** 拆分完成并运行存储测试
- **THEN** 哨兵别名 MUST 与 `storagedef` 定义为同一错误值且错误消息逐字节
  不变
- **AND** 全部版本化 bin fixture MUST 逐字节不变且继续被对应域测试加载

### Requirement: 存储分包保持测试入口集合

拆分 MUST 保持全部既有测试入口可寻址：六个包 `go test -list` 入口并集 MUST
等于拆分前 `packages/server/storage` 单包集合，测试函数名与 `t.Run` 标签
逐一不变。拆分后 MUST 能对单个域包定点运行测试而不编译执行其他域的测试。

#### Scenario: 入口并集与基线一致

- **GIVEN** 拆分前已持久化 `go test ./internal/storage -list '.*'` 全量快照
- **WHEN** 拆分与单元化完成并对 `./packages/server/storage/...` 各包取
  `-list` 并集
- **THEN** Test、Benchmark、Fuzz 入口集合 MUST 与快照一致
- **AND** 子测试标签 MUST 逐一不变

#### Scenario: 实体域迭代不为其他域付费

- **GIVEN** 开发者修改单个实体域的 codec 或迁移逻辑
- **WHEN** 运行 `go test ./packages/server/storage/<域> -race`
- **THEN** MUST NOT 编译或执行其他域包的测试
- **AND** 每个域包 MUST 可单独定点运行

### Requirement: 网络按域拆分子包

仓库 MUST 将 `packages/shared/network` 组织为编排根包加
`packages/shared/network/protocol`、`packages/shared/network/codec` 两个
子包；`packages/shared/network/tcp` MUST 保持现状不动。根包 MUST 保留会话
与传输编排（共享 packet stream 接口、登录状态机、Memory transport、
`ClientEndpoint`/`ServerEndpoint` 与 `ErrClosed`）；`protocol` MUST 承载
密封 `ClientPacket`/`ServerPacket` 接口、全部协议消息 DTO 与 `Validate`、
冻结包 ID 表 `registry` 与区块 wire DTO `snapshot`，并承载协议级校验
`ValidateDecodedClientWirePacket`（含 Handshake/Login 放行路径）；`codec`
MUST 承载 `Codec` 门面与双向分发、编码原语与帧封装。

#### Scenario: 依赖图接受网络子包布局并拒绝反向边

- **GIVEN** 仓库包含 `packages/shared/network` 的全部子包
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** MUST 接受根包到 protocol 与 codec 的依赖边及既有
  shared/core 边
- **AND** MUST 接受 codec → protocol、protocol → shared/companion、
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
codec, shared/core}；protocol → {shared/core, shared/companion}；
codec → {protocol, shared/core}；tcp → 根包（既有边不动）。根包的
companion 边 MUST 随 companion 消息文件移交 protocol 后移除；protocol 与
codec 之间 MUST 仅存在 codec → protocol 单向边；子包 MUST NOT 依赖根包。

#### Scenario: archcheck 增量登记并强制网络依赖边

- **GIVEN** `packages/audit` 的依赖白名单已按单元路径登记网络子包允许边
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** 实际存在的包间导入边 MUST 全部落在允许边集合内
- **AND** 向任何子包注入未登记依赖边（如 protocol → codec、codec → 根包）
  MUST 被拒绝

#### Scenario: companion 协议域沿用既有伙伴领域边

- **GIVEN** companion 消息文件的协议域代码引用伙伴领域类型
- **WHEN** 拆分完成并枚举依赖边
- **THEN** `packages/shared/network/protocol` MUST 仅经既有的
  `packages/shared/companion` 边访问伙伴领域类型
- **AND** 根包 MUST NOT 再保留 companion 边，该边方向与拆分前保持一致

### Requirement: 网络别名再导出保持消费面与协议语义

拆分 MUST 保持消费方生产代码零改动：根包 MUST 以类型别名、常量、错误与
var 函数别名再导出全部迁出符号，既有 `network.X` 引用 MUST 继续以同一
名称与类型/错误/常量身份解析；类型别名 MUST 保持方法集（`msg.Validate`
继续可用）；留守根包的会话与传输 API MUST 保持原生定义。拆分 MUST NOT
改变 wire 字节、包 ID、协议语义、校验规则、错误语义、Memory/TCP 行为或
`ProtocolVersion`；`chunk-snapshot-v1.bin` fixture MUST 逐字节随 codec 包
迁移。Memory 与 TCP transport MUST 继续使用相同的登录状态机和 packet
校验契约。单元化搬迁时，消费方（`packages/server/server`、
`packages/client/client`、`packages/server/sim`、
`packages/client/cmd/mornlea` 各子包、`packages/server/cmd/mornlea-server`
与 `packages/shared/network/tcp`）MUST 仅发生 import path 改写。

#### Scenario: 消费方符号继续可寻址

- **GIVEN** 消费方以 `network.X` 引用迁出符号
- **WHEN** 拆分与单元化搬迁完成并编译全仓
- **THEN** 全部既有 `network.X` 符号 MUST 继续以同一名称与类型/错误/常量
  身份解析
- **AND** 消费方生产代码与 tcp 生产代码 MUST 仅出现 import path 改写

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
`go test -list` 入口并集 MUST 等于拆分前根包与 tcp 集合，测试函数名与
`t.Run` 标签逐一不变。拆分后 MUST 能对单个子包定点运行测试而不编译执行
其他子包的测试。

#### Scenario: 入口并集与基线一致

- **GIVEN** 拆分前已持久化 `go test ./internal/network/... -list '.*'`
  全量快照（根包 164 + tcp 33 = 197 项）
- **WHEN** 拆分与单元化完成并对 `./packages/shared/network/...` 各包取
  `-list` 并集
- **THEN** 剥离快照的 `#` 分节行与空行后，Test、Benchmark、Fuzz 入口集合
  MUST 与快照逐名一致
- **AND** 子测试标签 MUST 逐一不变

#### Scenario: 单域迭代不为其他域付费

- **GIVEN** 开发者修改 protocol 或 codec 子包的协议消息或编解码逻辑
- **WHEN** 运行 `go test ./packages/shared/network/<子包> -race`
- **THEN** MUST NOT 编译或执行其他子包的测试
- **AND** 每个子包 MUST 可单独定点运行

### Requirement: Server persistence has a one-way package boundary

仓库 MUST 将世界区块与 metadata、玩家、伙伴和夜行者的存档加载、观察、异步
保存、重试、flush 及 worker 生命周期置于
`packages/server/server/persistence`。根 `packages/server/server` MUST
保留 Host、Server、权威 tick、登录、会话、发布和关服编排，并可以依赖该
子包；`packages/server/server/persistence` MUST NOT 反向依赖根包。

#### Scenario: 架构守卫接受唯一允许的父子依赖

- **GIVEN** 仓库包含 `packages/server/server` 与
  `packages/server/server/persistence`
- **WHEN** 架构依赖检查枚举全部内部生产包
- **THEN** MUST 接受 `packages/server/server -> packages/server/server/persistence`
- **AND** MUST 拒绝 `packages/server/server/persistence -> packages/server/server` 或任何未登记的内部依赖

#### Scenario: 持久化职责不留在根包

- **GIVEN** 世界、玩家、伙伴和夜行者都需要其既有存档生命周期
- **WHEN** 根 Server 或 Host 执行 tick、登录、登出或关服
- **THEN** 对应存档生命周期 MUST 由 `packages/server/server/persistence` 的单一所有者执行
- **AND** 根包 MUST NOT 保留第二套保存队列、重试状态或 worker 实现

### Requirement: 权威模拟具有单向子包与共享调参值

仓库 MUST 将权威模拟组织为 `packages/server/sim/contract`、
`packages/server/sim/realm`、`packages/server/sim/entity` 与
`packages/server/sim/runtime` 四个子包，调参值对象 MUST 位于
`packages/shared/tuning` 供模拟与配置双侧消费。根
`packages/server/sim` 不得保留生产 Go package、类型别名、转发函数或
兼容 facade。所有内部调用方 MUST 直接导入其所消费值或行为的所属子包。

`contract` MUST 不依赖 `realm`、`entity` 或 `runtime`；`realm` MUST
不依赖 `entity` 或 `runtime`；`entity` MAY 依赖 `contract` 与 `realm`，
但 `runtime` 是唯一允许同时编排其余三者与 `shared/tuning` 的模拟包。
所有直接依赖 MUST 由架构检查登记并验证。

#### Scenario: 架构检查接受约定方向并拒绝反向边
- **GIVEN** 仓库包含四个权威模拟子包与 `packages/shared/tuning`
- **WHEN** 架构依赖检查枚举全部内部生产包
- **THEN** 它 MUST 接受 `runtime` 对 `contract`、`realm`、`entity` 与
  `shared/tuning` 的编排依赖
- **AND** 它 MUST 拒绝 `contract` 对上层模拟包的依赖、`realm` 对
  `entity` 或 `runtime` 的依赖，以及 `entity` 对 `runtime` 的依赖

#### Scenario: 迁移后的调用方不经过根包兼容层
- **GIVEN** 既有 server、config 与客户端装配调用方已迁移
- **WHEN** 仓库编译其内部生产包
- **THEN** 它们 MUST 直接解析 `runtime.Engine`、`contract` 值或
  `shared/tuning` 值
- **AND** 不得存在可编译的生产 `packages/server/sim` facade、类型别名或
  转发 API

### Requirement: 子包迁移保持测试入口

迁移前持久化的 `packages/server/sim` Test、Benchmark、Fuzz 入口与
`t.Run` 标签 MUST 在迁移后仍可从 `packages/server/sim/...` 子包集合中
逐项取得（`tuning` 相关入口随包落在 `packages/shared/tuning`）。白盒
测试 MUST 与其生产所有者一同迁移，不得为访问旧私有状态而增加生产导出
API。

#### Scenario: 子包测试入口并集等于迁移前清单
- **GIVEN** 迁移前已保存 `internal/sim` 的 Test、Benchmark、Fuzz 入口和
  `t.Run` 标签清单
- **WHEN** 迁移后枚举 `packages/server/sim/...` 与
  `packages/shared/tuning` 的测试入口
- **THEN** 子包入口并集与每个 `t.Run` 标签 MUST 与迁移前清单完全一致
- **AND** 每个子包 MUST 能独立运行其 focused race 测试而不依赖已删除的
  根 `sim` package

### Requirement: 寻路实现是独立内部包

仓库 MUST 将可复用的有界寻路值与算法作为独立包提供，并在提取时保持既有
寻路行为与测试入口不变。

#### Scenario: pathfind owns reusable pathfinding values and algorithms
- **GIVEN** the repository builds its internal packages
- **WHEN** callers construct a path grid or execute a path search
- **THEN** they MUST use `packages/shared/pathfind` values and functions
- **AND** `packages/shared/pathfind` MUST directly depend only on
  `packages/shared/core`

#### Scenario: pathfinding extraction preserves existing behavior
- **GIVEN** the pre-extraction companion and server test entry inventory
- **WHEN** the package extraction is complete
- **THEN** existing Test, Benchmark, Fuzz names and `t.Run` labels MUST remain available
- **AND** path results, revision validation, errors and bounded resource behavior MUST remain unchanged

### Requirement: 伙伴 Agent 服务具有单向分层边界

仓库 SHALL 在 `packages/agent/companion` 提供独立 Python 服务，并把可发布
的 Agent harness/domain 与 FastAPI app、模型适配、MCP adapter、memory
storage 分离。依赖方向 MUST 是 app/CLI → harness/domain 与 storage
adapter → domain；domain 和 graph factory MUST NOT依赖 FastAPI、Uvicorn、
Go server 包或 Mornlea 世界实现。Go 生产代码 MUST 只通过 Agent HTTP
contract 与 MCP handler 交互，MUST NOT通过 Python FFI、shell 或嵌入解释器
调用服务。

#### Scenario: graph harness 可脱离 HTTP 测试

- **GIVEN** 测试提供 fake model、fake MCP 与临时 memory adapter
- **WHEN** 直接构造并运行 Planner 或 Dialogue graph
- **THEN** 测试 MUST 不启动 FastAPI/Uvicorn、不连接真实 Go 世界，并得到与 HTTP app 相同的严格 domain 输出

#### Scenario: app 不成为 domain 依赖

- **GIVEN** 架构检查枚举 Python import 与 Go/Python 边界
- **WHEN** domain/graph factory 导入 FastAPI、Uvicorn 或 Go 通过 shell/FFI 调用 Python
- **THEN** 门禁 MUST 失败；app 到 harness 的单向组合与 Go HTTP/MCP 边界 MUST 被接受
