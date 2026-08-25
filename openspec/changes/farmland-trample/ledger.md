# Ledger: farmland-trample

> 记录本 change 的进度、评审结论与全部裁决（Ruling）。格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。

## 裁决记录（认领与设计阶段，2026-08-25）

- **Ruling: 认领 B-05 并冻结独占文件集** — 原选 B-31（开箱中断进食）被 zcode2 链抢先（其提交 `5dd018d5` 早于本链），按「以 git 提交先后为准，后到者让位」让位；随后逐行核查剩余全部未认领行（依赖、独占文件集、版本互斥、先裁决类、大型行五类排除），B-05 是唯一无版本冲突且重叠面最小的可行行。认领提交 `9f48a2b4`（main，docs-only）。
- **Ruling: 与 A-01 在 `internal/sim/player.go` 的最小受控重叠** — 控制会话（用户）批准，同 B-31 先例：仅限摔落/落地结算区插入踩踏判定调用，该文件其余内容不触碰。落地边沿（`player.go` 的 `!wasOnGround` 判定）与摔落伤害共用同一次检测，farming 遗留 4 的「物理侧已有落地判定（摔落伤害在用），接一条钩子」即指此处。
- **Ruling: 结算挂点选 `advanceCrops` 首部而非 `engine_step.go`** — 阶段顺序契约要求一切区块写者位于 `reconcileSubscriptions` 之后，而落地边沿在其之前，故踩踏必须「边沿收集 + 写入区结算」两段；`engine_step.go` 是 A-01/A-04 双独占在途文件不得触碰；`advanceCrops`（`crop.go`）位于区块写入区、与耕地干湿转换同域共用同一份 pending。B-10 已归档，`crop.go` 现无任何在途认领；认领备注中防御性的「刻意不触碰 crop.go」据此修正并回写 backlog。
- **Ruling: `engine.go` 追加一行暂存字段属第三处最小受控重叠，同构处理** — 跨阶段载体必须是 `Engine` 字段：包级变量在多引擎并发测试（`-race`）下是数据竞争，Go 不允许跨文件拆结构体，`cropCellScratch` 挂 engine 正是同构先例。改动严格限制为结构体内 append-only 一行字段声明与注释、不改任何既有行，与 `player.go` 裁决同构；A-04 同期大改 `engine.go` 的合并序由集成裁决处理。错在认领时把重叠面低估为一处——三行挂钩（调用、调用、字段）才是该裁决的最小技术闭包。
- **Ruling: 范围冻结为玩家，伙伴踩踏延期** — 原文写「实体」，但 companion 物理路径无现成落地边沿（无摔落伤害语义），为其新建边沿检测超出认领独占集与最小闭环；先例 B-13 冲刺半边延期。誊入 proposal 非目标。
- **Ruling: 上方作物按采掘同形规则连带掉落** — 踩踏语义是「毁田」：跳进麦田毁庄稼是该机制的玩法代价；掉落完全复用 `cropYieldRolls` 与 `PrepareDropBatch` 预演，同一株同一 tick 踩掉与挖掉数量必然相同，确定性域统一。被否决：只转耕地不处理上方作物（麦田穿行会留成片悬空作物）；独立哈希流（破坏与采掘的重放一致域统一）。
- **Ruling: 掉落容量不足整格放弃（耕地保持原样）** — 采掘路径 `RejectDropCapacity`「绝不半掉落」先例的直接移植；踩踏无拒绝通道（非玩家命令），放弃即可观察且无信息丢失；重试机会=下一次落地边沿（跳一下），是「落地冲击」语义的自然读法。
- **Ruling: 设计批准** — bounded 短设计经用户显式批准（2026-08-25）后开工；三处最小受控重叠的完整清单在批准后的设计落定中澄清（`engine.go` 一行为技术必要组成，同构于已批裁决精神，未另行打断）。

## Task 1（sim 层踩踏收集与结算）

- 实现提交 `69fa0f5b`：`trample.go`（`tramplePendingCell`/`noteTrampleLanding`/`settleTramples`/`settleTrampleCell`/`commitTrample`，支撑层 `floor(Y − physics.GroundProbe)`、内域相交 2×2 列枚举）、`trample_test.go` 四主题、`property_trample_yield_parity_test.go` 双测试、三处最小重叠接线（`player.go` 3 行 / `crop.go` 5 行 / `engine.go` 5 行，全部纯追加）。red-first：四个 Trample 用例实现前失败（耕地未变泥土）；容量用例为负向断言实现前天然通过。全包 `-race` ok（30.9s）、`gofmt`/`go vet` 干净、`go test -list` 406→412 纯增。环境备注：worktree 首跑 `make rust`；go 经 GVM go1.26.0。
- 实现偏差（已核实可接受）：容量用例断言口径从 `DropsHash` 改为「物品计数 + 方块区 `Hash()` + revision」——预填掉落物在下落等待期合法老化会入 `DropsHash` 造成假红，方块区哈希不含掉落（`chunk.go:56`），口径等价且更锋利。
- 评审：**SPEC PASS**（七条 Scenario 逐条锚定、`assertTrampleBroadcasted` 钉死 pending 汇入、tick 对齐论证核实：`Step` 末尾才 `tick.Add(1)`、`advanceMiningOnce` 不动 tick）；**QUALITY PASS**（三处重叠逐行核对纯追加、红线零触碰、ε 论证经 Rust `clip_against` 实证、`settleTramples` 在 `samples <= 0` 早退之前保证暂存每 tick 清空、archcheck 实跑通过）。评审者复跑全绿。
- 评审 NB 与处置 Ruling：
  - **Ruling: NB1 双玩家同格幂等测试补强划入 Task 2（新增条目 2.4）** — spec 需求正文的 MUST 幂等条款目前只有 `settleTrampleCell` 读判的结构性覆盖，补直接双玩家用例锚定。
  - **Ruling: NB3 property 文件命名张力按关注点重组划入 Task 2（新增条目 2.5）** — 跨格覆盖是行为场景非被证性质，`tasks.md` 1.4 原文指派有误（控制会话转录失误），迁往 `trample_test.go` 零行为变化。
  - **Ruling: NB2 red 证据誊录保持 Task 3 义务不变** — tasks.md 3.3 既定安排。

## Task 2（回归与不触及核对）

- 实现提交 `ec985e43`：仅改 `internal/sim/trample_test.go`（+132：`TestTrampleDualPlayerLandingSameCellIsIdempotent` 与 helper `landBothPlayersFromAbove`、迁入跨格覆盖用例）与 `property_trample_yield_parity_test.go`（−23：迁出跨格覆盖用例）。2.1 server/client 回归全绿零改动（179s/5s `-race`）；2.2 不触及证据链——cmd 层零耕地构造、耕地唯一生产来源是 `TillSoil`（`farming.go:100`）、worldgen 无 farmland/wheat、benchmark `fixedBenchmarkPlayerInput` 的 `Jump:true` 只产生空候选结算（无数值观察义务）；2.3 archcheck 通过；2.4 幂等锚定测试首跑即绿（Task 1 读判实现已满足 MUST 条款，无实现缺陷）；2.5 纯迁移逐字节一致、`go test -list` 集合 400→401 唯一增量是新测试。
- 实现偏差（已核实可接受）：双玩家落地无法复用 `landPlayerAt`（`onlyMovementPlayer` 硬断言恰好一名玩家），新增最小 helper `landBothPlayersFromAbove`；按「单文件私有 helper 留在消费文件」落位。
- 评审：**SPEC PASS**（2.1–2.5 逐条独立核实：不触及证据三环复核、幂等测试五重锚定核实——真实双会话/同 tick 落地/覆盖格逐格相同/三重堵死双重结算/tick 对齐断言）；**QUALITY PASS**（helper 白盒直读权威状态是同包普遍模式、注释合规、红线零越界、复跑全绿）。评审者 NB：ledger/tasks 勾选待评审后誊录（本条即该誊录，流程节奏与 Task 1 一致）。

## Task 3（整分支门禁与收尾）

- 本任务无代码提交（收尾门禁：只改本 change 的 `tasks.md`/`ledger.md` 产物）。
- **3.1 门禁证据**（GVM go1.26.0，worktree 复用既有 Rust 构建产物）：
  - `gofmt -l .` 无输出（exit 0）；`go vet ./...` 无告警（5.1s）。
  - `go test ./... -race` 全量两次运行：
    - run 1（全冷跑，总 6:12.86）：26 包 ok——`cmd/mornlea` 364.272s、`internal/archcheck` 60.203s、`internal/mesh` 45.201s、`internal/storage` 39.468s、`internal/worldgen` 37.952s、`internal/sim` 23.642s（本 change 自身包，绿）等；唯一失败 `internal/server` 268.599s：`TestCompanionDialogueTerminalCoversFourTerminalStates/Stopped`（61.98s）在 `companion_dialogue_wiring_test.go:295` 的 `waitDialogueRequests(t, dialogue, 2)` 处按 60s `longWaitDeadline` 超时。
    - run 2（同命令复跑，总 2:58.80）：`internal/server` 全新执行 ok 177.139s，其余包复用 run 1 的 ok 缓存——即每个包都已在完全相同的门禁命令下真实通过至少一次，exit 0。
- **Ruling: 3.1 门禁采信复跑** — 决定：run 1 的 `TestCompanionDialogueTerminalCoversFourTerminalStates/Stopped` 超时认定为与 B-05 无关的既有台词域负载 flake，同命令复跑全绿即作为 3.1 的门禁证据；双数据点（run 1 红 / run 2 绿、负载观察）如实誊录，不稀释表述。为什么：五环证据链——① 本分支 diff 仅 `internal/sim` 五文件与 openspec 产物，`internal/server` 及失败测试文件零触碰（其最后修改为基线提交 `25f69af3`/`4292201f`）；② 该测试无任何 farmland/trample 语义（grep 零引用、测试世界无耕地，踩踏在其世界零工作量）；③ 该测试隔离复跑 `go test ./internal/server -race -run … -count=1` 2.767s 全绿；④ Task 2 已在本分支跑过 `./internal/server -race -count=1` 全绿（179s）；⑤ 仓史 `ci-retry-isolation` Task 6 已根修过同类台词「单在途守卫 + outcome 就绪时序」负载 flake（`7cb3d0cc`/`0b1712b6`/`25f69af3`/`07389b07`）。且 run 1 满载（191% CPU）下 server 包 268.6s vs 复跑 177.1s 的耗时差与 60s `longWaitDeadline` 超时吻合负载诱发；控制会话另引 F-03 收尾先例（CI 首跑 1 flake、重跑后 8/8 全绿，同一测试域）。错在：非本 change 缺陷——若归因本 change，则范围隔离、语义隔离与隔离复跑全绿三环均无法解释。
- **3.2** `openspec validate --all --strict --no-interactive`：勾选前后各跑一次，均 65 passed / 0 failed（含 `change/farmland-trample`）。
- **3.3 未决项清偿确认**：Task 1 评审 NB1→2.4、NB3→2.5 已在 Task 2 完成并双评审 PASS；NB2（red 证据誊录）由本节清偿——Task 1 节已含 red-first 记录（四个 Trample 用例实现前失败、容量用例为负向断言天然通过），终审引用即可。**无任何未决项需誊入 proposal「延期与放弃」**；proposal 既有「非目标」三条（伙伴踩踏、采掘耕地连带、流体冲毁联动）为认领阶段 Ruling 产物，非收尾新增欠账。
