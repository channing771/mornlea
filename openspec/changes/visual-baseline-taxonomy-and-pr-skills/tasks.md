## 1. OpenSpec 产物

- [x] 1.1 写 `proposal.md`、`design.md`、本 `tasks.md`，`.openspec.yaml` 置 `skip_specs: true`（文档与工具沉淀、无行为变更）；验证 `openspec validate --change visual-baseline-taxonomy-and-pr-skills --strict --no-interactive`

## 2. 基线分类规则入 README

- [x] 2.1 在 `testdata/visual-golden/README.md` 增补三类边界与选用规则节（窗口型归 `ui`、单帧稳定态归 `world`、跨 tick 过程归 GIF 并分子类；同一行为禁双存、例外注明理由；只引用入口命令与源码常量，不复制阈值与清单）；验证人工通读该节且 `openspec validate --all --strict --no-interactive` 通过

## 3. 项目级 skill 双写

- [x] 3.1 新增 `visual-baseline` skill（同文双写 `.claude/skills/visual-baseline/SKILL.md` 与 `.codex/skills/visual-baseline/SKILL.md`）：三类路由、抓帧/比对/更新入口、先目检后覆盖纪律；验证两文件逐字节一致（`diff` 无输出）
- [x] 3.2 新增 `pr-submit` skill（同文双写 `.claude/skills/pr-submit/SKILL.md` 与 `.codex/skills/pr-submit/SKILL.md`）：确认硬门禁（`confirm.sh ask/wait`）、`gh pr create` 规范、`pr-finalize.sh` 监听至全绿才合并、失败定位与上限轮次、回填归档；验证两文件逐字节一致（`diff` 无输出）

## 4. 收尾门禁

- [x] 4.1 收尾：`test -z "$(gofmt -l .)"`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`；`git status` 只含本 change 产物、README 规则节与 4 个新增 `SKILL.md`
