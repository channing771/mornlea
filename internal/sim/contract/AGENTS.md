# contract 包：跨边界纯值 DTO

`internal/sim/contract` 持有权威模拟跨包传递的不可变值类型，不持有权威状态或行为逻辑。

## 职责

- 定义 `Command`、`RejectReason`、`Rejection`、`GeneratedChunk`/`AcquiredChunk`、`BlockChange`/`ChunkChangeBatch`、`TickResult`、`PlayerUpdate`/`CompanionUpdate`、`InventoryUpdate`/`FurnaceUpdate`/`ChestUpdate`/`CraftingUpdate`、`HostileMob`/`HostileAction`、`CompanionAction` 等跨边界 DTO。
- 全部字段为值语义，跨 goroutine 发送成功后视为不可变；不暴露可变世界引用。
- 仅使用 `internal/core`、`internal/world`、`internal/companion`、`internal/physics` 的值类型，不感知 `realm`/`entity`/`runtime`。

## 依赖方向

- 允许：`internal/core`、`internal/world`、`internal/companion`、`internal/physics`。
- 禁止：依赖 `internal/sim/realm`、`internal/sim/entity`、`internal/sim/runtime`、`internal/sim/tuning` 或其他模拟子包；禁止依赖 `internal/server`/`internal/client`/`internal/render`。
- 方向由 `internal/archcheck` 的 `allowed` 与 `simAllowedEdges` 强制，校验入口 `TestInternalDependenciesAreOneWay`、`TestSimSubpackageDependencyDirections`。

## 关键文件

- `contract.go`：全部 DTO 定义。
- `contract_test.go`：值形状与序列化回归。

## 定点验证

- `go test ./internal/sim/contract -race -count=1`
- 依赖边界：`go test ./internal/archcheck -count=1`
