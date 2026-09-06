# first-person-viewmodel Specification

## Purpose
第一人称视角下在屏幕左右两侧呈现与第三人称手臂统一样式的双手：右手按已确认快捷栏选中呈现持物并随挖掘与攻击挥动，左手本闭环为空手占位并保留副手扩展位；全部相位由权威 tick 派生、可重放，不读墙钟，不新增服务端语义。
## Requirements
### Requirement: 双手与第三人称手臂统一样式

双手 cuboid 的尺寸、明暗与材质层号 SHALL 与第三人称手臂同源：臂尺寸 `0.1×0.7×0.25`、颜色为身体基色 `avatarShade(base, 0.82)`、材质为头部层 `+12` 的臂层；同 `PlayerID` 下第一人称手色与第三人称身体基色 MUST 一致。双手 SHALL 自屏幕左下角与右下角斜向入画：手臂长轴向画面中心倾斜（非竖直柱状），右手为主手（更靠画面中心、持物位更高）；中立持握下双手 MUST 分居左右半屏且不遮挡准星。

#### Scenario: 手色与身体基色一致

- **GIVEN** 同一 `PlayerID` 的第三人称身体与第一人称双手
- **WHEN** 渲染同一帧
- **THEN** 双手颜色 MUST 等于该身体基色经 `avatarShade(·, 0.82)` 的值，材质层 MUST 为其头部层 `+12`

#### Scenario: 空手仍呈现双手

- **GIVEN** 已确认快捷栏选中为空槽或未注册物品
- **WHEN** 处于游戏相位且会话存活
- **THEN** 左右手 MUST 以空手姿态呈现，右手 MUST NOT 出现持物几何

#### Scenario: 双手斜向入画而非竖直柱状

- **GIVEN** 游戏相位中立持握帧
- **WHEN** 渲染第一人称 viewmodel
- **THEN** 左右手长轴 MUST 向画面中心倾斜（左手顶端偏右、右手顶端偏左），臂根 MUST 落在左下/右下屏角之外，准星像素 MUST 不被双手覆盖

### Requirement: 双手随 HUD 隐藏（背包与菜单打开时无手臂）

双手 SHALL 与 HUD 常显层一体：背包/容器界面打开或非游戏菜单相位时 MUST NOT 呈现 viewmodel（与血条、饥饿、快捷栏同隐同现）；回到游戏相位且界面关闭后下一帧 MUST 恢复。静态 capture 场景表 MUST NOT 含手臂像素，GIF 动作剧本不受此限。

#### Scenario: 开背包隐藏双手

- **GIVEN** 游戏相位中已确认选中非空且双手正在呈现
- **WHEN** 打开背包（或切到暂停/菜单相位）
- **THEN** 当帧起 viewmodel MUST 为空，关闭背包回到游戏相位后 MUST 恢复呈现

#### Scenario: 静态场景表无手臂像素

- **GIVEN** 静态 capture 任一场景（含已确认背包的战斗/采掘景）
- **WHEN** 抓帧比对
- **THEN** 画面 MUST NOT 含 viewmodel 像素（与本 change 前 golden 逐字节一致）

### Requirement: 右手按选中呈现三形态持物

右手持物形态 SHALL 只由已确认镜像的选中槽决定（本地选择请求未确认时 MUST NOT 切换形态）：空槽/未注册为无持物；`core.ItemPlacement` 命中的为手持方块（微缩立方，顶面/侧面材质与世界一致）；其余为手持物品（扁长条程序化几何，工具与食物同形，形状不依赖 HUD sprite）。

#### Scenario: 确认才切换持物形态

- **GIVEN** 本地已切槽但服务端确认未到达
- **WHEN** 编码本帧 viewmodel
- **THEN** 持物形态 MUST 保持旧确认值，确认到达后下一帧 MUST 切换

#### Scenario: 方块与工具形态可辨

- **GIVEN** 选中分别为可放置方块与剑
- **WHEN** 渲染同一视角
- **THEN** 方块 MUST 为立方体、工具体 MUST 为扁长条，两者剪影 MUST 可辨

#### Scenario: 未注册物品不断言崩溃

- **GIVEN** 选中槽物品未注册
- **WHEN** 编码本帧 viewmodel
- **THEN** MUST 按无持物呈现且不 panic、不改变其他 pass

### Requirement: 挖掘时右手挥动

`miningOverlay` 为 active（含有效目标且裂纹阶段合法）期间，右手 SHALL 以挥动相位呈现挖掘动作，摆幅按手持档位取参数表；overlay 不可见（选框丢失、断线、暂停/菜单相位）时 MUST 回中立持握。

#### Scenario: 挖掘进度驱动挥动启停

- **GIVEN** 权威采掘 active 且裂纹阶段合法
- **WHEN** 连续渲染多帧
- **THEN** 右手挥动相位 MUST 随权威 tick 推进，overlay 清除后 MUST 回中立位

#### Scenario: 非游戏相位不挥动

- **GIVEN** 暂停或菜单相位且采掘镜像残留 active
- **WHEN** 渲染全景或暂停帧
- **THEN** viewmodel MUST 不呈现挥动（全景相位无 viewmodel，暂停帧为中立持握）

### Requirement: 攻击命中时右手挥动

`network.CombatHit` 严格递增确认后的 marker 窗内（既有 6 帧语义），右手 SHALL 完成一次攻击挥动；marker 到期后 MUST 回中立持握；陈旧或重复 `CombatHit` MUST NOT 触发挥动。

#### Scenario: 命中确认触发一次挥动

- **GIVEN** 新的 `CombatHit` 确认到达
- **WHEN** 渲染随后 6 帧
- **THEN** 第 1 帧 MUST 起挥，第 6 帧后 MUST 回中立，同确认 MUST NOT 二次触发

### Requirement: 工具六档摆幅与节奏

挥动摆幅/节奏 SHALL 按手持查参数表：空手、方块、剑、镐、铲、斧六档；斧铲在配方与采掘规则落地前取镐档默认值，落地后只改表值。参数表变更 MUST NOT 改变编码布局与 ABI。

#### Scenario: 剑与方块摆幅不同

- **GIVEN** 相同挥动相位下分别手持剑与方块
- **WHEN** 编码 viewmodel 实例
- **THEN** 两者旋转角 MUST 不相等且各自落在参数表标定区间内

### Requirement: 相位确定可重放且热路径有界

挥动相位 SHALL 是 `(权威 tick, 手持档, 触发沿)` 的纯函数，不读墙钟、帧间隔与本地随机数；手部位姿的根变换由本帧相机位姿（呈现输入）派生，相机空间偏移经该根变换烘焙为世界变换后由既有世界投影绘制。同相机位姿 + 同输入序列 MUST 逐帧相同。单帧 viewmodel 实例恒 ≤4（左手、右手、持物、保留一位），编码复用调用方缓冲零分配；无 viewmodel 输入的帧 MUST 与本 change 前逐字节一致。

#### Scenario: 同 tick 序列重放一致

- **GIVEN** 相同的相机位姿、相同的权威 tick 序列与相同的选中/overlay/marker 输入
- **WHEN** 两次编码 viewmodel 帧
- **THEN** 输出字节 MUST 逐字节一致

#### Scenario: 会话重置不清零污染

- **GIVEN** 断线重连或场景切换导致 tick 回退
- **WHEN** 编码新会话首帧
- **THEN** 相位累积 MUST 重新锚定，旧会话的挥动 MUST NOT 延续

