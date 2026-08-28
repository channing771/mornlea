## ADDED Requirements

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
