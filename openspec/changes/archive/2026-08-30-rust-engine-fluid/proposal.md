# Change: rust-engine-fluid

## Why

`docs/notes/go-rust-division.md` 的红线是「Rust 算数,Go 定规则」:对 ≥ 数千元素做
同一种数值变换的循环归 Rust,生产路径不得保留体素级 O(N) 密集循环。engine ABI v8
已承载 mesh/light、collision、physics step、worldgen、raycast、LOD shell 七类内核,
流体是权威 tick 内最后一块留在 Go 的大体素循环:

- **重扫扫描**(realm 合并后位于 `internal/sim/realm/environment.go` 的
  `State.enqueueChunkFluids`):每 tick 预算 `FluidRescanCellsPerTick`(默认
  65536 格)的逐格 `Blocks.Get` + `IsFluid` + 水源五邻检查,跨区段邻格读还要付
  `dimension.records` map 查找。
- **规则求值**(`internal/fluid` 的 `evalCell`):每 tick 预算
  `FluidUpdatesPerTick`(默认 512 项),每项 1 次 map 分配 + 至多 7 次经
  `fluidWorld` 的跨 map 世界读。

两类工作都是纯整数数值变换,由 `mornlea_engine` 承担后,Go 侧只剩状态与编排。
迁移前后同输入下的变更集、世界写入与再入队集合必须逐位一致,既有存档、测试语料
与 Memory/TCP parity 不受影响。

## What Changes

- engine ABI v8→v9 新增 `mornlea_fluid_eval_batch` 与 `mornlea_fluid_rescan` 两个
  无状态纯函数(调用方持有全部 buffer,失败返回状态码,panic 收敛)。
- `internal/fluid` 接入 nativeabi:`Queue.Advance` 阶段一改为「先按现行循环弹出
  ≤budget 项、再一次批量求值」;sim 侧重扫侧由 realm 按 (区块, 平面) 扫描单元
  现场组装以该平面被扫区块为中心的 18×384×18 邻域盒(含 8 邻块裙边列与区段
  元数据;五段平面各自组盒、每区块重扫至多 5 次编码——共享一份以重扫目标
  区块为中心的盒会把边界平面条带放到最外裙边列上,五邻不动点读越出盒外,
  区段级不动点也会错拿中心区块记录描述被扫邻块),交 `internal/fluid` 导出的
  扫描包装函数执行,`nativeabi` 边不进 sim。
- Go 实现转 test-only oracle:`evalCell`/`flowingSurvives` 与重扫扫描/两级不动点
  判据移入测试文件,新增逐位差分、golden vectors 与 fuzz 门禁;现有性质测试、
  e2e、Memory/TCP parity 原样保留,走公共 API 自动成为行为回归网。
- 行为逐位不变:重扫入队位置与 dueTick、`Advance` 变更集、记账三档(非流体均匀段
  1 格、水源均匀段不动点 1 格、逐格 1 格)与三个 tunable 语义零改动。
- 队列、游标、预算、同 tick 冲突合并、排序提交、作物冲毁结算等状态与编排全部
  留在 Go。

## Capabilities

### New Capabilities

- `rust-engine-fluid`: Rust engine 承担流体规则求值与重扫扫描的行为契约——与
  Go oracle 逐位一致、状态与编排留在 Go、迁移保持逐位行为与测试网、非 OK
  状态码以稳定中文文案 panic。

### Modified Capabilities

无。既有流体可观察行为(流动、传播、冲毁、重扫节奏)不变,本变更只迁移生产
实现载体。

## Impact

- 受影响包:`internal/fluid`、`internal/sim/realm`、`internal/nativeabi`、
  `internal/archcheck`(登记 `internal/fluid → internal/nativeabi` 新边)、
  `engine/crates/mornlea_engine`、`engine/include`。
- 调用方零改动:权威 tick 经 `internal/sim/runtime/engine_step.go` 调用
  `realm.State.AdvanceFluids` 的链路形状不变(`internal/sim/runtime/fluid.go`
  的同名测试镜像不属本次迁移范围,保持不动)。
- 兼容性:仅 engine ABI bump(v8→v9);协议 v32、区块 schema v9、世界 metadata v3、
  client ABI v11、benchmark scenario v20 零触碰。Go binary 与
  `libmornlea_engine` 仍为不可跨版本混装的 release unit;既有 mesh/light/
  collision/step/worldgen/raycast/LOD ABI 不动。
- 性能:重扫与求值从逐格 Go 循环改为批量 native 调用;benchmark 与 perfcheck
  数值只记录,不改变退出状态。
- 并发:两个 kernel 均为无状态纯函数,Go 侧队列与重扫状态的所有权、锁边界与
  goroutine 归属不变。

## 非目标

不改协议、存档 schema、client ABI、benchmark scenario;不改
`FluidUpdatesPerTick`/`FluidRescanCellsPerTick`/`FluidFlowDelayTicks` 三个
tunable 的语义;不删除 Go test-only oracle(删除留后续独立 change,沿
`drop-go-test-oracles` 先例);不动 mesh/light 等既有 kernel;不引入生产 Go
fallback;不迁移 `internal/sim/runtime/fluid.go` 的测试镜像代码。
