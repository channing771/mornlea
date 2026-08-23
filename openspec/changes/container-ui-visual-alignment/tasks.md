## 1. 程序化容器 atlas（后续执行 Task 2）

- [ ] 1.1 重新读取 `proposal.md`、两个 delta spec、`design.md` 和本文件；在 clean checkout 先运行 `make rust`，把 fresh implementer、共享基线与开始状态记入 `ledger.md`。
- [ ] 1.2 在 `internal/render/hud/atlas_test.go` 和按关注点新建的容器 atlas 测试中先写 RED：锁定栏位凹槽、背包/合成标题、箱子标题、熔炉标题、火焰和进度箭头六个固定 cell 的非空、两两互异、二值 alpha、确定性和 UV 严格界内；同时证明既有七个 survival cell 与全部物品列逐像素不变。
- [ ] 1.3 在 `internal/render/hud/atlas.go` 最小追加六个固定 cell 的程序化 painter/UV；火焰与箭头只用固定整数像素，复用既有 atlas 构建与 `hotbarTextureUV`，不新增 PNG、字体 glyph、registry/theme、依赖或资源生命周期。
- [ ] 1.4 运行 `gofmt`、atlas focused、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1` 与 `git diff --check`，记录 RED/GREEN 命令和关键 cell/offset 结果。
- [ ] 1.5 自证后提交候选，生成覆盖候选 parent..HEAD 的 committed review package/raw SHA-256，交给一名 fresh task reviewer 同时裁决 SPEC/QUALITY；finding 仅以追加 fix commit 修复并由同一 reviewer 复审，最多 5 轮，最终结论回填 ledger。

## 2. 三类 overlay 与交互红线（后续执行 Task 3）

- [ ] 2.1 在 `internal/render/hud/container_test.go`、`layout_test.go`、`renderer_test.go` 先写 RED：三类 overlay 各恰好一个标题 quad/零标题 glyph，原创框/凹槽/来源轮廓可判定；熔炉两个填充 quad 分别使用火焰/箭头 UV 与正确裁剪方向；slot/bar/hit-test 坐标不变，panel 只向上扩 20px header，最大打开态为 266，固定 267/700/13312/46912、48-byte instance 与 256-byte 对齐不变。
- [ ] 2.2 在同一关注点测试中锁定换肤前后的半开边界与中心点：`InventorySlotAt` 恰好覆盖 `0..35`、`FurnaceSlotAt` 恰好覆盖 `0..38`、`ChestSlotAt` 恰好覆盖 `0..62`，`RecipeButtonAt` 恰好覆盖现有十条配方；同时在正常、极窄与极矮 framebuffer 锁定 `containerTitleSize=16`、`containerTitleGap=4`、`containerHeaderHeight=20`，证明 `openHUDHeight` 包含同一 header，标题/header 与全部命中矩形不相交。
- [ ] 2.3 在 `cmd/mornlea` 现有 inventory/furnace/chest UI tests 中保留并扩充两次点击见证：第一次零消息且只选来源，第二次恰好一个整堆移动请求，inventory/container 镜像在确认前逐格不变。
- [ ] 2.4 在 `internal/render/hud/container.go`、`layout.go` 与必要的 renderer 接线中原位换 UV/颜色并各追加一个标题 quad；只新增共享 `containerTitleSize`、`containerTitleGap`、`containerHeaderHeight` 三个常量，panel 仅向上扩 header，`openHUDHeight` 增加同一高度，slot/bar/hit-test 几何不变。熔炉仅复用既有四个 bar quad：燃烧填充用火焰 UV 自下向上裁剪，熔炼填充用箭头 UV 自左向右裁剪，实例尺寸与 UV 端点按同一 fraction 变化，不新增组件/资源/pass。
- [ ] 2.5 运行 `gofmt`、HUD container/layout/renderer focused、`go test ./internal/render/hud ./cmd/mornlea -race -count=1`、archcheck、scenario v19 固定上传 focused、`gofmt -l .` 与 `git diff --check`。
- [ ] 2.6 自证后提交候选并生成 committed review package/raw SHA-256；交给 fresh task reviewer 同时裁决 SPEC/QUALITY，finding 追加修复、同一 reviewer 最多复审 5 轮，最终结论与 controller ruling 回填 ledger。

## 3. 两个容器 capture 与 17 张 golden（后续执行 Task 4）

- [ ] 3.1 在 `cmd/mornlea` capture tests 先写 RED：完整顺序恰好为 proposal 中的 17 项，`chest-container`/`furnace-container` 依次紧随 `inventory-crafting`，末三项与双阈值不变；两个夹具必须显式重置并钉住 inventory、互斥 container、来源、相机、时间与所需进度。
- [ ] 3.2 在 `cmd/mornlea/capture.go` 复用现有 `captureScene`/镜像 Apply/Reset 依次注册两个最小场景，不增加产品测试 API；箱子场景呈现 63 格、标题、来源与跨三行内容，熔炉场景呈现 39 格、标题、来源和部分火焰/箭头。
- [ ] 3.3 运行 capture order/fixture focused、`go test ./cmd/mornlea -race -count=1`、HUD race、archcheck 与 `git diff --check`；确认新场景不污染 `debug-panel` 及后续状态。
- [ ] 3.4 在可用 Metal 环境运行 `make visual-update VISUAL_OUT=build/visual-container-ui-visual-alignment-update`，一次重生成全部 17 张正式 golden；两张 far-horizon diagnostic controls 只作既有 guard 输出，不进入正式计数。
- [ ] 3.5 逐张人工复核 17 张正式图并把清单/结论写入 ledger；重点检查三个容器场景的原创框、凹槽、标题、来源、10 配方、熔炉图示、36/39/63 格和末三场景，禁止以 Mojang 像素或放宽阈值通过。
- [ ] 3.6 运行 `make visual-check VISUAL_OUT=build/visual-container-ui-visual-alignment-final`、capture focused、strict OpenSpec 与 `git diff --check`；自证后提交候选并交 fresh task reviewer 双裁决，finding 最多 5 轮追加修复/复审，最终证据回填 ledger。

## 4. 长期基线、全量门禁与整分支终审（后续执行 Task 5）

- [ ] 4.1 实现与 visual 验收完成后才逐字节同步 `AGENTS.md`/`CLAUDE.md` 的当前容器视觉与 17 场景能力，并更新 `docs/notes/progress.md`；不写 change 非目标，不推进任何版本。
- [ ] 4.2 对账 tasks/ledger，只勾选已有 RED/GREEN、focused、task review 与人工 visual 证据的条目；记录全部 implementer/reviewer、commit、finding、修复轮次和 controller ruling。
- [ ] 4.3 运行 `gofmt -l .`、`go test ./internal/render/hud ./cmd/mornlea -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`cmp -s AGENTS.md CLAUDE.md`、`openspec validate --all --strict --no-interactive` 与 `git diff --check`。
- [ ] 4.4 整分支终审前只运行一次 `make rust`、`make rust-check`、`go test ./... -race`、scenario v19 benchmark producer/perfcheck、`make visual-check VISUAL_OUT=build/visual-container-ui-visual-alignment-final-review` 与 strict OpenSpec；性能值只记录，不放宽 correctness、overflow、完整性、I/O、数据丢失或视觉阈值。
- [ ] 4.5 审计完整 committed diff：确认产品范围只含 HUD/capture/长期文档和 17 张正式 golden，无 network/sim/storage/server、协议/存档/ABI/配置/依赖/二进制 UI 源变化；生成完整 review package/raw SHA-256。
- [ ] 4.6 把 committed 整分支 package 交给与 task reviewers 不同的 fresh whole-branch reviewer 做 SPEC/QUALITY 双裁决与 17 图检查；finding 只以追加 fix commit 修复并由同一 reviewer 复审，最多 5 轮。最终双 PASS 后等待 controller 正常 push；未经用户明确批准不得归档。
