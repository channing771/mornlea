## Why

方块更新分裂在四套各自手写的机制里：随机 tick 抽样（作物/耕地退化/树苗/积雪，`AdvanceCrops` 的 splitmix64 链）、流体 dueTick 堆（`fluid.Queue`）、耕地湿度重判 FIFO（`AdvanceFarmlandMoisture`）与 chunk 级重扫队列。草蔓延（B-19）、沙落、红石都在等一个统一的挂载面。同时，「世界流式 + 持久化重构」的原始构想约八成已在近期基线落地（region 双 bank 增量格式、订阅并集兴趣管理、`UnsavedBytes` 背压、tick 线程零 I/O），剩余真实缺口是：`DiskStore` 单锁串行全部六类存档 I/O、region 句柄无限常驻、无每玩家视距、无多玩家流式 benchmark 场景。

2026-09-09 用户裁决：两大任务合并开发（裁剪合并 + 每玩家视距），一次性共享 tick 编排与 perf 契约重定。设计文档：`docs/superpowers/specs/2026-09-09-unified-block-updates-world-streaming-design.md`。

## What Changes

- 新建统一确定性方块更新调度器（`packages/server/updates`）：定时面 `Queue`（泛化自 `fluid.Queue`：全序、入队序无关、只提前不推迟、每域预算）+ 随机面 `Sampler`（现有 splitmix64 链原样导出）。
- 迁移：流体逐位不变；耕地湿度 FIFO→dueTick 调度（预算与平衡态不变，积压消费顺序改为确定性全序，是唯一行为可见变化）；随机抽样域逐位不变；全部入队点收敛到统一 API；tick 阶段收敛为单一 `phaseBlockUpdates`。
- 每玩家视距协商：协议 v39→v40，`LoginStart` 尾部追加 `ViewDistance u8`（2..64），服务端 clamp 后按会话计算订阅并集。
- `DiskStore` 拆锁并行化（per-region + 按类别），region 句柄 LRU 上限（server config，默认 256，in-flight 保护）；单 region 内串行与崩溃安全不变量不动。
- benchmark scenario v22→v23：多玩家探针按会话视距启用订阅，新增 `streaming` 指标族（已加载区块数、chunk 加载时延分位、峰值 RSS，只记录不阻断）；perfcheck 迁移白名单加 `22:23`。
- 非目标：草蔓延/沙落/红石本体（只交挂载面）；运行中动态调视距与设置页视距滑块；非区块四类存档 region 化；重扫队列与 `SweepUnsupported*` 即时反应面统一；重写已落地的 region 格式/兴趣管理/背压/Observe-Drain 编排。

## 延期与放弃

实现期全部裁决与验证证据见 `ledger.md`；以下为终审时仍开放的记档项（不阻塞本 change）：

1. **存档 job 单 FIFO 的加载优先级**：generate job 排在全部 load job 之后、探针 Workers=1 ~3 job/tick——大世界冷启动/传送暖起的首个改进候选（3.4 实测记档，也是 scenario v23 视距梯度收敛 2/4/6/8 的根因）。
2. **拆锁的既定代价**（3.3 评审 M-2/M-3/M-4，代码注释已声明；Backup 部分经终审 I-1 修复收敛）：`Sync` 全量 pin 扰动 LRU 且是唯一超限窗口；Backup 语义从「互斥快照」弱化为「region 文件复制经缓存句柄读锁与同 region 保存/Compact 提交串行（双 bank 撕裂窗口已关），非 region 聚合文件入口检查后尽力复制，与 Close 并发时在排空边界以 `os.ErrClosed` 中止」；`ChunkKeys` 快照粒度从全局一致弱化为逐 region 合法提交点。
3. **streaming 门禁的 race 覆盖**（3.4 评审 M-1）：端到端探针测试在 -race 下被既有 skip 跳过，streaming 完整性门禁只在非 race 构建生效（既有测试设计取舍）。
4. **perf 记录项**（均已入 `docs/notes/perf-baseline.md` v23 段，record-only）：`BenchmarkAdvanceEval` 微基准 +~10%（统一调度器跨域扫描+间接回调）；本机首测 tick p99 11.2ms、flying p99 17.9ms 达到记录性阈值（跨硬件 M2 vs 历史 M5 只作定性对照）。
5. **环境项**：`make build` 尾部可选资产包 `pixel_perfection` 的 ATTRIBUTION 拷贝在本机失败（该包未 provision，预存在条件，非本 change 缺陷）。
6. **哈希收敛余量**：`sim/runtime/weather.go`（天气时长掷骰）与 `shared/core/season.go`（共享域）的 splitmix64 仍为本地实现——明确范围外，可在后续 change 收敛到 `updates.Sampler`。
7. **设置页视距滑块**：视距现经配置文件 + 登录协商生效，UI 入口为顺延候选。

## Capabilities

### New Capabilities

- `deterministic-block-updates`: 统一调度器定时面与随机面的确定性、有界性、迁移语义保持与新域挂载契约。
- `world-streaming`: 每会话视距、磁盘 I/O 并行边界、region 句柄有界与流式指标族。

### Modified Capabilities

- `authoritative-farming`: 湿度积压候选的消费顺序从 FIFO 改为统一调度器确定性全序（预算、同 tick 重判与收敛语义不变）。
- `bounded-benchmark-workload`: scenario 版本链追加 v23（统一调度器 + 流式收尾），当前唯一可授权跨场景迁移改为 `22:23`。

## Impact

- 受影响包：`packages/server/updates`（新建）、`packages/server/fluid`、`packages/server/sim/realm`、`packages/server/sim/entity`、`packages/server/sim/runtime`、`packages/server/sim/tuning`、`packages/server/storage`（根 + `chunk`）、`packages/server/server`（订阅、config）、`packages/shared/network/protocol` + `codec`、`packages/shared/network`（login）、`packages/client/cmd/mornlea/benchmark` + `app`、`packages/client/client`（PerfReport）、`packages/tools/perfcheck`、`packages/audit`（依赖拓扑 + 版本矩阵）。
- 版本：协议 v39→v40（旧客户端握手拒绝）；benchmark scenario v22→v23；engine ABI v11、client ABI v18、chunk v9、玩家 v8、metadata v5、companions v5、hostile/passive v1 全部不变；存档零迁移（队列本就不持久化）。
- 并发：权威 tick 仍单线程串行；持久化仍全部 I/O 在 worker；新增的磁盘并行只发生在 worker 侧，tick 线程零 I/O 不变量保持。
- 性能：湿度调度时序重排与磁盘并行化均不改变既有绝对阈值与 `20%` 相对回归口径；`streaming` 指标族只记录；perf 基线重定按既有流程取证。
- 独立闭环：组 2（调度迁移）与组 3（流式）文件集不交叠（除收尾组），可分别评审、验证与回退；合并为一个 change 的理由是共享 scenario/perf 一次性重定与单一版本互斥窗口。
