# Rust Engine 流体迁移实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把流体系统的两个体素级纯 Go 循环——重扫扫描与逐格规则求值——迁入 Rust `mornlea_engine`(engine ABI v8→v9 双内核),Go 保留全部状态与编排,行为逐位不变。

**Architecture:** 两个无状态纯函数 kernel(`mornlea_fluid_eval_batch`、`mornlea_fluid_rescan`)以调用方持有 buffer 的既有形态接入 `internal/nativeabi`;`internal/fluid` 的 `Queue.Advance` 改为先弹队再一次批量求值,sim 侧重扫组装邻域盒交 `internal/fluid` 导出的扫描包装。Go 实现降级为 test-only oracle,差分 + golden 锁逐位一致。

**Tech Stack:** Go 1.26 + cgo(`internal/nativeabi`)、Rust 1.97.1(`engine/crates/mornlea_engine`)、OpenSpec change `rust-engine-fluid`。

**Spec:** `docs/superpowers/specs/2026-08-29-rust-engine-fluid-design.md`(执行者必须同时读 spec 与本计划;本计划在字节布局等细节上比 spec 更精确,冲突时以 spec 的行为不变性为准、布局以本计划为准)。

## Global Constraints

- 行为逐位不变:`Advance` 返回变更集、世界写入、再入队集合,重扫入队位置与 dueTick,全部与迁移前逐位一致;三个 tunable(`FluidUpdatesPerTick`/`FluidRescanCellsPerTick`/`FluidFlowDelayTicks`)语义零改动;重扫记账逐字保留(非流体均匀段 1 格、水源均匀段不动点 1 格、逐格 1 格,区段开始前查额度,单次调用至多超支一个区段)。
- 仅 engine ABI bump v8→v9;协议、存档 schema、client ABI、benchmark scenario 零触碰。
- `internal/nativeabi` 是唯一 Rust 桥;不得新增生产 fallback 或旁路;kernel 非 OK 状态码以稳定中文文案 panic(engine 保证失败不触碰 output)。
- engine FFI 契约:`extern "C"` 入口不穿 panic unwind;构造 slice 前校验 abi_version/指针/长度/对齐/重叠;校验失败不留下部分输出;固定容量与 overflow 是跨语言契约,不得静默截断。
- 代码注释/GoDoc/Rust doc 中文,标识符英文,注释引用 Go 标识符用反引号;**注释禁止任务编号**(形如 `A-01`)。
- Git 提交:单行英文 `<type>(<scope>): <subject>`,小写祈使句,无正文无页脚。
- 性能数值 record-only,不改变退出状态;新 worktree 先 `make rust`,重型门禁前后重建(共享 CARGO_TARGET_DIR)。
- 执行前提:`refactor/sim-subpackages` 已合入 main。**凡本计划书写 `internal/sim/fluid.go` 等路径处,执行时先用 `git grep -l rescanChunkFluids` / `git grep -l enqueueChunkFluids` 定位 realm 合并后的实际文件路径**,并以实际路径为准。
- 每个任务结束:定点 race 测试 + `go build ./...` + `go test ./internal/archcheck -count=1`(涉新边时)全绿才提交。

---

### Task 1: OpenSpec change 产物与基线快照

**Files:**
- Create: `openspec/changes/rust-engine-fluid/proposal.md`
- Create: `openspec/changes/rust-engine-fluid/design.md`
- Create: `openspec/changes/rust-engine-fluid/tasks.md`
- Create: `openspec/changes/rust-engine-fluid/ledger.md`
- Create: `openspec/changes/rust-engine-fluid/specs/rust-engine-fluid/spec.md`

**Interfaces:**
- Produces: change 名 `rust-engine-fluid`(后续任务在 ledger.md 追加裁决与验证证据);delta spec 的 4 条 Requirement 标题(后续任务的验收引用它们)。

- [ ] **Step 1: 写 proposal.md**

Why 节引用 spec 背景与动机(量化:重扫 65536 格/tick 逐格循环 + eval 512 项/tick 逐项 map 分配,均纯 Go,是 go-rust-division 红线上最后一块大体素循环);What Changes 节写「engine ABI v8→v9 新增 `mornlea_fluid_eval_batch` 与 `mornlea_fluid_rescan` 两个无状态纯函数;`internal/fluid` 接入 nativeabi;sim 侧重扫组装邻域盒;Go 实现转 test-only oracle;行为逐位不变」;非目标节写「不改协议/存档/client ABI/scenario、不改 tunable 语义、不删 oracle(留后续 change)、不动 mesh/light 等既有 kernel」。Capabilities 节声明 New Capability `rust-engine-fluid`。

- [ ] **Step 2: 写 delta spec(specs/rust-engine-fluid/spec.md)**

ADDED Requirements,每条至少一个 `#### Scenario:`(用 `openspec/changes/archive/2026-08-15-rust-engine-worldgen/specs/` 的条目措辞作模板):

1. **流体规则求值由 engine 承担** — Scenario: 批量求值产出与 Go oracle 逐位一致;Scenario: 陈旧项(自格非流体)产出空写集。
2. **流体重扫扫描由 engine 承担** — Scenario: 记账与非流体均匀段/水源均匀段不动点/逐格三档逐字一致;Scenario: 邻块未就绪平面跳过且不记额度。
3. **流体状态与编排留在 Go** — Scenario: 队列全序/同 tick 冲突取最强/排序提交/冲毁结算行为不变;Scenario: 三个 tunable 语义不变。
4. **迁移保持逐位行为与测试网** — Scenario: 现有性质测试、e2e、Memory/TCP parity 全绿不改断言;Scenario: kernel 非 OK 状态码以稳定中文文案 panic。

- [ ] **Step 3: 写 design.md 与 tasks.md**

design.md 以 spec 为基础,补字节布局(引用本计划 Task 2/4 的布局小节);tasks.md 列 Task 2-6 的标题与验收命令(每个任务一段,写明 Files 与定点验证命令)。

- [ ] **Step 4: 写 ledger.md 骨架并采集基线**

ledger.md 记:执行前提(realm 已合入,记当时 main HEAD)、基线证据:

```bash
go test ./internal/fluid -race -count=1
go test ./internal/sim/... -run 'Fluid' -race -count=1
go test ./internal/fluid -bench . -benchtime 1x -run '^' 2>&1 | tail -5
```

若 `-bench` 无输出(包内尚无 benchmark),在 ledger 记「基线无 micro-bench,eval/rescan bench 由 Task 3/5 新增,基线即首次 oracle 差分 bench」。同时记录 `git grep -c 'func Benchmark' internal/fluid internal/sim` 的输出备查。

- [ ] **Step 5: 验证并提交**

```bash
openspec validate rust-engine-fluid --strict --no-interactive
openspec validate --all --strict --no-interactive
git add openspec/changes/rust-engine-fluid
git commit -m "docs: add rust-engine-fluid change products"
```

Expected: validate 全绿。

---

### Task 2: eval kernel —— Rust 实现 + ABI v9 + nativeabi 绑定

**Files:**
- Modify: `engine/include/mornlea_engine.h`(版本史注释 + `MORNLEA_ENGINE_ABI_VERSION 9u` + `mornlea_fluid_eval_batch` 声明)
- Create: `engine/crates/mornlea_engine/src/fluid_eval.rs`
- Modify: `engine/crates/mornlea_engine/src/lib.rs`(mod 列表加 `fluid_eval`)
- Modify: `engine/crates/mornlea_engine/src/ffi.rs`(导出 `mornlea_fluid_eval_batch`,镜像 `mornlea_lod_shell` 的校验与 panic 收敛形态)
- Modify: `internal/nativeabi/native.go`(cgo 声明 + `FluidEvalBatch`)
- Modify: `internal/nativeabi/native_test.go`(`ABIVersion` 钉位 8→9 及版本注释;新增绑定形状测试)
- Test: `engine/crates/mornlea_engine/src/fluid_eval.rs` 内嵌 `#[cfg(test)]` 单测

**Interfaces:**
- Produces: `nativeabi.FluidEvalBatch(input, output []byte)`(input 长度 = 8 + N×14,N = 本 tick 弹出项数;output 长度 = N×12;非 OK panic)。
- Produces: 输入布局 v1——`u32 layout_version=1(LE)` + `u32 item_count` + 每项 7×u16 LE(槽位序:0=自格、1=上、2=下、3=+x、4=−x、5=+z、6=−z);输出布局——每项 4 条 × 3B(目标槽位 u8,0..6;BlockID u16 LE),无写槽位 = `0xFF, 0x00, 0x00`。

- [ ] **Step 1: 写 Rust 单测(先失败)**

`fluid_eval.rs` 内先写测试:构造 `WaterSourceID` 等价数值(下方 `block()` 函数给出 Go 侧 `iota` 的实际值,用 `mod tests` 断言)覆盖分支:源向下写等级 1、水平扩散等级 +1、等级 7 不再传播、存活判定(上方流体/更强水平邻居)、非源消亡写 Air、陈旧项空写、作物可替换、开启门可流入/关闭门与上半不可、源不可替换、弱水被强水替换。断言期望写集(槽位+ID)。

```rust
// engine/crates/mornlea_engine/src/fluid_eval.rs 核心逻辑(实现骨架,测试先行后补全):
pub const EVAL_LAYOUT_VERSION: u32 = 1;
pub const EVAL_ITEM_BYTES: usize = 14;
pub const EVAL_ITEM_OUTPUT_BYTES: usize = 12;
const SLOT_SELF: u8 = 0;
const SLOT_NO_WRITE: u8 = 0xFF;

// 方块编号是协议稳定值,与 Go internal/core/block.go 的 iota 逐一对应:
// Air=0, Barrier=1, Stone=2, Dirt=3, Grass=4, Bedrock=5, StoneBrick=6,
// WaterSource=60..WaterLevel7=67(iota 实际值以执行时 core/block.go 为准,
// 实现者必须先核对再固化,两侧注释互指)。
const AIR: u16 = 0;
const WATER_SOURCE: u16 = 60;
const DOOR_UPPER: u16 = 130; // 执行时按 core/block.go 核对

fn is_fluid(id: u16) -> bool { (60..=67).contains(&id) }
fn fluid_level(id: u16) -> u8 { if id == WATER_SOURCE { 0 } else { (id - 60) as u8 } }
fn is_crop(id: u16) -> bool { /* 小麦 8 阶段 + 马铃薯 + 胡萝卜三段连续区间,按 core/farming.go 核对 */ }
fn is_door(id: u16) -> bool { /* 按 core/block.go 的门区间核对 */ }
fn is_door_open_lower(id: u16) -> bool { /* 四个 Open 下半门 */ }

// replaceable 镜像 Go fluid.Replaceable 的判定表,顺序一致:
// 空气→真;上半门→假;开启下半门→真;关闭门→假;作物→真;非流体→假;源→假;等级比较。
fn replaceable(target: u16, new_level: u8) -> bool { /* 判定表 */ }

// eval_one 镜像 Go evalCell 三段:陈旧项跳过、存活判定(上邻任意流体或更强水平邻居)、
// 垂直优先即返、水平传播等级 +1 且 ≤7。writes 以 (slot, id) 写入 out 的定长 4 槽。
fn eval_one(cells: &[u16; 7], out: &mut [u8; EVAL_ITEM_OUTPUT_BYTES]) { /* ... */ }
```

- [ ] **Step 2: 跑 Rust 单测确认失败**

```bash
cd engine && cargo test -p mornlea_engine fluid_eval
```

Expected: FAIL(模块刚建、函数为 `todo!()` 或空实现)。

- [ ] **Step 3: 最小实现 eval_one/replaceable 及解析编码**

按 Step 1 骨架补全;解析层校验 `layout_version`、`input.len() == 8 + item_count*14`、`output.len() >= item_count*12`,违约返回对应状态(不写 output)。

- [ ] **Step 4: 跑 Rust 单测确认通过 + rust-check**

```bash
cd engine && cargo test -p mornlea_engine fluid_eval && make -C .. rust-check
```

Expected: PASS,rust-check 绿。

- [ ] **Step 5: C header 与 ffi.rs 导出**

header 版本史注释追加一段(措辞沿 v8 条目风格):`ABI v9:新增 mornlea_fluid_eval_batch 与 mornlea_fluid_rescan 流体双内核(rust-engine-fluid 变更)……`(rescan 声明 Task 4 追加);`#define MORNLEA_ENGINE_ABI_VERSION 9u`;声明:

```c
/*
 * mornlea_fluid_eval_batch:流体单格规则批量求值(无状态纯函数)。
 *
 * 输入 = u32 layout_version(当前 1,LE)+ u32 item_count + 每项 14 字节
 * (7 个 u16 LE 方块编号,槽位序:0=自格、1=上、2=下、3=+x、4=−x、5=+z、
 * 6=−z,与 Go internal/fluid 的 sixNeighbors 同序)。方块编号是协议稳定值,
 * 与 Go internal/core/block.go 的 iota 逐一对应。
 *
 * 输出 = 每项 12 字节:4 条候选写入 ×(目标槽位 u8(0..6;0xFF=无写入)+
 * BlockID u16 LE)。同一项内至多 4 条(垂直优先 1 条或水平传播 4 条或
 * 自格消亡 1 条),多余槽位为无写入哨兵。
 *
 * input_len 必须等于 8 + item_count*14,output 容量不足返回
 * MORNLEA_STATUS_INVALID_ARGUMENT(输出尺寸是输入的确定函数,无需两段式
 * 探测);layout_version 或 item_count 违约返回 MORNLEA_STATUS_INPUT;
 * 其余状态语义与既有导出一致。
 */
uint32_t mornlea_fluid_eval_batch(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len);
```

ffi.rs 按 `mornlea_lod_shell` 的形态写导出:前指针/长度/重叠校验 → `catch_unwind` 收敛 panic → 解析分发 → 写 `*output_len`。

- [ ] **Step 6: nativeabi 绑定**

`native.go` 加 `#cgo noescape mornlea_fluid_eval_batch` / `#cgo nocallback mornlea_fluid_eval_batch` 与:

```go
// FluidEvalBatch 把调用方拥有的 fluid eval input 与 output 传给 engine。
// input 与 output 的布局契约见 engine/include/mornlea_engine.h 的
// mornlea_fluid_eval_batch 注释;任何非 OK 状态都以稳定中文文案 panic,
// 且 engine 保证失败时不触碰 output。
func FluidEvalBatch(input, output []byte) {
	status := fluidEvalBatchVersion(ABIVersion, input, output)
	if status != StatusOK {
		panic(fluidEvalStatusPanicText(status))
	}
}
```

`native_test.go`:`ABIVersion != 8` 改 `!= 9`(版本注释同步写明 v9 承载流体双内核);新增 `TestFluidEvalBatchBinding`——手编一个 2 项输入(源格下方空气、等级 7 格),断言输出字节的槽位与 ID 逐字节;再断言 `layout_version=2` 返回 panic 文案含「INPUT」。

- [ ] **Step 7: Go 侧验证并提交**

```bash
go test ./internal/nativeabi -race -count=1
go build ./...
git add engine/include/mornlea_engine.h engine/crates/mornlea_engine/src/fluid_eval.rs engine/crates/mornlea_engine/src/lib.rs engine/crates/mornlea_engine/src/ffi.rs internal/nativeabi
git commit -m "feat: add rust engine fluid eval batch kernel with abi v9"
```

(涉及 Rust 重编,提交前确认 `make rust` 已重跑、dylib 回拷。)

---

### Task 3: eval kernel Go 接线 —— Advance 批量化 + oracle 差分/golden/fuzz/0-alloc

**Files:**
- Modify: `internal/fluid/queue.go`(`Advance` 阶段一改批量;`Queue` 加 eval scratch 字段)
- Create: `internal/fluid/eval_native.go`(输入编码 + 输出解码 + scratch 管理;包内唯一 nativeabi 调用点)
- Create: `internal/fluid/oracle_test.go`(从 `rules.go` 移入 `evalCell`/`flowingSurvives`,包内测试专用)
- Modify: `internal/fluid/rules.go`(删除 `evalCell`/`flowingSurvives`;`Replaceable`/`strongerWrite`/邻格函数保留)
- Modify: `internal/archcheck/dependency_test.go`(:`"internal/fluid": {"internal/core"}` → `{"internal/core", "internal/nativeabi"}`——本任务引入该导入边,必须随本任务登记,否则任务自身的 archcheck 门禁即红;注释说明流体 kernel 边)
- Test: `internal/fluid/eval_differential_test.go`、`internal/fluid/eval_golden_test.go`、`internal/fluid/eval_fuzz_test.go`、`internal/fluid/eval_alloc_test.go`、`internal/fluid/eval_bench_test.go`

**Interfaces:**
- Consumes: `nativeabi.FluidEvalBatch(input, output []byte)`(Task 2)。
- Produces: `internal/fluid` 的公共 API 不变(`Queue.Advance` 签名不动);`evalEncodeItem(w FluidWorld, pos core.BlockPos, dst []byte) int`(把 7 格写进 dst,供差分测试复用);bench `BenchmarkAdvanceEval`。

- [ ] **Step 1: 写差分测试(先失败)**

`eval_differential_test.go`:表驱动世界(复用 `helpers_test.go` 的测试世界构造),对每个用例:用 oracle `evalCell` 求出期望写集;手工构造 7 格输入(经 `evalEncodeItem`),调 `nativeabi.FluidEvalBatch`,解码输出,断言与 oracle 写集**逐项逐位一致**(槽位 + BlockID)。用例覆盖 Task 2 Step 1 的全部分支 + `BarrierID` 邻格 + 陈旧项。

```go
// eval_differential_test.go 骨架:
func TestFluidEvalBatchMatchesOracle(t *testing.T) {
	for _, tc := range evalDifferentialCases() { // world map + pos
		w := tc.world
		want := map[core.BlockPos]core.BlockID{} // evalCell(w, tc.pos) 的期望
		got := decodeEvalOutput(runEvalBatch(w, tc.pos))
		if !reflect.DeepEqual(got, want) { t.Fatalf(...) }
	}
}
```

- [ ] **Step 2: 跑差分确认失败**

```bash
go test ./internal/fluid -run TestFluidEvalBatchMatchesOracle -count=1
```

Expected: FAIL(`runEvalBatch`/`decodeEvalOutput` 未定义)。

- [ ] **Step 3: 实现 eval_native.go 与 Advance 批量化**

`eval_native.go`:`Queue` 增加私有字段 `evalInput, evalOutput []byte`(复用,按需扩容);`enqueueEvalItem` 把 7 格经 `w.BlockAt` 编码进 `evalInput`(同一线读语义:scope 外由 fluidWorld 读作 Barrier——编码不过 scope 判断,Barrier 语义由 `fluidWorld.BlockAt` 给出);`Advance` 阶段一改为:按现行循环(全序弹出、到期检查、探视守卫、budget 计数)把弹出项依次编码进 `evalInput`,循环结束后调一次 `FluidEvalBatch`,解码输出、按项还原绝对坐标、并入 `pendingWrites`(仍经 `strongerWrite`)。阶段二与再入队零改动。

- [ ] **Step 4: 跑包内全量测试**

```bash
go test ./internal/fluid -race -count=1
```

Expected: 全绿——性质测试(converge/order/budget/rescan)、e2e、queue_bounded 走公共 API 即走 Rust 路径,自动成为回归网;oracle 差分绿。

- [ ] **Step 5: golden vectors**

`eval_golden_test.go`:沿 `internal/physics/step_golden_vectors_test.go` 的源码字面量风格,固化 ≥8 个向量的**输出字节字面量**(eval 全分支 + Barrier + 陈旧项),断言 `FluidEvalBatch` 原始输出逐字节。向量来源:实现完成后从生产路径一次性采集、人工复核。

- [ ] **Step 6: fuzz 与 0-alloc**

`eval_fuzz_test.go`(沿包外 `internal/physics/physics_fuzz_test.go` 形态):任意 7 格输入下断言不变量——写目标槽位的值只能是空气/作物格上的流体/更弱流体,等级 ≤7,源格自身永不被写为非源。`eval_alloc_test.go`:`testing.AllocsPerRun` 断言「编码 + 调用 + 解码」helper 在 scratch 预热后 0 alloc。

```bash
go test ./internal/fluid -run 'TestFluidEvalFuzz|TestEvalNoAlloc' -count=1
go test ./internal/fluid -fuzz FuzzFluidEval -fuzztime 30s
```

- [ ] **Step 7: bench + 提交**

`eval_bench_test.go`:`BenchmarkAdvanceEval`(预置活跃水体队列,跑 `Advance`)与 `BenchmarkAdvanceEvalOracle`(build tag `fluid_oracle_bench` 下走 oracle,迁移期一次性对照,数值入 ledger)。跑 `-bench . -benchtime 1x` 确认可运行,数值 record-only 入 ledger:

```bash
git add internal/fluid
git commit -m "feat: route fluid advance eval through nativeabi batch kernel"
```

---

### Task 4: rescan kernel —— Rust 实现 + nativeabi 绑定

**Files:**
- Modify: `engine/include/mornlea_engine.h`(v9 注释补 `mornlea_fluid_rescan` + 声明)
- Create: `engine/crates/mornlea_engine/src/fluid_rescan.rs`
- Modify: `engine/crates/mornlea_engine/src/lib.rs`、`src/ffi.rs`
- Modify: `internal/nativeabi/native.go`(`FluidRescan`,返回 `(Status, int)` 供调用方处理两段式 overflow,沿 `lod` 的形态)
- Modify: `internal/nativeabi/native_test.go`(rescan 绑定形状测试)

**Interfaces:**
- Consumes: Task 2 的 v9 版本基座。
- Produces: `nativeabi.FluidRescan(input, output []byte) (Status, int)`;输入布局 v1(MFL1,三段:header 26B + 中心区块 24 区段记录 + 裙边 68 列×384 u16 + 元数据 9 区块×24×3B)。

- [ ] **Step 1: 字节布局定稿(写入 header 注释与 Rust 单测)**

```
header(26B): u32 layout_version=1 | i32 center_chunk_x | i32 center_chunk_z
  | u16 x0 | u16 x1 | u16 z0 | u16 z1(盒内局部列 0..17,含裙边)
  | u8 start_section(0..23) | u8 reserved=0 | u32 budget
中心区块 24 区段记录(按 y 区段 0..23):u8 kind(0=均匀) | u8 pad
  | kind=0: u16 uniform_id(记录共 4B)
  | kind=1: 4096×u16 LE(区段内序 x + z*16 + y16*256,与 Go blockIndex 一致)
裙边 68 列 × 384 u16(列序固定:(x=-1,z=0..15)、(x=16,z=0..15)、
  (z=-1,x=0..15)、(z=16,x=0..15)、四角 (-1,-1)/(16,-1)/(-1,16)/(16,16);
  列内 y 0..383)
元数据 9 区块 × 24 区段 × 3B(u8 uniform_flag + u16 id,flag=0 时 id=0):
  区块序:中心、(-1,-1)、(0,-1)、(1,-1)、(-1,0)、(1,0)、(-1,1)、(0,1)、(1,1)
盒内局部坐标:中心区块局部 (lx,lz) ∈ 0..15 映射盒 (lx+1, lz+1);y 全高 0..383
  对应世界 y_base + 0..383(y_base = core.MinY,由 Go 编码方保证)
```

- [ ] **Step 2: 写 Rust 单测(先失败)**

`fluid_rescan.rs` 测试:手工构造小盒(均匀水源段+裙边实心、混杂段、裙边 Barrier、y 越界)断言:盒访问器逐格正确;记账三档(非流体均匀段 1、水源均匀段+五邻实心 1、逐格 1);区段起点续扫与「单次调用至多超支一个区段」;产出坐标 = center_chunk 坐标换算的世界坐标;budget 用尽返回未扫完。

- [ ] **Step 3: 实现 + rust-check**

扫描循环镜像 Go `enqueueChunkFluids` 的结构(区段循环前查额度 → 均匀段捷径 → 逐格循环 → 水源 `fluidSourceIsFixedPoint` 五邻检查;区段级不动点 `fluidSectionIsFixedPoint` 用中心区段数据 + 元数据表判定),输出侧:positions 追加(u32 x、u32 y、u32 z LE 世界坐标)+ 尾部 summary `u32 spent | u8 done | u8[3] pad`;容量不足两段式返回 `OUTPUT_OVERFLOW` 并报告所需字节数。

```bash
cd engine && cargo test -p mornlea_engine fluid_rescan && make -C .. rust-check
```

- [ ] **Step 4: header 声明 + nativeabi 绑定 + 测试**

`mornlea_fluid_rescan(abi_version, input, input_len, output, output_capacity, output_len)`,注释写明三段布局与两段式 overflow;`native.go`:

```go
// FluidRescan 把调用方拥有的 fluid rescan input 与 output 传给 engine。
// 返回状态与写入 output 的字节数;OUTPUT_OVERFLOW 时 *output_len 为所需
// 容量且 output 未被触碰,由调用方扩容重试。其余非 OK 状态由调用方以
// 稳定中文文案 panic(见 internal/fluid 的包装)。
func FluidRescan(input, output []byte) (Status, int)
```

`native_test.go` 新增 rescan 绑定测试:手编最小盒(单均匀水源段+实心裙边),断言产出坐标与 spent;layout_version=2 断言状态 INPUT。

- [ ] **Step 5: 验证并提交**

```bash
go test ./internal/nativeabi -race -count=1
go build ./... && git add engine internal/nativeabi
git commit -m "feat: add rust engine fluid rescan kernel"
```

---

### Task 5: rescan kernel Go 接线 —— 邻域盒编码器 + 差分 + bench

**Files:**
- Create: `internal/fluid/rescan_native.go`(`RescanRegion`/`RescanScratch` 类型 + `ScanRescanRegion` 包装:拼 header、调 `FluidRescan`、OUTPUT_OVERFLOW 扩容重试、解码 positions/spent/done;非 OK(除 overflow)panic 稳定中文文案)
- Modify: `internal/sim/realm/environment.go`(**生产重扫路径**,2026-08-30 勘定:权威 tick 经 `internal/sim/runtime/engine_step.go:536` 直调 `realm.State.AdvanceFluids`,重扫链 = `State.runFluidRescans` → `State.rescanChunkFluids`(:525)→ `State.enqueueChunkFluids`;realm 自持 `fluidWorld` 适配器(:410,含 `settleFloodedCrop` 与 mutation 记录)):(a) 新增 `encodeRescanBox`(组装 MFL1 三段:中心区块 24 区段经 `IsUniform`/线性 `idAt` 采样,裙边 68 列经就绪邻块 `BlockAt` 或 Barrier 填充,元数据 9 区块×24 段);(b) `State.rescanChunkFluids` 的平面循环改为:组装盒 → `fluid.ScanRescanRegion` → 对返回坐标 `queue.Enqueue`(dueTick 同现行 `now+delay`);邻块未就绪平面跳过且不记额度不变
- Note: `internal/sim/runtime/fluid.go` 存有同名重扫拷贝(`Engine.rescanChunkFluids`/`enqueueChunkFluids` 等),非测试代码零调用(仅 `fluid_test.go` 引用)——**不属本次迁移范围,保持不动**,ledger 记录该勘定
- Create: `internal/sim/realm/rescan_differential_test.go`、`internal/sim/realm/rescan_bench_test.go`
- Modify: `internal/sim/realm/environment.go` 的 `State.enqueueChunkFluids`/`fluidSourceIsFixedPoint`/`fluidSectionIsFixedPoint`/`fluidRescanBlockAt` → 移入 oracle 测试文件(`environment_oracle_test.go`,package realm 测试专用)

**Interfaces:**
- Consumes: `nativeabi.FluidRescan`(Task 4)、Task 4 的 MFL1 布局。
- Produces: `fluid.ScanRescanRegion(box, meta []byte, region RescanRegion, scratch *RescanScratch) (positions []core.BlockPos, spent int, done bool)`;bench `BenchmarkRescanChunk`。

- [ ] **Step 1: 写差分测试(先失败)**

`rescan_differential_test.go`(**package realm**,差分放 sim 侧:realm 能直接构造 `Dimension`/区块数据,`internal/fluid` 不能):同一地形分别跑 Go oracle `State.enqueueChunkFluids`(oracle 文件内)与 `encodeRescanBox`+`ScanRescanRegion`,断言入队集合、`spent`、`done` 逐位一致。地形覆盖:全均匀海洋段(捷径命中)、混杂地表段、跨区段/跨区块邻格、邻块未就绪、budget 中途用尽(游标续扫)、y 上下界。

- [ ] **Step 2: 实现 encodeRescanBox 与接线**

按 Files 节实现。编码顺序与 Task 4 Step 1 布局逐字节对应;中心区段采样:`section.Blocks.IsUniform()` 命中走 kind=0,否则 kind=1 按 `blockIndex` 同序线性展开。**盒组装粒度 = 每区块一次**:`State.rescanChunkFluids` 入口组装一次(裙边列取就绪邻块数据,未就绪邻块列填 Barrier),平面循环内只改 header 的 x0..x1/z0..z1/start_section/budget 复用同一份盒体;未就绪邻块的平面在 Go 侧跳过、不调 kernel、不记额度(与现行语义逐字一致)。接线后 sim 侧不再直接依赖 `fluid.Replaceable`(生产调用点清零;`Replaceable` 本体保留在 `internal/fluid/rules.go` 作为 spec 判定面,供 oracle 与性质测试使用)。

- [ ] **Step 3: 全量验证**

```bash
go test ./internal/fluid ./internal/sim/... -race -count=1
go test ./internal/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -count=1
```

Expected: 全绿(性质 `property_rescan` 走公共 API 自动覆盖 Rust 路径;parity 证明 Memory/TCP 一致)。

- [ ] **Step 4: bench + ledger + 提交**

`rescan_bench_test.go`:`BenchmarkRescanChunk`(海洋型区块 + 地表型区块两例),`-bench . -benchtime 1x` 数值 record-only 入 ledger;对照 Task 1 基线与 Task 3 的 eval bench,记入 `openspec/changes/rust-engine-fluid/ledger.md`。

```bash
git add internal/fluid internal/sim openspec/changes/rust-engine-fluid/ledger.md
git commit -m "feat: route fluid rescan through nativeabi scan kernel"
```

---

### Task 6: 文档、archcheck 与全量门禁收尾

**Files:**
- Modify: `docs/notes/go-rust-division.md`(领域归属表加流体一行:规则求值与重扫扫描归 engine,状态/编排/冲毁结算留 Go)
- Modify: `docs/architecture.md`(边界描述同步流体)
- Modify: `docs/notes/test-quickstart.md`(定点命令补 `./internal/fluid`)
- Create: `internal/fluid/AGENTS.md`(包职责、nativeabi 边、oracle 地位、布局 v1 契约指针;按 `docs/agents-md-style.md`)
- Modify: `openspec/changes/rust-engine-fluid/ledger.md`(终局证据)

(注:`internal/fluid → internal/nativeabi` 的 archcheck 边登记已随 Task 3 落地——引入导入边的任务必须同步登记;本任务只复核门禁绿。)

**Interfaces:**
- Consumes: Task 2-5 的全部产物。
- Produces: 可合并的分支。

- [ ] **Step 1: archcheck 与文档同步**

```bash
go test ./internal/archcheck -count=1
```

Expected: PASS(新边登记后)。文档四处按 Files 列表同步,表述与 spec 一致。

- [ ] **Step 2: 全量门禁**

```bash
make rust && make rust-check
make dev-check
make test-race
openspec validate --all --strict --no-interactive
```

Expected: 全绿;任何红先修根因,不得绕过。输出摘要与关键数值记入 ledger.md。

- [ ] **Step 3: ledger 终局 + 提交**

ledger.md 补:两 kernel 的 bench 对照表(oracle/native 或迁移前/后)、门禁清单与结果、遗留(oracle 删除留后续 change)。提交:

```bash
git add -A && git commit -m "docs: sync fluid kernel docs and close rust-engine-fluid gates"
```

---

## Self-Review 记录

- Spec coverage:spec「目标边界」两 kernel → Task 2/4(Rust+ABI)与 Task 3/5(接线);「行为不变性」→ Task 3 Step 4 性质网 + Task 5 Step 3 parity;「失败语义」→ Task 2 Step 6 / Task 5 panic 文案;「验证门禁」七层 → Task 2 Step 1/4(Rust 单测)、Task 3 Step 1/5/6(差分/golden/fuzz/0-alloc)、Task 3+5 Step 4(bench)、Task 6(全量);「文档」→ Task 6。无缺口。
- Placeholder scan:Rust/Go 代码块中标注「执行时按 core/block.go 核对」的常量是刻意的核对指令而非 TBD——两侧常量值必须以代码为准,计划不猜测数值;其余步骤均有实际内容。
- Type consistency:`FluidEvalBatch(input, output []byte)`(Task 2 定义 = Task 3 消费);`FluidRescan(input, output []byte) (Status, int)`(Task 4 = Task 5);`ScanRescanRegion` 签名 Task 5 内一致;`RescanRegion` 字段与 Task 4 布局的 x0/x1/z0/z1/start_section/budget 一一对应。
