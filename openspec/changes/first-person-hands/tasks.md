## 1. Go viewmodel 编码（TDD）

- [x] 1.1 双手与持物编码：新建 `packages/client/render/viewmodel*.go`（双手 cuboid 与臂样式同源三值、手色 `avatarShade(base, 0.82)`、臂层头层 `+12`）+ 右手三形态（空/方块微缩立方/物品扁长条，只认已确认选中）+ 工具六档参数表（斧铲取镐默认并注释）+ 挥动相位纯函数（`(tick, 档, 触发沿)`，无墙钟，重放一致，tick 回退重锚）+ 同包主题测试；验证 `go test ./packages/client/render -race -count=1`。
- [x] 1.2 空输入零影响：无 viewmodel 输入时编码输出为空，`EncodeRenderFrame` 帧字节与变更前逐字节一致的回归测试；验证同 1.1。

## 2. 跨语言 client ABI v17 同步（同一任务组改齐）

- [x] 2.1 header/Rust/Go 三处 ABI 常量 16→17 + frame viewmodel TLV tag（下一个空闲值）+ Go `EncodeRenderFrame` 编解码 + Rust ffi 解码 + 三端一致性测试 + v16 拒绝测试；验证 `go test ./packages/client/client -race -count=1`、Rust `cargo test -p mornlea_client viewmodel` 与 `go test ./packages/audit -count=1`。

## 3. Rust viewmodel 绘制 pass

- [x] 3.1 相机空间叠加绘制（双手 + 持物，实例恒 ≤4，复用 avatar 实例布局与材质分支纪律；超限走整帧 `Invalid` 拒绝，与 avatar/drop/轮廓/裂纹的 `validate_frame` 纪律同形，见 design.md Decisions §2 裁决）+ 无段帧 draw 选择不变 + `#[cfg(test)]` 主题测试；验证 `cargo test -p mornlea_client` 相关主题测试与既有 render 测试全绿。

## 4. app 装配（TDD）

- [x] 4.1 从已确认 `Hotbar()` + `miningOverlay` + `combatFeedback` 派生 viewmodel 输入（确认纪律、相位门控、churn 时中立回落）并接入 `RenderFrame`，只加 `app_viewmodel*.go` 与 frame 接线行；验证 `go test ./packages/client/cmd/mornlea/app -race -count=1`。
- [x] 4.2 计数门：viewmodel 实例计入帧预算校验（Go 编码侧恒 ≤4，生产不可达超限；Rust 侧超限整帧拒绝见 design.md Decisions §2），超限帧稳定拒绝而非截断绘制；验证同 4.1。

## 5. 基线与收尾门禁（任务 7 后再次重做）

- [x] 5.1 capture golden 更新并逐图人工复核（画面新增双手为预期差异，其余像素零差异）；确认 benchmark scenario 是否需升版并记录裁决；执行 gofmt、六模块 `go vet`（或 `make dev-check`）、`make test-race`、`openspec validate --all --strict --no-interactive` 与整分支终审；结果和裁决写入 ledger，不推送、不合并。

- [x] 5.1 capture golden 更新并逐图人工复核（画面新增双手为预期差异，其余像素零差异）；确认 benchmark scenario 是否需升版并记录裁决；执行 gofmt、六模块 `go vet`（或 `make dev-check`）、`make test-race`、`openspec validate --all --strict --no-interactive` 与整分支终审；结果和裁决写入 ledger，不推送、不合并。

## 6. viewmodel 世界烘焙修复（任务 5 暴露的接缝 bug，设计裁决见 design.md Decisions §1）

- [x] 6.1 Go 编码改世界烘焙：`ViewmodelInput` 增相机位姿（位置 + yaw/pitch），根变换由相机位姿派生、既有相机空间偏移经根变换烘焙为世界变换，相位/三形态/六档/重置语义不动；加投影落点测试（固定相机下双手落在屏幕左右区域，断言 NDC/像素区间）；Rust 零改动；验证 `go test ./packages/client/render -race -count=1`。
- [x] 6.2 app 装配跟进：逐帧传入相机位姿 + `resetCapturePresentation` 调 viewmodel 重置（含场景首帧锁定测试，关闭任务 4 遗留 C3）；验证 `go test ./packages/client/cmd/mornlea/app -race -count=1` 与 `go test ./packages/audit -count=1`。

## 7. 斜持姿态与视觉基线（用户评审追加）

- [x] 7.1 斜持姿态：双手改自左下/右下屏角斜向入画（向中心倾斜 20°–35°，右手为主手），世界烘焙根变换多乘斜持旋转；以 capture 实拍 PNG 目检迭代锁定（≤5 轮，逐轮记录数值与结论），锁定后常量 + 落点测试（含斜持倾角断言）钉死；验证 `go test ./packages/client/render -race -count=1`。
- [x] 7.2 静态基线四景：`captureScenes` 尾部（`water-underwater` 之前）追加 `hand-tool`/`hand-block`/`hand-mining`/`hand-attack` + README 索引 + 顺序测试同步；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1`。
- [x] 7.3 动作 GIF 两剧本：`hand-mining`/`hand-attack` 复用 motion 录制循环 + `--motion-scene` 白名单扩展 + options 测试同步，GIF 入库不进比对；验证 `go test ./packages/client/cmd/mornlea -count=1` 与 capture 包测试。
- [x] 7.4 任务 5 再次重做（姿态改写既有含手 golden）：`visual-update` + `visual-check` + 全量门禁重跑，见 §5。

## 8. HUD 一体与静态禁手（用户二次评审，与任务 7 合并评审）

- [x] 8.1 生产门：`deriveViewmodelInput` 在背包/容器打开或非游戏相位时返回空 + 测试（开包当帧消失、关包恢复）；HUD 本体不动；验证 `go test ./packages/client/cmd/mornlea/app -race -count=1`。
- [x] 8.2 静态表全局禁手：capture 静态 runner 单点抑制（GIF/motion runner 不抑制）+ 锁定测试；撤销任务 7 的静态四景（场景表、README、顺序测试回退）；验证 capture 包测试。
- [x] 8.3 passive-death 并入 motion：GIF 迁入 `motion/` + 像素比对退役（保留生成）+ README 同步；验证相关测试调整。
- [x] 8.4 world golden 恢复本 change 前基线并 `visual-check` 全绿（含手场景逐字节一致即抑制成立）；任务 5 第三次重做全量门禁，见 §5。
