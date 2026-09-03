# rust-engine-fluid Specification

## Purpose
把流体的两类体素级数值变换——单格规则求值与区块重扫扫描——交由 Rust engine 唯一
生产实现,Go 只保留队列、预算、游标与提交编排,并以 Go 测试 oracle 保证迁移前后
行为逐位一致。
## Requirements
### Requirement: 流体规则求值由 engine 承担

`packages/server/fluid` 的单格流体规则(存活判定、垂直优先、水平传播等级 +1 且 ≤7、
可替换判定)MUST 由 engine 的批量求值内核唯一生产;Go 生产路径 MUST 不包含逐格
规则实现:`evalCell`/`flowingSurvives` 只允许存在于测试 oracle,`Replaceable`
判定表保留在 `packages/server/fluid/rules.go` 作为冻结判定面,生产路径零调用(生产
调用点已随迁移清除)。

#### Scenario: 批量求值产出与 Go oracle 逐位一致

- GIVEN 任意合法的 7 格邻域输入(自格 + 六邻,覆盖垂直流、水平扩散、多源汇合、
  作物与门替换、Barrier 边界)
- WHEN 调用批量求值内核
- THEN 每项的写集(目标槽位 + BlockID)与测试内 Go oracle `evalCell` 的产出
  逐项逐位一致

#### Scenario: 陈旧项产出空写集

- GIVEN 某队列项自格在求值时已不是流体(陈旧项)
- WHEN 该项进入批量求值
- THEN 该项产出空写集,不产生任何世界写入,与现行 Go 行为一致

### Requirement: 流体重扫扫描由 engine 承担

区块流体重扫的稠密扫描(逐格流体判定、水源五邻不动点检查、区段级捷径)MUST 由
engine 的重扫内核唯一生产;Go 生产路径 MUST 不包含逐格扫描实现,只保留待办队列、
(plane, section) 续扫游标、`FluidRescanCellsPerTick` 预算、邻域盒编码与入队。

#### Scenario: 记账与三档捷径逐字一致

- GIVEN 任意中心区块数据、裙边列与区段元数据(覆盖非流体均匀段、水源均匀段、
  混杂区段、跨区段与跨区块邻格)
- WHEN 调用重扫内核
- THEN 产出的入队坐标、实耗额度与区域是否扫完,与测试内 Go oracle
  `enqueueChunkFluids` 逐位一致;记账三档(非流体均匀段记 1 格、水源均匀段
  不动点记 1 格、逐格段逐格记 1 格,区段开始前查额度、单次调用至多超支一个
  区段)逐字保留

#### Scenario: 邻块未就绪平面跳过且不记额度

- GIVEN 被扫平面依赖的邻块尚未就绪
- WHEN 该平面到达重扫
- THEN 该平面被跳过、不调用 kernel、不消耗重扫额度,与现行 Go 行为逐字一致

### Requirement: 流体状态与编排留在 Go

流体队列(`Queue` 索引堆与 dueTick 全序)、`pendingWrites` 的 `strongerWrite`
同 tick 冲突合并、按 `lessPos` 排序提交、作物冲毁结算(`settleFloodedCrop`)、
`recordChange` 广播与变更再入队 MUST 继续由 Go 独占;`FluidUpdatesPerTick`、
`FluidRescanCellsPerTick`、`FluidFlowDelayTicks` 三个 tunable 的语义 MUST 零改动。

#### Scenario: 队列全序、冲突合并、提交与冲毁结算行为不变

- GIVEN 迁移前任意队列内容与世界状态
- WHEN 迁移后以相同输入执行 `Queue.Advance` 与重扫
- THEN 返回的变更集、世界写入、再入队集合及重扫入队位置与 dueTick 与迁移前
  逐位一致;同 tick 多写冲突仍取最强写,提交仍按 `lessPos` 全序,被淹作物仍按
  现行规则冲毁结算

#### Scenario: 三个 tunable 语义不变

- GIVEN 任意 `FluidUpdatesPerTick`、`FluidRescanCellsPerTick`、
  `FluidFlowDelayTicks` 取值
- WHEN 在迁移后的生产路径上运行
- THEN 求值预算截断、重扫额度消耗与推进节奏、待更新项延迟语义与迁移前逐字一致

### Requirement: 迁移保持逐位行为与测试网

本变更 MUST NOT 修改既有流体测试的任何断言;现有性质测试(`property_converge`/
`property_order`/`property_budget`/`property_rescan`)、e2e、`queue_bounded` 与
Memory/TCP parity 测试 MUST 全绿不改断言;kernel 非 OK 状态码 MUST 以稳定中文
文案 panic,且 engine 保证失败时不触碰输出缓冲。

#### Scenario: 既有测试不改断言全绿

- GIVEN 迁移前的全部流体性质测试、e2e 与 Memory/TCP parity 测试
- WHEN 在迁移后的生产路径上运行
- THEN 全部测试不修改断言而通过

#### Scenario: kernel 非 OK 状态码以稳定中文文案 panic

- GIVEN 构造的非法 kernel 请求(如 layout_version 违约或缓冲长度不符)
- WHEN 调用 Go 侧包装
- THEN 以包含稳定中文文案的 panic 失败,输出缓冲保持调用前内容,不存在静默
  降级或部分输出

### Requirement: engine ABI v9 承载流体批量入口

流体批量求值与重扫 exports SHALL 作为 engine ABI v9 相对 v8 的增量交付；C header、Rust identity 与 `packages/shared/nativeabi` 的当前版本常数 MUST 同步为 9。v9 MUST 原样保留 v8 引入的 20 字节 mesh registry layout 与既有 mesh/light/collision/raycast/physics/worldgen/LOD 行为。接受 ABI version 的当前 engine exports 对错误版本的共同边界 MUST 是先于语义输入解引用、payload 发布以及 engine/fluid 状态语义返回 `ABI_VERSION`，而不是要求 ABI 检查成为函数的第一条指令；为安全建立输出契约所需的非解引用 pointer/range 检查可以先执行，output metadata pointer 可以先校验，合法 metadata 中的长度或计数字段可以先清零。无效 metadata MUST 继续按该 export 的既有参数契约拒绝，不得要求其返回 `ABI_VERSION`。系统 MUST NOT 提供 Go fallback。client ABI 独立演进，不因该 engine 增量改变菜单或渲染 surface。

#### Scenario: engine v9 身份三端一致

- GIVEN 当前 engine C header、Rust 动态库与 Go `packages/shared/nativeabi`
- WHEN 检查版本常数和 `mornlea_engine_abi_version()`
- THEN 三端 MUST 均报告 9
- AND v8 mesh registry layout 与结果 MUST 保持不变

#### Scenario: 合法 output metadata 下 engine v8 调用方不能混装 v9 surface

- GIVEN 当前 engine ABI v9 动态库、传入版本 8 的调用方与该 export 所需的合法 output metadata pointer
- WHEN 调用任一共有或 v9 流体 export
- THEN 调用 MUST 返回 ABI version 错误
- AND 合法 metadata 的长度或计数字段 MAY 已被清零
- AND 调用 MUST NOT 语义解引用输入、发布任何 payload、进入 engine/fluid 状态语义或转入 fallback
