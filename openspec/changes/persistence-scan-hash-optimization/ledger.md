# Ledger: persistence-scan-hash-optimization

## 2026-08-30 控制会话

- **立项**：用户以 brainstorming 发起「分析当前游戏中的性能卡点，完成优化」。全链路画像扫描（用户选定范围）在 main `3e2d07c1` 以一次性 CPU profile 插桩完成（still/flying/server 三段，画像与报告 `/tmp/mornlea-perf-spike-*`，插桩已还原、工作区无残留）。首轮误在 frame-stutter worktree 采数（shell cwd 残留），数据作废并重采；采集时机器负载 13.9–21，帧时绝对值不可比，CPU 占比排序有效。
- **卡点裁决（用户）**：mesh 管线（~60% CPU）、每帧可见性剔除（still 32.8%）与客户端快照校验被在途会话认领，排除；八会话探针 CPU 近零不动。可选目标呈报后用户选定「#3+#4 合并」：`PersistenceStats` 每 tick ×3 次 O(N) 扫描（15s/12.5% 单核）与 `Chunk.Hash` 逐体素 SHA-256（11.6s/9.6% 单核、persistence p99 55.4ms 主因）。
- **设计裁决（用户）**：Part A 采用增量计数器路线（A2，否决每 tick 记忆化 A1——O(N) 迭代随规模回归）；change 定名 `persistence-scan-hash-optimization`；backlog 行 F-07。
- **事实核验（控制会话）**：`EstimatedBytes` 双计入「脏且在途」为现行行为，逐位保持；方块内容变更必经 `Mutation`/`EnvironmentMutation` 事务推进 revision，非方块槽位在 `PayloadBytes` 中为常量——`(revision, chunk 指针)` 估算缓存键精确；`Chunk.Hash` 调用方全集 7 处（memory×1、disk×2、region×2、runtime/entity ChunkHash×2）全部自动受益；`Dimension` 单写者纪律免锁。
- 产物：proposal/design/tasks/delta spec（`chunk-persistence` 能力，4 Requirements、8 Scenarios）就绪，worktree `feat/F-07-persistence-scan-hash` 自 main `8fa1fc74` 建立。

## 任务执行记录

### Task 1：`Chunk.Hash` 缓冲编码 + 摘要等价性（2026-08-30）

- **Implementer**（fresh subagent）：交付 `2839651e perf(world): buffer chunk hash encoding`（`appendBlocksLE` 线性序批量导出 + 每区段一次 `hash.Write`；oracle 随机等价/重排不变/三态覆盖测试 + `BenchmarkChunkHash`）与 `87b6fb5f perf(storage): reuse chunk hash in disk save dedup`（`validateAndNormalizeSavesWithHash` 注入缝 + 指针键哈希缓存，单候选零哈希；探针钉住同批次同区块至多哈希一次）。基准（record-only）：UniformAir 621→121µs（5.1×）、Mixed 687→226µs（3.0×）、DenseDirect 710→260µs（2.7×），0 allocs/op 保持；`go test ./internal/world ./internal/storage/... -race` 全绿、vet/gofmt/archcheck 干净。
- **Implementer 报备的偏离（控制会话裁决接受）**：①摘要逐字节相同即契约，1.1 无法构造先红测试，红→绿真实发生在 1.3 探针；②为探针引入最小未导出注入缝 `hashChunkFunc`（生产固定 `(*world.Chunk).Hash`）；③disk.go 新增显式 `internal/world` import（依赖边已登记，memory.go 同边先例）；④边界 block ID 取 0/32767（15 位直接态上界）；⑤测试按关注点独立成 `chunk_hash_test.go`。
- SPEC 评审（独立子代理）：**PASS-with-notes**，零阻塞。6 项裁决全 PASS（任务完整性/三不变量可判定且假想改错必红/保存判例逐路径等价/线性序与小端编码实现级核验/四项偏离可接受/非目标遵守）；两条记录性备注：DirectStorage 子测试失败消息名不副实（后经修复轮处理）、基准数值跨机器波动属噪声（数值只记录）。
- QUALITY 评审（独立子代理）：**PASS-with-notes**，1 Important + 1 Nit + 2 Nit。正确性（三态边界/逃逸分析 0 allocs/8KiB 栈缓冲合理）、注释命名（任务编号正则零匹配）、注入缝与风格全 PASS。Important：`DirectStorage` 子测试空转断言（直接态打包与写入序无关，两 chunk 存储表示相同，断言不可能因打包差异失败，注释声称验证排列敏感性属虚假信心）；Nit：取值池含 map 迭代随机序、种子不决定输入。
- **修复轮 1（原 implementer）**：`ed796a8f` 删除空转子测试、新增诚实判例 `DirectBoundaryIDs`（直接态+边界 ID 断言缓冲编码==逐体素 oracle，命名与注释如实说明钉的是解包一致性而非排列不变性）、取值池先排序再打乱（随机性收敛到种子一处）；`03d56782` 补 `[32]byte`＝`sha256.Size` 注释。`go test ./internal/world ./internal/storage/... -race -count=1` 全绿，gofmt/vet/编号正则干净。控制会话核验 diff 后接受修复，双评审结论闭合。


