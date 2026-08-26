# 耕地湿润即时化设计（B-09）

日期：2026-08-26；认领分支：`feat/B-09-instant-farmland-moisture`。

## 1. 背景

`authoritative-farming` 当前只在随机 tick 抽中耕地时扫描水平半径 4、同层或上一层的
`9×9×2 = 162` 个方块并更新干湿。该方案天然受随机 tick 样本数约束，但留下两个已归档
遗留：放水/失水后的状态有随机延迟；密集耕地会让随机作物阶段的单样本墙钟成本从一次读取
上升到最多 163 次读取，而既有 `cropCellsExamined` 只计样本格数，不能反映真实成本。

本变更用显式、有界、可恢复的湿度更新队列取代随机 tick 中的耕地扫描。普通单点流体变化
应在同一权威 tick 的流体阶段后更新附近耕地；极端批量变化必须服从固定读取预算并确定性
顺延，不能为“即时”引入无界 tick 工作。

## 2. 目标与非目标

### 2.1 目标

- 流体“有/无”变化主动唤醒所有可能受影响的耕地；
- 成功翻地后主动判定新耕地湿度；
- 每 tick 湿度阶段最多执行 65,536 次方块读取，超额待办不丢失；
- 队列不持久化，重启和区块重新进入 active 范围时通过预算化重扫恢复；
- 随机作物阶段不再扫描耕地湿度，成本以真实方块读取次数计量；
- 保持现有半径、层数、缺失邻块判干、双向转换与方块广播语义。

### 2.2 非目标

- 不增加耕地方块编号、附加状态字节、湿度值或干耕地退泥土计时；
- 不改变流体求值、冲毁作物规则、流体队列或其不动点论证；
- 不把预算做成 tunable，不增加配置项；
- 不修改协议、存档 schema、ABI、benchmark scenario、capture golden 或长期基线文档。

## 3. 设计原则

权威 tick 仍是世界方块与队列状态的唯一写者。湿/干方块编号继续是湿度事实的唯一持久化
表示；队列只负责决定何时重判，不成为第二份湿度状态。

所有顺序都来自现有全序输入或固定循环，不遍历 map 决定行为。所有预算按规则为作出决定
而执行的 `Dimension.BlockAt`/区块方块读取计数，不以“候选数”代替实际工作；写入助手为
防御性比较旧值所做的内部读取不重复计费。

## 4. 运行期状态

`Engine` 只追加一个聚合字段 `farmlandMoisture farmlandMoistureState`。状态包含：

- 候选 FIFO 与已入队集合；键为 `(DimensionID, BlockPos)`；
- 按 `ChunkKey` 登记并去重的重扫 job；
- 当前重扫 job 的坐标游标；
- 最近一 tick 的湿度阶段方块读取计数，供测试和 benchmark 读取。

FIFO 用切片保存插入顺序、map 只做存在性查询。消费后删除 map 键；切片前缀按固定阈值压紧
以避免长期保留已消费项。任何行为都不得依赖 map 遍历顺序。

队列与重扫状态不入存档、不进快照。候选按坐标去重；每次流体 membership 变化最多生产
162 个反向候选，离开 active Ready scope 的候选在消费时丢弃，重入由重扫恢复。

## 5. 生产触发点

### 5.1 流体 membership 变化

`fluidWorld.SetBlock` 在写入前已有旧方块、写入参数是新方块。仅当
`core.IsFluid(old) != core.IsFluid(new)` 时唤醒湿度队列：

- 流体等级之间互换不改变“附近是否有流体”，不得入队；
- 空气/作物等非流体变流体、流体退为空气均入队；
- B-07 对 `internal/fluid` 规则的修改仍经 `fluidWorld.SetBlock`，无需第二挂点。

若变化流体位于 `(x,y,z)`，可能受影响的耕地恰好是：

```text
farmlandY ∈ {y-1, y}
farmlandX ∈ [x-4, x+4]
farmlandZ ∈ [z-4, z+4]
```

共最多 162 格。入队按 `farmlandY`、`z`、`x` 升序，越出世界 Y 边界的位置跳过。重复位置
保留首次入队顺序，不追加第二份。

### 5.2 新耕地

`executeTillSoil` 成功把泥土/草改成干耕地后，只把目标格自身入队。它不展开 162 格，因为
一格新耕地不会改变其他耕地的湿度。

湿耕地与干耕地互转不再次入队；湿度方块不是流体，其变化也不影响邻格，禁止形成反馈环。

当前没有水桶或其他玩家可写流体路径。未来新增流体写者时，必须复用“old/new membership
变化”挂点，而不是依赖普通 `recordChange`；该约束由挂点枚举测试守卫。

## 6. tick 阶段顺序

`Step` 增加独立 `phaseFarmlandMoistureAdvance`，顺序固定为：

```text
advanceFluids
advanceFarmlandMoisture
advanceCrops
finishChanges
```

流体先推进，使本 tick 的最终水位先产生候选；湿度阶段再读取该状态并写湿/干耕地；作物
随机 tick 最后只读取正下方的湿/干方块。全部写入继续汇入同一份 `pendingChunkChanges`，
与流体、作物及其他方块变更共享一次 revision、广播和存盘。

该阶段单独登记，便于顺序测试和 benchmark 归因；不得折入流体或作物阶段。

## 7. 候选处理与读取预算

`farmlandMoistureReadsPerTick` 固定为 65,536。每 tick 先处理事件候选，再用剩余预算推进重扫。
预算不是 tunable：数值是正确性与性能契约，不占用 A-04 独占的 `tunables.go`。

候选处理规则：

1. 候选所属区块不在本 tick active Ready scope 时，删除该候选；重新进入由重扫恢复；
2. 读取候选格一次并计费；非耕地立即删除；
3. 若为耕地但剩余预算不足最坏 162 次邻域读取，保留在队首并结束本 tick；
4. 否则按既有 `dy,z,x` 顺序扫描湿润窗口，每次 `BlockAt` 都计费，遇到首个流体可提前结束；
5. 计算出的湿/干编号与当前相同则只删除候选；不同则写方块并经 `recordChange` 汇入 pending；
6. 写入失败时删除本项并记录结构化错误，不广播未落地的变化。

步骤 3 可能让本 tick 最多闲置 161 次读取额度，但保证一次湿度判断基于单一 tick 的一致世界
截面，不保存跨 tick 的半次扫描结果。候选初读后才发现额度不足时，该次读取仍计费，下一
tick 会重新读取；预算始终不超限。

一次孤立流体 membership 变化即使 162 个候选全是耕地，最坏读取数也为
`162 × (1 + 162) = 26,406`，低于单 tick 预算，因此在没有旧积压时同 tick 完成。批量变化
按 FIFO 顺延，不能绕过预算。

## 8. active scope 与恢复重扫

湿度阶段复用紧邻其前的 `advanceFluids` 已构造完成的 `engine.fluidScope`；该集合正是本 tick
active interest 与 Ready 区块的交集，不再维护第二套 previous/next map。`advanceFluids` 在
稳定的新 scope key 循环里同时登记独立湿度重扫 job；job 按登记顺序执行并去重，不读取或
修改流体重扫游标。

每个 job 扫描目标区块的完整 Y 范围和水平 4 格 halo，即
`24×24×core.SectionsPerChunk×core.SectionSize` 个位置，
坐标顺序固定为 `y,z,x`。每个位置读取一次并计费；读到耕地且该耕地自身所属区块位于当前
active Ready scope 时，把它加入候选 FIFO。halo 使新进入区块里的水能唤醒相邻已 active
区块边缘的耕地，也使新进入区块里的耕地读取相邻水体。

重扫用事件候选处理后的剩余预算，可跨 tick 保存游标。重扫产生的候选从下一 tick 开始按
FIFO 处理；未扫到前保留存档中的既有湿/干方块，不同步全扫、不伪造中间状态。

事件持续优先于重扫。当前流体规则在活动范围内收敛且无自持更新环，事件流最终停止，因此
不会永久饿死重扫；若未来引入永久流体源事件，必须重新裁决调度公平性。

## 9. 随机作物阶段与成本契约

`advanceCropCell` 删除耕地分支；随机 tick 只处理作物。作物仍读取抽中格一次，抽中作物时
再读取正下方方块一次，并读取既有列顶高度图。高度图访问不计作“方块读取”。

保留 `cropCellsExamined` 作为固定样本数证据，同时新增/改用方块读取指标：

```text
crop block reads <= 2 × readySections × RandomTicksPerSection
farmland moisture block reads <= 65,536
```

因此农业两阶段单 tick 的方块读取总上界只依赖 Ready section 数、固定随机样本数和固定湿度
预算，不随世界中作物或耕地总量增长。密集农田不再让随机作物样本额外扫描 162 格。

`crop_perf_test.go` 同时报告 `cells/op` 与 `block_reads/op`，并以解析式上界作为 correctness
门禁；墙钟数值只记录。benchmark scenario 不变，因为固定 benchmark workload 定义未变，
本变更正是同场景下被比较的实现变化。

## 10. 正确性边界

- 湿润几何保持水平切比雪夫距离 4、同层或上一层存在任意流体；
- 邻格未 Ready 时仍按无水处理；进入 Ready active scope 后由 halo 重扫纠正；
- 同 tick 多次流体变化只保留一个候选，处理时读取最终方块状态，结果与中间写入次序无关；
- 多格耕地处理互不影响湿润查询，FIFO 顺序只决定超预算时的完成先后；
- 队列溢出预算时不丢项，重启时不恢复队列但恢复后的 scope 重扫最终重建正确状态；
- 作物始终只读取已落在方块编号里的湿度，不引入第二份缓存或推测状态。

## 11. 兼容与回退

湿/干状态仍使用既有 `FarmlandDryID`/`FarmlandWetID` 并随区块存档。协议保持 v26，区块
schema v9、玩家 schema v7、metadata v2、`companions.ai` v4、engine/client ABI 与 benchmark
scenario 均不变，无数据迁移。

回退时删除湿度队列与阶段，并恢复 `advanceCropCell` 的耕地随机扫描即可。已保存的湿/干
方块在新旧实现中都合法；回退不需要改写存档。

## 12. 测试与验收

### 12.1 纯规则与队列

- 反向窗口精确覆盖水平距离 4、`y-1/y` 两层，距离 5 与越界 Y 不入队；
- 流体↔非流体触发，流体等级↔流体等级不触发；
- FIFO 插入/去重/消费顺序确定，同一输入重放得到相同顺序和方块结果；
- 单 tick 读取计数从不超过 65,536，额度不足的队首保留且下一 tick 继续；
- 大于预算的待办全部最终处理，无丢失、重复写或回绕。

### 12.2 tick 集成

- 放水与失水在无积压时同 tick 使半径内耕地变湿/干；
- 水平距离 5、下一层流体仍不湿润；跨区块半径边界保持；
- 翻地成功同 tick 判湿，拒绝翻地不入队；
- 流体→作物冲毁等 B-07 路径仍通过同一 membership 挂点；
- 阶段顺序固定为 fluid→moisture→crop，作物读取同 tick 新湿度；
- 候选离开 active scope 后被丢弃，重入后重扫恢复；
- 重启后队列为空，但预算化重扫最终纠正存档中的陈旧湿度；
- 新 Ready 区块的 halo 能纠正邻块边缘耕地。

### 12.3 成本与回归

- barren、planted、dense 三种夹具样本数相同，方块读取各自不超过解析式上界；
- 全耕地随机阶段不再执行 162 格湿润扫描；
- moisture 事件与重扫合计读取不超过固定预算；
- 运行既有 farming、fluid、trample、作物增长与跨区块测试，证明行为无回归。

### 12.4 门禁

```bash
go test ./internal/sim -race -count=1
go test ./internal/archcheck -count=1
go test ./... -race
go vet ./...
gofmt -l .
go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5
openspec validate --all --strict --no-interactive
```

性能数值只记录，不通过放宽阈值消除失败。报告完整性、真实 overflow、数据丢失和 I/O 错误
仍是门禁。

## 13. 否决方案

- **每耕地维护邻水计数缓存**：更新快，但新增大 map、计数失效、区块加载重建和更多写入
  挂点；湿/干方块已经是缓存，不需要第二份事实。
- **所有 `recordChange` 都展开 162 个候选**：会让采掘、作物生长、踩踏等无关变化制造
  大量待办；只挂真实流体 membership 变化与新耕地。
- **保留随机 tick 作为恢复兜底**：无法消除密集耕地单样本 162 次读取，也不能给恢复延迟
  明确预算。
- **加载时同步全扫**：大批区块同时 Ready 会击穿 tick；必须用可续游标。
- **严格要求所有变化同 tick 完成**：批量流动时工作量随变化数增长，违背权威 tick 有界性。
- **持久化队列**：湿/干事实已持久化，重扫可恢复；升级 schema 只保存调度状态，收益不足。

## 14. 认领范围裁决

最初认领备注把 `engine_changes.go` 列为中央挂点。设计核实后否决：该函数不知道写入前的旧
方块，若对所有变化展开候选，会把无关写入放大 162 倍。控制会话批准把挂点改为
`internal/sim/fluid.go` 的 old/new membership 转换与 `farming.go` 的成功翻地，并补充
`fluid_perf_test.go`/`companion_action_test.go` 的阶段边界守卫。随机作物阶段不再维护干湿后，
`tunables.go` 的既有注释会失真，故另批准仅更正 `RandomTicksPerSection` 说明的受控重叠，字段、
默认值与校验不动。
