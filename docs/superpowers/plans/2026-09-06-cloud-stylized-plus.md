# 云层风格化增强实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `sky.wgsl` 的二值方块云换成 3 octave FBM 密度云＋厚度明暗＋晨昏染色＋边缘羽化，保留单层投影与方块宇宙观，不升 ABI。

**Architecture:** 只改 `packages/engine/crates/mornlea_client/shaders/sky.wgsl` 的云区函数（`cloud_hash` 保留，`cloud_mask` 重写为密度函数＋光照函数）；`CloudOffsetAt` 双偏移语义不变；`render_frame`、`FrameInput`、`sky` uniform 112B 布局全部不动。

**Tech Stack:** WGSL（wgpu）、Rust 1.97.1（--locked）、Go 1.26。

**Spec:** `docs/superpowers/specs/2026-09-06-camera-cloud-lighting-design.md` 第 4 节（Track2）

## Global Constraints

- Rust 工具链 1.97.1，不升级 compiler／Cargo lock／wgpu 线。
- client ABI 保持 v15（相机轨升 v16 若先合，本计划 rebase 后仍不碰版本号与帧布局；冲突时以相机轨为准，本轨只改 shader 文本）。
- 预热后热路径不新增每帧资源；云计算纯 ALU（无纹理采样、无 storage、无 uniform 新增）。
- `shaders.rs` 的存在性＋入口点单测必须保持通过。
- 颜色输出钳制 0..1；`y>=192` 与 `direction.y<=0.001` 早出保留。
- 注释中文；提交信息单行英文；禁任务编号。
- capture golden 变更走显式更新＋逐图人审，双阈值不放宽。

---

### Task 1: FBM 密度场（3 octave value-noise）

**Files:**
- Modify: `packages/engine/crates/mornlea_client/shaders/sky.wgsl`（云区）
- Test: `cd packages/engine && cargo test -p mornlea_client --locked`（shader 单测）＋ `make visual-check` 对比观察

**Interfaces:**
- Consumes: 既有 `hash_cell`、`sky.camera_cloud`（xyz＝相机，w＝local 偏移）、`sky.cloud_macro_x`。
- Produces: `cloud_density(intersection: vec2f) -> f32`（0..1 连续密度）。

- [ ] **Step 1: 确认基线（改前抓一帧云天对照）**

Run: `cd packages/engine && cargo test -p mornlea_client --locked shaders`
Expected: PASS（基线绿）。

- [ ] **Step 2: 实现密度场，替换二值 mask 的形状部分**

```wgsl
fn cloud_value_noise(p: vec2f) -> f32 {
    let cell = vec2i(floor(p));
    let frac = fract(p);
    let smooth = frac * frac * (3.0 - 2.0 * frac);
    let a = f32(hash_cell(vec3u(bitcast<u32>(cell.x), bitcast<u32>(cell.y), 0u)) & 255u) / 255.0;
    let b = f32(hash_cell(vec3u(bitcast<u32>(cell.x + 1), bitcast<u32>(cell.y), 0u)) & 255u) / 255.0;
    let c = f32(hash_cell(vec3u(bitcast<u32>(cell.x), bitcast<u32>(cell.y + 1), 0u)) & 255u) / 255.0;
    let d = f32(hash_cell(vec3u(bitcast<u32>(cell.x + 1), bitcast<u32>(cell.y + 1), 0u)) & 255u) / 255.0;
    return mix(mix(a, b, smooth.x), mix(c, d, smooth.x), smooth.y);
}

const CLOUD_OCTAVES: u32 = 3u; // 钉死为 3，不得参数化

fn cloud_density(intersection: vec2f) -> f32 {
    // 基准格 16 block（与既有 cell 口径一致），macro 每 64 block 覆盖调制
    let base = (intersection - vec2f(sky.camera_cloud.w, 0.0)) / 16.0;
    var fbm = 0.0;
    var amp = 0.55;
    var freq = 1.0;
    for (var o = 0u; o < CLOUD_OCTAVES; o++) {
        fbm += amp * cloud_value_noise(base * freq);
        amp *= 0.5;
        freq *= 2.03;
    }
    let macro_cell = vec2i(floor(base / 4.0));
    let cover = f32((cloud_hash(macro_cell, sky.cloud_macro_x) >> 4u) & 255u) / 255.0;
    let threshold = mix(0.62, 0.38, cover); // macro 覆盖高处阈值低、云多
    return smoothstep(threshold, threshold + 0.25, fbm);
}
```

`cloud_mask` 保留早出与 `intersection` 计算，把原 `cell/macro_cell/hash/十字形` 四行替换为 `return select(0.0, smoothstep(0.02, 0.08, direction.y), cloud_density(intersection) > 0.003);`——不，先保留二值返回，本任务只换形状不断光照：`let d = cloud_density(intersection); return select(0.0, smoothstep(0.02, 0.08, direction.y) * smoothstep(0.0, 0.15, d), d > 0.003);`。阈值常量如实记录为调参点，`visual-check` 人审后不再动。

- [ ] **Step 3: 运行 shader 单测＋编译**

Run: `cd packages/engine && cargo test -p mornlea_client --locked shaders && make rust`
Expected: PASS（WGSL 编译错误会在 renderer 构造期暴露，`make rust` 只保证构建；真机渲染验证在 Task 3）。

- [ ] **Step 4: Commit**

```bash
git add packages/engine/crates/mornlea_client/shaders/sky.wgsl
git commit -m "feat(client): fbm density field for stylized clouds"
```

### Task 2: 厚度明暗与晨昏染色

**Files:**
- Modify: `packages/engine/crates/mornlea_client/shaders/sky.wgsl`（`cloud_mask` 返回密度、`fs_main` 云混合行）
- Test: 同 Task 1

**Interfaces:**
- Consumes: Task 1 的 `cloud_density`；`sky.sun_daylight`（xyz＝太阳方向，w＝daylight）。
- Produces: 带光照的云色（亮顶暗底＋低太阳高度染橙）。

- [ ] **Step 1: 把 `cloud_mask` 改为返回密度并加伪法线光照**

```wgsl
fn cloud_light(density: f32, direction: vec3f) -> vec3f {
    let sun_direction = normalize(sky.sun_daylight.xyz);
    let daylight = clamp(sky.sun_daylight.w, 0.0, 1.0);
    let day_cloud = vec3f(0.84, 0.88, 0.92);
    let night_cloud = vec3f(0.18, 0.22, 0.28);
    var base = mix(night_cloud, day_cloud, daylight);
    // 低太阳高度染橙：sun_direction.y 越接近地平线权重越大
    let dusk = (1.0 - smoothstep(0.0, 0.35, abs(sun_direction.y))) * step(0.001, daylight) * (1.0 - daylight * 0.5);
    base = mix(base, vec3f(0.98, 0.62, 0.42), clamp(dusk, 0.0, 1.0) * 0.65);
    // 厚度：密度高处提亮顶、密度低处压暗边（伪厚度，无真实法线）
    let shade = mix(0.72, 1.06, smoothstep(0.0, 1.0, density));
    return clamp(base * shade, vec3f(0.0), vec3f(1.0));
}
```

`fs_main` 原云混合行 `color = mix(color, mix(vec3f(0.18,0.22,0.28), vec3f(0.84,0.88,0.92), sky.sun_daylight.w), cloud * 0.82);` 改为 `let density = cloud_mask(direction); color = mix(color, cloud_light(density, direction), clamp(density, 0.0, 1.0) * 0.9);`。`cloud_mask` 签名改为返回密度（早出返回 0.0）。

- [ ] **Step 2: 运行测试**

Run: `cd packages/engine && cargo test -p mornlea_client --locked`
Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add packages/engine/crates/mornlea_client/shaders/sky.wgsl
git commit -m "feat(client): thickness shading and dusk tint for clouds"
```

### Task 3: 双速动画、golden 更新与收尾

**Files:**
- Modify: `packages/engine/crates/mornlea_client/shaders/sky.wgsl`（细节层流速差）、`testdata/visual-golden/world/`（云可见场景 golden）、`packages/client/render/daylight.go`（`CloudOffsetAt` 文档注释说明双偏移语义不变）。
- Test: `make visual-check`、`go test ./packages/client/cmd/mornlea/capture/ -race -count=1`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`

**Interfaces:**
- Consumes: Task 1／2。
- Produces: 更新后的云 golden＋人审记录。

- [ ] **Step 1: 细节层流速差（高频 octave 多走 1.7 倍 local 偏移）**

在 `cloud_density` 内：`let detail = base * 2.03 + vec2f(sky.camera_cloud.w * 0.7, 0.0);` 把第二、三 octave 的采样点从 `base * freq` 改为以 `detail` 为基准的连续频率（`detail * (freq / 2.03)`），低频 macro 漂移保持原速。改后跑 `cargo test -p mornlea_client --locked` 全绿。

- [ ] **Step 2: 显式更新云可见场景 golden 并逐图人审**

Run: 更新模式抓帧（仓库既有显式更新路径，不得自创脚本），然后 `make visual-check` 全绿；只接受云与天空渐变差异，任何地形／实体差异必须归因或修复。把人审结论（哪几张变、为什么合法）写进提交信息正文——不，提交信息只单行；结论记入本任务的 ledger／change 备注（grass-closeup 收尾先例：结论放 ledger）。

- [ ] **Step 3: 收尾门禁**

Run: `go test ./packages/client/cmd/mornlea/capture/ -race -count=1`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`
Expected: 全绿。

- [ ] **Step 4: Commit**

```bash
git add packages/engine/crates/mornlea_client/shaders/sky.wgsl packages/client/render/daylight.go testdata/visual-golden/world/
git commit -m "feat(client): dual-rate cloud drift with regenerated goldens"
```
