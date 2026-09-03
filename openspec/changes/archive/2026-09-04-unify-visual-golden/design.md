# 设计：统一视觉基线目录

## 目录归属

`testdata/visual-golden/` 落在仓库根：Go `testdata` 惯例按包就近，但两套基线分属 Go 包与前端目录，没有一处包目录能同时就近两者；仓库根是唯一中立位置。`assets/` 不收：那是生产材质（纹理包），基线 PNG 是测试夹具二进制，混放会模糊「生产资产—测试夹具」边界。

## 搬迁方式

`git mv` 整批搬迁，禁止复制后删除（复制会断历史）。搬迁后逐字节校验：`git diff --stat` 只显示重命名，`cmp` 抽查新旧路径字节一致；`make visual-check` 与 `make frontend-visual-check` 全绿即像素同一性成立，不重跑 `visual-update`。

## 路径同步点

- Go：`cmd/mornlea/capture/capture_image.go` 的 `captureGoldenDir` 常量改为 `testdata/visual-golden/world`；`capture_near_band_test.go:71` 的硬编码同改。其他引用一律经该常量，不新增路径字符串。
- 前端：`frontend/visual/visual.mjs` 的 `goldenDir` 改为经脚本内既有 `repoRoot` 变量指向 `testdata/visual-golden/ui`，不手写多层 `..`。
- 文档：`cmd/mornlea/capture/AGENTS.md` golden 纪律节、`cmd/mornlea/AGENTS.md` Directory Map、`engine/crates/mornlea_client/frontend/AGENTS.md` 基线节同步新路径；阈值数字不抄，只引用源码。

## 索引文档

`testdata/visual-golden/README.md` 中文，两表：world 表 24 行（文件名→`capture.go` 的场景名→一句话说明，以 `captureScenes` 为准）；ui 表 19 行（文件名→`fixture-names.ts` 的 fixture 名→对应组件）。另写更新入口（`make visual-update` / `make frontend-visual-update`）与先目检后覆盖纪律，不写阈值数字。

## 否决的替代方案

- 只建索引不搬文件：零管线改动但不满足「统一包下」要求，否决。
- 并入 `cmd/mornlea/capture/testdata/golden/ui/`：前端基线寄居 Go 包目录，归属不中立，且前端脚本要跨树引用，否决。
- symlink 兼容旧路径：跨平台脆弱且制造双源假象，否决；旧路径引用全部直改。
