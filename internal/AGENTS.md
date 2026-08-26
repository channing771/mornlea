# Go 内部包指南

## 作用域

本文件补充仓库根 `AGENTS.md`，适用于 `internal/`。更具体的边界见：

- `internal/client/AGENTS.md`
- `internal/nativeabi/AGENTS.md`
- `internal/network/AGENTS.md`
- `internal/sim/AGENTS.md`
- `internal/storage/AGENTS.md`

## 包所有权

- `sim` 持有服务端权威 tick、玩法结算和世界变更编排。
- `world` 持有区块、section、容器与掉落物等世界数据模型，不感知线上协议。
- `network` 持有 packet、codec、登录状态机以及 Memory/TCP transport，不决定玩法结果。
- `storage` 持有世界、玩家和伙伴的编码、迁移、恢复与磁盘生命周期。
- `server` 装配 Host、会话、权威模拟和持久化 worker；重 CPU、磁盘或网络工作不得阻塞权威 tick。
- `client` 持有只读权威镜像、输入预测、消息接收和渲染侧 CPU 编排，不成为第二个权威模拟器。

## 依赖边界

内部包允许的直接依赖以 `internal/archcheck/dependency_test.go` 的 `allowed` 表为唯一真相；不要在指南中复制该表。新增包或依赖边必须先证明方向合理并同步架构门禁。

engine C ABI 只能由 `internal/nativeabi` 接触；client C ABI 只能由 `internal/client` 接触。领域包通过这两个既有 bridge 调用 Rust，不得直接引入 C ABI 或另建生产 fallback。

## 测试组织

测试与被测代码同目录并按关注点组织。共享 helper 的准入、命名和拆分规则遵循 `docs/test-organization.md`，不要新建平行 helper 中心。

## 定点验证

- 包级示例：`go test ./internal/sim -race -count=1`；处理其他包时把路径替换为对应真实目录。
- 依赖边界：`go test ./internal/archcheck -count=1`。
- 当前文档入口：`docs/notes/go-rust-division.md`、`docs/test-organization.md`。
