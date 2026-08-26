## Context

参见 `proposal.md` 的动机与 `specs/authoritative-farming/spec.md` 的行为契约。当前 `advanceCropCell` 在随机样本落到耕地时调用 `farmlandIsWet`，一次读取目标格并最多查询 `9×9×2` 个邻格；流体推进写入集中在 `fluidWorld.SetBlock`，普通玩家放置还可在 `executePlacement` 合法地以固体覆盖流体，成功翻地集中在 `executeTillSoil`。`advanceFluids` 每 tick 已按 `activeInterestKeys()` 的稳定顺序构建 `engine.fluidScope`，该集合实际就是 active interest 与 `ChunkReady` 的交集。

全部相关状态继续由 `Engine.Step` 单写者拥有。变更只在 `internal/sim` 内复用现有 `core`、`world` 与 `fluid` 依赖，不增加 goroutine、锁、I/O、外部依赖或跨包依赖边。

## Goals / Non-Goals

**Goals:**

- 让无旧积压的单格流体 membership 变化（含成功玩家放置覆盖流体）和成功翻地在同 tick 完成湿度判定。
- 用固定 `65,536` 次候选检查约束事件 FIFO 工作，并用独立的 `65,536` 次规则查询读取约束事件处理与恢复重扫的合计工作。
- 保持候选、重扫与跨 tick 恢复顺序确定，且不依赖 map 遍历。
- 让随机作物阶段只推进作物，并显式报告样本数和规则查询读取数。

**Non-Goals:**

- 不新增可调参数、湿度缓存、邻水计数或持久化调度状态。
- 不修改 `internal/fluid` 的规则或队列，不接入所有 `recordChange` 写者。
- 不改变方块编号、协议、存档 schema、ABI、scenario 或 capture。

## Decisions

### 1. 用 FIFO 候选与去重集合维护湿度待办

在 `Engine` 中追加一个 `farmlandMoistureState` 聚合字段。状态持有 `(DimensionID, BlockPos)` 候选 FIFO、同集合的去重 map、消费下标、重扫状态，以及最近一个 tick 的候选检查与读取计数。切片决定行为顺序，map 只做 O(1) 存在性查询；消费前缀达到固定阈值且占切片至少一半时以 `pending = pending[head:]` 做 O(1) rebase，排空时复位为空切片供后续复用。键不含指针，rebase 保留 backing array 不会保留其他堆对象。

候选按首次入队顺序处理。每次查看 FIFO 队首先计一次候选检查，每 tick 最多 `65,536` 次；检查后再确认候选所属区块仍在当前 active Ready scope，不在则删除，重新进入时由重扫恢复。读取目标格一次；非耕地立即删除。若目标是耕地而本 tick 剩余额度不足最坏 162 次湿润邻域查询，则保留队首并结束本 tick，下一 tick 从完整判断重新开始。额度足够时沿既有 `dy,z,x` 顺序查询邻域，遇到首个流体提前结束；状态变化经 `Dimension.SetBlock` 与 `recordChange` 汇入当前 `pendingChunkChanges`。

候选检查预算计量每次 FIFO 队首查看，包括范围外、维度缺失、非 Ready 和因读取余额不足而保留的候选。读取预算只计规则为作出决定而读取的方块编号：候选目标、湿润邻域、重扫格、作物随机样本及其下方格；active scope 与维度 map 查询不是方块读取。`SetBlock` 为防御性比较旧值所做的内部读取不属于第二次规则查询，也不重复计费。候选先读后发现余额不足时，该次读取仍计费，下一 tick 重新读取；两个独立计数每 tick 都不超过 `65,536`。

**否决方案：** 每格邻水计数需要第二份可失效事实和加载重建；每格缓存判定 tick 仍留下随机延迟；优先队列没有 deadline，仅增加排序成本。

### 2. 只在真实流体 membership 变化与成功翻地处生产候选

`fluidWorld.SetBlock` 在写入前已经能读取旧方块。普通玩家的 `executePlacement` 也已把目标旧值保存在 `block`，且允许固体覆盖流体；它只在 `Dimension.SetBlock` 成功并报告 `changed` 后比较 `core.IsFluid(block) != core.IsFluid(placement)`。两条路径发生真实 membership 变化时，都复用 `enqueueFarmlandMoistureAroundFluid`，以变化格 `(x,y,z)` 反向枚举：

```text
farmlandY = y-1, y
farmlandZ = z-4 .. z+4
farmlandX = x-4 .. x+4
```

枚举顺序固定为 `farmlandY,z,x`，最多产生 162 个去重候选；越出世界 Y 范围的格跳过。流体等级之间互换不触发，因为湿润规则只关心 `core.IsFluid`。玩家放置的拒绝、`SetBlock` 错误和 `changed=false` 路径都不入队，也不改变既有扣料与成功确认语义。

`executeTillSoil` 仅在全部校验通过且 `SetBlock` 确实把目标写成干耕地后，将目标自身入队。拒绝路径和未变化路径不入队。湿/干耕地互转不是流体 membership 变化，也不重新入队。

一次 membership 写入的枚举工作固定为 162 次以内；流体推进写入受既有 `FluidUpdatesPerTick` 上界间接约束，玩家放置仍走既有命令边界，湿度候选消费者另受固定检查/读取预算约束。去重使同 tick 邻近变化不会复制同一耕地候选。无旧积压且 162 个候选全为耕地时，最坏规则查询读取为 `162×(1+162)=26,406`，低于单 tick 预算。

writer audit 的运行期 membership 写者只有两条：`fluidWorld.SetBlock` 可双向改变，`executePlacement` 可把流体覆盖为非流体。伙伴放置只接受空气目标；玩家/伙伴采掘不命中流体；翻地、湿度、作物与踩踏只在非流体农业编号间转换；`SetBlockForTest` 不是生产写者。世界加载/生成不产生事件，由进入 active Ready 的重扫恢复。

**否决方案：** 挂在 `recordChange` 会让采掘、作物生长、踩踏和容器等所有无关写入都展开 162 格，而且该汇聚点拿不到可靠的写前 membership；为两条现有写者增加共享抽象也没有复用收益。未来新增流体 membership 写者时必须复用 old/new membership 语义。

### 3. 独立阶段位于流体与作物之间

新增 `phaseFarmlandMoistureAdvance`，调用顺序固定为：

```text
advanceFluids
advanceFarmlandMoisture
advanceCrops
finishChanges
```

这样流体本 tick 的最终写入先产生候选，湿度阶段再读取最终水位，作物最后只读取正下方已经落盘的湿/干编号。三个阶段的方块写入继续共享同一份 `pendingChunkChanges`。`stepPhaseObserver`、流体净耗时 benchmark 的结束边界和阶段顺序测试同步增加新阶段，防止测量把湿度耗时误算进流体。

**否决方案：** 折入流体阶段会让 benchmark 无法归因且翻地候选的所有权不清；保留在作物阶段会继续混淆随机采样与恢复队列两种成本模型。

### 4. 复用流体阶段构建的 active Ready scope

湿度阶段直接读取 `engine.fluidScope`，不维护第二套 previous/next scope。`advanceFluids` 无论世界是否有水都会构建该集合；它发现新进入 key 的稳定循环同时把 key 登记到独立的湿度重扫队列。湿度重扫只复用 scope，不读取或修改 `internal/fluid.Queue` 与 `fluidRescanState`。

该复用减少两张逐 tick map 和一次 `activeInterestKeys()` 扫描。阶段顺序测试与重入测试承担耦合守卫：若未来流体阶段被条件跳过，必须先把 active Ready scope 提升为通用状态，不能让湿度阶段读取陈旧集合。

**否决方案：** 独立重建 scope 语义直接但重复热路径工作；复用 `fluidRescan.pending` 不成立，因为两种重扫集合、预算与游标完全不同。

### 5. 新进入区块执行有预算的完整高度 halo 重扫

湿度重扫状态独立持有按进入顺序排列的 `ChunkKey` FIFO、去重 map 和队首的一维坐标游标。离开 scope 的 job 以稳定过滤丢弃；队首变化时游标归零。每个 job 扫描目标区块水平边界向外扩 4 格后的 `24×24` 平面，以及 `[core.MinY, core.MaxY)` 全高度，坐标顺序固定为 `y,z,x`。

每检查一格消耗一次共享读取预算。读到耕地时，只有耕地自身所属区块仍在当前 scope 才加入候选 FIFO；halo 既让新进入区块里的水唤醒邻块边缘耕地，也让新进入区块里的耕地在后续候选处理时读取邻块水体。事件候选先消费预算，剩余额度推进重扫；重扫产生的候选从下一 tick 起处理。未扫到前保留区块中已有的湿/干编号，不制造中间状态。

事件优先可能延迟重扫，但在停止新增玩家命令后，有限 active Ready 范围内的权威流体推进会收敛且无自持更新环；玩家放置每次只能移除一格既有流体 membership，不能从物品创建流体；成功翻地每次只登记目标自身。因此固定输入产生的事件工作最终排空，剩余读取预算随后推进重扫；未来若增加无需新命令即可永久产生 membership 变化的机制，必须重新裁决事件与重扫的公平配额。

**否决方案：** 同步全扫会让批量 Ready 击穿 tick；只扫目标区块会漏跨区块半径；只扫表面需要维护额外耕地索引；持久化游标只保存调度状态却要求 schema 迁移。

### 6. 随机作物阶段删除耕地分支并增加读取计数

`advanceCropCell` 只处理作物。每条随机样本计一次读取，样本是作物时读取正下方格一次；高度图访问不算方块编号读取。因此：

```text
crop cells examined = ready sections × RandomTicksPerSection
crop block reads <= 2 × crop cells examined
farmland moisture block reads <= 65,536
```

`cropCellsExamined` 保留，新增 `cropBlockReads` 与湿度阶段读取计数供包内测试和 benchmark 使用。`crop_perf_test.go` 的各场景同时报告 `cells/op`、`block_reads/op`、区块数与作物数；全耕地 benchmark 还精确守卫并报告一个 Ready 区块、全世界高度列的 `98,304` 格耕地与零作物，证明每个样本只读取一次、不再扫描 162 格。性能墙钟只记录，解析式读取上界和工作负载规模是 correctness 门禁。

`tunables.go` 中关于 `RandomTicksPerSection` 同时控制耕地干湿的注释将做最小文字更正；字段、默认值、校验和 A-04 新增 tunable 区域不变。

**否决方案：** 删除 `cropCellsExamined` 会丢失固定抽样数证据；只看墙钟会受机器噪声影响；把湿度预算做成 tunable 会扩配置面且弱化固定性能契约。

### 7. 测试按关注点拆分

现有 `crop_test.go` 同时包含哈希抽样、生长规则、湿润集成和成本主题。因本变更必须修改湿润测试，按仓库测试组织规则把现有测试函数原名迁入关注点文件；前后用 `go test -list` 按集合语义比较，零行为重组不改测试名或子测试名。被多个文件消费的农业夹具移入现有 helper 中心；单文件 helper 留在消费文件。

新增测试分别覆盖候选枚举与 FIFO、预算和确定性、流体/玩家放置覆盖流体/翻地同 tick 集成、重启/重入 halo 恢复、阶段顺序和成本上界。玩家覆盖测试必须经真实 `CommandPlaceBlock` 与完整 `Step`，并守卫成功确认和扣料；测试不得让 `SetBlockForTest` 自动生产事件而掩盖漏接真实写者。

## Risks / Trade-offs

- [流体写入每格最多展开 162 次 map 去重] → 展开次数是固定常数并受流体处理预算约束；benchmark 记录新增湿度阶段，不放宽流体门禁。
- [事件 FIFO 在极端流动中可跨多个 tick 积压] → 坐标去重、固定候选检查与读取预算、active scope 丢弃保证每 tick 工作有界且不丢当前有效工作；同输入的积压顺序完全确定。
- [事件持续优先可能推迟恢复重扫] → 停止新增命令后，当前 fanout 只来自有限范围内必然收敛的流体推进和只能移除既有流体的玩家放置，翻地只产生单候选；增加无需新命令的永久 membership 生产者时必须引入显式公平配额并更新规格。
- [复用名为 `fluidScope` 的通用 Ready 集合形成隐式耦合] → 固定阶段顺序、scope 构造测试和重入测试锁定前置；不为一次消费重命名全套流体状态。
- [初次进入一个区块的完整 halo 重扫约 22 万格，需要数个 tick] → 共享 65,536 预算跨 tick 保存游标，期间保留存档状态，最终恢复而不阻塞 tick。
- [A-04 同时修改 `engine.go`、`engine_step.go` 与 `tunables.go`] → 只在已登记区域追加一个聚合字段、一个阶段和两段现有农业注释；合并时逐处裁决，不覆盖 hostile 状态或 tunable。

## Migration Plan

1. 先落地队列、预算和重扫的失败测试，再实现独立状态，不接生产触发点。
2. 接入流体 membership 和成功翻地触发点，验证同 tick 结果与拒绝路径。
3. 插入独立阶段并删除随机作物阶段的耕地分支，更新成本指标和 benchmark。
4. 运行 `internal/sim` race 测试、架构测试、全仓 race/vet、相关 benchmark 与 OpenSpec 严格校验。

无需数据迁移或灰度开关。部署后既有湿/干编号保持有效；首次 active Ready 重扫最终纠正陈旧状态。回退时删除新状态与阶段、移除流体写入、玩家放置与成功翻地三个候选生产挂点，并恢复 `advanceCropCell` 的耕地分支，存档仍可直接读取。

## Affected Files

- 新建：`internal/sim/farmland_moisture.go` 及按 queue/budget/rescan/integration 关注点组织的测试。
- 修改：`internal/sim/fluid.go`、`engine_placement.go`、`farming.go`、`crop.go`、`engine.go`、`engine_step.go`。
- 修改测试/测量：`crop_test.go`（按关注点拆分）、`farmland_moisture_integration_test.go`、`farmland_moisture_queue_test.go`、`crop_perf_test.go`、`fluid_perf_test.go`、`companion_action_test.go`；B-07 重叠仅限 `fluid_crop_test.go` 与共享 `helpers_test.go` 的湿度挂点证明；Task 5 修复另含 `internal/server/farming_loop_e2e_test.go` 的即时湿润期望与准确注释。
- 注释-only 受控重叠：`internal/sim/tunables.go` 的 `RandomTicksPerSection` 说明。
- 规划与记录：本 change 目录、`docs/superpowers/specs/2026-08-26-instant-farmland-moisture-design.md`、`docs/superpowers/plans/2026-08-26-instant-farmland-moisture.md`、执行 ledger。
