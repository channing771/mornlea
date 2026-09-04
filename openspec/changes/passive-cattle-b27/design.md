# Design: passive-cattle-b27

## Context

见 `proposal.md` Why。当前真相：夜行者独占被动/敌对实体栈（`packages/server/sim/entity/hostile.go` 定容升序集、PMST 同源 `hostile_mobs.bin`、`HostileSpawn/State/Despawn` 三消息、`render/avatar.go` 纯色 6-cuboid + `avatar.wgsl` 纯 vertex color）。默认材质为程序化注册表 + Pixel Perfection 子集（`assets/blocks.go` 植物 31..54 冻结）。本 change 复刻该栈为被动分支并贴图化，详见 specs 四能力。

## Goals / Non-Goals

- Goals：牛全链闭环可用；贴图写实且风格统一；协议/存档/ABI 只追加；权威 tick 有界。
- Non-Goals：不改夜行者/伙伴任一行为；不建 ECS；不做繁殖/牛奶；不碰旧 golden 像素语义。

## Decisions

### D1：复刻夜行者栈，不共享实体抽象（B-26 延后）

- 选择：新建 `passiveSet`/`StoredPassiveMob`/`Passive*` 消息，与 hostile 双轨并存。
- 理由：行为差异大（昼夜反侧、逃跑 vs 追击、无灼烧/远程消失），共享抽象此刻只会制造错误耦合；`feature-backlog` B-26 明确第二类 mob 落地后再评估。
- 否决：泛化 `MobSet[T]` 泛型——省 ~100 行，丢双轨独立演进能力。

### D2：牛上限 32/6（独立于夜行者 64/8）

- 选择：全服 32、每玩家附近 6。
- 理由：被动生物密度需求低于威胁生物；独立计数避免牛群挤占夜行者生成预算；数值由边界测试锁定，spec 已钉死。
- 否决：复用 64/8——牛群过密遮挡且掉落通胀。

### D3：Avatar 实例加 1×u32 材质槽，shader 复 terrain atlas 绑定范式

- 选择：实例 80→96 bytes（`transform mat4` 64 + `color vec4` 16 + `material u32` 4 + 12B 保留对齐，96%16==0 满足 uniform 步长；实现阶段由初估 88 按对齐锁定为 96），`avatar.wgsl` 加 `texture_2d<f32> + sampler`，UV 由 cuboid 面顶点本地坐标派生（0..1），cutout 沿用 `a<0.5 discard`。
- 理由：复用既有 atlas 上传/预热/容量语义，玩家/伙伴/夜行者分支传哨兵材质走原纯色路径，零回归。
- 否决：另起 entity-textured pass——多一管线、多一容量表、多一 golden 变量；否决 CPU 端逐像素 tint（热路径浪费）。
- 同步面：`entity.rs AVATAR_MAX_INSTANCES` 不变；`mornlea_client.h` + Go bridge + client ABI v14→v15 + 跨语言布局测试同批。

### D4：4 新层追加末位，牛身只取牛皮/牛头

- 选择：`LayerCowHide/LayerCowHead/LayerRawBeef/LayerCookedBeef` 追加枚举末位；`Material()` 牛身六面映射前两者，生/熟走掉落/HUD。
- 理由：植物区间连续性是 Rust `quad.rs` 识别作物的唯一依据，中间插入即错乱（`blocks.go:56` 纪律）。
- 否决：复用现有皮革/肉色层——辨识度不足且违反“材质可溯源”纪律。

### D5：外网 CC0 转制入库，程序化为回退

- 选择：牛身 `mobs_animal mobs_cow.png CC0`、牛肉 OGA `16x16 Food CC0`；转制 16×16 + Pixel Perfection 噪点统一；`PROVENANCE.json derived` + `ATTRIBUTION.md`；`applyPack` 门禁；程序化像素保留。
- 理由：用户批准免版税底线；boxy UV 与 cuboid 同构，转制量最小；溯源沿 `fix-grass-block-texture` 先例。
- 否决：原样塞 32×32——破坏 mip/上传预算且风格断裂；否决纯程序化牛身——用户明确拒纯色。

### D6：新存档文件独立（PMST），不扩展 hostile_mobs

- 选择：`passive_mobs.bin` 独立文件/schema v1，`storage/passive` 新域，`audit allowed` 登记 `hostile` 同形边。
- 理由：两类生物上限/字段演进独立；混文件导致任一损坏双双无法启动。
- 否决：`hostile_mobs.bin` 加 kind 字段——v1 golden 全废且重启语义纠缠。

## Risks / Trade-offs

- [实例布局变更错位] → Go/Rust 双侧布局单测 + 真机对照 + ABI 升版不同装；旧库拒混装。
- [外网素材风格断裂] → 转制评审必须逐图人工确认；不通过则回退程序化牛肉图标 + 牛身斑点程序化叠加（仍满足“非纯色”底线）。
- [golden 膨胀] → 只加 `passive-herd` 一景；旧景逐字节门禁先行。
- [牛群遮挡] → capture 坐标按 hostile-mob 先例做屏幕投影间距核算。
- [上限调参争议] → 32/6 为初值，任务评审可凭性能数据裁决微调，spec 同步改。

## Migration Plan

- 部署：新文件缺失即空集合；协议追加，旧客户端不混用；材质缺层回退程序化。
- 回滚：整分支 revert 即可；`passive_mobs.bin` 残留被新版忽略（旧版不识别但不删除）； wound：回滚后牛物品残留背包按未知物品既有语义处理（design 不新增语义，实现阶段以 `core` 现状为准）。

## Open Questions

- 无。Avatar 实例 88B 对齐细节与饥饿数值（生/熟具体点数）在任务内按 `core` 既有食物表（面包/腐肉）同档锁定，不改 spec 结构。
