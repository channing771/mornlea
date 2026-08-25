# E-10 `fix-spawn-support-top` 执行账本

- 认领基线：`c49cc50b`（`fix/E-10-spawn-support-top`）。
- 内容确认：`bounded`；`E-10-q1` 选择“枚举全部碰撞盒并复用完整支撑判定”；`E-10-approval1` 于 2026-08-25T05:55:10Z 显式批准短设计。
- 基线验证：`make rust` 通过；`go test ./internal/sim -race -count=1` 通过（8.899s wall）。
- Ruling：任务组标题采用 `Task 1` 形式，以兼容 SDD 的 `task-brief` 提取器；任务内容与 OpenSpec checkbox 编号不变 — 这是执行工具的格式要求 — 若判断错误只影响 brief 生成，不改变产品行为。
- 实现：先加入 `spawn_support_top_test.go` 三条真实耕地行为测试；focused RED 通过（出生实际为 `y=1`，期望 `y=0.9375`），随后 `findSpawnInColumn` 按最多 8 个碰撞盒顶面降序枚举，并复用 `playerBoundsAreFree` 与 `playerSupport` 的完整支撑判定。
- GREEN/门禁：focused GREEN、`go test ./internal/sim -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`test -z "$(gofmt -l .)"` 与 `openspec validate --all --strict --no-interactive` 通过；fresh `go test ./... -race` 退出码 0，通过 `cmd/mornlea`（206.389s）、`internal/archcheck`（20.854s）、`internal/sim`（cached）及其余全部包。
- Task 1 初审：SPEC 核心行为通过，QUALITY 提出 2 个 Important——SDD scratch report 被误提交、全量 race 缺最终证据。
- Task 1 修复 R1/5：提交 `12635638` 从 Git 索引移除 report，并补录 fresh 全量 race exit 0；scoped re-review 判定 2/2 ADDRESSED、无新 breakage，APPROVE。
- Task 1 完成：提交范围 `3b67935e..12635638`，任务级 SPEC + QUALITY 双评审 clean。
- 整分支终审：0 Critical、0 Important、3 Minor；提交 `fd384fc2` 收紧 safe 字面断言、删除 design 中未实现的“去重”表述并清理 delta 尾部空白。唯一 scoped re-review 判定 3/3 ADDRESSED、无新 breakage，APPROVE。
- 控制会话最终门禁：`scripts/agents/gates.sh` 全部通过——gofmt、`go vet ./...`、archcheck、OpenSpec strict 63/63、`make rust`、`go test ./... -race` 均 exit 0；其中 `internal/sim` race 7.515s、archcheck race 17.380s。
- record-only benchmark（Apple M5）：`BenchmarkEngineStepPlayer` 5865 ns/op、504 B/op、18 allocs/op；`BenchmarkEngineStepFourCompanions` 4069 ns/op、1536 B/op、28 allocs/op。改动只在 PendingSpawn 扫描，活跃 tick benchmark 不改变退出状态或基线。
- 规格沉淀：delta 已同步为主规格 `authoritative-spawn-support`，change 归档为 `2026-08-24-fix-spawn-support-top`；归档后 `openspec validate --all --strict --no-interactive` 为 63/63 通过。
