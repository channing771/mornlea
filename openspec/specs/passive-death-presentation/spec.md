# passive-death-presentation Specification

## Purpose

定义被动牛死亡后的可观察呈现：客户端按 despawn 原因位区分死亡与消失，死亡保留渲染 20 tick 并以红闪侧倒收尾，全部相位由权威 tick 派生，保证确定性可复现。

## Requirements

### Requirement: 死亡原因保留渲染并以红闪侧倒收尾

客户端 SHALL 在收到原因位为死亡的 `PassiveDespawn` 后保留该牛的渲染 20 tick：颜色向红插值（红闪）且身体 roll 转至 90°（侧倒），第 20 tick 后移除；原因位为消失时 MUST 立即移除，与既有语义一致。保留期间 MUST NOT 接受该 ID 的新 state（死亡后无权威更新），同 ID 的新 spawn MUST 按既有稳定规则重建身体。夜行者的 despawn 语义 MUST 不变。

#### Scenario: 死亡保留 20 tick 后移除

- **GIVEN** 客户端已有某牛镜像并收到其死亡原因的 despawn（`ServerTick` 为 T）
- **WHEN** 客户端推进到 T+19
- **THEN** 该牛 MUST 仍在渲染且呈现红闪侧倒中间态；推进到 T+20 后 MUST 被移除

#### Scenario: 消失立即移除

- **GIVEN** 客户端已有某牛镜像并收到其消失原因的 despawn
- **WHEN** 客户端处理该消息
- **THEN** 该身体 MUST 立即被移除，MUST NOT 进入 20 tick 保留

#### Scenario: 保留期间同 ID 重生按稳定规则处理

- **GIVEN** 某牛正处于死亡保留期
- **WHEN** 客户端收到同 ID 的新 spawn
- **THEN** 新身体 MUST 按既有重复 spawn 稳定规则处理（保留期状态不被新 spawn 污染）

### Requirement: 死亡相位由权威 tick 派生且禁用墙钟

死亡保留的红闪插值与侧倒角度 MUST 是 `(despawn ServerTick, 牛 ID, 当前 tick)` 的纯函数，与掉落物动画相位同形；MUST NOT 读取墙钟、帧间隔或本地随机数。相同 `(T, ID)` 序列重放 MUST 得到逐帧相同的颜色与角度。

#### Scenario: 相同 tick 序列重放一致

- **GIVEN** 同一死亡 despawn（T 与 ID 相同）
- **WHEN** 两次从 T 推进到 T+10 并记录每 tick 颜色与角度
- **THEN** 两次记录 MUST 逐 tick 相等

#### Scenario: 不同个体相位可辨

- **GIVEN** 同一 tick 死亡的两头牛（ID 不同）
- **WHEN** 推进到 T+5 并比较两者
- **THEN** 两者的红闪/侧倒相位 MUST 不恒等（ID 参与派生，避免整齐划一）

### Requirement: 服务端死亡原因事实来源有界

服务端 SHALL 在权威 tick 内结算死亡的同一 tick 记录死亡 ID 集合，并供同 tick 的会话发布投影为原因位；该集合为每 tick 有界（≤全服牛上限），MUST NOT 跨 tick 累积。出视野移除 MUST 恒为消失原因；死亡且同时出视野的个体 MUST 记死亡原因。

#### Scenario: 击杀当 tick 原因置位

- **GIVEN** 一头可见牛在本 tick 被伤害归零并移除
- **WHEN** 服务端发布本 tick 的 despawn
- **THEN** 该 ID 的原因位 MUST 为死亡

#### Scenario: 单纯出视野原因清位

- **GIVEN** 一头满血牛离开会话订阅范围但未死亡
- **WHEN** 服务端发布本 tick 的 despawn
- **THEN** 该 ID 的原因位 MUST 为消失

### Requirement: 死亡掉落关联呈现滞后于倒地

客户端 SHALL 把死亡 tick 窗口内、死亡牛身旁新出现的掉落与该死亡关联（同块邻域 + 有界 tick 窗，纯呈现启发式，不改权威掉落语义）：关联掉落在死亡相位 50% 之前 MUST 不渲染，50% 起以 scale-in 渐显并叠一次白色闪光；拾取判定不受影响（权威侧即时可拾）。

#### Scenario: 肉在倒地中途出现

- **GIVEN** 一头牛在 T 死亡（红闪侧倒 20 tick）
- **WHEN** 推进到 T+10
- **THEN** 牛肉掉落 MUST 开始可见并渐显，且 MUST 有一次白色闪光；T+10 之前 MUST 不可见

#### Scenario: 拾取不受呈现滞后影响

- **GIVEN** 一头牛在 T 死亡并掉落牛肉
- **WHEN** 玩家在 T+2 站到掉落格
- **THEN** 拾取 MUST 按权威规则结算，不因客户端滞后渲染而失败

### Requirement: 牛低头必须够到草皮且闲时点头

放牧低头角 MUST 以牛头几何约束：吻部下压后与站立平面距离 MUST ≤0.5 格（常量由呈现测试按牛头包围盒锁定，替代原固定经验角）。非放牧、非死亡的牛 SHALL 以权威 tick 派生的慢速小幅 pitch 点头（纯呈现，drop 旋转同先例，禁用墙钟）；点头 MUST NOT 改变身体朝向、位置或任何权威字段。

#### Scenario: 低头吻部贴草皮

- **GIVEN** 一头放牧置位的牛
- **WHEN** 渲染该牛
- **THEN** 吻部 MUST 落在站立平面上 0.5 格以内

#### Scenario: 闲时点头不碰权威

- **GIVEN** 一头漫游中的牛连续 40 tick 无状态变化
- **WHEN** 逐帧渲染
- **THEN** 牛头 MUST 有肉眼可辨的 pitch 起伏，且其位置/朝向/生命 MUST 与镜像逐字段一致
