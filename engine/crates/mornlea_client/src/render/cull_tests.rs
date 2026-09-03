//! cull compute 的确定性契约测试(源码扫描 + 离屏图像两类)。
//!
//! 背景:terrain pass 是单次 indirect draw,实例顺序 = `visible` 缓冲的槽位
//! 顺序;深度测试是 `CompareFunction::Less`,两片重叠面在 depth 插值落到
//! 同一个浮点值时**先画者胜**。因此实例顺序必须与输入一一确定,否则每次
//! 进程启动 GPU 调度不同,远处(深度精度只差几个 ulp)的植物切面与地形面
//! 的胜者会在少数像素上翻转——golden 门禁以逐像素比对,单像素翻转即红。
//!
//! 曾经的实现用 `atomicAdd(&args.instance_count, 1u)` 原子追加做紧凑:
//! 槽位取决于 invocation 的完成顺序,跨进程不确定。三段确定性紧凑
//! (计数 → 前缀和 → 定槽写入)取代它之后,槽位只由输入决定;但 Hillis-Steele
//! 扫描的第一步就要读邻居 lane 的初值,漏一道入口 barrier 会让前缀随机
//! 偏小、槽位前移重叠(见 `upload_order_must_not_change_image`)。本模块把
//! 「禁止原子追加、必须三段、扫描必须先同步」钉成契约,防回归。进程内调度
//! 往往稳定,图像用例拦不住某些回归,只能靠源码形态扫描补位。
//!
//! 扫描是**字面形态**的:只拦 `atomicAdd` 这一种写法与三个入口点名;等价
//! 的不确定写法(如分组原子、乱序 sort)不在钉内,须靠评审。

use super::shaders::CULL;
use super::*;

/// cull.wgsl 不得用原子追加分配实例槽位;实例顺序必须由
/// 计数(cs_count)→ 前缀和(cs_scan)→ 定槽写入(cs_place)三段唯一确定。
#[test]
fn cull_compaction_must_not_depend_on_atomic_append_order() {
    assert!(
        !CULL.contains("atomicAdd"),
        "cull.wgsl 不得用 atomicAdd 分配实例槽位:槽位随 invocation 完成顺序\
         \n 变化,等深度重叠面的绘制胜者跨进程不确定,golden 门禁会单像素翻转。\
         \n 实例顺序必须由 cs_count/cs_scan/cs_place 的确定性紧凑唯一决定。"
    );
    for entry in ["fn cs_count", "fn cs_scan", "fn cs_place"] {
        assert!(
            CULL.contains(entry),
            "cull.wgsl 缺少确定性紧凑入口 {entry}:计数、前缀和、定槽写入三段\
             \n 缺一不可,少任何一段都会退回顺序不确定的紧凑。"
        );
    }
    // 总实例数只允许由 scan 阶段单写者写入;若由各 section 原子累加,
    // instance_count 本身虽与顺序无关,但通常意味着紧凑退回了原子追加形态。
    assert!(
        CULL.contains("atomicStore(&args.instance_count"),
        "cull.wgsl 的总实例数必须由 cs_scan 单写者 atomicStore 写入,它是三段\
         \n 紧凑里唯一持有全局前缀和的阶段。"
    );
    // Hillis-Steele 扫描必须在首步 d=1 读邻居 lane 初值之前先同步一次:
    // 漏掉入口 barrier 时读到未落盘的旧值(workgroup 存储零初始化即 0),
    // 前缀偏小、实例槽位前移重叠,叠写胜者随 GPU 调度随机漂移。当前形态
    // 共 8 处:两个 64-lane 入口的 section_is_occluded 同步 ×2、
    // `scan_inclusive_64` 的入口 + 循环内双 barrier(×2 调用点)、cs_scan
    // 内联扫描的入口 + 循环内双 barrier。移除任何一处都应在此拦下。
    assert!(
        CULL.matches("workgroupBarrier();").count() >= 8,
        "cull.wgsl 的 workgroupBarrier 数量低于契约形态:扫描入口同步是三段\
         \n 紧凑正确性的硬前提,移除会让前缀读到未落盘的邻居初值。"
    );
}

// ---------------------------------------------------------------------------
// 以下是紧凑正确性的图像用例(源码扫描拦不住"顺序确定但区间算错"的回归)。
// ---------------------------------------------------------------------------

/// 离屏画面边长:16×16 个区段格、每格 2×2 像素,外加边距。
const VIEW: u32 = 512;
/// PosY(朝上顶面)的 face 编号,与 engine `quad.rs` 的 `Face::PosY` 一致。
const FACE_POS_Y: u32 = 3;
/// 一个 atlas 层(含全部 mip)的字节数:16²+8²+4²+2²+1² 个 RGBA 像素。
const ATLAS_LAYER_BYTES: usize = (256 + 64 + 16 + 4 + 1) * 4;
/// 俯视正交相机:世界 x/z ∈ [0,256) 恰好铺满画面,一格 = 2×2 像素;
/// 世界 y 越大 clip.z 越小(越近),叠层区段靠它在共格上确定性获胜。
fn wide_top_down_view_proj() -> [f32; 16] {
    [
        1.0 / 128.0,
        0.0,
        0.0,
        0.0, // 第 0 列:clip.x = wx/128
        0.0,
        0.0,
        -0.0001,
        0.0, // 第 1 列:clip.z 随 wy 减小
        0.0,
        1.0 / 128.0,
        0.0,
        0.0, // 第 2 列:clip.y = wz/128
        -1.0,
        -1.0,
        0.5,
        1.0, // 第 3 列:平移
    ]
}
/// 相机取画面中心上方,只服务背面剔除的点积判定(正交,位置不影响投影)。
const CAMERA: [f32; 3] = [128.0, 300.0, 128.0];

/// `wide_top_down_view_proj` 的逆:wx/wz=(clip+1)*128,wy=(0.5-clip.z)*10000。
/// sky pass 用它从 clip 坐标重建视线方向,传单位阵会画出乱真的"假云层",
/// 污染基于绝对颜色的断言(farmland 系用例做两帧差分,不受此影响)。
fn wide_top_down_view_proj_inv() -> [f32; 16] {
    [
        128.0, 0.0, 0.0, 0.0, // 第 0 列
        0.0, 0.0, 128.0, 0.0, // 第 1 列
        0.0, -10000.0, 0.0, 0.0, // 第 2 列
        128.0, 5000.0, 128.0, 1.0, // 第 3 列
    ]
}

/// pack 一条 1×1 的 PosY quad(位布局与 engine `quad::Quad::pack` 一致,
/// 与 plant/farmland 测试同一约定:手写一份而不是跨 crate 复用)。
fn pos_y_quad(material: u16) -> u64 {
    u64::from(material) << 23 | u64::from(FACE_POS_Y) << 20 | (0xFF << 39) | (0xFF << 47)
}

/// pack 一条落在 section 内格子 `(x, z)` 的 1×1 PosY quad。
fn pos_y_quad_at(material: u16, x: u32, z: u32) -> u64 {
    let mut q = pos_y_quad(material);
    q &= !0xFFu64;
    q |= u64::from(x) | (u64::from(z) << 4);
    q
}

/// 测试 atlas 四层的颜色:全部 b 通道 ≤ 60,与天空蓝(b≈255)可靠区分;
/// 层 0(网格底面,红主导)与层 2/3(叠层,绿/黄主导)拉开主导通道,供
/// 胜者断言使用。材质层号受纹理数组 ≤256 层的设备上限约束,不得一区段一层。
fn layer_color(t: usize) -> [u8; 4] {
    match t {
        0 => [200, 60, 60, 255],
        2 => [60, 200, 60, 255],
        3 => [200, 200, 60, 255],
        _ => [128, 90, 40, 255],
    }
}

/// 生成 `layers` 层纯色 atlas(逐层逐 mip,与 Go `AtlasPixels` 同布局)。
fn atlas_bytes(layers: usize) -> Vec<u8> {
    let mut out = Vec::with_capacity(layers * ATLAS_LAYER_BYTES);
    for t in 0..layers {
        for mip in 0..ATLAS_MIPS {
            let size = (ATLAS_TEX_SIZE >> mip).max(1) as usize;
            for _ in 0..size * size {
                out.extend_from_slice(&layer_color(t));
            }
        }
    }
    out
}

/// 建渲染器、上传四层 atlas 与区段、渲染一帧并回读;无 GPU 适配器时
/// 返回 None(调用方跳过,与既有离屏用例约定一致)。
fn render_sections(sections: &[(i32, i32, i32, u16)]) -> Option<Vec<u8>> {
    let mut renderer = OffscreenRenderer::new(VIEW, VIEW).ok()?;
    let layers = sections.iter().map(|s| s.3 as usize + 1).max().unwrap_or(1);
    assert!(renderer.upload_atlas(layers as u32, &atlas_bytes(layers)));
    let mut visible = Vec::new();
    for &(px, py, pz, material) in sections {
        let quad = pos_y_quad(material).to_le_bytes();
        assert!(renderer.upload_section((px, py, pz), &quad, &[]));
        visible.push((px, py, pz));
    }
    let frame = FrameInput {
        view_proj: wide_top_down_view_proj(),
        view_proj_inv: wide_top_down_view_proj_inv(),
        pos: CAMERA,
        daylight: 1.0,
        sun_direction: [0.0, 1.0, 0.0],
        star_visibility: 0.0,
        sky_color: [0.25, 0.5, 1.0, 1.0],
        cloud_macro_x: 0,
        cloud_local: 0.0,
        visible,
        ..Default::default()
    };
    assert_eq!(renderer.render_frame(&frame), FrameResult::Rendered);
    let mut image = vec![0u8; (VIEW * VIEW * 4) as usize];
    assert!(renderer.readback(&mut image));
    Some(image)
}

/// 取像素 `[r, g, b]`(回读是 BGRA)。
fn pixel(image: &[u8], x: u32, row: u32) -> [u8; 3] {
    let i = ((row * VIEW + x) * 4) as usize;
    [image[i + 2], image[i + 1], image[i]]
}

/// 区段格 `(gx, gz)`(每格 16×16 世界块) quad 的左上像素。
///
/// 屏幕行号带 Y 翻转:row = VIEW - 2*world_z(farmland 用例 `cell_pixel`
/// 的同一换算),quad 覆盖世界 z ∈ [16gz, 16gz+1),即行 510-32gz..=511-32gz;
/// x 方向无翻转,列恰在 `gx*32`。
fn cell_pixel(image: &[u8], gx: u32, gz: u32) -> [u8; 3] {
    pixel(image, gx * 32, 510 - 32 * gz)
}

/// 258 个区段(> `cs_scan` 的 256 lane)必须一个不漏地全部绘制,且叠层
/// 区段凭更近的深度确定性获胜。
///
/// 三段紧凑的任何一段算错(计数漏面、前缀和越界、定槽重叠)都会在
/// "候选数超过扫描 lane 数"的规模上暴露——既有离屏用例的区段数都以个位
/// 计,`cs_scan` 的多 lane 分片路径从未被图像断言覆盖过。布局:16×16 网格
/// 各放一个单面区段(world y=0),再叠两个区段在网格前两格正上方
/// (world y=16)——它们与网格共格、深度更近,应当确定性获胜。
#[test]
fn compaction_keeps_every_section_beyond_the_scan_lane_count() {
    let mut sections = Vec::new();
    for gz in 0..16u32 {
        for gx in 0..16u32 {
            let t = (gz * 16 + gx) as u16;
            // 区段 pos.y=4 → 世界 y=0(见 WORLD_MIN_Y 换算)。
            sections.push((gx as i32, 4, gz as i32, t));
        }
    }
    // 两个叠层区段:候选序排在 256/257,材质取绿/黄两层以供胜者断言。
    sections.push((0, 5, 0, 2));
    sections.push((1, 5, 0, 3));

    let Some(image) = render_sections(&sections) else {
        return;
    };
    for gz in 0..16u32 {
        for gx in 0..16u32 {
            let [r, g, b] = cell_pixel(&image, gx, gz).map(i32::from);
            assert!(
                b < 150,
                "区段格 ({gx},{gz}) 露出天空色:紧凑丢面(基址错位或区间重叠)\
                 \n 的典型症状,258 个候选一个都不能少"
            );
            let _ = (r, g);
        }
    }
    // 叠层区段必须凭更近的深度赢下共格:这两格的颜色来自绿/黄两层,
    // 而不是被压在下面的网格层 0(红主导)。
    let [r0, g0, _] = cell_pixel(&image, 0, 0).map(i32::from);
    assert!(
        g0 > r0,
        "共格 (0,0) 的胜者应是叠层区段(绿主导),实际红主导 {r0}/{g0}:\
         \n 深度决胜或叠层实例被丢"
    );

    // 对照帧:去掉叠层,同格由网格区段 0(红主导)获胜——两帧差异同时证明
    // 叠层真的画了、对照帧的紧凑也仍然完整。
    let grid_only = sections[..256].to_vec();
    let Some(image_without) = render_sections(&grid_only) else {
        return;
    };
    let [rw, gw, _] = cell_pixel(&image_without, 0, 0).map(i32::from);
    assert!(
        rw > gw,
        "去掉叠层后共格 (0,0) 应由网格区段 0(红主导)获胜,实际 {rw}/{gw}"
    );
}

/// 同一批区段、颠倒上传顺序(面池布局因此不同),图像必须逐字节一致。
///
/// 这是三段紧凑「槽位只由输入决定」契约的直接锁:紧凑依赖 Hillis-Steele
/// 扫描读邻居 lane 初值,入口漏一道 `workgroupBarrier` 时,前缀会随机偏小、
/// 实例槽位前移重叠,叠写胜者随 GPU 调度漂移——同一份世界内容在不同池布局
/// 下渲染出不同画面,golden 门禁整片翻红。每区段 256 条面(远超 64 lane,
/// 单区段也走满 lane 内多轮扫描)是为了让扫描竞态有最大暴露面;12×12 网格
/// 满铺顶面、格间无深度并列,图像差异只能来自槽位错乱而非胜者裁定。
#[test]
fn upload_order_must_not_change_image() {
    let build = |rev: bool| -> Option<Vec<u8>> {
        let mut renderer = OffscreenRenderer::new(VIEW, VIEW).ok()?;
        assert!(renderer.upload_atlas(4, &atlas_bytes(4)));
        // 12×12 网格、每区段 16×16 条 1×1 PosY quad(256 面),材料按
        // (x+z) 奇偶交替,任何槽位错乱都会改变可读出的像素颜色。
        let mut secs: Vec<Vec<u8>> = Vec::new();
        for gz in 0..12i32 {
            for gx in 0..12i32 {
                let mut bytes = Vec::new();
                for z in 0..16u32 {
                    for x in 0..16u32 {
                        let q = pos_y_quad_at(((x + z) % 2) as u16, x, z);
                        bytes.extend_from_slice(&q.to_le_bytes());
                    }
                }
                secs.push(bytes);
            }
        }
        let order: Vec<usize> = if rev {
            (0..secs.len()).rev().collect()
        } else {
            (0..secs.len()).collect()
        };
        for idx in &order {
            let gx = *idx as i32 % 12;
            let gz = *idx as i32 / 12;
            assert!(renderer.upload_section((gx, 4, gz), &secs[*idx], &[]));
        }
        // 可见列表固定按自然顺序:两次构建只差面池布局,不差候选序。
        let mut visible = Vec::new();
        for gz in 0..12i32 {
            for gx in 0..12i32 {
                visible.push((gx, 4, gz));
            }
        }
        let frame = FrameInput {
            view_proj: wide_top_down_view_proj(),
            view_proj_inv: wide_top_down_view_proj_inv(),
            pos: CAMERA,
            daylight: 1.0,
            sun_direction: [0.0, 1.0, 0.0],
            star_visibility: 0.0,
            sky_color: [0.25, 0.5, 1.0, 1.0],
            cloud_macro_x: 0,
            cloud_local: 0.0,
            visible,
            ..Default::default()
        };
        assert_eq!(renderer.render_frame(&frame), FrameResult::Rendered);
        let mut image = vec![0u8; (VIEW * VIEW * 4) as usize];
        assert!(renderer.readback(&mut image));
        Some(image)
    };
    let Some(a) = build(false) else {
        return;
    };
    let Some(b) = build(true) else {
        return;
    };
    let mut n = 0usize;
    let mut first = None;
    for row in 0..VIEW as usize {
        for x in 0..VIEW as usize {
            let i = (row * VIEW as usize + x) * 4;
            if a[i..i + 3] != b[i..i + 3] {
                n += 1;
                first.get_or_insert((x, row));
            }
        }
    }
    assert_eq!(
        n, 0,
        "上传顺序不同导致图像差异 {n} 处,首处 {first:?}:实例槽位依赖了\
         \n 面池布局,紧凑存在槽位错乱或扫描竞态。"
    );
}

/// far-horizon 量级(约 1.9 万候选区段)的重复帧一致性:同一渲染器、同
/// 输入连续渲染多帧,图像必须逐帧一致。紧凑/扫描在真实规模下的任何竞态
/// (如 `cs_scan` 多 lane 分片路径)都靠它兜底。
#[test]
fn repeated_frames_stay_identical_at_far_horizon_scale() {
    let mut renderer = match OffscreenRenderer::new(VIEW, VIEW) {
        Ok(r) => r,
        Err(_) => return,
    };
    assert!(renderer.upload_atlas(4, &atlas_bytes(4)));
    let mut visible = Vec::new();
    let mut k = 0i32;
    'outer: for gz in -70..70i32 {
        for gx in -70..70i32 {
            for py in 4..9i32 {
                if k >= 19458 {
                    break 'outer;
                }
                let quad = pos_y_quad((k % 2) as u16).to_le_bytes();
                assert!(renderer.upload_section((gx, py, gz), &quad, &[]));
                visible.push((gx, py, gz));
                k += 1;
            }
        }
    }
    let frame = FrameInput {
        view_proj: wide_top_down_view_proj(),
        view_proj_inv: wide_top_down_view_proj_inv(),
        pos: CAMERA,
        daylight: 1.0,
        sun_direction: [0.0, 1.0, 0.0],
        star_visibility: 0.0,
        sky_color: [0.25, 0.5, 1.0, 1.0],
        cloud_macro_x: 0,
        cloud_local: 0.0,
        visible,
        ..Default::default()
    };
    let mut first = Vec::new();
    for i in 0..30 {
        assert_eq!(renderer.render_frame(&frame), FrameResult::Rendered);
        let mut image = vec![0u8; (VIEW * VIEW * 4) as usize];
        assert!(renderer.readback(&mut image));
        if i == 0 {
            first = image;
        } else if image != first {
            panic!("第 {i} 帧与第 0 帧不一致:紧凑在大规模下存在竞态");
        }
    }
}
