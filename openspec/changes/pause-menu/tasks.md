# Tasks: pause-menu

> 每个任务由全新 implementer 子代理执行（brief 为唯一需求来源）；TDD 红绿循环。

## 1. 权威暂停门（`internal/server`）

- [x] 1.1 先写失败测试：新建 `internal/server/pause_test.go`——Pause 后连续多个调度周期世界时间 ticks 不变、重复 Pause 幂等、Resume 续接增量、同种子重放对照暂停段不改变后续结算结果。
- [x] 1.2 `internal/server/server.go` 实现原子暂停门并导出 `Pause()`/`Resume()`；`RunTicks` 每 ticker 到期先读门，置位时跳过本周期 `step(scheduled)`。
- [x] 1.3 验证：`go test ./internal/server -race -count=1` 全绿且既有测试零改动语义漂移。

## 2. Rust 暂停页（`mornlea_client`）

- [x] 2.1 先写失败测试：`ui/` 新增暂停页模块的 render 纯函数测试——「返回游戏」「退回主菜单」按钮列可判定；注明行按远程标志两分支呈现；Escape 产生与「返回游戏」相同的 back typed action。
- [x] 2.2 实现 UI 下行布局段内部版本 3→4 的暂停页编码侧约定（Go 编码 + Rust 解码），客户端 ABI 保持 v9；未开启暂停时下行走既有布局零成本。沿 D-03 调试面板的 Go/Rust 两侧同步纪律：同一 Task 内改齐两侧版本常量与解码回归测试。
- [x] 2.3 验证：Rust 侧 `cargo test -p mornlea_client`；Go 侧涉及文件若为独立包则 `go test ./internal/client -race -count=1`（以实际触碰为准）。本执行：Rust 侧全绿（167 passed / 0 failed，含新增 8 个暂停页测试）；**Go 侧零改动**，Go 侧命令不适用——Go 同值动作常量与编码按 design「关键取舍」第 5 条由接线任务建立后消费。

## 3. 相位机与接线（`cmd/mornlea`）

- [ ] 3.1 先写失败测试：`menuPhasePaused` 转换矩阵（Esc 开 / Esc 或按钮关、防重入只恢复一次）、退回主菜单复用会话拆链后主菜单可再次装配、TCP 形态下注明标志下发且不调用本地服务端暂停接口。
- [ ] 3.2 `app_menu.go` 增相位与按钮 id 常量；新增 `app_pause*.go` 承载暂停状态与动作处理；`interactive.go` 仅在 Esc switch 默认档接入打开动作、暂停档处理关闭；宿主装配处把嵌入服的门暴露给相位机。
- [ ] 3.3 验证：`go test ./cmd/mornlea -race -count=1`。

## 4. 收尾（实现者+终审）

- [ ] 4.1 `gofmt -l .` 无输出；`go vet ./...` 干净；`go test ./... -race` 通过。
- [ ] 4.2 `make rust` 与 `make rust-check` 通过；`make visual-check` 断言视觉基线零变化（暂停页不出现在任何场景）。
- [ ] 4.3 `openspec validate --all --strict --no-interactive` 通过；本表全部勾选核对。
- [ ] 4.4 ledger 记录评审结论、终审证据与最终裁决；未决项誊入 proposal「延期与放弃」（设置入口顺延已记，其余若发现补录）。
