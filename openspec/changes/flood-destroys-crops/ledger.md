# flood-destroys-crops Ledger

> 记录执行进度、评审结论与全部裁决（Ruling）。格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。

## 认领与确认（阶段 0–1）

- 2026-08-25：B-07 由 ox-alpha-implementer 认领（backlog 提交 `755b2228`），分支 `feat/B-07-flood-destroys-crops`。
- 内容确认：bounded 路径，短设计经需求方显式批准。裁决记录：
  - `Ruling: 掉落槽满时拒绝破坏并重试 — 对齐「背包无空位不破坏」纪律与数据丢失门禁 — 否决照常破坏丢产物与有界重试（上限无依据、单格重试成本有界无害）`

## 阶段 2

- worktree `.worktrees/feat/B-07-flood-destroys-crops` 自 main `bcc900fb` 创建；基线 `make rust` + `go test ./internal/fluid ./internal/sim -short -count=1` 全绿。
- change 四产物 + 本 ledger 已建；独占文件集见 backlog B-07 行（不触碰 A-01/A-04/B-10 独占文件）。

## 执行（阶段 3）

- 2026-08-26 分支变基到当时 main 尖端 `e57dc60c`（原 BASE bcc900fb 之后主线推进），下列哈希为变基后可达值
- 组1（任务 1.x）：DONE_WITH_CONCERNS @ `55a0a715`；评审 SPEC ✅ / QUALITY 1 Important + 2 Minor；Important 为注释反引号纪律，修复于 `dbd5be00` 后评审 clean，任务收尾 `02067d29..dbd5be00`。
- 组2（任务 2.x）：DONE_WITH_CONCERNS @ `a1435c46`+`05ab4b3d`（parity 落点 server 新主题文件，brief 许可）；评审 SPEC ✅ / QUALITY Approved，0 Important / 3 Minor。裁决：TDD 红证据采信报告文字（红绿单提交）；D2 单次提交论据由行为级测试与既有 property 背书，控制会话确认。
- 组3（任务 3.1）：@ `9770b1fc`（仅 fluid.go 注释）；评审 SPEC ✅ / QUALITY Approved / Findings None。
- 组4 门禁：gofmt / vet / archcheck / OpenSpec strict / make rust 全 PASS。全量 `-race` 唯一失败为 cmd/mornlea GPU 场景测试 10m 超时——Ruling: 满负载 flake（focused 重跑 111s 过、整包重跑 812s 过），按 test-quickstart 分诊协议不进修复循环。
- Ruling: 既有 flaky `TestMemoryTCPFluidDamBreakBroadcastParity` 不在本 change 处理 — 干净 BASE 上间歇失败且夹具无作物，与本变更语义无关，文件属 E-11（server 测试等待预算）独占域 — 终审后向需求方上报。

## 终审与收尾（阶段 4–5）

- 变基：分支变基到当时 main 尖端 `e57dc60c`（propose 提交 `02067d29` 的父；原 BASE bcc900fb 之后主线推进）。分支点后主线落地 B-10/B-05：`wheatSeedDropCount` 删除、成熟收获数量改为 `cropYieldRolls`(worldSeed, tick, 维度, 坐标) 确定性哈希——本分支 `settleFloodedCrop` 仍镜像旧固定表，编译失败且语义失配。终审 findings 一波清偿：
  - I-1：`settleFloodedCrop` 成熟分支改调包内共享 `cropYieldRolls`（tick 取值点 `Engine.tick.Load()` 与 `completeMining` 同一读取路径）；sim 侧确值断言改为按夹具已知 `(seed, 结算 tick, 维度, position)` 现算（`stepUntilFluidCropFlooded` 改为返回结算 tick）；spec 场景改「与玩家采掘该作物的掉落表完全相同（含其数量语义）」级表述；design.md D4 同步。
  - M-1：新增 `TestFluidCropSameTickDualSourceMergesToStrongestAndSettlesOnce`——等级 1 垂直候选 × 等级 2 水平候选同批写同一成熟作物格，断言最终值取最强者、作物格全程恰好一笔广播变更、掉落恰好一批且与 `cropYieldRolls` 现算值逐件相等。红证据：临时变异 `internal/fluid/queue.go` 合并分支为「先到先得」后用例 3/3 确定性红（弱候选先落笔冲毁、强候选次批改写 ⇒ 两笔广播），还原后绿；变异仅本地演示，未进入任何提交。
  - T1：`internal/fluid/rules.go` 作物分支注释把「存活判定、同 tick 冲突合并」误列入「全部经由谓词」——收窄为承重路径（垂直/水平写入判定 + `fluidSourceIsFixedPoint` / `fluidSectionIsFixedPoint` 两个 rescan 捷径）；design.md D1 同句同改。
  - server parity 连带适配：B-10 后产量按绝对 tick 取哈希，两宿主就绪耗时差（实测 memory=69 / tcp=54 tick）使结算绝对 tick 分岔、逐件 DeepEqual 假红——开录前两侧空转对齐到共同绝对 tick `floodCropParityAlignTicks=256`（就绪超限显式 Fatal 提示上调预算，不静默重试），DeepEqual 严格性不放宽；另按 trample 先例补结构守卫（两类各一堆、数量 ∈ [1,3]）。
  - 五条 deferred minors triage：①rules.go 注释枚举失真→本波次修复（见 T1）；②`internal/fluid/helpers_test.go:218` Fatalf 消息未同步作物例外前提→文件在 fluid 独占域（本波次仅豁免 rules.go 一处注释），deferred 随 fluid 包后续维护上报；③组2 报告跨文件标识符清单漏报 ClearDrop/WheatStage3ID→纯报告完整性瑕疵、无代码影响，关闭；④server parity 用例单跑压 -short 档线→随重型测试管理统一处置，deferred；⑤容量重试用例断言队列非空而非目标格精确排程→由收敛性质兜底，接受现状。
  - M-3/M-4：ledger 回填（本节）、tasks.md 复选框全数核对勾选（4.2 注记满负载 flake 处置）。
- 终审波次验证：`go build ./...` 通过；`go test ./internal/fluid ./internal/sim -race -count=1` 全绿；`go test ./internal/server -race -run 'Parity|FluidCrop' -count=1` 全绿；`gofmt -l .` 无输出；`go vet ./...` 通过；`openspec validate --all --strict --no-interactive` 通过。rebase 触及 cmd/mornlea 两个 golden PNG（hud-hotbar-health / hud-survival-feedback，来自主线侧），按要求补跑 `go test ./cmd/mornlea -race -run 'Capture|Golden' -count=1 -timeout 20m` 通过（摘要见 task-final-fix-report）。
