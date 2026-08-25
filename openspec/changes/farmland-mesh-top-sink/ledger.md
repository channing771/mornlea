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
