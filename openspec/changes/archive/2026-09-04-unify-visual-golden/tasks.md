# 任务

每个以下未勾选任务都是独立实现任务。控制会话按 `subagent-driven-development` skill 执行：一轮开发一轮审查，评审定义以 skill 内任务评审为准；控制会话不得直接实现。每项任务完成或修复后，必须在 `ledger.md` 记录任务编号、implementer、评审结论、发现、修复轮次和最终裁决，才可勾选或移交下一项。

## Task 1：搬迁基线并建索引

- [x] 1.1 `git mv` 24 张世界场景 PNG 自 `cmd/mornlea/capture/testdata/golden/` 到 `testdata/visual-golden/world/`，19 张前端部件 PNG 自 `engine/crates/mornlea_client/frontend/visual/golden/` 到 `testdata/visual-golden/ui/`；新建中文 `testdata/visual-golden/README.md` 索引（world 24 行对 `capture.go` 场景名，ui 19 行对 `fixture-names.ts`）。同步 `capture_image.go` 的 `captureGoldenDir`、`capture_near_band_test.go:71` 硬编码、`visual.mjs` 的 `goldenDir`（经既有 `repoRoot`），以及三处 `AGENTS.md`（capture 包、cmd/mornlea 总纲 Directory Map、frontend 基线节）。不得触碰工作区已有改动（`AGENTS.md`、`docs/development-process.md`、`docs/feature-backlog.md`、`docs/openspec.md`、`openspec/config.yaml` 的规则精简 diff）。验证：`git status` 确认旧目录无残留、`make visual-check`、`make frontend-visual-check`、`go test ./cmd/mornlea/capture -race -count=1`、`go test ./internal/archcheck -count=1`、`openspec validate --all --strict --no-interactive`。

## 收尾

- [x] 1.2 `openspec sync` 沉淀（本 change `skip_specs`，sync 应无主规格变化）→ `openspec archive unify-visual-golden` → 同步 `docs/notes/progress.md` 基线段 → 用户授权 merge 模式：直接推送 main（跳过 PR，本地门禁全绿，CI 在 main 上复核）。
