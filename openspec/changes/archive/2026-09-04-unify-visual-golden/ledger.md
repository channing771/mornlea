# ledger：unify-visual-golden

change 目标：`testdata/visual-golden/` 统一存放 24 张世界场景基线与 19 张前端部件基线，配中文索引；像素逐字节不变。

| 任务 | implementer | 评审 | 修复轮 | 验证 | 裁决 |
|---|---|---|---|---|---|
| Task 1.1 | fresh implementer（报告 `task-1-report.md`，提交 `b2e0d05b`） | 独立 reviewer：SPEC PASS + 质量 Approved，0 Critical/Important | 0（Minor 不进循环） | visual-check 24/24 零差异、frontend-visual-check 19/19、capture race ok、archcheck ok、openspec 87/87 | ACCEPTED |

## 过程记录

- 2026-09-04：change 创建（proposal/design/tasks，`skip_specs: true`）；用户已批准短设计（根 `testdata/visual-golden/` + `world/`/`ui/` + 中文 README）。
- 工作区已有改动（规则精简 5 文件）必须保留，implementer 不得触碰。
- Task 1.1 完成：43 PNG 全为 `rename (100%)`，6 处文本（README 索引 + 2 常量/脚本 + 3 AGENTS）同步；5 禁区文件零触碰（`visual-golden` 出现 0 次）。
- Ruling: 评审包 DIFF 段误装工作区未暂存 diff（修订符写在 pathspec 之后所致）——错在控制器打包命令，评审者已用 `git show b2e0d05` 直取对冲，结论不受影响；后续打包一律把修订区间写在 `--` 之前。
- Ruling: M1（`scripts/make_demo_gif.swift:6` 旧路径）与 M2（根 README 示例图、`visual-verification.md`、`engine/.../mornlea_client/AGENTS.md:43` 活引用）本任务不修——错不在实现（brief 限定三处 AGENTS），修它们属扩范围；parked，移交后续 change。M2 历史叙事类（`test-organization`、`repository-code-organization` 主规格、归档 changes）永不原地改。
- Ruling: 评审备注的既存漂移（两处 AGENTS 写“22 景”，实际 24 场景）本次未引入故不修——错在旧文档与场景表脱节；parked，另起 change 订正。
- Ruling: 旧两目录空目录残留不处理——git 不跟踪目录，`git status` 无残留即达标；`rmdir` 零 diff，可做可不做。
