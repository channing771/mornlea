# E-10 `fix-spawn-support-top` 执行账本

- 认领基线：`c49cc50b`（`fix/E-10-spawn-support-top`）。
- 内容确认：`bounded`；`E-10-q1` 选择“枚举全部碰撞盒并复用完整支撑判定”；`E-10-approval1` 于 2026-08-25T05:55:10Z 显式批准短设计。
- 基线验证：`make rust` 通过；`go test ./internal/sim -race -count=1` 通过（8.899s wall）。
- Ruling：任务组标题采用 `Task 1` 形式，以兼容 SDD 的 `task-brief` 提取器；任务内容与 OpenSpec checkbox 编号不变 — 这是执行工具的格式要求 — 若判断错误只影响 brief 生成，不改变产品行为。
