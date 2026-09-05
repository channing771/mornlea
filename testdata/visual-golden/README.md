# 视觉基线索引

本目录统一存放视觉回归基线，均为测试夹具二进制。

- `world/`：无窗口世界场景基线 24 张 PNG，对应 `cmd/mornlea/capture/capture.go` 的 `captureScenes`。
- `passive-death/`：被动牛 GIF 动态基线 4 个，对应 `cmd/mornlea/capture/passive_death_scripts.go` 的 `passiveDeathGIFScripts`（按 tick 步进抓帧，标准库 `image/gif` 编码，逐帧解码沿用双阈值比对）。
- `ui/`：前端 UI 部件基线 30 张，对应 `packages/engine/crates/mornlea_client/frontend/visual/fixture-names.ts` 的 `fixtureNames`。

旧目录 `cmd/mornlea/capture/testdata/golden/` 与 `engine/crates/mornlea_client/frontend/visual/golden/` 已清空，仅剩空目录，不再写入。

## world（24 张）

文件名即场景名加 `.png` 后缀，场景定义与顺序以 `captureScenes` 为准。

| 基线文件 | 场景名 | 说明 |
|---|---|---|
| `terrain-noon.png` | `terrain-noon` | 正午固定光照下的出生点地形远眺，锁定昼夜管线最亮相位的基准画面。 |
| `avatar-nametag.png` | `avatar-nametag` | 远端玩家与其双语昵称名牌在正午世界中的透视与字形呈现。 |
| `debug-panel.png` | `debug-panel` | 面板可见态下的高空世界底图，见证无头路径不引入额外面板像素。 |
| `skylight-tunnel.png` | `skylight-tunnel` | 天窗竖井内的日光下探与井壁明暗过渡。 |
| `block-light-room.png` | `block-light-room` | 午夜封闭石室的纯方块光衰减基线。 |
| `torch-night.png` | `torch-night` | 午夜石室内落地与壁挂火把的暖光照明与火苗精灵。 |
| `bed-night.png` | `bed-night` | 午夜石室内四朝向完整床的床面亮带与半高轮廓。 |
| `materials-showcase.png` | `materials-showcase` | 陈列台上全材质方块在正午日光下的质感对照。 |
| `target-block-feedback.png` | `target-block-feedback` | 相机正前方命中块的高亮描边与定位反馈。 |
| `grass-closeup.png` | `grass-closeup` | 近景草地条上短草列的交叉面片与贴地形态，短草外观的可辨识基线。 |
| `oak-grove.png` | `oak-grove` | 橡树群落的树冠、树干与林下地表的组合呈现。 |
| `ai-companion.png` | `ai-companion` | AI 伙伴在正午世界中的跟随站位与其呈现状态。 |
| `sword-combat.png` | `sword-combat` | 持剑攻击姿态与权威命中标记同帧的战斗反馈。 |
| `hostile-mob.png` | `hostile-mob` | 午夜草地火把亮池边缘夜行者群的站位与受击追逐态。 |
| `passive-herd.png` | `passive-herd` | 正午草地上 3 头贴图牛与 1 个纹理生牛肉掉落的站位与掉落相位。 |
| `passive-graze.png` | `passive-graze` | 正午草地上低头牛与常态牛的位姿对照，及牛吻部身前由草变泥土的一格。 |
| `water-surface-slope.png` | `water-surface-slope` | 俯视水池的水面高度斜坡与透水可见的池底材质。 |
| `mining-crack-early.png` | `mining-crack-early` | 同一目标砖块上的浅阶段世界空间采掘裂纹。 |
| `mining-crack-heavy.png` | `mining-crack-heavy` | 同一目标砖块上的最重阶段裂纹，与浅阶段对照判读加深。 |
| `main-menu.png` | `main-menu` | 主菜单相位全景底图，固定自转时刻的纯全景世界画面。 |
| `settings-menu.png` | `settings-menu` | 设置相位全景底图，同一全景世界的另一自转时刻。 |
| `avatar-detail.png` | `avatar-detail` | 原创旅人正面、侧面和背面同框，验收服装材质与静态轮廓。 |
| `far-horizon.png` | `far-horizon` | 高空远眺的近景地形、远环壳带、雾过渡与天空四段构图。 |
| `water-underwater.png` | `water-underwater` | 眼睛浸没的水下视角，水色叠加与穿水衰减同框。 |

## ui（30 张）

文件名即 fixture 名加 `.png` 后缀，清单以 `fixture-names.ts` 的 `fixtureNames` 为准，对应组件以 `fixtures.tsx` 的注册表为准。

| 基线文件 | fixture 名 | 对应组件 |
|---|---|---|
| `panel-inventory.png` | `panel-inventory` | 个人背包与2×2合法石砖配方。 |
| `panel-inventory-empty.png` | `panel-inventory-empty` | 空背包。 |
| `panel-inventory-full.png` | `panel-inventory-full` | 按物品堆叠上限填满背包、工具合法单件与半耐久。 |
| `items-all.png` | `items-all` | 生产注册表全物品图标与名称。 |
| `hud-hotbar-first.png` | `hud-hotbar-first` | HUD首格选中及邻格抬升。 |
| `hud-hotbar-last.png` | `hud-hotbar-last` | HUD末格选中及邻格抬升。 |
| `panel-workbench.png` | `panel-workbench` | 3×3合法石锄材料与产物。 |
| `panel-chest.png` | `panel-chest` | 箱子及统一背包。 |
| `panel-furnace.png` | `panel-furnace` | 熔炉三格与熔炼/燃烧进度。 |
| `panel-character.png` | `panel-character` | 只读人物页。 |
| `panel-inventory-narrow.png` | `panel-inventory-narrow` | 小窗口滚动背包。 |
| `panel-main-menu.png` | `panel-main-menu` | `MainMenu` 主菜单整屏。 |
| `panel-settings.png` | `panel-settings` | `SettingsPanel` 设置整屏。 |
| `panel-pause.png` | `panel-pause` | `PauseMenu` 暂停整屏。 |
| `panel-loading.png` | `panel-loading` | `LoadingScreen` 加载整屏。 |
| `panel-debug.png` | `panel-debug` | `DebugPanel` 完整行集合。 |
| `button-default.png` | `button-default` | `PixelButton` 可用态主菜单按钮。 |
| `button-disabled.png` | `button-disabled` | `PixelButton` 禁用态主菜单按钮。 |
| `button-pressed.png` | `button-pressed` | `PixelButton` 选中态窗口预设按钮。 |
| `input-text.png` | `input-text` | `PixelInput` 材质包路径只读输入。 |
| `preset-group.png` | `preset-group` | `PixelButton` 三按钮窗口预设组。 |
| `slider.png` | `slider` | `input.settings-slider` 音量滑块。 |
| `debug-rows.png` | `debug-rows` | `DebugPanel` 四行最小集合。 |
| `error-line.png` | `error-line` | `p.menu-error` 错误行。 |
| `hud-hotbar.png` | `hud-hotbar` | `HudRoot` 仅快捷栏。 |
| `hud-status.png` | `hud-status` | `HudRoot` 完整状态栈（生命、饥饿、氧气与进食轨道）。 |
| `hud-progress.png` | `hud-progress` | `ProgressTrack` 进食进度轨道。 |
| `hud-popup-crosshair.png` | `hud-popup-crosshair` | `HudRoot` 弹条、准星与命中标记同框。 |
| `hud-chat.png` | `hud-chat` | `HudRoot` 多行聊天。 |
| `hud-container-open.png` | `hud-container-open` | `HudRoot` 容器打开态翻转构图。 |

## motion（4 个）

motion 演示产物只验呈现、不进比对：`make visual-check` 与 `--update-golden`
都不感知本目录，`world/` 的 24 张 PNG 纪律也不含它。

| 演示文件 | 场景 | 帧数/时长 | 生成入口 |
|---|---|---|---|
| `break-burst.gif` | `break-burst-motion`：完整采掘生命周期——F0–4 目标静置，F5–24 采掘爬坡（裂纹 0→9 扫完），F25 破坏同帧（目标置空 + 采掘熄灭 + 泥土掉落注入），F25–44 粒子存续 + 掉落下落（3 格落差重力积分约 9 tick，F34 着陆），F34–49 掉落静置留存 | 50 帧，每帧 0.13 秒，循环约 6.5 秒 | `go run ./packages/client/cmd/mornlea --motion-demo testdata/visual-golden/motion/break-burst.gif`（仓库根运行） |

| `avatar-walk.gif` | 静止→慢走→快走→停稳；慢走40 tick与快走20 tick各走4.3格 | 100帧，20Hz，5秒 | 同上加 `--motion-scene avatar-walk`，输出改为 `avatar-walk.gif` |
| `drop-scatter.gif` | 触发前空场→四堆正式帧出生→散开下落→着陆 | 80帧，20Hz，4秒 | 同上加 `--motion-scene drop-scatter`，输出改为 `drop-scatter.gif` |
| `drop-density.gif` | 空场→1→4→9→16→32→移除一半至16堆→稳态 | 160帧，20Hz，8秒 | 同上加 `--motion-scene drop-density`，输出改为 `drop-density.gif` |

- 新演示的原始关键PNG写在输出路径加 `-frames/` 的旁路审查目录，不纳入golden；GIF是完整过程，PNG只帮助核对编码保真。
- 演示场景值住 `packages/client/cmd/mornlea/capture/motion_break_burst.go` 与 `motion_experience.go`，不追加进 `captureScenes`。
- 编码只用标准库 `image/gif`（全片共享自适应调色板，无抖色），固定输入逐字节一致。

## passive-death（4 个）

文件名即剧本名加 `.gif` 后缀，剧本定义以 `passiveDeathGIFScripts` 为准；单基线帧预算 ≤48（8fps×6s），比对时解码逐帧沿用双阈值，全部帧通过方为通过。

| 基线文件 | 剧本名 | 说明 |
|---|---|---|
| `graze.gif` | `graze` | 吃草前后：常态站立 6 帧接低头吃草 6 帧，常态牛对照。 |
| `lure.gif` | `lure` | 持麦靠近：远端玩家逐帧靠近静立牛，小麦掉落置于牛身前。 |
| `kill.gif` | `kill` | 击杀：第 4 帧死亡 despawn 并刷出生牛肉掉落，随后 20 帧红闪侧倒保留期。 |
| `beef-drop.gif` | `beef-drop` | 牛肉掉落：单个生牛肉掉落的浮动与旋转（权威 tick 派生）。 |

## 三类边界与选用规则

三大家族按“渲染源 × 时间维度”路由，文件名与数量以各注册表为准（本文不复制清单）：

- 第一类 UI 窗口型（`ui/`）：窗口与 WebView 层的部件级 PNG。清单以 `fixture-names.ts` 的 `fixtureNames` 为准；本机 Chrome 截图、既有双阈值比对，不进 CI。
- 第二类世界静态（`world/`）：无头离屏渲染收敛后的单帧稳定态 PNG。场景定义与顺序以 `captureScenes` 为准；`make visual-check` 比对、`make visual-update` 显式覆盖。
- 第三类过程 GIF（`motion/` + `passive-death/`）：跨 tick 状态迁移的全流程 GIF，按 tick 步进抓帧，覆盖触发前、结算、收敛全过程，不得只截片段。内分两小类：
  - `motion/` 演示：只验呈现、不进任何比对门禁，供人眼审查全流程；
  - `passive-death/` 门禁：逐帧沿用双阈值比对，全部帧通过方为通过，单基线帧预算有界。

路由纪律：窗口 chrome 只进 `ui/`，世界单帧只进 `world/`，时间过程只进 GIF；世界帧不得携带窗口 chrome 像素，UI 夹具不得复刻世界像素。

人物 `avatar-detail` PNG只钉正侧背材质与静态比例，`avatar-walk` GIF只审查距离驱动的步态与停稳，职责不同。四个旧GPU容器场景已迁移到前端同名 `panel-*` fixtures，世界图不承载面板。

去重纪律：同一行为禁止 PNG + GIF 双存。已并存且职责不同（门禁采样点与全流程人工审查物，如裂纹双帧与采掘演示）必须在本节注明理由；新增重叠由所属玩法 change 按本节收敛，本节只立规则不动既有基线。

## 更新入口与纪律

- 世界基线比对入口为 `make visual-check`（含 GIF 剧本），覆盖入口为 `make visual-update`，路径常量为 `capture/capture_image.go` 的 `captureGoldenDir` 与 `capture/passive_death_gif.go` 的 `passiveDeathGoldenDir`。
- 部件基线比对入口为 `make frontend-visual-check`（或在 `frontend/` 内执行 `corepack pnpm visual-check`），覆盖入口为 `make frontend-visual-update`（或 `corepack pnpm visual-update`），基线目录由 `visual/visual.mjs` 的 `goldenDir` 经 `repoRoot` 推导。
- 先目检后覆盖：基线只在预期视觉变化已逐图人工确认后更新，普通验证只比较不自动接受差异；漂移先看实拍图与差异图定位，再决定修代码还是更新基线；基线缺失不静默创建，必须显式请求更新。
- 比对口径为双阈值，定义与取值以源码为准（`capture/visual_compare.go` 与 `visual/visual.mjs` 的比对函数），本文档不复制具体数值。
