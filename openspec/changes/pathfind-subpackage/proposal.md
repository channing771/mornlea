## Why

`internal/companion` 目前同时承载伙伴规划编排和可复用的有界体素寻路实现；后者
只依赖 `internal/core`，也被权威服务端需要。将其独立为 core-only 子包可以让
所有权和依赖方向可检查，同时避免后续调用方继续经由伙伴包取得通用寻路能力。

## What Changes

- **BREAKING** 将通用寻路实现迁入 `internal/pathfind`，由该包唯一拥有路径网格、
  路径结果、版本校验、错误、容量边界和路径策略。
- 在同一 change 内迁移全部内部调用方；`internal/companion` 与
  `internal/server` 直接使用 `internal/pathfind`。
- 不在 `internal/companion` 保留类型别名、转发函数或其他兼容层。
- 登记并验证 `internal/pathfind` 仅直接依赖 `internal/core` 的单向依赖边界。
- 保留既有寻路结果、修订校验、错误、资源上限、测试入口和 `t.Run` 标签；不改变
  玩法、协议、存档或 ABI。

## Capabilities

### New Capabilities

无。该变更不引入新的用户可观察能力。

### Modified Capabilities

- `repository-code-organization`：建立 core-only 寻路子包边界，并要求提取保持既有
  寻路行为和测试入口。

## Impact

- 受影响生产包：新增 `internal/pathfind`，以及 `internal/companion` 与
  `internal/server` 的内部调用。
- 受影响架构守卫：`internal/archcheck/dependency_test.go`。
- `PlanSnapshot.ChunkRevisions` 保留 `json:"chunkRevisions"` 字段名、值形状、排序和
  验证语义；不需要协议或存档迁移。
- 该变更只改变仓库内部 import path，不影响线上 wire、存档、Rust ABI、client ABI、
  并发边界或既有资源上限。
