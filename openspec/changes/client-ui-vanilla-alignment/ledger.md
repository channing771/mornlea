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

### T3 容器原版式浮动面板 + tooltip — PASS（R1 一轮修复）

- Implementer 报告：`panel.go` 面板几何单源（`panelOrigin` + `containerPanelFrame`，5 类命中函数同源推导）；四类面板（个人 2×2 左上+右栏十条配方 / 工作台 3×3 / 熔炉原版图式 36/37/38 / 箱子 27 格）7-quad 面板族（投影+表面+四边亮边+标题）；`tooltip.go` 悬停中文名（右下优先/翻转/夹回、与弹条同源）；打开最坏实算 **264**（`openWorstQuads` 命名常量分解）、glyph 最坏 710≤768；saturation-jitter 直达测试补齐。
- 控制会话随裁决定稿：打开最坏 274→264（design D3、survival delta、container delta、tasks 3.4 四处同步）；D1 强调色语义收敛为「选中、进度、产物」三类（tooltip 不消费琥珀）；核实 `RecipeButtonAt` 在 main（94405319）已无生产消费方——配方入口为「呈现+可命中、无点击行为」的既有状态，T6 文档照实记录。
- SPEC R0 FAIL（1 项）→ R1 修复：响应式扫描打开分支补有效悬停 tooltip 夹具（箱 0 号格中心，任意缩放落格内），变异验证（基线 +500 → glyph 216 越界）证明断言有牙；SPEC(RE) PASS。
- QUALITY R0 FAIL（1 项）→ R1 修复：`appendCountAtSize` 前景笔重置 `hotbarSlotSize`→`size`（配方栏数字双层 24px 分离），新增 `TestAppendCountAtSizeKeepsLayeredPenAlignment`（shadow==fg+(1,1) 逐字断言，变异复现 110/133 分叉）；另恢复火焰 V1 端点断言、tooltip 坐标源注释措辞；QUALITY(RE) PASS。
- 移交项：`Prepare` 已 19 参——**下一次任何 overlay 追加前必须先做 overlay 输入 struct 收敛**；打开态聊天栈与面板层叠重叠为既有行为未处理。

### T4 egui 菜单换肤 — PASS（零修复轮）

- Implementer 报告：`ui/style.rs` 令牌表 + `ui_style()` 全局 Style 批量接线；删 13 个散落色常量；主菜单底色加深/按钮半透明面板+琥珀悬停描边/标题 32→40px（单一字重字体以字号承担权重，布局关系不变）；暂停遮罩对齐面板族；设置面板内侧亮边+输入焦点琥珀+错误 DANGER；F3 面板令牌+选中行琥珀左缘 3pt 窄条替代蓝色整行高亮；wire layout v1–v4、动作常量、事件批、pass 顺序、dithering 关闭零改动。
- 验证：`cargo test -p mornlea_client` 181 passed/0 failed（debug+release）、clippy 0 warning、fmt 干净、`make rust` 回拷成功、`go test ./internal/client` nativeabi 绿。
- SPEC PASS：wire/行为零改动经 diff 与测试双证；新增 10 个视觉断言全部引用令牌常量、带负向断言（聚焦前无琥珀）非恒真；golden 确定性护栏（无动画/时钟/随机符号）。
- QUALITY PASS：egui 0.35 API 逐项对照本机源码核实；单 Context 单接线路径；跨侧令牌同族性逐令牌核对（×255 口径 RGB 逐字节一致，刻意分歧均有注释）。
- 控制会话裁决：派生令牌 `ACCENT_WASH`/`CONTROL_WELL` 纳入 design D5 正式令牌表。
- 观察记录（非本 change 缺陷）：Go HUD 令牌自述 linear 但实际经 sRGB 管线原样透传，与 egui 侧严格 gamma 口径存在既有分岔——未来跨侧视觉 parity 时单独裁决。
