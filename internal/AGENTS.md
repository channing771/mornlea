# Go 内部包指南

## 作用域

本文件补充仓库根 `AGENTS.md`，适用于 `internal/`（余下客户端域包与
`internal/archcheck`）。更具体的边界见：

- `internal/client/AGENTS.md`

服务端域包（sim/fluid/storage/server 与 `cmd/mornlea-server`）已迁入
`packages/server` 模块，指南见 `packages/server/AGENTS.md`；双侧共享的领域包
（core/network/physics 等）在 `packages/shared`，局部指南随包目录（如
`packages/shared/network/AGENTS.md`）。

## 包所有权

- `client` 持有只读权威镜像、输入预测、消息接收和渲染侧 CPU 编排，不成为第二个权威模拟器。
- `render`、`mesh`、`lod`、`assets`、`audio` 持有呈现侧数据描述、CPU 编码与平台桥接职责。

## 依赖边界

内部包允许的直接依赖以 `internal/archcheck/dependency_test.go` 的 `allowed` 表为唯一真相；不要在指南中复制该表。新增包或依赖边必须先证明方向合理并同步架构门禁。

engine C ABI 只能由 `packages/shared/nativeabi` 接触；client C ABI 只能由 `internal/client` 接触。领域包通过这两个既有 bridge 调用 Rust，不得直接引入 C ABI 或另建生产 fallback。

## 测试组织

测试与被测代码同目录并按关注点组织。共享 helper 的准入、命名和拆分规则遵循 `docs/test-organization.md`，不要新建平行 helper 中心。

## 定点验证

- 客户端包示例：`go test ./internal/client -race -count=1`；处理其他包时把路径替换为对应真实目录。
- 依赖边界：`go test ./internal/archcheck -count=1`。
- 当前文档入口：`docs/notes/go-rust-division.md`、`docs/test-organization.md`。
