## 1. 原创心形与气泡 atlas（对应执行 Task 2）

- [ ] 1.1 重新读取 `proposal.md`、两个 delta spec、`design.md` 和本组任务；在 clean checkout 先运行 `make rust`，并把 implementer 身份与起始 commit 记入 `ledger.md`。
- [ ] 1.2 在 `internal/render/hud/atlas_test.go`、`health_test.go` 与 `oxygen_test.go` 先写失败测试：固定空/半/满心、空/满气泡五列顺序，证明每列非空且互异、alpha 仅 0/255、atlas 连续构建逐字节相等、偏移后的物品列仍逐像素等于 registry 顶面、全部列 UV 不越 16×16 cell；运行 `go test ./internal/render/hud -run 'TestHotbar(TextureAtlas|ColumnUV)' -count=1` 并记录预期失败。
- [ ] 1.3 在 `internal/render/hud/atlas.go` 与 `health.go` 最小实现原创程序化半心/气泡、固定列偏移及 `hotbarHeartUV`/`hotbarBubbleUV` 窄 helper；复用现有 atlas 与 `hotbarTextureUV`，不增加 PNG、sprite registry、主题对象或新依赖。
- [ ] 1.4 运行 `gofmt -w internal/render/hud/atlas.go internal/render/hud/atlas_test.go internal/render/hud/health.go internal/render/hud/health_test.go internal/render/hud/oxygen_test.go`、`go test ./internal/render/hud -race -count=1` 与 `git diff --check`，把命令、退出状态和关键计数写入 `ledger.md`。
- [ ] 1.5 由独立 reviewer 分别完成本组 spec compliance 与 code quality 裁决；发现进入最多 5 轮修复/复审，逐轮记录 finding 与 ruling，全部通过后提交 `git commit -m "feat: add survival HUD atlas icons"`。

## 2. 快捷栏、选中格、耐久与采掘轨道（对应执行 Task 3）

- [ ] 2.1 在 `internal/render/hud/layout_test.go` 与 `renderer_test.go` 先写失败测试：固定关闭态居中九格、面板外阴影/内表面、每格表面、选中格外扩高对比外框与强调内框、数量双层最多两位及耐久显示条件；运行 `go test ./internal/render/hud -run 'Test(ClosedHotbar|HotbarCount|Durability)' -count=1` 并记录预期失败。
- [ ] 2.2 在 `internal/render/hud/layout.go` 增加当前产品所需的包内常量和 `hotbarRowBounds`，让 `inventorySlotOrigin`、状态锚点与采掘轨道复用同一几何；只改关闭态快捷栏绘制，继续复用现有物品、数量和耐久 append 路径，不改变打开态容器视觉或 `InventorySlotAt`。
- [ ] 2.3 在 `layout_test.go` 先覆盖采掘 inactive、零 required ticks、0%、中段、超 100%、可采与不可采，证明可采有移动末端 cap、不可采有固定 warning notch，忽略 RGB 后几何仍不同；运行 `go test ./internal/render/hud -run 'TestMining' -count=1` 并记录预期失败。
- [ ] 2.4 在 `layout.go` 以现有纯色 `hotbarInstance` 最小扩展 `appendMiningBar`：比例钳制到 1，可采追加末端 cap，不可采追加固定 notch；按本组新增实例更新关闭分支容量组成，不增加 shader、贴图 cell 或动画。
- [ ] 2.5 运行 `gofmt -w internal/render/hud/layout.go internal/render/hud/layout_test.go internal/render/hud/renderer_test.go`、`go test ./internal/render/hud -race -count=1` 与 `git diff --check`，记录验证证据。
- [ ] 2.6 由独立 reviewer 分别核对 spec compliance 与 code quality，重点检查 `InventorySlotAt`、打开态容器视觉、权威物品语义和非颜色形状；最多 5 轮修复/复审并记录 ruling，通过后提交 `git commit -m "feat: restyle survival hotbar feedback"`。

## 3. 生命、氧气、响应式布局与固定容量（对应执行 Task 4）

- [ ] 3.1 在 `internal/render/hud/health_test.go` 先写失败测试，覆盖未确认、0/1/2/19/20/越界、零尺寸与打开/关闭态，断言十个空心及半/满覆盖数、关闭态快捷栏左上锚点和打开态快捷栏左下锚点；运行 `go test ./internal/render/hud -run 'TestHealth' -count=1` 并记录预期失败。
- [ ] 3.2 在 `internal/render/hud/health.go` 最小重写生命布局：删除未使用 atlas 参数，钳制到 `core.MaxHealth`，使用完整空/半/满 cell，并仅依赖 `hotbarRowBounds`、`open` 和 framebuffer 尺寸。
- [ ] 3.3 在 `internal/render/hud/oxygen_test.go` 先写失败测试，覆盖未确认、满值、0、1 tick、全部分段边界、`core.MaxOxygenTicks-1`、越界、零尺寸与打开/关闭态；断言满氧零实例、耗氧十空槽加整数向上取整覆盖，以及快捷栏右侧上下锚点；运行 `go test ./internal/render/hud -run 'TestOxygen' -count=1` 并记录预期失败。
- [ ] 3.4 在 `internal/render/hud/oxygen.go` 最小改为 atlas 气泡：钳制权威值，保留未确认/满氧隐藏，用纯整数公式计算十段覆盖，不再绘制纯色比例横条。
- [ ] 3.5 在 `internal/render/hud/renderer.go` 只把已有 `open` 传给生命/氧气布局；在 `layout.go` 扩展关闭态联合缩放边界，使状态行位于快捷栏上方、采掘位于状态行上方，打开态继续使用既有 `openHUDHeight`，零尺寸不生成实例。
- [ ] 3.6 在 `layout_test.go` 与 `renderer_test.go` 覆盖 1280×720、640×360、窄窗口和零尺寸的打开/关闭态；遍历全部 rectangle 验证 framebuffer 内，证明打开态生命/氧气不与 36 个 `InventorySlotAt` 可命中格相交，且改动前后边界命中样本语义一致。
- [ ] 3.7 在 `layout.go`/`renderer_test.go` 分别计算关闭与打开互斥分支上限，令 `healthQuads == 20`、`oxygenQuads == 20`，以两个合法最大组合见证 `maxHotbarQuads`；保留 `internal/render/hud/encode.go` 不变，并证明 48-byte 编码、256-byte offsets、区间和实际前缀契约。
- [ ] 3.8 运行 `gofmt -w internal/render/hud/health.go internal/render/hud/health_test.go internal/render/hud/oxygen.go internal/render/hud/oxygen_test.go internal/render/hud/layout.go internal/render/hud/layout_test.go internal/render/hud/renderer.go internal/render/hud/renderer_test.go`、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1` 与 `git diff --check`，记录验证证据。
- [ ] 3.9 由独立 reviewer 分别核对 spec compliance 与 code quality，重点检查权威值来源、满氧隐藏、打开态可见性/避让、窄窗口、固定容量和编码不变；最多 5 轮修复/复审并记录 ruling，通过后提交 `git commit -m "feat: align survival status around hotbar"`。

## 4. 确定性 survival feedback capture 与 golden（对应执行 Task 5）

- [ ] 4.1 在新建 `cmd/mornlea/capture_hud_test.go` 和既有 capture tests 先写失败测试：固定完整 15 场景顺序、`hud-survival-feedback` 紧随 `hud-hotbar-health`、`far-horizon` 倒数第二、`water-underwater` 唯一末尾，以及 HUD 夹具值和一次/重复 restore 后 predictor 指针与 mining overlay 恢复；运行 `go test ./cmd/mornlea -run 'Test(HUDCapture|CaptureSceneOrder|WaterUnderwater|FarHorizon)' -count=1` 并记录预期失败。
- [ ] 4.2 在 `cmd/mornlea/capture.go` 增加最小 `captureHUDFixture` 和 `captureScene.HUD`：保存原 predictor/mining，复用原有限物理状态构造固定 Ready/Overworld predictor，钉住 health/oxygen/yaw/pitch/mining，并返回幂等 restore；不修改产品 `Predictor` API或 `renderFrame` 热路径。
- [ ] 4.3 在 scene Apply 后、收敛帧前应用夹具并对所有后续返回路径 `defer restore()`；注册固定低血、`core.MaxOxygenTicks/3`、磨损工具、不可采 `4/9` 进度的 `hud-survival-feedback`，删除旧的部分心形 capture 限制注释，并更新既有顺序测试。
- [ ] 4.4 运行 `gofmt -w cmd/mornlea/capture.go cmd/mornlea/capture_ai_companion_test.go cmd/mornlea/capture_hud_test.go`、`go test ./cmd/mornlea -run 'Test(HUDCapture|Capture.*Scene|WaterUnderwater|FarHorizon)' -count=1`、`go test ./cmd/mornlea -race -count=1` 与 `git diff --check`。
- [ ] 4.5 在可用 Metal 环境运行 `make visual-update VISUAL_OUT=build/visual-bedrock-survival-hud-update`；确认场景共 15 张、新增 `hud-survival-feedback.png`，且只提交实际受影响的 `cmd/mornlea/testdata/golden` 文件，不移动场景尾序、不放宽双阈值。
- [ ] 4.6 人工逐图复核全部 15 张候选并把文件清单与结论写入 `ledger.md`；重点确认 `hud-hotbar-health` 的九格/双层选中/数量/耐久/满血/满氧隐藏，`hud-survival-feedback` 的低血/气泡/磨损工具/不可采中段形状，以及 `inventory-crafting` 的状态行避让；任何世界、实体、光照、水或 LOD 区域异常先修根因。
- [ ] 4.7 运行 `make visual-check VISUAL_OUT=build/visual-bedrock-survival-hud-final`、`go test ./cmd/mornlea -run 'Test(HUDCapture|Capture.*Scene|WaterUnderwater|FarHorizon)' -count=1` 与 `git diff --check`。
- [ ] 4.8 由独立 reviewer 分别核对 spec compliance 与 code quality，重点检查 fixture 仅限 capture、错误路径恢复、场景隔离、完整顺序与阈值；最多 5 轮修复/复审并记录 ruling，通过后提交 `git commit -m "test: lock survival HUD presentation"`。

## 5. 长期基线、全量验证与整分支终审（对应执行 Task 6）

- [ ] 5.1 仅在实现与 visual 验收完成后逐字节同步 `AGENTS.md`/`CLAUDE.md` 的当前能力描述，并在 `docs/notes/progress.md` 记录里程碑；不把 change 非目标写入长期基线，不推进协议 v23、engine ABI v6、client ABI v7、benchmark scenario v18 或任何存档 schema。
- [ ] 5.2 对账 `tasks.md` 与 `ledger.md`：只勾选已有实现、focused 验证、spec review 和 quality review 证据的条目，记录所有 implementer/reviewer、commit、finding、修复轮次、人工 visual 结论与 controller ruling。
- [ ] 5.3 运行 `gofmt -l .`、`go test ./internal/render/hud ./cmd/mornlea -race -count=1`、`go test ./internal/archcheck -count=1`、`cmp -s AGENTS.md CLAUDE.md` 与 `git diff --check`；`gofmt -l .` 必须无输出，其余命令必须退出 0。
- [ ] 5.4 在整分支终审前仅运行一次 `make rust`、`make rust-check`、`go test ./... -race`、`go vet ./...`、`make visual-check VISUAL_OUT=build/visual-bedrock-survival-hud-final-review` 与 `openspec validate --all --strict --no-interactive`；不得放宽 correctness、overflow、完整性、I/O、数据丢失或视觉阈值门禁。
- [ ] 5.5 用 `git diff --name-only "$(git merge-base HEAD main)"..HEAD`、`git diff --stat "$(git merge-base HEAD main)"..HEAD` 与 `git status --short` 复核范围；确认没有产品改动落入 `engine/`、`internal/network`、`internal/sim`、`internal/storage` 或 `cmd/mornlea-server`，没有 PNG UI 源资产、第三方依赖、配置键、协议字段或 ABI 变化。
- [ ] 5.6 由新的独立 reviewer 对 proposal/spec/design/tasks 与完整分支 diff 做最终 spec compliance 和 code quality 终审；发现回到对应任务组进入最多 5 轮修复/复审，最终结论、验证证据和所有 ruling 写入 `ledger.md`。
- [ ] 5.7 提交 `git commit -m "docs: record survival HUD milestone"` 后确认工作区仅剩已知无关改动；不得自行归档，取得用户明确批准后才运行 `openspec-archive-change`。
