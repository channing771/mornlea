# Ledger: persistence-scan-hash-optimization

## 2026-08-30 控制会话

- **立项**：用户以 brainstorming 发起「分析当前游戏中的性能卡点，完成优化」。全链路画像扫描（用户选定范围）在 main `3e2d07c1` 以一次性 CPU profile 插桩完成（still/flying/server 三段，画像与报告 `/tmp/mornlea-perf-spike-*`，插桩已还原、工作区无残留）。首轮误在 frame-stutter worktree 采数（shell cwd 残留），数据作废并重采；采集时机器负载 13.9–21，帧时绝对值不可比，CPU 占比排序有效。
- **卡点裁决（用户）**：mesh 管线（~60% CPU）、每帧可见性剔除（still 32.8%）与客户端快照校验被在途会话认领，排除；八会话探针 CPU 近零不动。可选目标呈报后用户选定「#3+#4 合并」：`PersistenceStats` 每 tick ×3 次 O(N) 扫描（15s/12.5% 单核）与 `Chunk.Hash` 逐体素 SHA-256（11.6s/9.6% 单核、persistence p99 55.4ms 主因）。
- **设计裁决（用户）**：Part A 采用增量计数器路线（A2，否决每 tick 记忆化 A1——O(N) 迭代随规模回归）；change 定名 `persistence-scan-hash-optimization`；backlog 行 F-07。
- **事实核验（控制会话）**：`EstimatedBytes` 双计入「脏且在途」为现行行为，逐位保持；方块内容变更必经 `Mutation`/`EnvironmentMutation` 事务推进 revision，非方块槽位在 `PayloadBytes` 中为常量——`(revision, chunk 指针)` 估算缓存键精确；`Chunk.Hash` 调用方全集 7 处（memory×1、disk×2、region×2、runtime/entity ChunkHash×2）全部自动受益；`Dimension` 单写者纪律免锁。
- 产物：proposal/design/tasks/delta spec（`chunk-persistence` 能力，4 Requirements、7 Scenarios）就绪，worktree `feat/F-07-persistence-scan-hash` 自 main `8fa1fc74` 建立。

## 任务执行记录

### Task 1：`Chunk.Hash` 缓冲编码 + 摘要等价性（2026-08-30）

- **Implementer**（fresh subagent）：交付 `2839651e perf(world): buffer chunk hash encoding`（`appendBlocksLE` 线性序批量导出 + 每区段一次 `hash.Write`；oracle 随机等价/重排不变/三态覆盖测试 + `BenchmarkChunkHash`）与 `87b6fb5f perf(storage): reuse chunk hash in disk save dedup`（`validateAndNormalizeSavesWithHash` 注入缝 + 指针键哈希缓存，单候选零哈希；探针钉住同批次同区块至多哈希一次）。基准（record-only）：UniformAir 621→121µs（5.1×）、Mixed 687→226µs（3.0×）、DenseDirect 710→260µs（2.7×），0 allocs/op 保持；`go test ./internal/world ./internal/storage/... -race` 全绿、vet/gofmt/archcheck 干净。
- **Implementer 报备的偏离（控制会话裁决接受）**：①摘要逐字节相同即契约，1.1 无法构造先红测试，红→绿真实发生在 1.3 探针；②为探针引入最小未导出注入缝 `hashChunkFunc`（生产固定 `(*world.Chunk).Hash`）；③disk.go 新增显式 `internal/world` import（依赖边已登记，memory.go 同边先例）；④边界 block ID 取 0/32767（15 位直接态上界）；⑤测试按关注点独立成 `chunk_hash_test.go`。
- SPEC 评审（独立子代理）：**PASS-with-notes**，零阻塞。6 项裁决全 PASS（任务完整性/三不变量可判定且假想改错必红/保存判例逐路径等价/线性序与小端编码实现级核验/四项偏离可接受/非目标遵守）；两条记录性备注：DirectStorage 子测试失败消息名不副实（后经修复轮处理）、基准数值跨机器波动属噪声（数值只记录）。
- QUALITY 评审（独立子代理）：**PASS-with-notes**，1 Important + 1 Nit + 2 Nit。正确性（三态边界/逃逸分析 0 allocs/8KiB 栈缓冲合理）、注释命名（任务编号正则零匹配）、注入缝与风格全 PASS。Important：`DirectStorage` 子测试空转断言（直接态打包与写入序无关，两 chunk 存储表示相同，断言不可能因打包差异失败，注释声称验证排列敏感性属虚假信心）；Nit：取值池含 map 迭代随机序、种子不决定输入。
- **修复轮 1（原 implementer）**：`ed796a8f` 删除空转子测试、新增诚实判例 `DirectBoundaryIDs`（直接态+边界 ID 断言缓冲编码==逐体素 oracle，命名与注释如实说明钉的是解包一致性而非排列不变性）、取值池先排序再打乱（随机性收敛到种子一处）；`03d56782` 补 `[32]byte`＝`sha256.Size` 注释。`go test ./internal/world ./internal/storage/... -race -count=1` 全绿，gofmt/vet/编号正则干净。控制会话核验 diff 后接受修复，双评审结论闭合。



### Task 2：`realm` 持久化统计增量记账 + 脏区块索引（2026-08-30）

- **Implementer**（fresh subagent）：交付 `7a63ff52 perf(realm): maintain persistence stats via incremental dirty index`（4 文件 +850/−22，仅 `internal/sim/realm`）。记账三件套 `refreshRecord`/`settleRecord`/`rebuildStats`；14 类挂接点逐一落地（Touch、Mutation.Commit、四态转换、ApplyGenerated/ApplyLoaded、MarkFailed×2、RequestUnload/CancelUnload、deleteCleanUnloading、派发在途字节、ApplyPersisted、FailPersistence、SetDimension、NewState）；`PersistenceStats` 变 O(维度数) 纯聚合读、`PersistenceSnapshots` 只迭代脏索引并复验全部四项过滤。oracle（全量扫描统计+全记录候选收集）移入测试；随机操作序列属性测试、双计入钉住、Dirty() 探针 O(1) 成本测试（旧实现红 2/2000，新 0/0）、SetDimension 重建覆盖。environment 不变量结论（全部内容写入经 Mutation.Commit 推进 revision；非方块槽位在 PayloadBytes 中为常量）写入 `recordStats` doc comment 并有专测。TDD 期间属性测试抓出真 bug（dirty→dirty 估算重算时旧值先被覆盖致聚合漏增量）并修复（先存 previousEstimate 再重算）。验证：sim 全树与 server 全树 `-race -count=1` 全绿（server 178s）、gofmt/vet 干净。
- **Implementer 报备的偏离（待评审裁决）**：①聚合放 `Dimension` 级而非 design 写的 State 级（Dimension 写入方法无 State 反向引用，避免扩面/循环；State 只持在途字节；spec 成本 Requirement 仍逐字满足）；②每记录缓存形状裁剪掉冗余字段；③`InFlightChunks=len(inFlightSaves)` 在「SetDimension 悬空在途」不可达态下与逐记录计数有理论差异（原实现该态 panic，生产行为不变，已注释+测试说明）；④O(1) 探针缝挂 `ChunkRecord.Dirty()`（生产恒 nil）。
- SPEC/QUALITY 双评审：进行中。
- SPEC 评审（独立子代理）：**PASS-with-notes**，零阻塞。6 项裁决全 PASS：14 类挂接点逐一核对落地（错误路径零半记账）；oracle 与旧实现逐行一致且从生产移除、操作池覆盖 16 类迁移操作（超出 spec 的 9 类）+SetDimension 专测、双计入显式断言、假想改错必红（5 类故意改错形态推演均会红）；O(1) 探针旧实现真红（2 vs 2000）；候选收集过滤/排序/预算与父提交逐字相同；非目标遵守（导出面不变、双计入未修正、无锁）。记录性发现 A：估算缓存键精确性实际依赖表示级不变量（palette 只增不减），「写后无事务回滚」路径当前被守卫排除不可达，已按裁决写入 design D1 不变量警示。
- QUALITY 评审（独立子代理）：**PASS-with-notes**，仅 3 Nit 无 Critical/Important。diff 顺序修复核验完整（先存 previousEstimate）；33 处 refresh/settle 调用点穷尽配对无双记；缓存键失效覆盖同 revision 换 chunk；环境不变量逐路径核验属实；并发边界零新增锁；性能反噬评估惰性重算正确（PayloadBytes 是纯元数据求和无逐方块遍历）；oracle/属性测试有牙齿。Nit：SetDimension 悬空在途的未来结算责任、`recordStats.dirty` 命名语义、派发处可复用缓存估算（收益微小）。
- **控制会话裁决**：①接受聚合放 `Dimension` 级（design D1 已同步修订并记录理由）；③重分类为 design 一致（`SetDimension` 生产零调用方，不可达态差异已在测试注释说明）；②④记录在案不构成偏离；发现 A 写入 design D1 不变量警示。三条 Nit 记录不修（SetDimension 无生产调用方、命名语义已有注释、微优化收益不足）。Task 2 无修复轮，双评审结论闭合。

### Task 3：收尾门禁与整分支终审（2026-08-30）

- **3.1 门禁（控制会话串行执行）**：`gofmt -l`（engine/ 外零输出）、`go vet ./...` 干净、受影响四组包 `go test ./internal/world ./internal/storage/... ./internal/sim/... ./internal/server/... -race -count=1` 14 包全绿（唯一非 ok 行是 storagedef 无测试文件的提示）、`go test ./internal/archcheck -count=1` ok。
- **3.2 OpenSpec**：`openspec validate persistence-scan-hash-optimization --strict` 通过；`openspec validate --all --strict --no-interactive` 79 passed / 0 failed。
- **3.3 record-only 前后对比（数值只记录，方向性参考）**：前=main `3e2d07c1` 无优化（负载 14–21，带 CPU 画像插桩），后=本分支（负载 4.7–6，无插桩）。persistence：p50 8.43→3.87ms（−54%）、p95 29.58→12.66（−57%）、**p99 55.43→24.48ms（−56%）**、max 113.4→70.8（−38%），快照数 5002→4867——该组是与负载最不敏感的保存路径直测，为最干净信号。still p99 48.1→13.7ms、flying p99 36.0→30.9ms（受负载差混杂，只作方向参考）。ticks p50 0.242→0.396ms / p99 0.444→0.744ms：小幅上升，单次 200 样本无统计力且远低于 10ms 门禁；不排除记账在 Touch/Commit 的每变更成本，归档后如复现可另行核查。两份报告存 `/tmp/mornlea-perf-spike-main.json` 与 `/tmp/mornlea-f07-after.json`；不覆盖任何基线 JSON。
- **3.4 整分支终审（独立子代理，只读）**：**PASS**。五维度全过：产物-代码一致（14 挂接点/D2 三分支/裁决同步逐项核对）、Task1×Task2 交叉无未处理影响（`Compact()` 生产调用点穷尽核验）、分支 diff 14 文件全在声明范围、契约足迹干净（无协议/schema/ABI/scenario/golden，导出面零变化，perf-baseline* 未触碰）、遗留清单五项均已在 design/ledger 记录。新发现仅一处文档计数笔误（本节已修正：delta spec 实为 7 个 Scenario）。前置条件（补跑门禁/进度文档）由本节与 progress.md 落地。
