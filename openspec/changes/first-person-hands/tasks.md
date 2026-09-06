## 1. Go viewmodel 编码（TDD）

- [ ] 1.1 双手与持物编码：新建 `packages/client/render/viewmodel*.go`（双手 cuboid 与臂样式同源三值、手色 `avatarShade(base, 0.82)`、臂层头层 `+12`）+ 右手三形态（空/方块微缩立方/物品扁长条，只认已确认选中）+ 工具六档参数表（斧铲取镐默认并注释）+ 挥动相位纯函数（`(tick, 档, 触发沿)`，无墙钟，重放一致，tick 回退重锚）+ 同包主题测试；验证 `go test ./packages/client/render -race -count=1`。
- [ ] 1.2 空输入零影响：无 viewmodel 输入时编码输出为空，`EncodeRenderFrame` 帧字节与变更前逐字节一致的回归测试；验证同 1.1。

## 2. 跨语言 client ABI v17 同步（同一任务组改齐）

- [ ] 2.1 header/Rust/Go 三处 ABI 常量 16→17 + frame viewmodel TLV tag（下一个空闲值）+ Go `EncodeRenderFrame` 编解码 + Rust ffi 解码 + 三端一致性测试 + v16 拒绝测试；验证 `go test ./packages/client/client -race -count=1`、Rust `cargo test -p mornlea_client viewmodel` 与 `go test ./packages/audit -count=1`。

## 3. Rust viewmodel 绘制 pass

- [ ] 3.1 相机空间叠加绘制（双手 + 持物，实例恒 ≤4，复用 avatar 实例布局与材质分支纪律）+ 无段帧 draw 选择不变 + `#[cfg(test)]` 主题测试；验证 `cargo test -p mornlea_client` 相关主题测试与既有 render 测试全绿。

## 4. app 装配（TDD）

- [ ] 4.1 从已确认 `Hotbar()` + `miningOverlay` + `combatFeedback` 派生 viewmodel 输入（确认纪律、相位门控、churn 时中立回落）并接入 `RenderFrame`，只加 `app_viewmodel*.go` 与 frame 接线行；验证 `go test ./packages/client/cmd/mornlea/app -race -count=1`。
- [ ] 4.2 计数门：viewmodel 实例计入帧预算校验，超限帧稳定拒绝而非截断绘制；验证同 4.1。

## 5. 基线与收尾门禁

- [ ] 5.1 capture golden 更新并逐图人工复核（画面新增双手为预期差异，其余像素零差异）；确认 benchmark scenario 是否需升版并记录裁决；执行 gofmt、六模块 `go vet`（或 `make dev-check`）、`make test-race`、`openspec validate --all --strict --no-interactive` 与整分支终审；结果和裁决写入 ledger，不推送、不合并。
