# 权威护甲

## Why

生存闭环收尾战役阶段一的第二行（积压表 B-24，依赖难度行已交付）。当前玩家对唯一的常规伤害源——夜行者近战（每击 3 伤害）——没有任何减伤手段，护甲条在 `archive/2026-08-29-client-ui-vanilla-alignment/design.md` 中被显式登记为「待机制落地后另行对齐」。铁锭已有完整获取链（铁矿→熔炼），价值闭包守卫不因本 change 新增任何不可获得物品。

2026-09-12 用户显式批准四项语义裁决：**仅铁质一套**（皮革不存在，牛只掉生牛肉）；**4 槽**（头/胸/腿/脚）；**点数 + 4%/点减免模型**；**耐久随本行交付**。

## What Changes

- `packages/shared/core` 新建护甲域单一真源：4 个槽位、每件护甲的护甲点数与耐久上限、`MaxArmorPoints = 20`、整数确定性减免公式；物品注册表追加铁质头盔/胸甲/护腿/靴子四件（编号 58..61，哨兵 `ItemIDMax` 58→62），配方注册表追加四条工作台配方。
- `packages/shared/network` 协议 v41→v42：`PlayerState` 载荷尾部（`Temperature` 之后）追加 `ArmorPoints uint8`；新 C→S 命令 `EquipArmor`（Play C→S ID 18，载荷仅 `Sequence`）；`RejectReason` 追加 `not_armor`。
- `packages/server/sim/entity`：`playerState` 增加独立 `armor [4]ItemStack` 装备状态（不进 `core.Inventory` 的 36 槽索引空间）；装备动作在权威侧与所选快捷栏格原子互换；近战结算点对玩家目标按冻结护甲点数减免有效伤害（覆盖敌怪→玩家与玩家→玩家）；受击产生减免时每件参与点数的完好护甲件消耗 1 点耐久；死亡掉落包含已装备护甲；`PlayerHash` 追加装备区。
- `packages/server/storage/player` 玩家 schema v8→v9：装备四格随快照持久化（每格沿用背包格同一 5 字节栈编码），v8 旧档只读迁移为空装备。
- `packages/client`：镜像 `ArmorPoints`、桥 `uiState` 追加 armor 分节（client ABI v18→v19，Go/Rust/TS 三端钉值）、WebView 状态行组件族新增护甲条（10 档图标、半档粒度、点数为 0 时零像素差异）、使用键手持护甲时上行 `EquipArmor`（沿 F-03 判定先例）。
- `packages/server/server`：命令接线与 Memory/TCP parity；重启保值集成测试。
- 新穿甲 HUD 部件基线场景（`frontend/visual` fixture `hud-armor`）与 golden 追加；audit 增加护甲域单一真源守卫。

## 契约与版本影响

- 协议 v41→v42（`PlayerState` 尾部 1 字节 + 新命令 ID 18 + 新拒绝原因）；旧版登录拒绝语义不变。
- 玩家 schema v8→v9（尾部追加 20 字节装备区；v1..v8 只读迁移，v8 迁移后装备为空）。
- client ABI v18→v19（桥 `uiState` 追加 armor 分节，三端钉值同步）。
- engine ABI、区块 schema、世界 metadata、`companions.ai`/`hostile_mobs`/`passive_mobs` schema、benchmark scenario 均不变。
- golden：点数为 0 时 HUD 零像素差异，既有场景不重生成；新增 1 张穿甲部件基线 golden（前端 `ui/` 30→31，世界场景 30 张逐位零差异）。

## 用户可观察结果

- 穿戴铁质护甲后 HUD 心形行上方出现护甲条，档位随总点数增减。
- 夜行者与 PvP 近战伤害可感知下降（满套 15 点时 3 伤害近战实际扣 1）。
- 护甲件有耐久，反复受击会逐渐损耗直至损坏形态；损坏件仍可穿戴但不再提供点数。
- 工作台出现四条铁质护甲配方；手持护甲按使用键即与对应槽位互换穿戴。

## 非目标

- 护甲在远端玩家/头像上的可见呈现。
- 附魔、皮革链、其他材质护甲。
- 通用装备栏容器 UI（护甲槽不进容器面板；穿戴只走使用键互换）。
- 护甲修复（铁砧/合成修复）。
- 伙伴装备护甲或感知护甲（伙伴 world actions 面不变）。
- 护甲减免不适用于摔落、溺水与饥饿（这些路径不经近战结算点，维持现状）。

## 受影响的包与文档

`packages/shared/core`、`packages/shared/network`、`packages/server/sim`（entity）、`packages/server/server`、`packages/server/storage`（player）、`packages/client`（client/mesh/assets/render/cmd 前端与 capture）、`packages/audit`、根 `AGENTS.md` 版本矩阵、`openspec/config.yaml` 上下文版本矩阵、`docs/notes/progress.md` 基线段、`docs/feature-backlog.md` B-24 行回填。

## 延期与放弃

（实现期登记，暂空）

> 修订（2026-09-12，控制会话）：原将护甲四件的快捷栏 sprite 顺延 D-11；实现任务组 1 时实测客户端存在「所有注册物品必须有图标」守护测试（顺延将带红 21 例），改判**任务组 6 交付四件程序化 sprite**，不等 D-11。

> 修订（2026-09-12，任务组 3 实现）：装备区字节布局由「每槽 3 字节、共 12 字节」更正为「每槽沿用背包格同一 5 字节栈编码（item u16 小端 + count 1 字节 + durability u16 小端）、共 20 字节」。原前提「背包格每格 3 字节」与 codec 现实不符——3 字节只是 v2/v3 legacy 无耐久布局；耐久 0/165 形态必须逐位保值的 MUST 也只有 5 字节编码可满足。

> 修订（2026-09-12，任务组 7 实现）：穿甲 HUD 场景载体由世界 capture 场景更正为前端部件基线（`frontend/visual` fixture）。原前提「无头 capture 画面呈现 HUD 条带」已不成立——常显 HUD（状态行、氧气、准星等）自 `webview-game-ui-unification` 起全量迁 WebView，无头路径 hudPush 纪律层零值退化、画面零 HUD 像素（water-underwater golden 目检佐证）；护甲条像素的唯一可入库载体是前端部件基线。世界场景 30 张与前端部件 30 张既有基线的零差异验收口径不变。
