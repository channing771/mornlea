# Tasks: 客户端 UI 对齐原版布局与风格精修

> 每组任务由 fresh implementer 实现，接受独立 SPEC 与 QUALITY 双评审；裁决与证据记入 `ledger.md`。控制会话不直接实现。

## 1. 生存 HUD 布局对齐 + 准星 + 物品名弹条

- [ ] 1.1 新增 `internal/render/hud/style.go` 令牌表（design D1），既有散落色值迁入并统一命名；行为不变。
- [ ] 1.2 `hotbarBottomMargin` 24→6；`closedHUDHeight` 按设计更新（含弹条行）；验证既有锚定/缩放/避让测试全绿。
- [ ] 1.3 准星：viewport 中心十字（投影+前景 4 quad，实例最先追加），菜单相位与零尺寸不产生；TDD 先红后绿。
- [ ] 1.4 物品名弹条：`PopupOverlay` 组装（`app_frame.go`）、40 权威 tick 窗口、容器/菜单抑制、确认镜像触发、glyph 双层居中；TDD 覆盖触发/过期/抑制/共存不遮挡。
- [ ] 1.5 `core.ItemDisplayName`（design D7）：方块回退 + 非方块显式表，全量覆盖测试。
- [ ] 1.6 预算重钉：`maxHotbarQuads` 320、`maxHotbarGlyphs` 768、关闭最坏恰 100 的固定预算测试；`layout.go` 注释 scenario v20 口径。
- [ ] 1.7 delta：`survival-hud-presentation` MODIFIED×2 + ADDED×2 与实现一致。
- 验证：`go test ./internal/render/... ./internal/core -race -count=1`、`go test ./internal/archcheck -count=1`。

## 2. HUD 视觉精修

- [ ] 2.1 心/半心/空心、气泡、鸡腿、火焰、箭头、凹槽、标题 cell mask 就地重绘（design D6）：不增列、不动 UV 收进；图集列采样稳定性既有测试保持全绿。
- [ ] 2.2 面板/选中/进度/文字统一套用令牌：投影+表面+1px 亮边三段面板；数字与文字双层规范统一。
- [ ] 2.3 saturation-jitter、数量阴影、耐久、采掘/进食互斥既有语义测试全绿；新增调色板令牌断言测试。
- 验证：`go test ./internal/render/... -race -count=1`。

## 3. 容器原版式浮动面板 + tooltip

- [ ] 3.1 `panelGeometry` 单源推导：四类面板绘制与全部命中测试（36/39/63 格、产物格、十条配方）共用面板矩形；穷举几何测试（面板与状态栈/快捷栏/进度条互不相交、全部界内）。
- [ ] 3.2 四类面板布局：个人背包（2×2+箭头+产物 + 右侧十条配方栏 + 3×9 + 快捷栏行）、工作台（3×3 图式）、熔炉（输入上/火焰中/燃料下 → 箭头 → 输出）、箱子（27 格）；标题 quad 各一。
- [ ] 3.3 tooltip：悬停非空栏位（含产物格）显示中文名（与弹条同源），指针右下优先、越界翻转、空格/面板外零实例；TDD。
- [ ] 3.4 打开最坏恰 274 预算测试；`openHUDHeight` 语义更新为含面板高度的缩放约束；delta：`container-ui-presentation` MODIFIED×3 + ADDED×1 与实现一致。
- 验证：`go test ./internal/render/... ./cmd/mornlea -race -count=1`（capture 包编译级）。

## 4. egui 菜单换肤

- [ ] 4.1 新增 `engine/crates/mornlea_client/src/ui/style.rs` 令牌（design D5）并接入主菜单/暂停/设置/F3；wire layout v1–v4、动作常量、事件编码零改动。
- [ ] 4.2 Rust 测试同步：既有颜色/像素断言更新为新令牌；新增令牌集中断言；`menu/pause/settings/debug` 全部 abi/render 测试绿。
- 验证：`cargo test -p mornlea_client`、`cargo build -p mornlea_client --release` 后回拷 dylib（共享 CARGO_TARGET_DIR 三件套），`go test ./internal/client ./cmd/mornlea -race -count=1`（nativeabi 路径）。

## 5. capture 场景 + golden 重生成

- [ ] 5.1 新增 `hud-item-name-popup` 场景（`hud-survival-feedback` 后）：确认选中 + 固定 tick 夹具、场景后恢复；场景顺序测试常量更新。
- [ ] 5.2 受影响场景夹具适配（准星/弹条/面板/tooltip 可控）；benchmark scenario v19→v20 全链（常量、测试、README×2、compatibility、architecture 如有）。
- [ ] 5.3 `make visual-update` 生成 24 张 golden，逐图人工复核（准星、弹条、面板、tooltip、菜单新样式）后提交；`make visual-check` 绿。
- [ ] 5.4 delta：`visual-verification` ADDED「选中弹条无窗口场景」与实现一致。
- 验证：`make visual-check`、`go test ./cmd/mornlea/... -race -count=1`。

## 6. 基线文档同步 + 全量门禁

- [ ] 6.1 文档：`docs/notes/gameplay.md`（准星、弹条、tooltip、浮动面板、菜单风格、配方右栏刻意分歧）、`limitations.md`（无护甲/经验条；弹条硬切）、`progress.md` chronicle、backlog D-10 完成态。
- [ ] 6.2 门禁：`make dev-check`、`make test-race-changed`、`openspec validate --all --strict --no-interactive`、`go test ./internal/archcheck -count=1`；重型门禁前后重建 Rust（陈旧 dylib 防护）。
- [ ] 6.3 整分支终审：对照 design D9 护栏逐条核对通过后开 PR。
