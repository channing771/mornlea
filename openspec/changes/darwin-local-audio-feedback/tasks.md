## 1. OpenSpec 与配置

- [x] 新建 change 产物与本地音频行为契约。
- [x] 为 `audioVolume` 写红测并实现默认值、严格解析、保存往返和未知字段白名单。
- [x] 运行 `make rust`、`go test ./internal/config -race -count=1`、严格 OpenSpec 校验和 diff check。

## 2. Darwin 音频后端

- [x] 为 Darwin 建立预分配 `AudioQueue` 后端和无声降级实现，并验证不分配播放路径。

## 3. 客户端事件接线

- [x] 在四类已确认事件边界各接线一次 cue，并覆盖拒绝、预测和重复状态无声。

## 4. 收尾

- [ ] 运行平台相关测试、`go test ./... -race`、`go vet ./...`、`gofmt -l .` 与 `openspec validate --all --strict --no-interactive`。
