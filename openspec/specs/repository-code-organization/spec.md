# repository-code-organization Specification

## Purpose
为全仓 Go 文件提供可判定的职责审计与纯结构重组契约，确保文件或包边界变化不会改变任何既有外部行为、固定 artifact 或架构守卫语义。
## Requirements
### Requirement: 代码组织重构保持外部行为
系统 MUST 确保 Mornlea 身份切换不改变任何未获批准的外部行为、测试入口或固定 artifact。测试入口基线 MUST 是 Task 7 初始 HEAD 持久化的清单，该 HEAD 已包含 Tasks 4–6 新增的数据迁移和命令路由测试。

#### Scenario: 身份切换只改变获批测试入口
- **GIVEN** Task 7 初始 HEAD 已持久化全部 Test、Benchmark 与 Fuzz 入口
- **WHEN** 完成原子身份切换
- **THEN** MUST 仅有以下 6 项重命名：`TestMCGodHasNoGraphicsDependencies` → `TestMornleaServerHasNoGraphicsDependencies`、`TestMcgoUsesLoginStreamsInsteadOfAttachedServerEndpoints` → `TestMornleaUsesLoginStreamsInsteadOfAttachedServerEndpoints`、`TestMcgoBenchmarkTCPPathUsesTheSharedLoginStateMachine` → `TestMornleaBenchmarkTCPPathUsesTheSharedLoginStateMachine`、`TestMCGodProcess` → `TestMornleaServerProcess`、`TestMCGodProcessReleasesWorldLockAfterSIGTERM` → `TestMornleaServerProcessReleasesWorldLockAfterSIGTERM`、`TestMCGodProcessSaveFailureExitsNonzero` → `TestMornleaServerProcessSaveFailureExitsNonzero`
- **AND** MUST 仅新增 `TestMornleaCurrentIdentity`
- **AND** 其余 Test、Benchmark、Fuzz 入口与 Task 1 后冻结的 fixture、golden 和性能 baseline MUST 保持不变
- **AND** benchmark 与 `perfcheck` 的性能数值及既有阈值 MUST 只保存记录且不得改变退出状态
- **AND** 只有报告结构、身份/provenance、真实 overflow、数据丢失、I/O 错误和非数值命令失败 MUST 阻断

#### Scenario: Apple M2 固定主线的同环境视觉失败不掩盖身份漂移
- **GIVEN** `system_profiler SPHardwareDataType` 的精确 Chip 为 `Apple M2`，且原始 Task 1 `origin/main` 仅有 `materials-showcase` 最大通道差 1、26 个差异像素（0.0113%）与 `oak-grove` 最大通道差 47、10 个差异像素（0.0043%）两个精确已知失败
- **WHEN** 原始主线与 Mornlea 分支在各自隔离 HOME 下运行同一非更新 capture
- **THEN** 两边 10 个场景 PNG 与上述两个失败的 actual/diff MUST 逐字节一致
- **AND** 两边 MUST 仅有上述两个失败且摘要完全一致，其余 8 个场景 MUST 通过 tracked golden
- **AND** 此裁决 MUST NOT 修改或放宽 golden、阈值、capture 代码或其他视觉失败

#### Scenario: 非 Apple M2 固定主线同环境视觉结果不漂移
- **GIVEN** `system_profiler SPHardwareDataType` 的精确 Chip 不是 `Apple M2`
- **WHEN** 原始 Task 1 `origin/main` 与 Mornlea 分支在各自隔离 HOME 下运行同一非更新 capture
- **THEN** 两边 10 个场景 PNG MUST 逐字节一致，且两次 `visual-check` MUST 退出 0
- **AND** 两边都 MUST 不产生 `*-actual.png` 或 `*-diff.png`
- **AND** 此裁决 MUST NOT 修改或放宽 golden、阈值或 capture 代码

### Requirement: 架构守卫不依赖单一源文件位置
架构守卫 MUST 对完整职责文件集合执行原有检查，不得绑定单一固定源文件位置。

#### Scenario: 同一职责分布到多个文件
- **GIVEN** 一个包内职责被拆到多个生产 Go 文件
- **WHEN** 运行架构守卫
- **THEN** 守卫 MUST 扫描完整职责文件集合并继续拒绝旧路径

### Requirement: 全仓 Go 文件完成职责审计
基线中的全部生产和测试 Go 文件 MUST 获得且仅获得一种职责审计结论。

#### Scenario: 文件无需修改但职责单一
- **GIVEN** 基线中的任意生产或测试 Go 文件
- **WHEN** 完成其所属包的审计任务
- **THEN** 该文件 MUST 被判定为保留、同包拆分、提取新包或删除之一

#### Scenario: 主线同步后的完整审计
- **GIVEN** `37cdb3e` 中 `cmd/` 与 `internal/` 下的 412 个 Go 文件
- **WHEN** 完成 Task 20 的主线同步审计
- **THEN** 36 个同包拆分、2 个提取到唯一新包 `internal/render/hud`、0 个删除、374 个保留的结论 MUST 互斥且合计为 412
- **AND** 不得新增其他包、恢复拆分前旧大文件或以更新 golden/baseline 掩盖差异

### Requirement: TCP transport 具有单向包边界

仓库 MUST 将 TCP listener、dial、stream、deadline 和 socket 生命周期实现置于
`internal/network/tcp`。`internal/network/tcp` MUST 依赖
`internal/network`，而 `internal/network` MUST NOT 依赖
`internal/network/tcp`。根包 MUST 保留登录和应用装配使用的共享 packet stream
接口。

#### Scenario: 依赖图接受 TCP 子包
- **GIVEN** 仓库包含 `internal/network/tcp`
- **WHEN** 架构依赖检查枚举全部内部包
- **THEN** 它 MUST 接受 `internal/network/tcp -> internal/network`
- **AND** 它 MUST 拒绝从 `internal/network` 到 `internal/network/tcp` 的任何反向依赖

#### Scenario: TCP 构造器由子包消费
- **GIVEN** 应用或测试需要创建 TCP listener 或 dial TCP stream
- **WHEN** 它针对整理后的仓库编译
- **THEN** 它 MUST 经由 `internal/network/tcp` 解析构造器
- **AND** 返回值 MUST 满足共享根包的 listener 或 packet stream 接口

### Requirement: Transport 包整理保持传输行为

仓库 MUST 在移动 TCP 实现文件时保持既有 Memory/TCP packet、登录、关闭、背压、
deadline、peer-address、校验和错误语义。该整理 MUST NOT 改变 wire bytes、协议
状态机转换或测试函数名。

#### Scenario: Memory 与 TCP 保持相同登录契约
- **GIVEN** 客户端和服务端使用 Memory transport 或 TCP transport
- **WHEN** 它们执行既有 handshake、登录和 Play packet 流
- **THEN** 两种 transport MUST 继续使用相同的登录状态机和 packet 校验契约
- **AND** packet 值和登录结果 MUST 保持不变

#### Scenario: 既有 transport 测试仍可寻址
- **GIVEN** TCP white-box 测试已迁入 TCP 子包
- **WHEN** 整理后的仓库运行包测试
- **THEN** 每个既有 TCP 测试函数和子测试标签 MUST 保持存在且未重命名
- **AND** Memory、codec、packet 和登录测试入口 MUST 保持存在

### Requirement: 客户端命令按功能域分包

仓库 MUST 将 `cmd/mornlea` 组织为薄 `package main`（CLI 解析与装配入口）加
`cmd/mornlea/app`、`cmd/mornlea/capture`、`cmd/mornlea/benchmark` 三个功能域
子包。依赖方向 MUST 为：main → app/capture/benchmark；capture → app；
benchmark → app。app MUST NOT 导入 capture 或 benchmark；capture 与 benchmark
MUST NOT 相互导入。

#### Scenario: 依赖图接受客户端子包并拒绝反向边

- **GIVEN** 仓库包含 `cmd/mornlea/app`、`cmd/mornlea/capture` 与
  `cmd/mornlea/benchmark`
- **WHEN** 架构依赖检查枚举全部包
- **THEN** MUST 接受 main → app、main → capture、main → benchmark、
  capture → app 与 benchmark → app
- **AND** MUST 拒绝 app → capture、app → benchmark 与 capture ↔ benchmark
  的任何依赖边

#### Scenario: 跨包访问经由消费端接口

- **GIVEN** capture 或 benchmark 需要读写 app 域状态（菜单、设置、面板、
  相机、渲染器等）
- **WHEN** 其编译 against 迁移后的仓库
- **THEN** 所需能力 MUST 通过各自包内定义的接口表达，由 `app.Application`
  实现
- **AND** app 包 MUST NOT 为 capture/benchmark 导出全量内部字段

### Requirement: 客户端分包保持测试入口集合

迁移 MUST 保持全部既有测试入口可寻址：测试函数名与 `t.Run` 标签逐一不变，
三个子包 `go test -list` 入口并集 MUST 等于迁移前 `cmd/mornlea` 单包集合。
分包后 MUST 能对单个子包运行测试而不编译执行其他子包的重型测试。

#### Scenario: 入口并集与基线一致

- **GIVEN** 迁移前已持久化 `go test ./cmd/mornlea -list '.*'` 全量快照
- **WHEN** 分包完成并对 `./cmd/mornlea/...` 各包取 `-list` 并集
- **THEN** Test、Benchmark、Fuzz 入口集合 MUST 与快照一致
- **AND** 子测试标签 MUST 逐一不变

#### Scenario: app 层迭代不为重型测试付费

- **GIVEN** 开发者修改 app 包内输入或 HUD 逻辑
- **WHEN** 运行 `go test ./cmd/mornlea/app -race`
- **THEN** MUST NOT 编译或执行 capture golden 抓帧与 benchmark 真实 renderer
  场景测试
- **AND** `go test ./cmd/mornlea/capture` 与
  `go test ./cmd/mornlea/benchmark` MUST 可单独定点运行

### Requirement: golden 资产随 capture 子包迁移且视觉结果不变

`cmd/mornlea/testdata/golden` MUST 随 capture 域迁移至
`cmd/mornlea/capture/testdata/golden`，golden 目录常量 MUST 同步，golden
图像内容 MUST 逐字节不变。

#### Scenario: 视觉门禁在迁移后保持全绿

- **GIVEN** golden PNG 已随包迁移且常量已更新
- **WHEN** 运行 `make visual-check`
- **THEN** 全部场景 MUST 通过 tracked golden 且退出 0
- **AND** MUST NOT 产生 `*-actual.png` 或 `*-diff.png`
- **AND** MUST NOT 修改、放宽或重新生成任何 golden 图像

### Requirement: 架构守卫覆盖客户端子包子树

针对 `cmd/mornlea` 生产源码的字符串级架构守卫（登录路径守卫、benchmark TCP
路径守卫等）MUST 扫描 `cmd/mornlea` 完整子树（含子包），不得因源码迁入子包
而丢失覆盖。

#### Scenario: 源码守卫随子包继续生效

- **GIVEN** 登录装配与 benchmark TCP 路径的生产源码已迁入子包
- **WHEN** 运行对应源码守卫
- **THEN** 守卫 MUST 继续要求 `network.NewMemoryStreamPair`、
  `network.LoginClient`、`networktcp.ListenTCP(` 等既有模式
- **AND** 守卫 MUST 继续拒绝 `server.NewEmbedded(`、`server.New(` 等违规模式

