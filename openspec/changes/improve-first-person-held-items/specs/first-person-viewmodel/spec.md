## MODIFIED Requirements

### Requirement: 双手与第三人称手臂统一样式

第一人称主手 SHALL 保持与第三人称身份同源的体素袖口/前臂，并具有独立可辨的肤色拳面和握持轮廓。空闲左手 MUST 隐藏；右手 SHALL 从最右下边缘自然斜向入画，前臂根在画面外，可见前臂 MUST NOT 呈横贯右侧的长筒；拳掌与物品握持处相接，中立位 MUST 不遮挡准星。

#### Scenario: 袖口与拳面可辨
- **GIVEN** 相同 PlayerID 的第一人称主手
- **WHEN** 呈现空手与持工具
- **THEN** 袖口 MUST 保留该身份原有衣着外观，拳面 MUST 独立于袖口且呈自然肤色，工具柄 MUST 接入抓握处

#### Scenario: 空手仅呈现右手

- **GIVEN** 已确认快捷栏选中为空槽或未注册物品
- **WHEN** 处于游戏相位且会话存活
- **THEN** 右手 MUST 以空手姿态呈现，左手 MUST 隐藏，右手 MUST NOT 出现持物几何

#### Scenario: 主手斜向入画

- **GIVEN** 游戏相位中立持握帧
- **WHEN** 渲染第一人称 viewmodel
- **THEN** 右手长轴 MUST 斜向入画，臂根 MUST 落在右下屏角之外，左下角 MUST 无空闲手臂像素，准星像素 MUST 不被主手覆盖

### Requirement: 右手按选中呈现三形态持物

右手持物形态 SHALL 只由已确认镜像的选中槽决定（本地选择请求未确认时 MUST NOT 切换形态）：空槽/未注册为无持物；完整立方方块为微缩立体模型，顶、底及侧面材质 MUST 与世界对应面一致；工具 MUST 采用有独立柄、刃和连接部位的立体体素结构，并与快捷栏原创素材保持材料颜色和类别一致；食物、材料以及火把、门、床等非完整立方物品 MUST 采用与快捷栏同源的原创图标轮廓及颜色，并具有可见厚度。剑、镐、锄 MUST 通过刃部与握柄的不同轮廓可辨，木/石/铁与损坏状态 MUST 保留各自外观。

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
- **THEN** MUST 保留各自类别的结构和材质差异，握柄 MUST 接入右手，刃部 MUST 不被拳面遮住

#### Scenario: 握持随挥动保持连接

- **GIVEN** 已确认手持方块或工具
- **WHEN** 从中立经历完整挖掘或攻击挥动再回中立
- **THEN** 手与物品 MUST 保持固定局部握持关系，不出现漂浮或脱手；典型 16:9、4:3 画幅与默认 FOV 下识别主体 MUST 可见，准星附近 MUST 保持清楚

#### Scenario: 材质更新反映到持物

- **GIVEN** 客户端加载新的有效材质覆盖
- **WHEN** 下一次呈现相应物品
- **THEN** 手持颜色 MUST 与当前快捷栏素材一致，非工具轮廓 MUST 与当前素材一致，不沿用过期缓存

### Requirement: 相位确定可重放且热路径有界

挥动相位 SHALL 由显式有效本地点击/持键、单调呈现 elapsed 与手持档驱动，不读墙钟或本地随机数；手部位姿的根变换由本帧相机位姿（呈现输入）派生，相机空间偏移经该根变换烘焙为世界变换后由既有世界投影绘制。同相机位姿 + 同输入序列 MUST 逐帧相同。单帧 viewmodel 实例恒 ≤257（主手及所有持物部件的联合硬上限），编码复用调用方缓冲零分配；无 viewmodel 输入的帧 MUST 与本 change 前逐字节一致。

#### Scenario: 同 tick 序列重放一致

- **GIVEN** 相同的相机位姿、相同的权威 tick 和呈现 elapsed 序列与相同的选中/本地点击/overlay/marker 输入
- **WHEN** 两次编码 viewmodel 帧
- **THEN** 输出字节 MUST 逐字节一致

#### Scenario: 会话重置不清零污染

- **GIVEN** 断线重连、场景切换或权威玩家状态 reset
- **WHEN** 编码新会话首帧
- **THEN** 本地呈现状态 MUST 清除，旧会话的挥动 MUST NOT 延续

#### Scenario: 超出预算明确失败

- **GIVEN** Rust 接收到超过 viewmodel 固定实例预算的输入
- **WHEN** 校验渲染载荷
- **THEN** MUST 按既有错误语义明确失败，不得静默截断；所有已注册物品的合法编码 MUST 在预算内

### Requirement: 挖掘时右手挥动

有效游戏主键持续按住期间，右手 SHALL 根据显式呈现 elapsed 连续挥动，不依赖有效目标或裂纹；松开后 MUST 完成当前动作并回中立。非游戏或界面遮挡期间 MUST 隐藏 viewmodel 并抑制新动作。

#### Scenario: 持键与松开
- **GIVEN** 游戏中有效主键按住，包含无目标空挥
- **WHEN** 连续推进呈现 elapsed 后松开
- **THEN** MUST 按工具档位重复动作，松开后完成当前动作并回中立，裂纹变化 MUST NOT 重启动作

### Requirement: 攻击命中时右手挥动

攻击挥动 SHALL 来自有效本地输入；`network.CombatHit` 的 marker/audio 确认规则保持原有权威语义，但确认 MUST NOT 触发、重播或延长主手动作。

#### Scenario: 迟到或重复命中不重播
- **GIVEN** 本地点击动作正在播放或已经完成
- **WHEN** 新的、陈旧的或重复 CombatHit 确认到达
- **THEN** 主手相位 MUST 保持仅由本地输入与 elapsed 决定，确认 MUST NOT 重启动作

## ADDED Requirements

### Requirement: 点击立即挥动且避让完整 HUD

有效游戏主键点击 SHALL 立即触发可见的前伸、下挥及回收动作，空手、工具与方块均适用；空挥 MUST 不等待服务端命中或采掘确认。动作 MUST 不修改权威伤害、采掘或物品状态，迟到的命中确认 MUST NOT 重复播放本地动作。持续按键 SHALL 以有界周期连续挥动。菜单、背包、聊天输入及首次鼠标捕获点击 MUST NOT 触发动作，会话重置 MUST 清除动作。

#### Scenario: 空挥与帧率无关
- **GIVEN** 游戏相位中空手或手持工具且无可命中目标
- **WHEN** 有效点击并按相同 elapsed 推进到相同动作时间
- **THEN** 动作 MUST 立即开始，完整轨迹含前伸、下挥与回收，不同渲染帧率下时长一致

#### Scenario: 完整 HUD 与主手共存
- **GIVEN** 640×360、1280×720 或典型4:3逻辑窗口且完整 HUD 可见
- **WHEN** 空手或手持工具/方块处于中立及挥动全过程
- **THEN** 主手构图 MUST 根据窗口与 FOV 适配，避开热栏及状态图标（含护甲与氧气行），识别部位 MUST 可见，握点 MUST 保持连接，前臂根 MUST 在画面外
