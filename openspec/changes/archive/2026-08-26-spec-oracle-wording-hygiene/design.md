# 设计：spec-oracle-wording-hygiene

## 勘察结论（2026-08-26，基线 `7cfd4c7c`）

三份主规格共 16 处 oracle 措辞逐一定位并核对现存测试网后得出旧措辞→新门禁映射（下表）。新措辞全部指向真实存在且当前全绿的测试实体；每条 MODIFIED Scenario 的 GIVEN/WHEN/THEN 与对应测试的实际行为一致。

### rust-engine-physics-step（7 处 → 2 条 MODIFIED requirement）

| 现规格位置 | 失真措辞 | 更正为（现存门禁） |
|---|---|---|
| Purpose | 「以 Go 测试 oracle 保证 arm64 逐位一致」 | （残留处置，见下节） |
| Req「物理 tick 积分由 Rust engine 独占生产」正文 | 「旧 Go 实现只允许存在于测试 oracle」 | Go 生产路径与测试都不保留旧积分副本；跨平台 float32 逐位一致契约由冻结位级 golden 向量把守 |
| Scenario「arm64 生产 Step 与 Go oracle 逐位一致」THEN | 「与测试内 Go oracle 实现逐位一致」 | 固定向量在任意平台经 `physics.Step` 复现 `math.Float32bits` 字面量——13 条向量覆盖地面行走/减速、起跳、空中重力与终端钳制、水中下沉/上浮/水平阻力、天花板碰撞、半砖 step、unknown 格阻挡与负零哨兵（`internal/physics/step_golden_vectors_test.go`） |
| Scenario「非 arm64 不使用平台相关 Go oracle 作逐位门禁」THEN | 「MUST NOT 将结果与平台相关的旧 Go oracle 作逐位比较」 | 非 arm64 平台运行同一套 golden 向量；冻结期望是平台无关字面量，测试不依赖任何随平台变化的参考实现 |
| Req「碰撞差分入口保留」正文 | 「仅供测试差分」 | 差分测试已随 E-04 删除；该出口现在供行为锁测试直接驱动碰撞解析（字面量期望 + 并发串行基准 + 零分配锁，`internal/physics/collision_native_test.go` 的 `TestNativeCollisionRejectedUnknownStepKeepsOrdinaryHitUnknownFalse` 等） |
| Scenario「碰撞差分测试继续通过」THEN | 「与 Go 碰撞 oracle 逐位一致」 | 输出位置、clipped mask、OnGround、UsedStep、HitUnknown 与采集自生产路径的冻结字面量逐位一致 |
| 其余 2 个 Scenario（对角加速/跳跃重力） | 无 oracle 措辞 | 原样保留 |

### rust-engine-collision-raycast（3 处 → 3 条 MODIFIED requirement）

| 现规格位置 | 失真措辞 | 更正为（现存门禁） |
|---|---|---|
| Req「Rust collision 保持共享物理结果」正文 | 「产生与冻结 Go oracle 逐位一致的……」 | 结果被位级 golden 向量与字面量期望钉住，任意平台重放逐位复现冻结值（`step_golden_vectors_test.go`、`collision_native_test.go`）；解析顺序 Y/X/Z、unknown 闭合边界、step 选中规则等既有语义子句原样保留 |
| Scenario「多 batch 保持遍历语义」THEN | 「……MUST 与冻结 Go oracle 一致」 | 遍历语义由跨 batch 续行行为锁与 XYZ 平局序测试钉住，floor/int32 回绕类算术缺陷由两条几何不变量（命中点位于命中格单位立方内、进入面法线与归一化方向点积为负）的性质 fuzz 与确定性孪生向量把守（`internal/core/raycast_fuzz_test.go`） |
| Scenario「两个平台逐位一致」THEN | 「两个平台 MUST 与 test-only Go oracle 逐位一致」 | 两平台输出与平台无关的冻结期望逐位一致：collision 经位级 golden 向量与字面量期望，raycast 经确定性孪生向量与性质不变量；WHEN 中「随机和 fuzz corpus」相应收敛为「固定语料」（随机差分对照已不存在，性质 fuzz 无跨平台比较目标） |
| 其余 Scenario 与正文 | 无 oracle 措辞 | 原样保留（MODIFIED 块须整体重述，含未改动部分） |

### rust-engine-worldgen（6 处 → 1 条 MODIFIED requirement）

| 现规格位置 | 失真措辞 | 更正为（现存门禁） |
|---|---|---|
| Purpose | 「并以 Go 测试 oracle 保证同种子世界逐位一致」 | （残留处置，见下节） |
| Req「世界生成由 Rust engine 独占生产」正文 | 「旧 Go 实现只允许存在于测试 oracle」 | 仓库不保留任何旧 Go 实现副本；同种子确定性由黄金摘要文件与生产黑盒双出口对照把守 |
| Scenario「同种子区块与 Go oracle 逐位一致」GIVEN/WHEN/THEN | 「区块内每个 (x,y,z) 的 BlockID 与测试内 Go oracle 实现完全一致」 | 冻结语料的种子与区块坐标上，全区块 BlockID 序列的 SHA256 摘要与提交入库的黄金文件逐字节一致（`internal/worldgen/testdata/golden_seed42.txt`，`generator_test.go`），同种子重复生成逐格一致。GIVEN 相应从「任意种子与任意区块坐标」收敛为「黄金文件冻结的种子与区块坐标」——黄金摘要只钉固定语料，不得虚构任意性覆盖 |
| Scenario「单点查询与整块生成一致」THEN | 「……且与 Go oracle 一致」 | 删除 oracle 从句；双出口对照改述为 GenerateChunk 区块稠密输出与单点查询两条生产公共出口互检（`tree_test.go`、`parity_test.go`） |
| Scenario「跨区块橡树一致」THEN | 「拼合后与 oracle 树形一致」 | 拼合结果与同种子单点查询语义逐格一致，根列树高保持冻结区间 4..6、原木优先与树叶仅覆盖空气规则不变（`parity_test.go` 跨界树对照、`tree_test.go` 树冠几何固定常量） |
| 其余 3 条 Requirement（perm 表播种、ABI 校验、既有行为规格） | 无 oracle 措辞 | 不入 delta |

## 关键决策

### D1：delta 只用 MODIFIED requirement，名称与 Scenario 标题零改动（批准采纳）

Requirement 名称不含 oracle 字样，Scenario 标题仅 3 处含（见残留处置）。保持全部标题不动使 diff 收敛到失真句本身，主规格 Requirement 排列与锚点稳定，评审只需读改述文本。

### D2：工具约束与残留处置（勘察发现，记录移交）

openspec 1.7.0 的两个 apply 行为决定了纯 delta 无法清掉全部 16 处：

1. **delta 的 `## Purpose` 对已存在的主规格被忽略**（`specs-apply.js` 明示「A delta Purpose only seeds a spec that does not exist yet」，apply 时告警）。因此 physics-step 与 worldgen 的 Purpose 各 1 句无法经 delta 更正。
2. **MODIFIED 的漂移守卫要求现规格每个 Scenario 标题在修改块中逐一出现**（缺失即 abort「Refresh the change spec before archiving to avoid dropping scenarios」），不支持 Scenario 改名。因此 physics 2 个、worldgen 1 个含 oracle 字样的 Scenario 标题无法经 MODIFIED 更正。

被否决的替代方案：
- **REMOVED + ADDED 同名重建**：openspec 把同一 requirement 同时出现在 ADDED 与 REMOVED 判为硬冲突；换名则违背 D1 的结构稳定决策。
- **接受残留不处理**：Purpose 句与标题是规格最显眼的入口文本，留着「Go 测试 oracle」会让本 change 的正文更正自相矛盾。

**处置**：5 处残留（2 句 Purpose + 3 个 Scenario 标题）列为归档阶段收尾项，由归档会话在 delta 应用后对主规格做五行直编——归档本就是唯一被授权直编主规格的阶段。替换文本如下，归档时照抄：

| 文件 | 残留行 | 归档时替换为 |
|---|---|---|
| `rust-engine-physics-step/spec.md` Purpose | 「并以 Go 测试 oracle 保证 arm64 逐位一致。」 | 「并以冻结的位级 golden 向量保证任意平台 float32 逐位一致。」 |
| `rust-engine-physics-step/spec.md` Scenario 标题 | 「arm64 生产 Step 与 Go oracle 逐位一致」 | 「生产 Step 与冻结 golden 向量逐位一致」 |
| `rust-engine-physics-step/spec.md` Scenario 标题 | 「非 arm64 不使用平台相关 Go oracle 作逐位门禁」 | 「golden 向量在非 arm64 平台同样逐位成立」 |
| `rust-engine-worldgen/spec.md` Purpose | 「并以 Go 测试 oracle 保证同种子世界逐位一致。」 | 「并以黄金摘要与生产黑盒双出口对照保证同种子世界逐位一致。」 |
| `rust-engine-worldgen/spec.md` Scenario 标题 | 「同种子区块与 Go oracle 逐位一致」 | 「同种子区块生成与冻结黄金摘要一致」 |

### D3：docs-only，无兼容性影响（批准采纳）

不改代码、测试、golden、协议、存档或 ABI；三份主规格描述的可观察行为集合不变，只把验收措辞从已删除的机器改述为现行门禁。无迁移问题：主规格在归档阶段原子更新，期间新旧措辞并存窗口内两套表述指向的测试都在全绿运行。

## 风险与回退

- 风险：改述文本过度声称覆盖范围（如把固定语料写成任意输入）。缓解：每条 THEN 都按真实测试断言粒度书写，GIVEN 收敛到实际语料范围。
- 回退：整 change 为文档删除级反转，`git revert` 单提交即完整回退。

## 验证方法

`openspec validate --all --strict --no-interactive` 全绿；`go test ./internal/archcheck -count=1` 通过（确认基线版本检查不受影响）；人工核对每条改述与所引测试文件的断言一致。
