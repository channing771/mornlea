# Ledger

## 执行记录段

| 日期 | 阶段 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-27 | 0 认领与批准 | **Ruling（用户）:** ① 控制会话核实 A-02（活跃 zcode 会话 5 分钟内有产出）与 B-17（opencode-implementer 已交设计+计划）均不可抢、其余排队行依赖未满足后，用户选定 D-02 晋升认领，语义边界取「单机暂停最小闭环」：权威 tick 冻结仅作用单机嵌入服，TCP 远程会话不宣称暂停；② 菜单条目两项方案：「返回游戏 / 退回主菜单」，设置入口顺延。批准来源为控制会话内问答显式选择（两轮）。 | 积压表晋升+认领 docs-only 提交 `b10bbfe7` 快进推送 origin/main；worktree `feat/D-02-pause-menu` 基于 origin/main 创建 |
| 2026-08-27 | 1 契约产物 | 创建 `pause-menu` 全套产物：proposal/design/tasks/ledger + `egui-tool-ui` delta（三条 ADDED Requirements：Esc 开合覆盖层含远程不宣称暂停、权威门冻结可恢复、退回主菜单拆解会话）。关键取舍四条记录于 design.md（服务端 tick 门优于客户端停发/cancel ctx；原子布尔无锁；TCP 同页不同标注；布局段内部版本 3→4 沿 D-03 先例且客户端 ABI 保持 v9）。独占文件集见 proposal「Impact」。 | 待跑：`openspec validate --all --strict --no-interactive` |

### 版本矩阵（基线 `b10bbfe7`）

全部不变：协议 v27、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、engine ABI v7、client ABI v9、benchmark scenario v19。唯一结构变化为 UI 下行布局段内部版本 3→4（仓库自有契约，沿 D-03 先例）；capture 场景表与视觉 golden 零变化。
