# E-14 `spec-oracle-wording-hygiene` 执行账本

- 认领基线：`7cfd4c7c`（`feat/E-14-spec-oracle-word`，worktree `.worktrees/E-14-spec-oracle-word`）。
- 分类裁决：**bounded**——纯主规格措辞更正（docs-only），无代码、测试、golden 或契约版本变化；任务 brief 即需求来源，单实现者无子派发。
- Ruling: delta 采用 MODIFIED-only——Requirement 名称与 Scenario 标题零改动，只改正文与 Scenario 项目符号文本（brief 明示，且 openspec 1.7.0 的 MODIFIED 漂移守卫本就要求 Scenario 标题逐一对应）。
- Ruling: 勘察发现 openspec 1.7.0 两项 apply 行为使纯 delta 清不掉全部 16 处失真：①delta 的 `## Purpose` 对已存在主规格被忽略；②MODIFIED 不支持 Scenario 改名。5 处残留（2 句 Purpose + 3 个 Scenario 标题）列为归档阶段五行直编收尾项，替换文本固化于 design.md D2；mesh 规格的 2 处 oracle 措辞维持原样（其 oracle 实存，措辞当前为真）。

## Task 1

- Implementer：本会话。通读三份主规格全文、三个测试文件（`step_golden_vectors_test.go` 13 向量、`raycast_fuzz_test.go` 五不变量 + 孪生、`parity_test.go`/`tree_test.go`/`generator_test.go` 黑盒网）与归档 change `2026-08-26-drop-go-test-oracles` 全部产物后落笔；每条 THEN 按真实测试断言粒度书写（如黄金摘要 GIVEN 收敛到固定语料、「两个平台逐位一致」WHEN 收敛掉随机差分语料）。
- 结论：SPEC review PASS；QUALITY review PASS（三条 nit，见 R1）。

## R1 修复

- 来源：SPEC/QUALITY 双评审 PASS 后遗留三条 nit——①delta worldgen「单点查询与整块生成一致」THEN 出处括注漏 `generator_test.go`（该文件确含 `TestBaseBlockAtMatchesGeneratedChunk` 等稠密双出口对照）；②「跨区块橡树一致」的树高区间缺具体数值「4..6」，与 design 映射表不一致；③tasks.md 2.1 写「65+ 通过」与实跑 67 不符。
- 处置：三处按 nit 原样清偿（worldgen delta 两行、tasks.md 一行）；未触碰 openspec/specs/、代码、测试、golden 与 mesh 规格。
- 门禁实跑：`openspec validate --all --strict --no-interactive` → **67 passed, 0 failed (67 items)**；`go test ./internal/archcheck -count=1` → ok。
- 提交：（本节随修复提交一并落盘）

## 终审

（待整分支终审）
