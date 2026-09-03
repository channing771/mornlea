# Go 内部包指南（过渡期）

## 作用域

本文件补充仓库根 `AGENTS.md`，适用于 `internal/`。客户端域包（client/render/
mesh/lod/audio/assets）已迁入 `packages/client` 模块，指南见
`packages/client/AGENTS.md`；服务端域包（sim/fluid/storage/server 与
`cmd/mornlea-server`）已迁入 `packages/server` 模块，指南见
`packages/server/AGENTS.md`；双侧共享的领域包在 `packages/shared`，局部指南
随包目录（如 `packages/shared/network/AGENTS.md`）。

## 现状与去向

- `internal/` 目前只余 `archcheck`（依赖方向、单元边界、身份与基线版本的
  架构门禁测试集）。
- 单元化收尾阶段 archcheck 将整体迁往 `packages/audit`（包名保持
  `archcheck`），届时本目录与 `internal/CLAUDE.md`、本指南一并消失；在那
  之前不要在 `internal/` 下新增生产 Go 包。

## 定点验证

- 依赖边界与全部架构守卫：`go test ./internal/archcheck -count=1`。
