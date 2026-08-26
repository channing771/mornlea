# Ledger: farmland-mesh-top-sink

## 认领与确认

- 2026-08-26 认领 D-07（backlog 行 + Discussion #71 评论 `DC_kwDOToJS8M4BFPbc`），分支 `feat/D-07-farmland-mesh-top-sink` 自 main `13912e5c`。
- 阶段 1 内容确认：分类 **bounded**；短设计两处取舍（registry 字节 vs 硬编码 ID；showcase 扩列 vs 无视觉验收）经用户会话内显式「批准」。
- 探索关键事实：耕地碰撞顶 15/16（`internal/physics/types.go:32`）；现有 18 个 capture 场景无一渲染耕地；`visual-verification` 主规格钉死 showcase 夹具集合（故需 MODIFIED delta）；engine crate 零硬编码游戏 ID。

## Ruling 记录

- Ruling: registry 追加 `block_top_raw` 而非 mesher 硬编码 ID —— 数据驱动是 engine 既有纯度（water 经 input、attenuation 经 registry），且为薄雪层等未来半高方块留通道 —— 硬编码会让 mesher 首次持有游戏 ID 常量。
- Ruling: 角赋值用常量而非邻域平均 —— 耕地是刚体、相邻格高度恒等，无斜面可插值；复用 `corner_height` 会把「上方为流体取满格」规则污染到水下耕地上缘 —— fluid_corners 的形状（顶层角取值/底层角为 0）保留，仅数值来源换成 registry 常量。
- Ruling: materials-showcase 扩两列作为唯一视觉验收路径 —— 否则本行为回归网盲区，违反阶段 4 呈现变更须 visual 验收的纪律 —— golden 影响收敛为单景显式再生。
- Ruling: engine ABI v6→v7 与 ENTRY_BYTES 18→19 同 Task 改齐 —— 双侧手工同步常量必须原子落地，容量测试与握手测试兜底 —— 拆开会产生半升级窗口。

## 任务评审

（随执行追加：每 Task 一条 SPEC 合规 + QUALITY 质量双裁决结论，修复循环逐轮记录。）

## Task 1 执行与范围修正

- 2026-08-26 Task 1 由全新 implementer 子代理完成（提交 `a936cff3`）：Go registry/编码/快照校验 + Rust input/mesher/ABI v7 + 耕地几何主题测试 + 跨语言 parity；验证全绿（engine 164 tests、clippy -D warnings、mesh/assets/nativeabi go test、gofmt 无输出）。
- Ruling: 追加 Task 1b（客户端 shader 解码半边）——implementer 发现角高度解码器只在 water.wgsl，terrain.wgsl 把 bit 12..19 当 w/h 尺寸读，耕地 quad 会渲染成巨型石板；控制会话已核实（terrain.wgsl:63-64、water.wgsl:69、cull.wgsl:132）。这是控制会话在 D2 写「客户端不变」时的错误假设，非实现者偏离——按「发现规格不成立先改产物」修正 design（新增 D2a）与 tasks，client ABI 经裁决保持 v8 不动（shader 内部分支无新导出）。
- 备忘：改 mornlea_engine.h 后 cgo 缓存不自动失效，门禁前需 `go clean -cache`（Task 1 实测踩坑）。

### Task 1 双评审与修复循环

- SPEC 评审：PASS。建议两条：(a) ledger 补记 known-red——`TestBaselineVersionsMatchCode` 在 Task 3.1 前必然红（AGENTS.md/CLAUDE.md 尚写 v6），属计划内过渡态；(b) Task 2 golden 再生前必须先做 D4 基线偏差归因（已写入 tasks 2.2）。
- QUALITY 评审：FAIL（1 阻断：quad.rs/quad.go 六处「角高度位=水面专属」注释被本提交证伪；2 建议：叠放回归钉、新注释标识符反引号）。
- 修复轮 R1（原 implementer，`21794e67`）：六处措辞改「带角高度的 quad（流体或短方块）」+ 双类互斥论证；新增 covered/farmland_under_fluid 两测钉住「上方方块/水不拉平到 15」；反引号补齐。验证全绿（engine 166 tests、clippy -D warnings、gofmt 零输出）。
- R1 复核：PASS——diff 纯注释+测试，无行为变更；断言实钉（缺面即 panic、夹具互斥数据核实）；残留「水面」字样均为真实水路径描述非排他主张。
- 结论：Task 1 关闭（SPEC PASS + QUALITY PASS，1 轮修复）。已知过渡态红：archcheck `TestBaselineVersionsMatchCode` 待 Task 3.1 消解。

### Task 1b 执行

- 全新 implementer 子代理交付 `213443e6`：terrain.wgsl 耕地区间角高度路径（29..30 判别、公式与 water.wgsl 同源）、shaders.rs 区间常量、离屏真实 GPU 差分渲染测试 4 条（含巨型石板回归锁与顶缘位移实测）、Go 跨语言层号钉子。
- Ruling: cull.wgsl 以注释+源码扫描钉代替新分支 —— 实现期核实现行剔除只读 face 平面、从不读 bit 12..19，行为已是 D2a 要求的语义；D2a 的「AABB 基于尺寸位」前提系控制会话误判，design 已同步改写 —— 加死分支只会污染热路径。
- 验证：client 103 tests / engine 166 tests / clippy -D warnings 全绿；visual-check 改动前后差异集合逐一相同（18 景既有机型偏差无新增）。

### Task 1b 双评审与修复循环

- SPEC 评审：PASS（5 建议）。QUALITY 评审：FAIL（1 阻断：cull.wgsl 顶面保守性论证方向写反——实际保留集 ⊆ 真实可见集、只漏画不多画；评审同时确认三方钉子双向超配、深度并列陷阱有明文、性能净收益）。
- 修复轮 R1（原 implementer，`82d70e49`）：cull 论证按真实几何改写（含误差带可达性与深下沉边界警告）；sink 测试 doc 改斜率表述；反引号补齐；got→want 对调；打包期断言表述如实化；扫描钉边界说明。验证全绿（client 103 tests、clippy -D warnings、gofmt 零输出）。
- R1 复核：PASS——两份 wgsl 剥离注释后逐位一致（语义零变化），新论证与剔除代码逐句吻合。
- 结论：Task 1b 关闭（SPEC PASS + QUALITY PASS，1 轮修复）。

### Task 1b R2 与 Task 2 双评审

- Task 1b R2（`43eb43b2`）：archcheck 反引号门禁语义为「反引号名必须存在于全仓 Go 声明」（comment_identifier_test.go:202），R1 给 wgsl/Rust 名加反引号必然红——Ruling: 非 Go 标识符一律纯文本、仅 Go 声明用反引号；全大写豁免意味着零保护，一并去除。反引号门禁单独复验 PASS。
- Ruling: 场景表真值修正——main 实际为 18 景（capture_ai_companion_test.go:62 钉死，`main-menu` 在 `water-surface-slope` 与 `far-horizon` 之间）；delta 初稿漏 `main-menu` 已改。连带发现：AGENTS.md「19 个正式场景…settings-menu」描述的 settings-menu capture 只存在于未合入的 D-01 分支（golden 仅在 codex/D-01-settings-menu 等分支），系 main 基线文档既有失真，非本变更引入；Task 3.1 同步基线文档时按「只写已集成事实」一并修正该句并在 progress 记录。
- Task 2 双评审：SPEC PASS + QUALITY PASS（0 阻断）。评审独立复核：golden 新旧逐像素解码比对（72/1700px 精确吻合、1667px 聚集新增列投影区）、红链 overlay 实证、俯角算术定量成立（z=-1 可辨 vs z=-8 亚像素）、湿/干列顶面色可区分。
- 建议（转入 Task 3 清账）：(a) ledger 补逐景机器偏差基线指标；(b) 俯角注释「约 18°」可精确为 17°（16.6–17.8°）——低价值，随顺手改动处理或保留；(c) 越界哨兵覆盖边界已在测试注释说明，无需扩展。

## 附：本机 visual-check 基线差异集合（Task 2 时点，供归因对账）

| 场景 | 最大通道差/差异像素数 |
|---|---|
| terrain-noon / avatar-nametag / inventory-crafting / chest-container / furnace-container | 7/10 @(423,123) |
| debug-panel | 3/6 |
| skylight-tunnel / target-block-feedback / water-surface-slope | 1/7 |
| block-light-room | 1/4 |
| water-underwater | 4/4 |
| oak-grove | 86/56 @(12,99) |
| ai-companion | 1/39 @(490,8) |
| far-horizon | 18/32 @(518,132) |
| materials-showcase | 0/0（Task 2 再生后；此前 1/35 本机偏差随再生消除） |

阈值外超阈景共 10 景；hud-hotbar-health / hud-survival-feedback / main-menu 为 0/0。

## Task 3 执行与收口

- 2026-08-26 Task 3 由全新 implementer 子代理完成（基线文档 engine ABI 同步 + 全量门禁 + mesh 基准记录）。

### 基线文档同步（3.1）

- AGENTS.md 与 CLAUDE.md 经 `cp` 同步后 `cmp` 验证逐字节相同；共三处最小修订，全部围绕「engine ABI v6」表述：
  - M5A 段演进链：`engine ABI v6——v6 新增 mornlea_lod_shell 远环壳出口` 扩为 `engine ABI v7——v7 新增 registry 条目 block_top_raw 一字节承载非满格方块顶面高度（耕地短方块呈现），v6 新增 mornlea_lod_shell 远环壳出口，v5 与 v4 已被 fluid 系列占用`，保持「vN 新增 X」链式句式。
  - 材质包段：原句「沿用……协议 v23、engine ABI v6 与 benchmark scenario v18，未推进这些版本」改为「沿用远环 LOD 合入后的协议 v23 与 benchmark scenario v18，二者未再推进（engine ABI 其后经耕地顶面呈现升到 v7，client ABI 其后经 egui 主菜单变更升到 v8）」。原因：archcheck 对文档中**每一处** `engine ABI vN` 与头文件宏做全量比对（baseline_test.go 用 FindAllStringSubmatch），历史句保留 v6 字样必然红，只能以括注承载升版事实。
  - egui 段契约枚举「benchmark scenario 保持 v19、协议 v26、engine ABI v6、区块 schema v9……不变」删去 `engine ABI v6、` 一项——该段是「egui 交付时未动的契约」历史枚举，改写 v7 反而失真；engine ABI 升版事实已由上一条括注承载。
- 场景计数项核查（brief 预期修正「19 个正式场景 / settings-menu」失真）：**本分支不存在该失真**。D07 基线的两份文档已写「18 个正式场景、`main-menu` 插在 `water-surface-slope` 之后、`far-horizon` 之前」，与 `capture_ai_companion_test.go:62` 钉死的 18 景顺序逐字吻合且全文无 settings-menu（仅 `.claude/settings.json` 一处路径含 settings 字样）；「19 景 + settings-menu」文本只存在于携带未合入 D-01 产物的工作树。判定：无需修改，如实记录。

### 门禁输出摘要（3.2）

- `go clean -cache`：先于门禁执行（Task 1 备忘：mornlea_engine.h ABI 常量变更后 cgo 缓存不自动失效）。
- `scripts/agents/gates.sh` 总退出码 1，六阶段结论：
  1. gofmt 检查 PASS；
  2. `go vet ./...` PASS；
  3. archcheck PASS——`TestBaselineVersionsMatchCode` 同步前红（每份文档三处 v6 vs 代码 7），同步后转绿（`go test ./internal/archcheck -count=1` ok 16.282s，含依赖边界与文档一致性）；
  4. OpenSpec strict PASS（65 passed, 0 failed）；
  5. `make rust` PASS；
  6. 全量 `go test ./... -race` FAIL——唯一失败包 cmd/mornlea 触发包级默认 10m 超时 panic（`TestScenarioV12GPUCompletionBatchIsRecordedInReport` 运行 2m25s 时告警，栈停在 cgo `mornlea_client_render_frame` 真实 GPU 调用），其余 24 个有测试的包全部 ok。
- 分诊复跑（按 `docs/notes/test-quickstart.md` 负载 flake 协议单独重跑失败包，不进修复循环）：`go test ./cmd/mornlea -race -count=1` 单独重跑 **ok（583s ≈ 9m43s）**。判定：**既有负载类等待预算边缘失败**——该包单独运行即贴近 10 分钟包预算上限，全量并发下任何额外负载都会越限；非本次引入（本次 diff 仅两份基线文档与 ledger），未调整任何超时、阈值或 golden。
- `openspec validate --all --strict --no-interactive`：65 passed, 0 failed（与 gates.sh 第 4 阶段同命令同结果）。
- 说明：gates.sh 步骤本身不含视觉门禁（`make visual-check` 不在其六阶段内）；本机既有机型偏差以上文附表为准，本次未重跑视觉、未触碰任何 golden。

### 性能记录（record-only，不设阈值）

- internal/mesh 既有唯一基准 `BenchmarkMeshTerrainSection`（16×16×8 伪随机地形 section 的 `mesh.MeshSection` CPU 网格化，夹具不含耕地、registry 高度字节全零，代表本变更后的常规地形路径）。本机 Apple M2，`go test ./internal/mesh -bench . -benchtime 1x -run '^$' -benchmem`：**2,052,791 ns/op、2016 quads/op、16128 upload_bytes/op、32,992 B/op、4 allocs/op**。单次迭代样本噪声大，仅作后续对比锚点；registry +1 字节与耕地 short 分支对该路径无可测影响预期（分支判别走 material 区间比较）。

### 遗留清零确认

- Task 2 双评审建议 (a) 逐景机器偏差基线指标：已由上文「附：本机 visual-check 基线差异集合」表承载，清零。
- 建议 (b) 俯角注释「约 18°」精确化为 17°：低价值润色，本任务硬约束限定只碰基线文档与 ledger 三文件、代码注释不在其列，按评审原话「随顺手改动处理或保留」选择保留；不构成行为或门禁欠账。
- 建议 (c) 越界哨兵覆盖边界：评审已确认测试注释说明充分，无需扩展，清零。
- tasks 3.1 中「docs/notes/progress.md 补段」按计划留待归档阶段执行，非遗留。
- 除上述外无未决项；proposal 未设「延期与放弃」节亦无内容需誊入。
