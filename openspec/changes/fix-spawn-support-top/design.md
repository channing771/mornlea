## Context

见 `proposal.md` 的 Why。`physics.BlockCollisionBoxes` 返回有界的 `CollisionBoxSet`，每格最多 8 个局部 AABB；耕地是当前唯一非满碰撞方块，顶面为 `15/16`。`validateRestoreCandidate` 与 `updateSafeLocation` 已通过 `playerSupport` 验证真实盒顶和完整支撑，只有共享的 `findSpawnInColumn` 仍把“存在碰撞盒”简化成脚底 `y+1`。

`findSpawnInColumn` 同时服务玩家与伙伴出生，并在候选身体无碰撞后复用既有三档流体出生阶梯。它在权威 tick 单写者内运行，不跨 goroutine，也不拥有或修改碰撞数据。

## Goals / Non-Goals

**Goals:**

- 让出生候选脚底精确落在真实碰撞盒顶面。
- 复用 `playerSupport`，使出生、要求支撑的恢复与 safe 点更新共享同一完整支撑定义。
- 保持列扫描、区块就绪等待和流体干燥度分档的既有顺序与边界。

**Non-Goals:**

- 不修改碰撞注册表、物理积分、玩家包围盒或 `GroundProbe`。
- 不新增缓存、通用表面索引或可配置策略。
- 不改变登录、死亡、协议或存档格式。

## Decisions

### D1：从全部碰撞盒顶面生成候选，并保持自上而下顺序

`findSpawnInColumn` 对每个已加载方块读取 `CollisionBoxSet`，把有效记录的局部 `Max.Y` 转成世界脚底 Y，并按顶面从高到低考察。固定的 8 项上限允许使用栈上固定容量数据完成排序，不引入堆分配；外层仍按世界 Y 自上而下，因此列扫描语义不变。

选择全部盒而不是只读第一个盒，是因为 `CollisionBoxSet` 的公开契约本就允许多盒，数组顺序不代表表面高低。只读第一个盒会把未来的多盒方块重新变成同类缺陷。

### D2：候选必须通过既有完整支撑判定

每个脚底候选先走现有身体空闲检查，再调用 `playerSupport`；只有 `completeSupport=true` 才进入 `spawnTierOf`。更高盒顶被阻挡或只提供部分支撑时继续考察较低候选。

被否决的替代方案是仅按候选盒自身的水平范围判断。该做法会复制 `playerSupport` 的跨格覆盖、边界和 epsilon 规则，形成第四套支撑口径；复用现有函数是更小且更可靠的根因修复。

### D3：只增加一个同主题行为测试文件

新增 `internal/sim/spawn_support_top_test.go`，用真实耕地方块验证三条消费者路径：出生落在 `15/16` 顶面、该精确位置可作为登录恢复候选、grounded 后可写入 safe 点。测试以字面期望值断言可观察位置；既有 `spawn_test.go`、`player_restore_test.go` 与 `player_lifecycle_test.go` 继续覆盖流体阶梯、部分支撑和 safe 更新的一般规则。

不为尚无生产方块消费者的任意多盒形状增加测试专用生产接口；全部盒枚举由实现循环和独立评审核对，出现第二个多盒方块时再用真实注册表条目补行为夹具。

## Risks / Trade-offs

- **[多个盒顶的数组顺序影响出生结果]** → 明确按顶面降序，不信任注册顺序。
- **[复用 `playerSupport` 增加扫描工作]** → 每个方块最多 8 个盒，出生候选与世界高度已有硬上限；工作只发生在待出生扫描，不进入 active 玩家热路径。
- **[浮点脚底误差导致支撑失败]** → 方块碰撞坐标沿用现有 `float32` 与 `CollisionEpsilon`；当前 `15/16` 可被二进制精确表示。
- **[玩家与伙伴行为同时变化]** → 两者本就共享出生函数与碰撞形状；保持共享可避免产生第二套出生语义。

## Migration Plan

无需数据迁移或灰度开关。部署后只影响新发生的出生回退；已有 current/safe 位置按既有恢复规则验证。回退时整支 revert 即可。
