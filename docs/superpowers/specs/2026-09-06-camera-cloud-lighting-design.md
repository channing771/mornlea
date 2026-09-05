# 相机迁 Rust × 云层风格化增强 × 光线物理化升级并行设计

- 日期：2026-09-06
- 编排：方案 A（三轨并行：3 worktree + 3 OpenSpec change + 共享契约冻结 + 集成分支）
- 状态：5 节设计已逐节通过，待用户评审本文件后转 `writing-plans`

## 1. 背景与目标

游戏中三处相互独立但同属渲染呈现的痛点需要一次性规划、分轨并行解决：

1. 性能：`packages/client/client/camera.go`（51 行纯 Go，`Forward/Rotate/Move/ViewProj` 经 `mgl32`）与 `packages/client/mesh/visibility.go` 的 `VisibleSectionsInto`（每帧 BFS + frustum）在渲染热路径上产生 Go 分配，受 GC 影响掉帧。
2. 云层：`packages/engine/crates/mornlea_client/shaders/sky.wgsl` 的 `cloud_mask` 是单层 `y=192` 平面十字形 hash 方块云（4×4 macro + 二值 mask），无 FBM、无厚度与光照，只有 `MacroX/Local` 漂移动画，观感不逼真。
3. 光线：`packages/client/render/daylight.go` 的 `DayNightAt`（`daylight=0.15+0.85*sun` 正弦）与 `terrain/water.wgsl` 的 `0.08+sky*(daylight-0.08)` 加固定面折扣（顶 1.0／底 0.5／侧 0.68／0.84），无色温、无 tone mapping、无半球环境，`lod.wgsl` 之外无雾化，光感生硬。

用户决策：三轨并行、由本会话自行编排；相机连可见性一起搬；云层做风格化增强；光线做物理化升级。

非目标：不搬 `ComputeConnectivity`（mesher 侧低频）；云不做双层与 raymarch；光不做阴影 map 与体积光；不碰 `mornlea_engine` 的体素光照 BFS（`light.rs` 传播不变）。

## 2. 总编排与共享契约冻结

三个独立 OpenSpec change（`camera-rust-migration`／`cloud-stylized-plus`／`lighting-physical`），各占一个 git worktree 并行开发，收尾一条集成分支依次 rebase。

先冻结共享契约（作为三轨的第一步前置任务完成，不以自然日为门禁），冻结物：

- `RenderFrame` header 192B 布局（`render.go` 的 `EncodeRenderFrame` 与 Rust `FRAME_HEADER_BYTES` 对应关系）；
- `sky` uniform（`view_proj_inv`／`sun_daylight`／`star_visibility`／`cloud_macro_x`／`camera_cloud`）；
- `terrain/water/lod` 的 shade 公式版本号。

冻结期间三轨只读契约。`sky.wgsl` 按区域定 owner 避免同文件冲突：云函数（`cloud_hash/cloud_mask` 及新增 FBM）归云轨，天空渐变与日月盘归光轨。`terrain/water/lod.wgsl` 归光轨，相机与可见性相关 Go/Rust 文件归相机轨。

每轨按 `subagent-driven-development` 执行（一轮开发一轮审查，ledger 记进度与裁决），控制会话不直接实现。被否决的替代方案：B 单分支串行（无冲突但 3 倍耗时，违背并行要求）；C 单大 change 三轨道（评审与回退不可分，违背最小闭环）。

## 3. Track1 相机与可见性迁 Rust（client ABI v15→v16）

架构：`mornlea_client` 新增相机内核模块，持有 `Forward/Rotate/Move/ViewProj/FrustumFrom` 纯函数与 `VisibleSections`（BFS 连通性＋frustum＋radius，含 Rust 侧复用缓冲，语义对齐 Go 的 `VisibilityScratch`）。Go 侧 `client.Camera` 与 `mesh.VisibleSectionsInto` 退化为薄封装（参数校验＋FFI＋解码），`app_frame.go` 调用点形状不变（仍产出 `viewProj/viewProjInv/rustVisible`）。

数据流：输入（`Pos/Yaw/Pitch/FovY/Aspect/Near/Far` ＋ `origin/radius/connectivity lookup`）→ Rust 内核 → 输出（`viewProj/viewProjInv/frustum` ＋有序 `Visible` 列表）→ 既有 `RenderFrame` 编码。`Connectivity` 编码、`Frustum` 平面顺序、`sectionAABB` 与 BFS 遍历顺序逐字节对齐 Go，保证可见列表确定性一致。

并发与所有权：内核无跨调用可变状态，复用缓冲由调用方持有并传入；FFI 不保留对方指针，调用结束后两侧不持有对方内存。Go 热路径门禁（有界工作、不阻塞 I/O）保持。

错误处理：ABI 版本错、pointer／length／容量非法转稳定状态码，Go 侧 panic（编程错误）或显式失败；失败不写部分 `Visible`，不发布部分帧。

测试：Rust 单测钉住与 Go 的逐字节一致性（固定种子相机位姿＋固定 connectivity 图的 golden 列表）；Go 侧保留薄封装校验测试；benchmark scenario v22 跑前后帧时间与分配计数对比（目标：热路径 Go 分配清零、P99 不回归）。

## 4. Track2 云层风格化增强（不升 ABI）

架构：只改 `sky.wgsl` 云函数与 `CloudOffsetAt` 语义说明，不加 frame 新字段，不升 client ABI。

组件：3 octave value-noise FBM（由既有 `hash_cell` 派生，octave 数由常量钉死为 3）× macro coverage 调制 × 密度边缘羽化（`smoothstep`）；厚度感用伪法线点乘日光的亮顶暗底；日出日落染橙复用 `sun_daylight`；动画保留 `MacroX/Local` 双偏移（低频漂移＋高频细节流速差）。

数据流：`worldTime → CloudOffsetAt → CloudMacroX/Local → sky uniform → cloud_mask → 与天空色 mix`，链路不变，只是 mask 从二值变为连续密度。

性能与约束：纯 ALU、无纹理采样、无每帧分配；octave 数由常量钉死；预热后热路径不建资源。`y>=192` 或 `direction.y<=0.001` 早出保留。

错误处理：无新失败面；密度钳制在 0..1，颜色钳制后输出。

测试：`sky` shader 非空与入口点单测；capture 云场景 golden 更新一次；`CloudOffset` 既有单测保持通过。

## 5. Track3 光线物理化升级（frame 语义＋shader，golden 统一大改）

架构：改 `render/daylight.go` 曲线与 `terrain/water/lod/sky.wgsl` 着色，不碰体素光照传播。

组件：

- `DayNightAt`：日光色温近似混色（按太阳高度角 mix 暖橙与正午白，非真实黑体辐射）、昼夜曲线加晨昏 shoulder（`smoothstep` 压缩两端）、`ClearColor` 与雾色同源。
- `terrain/water.wgsl`：`face_shade×AO×base` 后加半球环境（按法线 y mix 天空色与地面色）、线性空间做光照后经 ACES 近似 tone map 再输出（atlas 视为 sRGB 解码后线性）。
- `lod.wgsl`：距离雾加高度衰减混合。
- `sky.wgsl`（光轨区域）：天空渐变与太阳盘加前向散射光晕。

数据流：`worldTime＋phase offset → DayNight（Sun/Daylight/ClearColor/SunDirection/StarVisibility）→ frame uniform → 各 pass 着色`，链路不变，数值语义升级。

错误处理：曲线输出钳制（`daylight` 0..1、颜色 0..1）；雾参数 `full>start>0` 由既有 setter 保证；失败不写部分帧。

测试：`daylight_test.go` 更新曲线 golden；shader 单测；capture 全场景 golden 统一更新一次并在 change 内说明双阈值结果；`make visual-check` 人审晨昏与夜景。

## 6. 文件影响面

- Track1：`packages/engine/crates/mornlea_client/src/` 新增相机与可见性内核、`packages/engine/include/mornlea_client.h`、`packages/client/client/`（camera／bridge／ABI 版本与跨语言测试）、`packages/client/mesh/visibility.go` 薄封装、`packages/client/cmd/mornlea/app/app_frame.go` 适配。
- Track2：`packages/engine/crates/mornlea_client/shaders/sky.wgsl` 云区、`packages/client/render/daylight.go` 的 `CloudOffsetAt` 说明、`capture` 云场景 golden。
- Track3：`packages/client/render/daylight.go`、`shaders/terrain.wgsl`、`shaders/water.wgsl`、`shaders/lod.wgsl`、`shaders/sky.wgsl` 天空区、`capture` 全场景 golden。
- 版本：Track1 升 client ABI（v15→v16）并同步根版本矩阵与 `packages/audit` 基线；Track2 不升 ABI；Track3 是否升 ABI 视 frame 是否加字段而定（首选不加字段、不升 ABI）。

## 7. 验证门禁（每轨）

先写失败测试，再最小实现。定点：`make rust`、`go test ./packages/client/... -race -count=1`（按触及包替换路径）、`cargo test -p mornlea_client --locked`、`cargo test -p mornlea_engine --locked`（Track3 回归体素光照未被触碰）。收尾：`make dev-check`（或 `scripts/agents/gates.sh` 入口）、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`。Track1 加 benchmark 前后对比；Track2／3 加 `make visual-check`。真实 overflow、数据丢失、报告身份不完整与 I/O 错误保持硬失败，不放宽。自动测试不启动前台游戏窗口。

## 8. 风险与回退

- Track1 风险：遍历顺序或浮点差异导致可见列表抖动。缓解：golden 列表＋capture 对比；回退：Go 薄封装切回本地实现（ABI v16 保留，内核停用）。
- Track2 风险：FBM 过重致低端 GPU 掉帧。缓解：octave 常量＋capture 帧时间；回退：云函数常量切回二值 mask。
- Track3 风险：tone map 让全场景 golden 大面积变红。缓解：一次统一更新＋人审；回退：曲线参数回退到旧公式（shader 分支常量）。
- 集成风险：三轨同改 `sky.wgsl`。缓解：区域 owner＋集成分支串行 rebase＋`visual-check`。
