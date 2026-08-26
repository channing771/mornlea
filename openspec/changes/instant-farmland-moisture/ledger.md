# instant-farmland-moisture 执行账本

## Task Status

| Task | Implementer | Spec Review | Quality Review | Repair Rounds | Status |
|---|---|---|---|---:|---|
| 1 | `ses_fc3afd2a2ffeq80iW8UIKYPasR` | approved | approved | 0 | complete |
| 2 | `ses_fc39ce278ffew8TsrLujyjLcba` | approved | approved | 0 | complete |
| 3 | `ses_fc38ab39dffeZ3omLdaHb19SUl` | approved | approved | 1 | complete |
| 4 | `ses_fc35b03a0ffeMSdnFaxeADUFhQ` | approved | approved | 1 | complete |
| 5 | `ses_fc3255961ffevR5E7w69UMj8wQ` | approved | approved | 2 | complete |

## Rulings

| ID | Task | Finding | Decision | Evidence |
|---|---|---|---|---|
| B09-T1-R1 | 1 | Brief 中的原始 hash 命令同时散列 `go test` 的动态耗时汇总行，未改代码连续运行也会得到不同 hash | 保留原始命令结果，并另用排除 `ok` 汇总行的排序入口 hash 裁决测试集合；不改测试或生产代码迎合不稳定 hash | split 后原始 hash 连续为 `b11a78ad...`、`9b9138ce...`；稳定入口 hash 为 `dc4e65d...`，逐声明重构与 `HEAD:internal/sim/crop_test.go` 仅差文件头 import/空行 |
| B09-T1-R2 | 1 | Reviewer 无法从 diff 验证 feature branch commit 是否获得授权 | 控制会话确认用户在 Task 1 派发前已显式授权 B-09 每任务 commit，授权不含 push/merge/PR；无需代码修复 | 对话授权选择“授权任务 commits”；commit `db2a6a7d` 只含 Task 1 文件 |
| B09-T2-R1 | 2 | Reviewer 无法只从 commit diff 验证 RED 时序、命令输出与提交后 worktree 状态 | 接受 implementer 在 ignored SDD report 中保留的逐命令证据，并以控制会话观察到的 clean status 补足；无需重跑或修改代码 | `task-2-report.md` 第 76–223 行记录 RED/GREEN，控制会话在 `5fb868f8` 后运行 `git status --short --branch` 仅输出 branch header |
| B09-T3-R1 | 3 | Required `TestFluid` 回归揭示 brief 文件表外的 `fluid_crop_test.go` 仍要求“耕地保持干态”，与同 tick 湿润契约冲突 | 接受只更新这两条直接冲突的集成断言；不扩大生产范围 | 首轮 reviewer 裁定该范围必要且聚焦；`go test ./internal/sim -run 'TestFluid' -race -count=1` 修复后通过 |
| B09-T4-R1 | 4 | Brief 文件表未列 `farmland_moisture.go`、`crop_growth_test.go` 与 `farmland_moisture_rescan_test.go`，但显式任务与必跑门禁要求迁移常量并清理剩余随机湿度夹具 | 接受常量迁移及两处测试夹具最小修正；不接受额外生产行为 | 首轮 reviewer 裁定三处范围聚焦且有必要；完整 sim race 与 `TestCrop|TestFarmland` race 均通过 |

## Verification

| Command | Result | Evidence |
|---|---|---|
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.42s` |
| `go test ./internal/sim -list . \| LC_ALL=C sort \| shasum -a 256` (before) | recorded | `0ede9a0052a00d696d4b81f1952ff57c2bd587bdb5c30f79a304aae1c9b4470f  -` |
| `go test ./internal/sim -race -count=1` (before) | pass | `ok github.com/channing771/mornlea/internal/sim 14.011s` |
| `gofmt -w internal/sim/crop_sampling_test.go internal/sim/crop_growth_test.go internal/sim/crop_cost_test.go internal/sim/crop_helpers_test.go internal/sim/farmland_moisture_integration_test.go` | pass | no output |
| `go test ./internal/sim -list . \| LC_ALL=C sort \| shasum -a 256` (after) | unstable summary included | first `b11a78ad60e60b82a85912ab8fc65372a6a13e26d7593266904a4f6c83378831  -`; unchanged rerun `9b9138cee865c3d479eb89a941dab5919eff730b7e563d2197b1f39fec785fb5  -` |
| `go test ./internal/sim -list . \| LC_ALL=C sort \| rg -v '^ok[[:space:]]' \| shasum -a 256` | pass | before/after sorted entry hash `dc4e65d2da8ff1b5c950ac42878d28a01a5dfc1f7bf5a25826456c9c068b8e65  -` |
| `go test ./internal/sim -race -count=1` (after) | pass | `ok github.com/channing771/mornlea/internal/sim 14.038s` |
| declaration reconstruction `diff -u` against `HEAD:internal/sim/crop_test.go` | pass | no output; declarations, comments and bodies match byte-for-byte in original order |
| `git diff --check` | pass | no output |
| `go vet ./internal/sim` | pass | no output |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 5.560s` |
| `gofmt -l internal/sim/crop_sampling_test.go internal/sim/crop_growth_test.go internal/sim/crop_cost_test.go internal/sim/crop_helpers_test.go internal/sim/farmland_moisture_integration_test.go` | pass | no output |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |
| Task 1 independent spec/quality review | approved | no Critical/Important/Minor；仅 commit 授权为 diff 外不可验证项，经 `B09-T1-R2` 裁决 |

## Task 1 Files

- `internal/sim/crop_sampling_test.go`
- `internal/sim/crop_growth_test.go`
- `internal/sim/crop_cost_test.go`
- `internal/sim/crop_helpers_test.go`
- `internal/sim/farmland_moisture_integration_test.go`
- `internal/sim/crop_test.go` (deleted)
- `openspec/changes/instant-farmland-moisture/ledger.md`

## Task 2 Implementation Evidence

- 新增 `farmlandMoistureState` 候选 FIFO、惰性去重集合、固定阈值稳定压紧、独立重扫 FIFO 与平面游标；全部状态只由权威 tick 路径读写。
- 反向流体窗口按 `y,z,x` 固定顺序枚举至多 162 个有效候选；直接与反向入队均拒绝世界 Y 范围外坐标并按首次出现去重。
- 候选处理先丢弃 active Ready scope 外或失效维度的工作，再按每 tick 65,536 次方块读取硬预算处理；耕地邻域判断开始前预留完整 162 次查询，不保存跨 tick 的部分结果。
- 重扫 job 覆盖 `24×24×384 = 221,184` 个位置，按 `y,z,x` 平面游标跨 tick 续扫；事件候选优先消费共享预算，重扫发现的耕地只入队并从下一 tick 起处理。
- Task 2 未接入 `Engine.Step`、`fluidWorld.SetBlock`、`advanceFluids` 新 scope 或翻地成功路径；这些生产触发点保留给 Task 3。

## Task 2 RED Evidence

| Stage | Command | Expected failure |
|---|---|---|
| queue | `go test ./internal/sim -run 'TestFarmlandMoisture(ReverseWindow\|Queue)' -count=1` | compile failed：`enqueueFarmlandMoistureAroundFluid`、`farmlandMoisture`、`farmlandMoistureKey` 等尚不存在 |
| budget | `go test ./internal/sim -run 'TestFarmlandMoisture(Dry\|Budget\|Determin)' -race -count=1` | 4 tests failed：读取计数均为 0，过预算待办 10 tick 后仍剩 65,384 项 |
| rescan | `go test ./internal/sim -run 'TestFarmlandMoistureRescan' -race -count=1` | compile failed：重扫尺寸常量、坐标还原函数与 `enqueueChunk` 尚不存在 |

## Task 2 Verification

| Command | Result | Evidence |
|---|---|---|
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.32s` |
| `go test ./internal/sim -run 'TestFarmlandMoisture(ReverseWindow\|Queue)' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 1.358s` |
| `go test ./internal/sim -run 'TestFarmlandMoisture(Dry\|Budget\|Determin)' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 1.838s` |
| `go test ./internal/sim -run 'TestFarmlandMoistureRescan' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 2.777s` |
| `gofmt -w internal/sim/farmland_moisture.go internal/sim/farmland_moisture_queue_test.go internal/sim/farmland_moisture_budget_test.go internal/sim/farmland_moisture_rescan_test.go internal/sim/engine.go` | pass | no output |
| `go test ./internal/sim -run 'TestFarmlandMoisture(ReverseWindow\|Queue\|Budget\|Determin\|Rescan)' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 1.802s` |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 7.579s` |
| `go vet ./internal/sim` | pass | no output |
| `git diff --check` | pass | no output |
| `openspec validate --all --strict --no-interactive` (Task 2) | pass | `Totals: 66 passed, 0 failed (66 items)` |
| Task 2 independent spec/quality review | approved | no Critical/Important/Minor；diff 外证据项经 `B09-T2-R1` 裁决 |

Task 2 已通过独立规格评审与质量评审；实现没有提前进入 Task 3。

## Task 3 Implementation Evidence

- `fluidWorld.SetBlock` 仅在真实写入完成且 old/new `core.IsFluid` membership 改变后反向登记湿度候选；相同编号与流体等级互换均不登记。作物冲毁分支在掉落容量可能拒绝写入的前提下读回最终方块，再决定是否登记，避免失败写入产生假候选。
- `advanceFluids` 在既有稳定新 scope 循环中同时登记独立湿度重扫，直接复用 `Engine.fluidScope`，未修改 `internal/fluid`、due-tick、`recordChange` 或流体预算。
- `executeTillSoil` 只在 `Dimension.SetBlock` 返回 `changed=true` 且 `recordChange` 完成后登记目标，位置早于疲劳与耐久结算；既有拒绝表验证不增加湿度待办。
- `Engine.Step` 新增 `phaseFarmlandMoistureAdvance`，固定顺序为 fluid → moisture → crop；三个阶段继续共用同一 `pendingChunkChanges` map，成功翻地同 tick 只发布最终湿耕地变更。
- fresh engine 恢复与区块重入集成测试经完整 `Step` 驱动：逐 tick 断言湿度读取不超过 65,536；重入测试在半截 job 离开后断言游标清零，并以再次进入后的首 tick 游标恰为 65,536 证明从零重启。
- 流体性能样本改为分别记录 `phaseFluidAdvance`→`phaseFarmlandMoistureAdvance` 与 moisture→`phaseCropAdvance`；`step` 总耗时、队列规模坐标、风险下界与 overflow/完整性守卫均未放宽。
- `fluid_crop_test.go` 的旧随机延迟断言随 Task 3 的真实作物→流体 membership 写入更新为同 tick 湿润；未修改 Task 4 的 `crop.go` 随机 tick 逻辑或成本指标。
- 协议、存档 schema、engine/client ABI、benchmark scenario 与 capture golden 均未修改；Task 3 已在一轮测试证明修复后通过规格评审和质量评审。

## Task 3 RED Evidence

| Stage | Command | Expected failure |
|---|---|---|
| fluid membership | `go test ./internal/sim -run 'TestFarmlandMoistureFluid' -count=1` | `TestFarmlandMoistureFluidMembershipChanges`、距离 4 同层/上一层、作物冲毁和跨区块用例均失败：真实流体写入后耕地仍为干；流体等级互换控制用例通过 |
| till | `go test ./internal/sim -run 'TestTill(InWaterRange\|Rejects)' -count=1` | `TestTillInWaterRangePublishesWetFarmland` 失败：翻地结果为干耕地；既有拒绝表的待办不变断言通过 |
| phase/recovery | `go test ./internal/sim -run 'TestCompanionActionAppliesInIDOrderAfterPlayers\|TestFarmlandMoisture(Restart\|Reentry)' -count=1` | compile failed：`undefined: phaseFarmlandMoistureAdvance`，证明 standalone phase 尚未存在 |

## Task 3 GREEN Evidence

| Command | Result | Evidence |
|---|---|---|
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.28s` |
| `go test ./internal/sim -run 'TestFarmlandMoistureFluid' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 1.065s` |
| `go test ./internal/sim -run 'TestTill(InWaterRange\|Rejects)' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 0.389s` |
| `go test ./internal/sim -run 'TestCompanionActionAppliesInIDOrderAfterPlayers\|TestFarmlandMoisture(Restart\|Reentry)' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 0.981s` |
| `gofmt -w internal/sim/fluid.go internal/sim/farming.go internal/sim/engine_step.go internal/sim/farmland_moisture_integration_test.go internal/sim/farming_test.go internal/sim/companion_action_test.go internal/sim/fluid_perf_test.go` | pass | no output |
| `go test ./internal/sim -run 'TestFarmlandMoistureFluid\|TestTill\|TestCompanionActionAppliesInIDOrderAfterPlayers\|TestFarmlandMoisture(Restart\|Reentry)' -race -count=1` | pass | final rerun: `ok github.com/channing771/mornlea/internal/sim 2.680s` |
| `go test ./internal/sim -run 'TestFluid' -race -count=1` | pass | final rerun: `ok github.com/channing771/mornlea/internal/sim 7.768s`; first run exposed and led to correction of the stale dry-farmland integration expectation |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.178s` |
| `go vet ./internal/sim` | pass | no output |
| `gofmt -l` on all Task 3 Go files including `fluid_crop_test.go` | pass | no output |
| `git diff --check` | pass | no output |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |

## Task 3 Review Repair Round 1

本轮只修复测试证明与注释，不修改生产行为；复审已通过，不勾选 `tasks.md` 留待 change 收尾统一核对。

| Finding | Disposition | Evidence / Repair |
|---|---|---|
| Important 1：重入测试以湿耕地开始并恢复有水，cursor 可在未发现耕地时假绿 | valid，已修复 | 再次恢复邻块前用 `SetBlockForTest` 强制边界耕地为陈旧干态，并证明无旧候选；恢复首 tick 既断言 cursor 从 0 到 65,536，又断言重扫明确登记该耕地，后续完整 `Step` 必须把它改湿 |
| Important 2：拒绝翻地与容量满冲毁的 no-enqueue 证明在消费后观察 | 核心问题 valid；capacity 描述部分不实 | 翻地测试原先确实在完整 `Step` 后比较 `pending` 长度，会漏掉被同 tick 消费的错误候选；但当前 capacity 测试此前完全没有湿度候选断言，并非“消费后断言”。新增共享 phase watcher，在 `phaseFarmlandMoistureAdvance` 通知点、`advanceFarmlandMoisture` 调用前按目标 key 累计观察；三个翻地拒绝表与 capacity-full 窗口均要求未见候选 |
| Minor 3：性能报告注释仍称三条最坏口径 | valid，已修复 | 注释改为四条，并列出整 tick、流体、湿度与队列四种口径 |
| Minor 4：首次 `UnregisterSession` 把 bool 当删除成功 | valid，已修复 | `UnregisterSession` 的 bool 是 `hasSnapshot`；首次与第二次注销现在都在调用前后直接检查 `engine.sessions`，不再以 bool 推断删除 |

### Repair Mutation Evidence

常规 RED 对本轮不成立：生产行为在 `3f86925a` 已正确，本轮修的是测试对错误实现的辨别力。按 review 要求使用瞬态 faulty setup / mutation 验证测试会红，随后逐项恢复；最终 `git diff` 不含任何生产文件。

| Mutation / Faulty Setup | Command | Expected failure |
|---|---|---|
| 保留重扫 cursor 推进但临时跳过 `runFarmlandMoistureRescans` 的耕地入队 | `go test ./internal/sim -run '^TestFarmlandMoistureReentryRestartsRescan$' -count=1` | fail：`再次重入的重扫没有发现并登记边界耕地`；证明 cursor 单独移动已不能让测试假绿 |
| watcher 注册时临时注入目标候选，模拟拒绝路径错误入队 | `go test ./internal/sim -run '^TestTillRejects' -count=1` | 三组拒绝表均 fail：`在湿度阶段消费前产生了目标候选` |
| 同一 watcher 临时注入边界耕地候选，模拟 capacity-full 写入错误入队 | `go test ./internal/sim -run '^TestFluidCropCapacityFullRejectsAndRetriesUntilSlotFreed$' -count=1` | fail：`容量拒绝的作物写入在湿度阶段消费前产生了耕地候选` |

### Repair Focused GREEN

| Command | Result | Evidence |
|---|---|---|
| `go test ./internal/sim -run '^TestFarmlandMoistureReentryRestartsRescan$' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 0.657s` |
| `go test ./internal/sim -run '^TestTillRejects' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 0.910s` |
| `go test ./internal/sim -run '^TestFluidCropCapacityFullRejectsAndRetriesUntilSlotFreed$' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 0.397s` |

### Repair Verification

| Command | Result | Evidence |
|---|---|---|
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.31s` |
| `gofmt -w internal/sim/helpers_test.go internal/sim/farming_test.go internal/sim/fluid_crop_test.go internal/sim/farmland_moisture_integration_test.go internal/sim/fluid_perf_test.go` | pass | no output |
| `go test ./internal/sim -run 'TestFarmlandMoistureFluid\|TestTill\|TestCompanionActionAppliesInIDOrderAfterPlayers\|TestFarmlandMoisture(Restart\|Reentry)' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 3.091s` |
| `go test ./internal/sim -run 'TestFluid' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 7.498s` |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.393s` |
| `go vet ./internal/sim` | pass | no output |
| `gofmt -l` on all repair Go files | pass | no output |
| `git diff --check` | pass | no output |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |
| Task 3 independent spec/quality re-review | approved | repair round 1 关闭 2 Important 与 2 Minor；无新增 finding |

## Task 4 Implementation Evidence

- `advanceCrops` 每阶段入口只重置一次 `cropCellsExamined` 与 `cropBlockReads`；每条随机样本的 `Chunk.BlockAt` 计一次，只有作物再为正下方的 `Dimension.BlockAt` 计一次。
- `advanceCropCell` 对非作物读取一次后立即返回；已删除随机耕地分支和 `crop.go` 内的 `farmlandIsWet`，湿润几何常量移到 `farmland_moisture.go` 与唯一查询实现同处。
- 作物 benchmark 全部以解析式守卫 `cropBlockReads <= 2*cropCellsExamined` 并同时报告 `cells/op`、`block_reads/op`；全耕地 benchmark 额外要求两者精确相等，未增加墙钟阈值。
- 旧湿度用例改经真实 `fluidWorld.SetBlock` 生产 membership 事件并在无积压时同阶段断言；`TestFarmlandTurnsDryAfterWaterRemoved` 不再用 `SetBlockForTest` 移水，fixture-only writer 语义未修改。
- 随机作物测试夹具改为显式写入一致的持久化干/湿编号；未增加兼容 fallback，未改湿度队列、预算、阶段顺序、流体 hook、协议/schema/ABI/scenario/golden。

## Task 4 RED / GREEN Evidence

| Stage | Command | Result | Evidence |
|---|---|---|---|
| RED | `go test ./internal/sim -run 'TestCrop(TickCost\|AllFarmlandReads)' -count=1` | expected fail | `TestCropAllFarmlandReadsEachSampleOnce`: `全耕地阶段读取=0，想要每个样本一次、共 1536`; package `FAIL` |
| focused GREEN | same command after crop-only implementation | pass | `ok github.com/channing771/mornlea/internal/sim 0.849s` |
| stale fixture probe | `go test ./internal/sim -run 'TestCrop\|TestFarmland' -race -count=1` | expected adaptation failures | replay field made no changes because all persisted farmland IDs were dry；rescan scope test reused an already-advanced initial cursor |
| fixture GREEN | individual replay/rescan tests after fixture correction | pass | `ok .../internal/sim 0.859s`; `ok .../internal/sim 0.342s` |
| full-package probe | `go test ./internal/sim -count=1` | exposed one stale test | `TestZeroGrowthChanceNeverAdvancesCrop` still expected random ticks to wet dry farmland |
| final focused GREEN | `go test ./internal/sim -run 'TestCrop\|TestFarmland' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 4.125s` |
| final full GREEN | `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 35.512s` |

## Task 4 Benchmark Evidence

命令：`go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5`；结果 `PASS`，package `ok` 为 `25.231s`。20/20 条 benchmark 样本均同时打印 `cells/op` 与 `block_reads/op`，全部 `0 B/op`、`0 allocs/op`。

| Benchmark | ns/op samples | median ns/op | block_reads/op samples | median block_reads/op | cells/op |
|---|---|---:|---|---:|---:|
| FullInterestBarren | 174196, 172912, 172741, 176981, 175544 | 174196 | 14400, 14400, 14400, 14400, 14400 | 14400 | 14400 |
| FullInterestPlanted | 174037, 176932, 173847, 173600, 173041 | 173847 | 14400, 14400, 14401, 14400, 14400 | 14400 | 14400 |
| FullInterestDense | 175827, 175634, 176171, 176244, 175967 | 175967 | 14448, 14439, 14430, 14435, 14437 | 14437 | 14400 |
| AllFarmland | 2172, 2167, 2170, 2239, 2169 | 2170 | 72, 72, 72, 72, 72 | 72 | 72 |

## Task 4 Verification

| Command | Result | Evidence |
|---|---|---|
| `gofmt -w` + `gofmt -l` on all Task 4 Go files | pass | no output from format check |
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.44s` |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.247s` |
| `go vet ./internal/sim` | pass | no output |
| `git diff --check` | pass | no output |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |

Task 4 实现与验证完成，并在一轮恢复覆盖修复后通过规格与质量评审；`tasks.md` 留待 change 收尾统一核对。

## Task 4 Review Repair Round 1

### Finding Disposition

| Finding | Disposition | Evidence / Repair |
|---|---|---|
| Important：Task 4 删除旧「邻块未加载后随机 tick 变干」断言，却没有等价的重入 wet-to-dry 有界恢复覆盖；现有 reentry 只证明 dry-to-wet | valid，已补测试；生产代码无需修改 | active delta 的「区块重入后恢复边界湿度」明确覆盖离开期间**失去或获得**邻块流体；新增 `TestFarmlandMoistureReentryRecoversStaleWetFarmland` 以完整 `Engine.Step` 卸出含水邻块，在非 Ready scope 内无事件地移除水，重入后观察重扫候选并在每 tick 65,536 读取上限内把陈旧湿耕地改干；原 dry-to-wet 测试保持不变 |

### Test-First / Mutation Evidence

- 初次新增场景运行先暴露夹具问题：注销 dirty 生成区块会进入 `ChunkUnloading`，重入取消卸载并保留原水源，故测试以 `邻块重入后水格 ready=true block=水源` 失败；修正为在邻块非 Ready 时直接改底层测试区块，刻意不经过 `fluidWorld.SetBlock`、不生产事件，准确模拟 scope 缺席期间失水。
- 修正夹具后，新测试通过：`go test ./internal/sim -run '^TestFarmlandMoistureReentryRecoversStaleWetFarmland$' -count=1` → `ok github.com/channing771/mornlea/internal/sim 0.919s`。
- 瞬态 mutation 把 `runFarmlandMoistureRescans` 的 active farmland 入队条件改为 `active && false`；同一新测试按预期失败：`邻块无水重入 16 tick 后边界耕地=湿耕地，想要恢复为干耕地`，package `FAIL` (`0.624s`)。
- 恢复 mutation 后生产文件与 `HEAD` 无 diff，新测试再次通过：`ok github.com/channing771/mornlea/internal/sim 0.778s`。最终 repair diff 只含测试与本 ledger。

### Repair Verification

| Command | Result | Evidence |
|---|---|---|
| `gofmt -w internal/sim/farmland_moisture_integration_test.go` + `gofmt -l` | pass | format check no output |
| `go test ./internal/sim -run 'TestCrop\|TestFarmland' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 3.894s` |
| `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 35.518s` |
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.43s` |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.242s` |
| `go vet ./internal/sim` | pass | no output |
| `git diff --check` | pass | no output |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |

### Repair Benchmark Evidence

命令：`go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5`；结果 `PASS`，package `ok` 为 `24.824s`。20/20 条样本继续同时打印 `cells/op` 与 `block_reads/op`，且全部为 `0 B/op`、`0 allocs/op`。

| Benchmark | ns/op samples | median ns/op | block_reads/op samples | median block_reads/op | cells/op |
|---|---|---:|---|---:|---:|
| FullInterestBarren | 172558, 172715, 172531, 172650, 172942 | 172650 | 14400, 14400, 14400, 14400, 14400 | 14400 | 14400 |
| FullInterestPlanted | 173295, 172640, 172427, 171939, 172850 | 172640 | 14400, 14400, 14400, 14401, 14400 | 14400 | 14400 |
| FullInterestDense | 176728, 175182, 179040, 177588, 176539 | 176728 | 14438, 14437, 14438, 14434, 14431 | 14437 | 14400 |
| AllFarmland | 2184, 2170, 2174, 2171, 2169 | 2171 | 72, 72, 72, 72, 72 | 72 | 72 |

Repair round 1 已修复 finding 并通过复审；`tasks.md` 留待 change 收尾统一核对。

Task 4 independent spec/quality re-review：approved；1 Important 已关闭，无新增 finding。

## Task 5 Steps 1-4 Verification Evidence

Task 5 本轮只执行 brief 的 Steps 1-4；未执行 Step 5 独立整分支终审，未勾选
`tasks.md`，Task 5 状态继续为 pending。固定 checkout 为
`feat/B-09-instant-farmland-moisture`，起始 HEAD
`9c9bb2e06feb9c64769c036a8d9557c59a745f1f`，起始 `git status --short` 无输出。

### Task 5 Failure Triage And Ruling

| ID | Finding | Decision | Evidence |
|---|---|---|---|
| B09-T5-R1 | 首次全量 race 中 `TestFarmingLoopEndToEndMemory` 在已有距离 2 水源的夹具下仍要求翻地后为干耕地；这与本 change 的“范围内有水时成功翻地同 tick 发布湿耕地”契约冲突。该失败不是等待预算/超时类 load flake | 接受只修改 `internal/server/farming_loop_e2e_test.go` 的一条期望和一条已失真的 growth budget 注释；不改生产代码、不走负载重跑裁决。修复后从 Step 1 重新执行 Steps 1-4 | focused RED：`go test ./internal/server -run '^TestFarmingLoopEndToEndMemory$' -race -count=1 -v` 以 `落脚格 = 36，想要干耕地 35` 失败；focused GREEN：同命令通过并记录成熟用 519 tick；重启后的全量 race 通过，`internal/server` 为 `197.940s` |

### Task 5 Command Evidence

所有需要 runtime 的命令用 `/usr/bin/time -p` 只读包裹；表中命令列保留 brief
要求的实际子命令。首次全量 race 失败后按顺序停止，未在失败树上运行 `go vet`；
完成 `B09-T5-R1` 后从 Step 1 完整重启。

| Stage | Command | Result | Complete evidence / runtime |
|---|---|---|---|
| initial Step 1 | `gofmt -l .` | pass | no formatter output；real `0.17s` |
| initial Step 1 | `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 37.324s`；real `115.98s` |
| initial Step 1 | `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 3.922s`；real `4.47s` |
| initial Step 2 | `make rust` | pass | `Finished release profile [optimized] target(s) in 0.28s`；real `0.34s` |
| initial Step 2 | `go test ./... -race` | fail | only failure `TestFarmingLoopEndToEndMemory`：`翻地后落脚格 = 36，想要干耕地 35`；real `288.40s` |
| repair RED | `go test ./internal/server -run '^TestFarmingLoopEndToEndMemory$' -race -count=1 -v` | expected fail | same stale expectation reproduced；package `1.648s`；real `3.29s` |
| repair GREEN | same command after minimal test repair | pass | `作物由随机 tick 从 stage0 长到成熟用了 519 个权威 tick`；package `3.361s`；real `7.36s` |
| final Step 1 | `gofmt -l .` | pass | no formatter output；real `0.18s` |
| final Step 1 | `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 35.572s`；real `36.04s` |
| final Step 1 | `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.171s`；real `4.62s` |
| final Step 2 | `make rust` | pass | `Finished release profile [optimized] target(s) in 0.29s`；real `0.36s` |
| final Step 2 | `go test ./... -race` | pass | all packages passed；`internal/server 197.940s`，`internal/archcheck 27.136s`，其余命中本次相同 race 构建缓存或通过；real `199.48s` |
| final Step 2 | `go vet ./...` | pass | no output；real `3.48s` |
| Step 3 | `go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5` | pass | 20/20 samples；package `25.300s`；real `26.05s` |
| Step 3 guarded | `MORNLEA_FLUID_PERF=1 go test ./internal/sim -run '^TestFluidPerf' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 18.955s`；real `19.37s` |
| Step 3 report | guarded command with supplemental `-v` | pass | all three tests passed；package `18.263s`；real `18.71s`；完整日志保存在 Task 5 report |
| Step 4 | `openspec status --change instant-farmland-moisture` | pass | schema `spec-driven`，4/4 artifacts complete；real `1.48s` |
| Step 4 | `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)`；real `1.30s` |
| Step 4 | `git diff --check` | pass | no output；real `0.02s` |
| Step 4 snapshot | `git status --short` | recorded | before ledger/report write: ` M internal/server/farming_loop_e2e_test.go`；real `0.03s` |

### Task 5 Crop Benchmark Record

全部样本均为 `0 B/op`、`0 allocs/op`；20/20 条均有 `cells/op` 与
`block_reads/op`。前三组另有 `chunks=200` 和 `crops` 规模坐标。墙钟数值只记录；
解析式门禁要求 `cells/op=14,400`、`block_reads/op<=28,800`，全耕地额外要求
`block_reads/op=cells/op=72`，本轮全部满足。

| Benchmark | ns/op samples | median ns/op | block_reads/op samples | median block_reads/op | cells/op |
|---|---|---:|---|---:|---:|
| FullInterestBarren | 177480, 178194, 177606, 177343, 177452 | 177480 | 14400, 14400, 14400, 14400, 14400 | 14400 | 14400 |
| FullInterestPlanted | 178939, 177559, 178439, 178541, 178694 | 178541 | 14400, 14400, 14400, 14400, 14400 | 14400 | 14400 |
| FullInterestDense | 182252, 184581, 182887, 184142, 182217 | 182887 | 14440, 14430, 14427, 14433, 14433 | 14433 | 14400 |
| AllFarmland | 2219, 2221, 2219, 2216, 2165 | 2219 | 72, 72, 72, 72, 72 | 72 | 72 |

### Task 5 Fluid Performance Record And Gates

| Scenario | Samples | Peak queue | Scale gate | Worst Step | Worst fluid | Worst moisture |
|---|---:|---:|---|---:|---:|---:|
| 大坝溃决 | 12,000 | 140,122 | breach enqueue 10,608 >= 10,000；peak >= 100,000 | 11.458458ms | 3.927875ms | 2.304292ms |
| 瀑布 | 12,000 | 501,587 | peak >= 200,000 | 15.938667ms | 10.08975ms | 1.198333ms |
| 合成 20 万项 | 10 | 200,000 | peak >= 200,000 | 1.461792ms | 651.208µs | 839.791µs |

三组报告均包含整 tick、fluid、moisture 与 queue 四种最坏口径及每条样本的
queue before/after 坐标。只读探针的 `changed`/queue length 不变守卫未触发，全部
fixture 完整性与风险规模守卫通过；性能数值未参与退出判定，也未放宽 overflow、
状态突变或数据丢失类失败。

### Task 5 Scope Audit

- merge base：`153b16f4ae82a12f3614a7403b876426af8abb2e`；相对该基线含本轮未提交修复共 32 个文件，`3643 insertions(+), 1030 deletions(-)`。
- 变更范围仅为 B-09 规划/历史文档、`internal/sim` 实现与测试，以及 `B09-T5-R1` 的 `internal/server/farming_loop_e2e_test.go`。
- `git diff --name-only <base> -- internal/fluid` 无输出；`internal/sim/fluid.go` 是 sim 适配器生产 hook，不是 `internal/fluid` 包。
- 对 `internal/network`、`internal/storage`、`internal/nativeabi`、Rust `engine`/`client` 与 `**/testdata` 的同一 name-only 审计无输出；文件清单无协议、schema、ABI、benchmark scenario、capture 或 golden 文件。
- `internal/archcheck` 与 OpenSpec strict 同时通过，未发现基线版本漂移。Step 5 独立整分支终审明确留给 controller。

### Task 5 Delivered Worktree Snapshot

| Command | Result | Complete evidence |
|---|---|---|
| `git diff --check` | pass | no output；real `0.02s` |
| `git status --short` | recorded | ` M internal/server/farming_loop_e2e_test.go`；` M openspec/changes/instant-farmland-moisture/ledger.md`；real `0.05s` |

SDD report 已写入 ignored 路径 `.superpowers/sdd/2026-08-26-instant-farmland-moisture/task-5-report.md`，不出现在 tracked status 中。未 commit、amend、push、merge、PR 或 archive。

## Task 5 Final Review Repair Round 2

本轮固定 checkout 为 `feat/B-09-instant-farmland-moisture`，起始 HEAD
`873ae45a2933f6c06cda2d7b4463562d68b1ade7`，起始 `git status --short` 无输出。
先更新 proposal、delta spec、design 与 tasks，再写测试和生产代码；`tasks.md` 全部复选框
继续保持未勾选，Task 5 独立整分支终审仍为 pending。

### Findings And Rulings

| ID | Finding | Decision | Evidence |
|---|---|---|---|
| B09-T5-FR-I1 | 湿度事件循环只受方块读取预算约束，范围外/维度缺失候选可以零读取无界出队；`farmlandMoistureState.pop` 每消费 4096 项且过半时复制剩余后缀 | 接受。增加独立固定 `65,536` 候选检查预算与每 tick 计数；每次查看队首先计数，再做 scope/dimension 查询；稳定 copy 改为 O(1) slice rebase，排空、FIFO、去重和既有 compaction 行为保持 | 新回归以 65,537 个范围外候选证明首 tick 检查 65,536、读取 0、留 1，次 tick 检查 1 并排空；focused race、整包 race 与 mutation 均通过 |
| B09-T5-FR-I2 | 建议为重扫增加固定公平份额，防止事件持续优先导致 starvation | 否决当前改动。保留 event-first：`fluidWorld.SetBlock` 只在流体 membership 实际变化时产生 162 格 fanout；`authoritative-fluid` 主规格限定只在有限 active Ready 范围推进并要求重扫最终收敛到平衡不动点；成功翻地只产生目标自身一项；主规格同时拒绝玩家或伙伴把任何物品放为流体。当前合法生产源有限，事件最终排空；未来新增永久 membership 生产者时再引入公平配额 | `internal/sim/fluid.go` 的唯一 membership hooks 位于最终写入汇聚点；`internal/sim/farming.go` 只有成功翻地单项 hook；`openspec/specs/authoritative-fluid/spec.md` 的“流体不可放置”“活动兴趣范围”“重启后收敛/不动点”契约；active/historical design 与 Risks 已写入该 ruling |
| B09-T5-FR-I3 | 全耕地 benchmark 没有报告或守卫工作负载规模 | 接受。计数并守卫 active Ready 区块恰为 1、真实耕地恰为 `98,304`、作物恰为 0，并在 `ResetTimer` 后报告 `chunks`/`farmland`/`crops`，保留读取等式与只记录墙钟语义 | 五次样本都打印 `1.000 chunks`、`98304 farmland`、`0 crops`、`72 block_reads/op`、`72 cells/op`、`0 B/op`、`0 allocs/op` |
| B09-T5-FR-M1 | farming E2E growth 注释仍写八次命中/约 512 tick | 接受 comment-only 修正为七次命中/约 448 tick | focused server race 与全量 race 通过 |
| B09-T5-FR-M2 | 历史 design 要求防御性不可达 `SetBlock` 错误记录日志，与实现静默路径不符 | 接受文档修正，不给 tick 热路径增加理论不可达日志 | 历史 design 明确该分支静默删除候选且不广播未落地变化；生产语义不变 |
| B09-T5-FR-M3 | active design Affected Files 未列 B-07 helper/test 重叠及 Task 5 server E2E 修复 | 接受文档修正 | Affected Files 现明确 `fluid_crop_test.go`、共享 `helpers_test.go` 与 `internal/server/farming_loop_e2e_test.go` |

### TDD And Mutation Evidence

1. 添加范围外积压测试后，focused RED 因 `farmlandMoistureCandidatesPerTick` 与
   `candidateInspections` 不存在而编译失败。
2. 最小实现候选检查计数/上限与 O(1) rebase 后，
   `go test ./internal/sim -run '^TestFarmlandMoistureInspectionBudgetDefersOutOfScopeBacklog$' -count=1`
   通过，package `0.588s`。
3. 瞬态 mutation 把候选检查上限从 `65,536` 改为 `65,537`；同一测试按预期失败：
   `首 tick 候选检查=65537，想要 65536`，package `FAIL` (`0.674s`)。
4. 恢复 mutation 后同一测试再次通过；focused queue/budget/determinism race 通过，
   `ok github.com/channing771/mornlea/internal/sim 2.508s`。

### Refreshed Verification

| Stage | Command | Result | Evidence |
|---|---|---|---|
| focused | `go test ./internal/server -run '^TestFarmingLoopEndToEndMemory$' -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/server 3.169s` |
| Step 1 | `gofmt -l .` | pass | no formatter output |
| Step 1 | `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 35.747s` |
| Step 1 | `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.349s` |
| Step 2 | `make rust` | pass | `Finished release profile [optimized] target(s) in 0.28s` |
| Step 2 | `go test ./... -race` | pass | all packages passed；`cmd/mornlea 276.864s`、`internal/server 214.521s`、`internal/sim 48.297s`、`internal/archcheck 36.353s` |
| Step 2 | `go vet ./...` | pass | no diagnostic output |
| Step 3 | `go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5` | pass | 20/20 samples，package `25.309s`；全部 `0 B/op`、`0 allocs/op` |
| Step 3 guarded | `MORNLEA_FLUID_PERF=1 go test ./internal/sim -run '^TestFluidPerf' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 20.816s` |
| Step 4 | `openspec status --change instant-farmland-moisture` | pass | schema `spec-driven`，4/4 artifacts complete |
| Step 4 | `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |

### Refreshed Crop Benchmark Record

| Benchmark | ns/op samples | median ns/op | block_reads/op samples | median block_reads/op | cells/op | Workload coordinates |
|---|---|---:|---|---:|---:|---|
| FullInterestBarren | 184714, 184253, 187004, 184236, 183583 | 184253 | 14400, 14400, 14400, 14400, 14400 | 14400 | 14400 | 200 chunks, 0 crops |
| FullInterestPlanted | 189095, 190552, 188846, 184825, 184796 | 188846 | 14400, 14400, 14400, 14400, 14401 | 14400 | 14400 | 200 chunks, 256 crops |
| FullInterestDense | 188744, 188226, 191264, 191687, 189009 | 189009 | 14440, 14436, 14436, 14427, 14440 | 14436 | 14400 | 200 chunks, 51200 crops |
| AllFarmland | 2400, 2254, 2182, 2187, 2188 | 2188 | 72, 72, 72, 72, 72 | 72 | 72 | 1 Ready chunk, 98304 farmland, 0 crops |

墙钟数值只记录；解析式读取门禁、真实 workload 坐标、报告完整性、overflow 与数据丢失门禁
均未放宽。协议 v26、区块 schema v9、世界 metadata v2、玩家 schema v7、
`companions.ai` schema v4、engine/client ABI 与 benchmark scenario 均未改变。

### Repair Scope Audit

- `gofmt -l .` 与 `git diff --check` 均无输出；最终 OpenSpec strict 复跑仍为
  `66 passed, 0 failed`。
- `tasks.md` 不含 `[x]`/`[X]`，所有任务保持未勾选。
- repair diff 只涉及 active/historical B-09 文档、ledger、湿度队列实现/预算测试、作物
  benchmark 与 Task 5 farming E2E 注释。
- `git diff --name-only HEAD -- internal/fluid internal/network internal/storage internal/nativeabi engine client cmd/mornlea/testdata **/testdata`
  无输出；无协议、存档、ABI、Rust、scenario、capture 或 golden 改动。
- ignored Task 5 report 已追加本 repair round，不进入 tracked status。

## Task 5 Final Re-review Repair Round 2

本轮固定 checkout 为 `feat/B-09-instant-farmland-moisture`，起始 HEAD
`078a6a6db09b6d3e1e1d95eff100efdfb1787b84`，起始 `git status --short` 无输出。
本轮沿用 Task 5 implementer `ses_fc3255961ffevR5E7w69UMj8wQ` 执行修复；implementer 按 brief
不得再派生 subagent，独立终审仍由控制会话持有的 reviewer 席执行。proposal、delta spec、
design、tasks 与历史 writer audit 均先于生产代码更新；修复时所有 task checkbox 保持未勾选。

### Findings And Writer-audit Ruling

| ID | Finding | Decision | Evidence |
|---|---|---|---|
| B09-T5-R2-I1 | `executePlacement` 合法允许固体覆盖流体，但该路径绕过 `fluidWorld.SetBlock`，成功写入只经 `recordChange` 唤醒流体队列，未生产反向湿度候选 | 接受。在已有 `block` 旧值和 `placement` 新值上，仅于 `Dimension.SetBlock` 成功且 `changed=true` 的分支比较 `core.IsFluid` membership，并复用 `enqueueFarmlandMoistureAroundFluid`；拒绝、错误与 no-op 结构上不入队；协议与既有成功确认/扣料不变 | 真实 `CommandPlaceBlock` 回归覆盖 contained last irrigation water、无旧候选/重扫 backlog、active Ready scope、同 tick 变干、`PlacementSuccess(sequence=2)`、扣一件与 inventory publication |
| B09-T5-R2-I2 | `TestFarmlandMoistureQueueCompactsConsumedPrefix` 只断言值与长度，旧 suffix copy 同样通过，不能证明 O(1) rebase | 接受。跨过 4096/half 阈值前保存首个 surviving element 地址，rebase 后断言 `pending[0]` 地址完全相同 | 临时恢复旧 copy 后测试以地址不同失败；恢复 rebase 后通过 |

本轮 writer audit 纠正上一轮 `B09-T5-FR-I2` 中“`fluidWorld.SetBlock` 是唯一 membership
hook”的失真实证：生产运行期 membership 写者恰有两条，`fluidWorld.SetBlock` 可双向改变，
`executePlacement` 可把流体覆盖为非流体。伙伴放置只接受空气目标；玩家/伙伴采掘不命中
流体；翻地、湿度、作物和踩踏只转换非流体农业编号；加载/生成由 active Ready 重扫恢复；
`SetBlockForTest` 仅供测试。仍否决把挂点扩到全部 `recordChange` writers，也不为两条路径
新增抽象。

### RED, Mutation, And Focused GREEN

1. 首个 placement 夹具虽以“耕地仍湿”失败，但加入生产 hook 后仍失败；phase/target 守卫
   证明射线先命中玩家原落脚格，实际没有覆盖目标水，因此该次失败判为无效 RED，不作为
   行为证据。只修正夹具：打开射线途经的两格竖井，保留玩家同 tick 物理与真实 DDA。
2. corrected fixture 下临时移除唯一新增 hook；水格已成功变石头，但 focused test 按预期失败：
   `湿度阶段未观察到耕地候选：phaseSeen=true candidateSeen=false`，package `FAIL`
   (`0.812s`)。恢复最小 hook 后同一测试通过，package `0.804s`。
3. queue no-copy 测试在当前 rebase 上先通过，package `0.361s`；临时恢复旧
   `pending[:copy(...)]` 后按预期失败：rebase 后地址与原 surviving element 地址不同，
   package `FAIL` (`0.848s`)；恢复 O(1) rebase 后再次通过，package `0.335s`。
4. focused combined race：
   `go test ./internal/sim -run '^(TestPlayerPlacementRemovingLastIrrigationDriesFarmlandSameTick|TestFarmlandMoistureQueueCompactsConsumedPrefix)$' -race -count=1`
   → `ok github.com/channing771/mornlea/internal/sim 1.824s`。
5. broader focused placement/queue race：
   `go test ./internal/sim -run 'Test(PlayerPlacement|PlaceBlockThroughFluid|RejectedPlayerPlacement|FarmlandMoistureQueue)' -race -count=1`
   → `ok github.com/channing771/mornlea/internal/sim 2.009s`。

### Step 1-3 Verification And Triage

首次完整序列的 `internal/sim` race 通过（`36.541s`），随后 archcheck 确定性失败：新生产
注释把局部变量 `placement` 写进反引号，而 `TestCommentBacktickIdentifiersExist` 只接受全仓
Go 声明。该失败不是 wait-budget/load flake；改为不引用局部名字的“写前与写后方块编号”后，
从 Step 1 完整重启，未继续在失败树上运行后续门禁。

| Stage | Command | Result | Evidence |
|---|---|---|---|
| clean preflight | `make rust` | pass | `Finished release profile [optimized] target(s) in 0.29s` |
| initial Step 1 | `gofmt -l .` | pass | no formatter output |
| initial Step 1 | `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 36.541s` |
| initial Step 1 | `go test ./internal/archcheck -count=1` | fail | only failure `TestCommentBacktickIdentifiersExist` at `engine_placement.go:92:57`；package `4.767s` |
| final Step 1 | `gofmt -l .` | pass | no formatter output |
| final Step 1 | `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 36.298s` |
| final Step 1 | `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.514s` |
| final Step 2 | `make rust` | pass | `Finished release profile [optimized] target(s) in 0.28s` |
| final Step 2 | `go test ./... -race` | pass | all packages passed；`cmd/mornlea 271.161s`、`internal/server 209.635s`、`internal/sim 43.782s`、`internal/archcheck 33.048s` |
| final Step 2 | `go vet ./...` | pass | no diagnostic output |
| Step 3 | `go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5` | pass | 20/20 samples；package `25.073s`；全部 `0 B/op`、`0 allocs/op` |
| Step 3 guarded verbose | `MORNLEA_FLUID_PERF=1 go test ./internal/sim -run '^TestFluidPerf' -count=1 -v` | pass | three reports complete；package `19.448s` |

### Crop Benchmark Record

| Benchmark | ns/op samples | median ns/op | block_reads/op samples | median block_reads/op | cells/op | Workload coordinates |
|---|---|---:|---|---:|---:|---|
| FullInterestBarren | 177896, 178619, 178136, 178543, 178755 | 178543 | 14400, 14400, 14400, 14400, 14400 | 14400 | 14400 | 200 chunks, 0 crops |
| FullInterestPlanted | 179702, 185495, 180028, 180066, 179841 | 180028 | 14400, 14400, 14400, 14400, 14402 | 14400 | 14400 | 200 chunks, 256 crops |
| FullInterestDense | 183470, 182344, 183177, 183203, 183400 | 183203 | 14441, 14437, 14439, 14436, 14442 | 14439 | 14400 | 200 chunks, 51200 crops |
| AllFarmland | 2242, 2220, 2217, 2251, 2226 | 2226 | 72, 72, 72, 72, 72 | 72 | 72 | 1 Ready chunk, 98304 farmland, 0 crops |

### Guarded Fluid Performance Record

| Scenario | Samples | Peak queue | Scale gate | Worst Step | Worst fluid | Worst moisture |
|---|---:|---:|---|---:|---:|---:|
| 大坝溃决 | 12,000 | 140,122 | breach enqueue 10,608；peak risk ratio 70.1% | 12.817041ms | 5.807292ms | 3.002292ms |
| 瀑布 | 12,000 | 501,587 | initial frontier 402；peak risk ratio 250.8% | 14.754375ms | 9.340792ms | 3.679583ms |
| 合成 20 万项 | 10 | 200,000 | exact risk scale 200,000 | 1.831959ms | 1.009417ms | 865.917µs |

三组 verbose 报告均含整 tick、fluid、moisture、queue before/after、map 与排序口径；fixture
规模、报告完整性、只读探针、真实 overflow 和数据丢失门禁均未放宽。墙钟数值只记录。

### Step 4 And Scope Audit

| Command | Result | Evidence |
|---|---|---|
| `openspec status --change instant-farmland-moisture` | pass | schema `spec-driven`，4/4 artifacts complete |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |
| `git diff --check` | pass | no output |
| `git status --short` | recorded | 9 intended tracked files modified；ignored Task 5 report absent |
| restricted scope audit | pass | `git diff --name-only HEAD -- internal/fluid internal/network internal/storage internal/nativeabi engine client cmd/mornlea/testdata **/testdata` 无输出 |

- `tasks.md` 所有复选框仍为 `[ ]`；Task 5 final review 保持 pending。
- repair diff 仅含 active/historical B-09 artifacts、ledger、`engine_placement.go`、真实 placement
  integration 回归与 queue no-copy proof。
- 协议保持 v26；区块 schema v9、玩家 schema v7、world metadata v2、`companions.ai`
  schema v4、engine/client ABI、benchmark scenario、capture 与 golden 均未变化。

## Task 5 Final Review And Closure

| Item | Result | Evidence |
|---|---|---|
| Independent reviewer | approved | `ses_fc3076d9dffeR6y7fE3YoM6G5l` 对 `153b16f4..7fce9926` 复审 |
| Seven final gates | 7/7 pass | production/test 覆盖、双预算、确定性顺序、无持久队列恢复、crop 解耦、A-04/B-07 重叠与文档/回退声明全部通过 |
| Findings after repair round 2 | none | 0 Critical、0 Important、0 Minor |
| Merge readiness | yes | reviewer 最终结论 `Ready to merge? Yes` |

### Closure Rulings

| ID | Finding | Decision | Evidence |
|---|---|---|---|
| B09-T5-C1 | `tasks.md` 4.4 的历史组合 benchmark selector 写有 `Fluid`，仓库实际流体性能门禁是环境变量守卫的 `TestFluidPerf*`，不是 Go benchmark | 以五轮 `BenchmarkCropAdvance*` 加显式 `MORNLEA_FLUID_PERF=1 TestFluidPerf*` 作为等价且更强的完成证据；前者提供 crop metrics，后者提供真实 queue scale、overflow、只读探针与报告完整性门禁 | repair round 2 最新 crop 20/20 samples 与三份完整 guarded fluid reports 全部通过；终审认可性能声明准确 |
| B09-T5-C2 | 最终终审前 checklist 按流程保持未勾选 | 仅在 `7fce9926` 终审 7/7 通过且最新 Steps 1–4 证据齐全后统一勾选 20 项 | 本节与 `tasks.md` 同一次 closure 更新；change 不归档 |

Task 5 及 change 执行任务现已全部完成。保持 OpenSpec change active；未 archive、push、merge 或创建 PR。

### Post-Closure Verification

控制会话在勾选 checklist 后独立重跑最终门禁，不依赖 implementer 报告：

| Command | Result | Evidence |
|---|---|---|
| `gofmt -l .` | pass | no output |
| `make rust` | pass | `Finished release profile [optimized] target(s) in 0.30s` |
| `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 36.681s` |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 4.414s` |
| `go test ./... -race` | pass | all packages passed；`internal/archcheck 26.898s`，其余通过或命中同代码 revision 的 race cache |
| `go vet ./...` | pass | no diagnostic output |
| `go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5` | pass | 20/20 samples；package `25.545s`；全部 `0 B/op`、`0 allocs/op` |
| `MORNLEA_FLUID_PERF=1 go test ./internal/sim -run '^TestFluidPerf' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 20.592s` |
| `openspec status --change instant-farmland-moisture` | pass | 4/4 artifacts complete |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 66 passed, 0 failed (66 items)` |
| `git diff --check` | pass | no output |

最新 crop benchmark 的 `ns/op` 中位数依次为 barren `190792`、planted `190816`、dense
`180820`、all-farmland `2198`；读取中位数依次为 `14400`、`14400`、`14437`、`72`。
全耕地五次样本均报告 `1.000 chunks`、`98304 farmland`、`0 crops`、
`72 block_reads/op == 72 cells/op`。数值只记录，所有解析式与报告完整性门禁继续生效。

## Pull Request Integration Verification

用户选择创建 PR 后，控制会话先刷新真实远端基线，再把 `origin/main` 的
`eed30581ca3557e89ca78429b33f81da8cfc96a5` 无冲突合入 feature branch；merge commit 为
`53a51262730f4305e0afc13f9119b72732a3a0f1`。本地 `main` 自身已与远端分叉，故未作为集成
来源，也未把其未共享提交带入 PR。

### Integration Ruling

| ID | Finding | Decision | Evidence |
|---|---|---|---|
| B09-PR-R1 | 合并 `origin/main` 的 engine ABI v7 后，首次 sim race 在 `PhysicsStep` 返回 ABI mismatch；B-09 与 `origin/main` 的 native/physics/Rust ABI 文件无 diff，v7 dylib 自报版本正确，nativeabi test variant 通过 | 判定为 Go/cgo 普通依赖 archive 仍复用合并前 ABI v6 header 的本机构建缓存，不改代码；执行 `go clean -cache` 后从干净 Go build cache 重跑 | 原始 `TestMeleeHitChargesAttackerExhaustionExactlyOnce` 清缓存前稳定 panic，清缓存后定点通过；随后 sim race、archcheck 与全仓 race 从零通过 |

### Integrated-Tree Gates

| Command | Result | Evidence |
|---|---|---|
| `gofmt -l .` | pass | no output |
| `make rust` | pass | ABI v7 integrated tree release build `0.28s` |
| `go test ./internal/sim -race -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 37.247s` |
| `go test ./internal/archcheck -count=1` | pass | `ok github.com/channing771/mornlea/internal/archcheck 9.898s` |
| `go test ./... -race` | pass | clean-cache run；`cmd/mornlea 277.871s`、`internal/server 210.779s`、`internal/sim 47.541s`、`internal/archcheck 38.307s`，其余全部通过 |
| `go vet ./...` | pass | no diagnostic output |
| `go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5` | pass | 20/20 samples；package `25.525s`；全部 `0 B/op`、`0 allocs/op` |
| `MORNLEA_FLUID_PERF=1 go test ./internal/sim -run '^TestFluidPerf' -count=1` | pass | `ok github.com/channing771/mornlea/internal/sim 21.432s` |
| `openspec status --change instant-farmland-moisture` | pass | 4/4 artifacts complete |
| `openspec validate --all --strict --no-interactive` | pass | `Totals: 67 passed, 0 failed (67 items)` |

集成树 crop benchmark 的 `ns/op` 中位数依次为 barren `187255`、planted `192164`、dense
`194557`、all-farmland `2305`；读取中位数依次为 `14400`、`14401`、`14443`、`72`。
全耕地五次样本继续精确报告一个 Ready 区块、`98,304` 格耕地、零作物及
`72 block_reads/op == 72 cells/op`。墙钟只记录，所有 correctness 门禁保持有效。
