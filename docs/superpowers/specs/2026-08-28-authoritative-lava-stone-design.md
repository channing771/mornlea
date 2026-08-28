# B-28 岩浆与造石设计

## 状态

- 日期：2026-08-28
- backlog：B-28
- 设计分支：`docs/B-28-lava-stone-design`
- 设计基线：`b6d3a90d`
- 内容确认：用户已批准任务选择、单队列架构、地下岩浆囊、30 tick 岩浆传播、造石矩阵、玩家持续燃烧、橙色边缘反馈、兼容性边界与七组交付拆分
- 实施状态：仅完成设计；A-04、A-05、B-11 合入并清理版本与编号冲突前，不晋升、不认领、不创建实现 change

## 背景

当前世界只有水一种流体。服务端通过 `internal/fluid.Queue` 在固定预算内推进水，所有结果经 `sim.recordChange` 进入普通区块 revision、存档和发布链；Rust engine 独占 worldgen 和流体斜面 mesh，Rust client 独占 GPU 绘制。已有能力明确把岩浆、造石和黑曜石列为后续范围。

只增加测试可构造的岩浆能形成技术闭环，却不能改善正常生存体验。本设计因此同时交付新区块地下岩浆囊、水与岩浆反应、岩浆发光、玩家接触及离体燃烧和客户端状态反馈。它仍保持一个可独立评审和回退的闭环：玩家在新区域采掘时能发现危险岩浆，岩浆流出后可与水形成圆石或黑曜石，接触后会在有限时间内继续受伤，并可进入水中立即熄灭。

本设计不把两个流体过早扩张成数据驱动框架，也不建立第二条岩浆队列。水和岩浆共享同一个确定性、有界的权威推进器，只在固定种类、传播延迟、材质、发光和反应结果上分支。

## 当前基线与进入条件

设计基线的当前事实为：协议 v29、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、engine ABI v8、client ABI v9、benchmark scenario v19；`BlockIDMax=76`，Rust mesh registry 上限为 80，`MGW1` layout 为 2、header 为 564 bytes、材料表为 14 项。

这些数值只描述设计时的 `main`，不是实施时的预分配。B-28 的实现入口必须同时满足：

1. A-04、A-05、B-11 已合入 `main`，各自的 OpenSpec、版本、编号、capture 和归档均已收口；
2. A-05 与 B-11 对 metadata schema 的独立演进已在 `main` 上合并为单一迁移链；
3. `main` 工作树干净，B-28 与其他已认领任务不存在版本槽、方块编号、材质编号或文件冲突；
4. 从届时 `main` 重新读取所有 append-only 编号、registry 容量、`MGW1` layout、协议和 ABI，不沿用本文中的当前基线绝对值；
5. 建立新的 OpenSpec change `authoritative-lava-stone`，以本设计、届时代码测试和主规格为输入；
6. 若依赖合入后的架构使单队列、无 chunk schema 迁移、无 client ABI 变更或固定 HUD 容量任一前提不成立，先修订本设计并重新取得裁决。

## 目标

1. 追加一个源方块和七个流动等级组成的岩浆，以及一个黑曜石方块。
2. 在现有单一有界队列中推进水和岩浆，支持不同传播延迟、同种支撑和确定性的异种反应。
3. 在新生成区块中确定性放置完全封闭、单区块内的地下岩浆囊，保持旧区块逐字不变。
4. 让岩浆源接触水生成黑曜石、流动岩浆接触水生成既有圆石，并保证入队、预算和遍历顺序不改变结果。
5. 让全部岩浆等级发出等级 15 的方块光，并以原创程序化材质和流体斜面呈现。
6. 让玩家接触岩浆后最多持续燃烧 100 tick，进入水立即熄灭，伤害复用统一权威伤害与死亡路径。
7. 通过权威 `PlayerState` 驱动橙色边缘反馈；客户端不得自行预测燃烧状态。
8. 保持 Memory/TCP、整块 worldgen/单点 probe、重启前后和受限/不受限预算结果一致。

## 非目标

- 不做岩浆桶、水桶、岩浆物品、岩浆放置命令或无限岩浆源。
- 不做火焰方块、可蔓延火灾、烟雾、热浪、折射、岩浆环境音或独立 GPU pass。
- 不做通用状态效果系统；持续燃烧是玩家运行期的固定状态，不提供插件式 effect registry。
- 不让夜行者、伙伴或掉落物获得岩浆燃烧状态；夜行者白昼灼烧继续归 A-04。
- 不增加黑曜石物品、掉落、配方或用途；首版黑曜石可破坏但不产出物品。
- 不建设洞穴、完整生物群系、岩浆湖、地表火山或跨区块岩浆结构。
- 不让岩浆湿润耕地或复用水冲毁作物的掉落语义；岩浆破坏植被留给未来独立规则。
- 不修改 Rust physics 输入、碰撞积分或 client ABI。
- 不导入二进制美术或音频资源。

## 方案选择

### 采用：扩展现有有界流体内核

`core` 提供封闭的 `FluidKind` 和连续等级转换；`internal/fluid` 仍持有一个按位置与到期 tick 排序的队列；`sim` 持有水/岩浆延迟和权威写入；Rust worldgen、mesh 与 client renderer 继续持有各自既有职责。异种反应是同一候选归并过程的一部分，不在流体推进后追加旁路修补。

该方案只增加第二种真实流体所需的分支，复用已有确定性、预算、重扫、存档和发布证明，不预建没有第三个消费者的动态系统。

### 否决：独立岩浆队列与传播阶段

两条队列会让水和岩浆竞争同一空气格时需要跨阶段仲裁。若水先推进和岩浆先推进得到不同中间结果，系统必须再增加第三个反应阶段或回滚写入；预算、固定点、重启收敛和顺序无关证明也会重复。局部隔离不能抵消长期的双实现成本。

### 否决：数据驱动通用流体框架

以 registry 配置粘度、等级、光照、伤害和任意反应矩阵可以支持未来更多流体，但当前只有水和岩浆两个消费者。动态配置会扩大信任边界、热路径校验和错误面，而固定 switch 已足以清晰表达全部首版规则。

## 数据所有权与依赖方向

- `internal/core`：稳定方块编号、`FluidKind`、等级转换、方块发光/衰减/透明属性。
- `internal/fluid`：纯世界读取上的传播、反应、候选归并、单队列调度和确定性顺序；不依赖 `sim`、物品、协议或渲染。
- `internal/sim`：权威 tick 内的流体预算与延迟快照、区块重扫、玩家浸没与燃烧、统一方块写入和伤害结算。
- `internal/worldgen`：只编码 `MGW1`、调用 Rust 和解码，不增加 Go 生产生成器。
- Rust `mornlea_engine`：地下岩浆囊生成、整块/probe 同源、流体斜面 mesh 和方块光输入消费。
- `internal/network` / `internal/server`：`PlayerState.Burning` codec、校验、发布和 Memory/TCP parity，不决定燃烧结果。
- `internal/client` / `cmd/mornlea` / `internal/render`：只消费权威 burning 镜像并构造橙色 HUD 边缘，不反向确认模拟。
- Rust `mornlea_client`：继续只消费既有 terrain/water/HUD 上传，不增加新业务状态所有权。

权威 tick、网络、存储和渲染热路径不得增加无界集合、阻塞 I/O 或额外 goroutine。跨 goroutine 发布后的消息、候选和切片保持不可变。

## 方块、流体种类与属性

实施时在最终 `BlockIDMax` 前依次追加：

1. `LavaSourceID`；
2. `LavaLevel1ID`..`LavaLevel7ID`；
3. `ObsidianID`；
4. 新的 `BlockIDMax` 哨兵。

八个岩浆 ID 必须连续，source 的等级为 0，流动等级为 1..7。它们不映射物品、无碰撞、可被普通放置覆盖、交互射线可穿过；全部发光 15。黑曜石是不发光、完整碰撞、不透明的实心方块，可被采掘移除但不产生物品。

`core` 使用封闭类型和固定 switch：

```go
type FluidKind uint8

const (
	FluidNone FluidKind = iota
	FluidWater
	FluidLava
)
```

最小查询面为 `FluidKindOf(BlockID)`、`FluidBlock(FluidKind, level)`、`IsFluid`、`IsWater`、`IsLava` 和 `FluidLevel`。`IsFluid` 表示水或岩浆；所有实际只允许水的既有调用必须改为 `IsWater`，不能依赖“当前只有一种流体”的偶然事实。

农业湿润、作物水冲毁、氧气、水下视觉和水下环境只消费 `IsWater`。碰撞减速、放置覆盖、射线穿透、出生候选避开危险流体和通用 mesh 高度消费 `IsFluid` 或明确的 kind。未知和未注册方块返回 `FluidNone`，不得被当成等级 0 的合法流体。

发光与衰减继续消费唯一 `core.BlockEmission` / `core.BlockLightAttenuation` 表。全部岩浆等级 emission 为 15；水与岩浆的天空光/方块光衰减值由届时合流后的单一属性表表达，不在 assets、sim 或 shader 复制判定。

## 单队列传播与调度

### 延迟与预算

水保持默认 `FluidFlowDelayTicks=5`；新增 `LavaFlowDelayTicks=30`。两者都由 `sim.Tunables` 在 tick 入口取一次不可变快照，再传入 `internal/fluid.Queue.Advance`。两种流体共享现有 `FluidUpdatesPerTick` 和 `FluidRescanCellsPerTick`，不得为岩浆另设吞吐预算。

队列项仍只保存位置和 `dueTick`，不复制 fluid kind。处理时读取 tick 起始世界快照确定种类。同一位置被多次入队时继续保留最早到期值。变化格及其六邻重新入队时，水传播使用 5 tick、岩浆传播使用 30 tick；异种接触或竞争形成的反应候选使用相关两种流体中更早的到期时间，避免已相邻的水岩浆因岩浆慢速被延迟反应。

候选除目标 BlockID 外携带决定后续重排的 delay。候选归并必须同时满足：结果值确定、相同结果取最早 delay、操作可交换且可结合。实现可以使用固定值结构，不把动态 registry 或 transport 状态带入 `fluid` 包。

### 同种传播

只有同种流体能够支撑流动格：上方同 kind 流体，或水平同 kind 且等级更小的流体。异种流体不能让流动格存活。source 永不因普通传播自然消失；非 source 失去同种支撑后变为空气。

垂直优先与水平等级递减保持现有规则。传播到目标时必须携带 incoming kind；空气、同 kind 更弱的流动格和开启门可替换，source、同 kind 更强或相同等级、异种流体、关闭门、实心方块和作物不可替换。岩浆首版不复用水冲毁作物的原子掉落规则。

### 水岩浆反应

反应只检查六个面邻格，优先于本次普通传播：

| 当前岩浆 | 六邻含任意水等级 | 结果 |
|---|---|---|
| `LavaSourceID` | 是 | 当前岩浆格变 `ObsidianID` |
| `LavaLevel1ID..LavaLevel7ID` | 是 | 当前岩浆格变 `CobblestoneID` |

水格不被消耗。反应结果经同一 pending write、排序提交和 `sim.recordChange` 链写入，不能绕过 revision、存档或区块广播。

同 tick 水和岩浆都试图写入同一空气格时，该目标直接变为 `CobblestoneID`。反应固体优先于普通流体和空气候选；同 kind 流体之间继续取等级更小者。异种竞争、反应与普通传播的归并必须只依赖候选集合，不依赖 map 遍历、队列入队或区块加载顺序。

每个被处理位置最多检查六邻并产生固定数量候选。现有“每个处理项最多 4 个目标格”的成本说明必须更新为真实新上限；排序集合大小仍由 `FluidUpdatesPerTick × 固定候选上限` 约束。

### 重扫与固定点

`fluidSourceIsFixedPoint`、`fluidSectionIsFixedPoint` 和区块边界重扫必须识别异种六邻。即使 source 的五个传播方向都不可替换，只要六邻存在另一种流体，就不能被判为固定点。清空队列并重扫未平衡水岩浆边界后，必须收敛到与不中断运行相同的结果；真正平衡的水体、岩浆体和反应固体仍是重扫不动点。

## 地下岩浆囊 worldgen

### 生成分布

主世界 X/Z 每 `2×2` chunks 构成一个 supercell。固定 seed hash 在四个 chunk 中选择一个 host chunk；每个启用流体的 supercell 恰有一个候选岩浆囊，且每个 chunk 至多承载一个。负坐标必须使用 Euclidean division，保证 supercell 和 local 坐标在原点两侧连续确定。

host chunk 内参数由 seed、supercell 坐标和独立 salt 派生：

- 中心 local X/Z：各在 `4..11`；
- 中心 Y：`-39..-37`；
- X/Z 半径：各自为 2 或 3；
- Y 半径：2；
- 形状：纯整数椭球判定；
- 体积：约 33–73 个 `LavaSourceID`。

岩浆及其一格外壳必须完整位于 host chunk 内，且外壳不得是空气或其他流体。岩浆阶段只覆盖自然 stone/ore，不覆盖 bedrock、树木或海水。玩家挖开外壳后，普通权威流体队列才开始让 source 向外流动。

### 阶段顺序与出口一致性

Rust worldgen 顺序冻结为：

```text
地层与自然材料 / 矿石
→ 地下岩浆囊
→ 橡树
→ 海平面注水
```

`GenerateChunk` 与 `BaseBlockAt` 必须包含岩浆并逐格一致。`TerrainBlockAt` 继续保持纯自然材料语义，不返回岩浆；离线材料迁移使用该出口时不得向旧区块注入岩浆。LOD 只消费表层，不得因深层岩浆改变壳几何。

现有 `fluidEnabled` 同时门控自然水和自然岩浆，不增加第二个配置项。关闭时 `MGW1` 的 water 和 lava material 都编码为 `AirID`，Rust 显式跳过岩浆阶段；既有 dry worldgen golden 必须逐字不变。

### `MGW1` 与 ABI

设计基线的 `MGW1` 为 layout 2、14 项材料、564-byte header。B-28 在材料表尾部追加 lava source，移动 perm 起始偏移，并把 layout 与最终 engine ABI 各增加 1；C 函数签名和 dense/probe 输出布局不变。

若直接基于设计基线，派生结果是 layout 3、15 项材料、566-byte header，chunk input 574 bytes、probe 固定前缀 570 bytes、LOD input 582 bytes。实施不得直接复制这些预计数值，必须从最终 `main` 的 header、native 常量和 Rust parser 重新计算，并同步 Go/Rust header、FFI、native bridge、probe、LOD、golden 与混装拒绝测试。

材料校验允许 water 或 lava 分别等于 air 以表达门控关闭，也允许两者同时为 air；启用时 water 和 lava 不得相等或与其他材料冲突。非法 magic、layout、长度、材料、范围或输出容量必须在写输出前失败，不能发布部分 dense/probe/LOD 数据。

### 旧区块不改写

chunk palette 已能编码新的 BlockID，B-28 不改变区块记录布局，因此不因新增方块升 chunk schema。服务端只在 `LoadChunk` 返回缺失时请求生成；已保存区块直接恢复，不因 generator 规则升级重新生成。结果是：

- 已保存旧区块保持原 block、revision 和存储记录；
- 尚未生成或缺失的区块使用新规则；
- 旧 host chunk 不会让新邻区生成跨界半个岩浆囊；该 supercell 可以没有岩浆，但不能破坏旧内容。

## 玩家浸没与持续燃烧

### 浸没分类

现有水专属判断不能直接把 `IsFluid` 扩成水或岩浆，否则岩浆会错误触发溺水和水下蓝色视觉。共享判定改为至少产出：

```go
type SubmersionFlags struct {
	BodyInWater bool
	BodyInLava  bool
	EyeInWater  bool
}
```

Rust physics 的既有 `BodyInFluid` 输入取 `BodyInWater || BodyInLava`，因此水和岩浆都使用流体移动、阻力和摔落清除语义，不修改 physics ABI。氧气、水下视觉和出水恢复只读 `EyeInWater`。服务端权威与客户端预测从各自相同方块镜像运行同一个几何判定；客户端只预测运动浸没，不预测燃烧状态或伤害。

若身体同 tick 同时接触水和岩浆，水熄灭优先。出生候选把岩浆视为不可接受的危险流体，不把“完全浸没仍允许出生”的水域兜底扩展到岩浆；若候选范围只有岩浆，继续扩大/回退到既有安全出生锚点，而不是在岩浆中出生。

### 权威燃烧状态

每名 active 玩家持有两个不持久化的运行期计数器：

- `burnTicksRemaining`：离开岩浆后最多继续燃烧的剩余 tick，最大 100；
- `burnDamageTicks`：距下一次燃烧伤害的 tick，周期 20。

固定结算顺序为：

1. 身体接触水：两个计数器清零，本 tick 不产生燃烧伤害；
2. 否则身体接触岩浆：`burnTicksRemaining` 刷新为 100；首次进入燃烧时把 `burnDamageTicks` 设为 20；
3. 处于燃烧状态时递减伤害计数，到期经统一 `applyDamage(1)` 结算并重置为 20；
4. 未接触岩浆时递减 `burnTicksRemaining`，归零后同时清空伤害计数；
5. 所有伤害完成后继续走既有死亡结算。

因此持续站在岩浆中每 20 tick 受到 1 点伤害；离开后最多继续 100 tick，并可因水接触提前终止。进入岩浆不产生额外的无间隔首击。岩浆本 tick 新流入玩家身体格时，因玩家浸没阶段早于流体推进，最早从下一 tick 开始点燃；规格与测试必须锁定该阶段边界。

燃烧伤害不受难度影响，重置自动回血计时并可致死。死亡、断线、退回主菜单和新会话都清零燃烧状态；玩家 schema 不增加字段，停服墙钟也不补算。

## 协议与橙色边缘反馈

最终 `PlayerState` 的私有状态字段区追加严格 bool `Burning`，并把最终协议版本增加 1。字段的绝对偏移由依赖合入后的真实布局冻结；不得覆盖 A-04/A-05 已占用的 packet ID 或字段。codec 必须拒绝非法 bool、截断和尾随，Memory transport 也必须经过同一 Validate/codec 契约。

服务端每次发布玩家自身状态时令 `Burning = burnTicksRemaining > 0`。客户端预测器保存该权威镜像，但不能根据本地 `BodyInLava`、生命变化、方块镜像或输入自行开启/关闭燃烧反馈。陈旧 `PlayerState` 继续按既有 server tick/reconciliation 规则拒绝。

`Burning=true` 时，Go HUD 布局在现有 pass 中显示固定橙色屏幕边缘；false 时零 quad、零像素变化。每次实际扣血继续复用既有红色伤害边缘和 `CueDamage`，橙色边缘不能替代红色伤害确认。不增加火焰纹理、shader、独立 pass、动态 GPU 资源、火焰音频或 client ABI 字段。

断线、退回主菜单、capture 场景切换和新会话必须清除 burning 镜像，避免状态泄漏到下一局。固定 HUD quad 容量若在最终基线上不能容纳橙色边缘，实施必须回到设计层裁决，不能静默扩大上传布局或改变 benchmark 身份。

## 光照、mesh 与材质呈现

在最终材质层尾部 append `LayerLava` 和 `LayerObsidian`，不重排任何既有 layer。两者使用原创程序化纹理；黑曜石走普通不透明 terrain 几何，岩浆使用流体角高度但进入 terrain pass。水继续使用唯一半透明 water pass，不增加第三个透明池或 pipeline。

Rust mesher 只让“material 相同且 `fluid_height != 0`”的邻格参与角高度平均。水不得抬高岩浆角点，岩浆也不得抬高水角点；同种岩浆继续形成连续斜面。registry entry 保持现有固定字节布局，只调整最终 registry count/capacity。若追加方块超过最终 cap，必须成套提高 Go/Rust 固定 cap 并锁定恰好装满/超一拒绝，不能截断 registry。

terrain shader 显式识别 `LayerLava` 的流体角高度编码，不能把角高位解释成普通方块尺寸。相邻水和岩浆在反应提交前可能短暂共存：同种内部面隐藏，异种边界只由岩浆侧输出一个面，避免双面 z-fighting；反应固体发布后该临时边界自然消失。

全部岩浆等级通过唯一 emission 表发光 15。服务端若在 A-04 后提供局部方块光查询，客户端 mesh registry 与服务端必须消费同一 `core.BlockEmission`，不得各自建立岩浆特判。现有方块光 dirty、worker、revision、generation 和 presence 上限保持不变。

## 视觉验证

本行新增一个正式无窗口场景 `lava-pocket`，固定展示：

- 暗处的岩浆 source 与多个流动等级及其方块光衰减；
- 不同岩浆等级形成的连续斜面；
- 水接触 source 后的黑曜石；
- 水接触流动岩浆后的圆石；
- 水与岩浆短暂边界没有双面闪烁；
- 玩家处于权威 burning 状态时的橙色边缘，并同时能观察一次红色伤害反馈。

场景位置必须在最终场景表中重新协调，但继续保持 `far-horizon` 倒数第二、`water-underwater` 最后一项的硬约束。场景 `Apply` 显式清除前序菜单、HUD、浸没、伤害、远端实体和 burning 状态，不依赖前一场景。

按正式 producer 生成 `lava-pocket.png` 并逐图审核。既有 golden 原则上逐字节不变；共享方块光、fluid mesh 或 HUD 代码导致的任何变化必须逐图归因并明确批准，不能批量接受或放宽阈值。自动测试不得打开或聚焦前台游戏窗口。

## 兼容性与版本矩阵

实施时从最终 `main` 派生：

- 协议：因 `PlayerState.Burning` 增加 1；
- 玩家 schema：保持最终基线，不为 100-tick 运行态升版；
- 区块 schema：保持最终基线，palette 布局不变；
- 世界 metadata：保持最终基线，不记录 generator 版本或燃烧状态；
- `companions.ai` schema：保持最终基线；
- engine ABI：因 `MGW1` layout/material table 变化增加 1；
- client ABI：保持最终基线；
- benchmark scenario：保持最终基线，前提是固定上传布局和工作负载不变。

新增 BlockID 通过现有区块 snapshot/change wire 字段同步，不增加方块专用 packet。协议仍需升版，因为 PlayerState 布局变化，旧客户端必须在 Play 前按版本不匹配拒绝。旧程序不能安全解释含新 BlockID 的区块；项目不提供向后降级写入。

黑曜石没有 ItemID，ItemID 上界和玩家存档物品枚举不变。若实现调查发现现有 mining/drop 表无法表达“可破坏但无掉落”，优先复用流体/无掉落方块既有语义，不为黑曜石新增无用途物品。

## 错误与安全边界

- 无流动空间、异种之外的实心邻居和已平衡流体是正常无变化，不记 error。
- 未注册 BlockID、未知 `FluidKind`、等级大于 7 或 kind/连续区间不一致是内部不变量破坏，入口 fail closed。
- 流体 pending candidate、变更集合或 registry 超过固定容量时不得截断、提交部分世界或发布部分 mesh。
- `MGW1` magic/layout/长度/材料/输出容量错误必须在写输出前拒绝；Go/Rust 混装由 ABI 与 layout 双门禁阻断。
- worldgen 的 seed、supercell、host 选择和椭球判定只用确定性整数/hash，不使用全局 RNG 或浮点几何。
- 流体队列超预算的项原样顺延，不能丢失；反应结果不能绕过作物掉落、区块 revision 或存档失败语义。
- 非法 `PlayerState.Burning` bool、截断、尾随或陈旧 tick 不能进入客户端反馈状态。
- 音频不可用继续无声降级；燃烧事实、伤害和 HUD 不依赖音频成功。
- 权威 tick 不执行磁盘、网络、模型调用或无界扫描。

## 测试设计

### Core 与 assets

- 岩浆八级和黑曜石 append-only ID、注册上界、无 ItemID 与名称穷举。
- `FluidKindOf`、`FluidBlock`、`IsFluid`、`IsWater`、`IsLava`、`FluidLevel` 对全部已注册及未知 ID 穷举。
- 岩浆无碰撞、发光 15、流体衰减；黑曜石碰撞/不透明/不发光/无掉落。
- `LayerLava`、`LayerObsidian` append-only，程序化纹理确定且非透明空图。
- registry 最终 count 恰好接受，cap+1 原子拒绝。

### Fluid 与 sim

- 水默认 5 tick、岩浆默认 30 tick；未到期不处理，同位置取更早到期。
- 同 kind source/上方/水平等级支撑，异种不支撑；等级 7 不再水平传播。
- source lava + 六方向任意水等级均生成黑曜石；flowing lava + 水均生成圆石；水不消耗。
- 水/岩浆同 tick 竞争空气生成圆石；候选正反序、map 顺序与区块顺序结果一致。
- 反应优先于传播，固体候选优先，重复候选只提交一次 revision。
- 受限/不受限预算平衡态一致；清队列重扫与不中断运行一致；平衡态重扫零变化。
- fixed-point 快路径不能跳过异种六邻；单 tick 探视、候选和排序规模满足新固定上界。
- 岩浆不湿润耕地、不按水冲毁作物；开启门可流入、关闭门与实心方块阻挡。

### Worldgen 与 ABI

- `fluidEnabled=false` 的既有 dry golden 逐字不变。
- 固定 seed/坐标重复生成逐字一致，正负 supercell 使用 Euclidean division。
- 每个启用 supercell 恰有一个 host 候选，每 chunk 至多一个岩浆囊。
- 岩浆囊体积 33–73、全部 source，外壳非 air/非 fluid，岩浆及外壳不跨 chunk。
- `GenerateChunk` 与 `BaseBlockAt` 在内部、边界和外壳逐格一致；`TerrainBlockAt` 永不返回 lava。
- 旧区块保存后升级 generator，block/revision/记录保持不变；缺失邻居按新规则生成。
- `MGW1` 新 layout/header/chunk/probe/LOD 长度、material offset、Go/Rust ABI 和非法输入输出不变性。
- LOD 表层结果不因地下岩浆改变。

### Survival、协议与客户端

- 水、岩浆、空气及身体同时跨两种流体时 `SubmersionFlags` 权威/预测一致。
- 岩浆使用流体移动但不消耗氧气、不触发水下蓝色视觉；水仍立即熄灭。
- 首次接触后第 20 tick 扣 1，持续接触每 20 tick 扣 1；离开后剩余 100 tick 上界精确。
- 水与岩浆同 tick 接触时水优先；死亡、断线、新会话清零；重启不恢复。
- 燃烧伤害经 `applyDamage` 重置回血并可致死；难度不改变伤害。
- 流体在玩家阶段后流入身体格时下一 tick 才点燃。
- `PlayerState.Burning` round trip、严格 bool、截断、尾随、fuzz 和协议旧版本拒绝。
- Memory/TCP 对 burning、health、death/reset、流体方块和反应结果投影一致。
- 客户端只有权威 Burning 能开启橙边；本地浸没、输入、health 和 block 预测都不能触发。
- 断线、菜单、场景与新会话清零；false 时零 quad/零像素差异，true 时容量内固定橙边。

### Mesh、光照与 capture

- 同 kind 岩浆角高连续，水/岩浆角高不互相平均。
- 水保持 water pass，岩浆和黑曜石走 terrain pass；registry entry 布局不变。
- 异种临时边界只有岩浆侧一个面，无 z-fighting；反应后边界消失。
- 岩浆 emission 15 经 core 唯一表进入服务端查询和 mesh registry。
- `lava-pocket` 状态构造、场景顺序、固定容量、golden 和既有场景无未解释变化。

## 七组交付

正式 OpenSpec tasks 按以下顺序拆分，每组由 fresh implementer 以 TDD 实现，并接受独立 SPEC 与 QUALITY 双评审：

1. **OpenSpec 与合流后基线冻结**：读取最终编号、版本、registry/material/HUD 容量、阶段顺序和 A-04/A-05/B-11 契约；创建 proposal、delta specs、design、tasks、ledger，并 strict validate。
2. **Core 多流体模型与程序化材质**：追加方块和种类查询，收敛 water-only 调用，登记属性、名称、无掉落与 `LayerLava`/`LayerObsidian`。
3. **单队列慢岩浆、造石与固定点**：不同 delay、同种支撑、异种反应、候选归并、重扫、预算证明和流体性能报告。
4. **Rust worldgen 与 engine ABI**：地下岩浆囊、`MGW1` material/layout/header、chunk/probe/LOD 同源、旧区块不改写与跨语言门禁。
5. **光照、mesh 与 pass 分流**：岩浆 emission、同材质角高度、terrain shader 解码、异种边界、registry cap 和 Rust focused tests。
6. **玩家 burning、协议与 HUD**：浸没分类、100/20 tick 状态、统一伤害、`PlayerState.Burning`、橙边、生命周期 reset 和 Memory/TCP parity。
7. **视觉与整分支收尾**：`lava-pocket` producer/golden 和逐图审核；整分支规格/质量终审、完整门禁、OpenSpec sync/archive、backlog/Discussion、PR、CI、合入与 worktree 清理。

不得把 Task 4 的 worldgen ABI 与 Task 5 的 mesh/render 错误混在同一评审；不得在 Task 6 顺手扩展夜行者或通用状态效果。

## 验证门禁

任务级验证按受影响包和 Rust crate 选择最低相称层级。整分支至少运行：

```bash
make rust
go test ./internal/core ./internal/fluid ./internal/worldgen ./internal/lod ./internal/physics ./internal/sim ./internal/network/... ./internal/server ./internal/client ./internal/assets ./internal/mesh ./internal/render ./internal/render/hud ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
go vet ./...
make dev-check
make test-race-changed
make test-race
make rust-check
make visual-check
openspec validate --all --strict --no-interactive
git diff --check
```

worldgen/mesh/渲染变更先按仓库要求构建 Rust。golden 只通过正式 producer 更新并逐图审核。性能数字只记录，不放宽 overflow、数据丢失、固定容量、输出完整性或 I/O 错误门禁。

## 风险与缓解

1. **依赖分支版本与编号冲突**：A-04、A-05、B-11 都会改变实施基线。通过“依赖全部合入后才认领”和 Task 1 重新冻结消除预分配。
2. **异种候选破坏结合律**：若候选归并依赖处理顺序，Memory/TCP 和预算会分叉。用反应优先级、同 kind 强度和最早 delay 构成显式全序/结合操作，并做排列测试。
3. **固定点漏看异种六邻**：会让重启后的水岩浆永久不反应。固定点测试直接构造五个传播方向不可替换但第六邻异种的反例。
4. **`IsFluid` 扩展污染水专属语义**：会让岩浆湿润耕地、触发溺水或蓝色水下视觉。用 `IsWater` 审计和负面矩阵锁定。
5. **worldgen 半囊或旧区块改写**：跨 chunk 结构在新旧边界会被截断。岩浆和外壳完全限制在 host chunk，且 `TerrainBlockAt` 不含岩浆。
6. **燃烧状态与伤害反馈漂移**：客户端自行推断会产生假橙边。只接受权威 `PlayerState.Burning`，伤害继续由 health 变化驱动红边。
7. **registry/HUD 固定容量不足**：依赖合入后容量可能变化。Task 1 先算真实最坏值；不足时回设计，不静默扩大 ABI 或 benchmark 身份。
8. **岩浆收敛过慢**：30 tick 延迟是玩法选择，不能通过独立预算加速。性能报告记录队列长度和到达平衡的活动 tick，但只以单 tick 工作上界作硬门禁。

## 回退

实现未发布时可整支回退，恢复水-only `FluidKind`、旧 `MGW1`、旧协议和旧 capture。已发布后不能复用旧协议或 engine ABI 版本号；正常回退应发布新的版本，保留新 BlockID 的解码或显式拒绝。

旧世界中已经保存的岩浆、黑曜石或造石结果是普通方块数据。回退构建若不知道这些 ID，必须拒绝加载，不能静默转成空气、水或石头。玩家 burning 不进存档，因此不需要数据迁移回退。

## 已决结论

- 采用单一有界队列，不建设第二套岩浆系统或数据驱动流体框架。
- 新区块每个 `2×2` chunk supercell 选择一个完全位于单 host chunk 内的地下岩浆囊。
- 水默认 5 tick，岩浆默认 30 tick；共享同一处理预算。
- 岩浆源接触水变黑曜石，流动岩浆接触水变圆石，水不消耗。
- 黑曜石首版无物品、无掉落、无配方或用途。
- 玩家接触岩浆后最多持续燃烧 100 tick，每 20 tick 受 1 点伤害；水当 tick 熄灭。
- 只对玩家应用持续燃烧；夜行者、伙伴和掉落物不扩展。
- 客户端只由权威 `PlayerState.Burning` 显示橙色边缘，不做火焰模型或 shader。
- 实施等待 A-04、A-05、B-11 合入，随后重新冻结所有版本、编号和容量。
- 交付拆为七组，worldgen ABI 与 mesh/render 分开评审。
