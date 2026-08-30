# Pathfinding Subpackage Design

## Context

`internal/companion` 当前同时持有伙伴规划语义与通用的有界体素 A* 实现。
`pathfind.go`、`pathfind_policy.go` 及其测试只依赖 `internal/core`，并在不可变
网格快照上执行纯值计算。权威夜行者也需要相同能力，因此该实现不再属于伙伴
专属职责。

本次变更在已完成的 TCP 子包整理之后进行；在创建新的 OpenSpec change 前，先
归档 `network-tcp-subpackage`。A-04 夜行者变更处于开发中，必须在合并前 rebase
并迁移到新 import path。

## Goals

- 新建 `internal/pathfind`，作为有界、确定性体素路径计算的唯一所有者。
- 一次性迁移仓库内调用方，不在 `internal/companion` 保留类型别名或转发函数。
- 保持路径结果、错误、容量上限、确定性、JSON 字段及并发边界不变。
- 用架构守卫确保 `pathfind` 只依赖 `core`，并让 `companion` 单向依赖它。

## Non-Goals

- 不修改 A* 算法、移动规则、路径重算策略、网格尺寸或资源上限。
- 不改变伙伴计划协议、存档、线上 wire、Rust ABI 或 client ABI。
- 不在本变更中整理 `internal/render`、`internal/network` 的 Memory transport 或
  其他内部包。
- 不为 A-04 编写或合并夜行者功能；该分支只在其合并前完成 import rebase。

## Package Boundary

`internal/pathfind` 拥有以下纯计算值类型与函数：

- `PathCell`、`PathWindow`、`PathBlockTable`、`PathGrid` 和其构造函数。
- `ChunkRevision`、`PathResult`、`FindPath` 及所有路径错误和容量常量。
- `PathPolicy`、重验和重算冷却策略。

该包只导入 `internal/core`，不访问世界、存储、网络、伙伴状态或 goroutine。调用方
在权威 tick 边界生成不可变快照；`pathfind` 只读取该快照并返回值结果。

`internal/companion` 继续拥有 `PlanSnapshot`、Planner 和伙伴任务编排。其
`PlanSnapshot.ChunkRevisions` 字段改为 `[]pathfind.ChunkRevision`，保留原有
`json:"chunkRevisions"` tag、排序与验证语义。`internal/server` 及 A-04 后续直接
导入 `pathfind`，不再经由 `companion` 取得寻路类型或函数。

`internal/archcheck/dependency_test.go` 登记 `internal/pathfind -> internal/core`、
`internal/companion -> internal/pathfind` 和 `internal/server -> internal/pathfind`。
不登记反向边，也不引入兼容层。

## Migration

1. 把 `internal/companion/pathfind.go`、`pathfind_policy.go` 和
   `pathfind_test.go` 整体迁至 `internal/pathfind` 并改包名。
2. 将 `ChunkRevision` 与 `MaxPlanChunkRevisions` 从 `companion` 的 planner 类型
   定义迁入 `pathfind`，更新 `PlanSnapshot` 及其验证和测试引用。
3. 更新 `companion`、`server` 和其他仓库调用方的 import 与符号引用；全仓搜索
   确认不存在旧寻路 API 引用。
4. 增加包边界指南和 archcheck 白名单；归档前一个 TCP OpenSpec change，再为本次
   提取建立最小 OpenSpec change、delta spec、design、tasks 和 ledger。
5. A-04 合并前 rebase 并切换到 `pathfind`，不在本 change 内改动其玩法逻辑。

## Error Handling And Compatibility

错误值和错误文本随实现一起迁移，调用方继续以 `errors.Is` 或既有失败处理分支
判断。`NewPathGrid` 仍在读取任何方块前验证参数；路径计算继续不执行 I/O、不保留
调用方内存，也不引入共享可变状态。

这是仓库内部 import path 的破坏性变更，但所有调用方在同一 change 内迁移。由于
没有外部消费者，也不保留会掩盖所有权的兼容别名。`PlanSnapshot` 的序列化字段
名称、类型形状和值不变，故无需存档或协议迁移。

## Verification

- `go test ./internal/pathfind -race -count=1`
- `go test ./internal/companion ./internal/server -race -count=1`
- `go test ./internal/archcheck -count=1`
- `go vet ./...`
- `go test ./... -race -p=1 -count=1`
- `openspec validate --all --strict --no-interactive`

测试迁移保留现有 Test、Benchmark、Fuzz 名称和子测试标签。最终全仓搜索旧
`companion` 寻路符号，确保没有兼容转发或遗漏调用方。

## Risks And Rollback

最大风险是 A-04 的未合并分支仍引用旧 API。通过明确 rebase 责任、无别名迁移和
合并前 focused race 测试控制该风险。若迁移引入问题，回退本次单独的结构 change
即可恢复原始 import path；无需运行时数据修复或兼容迁移。
