# flood-destroys-crops 任务

## 1. fluid 规则变更

- [x] 1.1 `internal/fluid`：先写失败测试——`Replaceable` 对 `core.IsCrop` 八个阶段返回 true、对耕地/泥土等非作物实心块仍为 false（新测试文件 `rules_replaceable_crop_test.go`，或按关注点并入既有 `replaceable_test.go` 主题）；验证命令 `go test ./internal/fluid -run Replaceable -count=1`
- [x] 1.2 `internal/fluid/rules.go`：`Replaceable` 增加作物分支并更新判定表注释（含「作物可被流动水替换，冲毁结算由权威写入侧负责」的职责边界说明）；验证命令 `go test ./internal/fluid -count=1`
- [x] 1.3 `internal/fluid`：既有四个 `property_{rescan,budget,order,converge}_test.go` 与 `e2e_test.go` 全绿；若 property 夹具可自然扩展，补一条「含作物流场收敛」用例；验证命令 `go test ./internal/fluid -count=1`

## 2. sim 冲毁结算

- [x] 2.1 `internal/sim`：先写失败测试（新文件 `fluid_crop_test.go`）——垂直水流冲毁成熟作物产出与采掘同表的确定性双产物（数量按 `(seed, 结算 tick, 维度, position)` 现算）、未成熟产出 1 种子；水平传播冲毁同表；掉落槽满时本 tick 方块不变且下一 delay 重试、槽位释放后完成冲毁；单 tick 原子性（无半掉落）；同 tick 双源冲突合并取最强者且恰好结算一次；耕地保持且满足湿判定；验证命令 `go test ./internal/sim -run FluidCrop -count=1`
- [x] 2.2 `internal/sim/fluid_crop.go`（新建）：实现作物冲毁结算并在 `fluid.go` 的 `fluidWorld.SetBlock` 挂钩；中文注释说明 D2/D3/D4 决策与重试语义；验证命令 `go test ./internal/sim -run FluidCrop -count=1`
- [x] 2.3 `internal/sim`：Memory/TCP parity 用例——同一次溃坝冲毁农田在双传输下方块变更与掉落物一致；验证命令 `go test ./internal/sim -run 'Parity|FluidCrop' -count=1`

## 3. 论证文本同步

- [x] 3.1 `internal/sim/fluid.go`：更新 `fluidWorld` 边界注释与 rescan 相关注释中「任意其他非空气方块（实心）不可替换」的表述为含作物分支的新判定表；确认 `fluidSourceIsFixedPoint` / 区段级快路径零代码改动即可正确排除邻作物的水源；验证命令 `go test ./internal/sim ./internal/fluid -count=1`
- [x] 3.2 核查既有 capture 场景无农田临水构图、golden 预期逐字节不变（只核查记录，不改 golden）；如发现意外漂移，停下查根因并在 ledger 记录

## 4. 收尾门禁

- [x] 4.1 `gofmt -l .` 无输出、`go vet ./...` 通过
- [x] 4.2 `go test ./... -race` 全绿（cmd/mornlea GPU 场景满负载 flake 已按 test-quickstart 分诊协议单独复跑通过：focused 111s、整包 812s，见 ledger 执行节）
- [x] 4.3 `openspec validate --all --strict --no-interactive` 通过
- [x] 4.4 `scripts/agents/gates.sh` 全绿；性能数值若有变化仅记录
