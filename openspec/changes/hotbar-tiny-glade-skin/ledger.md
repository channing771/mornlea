# hotbar-tiny-glade-skin Ledger

基线 SHA：`5335c6ae`（main）。

## 阶段 1：内容确认

- Ruling: 需求方在 plan 模式对话中三问三答裁决——对象为底部物品快捷栏、
  范围为纯换肤、图标走内联 SVG — 分类 bounded（既有呈现通道的换肤，
  无新子系统、无协议变更）。显式批准见 ExitPlanMode（用户回复 ok）。
- Ruling: 纯换肤仍触碰主规格 SHALL（选中双层轮廓、数量双层文字、耐久
  背景填充均有点名约束），属行为契约变更——建轻量 change，不走直接
  修改豁免 — 先改规格再编码。

## 任务执行

### Task 1：粉彩换肤实现（TDD）

- 实现：commit `2a468068`（前端 7 文件 + dist 重建：tokens 粉彩加描边
  加选中加几何令牌、hud.css 贴条透明加粉彩圆角加橙选中加深棕字、
  Hotbar `data-index` 加空槽衬底、新建 slotDoodles 九枚线稿、assert
  加 geometry 加 HudRoot 测试同步）。
- 验证证据 @ `2a468068`：先改测试见红（4 项失败、166 通过），实现后
  `typecheck` 干净、`test` 8 文件 170/170、`build` 成功（50 模块）。
- SPEC 评审：**pass**（10/10；贴条透明、逐格粉彩、橙外框抬起、sage
  内衬真删、数量耐久语义未扰、衬底与 tile 互斥）。
- QUALITY 评审：**pass**（令牌纪律、双强调头注、内联 SVG 零裸值、
  无存储网络 API、注释无任务编号、文件集无越界）。
- Ruling: 非阻塞建议一处 — `--hud-select-inset` 令牌与注释（仍写
  sage 内衬）已无 hud.css 消费成孤儿 — 留待后续任务与 `SELECT_INSET`
  常量一并评估退役，不阻塞本 Task。Task 1 关闭。
