## Why

当前生存信息分散在快捷栏、左下角生命与纯色氧气/采掘条中，窄窗口和打开容器时也缺少统一的避让规则；这使关键状态难以扫读，并让颜色辨识成为采掘反馈的重要依赖。需要在不改变权威状态来源和渲染 ABI 的前提下，把已有信息收拢为一套可自动验收的生存 HUD。

## What Changes

- 把快捷栏固定为居中的九格生存栏，并以外扩双层边框提供不依赖颜色的选中反馈；保留物品数量和工具耐久的既有权威来源与显示条件。
- 把已确认生命呈现为十个空、半或满心；满氧继续完全隐藏，耗氧时以十个空槽及按权威剩余值向上取整的覆盖气泡呈现。
- 把权威采掘进度放在状态行上方，钳制到 100%，并让可采与不可采状态同时具有颜色和形状差异。
- 让快捷栏、状态行与采掘反馈在窄 framebuffer 中整体等比缩小；打开背包、合成、箱子或熔炉时只移动状态行避让既有可交互格子，不重绘这些容器界面。
- 保持固定 HUD 容量、现有 HUD pass、48-byte instance 编码和稳定态零每帧动态资源，并新增确定性 `hud-survival-feedback` capture 场景及对应 golden 验收。

非目标：不重做背包、合成、箱子或熔炉视觉，不增加 UI 配置、主题系统、二进制 UI 资产或第三方依赖，也不改变游戏状态的权威与预测边界。

## Capabilities

### New Capabilities

- `survival-hud-presentation`: 规定九格快捷栏、生命、氧气、采掘反馈、响应式缩放、容器避让和固定渲染资源契约。

### Modified Capabilities

- `visual-verification`: 在保留全部既有无窗口场景行为、完整渲染链路、场景尾序和双阈值的基础上，加入生存反馈场景并扩充 HUD 视觉验收。

## Impact

- 受影响代码集中在 `internal/render/hud` 的 CPU 侧 atlas、布局、容量与测试，以及 `cmd/mornlea` 的 capture 场景、测试和 golden。
- 后续里程碑记录会同步更新 `AGENTS.md`、`CLAUDE.md` 与 `docs/notes/progress.md`；本提案不修改 OpenSpec 主规格，归档时再同步稳定契约。
- 物品、生命、氧气和采掘数据继续来自客户端已有的权威镜像；不新增跨 goroutine 状态或阻塞工作。
- 不迁移协议 v23、玩家/区块/世界/伙伴存档 schema、engine ABI v6、client ABI v7、benchmark scenario v18 或配置格式；无兼容性迁移和 benchmark 工作负载变化。
