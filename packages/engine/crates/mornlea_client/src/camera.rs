//! 相机纯数学内核：前向、视图投影与视锥提取，与 Go 侧逐行对应。
//!
//! 本模块不依赖窗口与 GPU，只做数值计算；矩阵一律列主序 `[f32; 16]`，
//! 与 `mgl32.Mat4` 内存布局一致，供 FFI 出口复用。
//!
//! 位级一致契约：Go 侧在三处把中间结果收窄——三角函数经 `float64` 计算再
//! 转 `float32`、向量长度经 `float64` 开方再转 `float32`、归一化用乘倒数
//! 而非除法；本模块逐字复现这三处（`f64` 三角／开方、`1.0/len` 倒数相乘）。
//! 第四处是 Go 后端在帧循环内联上下文中的 FMA 融合：`normalize` 的三元
//! 点积取 `dot3_fused_b` 形状、视锥归一化的取 `dot3_fused` 形状、四元点积
//! 取 `mul4` 内形状、减法取 `cross` 内形状（调用点不同融合不同，拟合结论）；
//! 独立调用的 `mgl32` 方法体求值为非融合 plain（融合与内联上下文有关，
//! 不能按源码字面推断，必须以权威帧循环输出为准）。`sqrt` 正确舍入两侧
//! 一致；`sin/cos/tan` 的 `f64` 实现差异由跨语言 parity 测试钉住。

/// 相机前向，语义与 Go `Camera::Forward` 一致：yaw=0、pitch=0 时朝向 -Z。
///
/// 位级对应：Go 先 `float64` 求三角再收窄（`float32(math.Sin(...))`），
// Rust 侧同样 `f64` 计算后 `as f32`；一元负号在收窄之后（两侧皆精确运算）。
pub fn forward(yaw: f32, pitch: f32) -> [f32; 3] {
    let cp = (pitch as f64).cos() as f32;
    [
        -((yaw as f64).sin() as f32) * cp,
        (pitch as f64).sin() as f32,
        -((yaw as f64).cos() as f32) * cp,
    ]
}

/// 透视投影，逐字对应 Go `core::Perspective`：右手坐标系、WebGPU `[0,1]` 深度。
///
/// 位级对应：`f` 经 `float64` 的 `tan` 与除法再收窄（Go 的
/// `float32(1 / math.Tan(float64(fovY)/2))`）；`inv` 的 `1/(near-far)`
/// 是 `f32` 除法（无类型常量 `1` 取操作数类型），两侧一致。
pub fn perspective(fov_y: f32, aspect: f32, near: f32, far: f32) -> [f32; 16] {
    let f = (1.0 / ((fov_y as f64) / 2.0).tan()) as f32;
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
    // 归一化使 `w` 成为真实距离；零长度平面保持原样。长度经 `f64` 开方
    // 再收窄（对应 Go `Vec3.Len` 的 `float32(math.Sqrt(float64(dot)))`，
    // `sqrt` 正确舍入两侧一致），点积取融合累加形状（见 `dot3_fused`），
    // 缩放用乘倒数（对应 `Vec4.Mul(1/l)`，除法只算一次倒数——与直接除法
    // 差 1ulp，不可互换）。
    for p in f.iter_mut() {
        let len = ((dot3_fused([p[0], p[1], p[2]], [p[0], p[1], p[2]])) as f64).sqrt() as f32;
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

/// 列主序 4x4 矩阵乘法，与 mgl32 `Mat4::Mul4` 逐项对应；点积取融合累加
/// 形状（首项之后逐项单舍入并入，见拟合注释——`Mul4` 独立调用体为 plain，
/// 帧循环内联上下文中融合，跨语言逐位 parity 测试钉住本形状）。
fn mul4(a: [f32; 16], b: [f32; 16]) -> [f32; 16] {
    let mut out = [0.0; 16];
    for col in 0..4 {
        for row in 0..4 {
            let m1 = a[4 + row] * b[col * 4 + 1];
            let t1 = a[row].mul_add(b[col * 4], m1);
            let t2 = a[8 + row].mul_add(b[col * 4 + 2], t1);
            out[col * 4 + row] = t2 + a[12 + row] * b[col * 4 + 3];
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

/// 三元点积（融合累加形状 A）：首乘积独立舍入，其后每项经单舍入 FMA 并入。
/// 即 `t1 = a1*b1+m0`、`r = a2*b2+t1`（`m0` 为首乘积独立舍入值）。
/// 对应 Go 后端在帧循环内联上下文中视锥归一化的实际求值；独立调用的
/// `mgl32` 方法体求值为非融合 plain——融合与内联上下文有关，不能按源码
/// 字面推断，必须以权威帧循环输出为准；跨语言逐位 parity 测试钉住本形状
/// （工具链升级若改变 Go 融合会直接变红）。
fn dot3_fused(a: [f32; 3], b: [f32; 3]) -> f32 {
    let m0 = a[0] * b[0];
    let t1 = a[1].mul_add(b[1], m0);
    a[2].mul_add(b[2], t1)
}

/// 三元点积（融合累加形状 B）：首项保持精确积直接并入，即
/// `t1 = a0*b0+m1`（`m1` 为次乘积独立舍入值）、`r = t1+a2*b2`。
/// 对应 `normalize`（相机前向／边向量归一化）在帧循环内的实际求值；
/// 同一 `Len` 在视锥归一化处取形状 A（调用点不同融合不同，拟合结论），
/// 两处不得共用。
fn dot3_fused_b(a: [f32; 3], b: [f32; 3]) -> f32 {
    let m1 = a[1] * b[1];
    let t1 = a[0].mul_add(b[0], m1);
    a[2].mul_add(b[2], t1)
}

fn cross(a: [f32; 3], b: [f32; 3]) -> [f32; 3] {
    // 融合形状对应 Go 后端在帧循环内联上下文中的实际求值：`x*y-z` 融合成
    // 单舍入 FMSUB（另一乘积先独立舍入再作为加项并入），即每分量形如
    // `-(b0.mul_add(b1, -(a0*a1)))`（内层取负精确，外层再取负）。
    // 注意独立调用的 `mgl32.Vec3.Cross` 方法体求值为非融合 plain——融合与
    // 内联上下文有关，不能按源码字面推断，必须以权威帧循环输出为准；
    // 跨语言逐位 parity 测试钉住本形状（工具链升级若改变 Go 融合会直接变红）。
    // 含精确零项的 s-cross 在两种形状下输出一致（零吸收精确），故不拆分。
    [
        -(a[2].mul_add(b[1], -(a[1] * b[2]))),
        -(a[0].mul_add(b[2], -(a[2] * b[0]))),
        -(a[1].mul_add(b[0], -(a[0] * b[1]))),
    ]
}

fn normalize(v: [f32; 3]) -> [f32; 3] {
    // 对应 mgl32 `Normalize` 的 `(1/|v|)*v`：长度经 `f64` 开方再收窄，
    // 再乘倒数——与 `v[i]/len` 的直接除法差 1ulp，不可互换；点积取
    // 融合累加形状 B（见 `dot3_fused_b`）。
    let len = (dot3_fused_b(v, v) as f64).sqrt() as f32;
    let inv = 1.0 / len;
    [v[0] * inv, v[1] * inv, v[2] * inv]
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
