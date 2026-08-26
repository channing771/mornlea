# 任务：spec-oracle-wording-hygiene

## 1. delta specs

- [x] 1.1 写三份 MODIFIED-only delta（Requirement 名称与 Scenario 标题零改动，只改正文与 Scenario 项目符号）：
  - `specs/rust-engine-physics-step/spec.md`：Req「物理 tick 积分由 Rust engine 独占生产」（正文 + 2 个 Scenario 文本；对角加速/跳跃重力 2 个 Scenario 原样重述）与 Req「碰撞差分入口保留」（正文 + Scenario 文本），改述为位级 golden 向量与字面量行为锁。
  - `specs/rust-engine-collision-raycast/spec.md`：Req「Rust collision 保持共享物理结果」正文、Req「Rust raycast 保持惰性 callback 契约」的「多 batch 保持遍历语义」THEN、Req「additive native ABI 原子且无跨调用所有权」的「两个平台逐位一致」THEN，改述为几何不变量 fuzz + 确定性孪生 + 冻结期望。
  - `specs/rust-engine-worldgen/spec.md`：Req「世界生成由 Rust engine 独占生产」（正文 + 3 个 Scenario 文本），改述为黄金摘要文件 + 同种子重放 + 双出口对照 + 树冠几何/跨界树/树高区间性质。

## 2. 验证门禁

- [x] 2.1 `openspec validate --all --strict --no-interactive` 全绿（67 通过、0 失败；归档阶段 validate 总数变化以实跑为准）。
- [x] 2.2 `go test ./internal/archcheck -count=1` 通过（基线版本与基线文档门禁不受影响）。
- [x] 2.3 `git status` 确认改动只含本 change 目录；单提交 `docs(spec): correct stale Go-oracle gate wording in rust-engine main specs (E-14)`。

## 3. 归档阶段收尾项（归档会话执行，非本任务勾选）

按 design.md D2 替换表对主规格做五行直编：physics-step 与 worldgen 的 Purpose 各 1 句、physics 的 2 个 Scenario 标题、worldgen 的 1 个 Scenario 标题。直编后复跑 `openspec validate --all --strict --no-interactive`。mesh 规格的 2 处 oracle 措辞不在本 change 范围，待 mesh oracle 切片独立认领时随体更正。
