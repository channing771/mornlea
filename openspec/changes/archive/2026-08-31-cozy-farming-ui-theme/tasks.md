## 1. Go HUD 令牌换肤

- [x] 1.1 按 design.md D2 调色板重写 `internal/render/hud/style.go` 令牌族：panel/slot/border/shadow 暖化、强调拆为 `accentSelected`（sage）与 `accentProgress`（wheat）双族、新增 `textOnPanelFg`、tooltip 表面与前景切换、hotbar 贴条暖深棕化、`containerSourceHighlightColor` → wheat；保持全部令牌名以外的消费方零结构改动。验证：`go test ./internal/render/hud -race -count=1` 中钉值测试先红后绿（同任务更新 `style_test.go` 钉值表与色族注释）。
- [x] 1.2 同步更新 `internal/render/hud/atlas.go` 的 `paintContainerSlot`/`paintContainerTitle` 调色板为暖色族（凹槽暖米棕、标题深暖棕字），并同步 `atlas_mask_test.go` 的语义色族断言（心红/气泡青/鸡腿棕色族断言不变）。验证：`go test ./internal/render/hud -race -count=1` 全绿。
- [x] 1.3 检查 `style_test.go` 中 `TestStyleTokensAreTheOnlyFloatColorSource`、`TestOpenPanelsConsumePanelTokens`、`TestChatTextUsesTextPrimaryTokens`、`TestHotbarDigitsUseTextPrimaryTokens` 等结构性断言在新令牌名/新字色分工下仍成立；需要调整断言时只改断言的目标令牌语义，不放松「唯一样式源」门禁。验证：`go test ./internal/render/hud -race -count=1` 全绿，`gofmt` 与 `go vet ./internal/render/hud` 通过。

## 5. 布局修订（评审反馈：状态条间距 / MC 式合成区 / 配方独立面板）

> **范围裁决（用户指令）**：用户已裁决游戏内 HUD 与全部界面迁移到 CSS 前端
> （WebView）实现（详见 `webview-game-ui-unification` change），布局样式以
> 一次性 HTML 预览稿（/tmp/mornlea-ui-preview）为准。Go 渲染路径上的 5.2/
> 5.3/5.4 属将被退役路径的丢弃功，**取消**；5.1（状态栈间隙，过渡期体验改
> 善）已完成评审待提交。取消的任务不回滚 design.md 的 D7/D8 记录——其布
> 局语义由 webview-game-ui-unification 的 CSS 实现承接。

- [x] 5.1 新增 `statusHotbarGap = 10` 常量（design.md D6）：`statusBarBounds` 主状态行推导、`closedHUDHeight`、打开态 bottomMargin 同步消费；氧气行与主行的 `statusBarGap` 堆叠距离不变；相关布局测试（closedHUDHeight/行位置断言）同步更新。验证：`go test ./internal/render/hud -race -count=1` 全绿，`gofmt`/`go vet` 通过。
- [ ] ~~5.2 背包合成区右上重排~~（已取消：由 webview-game-ui-unification 的 CSS 实现承接 MC 式布局）
- [ ] ~~5.3 配方独立浮动面板~~（已取消：同上，配方独立组件在 CSS 前端实现）
- [ ] ~~5.4 重新执行 2.2~~（改由收尾任务执行一次 golden 对齐：仅覆盖 5.1 几何与主题换肤，不做 Go 布局重排）

## 2. HUD golden 波及面确定与重生成

- [x] 2.1 构建前端 dist 与 `make rust` 后以 compare 模式运行无窗口 capture，实测确定波及场景清单（实测 23 景漂移：除 main-menu/settings-menu 纯全景零差异外的全部场景——`paintContainerSlot` 为 hotbar 槽位与容器槽位共用 atlas cell，hotbar 出现在所有世界场景，属「Hotbar Slot 换 Mornlea Palette」需求的合法波及；差异 bbox 精确吻合 hotbar 贴条几何 476×60，世界/天空/地形逐像素一致）。验证：compare 输出已记录入本 change ledger，无未可解释波及。
- [x] 2.2 以显式 update 模式重生成 2.1 确定的受影响 golden（23 景；main-menu/settings-menu 保持逐字节不变），产出差异可视化材料，逐图人工复核（用户执行）后接受；25 景清单、场景顺序、双阈值全部不变。验证：再次 compare 全部 25 景通过；`git status` 中仅受影响 PNG 变化。（最终态经 4.4 对齐 5.1 几何后提交 `1b1ecafc`，用户终审通过）

## 3. WebView 菜单层换肤与响应式

- [x] 3.1 按 design.md D2/D3 重写 `frontend/src/tokens.css`：暖色调色板令牌、双强调体系、`color-scheme: light`、`:root:root` 换肤段映射链结构不变仅换值；新增/调整响应式尺寸令牌（`clamp()`/`min()` 口径：按钮宽高、按钮列间距、标题间距、标题字号、debug 面板宽度上限）。验证：`corepack pnpm install --frozen-lockfile` 后 `make frontend-check` 中类型检查与组件测试通过（失败先修测试再继续）。
- [x] 3.2 更新 `frontend/src/ui/ui.css`：主菜单按钮间距/尺寸消费新令牌修复拥挤；设置/暂停/调试面板表面与文字换暖色；tooltip 无涉及；hover/focus/selected 态切换 sage wash 与 sage 焦点环；滑块拇指换 wheat；全文件保持零裸色值/裸字号/裸时长纪律。验证：`make frontend-check` 全绿；`grep -nE '#[0-9a-fA-F]{3,8}|rgba?\(' frontend/src/ui/ui.css` 仅 @import 与字体声明无关命中为空。
- [x] 3.3 运行前端视觉基线 update 入口重生成 `frontend/visual/golden/` 12 张部件基线，逐图人工复核（用户执行）后接受；再跑 check 入口确认零漂移。验证：check 入口退出码 0。
- [x] 3.4 重建 `dist/`：连续两次 `vite build` 实测字节一致后提交入库 dist。验证：`make frontend-check` 末行 `git diff --exit-code -- <dist>` 通过。

## 4. 收尾门禁与文档

- [x] 4.1 `cd engine && cargo test -p mornlea_client --locked` 全绿（WebView 层无行为变化，若有钉值断言波及按 1.3 同口径处理）。验证：命令退出码 0。
- [x] 4.4 golden 对齐收尾：以显式 update 重生成受 5.1 几何影响的 HUD 类 golden（compare 实测确定清单；main-menu/settings-menu 必须逐字节不变），我方多模态自检实拍帧后连同前端 12 张基线交用户终审。验证：compare 全部 25 景通过。
- [x] 4.2 全量门禁：`gofmt`、`go vet ./...`、`go test ./internal/render/hud ./internal/archcheck -race -count=1`、`make frontend-check`、`make dev-check`、`openspec validate --all --strict --no-interactive`。验证：全部退出码 0。
- [x] 4.3 更新 `docs/notes/progress.md` 编年史条目（主题换肤、golden 重生成范围、复核记录）与 `engine/crates/mornlea_client/frontend/AGENTS.md` 的强调色纪律条款（「琥珀是唯一强调色相」改为 Mornlea 双强调体系表述），change ledger 记录验证输出与人工复核结论。验证：`openspec validate --all --strict --no-interactive` 通过。
