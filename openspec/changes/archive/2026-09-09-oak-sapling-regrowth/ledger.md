# B-33 树苗与橡树再生（SDD ledger 摘要）

> 完整逐轮记录见 SDD 工作区 `.superpowers/sdd/oak-sapling-regrowth/progress.md`（分支交付时随工作区保留在开发机）。本文件是归档留档的浓缩版。

## 任务完成

| Task | 内容 | 结论 |
|---|---|---|
| T1 | 树苗编号与语义（`core`） | complete（`544cbd22..23ae7916`，review clean） |
| T2 | Rust 树形 ABI 与版本矩阵（engine ABI v11） | complete（`23ae7916..2df870b1`，review clean） |
| T3 | 种植、采掘与掉落 + 流体镜像 | complete（`bc01e4db..854ca1b5`，1 轮修复） |
| T4 | 随机 tick 生长与环境移除 | complete（`ee72f64a..a97ef862`，review clean） |
| T5 | 客户端呈现 | complete（`146a7960..a887c3b4`，review clean） |
| T6 | capture 场景与 golden | complete（`a887c3b4..0d7b9b38`，review clean） |
| T7 | 伙伴边界 | complete（`0d7b9b38..075b894c`，review clean） |
| T8 | 收尾门禁与基线文档 | complete（`4516d016`，7 项门禁全绿） |
| 终审 | 整分支终审（Merge GO） | complete + 一次修复波（`fa582953`、`43b1aa96`），scoped 复审 5/5 ADDRESSED |

## 关键裁决

- 树形几何由新增 engine ABI `mornlea_tree_blocks`（v10→v11）在 Rust 单一真源计算，Go 只做空间校验与原子写入（用户批准）。
- 生长判定用固定 `1/8` 常量而非新 tunable（契约是固定确定性速率）。
- 生长只取普通橡树家族（高 5..7、半径 ≤2、无分杈/珍异），根坐标 Y 由调用方前置校验（引擎对 `root_y > 311` 硬拒，Go 桥 panic）。
- 树苗自身格是树干底格，不参与占用校验；其余几何格必须 `AirID`/`ShortGrassID`。
- 树叶→树苗判定只作用于玩家采掘；伙伴采掘树叶保持单一 `ItemLeaves`。
- 耐久豁免对玩家与伙伴同样成立（T3 修复轮把伙伴结算接入共享豁免入口）。
- 树苗物品图标经 `blockItemTexture` 回退（透明源像素为黑，与玻璃同路径），不新增图标层。
- benchmark scenario 保持 v22：注册表追加不改变固定 benchmark 世界内容，沿用雪层先例；新增 `bounded-benchmark-workload` 要求固化该口径。
- 工作区目录改用 `.superpowers/sdd/oak-sapling-regrowth/`，避开被 git 跟踪的另一计划 `tasks/` 目录。
- 版本矩阵：engine ABI v10→v11 是唯一升版；协议 v38、player v8、chunk v9、metadata v4、`companions.ai` v5、`hostile_mobs` v1、`passive_mobs` v1、client ABI v18、benchmark scenario v22 与既有 golden 均不变。

## 延期与放弃

见 `proposal.md`「延期与放弃」节；全部为 Minor/不可达/独立后续行，不阻塞归档。
