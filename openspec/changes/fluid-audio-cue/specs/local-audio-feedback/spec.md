# fluid-audio-cue Delta

## MODIFIED Requirements

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
- **THEN** MUST 视为不在水中：既不播放 cue，也不把它记为上升沿的前驱真值

#### Scenario: 会话重置清空浸没基线

- **GIVEN** 玩家处于身体浸没中收到 `Reset` 或未就绪状态
- **WHEN** 随后第一条新鲜权威状态仍显示身体浸没
- **THEN** 基线缺席 MUST NOT 视为上升沿，MUST NOT 播放 cue；此后须先离开水中再入水才重新触发
