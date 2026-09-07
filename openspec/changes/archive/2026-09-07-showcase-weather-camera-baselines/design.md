## Context

`weather-camera-tree-diversity` 已落地天气呈现与三态视角（分支 `feat/weather-camera-tree-diversity` 待合入；本 change 基于合入后的 main 开发）。视觉基线现状：`world/` 24 张 PNG（`captureScenes` 注册）、`motion/` 10 个 GIF（6 个 motion 演示 + 4 个被动牛剧本，只供人眼审查）。无头抓帧强制第一人称与晴天（既有纪律），故新能力在现有基线中零覆盖。

## Goals / Non-Goals

**Goals:**
- 雨天稳定态、第三人称双机位有 PNG 基线并进 `visual-check` 比对。
- 天气切换全过程有 GIF 演示供人眼审查（不进比对）。
- 既有 24 张 golden 字节不变；阈值、尾序、更新纪律不变。

**Non-Goals:**
- 不做雪天独立场景（雪线派生，`rain-noon` 取雪线下机位即锁定雨形）。
- 不给 GIF 设比对阈值；不动 24 个既有场景的夹具与相机。

## Decisions

### D1 新场景插到 `mining-crack-heavy` 之后、`main-menu` 之前

- 理由：`far-horizon` 倒数第二、`water-underwater` 唯一末尾是多处 spec 的硬约束；菜单双景相邻约束要求新场景不进菜单区；裂纹双景之后是唯一的无约束插入点。
- 否决：插到队尾（破坏尾序约束）；插到 `oak-grove` 旁（打断既有关联顺序 `oak-grove→ai-companion→sword-combat→hostile-mob`）。

### D2 雨天夹具复用正午地形机位，天气经 headless 固定注入

- 做法：capture harness 新增固定天气注入（仅抓帧路径，复用 `Predictor` 天气接受口径，不碰游戏内权威）；机位选雪线以下已验证有树的地形。
- 否决：复用全景菜单路径（全景强制晴天，语义冲突）。

### D3 第三人称场景复用同一世界位置与朝向，仅改 `CameraMode`

- 理由：双图差异只来自机位，审查可辨认性最直接；复用 3.2/3.3 的装配（自身渲染 + 后拉相机）。
- 否决：为场景定制摆拍位姿（引入与游戏不一致的特殊路径）。

### D4 `weather-cycle` 走既有 motion 演示形态（`--motion-demo` 同族新入口）

- 做法：天气段按压缩 tick 推进（晴/雨/雷暴各一段，不按真实分钟时长），8fps 上限 96 帧，标准库 `image/gif` 编码，确定可复现。
- 理由：与 `break-burst` 等 6 个演示同纪律（只审查不比对）；压缩 tick 避免分钟级录制。
- 否决：进门禁比对（天气是连续环境态，无“结算帧”可钉，阈值无意义）。

## Risks / Trade-offs

- [场景清单 24→27，所有顺序测试需同步] → 与清单实现同任务更新，`capture_scene_order` 类测试即挡板。
- [雨粒子逐帧抖动导致基线不稳定] → 粒子相位必须绑定固定 tick（沿用 2.2 的 `(序号, 权威tick)` 纯函数），先连跑两次确认阈值内再入库。
- [GIF 体积] → 96 帧上限 + 自适应调色板既有纪律，超预算拒绝。

## Migration Plan

- 本 change 只加文件（3 场景代码、3 PNG、1 GIF）与清单扩展；回退即整分支 revert，既有 24 张不受影响。
- 落地顺序：场景代码 → 本地抓帧目检 → `--update-golden` 入库 → `visual-check` 全绿。

## Open Questions

- 无。
