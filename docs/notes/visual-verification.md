# 视觉验证

`--capture <目录>` 让 `mornlea` 走无头 offscreen 路径做像素级回归，本文承接 README「视觉验证」一节，说明固定场景清单、与 golden 基线的双阈值比对，以及更新基线的使用纪律。行为规格见 `openspec/specs/visual-verification/spec.md`，设计细节见[视觉验证设计文档](../superpowers/specs/2026-08-07-visual-verification-design.md)。

## 场景与基线

抓帧依次跑完 `packages/client/cmd/mornlea/capture/capture.go` 里表驱动的 25 个固定场景：`terrain-noon`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`grass-closeup`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。常显 HUD（快捷栏贴条与选中框、状态行、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现与权威命中 marker）的 GPU 呈现已退役，只承载这部分像素的 `hud-hotbar-health`、`hud-survival-feedback` 与 `hud-item-name-popup` 三景随之移出清单，其呈现验收由前端 HUD 组件断言与 `frontend/visual` 部件基线承接；容器四景继续验收 GPU 保留面（容器浮动面板与悬停 tooltip），世界类场景画面中常显 HUD 条带与准星的消失属合法波及。`mining-crack-early` 与 `mining-crack-heavy` 两景呈现世界空间采掘裂纹，依次紧随 `water-surface-slope`。`grass-closeup` 为短草近景基线，紧随 `target-block-feedback` 且先于 `oak-grove`。场景按清单顺序共用同一个 application，`far-horizon` 恒为倒数第二，`water-underwater` 恒为唯一末场景。每张 640×360 PNG 都与仓库根 `testdata/visual-golden/world/` 下的 25 张基线比对；抓帧与 golden 固定使用内嵌默认材质，不随本机配置或用户材质漂移。抓帧模式不能与 `--benchmark` 或 `--connect` 同时启用。
比对是双阈值而非逐字节相等（定义见 `packages/client/cmd/mornlea/capture/visual_compare.go`）：单像素最大通道差与差异像素占比，两项都在阈值内才算通过。当前阈值为最大通道差 `2`、差异像素占比 `0.01%`，取值来自同机重复抓帧的实测漂移分布（见设计文档 §6）。基线缺失时不会静默创建，必须显式请求更新。

```bash
make visual-check              # 抓帧并与基线比对，输出目录默认 build/visual
VISUAL_OUT=/tmp/shots make visual-check   # 自定义输出目录
make visual-update             # 重新生成基线，写入仓库根 testdata/visual-golden/world/
```

比对失败时，实拍图与差异图（差异像素涂红，其余像素按基线压暗）会写进输出目录的 `<场景>-actual.png` 与 `<场景>-diff.png`，供人眼定位问题区域。

## 自然短草的 golden 重归因（24 景基线）

自然短草（`ShortGrassID` 84、材质层 68、worldgen `MGW1` layout 3、engine ABI v10）是纯世界侧变更：生产 worldgen 在合格草地上方空气格确定性生成短草，经既有四 quad alpha-cutout 植物路径呈现。并入 main 的 24 景基线（v21，常显 HUD 退役 + 采掘裂纹）后做了一次显式重归因：24 景中 9 景更新、15 景逐字节不变。更新景与差异证据——`terrain-noon`/`avatar-nametag`/`debug-panel`（同机位共享地形背景，各 10969 px、最大通道差 177，首差异像素同在 (409,129)）、`inventory-crafting` 8062 px、`workbench-crafting` 7893 px、`chest-container` 7960 px、`furnace-container` 8372 px（容器族同背景机位，最大通道差 166）、`oak-grove` 19612 px（自有 3×3 worldgen 夹具，差分夹具测试钉住短草是唯一差异源）、`far-horizon` 24311 px（远视距可见草格最多）。全部差异逐图归因于自然短草这一唯一世界增量；封闭合成场景（`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`mining-crack-early/heavy`、`water-underwater`）不经生产地形 worldgen，逐字节不变；`main-menu`/`settings-menu` 两景沿用分支已含自然短草的归因基线（合并相对其逐字节不变，相对 main 前代则含短草可见变化）。确定性由既有机制保证（世界时间冻结 `SetWorldTimeFrozen`、Rust client 三遍 cull 压缩 `cs_count`/`cs_scan`/`cs_place` 与计数缓冲确定性回填）：重归因后连续两次全新目录完整 `visual-check` 均 24/24 零差异。注意 `terrain-noon` 与 `debug-panel` 的世界底图本就同源（后者只叠加已退役 HUD 之外的调试读数路径），两者差异像素数一致属预期，不是基线复制错误。

## `make visual-update` 的使用纪律

**什么时候该跑**：只在渲染行为**有意**改变、且已经人工打开新产出的 PNG 确认画面正确之后。golden 基线一旦冻错，后续所有比对都在维护一个错误的基线。

**什么时候不该跑**：比对红了但看不出改动位置或原因、又或者只是想让门禁通过。红灯是要查的信号，不是要覆盖的噪声——`packages/client/render`、Rust renderer 或 shader 相关改动的评审必须实际打开差异图，不能只看比对器的数值结论。

视觉验证不接入全量 Go 测试（`make test` / `make test-race`）或 CI（需要 GPU），只作为本地 make target 与人工评审步骤存在。
