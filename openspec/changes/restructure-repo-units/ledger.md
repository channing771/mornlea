# Ledger: restructure-repo-units

裁决与进度记录。每任务一行裁决（SPEC/QUALITY），评审循环以 subagent-driven-development 为准。

| 日期 | 任务 | 裁决 | 记录 |
|---|---|---|---|
| 2026-09-04 | S0 change 立项 | PLANNED | proposal/design/specs delta（repository-code-organization：3 ADDED + 18 MODIFIED）/tasks（T1–T8）完成；模块切割以 archcheck `allowed` 表为权威输入验证为 DAG；用户裁决：全单元入 `packages/`、engine 保留原名、services 改 agent 服务族（companion 居 `packages/agent/companion`） |
| 2026-09-04 | T1.1–1.4 实现+评审 | PASS | 实现：unit_boundary_test.go（六单元边表+drift+动态激活）、卫生清理（client.test/mesh.test/空 golden 目录/.gitignore/make_demo_gif 路径）、git mv companion → packages/agent/companion（含 Python 仓库根定位深度 +1 的 9 处必需同步）、verify-native-artifact.sh 收敛 CI 三处重复块 + frontend job 改 make frontend-check（corepack enable 替代 action-setup）。定向验证全绿（archcheck/companion-agent-check/integration/frontend-check）。评审四偏离全 ACCEPT；P2 三条：①脚本测试可补钉 test -f/重定向语句（后续）②project-identity 直改已回退、改走本 change delta spec ③commit 时确认脚本可执行位。裁决：shared→∅ 空边表与 design 一致；go.mod require 解析器核验正确（replace/exclude 不误判）；corepack ubuntu 权限前提已核实 |
