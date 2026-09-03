## Why

当前农业闭环仍靠新玩家材料包固定赠送 `64` 颗种子启动，世界本身没有可探索、可采集的第一颗种子来源，也让已经交付的通用植物几何只有作物一个消费者。现在补入自然短草并把除草作为种子来源，可以用一个独立、可回退的闭环清偿这项临时设计，同时保持服务端权威与 Rust worldgen 的既有边界。

## What Changes

- 在稳定方块编号末尾追加一种单格 `ShortGrassID`（不追加对应物品）：服务当前 Overworld 新区块的 Rust worldgen 在草地表面按世界种子和世界坐标确定性分布短草，橡树结构保持优先，已保存区块不扫描、不迁移、不补种。
- 短草复用既有植物交叉斜面、terrain cutout 与植物光照路径，使用本项目原创程序化纹理；短草不提供碰撞、不是完整遮光方块，玩家可穿过。
- 客户端派生天空光时，只有完整不透明方块阻断，作物与短草等植物透光且不产生额外衰减，流体继续产生额外衰减，缺失或未知邻区继续阻断；静态方块光只向空气或植物格传播并每格衰减 `1`，玻璃、水及任何其他非空气、非植物方块仍阻断。
- 玩家以任意手持状态采除短草均在 `1` 个权威 tick 完成；结算参考 Minecraft 的普通除草手感，以可重放且重试稳定的确定性 `1/8` 判定掉落恰好 `1` 颗小麦种子，其余情况无掉落。种子进入既有权威世界掉落物系统而非直接写入背包；需要掉落但容量不足时，方块、掉落物与 revision 保持原子不变。
- **BREAKING（新玩家初始状态）**：只对玩家存档明确不存在的登录取消固定 `64` 颗种子赠送，保留现阶段前 `14` 格材料整栈；已有玩家的全部栏位逐槽保留，不删除、补发或重排已有种子。
- 伙伴继续显式拒绝采掘短草；本闭环不加入双格高草、短草物品、剪刀、Fortune/附魔、骨粉生草、自然再生或通用植被系统，也不引入 Mojang 版权资源。
- **BREAKING（内部 ABI）**：worldgen `MGW1` 请求材料表由 `14` 项扩为 `15` 项，layout `2` 升为 `3`、header `564` 字节升为 `566` 字节，engine ABI v9 升为 v10；Go 仍只编码请求并经 `internal/nativeabi` 调用 Rust，生产路径不增加 fallback。
- 固定 benchmark 世界会因自然短草而改变，benchmark scenario v20 升为 v21，比较器当前唯一允许的显式跨 workload 迁移改为 `20:21`；受影响的固定世界抓帧与菜单全景按既有无窗口完整链路重立并复核，视觉阈值和场景清单不放宽。

## Capabilities

### New Capabilities

- `natural-grass-generation`: 定义单格短草的稳定编号、确定性自然生成、橡树优先与仅影响未生成区块的兼容边界，以及无碰撞、非完整遮光的植物方块语义。

### Modified Capabilities

- `authoritative-mining`: 在固定采掘规则中加入短草的 `1` tick 权威结算、确定性 `1/8` 单种子世界掉落、无掉落路径、容量失败原子性与伙伴拒绝。
- `authoritative-fluid`: 把短草加入流动水的可替换目标，覆盖时零掉落且不受掉落容量限制，同时保持作物冲毁的既有掉落与容量重试语义。
- `tool-durability`: 把玩家成功采除短草加入第三类明确零耐久豁免，任意工具均不磨损且耐久为 `1` 的工具不会转为损坏形态。
- `authoritative-farming`: 把“第一颗种子”从新玩家固定赠送改为玩家采除自然短草取得，同时继续保证可启动的种植闭环。
- `common-block-materials`: 缺失玩家的一次性材料包不再包含起步种子，已有玩家与未确认登录的保护语义保持不变。
- `rust-engine-worldgen`: Rust 独占的生产世界生成新增短草覆盖层，并更新“既有方块输出不变”要求及 worldgen ABI 输入契约。
- `bounded-benchmark-workload`: 固定 benchmark 世界纳入自然短草，scenario 升至 v21，并把唯一显式迁移更新为 `20:21`。
- `authoritative-daylight`: 把直射天空光遮挡规则收敛为仅完整不透明方块阻断，明确植物无额外衰减、其他非完整遮光方块可透过，同时保持流体额外衰减和缺失或未知邻区阻断。
- `static-block-light`: 把作物与短草加入静态方块光可传播目标，保持每格衰减 `1`，并继续要求玻璃、水及任何其他非空气、非植物方块阻断。
- `visual-verification`: 固定地形与菜单全景覆盖新的自然短草外观，并要求只重立和复核受影响基线、保持既有阈值及无窗口完整渲染链路。

## 延期与放弃

以下事项经任务级与 whole-change 双评审确认为 Minor/latent/流程性,不阻塞本 change,逐条誊自 ledger 各 Ruling:

- `worldgen.rs` 中 `apply_short_grass`（约 `:458`）与 `short_grass_block_at`（约 `:339`）对世界顶部 Y 的守卫不对称——仅当 `surface == WORLD_MAX_Y-1` 时可达,冻结地形常数下不可达;未来任何调整地形常数的 change 必须先对齐两路径守卫。（Task 2.1 Ruling deferred,whole-change 复核确认为 latent-only。）
- Go light oracle 以 `core.IsPlant`、Rust `light.rs` 以 material 离散集合 `[31..54]∪{68}` 表达植物判定——当前生产 registry 下 outcome-equal,由 `internal/core`/`internal/assets` 穷举测试与真实 FFI 走廊绝对值断言把守;未来新增 plant 材质方块必须保持 material⟺`IsPlant` 一致,否则两侧光照实现会静默分叉。（Task 1.1 Ruling deferred。）
- `engine/crates/mornlea_engine/src/ffi.rs:3033-3034` 注释把偏移 26 的 stone 槽误写为 dirt——断言与语义正确,仅注释措辞;`worldgen.rs` 测试助手 `audit_short_grass` else 分支 `assert_ne!` 恒真——死断言无危害。两项均记录不返工。（Task 2.1 findings。）
- `light.rs` `build_block` 对每个存在的发射级做一次全 volume 重扫——最坏 16 次、生产 torch=14/light=15 为 3 次,有界、零分配,记录备查不优化。（Task 1.1 Ruling。）
- `docs/notes/lan-server.md` 的陈旧版本矩阵（engine ABI v8/client ABI v11/scenario v20）为 F-04 独占文件,本 change 按 Setup Ruling 不动,由 F-04 或控制会话在其独占范围内补齐。（Setup Ruling + Task 8.1 Info。）
- 流程交接记录:Task 5.1 R1 与 Task 7.1 R1/R2 的修复轮由接手 agent 完成（原 implementer 会话分别因平台配额中止、被用户暂停不可恢复）,scoped 修复说明与独立复评完整,协议实质未破。（Task 5.1/7.1 Ruling。）
- 合并义务:本分支版本矩阵如实记录 client ABI v13 与 `companions.ai` schema v4（分支代码实况,早于 main 后续 v14/v5 bump）;PR 并 main 时必须按 main 最终基线重新核对版本矩阵与版本相关文档段落。（Task 8.1 Ruling + whole-change M4。）

## Impact

- 主要影响 `internal/core` 方块注册、`internal/assets` 程序化 layer、`internal/mesh` 与 Rust mesher 的植物材质判定集合（从 `[31..54]` 变为 `[31..54] ∪ {68}`，`55..67` 不移动）、Rust light kernel 及其 Go oracle、`internal/sim/entity` 玩家采掘与工具耐久结算、伙伴采掘白名单、`internal/server/persistence` 缺失玩家快照，以及 `internal/worldgen`、`internal/nativeabi`、Rust fluid kernel 及其 Go oracle 和 `mornlea_engine` worldgen。
- benchmark producer/comparator、capture 固定世界与受影响 golden、版本矩阵文档和 OpenSpec 主规格需要同步。`plant-visual-presentation` 与 `deterministic-tree-generation` 的既有 Requirement 不改变，只作为第二消费者与树优先回归门禁继续成立。
- 线上协议保持 v32；player schema v8、chunk schema v9、world metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、client ABI v13 均不变，也不改变 packet 或存档字节布局。旧存档可直接加载；新短草只写入之后生成并保存的区块，旧程序不认识新增编号，因此降级需恢复升级前备份，且不支持新旧二进制混用。
- 光照修订继续复用既有固定容量传播工作内存，不扩大无界队列，也不改变 packed light、mesh registry、quad 或 light kernel 的 ABI 输入输出布局；本变更唯一 ABI 动作仍是 worldgen 所需的 engine ABI v9 升至 v10。
- 权威 tick 仍是每名活动玩家一次有界射线与常数工作；worldgen 只增加每个生成格的固定整数判定，光照传播仍受既有固定容量上限约束，不引入 goroutine、热路径 I/O、无界遍历或进程级随机源。
