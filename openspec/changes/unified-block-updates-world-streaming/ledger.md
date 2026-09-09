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
- 35ba63e4 (Task 2.2): updates/fluid -count=2、realm -count=2、runtime/entity/audit 全绿（评审者亲跑）；评审逐条核验 delta spec 条款与 9 条重定断言（数值断言零改动、顺序口径换全序口径处均有等价或更强断言、删除项确属失效 FIFO 机制）；oracle 三合一（真实入队路径 + ground truth + 重放一致 + 16 tick 收敛上界防活锁）判别力成立；探视界在过滤模式下不升反降。

- 9093b9b8 (Task 2.3): realm -count=2、runtime/updates/entity/tuning/audit 全绿（评审者亲跑）；评审者独立复现加固——/tmp 重建基线 47aaf241（cargo release 重建，engine 零改动 ABI 稳定），旧 splitmix64 实现重放冻结表逐位一致 + 20 万组随机输入单元级比对 + sampler.go 区间零 diff 三层互证；realm 本地哈希家族与盐值常量零残留（PCG 鉴别常量非本家族）。

- 6327eb67 (Task 2.4): runtime -count=2、realm/entity/fluid/updates/contract/audit 全绿、server -race 抽验 251s 绿（评审者亲跑）；评审逐语句比对 engine_step.go（唯一改动=删两次纯观测通知）、逐规则核验门面三规则与旧四处手写等价（含 bucket 写前校验 old==block 论证、~30 机械站点分类核验派生不可触发）、源序守卫与入队守卫正反判别力点名验证；256 tick 重放 KAT 与同 tick 变干测试零改动通过。

- 2cdd3599 (Task 3.1): network/codec/protocol/tcp 四包 -race、server 登录/握手族、audit、client app 定点全绿（评审者亲跑）；golden hex 逐字节核对（v40=0x28、视距 32=0x20、版本字节 0x27→0x28）；评审独立核实 `config.Fields` 的 render.viewDistance Min 2/Max 64 与协议域同域（无「配置合法但登录被拒」回归）；sim/storage diff 为空。

- ef14f83c (Task 3.2): runtime -count=2、entity/contract、server 订阅/登录/传送族、network 四包、audit 全绿（评审者亲跑）；评审者做四变异测试全部咬人（去 +1 / 去 min 钳制 / 回退全局半径循环 / warp 丢视距）；storage/benchmark/engine/client diff 全空；`PlayerRestore.ViewDistance` 零值无歧义核验（不持久化、接纳点覆盖、wire 谓词拒 0）。

- fe58ce76 (Task 3.4): benchmark 包 -race -count=2、client/perfcheck -race、server Streaming|Observer 定点、audit 全绿（评审者亲跑）；评审核实瓶颈机制逐点（单 FIFO、生成排全装载后、Workers=1、channel 每 tick 补 2）与 4489 区块算术自洽；真实归档 v15 基线 JSON 自比较 exit 0 复核；PerfReport 追加式与 ≤22 放行有正反测试。

## 进度

- Ruling: Task 1.1 三处偏离全部接受 — (a) 每 kind 一个索引堆替代单堆：单堆下预算耗尽/未注册域条目占堆顶会破坏探视有界与域隔离，分域堆 + `candidateLess` 全局选择保持弹出序=全局全序（评审以测试本地独立 oracle 核验，夹具对 kind 优先序有判别力）；brief 的单堆描述是对实现的不当约束，spec 只约束弹出序与不变量。(b) `simAllowedEdges` 同步加边：`TestSimAllowedEdgesMatchesGlobalAllowed` 集合相等测试强制，非扩大范围。(c) `Advance(now)` 预算取自注册表：与 spec「域标识、每 tick 预算、处理回调」三元组一致，Register 可重复调用供 2.x 配置快照重注册。
- Task 1.1: complete (commits 9c901883..7e89002c, review PASS, 0 Critical/Important; deferred minors: ① `Handler` doc 未禁回调内重入 `Advance`（内层 defer 提前冲刷 deferred 会破坏外层一次推进至多一次；无现实调用方，2.1 写首个真实 handler 时顺手补 doc 禁令或 panic 守卫）② sampler.go 值接收器方法内冗余 `sampler := Sampler{}` 构造（纯风格，1.2 顺手清理））。
- Ruling: Task 1.2 勘察与两处最小跟进接受 — entity 无有状态 RNG（此前调研所记 `entity.NewState` 为误报）；`yield.go` 的 realm 家族死副本直接删除、`ShortGrassSeedDropRoll` 保留 entity 纯转发 shim（runtime 委托与 server 夹具入口不变）；entity `AGENTS.md` 允许列表一行同步（局部指南与 audit 一致）；包级零值 `var sampler = updates.Sampler{}` 为无状态纯函数集合统一调用点，非可变状态。
- Task 1.2: complete (commits 63d733a2..6a9f2907, review PASS, 0 Critical/Important; deferred minor: `TestSamplerDomainSaltsIndependent` 未扩展到 entity 家族、全量 10 盐值两两互异无单点断言（评审者已数值验证互异成立）——2.3 顺手补全量互异断言)。组 1（统一调度器基座）全部完成。
- Task 2.1: complete (commits c5378e73..ce049bf3, review PASS-with-findings, 0 Critical/Important, 3 Minor)。实现差异接受：5 个只读观测口供白盒断言、fluid.Queue 镜像字段使 queue_bounded_test 断言逐字不动、`Scheduler()` 同实例注册通道（kind 断尾序保证同格同 tick 先流体后湿度）；realm 零改动；Handler 重入禁令已兑现（1.1 遗留①）。
- Ruling: 2.1 评审三 Minor 路由 — ① `BenchmarkAdvanceEval` ~310.6µs→~341.4µs（+~10%，record-only，跨域候选扫描+间接回调推测；记入 perf 台账，2.2 对照、4.1 取证）② realm `queue.Len()==0` 跳过判断（environment.go:888）在湿度注册同实例后口径变为跨域总数——2.2 必须改用 `LenOf(KindFluidFlow)` ③ `advanceWorld` 残留引用隐患——2.2 在 `Scheduler()` doc 补「不得绕过 `fluid.Advance` 直接推进 FluidFlow 域」。
- Ruling: 2.2 共享实例推进方式定为「kind 过滤 Advance」— `updates.Queue` 增加按 kind 子集推进的入口（缺省全注册域，向后兼容；1.1 的每域独立堆使该入口实现平凡），fluid 与 moisture 各自在自己的 tick 阶段按域过滤推进 — 为什么：湿度注册到同一实例后无过滤的 `Advance(now)` 会在湿度阶段连带弹出到期流体条目（`FluidFlowDelayTicks` 可调到 0 时必然发生），流体 handler 编码后无提交编排、且 `advanceWorld` 已过期；按域过滤同时保住每维度单实例、跨域全序与既有阶段顺序 — 若错，代价是回退为每域独立实例（跨域全序文档化损失，无行为影响）。
- Ruling: 2.2 brief 内「范围外候选不重入队」与「保持原 dueTick 重入队」两句并存的歧义，按实现者读法裁决 — 范围外=HandleConsumed 检查即消费（与旧 FIFO pop 丢弃语义一致，且 delta spec「后续阶段最终排空」在静态积压下只有消费语义可满足）；原 dueTick 回插（HandleDeferred）专用于读预算不足的顺延并暂停该域本次推进（无活锁：预算每 tick 全局重建）。`HandleResult` 签名与 `ClearKind`（ResetFarmlandMoisture 只清湿度域）接受为必要最小 API。
- Task 2.2: complete (commits fb9d02ee..35ba63e4, review PASS, 2 Minor)。Ruling 落实核验：②LenOf 口径 ③Scheduler doc ④fluid.Advance 改 AdvanceKinds(now, KindFluidFlow) 双向堵漏。Minor 路由：①跨维度全局双预算缺直接双维夹具（A 维耗尽全局额度后 B 维首候选顺延）——2.3 顺手补；②`dimension == nil` 防御分支语义从丢弃变保留——生产不可达（fluidQueues 只增不减）且重扫兜底、最终态一致，记台账不动作。
- Task 2.3: complete (commits 47aaf241..9093b9b8, review PASS, 0 findings)。realm 本地 splitmix64 家族净删约 150 行、5 调用点逐位接线；新冻结重放 KAT（256 tick、四域写入、含恒真守卫）经评审者 /tmp 基线重建独立复现逐位一致；骑手一（10 盐两两互异+长度守卫）与骑手二（双维度全局预算夹具，判别力成立：预算误为每维一份即红）双双落地；注释-only 5 文件逐行核验无行为行（audit 反引号门禁驱动）。组 2 剩 2.4。
- Task 2.4: complete (commits 240358e2..6327eb67, review PASS, 2 Minor)。相位表收敛 5 相位、执行顺序逐语句不变（三层证据：逐语句 diff + go/parser 源序守卫带咬人反例 + 零改动 256 tick 重放 KAT）；入队单点化为 `EnqueueBlockWrite` 门面（三规则单源，bucket/farming/placement 四直连点派生等价经逐规则核验，~30 机械站点派生不可触发分类核验），entity 生产文件扫描守卫带正反例；fluid_perf 流体/湿度两列退役为单列（旧列无断言、文档如实标注）。Minor 路由：① `watchFarmlandMoistureCandidateAtPhase` 死 helper（基线即零调用）与其不准确注释——4.1 顺手删除；② `companion_placement.go:178` 硬编码 `core.AirID` 未按 doc 透传写前旧值（当前安全已核验：同函数空气校验紧邻写入；(AirID, placement) 不触发湿窗口）——4.1 顺手改透传 SetBlock 返回值。**组 2（调度迁移）全部完成。**
- Ruling: 3.1 拒绝码复用 `LoginProtocolViolation`(5) 接受 — 7 码枚举中 5 号语义「登录载荷违反协议」与域外视距精确匹配（评审核验非偷懒复用）；解码放行口走「交驱动回 LoginReject」与身份错误同模式（不裸断连），编码侧已拒故 wire 无合法域外产物。`LoginClient` 同步扩参（显式传值无隐藏默认）接受。版本矩阵两处同步、v39 失配拒绝经表达式循环自动继承。
- Task 3.1: complete (commits a3377f40..2cdd3599, review PASS, 1 Minor)。三层拒绝证据（校验矩阵/假 stream 驱动/真实 TCP 原始字节）+ golden 逐字节 + fuzz 新种子；配置域与协议域同域（config.Fields render.viewDistance Min2/Max64 既有约束）。Minor 路由：packet.go 与 login.go 的域校验谓词双写——3.2 接触时提炼 `ValidLoginViewDistance` 谓词。
- Ruling: 3.2 链路形态接受 — `contract.PlayerRestore.ViewDistance`（0=未声明、不持久化、wire 谓词拒 0 故零值无歧义）避免 ~17 处签名扰动；`+1` 换算全仓仅 entity 一处、`boundedSessionViewRadius` 唯一钳制点、RegisterPlayer 与 reconcile 同源计算不漂移；warp 重建订阅携带声明视距；probe（ViewRadius=0）经公式自然保持零视界现状。审计 `expectedRuntimeSubscriptionFields` 白名单登记是该门禁的设计流程。
- Task 3.2: complete (commits 1aa14463..ef14f83c, review PASS, 0 findings)。四 Scenario 双向精确集合相等 + 评审核实四变异全部咬人；谓词骑手落实（`ValidLoginViewDistance` + 闭域测试）；server 层接线测试用 ViewRadius=3 夹具的分层理由成立（OutboxCapacity 512 限制，数值钉住归 runtime 层）。
- Ruling: 3.3 评审 I-1 裁决——delta spec R3 的「任一时刻 MUST NOT 超过 N」与其自身 Scenario「淘汰 MUST 被推迟」+「读写全成功」在「候选全在途且需打开新 region」时逻辑不相容（阻塞等待方案与 R1 并行目标相悖且无延迟预算约束），按流程先修产物：R3 改为「稳态收敛（每次插入/归还时收缩到上限内）+ 在途窗口内 MAY 超限但 MUST NOT 超过上限加在途引用数 + 最后引用归还后下次治理收回」——提交 8b74fa2e，实现与测试断言形态自洽。
- Ruling: 3.3 修复轮的 skip-busy 选举（`electEvictableRegionLocked`）接受 — TDD 红灯证明旧「尾部在途即整轮让步」连修订后 R3 的窗口上界都违反（在途句柄沉尾后上方空闲句柄全部不可回收，无界增长）；选举淘汰首个 refs==0 使「超额必然全部在途 ⇒ ≤cap+在途」成立，且 LRU 语义保持（淘汰空闲集内最久未用者）。
- Ruling: 3.3 修复者观察路由——`CreateRegion` 临时文件 `.create-*`（region.go:66）同属备份过滤盲区（交错窗口更短、无 hook 停靠点），4.1 收尾顺手补一行过滤（不单开任务）。
- Task 3.3: complete (commits 2cfd1558→d19ffbd4 实现 + 8b74fa2e spec 修订 + 04582298 修复轮；review PASS-with-findings → 修复轮 → 复审 PASS)。拆锁形态：regionMu 只管缓存治理、chunk.Region.mu（既有）单 region 串行、五类独立锁、closing/closed 原子化、Close 排空等价；LRU+引用计数+skip-busy 选举；配置链路 server.Config.RegionHandleCacheCap(256)→OpenOptions；7+2 新测试钉住全部 spec Scenario。评审 I-2（Backup 漏 compact 临时文件，拆锁引入的真实回归）已修（一行过滤+门闩测试，TDD 先红后绿）。评审 M-2/M-3/M-4（Sync 全量 pin 扰动 LRU、Backup 与 Close 语义从互斥快照变尽力复制、ChunkKeys 快照粒度弱化为逐 region 提交点）记台账知悉——均为拆锁的既定代价，注释已声明。
- Ruling: 3.4 梯度偏离接受——视距梯度收敛 `2/4/6/8`（按登录顺序固定）而非 design 早期举例的 2/8/16/32：实测证明存档 job 单 FIFO 且 generate 排全部 load 之后、探针 Workers=1 ~3 job/tick，视距 32 的 4489 区块双趟 job 在探针窗口内零 Ready、streaming 完整性门禁必败；delta spec 只要求「按会话视距启用订阅」无数值钉死，数值属场景身份细节（代码注释+专用测试双重钉死）。design.md D6 措辞已同步修正。**瓶颈观察记档（后续候选）**：存档 job 队列「生成排全部装载之后」的单 FIFO 语义是大世界冷启动的首个加载优先级改进点，不属本 change。
- Task 3.4: complete (commits 5c3dff27..fe58ce76, review PASS-with-findings, 1 Important=design 措辞滞后已修, 2 Minor 观察)。StreamingObserver tick 路径成本有界（nil 门+当 tick 新切片+锁内同步与 InterestObserver 同模式）、探针 join 后汇总的 happens-before 核实、PerfReport 追加式、perfcheck `22:23` 唯一授权与旧报告兼容（真实 v15 基线自比较 exit 0）；M-1（端到端探针 -race 下被既有 skip 跳过、streaming 门禁只在非 race 生效——既有测试设计取舍，评审已用非 race 补证）与 M-2（冗余断言）记台账不动作。**组 3（流式收尾）全部完成。**
-（SDD 执行期逐任务追加：Task 完成记录 + 评审结论 + Ruling）
