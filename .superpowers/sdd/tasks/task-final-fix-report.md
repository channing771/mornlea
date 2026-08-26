# B-07 flood-destroys-crops 整分支终审修复报告（单波次）

状态：DONE。工作目录 `.worktrees/feat/B-07-flood-destroys-crops`，变基基线为当时 main 尖端 `e57dc60c`（propose 提交 `02067d29` 的父；本分支现行任务 3 提交 `9770b1fc` 不在 main 上）。

## Findings 处置

| Finding | 处置 | 落点 |
|---|---|---|
| I-1 编译+语义适配 | `settleFloodedCrop` 成熟分支改调包内共享 `cropYieldRolls`（tick 取值点 `Engine.tick.Load()` 与 `completeMining` 同一读取路径）；未成熟分支保持 `core.BlockDrop` 单种子 | `internal/sim/fluid_crop.go` |
| I-1 测试确值 | 期望产量按夹具已知 `(seed, 结算 tick, 维度, position)` 调 `cropYieldRolls` 现算；`stepUntilFluidCropFlooded` 改为返回结算 tick（Step 返回后 `Load()−1`）。覆盖成熟、原子性用例；容量重试/水平冲毁用例为未成熟分支（单种子），断言无需变动 | `internal/sim/fluid_crop_test.go` |
| I-1 server parity | 见下方「parity 连带适配」 | `internal/server/fluid_crop_parity_test.go` |
| I-1 规格/设计 | spec 场景改「与玩家采掘该作物的掉落表完全相同（含其数量语义）」级表述，保留成熟多产物 vs 未成熟单产物区分；design.md D4 同步删除固定数量与已删常量表述 | `specs/authoritative-fluid/spec.md`、`design.md` |
| M-1 补测试落点 | 新增双源夹具用例（见下） | `internal/sim/fluid_crop_test.go` |
| T1 注释收窄 | 「全部经由谓词」枚举收窄为承重路径：垂直优先与水平递减两处写入判定 + sim 重扫侧两个不动点捷径（`fluidSourceIsFixedPoint` / `fluidSectionIsFixedPoint`）；存活判定（`flowingSurvives`）与冲突合并（`strongerWrite`）不经 `Replaceable`，移出枚举。design.md D1 同句同改 | `internal/fluid/rules.go:29` 一处注释、`design.md` |
| M-3 ledger | 「执行」「终审与收尾」两节回填：任务完成哈希、评审结论、满负载 flake 裁决、既有 flaky 上报裁决、五条 deferred minors triage、本次适配记录；占位符清零 | `openspec/changes/flood-destroys-crops/ledger.md` |
| M-4 tasks | 全部复选框核对后勾选；4.2 注记 cmd/mornlea 满负载 flake 已按分诊协议单独复跑通过；2.1 固定数量表述同步为现算表述 | `openspec/changes/flood-destroys-crops/tasks.md` |

## M-1 用例设计与红证据

`TestFluidCropSameTickDualSourceMergesToStrongestAndSettlesOnce`：等级 1 垂直候选（作物正上方水源，垂直优先写等级 1）× 等级 2 水平候选（东侧等级 1 流动水 F：上方藏一格不入队水源作 `flowingSurvives` 支撑、下方泥土挡垂直分支，水平递减写等级 2）在同一准备步入队 ⇒ 两候选落进同一次合并批次。断言：

1. 作物格整个收敛窗口内恰好一笔广播变更（合并先于提交）；
2. 最终生效值为最强者等级 1；
3. 掉落恰好一批且与 `cropYieldRolls(seed, 结算tick, 维度, position)` 现算值逐件相等。

红证据：临时把 `internal/fluid/queue.go` 合并分支变异为「先到先得」（保留首个候选）——用例 3/3 确定性红，失败形态正是「弱候选先落笔冲毁、强候选次批改写 ⇒ 作物格两笔广播」；随后 `git checkout --` 还原，用例复绿。变异仅本地演示红绿，未进入任何提交。

## server parity 连带适配（I-1 的真实语义后果）

B-10 后成熟产量按 `(seed, 权威绝对 tick, 维度, 坐标)` 取哈希。该测试的两个宿主各自独立跑完整模拟，就绪耗时不同（实测 memory=69 / tcp=54 tick），结算绝对 tick 分岔 ⇒ 同一株小麦两侧收到不同的「合法」产量，跨传输逐件 DeepEqual 假红（实测 memory seeds×3+wheat×2 vs tcp seeds×2+wheat×1）。旧固定数量时代对此免疫情形消失。

处置：**对齐而不是放宽**——两侧就绪后、开录前各自空转到共同绝对 tick `floodCropParityAlignTicks = 256`（就绪耗时超限则显式 Fatal 提示上调预算，不静默重试；空转期世界静止无录像内容）。对齐后行走脚本每个事件落在相同绝对 tick 上，产量哈希输入逐项相同，`Drops` DeepEqual 严格性原样保留；另按 trample_test 先例补一组结构守卫（作物格上小麦、种子各一堆、数量 ∈ [1,3]）。

## 验证摘要

- `go build ./...` ✅
- `go test ./internal/fluid ./internal/sim -race -count=1` ✅（fluid 12.6s / sim 22.0s）
- `go test ./internal/server -race -run 'Parity|FluidCrop' -count=1` ✅（14.0s）
- `gofmt -l .` 无输出；`go vet ./...` ✅
- `openspec validate --all --strict --no-interactive` ✅（65 passed, 0 failed）
- rebase 面核查：`git diff --stat bcc900fb main -- cmd/mornlea` 显示 2 个 golden PNG 变化（hud-hotbar-health.png、hud-survival-feedback.png，来自主线侧提交，非本分支改动）⇒ 按要求补跑 `go test ./cmd/mornlea -race -run 'Capture|Golden' -count=1 -timeout 20m` ✅（ok 3.5s）

## 疑虑

- 五条 deferred minors 中 ②（`internal/fluid/helpers_test.go:218` Fatalf 消息未同步作物例外前提）与 ④（server parity 用例压 `-short` 档线）维持 deferred，需后续归属方接手；其余三条已闭环（①本波次修复、③关闭、⑤接受现状）。
- `TestMemoryTCPFluidDamBreakBroadcastParity` 既有 flaky 属 E-11 独占域，本轮 `Parity|FluidCrop` 两轮实跑均过，仍建议向需求方上报。

## 复审更正记录（2026-08-26）

终审修复提交（`e52927c5`/`8d0234f8`）经复审确认五条 findings 全部 ADDRESSED，但修复产物自身引入两条 Important（记录完整性），本节随更正提交一并落账：

1. 基线表述不实：ledger 终审节与本报告首行原写「变基到 main `9770b1fc`」——`git merge-base --is-ancestor` 实证 `9770b1fc` 不在 main 上，它是本分支变基重放后的任务 3 提交；真实基线是当时 main 尖端 `e57dc60c`（`git branch --contains` 与 `merge-base --is-ancestor e57dc60c main` 双证）。两处已更正。
2. ledger「执行」节死哈希：组1 行的 `6cd469c6` / 收尾范围端点 `b65fbe6f` 为变基前死哈希（`git merge-base --is-ancestor <hash> HEAD` 判不可达），同句却混用现行可达的 `dbd5be00`。按「变基重放保消息不变、标题匹配」核实对应：`6cd469c6`「feat(fluid): 作物格对流动水可替换（B-07 任务1）」→ `55a0a715`；`b65fbe6f`「docs(fluid): 补齐注释反引号纪律（B-07 任务1 R1）」→ `dbd5be00`；收尾范围起点 `029af13a`（propose）→ `02067d29`。已全部替换为变基后可达值，并在「执行」节首行加注记。

顺手更正不计级 nit：tasks.md 4.2 的指引由「见 ledger 终审节」改为「见 ledger 执行节」（满负载 flake 裁决实际记录在执行节）。
