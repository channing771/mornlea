## MODIFIED Requirements

### Requirement: 世界生成由 Rust engine 独占生产

高度图地形、地表分层(草/土/石/基岩/雪/沙/黏土/砂砾)、矿石替换与橡树结构 MUST
由 Rust engine 独占生产;Go 生产路径 MUST 不包含噪声求值、地形分层、矿石哈希或
树候选实现,仓库 MUST 不保留任何旧 Go 实现副本;同种子世界的确定性由黄金摘要
文件与生产黑盒双出口对照把守。

#### Scenario: 同种子区块与 Go oracle 逐位一致

- GIVEN 黄金文件冻结的种子与区块坐标
- WHEN 调用 worldgen.GenerateChunk 并对全区块 BlockID 序列取 SHA256 摘要
- THEN 该摘要 MUST 与提交入库的黄金文件逐字节一致(internal/worldgen/testdata),
  同种子重复生成 MUST 逐格一致

#### Scenario: 单点查询与整块生成一致

- GIVEN 任意种子、世界坐标 (wx,wz) 与合法 Y
- WHEN 分别调用 HeightAt/TerrainBlockAt/BaseBlockAt 与 GenerateChunk
- THEN 单点结果与该坐标所在生成区块的对应方块一致,对照由区块稠密输出与单点查询
  两条生产公共出口互检(internal/worldgen/tree_test.go、parity_test.go)

#### Scenario: 跨区块橡树一致

- GIVEN 一棵候选橡树的树冠横跨相邻两个区块
- WHEN 分别生成这两个区块
- THEN 两个区块内该树的原木与树叶方块拼合后与同种子单点查询语义逐格一致,
  根列树高保持冻结区间,原木优先、树叶仅覆盖空气的规则保持不变
  (internal/worldgen/parity_test.go 的跨界树对照与 tree_test.go 的树冠几何性质)
