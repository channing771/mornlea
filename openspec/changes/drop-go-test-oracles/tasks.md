# 任务：drop-go-test-oracles

## 1. physics 切片

- [ ] 1.1 删除 `internal/physics/motion_helpers_test.go` 整文件；从 `step_native_test.go` 删除两个 `TestStepProductionMatchesGoIntegrationOracle*` 差分测试（保留 `TestStepInputLayoutV2` 与其 helpers）；从 `collision_native_test.go` 删除差分测试与 oracle 断言助手，把以 oracle 计算期望的行为性用例改写为字面量期望（测试函数名与子测试名一律不变）；拆分 `collision_helpers_test.go`——共享 fixture 保留、oracle 专属删除。
- [ ] 1.2 新建位级 golden 向量测试文件 `internal/physics/step_golden_vectors_test.go`：约 8–12 个代表性用例（平地行走/减速停止/跳起/天花板碰撞/水中下沉·上浮·水平阻力/±0 速度哨兵/半砖 step），期望值以 `math.Float32bits` 字面量表达，注释注明采集来源与「跨平台逐位一致」契约；采集后做一次变异自查（翻转一位确认失败，随后还原）。
- [ ] 1.3 验证：`go test ./internal/physics -race -count=1` 全绿；`go vet ./internal/physics`；gofmt 无输出。

## 2. core raycast 切片

- [ ] 2.1 删除 `internal/core/raycast_helpers_test.go` 整文件与 `raycast_fuzz_test.go` 的 `FuzzNativeRaycastMatchesGoOracle`；删除 `raycast_native_test.go` 中消费 oracle 的差分测试（保留布局锁与 cursor batch 行为锁，函数名不变）。
- [ ] 2.2 扩展 `FuzzRaycastBlocks`：命中点在命中格单位立方内（局部坐标 ∈ [0,1]³）、`hit.Face` 法线与归一化方向点积 < 0；两条不变量各配一条确定性单元断言（新用例可入既有主题文件）。
- [ ] 2.3 验证：`go test ./internal/core -race -count=1` 全绿；`go test ./internal/core -run FuzzRaycastBlocks -fuzz FuzzRaycastBlocks -fuzztime 15s` 无失败；`go vet ./internal/core`；gofmt 无输出。

## 3. worldgen 切片 + 基线同步

- [ ] 3.1 删除 `internal/worldgen/oracle_test.go` 整文件；`parity_test.go` 删除差分主体（`assertChunkMatchesOracle`、`TestRandomSeedChunkParity`、`FuzzWorldgenOracleParity`），`TestOakTreeSpansChunkBorderConsistently` 视被测体保留或改写到生产黑盒；`tree_test.go`/`noise_test.go`/`ore_test.go`/`material_test.go`/`fluid_test.go` 按设计 D4 规则逐一处置——被测体是生产黑盒的保留、是 oracle 白盒的删除或改写为 `GenerateChunk` 黑盒用例（保留的函数名与子测试名一律不变）。
- [ ] 3.2 基线句同步：`AGENTS.md` 三句失效 oracle 表述窄 hunk 更新，`CLAUDE.md` 复制为逐字节相同；`openspec/config.yaml` context 同句收敛；修订生产文件中引用 `oracle_test.go` 的注释（`generator.go:7` 等）；progress.md 不动。
- [ ] 3.3 验证：`go test ./internal/worldgen -race -count=1` 全绿；`go test ./internal/archcheck -count=1`（基线文档逐字节一致 + 注释标识符门禁）。

## 4. 收尾门禁

- [ ] 4.1 全量：`make rust`；`go test ./... -race`；`go vet ./...`；`test -z "$(gofmt -l .)"`；`openspec validate --all --strict --no-interactive`。
- [ ] 4.2 ledger 记录全部评审结论与裁决；proposal「延期与放弃」节核对 mesh 切片移交项；更新 tasks 勾选状态。
