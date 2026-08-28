## ADDED Requirements

### Requirement: 存储世界按域拆分子包

仓库 MUST 将 `internal/storage` 组织为编排根包加
`internal/storage/storagedef`、`internal/storage/region`、
`internal/storage/chunk`、`internal/storage/player`、
`internal/storage/companion`、`internal/storage/hostile` 六个子包。根包 MUST
保留 `Store`/`WorldStore` 等接口与 disk/memory/world_files/backup/metadata/
chunk_keys 编排；`storagedef` MUST 只承载 `ErrCorrupt`/`ErrFutureVersion` 跨域
哨兵；`region` MUST 承载 region 容器与 `RegionKey`/`RegionFor`；`chunk`/
`player`/`companion`/`hostile` MUST 各自承载对应实体的 codec、迁移、类型、域
测试与版本化 bin fixture。

#### Scenario: 依赖图接受存储子包布局并拒绝反向边

- **GIVEN** 仓库包含 `internal/storage` 的全部六个子包
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** MUST 接受根包到六个子包的依赖边
- **AND** MUST 接受 chunk → region、实体域 → storagedef、region → storagedef
  等已登记的消费边
- **AND** MUST 拒绝任何子包指向根包的反向依赖与子包之间未登记的相互依赖
  （chunk → region 除外）

#### Scenario: region 容器经最小门面暴露

- **GIVEN** 根包 disk 编排需要打开、读写、压缩 region 容器并注入测试文件钩子
- **WHEN** 针对拆分后的仓库编译
- **THEN** `region` 包 MUST 经 `Region` 门面类型与 open/save/load/compact 入口
  及文件钩子类型暴露容器能力
- **AND** `region` 包导出面 MUST NOT 引用 chunk 包类型
- **AND** 根包 MUST 继续持有 region 缓存与 `ChunkKeys` 编排

### Requirement: 存储子包依赖方向单向

存储子包的依赖方向 MUST 单向且由架构依赖检查登记：根包 → 六个子包；
chunk → {region, storagedef, core, world}；
player/companion/hostile → {storagedef, core, world}；region → {storagedef,
core}。companion 存储域 MUST 经既有 `internal/companion` 边访问伙伴领域类型，
不得新增其他子包间依赖。

#### Scenario: archcheck 登记并强制存储依赖边

- **GIVEN** `internal/archcheck` 的依赖白名单已登记存储子包允许边
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** 实际存在的包间导入边 MUST 全部落在允许边集合内
- **AND** 向任何子包注入未登记依赖边（如 player → chunk、region → chunk）
  MUST 被拒绝

#### Scenario: companion 存储域沿用既有伙伴领域边

- **GIVEN** companion 存储域的 codec 引用伙伴领域类型
- **WHEN** 拆分完成并枚举依赖边
- **THEN** `internal/storage/companion` MUST 仅经既有的
  `internal/companion` 边访问伙伴领域类型
- **AND** 该边方向与拆分前保持一致

### Requirement: 存储别名再导出保持消费面与格式语义

拆分 MUST 保持全部既有消费方源码零改动：根包 MUST 以类型别名与错误值别名
再导出全部迁出符号，既有 `storage.X` 引用 MUST 继续以同一名称与类型身份解析；
`ErrCorrupt`/`ErrFutureVersion` 别名 MUST 与 `storagedef` 定义为同一错误值。
拆分 MUST NOT 改变 schema 号、迁移表、错误消息、原子替换路径或格式字节；
版本化 bin fixture MUST 逐字节随所属域包迁移。

#### Scenario: 消费方符号继续可寻址

- **GIVEN** `internal/server`、`cmd/mornlea/app`、`cmd/mornlea/benchmark`、
  `cmd/mornlea-server` 等消费方以 `storage.X` 引用迁出符号
- **WHEN** 拆分完成并编译全仓
- **THEN** 全部既有 `storage.X` 符号 MUST 继续以同一名称与类型身份解析
- **AND** 消费方源码 MUST 零改动

#### Scenario: 错误哨兵身份与 fixture 字节不变

- **GIVEN** 既有测试以 `errors.Is` 匹配 `ErrCorrupt`/`ErrFutureVersion` 并断言
  错误消息文本
- **WHEN** 拆分完成并运行存储测试
- **THEN** 哨兵别名 MUST 与 `storagedef` 定义为同一错误值且错误消息逐字节
  不变
- **AND** 全部版本化 bin fixture MUST 逐字节不变且继续被对应域测试加载

### Requirement: 存储分包保持测试入口集合

拆分 MUST 保持全部既有测试入口可寻址：六个包 `go test -list` 入口并集 MUST
等于拆分前 `internal/storage` 单包集合，测试函数名与 `t.Run` 标签逐一不变。
拆分后 MUST 能对单个域包定点运行测试而不编译执行其他域的测试。

#### Scenario: 入口并集与基线一致

- **GIVEN** 拆分前已持久化 `go test ./internal/storage -list '.*'` 全量快照
  （223 Test + 7 Benchmark + 4 Fuzz）
- **WHEN** 拆分完成并对 `./internal/storage/...` 各包取 `-list` 并集
- **THEN** Test、Benchmark、Fuzz 入口集合 MUST 与快照一致
- **AND** 子测试标签 MUST 逐一不变

#### Scenario: 实体域迭代不为其他域付费

- **GIVEN** 开发者修改单个实体域的 codec 或迁移逻辑
- **WHEN** 运行 `go test ./internal/storage/<域> -race`
- **THEN** MUST NOT 编译或执行其他域包的测试
- **AND** 每个域包 MUST 可单独定点运行
