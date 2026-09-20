# B-42 潜行与双击疾跑 — 设计文档

- 日期：2026-09-11
- 分类：architectural（跨协议/物理/客户端输入/服务端结算，多包协同，需 OpenSpec change）
- 范围裁决（用户已确认四节）：全量一次交付；`Shift` 按住 = 潜行，疾跑改双击 `W` 触发，`Ctrl` 退役；协议 `v40→v41`，Step header 布局 `v3→v4`，其余矩阵不变。

## 1. 背景与目标

### 1.1 现状基线（已核实）

- 移动输入链：`Window.KeyDown`（Rust 快照位，`ShiftLeft=5`/`ControlLeft=6`/`AltLeft=27` 已传输，`input.rs:key_bit`）→ `interactive.go` 组装 `Movement`/`Actions` → `applyInteractiveInput` 构造 `client.Control`（`Sprinting` 由 `Ctrl||Shift` 按住直接置位）→ `predictor.Advance` → `network.PlayerInput`（`Sprinting` 在 `Eating` 之后，协议 v28 追加）→ `session_ingress.translateClientMessage` → `contract.Command` → `entity.tick` → `physics.StepWithTunables`。
- 疾跑门控双层：sim 侧饥饿 `<6` 清零 `Sprinting`（`entity/player.go:536`），physics 侧 `Sprinting && MoveZ>0 && OnGround && !BodyInFluid` 时 `WalkSpeed*1.3`（`step.go:83`），疲劳按实际加速 tick 计费（`player.go:582`）。
- Step header v3（160B）：`bytes[129]=Sprinting`，`bytes[148:152]=SprintMultiplier`，`128` 为浸没标志，`130..132`、`152..160` 保留零，Rust 逐字节校验保留区（`step.go:188`）。
- 放置/交互分流现状：客户端 `placeBlock()`（`app_input.go:26`）按本地镜像先判开容器（`Furnace/Chest/Workbench` → `OpenContainer`）、锄地（`TillSoil`）、骨粉（`BoneMeal`），最后才发 `PlaceBlock`；服务端 `openContainer`/`executePlacement` 各自做权威射线，互不感知对方。门（`CommandInteractDoor`）与床（`CommandInteractBed`）只有 `contract` 命令与服务端结算，**无客户端上行消息映射**（`session_ingress` 无对应分支，客户端零发送点）——本行不补 wiring，只预留潜行分流位。
- 水桶（`CollectWater`/`PlaceWater`）同样只有服务端映射与测试，客户端无发送点；本行不碰。

### 1.2 目标

- `Shift` 按住 = 潜行意图：慢速移动（默认 `0.3x`）、站立边缘防坠落、对交互方块潜行放置（压交互、直发放置）。
- 疾跑改双击 `W` 触发并锁存：第二次 `W` 上升沿距上次 `W` 释放 `≤300ms` 且 `W` 保持按住即置位 `Sprinting`，直到松开 `W`、按下 `Shift`、打开界面/聊天/暂停时清零；饥饿/地面/浸没仍由服务端权威门控。
- `Ctrl` 不再触发疾跑，直接退役；`Alt` 保持空闲；键位重绑留给 D-15。
- 互斥裁决（潜行优先）：`Sneaking` 置位时疾跑加速与疾跑疲劳一律跳过；潜行减速只在站立 + 非浸没时生效（空中/水中不减速，但 `Sneaking` 位仍保留给放置逻辑）。

### 1.3 非目标（延期与放弃）

- 门/床交互的客户端 wiring（本行只在服务端留潜行分流位，不新增网络消息）。
- 锄地/骨粉/水桶在潜行下的行为变更（保持原样，不压制）。
- 潜行视线高度降低与姿态呈现（留 D-09 第三人称/姿态）。
- 潜行耐力/疲劳新增行、FOV/HUD/音效、设置页键位重绑（D-15）。
- 新 capture 场景；既有 30 景零漂为门禁。

## 2. 输入与键位（第 1 节，已批准）

- `Sneaking = ShiftLeft` 按住（`window.KeyDown(client.KeyLeftShift)`），随每 tick 输入上行，是**持续位**（同 `Eating` 同形）。
- `Sprinting` 改由双击 `W` 状态机推导（客户端纯本地，`InputState` 或交互层持有）：
  - 记忆上次 `W` 释放时刻（`armedAt`）；`W` 上升沿到达且距 `armedAt` `≤300ms` → 锁存 `sprintingLatched=true`；
  - 第一次释放只负责记录 `armedAt`（此时锁存尚未置位，不存在“清零”问题）；**锁存置位后的** `W` 松开、或 `Shift` 按下、或界面（背包/容器/聊天/暂停/面板）打开、或光标未捕获时 → 清零锁存，并同时清除 `armedAt`（防关菜单后陈旧窗口误触发）；
  - 上行 `Sprinting = latched && W仍按住 && allowActions`；服务端门控不变，误触发由权威纠正 + 预测重放收敛。
  - 时钟源必须可注入（测试用假时钟/ tick 计数），窗口常量 `300ms` 集中一处，注释写明手感取舍。
- `CtrlLeft` 不再参与 `Sprinting` 推导（删除或分支，测试同步改）；`AltLeft` 不用。
- 现有直接置 `Sprinting` 位的单测（sim/physics/codec）不受影响——改的只是客户端推导，不改 wire 语义。

## 3. 协议与物理（第 2 节，已批准）

- 协议 `v40→v41`：`PlayerInput` 尾部追加 `Sneaking bool`（紧跟 `Sprinting` 之后），纯追加；`packet.go` 版本史注释、`ProtocolVersion` 常量、`TestProtocolVersionPinned`、codec 双向编解码 + golden 尾字节夹具 + fuzz 语料、登录版本拒绝、`Validate`（bool 无值域校验，只查非有限旋转既有项）、Memory/TCP parity 同步。
- `contract.Command` 追加 `Sneaking`；`translateClientMessage` 同形搬运；`tick.go` 的 `CommandPlayerInput` 分支写入 `player.input.Sneaking`（并更新 `sneakingHeld` 锁存，见 §4），`CommandPlaceBlock` 等分支不受影响；`validPlayerInput` 不变。
- `physics.Input` 追加 `Sneaking`；`Tunables` 追加 `SneakSpeedMultiplier`（默认 `0.3`，`≈1.29m/s`），`DefaultTunables`/`SetTunables`/config tag 同步（tag 名与 `config.Fields()` 对齐）。
- Step header 布局 `v3→v4`（总长保持 160B，不升 engine ABI，按 B-30 先例只升布局版本）：
  - `bytes[130]=Sneaking`（现保留区首字节）；
  - `bytes[152:156]=SneakSpeedMultiplier`（`152..160` 现 8 字节保留区用前 4 字节，`156..160` 继续置零）；
  - Rust `step.rs` 同步解码 + 保留区校验收紧（仅 sprint/sneak 区可非零）+ 积分：潜行有效时目标速度 `WalkSpeed*Sneak`，且**潜行优先于疾跑**（`if sneaking_effective {...} else if sprinting_gated {...}`）。
- 门控（Go 与 Rust 逐位一致，两层复核保证 sweep bounds 自检一致）：
  - 潜行减速有效 `= Sneaking && OnGround && !BodyInFluid`（`MoveZ/X` 不限，含静止/后退；空中与水中不减速）；
  - 疾跑加速有效 `= Sprinting && !Sneaking有效 && MoveZ>0 && OnGround && !BodyInFluid && Hunger>=6`（sim 侧先按饥饿与潜行清零，再进 physics）；
  - 疾跑疲劳只在疾跑实际加速 tick 计费（既有判据追加 `!sneakingEffective`）；潜行本身不新增疲劳行。
- `client.Control` 与 `predictor` 同形追加 `Sneaking`；`MirrorCollisionSource` 不变。

## 4. 边缘保护与潜行放置（第 3 节，已批准）

### 4.1 边缘保护（权威 + 预测同函数）

- 新纯函数（如 `physics.ClampSneakEdge(state, displacement, source)` 或等价布尔），Go 单一真源，sim 与客户端预测同调：
  - 生效条件：`OnGround && sneakingEffective && !BodyInFluid && 水平位移非零 && !Jump`（跳跃可主动跳下；击退/水推走非输入路径，不拦截）；
  - 判据：目标脚底支撑消失（复用既有实心碰撞谓词，如 `BlockCollisionBoxes` 非空集，查目标脚底及足印边缘数格，有界常量），则把水平位移钳制到边内（或取消水平分量，保留垂直积分）；
  - 有界性：每步只读常数块（足印下 ≤4 格 + 边缘探针），不扫描区块；未加载格按有支撑处理（不误钳制，`HitUnknown` 路径已有语义）。
- Rust 侧不实现钳制（保持积分纯粹）：Go 在 `StepWithTunables` 前（sweep bounds 与编码用钳制后目标速度）或后（对输出位移钳制）二选一，design 要求选定一种并保证 sweep 自检一致（推荐**输入侧钳制目标速度**，sweep/Rust/输出天然一致）。

### 4.2 潜行放置（客户端预判 + 服务端权威）

- 客户端 `placeBlock()`：若本帧 `Sneaking` 置位，跳过开容器分支（`OpenContainer`），直接走到底的 `PlaceBlock` 发送（锄地/骨粉分支保留，见非目标）；门/床未来分支同样跳过（现无分支，只留注释）。
- 服务端权威：`player` 新增 `sneakingHeld`（每 `CommandPlayerInput` 更新，与 `miningHeld`/`eatingHeld` 同形）；`openContainer` 在权威射线命中后、建立查看关系前，若 `sneakingHeld` 为真则拒绝（复用 `RejectInvalidInput`，不新增 wire 值）；`executeInteractDoor`/`executeInteractBed` 同位置预留同样分流（现无上行触发，代码先落位、测试覆盖 handler 层）。
- `PlaceBlock` 本来就不自动交互，无需改；潜行中的 `PlaceBlock` 走正常放置校验（含容器槽位预留、耕地/树苗/火把前置）。

## 5. 版本、测试与落地（第 4 节，已批准）

- 版本矩阵：协议 `v40→v41`、`stepLayoutVersion 3→4`；engine ABI v11、client ABI v18、玩家 schema v8、区块 v9、metadata v5、companions v5、hostile v1、passive v1、benchmark scenario v23 **全部不变**。
- 测试矩阵（行为先行，红绿重构）：
  - 网络：codec 尾字节 golden（`Sneaking` 真/假）、旧版登录拒绝、fuzz 尾随字节拒绝、Memory/TCP parity；
  - 物理：潜行减速 golden 向量、潜行优先 parity（Go/Rust 双出口对照）、sweep bounds 自检；
  - sim：慢速门控（地面/空中/水中三态）、潜行压疾跑（加速 + 疲劳双断言）、饥饿 `<6` 与潜行正交；
  - 边缘：钳制单测（平地直行悬崖边必停、跳跃可下、移动零位移不钳制）+ sim/预测同函数回归；
  - 客户端：双击 `W` 状态机（假时钟：窗口内/外、松 `W` 清零、按 `Shift` 清零、界面打开清零）、`Ctrl` 退役回归、`placeBlock` 潜行分支（容器前放块、锄地骨粉不变）；
  - 服务端：`sneakingHeld` 锁存 + 潜行中 `OpenContainer` 拒绝 + parity；
  - 全量：受影响包 `-race`、六模块 `go vet`（或 `make dev-check` 入口）、`openspec validate --all --strict`、`visual-check` 30 景零漂。
- OpenSpec 落地：新建 change（如 `sneak-double-tap-sprint`），delta 含新建 `sneak` 能力 + `sprint` MODIFIED（触发方式）+ 容器/门床交互 MODIFIED（潜行分流）；`tasks.md` 收尾含 gofmt、全量 race、vet 与严格校验；B-42 行内认领并声明独占文件集后开工（串行链外独立行，不占 A 列车版本槽；协议 v41 槽需确认无在途互斥持有——现无）。
- 风险与回退：双击误触发（窗口 300ms 可调，服务端门控兜底）；`Ctrl` 肌肉记忆（更新操作说明，F-04 的 lan-server 文档不在本行范围）；header 混装（布局版本校验 fail-fast）；回退 = 整 change revert（纯追加，无迁移）。

## 6. 被否决的替代方案

- **Alt = 潜行、保留 Ctrl/Shift 双键疾跑**：改动最小且 Rust 零改，但用户明确选择 `Shift` 潜行 + 双击疾跑的手感路线；且双键疾跑与潜行同键（`Shift`）语义纠缠，不解决根本冲突。否决。
- **只做慢移 + 放置、砍边缘保护**：工作量减半，但用户明确全量交付；边缘保护是潜行玩法的核心承诺（高空搭路不坠落），砍掉等于交付半个机制。否决。
- **潜行压制锄地/骨粉/水桶**：与 MC 手感不符（潜行不阻止锄地），且会破坏既有 farming/bucket 循环测试；保持原行为。否决。
- **潜行降低视线高度**：需动相机/射线原点/呈现三方，归 D-09 姿态呈现；本行不做。否决。
