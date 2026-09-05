## ADDED Requirements

### Requirement: GIF 动态基线覆盖牛行为剧本

系统 SHALL 为牛行为剧本提供 GIF 动态基线：吃草前后、持麦靠近、击杀与牛肉掉落按 tick 步进抓帧（禁用墙钟），以标准库 `image/gif` 编码并存入 `testdata/` 下 `.gif` 基线。单基线帧预算 MUST 有界（建议 ≤8fps×6s=48 帧，参照录制上限与 manifest 纪律）。比对时解码逐帧并沿用双阈值（最大通道差与差异像素占比）逐帧裁决；全部帧通过方为通过。只允许新增基线，既有 PNG 基线 MUST 逐字节不动。

#### Scenario: 剧本 GIF 可复现生成

- **GIVEN** 同一份代码与同一台机器
- **WHEN** 连续两次生成同一剧本 GIF 基线
- **THEN** 两次解码后的逐帧差异 MUST 落在既定双阈值以内

#### Scenario: 击杀剧本覆盖死亡与掉落

- **GIVEN** 击杀剧本的 GIF 基线
- **WHEN** 逐帧解码审查
- **THEN** 序列 MUST 包含红闪侧倒的死亡过渡帧与牛肉掉落小方块帧

#### Scenario: 超帧预算被拒绝

- **GIVEN** 一次请求超过帧预算上限的 GIF 录制
- **WHEN** 系统校验参数
- **THEN** 系统 MUST 在任何帧捕获之前拒绝该请求

#### Scenario: 旧 PNG 基线不受影响

- **GIVEN** 新增的 GIF 基线已入库
- **WHEN** 运行既有 PNG 视觉比对
- **THEN** 全部既有 PNG 基线的字节 MUST 与入库前一致，且比对 MUST 继续使用既有双阈值

### Requirement: GIF 编码使用自适应调色板

GIF 基线编码 MUST NOT 使用固定 256 色调色板（如 Plan9，绿/棕损伤肉眼可见）：编码器 SHALL 按基线逐个构建自适应调色板（直方图取色 + 确定性并列决胜 + 抖动），标准库内实现，不引入新依赖。同一输入 MUST 逐字节确定输出；比对仍解码逐帧沿用双阈值。

#### Scenario: 草地牛肉颜色保真

- **GIVEN** 同一帧 raw 像素
- **WHEN** 分别经固定调色板与自适应调色板编码再解码
- **THEN** 自适应版本的草绿/牛肉红棕与 raw 的通道差 MUST 显著小于固定版本，且输出 MUST 确定可复现

### Requirement: GIF 剧本呈现完整过程语义

lure 剧本 MUST 呈现牛跟随：牛逐帧向持麦玩家移动并止步、朝向玩家（跟随逻辑由 sim 单测兜底，GIF 只验呈现）。graze 剧本 MUST 呈现草→泥土切换：前段牛低头、结算帧草方块变为泥土（Step 内经既有夹具写块路径切换，只允许切换触发格一格）。kill 剧本 MUST 呈现先倒后肉：掉落 upsert 时机保持权威诚实（死亡当 tick），呈现滞后由客户端关联逻辑承载（见死亡呈现）。

#### Scenario: lure 牛位移跟随

- **GIVEN** lure 剧本的 GIF 基线
- **WHEN** 逐帧解码审查首末帧牛位
- **THEN** 牛 MUST 向玩家方向位移且末帧距玩家约止步距离，玩家 MUST 在帧内可见

#### Scenario: graze 草变泥土同镜呈现

- **GIVEN** graze 剧本的 GIF 基线
- **WHEN** 逐帧解码审查触发格
- **THEN** 前段 MUST 为草地 + 低头牛，结算帧起 MUST 为泥土格
