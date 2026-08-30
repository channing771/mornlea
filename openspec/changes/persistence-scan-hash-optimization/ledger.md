# Ledger: persistence-scan-hash-optimization

## 2026-08-30 控制会话

- **立项**：用户以 brainstorming 发起「分析当前游戏中的性能卡点，完成优化」。全链路画像扫描（用户选定范围）在 main `3e2d07c1` 以一次性 CPU profile 插桩完成（still/flying/server 三段，画像与报告 `/tmp/mornlea-perf-spike-*`，插桩已还原、工作区无残留）。首轮误在 frame-stutter worktree 采数（shell cwd 残留），数据作废并重采；采集时机器负载 13.9–21，帧时绝对值不可比，CPU 占比排序有效。
- **卡点裁决（用户）**：mesh 管线（~60% CPU）、每帧可见性剔除（still 32.8%）与客户端快照校验被在途会话认领，排除；八会话探针 CPU 近零不动。可选目标呈报后用户选定「#3+#4 合并」：`PersistenceStats` 每 tick ×3 次 O(N) 扫描（15s/12.5% 单核）与 `Chunk.Hash` 逐体素 SHA-256（11.6s/9.6% 单核、persistence p99 55.4ms 主因）。
- **设计裁决（用户）**：Part A 采用增量计数器路线（A2，否决每 tick 记忆化 A1——O(N) 迭代随规模回归）；change 定名 `persistence-scan-hash-optimization`；backlog 行 F-07。
- **事实核验（控制会话）**：`EstimatedBytes` 双计入「脏且在途」为现行行为，逐位保持；方块内容变更必经 `Mutation`/`EnvironmentMutation` 事务推进 revision，非方块槽位在 `PayloadBytes` 中为常量——`(revision, chunk 指针)` 估算缓存键精确；`Chunk.Hash` 调用方全集 7 处（memory×1、disk×2、region×2、runtime/entity ChunkHash×2）全部自动受益；`Dimension` 单写者纪律免锁。
- 产物：proposal/design/tasks/delta spec（`chunk-persistence` 能力，4 Requirements、8 Scenarios）就绪，worktree `feat/F-07-persistence-scan-hash` 自 main `8fa1fc74` 建立。

## 任务执行记录

（后续按任务组追加：implementer 派发、SPEC/QUALITY 评审结论、修复轮与裁决。）
