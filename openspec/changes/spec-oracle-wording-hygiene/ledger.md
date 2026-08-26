# E-14 `spec-oracle-wording-hygiene` 执行账本

- 认领基线：`7cfd4c7c`（`feat/E-14-spec-oracle-word`，worktree `.worktrees/E-14-spec-oracle-word`）。
- 分类裁决：**bounded**——纯主规格措辞更正（docs-only），无代码、测试、golden 或契约版本变化；任务 brief 即需求来源，单实现者无子派发。
- Ruling: delta 采用 MODIFIED-only——Requirement 名称与 Scenario 标题零改动，只改正文与 Scenario 项目符号文本（brief 明示，且 openspec 1.7.0 的 MODIFIED 漂移守卫本就要求 Scenario 标题逐一对应）。
- Ruling: 勘察发现 openspec 1.7.0 两项 apply 行为使纯 delta 清不掉全部 16 处失真：①delta 的 `## Purpose` 对已存在主规格被忽略；②MODIFIED 不支持 Scenario 改名。5 处残留（2 句 Purpose + 3 个 Scenario 标题）列为归档阶段五行直编收尾项，替换文本固化于 design.md D2；mesh 规格的 2 处 oracle 措辞维持原样（其 oracle 实存，措辞当前为真）。

## Task 1

- Implementer：本会话。通读三份主规格全文、三个测试文件（`step_golden_vectors_test.go` 13 向量、`raycast_fuzz_test.go` 五不变量 + 孪生、`parity_test.go`/`tree_test.go`/`generator_test.go` 黑盒网）与归档 change `2026-08-26-drop-go-test-oracles` 全部产物后落笔；每条 THEN 按真实测试断言粒度书写（如黄金摘要 GIVEN 收敛到固定语料、「两个平台逐位一致」WHEN 收敛掉随机差分语料）。
- 结论：（待评审）

## 终审

（待整分支终审）
