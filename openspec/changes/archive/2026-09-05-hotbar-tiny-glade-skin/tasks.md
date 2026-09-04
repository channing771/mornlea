# hotbar-tiny-glade-skin 任务

约束：fresh implementer + TDD + 双评审；注释中文无任务编号；提交单行英文。
基线 SHA `5335c6ae`。

## 1. 粉彩换肤实现（失败测试先行）

- [x] 1.1 失败测试先行：`hud.assert.test.tsx` 样式表层钉改新风格（选中格
  暖橙外框加抬起、数量深棕前景、贴条透明无底带）；`HudRoot.test.tsx`
  视需要同步（空槽衬底节点与 tile 互斥）；新增 px 令牌先入
  `geometry.test.ts` 互钉表再在 tokens.css 落值。先跑 vitest 见红。
- [x] 1.2 实现：`tokens.css` 新增粉彩加描边加选中加圆角加阴影令牌；
  `hud.css` 重写贴条加格加选中加数量加耐久；`Hotbar.tsx` 加
  `data-index`、空槽渲淡印衬底；新建 `slotDoodles.tsx`（九个淡印手绘
  内联 SVG）；`make frontend-check` 重建 dist。
- [x] 1.3 验证：`make frontend-check` 全绿；评审 grep（`src/` 无裸色值、
  `localStorage|fetch(` 为空）。

## 2. 视觉基线与收尾门禁

- [x] 2.1 `make frontend-visual-update` 重拍 `hud-hotbar`（及受扰的
  `hud-status` 等），人工目检确认粉彩风格与可读性后入库；再跑
  `make frontend-visual-check` 全绿。
- [x] 2.2 `gofmt -l .` 无输出（本 change 无 Go 改动，空跑见证）；
  `openspec validate --all --strict --no-interactive`；
  文档同步（若局部指南提及快捷栏外观）。
