# Tasks: saturation-jitter

## 1. 模拟层
- [ ] `internal/sim/player.go` 增 `saturationZero bool` 瞬态位，`resetHunger`/`applyExhaustion` 出口与 `advanceEating` 结算后统一 `saturationZero = saturationMilli==0`
- [ ] `internal/sim/hunger.go` 注释固定表（不改阈值），单测覆盖零/非零边界与阈值循环后仍正确定格

## 2. 网络层
- [ ] `internal/network/packet.go` `ProtocolVersion 28→29` + 冻结注释
- [ ] `message_player_state.go`/`codec_client.go` 尾部 bool 编解码（`Sprinting` 后），`codec_golden` 与冻结单测更新

## 3. 客户端/呈现
- [ ] `internal/client/mirror.go`/`predictor.go` 透传 `SaturationZero`（不预测，仅镜像权威值）
- [ ] `internal/render/hud` 或 `cmd/mornlea` 饥饿条抖动分支（`SaturationZero==true` 时加偏移，false 时零改动）

## 4. 验证
- [ ] `go test ./internal/sim -run TestSaturationZero -count=1`、`go test ./internal/network -run TestPlayerState -count=1`
- [ ] `gofmt -l .`、`go vet ./...`、`go test ./... -short -race`、`openspec validate --all --strict`
