# ledger — unified-block-updates-world-streaming

- Worktree: /Users/chen/work/mornlea/.worktrees/unified-block-updates-world-streaming, branch feat/unified-block-updates-world-streaming, base main@8d94a4e8。
- Harness note: `task` tool 无 model 参数，子代理继承会话模型；skill 的模型分层只记意图。
- Spec authority: openspec/changes/unified-block-updates-world-streaming/{proposal.md,specs/*/spec.md,design.md,tasks.md}。
- 设计全文与批准记录：docs/superpowers/specs/2026-09-09-unified-block-updates-world-streaming-design.md（2026-09-09 用户显式批准）。

## Pre-flight 裁决

- Ruling: 合并范围 = 「裁剪合并 + 每玩家视距」— 用户 2026-09-09 问答裁决：任务 2 原描述约八成已落地（region 双 bank/兴趣管理/背压/Observe-Drain 已在 F-07、multidimension 等基线验证），只做剩余缺口并叠加每玩家视距；非区块存档 region 化、动态调视距、设置页滑块列非目标 — 若错，代价是后续独立 change 补做。
- Ruling: 合并为单一 change — 两组文件集不交叠、可分别回退，合并理由是共享 scenario/perf 一次性重定与单一版本互斥窗口（用户明确要求合并开发）。
- Ruling: 湿度 FIFO→dueTick 是唯一允许的行为可见变化 — 预算/平衡态/同 tick 重判保持，积压消费顺序改全序；`authoritative-farming` 出 MODIFIED delta（仅一条 Requirement 的一个 Scenario 措辞 + 承载方声明）。
- Ruling: engine ABI 不升版 — 调度统一在 Go 侧，Rust kernel（fluid_eval_batch 等）接口不动；用户原文预期「大概率升版」经现状核对不成立。
- Ruling: `authoritative-fluid`、`tunable-constants`、`chunk-persistence` 无 delta — 流体行为逐位不变、预算参数名值不变（region 上限走 server config）、磁盘并行化无可观察格式变化；若实现期发现规格措辞与迁移冲突，先补 delta 再继续。
- Ruling: `bounded-benchmark-workload` 同时 MODIFIED 两条 Requirement — 版本链 Requirement（v23 + 迁移 22:23）与 B-33 的「追加材质层」Requirement（其 v22/21:22 钉值随版本链推进过期，改为「追加不单独升版 + 迁移随版本链走」）。

## Pre-flight conflict scan

| 交叠点 | 关系 | 结论 |
|---|---|---|
| 1.1 ↔ 2.1/2.2/2.3（`updates.Queue`/`Sampler`） | 1.1 产出、组 2 消费 | 顺序执行，组 2 各任务依赖 1.1 完成 |
| 1.2 ↔ 2.3（`updates.Sampler` 消费方） | 1.2 收敛 entity 副本、2.3 收敛 realm 链 | 同一导出面两次接线，顺序执行无冲突 |
| 2.4 → 2.1/2.2/2.3（`phaseBlockUpdates` 重排） | 2.4 汇总编排 | 2.4 最后做；`stepPhase` 观测常量测试在 2.4 内同步 |
| 3.1 → 3.2 → 3.4（协议字段 → 会话半径 → 场景） | 链式依赖 | 顺序执行；3.4 依赖 3.2 的会话视距 |
| 3.3（disk.go 拆锁） | 与组 2/3.1/3.2 文件集不交叠 | 可独立；`packages/server/server` config 字段与 3.1/3.2 无文件冲突 |
| 版本矩阵（AGENTS.md/openspec/config.yaml/audit） | 3.1 升协议、3.4 升 scenario 各自同步矩阵 | 沿 multidimension 先例：版本升版任务自带矩阵同步并跑 `go test ./packages/audit -count=1`；4.1 只核验 |
| `packages/audit/dependency_test.go` | 1.1（fluid/sim/realm）与 1.2（sim/entity）各加边 | 顺序提交，1.2 不重复加边 |
| benchmark golden / `perf-baseline.json` | 3.4 触碰 scenario 版本 | 基线提升走显式流程（guard.mjs 高危路径），4.1 取证 |

## 验证证据（按 SHA 复用）

- main@8d94a4e8: worktree `make rust` exit 0（/tmp/e19-rust-build.log）。
- 9c901883→6a9f2907 (Task 1.2): `go test ./packages/server/sim/entity -race -count=2` 双绿、`./packages/server/updates` 绿、`./packages/server/sim/runtime -race -count=1` 绿（runtime 委托路径回归）、`./packages/audit` 绿、`gofmt` 空（评审者亲跑）；评审者以 /tmp 探针对 63d733a2 旧实现做机械提取比对——SplitMix64 20 万输入、13 函数 5 万随机元组（含负种子/边界维度/chance 边界）逐位一致、26 个 KAT 锚点经旧实现原样复现、10 盐值两两互异；探针曾真实报出自身 bug 的 mismatch，比对具备判别力。
- ce049bf3 (Task 2.1): fluid/updates/realm/runtime/audit 全绿（评审者亲跑，fluid -count=2）；`MORNLEA_FLUID_PERF=1` 三场景 PASS；评审对新旧堆实现逐行机械比对（lessItem/lessPos/sift/push/pop/swap 逐字一致、去重/Clear/预算语义等价、四种退出路径探视计数逐步对应）；稳态 1 allocs/op（24B，迁移前既有的 pendingWrites map，Register 重注册零分配）。

## 进度

- Ruling: Task 1.1 三处偏离全部接受 — (a) 每 kind 一个索引堆替代单堆：单堆下预算耗尽/未注册域条目占堆顶会破坏探视有界与域隔离，分域堆 + `candidateLess` 全局选择保持弹出序=全局全序（评审以测试本地独立 oracle 核验，夹具对 kind 优先序有判别力）；brief 的单堆描述是对实现的不当约束，spec 只约束弹出序与不变量。(b) `simAllowedEdges` 同步加边：`TestSimAllowedEdgesMatchesGlobalAllowed` 集合相等测试强制，非扩大范围。(c) `Advance(now)` 预算取自注册表：与 spec「域标识、每 tick 预算、处理回调」三元组一致，Register 可重复调用供 2.x 配置快照重注册。
- Task 1.1: complete (commits 9c901883..7e89002c, review PASS, 0 Critical/Important; deferred minors: ① `Handler` doc 未禁回调内重入 `Advance`（内层 defer 提前冲刷 deferred 会破坏外层一次推进至多一次；无现实调用方，2.1 写首个真实 handler 时顺手补 doc 禁令或 panic 守卫）② sampler.go 值接收器方法内冗余 `sampler := Sampler{}` 构造（纯风格，1.2 顺手清理））。
- Ruling: Task 1.2 勘察与两处最小跟进接受 — entity 无有状态 RNG（此前调研所记 `entity.NewState` 为误报）；`yield.go` 的 realm 家族死副本直接删除、`ShortGrassSeedDropRoll` 保留 entity 纯转发 shim（runtime 委托与 server 夹具入口不变）；entity `AGENTS.md` 允许列表一行同步（局部指南与 audit 一致）；包级零值 `var sampler = updates.Sampler{}` 为无状态纯函数集合统一调用点，非可变状态。
- Task 1.2: complete (commits 63d733a2..6a9f2907, review PASS, 0 Critical/Important; deferred minor: `TestSamplerDomainSaltsIndependent` 未扩展到 entity 家族、全量 10 盐值两两互异无单点断言（评审者已数值验证互异成立）——2.3 顺手补全量互异断言)。组 1（统一调度器基座）全部完成。
- Task 2.1: complete (commits c5378e73..ce049bf3, review PASS-with-findings, 0 Critical/Important, 3 Minor)。实现差异接受：5 个只读观测口供白盒断言、fluid.Queue 镜像字段使 queue_bounded_test 断言逐字不动、`Scheduler()` 同实例注册通道（kind 断尾序保证同格同 tick 先流体后湿度）；realm 零改动；Handler 重入禁令已兑现（1.1 遗留①）。
- Ruling: 2.1 评审三 Minor 路由 — ① `BenchmarkAdvanceEval` ~310.6µs→~341.4µs（+~10%，record-only，跨域候选扫描+间接回调推测；记入 perf 台账，2.2 对照、4.1 取证）② realm `queue.Len()==0` 跳过判断（environment.go:888）在湿度注册同实例后口径变为跨域总数——2.2 必须改用 `LenOf(KindFluidFlow)` ③ `advanceWorld` 残留引用隐患——2.2 在 `Scheduler()` doc 补「不得绕过 `fluid.Advance` 直接推进 FluidFlow 域」。
- Ruling: 2.2 共享实例推进方式定为「kind 过滤 Advance」— `updates.Queue` 增加按 kind 子集推进的入口（缺省全注册域，向后兼容；1.1 的每域独立堆使该入口实现平凡），fluid 与 moisture 各自在自己的 tick 阶段按域过滤推进 — 为什么：湿度注册到同一实例后无过滤的 `Advance(now)` 会在湿度阶段连带弹出到期流体条目（`FluidFlowDelayTicks` 可调到 0 时必然发生），流体 handler 编码后无提交编排、且 `advanceWorld` 已过期；按域过滤同时保住每维度单实例、跨域全序与既有阶段顺序 — 若错，代价是回退为每域独立实例（跨域全序文档化损失，无行为影响）。
-（SDD 执行期逐任务追加：Task 完成记录 + 评审结论 + Ruling）
