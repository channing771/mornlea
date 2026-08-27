## Why

`internal/server` 多个由测试主动推进权威 tick 的登录就绪循环没有总预算；如果 Ready、背包或视野永远不收敛，它们只会在 `go test` 的全局五分钟超时后失败，既拖慢反馈，也不能指出缺少的就绪条件。农业与饥饿端到端用例已经证明固定 tick 预算能把这类挂起转成可读断言，现在应把既有同形循环收敛到一个测试 helper。

## What Changes

- 在 `internal/server` 既有测试 helper 中提供统一的登录 tick 预算等待，并以单元测试钉住「已满足时零推进」「预算内成功」和「预算耗尽」边界。
- 把所有仍无总预算的手动 tick 登录等待迁入该 helper，并把农业、饥饿用例的两份内联预算一并同源化。
- 超限失败携带场景标签、预算和调用方提供的 Ready/背包/视野诊断；测试仍由调用方决定具体就绪条件。
- 保留已经由 `context`、墙钟 deadline 或有限 tick 数约束的等待；不顺手改写非登录业务等待。

## Capabilities

### New Capabilities

无。本变更只整理测试基础设施，已在 `.openspec.yaml` 中声明 `skip_specs: true`。

### Modified Capabilities

无。玩家可观察行为与现有主规格均不改变。

## Impact

- 只修改 `internal/server/*_test.go`、既有 server 测试 helper 与本 change 产物；不修改生产 Go/Rust 代码。
- 不改变协议、存档 schema、engine/client ABI、benchmark scenario、依赖或配置格式，也不生成视觉 golden。
- tick 预算是挂起检测而非性能门禁：正常用例条件满足后立即返回；预算取现有 farming/hunger 已验证的宽松值，不以降低数值来制造速度断言。
- Memory/TCP 测试仍走原有权威模拟、消息接收与镜像应用路径，不引入单机旁路或并发语义变化。

## 非目标

- 不统一 `internal/server` 的全部时间常量，也不调整 `waitDeadline`/`longWaitDeadline`。
- 不迁移已经有界的 `Recv(ctx)`、墙钟轮询、缺席断言、超时触发断言或业务阶段等待。
- 不借此修改登录实现、区块生成、发布顺序或任何产品行为。

## 用户可观察结果

游戏行为完全不变。开发者运行受影响的 server 测试时，登录就绪若不再收敛，会在固定 tick 预算耗尽时得到带状态诊断的失败，而不是等待包级全局超时。
