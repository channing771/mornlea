# 设计：权威护甲

## 数据所有权与依赖方向

- **core（`packages/shared/core`）**：护甲域唯一真源——新文件 `armor.go` 定义槽位枚举、护甲件→槽位映射、每件点数与耐久上限表、`MaxArmorPoints`、`ArmorPoints(worn)` 求和（损坏件计 0）与 `ReducedDamage(damage, points)`；`item.go` 追加四物品与哨兵推进；`recipe.go` 追加四配方。所有其他包只读消费，禁止复制表值。
- **sim（`packages/server/sim/entity`）**：装备状态的唯一写者。`playerState` 增加私有 `armor [4]core.ItemStack`；新文件 `armor.go` 承载装备互换结算与减免钩子；`combat.go` 的快照结构增加 `armorPoints` 冻结字段并在 `settleCombatIntent` 目标为玩家时调用 `core.ReducedDamage`；`player.go` 快照/恢复、`PlayerHash` 与死亡掉落扩展；耐久消耗复用 `consumeToolDurabilityAt` 同族机制新增护甲变体。世界状态仍是单 goroutine 顺序推进，无新并发边界。
- **network（`packages/shared/network`）**：`protocol/packet.go` 版本 42；`message_player.go` 尾部追加 `ArmorPoints`；`message_command.go` 新 `EquipArmor{Sequence}` 与 `not_armor` 拒绝原因；`registry.go` 登记命令 ID 18 与拒绝原因 ID。纯追加，无既有字段重排。
- **storage（`packages/server/storage/player`）**：schema v9 在既有载荷尾部追加 4×5 字节装备区（每槽沿用背包格同一 5 字节栈编码：item u16 小端 + count 1 字节 + durability u16 小端）；`player_types.go` 的 `StoredPlayer`/`PlayerSave` 增加装备字段；`player_migration.go` 增加 v8→v9 只读迁移；testdata 增加 v9 fixture 并保留 v8 fixture 作迁移输入。
- **server（`packages/server/server`）**：`session_ingress.go` 接线 `EquipArmor`；parity 与重启保值集成测试。
- **client（`packages/client`）**：镜像 `ArmorPoints`；桥 `uiState` armor 分节（client ABI v18→v19，Go 组装、Rust 中继零行为、TS 类型三端钉值）；前端状态行组件族新增护甲条；`cmd/mornlea` 输入路径在使用键上升沿且手持护甲时上行 `EquipArmor`（不发 `PlaceBlock`）；capture 新场景。

## 关键裁决

### D1 减免挂在近战结算点，不进 `applyDamage`

`playerState.applyDamage` 是摔落、溺水、饥饿、近战四类伤害的唯一入口；在其中减免会错误减免前三类。结算点 `settleCombatIntent` 已有快照冻结纪律（栏位、物品、冷却），护甲点数随同一快照冻结，天然满足「结算时刻点数」语义且覆盖敌怪→玩家与玩家→玩家。
**否决替代**：在 `applyDamage` 内按伤害来源枚举减免——需要引入伤害来源枚举并改动全部四个调用方，契约面更大且违反「最小受控」；每 tick 在玩家状态上缓存点数——引入缓存失效路径，快照冻结已足够。

### D2 装备不进 `core.Inventory` 的 36 槽索引空间

护甲四槽独立为 `playerState` 上的定长数组，不占用 `InventorySlots` 统一索引。
**否决替代**：把 `InventorySlots` 扩到 40——`MoveInventoryStack`/`MoveCraftingStack`/容器装回不变量/`Slot()` 索引语义全部要重排，波及面远超本行；独立 side schema（沿 `hostile_mobs` 先例）——装备与玩家状态强耦合，拆两套文件徒增原子性与恢复序复杂度。

### D3 点数 + 4%/点，纯整数

公式 `max(1, damage×(100−4×points)/100)`，int32 整数乘除、向零取整，单表达式无浮点，Memory/TCP/重放三方天然一致。铁质四件点数取 2/6/5/2（合计 15），耐久上限取 165/240/225/195，与既有剑耐久数值口径（59/131/250）同族。
**否决替代**：浮点百分比——引入跨语言舍入歧义；每件固定减伤——多件叠加语义与点数条 HUD 呈现都不如点数模型干净。

### D4 不新增私有成功确认消息

装备互换改变所选快捷栏格，既有 `InventoryUpdate` 广播已覆盖客户端可见状态；`ArmorPoints` 随 `PlayerState` 下行。
**否决替代**：仿 `PlaceBlockSucceeded` 增加私有确认——无信息增量，徒占协议面。

### D5 HUD 零点数零像素

点数为 0 时护甲条不渲染任何像素，保证协议 v41 既有 golden 逐位零漂移（沿 B-12 `SaturationZero` 抖动门控先例）；穿甲呈现走新 capture 场景。
**否决替代**：常显空护甲条——全部世界场景 golden 重生成，违背最小视觉扰动。

### D6 损坏形态护甲可穿戴、贡献 0 点

与剑的损坏形态语义同族（不消失、保留传递）；避免「耐久归零即销毁」引入新的物品销毁路径。
**否决替代**：耐久归零销毁——需要新的物品移除结算与装备槽清空路径，超出本行最小闭环。

## 兼容、迁移与回退

- **线上协议**：v41 客户端连 v42 服务端按既有单向纪律在登录层拒绝；`PlayerState` 载荷尾部追加、命令 ID 与拒绝原因均为注册表追加，无既有 ID 重排。
- **存档**：v1..v8 只读迁移读入（装备空），首次保存写 v9；无回写旧版路径（沿全仓存档单向纪律）。
- **回退方案**：整分支 revert 即回到 v41/v8/v18 基线；因存档只前向，回退后 v9 存档不可读属已接受代价（与历次 schema 升版一致）。
- **验证方法**：定点 `-race`（core/network/storage/sim/server/client 六组）→ parity（Memory/TCP 装备与减免一致）→ 重启保值集成 → `make frontend-check` → `make visual-check`（既有场景零差异 + 新场景）→ 全量 `make test-race` + `make rust` + `openspec validate --all --strict`（阶段 4 门禁）。

## 版本槽位持有

本 change 持有：协议 v41→v42、玩家 schema v8→v9、client ABI v18→v19。engine ABI、区块 schema、世界 metadata、benchmark scenario、`companions.ai`/`hostile_mobs`/`passive_mobs` schema 不变（绝对版本号在合入时按届时 main 复核）。
