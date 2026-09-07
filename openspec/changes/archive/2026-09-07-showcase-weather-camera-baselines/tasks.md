## 1. 雨天与视角场景

- [x] 1.1 `rain-noon` 场景构造与 headless 天气注入（`packages/client/cmd/mornlea/capture`；先失败测试再实现；`go test ./packages/client/cmd/mornlea/capture -race -count=1`）
- [x] 1.2 `camera-third-back` 与 `camera-third-front` 场景构造（同包；同上验证命令）
- [x] 1.3 场景清单扩展 24→27 与顺序测试同步（同包；`go test ./packages/client/cmd/mornlea/capture -race -count=1`，`far-horizon` 倒数第二与 `water-underwater` 末尾不断言破裂）

## 2. 天气过程演示

- [x] 2.1 `weather-cycle` motion 演示入口（压缩 tick、96 帧上限、确定编码；`go test ./packages/client/cmd/mornlea/capture -race -count=1`，连跑两次逐字节一致）

## 3. 基线入库与门禁

- [x] 3.1 新基线目检入库（`make visual-update` 显式生成，逐图人工复核 3 PNG + 1 GIF 后入库；`make visual-check` 27/27 全绿；既有 24 张字节不变）
- [x] 3.2 收尾门禁（`gofmt -l`、`make dev-check`、`openspec validate --all --strict --no-interactive`；benchmark 数值只记录）
