# 权威护甲

## Why

生存闭环收尾战役阶段一的第二行（积压表 B-24，依赖难度行已交付）。当前玩家对唯一的常规伤害源——夜行者近战（每击 3 伤害）——没有任何减伤手段，护甲条在 `archive/2026-08-29-client-ui-vanilla-alignment/design.md` 中被显式登记为「待机制落地后另行对齐」。铁锭已有完整获取链（铁矿→熔炼），价值闭包守卫不因本 change 新增任何不可获得物品。

2026-09-12 用户显式批准四项语义裁决：**仅铁质一套**（皮革不存在，牛只掉生牛肉）；**4 槽**（头/胸/腿/脚）；**点数 + 4%/点减免模型**；**耐久随本行交付**。

## What Changes

- `packages/shared/core` 新建护甲域单一真源：4 个槽位、每件护甲的护甲点数与耐久上限、`MaxArmorPoints = 20`、整数确定性减免公式；物品注册表追加铁质头盔/胸甲/护腿/靴子四件（编号 58..61，哨兵 `ItemIDMax` 58→62），配方注册表追加四条工作台配方。
- `packages/shared/network` 协议 v41→v42：`PlayerState` 载荷尾部（`Temperature` 之后）追加 `ArmorPoints uint8`；新 C→S 命令 `EquipArmor`（Play C→S ID 18，载荷仅 `Sequence`）；`RejectReason` 追加 `not_armor`。
- `packages/server/sim/entity`：`playerState` 增加独立 `armor [4]ItemStack` 装备状态（不进 `core.Inventory` 的 36 槽索引空间）；装备动作在权威侧与所选快捷栏格原子互换；近战结算点对玩家目标按冻结护甲点数减免有效伤害（覆盖敌怪→玩家与玩家→玩家）；受击产生减免时每件参与点数的完好护甲件消耗 1 点耐久；死亡掉落包含已装备护甲；`PlayerHash` 追加装备区。
- `packages/server/storage/player` 玩家 schema v8→v9：装备四格随快照持久化（每格沿用 3 字节栈编码），v8 旧档只读迁移为空装备。
- `packages/client`：镜像 `ArmorPoints`、桥 `uiState` 追加 armor 分节（client ABI v18→v19，Go/Rust/TS 三端钉值）、WebView 状态行组件族新增护甲条（10 档图标、半档粒度、点数为 0 时零像素差异）、使用键手持护甲时上行 `EquipArmor`（沿 F-03 判定先例）。
- `packages/server/server`：命令接线与 Memory/TCP parity；重启保值集成测试。
- 新 capture 场景（穿甲 HUD）与 golden 追加；audit 增加护甲域单一真源守卫。

## 契约与版本影响

- 协议 v41→v42（`PlayerState` 尾部 1 字节 + 新命令 ID 18 + 新拒绝原因）；旧版登录拒绝语义不变。
- 玩家 schema v8→v9（尾部追加 12 字节装备区；v1..v8 只读迁移，v8 迁移后装备为空）。
- client ABI v18→v19（桥 `uiState` 追加 armor 分节，三端钉值同步）。
- engine ABI、区块 schema、世界 metadata、`companions.ai`/`hostile_mobs`/`passive_mobs` schema、benchmark scenario 均不变。
- golden：点数为 0 时 HUD 零像素差异，既有场景不重生成；新增 1 张穿甲场景 golden。

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
