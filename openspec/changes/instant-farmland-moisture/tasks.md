## 1. 测试组织与执行账本

- [ ] 1.1 控制会话在 `openspec/changes/instant-farmland-moisture/ledger.md` 初始化任务、implementer、规格评审、质量评审、修复轮次与 ruling 表；每次派发和裁决即时记录。
- [ ] 1.2 在 `internal/sim` 记录 `go test ./internal/sim -list .` 的测试函数集合，把多主题 `crop_test.go` 按抽样/生长/湿润/成本关注点拆分，复用 helper 按仓库单一中心规则落位；再次运行 `go test ./internal/sim -list .` 并按集合语义确认测试名与子测试名零变化。
- [ ] 1.3 运行 `go test ./internal/sim -race -count=1`，确认测试重组零行为变化后再进入功能实现。

## 2. 有界候选队列与恢复重扫

- [ ] 2.1 在 `internal/sim/farmland_moisture_queue_test.go` 先写失败测试，覆盖反向 `9×9×2` 枚举的 `y,z,x` 顺序、世界 Y 边界、FIFO 去重、消费压紧和离开 active Ready scope 后丢弃；运行 `go test ./internal/sim -run 'TestFarmlandMoisture(ReverseWindow|Queue)' -count=1` 验证失败。
- [ ] 2.2 在 `internal/sim/farmland_moisture.go` 与 `engine.go` 实现 `farmlandMoistureState`、固定 `65,536` 读取预算、候选入队/处理和聚合状态字段；运行上一步定点测试至通过。
- [ ] 2.3 在 `internal/sim/farmland_moisture_budget_test.go` 先写失败测试，覆盖目标读取、162 次最坏邻域查询、余额不足保留队首、单 tick 不越预算、大于预算的待办最终排空及相同输入逐 tick 结果一致；实现最小处理逻辑并运行 `go test ./internal/sim -run 'TestFarmlandMoisture(Budget|Determin)' -race -count=1`。
- [ ] 2.4 在 `internal/sim/farmland_moisture_rescan_test.go` 先写失败测试，覆盖完整高度 `24×24` halo 的 `y,z,x` 游标、事件优先、跨 tick 续扫、离开 scope 丢弃、重新进入从头扫描以及重扫候选只接受 active Ready 耕地；实现独立重扫状态并运行 `go test ./internal/sim -run 'TestFarmlandMoistureRescan' -race -count=1`。

## 3. 生产触发点与 tick 阶段

- [ ] 3.1 在 `internal/sim/farmland_moisture_integration_test.go` 先写失败测试，通过真实 `fluidWorld.SetBlock` 覆盖非流体↔流体触发、流体等级↔流体等级不触发、距离 4/5、同层/上一层/下一层及跨区块边界；运行 `go test ./internal/sim -run 'TestFarmlandMoistureFluid' -count=1` 验证失败。
- [ ] 3.2 修改 `internal/sim/fluid.go`，在 `fluidWorld.SetBlock` 的真实写入路径按 old/new `core.IsFluid` membership 生产候选，并在 `advanceFluids` 识别新 scope key 时登记独立湿度重扫；运行 3.1 定点测试至通过，同时运行 `go test ./internal/sim -run 'TestFluid' -race -count=1` 防止流体回归。
- [ ] 3.3 扩展 `internal/sim/farming_test.go` 的成功/拒绝对照，先证明范围内有水的成功翻地未在同 tick 发布湿耕地；修改 `farming.go` 仅在真实成功写入后入队目标，运行 `go test ./internal/sim -run 'TestTill' -race -count=1` 至通过。
- [ ] 3.4 在 `internal/sim/companion_action_test.go` 增加 `phaseFarmlandMoistureAdvance` 顺序期望，在重启/重入集成测试中覆盖陈旧湿度恢复；修改 `engine_step.go` 令阶段固定为 fluid→moisture→crop，并在 `fluid_perf_test.go` 把流体净耗时结束边界改为新阶段。运行 `go test ./internal/sim -run 'TestStepPhaseOrder|TestFarmlandMoisture(Restart|Reentry)' -race -count=1`。

## 4. 作物阶段解耦与成本门禁

- [ ] 4.1 在拆分后的作物成本测试中先增加 `cropBlockReads <= 2×cropCellsExamined` 和全耕地 `cropBlockReads == cropCellsExamined` 断言，验证当前随机耕地扫描使测试失败。
- [ ] 4.2 修改 `internal/sim/crop.go`，删除 `advanceCropCell` 的耕地分支与随机湿润扫描，只保留作物样本和正下方读取，并维护 `cropBlockReads`；对 `tunables.go` 的 `RandomTicksPerSection` 现有农业注释做最小准确性更正，不改字段、默认值或校验。运行 `go test ./internal/sim -run 'TestCrop|TestFarmland' -race -count=1`。
- [ ] 4.3 更新 `internal/sim/crop_perf_test.go`，让全部场景报告 `block_reads/op`，把全耕地场景改为“一样本一次读取”的回归证据；更新 `fluid_perf_test.go` 同时记录 fluid/moisture/crop 阶段边界，不放宽任何 correctness 门禁。
- [ ] 4.4 运行 `go test ./internal/sim -run '^$' -bench 'Benchmark(CropAdvance|CropAdvanceAllFarmland|Fluid)' -benchmem -count=5`，记录性能数值并确认 `cells/op`、`block_reads/op`、真实 overflow 与报告完整性均有效。

## 5. 验证与评审收尾

- [ ] 5.1 对每个任务使用全新 implementer 子代理；每项完成后由独立 reviewer 分别裁决规格合规与代码质量，发现进入最多 5 轮修复循环，并把结论与 ruling 写入 `ledger.md`。
- [ ] 5.2 运行 `gofmt -w` 仅格式化本 change 修改的 Go 文件，再运行 `gofmt -l .` 并确认无输出。
- [ ] 5.3 依次运行 `make rust`、`go test ./internal/sim -race -count=1`、`go test ./internal/archcheck -count=1`、`go test ./... -race` 与 `go vet ./...`；任何失败修复根因，不放宽门禁。
- [ ] 5.4 运行 `openspec validate --all --strict --no-interactive`，核对协议 v26、区块 schema v9、玩家 schema v7、世界 metadata v2、`companions.ai` schema v4、ABI、benchmark scenario 与 capture golden 均未变化。
- [ ] 5.5 派发独立整分支终审，核对 proposal、delta spec、design、实现、测试和 `tasks.md` 一致；把终审结论写入 `ledger.md`，仅在全部验证有新鲜证据后勾选完成项。
