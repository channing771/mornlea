# Execution Ledger

## Baseline

- 基线提交：`6fd03011`（本 worktree `split-network-subpackages`，HEAD
  `6fd03011cda15b369c76617612a53ac1d690b6a2`，工作区除本 change 产物外
  干净）。
- 基线快照：`go test ./internal/network/... -list '.*'` 输出（剔除空行与
  ok 行，条目按原始输出逐行保留，以 `#` 分节注释标注包边界与 Test/
  Benchmark/Fuzz 分组）持久化于本 change 目录 `baseline-test-list.txt`，
  计数 **根包 164（Test 151 + Benchmark 7 + Fuzz 6）+ tcp 33（Test 33）
  = 197**，三类逐项核对一致；后续任务以「剥离 `#` 行后的逐名并集」与
  基线比对。
- 计时基线（同基线 SHA、本 worktree 实测，2026-08-28，darwin/arm64，
  go1.26.0）：
  - race：`go test ./internal/network -race -count=1` →
    `ok ... 2.797s`（首次实测）；复跑确认 `ok ... 2.629s`。均 PASS。
  - 非 race：`go test ./internal/network -count=1` → `ok ... 1.212s`。
  - 后续任务以本 ledger 记录的实测值为对照基准，计时只记录不设门槛。
- 文件计数基线：根包 **53 个 Go 文件**（实测 23 生产 5273 行 + 30 测试
  7476 行）；执行计划 Why 节写的「51 个 Go 文件」与其自身 23+30 分解矛盾，
  系算术笔误，以本实测 53 为准（分解数字 23/30 与行数约值与计划一致）。
  `tcp/` 6 文件不动。
- fixture 基线：`internal/network/testdata/chunk-snapshot-v1.bin` 1 个，
  随 `chunk_codec_test.go` 迁至 `internal/network/codec/testdata`
  （Task 3），实测全仓无其他引用。
- 消费面基线：9 个包引用根包——`internal/server` 127 符号 2033 处、
  `cmd/mornlea/app` 616、`internal/client` 411、`cmd/mornlea/capture`
  120、`cmd/mornlea/benchmark` 39、`cmd/mornlea` 36、`cmd/mornlea-server`
  34、`internal/sim` 测试 5 处，加 `internal/network/tcp` 生产代码 400+
  处（执行计划先行实测口径）；拆分期间消费方与 tcp 生产源码零改动。
- archcheck 基线（`internal/archcheck/dependency_test.go`）：
  `"internal/network": {"internal/companion", "internal/core"}`、
  `"internal/network/tcp": {"internal/network"}`。

## Rulings

- Ruling（preflight-1，控制器裁决）：Task 2 根包测试引用已迁移 unexported
  符号时，可就地 `protocol.` 限定或按被测主体提前随迁——两种处理均合规，
  由 implementer 按耦合事实选择；测试函数名与 `t.Run` 标签逐名保留是硬
  约束。
- Ruling（preflight-2，控制器裁决）：archcheck allowed 表按任务增量登记
  ——Task 2 登记 protocol 边并移除根包 internal/companion 边；Task 3 登记
  codec 边；Task 4 只做文档与 CI 门禁，不改 archcheck 表。
- Ruling：基线文件计数以实测 53（23 生产 + 30 测试）为准，执行计划
  「51 文件」系笔误——计划自身的 23+30 分解与行数实测一致；按「不强迫
  数字吻合、记录实测」纪律处理，proposal 采用实测值。
- 本 change 尚无实现期裁决；Task 2–4 的实施裁决、临时导出清单与回收记录
  随各任务评审追加于本节之后。
