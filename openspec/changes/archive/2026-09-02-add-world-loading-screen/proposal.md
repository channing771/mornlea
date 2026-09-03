# add-world-loading-screen

## 背景

交互客户端从主菜单点击「进入游戏」后，`startWorld` 完成同步装配即直接进入
游戏相位：主菜单立即消失，世界以「每帧 drain 64 条消息、mesh 64 段、上传
4MiB」的稳态节流管线渐进浮现。初始视距快照共 `(2*(ViewDistance+1)+1)^2`
（默认 4489）个区块列，浮现期间画面只有天空清屏色与程序化天空，没有任何
加载门控或进度反馈——用户面对一段「画面透明、逐渐长出地形」的等待期。

无头路径（benchmark/capture）已存在完整的初始加载判据
`cmd/mornlea/app/app_load.go`：`LoadedChunkTarget`（目标列数）+
`ApplicationLoadComplete`（快照齐 + mesher 队列空 + 上传收敛）+
`WaitUntilLoaded`（4096/帧的 drain 与 mesh 预算推进循环）。交互路径没有
等价物。

## 目标

- 点击「进入游戏」且装配成功后进入新的 `loading` 相位：主菜单消失，WebView
  呈现不透明加载屏（标题、进度条、区块计数），半成品世界不可见。
- 加载屏期间继续推进世界加载（drain 消息、mesh、上传），预算与无头
  `WaitUntilLoaded` 同源（`MessageDrainMax`），交互初始加载因此比现状更快收敛。
- 加载完成判据复用无头单一定义：`LoadedChunkTarget` + `ApplicationLoadComplete`；
  收敛后捕获光标、进入既有游戏相位，首帧即为完整世界与正确相机。
- `loading` 相位归 WebView 菜单族（可见、firstResponder、不捕获光标），加载期
  无游戏输入、无 HUD、无上行动作（Enter 不得重复触发进入游戏）。
- 进度语义：`loading` 分节下行 `loaded`（已就绪区块列数）与 `total`（目标
  列数），前端只做比例换算与格式化，语义权威在 Go。

## 非目标

- 不异步化 `startWorld`：装配（开存档 + 登录）仍同步执行，点击后菜单短暂
  冻结在最后一帧的现状保留；加载屏覆盖其后真正耗时的区块流送期。
- 不修改协议、存档 schema、engine ABI、client ABI、benchmark scenario 版本；
  不改帧 TLV 与既有 C ABI 出入口。
- 不为 `-connect` 远程会话提供加载屏（该路径 WebView 永不挂载、直进游戏相位）；
  如需覆盖另立 change。
- 不提供加载中途取消/超时；异常卡死以窗口关闭退出，5 秒进度日志辅助诊断。
- 不改无头 `WaitUntilLoaded` 行为与 24 张世界像素 golden（加载屏是 WebView
  chrome，无头路径不挂载 WebView，验收走 vitest 组件断言与前端部件基线）。

## 用户可观察结果

- 点击「进入游戏」后短暂停留在主菜单（装配期），随后出现不透明加载屏：
  标题「正在生成世界…」、像素风进度条随已加载区块列数推进、下方显示
  「区块 x / y」计数。
- 加载期间鼠标不被捕获、按键无游戏效果；进度条走满并完成网格与上传收敛后，
  加载屏消失、光标被捕获、直接看到完整世界。
- 暂停页「退回主菜单」后再次「进入游戏」：重新走同一加载相位，进度条从头
  推进。

## 受影响的包与文档

- `cmd/mornlea/app`：`MenuPhase` 增加 `MenuPhaseLoading`；`startWorld` 成功
  置 loading 相位；`RunInteractive` 增加 loading 路由与 `runLoadingPhase`
  循环；`uiState` 组装新增 `loading` 分节。
- `engine/crates/mornlea_client/frontend/src`：`bridge/schema.json` 相位枚举
  与 `loadingState` 分节；`bridge/client.ts` 类型与守卫；新组件
  `ui/LoadingScreen.tsx`；`App.tsx` 相位路由与键盘路由；`visual/` 新 fixture
  与部件基线。
- `engine/crates/mornlea_client/src`：`webview.rs` 菜单族相位测试清单与
  `overlay.rs` 模块文档的菜单族枚举补 `loading`（`state_wants_visible` 的
  `phase != "game"` 判定零行为变更）。
- `docs/feature-backlog.md`：D-19 行（已随认领登记）。
- `openspec/specs/webview-menu-ui`：装配条款改为装配成功进入加载相位（本
  change 的 delta）。

## 兼容性影响

- 无协议、存档、engine ABI、client ABI、benchmark scenario 版本变化；版本
  矩阵不变。
- 桥 schema 追加相位枚举值 `loading` 与 `loadingState` 分节：Go/Rust/TS 三端
  钉值测试同步；Rust 下行浅校验只要求 `phase` 为字符串，无行为变更；未知
  分节/相位的既有拒绝纪律对新值同样生效。
- 无头路径（benchmark/capture）不经过菜单相位机，行为零变化；既有世界 golden
  逐位不变。
- 加载相位 drain/mesh 预算提升到与 `WaitUntilLoaded` 同源（每帧 4096），仅
  影响交互初始加载的收敛速度（更快），不影响稳态游戏帧预算。
