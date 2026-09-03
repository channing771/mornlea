struct SectionRec {
    origin:      vec4i,
    face_offset: u32,
    face_count:  u32,
    origin_idx:  u32,
    _pad:        u32,
};

struct CullUniforms {
    cam_pos:      vec4f,
    view_proj:    mat4x4f,
    hiz_size:     vec4f,
    hiz_uv_scale: vec4f,
    // .x = HiZ 启用位;.y = 候选 section 数(cs_scan 需要它做前缀和边界,
    // 与 record 帧写入的 `cull_data[116..120]` 对应)。
    flags:        vec4u,
};

struct DrawArgs {
    index_count:    u32,
    instance_count: atomic<u32>,
    first_index:    u32,
    base_vertex:    u32,
    first_instance: u32,
};

@group(0) @binding(0) var<uniform>             u:        CullUniforms;
@group(0) @binding(1) var<storage, read>       sections: array<SectionRec>;
@group(0) @binding(2) var<storage, read>       faces:    array<u32>;
@group(0) @binding(3) var<storage, read_write> visible:  array<vec4u>;
@group(0) @binding(4) var<storage, read_write> args:     DrawArgs;
@group(0) @binding(5) var hiz: texture_2d<f32>;
// 每个 section 的可见面数:cs_count 写原始计数,cs_scan 就地改写为按候选
// 顺序的排他前缀(cs_place 拿它当本 section 的实例基址)。
@group(0) @binding(6) var<storage, read_write> counts:   array<u32>;

var<workgroup> section_is_occluded: bool;
// cs_count/cs_place(64 lanes)的 lane 合计;两入口不同帧内先后运行,
// 互不并发,共用一块 workgroup 存储。
var<workgroup> lane_sums: array<u32, 64>;
// cs_scan(256 lanes)的 lane 合计,槽位数不同,单独一块。
var<workgroup> scan_sums: array<u32, 256>;

fn axis_vec(axis: u32) -> vec3f {
    if (axis == 0u) { return vec3f(1.0, 0.0, 0.0); }
    if (axis == 1u) { return vec3f(0.0, 1.0, 0.0); }
    return vec3f(0.0, 0.0, 1.0);
}

fn section_occluded(origin: vec3f) -> bool {
    var min_uv = vec2f( 1e30,  1e30);
    var max_uv = vec2f(-1e30, -1e30);
    var min_z = 1e30;

    for (var i = 0u; i < 8u; i++) {
        let corner = origin + vec3f(
            f32( i        & 1u) * 16.0,
            f32((i >> 1u) & 1u) * 16.0,
            f32((i >> 2u) & 1u) * 16.0,
        );
        let clip = u.view_proj * vec4f(corner, 1.0);
        if (clip.w <= 0.0) {
            return false;
        }
        let ndc = clip.xyz / clip.w;
        let uv = ndc.xy * vec2f(0.5, -0.5) + vec2f(0.5, 0.5);
        min_uv = min(min_uv, uv);
        max_uv = max(max_uv, uv);
        min_z = min(min_z, ndc.z);
    }

    min_uv = clamp(min_uv, vec2f(0.0), vec2f(1.0));
    max_uv = clamp(max_uv, vec2f(0.0), vec2f(1.0));
    let size_px = (max_uv - min_uv) * u.hiz_size.xy;
    let level = clamp(
        ceil(log2(max(max(size_px.x, size_px.y), 1.0))),
        0.0, u.hiz_size.z);

    let dim = vec2i(textureDimensions(hiz, u32(level)));
    let padded_min = min_uv * u.hiz_uv_scale.xy;
    let padded_max = max_uv * u.hiz_uv_scale.xy;
    let p0 = clamp(vec2i(floor(padded_min * vec2f(dim))), vec2i(0), dim - 1);
    let p1 = clamp(vec2i(floor(padded_max * vec2f(dim))), vec2i(0), dim - 1);
    let d00 = textureLoad(hiz, vec2i(p0.x, p0.y), i32(level)).r;
    let d10 = textureLoad(hiz, vec2i(p1.x, p0.y), i32(level)).r;
    let d01 = textureLoad(hiz, vec2i(p0.x, p1.y), i32(level)).r;
    let d11 = textureLoad(hiz, vec2i(p1.x, p1.y), i32(level)).r;
    let d = max(max(d00, d10), max(d01, d11));
    return min_z > d;
}

// 单面背面剔除判定(纯函数:同 `lo`、同 uniform 必得同结果——cs_count 与
// cs_place 各自求值一遍,靠这份纯度保证两段的可见集逐面一致)。
fn backface_culled(lo: u32, origin: vec3f) -> bool {
    let face = (lo >> 20u) & 0x7u;
    let cell = vec3f(
        f32( lo         & 0xFu),
        f32((lo >>  4u) & 0xFu),
        f32((lo >>  8u) & 0xFu),
    );

    var normal: vec3f;
    var local: vec3f;
    if (face >= 6u) {
        // 植物的交叉斜面（face 6/7）没有轴向法线，`face >> 1` 会得到 3、
        // `axis_vec` 只会给出一个与几何无关的方向，背面剔除随即按错误的法线
        // 把整片斜面剔掉——那正是"某个方向看不见植物"的成因。
        //
        // 两条对角线的法线（不必归一化，这里只用点积的**符号**）：
        //
        //   face 6：切向 (1,0,1) × (0,1,0) = (-1,0,1)，取其反向 (1,0,-1)
        //   face 7：切向 (-1,0,1) × (0,1,0) = (-1,0,-1)，取其反向 (1,0,1)
        //
        // 取哪个反向无所谓：正/背两条 quad 把两个符号都用上了，任何视角下
        // 恰好留一条。这里取反向只是让两条对角线的常量看起来对称。
        //
        // bit 12 的正/背标志把法线取反。于是一格里的 4 条 quad，任何视角下
        // 每条对角线恰好留下一条、共两条——这就是"出 4 条而不是 2 条 + 关
        // 剔除"的全部理由：剔除照常生效，几何不因视角缺失，也不必为植物
        // 新增管线或每帧状态切换。
        var n = vec3f(1.0, 0.0, -1.0);
        if (face == 7u) {
            n = vec3f(1.0, 0.0, 1.0);
        }
        if (((lo >> 12u) & 1u) == 1u) {
            n = -n;
        }
        normal = n;
        // 判定点取格心：斜面过格心，点积对整片平面同号。
        local = cell + vec3f(0.5, 0.5, 0.5);
    } else {
        // 轴向面（face 0..5）的剔除只依赖 face 平面，**绝不读 bit 12..19**：
        // 普通贪心 quad 在那里装 w/h 尺寸，耕地等短方块与火把倾斜薄板
        // （terrain.wgsl 按 material 区间 29..30 与火把层判别）在同一批位
        // 装四角高度原值。两种语义都
        // 与背面剔除无关——这里按**满格**处理短方块，误差有界且方向单一：
        //
        //   顶面：保留条件是「相机高于判定平面」（+Y 法线的点积符号），
        //         而整格判定平面（y+1）恒在真实下沉表面（y+(raw+1)/16）
        //         **上方**，于是保留集 ⊆ 真实可见集——只可能「该画时
        //         不画」（漏画），绝无多画。唯一误差 = 眼点落入真实表面
        //         上方的 ≤(满格−下沉高度) 竖直带内时顶面被误剔：raw=14
        //         时带宽 1/16，正常站姿眼高不可达；游泳贴水面掠过耕地
        //         顶面时瞬态可达，属可接受的有界近似。若未来短方块下沉
        //         幅度加大，误差带随之加宽、可能留下可见空洞，须重新
        //         评估本近似。
        //   侧面：几何只是上缘两角下沉，所在竖直平面与整格平面共面，
        //         判定逐点等价。
        //   底面：mesher 保证四角为 0、不下沉，与整格完全一致。
        //
        // 若在这里解码尺寸位反而制造回归：耕地的角高度会被当成尺寸放大
        // AABB 或错剔几何（terrain.wgsl 曾因此把耕地渲染成巨型石板）。
        let axis = face >> 1u;
        normal = axis_vec(axis);
        if ((face & 1u) == 0u) {
            normal = -normal;
        }
        local = cell + normal * f32(face & 1u);
    }
    let world = origin + local;

    return dot(normal, world - u.cam_pos.xyz) >= 0.0;
}

// 64-lane 包含式前缀扫描（Hillis-Steele）：进入前 `lane_sums[lane]` 是本
// lane 的合计,返回后是 lane 0..=lane 的包含式前缀。步内「读旧值 → barrier
// → 写 → barrier」两道 barrier 保证读写不交叉;`d` 与循环出口对整个
// workgroup 一致,barrier 处于一致控制流。
fn scan_inclusive_64(lane: u32) {
    // 进入扫描前先同步一次:调用方刚写完 `lane_sums[lane] = acc`,第一步
    // d=1 就要读邻居 lane 的初值——没有这道 barrier,读到的是邻居尚未落盘
    // 的旧值(workgroup 存储零初始化,即 0),前缀随之偏小、槽位前移重叠,
    // 两帧间的胜者随机翻转。这是三段紧凑引入时漏掉的同步。
    workgroupBarrier();
    var d = 1u;
    loop {
        if (d >= 64u) {
            break;
        }
        var src = 0u;
        if (lane >= d) {
            src = lane_sums[lane - d];
        }
        workgroupBarrier();
        lane_sums[lane] = lane_sums[lane] + src;
        workgroupBarrier();
        d <<= 1u;
    }
}

// 阶段一(计数):每 section 一个 workgroup,把可见面数写入 `counts[wg.x]`。
// 槽位与顺序无关——计数是加法,交换律保证结果确定,这是三段紧凑里唯一
// 允许"任意顺序求和"语义的阶段。
@compute @workgroup_size(64)
fn cs_count(
    @builtin(workgroup_id)        wg:  vec3u,
    @builtin(local_invocation_id) lid: vec3u,
) {
    let sec = sections[wg.x];
    if (lid.x == 0u) {
        section_is_occluded =
            u.flags.x != 0u && section_occluded(vec3f(sec.origin.xyz));
    }
    workgroupBarrier();

    var acc = 0u;
    if (!section_is_occluded) {
        let origin = vec3f(sec.origin.xyz);
        for (var i = lid.x; i < sec.face_count; i += 64u) {
            let lo = faces[(sec.face_offset + i) * 2u];
            if (!backface_culled(lo, origin)) {
                acc = acc + 1u;
            }
        }
    }
    lane_sums[lid.x] = acc;
    scan_inclusive_64(lid.x);
    if (lid.x == 63u) {
        counts[wg.x] = lane_sums[63u];
    }
}

// 阶段二(前缀和):单个 256-lane workgroup,把 `counts` 就地改写成按候选
// 顺序的排他前缀,并把全体合计作为总实例数写入 `args.instance_count`
// (单写者 atomicStore,只借用 atomic 语义落盘,不参与任何顺序)。
// lane 按连续分片认领区间,Hillis-Steele 步序固定,输出与输入一一确定。
// 分片 MUST 是连续区间而不是 lane 交错步进:排他前缀要落在**自然下标
// 顺序**上——交错分片得到的是 lane-major 置换序,虽然区间仍然无缝铺满、
// 不丢实例,但实例顺序不再跟随候选序(由近及远的 BFS 序),等深度重叠面
// 的胜者会在整片远景上翻转,既有 golden 基线大面积失配;候选 ≤256 个
// section 时两种分片恰好同构,只有大场景才暴露。
@compute @workgroup_size(256)
fn cs_scan(@builtin(local_invocation_id) lid: vec3u) {
    let n = u.flags.y;
    let lane = lid.x;
    let chunk = (n + 255u) / 256u;
    let beg = min(lane * chunk, n);
    let end = min(beg + chunk, n);

    var acc = 0u;
    for (var i = beg; i < end; i++) {
        acc = acc + counts[i];
    }
    scan_sums[lane] = acc;
    // 与 `scan_inclusive_64` 同理:首步 d=1 读邻居初值前先落盘同步。
    workgroupBarrier();
    var d = 1u;
    loop {
        if (d >= 256u) {
            break;
        }
        var src = 0u;
        if (lane >= d) {
            src = scan_sums[lane - d];
        }
        workgroupBarrier();
        scan_sums[lane] = scan_sums[lane] + src;
        workgroupBarrier();
        d <<= 1u;
    }
    if (lane == 255u) {
        atomicStore(&args.instance_count, scan_sums[255u]);
    }

    var running = scan_sums[lane] - acc;
    for (var i = beg; i < end; i++) {
        let v = counts[i];
        counts[i] = running;
        running = running + v;
    }
}

// 阶段三(定槽写入):每 section 一个 workgroup。全局槽位 = `counts[wg.x]`
// (阶段二的排他前缀,即候选序更早的 section 的可见总数) + lane 排他基址
// + lane 内顺序计数——三段全部只由输入决定。这里曾经用原子追加分配槽位,
// 槽位随 invocation 完成顺序漂移:terrain pass 的深度测试是 `Less`,远处
// 重叠切面在 depth 插值落到同一浮点值时先画者胜,实例顺序漂移让那少数
// 像素随进程启动随机翻转,golden 门禁因此拦不住;MUST NOT 回到原子追加。
@compute @workgroup_size(64)
fn cs_place(
    @builtin(workgroup_id)        wg:  vec3u,
    @builtin(local_invocation_id) lid: vec3u,
) {
    let sec = sections[wg.x];
    if (lid.x == 0u) {
        section_is_occluded =
            u.flags.x != 0u && section_occluded(vec3f(sec.origin.xyz));
    }
    workgroupBarrier();
    if (section_is_occluded) {
        return;
    }
    let origin = vec3f(sec.origin.xyz);

    var acc = 0u;
    for (var i = lid.x; i < sec.face_count; i += 64u) {
        let lo = faces[(sec.face_offset + i) * 2u];
        if (!backface_culled(lo, origin)) {
            acc = acc + 1u;
        }
    }
    lane_sums[lid.x] = acc;
    scan_inclusive_64(lid.x);

    let base = counts[wg.x];
    var running = lane_sums[lid.x] - acc;
    for (var i = lid.x; i < sec.face_count; i += 64u) {
        let words = (sec.face_offset + i) * 2u;
        let lo = faces[words];
        if (backface_culled(lo, origin)) {
            continue;
        }
        visible[base + running] = vec4u(lo, faces[words + 1u], sec.origin_idx, 0u);
        running = running + 1u;
    }
}
