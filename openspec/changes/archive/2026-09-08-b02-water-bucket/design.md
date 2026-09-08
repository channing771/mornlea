# B-02 可搬运水源（design，如实记录）

> 脑风暴输入：`docs/superpowers/specs/2026-09-08-b02-bucket-design.md`；实现计划：`docs/superpowers/plans/2026-09-08-b02-bucket.md`；分支 `feat/B-02-water-bucket`（PR #166，11 commits + 2 修复波）。

## 数据所有权与依赖方向

- 权威全部在 Go：`core` 只加编号与谓词；`fluid` 加无限水纯函数；`sim/entity/bucket.go` 做取/放原子事务；`network` 加 2 kind。Rust 只改 `fluid_eval` 求值核（布局 v1 不动，ABI 不升），Go oracle 同字同步，差分门禁兜底。
- `ItemPlacement` 永不映射流体（`TestNoItemPlacesAsFluid` 意图保留，改写为「仅专用命令可写源」）；取水用新 `CollectTarget`（仅源），放水复用穿水命中 + 贴面落点（`fluid-survival` 既有语义）。
- 命令分发落在 `sim` tick 两段式（`command.go` 仅别名，无分发点）；桶成功当 tick 抑制采掘（`bucketSuppressedMining` 置位—消费自清）。
- 湿度联动复用既有同 tick 重判：取（源→空气）无条件入队；放按流体成员翻转条件入队（与 `executePlacement` 同式）；流动→源升级不入队（成员未变）。

## 并发与预算

- 无限水在 `eval_one` 内排序为消亡 → 垂直 → 无限 → 水平；输出仍是定长 12B 候选写，队列、预算、全序与 `strongerWrite` 合并语义不变。
- 取/放写入经既有 `recordChange` 汇入同一批变更，不新增协议消息；`PlaceBlockSucceeded` 序列号复用于桶成功 cue（严格递增门）。

## 被否决的替代方案

- 单 `UseBucket` 上下文命令：省 1 kind 但分叉 4 结果，拒绝语义模糊——否决。
- 复用 `PlaceBlock` + `ItemPlacement(水桶)=WaterSource`：混入固体校验，取水仍需新通道，且破坏现有守护意图——否决。

## 风险与回退

- 平衡态定义变化（双源夹缝新增源）：旧世界经重扫收敛；回退即 revert 本分支（ID 全部 append-only，无迁移）。
- 客户端图集加列后移人物层：Go/Rust 层号与 shader 断言已同步（T7 修复轮），`avatar-detail` 在双阈值门内。

## 验证方法

- `go test ./packages/shared/... ./packages/server/... ./packages/client/... -race`、audit、visual-check（`bucket-pond` 新增，既有零漂）、fuzz 8M execs、差分 17/17、golden 17/17、`openspec validate --all --strict`、CI 9/9（PR #166）。
