## Why

HUD 物品图集的宽度随 `core.ItemIDMax` 增长（当前 50 列 = 800 纹素，每追加物品扩 16 纹素）。`hotbarTextureUV` 以「列纹素边界 ÷ 当前图集宽度」计算归一化 UV，宽度一变，全部既有列的 f32 UV 重归一化，解码回纹素空间产生 ~5e-5 纹素量级的位移；HUD 在非整数缩放下存在大量恰好落在列边界 ±ε 内的采样点，会翻转归属相邻列。`authoritative-farming` 期间 `hud-hotbar-health` golden 因此出现 0.115% 的像素漂移（farming design 遗留清单第 18 条），当时按「确认采样层正确 → 重生成」处理并把修复显式延期为本独立变更。

## What Changes

- `internal/render/hud/atlas.go` 的 `hotbarTextureUV` 改为对称亚纹素收进（micro-inset）：每列左右 UV 界向列内收进固定余量（1/256 纹素），使任何采样点与列边界的距离恒大于任意现实图集宽度下的 f32 重归一化噪声上界。
- 函数签名与全部调用点保持不变；Rust `mornlea_client`、client ABI、协议、存档 schema 与 benchmark scenario 零改动。
- 新增机械化的属性测试：把 UV 计算提取为宽度参数化的内部函数后，钉死「解码界在本列内且距边界有安全裕度」「相邻列区间不重叠」「同一列在不同图集宽度下解码纹素集合相同」三条性质。

## 用户可观察结果

- 向物品表追加新物品（图集扩列）后，既有 HUD 图标（心、气泡、鸡腿、容器 cell、方块/物品缩略图）渲染的纹素集合不再随扩列变化——图标不会串味到相邻列材质，也不会亚像素漂移。
- 本变更自身对当前 golden 的预期影响为零或接近零（整数缩放下逐位不变）；若本地 capture 对比发现字节差，按设计文档的裁决流程呈报，不静默再生成 golden。

## 非目标

- 不改 glyph 图集与文本流（glyph offset 是独立的固定上传布局，无此漂移问题）。
- 不改 HUD quad 布局、容量常量（267 quad / 700 glyph / 13312-byte glyph offset / 46912-byte 上传容量）与 benchmark scenario。
- 不引入半纹素中心对齐（会使图标视觉放大 ~6.7% 并触发 golden 大面积重生成）与 shader 侧归一化（需动 client ABI）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `survival-hud-presentation`: 追加「HUD 图集列采样稳定性」要求——任何 HUD 图标的采样 MUST 落在其自己的图集列纹素范围内且远离列边界一个安全裕度，图集宽度增长 MUST NOT 改变既有列的采样纹素集合。

## Impact

- 受影响包：仅 `internal/render/hud`（`atlas.go` 与同目录测试文件）。
- 兼容性：无协议、存档、配置或版本号变更；`hotbarTextureUV` 对外签名不变，`container.go` 等消费文件零改动。
- 并发/性能：UV 计算仍在布局期一次完成，每列两次除法不变，无热路径新增分配。
- 视觉基线：capture golden 不由本分支修改；本地对比验证若发现差异则升级裁决。
