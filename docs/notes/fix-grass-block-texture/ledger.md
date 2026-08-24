# 草方块侧面纹理修复 · 执行台账

| 轮次 | 阶段 | implementer | spec review | quality review | ruling |
|---|---|---|---|---|---|
| 1 | 诊断 | 控制会话（本会话） | - | - | 根因：内嵌 Pixel Perfection `grass_side.png` 是 Minetest overlay（row8-15 全透明、row5-7 半透明），`terrain.wgsl` 对 a<0.5 片段 discard，`LayerGrassSide` 为不透明层无合成 → 草方块侧面下半部穿透。修复方向：asset 侧合成不透明图 + provenance/署名 + 守卫测试 + golden 重生成；不动 shader/分类/loader 契约（Ruling 1）。 |
## 实现轮次（R1）

- Implementer 子代理（$TASK1_IMPL）完成实现但被 600s 运行上限中断，未及写报告；改动全部在工作树。改动范围（控制器核对）：
  `textures/grass_side.png`（换为合成不透明图，SHA 57e11b04…）、`PROVENANCE.json`（grass_side 条目新增 `derived`，sha256 更新）、
  `ATTRIBUTION.md`（派生例外段）、`default_pack_test.go`（+68 行：provenance derived 断言 + `TestOpaqueLayersAreFullyOpaque` 守卫）、
  `docs/texture-packs.md`（+7 行 alpha 契约）、`docs/notes/progress.md`（+2 行）、`scripts/composite_grass_side.go`（新建）、12 张 golden 重生成。
  未触碰 shader/mesh/分类/loader/协议/存档/ABI（diff --stat 核实）；`AGENTS.md`/`CLAUDE.md`/`docs/superpowers/`/`openspec/` 未动。

## 控制器验证证据

- 脚本确定性：`go run scripts/composite_grass_side.go` 连跑 3 次，`grass_side.png` SHA 恒为 57e11b043af8a3c262c2cbfbe6ccb63ea09cfa5cf0d392c2aea78de64de02b5e，与 `PROVENANCE.json` 记录一致（GOCACHE 需设在本工作区内）。
- 新纹理：16×16、alpha!=255 像素 = 0（全不透明）。
- `go test ./internal/assets -count=1` → ok（0.502s）；`gofmt -l …` 无输出；`go test ./internal/archcheck -count=1` → ok（13.2s）；`go vet ./...` → 无输出。
- `go test ./cmd/mornlea -run 'Capture|Scene|NearBand|Compare|WaterUnderwater|OakGrove|MaterialShowcase' -count=1` → ok。
- `make visual-check`（fresh render vs 新 golden）：17/17 场景，每场最大通道差 0、差异像素 0/230400，exit 0。
## 独立任务评审（评审者 405d66bb-678a-4036-aa4c-4a790b5984e8）

- SPEC: PASS（方案 1–7 全部落实；无硬约束违背；PROVENANCE sha256 与资产一致；脚本 /tmp 副本连跑 2 次逐字节一致；
  golden 变化集说明：HUD/容器 5 张也含草侧世界背景（~4% 同 terrain-noon），洞穴类、仅顶面可见类 0% 不变，符合判据）。
- QUALITY: PASS，发现 1×P2（Q1 文档函数名拼写 downSampleCutout→downsampleCutout）+ 4×P3（Q2 注释反引号、Q3 Minetext 拼写、Q4 双重 close、Q5 命名不一致）。

## 修复轮 R2（仅文本级，控制会话按"小型拼写修复/纯格式修改"豁免直接应用）

- Q1：docs/texture-packs.md 函数名更正为 `downsampleCutout`。
- Q2：default_pack_test.go 与脚本注释的 Go 标识符补反引号。
- Q3：脚本注释 Minetext → Minetest。
- Q5：progress.md 条目名统一为 `fix-grass-block-texture`。
- Q4 **Ruling：不改** —— `defer file.Close()`+显式 `return file.Close()` 是标准惯用法：二次 close 错误被 defer 丢弃、显式 close 保留错误传播，无泄漏路径（一次性 CLI）。改掉反而丢失关闭错误检查。Cost if wrong: 无——语义与现状等价，仅少一个无关紧要的二次调用。
- 修复轮验证：gofmt 干净；`go test ./internal/assets -count=1` ok（0.574s）；`go test ./internal/archcheck -count=1` ok（2.622s）。
- Scoped 复审：已请原评审者复核上述改动（进行中）。
## Scoped 复审（评审者 405d66bb-678a-4036-aa4c-4a790b5984e8）

- 裁决 PASS：Q1/Q2/Q3/Q5 已按说明落实；Q4 裁决成立（不改）；S1 观察项由控制器 visual-check 证据作证。
- 评审者遗留 P3 备注（`default_pack_test.go:376` `waterAlpha` 缺反引号）——控制器顺手修正并复验（gofmt 干净、`go test ./internal/assets` ok 0.346s）。
- 终审结论：无 P1/P2 未决项；SPEC 与 QUALITY 双 PASS；可交付。

## 控制会话终审（整分支）

- 交付范围：1 个合成资产 + 可复现脚本 + provenance/署名 + 守卫测试 + 2 份文档 + 12 张 golden。
- 验证全绿：`make rust`（0.14s 增量）、`go test ./internal/assets -count=1`、`go test ./cmd/mornlea -run 'Capture|Scene|NearBand|Compare|WaterUnderwater|OakGrove|MaterialShowcase'`、
  `go test ./cmd/mornlea -count=1`（491.8s）、`go test ./... -race -count=1`（全包 ok、exit 0）、`go vet ./...`（无输出）、
  `go test ./internal/archcheck -count=1`（含注释标识符门禁）、`gofmt -l .`（无输出）、`make visual-check`（17/17 场景、每场最大通道差 0、差异像素 0/230400、exit 0）。
- 未动：shader/mesh/层分类/`applyPack` 语义/协议/存档/engine ABI/client ABI/benchmark scenario；`AGENTS.md`/`CLAUDE.md`/`docs/superpowers/`/`openspec/`。
- 工作树未提交（main 分支）；改动清单见最终答复。
## 第二轮:侧脸纹理朝向 bug(用户复报"草方块侧面显示异常")

- 上一轮(alpha 合成)后用户仍报侧脸异常。控制会话复核发现**真正的**侧脸 bug:
  `terrain.wgsl` `face_uv` 对 ±X 面用 `(world.y, world.z)`(u=纵向→纹理旋转 90°)、±Z 面用 `(world.x, world.y)`
  (`v=world.y`,wgpu v=0 为图像顶行→纹理上下颠倒),植物分支 `(world.x, world.y)` 同样颠倒。
  模拟公式输出:+Z 面"上 8 行泥土、下 8 行草"、+X 面"左侧绿色竖条"——与设计意图(草带在侧面顶部)相反。
- 意图证据:`voxel-visual-presentation` 规格「草缘…下垂像素」「原木侧面 MUST 显示纵向树皮」;程序化测试断言草带在纹理顶行。
- 影响:所有方向性侧面材质(草侧、熔炉正面、箱锁、原木 ±X 树皮、雪侧)与小麦交叉斜面;同源公式在 terrain/lod/water 三处。
- **Ruling 2**:修复选 shader 侧统一 `-world.y`(v 分量取负,采样器 Repeat 环绕安全,仍满足"采样相位由世界坐标决定/连续"规格),
  不改纹理数据(纹理按"图像顶行=方块顶部"创作;反向改纹理会破坏 wrap/结构测试与全部其它方向性材质的语义)。
  Cost if wrong: 世界坐标 UV 相位仍由世界坐标确定,仅符号翻转,连续性论断不变;远端/近端正交接缝由同一公式保证。
- 上一轮的 alpha 合成修复保留(不透明化是朝向正确显示的前提)。
- 派发 R2 implementer(458c648f-1d73-47c1-8752-de5010b5a590,后台):shader×3 + 植物分支 + 离屏语义守护测试 + golden 重生成。
## R2 实现完成（implementer 458c648f-1d73-47c1-8752-de5010b5a590）

- DONE。shader×3（terrain/lod/water 的 face_uv 纵向取 `-world.y`、植物分支同步）+ 新 `side_tests.rs`（5 用例：+Z/+X/植物/−Z 红上绿下 + 空场景守卫）+ progress.md + 17 张 golden 全部重生成。
- 变异验证（实现者）：临时退回旧公式 → 4/5 失败（±Z/植物 r_up=0 g_up=128 上下颠倒；+X r_up=64 g_up=64 旋转均分）→ 证明测试有牙齿；恢复后 visual-check 17/17、每场景 0/230400、exit 0。
- R2 评审：已派原评审者（405d66bb…）独立复核（进行中）。
- 控制器并行终审：全仓 `go test ./... -race` 与 `make rust-check` 后台运行中。
## R2 评审（评审者 405d66bb…）

- SPEC PASS、QUALITY PASS：无 P1/P2；3×P3（Q1 helper 复制可未来集中、Q2 夹具设计说明、Q3 ±X/±Z 对面镜像为既有性质）。
- 独立运行：side_tests 5/5、mornlea_client 全量 54/54、go test ./cmd/mornlea capture/nearband ok、assets ok、fmt/clippy 干净。
- 无需修复轮。R1+R2 进入终审。

## 控制器终审（整分支）

- 最终态验证全绿：`cargo fmt --check`+`clippy -D warnings`+`cargo test --workspace --locked`（exit 0，含 side_tests 5/5 真实 GPU）、
  `go test ./... -race -count=1`（全包 ok、exit 0）、`go vet ./...`（无输出）、`go test ./internal/archcheck`（ok）、
  `go test ./internal/assets`（ok）、`go test ./cmd/mornlea -run Capture|Scene|NearBand|Compare`（ok）、`gofmt -l .`（无输出）。
- 视觉：implementer `make visual-update`（LOD on/off 近环 control 通过）→ `make visual-check` 17/17、每场 0/230400、exit 0；
  控制器最终再跑一次 visual-check（进行中）。
- 交付面（R1+R2）：不透明 grass_side 合成资产 + 可复现脚本 + provenance/署名 + alpha 守卫测试；shader 侧脸朝向修正 ×3 + 植物分支 +
  side_tests 语义守护；17 张 golden 重生成；docs（texture-packs.md、progress.md ×2 条目）。
- 未动：mesh/层分类/`applyPack`/协议/存档/engine ABI/client ABI/benchmark scenario；`AGENTS.md`/`CLAUDE.md`/`docs/superpowers/`/`openspec/`。
