## MODIFIED Requirements

### Requirement: 双手与第三人称手臂统一样式

第一人称主手 SHALL 保持体素前臂风格；明暗与材质层号 SHALL 与第三人称手臂同源：颜色为身体基色 `avatarShade(base, 0.82)`、材质为头部层 `+12` 的臂层；同 `PlayerID` 下第一人称手色与第三人称身体基色 MUST 一致。空闲左手 MUST 隐藏；右手 SHALL 从右下方斜向入画，前臂根在画面外，拳面与物品握持处相接，中立位 MUST 不遮挡准星。

#### Scenario: 手色与身体基色一致

- **GIVEN** 同一 `PlayerID` 的第三人称身体与第一人称双手
- **WHEN** 渲染同一帧
- **THEN** 双手颜色 MUST 等于该身体基色经 `avatarShade(·, 0.82)` 的值，材质层 MUST 为其头部层 `+12`

#### Scenario: 空手仅呈现右手

- **GIVEN** 已确认快捷栏选中为空槽或未注册物品
- **WHEN** 处于游戏相位且会话存活
- **THEN** 右手 MUST 以空手姿态呈现，左手 MUST 隐藏，右手 MUST NOT 出现持物几何

#### Scenario: 主手斜向入画

- **GIVEN** 游戏相位中立持握帧
- **WHEN** 渲染第一人称 viewmodel
- **THEN** 右手长轴 MUST 斜向入画，臂根 MUST 落在右下屏角之外，左下角 MUST 无空闲手臂像素，准星像素 MUST 不被主手覆盖

### Requirement: 右手按选中呈现三形态持物

右手持物形态 SHALL 只由已确认镜像的选中槽决定（本地选择请求未确认时 MUST NOT 切换形态）：空槽/未注册为无持物；完整立方方块为微缩立体模型，顶、底及侧面材质 MUST 与世界对应面一致；工具、食物、材料以及火把、门、床等非完整立方物品 MUST 采用与快捷栏同源的原创图标轮廓及颜色，并具有可见厚度。剑、镐、锄 MUST 通过刃部与握柄的不同轮廓可辨，木/石/铁与损坏状态 MUST 保留各自外观。

#### Scenario: 确认才切换持物形态

- **GIVEN** 本地已切槽但服务端确认未到达
- **WHEN** 编码本帧 viewmodel
- **THEN** 持物形态 MUST 保持旧确认值，确认到达后下一帧 MUST 切换

#### Scenario: 方块与工具形态可辨

- **GIVEN** 选中分别为可放置方块与剑
- **WHEN** 渲染同一视角
- **THEN** 方块 MUST 为立方体、工具 MUST 保留剑刃与握柄轮廓，两者剪影 MUST 可辨

#### Scenario: 未注册物品不断言崩溃

- **GIVEN** 选中槽物品未注册
- **WHEN** 编码本帧 viewmodel
- **THEN** MUST 按无持物呈现且不 panic、不改变其他 pass

#### Scenario: 工具类别与损坏状态可辨

- **GIVEN** 依次选中所有已注册剑、镐、锄及损坏形态
- **WHEN** 同一视角渲染持物
- **THEN** MUST 保留各自图标的轮廓和材质差异，握柄 MUST 接入右手，刃部 MUST 不被拳面遮住

#### Scenario: 握持随挥动保持连接

- **GIVEN** 已确认手持方块或工具
- **WHEN** 从中立经历完整挖掘或攻击挥动再回中立
- **THEN** 手与物品 MUST 保持固定局部握持关系，不出现漂浮或脱手；典型 16:9、4:3 画幅与默认 FOV 下识别主体 MUST 可见，准星附近 MUST 保持清楚

#### Scenario: 材质更新反映到持物

- **GIVEN** 客户端加载新的有效材质覆盖
- **WHEN** 下一次呈现相应物品
- **THEN** 手持轮廓及颜色 MUST 与当前快捷栏素材一致，不沿用过期缓存

### Requirement: 相位确定可重放且热路径有界

挥动相位 SHALL 是 `(权威 tick, 手持档, 触发沿)` 的纯函数，不读墙钟、帧间隔与本地随机数；手部位姿的根变换由本帧相机位姿（呈现输入）派生，相机空间偏移经该根变换烘焙为世界变换后由既有世界投影绘制。同相机位姿 + 同输入序列 MUST 逐帧相同。单帧 viewmodel 实例恒 ≤257（主手与最多 256 个有厚度像素部件），编码复用调用方缓冲零分配；无 viewmodel 输入的帧 MUST 与本 change 前逐字节一致。

#### Scenario: 同 tick 序列重放一致

- **GIVEN** 相同的相机位姿、相同的权威 tick 序列与相同的选中/overlay/marker 输入
- **WHEN** 两次编码 viewmodel 帧
- **THEN** 输出字节 MUST 逐字节一致

#### Scenario: 会话重置不清零污染

- **GIVEN** 断线重连或场景切换导致 tick 回退
- **WHEN** 编码新会话首帧
- **THEN** 相位累积 MUST 重新锚定，旧会话的挥动 MUST NOT 延续

#### Scenario: 超出预算明确失败

- **GIVEN** Rust 接收到超过 viewmodel 固定实例预算的输入
- **WHEN** 校验渲染载荷
- **THEN** MUST 按既有错误语义明确失败，不得静默截断；所有已注册物品的合法编码 MUST 在预算内
