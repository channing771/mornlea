# attack-exhaustion Ledger

## 2026-08-25 认领与确认

- 认领：`docs/feature-backlog.md` B-13 攻击疲劳半边，认领人 `ox-alpha-implementer @ feat/B-13-attack-exhaustion`，docs-only 提交 `bcc900fb`（主仓 main）。
- 路径分类：**bounded**（仓库既有流程改动——既有近战成功路径加一行固定疲劳表项），对话内短设计。
- 设计确认：用户显式批准（「批准」）。设计要点：`exhaustionMeleeMilli = 100`（参考实现 0.1 × 1000 同口径）；判定点在 `advancePlayerMelee` 意图冻结处；共享 melee 测试 helper 先迁 `helpers_test.go` 中心再写新测试文件。

## Rulings

- Ruling: 判定点选在意图冻结处而非伤害结算循环 — 冻结即收费与该处既有副作用聚合点（采掘抑制、目标冷却写入）同位，且结算循环保持 `applyDamage` 单一职责，未来 A-06 统一战斗调整结算顺序时疲劳语义不被动漂移 — 放结算循环会让资源语义寄生在伤害结算顺序上。

## Rulings（续）

- Ruling: 测试断言用字面量 100 而非引用尚未存在的 `exhaustionMeleeMilli` 常量 — 保证 RED 是「行为缺失失败」而非编译失败 — 代价是实现改值须同步断言，由 spec 固定值兜底。
- Ruling: `TestMemoryTCPFluidDamBreakBroadcastParity` 的偶发失败为既有负载敏感型 TCP 对齐竞态，不属本 change 修复范围 — 交替二进制对照实验证明失败率只随机器负载波动、与代码版本无关（HEAD 与 merge-base 在静载窗口同为 0/30），且该测试不发送输入、近战路径不可达 — 不修则 PR CI 偶发重试（F-03 先例）；修复归属 E-11 独占的 `internal/server/*_test.go`，认领行不得抢。

## 任务进度

- Task 1.1：✅ e07a015c（helper 迁移字节级一致，`go test -list` 集合一致）
- Task 1.2：✅ 1ac97f12（RED 三性质测试，断言失败形态正确）
- Task 2.1：✅ cfb7ef72（GREEN：常量行 + 意图冻结处调用；包内 -race 零回归）
- Task 3.1：✅（sim/archcheck -race、vet、gofmt、openspec strict 65/65 全过）
- Task 3.2：✅（全量 -race 一度遇上述既有 flake，静载重跑全绿）

## 评审结论

- Task A 评审：Spec ✅ / Quality Approved（两条 Minor 记账：RED 转录行号偏一位；相邻双人布置可选去重）。
- Task B 评审：Spec ✅ / Quality Approved（一条 Minor 记账：新注释三处标识符未加反引号）。
- Task C 评审：见终审前补充。

## 任务进度（终稿）

- Task 3.1/3.2：✅ gates.sh 全绿（`GOFLAGS=-timeout=30m`，exit=0）；全量 `-race` 含 internal/server 全 ok。
- Task C 评审：Spec ✅ / Approved（实现者零越权，BLOCKED 证据保真）。

## Rulings（终审与收尾补充）

- Ruling: AGENTS.md/CLAUDE.md 基线句随本变更更新为六项固定表（终审 Important 发现；A-batch 各行声明不触碰基线文档、A-07 未认领，本行独立 change 不受其集成独占约束）——不同句子的改动 git 可自行调和。
- Ruling: design.md「由聚焦测试锁定该语义」改为结构保证措辞——死亡结算 `resetHunger` 抹平濒死者读数，该语义跨 tick 边界不可观察，虚指测试属计划产物失准，先改产物再继续。
- Ruling: 新增注释两处交叉引用补反引号、测试文件 RED 时态理由句刷新；既有行的裸标识符按最小 diff 不动。
- Ruling: 跳过 GUI benchmark 实测记录——固定工作负载无战斗输入，本变更不可观察；全量门禁绿已覆盖（数值只记录原则下如需正式 v19 刷新可随时补跑）。
- Ruling: `TestMemoryTCPFluidDamBreakBroadcastParity` flake 为既有负载敏感竞态，归属 E-11 独占文件集，不在本 change 修复（证据链：HEAD 高负载 ~4/30 vs 静载双二进制各 0/30；夹具不发输入近战路径不可达）。

## 整分支终审

- Verdict: With fixes → 修复波 ef3f8cc1（6 文件 +8/−8，零行为变化）→ scoped re-review ALL ADDRESSED, no new breakage。
- Deferred minors（终审 triage）：RED 转录行号偏移 defer；双人布置去重 defer。
