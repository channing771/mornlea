# 客户端域 Go 模块指南

## 作用域

本文件补充仓库根 `AGENTS.md`，适用于 `packages/client/`（客户端域独立 Go 模块，
go.work 成员）。更具体的边界见各子目录 `AGENTS.md`：

- `packages/client/client/AGENTS.md`（镜像、预测与 client C ABI bridge）
- `packages/client/cmd/mornlea/AGENTS.md`（客户端命令子树总纲；app/capture/
  benchmark/devcapture 各有局部指南）

## 包所有权

- `client` 持有只读权威镜像、输入预测、消息接收、client ABI bridge 和渲染侧 CPU 编排，不成为第二个权威模拟器。
- `render`（含 `hud`）、`mesh`、`lod`、`assets`、`audio` 持有呈现侧数据描述、CPU 编码与平台桥接职责。
- `cmd/mornlea` 是图形客户端应用入口（薄 main 加 app/capture/benchmark/devcapture 四个功能域子包）。
- `assets.Registry` 持有全注册物品的 16×16 RGBA 只读图标缓存；完整方块从当前
  顶面/侧面材质生成等距小方块，透明轮廓物品同时占 atlas 追加层供世界薄片采样。
  原创建图与 contact sheet 入口见 `assets/ITEM_ICONS.md`。

## 依赖边界

- 模块生产代码的兄弟 require 以 tidy 收敛为准：`shared` 由域库直接消费；`server`（及其传递引入的 `contracts`）仅由 `cmd/mornlea` 应用入口消费——普通本地与 benchmark 模式在进程内装配本地权威 Host，与 tools 组合两侧的入口同构。允许 require 边由 `packages/audit/unit_boundary_test.go` 强制。
- client 域库包（cmd/mornlea 子树之外的任何包，含测试文件）MUST NOT import `packages/server`；该禁令由 `packages/audit/server_client_boundary_test.go` 源码守卫强制。
- 包级依赖方向以 `packages/audit/dependency_test.go` 的 `allowed` 表为唯一真相，不要在指南中复制该表。新增包或依赖边必须先证明方向合理并同步架构门禁。
- engine C ABI 只能由 `packages/shared/nativeabi` 接触；client C ABI 只能由 `packages/client/client` 接触。领域包通过这两个既有 bridge 调用 Rust，不得直接引入 C ABI 或另建生产 fallback。

## 测试组织

测试与被测代码同目录并按关注点组织。共享 helper 的准入、命名和拆分规则遵循 `docs/test-organization.md`，不要新建平行 helper 中心。

## 定点验证

- 客户端域包示例：`go test ./packages/client/client -race -count=1`；处理其他包时把路径替换为对应真实目录。
- 客户端命令子树：`go test ./packages/client/cmd/mornlea/... -race -count=1`；无窗口视觉 `make visual-check`。
- 依赖边界：`go test ./packages/audit -count=1`。
- 当前文档入口：`docs/notes/go-rust-division.md`、`docs/test-organization.md`。
