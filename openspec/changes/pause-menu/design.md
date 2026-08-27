# Design: pause-menu

## 决策记录（用户批准的边界）

2026-08-27 用户在本会话内两轮显式批准：① 认领方向选「D-02 单机暂停最小闭环」——权威 tick 冻结仅作用于单机嵌入服，TCP 远程会话只呈现本地覆盖层、不宣称暂停、协议零变更；② 菜单条目选两项方案——仅「返回游戏 / 退回主菜单」，设置入口顺延。本设计与 delta specs 不得越出该边界。

## 架构落点与数据所有权

| 单元 | 所有权 | 职责 |
| --- | --- | --- |
| 权威暂停门 | `internal/server/server.go` | 原子布尔门 + 导出 `Pause()`/`Resume()`；`RunTicks` 每个 ticker 到期先读门，置位时跳过 `step(scheduled)`（整个 tick 不存在：世界时间、随机 tick、作物、流体、实体、持久化调度全部停走）。消息接收 goroutine 与既有缓冲不受影响 |
| 暂停相位机 | `cmd/mornlea/app_menu.go` + 新增 `app_pause*.go` | `menuPhasePaused` 相位；按钮 id 常量沿用主菜单编号约定（下一空闲号）；防重入标记仿 `starting` |
| Esc 栈扩展 | `cmd/mornlea/interactive.go` 仅一处 switch 分支 | 游戏相位默认档从「仅释放光标」升级为「打开暂停菜单」（本身必须释放光标）；暂停相位下 Esc = 返回游戏。优先级保持在聊天/背包/egui 面板之后——暂停期不新开聊天，打开动作只在默认档触发 |
| 会话拆链 | 复用既有 closeClientSession 路径 | 「退回主菜单」= 先安全恢复/关闭会话（等价窗口关闭路径），再复位到 `menuPhaseMenu`，「进入游戏」可重新装配 |
| 暂停页呈现 | `mornlea_client/src/ui.rs` 及 `ui/` 新页模块 | 纯函数 render + typed action 回传，沿 settings/debug 先例；UI 下行布局段内部版本 3→4 |

## 关键取舍

1. **门安在服务端而非客户端**：客户端「停发输入」被否决——权威模拟照走不是暂停；cancel ctx 后重装配被否决——代价是世界卸载/存档抖动且不可「立刻恢复原状」。tick 级冻结是唯一同时满足「真实停止」与「恢复无损」的点。
2. **原子布尔、不加锁**：`RunTicks` 生产者是单一 goroutine，门只需 `atomic.Bool` 的 Load；`Pause()`/`Resume()` 幂等，重复调用无副作用。
3. **TCP 会话同一菜单、不同标注**：复用同一条呈现路径，仅在页面追加注明行（Go 按传输形态下发标志位/文案），不在远程路径上调用任何服务端 API。
4. **页面级布局版本**：沿既有先例每个页面携带独立布局版本常量（主菜单 1、设置 2、调试 3），本变更新增 `UI_PAUSE_LAYOUT_VERSION = 4`，既有三个常量与下线格式不动；客户端 ABI 保持 v9 不变。未开启暂停时下行走既有布局，零成本。
5. **跨语言动作编号契约（控制会话钉死，T2/T3 共同遵守）**：「返回游戏」= 8、「退回主菜单」= 9，延续主菜单按钮动作表 1..7 之后且互不重叠；Esc 键与「返回游戏」产生同一编号 8（沿设置页 Esc≡返回(7) 先例）。Task 2 在 Rust 侧建立常量并用测试钉住；Task 3 在 Go 侧建立同值常量（沿用 `menuAction*` 命名族）并消费事件。
5. **golden 零变化承诺**：暂停页不出现在任何 capture 场景中，视觉基线不动；阶段 4 以 `make visual-check` 断言不变。

## 测试矩阵

| 层 | 内容 |
| --- | --- |
| `internal/server` | 门红绿测试：Pause 后连续调度 step 世界时间 ticks 不变、Resume 续接增量且确定性重放一致；Pause/Resume 幂等 |
| `cmd/mornlea` | 相位机转换（开/关/退回主菜单）、Esc 栈新档位、防重入、TCP 形态注明标志、teardown 复用既有拆链（既有测试不改语义） |
| `mornlea_client` | 暂停页 render 纯函数测试（按钮列、注明行有无两个分支）、Escape→back 动作映射、layout v4 解码回归 |

## 受影响文件

`internal/server/server.go`、`internal/server/*_test.go`（新增暂停门测试）；`cmd/mornlea/app_menu.go`、`interactive.go`、新增 `app_pause*.go` 与测试、宿主装配微调；`engine/crates/mornlea_client/src/ui.rs`、`src/ui/` 新增页模块与测试；OpenSpec change 目录。

## 兼容 / 风险 / 回退

- 无协议/schema/ABI 升版，无存档格式变化；旧存档在暂停语义下行为不变（暂停只是不推进）。
- 风险：Esc 栈插点若破坏既有聊天/背包/面板路径由定点回归测试兜底；暂停期间到达的网络命令在恢复后按序结算，暂停时长为人工尺度，不存在无界堆积的新来源。
- 回退：整分支单 revert 即可，无持久化或线上兼容包袱。
