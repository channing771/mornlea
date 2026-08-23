## 1. OpenSpec 与配置

- [x] 新建 change 产物与本地音频行为契约。
- [x] 为 `audioVolume` 写红测并实现默认值、严格解析、保存往返和未知字段白名单。
- [x] 运行 `make rust`、`go test ./internal/config -race -count=1`、严格 OpenSpec 校验和 diff check。

## 2. Darwin 音频后端

- [x] 为 Darwin 建立预分配 `AudioQueue` 后端和无声降级实现，并验证不分配播放路径。

## 3. 客户端事件接线

- [x] 在四类已确认事件边界各接线一次 cue，并覆盖拒绝、预测和重复状态无声。

## 4. v26 放置成功确认

- [x] 在 `internal/network` 为 Play S→C ID 20 `PlaceBlockSucceeded{Sequence uint64}` 写 codec/registry/golden/fuzz/非法与截断载荷红测，保持 ID 21 未分配，将协议升为 v26 并验证 v25 及更早握手被拒绝。
- [x] 在 `internal/sim` 写红测后为每个成功玩家放置输出一个 `(Session, Sequence)` 结果；拒绝不输出，同 tick/跨 tick 连续放置不丢序号。
- [x] 在 `internal/server` 写 Memory/TCP 红测后只向发起会话发布成功应答，拒绝只发 `CommandRejected`，发布失败沿用既有 outbox 关闭规则。
- [x] 在 `cmd/mornlea` 写红测后删除放置 delta+库存 matcher，仅以 reset 清空的最高已消费 sequence 触发一次 cue；重复/旧序号、拒绝与无关状态无声。
- [x] 同步 AGENTS.md/CLAUDE.md 的协议 v26 基线并保持逐字节相同；运行 `make rust`、受影响包 race 测试、`go test ./internal/archcheck -count=1`、`go vet ./...`、`gofmt -l .`、严格 OpenSpec 校验与 `git diff --check`。

## 4A. Task 4 fix round 4

- [ ] 安全重放完整音频栈到 melee PR #66 修复 HEAD `82eb03b`，保存旧 HEAD 备份，并以 `git range-diff` 核对 11 个提交内容等价。
- [ ] 为 active→inactive/rejected→无关移除旧目标无声写 RED，并在成功增量优先顺序下保留正常采掘完成 cue。
- [ ] 为同一 `PlayerState` 同时确认进食与伤害写 RED，以零分配定长结果独立返回并各播放一次。
- [ ] 在 delta spec、design、plan 中补齐有效本地 UI click 与空白/禁用/发送失败静音契约，不删除既有行为。
- [ ] 运行 focused RED/GREEN、四包组合 race、archcheck、vet、`gofmt -l .`、59 项严格 OpenSpec、`git diff --check` 与 AGENTS/CLAUDE 逐字节一致检查。

## 5. 收尾

- [ ] 运行平台相关测试、`go test ./... -race`、`go vet ./...`、`gofmt -l .` 与 `openspec validate --all --strict --no-interactive`。
