# hotbar-tiny-glade-skin 设计

基线 SHA：`5335c6ae`（main）。需求：需求方裁决——底部快捷栏换成参考图
（Tiny Glade 底部工具栏）的粉彩手绘风格，高度还原；纯换肤，不动布局功能。

## 决策

1. **只动前端呈现层**：`packages/engine/crates/mornlea_client/frontend`
   内 `tokens.css`、`hud/hud.css`、`hud/Hotbar.tsx`、新建
   `hud/slotDoodles.tsx`、测试与 visual 基线。桥协议、geometry 常量、
   Go 组装、Rust 内嵌一律不动——九格、语义、缩放分母是既有契约。
2. **令牌单源纪律**：新增表面色 `--hud-tool-1..9`（九块粉彩）、
   `--hud-tool-ink`（描边墨）、`--hud-tool-selected`（选中橙）、几何
   `--hud-tool-radius`、阴影 `--hud-tool-shadow`。新增 `--hud-*` px 令牌
   必须同步归入 `geometry.test.ts` 的两张互钉表之一（圆角半径进
   `TOKENS_WITHOUT_GEOMETRY_CONSTANT`），否则完备性断言即红。
3. **双强调纪律**：九粉彩属表面填充、非强调色；选中橙登记为 wheat 系的
   暖向变体（进度加重要信息的暖色延续），sage 仍管焦点加 hover、danger
   仍管错误，不引入第三强调色相。头注同步说明。
4. **图标走内联 SVG**：仿 `icons.tsx` 的 mask→path 模式新建
   `slotDoodles.tsx`，九个淡印手绘线稿只做空槽衬底；有物品时隐藏，
   与 `hud-slot-tile` 互斥。不走 PNG 材质管线（避开 16×16 加命名槽加
   mip 约束），不新增二进制资产（`AGENTS.md` 字体白名单纪律）。
5. **选中态仍是几何标记**：暖橙 3px 外扩外框加抬起阴影，删 sage 内衬
   平铺——spec 的「忽略颜色后可区分」由外扩几何加阴影抬起承担。
6. **任务切分**：失败测试先行（assert 样式表层钉改新风格、Hotbar 空槽
   衬底加选中类加 data-index）与实现同一任务（前后端无契约变更，
   同批改齐即可）；visual 基线重拍与门禁收尾独立任务。

## 被否决的替代方案

- **PNG 贴图经材质包管线**：需新增 11 个材质槽位并先把 `item→layer`
  映射接到 `.hud-slot-tile` 背景（当前 tile 是 CSS 占位，不读材质层），
  改动面远超换肤，且 16×16 像素管线画不出手绘线稿感——否决。
- **11 格布局连功能一起改**：格数由 `core.HotbarSlots=9` 与桥协议钉死，
  动格数即协议加存档加 Go 加 Rust 全链变更，与「纯换肤」需求相悖——
  另开 change，本 change 只取图中前 9 色顺序。

## 受影响文件

| 层 | 文件 | 动作 |
|---|---|---|
| 前端 | `frontend/src/tokens.css` | 新增粉彩加描边加选中加圆角加阴影令牌 |
| 前端 | `frontend/src/hud/hud.css` | 重写贴条（透明）加格（粉彩圆角）加选中（橙外框抬起）加数量（深棕字）加耐久（亮底对比） |
| 前端 | `frontend/src/hud/Hotbar.tsx` | 格节点加 `data-index`，空槽渲淡印衬底 |
| 前端 | `frontend/src/hud/slotDoodles.tsx` | 新建：九个淡印手绘内联 SVG |
| 前端 | `frontend/src/hud/hud.assert.test.tsx` | 样式表层钉改新风格（橙外框、深棕字） |
| 前端 | `frontend/src/hud/geometry.test.ts` | 新增 px 令牌归入互钉表 |
| 前端 | `frontend/src/hud/HudRoot.test.tsx` | 视需要同步（空槽衬底节点） |
| 前端 | `frontend/visual/fixtures.tsx` | 沿用既有 `hud-hotbar` 夹具（已覆盖选中加数量加耐久加空格），不新增 fixture 名 |
| 前端 | `frontend/dist/`（入库产物） | 重建 |
| 基线 | `testdata/visual-golden/ui/hud-hotbar*.png` | `visual-update` 重拍，人工目检后入库 |

## 风险与回退

- 风险：亮底对比度（数量字加耐久条在粉彩底上的可读性）——由深棕前景
  加浅投影承担，visual 基线人工目检兜底。
- 风险：粉彩与双强调纪律冲突——头注明确「表面色非强调色、选中橙归
  wheat 暖向」，评审重点看此两处。
- 回退：revert 单一分支；无协议加存档加 ABI 迁移。

## 验证

- `make frontend-check`（typecheck 加 vitest 加 build 加 dist 一致性）；
- `make frontend-visual-check`（本机 Chrome，不进 CI；预期 `hud-hotbar`
  漂移——先 `visual-update` 重拍、人工目检后再 check）；
- 评审 grep：`src/` 无裸色值、`localStorage|fetch(` 为空；
- `openspec validate --all --strict --no-interactive`。
