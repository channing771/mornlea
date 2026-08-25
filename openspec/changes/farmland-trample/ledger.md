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
