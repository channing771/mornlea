# Design: rust-engine-fluid

## Context

见 proposal.md「Why」。现状(2026-08-30,realm 已合入 main):`internal/fluid` 全部为
Go 生产实现——`Queue` 为索引最小堆(`order`/`index` 双射,过时条目结构性不可能),
`Advance(now, w, budget, delay)` 两阶段:阶段一只读求值并合并进 `pendingWrites`
(`strongerWrite` 可交换可结合),阶段二按 `lessPos` 全序一次性提交并对变更格及六邻
再入队;`evalCell`/`Replaceable`/`flowingSurvives` 为纯整数规则。生产重扫路径位于
`internal/sim/realm/environment.go`:`State.AdvanceFluids` → `State.runFluidRescans` →
`State.rescanChunkFluids` → `State.enqueueChunkFluids`(两级不动点捷径:
非流体均匀段 1 格、水源均匀段 1 格、逐格 1 格记账),由
`internal/sim/runtime/engine_step.go` 的权威 tick 直调;`internal/sim/runtime/fluid.go`
存有同名测试镜像,非测试代码零调用,不属本次迁移范围。engine 现有 ABI v8
(mesh/light/collision/step/worldgen/raycast/LOD shell)。

## Goals / Non-Goals

- Goals:两类稠密循环(规则求值、重扫扫描)迁入 engine;行为逐位不变(变更集、
  世界写入、再入队集合、重扫入队位置与 dueTick、记账三档);Go 保留全部状态与
  编排;oracle 差分 + golden + fuzz + 0-alloc 门禁。
- Non-Goals:见 proposal.md「非目标」;另不做重扫盒的跨 tick 缓存,不做
  Palette 直通(布局已预留,实测超预期再启用)。

## Decisions

### 两个 ABI 入口,engine ABI 8→9(additive)

与 `mornlea_raycast_batch`/`mornlea_lod_shell` 同款形态:调用方持有全部
input/scratch/output buffer,`#cgo noescape nocallback`,失败返回状态码,panic
收敛为 `MORNLEA_STATUS_PANIC`,engine 保证失败时不触碰输出。

**`mornlea_fluid_eval_batch`(批量规则求值,布局 v1)**

- 输入 = `u32 layout_version=1`(LE)+ `u32 item_count` + 每项 14 字节:7 个
  `u16` LE 方块编号,槽位序 0=自格、1=上、2=下、3=+x、4=−x、5=+z、6=−z,与
  `internal/fluid` 的 `sixNeighbors` 同序。方块编号是协议稳定值,与
  `internal/core/block.go` 的 iota 逐一对应(实现时两侧常量以代码核对、注释互指)。
- 输出 = 每项 12 字节:4 条候选写入 × 3B(目标槽位 u8(0..6;0xFF=无写入哨兵)+
  BlockID u16 LE)。同一项内至多 4 条(垂直优先 1 条或水平传播 4 条或自格消亡
  1 条)。
- `input_len` 必须等于 8 + item_count×14,容量不足返回
  `MORNLEA_STATUS_INVALID_ARGUMENT`(输出尺寸是输入的确定函数,无需两段式);
  `layout_version`/`item_count` 违约返回 `MORNLEA_STATUS_INPUT` 且不写输出。
- Rust 内完整实现 `evalCell` 三段(陈旧项跳过、存活判定、垂直优先即返、水平
  传播等级 +1 且 ≤7)与 `Replaceable` 判定表(空气/作物/开启门四态/源不可替换/
  弱水可被强水替换);作物与门表按 worldgen 材料表先例在 Rust 侧建表。

**`mornlea_fluid_rescan`(重扫扫描,magic 语义 `MFL1`,布局 v1,三段)**

- header(26B):`u32 layout_version=1 | i32 center_chunk_x | i32 center_chunk_z |
  u16 x0 | u16 x1 | u16 z0 | u16 z1`(盒内局部列 0..17,含裙边;即 plane 0 整块
  或边界条带)`| u8 start_section(0..23) | u8 reserved=0 | u32 budget`。
- 中心区块 24 区段记录(按 y 区段 0..23):`u8 kind`(0=均匀)与 `u8 pad`;
  kind=0 记 `u16 uniform_id`(记录共 4B),kind=1 展开为 4096×u16 LE,区段内序
  `x + z*16 + y16*256` 与 Go `blockIndex` 一致。
- 裙边 68 列 × 384 u16:列序固定 `(x=-1,z=0..15)`、`(x=16,z=0..15)`、
  `(z=-1,x=0..15)`、`(z=16,x=0..15)`、四角 `(-1,-1)`/`(16,-1)`/`(-1,16)`/
  `(16,16)`;列内 y 0..383。未就绪邻块列填 `BarrierID`。
- 元数据 9 区块 × 24 区段 × 3B(`u8 uniform_flag + u16 id`,flag=0 时 id=0),
  区块序:中心、`(-1,-1)`、`(0,-1)`、`(1,-1)`、`(-1,0)`、`(1,0)`、`(-1,1)`、
  `(0,1)`、`(1,1)`,供区段级不动点捷径判定。
- 盒内局部坐标:中心区块局部 `(lx,lz) ∈ 0..15` 映射盒 `(lx+1, lz+1)`;y 全高
  0..383 对应世界 `y_base + 0..383`(`y_base = core.MinY`,由 Go 编码方保证)。
- 输出:positions 追加(`u32 x、u32 y、u32 z` LE 世界坐标)+ 尾部 summary
  `u32 spent | u8 done | u8[3] pad`;容量不足两段式返回 `OUTPUT_OVERFLOW` 并
  报告所需字节数,output 未被触碰,由调用方扩容重试。
- 调用粒度 = (区块, 平面) 扫描单元,区段游标续扫语义与现行 (plane, section)
  游标逐字对应;邻块未就绪的平面在 Go 侧跳过、不调 kernel、不记额度,同现行。

### 调用时序与盒组装

- eval 侧等价性依据:现行 `Advance` 阶段一本就把写入延后到阶段二(`evalCell`
  只读,`queue.go` 注释已论证读写隔离),故「先按现行循环弹出 ≤budget 项(全序、
  到期检查、探视守卫、budget 计数)、再一次批量求值、解码后仍经 `strongerWrite`
  并入 `pendingWrites`」与逐个求值逐位等价。7 格邻域编码直接调用现行
  `fluidWorld.BlockAt`(scope 外/未就绪 → Barrier),不写第二份读路径。
- 盒组装粒度 = 每区块一次:`State.rescanChunkFluids` 入口现场组装(中心区段经
  `IsUniform`/线性 `idAt` 采样,裙边列取就绪邻块数据或 Barrier,元数据 9 区块),
  平面循环内只改 header 的 x0..x1/z0..z1/start_section/budget 复用同一份盒体;
  不跨 tick 缓存,语义与现行逐 tick 读世界一致,免去陈旧性论证。

### 状态与编排刻意留 Go

队列、游标、预算、`strongerWrite` 同 tick 冲突合并(次序无关性的承重点,合并量
≤4×budget,不在热点上)、`lessPos` 排序提交、`settleFloodedCrop` 冲毁结算、
`recordChange` 广播、变更再入队全部留 Go;接线后生产代码对 `fluid.Replaceable`
的调用清零(本体保留在 `internal/fluid/rules.go` 作为冻结判定面,供 oracle 与
性质测试使用)。

### 旧 Go 实现降为测试 oracle

`evalCell`/`flowingSurvives` 移入 `internal/fluid` 的 oracle 测试文件,重扫的
`enqueueChunkFluids`/两级不动点判据移入 realm 的 oracle 测试文件;新增逐位差分
(eval 任意 7 格组合;rescan 均匀/混杂/海洋/区块边界/未就绪/budget 中断)、golden
vectors(垂直流、水平扩散、多源汇合取最强、作物冲毁、门四态、世界边界 Barrier、
陈旧项跳过)、eval fuzz 不变量与 0-alloc 锁定。现有性质测试与 e2e 走公共 API,
公共 API 已路由 Rust,自动成为行为回归网。oracle 删除沿 `drop-go-test-oracles`
先例留给后续独立 change。

### 失败语义

两个 kernel 的非 OK 状态码以稳定中文文案 panic,沿 `internal/physics/step.go`/
`internal/worldgen/generator.go` 先例:输入缓冲由 Go 编码、长度编码时即知,非 OK
只能是契约 bug。不走 mesh 的客户端降级路径——流体没有「少渲染一帧」式安全降级,
静默吞掉写入才是真正的行为改变。

## 依赖与并发

- 依赖方向:`internal/fluid → internal/nativeabi`(先例:`internal/physics`、
  `internal/worldgen` 直连),`internal/archcheck` allowed 表登记新边;
  `internal/fluid → internal/core` 既有边不变。rescan 侧 nativeabi 边不进 sim:
  realm 组装盒,交 `internal/fluid` 导出的扫描包装(`ScanRescanRegion`)执行。
- 两个 kernel 均无状态纯函数,可并发调用;`Queue` 的 eval scratch
  (`evalInput`/`evalOutput`)与重扫 scratch 为调用方私有,所有权与锁边界不变,
  权威 tick 内调用时序不变。

## 兼容与回退

- 仅 engine ABI bump v8→v9;协议、存档 schema、client ABI、benchmark scenario
  零触碰,版本互斥规则自动满足;Go 与 `libmornlea_engine` 仍为不可跨版本混装的
  release unit,`$ORIGIN` 约定不变;ABI 握手拒绝混装测试沿 `native_test.go` 先例。
- 存档字节不变;旧世界加载后流体继续按相同规则演化(oracle 差分 + golden 锁定)。
- 回退:单 PR revert;oracle 即旧实现副本,可随时移回生产。

## 平台假设

两个 kernel 只含整数表运算,无浮点,无 physics 那类 arm64 float 逐位问题;Go/Rust
在全平台逐位一致,差分与 golden 门禁全平台启用。

## 风险

- **邻域盒编码 bug**:裙边 8 区块、双态区段,错一格即行为分叉——逐位差分 + 边界
  地形 golden 兜住;编码器单测覆盖 8 个裙边方向与未就绪邻居。
- **重扫组装成本**:每区块一次组装、平面循环复用,全展开最坏 ≈244KB,均匀段占
  绝对多数时实际远小;数值 record-only,若实测超预期,布局已预留 Palette 直通
  路径。
- **ABI 混装**:握手拒绝混装测试覆盖;`make rust` 重建纪律防陈旧 dylib 静默挂起。
- **基线缺失**:`internal/fluid` 现无 micro-bench(见 ledger 基线记录),迁移收益
  对照以首次 oracle 差分 bench 为基线,scenario 数值只记录。
