# add-world-loading-screen Ledger

基线 SHA：`06809724`（main，含 D-19 认领行）。工作分支：
`feat/world-loading-screen`（worktree `.worktrees/world-loading-screen`）。

## 阶段 1：内容确认

- Ruling: 需求确认以需求方任务书为准 — 2026-09-02 用户直接委托：进入游戏后
  初始区块渐进浮现期间画面只有天空底色（"透明"），要求 MC 式加载页（加载时
  地图加载 + 进度条，完成后再进入世界）。控制会话经三路代码探索（装配流程/
  菜单 WebView 层/OpenSpec 与测试基线）后呈方案（新增 `MenuPhaseLoading`
  相位 + 不透明 WebView 加载屏 + 复用无头 `LoadedChunkTarget`/
  `ApplicationLoadComplete` 单一判据 + 与 `WaitUntilLoaded` 同源预算），
  用户经计划审批显式批准；分类为 `bounded`（仓库既有相位机、桥协议与加载
  判据内的客户端改动，无新子系统），设计结论全部落入本 change 的
  proposal/design。— 依据：development-process 阶段 1「批准来源 = 用户或控制
  会话的显式确认」。— 无偏差。

## 任务执行

（每任务：implementer 派发、评审结论、验证证据按 SHA 记录于此）

### Task 1：Go 相位机、加载循环与桥 schema

- 派发：pending。

### Task 2：前端加载屏组件

- 派发：pending。

### Task 3：Rust 相位清单与收尾门禁

- 派发：pending。
