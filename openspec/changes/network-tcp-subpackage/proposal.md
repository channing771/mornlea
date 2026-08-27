## Why

`internal/network` 同时平铺协议、编解码、登录、Memory 和 TCP 实现，TCP
实现已经形成独立的传输职责。现在拆出 TCP 子包，可以让传输依赖方向可检查，
并为后续整理其他包建立低风险的分阶段路径。

## What Changes

- **BREAKING** 将 TCP listener、dial、stream、deadline 和 socket 生命周期从
  `internal/network` 移入 `internal/network/tcp`。
- 保留 `internal/network` 的 packet、codec、登录、公共 stream 接口和 Memory
  transport。
- 将仓库内 TCP 构造调用改为 `network/tcp` 子包调用。
- 将 TCP 私有测试迁入新包，保留测试函数名和子测试标签。
- 在 `internal/archcheck` 登记 `network/tcp -> network` 的单向依赖。
- 不修改线上协议、存档、ABI、登录语义或 Memory/TCP 行为。

## Capabilities

### New Capabilities

无。该变更只建立代码组织边界，不引入新的用户可观察能力。

### Modified Capabilities

- `repository-code-organization`：为 TCP 实现建立独立子包，并要求其依赖方向
  单向且测试入口行为保持不变。

## Impact

- 受影响生产包：`internal/network`、新增 `internal/network/tcp`。
- 受影响调用方：`cmd/mornlea`、`cmd/mornlea-server`、`internal/client`、
  `internal/server` 及网络 benchmark/test helper。
- 受影响架构守卫：`internal/archcheck/dependency_test.go`。
- 这是仓库内部源码 import path 的变更；无线上 wire、存档或 ABI 兼容性影响。
