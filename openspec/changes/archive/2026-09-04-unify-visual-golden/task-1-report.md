# Task 1.1 实施报告

- 提交：`b2e0d05b963614df164fec21f89c3f401cfad595`
- 信息：`docs(golden): unify visual baselines under testdata/visual-golden`
- 范围：仅 Task 1.1（搬迁基线并建索引）；未触碰工作区已有 5 文件改动，未重跑任何 `visual-update`，未派生子代理。

## 改动清单

搬迁（`git mv`，像素逐字节不变，`sha256` 内容哈希搬迁前后一致）：

- `cmd/mornlea/capture/testdata/golden/*.png`（24 张）→ `testdata/visual-golden/world/*.png`
- `engine/crates/mornlea_client/frontend/visual/golden/*.png`（19 张）→ `testdata/visual-golden/ui/*.png`

新建：

- `testdata/visual-golden/README.md`（中文）：`world` 表 24 行（文件名→`capture.go` 的 `captureScenes` 场景名→一句话说明），`ui` 表 19 行（文件名→`fixture-names.ts` 的 fixture 名→`fixtures.tsx` 对应组件），另写更新入口（`make visual-update` / `make frontend-visual-update`，比对入口 `make visual-check` / `make frontend-visual-check`）与先目检后覆盖纪律，不抄阈值数字。

路径同步（4 组）：

- `cmd/mornlea/capture/capture_image.go`：`captureGoldenDir` 常量改为 `testdata/visual-golden/world`，注释中文且 `captureGoldenDir` 加反引号。
- `cmd/mornlea/capture/capture_near_band_test.go` 约 71 行硬编码改为 `filepath.Join("testdata", "visual-golden", "world")`。
- `engine/crates/mornlea_client/frontend/visual/visual.mjs`：`goldenDir` 改为 `path.join(repoRoot, "testdata", "visual-golden", "ui")`（经既有 `repoRoot` 变量，未手写多层 `..`）；同文件用法注释同步为新路径。
- 三处 `AGENTS.md`（正文中文、标识符英文、无任务编号）：`cmd/mornlea/capture/AGENTS.md` golden 纪律节路径更新为 `testdata/visual-golden/world`；`cmd/mornlea/AGENTS.md` Directory Map 删除已不存在的 `testdata/golden/` 嵌套行，`capture/` 行注明基线在仓库根 `testdata/visual-golden/world/`；`engine/crates/mornlea_client/frontend/AGENTS.md` 基线节两处 `visual/golden/*.png` 更新为 `testdata/visual-golden/ui/*.png`。

## 验证命令与输出摘要

1. `git status` 确认旧目录无残留：`cmd/mornlea/capture/testdata/golden/` 与 `engine/crates/mornlea_client/frontend/visual/golden/` 均为空（仅剩空目录，无 PNG 残留）；新目录 `world/` 24 张、`ui/` 19 张；`git diff --cached --stat` 43 个重命名零增删。通过。
2. `make rust`（前置）：`cargo build --locked --release` 成功，`mornlea_engine` 与 `mornlea_client` 均编译通过并重签。通过。
3. `go test ./cmd/mornlea/capture -race -count=1`：首次在未构建 Rust 产物时 panic（`internal/nativeabi.raycastBatchResult`，缺 dylib，属环境前置缺失非回归）；补跑 `make rust` 后重跑 `ok ... 11.339s`。通过。
4. `make visual-check`：24/24 场景差异像素均为 0（`0/230400`），退出码 0，无 `*-actual.png`/`*-diff.png` 产出（行为即全绿）。通过。
5. `make frontend-visual-check`：Chrome 存在无需跳过；19/19 部件通过，差异像素均为 0（`0/921600`），末行“全部 19 个部件与基线一致”，退出码 0。通过。
6. `go test ./internal/archcheck -count=1`：`ok ... 6.928s`。通过。
7. `openspec validate --all --strict --no-interactive`：`Totals: 87 passed, 0 failed (87 items)`，含 `change/unify-visual-golden`。通过。

补充：搬迁前后 `sha256` 内容哈希逐集合比对一致（`WORLD-BYTE-IDENTICAL` / `UI-BYTE-IDENTICAL`）；禁区 5 文件（`AGENTS.md`、`docs/development-process.md`、`docs/feature-backlog.md`、`docs/openspec.md`、`openspec/config.yaml`）的 diff 中 `visual-golden` 出现 0 次，未触碰。

## 遗留问题

- 范围外仍引用旧路径的文档未动（属 Task 1.1 非目标，需后续 change 另行处理）：根 `README.md` / `README.en.md` 示例图、`docs/notes/visual-verification.md` 基线段、`docs/test-organization.md`、`openspec/specs/repository-code-organization/spec.md` 及归档 change、`engine/crates/mornlea_client/AGENTS.md` 的 `frontend/visual/golden/` 提及。功能无影响（生产与门禁均走已同步的常量与脚本变量）。
- 旧基线目录现为空目录（git 不跟踪空目录），残留在工作树无害；如需彻底移除可由收尾任务决定，本任务未删除以保持最小 diff。
