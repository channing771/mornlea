# Task 6 report — viewmodel 世界烘焙修复

- Status: DONE（6.1/6.2 实现 + 验证闭环；一条已隔离归因的 capture 像素探针失败作为预期后续，见 Concerns-1）
- Commit: 见回消息（单行英文，无正文无签名）
- Gate 摘要：落点测试先红（编译红 + 行为红双证据）后绿 → `render` 全绿 → `app` 全绿 → `audit` 全绿 → Task 1/2 回归绿（含 `client` 帧基线）→ Rust 零改动 → capture 仅 bed-night 探针红（已隔离证明是修复生效的预期像素变化，见 §5）

## 1. 根因复述（Task 5 §3 的裁决落地）

Go 以 `root=Ident4()` 把相机空间偏移直接 bake 进实例 mat4，Rust 用世界 VP
投影（`clip = view_proj * instance * local`）：双手恒钉在世界原点附近
（y=-0.45 地面之下），屏幕上永远不可见。按 design 裁决修复：Go 按本帧相机
位姿烘焙世界变换，Rust 零改动——`ViewmodelInput` 增相机位姿（位置 +
yaw/pitch），根变换由相机位姿派生，既有相机空间偏移经根变换烘焙为世界变换
后走既有世界 VP。

## 2. 改动

- `packages/client/render/viewmodel.go`
  - `ViewmodelInput` 增 `CamPos mgl32.Vec3`、`CamYaw`、`CamPitch`（本帧呈现
    相机位姿；零值即旧链单位根，重放比较须连同位姿固定）。
  - 新增 `viewmodelRootFromCameraPose`：根 = 平移（位姿）× 偏航绕 Y × 俯仰
    绕 X。旋转顺序与展示相机朝向公式同构；scratch 逐字验算 5 组位姿（含
    Task 5 两个见证相机）下与同位姿 `LookAtV` 视图逆的最大元差
    ≤3.81e-06（float32 噪声级），故采用无退化、无逐帧求逆的闭形。
  - `buildViewmodelParts` 根由 `Ident4` 改为该函数；挥动旋转仍在相机空间绕
    臂根发生，再随刚体根整体落到世界。相位/三形态/六档/重置语义零改动。
- `packages/client/cmd/mornlea/app/app_viewmodel.go`
  - `deriveViewmodelInput` 直通本帧呈现相机位姿（`a.camera` 即帧循环的
    `cam`：非全景相位两者同一指针，全景相位返回 nil 无需位姿，故无需改帧
    接线行签名）。
  - 新增 `ResetViewmodel`（转调编码器重置，供场景清场；会话重置与权威
    reset 仍直调编码器重置，同语义）。
- `packages/client/cmd/mornlea/capture/scene_application.go`：`SceneApplication`
  增 `ResetViewmodel`（capture 实际消费的最小面；`windowedCaptureApp` 内嵌
  接口不受影响）。
- `packages/client/cmd/mornlea/capture/capture_scene.go`：
  `resetCapturePresentation` 在 `ResetCombatFeedback` 同落点调
  `ResetViewmodel`，关闭 C3（Task 5 §4 前瞻警示的跨场景重置）。

## 3. 落点测试 oracle 与红绿证据

新文件 `packages/client/render/viewmodel_projection_test.go`（世界烘焙投影主题）：

- Oracle 方法：固定相机（位置 (10,3,10)/yaw0/pitch-0.1，远离原点）+ 逐字
  复用的相机数学（`Forward` 朝向公式 → `LookAtV` 视图 → `core.Perspective`
  投影按 `ViewProj` 同序组合，480×480 帧）作为投影 oracle，不反向依赖展示
  相机所在包（`render` 低层方向 + 审计包边门禁）。
- `TestViewmodelLandingProjectionOnScreen`：中立双手实例中心须距相机
  1.5m 内、w>0、NDC 落屏内、左右手分居 NDC/像素左右两半。
- `TestViewmodelZeroPoseRootIsIdentity`：零位姿根为单位阵（Task 1/2 零位姿
  回归口径走旧链）。
- `TestViewmodelPoseEntersReplayBytes`：同位姿同输入逐字节一致，相机平移
  字节必变（位姿死亡即红）。
- 红证据（stash 隔离实测）：① 编译红（`CamPos` 未定义）；② 旧根行为红——
  第 0 只手距相机 15.32 米（原点钉住）vs 想要 1.5 米内。绿证据：实现后三测
  试全过。

app 侧 `app_viewmodel_test.go` 新增：

- `TestDeriveViewmodelInputCarriesCameraPose`（逐帧位姿直通 + 进字节）。
- `TestSceneFirstFrameNeutralAfterViewmodelReset`（场景首帧锁定：清场落点
  重置后，旧窗不延续、新沿重开，首帧与新编码器逐字节一致；修测试时发现
  窗龄计数陷阱——首帧期望须取新编码器首编码而非二次编码，实现未动）。

## 4. 回归命令与输出

| 命令 | 结果 |
|---|---|
| `go test ./packages/client/render -race -count=1` | ok（全绿，含 Task 1 全部 26+ 相位/重放测试与新增 3 落点测试） |
| `go test ./packages/client/cmd/mornlea/app -race -count=1` | ok（67s，全绿，含 Task 2 接线门限与新增 2 装配测试） |
| `go test ./packages/client/client -race -count=1 -run TestEncodeRenderFrameWithoutViewmodel` | PASS（Task 1 帧级空段基线：无输入帧逐字节不动） |
| `go test ./packages/audit -count=1` | ok（含包边/注释标识符门禁；中途因测试注释反引号外来标识符红一次，已改 plain text 后绿） |
| `go vet` + `gofmt -l`（三包） | 干净 |
| `git status` Rust 文件 | 零修改（`packages/engine` 无条目） |

## 5. 变更文件

- 改：`packages/client/render/viewmodel.go`、`packages/client/cmd/mornlea/app/app_viewmodel.go`、
  `packages/client/cmd/mornlea/capture/capture_scene.go`、
  `packages/client/cmd/mornlea/capture/scene_application.go`
- 增：`packages/client/render/viewmodel_projection_test.go`、
  `app_viewmodel_test.go` 内 2 测试（同文件主题内加法）
- 未碰：`app_frame.go` 接线行（位姿经 `a.camera` 直通，签名不动）、ABI/协议/存档/golden、`progress.md`/`ledger.md`

## 6. 自评

- TDD 纪律完整：编译红 + 行为红（15.32m）双证据后才实现；根变换数学先经
  scratch 对 `LookAtV` 逆逐字验算再落码，不靠手算断言。
- 最小 diff：渲染语义只动根一行，相位/形态/档位/重置逻辑零触碰；零位姿退
  化保证全部历史测试走旧链。
- 隔离定位代替猜测：bed-night 失败经“旧根 + 其余全保留”对照证明源于双手
  在屏（修复生效），而非重置接线误伤。

## Concerns

1. （预期后续，非本任务范围）`TestBedNightScenePixelsShowMultiOrientationBedsAtNight`
   在本修复下失败（东西向床头亮带探针）：bed-night 相机近处布床 + 清场确认
   空背包 → 中立双手按设计在屏并遮挡探针像素。隔离证据：旧根 + 其余改动全
   保留时该测试通过。探针重定位/golden 重拍（`visual-update` + `visual-check`
   全绿）须另起收尾步骤并经人工逐图确认，本任务未动任何基线。
2. 已知限制（design 已记录）：相机贴墙/穿几何时手可能被世界裁剪，实现期不
   处理，后续 change 另起通道。
3. scenario 保持 v22（沿 Task 5 §5 结论：固定输入/被测世界/分辨率全不动，
   性能数值只记录；但逐帧字节已变——升版必要性待收尾 change 按升级纪律重裁）。
