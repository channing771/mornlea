# Ledger: crop-random-drop-count

> 记录本 change 的进度、评审结论与全部裁决（Ruling）。格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。

## 裁决记录（认领与设计阶段，2026-08-25）

- **Ruling: B-01 拆分，本 change 只承接其作物半边的姊妹行 B-10** — 用户（控制需求方）裁决：B-01 的肉类与熟食熔炉食谱并入 B-27（被动生物）随其落地交付；作物半边因与第一批次 A-02 独占文件集（`internal/assets`、`internal/mesh`、engine crate）重叠且 mesh registry 容量仅剩 3/48 条与批次编号契约耦合，按积压表「换行或延迟」规则延迟。原选 B-01 全量的路径违反「依赖行已满足才可认领」（肉类依赖未开工的 B-27）。已回写 `docs/feature-backlog.md` B-01/B-10 两行（提交 `6a06dd12`）。
- **Ruling: 认领 B-10 并冻结独占文件集** — `internal/sim/mining.go`、`internal/sim/crop.go`、同包 `*_test.go` 与本 change 产物；刻意不触碰 `tunables.go`/`drop.go`/`hunger.go` 与 `internal/core` 编号段（A-01/A-04 已认领）。核实这些文件不在任何已认领行的独占集内。
- **Ruling: server 测试文件与 E-11 的边界** — 本 change 需改 `internal/server/farming_loop_e2e_test.go` 的收获数量断言，该文件在 E-11 独占的 `internal/server/*_test.go` 内；按 A-04 行先例记录「仅改行为断言、不碰等待助手/helper，关注点不相交」，合并序如遇 E-11 重排由集成裁决。
- **Ruling: 掉落数量范围采用「小麦 1–3、种子 1–3，两次独立抽取」** — 用户在三个候选（小麦固定 1 / 组合表四选一）中选定；经济均值从每株 3 件升到 4 件被显式接受。
- **Ruling: 数量范围不进 tunable** — 固定常量表先例（饥饿疲劳表）；同时避开 A-04 独占的 `tunables.go`。
- **Ruling: 设计批准** — bounded 短设计（阶段 0.5）经用户显式批准后开工。

## 工具链事件

- 机器迁移丢失 node/npm 与全局 `openspec` CLI；经用户确认后经 fnm 提供的 node v24.19.0 重装钉死版本 `@fission-ai/openspec@1.7.0`（docs/openspec.md:18）。

## Task 1（sim 层 yield 哈希与收获接入）

- 实现提交 `af1b3a76`：`cropYieldRollSalt`/`cropYieldRolls`（crop.go）、mining.go 接入、`property_crop_yield_test.go` 三性质（重放确定 / 区间穷举 5376 样本 / 双流独立）、farming_test.go 断言区间化。red-first：实现前 `-run TestCropYield` 构建失败，green 后全包 `-race` ok。
- **Ruling: 范围偏差接受（mining_test.go 孪生断言）** — brief 文件清单漏列 `internal/sim/mining_test.go:494` 的同模式精确断言；该文件属认领裁决独占集「同包 `*_test.go`」，修复为最小镜像（断言+注释各一处）。brief 窄化转录失误，非实现越权。
- 评审：SPEC PASS；QUALITY FAIL（blocking：crop.go 取模偏差注释「比 `%100` 小十余个数量级」数学失真）。
- 修复循环 R1（1/5）：提交 `fc630c2e`——删除失真比较从句、design.md D1 数字对齐（`1e-17`→`1/2^64` 上界表述）、新增行裸词标识符补反引号。QUALITY 复核 PASS。
- **遗留（冻结集外，待 Task 3 / 集成裁决清偿）**：
  - `internal/core/item.go:222` 与 `internal/companion/plan_types.go:499` 的「1 小麦 + 2 种子」固定掉落注释因本变更失真；前者属 A-01 独占文件，只能由后续 change/集成处理；
  - `AGENTS.md`/`CLAUDE.md` 基线句「掉落固定不随机：成熟作物 1 小麦 + 2 种子」在本 change 合入后失真——归档收尾须同步两份基线与 `progress.md`（若与第一批次 A-06/A-07 合流窗口冲突，按集成裁决顺延）；
  - server e2e 断言失配为 Task 2 计划内状态（SPEC 评审已核实仅 `TestFarmingLoopEndToEndMemory` 受影响）。

## Task 2（server e2e 断言同步）

- 实现提交 `138d92c2`：仅 `internal/server/farming_loop_e2e_test.go`——等待循环改区间成员判定（骨架与 tick 上界保留）、小麦 [1,3]、种子增量锚定 `seedsAfterPlant+[1,3]`、再种存量 `seedsAfterHarvest-1`、自持断言从「净赚」改回规格本义「不亏」。
- **Ruling: 自持断言语义修正** — 主规格与 delta 从未承诺净赚，旧断言是固定掉落时代的超规格强化；种子下限 1 下最坏打平合法，「不亏 ⟹ 存量单调不减 ⟹ 循环不死」逐字对齐规格。非放松门禁。
- **Ruling: 断言位置超出 brief 引用行号** — 第 7 步两条产量相关精确断言在 :349-358（brief 写约 ：315-338）；属 brief 授权的「同函数内依赖产量的精确断言」范围，按区间化一并处理。
- 评审：QUALITY PASS（含 `-count=3` 无 flake、生长预算 10 倍余量论证）；SPEC PASS（覆盖无删减、E-11 边界零触碰、`cmd/` 复核 0 命中权威采掘结算——tasks 2.3 关闭，无 benchmark/capture 观察义务）。
- 环境备注：本机 Go 经 gvm（go1.26.0）。

## Task 3 / 整分支终审

- 门禁（2026-08-25，worktree 内 `scripts/agents/gates.sh` + 补跑）：gofmt 无输出、`go vet ./...` 通过、archcheck ok、OpenSpec strict 65/65、`make rust`（固定 1.97.1）通过、全量 `go test ./... -race` 全绿（含 `cmd/mornlea` 600s 重型套件）。
- 备注：本机 Go 经 gvm（go1.26.0）、node 经 fnm（v24.19.0）；非交互 shell 需显式拼 PATH，否则 gates 第 4/6 步因命令缺失误报 FAIL（已复核为工具可用性问题，非校验失败）。
- 整分支终审：**PASS**（全新 reviewer，8 条清单全过）——规格合规与主规格替换自洽；范围/上限无放宽；并发与重放契约成立；wire/存档零触碰（network/storage/core/companion diff 为空）；注释与产物内部一致；测试组织合规；提交卫生合格。non-blocking 三条：① design.md D2 的 `crop.go:251` 行号漂移（标识符正确）；② 终审期一次高负载下 server 等待预算偶发超时（隔离复跑 0.1s，本分支零触碰文件，属 E-11 领域）；③ 合入→归档间的基线句失真窗口，合入与归档宜紧耦合。**合入建议：可直接 PR**。
- 归档收尾待办（阶段 5，合入后执行）：同步 `AGENTS.md`+`CLAUDE.md` 掉落基线句与 `docs/notes/progress.md` → 回填 backlog B-10 行为「已完成」→ `openspec sync` + `openspec archive`。

## 分支提交履历

- `988871f9` docs(openspec): propose crop-random-drop-count
- `af1b3a76` sim: hash-driven mature wheat yield (B-10 task 1)
- `fc630c2e` sim: fix yield comment math and backtick identifiers (B-10 R1)
- `138d92c2` server: align e2e assertions with hashed yield (B-10 task 2)
- `7c8fbd68` docs(openspec): record task reviews and deferred items
