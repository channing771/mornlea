# Mornlea 项目指南

## 项目定位

本仓库是 Go 1.26 编写的独立体素游戏 Mornlea，Go module 为 `github.com/channing771/mornlea`，包含自研客户端、权威服务端、世界存储、物理和 Rust wgpu 渲染。它不追求兼容官方 Minecraft 的协议、存档或版权资源。

当前代码基线是 `authoritative-farming` 之后叠加远环 LOD 变更 `rust-engine-lod-shell` 的变基整合态，已经包含协议 v26（v26 新增仅回发成功放置发起者的 `PlaceBlockSucceeded` 序号确认；v24 为 authoritative-hunger 上线的 `PlayerInput.Eating` 与 `PlayerState.Hunger`；v23 起登录成功应答携带权威世界种子，供客户端确定性生成远环壳；v22 为 authoritative-farming 交付的翻地命令段）、Memory/TCP 共用登录与权威模拟、TCP 直连、无图形专用服务端、稳定玩家身份、玩家 schema v7、区块 schema v9、世界 metadata v2、独立 `companions.ai` schema v4、最多八名玩家的局域网同步与远端玩家呈现，以及最多四个可配置、服务端权威的具名伙伴（M5C 起可经任务移动、跟随、采掘与放置，M5D 起具备性格台词与近期记忆；M5E 清偿 M5B–M5D 终审欠账，规划步骤的字段排他把显式 JSON null 视为字段出现、与非法值同拒）。伙伴使用独立 `CompanionID`，active 与 inactive 身体记录合计最多 64 条；客户端通过统一 Avatar/NameTag pass 呈现伙伴（移动按远端玩家同机制插值），并提供有界 Unicode 聊天输入与 HUD。M5B 交付第一个 AI-native 闭环：`@伙伴名 指令` 寻址成功后进入每伙伴 16 条持久 FIFO（满员 QueueFull 同步拒绝），OpenAI-compatible Planner 在 worker 上把有界观察快照转为严格 JSON `go_to` 计划（30 秒超时不重试、64 KiB 上限、密钥只从环境变量解析），确定性整数 A*（4096 节点、固定邻居序，归属 Go）与路径点重验驱动 Task Runner 经 `sim.CompanionAction`（玩家命令后按 ID 字节序、统一物理积分、复用 Rust 出口）移动伙伴；任务状态机每次迁移广播 ChatEvent，deadline 用持久化 WorldTimeTicks；`companions.ai` schema v2 落盘当前任务与 FIFO（文件上界 350,208 bytes、v1 只读迁移）。M5C 交付伙伴世界交互与持续跟随：计划步骤全集扩展为 go_to/follow/mine/place 四 kind（观察快照追加在线玩家集合），follow 是最后一步的无限期任务（4 格水平跟随距离内停止移动、deadline 豁免、目标离线按 TaskFailWorldChanged 失败），mine 先走近再复用玩家权威计时采掘规则、完成 tick 内方块改空气、扣工具耐久与产物入背包三方原子（背包无空位以 TaskFailInventoryFull 失败且不破坏方块，容器与多掉落方块被计划与模拟双重拒绝），place 先走近再在单一权威 tick 内按玩家放置规则原子扣料写入；`@伙伴名 停止`（精确文本）是唯一绕过 FIFO 的控制指令，作用于持续跟随任务时转入 Stopped 终态、广播 TaskStopped 并立即执行原队首任务，否则只向发令者同步拒绝（NotFollowing）；协议 v18 追加 TaskStopped/NotFollowing 与 TaskFailReason 扩展，`companions.ai` schema v3 任务区步骤按 kind 变长（13/15/17 bytes、文件上界 430,080 bytes、v2/v1 只读迁移）。M5D 交付性格与近期记忆（完全旁路的表达平面，任务事实平面零语义变化）：`ai.companions[].persona` 可选内联人设（≤4,096 bytes）或配置同目录 `personas/<canonical 名称>.txt` 外部文件（内联优先、双源告警、损坏降级空人设；`Definition.Persona` 是磁盘镜像、`ResolvedPersona`（`json:"-"`）是生效值，Save/迁移绝不吸收外部文件或清除越界原文）；Dialogue worker 与 Planner 共用 endpoint/model 但提示与输入完全隔离（人设 + 最近摘要 + 事实节点 + 极小环境摘要），台词触发节点确定性（进入 Running 一次、按计划长度均匀选择 ≤6 个步骤完成节点、终态一次——末个选中步骤折入终止节点，每任务 ≤8 次；follow 仅开始/首次到达跟随距离/终止三节点），共享全服 4 模型槽（Planner tick 边界重试、Dialogue 无槽即跳过不排队）、每伙伴 1 在途、世代过时即丢、模型失败只跳过台词绝不改任务状态；终态响应捎带 ≤2 KiB 最近对话摘要（失败保留旧摘要、只喂后续 Dialogue，persona 与摘要绝不进入 Planner）；协议 v19 追加 `CompanionSpeech`（伙伴身份 + ≤256 bytes 台词、广播全部在线玩家，任务事件仍不携带模型自由文本），客户端以 `伙伴名：台词原文` 一行呈现（唯一显示模型文本的位置）；`companions.ai` schema v4 每记录可选摘要区（文件上界 438,280 bytes、v3/v2/v1 只读迁移、inactive 不存摘要、解码 payload 读空门禁）。`authoritative-fluid`（M4 系列基础玩法补齐第一批）交付服务端权威的水：`internal/core` 追加 8 个稳定流体编号（`WaterSourceID` + `WaterLevel1..7ID`，1 最强），流体不进物品表、不可放置、不提供碰撞体、非不透明；新包 `internal/fluid` 以显式待更新队列驱动流动，处理前按 `(dueTick, ChunkKey, y, z, x)` 全序排序（绝不遍历 map），受 `FluidUpdatesPerTick`（默认 512）硬约束、超额顺延不丢弃，同 tick 冲突写入取流体等级最小者且与处理次序无关；队列**不持久化**，重启靠边界重扫恢复——「平衡态是重扫的不动点」在当前规则集下可证（支撑关系良基故无自持环，`level(c) = 1 + min(水平邻居 level)` 归纳即得不动点唯一）。`sim.Engine.Step` 新增 `advanceFluids` 阶段，只推进活动兴趣范围，变更经既有 `recordChange` 汇入同一批 `pendingChunkChanges`，不新增任何协议消息类型；重扫入队另有 `FluidRescanCellsPerTick`（默认 65536）预算并跨 tick 分摊，内部水源可证为不动点故海洋重扫从 O(体积) 降到 O(表面)。worldgen 注水在 Rust 内与高度图、分层、矿石、树木同批完成，`Materials` 13→14，门控放 Go 侧（关闭时 `water` 字段传 air 编号、Rust 侧零分支），故关闭态生成路径与门控引入前逐位一致；材料表互异性校验为 `(air, water)` 开且仅开这一对豁免。配置项 `fluidEnabled` **默认开启**；benchmark 与 capture 两条路径都强制读编译默认值、不随用户配置漂移，其中 benchmark 另钉死不注水（固定工作负载必须与「默认是否注水」这个产品决策解耦），capture 跟随编译默认值故 golden 覆盖默认注水的世界。`fluid-presentation-survival` 交付了流体的呈现与生存：斜水面几何（角高度取相邻流体格 `h_raw = 14 - level` 的整数平均、任一参与格上方为流体取 15，全整数运算，角高度打进 quad 释放的位、quad 仍是 8 字节）；按 material 分流的半透明 water pass（深度测试开、深度写关，排在 terrain pass 之后 HiZ build 之前，按区段距离由远及近，不接 GPU culling，水面实例池 2Mi 条 = 32 MiB 一次性固定显存，预热后零每帧动态资源）；天空光穿过流体每格额外衰减 1（`RegistryView` 的 `light_attenuation` 查表，方块光模型不变、水与玻璃一样阻断）；权威与预测共用同一对浸没标志 `BodyInFluid`/`EyeInFluid`（`physics.SubmersionFlags` 一处实现），驱动水中积分（重力衰减、垂直终端速度压低、`Jump` 改持续上浮、水平阻力，全部走 tunable）与入水消除摔落伤害；权威氧气 `MaxOxygenTicks = 300`，眼睛浸没时每 tick −1、归零后每 `DrownDamageIntervalTicks`（默认 20）经既有伤害入口扣 1 血、出水立即回满、不入存档，协议 v21 随 `PlayerState` 同步并由 HUD 氧气条（未满时才出现、复用同一 HUD 图集与 pass）呈现；相机浸没时叠水色 tint 并压低可见半径（判定复用同一个 `EyeInFluid`，不存在第二套）；八处 `core.RaycastBlocks` 调用点的 solid 谓词区分流体，水下可瞄准、采掘、开箱与放置；出生点选取不把流体格判为落脚点。capture 视觉门禁新增 `water-surface-slope` 与 `water-underwater` 两个水景场景，后者 MUST 排在场景表最后；远环 `far-horizon` 场景插在它之前（倒数第二）。`authoritative-farming` 交付了服务端权威的农业：`internal/core` 有 10 个稳定农业方块编号（`FarmlandDryID`/`FarmlandWetID` 与 `WheatStage0..7ID`，阶段即编号故区块 schema 不变）与 4 个物品编号（`ItemStoneHoe`/`ItemIronHoe` 及其损坏形态、`ItemWheatSeeds`、`ItemWheat`，锄头接既有耐久体系与固定配方 9/10）；mesh registry 条目上限为 48（Rust `MAX_REGISTRY_ENTRIES`、Go `nativeMaxRegistryEntries` 与 `maxNativeInputBytes` 三处各自硬编码、手工同步，两侧一致由喂满一次跨语言调用的容量测试守，`input.rs` 的 `BLOCKS_BYTES` 里那个 27 是 3×3×3 邻域区段数、与本上限无关）；协议 v22 的 `TillSoil` 是三个农业动作里唯一的新命令（种植复用放置、收获复用采掘），手持锄头对泥土或草、目标正上方为空气且在既有触及距离内时把目标格变耕地并扣减恰好一点耐久——`tool-durability` 的判据因此是「工具确实完成了一次有效作用」而非「方块是否被移除」，四条拒绝路径均不扣耐久；作物 × 完好锄头是唯一豁免，锄头收获作物不磨损，锄头破坏非作物仍按既有规则磨损。`sim.Engine.Step` 的 `advanceCrops` 阶段排在 `advanceFluids` 之后、`finishChanges` 之前（耕地干湿读流体方块，须先流动后判湿；变更与其他方块变更共用同一批 revision 与广播），按 `(ChunkKey, sectionY)` 全序枚举活动兴趣范围内的已加载 section，每 section 用 splitmix64 纯整数哈希抽 `RandomTicksPerSection`（默认 3）格，抽中的作物在露天（读既有列顶高度图，非空气即遮挡）且正下方是湿耕地时按 `CropGrowthChancePercent`（默认 50）推进一阶段；单 tick 触及的格数恒等于「已就绪活动区块数 × 24 区段 × 抽样数」，与世界里的作物数量无关。耕地干湿在被抽中时按「水平切比雪夫距离 ≤ 4、同层或上一层存在流体方块」重判并双向转换。掉落数量由 `(worldSeed, 权威 tick, 维度, 方块坐标)` 的 splitmix64 纯整数哈希确定且同输入重放一致：成熟小麦收 1–3 小麦 + 1–3 种子（两类独立抽取、种子下限 1 保底耕种循环不死），未成熟 1 种子、耕地 1 泥土，作物采掘 1 tick、耕地 5 tick；伙伴由 `internal/sim/mining.go`、`internal/companion/plan_types.go` 与 `internal/sim/companion_placement.go` 三处防御清单显式拒绝种地与收获。缺失玩家一次性材料包在 14 格材料紧随其后补一格 64 颗小麦种子，HUD 合成面板有 10 行（含石锄与铁锄）。`plant-visual-presentation` 是通用的植物几何能力：每格出 4 片交叉斜面 quad（`face` 字段 6/7 表两条对角线、植物 quad 恒为 1 的 `w`/`h` 位中一位表正背，`face ∈ {6,7} ⟺ material ∈ 植物区间` 在 Go 解包与 Rust 发射端双向强制），quad 仍是 8 字节、bit 63 仍空闲、走既有 terrain pass 的 alpha cutout 且零新绘制阶段，不参与贪心合并，光照取正上方相邻格，天空光穿过植物不额外衰减。作物零碰撞体、耕地碰撞高 15/16——这是全仓第一个非满立方体碰撞体，新的 `BlockCollisionBoxes` 消费者不得假定「非空气即整格」。`authoritative-hunger` 交付了服务端权威的饥饿：每个玩家有饥饿值（`0..20`）、饱和度（千分位、不超过饥饿值×1000）与疲劳值（千分位）三层权威状态，全部定点整数，推进不含浮点——唯一的浮点入口是 `swimExhaustionMilli` 的水平位移换算，向下取整后立刻回到整数域。疲劳来源是五项固定表（刻意不做 tunable，比例关系即玩法）：起跳 50、身体浸没时每水平移动一格 10、采掘完成 5、翻地完成 5、自然回血每点生命 6000，全部判定点只在玩家的成功路径上；`applyExhaustion` 每累积满 `ExhaustionThresholdMilli`（默认 4000）就减去一个阈值并扣一点饱和度，饱和度为零改扣一点饥饿值、饥饿值为零不再扣，一次跨多个阈值逐个结算并保留余数，一次跨阈值最多消耗一种资源。自然回血由 `RegenHungerThreshold`（默认 18）门控，饥饿值低于它时既有回血计时照推但不产生回复；饥饿值为零后每 `StarvationDamageIntervalTicks`（默认 80）经既有 `applyDamage` 入口扣 1 血，生命值到 1 即停（计时冻结而非照推）、不致死。进食是与采掘同形的持续输入状态机：`PlayerInput.Eating` 为真、选中格在食物表内且饥饿未满时逐 tick 推进，开始时记录 `(slot, item)`，`EatingTicks`（默认 32）到时在单一 tick 内原子结算（扣 1 件、饥饿加该食物固定值并钳到 20、饱和加固定值并钳到饥饿值×1000）；松手、切换栏位、同格换物、受伤、死亡、打开容器界面或视野未就绪都清零进度且不扣料（中断优先于同 tick 的结算，恰在结算 tick 开箱也不扣料不回饱；关箱后从第 1 tick 重新推进）。食物表只有面包（饥饿 5、饱和 6000），由固定配方 11（3 小麦 → 1 面包）合成，`internal/core` 追加 `ItemBread`（堆叠 64、不可放置）。三层状态随玩家 schema v7 持久化，v6 存档按 `Hunger=20 / Saturation=5000 / Exhaustion=0` 迁移，死亡重生回同一初值；协议 v24 只上线两个字段——`PlayerInput.Eating` 与 `PlayerState.Hunger`（`Validate` 拒 >20），饱和度与疲劳值不上线。HUD 在既有 hotbar pass 与图集内新增快捷栏右侧的 10 格鸡腿饥饿条（程序化两列图标、半格粒度、满时仍显示、零新绘制管线、只画权威确认值），这 20 个 quad 使固定上传布局移动（quad 容量 247 → 267、glyph offset 12288 → 13312、总容量 45888 → 46912 bytes），benchmark scenario 因此为 v19、唯一显式场景迁移为 `18:19`。伙伴不接饥饿：`companionState` 没有三层状态、疲劳判定点不在伙伴路径、也不进食，由 AST 名字守卫、运行时三层状态守卫与进食守卫三条测试锁定。遗留与简化清单随 `authoritative-hunger` 的 `design.md` 归档。既有能力还包括权威快捷栏、持久掉落物、固定 36 格背包、固定合成、确定性矿石、多人共享权威熔炉、权威计时采掘、权威单件原地丢弃、服务端权威工具耐久、损坏物品、共享箱子、权威生命值与死亡结算和确认伤害红色边缘反馈、14 种常见块状材料与缺失玩家一次性材料包、世界坐标 terrain UV、玻璃/树叶单 pass alpha cutout、无窗口 `materials-showcase` 与 `ai-companion`、确定性自然材料与橡树生成、材料加工闭环、目标方块反馈、发光块固定配方、24000 tick 权威昼夜、客户端派生的 `0..15` 传播天空光与静态方块光、登录种子驱动的确定性远环 LOD 壳（海平面以下的窗口顶面钳到海平面并取水材质、水下窗不发裙边、陆海交界按钳制后高度由陆侧发裙，流体关闭时逐位回到干盆地呈现），以及程序化方块云。M5A 继承 M4Q 的 Mornlea 项目身份和 M4P 固定 Rust 1.97.1 `mornlea_engine` cdylib；Rust 是 mesh/light、collision resolver、raycast、物理 tick 积分与世界生成（地形/矿石/橡树/海平面注水/远环壳，engine ABI v6——v6 新增 `mornlea_lod_shell` 远环壳出口，v5 与 v4 已被 fluid 系列占用）的唯一生产实现，客户端窗口、事件循环与全部 GPU 渲染（terrain/sky/云/culling/HiZ/实体/文本/HUD，窗口 surface 与离屏）由独立 `mornlea_client` cdylib（winit + wgpu，client ABI v9——v9 起 UI 下行保留 layout v1 主菜单并新增 layout v2 设置页，上行改为有版本、有类型、有长度且固定 64 条上限的结构化事件批，容量不足时整批不写、不计数、不出队；v8 起 egui 菜单出口（字体上传/事件排空）、v7 起雾参数化 setter、v6 起远环 tile 出口、v5 为 water pass）独占生产、Go 已无 GLFW 与 WebGPU 依赖，`internal/gfx` 已删除；Go 仍拥有 app、world、sim、network、storage、render 的 CPU 半部（布局、编码、上传调度）、物理 state/input/tunable/snapshot 编码、yaw 三角与 prism 构建、worldgen seed→perm 播种与 `world.Chunk` 回写，旧 Go 积分与 worldgen 实现只作测试 oracle，只有 `internal/nativeabi` 接触 engine C ABI，`internal/mesh` 与 `internal/physics` 是领域调用方，且没有生产 Go fallback。客户端命令为 `mornlea`，专用服务端为 `mornlea-server`；专服保持无图形，但 Linux 发布必须把相邻 `libmornlea_engine.so` 作为同一不可跨版本混装的 release unit。benchmark scenario 为 v19；当前唯一显式场景迁移是 `18:19`，性能数值只记录，报告完整性、身份、真实 overflow、数据丢失和 I/O 错误仍是门禁。M2 v15 与 M5 v14 基线保持原字节。TCP 仅面向可信局域网且没有认证或加密。已交付里程碑与协议/存档版本演进见 `docs/notes/progress.md`；`docs/superpowers/` 是历史背景，当前行为必须以代码、测试与 OpenSpec 主规格核实。

当前协议演进明确为：v26 新增只回发成功放置发起者的 `PlaceBlockSucceeded(sequence)` 序号确认；v25 把既有 `Mining` 连续输入定义为 primary action；v24 新增 `PlayerInput.Eating` 与 `PlayerState.Hunger`。v25 的服务端权威玩家近战在同一维的 active 玩家中取 3 格内射线最近命中，等距以 `SessionID` 决定；固体方块遮挡、流体穿透，每次命中经既有伤害入口结算 2 点伤害并给目标施加 10 tick 冷却。同 tick 先冻结全部近战意图再统一结算，命中者该 tick 抑制采掘、下一 tick 由连续输入重新判定；Memory 与 TCP 复用同一权威路径。

Darwin 图形客户端提供本地确认音频：不引入第三方依赖或二进制音频资产，启动时预分配 `AudioQueue` 与固定 PCM，设备不可用、队列繁忙或播放失败均无声降级。顶层配置 `audioVolume` 控制总音量；采掘、放置、进食与受伤只在权威确认边界播放 cue。放置只消费 v26 `PlaceBlockSucceeded(sequence)`，重复或旧序号、拒绝、无关状态和其他会话均静音；capture、benchmark 与无图形专用服务端不请求音频设备。

图形客户端默认在完整程序化材质注册表上应用内嵌的 Pixel Perfection 子集，未映射的 layer 最终回退到程序化材质。可选顶层配置 `texturePackPath` 只在启动时从本地目录读取 16×16 PNG 并按逻辑 layer 逐层覆盖；benchmark 与 capture 忽略本地覆盖，无图形专用服务端不加载客户端资产。该材质能力沿用远环 LOD 合入后的协议 v23、engine ABI v6 与 benchmark scenario v18，未推进这些版本（client ABI 其后经 egui 主菜单变更升到 v8）。

图形客户端已使用原创程序化生存 HUD：关闭容器时显示居中九格快捷栏，选中格以外扩高对比外框和强调色内框构成双层轮廓，数量与工具耐久继续显示权威镜像；采掘反馈以移动末端标记和固定警示缺口区分可采与不可采形状。HUD 图集在物品列前固定为空/半/满心、空/满气泡、空/满鸡腿七个程序化 cell；已确认生命与耗损氧气各用十个 resolved 单实例槽表达，已确认饥饿用十个常驻空槽加最多十个填充、奇数值只覆盖末个鸡腿右半边，未确认值不显示。生命与饥饿各以 169px 设计宽度锚定快捷栏左右边缘；满氧隐藏只省略气泡实例，耗损氧气沿饥饿右边缘向外堆叠，关闭态在其上方、打开态在其下方；永久两行状态栈参与缩放和采掘/聊天锚点，并避让可交互格，聊天面板与字形使用同一有界缩放比保持在 framebuffer 内且不与状态栈相交。HUD 保持 benchmark scenario v19 的 267 quad、700 glyph、13312-byte glyph offset 与 46912-byte 固定上传容量；15 个无窗口 capture 场景依次为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。

图形客户端的背包/合成、熔炉与箱子打开态已统一为原创程序化容器像素界面：固定 HUD atlas 的凹槽、三种标题、火焰与箭头 cell 分别供三类 overlay 复用，每个 overlay 只追加一个零 glyph 标题 quad，面板仅向上扩 20px header，栏位与命中几何保持原值；熔炉复用原有进度 bar quad，以火焰自下向上、箭头自左向右裁剪表达权威进度。普通背包、熔炉、箱子的统一栏位仍分别为 36、39、63 格，十条固定配方和两次点击整堆权威移动语义不变；最大打开态为 266 quad，仍装入 267 quad、700 glyph、13312-byte glyph offset 与 46912-byte 固定上传容量。无窗口 capture 现有 19 个正式场景，`main-menu`、`settings-menu` 依次插在 `water-surface-slope` 之后、`far-horizon` 之前（`far-horizon` 仍倒数第二、`water-underwater` 仍最后），`chest-container`、`furnace-container` 紧随 `inventory-crafting`。

图形客户端已交付基于 egui 0.35 线（`wgpu` 29 同线、不引入 `egui-winit`、关闭 `default_fonts`）的工具型 UI 基础、主菜单与设置页：交互式客户端启动停留在主菜单（标题「Mornlea」、中心纵排按钮列（进入游戏/设置/退出游戏可用，多人游戏禁用）、底部版本行与红色错误行），打开世界存档、启动本地权威服务端、登录与远环播种全部延迟到点击「进入游戏」之后；`-connect`、benchmark 与 capture 路径跳过交互菜单；菜单期间光标不捕获、游戏输入不生效；装配失败在菜单内显示错误行且可重试或退出。设置页只包含总音量、材质包目录与 `640x360`/`960x540`/`1280x720` 三个固定 16:9 窗口预设；Go 持有 committed/draft，取消恢复已保存值，dirty 返回不静默丢弃，保存按「读取变化且非空的材质候选并完整校验、单次读取并完整校验最新配置、只 patch 三项 raw JSON 且保留其他原文、rename 原子提交、更新 committed、应用当前进程音量与窗口（Rust 按当前 NSWindow 所在 `NSScreen.visibleFrame` 的逻辑 point 与 style/chrome 钳制 content 并重定位 outer frame，Go 再守物理 framebuffer 上限）」执行；rename 前失败保留草稿且磁盘与运行态均不改变，rename 后目录同步异常则显式标记已提交、应用新运行态并显示持久性警告。材质 atlas 不热替换，保存的候选下次启动生效。配置格式仍为 v1，新增可选 `windowSize`（缺省 `1280x720`），`texturePackPath` 必须为最多 1024 个 UTF-8 字节且无 CR/LF 的单行原文。菜单字体复用已授权（OFL-1.1、含 provenance）的 Noto Sans CJK SC 经 client ABI 一次性上传，不新增二进制资产；egui 以一条附加 render pass（排在 HUD/debug 之后）挂进既有渲染器，帧不携带 UI 段时完全零参与；既有程序化 HUD、容器 overlay、debug 面板与聊天呈现不迁移。该能力演进自选型文档 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md`（工具型 UI 选型：egui，弃 iced/Slint/RAUI/bevy_ui/Go GUI/webview）。benchmark scenario 保持 v19、协议 v26、engine ABI v6、区块 schema v9、世界 metadata v2、玩家 schema v7 与 `companions.ai` schema v4 不变；除启用设置按钮而更新的 `main-menu.png` 和新增的 `settings-menu.png` 外，其余既有场景 golden 逐字节不变。

Raycast 生产路径同样由 Rust `mornlea_engine` 独占 DDA 遍历；`internal/core` 只保留输入校验、一次归一化、64-record batch 驱动、惰性 callback 与 `RayHit.Point`，旧 Go DDA 只是测试 oracle，没有生产 fallback。

出生支撑已清偿 farming 遗留：`findSpawnInColumn` 枚举候选方块提供的全部碰撞盒，按真实盒顶从高到低复用 `playerBoundsAreFree` 与 `playerSupport` 的完整支撑判定；玩家与伙伴共用该出生路径，登录恢复与 grounded safe 点也沿用同一支撑语义。协议、存档 schema、engine/client ABI 与 benchmark scenario 均不变。

## 开始工作前

1. 阅读 `openspec/config.yaml`。
2. 若任务属于某个 OpenSpec change，依次阅读该 change 的 `proposal.md`、delta specs、`design.md` 和 `tasks.md`。
3. 只读取 `docs/superpowers/` 中与当前变更直接相关的资料，再用代码和测试核实现状。
4. 检查 `git status`，保留用户已有及与任务无关的改动。
5. clean checkout 先运行相应 Make Rust target，再执行直接的 focused Go 命令。

复杂功能、新模块、跨包重构、存档/协议变更和性能契约变更应走 OpenSpec。小型拼写修复、纯格式修改和一次性实验可以直接修改，但仍须完成相称的验证。具体流程见 `docs/openspec.md`。

## 架构边界

- 服务端是世界和玩家状态的唯一权威；客户端持有只读镜像并进行预测/呈现。
- 单机模式和远程模式必须复用同一套模拟与校验逻辑，不能为单机绕过传输边界。
- 内部包的允许依赖以 `internal/archcheck/dependency_test.go` 为准；新增包必须同步登记并证明依赖方向合理。
- GPU 只经 Rust `mornlea_client` 渲染器;任何 Go 包不得导入 WebGPU 绑定(archcheck 全仓禁止)。
- `sim` 不得依赖渲染包，`world` 不得依赖 `network`。
- 跨 goroutine 发送成功后的消息及其切片视为不可变；重 CPU、磁盘和网络工作不得阻塞权威 tick 或渲染热路径。
- 不得放宽既有正确性、资源上限、报告完整性、真实 overflow 或数据丢失门禁来让测试通过；benchmark 与 `perfcheck` 的性能数值只保存记录，不改变退出状态。
- 仓库不得加入 Mojang 版权材质或其他未经授权的二进制美术资源。

## 实现约定

- 修改保持聚焦，优先复用现有抽象，不顺手重构无关代码。
- 新增或修改行为时先写失败测试，再完成最小实现和重构。
- 测试文件编排遵循「按关注点一文件」：测试与被测代码同目录（同包白盒或 `foo_test` 外部测试包），不做集中式 `tests/` 镜像目录——`go test` 只认同目录 `_test.go`，白盒断言也依赖同包。一个测试文件只装一个主题或一条被证性质；多主题巨型测试文件在因任何改动被触碰时应顺势按关注点拆分。拆分是零行为变化重组：测试函数名与子测试名一律不动，`go test -list` 的前后一致按**集合语义**验收（声明顺序随文件拆分必然改变）。命名判据：`property_` 前缀表「被证性质」，`_integration`/`_restart`/`_fuzz`/`_parity`/`_oracle` 等后缀表「测试种类」，两者语义不同，不得叠加出 `property_x_integration_test.go` 一类的名字。Rust 侧的等价映射（`#[cfg(test)]` 主题子模块文件 + helper 中心）见 `docs/test-organization.md` 的「Rust 映射」一节。
- 共享测试 helper 每包只设一个中心：优先扩展包内既有的 `*_helpers_test.go`，没有才新建 `helpers_test.go`，不得另立并行中心；纯 helper 文件必须以 `*_helpers_test.go` 命名，不得顶着普通测试文件名却不含任何测试函数。helper 落位前先 grep 其引用文件集合：被多于一个测试文件引用的才迁入中心，单文件私有的留在消费它的测试文件内；中心超过约 500 行或横跨两个以上不相干域时应按域再拆。范例：`internal/fluid` 的 `helpers_test.go` 与 `property_{rescan,budget,order,converge}_test.go`（由 805 行的 `property_test.go` 按被证性质拆出）。判定判据（主题混装是唯一硬判据、单主题大文件不拆）、拆分操作步骤与验收清单见 `docs/test-organization.md`；Rust 侧的等价映射（`#[cfg(test)]` 主题子模块文件 + helper 中心）见该文件的「Rust 映射」一节。
- Go 代码必须经过 `gofmt`；错误要保留上下文，生命周期资源必须可关闭且关闭应安全。
- 长期基线文档（本文件的「项目定位」）只陈述**当前存在的能力与契约版本**。「本变更不做 X」是单个 OpenSpec change 的非目标，写在该 change 的 `proposal.md` 里并随它一起归档，不要写进这里：这类否定式断言在别处新增能力时会静默变假且现场没有任何信号——本文件就曾因此留着「伙伴不创建 Planner/FIFO/路径/persona/摘要」达四个里程碑，而那些能力早已交付。稳定的架构事实（「不追求兼容官方 Minecraft」「TCP 没有认证或加密」「没有生产 Go fallback」）是设计边界而非能力缺口，不在此列。契约版本号一侧由 `internal/archcheck` 的 `TestBaselineVersionsMatchCode` 兜底。
- `AGENTS.md` 与 `CLAUDE.md` 必须**逐字节相同**，由 `internal/archcheck` 的 `TestBaselineDocsAreIdentical` 兜底。二者曾因只更新其中一份而分叉，导致 `CLAUDE.md` 的基线描述滞后四个里程碑——改任何一份都必须同步另一份。
- 代码注释、GoDoc 和 Rust doc comment 说明必须使用中文；Go/Rust 标识符、wire magic、协议字段名、外部 API 名称和约定俗成的技术术语保留英文。
- 注释必须丰富且有信息量：Go 的每个导出标识符都应有中文 GoDoc，Rust 的每个导出项都应有中文 doc comment（`///`）；关键算法、复杂逻辑、边界条件、并发约束、unsafe/FFI 边界、magic 数值和"为什么这么做"的设计决策必须配中文注释。注释应解释意图与权衡，而不是机械复述代码。
- 注释中提及 Go 标识符（类型、函数、方法、字段、常量）必须用反引号包裹（`` `core.BlockIDMax` ``、`` `Queue.pending` ``）；`internal/archcheck` 的注释标识符门禁据此检查其在全仓是否仍然存在，裸词不在检查范围。该门禁只抓「标识符已删除或改名」，枚举式计数（「三个阶段」）与说明性数字（「约 N µs」）的漂移不在其覆盖内，须靠评审；前两个变更各出过多次「注释里的名字已被同一个 commit 删掉」的失真，人工检查已证实挡不住。
- 文档、测试说明和开发者可读文本优先使用中文；Go 标识符、wire magic 和约定俗成的技术术语保留英文。
- 涉及协议、存档格式或基准输出的变更必须说明兼容性与迁移策略，并保留 golden/fuzz/故障注入覆盖。
- 自动测试不得启动或聚焦前台游戏窗口；只有用户明确要求人工验收时才运行交互式客户端。
- `.codex/hooks.json` 与 `.claude/settings.json` 共用 `scripts/agent-hooks/guard.mjs`。Hook 失败时修复根因；不得关闭、改写或用 `MORNLEA_HOOKS_ALLOW_NO_SPEC=1` 绕过，除非用户明确批准例外。

## 开发工作流

- 所有开发任务（OpenSpec change 执行、多步骤修复与重构）必须以 `subagent-driven-development` 规范执行：每个任务派发全新的 implementer 子代理，任务 brief 是其唯一需求来源；任务完成后必须通过独立的任务评审（规格合规 + 代码质量双裁决）；评审发现进入修复循环，单任务最多 5 轮，超限逐条裁决并记录；全部任务完成后进行整分支终审。
- 执行进度、评审结论与所有裁决（ruling）必须记录在 ledger 文件；控制会话只负责派发、协调与裁决，不得绕过子代理直接实现；implementer 子代理不得自行派生子代理或评审者。
- 小型拼写修复、纯格式修改和一次性实验沿用「开始工作前」的直接修改豁免，不强制子代理流程，但仍须完成相称的验证。

## 验证

按风险从小到大执行：

```bash
make rust
go test ./path/to/affected/package -race -count=1
go test ./internal/archcheck -count=1
go test ./... -race
go vet ./...
gofmt -l .
```

`gofmt -l .` 应无输出。渲染、tick、存储或协议热路径发生变化时，还要运行对应 benchmark、fuzz/golden 测试或 `cmd/perfcheck`，其性能数值只记录；报告完整性、真实 overflow 和数据丢失仍是门禁。

迭代期（本地开发循环）用快检替代全量：`make dev-check` 依次执行 `gofmt -l .` 检查、`go vet ./...`、`go test ./... -short` 与 Rust `fmt --check`/`clippy --workspace --all-targets -- -D warnings`/`test --workspace --locked`。`-short` 只跳过实测耗时 ≥1.5 秒的重型测试（capture 场景、benchmark 场景、长 tick 集成/容量测试），是「快」与「正确性」分离的迭代工具：CI 与最终门禁不带 `-short`，短模式不放宽任何正确性门禁。按改动域定点测试的命令映射、Go 缓存与 `-count=1` 的使用纪律见 `docs/notes/test-quickstart.md`。需要 race 覆盖时用 `make test-race-short`（`go test ./... -race -short`，与 `-short` 跳过同一组重型测试，本机实测约 5 倍提速）；任务闭环的 race 覆盖优先用 `make test-race-changed`（`scripts/agents/race-changed.sh` 从 git diff 求改动包及其反向依赖闭包、恒含 `internal/archcheck`，`RACE_BASE` 可换基线）。全量 `make test-race` 仍用于提交前与 CI，且本地不与 CI 双跑：CI 的 `go-race` 三分片并行是全量兜底，本地全量只在并发时敏域变更或复现 CI 失败时执行。测试分层纪律（T0–T3）与负载 flake 分诊协议（高负载下等待预算类失败先单独重跑失败包判定，不进修复循环）见 `docs/notes/test-quickstart.md`。上述全量门禁（`make rust-check`、`go test ./... -race`、对应 golden/benchmark/`perfcheck`）保持不变。

OpenSpec 产物提交前执行：

```bash
openspec validate --all --strict --no-interactive
```

## OpenSpec 纪律

- `spec.md` 只描述可观察行为和验收场景；实现选择放在 `design.md`；执行顺序放在 `tasks.md`。
- 代码实现与计划不一致时，先更新 change 产物，不能只改代码让规格失真。
- 归档前完成验证并核对所有任务；归档的意义是把稳定行为沉淀到 `openspec/specs/`，不是简单清理目录。
- 旧的 `docs/superpowers/` 保留为历史背景，不批量迁移；后续主规格通过真实 change 的归档逐步形成。

## 自动 Hook 门禁

- `PreToolUse` 阻止高破坏性的 Git、强制推送和宽范围递归删除命令。
- `PostToolUse` 在文件编辑后检查本次改动中的 Go 文件是否已经 `gofmt`。
- `Stop` 检查 diff、OpenSpec、架构依赖、受影响包的 race 测试和 `go vet`；协议、存档、性能基线、依赖边界或跨组件实现改动必须关联完整 active change。
- Hook 是机械护栏而非安全沙箱；CI 仍是最终共享门禁，人工评审仍负责语义正确性。
