# E-04 `drop-go-test-oracles` 执行账本

- 认领基线：`88094977`（`chore/E-04-drop-go-oracles`，worktree `.worktrees/E-04-drop-go-oracles`）。
- 分类裁决：**bounded**——纯测试基建删除 + 基线句窄同步，无生产行为变化、无新子系统；用户 2026-08-25 批准短设计（含两项决策：physics 位级 golden 向量采纳、基线句随本 change 同步采纳）。
- 勘察修正：认领备注「`generator.go` 内失去消费者的 test-only 旧实现」不成立——旧 worldgen 实现早已只存在于 `oracle_test.go` 自包含副本；本 change 生产代码零改动（注释级修订除外）。
- Ruling: change 设置 `skip_specs: true` — 本行只删除测试基础设施并同步基线表述，不改变玩家可观察行为或主规格 Requirement — 为测试删除虚构产品 capability 会让规格失真（同 E-11 先例）。
- Ruling: `internal/mesh` greedy/light oracle 切片延迟 — A-02 独占 `internal/mesh` 且正在演进 Rust mesher，此刻删除差分网恰在变动中的代码上方；待 A 批次合流后另行认领（移交项见 proposal）。

## Task 1

- Implementer：fresh 子代理（会话 `ses_fc694218fffeTPTv4iKBQOaXAI`），冻结契约 `f7d9b1ea`。先立 `step_golden_vectors_test.go`（12 向量）并完成变异自查，再删除 oracle；`collision_helpers_test.go` 整删（共享 fixture 实际定义于消费文件，未受影响）；两个行为性用例（RejectedUnknownStep 隔离、ConcurrentCalls）改字面量/串行基准，函数名不变。
- SPEC review：PASS——删除集与 design 清单逐项吻合、保留网完整（`go test -list` 集合比对）、生产文件零改动、门禁四项独立重跑全绿。
- QUALITY review：FAIL（Important）——文件头「覆盖全部分支」自证伪：空中走路速度钳制子分支无向量覆盖且旧套件本有同名用例，数值覆盖随切片归零。
- R1/5 修复：新增 airborne walk speed clamp 向量（与被删旧套件同参数同形，位模式采集固化），文件头改为逐分支枚举；落实全部反引号 Minor 项。
- R2/5 修复（复核新发现）：`` `math.Float32bits` `` 为 stdlib 标识符，反引号触发 archcheck `TestCommentBacktickIdentifiersExist`——去反引号一行修复。复核者预授权「重跑 archcheck + physics 即终审」，两门禁全绿收口。
- 门禁：`go test ./internal/physics -race -count=1` ok、`go vet` 无输出、gofmt 无输出、archcheck ok；变异自查两次独立复现成功。提交 `e914ddb3`（净 −431 行）。

## Task 2

- Implementer：fresh 子代理（会话 `ses_fc64f2393ffezqZcfJOl4ENJK5`），冻结契约 `e914ddb3`。`raycast_helpers_test.go` 整删（删前 grep 确认消费集合）；差分 fuzz 与三件套差分测试删除；`FuzzRaycastBlocks` 补两条几何不变量并各配确定性孪生；`TestNativeRaycastConcurrentCalls` 去 oracle 化改串行 native 基线（函数名不变）；`raycast_bench_test.go` 零改动（只用生产 API）。
- SPEC review：PASS——删除集吻合、`go test -list` 集合比对恰为「−4 差分 +2 孪生」、法线约定与生产 `block.go` 同源、五门禁独立重跑全绿。
- QUALITY review：FAIL（Important）——评审者以 Rust 变异实验证实不变量一注释归因失真：纯遍历序错误下 record 仍几何自洽，序锁实际由 `TestRaycastBlocksUsesXYZTiePriority` 与 Rust 侧同源测试承担；「必然挤出/至少半格」量词不实。
- R1/5 修复：锁定面改述为「格与距离失步」类并补覆盖分工句；M-1 tMax 序注释按手算序改写；M-2 斜向孪生扩为全表显式 Face 期望钉死（六轴向 + XYZ 平局 NegZ）。
- 复核：PASS（四复核点全落实、门禁重跑全绿）。提交 `f094de48`（净 −102 行）。

## Task 3

- Implementer：fresh 子代理（会话 `ses_fc61ad0d0ffeQNt56hIhAyeg0c`），冻结契约 `f094de48`。`oracle_test.go` 整删；parity 差分主体删除、border 一致性改生产自一致（chunk dense vs 单点 probe 双出口）；tree 白盒三删、两改黑盒（树冠几何期望为硬编码冻结常量，非循环自证）、一原样；noise/ore/material/fluid 核实为零 oracle 引用未触碰；基线句纯从句删除（AGENTS.md 两处 + config.yaml 同两句），CLAUDE.md `cp` 后 `cmp` 一致；archcheck 另强制四处新暴露失真注释的注释级修订（capture_near_band.go/app_lod.go/fluid_perf_test.go/generator.go——stash 实证 HEAD 基线为绿）。
- Ruling: cmd/mornlea 两文件的注释级修订虽落在 A 批次声明的 app 文件邻近，但属 archcheck 门禁强制且仅注释行——合并冲突面为一行注释，可接受。
- SPEC review：PASS——删除集与 D4 规则逐用例抽查吻合（被测体判定全部正确）、前提核实与被证断言分离守住、基线句零版本触碰、四门禁独立重跑全绿。
- QUALITY review：PASS（非阻塞两项）——① ledger 占位残留（控制会话记账瑕疵，已修）；② engine `worldgen.rs:5` 模块文档仍引用已删除的差分门禁。后者已由 implementer 会话一行修复（改为「冻结语义要求与删除前迁移基线逐位一致，由生产黑盒测试与 golden 字节锁锁定」，cargo build 通过）。

## 终审

（待整分支终审）
