# remove-mining-hud-bar 设计

基线 SHA：`dca9197e`（main）。需求：需求方裁决——采掘进度条与裂纹并存
重复，撤屏幕条，裂纹为唯一进度反馈。

## 决策

1. **桥分节彻底删除而非保留死数据**：`hudState.mining`（schema.json、
   TS `client.ts`、Go `UIHudMining`）三端同批删除——仓库纪律禁止为退役
   路径保留死资源；`ui_push_state` 信封其余字段不动。
2. **`MiningOverlay` 保留并瘦身**：Active/Target/HasTarget/
   ProgressTicks/RequiredTicks 全部仍被世界空间裂纹消费（`deriveBlockCrack`
   → `BlockCrackStage`）；`Harvestable` 唯一消费方是被删除的 UI 分节，
   一并删除字段（镜像、赋值点、capture 夹具、测试字面量同步）。
3. **进食条独立**：`ProgressTrack` 的 `ProgressKind` 收缩为 `"eating"`，
   cap/notch 分支、`.hud-mining-*` 令牌、`PROGRESS_CAP/NOTCH` 常量删除；
   轨道锚点（`--hud-progress-track-gap` 堆叠、`DESIGN_HEIGHT` 记账）保留
   ——进食条仍占同一行。Go 侧 `assembleHUDState` 删除 mining 分节与
   `!Active` 门控（互斥随采掘条消失），eating 直接组装。
4. **裂纹 golden 不动**：无头 capture 从未渲染 WebView 进度条，两张裂纹
   golden 无进度条像素，无需重生成。前端视觉基线
   `visual/golden/hud-progress.png` 需重拍（fixture 只留 eating）。
5. **任务切分**：三端桥同步 + 组件 + Go 分节 + 测试必须同一任务（两侧
   手工同步的契约同批改齐）；收尾门禁（frontend-check 含 dist 入库、
   focused race、openspec strict、visual-check 复跑）独立收尾任务。

## 受影响文件

| 层 | 文件 | 动作 |
|---|---|---|
| 前端 | `frontend/src/hud/ProgressTrack.tsx` | kind 收缩、删标记分支 |
| 前端 | `frontend/src/hud/HudRoot.tsx` | `resolveProgress` 去 mining |
| 前端 | `frontend/src/bridge/client.ts` + `schema.json` | 删 HudMining/mining 分节 |
| 前端 | `frontend/src/hud/hud.css` + `src/tokens.css` | 删 mining 样式与令牌 |
| 前端 | `frontend/src/hud/geometry.ts` | 删 cap/notch 常量 |
| 前端 | 各 `.test.tsx/.ts` + `visual/fixtures.tsx` + golden | 同步删改 |
| Go | `internal/client/ui_hud_state.go` | 删 `UIHudMining`/`Mining` 字段 |
| Go | `cmd/mornlea/app/app_ui_state.go` | 删分节与互斥门控 |
| Go | `internal/render/hud/presentation.go` | 删 `Harvestable` |
| Go | `cmd/mornlea/app/app_messages.go` 等 | 赋值点与测试同步 |
| 前端 | `frontend/dist/`（入库产物） | 重建 |

## 风险与回退

- 风险：三端 schema 漂移——由 schema.test/client.test/Go JSON 测试三侧
  钉值 + 同批提交消除。
- 回退：revert 单一分支；无协议/存档/ABI 迁移。

## 验证

- `make frontend-check`（typecheck + vitest + build + dist 一致性）；
- `go test ./internal/client ./cmd/mornlea/app ./cmd/mornlea/capture -race -count=1`；
- `make visual-check`（24 场景零差异，确认裂纹与容器面不受扰）；
- `openspec validate --all --strict --no-interactive`。
