## Why

本机客户端缺少不改变权威状态的即时声音反馈；玩家不能仅靠听觉确认常用交互是否成功。

## What Changes

- 为 Darwin 图形客户端加入本地、可静音的四类确认 cue。
- 新增 `audioVolume` 顶层配置，默认 `0.7`，并严格限制在 `0..1`。
- 协议 v26 新增 Play S→C ID 20 `PlaceBlockSucceeded`，只携带发起命令的 `Sequence u64`，使客户端可以将放置 cue 绑到本会话的权威原子成功。
- 设备不可用、无头和非 Darwin 路径降级为无声，不影响游戏启动或模拟。

## Non-Goals

- 不新增通用命令成功框架、存档字段、音频资源包、混音器或第三方依赖。
- 不让声音成为权威状态、输入确认或游戏流程的前提。

## Impact

- 影响 `internal/config`、`internal/network`、`internal/sim`、`internal/server`、Darwin 客户端接线与本地音频实现。协议从 v25 升为 v26 并拒绝 v25 及更早对端；存档、engine/client ABI 和 benchmark scenario 不变。
