# 设计 — 主菜单全景收敛后揭示

## 判定与数据所有权

- 揭示门住在 `Application`（vista 的唯一所有者）：`menuVistaForFrame` 产出的 vista 附带收敛判定（复用 capture 依赖的 `MenuVistaPending` 口径），`RenderFrame` 在未收敛时走「仅天空清屏」分支（与构建失败降级共用清屏出口），不提交任何地形/LOD pass。
- 相机 tick 只在揭示帧推进：现状 `RenderFrame` 每渲染帧自增 vista tick；改为揭示帧才自增，保证收敛后从 tick 0 起的帧序列与现状收敛段逐位一致（golden 零漂移的构造性前提）。
- 泵速常量 `menuVistaChunksPerFrame` 4 → 12；环形入队顺序不变（确定性调度不回退）。等待期允许更长帧时（worldgen 在主线程泵）：菜单期无游戏输入、WebView 合成独立于 wgpu 帧，观感不受影响。

## 受影响文件

- `packages/client/cmd/mornlea/app/app_menu_vista.go`（pending 出口、tick 推进时机、泵速）
- `packages/client/cmd/mornlea/app/app_frame.go`（揭示门分支）
- `packages/client/cmd/mornlea/app/app_menu_vista_test.go`（新增门测试；`TestMenuVistaDoesNotAssembleWorld` 红线测试不动）

## 取舍

- **否决「预渲染循环视频/图片序列」**：渲染客户端无图像/视频解码与全屏纹理 pass，新增 ABI 面与纹理入口保守口径冲突，且丢失「与游戏同一渲染路径」的保真；固定种子 vista 本身就是「预设地图」，收敛门即达成用户可观察目标。
- **否决「缓存首次装配的快照字节」**：收益仅限同进程二次进菜单，失效面与复杂度不成比例；收敛门已消除可见问题。
- **不额外加启动屏/淡入**：窗口与 WebView 立即呈现，等待期即天空底色，无需新呈现态。
- **等待期呈现语义澄清**：揭示门与构建失败共用 nil 出口。主路径（启动进主菜单）游戏镜像为空，等待帧即纯天空清屏；暂停退回主菜单的边角沿用该出口的既有行为——等待期以游戏相机呈现冻结的旧世界直至收敛，旧世界几何不是「部分装配的全景几何」，场景验收（无逐块浮现、按钮可交互）仍成立，且相对变更前（第一帧即被部分装配几何替换）无回退。

## 验证

`go test ./packages/client/cmd/mornlea/app -race -count=1`；`SCENES=main-menu,settings-menu make visual-check`（golden 必须零漂移）。
