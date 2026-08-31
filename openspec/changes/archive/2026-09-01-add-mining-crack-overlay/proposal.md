# add-mining-crack-overlay

## 背景

玩家采掘方块时，唯一的进度反馈是快捷栏上方的 HUD 采掘进度条。方块本身没有任何
视觉变化，玩家必须把视线从目标方块移开才能读进度，与体素游戏的直觉不符
（同类游戏以方块表面逐级加深的裂纹作为主要进度反馈）。

权威采集链路已经完整存在：服务端 `internal/sim/runtime` 逐 tick 推进
`miningState{target, block, progressTicks, requiredTicks}`，并经
`network.PlayerState{MiningActive, MiningTarget, MiningProgressTicks,
MiningRequiredTicks, MiningHarvestable}` 每 tick 发布给客户端；客户端在
`cmd/mornlea/app` 已镜像为 `hud.MiningOverlay`（当前只喂 HUD 进度条）。
渲染侧已有"复用单位立方体 + 帧内 TLV 实例流"的选框先例
（`render.BlockOutline` → tag 3 → Rust `EntityPass`），具备零 ABI bump
新增覆盖层的全部基础设施。

## 目标

- 玩家采掘时，在当前权威目标方块的六个表面上叠加像素风裂纹：随权威采集进度
  以 10 个离散阶段逐级加深，进度饱和（即将破坏）时裂纹最重。
- 裂纹是原方块材质之上的透明 cutout overlay，覆盖整个方块，不替换、不修改
  原方块材质。
- 全部状态来自最后确认的权威采掘状态镜像：不新建任何动画计时器、不预测进度、
  不改动服务端采集规则。
- 停止采集、切换目标、方块被破坏、打开界面/断线/reset 时，裂纹立即消失且
  不残留。
- 渲染成本固定有界：单一可复用 overlay（常驻 1 实例容量）、复用方块 atlas
  新增 10 张程序化裂纹层、帧内零分配。

## 非目标

- 不修改服务端采集状态机、采集速度、工具加成、方块硬度、掉落与耐久结算。
- 不修改协议消息、存档 schema、engine ABI 与 client ABI 版本（帧内 TLV
  追加 tag 沿用 client ABI v5 内追加 tag 8 的先例）。
- 不为其他玩家/伙伴的采掘呈现裂纹（网络协议不携带其采掘状态）。
- 不新增粒子或音效；HUD 采掘进度条保持现状（裂纹与进度条并存）。
- 不引入 Mojang 或其他未经授权的美术资源；裂纹纹理为原创程序化像素。

## 用户可观察结果

- 按住采集键：目标方块表面出现浅裂纹，随时间逐级加深为大面积破裂，方块破坏
  的同一帧裂纹消失。
- 采集途中松开键或准星移开：裂纹立即消失；准星移到新方块则裂纹出现在新目标上。
- 从任何方向观察被采掘方块：六面都呈现同一阶段裂纹，被地形遮挡的部分不穿透。
- 打开背包/菜单、断线或 reset 期间不显示裂纹。

## 受影响的包与文档

- `internal/assets`：层枚举末位追加 `LayerCrack0..LayerCrack9`，新增程序化
  裂纹纹理与 cutout 分类、材质包覆盖槽位。
- `internal/render`：新增 `BlockCrack` 呈现输入、进度→阶段映射与实例编码。
- `internal/render/hud`：`MiningOverlay` 补充权威目标字段（HUD 布局不消费）。
- `internal/client`：`RenderFrame` 新增裂纹实例流与帧内 TLV tag 10。
- `cmd/mornlea/app`：从 `hud.MiningOverlay` 派生本帧裂纹流并接线。
- `engine/crates/mornlea_client`：`parse_frame` 新增 tag 10、新增 crack pass
  （新 shader + 新模块）、atlas 上传时重建 bind；`water_tests` pass 计数门禁
  5→6。
- `cmd/mornlea/capture`：新增裂纹视觉场景与 golden 基线。
- `openspec/specs/voxel-visual-presentation`：放宽"恰好一个额外半透明阶段"
  边界为水面之外再允许本裂纹阶段（本 change 的 delta）。

## 兼容性影响

- 无协议、存档、ABI 版本变化；基准版本矩阵不变。
- 裂纹实例流为空时帧字节与现状逐位一致（TLV 段按条件追加，镜像 overlay/water
  的条件段先例），既有 golden 场景不受影响。
- 渲染 pass 起始调用点总数 5→6，受 `water_tests` 源码扫描门禁约束，与本
  change 对 `voxel-visual-presentation` 的 delta 同批落地的。
