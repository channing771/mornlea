# 设计：客户端 UI 对齐原版布局与风格精修

## Context

HUD 布局逻辑全部在 Go 侧 `internal/render/hud/`（固定容量 quad/glyph 字节流，经 client ABI 帧 TLV tag 6 下行），Rust 侧 `hotbar.wgsl` instanced-quad pass 只负责绘制；菜单类是 Rust egui（帧 TLV tag 9，layout v1–v4）。命中测试与绘制共用同一几何函数。现有锚定契约（生命/饥饿锚快捷栏两端、169 design px 状态行、氧气向外堆叠、`hudScale` 统一缩放）经 spec 钉死且与本变更方向一致，全部保留。

品味纪律（用户裁决「混合：像素 × 现代面板」）：单一强调色（琥珀，沿用既有 `hotbarSelectedInnerColor` 色相）、中性深色一族、零 em-dash、形状一致（面板一律直角小圆角语义、槽位一律方凹槽）、颜色无关可辨（几何信息不依赖颜色）。

## Dials

`DESIGN_VARIANCE 3`（HUD 依赖肌肉记忆，锚定关系严格）、`MOTION 1`（静态 quad 管线 + golden 确定性，无动画系统可用；弹条以硬切出现/消失替代淡出）、`DENSITY 5`（游戏 HUD 紧凑但分层清晰）。

## D1 风格令牌（唯一样式来源）

新增 `internal/render/hud/style.go` 集中全部颜色令牌（既有散落色值迁入并统一）：

| 令牌 | 值（linear RGBA） | 用途 |
|---|---|---|
| `panelShadow` | (0.008, 0.010, 0.014, 0.90) | 面板投影层（外扩 2 design px） |
| `panelSurface` | (0.045, 0.052, 0.062, 0.84) | 面板表面（半透明，透出世界） |
| `panelBorderLight` | (0.92, 0.93, 0.95, 0.16) | 1 design px 亮边 |
| `slotWell` | (0.020, 0.024, 0.030, 0.92) | 槽位凹槽 |
| `slotWellEdge` | (1, 1, 1, 0.07) | 凹槽上沿内高光 1 design px |
| `accentAmber` | (1, 0.72, 0.24, 0.98) | 唯一强调色：选中格内衬、进度填充、产物格轮廓 |
| `textPrimaryFg` | (0.96, 0.96, 0.97, 1) / 阴影 (0, 0, 0, 0.85) | 全部 HUD 文字双层 |
| `crosshairShadow` / `crosshairFg` | (0, 0, 0, 0.55) / (0.96, 0.96, 0.96, 0.92) | 准星双层 |
| 语义色保留 | 生命红系、饥饿棕系、氧气青白系、耐久绿→红、采掘可采绿/不可采琥珀橙、来源高亮青（「待移动来源」状态色，几何区分：外扩整格描边） | 图标与状态语义色不变相 |

规则：强调色只允许出现在「选中、进度、产物」三类语义；警告与错误沿用既有红/橙语义色；禁止新增第二种强调色相。`panelBorderLight` 亮边只适用于容器浮动面板与 egui 面板；底部快捷栏贴条保持「投影+表面」双层结构（无边），与原版热栏的贴条形态一致——这也保住关闭态最坏恰 100 的预算钉。Rust 侧对应新增 `engine/crates/mornlea_client/src/ui/style.rs`（egui 令牌，见 D5），两侧令牌表并排写进本文件，互不引用（无 ABI 通道）。

## D2 生存 HUD 布局对齐 + 准星 + 物品名弹条

- **底部收紧**：`hotbarBottomMargin` 24→6（选中格 3px 外扩描边仍在界内）；`statusBarGap`、行距 20（`healthHeartSize + statusBarGap`）与全部锚定关系不变；`closedHUDHeight` 相应缩减并新增弹条行（见下）。
- **准星**：viewport 几何中心锚定，十字由横 11×3、竖 3×11 design px 两臂组成；先画深色投影（偏移 +1,+1）再画前景，共 4 quad；进 glyph 流为零。HUD 可见即绘制（容器打开时被浮动面板覆盖属预期呈现层叠）；菜单相位 `hudVisible=false` 自然不画。实例追加顺序：准星最先（保证面板后画覆盖）。
- **物品名弹条**：触发 = 已确认镜像选中下标变化（比较上一确认值）；文本 = `core.ItemDisplayName`（D7）；锚点 = 快捷栏水平居中、底边 = 采掘/进食轨道上沿 + 6 design px（与进度条共存不遮挡）；持续 = 确认变化所属权威 tick 起 40 tick；抑制 = 容器打开或菜单相位；零 quad、纯 glyph 双层。应用层在 `app_frame.go` 组装 `PopupOverlay{Text, ShownAtTick, WorldTick}`，可见性由 `(WorldTick - ShownAtTick) < 40` 判定（tick 驱动，golden 可确定）。
- **高度预算**：`closedHUDHeight = 6 + 48 + 2×20 + 16 + 12 + 22`（末项 = 弹条行 6+16），弹条纳入缩放高度防裁剪。

## D3 资源契约重钉（scenario v19→v20）

新增 quad 来源：准星 4（关闭+打开）、浮动面板描边 4（打开）、tooltip 背景 2（打开）。最坏组合重算并以固定预算测试钉住：

- 关闭最坏：96 + 4 = **100**
- 打开最坏（T3 实算钉值）：**264** = 准星 4 + 面板族 7 + 选中 1 + 来源高亮 1 + 36 格 + 72 色块 + 18 耐久 + 箱子内容 81 + 状态栈 40 + 聊天 2 + tooltip 背景 2（基线 257 + 面板族净增 3 + tooltip 2；以 `openWorstQuads` 命名常量固定预算测试钉住）
- 容量：`maxHotbarQuads` 267→**320**；quad 区 256 + 320×48 = 15616 = glyph offset（256 对齐保持）
- glyph：`maxHotbarGlyphs` 700→**768**（现有最坏 + 弹条 ≤64（32 rune × 阴影/前景双层）+ tooltip ≤16 后留余量；关闭态实测最坏 548；实现须以测试断言实测最坏并记录）；总容量 15616 + 768×48 = **52480**
- benchmark scenario v19→v20：scenario 常量、测试断言、README 双语版本矩阵、`docs/notes/compatibility.md`、`docs/architecture.md`（如有提及）、`internal/render/hud/layout.go` 注释同步。协议/schema/ABI/配置版本全部不动。

## D4 容器原版式浮动面板

- **几何单源**：新增 `panelGeometry(width, height, viewport) → origin`，垂直居中于「顶边到打开态底部状态栈上沿」的剩余空间、水平居中；全部四类面板与 `InventorySlotAt`/`ChestSlotAt`/`FurnaceSlotAt`/`CraftingSlotAt`/`RecipeButtonAt` 从同一组面板矩形推导，禁止第二份坐标常量。
- **面板构成**（自上而下）：标题行（atlas 标题 cell，1 quad）→ 容器图示区 → 玩家背包 3×9 → 12 px 行距 → 快捷栏行。面板宽 = 快捷栏行宽（464）+ 2×面板内边距；个人背包面板额外加右侧配方栏宽。面板 quad：投影 + 表面 + 四边 1px 亮边 + 标题 = 7。
- **个人背包**：左上 2×2 权威网格 + 箭头 + 产物格（原版位置）；右侧竖排十条固定配方入口（紧凑行高，保留 `RecipeButtonAt` 全部下标与两次点击语义）——对齐原版配方册的刻意分歧，记录于 gameplay/limitations。
- **熔炉**：原版图式：输入栏（上）— 火焰 cell（中，自下向上裁剪）— 燃料栏（下）构成左列；箭头 cell（自左向右裁剪）指向右侧输出栏；三格下标 36/37/38 不变。
- **工作台/箱子**：3×3 网格 + 箭头 + 产物格 / 27 格箱格，分别置于图示区。
- **tooltip**：容器打开且指针悬停非空栏位（含产物格）时，指针右下侧呈现名称（投影+表面 2 quad + glyph 双层），越界时翻转到指针左上；空格/面板外/未打开零实例。指针位置沿用容器打开时的既有鼠标坐标源。
- **状态栈避让**：打开态主状态行+氧气仍在快捷栏下方（既有行为），面板位于其上方空间，互不相交由几何测试穷举断言。

## D5 egui 菜单换肤

新增 `engine/crates/mornlea_client/src/ui/style.rs`：`PANEL_FILL rgba(0.045,0.052,0.062,0.92)`、`PANEL_STROKE rgba(1,1,1,0.12)`、`TEXT_PRIMARY (0.94,0.95,0.96)`、`TEXT_SECONDARY (0.62,0.65,0.70)`、`ACCENT_AMBER (1,0.72,0.24)`、`DANGER (0.94,0.35,0.35)`、按钮圆角 2、无阴影。应用面：

- 主菜单：底色加深为 (0.055,0.062,0.075)；按钮 = 半透明面板 + 1px 亮边 + 悬停/焦点琥珀描边；标题加大字重（尺寸可调，布局关系不变）；版本行用 `TEXT_SECONDARY`。
- 暂停/设置：遮罩保留，面板与控件套用令牌；错误行沿用 `DANGER`。
- F3 调试面板：面板令牌 + 标签 `TEXT_SECONDARY`/值 `TEXT_PRIMARY`，选中行琥珀左缘标记，编辑态琥珀描边。
- **不动**：wire layout v1–v4、动作常量、事件编码、依赖版本线、pass 顺序（egui 仍在 HUD 后）、全部行为 spec 语义。Rust 侧 render/abi 测试同步令牌断言（既有像素/颜色断言更新）。

## D6 图标精修（就地重绘）

心/半心/空心、气泡、鸡腿、火焰、箭头、凹槽、容器标题 cell 的像素 mask 在 `atlas.go`/各 hud 文件就地重绘：轮廓更干净（去杂点、1px 描边语义统一）、内部明暗两层，剪影保持可辨识且与既有剪影同族（构图对照官方参考仅作证据）。不新增图集列、不改列界、不动 UV 收进算法（「HUD 图集列采样稳定性」要求原样保持）。语义色族不变（D1 表）。

## D7 物品中文名来源

新增 `internal/core/item_name.go`：`ItemDisplayName(id ItemID) (string, bool)`——放置方块的物品回退 `BlockDisplayName(对应方块)`；非方块物品（工具/食物/床等）用显式中文表覆盖至 `ItemIDMax`，测试断言全量覆盖无缺名。客户端弹条与 tooltip 同源消费；不新增 wire 字段。

## D8 视觉验证

- 场景表 23→24：`hud-item-name-popup` 插入 `hud-survival-feedback` 之后、`avatar-nametag` 之前；夹具 = 确认选中指向已知名物品 + 固定 worldTick 处于 40 tick 窗口内；场景结束恢复夹具。
- 全部 24 张 golden 显式更新（`make visual-update`）并逐图人工复核（准星、弹条、面板、tooltip、菜单新样式逐一确认）；双阈值与更新守卫（far-horizon LOD control、场景顺序测试）全部保持。
- 受影响断言同步：场景顺序测试、`TestMainMenuCaptureScenePosition`、`TestWaterUnderwaterCaptureSceneIsLast` 等常量表。

## D9 护栏（验收时逐条核对）

1. 不新增 HUD shader、GPU pass、上传格式、API、ABI、配置项或依赖；egui 依赖线与 pass 顺序不动。
2. 协议、schema、metadata、engine/client ABI、配置版本、存档全部不变；唯一版本动量是 benchmark scenario v19→v20。
3. 弹条/tooltip 只消费确认镜像与本地输入，零预测、零新协议。
4. 命中语义不变：栏位下标集合、两次点击整堆移动、产物取出、Esc 栈、容器打开输入抑制、`RecipeButtonAt` 十条。
5. 全部像素原创程序化；Mojang 零像素；官方参考只作构图证据。
6. saturation-jitter、数量阴影、耐久条、采掘/进食互斥、满氧零实例等既有可观察语义逐一保持（既有测试全绿）。
7. 弹条硬切出现/消失（无动画）；golden 确定性不受 wall-clock 影响（tick 驱动）。
8. 中文注释、无任务编号入注释、标识符反引号等工程纪律照常。

## 非目标

- 护甲条、经验条/等级（无对应游戏机制，待机制落地后另行对齐）。
- 弹条/tooltip 淡入淡出动画、容器拖拽拆分搬运（B-35 范围）。
- 材质包 HUD 覆盖（D-06）、第三人称（D-09）。
- 主菜单世界背景 panorama。
