# instant-farmland-moisture 执行账本

## Task Status

| Task | Implementer | Spec Review | Quality Review | Repair Rounds | Status |
|---|---|---|---|---:|---|
| 1 | `ses_fc3afd2a2ffeq80iW8UIKYPasR` | approved | approved | 0 | complete |
| 2 | OpenCode implementer（按用户要求不派生子代理） | pending | pending | 0 | implemented |
| 3 | pending | pending | pending | 0 | pending |
| 4 | pending | pending | pending | 0 | pending |
| 5 | control | pending | pending | 0 | pending |

## Rulings

| ID | Task | Finding | Decision | Evidence |
|---|---|---|---|---|
| B09-T1-R1 | 1 | Brief 中的原始 hash 命令同时散列 `go test` 的动态耗时汇总行，未改代码连续运行也会得到不同 hash | 保留原始命令结果，并另用排除 `ok` 汇总行的排序入口 hash 裁决测试集合；不改测试或生产代码迎合不稳定 hash | split 后原始 hash 连续为 `b11a78ad...`、`9b9138ce...`；稳定入口 hash 为 `dc4e65d...`，逐声明重构与 `HEAD:internal/sim/crop_test.go` 仅差文件头 import/空行 |
| B09-T1-R2 | 1 | Reviewer 无法从 diff 验证 feature branch commit 是否获得授权 | 控制会话确认用户在 Task 1 派发前已显式授权 B-09 每任务 commit，授权不含 push/merge/PR；无需代码修复 | 对话授权选择“授权任务 commits”；commit `db2a6a7d` 只含 Task 1 文件 |

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

Task 2 的独立规格评审与质量评审尚待控制会话执行；本实现没有提前进入 Task 3。
