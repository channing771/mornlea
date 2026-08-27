# Spec Oracle Wording Hygiene

## Why

change `drop-go-test-oracles`（E-04，已归档于 `openspec/changes/archive/2026-08-26-drop-go-test-oracles/`）删除了 physics、core raycast、worldgen 三切片的 test-only Go oracle 与差分门禁，并以新门禁补位：physics 换成 13 条冻结位级 golden 向量（`internal/physics/step_golden_vectors_test.go`），raycast 换成两条几何不变量的性质 fuzz 加确定性孪生向量（`internal/core/raycast_fuzz_test.go`），worldgen 改由黄金摘要文件与生产黑盒双出口对照把守（`generator_test.go`、`tree_test.go`、`parity_test.go`）。

但三份主规格的验收措辞仍描述已删除的机器：「与 Go 测试 oracle 逐位一致」「非 arm64 不使用平台相关 Go oracle」「与冻结 Go oracle 一致」「与 oracle 树形一致」等共 16 处。E-04 当时的裁决是「迁移规格冻结惯例，待后续 spec-hygiene 独立 change 统一修订」（见其 ledger 移交项）。这些句子现在指向不存在的测试实体，误导读者对现行回归网的判断——正是 AGENTS.md 警告的基线失真形态。

## What Changes

只改三份主规格的失真措辞，经本 change 的 delta specs 在归档阶段应用：

- `openspec/specs/rust-engine-physics-step/spec.md`：2 条 Requirement 的正文与 Scenario 文本改述为「13 条冻结位级 golden 向量钉住跨平台 float32 逐位一致契约」与「碰撞出口由采集自生产的字面量期望、串行并发基准与零分配锁把守」。
- `openspec/specs/rust-engine-collision-raycast/spec.md`：3 条 Requirement 的正文或 Scenario 文本改述为「结果被位级 golden 向量、字面量期望、几何不变量性质 fuzz 与确定性孪生向量钉住」。
- `openspec/specs/rust-engine-worldgen/spec.md`：1 条 Requirement 的正文与 3 个 Scenario 文本改述为「黄金摘要文件 + 同种子重放 + chunk dense vs 单点 probe 双出口对照 + 既有树冠几何/跨界树一致性/树高区间性质」。

全部 delta 采用 MODIFIED requirement：Requirement 名称与 Scenario 标题一律不动，只改正文与 Scenario 项目符号文本，保持规格结构稳定。无代码、测试、golden、capture、协议或存档变化。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `rust-engine-physics-step`：2 条 Requirement 措辞更正（行为契约本身不变）。
- `rust-engine-collision-raycast`：3 条 Requirement 措辞更正（行为契约本身不变）。
- `rust-engine-worldgen`：1 条 Requirement 措辞更正（行为契约本身不变）。

## Impact

- 只新建本 change 目录并修改上述三份主规格（在归档阶段经 delta 应用）；不触碰任何 Go/Rust 生产代码、测试、golden 或 capture 场景。
- 无协议、存档 schema、engine/client ABI、benchmark scenario、依赖或配置格式变化；docs-only，无兼容性影响。
- 门禁不放宽：主规格描述的门禁从「已删除的 oracle 差分」更正为「现存测试网」，实际测试覆盖不变。

## 非目标

- 不修改 `openspec/specs/rust-engine-mesh/spec.md` 的 2 处 oracle 措辞——mesh greedy/light oracle 至今实存，其措辞当前为真，待 A 批次合流后 mesh oracle 切片落地时随体更正。
- 不改变任何 Requirement 或 Scenario 的可观察行为语义；纯措辞卫生。
- 不修订 progress.md 等历史记录（历史条目允许提及已删除之物）。

## 用户可观察结果

游戏行为完全不变。三份主规格的验收措辞如实描述现存回归网：读者据规格能找到真实把守逐位一致性与世界确定性的测试文件，而不是追索一份已在 E-04 删除的 Go oracle。

## 延期与放弃

- `rust-engine-mesh` 的 2 处 oracle 措辞：mesh oracle 切片因 A 批次仍在演进 Rust mesher 而未删除（E-04 移交项），届时独立认领并同步更正措辞。
- 两份 Purpose 段落与三个 Scenario 标题中的 oracle 字样：openspec 1.7.0 对已存在的主规格忽略 delta 的 `## Purpose`（apply 时告警「delta Purpose ignored」），且 MODIFIED requirement 的漂移守卫要求 Scenario 标题与现规格逐一对应、不支持改名。因此 `rust-engine-physics-step` 与 `rust-engine-worldgen` 的 Purpose 各 1 句、physics 的 2 个 Scenario 标题（「arm64 生产 Step 与 Go oracle 逐位一致」「非 arm64 不使用平台相关 Go oracle 作逐位门禁」）、worldgen 的 1 个 Scenario 标题（「同种子区块与 Go oracle 逐位一致」）无法经 MODIFIED delta 更正，作为归档阶段收尾项由归档会话对主规格做五行直编（详见 design.md「工具约束与残留处置」）。
