## Why

当前生存信息分散在快捷栏、生命、氧气、饥饿与采掘反馈中，窄窗口和打开容器时也缺少统一的避让规则；这使关键状态难以扫读，并让颜色辨识成为采掘反馈的重要依赖。`authoritative-hunger` 已在 main 建立权威饥饿与 HUD 呈现，本变更需要在不改变其权威语义和渲染 ABI 的前提下，把这些信息整合为一套可自动验收的生存 HUD。

## What Changes

- 把快捷栏固定为居中的九格生存栏，并以外扩双层边框提供不依赖颜色的选中反馈；保留物品数量和工具耐久的既有权威来源与显示条件。
- 把已确认生命呈现为十个直接解析为空、半或满心的槽位；满氧继续完全隐藏，耗氧时把十个槽位按权威剩余值向上取整直接解析为空或满气泡；已确认饥饿始终显示十个空鸡腿槽与最多十个覆盖填充，未确认时隐藏。
- 让生命与饥饿组成稳定的主状态行：生命起点精确对齐快捷栏左边，饥饿终点精确对齐快捷栏右边；耗损氧气单独沿饥饿右边缘向快捷栏外侧堆叠，满氧隐藏时不改变主状态行位置。HUD 图集在物品列前依次保留空/半/满心、空/满气泡、空/满鸡腿七个原创程序化 cell。
- 把权威采掘进度放在关闭态完整两行状态栈上方，钳制到 100%，并让可采与不可采状态同时具有颜色和形状差异。
- 让快捷栏、两行状态栈与采掘反馈在窄 framebuffer 中整体等比缩小；打开背包、合成、箱子或熔炉时把主状态行和向下外扩的氧气行一起避让既有可交互格子，不重绘这些容器界面。
- 继承 main 的 benchmark scenario v19 固定 HUD 契约：267 quad、700 glyph、13312-byte glyph offset 与 46912-byte 总容量；保持现有 HUD pass、48-byte instance 编码和稳定态零每帧动态资源，并在合并后的 Pixel Perfection + hunger 基线上验收既有 15 个确定性 capture 场景。

非目标：不重做背包、合成、箱子或熔炉视觉，不增加 UI 配置、主题系统、二进制 UI 资产或第三方依赖，也不改变游戏状态的权威与预测边界。

## Capabilities

### New Capabilities

- `survival-hud-presentation`: 规定九格快捷栏、生命、氧气、饥饿、采掘反馈、响应式缩放、容器避让和固定渲染资源契约。

### Modified Capabilities

- `visual-verification`: 在保留全部既有无窗口场景行为、完整渲染链路、场景尾序和双阈值的基础上，加入生存反馈场景并扩充 HUD 视觉验收。

## Impact

- 受影响代码集中在 `internal/render/hud` 的 CPU 侧 atlas、布局、容量与测试，以及 `cmd/mornlea` 的 capture 场景、测试和 golden。
- 后续里程碑记录会同步更新 `AGENTS.md`、`CLAUDE.md` 与 `docs/notes/progress.md`；本提案不修改 OpenSpec 主规格，归档时再同步稳定契约。
- 物品、生命、氧气、饥饿和采掘数据继续来自客户端已有的权威镜像；不新增跨 goroutine 状态或阻塞工作。
- 本变更不重复引入 `authoritative-hunger` 已完成的网络或存档迁移，而是继承 main 的协议 v24、玩家 schema v7 与 benchmark scenario v19。区块 schema v9、世界 metadata v2、`companions.ai` schema v4、engine ABI v6、client ABI v7 和配置格式保持不变。
- 正常合并 main 时需要语义化解 HUD/source/长期文档冲突，并在合并后的 Pixel Perfection + hunger 基线上重新生成、逐张人工复核全部 15 张正式 golden；场景顺序和双阈值保持不变。
