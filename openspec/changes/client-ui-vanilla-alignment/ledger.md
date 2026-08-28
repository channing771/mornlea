# Ledger: client-ui-vanilla-alignment

## 2026-08-29 控制会话：change 设立

- 用户三项裁决：范围 = HUD + 容器 + egui 全量；容器 = 原版式浮动面板；风格 = 混合（像素图标 × 现代面板，深色半透明 + 1px 亮边 + 单一琥珀强调色）。
- 探底结论：HUD 布局全在 Go `internal/render/hud/`，命中共源；准星从未实现；容器为底栈行式；egui spec 只钉行为不钉颜色（无需 delta）；`core.BlockDisplayName` 已有、物品级中文名缺。
- 资源契约重钉：quad 267→320（关闭最坏 100、打开最坏 274）、glyph 700→768、glyph offset 15616、总容量 52480；benchmark scenario v19→v20。协议/schema/ABI/配置版本全部不动。
- 产物：proposal.md、design.md（D1–D9）、tasks.md（6 组）、三份 delta spec。待 `openspec validate --strict` 后提交并开工 T1。

## 评审与裁决记录

（随流水线追加：每个任务组的 implementer/SPEC/QUALITY 结论、修复轮次、整分支终审。）
