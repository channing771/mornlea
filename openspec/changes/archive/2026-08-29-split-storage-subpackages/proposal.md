# split-storage-subpackages

## Why

`internal/storage` 以单一包平铺 65 个 Go 文件（约 1.86 万行，其中 42 个测试
文件），世界编排（disk/memory/world_files/backup/metadata/chunk_keys）、region
容器与 chunk/player/companion/hostile 四个实体的 codec、迁移和类型共享同一命名
空间，包内边界只靠文件名前缀约定。后果：

1. 测试无法定点：整包 `-race` 实测 24.2s，迭代单个实体 codec 也要为其余三个
   实体域与 region 容器的故障注入、crash 恢复、压缩等重型测试付费，违反测试
   分层纪律的意图。
2. 职责边界不可检查：`internal/archcheck` 只能约束整包 `internal/storage` 的
   依赖边，域与域之间的耦合（如实体 codec 对 region 容器的内部调用）不可见。

拆出域子包后，`go test ./internal/storage/player` 等定点命令不再编译执行其他
域的测试，与 T0–T3 分层及 `race-changed` 改动闭包天然配合。

## What Changes

- **BREAKING（仓库内部源码结构）** 将 `internal/storage` 拆为根包加六个子包：
  - 根包保留 `Store`/`WorldStore` 等接口、`disk`/`memory`/`world_files`/
    `backup`/`metadata`/`chunk_keys` 编排，并以别名再导出迁出符号；
  - `internal/storage/storagedef`：`ErrCorrupt`/`ErrFutureVersion` 跨域哨兵叶子；
  - `internal/storage/region`：region 格式原语（superblock/bank 编解码、扇区
    空间分配）与 `RegionKey`/`RegionFor`；
  - `internal/storage/chunk`：chunk 信封编解码、迁移、chunk 值类型与 region
    记录层容器（现 `*region` 及其读写、压缩、崩溃恢复）；
  - `internal/storage/player`、`internal/storage/companion`、
    `internal/storage/hostile`：各实体 codec、迁移、类型、域测试与版本化
    bin fixture。
- 依赖方向单向并由 `internal/archcheck` 登记：根包 → region/chunk/player/
  companion/hostile 五个域子包与 storagedef 叶子（经错误别名消费哨兵）；
  chunk → region；实体域 → storagedef；region → storagedef；子包之间不得互相
  导入（chunk → region 除外）；companion 域保留既有 `internal/companion` 边。
- 消费面零改动：`internal/server`、`cmd/mornlea/app`、`cmd/mornlea/benchmark`、
  `cmd/mornlea-server` 等消费方的生产代码不得因拆分而修改，既有 `storage.X`
  符号引用与类型身份不变；消费方测试因 fixture 随域包迁移所需的相对路径更新
  不在此限。根包别名再导出保证既有 `storage.X` 符号全部继续可寻址，
  `ErrCorrupt`/`ErrFutureVersion` 别名与 `storagedef` 定义为同一错误值。
- 测试入口并集不变：拆分前 `go test ./internal/storage -list '.*'` 的
  223 Test + 7 Benchmark + 4 Fuzz 逐名冻结进基线，拆分后各子包 `-list` 并集与
  基线完全一致；测试函数名与 `t.Run` 标签逐一不变。
- 版本化 bin fixture（22 个）随所属域包 `git mv` 迁移，内容逐字节不变。
- 文档同步：`internal/storage/AGENTS.md` 重写为子包地图与依赖方向总纲，五个
  域子包各一份 AGENTS.md；`.github/workflows/ci.yml` 存档测试分片与
  `docs/notes/test-quickstart.md` 定点命令改用 `./internal/storage/...`。
- 不合并任何域、不改变任何格式、schema 号、迁移表、错误消息与错误语义、原子
  替换路径或存档字节。

## Capabilities

### New Capabilities

无。该变更只建立世界存储的代码组织边界，不引入新的用户可观察能力。

### Modified Capabilities

- `repository-code-organization`：为世界存储建立根包 + 六子包布局（region 只收
  格式原语，chunk 承载记录层容器），要求依赖方向单向、别名再导出保持消费面、
  测试入口集合保持不变。

## Impact

- 受影响生产包：`internal/storage`（缩为编排根包）与新增
  `internal/storage/{storagedef,region,chunk,player,companion,hostile}`。
- 消费方生产代码零改动：`internal/server`、`cmd/mornlea/app`、
  `cmd/mornlea/benchmark`、`cmd/mornlea-server` 对 `storage.X` 的既有引用保持
  不变（别名再导出承接）；消费方测试仅 fixture 相对路径随域迁移适配
  （如 `internal/server` 的 player golden 路径）。
- 受影响架构守卫：`internal/archcheck/dependency_test.go` 登记新包与允许边。
- 受影响资产：`internal/storage/testdata` 的 22 个版本化 bin 按域随包
  `git mv`（git 保留历史）。
- 受影响文档：`internal/storage/AGENTS.md` 及五个子包 AGENTS.md、
  `.github/workflows/ci.yml`、`docs/notes/test-quickstart.md`。
- 这是仓库内部源码 import path 的变更；无线上 wire、存档、协议或 ABI 兼容性
  影响。race/非 race 计时只记录，不改变任何退出状态。
