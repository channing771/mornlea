# 相机与可见性迁 Rust 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把每帧热路径的相机数学与可见 section BFS（含 frustum）迁入 `mornlea_client` Rust 内核，Go 只留薄封装，热路径 Go 分配清零。

**Architecture:** `mornlea_client` 新增 `camera.rs`（纯数学）与 `visibility.rs`（BFS＋frustum），经两个新 client ABI v16 FFI 出口暴露；`packages/client/client` 新增 darwin 门控 bridge，`app_frame.go` 调用点形状不变；`mesh` 保留纯 Go 参考实现供非 darwin 构建与交叉验证。

**Tech Stack:** Go 1.26（mgl32 列主序）、Rust 1.97.1（--locked）、C ABI、wgpu 无新增（纯 CPU 内核）。

**Spec:** `docs/superpowers/specs/2026-09-06-camera-cloud-lighting-design.md` 第 3 节（Track1）

## Global Constraints

- Rust 工具链 1.97.1，不升级 compiler／Cargo lock／wgpu 线。
- 任何 Go 包不得导入 WebGPU 绑定；GPU 渲染仍由 Rust client 独占。
- client C ABI 只由 `packages/client/client` 接触；header、Rust FFI、Go bridge、ABI 版本、跨语言测试成套演进；当前 v15→v16。
- 生产路径无 Go fallback：darwin 走 Rust 内核；`mesh` 的纯 Go 实现是参考实现与非 darwin 构建支撑，不是生产旁路。
- `extern "C"` 不得让 panic 穿过 FFI；先校验 ABI、pointer、length、alignment、overlap、容量；失败不写部分输出。
- Rust 导出项用中文 `///` doc comment（安全前提、所有权、失败语义、ABI 同步面）；Go 注释中文，反引号包裹标识符。
- 跨 goroutine 发送成功后的消息及其 slice 视为不可变；热路径有界、无阻塞 I／O、无每帧堆分配（Rust 侧复用缓冲由调用方传入）。
- 提交信息单行英文 `<type>(<scope>): <subject>`；注释禁任务编号。
- 根 `AGENTS.md` 版本矩阵 client ABI v15→v16 与 `packages/audit` 基线同步更新。

---

### Task 1: Rust 相机数学内核与 golden  parity

**Files:**
- Create: `packages/engine/crates/mornlea_client/src/camera.rs`
- Modify: `packages/engine/crates/mornlea_client/src/lib.rs`（`pub mod camera;`）
- Create: `packages/client/client/camera_golden_test.go`（Go 侧输出固定向量）

**Interfaces:**
- Consumes: `core.Perspective`（WebGPU [0,1] 深度）、`Camera.Forward/Rotate/ViewProj` 语义。
- Produces: `camera::forward(yaw,pitch)->[f32;3]`、`camera::view_proj(pos,yaw,pitch,fov_y,aspect,near,far)->[f32;16]`（列主序，与 mgl32 内存布局一致）、`camera::frustum_from(view_proj)->[[f32;4];6]`。

- [ ] **Step 1: 写 Go golden 输出测试**

```go
// packages/client/client/camera_golden_test.go
func TestCameraGoldenVectors(t *testing.T) {
	cam := &Camera{Pos: mgl32.Vec3{1.5, 65.25, -3.75}, Yaw: 0.7, Pitch: -0.25, FovY: 1.22173, Aspect: 16.0 / 9.0, Near: 0.1, Far: 1536}
	t.Logf("FORWARD %.8f %.8f %.8f", cam.Forward()[0], cam.Forward()[1], cam.Forward()[2])
	vp := cam.ViewProj()
	t.Logf("VIEWPROJ %.8f %.8f %.8f %.8f", vp[0], vp[5], vp[10], vp[15])
}
```

- [ ] **Step 2: 运行并记录输出为 Rust 断言真值**

Run: `go test ./packages/client/client -run TestCameraGoldenVectors -v -count=1`
Expected: PASS，把打印的 7 个数值逐字抄入 Rust 测试。

- [ ] **Step 3: 实现 Rust 内核（公式与 Go 逐行对应）**

```rust
/// 相机前向，语义与 Go `Camera::Forward` 一致：yaw=0、pitch=0 时朝向 -Z。
pub fn forward(yaw: f32, pitch: f32) -> [f32; 3] {
    let cp = pitch.cos();
    [-yaw.sin() * cp, pitch.sin(), -yaw.cos() * cp]
}
```

`perspective` 逐字照抄 `core.Perspective`（`f=1/tan(fov/2)`，`inv=1/(near-far)`，矩阵 `[f/aspect,0,0,0, 0,f,0,0, 0,0,far*inv,-1, 0,0,far*near*inv,0]` 列主序）。`view` 用 LookAt：以 mgl32 `LookAtV` 源码（module cache）为准转写，并在 Rust 测试中用 Step 2 的 golden 向量钉死（容差 1e-5）。`frustum_from` 逐字照抄 `core.FrustumFrom`：行提取 `row(i)=[m[i],m[4+i],m[8+i],m[12+i]]`，左＝r3＋r0、右＝r3－r0、下＝r3＋r1、上＝r3－r1、近＝r2、远＝r3－r2，逐平面归一化（零长度跳过）。

- [ ] **Step 4: 运行 Rust 测试**

Run: `cd packages/engine && cargo test -p mornlea_client --locked camera`
Expected: PASS（含 golden 向量断言与极角钳制 `pitch=±(π/2-0.01)` 断言）。

- [ ] **Step 5: Commit**

```bash
git add packages/engine/crates/mornlea_client/src/camera.rs packages/engine/crates/mornlea_client/src/lib.rs packages/client/client/camera_golden_test.go
git commit -m "feat(client): add rust camera math kernel with go golden parity"
```

### Task 2: Rust 可见性内核与 FFI 出口

**Files:**
- Create: `packages/engine/crates/mornlea_client/src/visibility.rs`
- Modify: `packages/engine/crates/mornlea_client/src/lib.rs`、`packages/engine/crates/mornlea_client/src/ffi.rs`、`packages/engine/include/mornlea_client.h`
- Test: `packages/engine/crates/mornlea_client/src/visibility.rs` 内 `#[cfg(test)]` 模块

**Interfaces:**
- Consumes: Task 1 的 `frustum_from`；Go `mesh.Connectivity` u16 位布局。
- Produces: `visibility::select_visible(origin:[i32;3], radius:i32, frustum:[[f32;4];6], conn: &[(i32,i32,i32,u16)], out: &mut Vec<[i32;3]>)`；FFI `mornlea_client_camera_visible_len`＋`mornlea_client_camera_visible_fetch`（v16 新增）。

位布局（与 `mesh.pairBit` 逐位一致，`a<b`）：`bit(a,b)=a*5-a*(a-1)/2+b-a-1`，`Connected(a,b)` 含 `a==b` 为真，`opposite(f)=f^1`，步进 `stepOf` 与 Go 一致。BFS 顺序与 `VisibleSectionsInto` 逐语句一致：`originBit=1<<6`、`scheduledBit=1<<7`，起点先 emit，队列按 `pending&^scheduledBit` 展开，`origin` 节点 `allowedExits=0x3f`，半径约束 `|dx|,|dz|<=radius`、`0<=Y<24`，frustum 用正顶点测试（`p·positive+w<0` 则剔除），`emitted` 去重但 `seen` 按 exit 位多次调度。`lookup` 缺失（未加载）跳过展开但保留 emit。

FFI 形状（照抄既有 render_* 校验顺序：ABI→handle→pointer→length→容量）：

```rust
pub unsafe extern "C" fn mornlea_client_camera_visible_len(
    abi_version: u32, origin_x: i32, origin_y: i32, origin_z: i32, radius: i32,
    frustum: *const f32, frustum_len: usize,
    conn_xyz: *const i32, conn_mask: *const u16, conn_len: usize,
    out_count: *mut u32,
) -> u32;
pub unsafe extern "C" fn mornlea_client_camera_visible_fetch(
    abi_version: u32, out_xyz: *mut i32, out_cap: usize, out_written: *mut usize,
) -> u32;
```

同一次输入的 len→fetch 配对由调用方同一线程顺序调用；`radius<0/>32`、空指针、长度不整除、`conn` 过大返回 `INVALID_ARGUMENT`；输出装不下返回 `CAPACITY` 且不写部分结果。最近一次结果缓存在线程局部复用缓冲（预热后零分配）。

- [ ] **Step 1: 写 Rust parity 测试（固定 connectivity 图 vs Go 输出）**

先在 Go 侧用既有 `VisibleSectionsInto` 对固定输入（origin=(0,5,0)、radius=2、全连通＋一道 solid 墙、固定 frustum）打印输出顺序，抄入 Rust 测试断言逐项相等。

- [ ] **Step 2: 实现 `visibility.rs`＋FFI＋header 声明**

`CLIENT_ABI_VERSION` 保持 15（本任务内不升，Task 3 统一升 16；FFI 函数先以 15 门控并通过单元测试）。

- [ ] **Step 3: 运行测试**

Run: `cd packages/engine && cargo test -p mornlea_client --locked visibility`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add packages/engine/crates/mornlea_client/src/visibility.rs packages/engine/crates/mornlea_client/src/lib.rs packages/engine/crates/mornlea_client/src/ffi.rs packages/engine/include/mornlea_client.h
git commit -m "feat(client): add rust visibility kernel and ffi"
```

### Task 3: ABI v16 翻版与 Go 薄封装

**Files:**
- Modify: `packages/engine/include/mornlea_client.h`（`15u`→`16u`）、`packages/engine/crates/mornlea_client/src/ffi.rs`（`CLIENT_ABI_VERSION=16`、单测里 `v15 的直接前代` 文案→v16）、`packages/client/client/camera.go`（保留纯 Go 实现为参考）、Create `packages/client/client/camera_native.go`（`//go:build darwin`，cgo 绑定两个新出口＋`VisibleSectionsNative`）、`packages/client/client/camera_native_test.go`（与 Go 参考实现的随机 parity fuzz：100 组随机位姿＋随机 connectivity 图逐项相等）。

**Interfaces:**
- Consumes: Task 1／2 的 Rust 内核。
- Produces: `NativeViewProj(cam *Camera) ([16]float32, core.Frustum)`、`NativeVisibleSections(origin core.SectionPos, radius int, frustum core.Frustum, lookup func(core.SectionPos) (mesh.Connectivity, bool)) []core.SectionPos`（返回 `[]core.SectionPos`，下游 `FrameStats` 与 `rustVisible` 组装循环逐字不动）。

- [ ] **Step 1: 翻 ABI 版本号三处并跑跨语言门禁**

Run: `make rust && cd packages/engine && cargo test -p mornlea_client --locked`
Expected: PASS（含 ABI 版本错配单测）。

- [ ] **Step 2: 写 Go parity fuzz 测试（先失败）**

Run: `go test ./packages/client/client -run TestNativeParity -count=1`
Expected: FAIL（`camera_native.go` 不存在，编译失败即红）。

- [ ] **Step 3: 实现 `camera_native.go`（参数校验＋两次调用＋解码，失败 panic 文案沿用 render.go 的 `renderStatusText` 风格）**

- [ ] **Step 4: 运行测试**

Run: `go test ./packages/client/client -race -count=1`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add packages/engine/include/mornlea_client.h packages/engine/crates/mornlea_client/src/ffi.rs packages/client/client/camera_native.go packages/client/client/camera_native_test.go
git commit -m "feat(client): bump client abi to v16 with native camera bridge"
```

### Task 4: app 接线、证据与收尾门禁

**Files:**
- Modify: `packages/client/cmd/mornlea/app/app_frame.go`（`cam.ViewProj()`→`NativeViewProj`、`mesh.VisibleSectionsInto`→`NativeVisibleSections`，`rustVisible` 组装保留）、`packages/audit` 基线（版本号）、根 `AGENTS.md` 版本矩阵、相关 `AGENTS.md`。
- Test: `go test ./packages/client/cmd/mornlea/app -race -count=1`、`make visual-check`（24 景零差异）、benchmark scenario v22 前后帧时间＋分配计数对比。

**Interfaces:**
- Consumes: Task 3 的 bridge。
- Produces: 热路径 Go 分配证据（`VisibleSectionsInto` 的 scratch 复用保留给非 darwin；darwin 帧循环零新增分配）。

- [ ] **Step 1: 改 `app_frame.go` 调用并跑 app 测试**

`mgl32.Mat4` 底层即 `[16]float32`，转换零成本。`app_frame.go` 约 248 行附近改写为：

```go
vpArr, frustum := client.NativeViewProj(cam)
viewProj := mgl32.Mat4(vpArr)
viewProjInv := viewProj.Inv()
```

随后删除 `core.FrustumFrom(viewProj)` 调用（改用返回的 `frustum`），`mesh.VisibleSectionsInto(...)` 改为 `client.NativeVisibleSections(cameraSectionPos(cam.Pos), underwater.VisibleRadius, frustum, activeScheduler.Connectivity)`（返回 `[]core.SectionPos`，直接赋给 `a.visibleSections`）；`rustVisible` 组装循环与 `BillboardCamera`（仍用 `viewProj`）保持不动。`render` 与 `core` 的既有 import 不得删除（`BillboardCamera` 仍需）。

Run: `go test ./packages/client/cmd/mornlea/app -race -count=1`
Expected: PASS。

- [ ] **Step 2: 跑视觉与性能证据**

Run: `make visual-check`（24/24 零差异）；benchmark 前后对比记录 P50/P99 与 `allocs/op`（只记录，不改变退出状态）。

- [ ] **Step 3: 收尾门禁**

Run: `make dev-check`（或 `scripts/agents/gates.sh` 入口）、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`
Expected: 全绿。

- [ ] **Step 4: Commit**

```bash
git add packages/client/cmd/mornlea/app/app_frame.go packages/audit AGENTS.md packages/client/cmd/mornlea/app/AGENTS.md
git commit -m "feat(client): drive frame loop with native camera and visibility"
```
