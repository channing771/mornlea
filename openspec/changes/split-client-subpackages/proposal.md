# split-client-subpackages

## Why

`cmd/mornlea` 以单一 `package main` 平铺 94 个 Go 文件（约 2.4 万行），
`application` 装配、输入/UI、capture 视觉 golden 与 benchmark 性能场景混在
同一包内。后果：

1. 结构不清晰：四个互相独立的关注点共享一个命名空间，包内边界只靠文件名约定。
2. 测试无法定点：`go test ./cmd/mornlea` 实测约 4 分钟，耗时集中在 capture
   golden 抓帧（13 个测试文件）与 benchmark 真实 renderer 场景（8 个测试文件）；
   迭代 app 层（输入/HUD/菜单）时也要为无关重型测试付费，违反测试分层纪律的
   意图。

拆出功能域子包后，`go test ./cmd/mornlea/app` 不再编译运行 capture/benchmark
测试，capture、benchmark 可单独定点；与 T0–T3 分层及 `race-changed` 改动闭包
天然配合。

## What Changes

- **BREAKING（仓库内部源码结构）** 将 `cmd/mornlea` 拆为薄 `package main`
  （CLI 解析与装配入口）加三个子包：
  - `cmd/mornlea/app`：`application` 主体、输入/UI、帧循环、生命周期、音频、
    LOD、消息；
  - `cmd/mornlea/capture`：capture 场景、golden 比对、`testdata/golden` 资产；
  - `cmd/mornlea/benchmark`：benchmark 测量、多人 benchmark 观察者与探针。
- `application` 类型导出为 `app.Application`；capture/benchmark 通过各自包内
  定义的消费端接口访问所需 app 状态，`app` 只导出最小方法集。
- 依赖方向钉死：main → app/capture/benchmark；capture、benchmark → app；
  capture 与 benchmark 互不依赖；app 不得反向导入 capture/benchmark。方向由
  `internal/archcheck` 新断言强制。
- 全部测试函数名与 `t.Run` 标签保持不变；迁移后各包 `go test -list` 入口
  并集与迁移前 `cmd/mornlea` 单包集合一致。
- 按 deer-flow harness 的分层文档模式重组文档：`cmd/mornlea/AGENTS.md` 重写
  为子包地图与依赖方向总纲，每个子包一份 AGENTS.md（不变量 + 精确路径 +
  钉死回归的测试名）加薄 CLAUDE.md。
- 不修改线上协议、存档、ABI、登录语义、Memory/TCP 行为、golden 图像与性能
  场景语义。

## Capabilities

### New Capabilities

无。该变更只建立客户端命令的代码组织边界，不引入新的用户可观察能力。

### Modified Capabilities

- `repository-code-organization`：为客户端命令建立薄 main + app/capture/
  benchmark 子包布局，要求依赖方向单向、测试入口集合保持不变、golden 资产
  随 capture 子包迁移且路径常量同步。

## Impact

- 受影响生产包：`cmd/mornlea`（缩为薄 main）与新增 `cmd/mornlea/app`、
  `cmd/mornlea/capture`、`cmd/mornlea/benchmark`。
- 受影响调用方与入口：`Makefile`（`test-multiplayer` 包路径）、
  `internal/archcheck`（源码字符串守卫扫描范围、依赖方向断言）、
  `docs/notes/test-quickstart.md` 定点命令表、根 `AGENTS.md` 局部指南清单。
- 受影响资产：`cmd/mornlea/testdata/golden` 随迁至
  `cmd/mornlea/capture/testdata/golden`（git mv 保留历史），
  `captureGoldenDir` 常量同步。
- 这是仓库内部源码 import path 的变更；无线上 wire、存档或 ABI 兼容性影响。
  性能基准数值只记录，不改变退出状态；golden 内容逐字节不变。

## 延期与放弃

- `internal/client` 的 `Receiver` 缺少就绪探测 API（inbox 深度/ack 信号），测试
  helper 只能以 sleep 概率性交接（T2 中 1ms → 50ms）——登记独立跟进项以消除
  时序 flake 根因；涉及 `internal/*` 重构，超出本 change 非目标。
- 全仓继承的任务编号注释清理（`cmd/mornlea/main.go:112`、`app/app.go:56`、
  `app/app_startup.go:715`、`app/eating_overlay_test.go:5` 等）——留独立
  cleanup 任务，不混入本 change。
