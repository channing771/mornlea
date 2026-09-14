# 冷启动与持久化解耦设计

日期：2026-09-11
方向：引擎与架构 / 性能与扩展性
成功标准：冷启动与持久化（大存档冷启动 wall-time 下降，只记录不设硬门禁）

## 1. 背景

世界存档异步生命周期由 `packages/server/server/persistence` 持有，世界区块与 metadata 经有界 channel 与固定 worker 落盘，权威 tick 只做有界非阻塞调度。增量记账与 `Chunk.Hash` 缓冲化已落地，稳态 p99 已降；剩余缺口是冷启动：存档 job 单 FIFO 生成排在全量装载之前，首屏收敛慢。客户端已有 `LoadedChunkTarget` / `WaitUntilLoaded` / `ApplicationLoadComplete` 单一完成定义与加载屏同源进度，可复用。

真相优先级：代码与测试 > `openspec/specs/` > `docs/architecture.md` > `docs/notes/progress.md` > `docs/superpowers/`。

## 2. 目标与成功标准

目标：冷启动时加载 I/O 优先、保存让路；启动按出生点兴趣范围分阶段装载，外圈后台预取；完成判据复用既有单一定义。

非目标：不改存档格式、玩家/区块/metadata/伙伴 schema，不改协议、engine ABI、client ABI、benchmark scenario；不重构稳态吞吐与背压估算语义（双计入维持现状，另行裁决）；不新增 benchmark 场景；不做压缩与格式演进。

成功：大存档冷启动 wall-time 下降（只记录）；`WaitUntilLoaded` 超时零回归；flush/close 语义不变；capture golden 零差异。

## 3. 架构与组件

- `persistence.World` 新增 load lane：`saveJobs` FIFO 不动，load 请求走独立有界队列与独立 worker（默认小，如 2）。tick 路径只做非阻塞 `Observe/Drain`，磁盘 I/O 只在 worker 内。
- `storage` 加载面只读并发：复用 region 句柄，加只读 LRU；不碰写路径、去重、哈希与估算语义。`Chunk` 编解码不动。
- `app` 分阶段装载：首阶段按出生点 `ViewDistance + 1` 半径装载至 `LoadedChunkTarget` 收敛进世界，后台预取外圈；进度复用 `loadingProgressSection`，完成判据仍是 `ApplicationLoadComplete`。

触碰面：`packages/server/server/persistence/world*.go`、`packages/server/storage` 加载半部（`disk`、`region`、`chunk`）、`packages/client/cmd/mornlea/app/app_load.go` 与加载 UI 状态组装。零 wire/schema/ABI 变更。load worker 数、重试次数与退避 tick 等具体数值不在本设计钉死，随后续 OpenSpec change 提案时固定。

## 4. 数据流与并发边界

- 启动：`Server` 装配期按出生点算首批列集合，load lane 批量取，回填 `Engine` 可见状态，经 `SnapshotChunks` 下发，客户端 `WaitUntilLoaded` 收敛后切后台预取。
- 稳态：权威 tick 仍只调世界与玩家/伙伴/敌怪的 `Observe/Drain/Poll`，不等待落盘；`Flush/Close` 短锁后即放，语义不变。
- 不可变规则：worker 只收克隆快照，发送成功后视为不可变；load 结果与 save 重试无共享可变状态；取消丢弃整结果，不发布部分输出。
- 热路径：权威 tick、渲染与网络热路径不执行无界工作，不阻塞磁盘与网络。

## 5. 错误处理与回退

- load 单列失败只重试该列（有界次数加 tick 退避），不阻塞首屏其他列；持续失败经 `Status.LastError/LastErrorAt` 上报，不吞错。
- save 失败路径不动；load 让路不饿死 save，load 完成或超时后配额归还。
- 回退：关闭开关回到当前行为（单 FIFO 加同步首载）；开关只走配置，不进存档或协议。overflow、数据丢失与 I/O 错误显式失败。

## 6. 测试与门禁

- 单测：load 优先与让路、首批收敛、失败列隔离重试、flush/close 不变、存档 round-trip 零改动。
- 定点：`go test ./packages/server/server/persistence -race -count=1`、`go test ./packages/server/storage/... -race -count=1`、app 加载判据包、`go test ./packages/audit -count=1`。
- 门禁：`make dev-check` 起步，阶段边界补 `test-race-changed` 或 `test-race`、`openspec validate --all --strict --no-interactive`；`make visual-check` 零差异；冷启动 wall-time 只记录进 `docs/notes/`。

## 7. 被否决的替代方案

- 全量 region 并行装载加读写拆锁：吞吐最高，但并发复杂度最大，易踩背压估算语义，普通存档感知弱。否决，加载侧只取只读并发一小片。
- 记录型优化（并行哈希与编码缓冲）：风险最小，但低垂果实已摘，边际递减且撑不起长任务。否决。
- 压缩与格式演进：收益不确定且触迁移面。按 YAGNI 砍掉。

## 8. 风险与后续

- load 与 save 配额反转导致 save 饿死：以配额归还与超时上限约束，测试钉住。
- region 只读 LRU 与写路径句柄语义分叉：加载面只读，写面不动，审计依赖边界。
- 后续候选：设置页视距滑块、存档 job 调度细化、多玩家流式场景，另行立项。
