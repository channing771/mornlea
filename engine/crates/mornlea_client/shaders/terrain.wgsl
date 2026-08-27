struct Camera {
    view_proj: mat4x4f,
    // cam_pos.xyz 是相机位置，cam_pos.w 是本帧固定昼夜亮度 daylight。
    cam_pos:   vec4f,
};

@group(0) @binding(0) var<uniform>       camera:    Camera;
@group(0) @binding(1) var<storage, read> instances: array<vec4u>;
@group(0) @binding(2) var<storage, read> origins:   array<vec4i>;
@group(0) @binding(3) var                atlas:     texture_2d_array<f32>;
@group(0) @binding(4) var                atlas_smp: sampler;

struct VsOut {
    @builtin(position) clip:  vec4f,
    @location(0)       uv:    vec2f,
    @location(1)       layer: f32,
    @location(2)       shade: f32,
};

fn axis_vec(axis: u32) -> vec3f {
    if (axis == 0u) { return vec3f(1.0, 0.0, 0.0); }
    if (axis == 1u) { return vec3f(0.0, 1.0, 0.0); }
    return vec3f(0.0, 0.0, 1.0);
}

// face_uv 把世界坐标分量投影到材质的 (u, v)。约定：wgpu 纹理坐标 v=0 是
// 图像顶行、世界 y 轴向上，因此侧面（以及交叉斜面）的纵向分量必须取
// -world.y，才能让纹理顶行（草缘、原木树皮的顶部等）落在方块顶部；否则
// v=world.y 会让图像顶行落到方块底部（上下颠倒），±X 面把 u 当纵向更会把
// 整张纹理旋转 90°。采样器地址模式为 Repeat，负 v 会正确环绕，世界坐标 UV 的
// 相位仍然只由世界坐标决定、取负不改写连续性。
fn face_uv(world: vec3f, axis: u32) -> vec2f {
    if (axis == 0u) { return vec2f(world.z, -world.y); } // ±X 面：水平=world.z、纵向=-world.y
    if (axis == 1u) { return vec2f(world.z, world.x); }  // ±Y 面（顶/底）：两个水平轴，正确
    return vec2f(world.x, -world.y);                     // ±Z 面：水平=world.x、纵向=-world.y
}

fn face_shade(face: u32) -> f32 {
    switch face {
        case 3u: { return 1.00; }
        case 2u: { return 0.50; }
        case 0u, 1u: { return 0.68; }
        // 6/7 是植物的两条交叉斜面：两条取同一个系数，否则两片会在交线处
        // 出现一道明暗接缝。取满值（与顶面同）——交叉面没有"朝向哪个轴"可言，
        // 按方向打折只会让作物无端发暗。
        case 6u, 7u: { return 1.00; }
        default: { return 0.84; }
    }
}

// 耕地材质层闭区间（干/湿两态）。material 落入区间 ⟺ 这条 quad 是 registry
// block_top_raw 非零的短方块，bit 12..19/55..62 是角高度原值而不是 w/h 尺寸。
//
// 判别为什么走 material 区间：quad 布局只剩 bit 63 一个空闲位且必须留空，
// 「是不是短方块」占不到任何位；植物又把 face 6/7 占为交叉斜面判别。耕地是
// 轴向面（face 0..5），与植物按 face 天然互斥，material 区间是唯一不冲突的
// 判别通道。数值真值源是 Go internal/assets 的 `LayerFarmlandDry`/`LayerFarmlandWet`
// （29/30），Rust 侧复述见 src/render/shaders.rs 的
// `FARMLAND_MATERIAL_FIRST`/`FARMLAND_MATERIAL_LAST`，三方由 render/farmland_tests.rs
// 钉在一起——在这里改数字必须同步另外两处。
fn farmland_material(mat: u32) -> bool {
    return mat >= 29u && mat <= 30u;
}

// 火把材质层（五种形态共用一张竖直火柄纹理）。墙面火把的倾斜薄板携带角高度
// （支撑侧 9/16、远离侧 14/16，贴面帽四角全 0），与耕地共用同一条角高度解码
// 路径——判别同样走 material：火把薄板是轴向面（face 0..5），与植物（face 6/7）
// 按 face 天然互斥，material 是唯一不冲突的判别通道。数值真值源是 Go
// internal/assets 的 `LayerTorch`（层枚举末位追加，iota 当前 58），Rust 侧复述
// 见 src/render/shaders.rs 的 `TORCH_MATERIAL`，由 render/farmland_tests.rs 的
// 源码扫描钉在一起——在这里改数字必须同步另外两处。
fn torch_material(mat: u32) -> bool {
    return mat == 58u;
}

// corner_height 取出第 vi 个顶点的 4-bit 角高度原值，与 water.wgsl 逐字同源
// （WGSL 没有 include，两个 pass 各持一份；改位布局必须两边一起改）。位布局与
// engine 的 quad.rs（SHIFT_W / SHIFT_H / SHIFT_CORNER2 / SHIFT_CORNER3）逐位对应：
//
//   | quad 位 | 内容   | 本着色器读法          |
//   |---------|--------|-----------------------|
//   | 12..15  | 角 0   | lo >> 12              |
//   | 16..19  | 角 1   | lo >> 16              |
//   | 55..58  | 角 2   | hi >> 23（55 - 32）   |
//   | 59..62  | 角 3   | hi >> 27（59 - 32）   |
//
// 角顺序与 cu/cv 表一致，即局部 (u,v) 的 (0,0) (1,0) (1,1) (0,1)，于是顶点
// 索引 vi 直接就是角索引。
fn corner_height(lo: u32, hi: u32, vi: u32) -> u32 {
    switch vi {
        case 0u:  { return (lo >> 12u) & 0xFu; }
        case 1u:  { return (lo >> 16u) & 0xFu; }
        case 2u:  { return (hi >> 23u) & 0xFu; }
        default:  { return (hi >> 27u) & 0xFu; }
    }
}

@vertex
fn vs_main(
    @builtin(vertex_index)   vi: u32,
    @builtin(instance_index) ii: u32,
) -> VsOut {
    let inst = instances[ii];
    let lo = inst.x;
    let hi = inst.y;

    let x     = f32( lo          & 0xFu);
    let y     = f32((lo >>  4u) & 0xFu);
    let z     = f32((lo >>  8u) & 0xFu);
    let face  =      (lo >> 20u) & 0x7u;
    let mat   = ((lo >> 23u) & 0x1FFu) | ((hi & 0x7Fu) << 9u);
    let ao    = (hi >>  7u) & 0xFFu;
    let light = (hi >> 15u) & 0xFFu;

    let axis = face >> 1u;
    let positive = f32(face & 1u);
    let ua = (axis + 1u) % 3u;
    let va = (axis + 2u) % 3u;

    var cu = array<f32, 4>(0.0, 1.0, 1.0, 0.0);
    var cv = array<f32, 4>(0.0, 0.0, 1.0, 1.0);

    // face 6/7 是植物的交叉斜面。这条分支**绝不解码尺寸**：植物 quad 借走了
    // bit 12..19 装正/背标志与保留位，按 `w-1`/`h-1` 解码得到的是把标志位当
    // 尺寸读的垃圾值。水面当年就栽在同一处——角高度上线后着色器仍按 `w-1`/
    // `h-1` 解码，每片水被画成 8×8 到 16×16 的巨型石板。
    //
    // 耕地（及一切 registry block_top_raw 非零的短方块）是同一坑的第三个受害者
    // 候选：mesher 对带角高度的 quad 不贪心合并、恒 1×1 出面，bit 12..19/55..62
    // 装的是四角高度原值，沿用 `w/h` 解码同样会摊成巨型石板。因此 material 落入
    // 耕地区间时走与 water.wgsl 同源的角高度路径：满格水平范围 + 顶面顶点抬升
    // `(raw+1)/16`（raw=14 即 15/16，恰等于耕地碰撞盒高度）。与流体的邻域平均
    // 不同，耕地四角取同一常量、没有斜面，直接查位即可。
    //
    // `w`/`h` 因此只在普通轴向面的分支里解码：三条路径互斥，由 face ∈ {6,7} 与
    // 耕地/火把 material 集合保证。两条保证的强度不同，如实记录：植物 material 只
    // 出现在 face 6/7 上是 mesher 打包期显式断言（两侧同口径当场拒绝）；而
    // 「短方块的 material 都落在耕地区间内」不是打包期查得出来的——它是 Go
    // registry 数据的传递性事实（当前只有耕地条目 block_top_raw 非零），区间外
    // 的未来短方块会**静默**走 w/h 路径、顶面不下沉。这是 D2a 选定 material
    // 判别时已接受的边界：新增短方块必须同步扩宽本区间并更新三处钉子。火把
    // 倾斜薄板（角高度承载「向远离支撑方向倾斜」）是同一判别通道的第二个
    // 消费者，走 `torch_material` 同一条角高度路径。
    //
    // 两条对角线（`cu[vi]` 是水平参数 s，`cv[vi]` 是竖直参数 t）：
    //
    //   face 6：(x + s, y + t, z + s)         —— 自 (x, z) 走向 (x+1, z+1)
    //   face 7：(x + 1 - s, y + t, z + s)     —— 自 (x+1, z) 走向 (x, z+1)
    //
    // 正面与背面几何完全相同（terrain 管线 cull_mode 为 None，正背都画），
    // 正/背标志只被 cull.wgsl 用来取相反的法线做背面剔除，这里不读。
    var local: vec3f;
    if (face >= 6u) {
        let s = cu[vi];
        var px = x + s;
        if (face == 7u) {
            px = x + 1.0 - s;
        }
        local = vec3f(px, y + cv[vi], z + s);
    } else if (farmland_material(mat) || torch_material(mat)) {
        local = vec3f(x, y, z)
            + axis_vec(axis) * positive
            + axis_vec(ua) * cu[vi]
            + axis_vec(va) * cv[vi];
        // 角高度非零 ⟺ 该顶点落在这一格的顶面那一层（侧面的下沿顶点与底面
        // 顶点语义上在方块底面、记 0；mesher 只给 y == 格顶的顶点写常量）。
        // 实际高度是 (raw+1)/16：raw=14 即 15/16。
        let raw = corner_height(lo, hi, vi);
        if (raw != 0u) {
            local.y = y + (f32(raw) + 1.0) / 16.0;
        }
    } else {
        let w = f32(((lo >> 12u) & 0xFu) + 1u);
        let h = f32(((lo >> 16u) & 0xFu) + 1u);
        local = vec3f(x, y, z)
            + axis_vec(axis) * positive
            + axis_vec(ua) * (cu[vi] * w)
            + axis_vec(va) * (cv[vi] * h);
    }

    let o = origins[inst.z];
    let world = vec3f(o.xyz) + local;
    let ao_level = f32((ao >> (vi * 2u)) & 0x3u);
    let ao_factor = 0.55 + 0.45 * (ao_level / 3.0);
    let sky = f32((light >> 4u) & 0xFu) / 15.0;
    let block = f32(light & 0xFu) / 15.0;
    let daylight = clamp(camera.cam_pos.w, 0.0, 1.0);
    let sky_base = 0.08 + sky * (daylight - 0.08);
    let base = max(sky_base, block);

    // 交叉斜面的 uv 显式取 (world.x, -world.y)：一片斜面在这两轴上恰好各跨一格，
    // 于是整张材质不多不少铺满一片。斜面的 v 轴正是纵向（world.y），必须与侧面
    // 一样取 -world.y，才能让纹理顶行（草缘等）落在格子顶部，否则小麦等植物
    // 纹理上下颠倒。**不能**图省事交给 face_uv——那里的 axis 只有 0/1/2 三种
    // 语义，植物的 axis = face >> 1 = 3 只是碰巧落进 default。
    var uv = face_uv(world, axis);
    if (face >= 6u) {
        uv = vec2f(world.x, -world.y);
    }

    var out: VsOut;
    out.clip  = camera.view_proj * vec4f(world, 1.0);
    out.uv    = uv;
    out.layer = f32(mat);
    out.shade = face_shade(face) * ao_factor * base;
    return out;
}

@fragment
fn fs_main(in: VsOut) -> @location(0) vec4f {
    let c = textureSample(atlas, atlas_smp, in.uv, i32(in.layer));
    if (c.a < 0.5) { discard; }
    return vec4f(c.rgb * in.shade, 1.0);
}
