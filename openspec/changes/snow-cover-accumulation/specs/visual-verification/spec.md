# visual-verification Specification

## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与世界呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退。常显 HUD（快捷栏贴条与选中框、状态行图标、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现与权威命中 marker）的 GPU 呈现已退役，无头抓帧路径 MUST NOT 产生这部分像素；它们的呈现验收 SHALL 由 `game-overlay-webview` capability 的前端组件断言与 `frontend/visual` 部件基线承接（本机 Chrome 截图、既有双阈值），MUST NOT 再由 capture golden 承接。GPU 保留面已迁前端同名 `panel-*` fixture（世界图不承载面板），容器面板与 tooltip 的像素验收 SHALL 由前端组件断言与 `frontend/visual` 部件基线承接。世界类场景 golden 中常显 HUD 条带与准星的消失属合法波及，随本 change 经既有显式更新路径重新生成并逐图复核。更新基线时 MUST 继续执行既有显式更新、无窗口完整渲染链路和双阈值规则；不得创建或聚焦前台游戏窗口，不得导入、临摹或复制 Mojang 像素。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木顶面年轮与侧面树皮，以及干耕地与湿耕地各至少一个可见列（含下沉顶面的完整几何）。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行（28 景）：`terrain-noon`、`avatar-nametag`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`grass-closeup`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`passive-herd`、`passive-graze`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`rain-noon`、`camera-third-back`、`camera-third-front`、`snow-cover`、`main-menu`、`settings-menu`、`avatar-detail`、`far-horizon`、`water-underwater`。`hud-hotbar-health`、`hud-survival-feedback` 与 `hud-item-name-popup` 三景随常显层 GPU 呈现退役从清单移除，容器四景已退役并迁前端同名 `panel-*` fixture，清单 MUST NOT 再包含任何只承载常显 HUD 像素或容器面板像素的场景。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保持 `sword-combat`、`hostile-mob`、`water-surface-slope` 的相邻顺序，`mining-crack-early` 与 `mining-crack-heavy` MUST 依次紧随 `water-surface-slope`，`rain-noon`、`camera-third-back` 与 `camera-third-front` MUST 依次紧随 `mining-crack-heavy`，`snow-cover` MUST 紧随 `camera-third-front` 且先于 `main-menu`，`settings-menu` MUST 紧随 `main-menu`，`avatar-detail` MUST 紧随 `settings-menu`，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景。`snow-cover` 场景 MUST 由冬季、雪形态降水与预铺满档雪层的确定性夹具定义：画面 MUST 呈现积雪地表（短方块雪层几何）、雪形降水粒子与冬季冷色天空 tint。所有场景 MUST 使用与交互客户端相同的完整呈现链路收敛后无窗口抓取，且不得创建或聚焦前台游戏窗口。

#### Scenario: 地形与 HUD 风格变化产生可审查基线

- **GIVEN** 既有固定场景与渲染链路可用
- **WHEN** 显式更新本变更影响的视觉基线
- **THEN** `terrain-noon` MUST 包含当前内嵌默认材质及没有内嵌映射 layer 的程序化回退
- **AND** `terrain-noon` 的画面 MUST NOT 出现快捷栏、状态行、氧气、采掘/进食轨道、弹条、准星、聊天或命中 marker 像素
- **AND** 该图 MUST 由无窗口完整渲染链路产出并继续使用既有双阈值

#### Scenario: 常显 HUD 像素退出无头抓帧

- **GIVEN** 常显 HUD 的 GPU 呈现已退役且容器界面关闭
- **WHEN** 抓取任一非菜单相位的固定场景
- **THEN** 画面 MUST NOT 出现任何常显 HUD 像素，与 `survival-hud-presentation`「容器保留面 GPU 资源契约重钉」的关闭态 0 quad/0 glyph 一致
- **AND** 快捷栏、状态行、氧气、采掘/进食轨道、弹条、准星、聊天与 marker 的呈现验收 MUST 由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接

#### Scenario: 完整场景顺序扩展为 28 项

- **GIVEN** 完整无窗口 capture 场景清单
- **WHEN** 检查全部场景名称与顺序
- **THEN** 清单 MUST 恰好包含本 requirement 列出的 28 项，且顺序与之逐项一致
- **AND** 清单 MUST NOT 包含 `hud-hotbar-health`、`hud-survival-feedback` 或 `hud-item-name-popup`
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 雪景场景入册且旧景不变

- **GIVEN** 更新后的官方场景清单
- **WHEN** 全量抓帧比对
- **THEN** `snow-cover` MUST 位于 `camera-third-front` 与 `main-menu` 之间产出新 golden，其余 27 景 MUST 与既有 golden 零差异

#### Scenario: 雪景内容可辨

- **GIVEN** `snow-cover` 场景夹具（冬季、雪形态降水、预铺 4 档雪层）
- **WHEN** 完成收敛并抓帧
- **THEN** 画面 MUST 同时呈现雪白地表（短方块雪层几何）与雪形降水粒子，天空 MUST 带冬季冷色 tint
