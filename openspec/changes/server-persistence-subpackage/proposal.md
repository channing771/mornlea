## Why

`internal/server` currently mixes authoritative tick/login orchestration with four
independent storage lifecycles: world chunk and metadata saves, player saves,
companion saves, and hostile-mob saves. Extracting the storage-facing lifecycle
code gives each package a checkable owner and lets persistence tests run without
compiling unrelated session, publication, or AI orchestration paths.

## What Changes

- 新增 `internal/server/persistence`，唯一持有世界、玩家、伙伴和夜行者的存档
  加载、观察、异步保存、重试、flush 与 worker 生命周期。
- `internal/server` 保留 `Host`、`Server`、权威 tick、登录、会话、发布和关服
  编排；现有根包公开 API 继续可用，仅改为调用 persistence 子包。
- 迁移持久化实现及其白盒测试，保留全部 Test、Benchmark、Fuzz 入口及 `t.Run`
  标签。
- 在 `internal/archcheck` 登记单向依赖，在 `docs/architecture.md` 和子包
  `AGENTS.md` 记录所有权、依赖与并发边界。

## Non-Goals

- 不改变存档 schema、磁盘文件、协议、Rust ABI、client ABI 或任何玩法行为。
- 不改变 autosave 节奏、重试退避、背压、worker 数量、channel 容量、关服 flush
  顺序或错误处理。
- 不整理会话、发布、区块生成或伙伴/夜行者的权威 tick 编排。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `repository-code-organization`：建立 server 持久化子包的单向边界，并要求结构
  迁移保持既有持久化行为、根包 API 和测试入口。

## Impact

- 受影响生产包：`internal/server`、新增 `internal/server/persistence`。
- 受影响架构与文档：`internal/archcheck/dependency_test.go`、
  `docs/architecture.md`、新增 `internal/server/persistence/AGENTS.md`。
- 新子包只依赖现有低层领域与存储包；`internal/server` 可以依赖该子包，子包
  不得反向依赖 `internal/server`。
- 该变更是仓库内部 import path 和所有权整理；不需要协议、存档或并发迁移，
  不应改变性能门禁或运行时资源上限。
