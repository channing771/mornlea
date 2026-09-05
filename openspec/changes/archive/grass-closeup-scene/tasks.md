## 1. 场景夹具与注册

- [x] 1.1 在 `packages/client/cmd/mornlea/capture/capture_scene.go` 新增 `prepareGrassCloseup`（空气邻域基线 + 草地支撑条 + 2~3 列手工短草）与 `applyGrassCloseupCaptureState`（固定正午、固定近景机位、清空继承状态），在 `packages/client/cmd/mornlea/capture/capture.go` 的 `captureScenes` 中 `target-block-feedback` 之后、`oak-grove` 之前插入 `grass-closeup` 行
- [x] 1.2 新增 `packages/client/cmd/mornlea/capture/capture_grass_closeup_test.go`：夹具数据面断言（短草列全立于草地正上方）与场景内像素断言（复用差分法，至少一株差分 ≥ 150px、上缘透空、贴地）；验证 `go test ./packages/client/cmd/mornlea/capture -run TestGrassCloseup -count=1 -v`

## 2. 数量与顺序门禁更新

- [x] 2.1 更新 `capture_oak_grove_grass_test.go` 的 `TestCaptureOfficialSceneListStaysAtTwentyFour` 与 `capture_scene_order_test.go` 的 24 数/顺序断言至 25（含 `grass-closeup` 紧随 `target-block-feedback` 先于 `oak-grove`）；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1`

> 收尾注（2026-09-06）：实现早已合入 main，后续 cream 等变更将清单演进为 24 景（容器四景退役、`avatar-detail` 加入），`grass-closeup` 位置保持 `target-block-feedback` 之后、`oak-grove` 之前；数量断言以当前 `captureScenes` 真值为准，全包 race 已绿。

## 3. 基线生成与复核

- [x] 3.1 显式生成 `testdata/visual-golden/world/grass-closeup.png`（更新模式抓帧），人工逐图终审短草可辨识后接受；验证 `make visual-check` 全 25 景通过且既有 24 张 golden 逐字节不变（`git status` 只新增一张 PNG）
- [x] 3.2 收尾：`gofmt`、`go vet ./packages/client/...`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`

> 收尾注（2026-09-06）：`grass-closeup.png` 已在 golden 目录，`visual-check` 路由为 24 world PNG；收尾门禁（gofmt/vet/audit 全绿、validate 96 全绿）已在同步后基线上重跑通过。
