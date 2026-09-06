## Context

动机见 proposal.md。现状约束：`captureScenes` 表驱动、全部场景共用一个 application（后场景 MUST NOT 继承前场景状态）；24 数被 `TestCaptureOfficialSceneListStaysAtTwentyFour`、`capture_scene_order_test.go` 顺序测试与主规格 6 处钉死；`oak-grove` 像素断言已证明交叉植物差分方法可行（`oakGroveCellDiff`）；`materials-showcase` 有手工摆拍夹具先例（耕地两列）。

## Goals / Non-Goals

**Goals:**

- 新增 `grass-closeup` 正式场景：手工确定性夹具 + 近景固定机位，短草在画面中达到像素级可辨识（差分 ≥ 150px 的判据复用 oak-grove 方法）。
- 24 → 25 的规格、门禁与 golden 同批更新，既有 24 张 golden 逐字节不变。

**Non-Goals:**

- 不改 worldgen 密度/分布、不改渲染与材质、不放宽双阈值、不碰协议/存档/ABI/benchmark。

## Decisions

- **插入位置 `target-block-feedback` 之后、`oak-grove` 之前**：保住 `oak-grove→ai-companion` 紧随链与全部其它相邻约束（`sword-combat` 链、mining-crack 对、`main-menu/settings-menu` 对、`far-horizon` 倒二、`water-underwater` 居末）；备选插 `materials-showcase` 后被否决——同样满足约束但离植物主题更远，`target-block-feedback` 旁的空地主题（单方块特写）与近景草更同类。
- **手工夹具而非 worldgen 种子夹具**：worldgen 1/4 稀疏命中在近景机位下不可控哪格有草；手工在草地支撑上立 2~3 列短草（`ShortGrassID` 立于 `GrassID` 正上方，满足支撑不变量），位置逐块冻结，`oak-grove` 的自然生成覆盖率不断言不受影响。备选种子夹具被否决——构图依赖哈希命中，换种子即翻图。
- **复用 oak-grove 差分判据**：同一 `oakGroveCellDiff` 方法（阈值与顶/底带划分）对新场景做“至少一株可辨识”断言，阈值取已验证的 150px；备选新写一套判据被否决——重复逻辑且阈值无实测依据。
- **场景名 `grass-closeup`**：kebab-case ASCII，与既有命名同形；golden 文件同名。

## Risks / Trade-offs

- [新场景插入中部，后续场景继承污染] → 每个 Apply 已按纪律显式重置呈现状态；`visual-check` 全 25 景比对会暴露任何泄漏，既有 24 张逐字节不变即证明无污染。
- [手工夹具与自然生成漂移] → 自然路径由 oak-grove 数据面测试覆盖；本场景只锁定外观基线，职责分离。
- [机位选址依赖地形高度] → 夹具全手工（含支撑草地），不依赖 worldgen 高度，高度即常量。

## Migration Plan

- 落地：场景行 + 夹具 + Apply + 数量/顺序测试更新 + `grass-closeup.png` 新 golden（显式更新 + 逐图人工终审）。
- 回退：删除场景行与 golden，恢复 24 数断言与规格；既有 24 张 golden 未动，回退零残留。

## Open Questions

无。
