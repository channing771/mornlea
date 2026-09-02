# 前置 spike 执行清单（hitTest 分级 / GameOverlay 合成开销）

状态：**已自动化执行（2026-08-31）**，结论见第 6 节与 `design.md` D7。代码侧
（`engine/crates/mornlea_client`）具备一条命令可跑的 spike 入口、输入序列清单、
测量埋点与日志判读方法；第 1–5 节为手工执行口径（保留作复测脚本），第 6 节为
自驱动档的实际执行记录。

## 0. spike 入口与环境变量

spike 实现整体隔离在 `engine/crates/mornlea_client/src/overlay_spike.rs`（帧
探针与参与模式状态机）与 `engine/crates/mornlea_client/src/spike_auto.rs`
（自驱动档；`lib.rs` 以私有模块挂载，验证完成后整体移除）。三个环境变量门控，
默认全关 = 生产行为逐字节不变：

| 变量 | 取值 | 语义（**B.2 生产化之后**，见下方迁移说明） |
| --- | --- | --- |
| `MORNLEA_SPIKE_OVERLAY` | 未设置 / `0` / `off` | 不强制：生产路径，参与模式由下行相位逐次推导（子类挂载；游戏相位 = GameOverlay 常驻可见 + `hitTest:` 返回 nil） |
| | `menu` | 对照臂：强制 `OverlayMode::Menu`，游戏相位回退旧的「隐藏 + 归还焦点」语义（**S2 复测的基线臂**） |
| | `1` / `game` / `overlay` | 验证臂：强制 `OverlayMode::GameOverlay`，与生产的游戏相位行为逐项一致 |
| `MORNLEA_SPIKE_FPS` | 未设置 / `0` | 不启用帧探针 |
| | `1` | 在 renderer present 边界采样，每 120 帧向 stderr 输出一行摘要 |
| `MORNLEA_SPIKE_AUTO` | 未设置 / `0` | 手工档：按第 1–5 节人工操作 |
| | `1` | 自驱动档：进程内自动进入游戏 → 合成事件跑 S1 序列并断言 → S2 两组取数 → 写 `build/spike-result.json` 与 `build/spike-report.md` 后退出进程 |

> **臂位语义迁移（B.2 两态参与模型生产化之后，复测必读）**：A 组执行时的
> `Off` 档 = 「裸 `WKWebView` + 游戏相位隐藏」的无 WebView 参与基线；生产化
> 之后「不强制」即为生产路径，游戏相位是 GameOverlay 常驻可见——旧基线语义
> 不再作为缺省档存在。因此复测时：
>
> - **旧基线臂 = 现 `MORNLEA_SPIKE_OVERLAY=menu` 强制档**（子类挂载、游戏相位
>   隐藏）。S2 基线必须用它，否则是「GameOverlay 比 GameOverlay」，判据恒过、
>   无信息量。
> - `menu` 对照臂含义不变：分离「子类化本身」与「hitTest 改写」两个变量
>   （子类自 B.2 起是唯一构造路径，该臂同时承担「隐藏语义对照」）。
> - `game` 验证臂与生产的游戏相位行为一致；「无 WebView 参与」的零参与基线
>   已不构成可运行臂（无头路径从不挂载 WebView），如需再取该口径，只能以
>   不挂载 WebView 的路径另行构造，不在本清单范围内。
> - 日志格式变化：挂载留痕由旧 `mornlea spike overlay: 挂载 mode=GameOverlay …`
>   改为 `挂载 forced_mode=GameOverlay …`（强制档）或
>   `挂载 phase-driven(生产参与模式)留痕 …`（不强制）；相位切换行
>   `相位切换 mode=… wants_visible=… hide=… passthrough=… focus_game_view=…`
>   不变。

两态的切换点就是既有相位切换点（`MenuWebview::push_state` →
`apply_transition`）：动作表唯一真相是 `crate::overlay::plan_transition`
（生产枚举 `OverlayMode::{Menu,GameOverlay}`），强制档位只改「模式来源」；
两态动作表由 rust 侧测试钉住（`cargo test -p mornlea_client` 中的
`overlay::tests::*`）。

## 1. 启动命令

> **B.2 后基线口径**：下表「基线」为 A 组执行时的旧口径（无 WebView 参与），
> 生产化之后不再由缺省档提供。复测请以 `MORNLEA_SPIKE_OVERLAY=menu` 作为
> 基线/对照臂（见第 0 节迁移说明），S2 基线同理。

| 臂 | 命令 | 用途 |
| --- | --- | --- |
| 基线（A 组旧口径） | `make run 2>&1 \| tee /tmp/mornlea-baseline.log` | 无 WebView 参与（A 组执行时：游戏相位隐藏；现需 `MORNLEA_SPIKE_OVERLAY=menu` 复现该语义） |
| S1 对照臂 / 复测基线 | `MORNLEA_SPIKE_OVERLAY=menu make run 2>&1 \| tee /tmp/mornlea-s1-menu.log` | 子类化本身无副作用；游戏相位回退隐藏语义（**复测基线臂**） |
| S1 验证臂 | `MORNLEA_SPIKE_OVERLAY=game make run 2>&1 \| tee /tmp/mornlea-s1-game.log` | hitTest 穿透可行性（与生产游戏相位一致） |
| S2 基线（复测口径） | `MORNLEA_SPIKE_OVERLAY=menu MORNLEA_SPIKE_FPS=1 make run 2>&1 \| tee /tmp/mornlea-s2-baseline.log` | 帧耗时基线（隐藏语义对照） |
| S2 验证臂 | `MORNLEA_SPIKE_OVERLAY=game MORNLEA_SPIKE_FPS=1 make run 2>&1 \| tee /tmp/mornlea-s2-game.log` | GameOverlay 帧耗时 |

启动后先确认 stderr 里有对应臂位的 spike 日志，没出现说明环境变量没传到位：

- 验证臂 / 对照臂挂载：`mornlea spike overlay: 挂载 forced_mode=GameOverlay …`
  （或 `Menu`）；不强制时为 `挂载 phase-driven(生产参与模式)留痕 …`
  （B.2 前的格式为 `挂载 mode=…`）
- 相位切换：`mornlea spike overlay: 相位切换 mode=… wants_visible=… hide=… passthrough=… focus_game_view=…`
- 帧探针：`mornlea spike fps: 帧探针开启,present 边界每 120 帧输出摘要`，之后每约 2 秒一行
  `mornlea spike fps: frames=… frame_us(mean=… p50=… p95=… max=…) interval_us(…)`

执行环境约定：同一 worktree 一次只跑一条臂；插电、屏幕亮度固定、关闭其他
重负载应用；两臂窗口尺寸保持一致（默认预设即可）；`make run` 会先重建 Rust
dylib，属预期。

## 2. S1 输入序列清单（hitTest 分级可行性）

基线臂先跑一遍并记录行为，S1 两臂各跑一遍逐项对照。前置：启动后停在主菜单
（此刻 WebView 已挂载），点击「进入游戏」进入游戏相位；S1 验证臂进入游戏后
菜单页会降为近透明（A 组为 spike 脚手架生效；B.2 之后由生产的页面相位透明
承担，不再淡化页面内容），可看到世界与 GPU HUD。

> **B.2 后基线口径**：本节的「基线」请用 `MORNLEA_SPIKE_OVERLAY=menu`
> （旧的无 WebView 参与基线随生产化退役，见第 0 节迁移说明）。

| # | 操作 | 预期（与基线逐项一致） | 基线 | menu 臂 | game 臂 |
| --- | --- | --- | --- | --- | --- |
| 1 | WASD 移动 | 四向移动即时生效，无吞键、无延迟、无重复 | ☐ | ☐ | ☐ |
| 2 | 鼠标左键按住采掘 / 右键放置 | 采掘进度与放置行为一致；指针灵敏度与捕获状态一致（光标仍被锁定隐藏） | ☐ | ☐ | ☐ |
| 3 | 数字键 `1`-`9` 与滚轮切换快捷栏 | 选中格切换即时，物品名弹条与耐久呈现一致 | ☐ | ☐ | ☐ |
| 4 | `Enter` 进入聊天 → 输入中英文文字 → `Enter` 发送 | 文本完整入框，IME 组合不重复；聊天呈现一致 | ☐ | ☐ | ☐ |
| 5 | `Esc` 暂停 → 再 `Esc` 恢复 | 暂停覆盖层开合一次到位，无同帧回声/双投 | ☐ | ☐ | ☐ |
| 6 | `E` 打开背包 → 点击格子移动堆叠 → `Esc` 关闭 | 光标释放与重新捕获正确；点击命中格子准确 | ☐ | ☐ | ☐ |
| 7 | 暂停菜单「退回主菜单」→ 再次「进入游戏」 | 两态切换第二轮行为不变（穿透标志随相位恢复） | ☐ | ☐ | ☐ |
| 8 | 全程观察 | 无任何因 WebView 存在导致的输入异常；菜单期按钮可点、游戏期点击不落菜单 | ☐ | ☐ | ☐ |

同时记录（从各自日志文件 grep）：

```bash
grep -c "相位切换" /tmp/mornlea-s1-game.log      # 每次 进入/退出游戏 各一行
grep "相位切换" /tmp/mornlea-s1-game.log         # 逐行核对 hide/passthrough/focus 三元组
grep "挂载\|卸载" /tmp/mornlea-s1-game.log       # 各恰一行
```

S1 判定（design.md D7 判据）：

- `game` 臂每一步与基线一致 → 穿透可行性成立；
- `menu` 臂与基线完全一致 → 子类化本身无副作用（若 `game` 臂异常而 `menu`
  臂正常，问题定位在 `hitTest:` 改写；若 `menu` 臂也异常，问题在子类化）；
- 任一步不一致 → S1 判据不达成，如实记录现象与臂位，回 propose 阶段回炉。

## 3. S2 测量步骤（GameOverlay 常驻合成开销）

每臂两组、各 60 秒，均在游戏相位内进行：

> **B.2 后基线口径（复测必读）**：S2 判据是「GameOverlay 臂 ≥ 基线 × 0.95」，
> 基线必须是**非 GameOverlay 参与语义**，否则同一 GameOverlay 行为自比、判据
> 恒过且无信息量。生产化之后缺省档即生产 GameOverlay，因此复测基线请用
> `MORNLEA_SPIKE_OVERLAY=menu MORNLEA_SPIKE_FPS=1 …`（游戏相位隐藏的对照臂），
> 而不是第 1 节 A 组旧口径的裸基线命令。

1. **空载组**：进入游戏后站立不动、不碰鼠标，持续 60 秒；
2. **持续动画组**：对准方块持续按住左键采掘（方块破损进度反复推进，含快捷
   栏/弹条/准星变化），持续 60 秒。

结束后从日志提取摘要行并求稳态均值（每 120 帧一行 ≈ 2 秒一行；丢弃前 5 行
预热，其余取算术平均）：

```bash
grep "mornlea spike fps: frames=" /tmp/mornlea-s2-baseline.log > /tmp/s2-baseline.txt
grep "mornlea spike fps: frames=" /tmp/mornlea-s2-game.log    > /tmp/s2-game.txt
wc -l /tmp/s2-baseline.txt /tmp/s2-game.txt                   # 至少各 25 行(约 50s 稳态)
```

判读方法：

- 帧率证据取 `interval_us(mean=…)`；`frame_us(mean=…)` 是 present 边界的
  CPU 耗时（含 surface 获取、录制、提交与 present 调度），`p95`/`max` 是
  长尾参考；
- 对每组求各列平均后对比两臂：**判据 = GameOverlay 臂 `interval_us` 平均
  ≥ 基线 × 0.95**（交互帧率不低于无 WebView 基线 −5%）；
- `mean` 达标但 `p95` 恶化超过 10% 时，判据仍算达成，但须在 D7 记为风险项
  （长尾帧会造成可感知卡顿）；
- 功耗（只记录，不设硬门禁）：Activity Monitor 观察两臂「能耗影响」，或
  `sudo powermetrics --samplers cpu_power -i 1000 -n 60 > /tmp/pm-<臂>.txt`
  取 60 秒均值。

口径说明（回填 D7 时必须写明）：GameOverlay 验证臂合成的是**菜单页面**（经
脚手架降为近透明），不是 Phase 1 最终的稀疏 HUD 组件——本次测量偏保守上界；
若上界已达标，真实 HUD 只会更宽松；若上界不达标，需按 design D7 的回炉路径
（降采样/分层合成/局部重绘）重议，不得静默带病实施。

## 4. 结果回填模板

执行完成后把下表填好（结论另写入 `design.md` D7 对应条目，标注「已执行」、
日期与机型/系统版本）：

```markdown
### S1 hitTest 分级（执行日期：____，机型/系统：____）
- 基线臂：8 项全部符合预期（是/否，异常项：__）
- menu 对照臂：8 项与基线一致（是/否，差异：__）
- game 验证臂：8 项与基线一致（是/否，差异：__）
- 两态切换日志：进入/退出游戏各 1 行（是/否，实际：__）
- 判据结论：达成 / 不达成；备注：__

### S2 合成开销（执行日期：____，机型/系统：____）
| 组 | 臂 | interval_us mean | interval_us p95 | frame_us mean | frame_us p95 |
| --- | --- | --- | --- | --- | --- |
| 空载 | 基线 |  |  |  |  |
| 空载 | GameOverlay |  |  |  |  |
| 持续动画 | 基线 |  |  |  |  |
| 持续动画 | GameOverlay |  |  |  |  |
- 帧率判据（≥ 基线 × 0.95）：空载 __ / 持续动画 __
- 长尾（p95 恶化 >10% 记风险）：__
- 功耗观察（能耗影响/powermetrics W）：基线 __ / GameOverlay __
- 判据结论：达成 / 不达成；备注：__
```

## 5. 已知偏差与限制

- GameOverlay 验证臂呈现的是菜单页内容（近透明脚手架），非最终 HUD 组件；
  HUD 组件属本 change 任务 C.1。
- 脚手架只在 `game` 档位随游戏相位注入（`body` 背景透明、`#root` 降至
  0.08 不透明度），菜单相位撤除；`menu` 臂与生产外观完全一致。脚手架选择器
  只锚定 `index.html` 的 `body`/`#root`，前端源码零改动。游戏相位内调试面板
  （F3）会被同样压暗，属预期。
- 帧探针统计 present 边界 CPU 耗时与帧间隔，非 GPU 耗时；帧率判读以
  `interval_us` 为准。
- spike 全部入口默认关闭；benchmark/capture/`-connect` 无头路径从不挂载
  WebView，spike 不改变这一事实（挂载门与探针默认关闭由 rust 侧测试钉住：
  `window::tests::mount_gate_keeps_headless_paths_free_of_webview`、
  `overlay_spike::tests::probe_gating_defaults_to_disabled`）。
- **B.2 之后**：WebView 子类成为唯一构造路径，页面相位的透明改由生产注入
  承担，A 组的 spike 脚手架（`#root` 降至 0.08 不透明度）随生产化移除——本节
  上述两条为 A 组执行时口径，复测不再复现。

## 6. 自动化执行记录（已执行，2026-08-31）

> **B.2 后语义迁移说明**：本节记录的是 A 组执行时口径。当时「不强制」=
> 无 WebView 参与基线（裸 `WKWebView`，游戏相位隐藏）；两态参与模型生产化
> 之后，「不强制」即生产路径（游戏相位 GameOverlay 常驻可见），旧基线语义
> 需用 `MORNLEA_SPIKE_OVERLAY=menu` 强制档复现（复测口径见第 0/1/3 节）。
> 挂载留痕格式也已由本节的 `挂载 mode=GameOverlay …` 改为
> `挂载 forced_mode=…`（强制档）/ `挂载 phase-driven(生产参与模式)留痕 …`
> （不强制）；`相位切换 mode=…` 行格式不变。下表数据与结论不受影响——它们
> 比较的是「旧基线 vs GameOverlay」，迁移后等价于「`menu` 强制档 vs
> `game`/生产」。

用户裁决改为**全自动化验证**：由自驱动档（`MORNLEA_SPIKE_AUTO=1`）在进程内
驱动整套脚本——自动经 WebView `evaluateJavaScript` 点击「进入游戏」（走真实
React onClick → 桥上行 → Go 装配路径），再经 `NSApplication postEvent:atStart:`
注入合成 `NSEvent`（走真实事件管线：窗口 `hitTest` → responder 链 → winit 视图，
不需要辅助功能权限）逐项断言，随后 S2 两组各 60s 取数，落盘后进程退出。
执行前已向控制会话声明：窗口会出现并接收合成输入，期间不得触碰键鼠。

执行矩阵（同一 worktree、同一构建，逐条顺序执行，前一条进程退出后再起下一条）：

| 臂 | 命令 | 退出 | 门禁断言 |
| --- | --- | --- | --- |
| 基线 | `MORNLEA_SPIKE_AUTO=1 MORNLEA_SPIKE_FPS=1 make run` | 0 | 34/34 |
| S1-menu | `MORNLEA_SPIKE_AUTO=1 MORNLEA_SPIKE_OVERLAY=menu MORNLEA_SPIKE_FPS=1 make run` | 0 | 35/35 |
| S1-game + S2 | `MORNLEA_SPIKE_AUTO=1 MORNLEA_SPIKE_OVERLAY=game MORNLEA_SPIKE_FPS=1 make run` | 0 | 34/34 |

机型/系统：Apple M2 / macOS 26.6.2；窗口 1280×720 pt(content)。每臂一次运行
同时完成 S1 序列与 S2 两组测量（基线臂也跑 S2 作对照）。

### S1 hitTest 分级（执行日期：2026-08-31，机型/系统：Apple M2 / macOS 26.6.2）

断言口径：每条输入步骤以「自步骤基准以来」的输入计数探针增量判定（键位掩码取
累计值 + 事件数取增量），玩法层响应以相位翻转（暂停覆盖层开合）与 HUD 顶点流
字节变化（容器面板开合）佐证；桥上行事件在游戏静默相位内必须为 0
（GameOverlay 穿透断言）。

- 基线臂：门禁断言 34/34 全部符合预期（无异常项）
- menu 对照臂：门禁断言 35/35 与基线一致（无差异）
- game 验证臂：门禁断言 34/34 与基线一致（无差异）
- 两态切换日志：进入/退出游戏与暂停开合的相位切换逐行核对
  （game 臂 `hide=false passthrough=true focus_game_view=true` 与
  `hide=false passthrough=false focus_game_view=false` 交替，穿透标志随相位恢复）
- 桥上行：三臂游戏静默相位内均为 0 条（观察 9.5k–9.7k 帧）
- **判据结论：达成**（详见 `design.md` D7）

### S2 合成开销（执行日期：2026-08-31，机型/系统：Apple M2 / macOS 26.6.2）

| 组 | 臂 | interval_us mean | interval_us p95 | frame_us mean | frame_us p95 |
| --- | --- | --- | --- | --- | --- |
| 空载 | 基线 | 16666 | 17075 | 1258 | 1466 |
| 空载 | GameOverlay | 16665 | 17117 | 1270 | 1513 |
| 持续动画 | 基线 | 16665 | 17171 | 1356 | 1721 |
| 持续动画 | GameOverlay | 16665 | 17128 | 1340 | 1702 |

（µs；每组 60s、丢弃 600 帧预热后取环形窗口 2048 样本；`interval_us` 为帧率
证据，`frame_us` 为 present 边界 CPU 耗时。）

- 帧率判据（≥ 基线 × 0.95）：空载 **达成**（比值 1.0000）/ 持续动画 **达成**
  （比值 1.0000）
- 长尾（p95 恶化 >10% 记风险）：空载 +0.25%、持续动画 −0.25%，无风险项
- 功耗观察：未测量（无 `powermetrics` 授权，按既有口径不设硬门禁）
- **判据结论：达成**；备注：验证臂合成的是静态菜单页面（近透明脚手架），WebKit
  无逐帧重绘，Phase 1 逐帧 HUD 下行可能带来额外成本，C.1 落地后应以真实组件
  复测一次空载组。

### 自动化限制与最小人工兜底清单

以下三项为自动化手段限制，三臂表现一致，不影响跨臂比较，已记为非门禁观察：

1. **视角旋转**：合成 `NSEvent` 不携带 delta 字段，`DeviceEvent::Motion` 在
   NSApplication 级分发、本就不经过 WebView 的 `hitTest`。人工兜底：GameOverlay
   态下移动鼠标确认视角跟手、灵敏度与基线一致（约 10 秒）。
2. **右键放置**：`+mouseEventWithType:` 构造的右键事件被 AppKit 在进入 responder
   链之前丢弃（左键同路径正常，三臂均未观察到副键计数）。人工兜底：GameOverlay
   态下右键单击确认放置生效（约 10 秒）。
3. **「退回主菜单 → 再次进入游戏」的装配**：两次运行均未在 120s 内完成（应用
   装配行为，与 WebView 参与无关；暂停开合 ×2 已覆盖两态切换的穿透标志恢复）。
   人工兜底：GameOverlay 态下退回主菜单再进入，确认第二轮输入行为不变。

另：滚轮切换快捷栏不在输入快照契约内（Rust 快照无滚轮字段，Go 侧无消费），
本清单第 2 节第 3 项的滚轮部分无对应断言，属既有客户端能力缺口，与本 spike 无关。
