## Why

常显 HUD 已采用奶油粉彩外观，但容器仍由 GPU 绘制，物品仍有色块占位，鼠标响应链与两份布局也不一致。人物与掉落物已有基础呈现，却缺少可辨细节和自然的步态、散落效果，需要形成同一套可实际操作的游戏视觉体验。

## What Changes

- 按用户补充要求，所有面板类统一走 React/WebView：背包、个人/工作台合成、箱子、熔炉、人物信息与 tooltip 共用奶油样式；退役生产 GPU 面板调用。
- 经严格语义桥事件实现栏位点击、合成产物取出、配方、关闭与快捷栏选择；明确自由光标入口、原生输入隔离与高 DPI 坐标口径。
- HUD 选中格抬升，邻格轻微跟随，尊重减少动态效果；保留键盘快捷键与权威确认语义。
- 复用当前材质并为缺失类别补充原创物品轮廓，HUD/面板与非方块掉落共用物品视觉来源。
- 为玩家与伙伴增加原创面部、发型、衣服与鞋履细节，校准按水平位移推进的步态。
- 同方块内掉落按稳定 ID 有界分散，保证通常数量不重叠，高密度采用有界分层/缩放；非方块使用透明轮廓薄片。

## Capabilities

### New Capabilities

- `cream-game-presentation`: 奶油面板交互、物品视觉、精细人物与分散掉落的端到端体验。

### Modified Capabilities

- `container-ui-presentation`: 全部容器与 tooltip 由前端呈现，保留权威操作语义。
- `game-overlay-webview`: 游戏自由光标/容器状态允许 WebView 交互，捕获光标时穿透。
- `survival-hud-presentation`: 选中动效、真实图标与 GPU 面板退役资源语义。
- `avatar-locomotion`: 按水平位移与身体腿长校准步幅，跳跃/传送不错误推进。

## Impact

涉及 client/app、client 桥、assets、render、Rust mornlea_client 的 WebView/输入与前端，及对应测试、指南和规格。服务端仍为唯一权威；不修改网络协议 v35、存档 schema、engine ABI v10、client ABI v15 的 C 布局与现有固定 GPU 容量。仅进程内 JSON UI 桥同步扩展。无新外部依赖、无外部版权素材；像素生成与图标编码在装配阶段完成，帧循环仅消费缓存。非目标为装备系统、人物自定义系统、物理掉落坐标持久化与 Minecraft 兼容。
