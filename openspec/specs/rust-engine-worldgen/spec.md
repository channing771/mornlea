# rust-engine-worldgen Specification

## Purpose

把世界生成(地形分层、矿石、橡树)交由 Rust engine 唯一生产实现,Go 只保留 seed 播种、领域 API 与编码,并以黄金摘要与生产黑盒双出口对照保证同种子世界逐位一致。
## Requirements
### Requirement: 世界生成由 Rust engine 独占生产

高度图地形、地表分层(草/土/石/基岩/雪/沙/黏土/砂砾)、矿石替换、橡树结构与自然短草 MUST
由 Rust engine 独占生产;Go 生产路径 MUST 不包含噪声求值、地形分层、矿石哈希、
树候选或短草分布实现,仓库 MUST 不保留任何旧 Go 实现副本;同种子世界的确定性由黄金摘要
文件与生产黑盒双出口对照把守。

#### Scenario: 同种子区块生成与冻结黄金摘要一致

- GIVEN 黄金文件冻结的种子与区块坐标
- WHEN 调用 worldgen.GenerateChunk 并对全区块 BlockID 序列取 SHA256 摘要
- THEN 该摘要 MUST 与包含自然短草结果的当前黄金文件逐字节一致(internal/worldgen/testdata),
  同种子重复生成 MUST 逐格一致

#### Scenario: 单点查询与整块生成一致

- GIVEN 任意种子、世界坐标 (wx,wz) 与合法 Y
- WHEN 分别调用 HeightAt/TerrainBlockAt/BaseBlockAt 与 GenerateChunk
- THEN BaseBlockAt MUST 与 GenerateChunk 的对应方块逐格一致并包含自然短草,
  HeightAt 与 TerrainBlockAt MUST 保持不含植物的既有地形语义,对照由区块稠密输出与单点查询
  两条生产公共出口互检(internal/worldgen/generator_test.go、tree_test.go、
  parity_test.go)

#### Scenario: 跨区块橡树一致

- GIVEN 一棵候选橡树的树冠横跨相邻两个区块
- WHEN 分别生成这两个区块
- THEN 两个区块内该树的原木与树叶方块拼合后与同种子单点查询语义逐格一致,
  根列树高保持冻结区间 4..6,原木优先、树叶仅覆盖空气的规则保持不变,
  自然短草 MUST NOT 覆盖树干或树叶
  (internal/worldgen/parity_test.go 的跨界树对照与 tree_test.go 的树冠几何性质)

### Requirement: perm 表由 Go 播种并随调用传入

seed→512 项 Perlin perm 表 MUST 继续由 Go `math/rand` 语义计算;engine MUST 只
消费传入的 perm 表,不得内置任何随机源。相同 seed 在迁移前后 MUST 产生相同的
高度、地形材料、矿石与橡树结果;自然短草 MAY 只把满足新生成条件的既有空气格改为
`ShortGrassID`,MUST NOT 改写任何既有非空气方块。

#### Scenario: 相同 seed 迁移前后世界不变

- GIVEN 迁移前某 seed 生成过的任意区块字节
- WHEN 迁移后用同一 seed 再生成同一区块
- THEN 既有高度、地形材料、矿石与橡树 MUST 逐格一致,
  输出差异 MUST 只发生在此前为空气且当前满足自然短草条件的格,
  既有存档加载后 MUST 不回填短草

### Requirement: worldgen ABI 输入校验拒绝

engine worldgen 入口 SHALL 接受 `MGW1` layout `3` 的有效请求，其材料表 MUST 从
`14` 项扩为 `15` 项并包含 `ShortGrassID`，header MUST 从 `564` 字节扩为 `566` 字节，
engine ABI MUST 从 v9 升为 v10。启用 worldgen 注水时 15 个材料 ID MUST 全部互异；关闭
注水时 MUST 只允许 `water == air` 这一项门控别名，其余材料 ID 仍 MUST 互异且不得与
`water`/`air` 重复。入口收到旧 layout `2`、旧长度、错误 magic、非豁免材料 ID 重复、
Y 范围非法或输出缓冲长度不符时 MUST 返回输入错误状态且 MUST 不修改输出缓冲;
Go 侧 MUST 把该状态转换为带稳定中文文案的错误报告,MUST 不产出部分生成的区块。

#### Scenario: 有效 layout 3 请求生成完整输出

- GIVEN magic 为 `MGW1`、layout 为 `3`、header 为 `566` 字节，且材料表在启用注水时 15 项互异、关闭注水时只令 `water == air` 的有效请求
- WHEN 通过 engine ABI v10 调用 worldgen 入口
- THEN engine MUST 接受请求并完整写出确定性的区块结果
- AND 结果中的自然短草 MUST 使用请求材料表中的 `ShortGrassID`
- AND 关闭注水时输出 MUST 不含流体且 `water == air` MUST NOT 被误判为非法重复

#### Scenario: 非法输入被拒绝且输出缓冲不变

- GIVEN 构造的非法 worldgen 请求(如 layout `2`、header `564` 字节、`grass == dirt` 这类非豁免材料重复或输出缓冲长度不符)
- WHEN 调用 engine worldgen 入口
- THEN 返回输入错误状态,输出缓冲保持调用前内容,Go 报告稳定中文错误

### Requirement: 既有世界生成行为规格保持不变

`deterministic-ore-generation`、`deterministic-tree-generation`、
`natural-material-generation` 中描述的全部可观察行为 MUST 继续成立;本变更 MAY
只在该生成器服务的当前 Overworld 新区块中把满足自然短草条件且在橡树、海水及其他既有内容结算后仍为空气的格
写为 `ShortGrassID`,MUST NOT 改变任何既有非空气方块、地形高度或 LOD 表面；`MGW1` 输入
MUST NOT 为此增加维度字段或维度分支。

#### Scenario: 既有行为测试继续通过

- GIVEN 既有矿石、橡树与自然材料测试语料
- WHEN 在加入自然短草后的生产路径上运行这些测试
- THEN 既有矿石、橡树与自然材料断言 MUST 不修改而通过,
  只有明确覆盖自然短草空气层的摘要与夹具 MAY 按新结果更新

#### Scenario: 非短草位置输出保持不变

- GIVEN 当前 worldgen 中相同世界种子与坐标的升级前后生成结果
- WHEN 当前格不同时满足最终 `GrassID` 地表上方空气及短草分布判定
- THEN 当前格的方块输出 MUST 与升级前逐位一致

