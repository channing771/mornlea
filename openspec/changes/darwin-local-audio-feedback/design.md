## Context

音频只消费客户端已经确认的本地事件，绝不参与权威模拟决策。既有世界增量与库存镜像不携放置命令序号，无法在多命令/多玩家交错时同时保证“成功恰好一次”和“无关/拒绝绝不误响”。因此 v26 只新增一个放置成功确认边界；任何音频初始化或播放失败仍必须退化为无声。

## Decisions

- Darwin 实现使用单一 `AudioQueue`、8 个预分配 buffer、22050 Hz mono signed-int16 PCM；初始化后与每次播放均不得分配。
- 四个 cue 分别对应：成功采掘完成、成功放置完成、成功进食完成、收到伤害确认。只有对应确认边界发生时才播放；被拒绝、预测或重复状态不得播放。
- v26 在 Play S→C ID 20 新增 `PlaceBlockSucceeded{Sequence uint64}`，载荷恰为 8-byte little-endian u64，ID 21 保持未分配。模拟只在 `executePlacement` 已原子写方块并恰减一件后将 `(Session, Sequence)` 追加到当 tick 有界结果；server 只向该 session 发布。拒绝仍只有 `CommandRejected`。
- 客户端删除放置目标增量+库存减量 matcher，只保存本会话最高已消费成功序号；首次更大序号播放，重复/旧序号无声，reset/close 清零。不新建队列、map、timeout、retry 或通用命令成功框架。
- `audioVolume` 是 `Config` 的独立顶层 `float32`，默认 `0.7`，显式值必须为闭区间 `0..1`；它不进入 `Fields()` 或调试面板。
- 无头、非 Darwin 和设备初始化失败返回无声实现；客户端继续启动，模拟、网络和渲染行为不变。

## Ownership, Dependencies, and Concurrency

- Darwin 本地音频组件独占 `AudioQueue`、8 个 buffer 与其 cue 数据；`cmd/mornlea` 在图形客户端生命周期内创建和关闭它，配置只在创建时提供总音量。客户端事件接线只决定是否提交 cue，不拥有队列或 buffer。
- 依赖方向只能从 `cmd/mornlea` 或客户端接线流向本地音频组件；`sim` 只产生放置成功事实，`server` 将它映射为 `network` 应答，三者均不依赖音频。无声实现保持同一调用入口且不触碰设备，但客户端仍必须正常消费和去重成功应答。
- cue 播放调用发生在客户端确认事件的串行处理线程；AudioQueue 回调在系统实时播放线程中只复用预分配 buffer 并重新入队，不读取客户端状态、配置或网络数据，也不分配或加锁。
- 预计改动 `internal/config`、`internal/network`、`internal/sim`、`internal/server`、Darwin/非 Darwin 本地音频文件、`cmd/mornlea` 初始化/关闭和客户端确认事件接线及其测试。存档、engine/client ABI 与 benchmark scenario 不改。

## Risks and Verification

设备 API 受宿主环境影响，故以无声降级保护启动路径，并用配置边界测试和平台隔离构建测试验证。协议 v26 与 v25 不兼容，握手阶段直接拒绝旧对端；Memory/TCP codec 一致性、golden/fuzz/截断载荷和连续放置集成测试锁定新边界。回退时可整体撤销 v26 应答与客户端消费点；存档无迁移。
