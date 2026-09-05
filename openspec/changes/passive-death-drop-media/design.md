# Design: passive-death-drop-media

## Context

见 `proposal.md` Why。当前真相（本分支已含 `passive-graze-lure` 的 HEAD）：协议 v34，`PassiveState` 尾部已有放牧位（38B），`PassiveDespawn` 仍是纯 ID record（8B）；服务端 `publishPassives` 对死亡与出视野一视同仁（镜像有而快照无即 despawn）；客户端 `Passives.ApplyDespawn` 立删；掉落小方块走纯色 `ItemColor`（`avatarMaterialSolid` 哨兵）；capture 只有 PNG 单帧基线。详见四个 delta spec。

## Goals / Non-Goals

- Goals：despawn 原因位端到端（v35）+ 客户端 20 tick 红闪侧倒（确定性）+ 掉落 atlas 采样（含缺层审计与追加）+ GIF 动态基线四件套。
- Non-Goals：不改存档 schema（原因/死亡态瞬态，不落盘）；不动夜行者；不动 96B 实例布局与 client/engine ABI；不重排 `captureScenes`，旧 PNG 逐字节不动。

## Decisions

### D1：despawn record 尾部 +1 字节原因位，协议 v34→v35

- 选择：`PassiveDespawnRecord` 从 u64 ID 变为 u64 ID + u8 reason（0=消失，1=死亡）；`PassiveDespawnWireBytes` 8→9，上限与拒绝矩阵同步（reason 非 0/1 整包拒绝）；`ProtocolVersion` 34→35，基线矩阵（`AGENTS.md` + `openspec/config.yaml`）同步。
- 理由：与放牧位同形的尾部追加纪律；解码长度推导测试继续锁死形状。
- 否决：复用 ID 符号位/高位偷渡——破坏固定字节推导纪律；发第二种 despawn 消息——包 ID 追加面留给真正的全新消息，不为 1 位原因开新 ID。

### D2：死亡事实由 engine 当 tick 死亡 ID 集合供给发布

- 选择：`settlePassiveDeaths` 在移除同时记录本 tick 死亡 ID（有界 scratch，≤32）；经 runtime 窄委派以 `PassiveDeaths()` 暴露，`publishWithChats` 同 tick 取一次传给 `publishPassives` 做原因投影（死亡集合命中即 1，否则 0）。
- 理由：发布侧只看得到 tick 末快照（死者已移除，health 0 不可见），服务端无法从快照差分区分死亡与出视野；死亡集合与快照同源同 tick，无跨 tick 状态。
- 否决：服务端记全局上一快照差分——会话可见性各异，全局差分无法回答“该会话的这次 despawn 是否死亡”；经 `TickResult` 另起通道——被动快照本就走 Engine 直查，另起通道分裂事实源。

### D3：客户端死亡保留 20 tick，用权威 tick 而不用墙钟推进

- 选择：`Passives` 新增 dying 表（ID→死亡 tick T0 + 冻结位姿）；`ApplyDespawn` 遇原因 1 转存 dying 而非删除，原因 0 立删；镜像记录见过的最大 `ServerTick` 为当前 tick，保留进度 = 当前 tick − T0，满 20 移除；保留期同 ID 新 state 丢弃、新 spawn 按既有稳定规则处理。
- 理由：`Advance(elapsed)` 吃墙钟，死亡动画必须由权威 tick 派生才可重放；最大 tick 单调前推进度天然有界。
- 否决：墙钟计时器——与掉落动画的确定性先例相悖，重放与 CI 必漂。

### D4：红闪侧倒复用既有 96B 实例通道，不动 ABI

- 选择：呈现侧新增死亡相位函数（`dropAnimationPhase` 同形：T0 + ID 混合派生，红闪插值系数与 roll 角度皆为其纯函数）；roll 合成进牛实例的 mat4，红闪调制颜色通道（贴图路径颜色为白，正好做 tint 系数）；`Avatar`/呈现值只加 Go 侧字段，零值时 transform 与变更前逐字节一致。
- 理由：位姿/颜色全走既有通道，ABI、容量、实例字节全不动；俯仰通道继续归放牧位，roll 与 pitch 正交不打架。
- 否决：新增实例字段/新 pass——为 20 tick 装饰改 ABI 不值；复用放牧 pitch 表达侧倒——俯仰轴（Z）与侧倒滚转语义混淆，且死亡与吃草可叠加时无法区分。

### D5：掉落小方块单层采样，缺层只在末位追加

- 选择：`buildItemDropParts` 由纯色改为层采样：可放置物品取注册表既有材质（代表面取顶面层，与世界同源）；食物/非放置物品先审计映射到牛肉/小麦等已有层；审计先行产出“可掉落物品 × 已有层”缺口清单，缺的层才在枚举末位追加（植物 31..54、火把 59、床 60..67、短草 68、裂纹 69..78、牛/牛肉层一律不动），缺层像素为原创程序化 + `PROVENANCE.json`/`ATTRIBUTION.md` 溯源（短草/裂纹/牛批次先例）。
- 理由：实例只带一个 u32 material，单层是格式上限内的唯一选择；顶面代表层对方块类足够可辨（草顶 vs 泥土可分）。
- 否决：六面各异材质——实例格式装不下；引入外部 PNG/Mojang 提取物——资源边界红线，无法确权即停下报 BLOCKED。

### D6：GIF 基线是独立于 PNG 场景表的 tick 步进 runner

- 选择：capture 包新增 GIF 剧本 runner（非 `captureScenes` 行，不触碰 24 景顺序与 PNG）：剧本按固定 tick 步进 `app.Frame`（`physics.FixedDelta`，禁用墙钟），每步 `Readback` 一帧，标准库 `image/gif` 编码，基线存新目录 `testdata/visual-golden/passive-death/` 下 `.gif`，比对解码逐帧复用 `compareImages` + 既有双阈值；帧预算 ≤48（8fps×6s），参数校验在首帧捕获前。
- 理由：PNG 表动一行动全身（顺序测试 + golden 张数 + README 索引），GIF 另起目录零波及旧基线；tick 步进 + tick 钉死（牧场 PinVolatile 先例）消除机器速度漂移。
- 否决：把 GIF 塞进既有场景表——每个 GIF 帧都是新基线资产，场景表只承载单帧 PNG；用 devcapture 的 HTTP 录制——那是交互窗口的调试路径，不是确定性无头基线。

## Risks / Trade-offs

- [与 graze-lure 的串行] → 本分支已基于其 HEAD；收尾 `git fetch origin && git rebase origin/feat/passive-graze-lure`，冲突即停报 BLOCKED；归档顺序 graze-lure 在先。
- [缺层数量超预期] → 审计任务先产清单再定层数；每层配像素唯一性 + provenance 测试，工作量线性可估。
- [GIF 漂移] → tick 步进 + 权威 tick 钉死 + 逐帧双阈值； budgets 小（≤48 帧小图），仓库膨胀可控。
- [死亡保留与容量] → dying 表与镜像共用 32 上限语义，保留期新 spawn 按稳定规则忽略，不驱逐活体。

## Migration Plan

- 部署：协议 v34→v35 只追加；旧客户端握手拒绝（既有语义）；存档不动，老世界直接兼容（原因/死亡态瞬态）。
- 回滚：整分支 revert；残留原因位 despawn 被旧版按版本拒绝，无脏数据；GIF 基线随分支走，不污染 PNG。

## Open Questions

- 无。抽选外全部常量（20 tick、红闪系数、roll 90°、48 帧预算）在任务内锁定，评审可调。

## Polish 附录（用户验收后打磨，与主体同分支交付）

### P1：GIF 自适应调色板

- 选择：逐基线直方图取色（Top-N + 确定性并列决胜：计数降序、同计数按色值升序）+ Floyd-Steinberg 抖动，stdlib 内实现。
- 理由：Plan9 固定调色板是青草地/粉泥土的唯一成因（渲染器无辜，herd.png 为证）；自适应后 raw→gif 保真，阈值裁的仍是录制漂移。
- 否决：引入外部量化库/ffmpeg/mp4——新依赖、CI 负担，无必要。

### P2：低头角按颈几何重算 + 闲时点头/看人

- 选择：低头角常量改为“吻部距站立平面 ≤0.5 格”反推（读牛头包围盒与转轴，测试锁定具体弧度）；闲时点头为客户端 tick 派生 pitch 正弦（drop 旋转同先例）；看人为服务端闲时朝向规则（漫游且 6 格内有 active 玩家则转向，每 tick 有界角；逃跑/吃草/引诱让路），位置速度不动。
- 理由：位姿走既有 Pitch 通道，零 ABI 变更；看人放服务端才不违反“客户端不猜玩法”。
- 否决：头部 yaw 通道（要改 96B 实例布局 → ABI v16，为装饰不值）。

### P3：死亡掉落关联滞后呈现

- 选择：客户端以（死亡牛位置邻域 2 格 + upsert tick ∈ [deathTick, deathTick+20]）关联掉落；关联掉落在相位 <50% 不渲染，≥50% 起 scale-in + 白闪一次；拾取走权威不受影响。
- 理由：不改服务端时序（掉落即时可拾的权威语义不变）；启发式只影响呈现，稀疏击杀下精确，密集击杀下任取其一亦可接受（注释写清）。
- 否决：服务端延迟发布掉落——会拖慢拾取，改权威语义。

### P4：GIF 剧本语义升级

- 选择：lure 重写（牛逐帧追人止步+朝人，持麦人帧内可见）；graze 在 Step 内切换触发格草→泥（需给 `SceneApplication` 加 capture-only 写块口，app 实现、测试守卫）；kill 保持 upsert 时机诚实（滞后由 P3 呈现承载）。
- 理由：GIF 是呈现基线，sim 逻辑由单测兜底；写块口限定 capture 消费，`audit` 依赖方向不变。

## Polish 附录二（Avatar 动作与掉落形态，MC 对齐）

### P5：非方块掉落改为单贴图薄片

- 选择：掉落按物品分类分形——方块类保持迷你纹理立方体；非方块类（食物/工具/种子等）改为单张贴图薄片（双面同图，约 1/2 缩放），浮动+绕 Y 旋转不变。若实例管线只有立方体图元，则用极薄扁盒（厚度 ~0.02）表达薄片，零管线变更。
- 理由：MC 同形（方块=缩小立方体，非方块=贴图）；用户明确“只需要一张贴图”。
- 否决：billboard 广告牌（需相机朝向通道，实例布局要加字段 → ABI 变更，为掉落不值）。

### P6：四肢摆动（tick 派生）与引诱朝向

- 选择：摆动相位 = f(serverTick, 实体 ID)，速度由呈现位置差分估计，阈值下回中；死亡/低头位姿优先。引诱牛（含止步）每 tick 朝向持麦玩家——位置不动只转朝向，走权威 yaw（sim 改，非呈现猜）。
- 理由：沿 `PassiveIdleNodPitch`/`dropAnimationPhase` 既有范式，零协议零 ABI；止步朝向必须权威（呈现改 yaw 会与服务端快照打架）。
- 否决：头部 yaw 通道（ABI v16，否决过）；客户端改身体 yaw（与快照不一致，快照一到就抽搐）。

### P7：牛皮按 Pixel Perfection 重绘

- 选择：只重画 `cow_hide`/`cow_head` 程序化像素（奶油风：柔和高光、规范斑块、面部五官、蹄色），几何与层号不动；对照包内棕色系（泥土/木板）取色，与牛肉层同暖。
- 理由：包内无牛皮文件（已审计），几何已定型；像素重绘零契约影响。
- 否决：引入外部牛皮（上轮已因确权降级，不重蹈）。

### P8：走路摆动波及全部 Avatar 的 golden 再生

- 选择：摆幅取小（MC 感知下限之上、夸张之下），capture 固定 tick 下确定性姿态；全部含 Avatar 的 PNG/GIF 按“预期视觉变化逐图确认”纪律再生。
- 理由：玩家/伙伴/夜行者同通道，修牛 Diablo 伐木累——一次并到本波，不留“牛会走人不走”的半吊子。
