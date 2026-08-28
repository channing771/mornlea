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

### T2 HUD 视觉精修 — PASS（零实现修复轮；1 次测试夹具修正）

- Implementer 报告：13 个图集 cell mask 就地重绘（心 142→100px 双叶收尖、气泡外扩对齐视觉字号、鸡腿肉盘收紧、火焰对称泪滴、箭头青蓝→中性灰三角头——唯一色相收敛已裁决、凹槽 1px 边、三标题 cell 换干净字形）；聊天/数字/打开面板/耐久/进食/来源高亮全部迁入 `style.go` 令牌；底部贴条保持双层无边，关闭最坏仍恰 100、打开 261；补 `appendCrosshair` 零尺寸直达测试（T1 移交③收口）。
- SPEC PASS：墓碑断言经 HEAD 实算核验为真值（13 SHA + 5 面积计数全吻合）；图集结构零改动（3 项 UV 稳定性测试绿）；T1 文件零触碰、准星/弹条测试全绿；「无第二份色值源」扫描测试判定面经枚举验证有效。
- QUALITY PASS：mask 几何逐项核验（对称/连通/1px 轮廓）；扫描 tripwire 定位恰当（误报响亮、漏报需刻意规避、其上有钉值+行为断言纵深）；热路径零变化（atlas 仅启动期构建）；`-count=2` 复跑无 flake。
- 控制会话裁决：`containerSourceHighlightColor` 青蓝列入 design D1 语义色保留表（「待移动来源」状态色，几何区分外扩描边）。
- 移交 T3 顺手项：style.go:20 亮边适用面注释措辞、health.go:139「上下对称」措辞、`atlas_mask_test.go:195`/`style_test.go:101` 空赋值笨拙、layout.go:257 分隔线几何注释（2 design px 非 1px）。
