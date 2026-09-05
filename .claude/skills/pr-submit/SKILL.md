---
name: pr-submit
description: 任务完成后提交 PR 的唯一入口：先经用户确认，再推送、建 PR、监听 CI 至全绿、合并、回填归档。任何直接合并都必须显式指定才允许。
---

## 确认硬门禁（建 PR 之前）

- 经 `scripts/agents/confirm/confirm.sh ask` 发起确认（标题含任务行 ID 与短设计），再 `confirm.sh wait` 等待回复；通道见 `docs/agents/confirmation-channel.md`（飞书优先，超时或不可用降级 GitHub Discussion 协议并停在确认点）。
- 未收到明确批准不得 `gh pr create`，不得静默开工或自行放行。

## 建 PR

- 推送分支后 `gh pr create`：标题沿用单行英文格式（祈使句、小写开头、不以句号结尾，含任务行 ID），正文用英文并采用 `.github/PULL_REQUEST_TEMPLATE.md`（`Summary` 列改动与契约/版本影响，`Validation` 逐行列验证命令与门禁结果，可附 OpenSpec change 链接）。
- 默认走 `AGENT_MODE=pr` 全链路；仅在用户显式指定时才跳过 PR 直接本地合并推送（仍需本地全绿）。

## 监听 CI 至全绿

- 首选 detached 守护：`nohup scripts/agents/pr-finalize.sh <PR号> >> <log> 2>&1 &`（监听 CI、失败自动 failed-only 重跑最多 3 轮、全绿后合并；3 轮仍红则保持 OPEN 交人工）。
- 手动等价链：`gh pr checks --watch` → 失败用 `gh run view --log-failed` 定位 → 本地修复推送 → 重新监听，上限 10 轮，超限停止并报告。
- 合并硬门禁：检查总数大于 0 且每项成功态才可 `gh pr merge --merge`；只数失败项会把 PENDING 误判为可合并，禁止抢跑。

## 合并后回填

- `git checkout main && git pull --ff-only`；按 `docs/development-process.md` 阶段 5 回填 backlog 状态、Discussion 同步与 change 归档（`openspec sync` → `openspec archive`）。
