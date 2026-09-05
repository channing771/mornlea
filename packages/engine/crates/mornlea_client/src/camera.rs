//! 相机纯数学内核：前向、视图投影与视锥提取，与 Go 侧逐行对应。
//!
//! 本模块不依赖窗口与 GPU，只做 `f32` 数值计算；矩阵一律列主序 `[f32; 16]`，
//! 与 `mgl32.Mat4` 内存布局一致，供后续 FFI 出口复用。

/// 相机前向，语义与 Go `Camera::Forward` 一致：yaw=0、pitch=0 时朝向 -Z。
pub fn forward(yaw: f32, pitch: f32) -> [f32; 3] {
    let cp = pitch.cos();
    [-yaw.sin() * cp, pitch.sin(), -yaw.cos() * cp]
}

/// 透视投影，逐字对应 Go `core::Perspective`：右手坐标系、WebGPU `[0,1]` 深度。
pub fn perspective(fov_y: f32, aspect: f32, near: f32, far: f32) -> [f32; 16] {
    let f = 1.0 / (fov_y / 2.0).tan();
    let inv = 1.0 / (near - far);
    [
        f / aspect,
        0.0,
        0.0,
        0.0,
        0.0,
        f,
        0.0,
        0.0,
        0.0,
        0.0,
        far * inv,
        -1.0,
        0.0,
        0.0,
        far * near * inv,
        0.0,
    ]
}

/// 视图矩阵，转写自 mgl32 `LookAtV`：位置为相机点，朝向由 yaw/pitch 经 `forward` 给出，上向量固定 `(0,1,0)`。
///
/// 与 `LookAtV` 一字对应：先算出目标点 `center = pos + forward` 再回减求前向，
/// 其 `f32` 舍入（含大坐标下的相消）语义是跨语言 parity 的一部分，不得约去。
pub fn view(pos: [f32; 3], yaw: f32, pitch: f32) -> [f32; 16] {
    let fwd = forward(yaw, pitch);
    let center = add3(pos, fwd);
    let f = normalize(sub3(center, pos));
    let s = normalize(cross(f, [0.0, 1.0, 0.0]));
    let u = cross(s, f);
    // 旋转部分列主序排布，与 `LookAtV` 的 `Mat4` 字面量一致，再右乘平移到 `-pos`。
    let rot = [
        s[0], u[0], -f[0], 0.0, //
        s[1], u[1], -f[1], 0.0, //
        s[2], u[2], -f[2], 0.0, //
        0.0, 0.0, 0.0, 1.0,
    ];
    let trans = [
        1.0, 0.0, 0.0, 0.0, //
        0.0, 1.0, 0.0, 0.0, //
        0.0, 0.0, 1.0, 0.0, //
        -pos[0], -pos[1], -pos[2], 1.0,
    ];
    mul4(rot, trans)
}

/// 视图投影矩阵，对应 Go `Camera::ViewProj` 的 `Perspective(...).Mul4(view)` 顺序。
pub fn view_proj(
    pos: [f32; 3],
    yaw: f32,
    pitch: f32,
    fov_y: f32,
    aspect: f32,
    near: f32,
    far: f32,
) -> [f32; 16] {
    mul4(perspective(fov_y, aspect, near, far), view(pos, yaw, pitch))
}

/// 视锥提取，逐字对应 Go `core::FrustumFrom`：行提取、左右下上近远组合、逐平面归一化（零长度跳过）。
/// 返回顺序：左、右、下、上、近、远。
pub fn frustum_from(view_proj: [f32; 16]) -> [[f32; 4]; 6] {
    let m = view_proj;
    // `mgl32.Mat4` 列主序：行 `i` 取各列第 `i` 项。
    let row = |i: usize| [m[i], m[4 + i], m[8 + i], m[12 + i]];
    let (r0, r1, r2, r3) = (row(0), row(1), row(2), row(3));
    let mut f = [
        add(r3, r0), // 左
        sub(r3, r0), // 右
        add(r3, r1), // 下
        sub(r3, r1), // 上
        r2,          // 近（WebGPU `[0,1]` 深度直接取第 2 行）
        sub(r3, r2), // 远
    ];
    // 归一化使 `w` 成为真实距离；零长度平面保持原样。
    for p in f.iter_mut() {
        let len = (p[0] * p[0] + p[1] * p[1] + p[2] * p[2]).sqrt();
        if len > 0.0 {
            let inv = 1.0 / len;
            p[0] *= inv;
            p[1] *= inv;
            p[2] *= inv;
            p[3] *= inv;
        }
    }
    f
}

/// 列主序 4x4 矩阵乘法，与 mgl32 `Mat4::Mul4` 逐项对应。
fn mul4(a: [f32; 16], b: [f32; 16]) -> [f32; 16] {
    let mut out = [0.0; 16];
    for col in 0..4 {
        for row in 0..4 {
            out[col * 4 + row] = a[row] * b[col * 4]
                + a[4 + row] * b[col * 4 + 1]
                + a[8 + row] * b[col * 4 + 2]
                + a[12 + row] * b[col * 4 + 3];
        }
    }
    out
}

fn add3(a: [f32; 3], b: [f32; 3]) -> [f32; 3] {
    [a[0] + b[0], a[1] + b[1], a[2] + b[2]]
}

fn sub3(a: [f32; 3], b: [f32; 3]) -> [f32; 3] {
    [a[0] - b[0], a[1] - b[1], a[2] - b[2]]
}

fn cross(a: [f32; 3], b: [f32; 3]) -> [f32; 3] {
    [
        a[1] * b[2] - a[2] * b[1],
        a[2] * b[0] - a[0] * b[2],
        a[0] * b[1] - a[1] * b[0],
    ]
}

fn normalize(v: [f32; 3]) -> [f32; 3] {
    let len = (v[0] * v[0] + v[1] * v[1] + v[2] * v[2]).sqrt();
    [v[0] / len, v[1] / len, v[2] / len]
}

fn add(a: [f32; 4], b: [f32; 4]) -> [f32; 4] {
    [a[0] + b[0], a[1] + b[1], a[2] + b[2], a[3] + b[3]]
}

fn sub(a: [f32; 4], b: [f32; 4]) -> [f32; 4] {
    [a[0] - b[0], a[1] - b[1], a[2] - b[2], a[3] - b[3]]
}

#[cfg(test)]
mod tests {
    use super::*;

    const EPS: f32 = 1e-5;

    #[test]
    fn camera_golden_vectors_match_go() {
        let fwd = forward(0.7, -0.25);
        for (got, want) in fwd.iter().zip([-0.624_190_5, -0.24740396, -0.74106514]) {
            assert!(
                (got - want).abs() <= EPS,
                "前向分量 {got} 与真值 {want} 不一致"
            );
        }
        let vp = view_proj(
            [1.5, 65.25, -3.75],
            0.7,
            -0.25,
            1.22173,
            16.0 / 9.0,
            0.1,
            1536.0,
        );
        for (idx, want) in [
            (0, 0.61442345),
            (5, 1.383_750_4),
            (10, -0.741_113),
            (15, 14.300_529),
        ] {
            assert!(
                (vp[idx] - want).abs() <= EPS,
                "投影矩阵 [{idx}] = {}，真值 {want}",
                vp[idx]
            );
        }
        for (i, plane) in frustum_from(vp).iter().enumerate() {
            let len = (plane[0] * plane[0] + plane[1] * plane[1] + plane[2] * plane[2]).sqrt();
            assert!(
                (len - 1.0).abs() <= EPS,
                "视锥平面 {i} 法线未归一化，长度 {len}"
            );
        }
    }

    #[test]
    fn camera_pitch_clamp_limit_stays_unit() {
        let limit = std::f32::consts::FRAC_PI_2 - 0.01;
        for pitch in [limit, -limit] {
            let f = forward(0.7, pitch);
            assert!(
                f.iter().all(|v| v.is_finite()),
                "钳制极角下前向出现非有限值"
            );
            let len = (f[0] * f[0] + f[1] * f[1] + f[2] * f[2]).sqrt();
            assert!((len - 1.0).abs() <= EPS, "钳制极角下前向长度 {len} ≠ 1");
            assert!(
                (f[1] - pitch.sin()).abs() <= EPS,
                "钳制极角下 Y 分量不符合正弦"
            );
        }
    }
}
