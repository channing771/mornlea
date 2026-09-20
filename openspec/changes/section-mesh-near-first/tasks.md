# Tasks

## 1. 失败测试先行：新排序契约

- 目标文件：`packages/client/client/mesher_ready_queue_test.go`
- 改钉：距离升序、平局字典序、中心移动后重排、烘焙结果与顺序无关
- 验证：`go test ./packages/client/client -run 'TestReadyQueue|TestMesher' -count=1`（实现前必须失败）

## 2. 复合键实现

- 目标文件：`packages/client/client/mesher_ready_queue.go`、`packages/client/client/mesher.go`、（必要时）`packages/client/cmd/mornlea/app/app_frame.go`
- 复合键、中心变更重建堆、每帧预算语义不变
- 验证：`go test ./packages/client/client -race -count=1`；`go test ./packages/client/cmd/mornlea/app -count=1`

## 3. 回归与收尾门禁

- 世界景子集 `make visual-check` 零漂移；`gofmt`；六模块 `go vet` 或 `make dev-check`；`openspec validate section-mesh-near-first --strict --no-interactive`
