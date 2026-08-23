## Context

音频只消费客户端已经确认的本地事件，绝不参与权威模拟或网络传输。任何初始化或播放失败都必须退化为无声。

## Decisions

- Darwin 实现使用单一 `AudioQueue`、8 个预分配 buffer、22050 Hz mono signed-int16 PCM；初始化后与每次播放均不得分配。
- 四个 cue 分别对应：成功采掘完成、成功放置完成、成功进食完成、收到伤害确认。只有对应确认边界发生时才播放；被拒绝、预测或重复状态不得播放。
- `audioVolume` 是 `Config` 的独立顶层 `float32`，默认 `0.7`，显式值必须为闭区间 `0..1`；它不进入 `Fields()` 或调试面板。
- 无头、非 Darwin 和设备初始化失败返回无声实现；客户端继续启动，模拟、网络和渲染行为不变。

## Ownership, Dependencies, and Concurrency

- Darwin 本地音频组件独占 `AudioQueue`、8 个 buffer 与其 cue 数据；`cmd/mornlea` 在图形客户端生命周期内创建和关闭它，配置只在创建时提供总音量。客户端事件接线只决定是否提交 cue，不拥有队列或 buffer。
- 依赖方向只能从 `cmd/mornlea` 或客户端接线流向本地音频组件；该组件不得让 `sim`、`network` 或权威状态反向依赖音频。无声实现保持同一调用入口且不触碰设备。
- cue 播放调用发生在客户端确认事件的串行处理线程；AudioQueue 回调在系统实时播放线程中只复用预分配 buffer 并重新入队，不读取客户端状态、配置或网络数据，也不分配或加锁。
- 预计改动 `internal/config/config.go` 与测试、Darwin/非 Darwin 本地音频文件、`cmd/mornlea` 初始化/关闭和客户端确认事件接线及其测试；不改 `sim`、`network`、协议或存档。

## Risks and Verification

设备 API 受宿主环境影响，故以无声降级保护启动路径，并用配置边界测试和平台隔离构建测试验证。无协议、存档或迁移影响。
