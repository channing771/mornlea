## ADDED Requirements

### Requirement: 追加材质层不改变固定 benchmark 工作负载

本变更在稳定编号上只追加一对树苗编号（`SaplingID` 与 `ItemSapling`）并把树苗材质层加入植物判定集合，benchmark scenario MUST 保持 `v22`：固定 benchmark 世界 MUST 不含任何 `SaplingID`，被测世界的逐格内容与网格输出 MUST 与追加前逐格一致，离屏目标、阶段时长、运动、样本、指标、绝对阈值与 `20%` 相对回归阈值 MUST 全部保持不变，当前唯一可授权的跨场景迁移 MUST 仍只有 `21:22`。被测进程的植物材质判定集合现为 `[31..54] ∪ {68, 164}`（`55..67` MUST NOT 移动），engine ABI 现为 `v11`，mesh registry 的烘焙条目数由 `89` 增至 `90`；这些只改变被测进程的材质集合、ABI 版本与固定长度 registry 快照的条目数，MUST NOT 让任何新方块出现在固定 benchmark 世界，也 MUST NOT 单独构成升版理由。只追加编号而不改变固定 benchmark 世界方块的登记（雪层编号是既有先例）MUST NOT 单独触发场景升版；`v18` 关于 registry 条目数与 FFI 输入变长的记述是**该次**变更的历史升版理由（同批变更还移动了 Hotbar HUD 固定上传布局并新增了每 tick 的作物阶段），MUST NOT 被当作「每次追加编号都必须升版」的常设规则。

#### Scenario: v22 固定世界不含树苗且场景与迁移不变

- **GIVEN** scenario `v22` 的固定 benchmark 世界与当前被测进程
- **WHEN** producer 生成报告、比较器读取该报告
- **THEN** 被测世界 MUST 不含任何 `SaplingID`
- **AND** 报告的场景版本 MUST 保持 `v22`，`2560×1440` 离屏目标、阶段时长、样本、指标、绝对阈值与 `20%` 相对回归阈值 MUST 与 v22 现状逐项相同
- **AND** 比较器 MUST 仍只接受唯一的 `21:22` 跨场景迁移，其它迁移参数 MUST 继续失败

#### Scenario: 追加编号与材质层只改变进程侧事实

- **GIVEN** 当前被测进程的植物材质判定集合、engine ABI 版本与 mesh registry 烘焙条目数
- **WHEN** mesher 与 benchmark 分别求值植物材质判定与 registry 快照
- **THEN** 植物材质判定集合 MUST 为 `[31..54] ∪ {68, 164}` 且 `55..67` MUST NOT 移动
- **AND** engine ABI MUST 为 `v11`，烘焙条目数 MUST 为 `90`，固定长度 registry 上限 MUST 保持不变
- **AND** 该追加 MUST NOT 单独触发 scenario 升版：固定 benchmark 世界出现的方块 MUST 与追加前逐格一致，网格输出与阈值 MUST 不变
