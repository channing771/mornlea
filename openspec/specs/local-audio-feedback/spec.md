# local-audio-feedback Specification

## Purpose

在不影响模拟、网络与渲染的前提下，为图形客户端的有效本地操作和权威确认提供有界、可静音降级的本地音频反馈。

## Requirements

### Requirement: 本地 cue 只在确认或有效本地操作边界播放

图形客户端 SHALL 只在有效本地 UI 操作、成功采掘完成、收到本会话首次有效的 `PlaceBlockSucceeded`、成功进食完成、收到伤害确认，以及权威确认位置上身体浸没标志（`BodyInFluid`）由 false 变 true 的入水边沿这些边界各播放对应的一次本地 cue。预测帧率路径上的浸没状态、拒绝、失败、空白或禁用 UI、旧/重复放置序号、持续浸泡、出水、未加载方块镜像处的求值结果和同一确认的重复镜像 MUST NOT 播放 cue。同一权威状态同时首次确认进食完成与伤害时，两个 cue MUST 各播放一次，不得以优先级丢弃任一确认。

#### Scenario: 有效本地 UI 操作播放 click

- **GIVEN** 玩家点击可合成按钮、首次选择有效来源、再次点击同格取消，或完成一个可发送的合法移动请求
- **WHEN** 本地操作生效，且需要发送的请求已成功发送
- **THEN** MUST 播放恰好一个 `CueUIClick`

#### Scenario: 无效本地 UI 操作无声

- **GIVEN** 玩家点击空白、未确认或禁用项、不可合成按钮、把熔炉输出作为目标，或请求发送失败
- **WHEN** 客户端处理该点击
- **THEN** MUST NOT 播放 cue

#### Scenario: 成功事件播放一次

- **GIVEN** 客户端收到任一既有四类事件的首次成功确认
- **WHEN** 客户端处理该确认
- **THEN** MUST 播放恰好一个对应 cue

#### Scenario: 非确认路径无声

- **GIVEN** 预测、拒绝、失败或重复确认
- **WHEN** 客户端处理该状态
- **THEN** MUST NOT 播放 cue

#### Scenario: inactive 状态使旧采掘目标失效

- **GIVEN** 客户端已记录 active 采掘目标，随后收到新鲜的 `MiningActive=false` 权威状态或对应拒绝后的 inactive 状态
- **WHEN** 更晚的无关已应用增量把旧目标移除
- **THEN** MUST NOT 播放采掘完成 cue

#### Scenario: 正常采掘完成保留 cue

- **GIVEN** 服务端按顺序先发布成功移除目标的方块增量，再发布 `MiningActive=false` 权威状态
- **WHEN** 客户端依次处理两条消息
- **THEN** MUST 在增量边界播放恰好一次采掘完成 cue

#### Scenario: 同一状态独立确认进食与伤害

- **GIVEN** `InventoryState` 已确认选中食物恰减一件
- **WHEN** 下一条新鲜 `PlayerState` 同时确认 hunger 上升与 health 下降
- **THEN** MUST 各播放一次进食完成 cue 和伤害 cue

#### Scenario: 放置序号按会话去重

- **GIVEN** 客户端已消费某会话的一个 `PlaceBlockSucceeded` 序号
- **WHEN** 收到重复或更旧序号，或会话 reset 后收到新会话重新起步的序号
- **THEN** 前者 MUST NOT 播放 cue，后者 MUST 被视为新会话的首次确认

#### Scenario: 入水上升沿播放一次水花

- **GIVEN** 连续新鲜权威状态把玩家位置从无流体格序列带到流体格序列，客户端在每条状态上以共享 `physics.SubmersionFlags` 与只读方块镜像求值身体浸没标志
- **WHEN** 该标志在相邻两条已应用状态之间由 false 变 true
- **THEN** MUST 在该状态的确认边界播放恰好一次 `CueWaterSplash`

#### Scenario: 持续浸泡与出水无声

- **GIVEN** 玩家连续多条权威状态均处于身体浸没，随后回到非浸没
- **WHEN** 各条状态被应用
- **THEN** 持续浸没期间 MUST NOT 播放 cue，出水（true 变 false）MUST NOT 播放 cue；此后再次入水 MUST 视为新上升沿重新播放一次

#### Scenario: 未加载方块镜像按干燥处理

- **GIVEN** 权威位置所在区块尚未到达客户端镜像（`IsFluidAt` 按契约返回 false）
- **WHEN** 该状态上求值身体浸没标志
- **THEN** MUST 视为不在水中且不播放 cue；该求值结果与其他求值一样按字面记账为当条状态的浸没读数——因此玩家在持续浸没中跨越区块到达边界时，MUST 允许至多一次补响（客户端首次可知入水的时刻），这是接受的单 bool 读数语义的已知边界

#### Scenario: 会话重置清空浸没基线

- **GIVEN** 玩家处于身体浸没中收到 `Reset` 或未就绪状态
- **WHEN** 随后第一条新鲜权威状态仍显示身体浸没
- **THEN** 基线缺席 MUST NOT 视为上升沿，MUST NOT 播放 cue；此后须先离开水中再入水才重新触发

### Requirement: 放置成功确认建立唯一因果边界

协议 v26 SHALL 在 Play S→C ID 20 定义载荷恰为 `Sequence u64` 的 `PlaceBlockSucceeded`，并保持 ID 21 未分配。服务端只能在某 `PlaceBlock` 已于同一权威 tick 原子完成世界写入和恰减一件物品后，向发起会话发送恰好一个同序号成功确认。拒绝或失败 MUST 只发既有 `CommandRejected`，不得同时产生成功确认。

#### Scenario: 连续放置分别确认

- **GIVEN** 同一会话使用同一 slot/item 连续发起两个可成功放置命令
- **WHEN** 两个命令在同 tick 或跨 tick 完成
- **THEN** 发起会话 MUST 收到两个各自序号的成功确认，其他会话 MUST NOT 收到

#### Scenario: 拒绝放置没有成功确认

- **GIVEN** 某 `PlaceBlock` 被权威模拟拒绝
- **WHEN** 服务端发布本 tick 结果
- **THEN** 发起会话 MUST 只收到同序号 `CommandRejected`，且任何会话 MUST NOT 收到该序号成功确认

### Requirement: 协议 v26 拒绝旧对端

客户端与服务端 SHALL 只接受协议 v26，并 MUST 在 Play 之前拒绝 v25 及更早版本。此次升版 MUST NOT 修改存档 schema、engine/client ABI 或 benchmark scenario。

#### Scenario: v25 对端在握手阶段被拒绝

- **GIVEN** 客户端或服务端声明协议 v25
- **WHEN** 对端处理握手
- **THEN** MUST 在进入 Play 前以版本不匹配拒绝

### Requirement: 总音量严格配置

系统 SHALL 将顶层 `audioVolume` 作为本地 cue 总音量。字段缺席时 MUST 为 `0.7`；`0`、`0.25` 与 `1` MUST 原样加载并保存往返；小于 `0`、大于 `1`、`null` 和非数值 MUST 使配置加载失败。该字段 MUST NOT 被报告为未知字段，也 MUST NOT 出现在数值 `Fields()` 或调试面板中，且配置版本 MUST 保持 `1`。

#### Scenario: 合法音量往返

- **GIVEN** 含任一合法 `audioVolume` 值的配置
- **WHEN** 系统加载后保存并再次加载
- **THEN** 音量 MUST 保持相同值

#### Scenario: 非法音量失败

- **GIVEN** `audioVolume` 为 `-0.01`、`1.01`、`null` 或字符串
- **WHEN** 系统加载配置
- **THEN** MUST 返回带 `audioVolume` 上下文的错误

### Requirement: 不可用音频无声降级

系统 SHALL 在无头、非 Darwin 或 Darwin 设备初始化失败时构造无声播放路径。该路径 MUST 零初始化、不得创建音频设备，并且 MUST NOT 阻止客户端启动、模拟、网络或渲染。

#### Scenario: 平台或设备不可用

- **GIVEN** 无头、非 Darwin 平台或不可用的 Darwin 音频设备
- **WHEN** 客户端初始化本地音频
- **THEN** 系统 MUST 使用无声路径并继续启动

### Requirement: CombatHit 以严格递增确认触发固定原创 cue

图形客户端 SHALL 只在收到本会话 `ServerTick` 严格大于上一条已接受 combat 确认的合法 `CombatHit` 时播放恰好一次 `CueCombatHit`，并 MUST 使用独立于全局 application server tick 的 combat 去重状态。输入、预测射线、目标 health 镜像、inventory 耐久变化、重复或陈旧确认 MUST NOT 播放该 cue。`CueCombatHit` MUST 由既有程序化 synth 和预分配播放队列生成，固定参数为 1323 samples、520→180 Hz、amplitude 10500；little-endian PCM SHA-256 MUST 为 `17752cdda0232ebb88b0e6db1e39fa4a4889e5469bac0c28a07044b677710dae`。音频设备不可用 MUST 继续无声降级，不得影响 hit marker 或权威命中事实。

#### Scenario: 新确认播放恰好一次固定 cue

- **GIVEN** combat 去重状态尚未接受任何确认
- **WHEN** 客户端收到合法 `CombatHit{ServerTick:1}`
- **THEN** MUST 播放恰好一次 `CueCombatHit`，生成 PCM 的样本数、频率、幅度和 SHA-256 MUST 与固定值完全一致

#### Scenario: 重复与陈旧确认无声

- **GIVEN** 客户端已经接受 `ServerTick=2` 的 combat 确认
- **WHEN** 随后收到 tick 2、1 或 0 的确认
- **THEN** MUST 不播放 cue，也 MUST 不重新武装反馈

#### Scenario: 非确认状态变化无声

- **GIVEN** 玩家持续 primary input，目标 health 或选中剑耐久镜像发生变化，但没有新鲜 `CombatHit`
- **WHEN** 客户端处理这些输入与镜像
- **THEN** MUST 不播放 `CueCombatHit`

#### Scenario: 音频不可用不吞命中反馈

- **GIVEN** 客户端使用无声音频路径
- **WHEN** 收到严格递增的合法 `CombatHit`
- **THEN** cue 播放 MUST 无声降级，combat 确认与 HUD marker MUST 仍正常接受
