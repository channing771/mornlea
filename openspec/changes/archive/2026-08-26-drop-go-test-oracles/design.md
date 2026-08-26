# 设计：drop-go-test-oracles

## 勘察结论（2026-08-25，基线 `88094977`）

认领备注与实际足迹的一处偏差已核实：`internal/worldgen/generator.go` 内**没有**失去消费者的 test-only 旧实现——旧噪声/地形/矿石/橡树代码早已只以自包含副本形式存在于 `oracle_test.go`（文件头注释明示「刻意不依赖任何生产内部标识符」）。因此本 change 对三个包的**生产文件零改动**，全部工作在 `*_test.go` 与基线文档。

### 精确删除/保留清单

**physics（`internal/physics`）**

| 文件 | 处置 |
|---|---|
| `motion_helpers_test.go`（102 行） | **整文件删除**：`oracleStep` 及其私有 helpers 是生产曾位于 `motion.go` 的逐字副本 |
| `step_native_test.go`（352 行） | 保留 `TestStepInputLayoutV2` 与其编码镜像 helpers（ABI 布局锁，无 oracle 依赖）；删除 `TestStepProductionMatchesGoIntegrationOracle`、`TestStepProductionMatchesGoIntegrationOracleExtended`（含 128 例随机语料差分） |
| `collision_native_test.go`（673 行） | 删除全部差分测试与 `testAssertProductionMatchesOracle` 断言助手；个别以 oracle 计算期望值的**行为性**用例（如 rejected-step 保持 `HitUnknown=false`）改写为字面量期望后保留 |
| `collision_helpers_test.go`（265 行） | 拆分：共享 fixture（`testCollisionWorld`、`fullCube` 等，被非 oracle 测试消费）留在原文件或迁入既有 helper 中心；`oracleResolveCollision` 及仅被差分消费的部分删除 |
| `physics_fuzz_test.go` | 保留两个纯性质测试/fuzz；删除 `FuzzNativeCollisionMatchesGoOracle`；`TestStepDeterministic` 等性质网不动 |

**core raycast（`internal/core`）**

| 文件 | 处置 |
|---|---|
| `raycast_helpers_test.go` | **整文件删除**：`oracleRaycastBlocks`/`oracleRaycastRecords` 及解码 helpers |
| `raycast_native_test.go`（412 行） | 保留 `TestRaycastInputCursorAndRecordLayoutV1` 与 cursor batch 行为锁；删除差分测试 |
| `raycast_fuzz_test.go`（86 行） | 删除 `FuzzNativeRaycastMatchesGoOracle`；`FuzzRaycastBlocks` 追加两条不变量：① 命中点位于命中格单位立方内；② `hit.Face` 法线与归一化射线方向的点积 < 0（面是进入面） |

**worldgen（`internal/worldgen`）**

| 文件 | 处置 |
|---|---|
| `oracle_test.go`（401 行） | **整文件删除**：pointwise oracle 全套 + `TestOracleMatchesProduction` + `TestOraclePointQueriesMatchProduction` |
| `parity_test.go`（130 行） | 删除 `assertChunkMatchesOracle`、`TestRandomSeedChunkParity`、`FuzzWorldgenOracleParity`；`TestOakTreeSpansChunkBorderConsistently` 若基于生产则保留，若依赖 oracle 则改写到生产黑盒 |
| `tree_test.go` | 白盒用例（hash 选择器位检验等，被测体是 oracle 副本）随体删除；可经 `GenerateChunk` 公共输出表达的性质改写为黑盒用例保留（命名不变） |
| `noise_test.go`/`ore_test.go`/`material_test.go`/`fluid_test.go` | 逐一核对被测体：作用于生产的保留；作用于 oracle 的按同规则删除或改写 |

**基线文档**

| 文件 | 处置 |
|---|---|
| `AGENTS.md` / `CLAUDE.md` | 三句失效表述窄 hunk 更新：「旧 Go 积分与 worldgen 实现只作测试 oracle」「旧 Go DDA 只是测试 oracle」及 context 镜像句——删除后这些句子的主语不复存在。两文件逐字节相同由 archcheck 兜底。版本号句零触碰 |
| `openspec/config.yaml` | context 中「旧 Go DDA 只是测试 oracle，生产无 fallback」句同步收敛为「生产无 fallback」表述 |
| 生产注释 | `generator.go:7`、`motion.go`/`step.go`/`collision.go`/raycast 生产文件中引用「oracle_test.go」的注释随删除同步修订（注释级改动，archcheck 注释标识符门禁会抓失真引用） |

progress.md 历史条目一律不动（历史记录允许提及已删除之物）。

## 关键决策与被否决方案

### D1：physics 确定性子集转位级 golden 向量（批准采纳）

现有 e2e/transport parity 锁的是同进程行为，锁不住跨平台位漂移；而重放一致契约要求跨平台逐位相同。把约 8–12 个代表性用例（平地行走、跳起、天花板碰撞、水中下沉/上浮/水平阻力、±0 速度哨兵、半砖 step）的 `StepResult` 以 `math.Float32frombits(0x…)` 字面量钉住。向量值从当前生产路径一次性采集并人工复核合理性。

- 否决「纯删不补」：位确定性是规格契约，e2e 只能查到粗粒度偏差，字面向量是唯一廉价的位级网。
- 否决「保留 oracle 只跑一次生成向量再删」之外的方案（如运行时快照文件）：字面量进源码即可评审、可 diff、零 I/O。

### D2：性质 fuzz 补位而非暴力参考实现（批准采纳）

raycast 差分 fuzz 的替代考虑过「fuzz 体内内置 brute-force slab 遍历作参考」——否决：那是在 fuzz 里重写一份小 oracle，正是本 change 要消灭的形态；且 `FuzzRaycastBlocks` 已覆盖「有限输入无错 + 命中距离有界 + 命中格实心」，补两条几何不变量后，剩余唯一显著缺口（记录序列完整性）由 native parity 测试与布局锁共同覆盖，风险可接受。

### D3：基线句随本 change 同步而非移交 A-07（批准采纳）

A 批次功能分支不触碰 `AGENTS.md`/`CLAUDE.md`（集成任务 A-07 独占基线重写），但本 change 若不同步，「旧 Go X 只作测试 oracle」三句在合并瞬间变假且无任何信号——正是 AGENTS.md 明文警告的基线失真形态。窄 hunk（每处一句）与 A-07 的整段重写在 git 层面是分离改动；先合先改、后合者 rebase 即得干净基线。

### D4：白盒树用例随体消亡而非导出生产探针

`tree_test.go` 的 hash 位检验用例被测体是 oracle 副本的私有函数；为保它们而导出生产内部探针是反向扩大公共面。其中「选择器跨区块稳定」「树冠/原木优先级」等真性质已有生产黑盒孪生或可低成本改写；纯实现细节断言（如 salt 常量的特定位模式）随实现消亡属 §15 sanction 范围。

## 并发与合并序

- 与 A 批次五个功能分支零文件交集（它们不触碰 physics/core/worldgen 的 test 文件）；与 E-12（codec_client/capture）、B-13（sim combat/hunger）零交集。
- 唯一共享文件是 `AGENTS.md`/`CLAUDE.md`/`config.yaml` 的窄 hunk；按 D3 处理合并序。
- 本分支从 `main@88094977` 创建。

## 风险与回退

- 风险：删除后某行为回归失去最早预警。缓解：布局锁、性质 fuzz（增强后）、golden 向量、包内既有行为单测、server e2e parity 五层网全部在位；Rust 侧自身 `#[cfg(test)]` 单测独立演进。
- 回退：整 change 为 test-only 删除，`git revert` 单提交即完整回退，无存档/协议牵连。

## 验证方法

每任务定点 race 测试 + 收尾全量门禁（`make rust`、`go test ./... -race`、`go vet`、gofmt、OpenSpec strict）。golden 向量另加变异自查：临时翻转一个向量的低位确认测试确实失败（变异不留痕，验证后还原）。
