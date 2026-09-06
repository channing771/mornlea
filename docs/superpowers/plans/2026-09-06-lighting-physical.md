# 光线物理化升级实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 日光色温＋晨昏 shoulder 曲线、半球环境＋ACES tone map、高度雾＋太阳前向散射，终结生硬光线；全部 24 张 world golden 统一重生成＋人审。

**Architecture:** `render/daylight.go` 曲线升级（输出语义不变：`DayNight` 各字段含义与 `TerrainBrightness` 公式形状保留）；`terrain/water.wgsl` 在既有 `shade` 后加半球环境、片元输出前加 ACES；`lod.wgsl` 距离雾加高度衰减；`sky.wgsl` 天空区（非云区）加太阳光晕。体素光照 BFS（`mornlea_engine/light.rs`）不动。

**Tech Stack:** Go 1.26、WGSL、Rust 1.97.1（--locked）。

**Spec:** `docs/superpowers/specs/2026-09-06-camera-cloud-lighting-design.md` 第 5 节（Track3）

## Global Constraints

- Rust 工具链 1.97.1，不升级 compiler／Cargo lock／wgpu 线。
- 不新增 frame 字段、不升 client ABI（与相机／云轨的共享契约冻结一致；`sky` uniform 112B 布局不动）。
- atlas 视为 sRGB 编码输入，着色在线性空间计算，输出前 ACES＋线性→sRGB。
- 颜色钳制 0..1；雾参数 `full>start>0` 不变（既有 setter 保证）。
- 注释中文；提交信息单行英文；禁任务编号。
- golden 走显式更新＋逐图人审，双阈值不放宽；`make visual-check` 必须人审通过才能接受。

---

### Task 1: 昼夜曲线（色温近似＋晨昏 shoulder）

**Files:**
- Modify: `packages/client/render/daylight.go`、`packages/client/render/daylight_test.go`、`packages/client/cmd/mornlea/app/app_celestial_test.go`（若断言旧曲线则同步更新）
- Test: `go test ./packages/client/render/ -race -count=1`

**Interfaces:**
- Consumes: `core.DisplayDayPhase(worldTime, offset)`（相位算式不许动）。
- Produces: `DayNightAt`（同签名，数值语义升级）、`TerrainBrightness`（形状保留，`indoorBrightness=0.08` 保留）。

- [ ] **Step 1: 先跑出现有测试基线**

Run: `go test ./packages/client/render/ -count=1`
Expected: PASS（记住失败即停，先修环境再动曲线）。

- [ ] **Step 2: 实现新曲线（红测试先行：把旧期望值改成新公式的手算值）**

```go
// sun 保持 max(0, sin(2π·phase/24000))；daylight 加晨昏 shoulder：
sunClip := sun // 0..1
shoulder := sunClip * sunClip * (3 - 2*sunClip) // smoothstep
daylight := float32(0.12 + 0.88*shoulder)
// 色温近似混色（非真实黑体辐射）：warmth = 1-smoothstep(0,0.5,sunClip)，暖光 (1.0,0.55,0.30)→白光 (1,1,1)
```

具体实现：`warmth := 1 - smoothstep(0, 0.5, sun)`（smoothstep 用 `t*t*(3-2t)` 手写，不引入依赖）；`sunTint := lerp([1.0,0.55,0.30],[1,1,1],1-warmth)`；`ClearColor` 在既有 night→day lerp 后再乘 `sunTint`（夜间 sun=0 时 warmth=1 会染暖——必须用 `daylight` 门控：`tintStrength = smoothstep(0.12, 0.4, daylight)`，夜间保持纯净夜空）。`Sun` 字段语义不变（仍是 max(0,sin)），`StarVisibility` 公式不变。`TerrainBrightness` 不动。

先改测试期望：用新公式手算 `DayNightAt(6000,0).Daylight`（正午 sun=1→shoulder=1→1.0）与 `DayNightAt(0,0)`（sun=0→0.12）等锚点，跑红后再实现。

- [ ] **Step 3: 运行测试**

Run: `go test ./packages/client/render/ -race -count=1 && go test ./packages/client/cmd/mornlea/app -run Celestial -count=1`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add packages/client/render/daylight.go packages/client/render/daylight_test.go packages/client/cmd/mornlea/app/app_celestial_test.go
git commit -m "feat(client): warmer low sun and shouldered daylight curve"
```

### Task 2: 地形与水面着色（半球环境＋ACES）

**Files:**
- Modify: `packages/engine/crates/mornlea_client/shaders/terrain.wgsl`、`packages/engine/crates/mornlea_client/shaders/water.wgsl`
- Test: `cd packages/engine && cargo test -p mornlea_client --locked`

**Interfaces:**
- Consumes: Task 1 的 `daylight`／`ClearColor`（经 camera uniform `cam_pos.w` 与 LOD `fog_color` 传入，uniform 布局不动）；既有 `face_shade／ao_factor／base`。
- Produces: 同实例布局、同 atlas 采样，片元输出经 tone map。

- [ ] **Step 1: 顶点加半球环境（terrain 与 water 同改，两文件各持一份，改位布局必须两边一起改）**

```wgsl
// out.shade 组成改为：face_shade * ao_factor * base * hemi，其中
// hemi 按面法线在天空色与地面色之间 mix：顶面取天空环境 1.0，底面取 0.35，侧面 0.7。
// 法线由 face 推导：face==3 → n=vec3f(0,1,0)；face==2 → (0,-1,0)；face 0/1 → ±X；
// face 4/5 → ±Z；face 6/7（植物交叉面）→ 取 (0,1,0)。
// hemi = mix(0.38, 1.0, n.y * 0.5 + 0.5)；植物面固定 0.95（沿用“交叉面不打折”先例）。
```

`vs_main` 末尾 `out.shade = face_shade(face) * ao_factor * base;` 改为乘 `hemi`。`face_shade` 表格与 AO 公式逐字保留。

- [ ] **Step 2: 片元加 ACES 近似＋线性→sRGB**

```wgsl
fn aces_approx(x: vec3f) -> vec3f {
    // Narkowicz 近似，常量逐字采用
    return clamp((x * (2.51 * x + 0.03)) / (x * (2.43 * x + 0.59) + 0.14), vec3f(0.0), vec3f(1.0));
}
fn linear_to_srgb(x: vec3f) -> vec3f {
    return mix(x * 12.92, 1.055 * pow(clamp(x, vec3f(0.0), vec3f(1.0)), vec3f(1.0 / 2.4)) - 0.055, step(vec3f(0.0031308), x));
}
```

terrain `fs_main`：`let c = textureSample(...); if (c.a < 0.5) { discard; } return vec4f(c.rgb * in.shade, 1.0);` 改为 `let linear = pow(c.rgb, vec3f(2.2)) * in.shade; return vec4f(linear_to_srgb(aces_approx(linear * 1.0)), 1.0);`（曝光 1.0 钉死，不参数化）。water 同改，保留 `return vec4f(..., c.a)` 的 alpha 语义（alpha 不进 tone map）。

- [ ] **Step 3: 运行测试**

Run: `cd packages/engine && cargo test -p mornlea_client --locked && make rust`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add packages/engine/crates/mornlea_client/shaders/terrain.wgsl packages/engine/crates/mornlea_client/shaders/water.wgsl
git commit -m "feat(client): hemisphere ambient and aces tonemapping"
```

### Task 3: 高度雾与太阳光晕

**Files:**
- Modify: `packages/engine/crates/mornlea_client/shaders/lod.wgsl`、`packages/engine/crates/mornlea_client/shaders/sky.wgsl`（只动天空渐变＋日盘区，不动云区函数）
- Test: 同 Task 2

**Interfaces:**
- Consumes: `camera.fog`（起雾／全雾距离，不动）、`camera.fog_color`（天空色同源，不动）、`sky.sun_daylight`。
- Produces: 同管线绑定，不新增 uniform。

- [ ] **Step 1: LOD 高度雾混合**

```wgsl
let dist = distance(in.world, camera.cam_pos.xyz);
let dist_fog = clamp((dist - camera.fog.x) / (camera.fog.y - camera.fog.x), 0.0, 1.0);
// 高度衰减：世界 y 越低雾越浓（河谷聚雾），以 y=64 为基准、每低 32 格权重翻倍封顶
let height_fog = clamp((64.0 - in.world.y) / 96.0, 0.0, 1.0) * 0.35;
let fog = clamp(dist_fog + height_fog * dist_fog, 0.0, 1.0);
return vec4f(mix(c.rgb * in.shade, camera.fog_color.rgb, fog), 1.0);
```

- [ ] **Step 2: 天空太阳前向散射光晕**

在 `sky.wgsl` `fs_main` 的 `sun_disc` 计算后加：`let glow = pow(clamp(dot(direction, sun_direction), 0.0, 1.0), 24.0) * select(0.0, 1.0, sun_direction.y > -0.05) * clamp(sky.sun_daylight.w * 2.0, 0.0, 1.0); color += vec3f(1.0, 0.75, 0.5) * glow * 0.35;`（只加色不减色，不破坏夜空与星星）。

- [ ] **Step 3: 运行测试**

Run: `cd packages/engine && cargo test -p mornlea_client --locked`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add packages/engine/crates/mornlea_client/shaders/lod.wgsl packages/engine/crates/mornlea_client/shaders/sky.wgsl
git commit -m "feat(client): height falloff fog and sun forward glow"
```

### Task 4: 全量 golden 重生成、人审与收尾

**Files:**
- Modify: `testdata/visual-golden/world/`（24 张重生成）、`packages/client/render/daylight_test.go` 快照若有（无则跳过）
- Test: `make visual-check`（人审）、`go test ./packages/client/cmd/mornlea/capture/ -race -count=1`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`

- [ ] **Step 1: 显式更新全部 world golden**

Run: 仓库既有显式更新路径（不得自创脚本）。预期：24 张几乎全变（tone map 全局生效），这是计划内的。

- [ ] **Step 2: 逐图人审（硬门：必须人类看过才能接受）**

人审清单：正午不过曝（`terrain-noon` 高光面 ≤ 1.0 无大面积死白）、夜景可辨（`torch-night` 火把池边界清晰）、晨昏不断层（天空渐变平滑）、雾不吞近景（`far-horizon` 近环行与 control 一致）。结论记 ledger，不放宽阈值。

- [ ] **Step 3: 收尾门禁**

Run: `go test ./packages/client/cmd/mornlea/capture/ -race -count=1`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`
Expected: 全绿。

- [ ] **Step 4: Commit**

```bash
git add testdata/visual-golden/world/ packages/client/render/
git commit -m "feat(client): regenerate world goldens for physical lighting"
```
