# Proposal: fluid-audio-cue

## 背景

规划行 B-32（fluid-presentation proposal 显式非目标「不做流体音效」的清偿）。`local-audio-feedback` 能力当前在五类边界播放本地 cue（UI click、采掘完成、放置成功、进食完成、伤害确认），玩家入水没有任何音频反馈。既有 cue 纪律（只在权威确认边界触发、无声降级、总音量 `audioVolume`）已稳定运行，本变更是该能力表的第六行。

## 目标

- 玩家身体入水时（权威确认位置上 `BodyInFluid` 由 false→true 的上升沿）播放恰好一次水花 cue。
- 复用既有 `cueSpec` 方波合成器、`localAudioFeedback` 基线模式与确认边界纪律，零新增合成与播放管线。

## 非目标

- 出水音、持续水中环境音、水下滤波等 DSP 效果（范围冻结，另行认领）。
- 音量分级或按 cue 类型独立音量配置。
- 任何协议、存档 schema、engine/client ABI 或视觉 golden 变更。
- 服务端下发浸没标志 wire 位：标志是已确认位置与世界镜像的纯函数，客户端可自行导出。

## 用户可观察结果

- 走进水中听到一次短促下滑水花声；持续浸泡、出水、再入水各自符合边沿语义；传送或重生不产生幻响。
- `audioVolume=0` 或设备不可用时完全无声，行为与既有 cue 一致。

## 受影响的包或文档

- `internal/audio/cue.go`：新增 `CueWaterSplash` 与一条 `cueSpecs` 条目。
- `cmd/mornlea/app_audio.go`：`localAudioFeedback` 新增浸没基线位与上升沿检测。
- `cmd/mornlea/app_messages.go`：权威 `PlayerState` 应用点单点接线。
- 对应测试文件；归档时同步 `openspec/specs/local-audio-feedback/spec.md`、基线文档与 backlog 回填。

## 兼容性

无协议、存档、ABI 变更。capture/benchmark 与无图形专用服务端不请求音频设备（`playCue=nil`），行为逐位不变。回退即 revert 本 change 分支。
