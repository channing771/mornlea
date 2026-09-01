# 视觉验证

`--capture <目录>` 让 `mornlea` 走无头 offscreen 路径做像素级回归，本文承接 README「视觉验证」一节，说明固定场景清单、与 golden 基线的双阈值比对，以及更新基线的使用纪律。行为规格见 `openspec/specs/visual-verification/spec.md`，设计细节见[视觉验证设计文档](../superpowers/specs/2026-08-07-visual-verification-design.md)。

## 场景与基线

抓帧依次跑完 `cmd/mornlea/capture/capture.go` 里表驱动的 24 个固定场景：`terrain-noon`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。常显 HUD（快捷栏贴条与选中框、状态行、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现与权威命中 marker）的 GPU 呈现已退役，只承载这部分像素的 `hud-hotbar-health`、`hud-survival-feedback` 与 `hud-item-name-popup` 三景随之移出清单，其呈现验收由前端 HUD 组件断言与 `frontend/visual` 部件基线承接；容器四景继续验收 GPU 保留面（容器浮动面板与悬停 tooltip），世界类场景画面中常显 HUD 条带与准星的消失属合法波及。`mining-crack-early` 与 `mining-crack-heavy` 两景呈现世界空间采掘裂纹，依次紧随 `water-surface-slope`。场景按清单顺序共用同一个 application，`far-horizon` 恒为倒数第二，`water-underwater` 恒为唯一末场景。每张 640×360 PNG 都与 `cmd/mornlea/capture/testdata/golden/` 下的 24 张基线比对；抓帧与 golden 固定使用内嵌默认材质，不随本机配置或用户材质漂移。抓帧模式不能与 `--benchmark` 或 `--connect` 同时启用。
比对是双阈值而非逐字节相等（定义见 `cmd/mornlea/capture/visual_compare.go`）：单像素最大通道差与差异像素占比，两项都在阈值内才算通过。当前阈值为最大通道差 `2`、差异像素占比 `0.01%`，取值来自同机重复抓帧的实测漂移分布（见设计文档 §6）。基线缺失时不会静默创建，必须显式请求更新。

```bash
make visual-check              # 抓帧并与基线比对，输出目录默认 build/visual
VISUAL_OUT=/tmp/shots make visual-check   # 自定义输出目录
make visual-update             # 重新生成基线，写入 cmd/mornlea/capture/testdata/golden/
```

比对失败时，实拍图与差异图（差异像素涂红，其余像素按基线压暗）会写进输出目录的 `<场景>-actual.png` 与 `<场景>-diff.png`，供人眼定位问题区域。

## `make visual-update` 的使用纪律

**什么时候该跑**：只在渲染行为**有意**改变、且已经人工打开新产出的 PNG 确认画面正确之后。golden 基线一旦冻错，后续所有比对都在维护一个错误的基线。

**什么时候不该跑**：比对红了但看不出改动位置或原因、又或者只是想让门禁通过。红灯是要查的信号，不是要覆盖的噪声——`internal/render`、Rust renderer 或 shader 相关改动的评审必须实际打开差异图，不能只看比对器的数值结论。

视觉验证不接入 `go test ./...` 或 CI（需要 GPU），只作为本地 make target 与人工评审步骤存在。
