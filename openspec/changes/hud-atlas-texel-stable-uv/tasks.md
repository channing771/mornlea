## 1. 属性测试先行（RED）

- [x] 1.1 在 `internal/render/hud/atlas.go` 提取包内私有 `hotbarColumnUV(column, width int) [4]float32`（精确边界计算，行为与现状逐位相同），`hotbarTextureUV` 改为钉死 `hotbarTextureWidth` 的薄包装；签名对外不变。验证：`go test ./internal/render/hud -count=1` 全绿（纯重构零行为变化）。
- [x] 1.2 新建 `internal/render/hud/atlas_uv_stability_test.go`，按 design D2 写三条失败的性质测试：解码界在本列内且距边界 ≥ 1/512 纹素；相邻列区间无重叠；同一列在宽度集 {当前 800, 816, 832(2 的幂倍), 1024, 4096} 下以列内均匀探针解码得到相同纹素下标集合。验证：`go test ./internal/render/hud -run TestHotbarColumnUV -count=1` 确认新性质测试失败（RED）。

## 2. 收进实现（GREEN）

- [x] 2.1 在 `hotbarColumnUV` 内实现对称亚纹素收进 `hotbarUVInsetTexels = 1.0/256.0`（含中文注释钉死噪声上界公式与适用域），使 1.2 的性质测试转绿；既有 `TestHotbarColumnUVStaysInsideItsOwnColumn` 保持通过。验证：`go test ./internal/render/hud -race -count=1`。

## 3. 视觉基线对比（golden 零触碰）

- [x] 3.1 在本 worktree 本地运行 capture 场景 hud-hotbar-health、hud-survival-feedback、inventory-crafting、chest-container、furnace-container、debug-panel，输出与仓库内 golden PNG 逐字节对比并记录结果；若存在字节差，按 design D3 停下升级裁决，不得修改任何 golden。验证：对比记录写入 ledger.md（零差或已升级裁决）。

## 4. 收尾门禁

- [ ] 4.1 运行 `gofmt -l .`（无输出）、`go vet ./...`、`go test ./... -race` 与 `go test ./internal/archcheck -count=1`。验证：全部通过。
- [ ] 4.2 运行 `openspec validate --all --strict --no-interactive` 并把任务勾选状态、评审结论写入 ledger.md。验证：校验通过且 ledger 完整。
