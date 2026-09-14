## MODIFIED Requirements

### Requirement: 工作负载变化使用新场景版本

Avatar、NameTag 与 Hotbar HUD 为容纳最多七名远端玩家、四个伙伴、一个目标标签和固定聊天 overlay 而改变固定 GPU 上传布局、offset 与每帧写入字节数后，benchmark 报告 MUST 标记为 scenario v16。被测进程因流体呈现与生存能力而改变（mesh registry 条目加宽、quad 位布局改写、光照传播改为按方块查衰减表、新增水面绘制阶段与其固定容量实例缓冲、物理 `StepInput` 头版本递增）后，benchmark 报告 MUST 标记为 scenario v17。被测进程因农业而再次改变（mesh registry 条目上限与实际烘焙条目数同时抬高，使每次 mesh 调用的 FFI 输入变长；合成面板行数增加，使 Hotbar HUD 的固定上传布局、glyph offset、总容量与空聊天帧每帧实际写入字节数全部移动；权威 tick 新增一个每 tick 枚举活动兴趣范围内全部区段的作物阶段）后，benchmark 报告 MUST 标记为 scenario v18。被测进程因饥饿而再次改变（Hotbar HUD 新增饥饿条，使其固定上传布局、glyph offset、总容量与空聊天帧每帧实际写入字节数全部再次移动；HUD 图集在既有爱心之后新增两列程序化图标；权威 tick 新增饥饿三层状态的推进与结算）后，benchmark 报告 MUST 标记为 scenario v19。被测进程因客户端 UI 对齐而再次改变（Hotbar HUD 新增准星、物品名弹条、容器浮动面板描边与悬停 tooltip 背景，使其固定上传布局、glyph offset、总容量与空聊天帧每帧实际写入字节数全部移动；HUD 图集既有 cell 程序化重绘但不新增列；权威 tick 语义不变）后，benchmark 报告 MUST 标记为 scenario v20。被测进程因常显 HUD 层迁出 GPU 而再次改变（GPU 保留面只保留容器浮动面板族与悬停 tooltip：关闭容器界面的帧从 v20 关闭态最坏 100 quad/548 glyph 预算降到 0 quad/0 glyph，打开任一容器界面的最坏从 264 quad/700 glyph 预算收缩到 218 quad/268 glyph 预算——tooltip 按 8 rune 截断上限封顶，注册表实测最长名见证 262；固定 quad 上限 320、glyph 上限 768、glyph offset 15616 bytes、总容量 52480 bytes、48-byte instance 与 256-byte 区间对齐全部保持不变，缩出的容量全部沉淀为分支最坏与每帧实例前缀的缩小，因此每帧写入字节数移动；HUD 图集按整张贴图上传的契约与列布局不变；权威 tick 语义不变；benchmark 无头观察路径零 WebView 参与保持）后，benchmark 报告 MUST 标记为 scenario v21。被测进程因自然短草而再次改变（稳定方块与 mesh registry 追加短草、植物材质判定集合从 `[31..54]` 变为 `[31..54] ∪ {68}` 且 `55..67` 不移动、worldgen `MGW1` 请求扩为 layout `3`、engine ABI 升为 v10、固定 benchmark 世界出现确定性短草）后，benchmark 报告 MUST 标记为 scenario v22。被测进程因统一方块更新调度器与世界流式收尾而再次改变（耕地湿度重判的积压消费从 FIFO 改为统一调度器确定性全序、流体定时队列与随机抽样哈希链迁移到统一调度器、服务端多玩家探针按会话视距启用区块订阅与流式、存档 I/O 按类别与 region 并行化并新增 region 句柄有界治理、报告新增 `streaming` 记录性指标族）后，benchmark 报告 MUST 标记为 scenario v23。benchmark 的世界内容 MUST 与「注水是否默认开启」这一产品决策解耦：benchmark 路径的注水门控 MUST 被钉死为关闭，MUST NOT 读取用户配置，也 MUST NOT 随默认值翻转而改变，因此 v17、v18、v19、v20 与 v21 的被测世界 MUST 与 v16 逐格一致，且 MUST NOT 含任何农业方块；v22 与 v23 MUST 继续不注水且不含农业方块，但 MAY 只因自然短草生成层在合格空气格新增 `ShortGrassID`。固定 benchmark 输入 MUST 继续是七名远端玩家、零伙伴且不注入聊天；v17、v18、v19、v20、v21、v22 与 v23 MUST 保持 `2560×1440` 离屏目标、阶段时长、运动、样本、绝对阈值与 `20%` 相对回归阈值不变，其与前一场景的唯一差异 MUST 是上述被测进程自身的变化；v23 在既有指标族之外 MUST 只新增 `streaming` 记录性指标族。p99、FPS、RSS、GPU、tick、队列高水位、绝对阈值和相对回归结果 MUST 只记录和报告，不得导致 producer、比较器或 CI 失败；只要报告结构、字段和样本完整且数据有效，producer MUST 写出 JSON。既有 scenario v6 至 v22 报告与基线 MUST 保持可读取，比较器不得把不同 scenario 静默作相对比较。当前不同场景之间 MUST 只接受唯一显式 `22:23` 迁移；该迁移 MUST 校验报告完整性与硬件身份并跳过跨 workload 的相对回归判定。历史 `21:22`、`20:21`、`19:20`、`18:19`、`17:18`、`16:17`、`15:16`、`14:15` 与更早迁移只作为既有报告和归档证据，不再是当前可授权迁移。任何其他迁移参数、损坏或缺字段报告、样本不完整、硬件身份不兼容、transport 或 commit 身份不一致、真实 overflow 或数据丢失以及 I/O 错误 MUST 继续失败；队列高水位数值本身 MUST 只记录。

#### Scenario: v16 固定上传布局完整

- **GIVEN** scenario v16 使用固定七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备 Avatar、NameTag 与 Hotbar HUD 固定上传
- **THEN** Avatar MUST 容纳 66 个 body parts、instance 区 5280 bytes、indirect offset 5536、总上传 5556 bytes
- **AND** NameTag MUST 容纳 12 个标签、background 区 768 bytes、glyph offset 1024、glyph 区 24576 bytes、总上传 25600 bytes
- **AND** Hotbar HUD MUST 容纳 236 个 quad 与 700 个 glyph、glyph offset 11776、总容量 45376 bytes，空聊天帧实际写入 MUST 为 11776 bytes

#### Scenario: v17 只因被测进程自身的变化区别于 v16

- **GIVEN** scenario v17 使用与 v16 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** 离屏目标、阶段时长、运动、样本与指标 MUST 与 v16 相同
- **AND** 二者的唯一差异 MUST 是被测进程自身因流体呈现与生存而改变的部分（registry 条目宽度、quad 位布局、光照衰减查表、水面绘制阶段与其固定实例缓冲、`StepInput` 头版本）

#### Scenario: v18 只因被测进程自身的变化区别于 v17

- **GIVEN** scenario v18 使用与 v17 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** 离屏目标、阶段时长、运动、样本与指标 MUST 与 v17 相同
- **AND** 被测世界 MUST 仍不注水且不含任何农业方块
- **AND** 二者的唯一差异 MUST 是被测进程自身因农业而改变的部分（mesh registry 条目上限与烘焙条目数、Hotbar HUD 固定上传布局与 glyph offset、权威 tick 的作物阶段）

#### Scenario: v18 的 Hotbar HUD 固定上传布局已移动

- **GIVEN** scenario v18 使用与 v17 相同的空聊天输入
- **WHEN** producer 准备 Hotbar HUD 固定上传
- **THEN** Hotbar HUD MUST 容纳 247 个 quad 与 700 个 glyph、glyph offset 12288、总容量 45888 bytes，空聊天帧实际写入 MUST 为 12288 bytes
- **AND** v17 的同一组数值 MUST 是 238 个 quad、glyph offset 11776、总容量 45376 bytes（v16 的 236 个 quad 与 v17 的 238 个只差氧气条，两者的 quad 区对齐到同一个 256 字节边界，故 offset 与总容量在 v16→v17 未移动），因此 v17 与 v18 两代报告的每帧上传字节数 MUST NOT 被直接比较

#### Scenario: v19 只因被测进程自身的变化区别于 v18

- **GIVEN** scenario v19 使用与 v18 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** 离屏目标、阶段时长、运动、样本与指标 MUST 与 v18 相同
- **AND** 被测世界 MUST 仍不注水且不含任何农业方块
- **AND** 二者的唯一差异 MUST 是被测进程自身因饥饿而改变的部分（Hotbar HUD 的饥饿条与其固定上传布局、HUD 图集新增的两列程序化图标、权威 tick 的饥饿状态推进）

#### Scenario: v19 的 Hotbar HUD 固定上传布局已移动

- **GIVEN** scenario v19 使用与 v18 相同的空聊天输入
- **WHEN** producer 准备 Hotbar HUD 固定上传
- **THEN** Hotbar HUD MUST 容纳 267 个 quad 与 700 个 glyph、glyph offset 13312、总容量 46912 bytes，空聊天帧实际写入 MUST 为 13312 bytes
- **AND** v18 的同一组数值 MUST 是 247 个 quad、glyph offset 12288、总容量 45888 bytes（饥饿条的 20 个 quad 使 quad 区跨过下一个 256 字节对齐边界，offset 与总容量因此同时移动），所以 v18 与 v19 两代报告的每帧上传字节数 MUST NOT 被直接比较

#### Scenario: v20 只因被测进程自身的变化区别于 v19

- **GIVEN** scenario v20 使用与 v19 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** 离屏目标、阶段时长、运动、样本与指标 MUST 与 v19 相同
- **AND** 被测世界 MUST 仍不注水且不含任何农业方块
- **AND** 二者的唯一差异 MUST 是被测进程自身因客户端 UI 对齐而改变的部分（Hotbar HUD 的准星、物品名弹条、容器浮动面板描边与 tooltip 背景及其固定上传布局、HUD 图集既有 cell 的程序化重绘）

#### Scenario: v20 的 Hotbar HUD 固定上传布局已移动

- **GIVEN** scenario v20 使用与 v19 相同的空聊天输入
- **WHEN** producer 准备 Hotbar HUD 固定上传
- **THEN** Hotbar HUD MUST 容纳 320 个 quad 与 768 个 glyph、glyph offset 15616、总容量 52480 bytes，空聊天帧实际写入 MUST 为 15616 bytes
- **AND** v19 的同一组数值 MUST 是 267 个 quad、glyph offset 13312、总容量 46912 bytes（准星、弹条与 tooltip 的容量重钉使 quad 区跨过下一个 256 字节对齐边界，offset 与总容量因此同时移动），所以 v19 与 v20 两代报告的每帧上传字节数 MUST NOT 被直接比较

#### Scenario: v21 只因被测进程自身的变化区别于 v20

- **GIVEN** scenario v21 使用与 v20 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** 离屏目标、阶段时长、运动、样本与指标 MUST 与 v20 相同
- **AND** 被测世界 MUST 仍不注水且不含任何农业方块
- **AND** 二者的唯一差异 MUST 是被测进程自身因常显 HUD 层迁出 GPU 而改变的部分（GPU 保留面只保留容器浮动面板族与悬停 tooltip，常显层绘制随迁移退役、固定上传布局不收缩、HUD 图集整张上传契约不变、权威 tick 语义不变）

#### Scenario: v21 的 Hotbar HUD 保留面已收缩

- **GIVEN** scenario v21 使用与 v20 相同的空聊天输入
- **WHEN** producer 准备 Hotbar HUD 固定上传
- **THEN** 固定 quad 上限 320、glyph 上限 768、glyph offset 15616 bytes、总容量 52480 bytes、48-byte instance 与 256-byte 区间对齐 MUST 全部保持不变，MUST NOT 因保留面收缩而收缩或重排
- **AND** 关闭容器界面的帧 MUST 恰好产生 0 quad 与 0 glyph（v20 关闭态最坏是 100 quad 与 548 glyph 预算），打开任一容器界面的最坏 MUST 恰好为 218 quad 与 268 glyph 预算、实测最长名见证 262（v20 打开态最坏是 264 quad 与 700 glyph 预算），权威命中 marker 的呈现已迁 WebView 组件、两分支 MUST NOT 再产生 marker quad
- **AND** 因此 v20 与 v21 两代报告的每帧 HUD 实例前缀字节数 MUST NOT 被直接比较

#### Scenario: v22 只因自然短草区别于 v21

- **GIVEN** scenario v22 使用与 v21 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** `2560×1440` 离屏目标、阶段时长、运动、样本、指标、绝对阈值与 `20%` 相对回归阈值 MUST 与 v21 相同
- **AND** 被测世界 MUST 仍不注水且不含任何农业方块
- **AND** 二者的唯一差异 MUST 是被测进程因自然短草而改变的稳定方块与 mesh registry、植物材质判定集合 `[31..54] ∪ {68}`（`55..67` 不移动）、worldgen ABI 及固定世界短草格

#### Scenario: v23 只因统一调度器与世界流式区别于 v22

- **GIVEN** scenario v23 使用与 v22 相同的七名远端玩家、零伙伴和空聊天输入
- **WHEN** producer 准备该场景
- **THEN** `2560×1440` 离屏目标、阶段时长、运动、样本、绝对阈值与 `20%` 相对回归阈值 MUST 与 v22 相同
- **AND** 被测世界 MUST 仍不注水、不含任何农业方块，且固定世界方块 MUST 与 v22 逐格一致
- **AND** 二者的唯一差异 MUST 是被测进程因统一方块更新调度器与世界流式收尾而改变的部分（湿度积压消费全序、统一调度器承载流体定时与随机抽样、多玩家探针按会话视距订阅、存档 I/O 并行化与 region 句柄有界治理、新增 `streaming` 记录性指标族）

#### Scenario: benchmark 世界内容不随注水默认值漂移

- **GIVEN** 一份把注水门控写为开启的用户配置，且编译期默认值也是开启
- **WHEN** 以 benchmark 路径启动 producer
- **THEN** 该次运行的注水门控 MUST 为关闭
- **AND** v21 的被测世界 MUST 与注水默认开启之前逐格一致
- **AND** v22 与 v21 的世界差异 MUST 只包含确定性自然短草格

#### Scenario: v23 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v23 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标（含 `streaming` 指标族）和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: v22 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v22 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: benchmark 观察路径零 WebView 参与

- **GIVEN** benchmark 以无头路径运行（离屏 renderer，不创建交互窗口）
- **WHEN** producer 准备并测量预热、still、flying 与 GPU 采样各阶段
- **THEN** 被测进程 MUST NOT 初始化、挂载或呈现任何 WebView，MUST NOT 下行任何桥状态（HUD 状态纪律层出口为 nil，零组装求值）
- **AND** 常显 HUD 的呈现职责属于交互客户端的 WebView 组件，其存在 MUST NOT 为被测帧增加桥下行或 WebView 合成工作量

#### Scenario: v21 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v21 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: v20 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v20 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: v19 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v19 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: v18 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v18 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: 只接受 18:19 跨场景迁移

> 标题沿用历史名（openspec 的 MODIFIED 漂移守卫不支持 Scenario 改名）；本变更后该场景语义为 22:23，历史 `21:22` 随 producer 升到 scenario v23 退役。

- **GIVEN** 一份 scenario v22 基线与一份 scenario v23 当前报告
- **WHEN** 比较器以显式 `22:23` 迁移参数运行
- **THEN** 比较器 MUST 校验报告完整性与硬件身份并跳过跨 workload 的相对回归判定
- **AND** 任何其他迁移参数（含已退役的 `21:22`、`20:21`、`19:20` 与 `18:19`）MUST 失败

#### Scenario: v17 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v17 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: v16 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v16 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

#### Scenario: v15 同场景比较只记录性能

- **GIVEN** baseline 与 current 都是完整有效且身份兼容的 scenario v15 报告
- **WHEN** 比较器执行同场景比较
- **THEN** 比较器 MUST 输出该场景既有绝对指标和相对回归记录，且任何性能数值变差都 MUST 返回成功

### Requirement: 追加材质层不改变固定 benchmark 工作负载

本变更在稳定编号上只追加一对树苗编号（`SaplingID` 与 `ItemSapling`）并把树苗材质层加入植物判定集合，该追加 MUST NOT 单独构成 benchmark 场景升版理由：固定 benchmark 世界 MUST 不含任何 `SaplingID`，被测世界的逐格内容与网格输出 MUST 与追加前逐格一致，离屏目标、阶段时长、运动、样本、指标、绝对阈值与 `20%` 相对回归阈值 MUST 全部保持不变。被测进程的植物材质判定集合现为 `[31..54] ∪ {68, 164}`（`55..67` MUST NOT 移动），engine ABI 现为 `v11`，mesh registry 的烘焙条目数由 `89` 增至 `90`；这些只改变被测进程的材质集合、ABI 版本与固定长度 registry 快照的条目数，MUST NOT 让任何新方块出现在固定 benchmark 世界。只追加编号而不改变固定 benchmark 世界方块的登记（雪层编号是既有先例）MUST NOT 单独触发场景升版；`v18` 关于 registry 条目数与 FFI 输入变长的记述是**该次**变更的历史升版理由（同批变更还移动了 Hotbar HUD 固定上传布局并新增了每 tick 的作物阶段），MUST NOT 被当作「每次追加编号都必须升版」的常设规则。场景版本与跨场景迁移授权随版本链当前状态走：该追加落地时场景为 `v22`、唯一可授权迁移为 `21:22`；统一方块更新调度器与世界流式收尾变更合入后场景为 `v23`、唯一可授权迁移为 `22:23`，该推进 MUST NOT 归因于材质层追加。

#### Scenario: v22 固定世界不含树苗且场景与迁移不变

- **GIVEN** scenario `v22` 的固定 benchmark 世界与当前被测进程
- **WHEN** producer 生成报告、比较器读取该报告
- **THEN** 被测世界 MUST 不含任何 `SaplingID`
- **AND** 该追加 MUST NOT 单独触发场景升版：固定 benchmark 世界出现的方块 MUST 与追加前逐格一致，网格输出与阈值 MUST 不变
- **AND** 跨场景迁移 MUST 只接受版本链当前授权的唯一迁移（本变更合入前为 `21:22`，合入后为 `22:23`），其它迁移参数 MUST 继续失败

#### Scenario: 追加编号与材质层只改变进程侧事实

- **GIVEN** 当前被测进程的植物材质判定集合、engine ABI 版本与 mesh registry 烘焙条目数
- **WHEN** mesher 与 benchmark 分别求值植物材质判定与 registry 快照
- **THEN** 植物材质判定集合 MUST 为 `[31..54] ∪ {68, 164}` 且 `55..67` MUST NOT 移动
- **AND** engine ABI MUST 为 `v11`，烘焙条目数 MUST 为 `90`，固定长度 registry 上限 MUST 保持不变
- **AND** 该追加 MUST NOT 单独触发 scenario 升版：固定 benchmark 世界出现的方块 MUST 与追加前逐格一致，网格输出与阈值 MUST 不变
