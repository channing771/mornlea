# Design: fluid-audio-cue

## 数据所有权与依赖方向

- 浸没基线（`hasBodyInFluid`/`bodyInFluid` 两个标量）归 `cmd/mornlea/localAudioFeedback` 独有，与既有 health/hunger 基线同模式；不进预测器、不进权威状态、不持久化。
- 求值发生在权威 `PlayerState` 应用点（`app_messages.go` 消息循环，单 goroutine），无新增并发边界。
- `physics.SubmersionFlags` 是权威/预测共用的唯一浸没实现（`fluid-presentation-survival` 立规）；本变更只新增一个调用点，不得出现第二套判定。
- 依赖全部为既有边：`cmd/mornlea` 已 import `internal/audio`、`internal/physics`、`internal/client`（`MirrorCollisionSource` 构造先例就在同文件）；archcheck 白名单无变化。

## Decision 1：求值点在消息循环而非预测器

在 `app_messages.go` 应用状态处就地调用 `physics.SubmersionFlags(state.Position, client.MirrorCollisionSource{...})`。被否决的替代方案：让 predictor 在 reconcile 时存储 bodyInFluid 并暴露 getter——它复用同一次求值，但要把新公共 API 加进 `internal/client`，扩大独占文件集并引入第二个浸没标志读取者；收益仅是省掉一条冷路径上的纯函数求值（每条状态消息数次方块查表）。cue 纪律要求确认边界，predictor 的值还混有预测帧率路径语义，边界更模糊。

## Decision 2：边沿检测收在 `localAudioFeedback`

上升沿判定 `wasDry && nowWet` 与基线更新和 health/hunger 完全同构；首观测（基线缺席）一律不算上升沿——与采掘 cue「松键后旧目标作废」的保守语义一致。`Reset()` 已有的整体清零覆盖传送、重生与会话重建。

## Decision 3：合成器零新增

水花用既有方波下滑音 + 线性衰减包络表达：`{samples: 2000, startHz: 800, endHz: 220, amplitude: 11000}`（约 91 ms，比伤害/采掘亮、比 UI click 长）。被否决：为水花写噪声/滤波合成器——新增合成代码与测试面，违反最小 diff；音色微调属实现自由度，不进规格。

## 受影响文件

- `internal/audio/cue.go`：常量 + 一行 cueSpecs 条目。
- `cmd/mornlea/app_audio.go`：两个字段、参数、返回位与映射。
- `cmd/mornlea/app_messages.go`：一次求值 + 一个 cue 分支。
- 对应测试文件按「一文件一关注点」落位。

## 兼容与回退

无 wire/schema/golden 变更；capture/benchmark/专服路径 `playCue=nil` 逐位不变。回退即 revert 分支。

## 验证

- RED：六条边沿性质测试先行失败。
- GREEN 后：`go test ./internal/audio ./cmd/mornlea -race -count=1`、`make test-race-changed`、`gofmt -l .`、`go vet ./...`、`openspec validate --all --strict --no-interactive`。
