# Rust Engine 流体迁移设计

- 日期:2026-08-29
- 状态:设计已逐节评审确认(边界方案 A、时序「等 realm 合并」均为用户裁决)
- 后续:writing-plans 产出实现计划;执行启动以 `refactor/sim-subpackages`(realm)合入 main 为前提

## 背景与动机

`docs/notes/go-rust-division.md` 的判定规则:≥ 数千元素的同种数值变换归 Rust,生产路径不得出现体素级 O(N) 密集循环。engine ABI v8 已承载 mesh/light、collision、physics step、worldgen、raycast、LOD shell 七个无状态纯函数;流体是权威 tick 内剩下的最后一块大体素级纯 Go 循环:

- **重扫扫描**(`internal/sim/fluid.go` 的 `enqueueChunkFluids` + 两级不动点捷径):每 tick 预算 `FluidRescanCellsPerTick`(默认 65536 格)的逐格 `Blocks.Get` + `IsFluid` + 水源五邻检查,跨区段邻格读还要付 `dimension.records` map 查找。
- **规则求值**(`internal/fluid` 的 `evalCell`):每 tick 预算 `FluidUpdatesPerTick`(默认 512 项),每项 1 次 map 分配 + 至多 7 次经 `fluidWorld` 的跨 map 世界读。

队列、游标、预算、冲突合并等**状态与编排全部留在 Go**(见目标边界);本次只把「稠密扫描」与「逐格规则表」两类数值工作迁入 `mornlea_engine`。遵循既有先例:mesh+light 一 change 双 kernel、迁移期 oracle 差分门禁、性能数值 record-only。

## 现状事实(设计依据)

- `internal/fluid`:`Queue` 是索引最小堆(`order`/`index` 双射,过时条目结构性不可能);`Advance(now, w, budget, delay)` 两阶段——阶段一只读求值并合并进 `pendingWrites`(`strongerWrite` 可交换可结合,结果与处理次序无关),阶段二按 `lessPos` 全序一次性提交并对变更格及其六邻再入队。`evalCell`/`Replaceable`/`flowingSurvives` 为纯整数规则(源不死、存活判定、垂直优先、水平递减 ≤7、作物/门/流体等级替换表)。
- `internal/sim/fluid.go`:`fluidWorld` 适配器把 scope 外、未就绪、世界高度之外的格读作 `core.BarrierID`(封闭边界,防假开口死循环);`fluidRescanState` 待办队列 + (plane, section) 续扫游标,重扫跨 tick 分摊;`enqueueChunkFluids` 两级捷径按「非流体均匀段 1 格、水源均匀段不动点 1 格、逐格 1 格」记账,单次调用至多超支一个区段;全部权威方块写入汇聚在 `recordChange` → `enqueueFluidUpdate`(自格 + 六邻)。
- `internal/world.PalettedContainer`:uniform / indexed palette / direct 三态存储,`IsUniform` O(1)。
- `internal/nativeabi`:无持久句柄,调用方持有全部 input/scratch/output buffer,`#cgo noescape nocallback`;physics/worldgen 对非 OK 状态码以稳定中文文案 panic(engine 保证失败时不触碰 output)。
- benchmark scenario v19 不注水,场景内流体活动极少;流体收益集中在重扫活跃期(区块成批进入推进范围)与水活跃世界,因此数值只记录、不设门禁。

## 目标边界

### engine ABI v8 → v9(additive,两个新无状态纯函数)

与 `mornlea_raycast_batch` 同款形态:调用方持有全部 buffer,失败返回状态码,panic 收敛为 `MORNLEA_STATUS_PANIC`。

**① `mornlea_fluid_rescan`(重扫扫描内核)**

- 输入:一个**邻域盒**——以本扫描单元被扫区块为中心的 18×384×18 体素盒(中心区块全量 + 四侧 1 格裙边;裙边取自周围 8 个区块的贴面列,不可达读 `BarrierID`);五段平面各自以自己的被扫区块为盒中心组盒(每区块重扫至多 5 次编码)——共享一份以重扫目标区块为中心的盒会把边界平面条带放到最外裙边列上,五邻不动点读越出盒外,区段级不动点也会错拿中心区块记录描述被扫邻块。区段压缩表示:均匀段 = 标记 + ID(4B),非均匀段展开 4096×2B。附小元数据表:被扫区块与周围 8 个裙边贡献区块(共 ≤9)各 24 区段的 uniform 位与均匀 ID(≈650B),供区段级不动点捷径判定。
- 输入另含扫描区域说明(x0..x1、z0..z1 列范围,即 plane 0 的整块或 `fluidBoundaryPlanes` 的边界条带;后者以邻块为盒中心,条带落在该盒的中心列)、起始区段游标与剩余额度。
- 输出:应入队坐标列表(紧凑变长)+ 实耗额度 + 区域是否扫完。Rust 内实现与现行逐字一致的记账(非流体均匀段 1 格、水源均匀段不动点 1 格、逐格 1 格;区段开始前查额度,单次调用至多超支一个区段)。
- **调用粒度 = (区块, 平面) 扫描单元**,区段游标续扫语义与现行 (plane, section) 游标逐字对应;邻块未就绪跳过且不记额度,同现行。
- **邻域盒每次调用现场组装**(读现行区块数据),不跨 tick 缓存——语义与现行逐 tick 读世界完全一致,免去陈旧性论证;组装成本为一次性 memcpy 量级,远低于 tick 预算。
- Go 保留:`fluidRescanState` 待办与游标、`FluidRescanCellsPerTick` 预算、`Queue.Enqueue`、从 `dimension.records` 组装邻域盒的编码器。

**② `mornlea_fluid_eval_batch`(规则求值内核)**

- 输入:≤`FluidUpdatesPerTick` 项 × 固定 7 格 block ID(自格 + 六邻,按现行 `sixNeighbors` 槽位序:上、下、+x、−x、+z、−z),2B/格。
- 输出:每项至多 4 条写入(目标槽位 ∈ {自格, 六邻} + BlockID),每项写集定长排布,精确字节布局在 ABI header 定稿。Rust 内完整实现 `evalCell` 三段逻辑(陈旧项自格非流体跳过、非源存活判定、垂直优先即返、水平传播等级 +1 且 ≤7)与 `Replaceable` 判定表(空气/作物/开启门四态/源不可替换/弱水可被强水替换);作物与门表按 worldgen 材料表先例在 Rust 侧建表。
- Go 保留:`Queue` 索引堆与 dueTick 全序、`pendingWrites` 的 `strongerWrite` 合并、按 `lessPos` 排序提交、`settleFloodedCrop` 冲毁结算、`recordChange` 广播、变更再入队。**同 tick 冲突合并刻意留 Go**:它是次序无关性的承重点,且合并量 ≤4×budget,不在热点上。

### 调用点与依赖边

- `internal/fluid` 新增对 `internal/nativeabi` 的依赖(先例:`internal/physics`、`internal/worldgen` 直连 nativeabi):`Queue.Advance` 内把逐格 `evalCell` 换成一次批量调用。
- 重扫侧:sim(届时为 `sim/realm`)组装邻域盒,交 `internal/fluid` 导出的扫描包装函数执行;`nativeabi` 边不进 sim。
- `internal/archcheck` allowed 表登记新边;`internal/fluid → internal/core` 既有边不变。

## 行为不变性与失败语义

**冻结面(逐位不变)**

- `Advance` 在相同队列内容与相同世界状态下返回的变更集、世界写入、再入队集合逐位一致;重扫产生的入队位置与 dueTick 逐位一致。
- Go 侧编排零改动:dueTick 全序、`strongerWrite`、`lessPos` 提交序、`settleFloodedCrop`、`recordChange`、tunable 快照;`FluidUpdatesPerTick`/`FluidRescanCellsPerTick`/`FluidFlowDelayTicks` 语义零改动。
- 调用时序等价性依据:现行 `Advance` 阶段一本就把写入延后到阶段二(`evalCell` 只读,`queue.go` 注释已论证读写隔离),故「先按现行循环弹出 ≤budget 项、再一次批量求值」与「逐个求值」逐位等价。
- 7 格邻域编码直接调用现行 `fluidWorld.BlockAt`(scope 外/未就绪 → Barrier),不写第二份读路径;重扫盒的裙边读语义与现行 `fluidRescanBlockAt` 一致。
- Rust 侧纯整数表运算,无浮点,无 physics 那类 arm64 float 逐位问题;确定性由 oracle 差分 + golden 锁定。
- 预算记账逐字保留(含区段级捷径随扫描进 Rust):海洋区块重扫额度消耗保持 1 格/段,重扫推进节奏与水开始流动的时机不变。

**失败语义**

两个 kernel 的非 OK 状态码以稳定中文文案 panic,沿 `internal/physics/step.go`/`internal/worldgen/generator.go` 先例:输入缓冲由 Go 编码、长度编码时即知,non-OK 只能是契约 bug;engine 保证失败不触碰输出。不走 mesh 的客户端降级路径——流体没有「少渲染一帧」式安全降级,静默吞掉写入才是真正的行为改变。

**Oracle 策略**

`evalCell`/`flowingSurvives`/`enqueueChunkFluids`/两级不动点判据转为 test-only oracle;`Replaceable` 判定表保留在 `internal/fluid/rules.go` 作为冻结判定面,生产路径零调用;新增逐位差分测试与 golden vectors;现有全部性质测试(`property_converge`/`property_order`/`property_budget`/`property_rescan`、`e2e`、`queue_bounded`)原样保留——它们走 `internal/fluid` 公共 API,公共 API 已路由 Rust,自动成为行为回归网。oracle 删除按 `drop-go-test-oracles` 先例留给后续独立 change。

## 验证门禁

1. Rust crate 内单测:规则表、不动点判据、盒编码解析、记账;门禁 `make rust-check`。
2. Go oracle 逐位差分:eval 批量(任意 7 格组合)与重扫入队集合(均匀/混杂/海洋/区块边界地形)。
3. golden vectors:垂直流、水平扩散、多源汇合取最强、作物冲毁、开启/关闭门、世界边界 Barrier、陈旧项跳过。
4. eval kernel fuzz:任意输入下不变量(只写空气/作物/更弱流体、等级 ≤7、源不可被写入)。
5. 0-alloc 锁定:批量求值路径零堆分配(physics 先例);重扫编码器 scratch 复用。
6. 现有性质测试 + e2e + Memory/TCP parity 原样保留;ABI 握手拒绝混装测试沿 `native_test.go` 先例。
7. 性能证据(record-only):ledger 记迁移前基线(`internal/fluid` 定点 bench + benchmark v19 权威 tick),迁移后同口径复测;重扫微基准为主要证据;数值只记录,不改变退出状态。

## 文档与 ABI 工程纪律

- ABI 四处同步:C header、crate `ffi.rs`、`internal/nativeabi`、版本/容量测试;FFI 契约照 `engine/AGENTS.md`(校验指针/长度/对齐/重叠、失败不留部分输出、panic 收敛状态码)。
- **仅 engine ABI bump(v8→v9)**:协议、存档 schema、client ABI、benchmark scenario 零触碰,版本互斥规则自动满足。
- 文档同步:`docs/notes/go-rust-division.md` 领域表加流体;`docs/architecture.md` 边界描述;`docs/notes/test-quickstart.md` 定点命令;`internal/fluid` 局部 AGENTS.md(说明 nativeabi 边与 oracle 地位)。
- 新 worktree 先 `make rust`(共享 CARGO_TARGET_DIR 回拷);重型门禁前后重建。

## 执行时序与骨架

前提:`refactor/sim-subpackages` 合入 main(fluid/crop/farmland 落位 `internal/sim/realm`);本设计不依赖 realm 内部结构,只依赖调用点所在包名落定。

SDD 流程(fresh implementer + 双评审,进度与裁决入 ledger):

- T1:OpenSpec change 产物 + 基线快照(现有流体 bench 计时入 ledger)。
- T2:eval kernel(Rust 实现 + ABI 出口 + nativeabi 绑定 + 差分/golden/fuzz/0-alloc + `Advance` 接线)。
- T3:rescan kernel(盒编码器 + Rust 扫描 + 记账 + 差分 + sim 侧接线)。
- T4:文档门禁收尾(AGENTS.md/archcheck/division/architecture/test-quickstart + dev-check + test-race + rust-check)。
- 终审 whole-branch review → PR → CI → 合并 → openspec archive。

## 风险

- **邻域盒编码 bug**:裙边来源 8 个区块、压缩区段双态,编码错一格即行为分叉——由逐位差分 + 边界地形 golden 兜住;编码器单测覆盖 8 个裙边方向与未就绪邻居。
- **realm 合并时间不可控**:设计已把调用点约束在包边界上;若 realm 落点变化,仅 T3 接线 hunk 需要调整。
- **重扫组装成本**:现场组装邻域盒(每区块重扫至多 5 次,每 (区块, 平面) 扫描单元一次),单盒全展开最坏 ≈250KB(26 + 24×8194 + 68×384×2 + 648 ≈ 249.6KB;均匀段占多数时实际远小),仅发生在该区块的重扫窗口内;若实测超预期,压缩编码已预留 Palette 直通路径。
- **ABI 混装**:握手拒绝混装测试覆盖;`make rust` 重建纪律防陈旧 dylib 静默挂起。

## 验收标准

- 行为零变化:差分、golden、fuzz、性质、e2e、Memory/TCP parity 全绿。
- `make rust`/`make rust-check` 绿;ABI v9 握手与容量测试就位;`go build ./...` 全绿。
- archcheck 新边登记并通过;openspec strict 通过。
- 性能数值记录入 ledger 与 `docs/notes/perf-baseline*`,不设门禁。
