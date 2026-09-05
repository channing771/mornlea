## Why

视觉基线实际已是三大家族（`ui` 窗口部件、`world` 世界静态、`motion` + `passive-death` 过程 GIF），但分类依据只散落在 README 表格与各包注释里，静态与全流程 GIF 的选用没有成文规则，已出现同一行为 PNG 与 GIF 双存（如吃草静态对照与吃草 GIF、裂纹双帧与采掘全流程演示）。同时 PR 提交前确认、CI 监听、合并、归档链只存在于开发流程文档与零散脚本中，每次靠人工拼凑命令。

## What Changes

- 在 `testdata/visual-golden/README.md` 增补三类边界与选用规则：窗口型归 `ui`、单帧稳定态归 `world`、跨 tick 过程归 GIF（内分演示与门禁两小类）；同一行为禁止 PNG + GIF 双存，例外必须在 README 注明理由。
- 新增项目级 skill `visual-baseline`：三类路由、抓帧/比对/更新入口与先目检后覆盖纪律，只引用既有常量与命令，不复制阈值与清单。
- 新增项目级 skill `pr-submit`：提交 PR 前经用户确认，确认后推送、`gh pr create`、`pr-finalize.sh` 监听 CI 至全绿、合并、回填归档；默认 `AGENT_MODE=pr`，直接合并仅在显式指定时允许。
- 两个 skill 同文双写 `.claude/skills/` 与 `.codex/skills/`。

## Capabilities

本 change 为文档与工具习惯沉淀，不改变游戏可观察行为，不新增或修改任何 spec capability；`specs/` 无 delta，以 `skip_specs: true` 标记。

### New Capabilities

无。

### Modified Capabilities

无。

## Impact

- 影响：`testdata/visual-golden/README.md`（只增规则节，不改基线文件）、新增 4 个 `SKILL.md`（两 skill × 双目录）、本 change 产物。
- 不影响：任何 PNG/GIF 基线字节、`captureScenes`、`fixtureNames`、阈值、协议/存档/ABI/benchmark；不增删场景，不放宽门禁。
- 兼容性：纯文档与 skill 新增；回退即删除新增节与 skill 目录。
