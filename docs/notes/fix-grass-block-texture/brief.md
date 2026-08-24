# 任务 brief：修复草方块侧面显示异常

## 背景与根因（已排查确认）

Mornlea 图形客户端的产品默认材质是内嵌 Pixel Perfection 子集（`internal/assets/packs/pixel_perfection/`，
固定 upstream commit `7935d064fc6f993d1b5038ed5ec17a615600cf0a`，CC BY-SA 4.0）。

根因：`textures/grass_side.png` 直接复制自 Minetest 纹理包的 `default/default_grass_side.png`——
它在 Minetest 里是 **overlay** 型纹理（与 `dirt` 用 `^` 合成后才显示），本身只有顶部约 5 行绿色草缘完全不透明：
实测每行平均 alpha 为 row0-4 = 255、row5 ≈ 242、row6 ≈ 174、row7 ≈ 42、row8-15 = 0（共 134 个完全透明像素 + 16 个半透明像素）。

Mornlea 渲染路径对每个材质层做**直接采样**：
- `engine/crates/mornlea_client/shaders/terrain.wgsl` 的 `fs_main`：`let c = textureSample(...); if (c.a < 0.5) { discard; }`
- `internal/assets/blocks.go` 的 `isCutoutLayer` 只包含 leaves/glass/wheat（`LayerGrassSide` 是不透明层），因此
  `applyPack`（`internal/assets/pack.go`）只做像素替换、没有任何合成，`LayerGrassSide` 拿到的就是这张半透明 overlay。

效果：草方块侧面下半部（约 8/16 高度）被 discard——侧面出现看穿/破洞；mip 降采样后远处整块侧面被 discard（全仓 alpha 扫描确认
只有 grass_side 这一个不透明层存在 alpha != 255 的像素，其它层要么全不透明、要么是合法的二值 alpha cutout）。

## 修复方案（已确定，不得改变渲染管线/层分类/loader 契约）

1. **替换内嵌资产**：把 `textures/grass_side.png` 换成**完全不透明**的合成图——用直通 alpha（straight alpha source-over）把
   当前 `grass_side.png`（草缘 overlay）合成到本 pack 的 `dirt.png` 之上：每通道 `out = (s*a + d*(255-a) + 127) / 255`（整数、8-bit），`out.A = 255`。输出仍为 16×16 PNG，覆盖原文件。
2. **可复现脚本**：新建 `scripts/composite_grass_side.go`（package main，中文 GoDoc/注释，只依赖 stdlib `image`/`image/png`/os），
   读取 `internal/assets/packs/pixel_perfection/textures/grass_side.png` 与同目录 `dirt.png` 并原位写回合成结果；
   运行两次必须逐字节一致（Go png 编码确定）。脚本注释写明算法与"源文件 + 取整规则"。
3. **Provenance 更新**：`PROVENANCE.json` 中 grass_side 条目：
   - `sha256` 改为新文件的 SHA-256；
   - `source` 保持 `default/default_grass_side.png`（草缘来源）；
   - 新增一个字段（例如 `"derived": {"sources": ["default/default_grass_side.png", "default/default_dirt.png"], "note": "..."}`）
     记录派生（straight-alpha source-over over dirt，输出不透明 16×16）。
   - **不要**改动顶层 `modification` 字符串（`TestEmbeddedDefaultPackProvenance` 对它精确相等断言；逐文件派生在条目里已记录）。
   - `default_pack_test.go` 的 `pixelPerfectionProvenance` 结构体加上 `Derived` 字段并为 grass_side 断言其存在，锁住"派生必须被记录"。
4. **署名更新**：`ATTRIBUTION.md` 保留现有行（测试要求 `ATTRIBUTION.md` 必须包含子串 `without pixel transformations`），
   另起一段注明例外：`grass_side.png` 是派生合成图（绿草缘 source-over 于 `default_dirt.png`），按 CC BY-SA 4.0 保留归属。
5. **守卫测试**：在 `internal/assets/default_pack_test.go` 新增 `TestOpaqueLayersAreFullyOpaque`：
   - 对 `NewRegistry()`（程序化）与 `NewDefaultRegistry()`（内嵌默认）的全部 layer：
   - 若 `isCutoutLayer(layer)` → 256 个像素 alpha 必须是 0 或 255（二值）；
   - 否则 → 256 个像素 alpha 必须全部 == 255。
   - 中文注释写明判据来源：terrain.wgsl 对 alpha<0.5 的片段 discard，不透明层带透明像素 = 方块面穿透。
6. **文档**：
   - `docs/texture-packs.md` 的「材质文件」节补充：非 cutout 层素材必须全图不透明（alpha=255）；cutout 层（leaves、glass、wheat_0..7）
     alpha 必须为 0 或 255；原因：terrain 着色器丢弃 alpha<0.5 的片段。并说明 grass_side 建议直接给完整的"泥土+草缘"合成图
     （内嵌默认即如此），Minetest 风格 overlay 需要自行合成。
   - `docs/notes/progress.md` 最末尾追加一段（中文、仿照既有 fix 条目风格）：根因（Minetest overlay 直接入库 + terrain discard 半透明片段）、
     修复（合成不透明 grass_side、provenance/署名更新、守卫测试）、验证结果。
7. **golden 重生成**：先 `make rust`（本机 rustup 1.97.1 已装且 engine/target/release 已存在，通常秒级增量），再 `make visual-update`
   （会重写 `cmd/mornlea/testdata/golden/*.png`，内部自带 LOD on/off 近环 control）。用 `git status` 检查变化的 golden 集合应只包含
   能看到草方块侧面的场景（至少 terrain-noon、materials-showcase、oak-grove、far-horizon；water-*、avatar-nametag、
   target-block-feedback、debug-panel 可能变化；HUD/容器类、洞穴类应不变）。**若某个明显含草的场景没有变化，停下来报告**。

## 硬性约束

- 不改：shader、mesh、quad 布局、layer 编号、`isCutoutLayer` 判定、`applyPack` 的替换语义（pack 像素直接替换契约不变）、
  协议/存档/engine ABI/client ABI/benchmark scenario。
- 不触碰：`AGENTS.md`、`CLAUDE.md`、`docs/superpowers/`、`openspec/changes/archive/`、`openspec/specs/`。
- 所有新注释/文档用中文；Go 代码必须 gofmt；错误带上下文。
- 不要提交 git；改动全部留在工作树，控制会话会统一处理版本控制。
- 不要删除旧 golden、不要放宽任何阈值、不要绕过 near-band control（`make visual-update` 即为规范路径）。

## 验证（按此顺序，报告里给出每条的命令与结果）

1. `gofmt -l .`（应无输出；至少新改的 Go 文件）
2. `go test ./internal/assets -count=1`
3. `go test ./cmd/mornlea -run 'TestCapture|Test.*Capture|Golden|Visual|Scene' -count=1` —— 先跑 capture 相关测试确认 golden 匹配；
   再用 grep 找 `cmd/mornlea` 里比较 golden 的测试名并运行；包内完整测试（无 -run）如果时间可接受也跑一遍并记录耗时。
4. 在报告中列出：改动文件清单、每处改了什么、测试结果、`git status` 显示的 golden 变化清单、与 brief 的任何偏差及理由。

## 报告

写报告到 `docs/notes/fix-grass-block-texture/report-implementer-r1.md`（自建该目录），然后回复：
- Status: DONE | DONE_WITH_CONCERNS | BLOCKED | NEEDS_CONTEXT
- 一句话测试总结
- golden 变化清单（文件名列表）
- 疑虑（如有）
- 报告文件路径

你不得派生子代理或评审者；评审由控制会话负责。
