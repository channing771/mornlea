## MODIFIED Requirements

### Requirement: 进食是持续输入驱动的权威动作

进食 SHALL 由持续输入驱动:玩家手持食物并保持进食输入时,服务端 MUST 逐 tick 推进进度;进度达到固定 tick 数时 MUST 在单一 tick 内原子结算——扣除一件食物、按该食物的固定值增加饥饿值(不超过 `20`)与饱和度(不超过饥饿值)。进食输入中断、选中栏位变化、栏位物品变化、受伤、死亡、会话打开容器界面或会话视野尚未就绪 MUST 清零进度且 MUST NOT 扣除食物;中断条件与结算条件在同一 tick 同时成立时,中断 MUST 优先于结算。饥饿值已满时 MUST NOT 推进进度。

#### Scenario: 持续进食到时结算

- **GIVEN** 玩家手持 2 个面包、饥饿值 10、饱和度 0
- **WHEN** 玩家保持进食输入达到固定 tick 数
- **THEN** 面包 MUST 变为 1,饥饿值 MUST 变为 15,饱和度 MUST 变为面包的固定饱和值

#### Scenario: 中途松手不扣料

- **GIVEN** 玩家手持面包并已推进若干 tick
- **WHEN** 玩家停止进食输入
- **THEN** 进度 MUST 清零,面包数量 MUST 不变

#### Scenario: 中途切换栏位不扣料

- **GIVEN** 玩家手持面包并已推进若干 tick
- **WHEN** 玩家切换到另一快捷栏位并保持进食输入
- **THEN** 原栏位的面包数量 MUST 不变,且新栏位 MUST NOT 被扣除

#### Scenario: 打开容器中断进食不扣料

- **GIVEN** 玩家手持面包并已推进若干 tick
- **WHEN** 玩家打开箱子或熔炉的容器界面并保持进食输入
- **THEN** 进度 MUST 清零,面包数量 MUST 不变

#### Scenario: 恰在结算 tick 打开容器不结算

- **GIVEN** 玩家手持面包、进食进度为固定 tick 数减一
- **WHEN** 下一 tick 容器界面处于打开状态且进食输入保持
- **THEN** 面包数量 MUST 不变,饥饿值与饱和度 MUST 不变

#### Scenario: 视野未就绪不推进进食

- **GIVEN** 玩家手持食物并保持进食输入,其会话视野尚未就绪
- **WHEN** 权威 tick 推进
- **THEN** 进食进度 MUST 保持清零,食物数量 MUST 不变

#### Scenario: 饥饿已满不推进

- **GIVEN** 玩家手持面包、饥饿值 20
- **WHEN** 玩家保持进食输入任意 tick
- **THEN** 面包数量 MUST 不变

#### Scenario: 饱和度不超过饥饿值

- **GIVEN** 玩家饥饿值 17、饱和度 0
- **WHEN** 玩家吃下一个面包
- **THEN** 饥饿值 MUST 变为 20,饱和度 MUST 不超过 20 的等价值
