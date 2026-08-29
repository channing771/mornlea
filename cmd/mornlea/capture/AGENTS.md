# capture 包：视觉 golden 契约

`cmd/mornlea/capture` 实现无窗口视觉回归：按固定场景表驱动离屏 application
抓帧，与入库 golden 基线做双阈值比对。行为规格见
`openspec/specs/visual-verification/spec.md`，使用纪律见
`docs/notes/visual-verification.md`。本包只依赖 app，不依赖 benchmark
（方向由 `TestClientCommandSubpackageDependencyDirections` 强制）。

## golden 纪律 (`capture/capture_image.go`, `capture/visual_compare.go`)

- golden 只在预期视觉变化已逐图人工确认后更新；普通验证只比较，不自动接受
  差异。基线一旦冻错，后续所有比对都在维护错误的基线。
- golden 目录是仓库根相对路径常量 `captureGoldenDir`
  （`cmd/mornlea/capture/testdata/golden`）；引用它一律经常量，不在别处复制
  路径字符串。
- 比对是双阈值而非逐字节相等：单像素最大通道差与差异像素占比，定义与取值以
  `visual_compare.go` 及其性质测试为准；不要在文档复制会漂移的数字。基线缺失
  时不会静默创建，必须显式请求更新。
- 分辨率与消息预算取自 app 导出常量（`CaptureWidth`/`CaptureHeight`、
  `MessageDrainMax`），刻意小于 benchmark 的 2560×1440，不在本包复制数值。
- 抓帧与 golden 固定使用内嵌默认材质：忽略用户材质覆盖、不创建交互窗口、
  不请求音频设备（main 装配层对 capture 路径强制回落 `config.Defaults()`）。

## 场景表 (`capture/capture.go`)

- 场景清单是表驱动的 `captureScenes`，新增场景即新增一行；全部场景按表序
  共用同一个 application，前一场景留下的呈现状态由后续场景的 `Prepare`/
  `Apply` 负责清场；`resetCapturePresentation` 负责清 `combatFeedback` 与相关呈现，避免污染后续场景。
- 场景顺序与 24 项正式清单以 `captureScenes` 及其顺序测试为准，固定上传容量以布局代码和容量
  测试为准；不要在指南复制会漂移的清单或数字。

## 消费端接口 (`capture/scene_application.go`)

- `SceneApplication` 是 capture 对宿主应用状态的唯一访问面：场景表的
  `Prepare`/`Apply`/`PinVolatile` 闭包与抓帧管线只经接口方法读写，不感知
  `app.Application` 具体类型（`*Application` 隐式实现）；战斗场景仅通过 `ArmCombatMarker`、`ResetCombatFeedback`、`CombatMarkerVisible` 三个最小消费方法与 `Application` 的 `combatFeedback` 交互。
- 方法集以 capture 的实际引用为准：不为对称性添加无人消费的方法，也不把
  app 内部字段经接口扩散。`Panel`/`ChatInput` 的返回类型经 app 导出别名
  `PanelState`/`ChatInput` 表达，具体结构体保持非导出。

## helper 中心与回归测试 (`capture/capture_test_helpers_test.go`)

- 共享夹具（场景表名称查找、AI 伙伴确定性呈现状态的构造与断言）住
  `capture_test_helpers_test.go`；白盒构造下沉为 app 导出的测试装配入口
  `application.NewPresentationApplicationForTest`，不在本包复制装配。
- 钉死回归的测试入口：
  - `TestCaptureSceneOrderAndAICompanionDeterminism`（场景完整顺序与
    ai-companion 夹具确定性）；
  - `TestTorchNightCaptureScenePosition`（torch-night 紧随 block-light-room）、
    `TestWaterUnderwaterCaptureSceneIsLast`（water-underwater 恒末位）；
  - `visual_compare_test.go` 的比对性质测试（`TestCompareImagesIdentical`、
    `TestDiffPixelRatioGate` 等）；
  - `TestCaptureSettled`（场景抓帧前的 settled 判据）。

## Focused Verification

- 定点测试：`go test ./cmd/mornlea/capture -race -count=1`（golden 比对为
  CPU 操作，秒级）。
- 无窗口视觉门禁：`make visual-check`（全部场景通过 tracked golden，不产生
  `*-actual.png`/`*-diff.png`）；更新基线用 `make visual-update`，更新后必须
  重跑 `make visual-check`。
