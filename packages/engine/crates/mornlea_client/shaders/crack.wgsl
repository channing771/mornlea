// 采掘裂纹 overlay pass：在目标方块六个表面叠加透明 cutout 裂纹层。
//
// 实例由 Go `EncodeBlockCrackInstances` 编码：transform 是含平移与 1.002
// 外扩的列主序 mat4，atlas 层号是 `LayerCrack0 + stage` 的 f32。层号在实例
// 布局的 offset 64..68，用 vec4f 承接（取 .x）而不是 vec3f——WGSL 的 vec3f
// 对齐为 16，offset 68 的 vec3f 成员不合法；.yzw 是实例尾部的 12 字节零填充。

struct Camera {
    view_proj: mat4x4f,
    // daylight.x 是本帧固定昼夜亮度，与 avatar/terrain 使用同一相位。
    daylight: vec4f,
};

struct CrackInstance {
    transform: mat4x4f,
    // .x 是 atlas 裂纹层号；.yzw 承接实例尾部零填充。
    layer: vec4f,
};

@group(0) @binding(0) var<uniform>       camera:    Camera;
@group(0) @binding(1) var<storage, read> instances: array<CrackInstance>;
@group(0) @binding(2) var                atlas:     texture_2d_array<f32>;
@group(0) @binding(3) var                atlas_smp: sampler;

struct VsOut {
    @builtin(position) clip:    vec4f,
    @location(0)       uv:      vec2f,
    @location(1)       layer:   f32,
    @location(2)       daylight: f32,
};

@vertex
fn vs_main(
    @location(0)             local: vec3f,
    @location(1)             uv:    vec2f,
    @builtin(instance_index) ii:    u32,
) -> VsOut {
    let instance = instances[ii];
    var out: VsOut;
    out.clip = camera.view_proj * (instance.transform * vec4f(local, 1.0));
    out.uv = uv;
    out.layer = instance.layer.x;
    // camera 只对 vertex 阶段可见（与 avatar 同一约定），昼夜亮度作为逐顶点
    // 常量插值到片元：同一帧内取值处处相同，插值不改变数值。
    out.daylight = camera.daylight.x;
    return out;
}

@fragment
fn fs_main(in: VsOut) -> @location(0) vec4f {
    let c = textureSample(atlas, atlas_smp, in.uv, i32(in.layer));
    // 与 terrain cutout 同阈值（0.5）的二值化丢弃：裂纹层背景 alpha 为 0、
    // 裂纹像素 alpha 为 1，背景像素整体丢弃，overlay 才不遮挡原方块材质。
    if (c.a < 0.5) { discard; }
    // rgb 乘 daylight 与 avatar 同相位（夜间裂纹随之变暗）；裂纹是叠加层，
    // 刻意不做面向法线的漫反射，避免与方块自身的明暗着色打架。经 discard
    // 后 alpha 恒为 1，配合管线的 alpha blend 等价不透明覆盖。
    return vec4f(c.rgb * in.daylight, c.a);
}
