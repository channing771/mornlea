# Ledger

## 执行记录段

| 日期 | 阶段 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-27 | 0 认领与批准 | **Ruling（用户）:** ① 控制会话核实 A-02（活跃 zcode 会话 5 分钟内有产出）与 B-17（opencode-implementer 已交设计+计划）均不可抢、其余排队行依赖未满足后，用户选定 D-02 晋升认领，语义边界取「单机暂停最小闭环」：权威 tick 冻结仅作用单机嵌入服，TCP 远程会话不宣称暂停；② 菜单条目两项方案：「返回游戏 / 退回主菜单」，设置入口顺延。批准来源为控制会话内问答显式选择（两轮）。 | 积压表晋升+认领 docs-only 提交 `b10bbfe7` 快进推送 origin/main；worktree `feat/D-02-pause-menu` 基于 origin/main 创建 |
| 2026-08-27 | 1 契约产物 | 创建 `pause-menu` 全套产物：proposal/design/tasks/ledger + `egui-tool-ui` delta（三条 ADDED Requirements：Esc 开合覆盖层含远程不宣称暂停、权威门冻结可恢复、退回主菜单拆解会话）。关键取舍四条记录于 design.md（服务端 tick 门优于客户端停发/cancel ctx；原子布尔无锁；TCP 同页不同标注；布局段内部版本 3→4 沿 D-03 先例且客户端 ABI 保持 v9）。独占文件集见 proposal「Impact」。 | 待跑：`openspec validate --all --strict --no-interactive` |
| 2026-08-27 | 3 Task1 权威暂停门实现 | TDD 红绿完成：`internal/server/pause_test.go` 五个测试（冻结多调度周期、重复 Pause 幂等、Resume 从冻结值续接增量、同种子孪生世界逐 tick 对照暂停段不改变续跑结果、RunTicks 调度层观察者证实暂停窗口整个 tick 不存在）先红（Pause/Resume 未定义编译失败）后绿。门落点：`Server.paused atomic.Bool` + 导出 `Pause()`/`Resume()`（幂等）；短路安放两处共享同一原子位——`step()` 编排最前（observer 登记之前，覆盖包括显式 `Step` 在内的全部推进点，保证暂停期任何调用方无法推进世界时间）与 `RunTicks` ticker 到期的前置读（免 `stepMu` 争用）。门不持锁、热路径零分配，消息通路与 worker 不受影响。 | `go test ./internal/server -race -count=1` ok（204s 全量含新增）；`go test ./internal/archcheck -count=1` ok；`gofmt -l internal/server` 无输出 |
| 2026-08-27 | 3 Task1 R1 清理轮（SPEC/QUALITY 双评审 PASS 后四条 nit） | ① 测试改名 `TestPauseFreezesWorldTimeAcrossExplicitSteps` 名实相符（走显式推进点；调度循环性质由 RunTicks 观察者测试覆盖）；② `Pause` doc 补并发边界——与在途 tick 并发时已开始的一个 step 会跑完本轮编排，其后周期才被短路（门不持锁固有语义）；③ 「整个 tick 不存在 / worker 与消息通路存活」表述收敛至 `paused` 字段注释单一落点，`step`/`RunTicks` 注释与 `Pause` doc 仅留局部落点理由；④ 种子常量按实际用途拆分：孪生世界改用局部常量 `twinWorldSeed`，调度器测试取独立任意种子。行为逻辑零改动。 | `gofmt -l internal/server` 无输出；`go test ./internal/server -race -count=1` ok（202s） |

### 版本矩阵（基线 `b10bbfe7`）

全部不变：协议 v27、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、engine ABI v7、client ABI v9、benchmark scenario v19。唯一结构变化为 UI 下行布局段内部版本 3→4（仓库自有契约，沿 D-03 先例）；capture 场景表与视觉 golden 零变化。
