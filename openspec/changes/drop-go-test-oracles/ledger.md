# E-04 `drop-go-test-oracles` 执行账本

- 认领基线：`88094977`（`chore/E-04-drop-go-oracles`，worktree `.worktrees/E-04-drop-go-oracles`）。
- 分类裁决：**bounded**——纯测试基建删除 + 基线句窄同步，无生产行为变化、无新子系统；用户 2026-08-25 批准短设计（含两项决策：physics 位级 golden 向量采纳、基线句随本 change 同步采纳）。
- 勘察修正：认领备注「`generator.go` 内失去消费者的 test-only 旧实现」不成立——旧 worldgen 实现早已只存在于 `oracle_test.go` 自包含副本；本 change 生产代码零改动（注释级修订除外）。
- Ruling: change 设置 `skip_specs: true` — 本行只删除测试基础设施并同步基线表述，不改变玩家可观察行为或主规格 Requirement — 为测试删除虚构产品 capability 会让规格失真（同 E-11 先例）。
- Ruling: `internal/mesh` greedy/light oracle 切片延迟 — A-02 独占 `internal/mesh` 且正在演进 Rust mesher，此刻删除差分网恰在变动中的代码上方；待 A 批次合流后另行认领（移交项见 proposal）。

## Task 1

（待派发）

## Task 2

（待派发）

## Task 3

（待派发）

## 终审

（待整分支终审）
