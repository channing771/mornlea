# Tasks: fluid-audio-cue

## 1. Task A（RED）：边沿性质失败测试

- [x] 1.1 在 `cmd/mornlea` 为 `localAudioFeedback` 的入水边沿新增测试文件，覆盖六条性质：上升沿恰响一次、持续浸没静默、出水静默、出水后再入水重新触发、`Reset` 清基线后首次在水中不触发、未就绪/Reset 路径重置基线。断言只用字面量布尔返回值，不引用尚不存在的 `CueWaterSplash`。验证：`go test ./cmd/mornlea -run 'WaterSplash' -count=1` 失败且为编译外行为缺失。
- [x] 1.2 运行 `gofmt -l .` 与 `go vet ./cmd/mornlea ./internal/audio`。

## 2. Task B（GREEN）：最小实现

- [x] 2.1 `internal/audio/cue.go` 新增 `CueWaterSplash` 常量与 `cueSpecs` 条目；补一条合成性质测试（非零、末样本衰减到零附近）。
- [x] 2.2 `cmd/mornlea/app_audio.go`：`ObservePlayerState` 增加 `bodyInFluid bool` 参数与 splash 返回位，按设计 Decision 2 实现边沿检测；既有调用点与测试同步。
- [x] 2.3 `cmd/mornlea/app_messages.go`：应用状态处以 `client.MirrorCollisionSource` 就地求值并接线 cue 分支。
- [x] 2.4 验证：`go test ./internal/audio ./cmd/mornlea -race -count=1` 全绿；`make test-race-changed RACE_BASE=origin/main`。

## 收尾（控制会话）

- [x] 3.1 `gofmt -l .` 无输出、`go vet ./...`、`go test ./internal/archcheck -count=1`。
- [x] 3.2 `openspec validate --all --strict --no-interactive`。
- [x] 3.3 整分支门禁 `GOFLAGS=-timeout=30m scripts/agents/gates.sh`。
