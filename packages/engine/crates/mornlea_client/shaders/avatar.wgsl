struct Camera {
    view_proj: mat4x4f,
    // daylight.x 是本帧固定昼夜亮度，与地形使用同一相位。
    daylight:  vec4f,
};

struct AvatarInstance {
    transform: mat4x4f,
    color:     vec4f,
    // material 是实例的材质层号：哨兵 0xFFFFFFFFu 走原纯色路径，
    // 其余值是方块 atlas 的层号（牛皮/牛头）。_pad0..2 是实例尾部
    // 12 字节保留填充（编码侧恒写零）：实例用作 storage 数组元素时
    // 步长须为 16 的倍数，96 字节步长与 Go `avatarInstanceBytes` 一致。
    material: u32,
    _pad0: u32,
    _pad1: u32,
    _pad2: u32,
};

@group(0) @binding(0) var<uniform>       camera:    Camera;
@group(0) @binding(1) var<storage, read> instances: array<AvatarInstance>;
@group(0) @binding(2) var                atlas:     texture_2d_array<f32>;
@group(0) @binding(3) var                atlas_smp: sampler;

struct VsOut {
    @builtin(position) clip:     vec4f,
    @location(0)       color:    vec4f,
    @location(1)       normal:   vec3f,
    @location(2)       uv:       vec2f,
    @location(3)       material: u32,
    @location(4)       daylight: f32,
    @location(5)       face:     u32,
};

fn cube_normal(face: u32) -> vec3f {
    switch face {
        case 0u: { return vec3f( 1.0,  0.0,  0.0); }
        case 1u: { return vec3f(-1.0,  0.0,  0.0); }
        case 2u: { return vec3f( 0.0,  1.0,  0.0); }
        case 3u: { return vec3f( 0.0, -1.0,  0.0); }
        case 4u: { return vec3f( 0.0,  0.0,  1.0); }
        default: { return vec3f( 0.0,  0.0, -1.0); }
    }
}

// avatar_uv 由 cuboid 面顶点本地坐标派生 0..1 全图 UV：侧面纵向取
// 0.5-y（纹理 v=0 的顶行落在方块顶部，与 terrain/crack 同朝向），
// 水平按面取对应轴；顶/底面按环绕序铺满。面序与 `cube_normal` 一致。
fn avatar_uv(local: vec3f, face: u32) -> vec2f {
    switch face {
        case 0u: { return vec2f(local.z + 0.5, 0.5 - local.y); } // +X
        case 1u: { return vec2f(0.5 - local.z, 0.5 - local.y); } // -X
        case 2u: { return vec2f(local.x + 0.5, local.z + 0.5); } // +Y
        case 3u: { return vec2f(local.x + 0.5, 0.5 - local.z); } // -Y
        case 4u: { return vec2f(0.5 - local.x, 0.5 - local.y); } // +Z
        default: { return vec2f(local.x + 0.5, 0.5 - local.y); } // -Z
    }
}

// 人物内部材质每六层对应六面；本地 -Z 是前向，后脑独占 +Z 层。
// 旧牛、敌怪、物品与纯色哨兵保持原采样规则。
fn avatar_face_material(material: u32, face: u32) -> u32 {
    if (material >= 112u && material < 160u) {
        return material + face;
    }
    return material;
}

// 物品透明轮廓层紧邻牛头层之后、人物分面层之前。掉落薄片仍用有厚度的
// cuboid 做保守布局，但片元只保留本地 ±Z 两张大面，避免四个窄侧面把整幅
// 图标压成矩形边框。方块、牛、人物与纯色粒子均落在此区间之外。
fn item_icon_material(material: u32) -> bool {
    return material >= 81u && material < 112u;
}

@vertex
fn vs_main(
    @location(0)             local: vec3f,
    @builtin(vertex_index)   vi:    u32,
    @builtin(instance_index) ii:    u32,
) -> VsOut {
    let instance = instances[ii];
    let world = instance.transform * vec4f(local, 1.0);

    var out: VsOut;
    out.clip = camera.view_proj * world;
    // camera 只对 vertex 阶段可见，昼夜亮度在这里乘入颜色再插值到片元。
    // 纯色分支语义与变更前逐位一致；贴图分支另经 daylight  varying 拿亮度。
    out.color = vec4f(instance.color.rgb * camera.daylight.x, instance.color.a);
    out.normal = normalize(mat3x3f(
        instance.transform[0].xyz,
        instance.transform[1].xyz,
        instance.transform[2].xyz,
    ) * cube_normal(vi / 4u));
    let face = vi / 4u;
    out.uv = avatar_uv(local, face);
    out.material = avatar_face_material(instance.material, face);
    out.daylight = camera.daylight.x;
    out.face = face;
    return out;
}

@fragment
fn fs_main(in: VsOut) -> @location(0) vec4f {
    let light_dir = normalize(vec3f(0.35, 0.85, 0.4));
    let diffuse = 0.58 + 0.42 * max(dot(in.normal, light_dir), 0.0);
    if (in.material != 0xFFFFFFFFu) {
        if (item_icon_material(in.material) && in.face < 4u) { discard; }
        let c = textureSample(atlas, atlas_smp, in.uv, i32(in.material));
        // 与 terrain/crack 同阈值（0.5）的二值化丢弃；牛身层不透明，
        // 本分支恒不触发，只承接契约。
        if (c.a < 0.5) { discard; }
        return vec4f(c.rgb * in.daylight * diffuse, c.a);
    }
    return vec4f(in.color.rgb * diffuse, in.color.a);
}
