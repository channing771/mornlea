## 1. 全尺寸散列（TDD）

- [x] 1.1 先把散列缩放四档期望改 1 并执行见红，再在 `packages/client/render/drop_scatter.go` 删除密度缩放（scale 恒 1，`bob`/层距随之取全尺寸），放宽越界/交叠断言为 delta 规格值（稀疏零探出、密集 ≤0.14 格），保留分组/排序/唯一 cell/抖动/分层/800 容量/支撑语义；运行 `go test ./packages/client/render -race -count=1` 与 `go test ./packages/audit -count=1`，失败定位后修复，记录 red/green 输出。

## 2. 基线与收尾门禁

- [x] 2.1 确认 motion/world 基线比对机制，重出受影响的 `drop-scatter.gif`/`drop-density.gif` 并目检（不动 `avatar-walk.gif`/`break-burst.gif`/`avatar-detail.png`）；执行 gofmt、`make test-race`、`make dev-check`、`openspec validate --all --strict --no-interactive` 与整分支独立审查；结果和裁决写入 ledger，不推送、不合并。
