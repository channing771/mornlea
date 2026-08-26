# instant-farmland-moisture 执行账本

## Task Status

| Task | Implementer | Spec Review | Quality Review | Repair Rounds | Status |
|---|---|---|---|---:|---|
| 1 | `ses_fc3afd2a2ffeq80iW8UIKYPasR` | approved | approved | 0 | complete |
| 2 | `ses_fc39ce278ffew8TsrLujyjLcba` | approved | approved | 0 | complete |
| 3 | pending | pending | pending | 0 | pending |
| 4 | pending | pending | pending | 0 | pending |
| 5 | control | pending | pending | 0 | pending |

## Rulings

| ID | Task | Finding | Decision | Evidence |
|---|---|---|---|---|
| B09-T1-R1 | 1 | Brief 中的原始 hash 命令同时散列 `go test` 的动态耗时汇总行，未改代码连续运行也会得到不同 hash | 保留原始命令结果，并另用排除 `ok` 汇总行的排序入口 hash 裁决测试集合；不改测试或生产代码迎合不稳定 hash | split 后原始 hash 连续为 `b11a78ad...`、`9b9138ce...`；稳定入口 hash 为 `dc4e65d...`，逐声明重构与 `HEAD:internal/sim/crop_test.go` 仅差文件头 import/空行 |
| B09-T1-R2 | 1 | Reviewer 无法从 diff 验证 feature branch commit 是否获得授权 | 控制会话确认用户在 Task 1 派发前已显式授权 B-09 每任务 commit，授权不含 push/merge/PR；无需代码修复 | 对话授权选择“授权任务 commits”；commit `db2a6a7d` 只含 Task 1 文件 |
| B09-T2-R1 | 2 | Reviewer 无法只从 commit diff 验证 RED 时序、命令输出与提交后 worktree 状态 | 接受 implementer 在 ignored SDD report 中保留的逐命令证据，并以控制会话观察到的 clean status 补足；无需重跑或修改代码 | `task-2-report.md` 第 76–223 行记录 RED/GREEN，控制会话在 `5fb868f8` 后运行 `git status --short --branch` 仅输出 branch header |

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
- 协议、存档 schema、engine/client ABI、benchmark scenario 与 capture golden 均未修改；Task 3 规格评审和质量评审保持 pending。

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

Task 3 规格评审与质量评审仍保持 pending；本轮只修复测试证明与注释，不修改生产行为，不勾选 `tasks.md`。

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
