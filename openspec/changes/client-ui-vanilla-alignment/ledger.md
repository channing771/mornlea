# Ledger: client-ui-vanilla-alignment

## 2026-08-29 控制会话：change 设立

- 用户三项裁决：范围 = HUD + 容器 + egui 全量；容器 = 原版式浮动面板；风格 = 混合（像素图标 × 现代面板，深色半透明 + 1px 亮边 + 单一琥珀强调色）。
- 探底结论：HUD 布局全在 Go `internal/render/hud/`，命中共源；准星从未实现；容器为底栈行式；egui spec 只钉行为不钉颜色（无需 delta）；`core.BlockDisplayName` 已有、物品级中文名缺。
- 资源契约重钉：quad 267→320（关闭最坏 100、打开最坏 274）、glyph 700→768、glyph offset 15616、总容量 52480；benchmark scenario v19→v20。协议/schema/ABI/配置版本全部不动。
- 产物：proposal.md、design.md（D1–D9）、tasks.md（6 组）、三份 delta spec。待 `openspec validate --strict` 后提交并开工 T1。

## 评审与裁决记录

### T1 生存 HUD 布局对齐 + 准星 + 物品名弹条 — PASS（零修复轮）

- Implementer 报告：`style.go` 令牌表（语义色原值迁入）、底边距 24→6、`closedHUDHeight` 144（含弹条行 6+16）、准星 4 quad 先于面板、弹条 40 tick 双重抑制（记录侧+呈现侧）、`core.ItemDisplayName`（23 个显式名 + 方块回退，全量覆盖测试）、容量重钉 320/768/15616/52480、关闭最坏恰 100 固定断言。红→绿证据齐全（`TestItemDisplayName*`、准星中心投影修正、弹条 tofu advance 修正、未确认帧 +4 语义同步）。
- SPEC PASS：delta 4 Requirement 逐场景钉住；未修改 Requirement 回归全绿（图集 UV、169px、氧气堆叠、saturation-jitter、采掘/进食互斥）；范围无越界；注释纪律干净；测试非恒真（40 tick 双侧边界 39/40）。
- QUALITY PASS：零分配热路径保持（AllocsPerRun=0）；命中/绘制单源保持（边距变更经共享 `hotbarRowBounds`）；注释中文无任务编号。非阻塞备注移交：①聊天/数字/打开面板/耐久色未迁令牌（T2 收口）；②`panelBorderLight` 等预埋令牌零消费（T3 消费）；③`appendCrosshair` 零尺寸门缺直达测试（T2 补）。
- 控制会话裁决：①delta 弹条措辞「MUST NOT 增加固定 glyph 容量」→「MUST 始终在固定 glyph 预算内呈现」；design D3 弹条 glyph ≤16→≤64（实测关闭最坏 548）。②打开最坏实算基线 257（非 266），现钉 261；274 钉值待 T3 实算后修正 design/delta（先例：glyph 预算实测修正）。③1px 亮边收窄为浮动面板/egui 待遇，底部贴条保持双层——保住关闭最坏 100（design D1/D2、tasks 2.2 已同步）。
