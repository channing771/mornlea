//! terrain/cull pass 的耕地短方块（角高度）解码测试（farmland-mesh-top-sink D2a）。
//!
//! 与 plant/side/water 测试同源但**不复用**它们的私有夹具：位打包在这里手写
//! 一份——位布局本身是 engine 与 client 两个 crate 之间的契约，任一侧单方面
//! 改位都会以图像不符或断言失败的形式暴露。全部用例共用正交相机：俯视投影下
//! 「一个方块 = 屏幕上一块 16×16 像素的方格」，斜视投影把世界 `y` 掺进
//! `clip.y`，角高度直接体现为屏幕行号（与 water_tests.rs 同一手法）。
//!
//! mesher 侧（Task 1）让耕地 quad 携带角高度：bit 12..19 角 0/1、55..62 角
//! 2/3、恒 1×1 不贪心合并。客户端若沿用 `w/h` 尺寸解码，耕地的角高度原值
//! 14 会被读成 15×15 的巨型石板盖住邻格；本文件钉住 terrain.wgsl 按
//! **material 区间**分流到与 water.wgsl 同源的角高度路径，并把区间常量、
//! Go 层枚举与 shader 字面量三方钉在一起。同一判别路径此后接入了两批新短
//! 方块：墙面火把的倾斜薄板（`TORCH_MATERIAL`，单层）与床的 9/16 半高板
//! （`BED_MATERIAL_FIRST/LAST`，八层），三者的守卫与扫描都在本文件。

use super::shaders::{
    BED_MATERIAL_FIRST, BED_MATERIAL_LAST, CULL, FARMLAND_MATERIAL_FIRST, FARMLAND_MATERIAL_LAST,
    TERRAIN, TORCH_MATERIAL,
};
use super::*;

/// PosY（朝上的顶面）的 face 编号，与 `quad.rs` 的 `Face::PosY` 一致。
const FACE_POS_Y: u32 = 3;
/// 测试 atlas 第 0 层：不透明红，铺作耕地四周的裸地面。
const MAT_RED: u16 = 0;
/// 干/湿耕地在测试 atlas 里的层号取区间两端，一条用例同时覆盖边界值。
const MAT_FARMLAND_DRY: u16 = FARMLAND_MATERIAL_FIRST;
const MAT_FARMLAND_WET: u16 = FARMLAND_MATERIAL_LAST;
/// 生产夹具值：耕地顶面高度原值 14，呈现高度 (14+1)/16 = 15/16，
/// 恰等于物理碰撞体高度（internal/physics 的 farmlandCollisionHeight）。
const FARMLAND_TOP_RAW: u8 = 14;
/// 生产夹具值：床面高度原值 8，呈现高度 (8+1)/16 = 9/16，与 engine
/// `greedy/bed.rs` 的 `BED_TOP_RAW` 及物理床碰撞体同线。
const BED_TOP_RAW: u8 = 8;
/// 满格对照的高度原值：(15+1)/16 = 1，即未下沉的整格顶面。
const FULL_TOP_RAW: u8 = 15;
/// 一个 atlas 层（含全部 mip）的字节数：16²+8²+4²+2²+1² 个 RGBA 像素。
const ATLAS_LAYER_BYTES: usize = (256 + 64 + 16 + 4 + 1) * 4;
/// 离屏画面边长。
const VIEW: u32 = 64;
/// 默认相机位置：区段正上方（顶面朝上可见，cull 背面剔除按它判定）。
const DEFAULT_CAMERA: [f32; 3] = [8.0, 100.0, 8.0];

/// pack_quad 复刻 `mornlea_engine::quad::Quad::pack` 的位布局。
///
/// 这里**必须**手写一份而不是复用 engine：client crate 不依赖 engine（理由同
/// water_tests.rs 的同名注释）。带角高度的 quad 必须是 1×1——这正是本主题的
/// 前提，违反即 panic。
#[allow(clippy::too_many_arguments)]
fn pack_quad(
    x: u8,
    y: u8,
    z: u8,
    face: u32,
    material: u16,
    ao: u8,
    light: u8,
    corners: [u8; 4],
) -> u64 {
    assert!(corners.iter().all(|&corner| corner <= 15));
    let low = u64::from(corners[0]) << 12 | u64::from(corners[1]) << 16;
    let high = u64::from(corners[2]) << 55 | u64::from(corners[3]) << 59;
    u64::from(x)
        | u64::from(y) << 4
        | u64::from(z) << 8
        | low
        | u64::from(face) << 20
        | u64::from(material) << 23
        | u64::from(ao) << 39
        | u64::from(light) << 47
        | high
}

/// 一格裸地面：区段局部 y=0 的满格红色顶面（顶面在局部 y=1）。
///
/// 区段坐标经 origin 表换算为世界坐标：origin.y = `pos.1*16 + WORLD_MIN_Y`，
/// 本文件统一用区段 (0,4,0)，即世界 y 从 0 起。角高度全 0 时 pack 出的
/// bit 12..19 恰好是 `w-1=0 / h-1=0`——普通 1×1 quad 与带角高度 quad 共用
/// 这段位，两种形态在这里自然统一。
fn floor_cell(bx: u8, bz: u8) -> u64 {
    pack_quad(bx, 0, bz, FACE_POS_Y, MAT_RED, 0xFF, 0xFF, [0; 4])
}

/// 一格耕地顶面：顶面从局部整格 y=1 下沉到 (raw+1)/16。
fn farmland_cell(bx: u8, bz: u8, material: u16, raw: u8) -> u64 {
    pack_quad(bx, 0, bz, FACE_POS_Y, material, 0xFF, 0xFF, [raw; 4])
}

/// 俯视正交 view_proj（列主序，与 WGSL 的 mat4x4f 内存布局一致），与
/// water_tests.rs 同款：世界 x/z 各除以 2 平移 4，一个方块恰为 16×16 像素。
fn top_down_view_proj() -> [f32; 16] {
    [
        0.5, 0.0, 0.0, 0.0, // 第 0 列
        0.0, 0.0, -0.005, 0.0, // 第 1 列
        0.0, 0.5, 0.0, 0.0, // 第 2 列
        -4.0, -4.0, 0.5, 1.0, // 第 3 列
    ]
}

/// 斜视 view_proj：在俯视基础上把世界 `y` 以大斜率掺进 `clip.y`。
///
/// ```text
/// clip.x = world.x * 0.25 - 2
/// clip.y = world.y * 2.0 + world.z * 0.125 - 3.06
/// clip.z = 0.5 - world.y / 200
/// ```
///
/// 本文件的区段 (0,4,0) 经 origin 表（`pos.1*16 + WORLD_MIN_Y`）落在世界 y=0，
/// 耕地顶面因此在世界 y≈1：clip.y = 2 + [1.0,1.125] − 3.06 ≈ [−0.06,0.07]，
/// 满格对照与下沉两版都完整落在画面内。
///
/// 高度差 Δy 对应 `clip.y` 差 `2Δy`，即屏幕 `2Δy / 2 * 64 = 64Δy` 行。耕地
/// （raw 14，顶面 15/16）与满格对照（raw 15，顶面 1）差 Δy = 1/16 → **恰好
/// 4 行**，信号远超光栅化抖动；y 斜率再小（如 water_tests 的 0.15）会把
/// 1/16 缩到亚像素级，无法判级。
fn oblique_view_proj() -> [f32; 16] {
    [
        0.25, 0.0, 0.0, 0.0, // 第 0 列
        0.0, 2.0, -0.005, 0.0, // 第 1 列
        0.0, 0.125, 0.0, 0.0, // 第 2 列
        -2.0, -3.06, 0.5, 1.0, // 第 3 列
    ]
}

/// 生成 `colors.len()` 层纯色 atlas（逐层逐 mip，与 Go `AtlasPixels` 同布局）。
fn atlas_bytes(colors: &[[u8; 4]]) -> Vec<u8> {
    let mut out = Vec::with_capacity(colors.len() * ATLAS_LAYER_BYTES);
    for color in colors {
        for mip in 0..ATLAS_MIPS {
            let size = (ATLAS_TEX_SIZE >> mip).max(1) as usize;
            for _ in 0..size * size {
                out.extend_from_slice(color);
            }
        }
    }
    out
}

/// 测试用的纯色表：层数必须大于 [`BED_MATERIAL_LAST`]，否则床采样层
/// 越界（WGSL 对越界层号的行光是未定义行为，绝不能让断言建立在它上面）。
///
/// 层 0 不透明红（裸地面）、层 29（干耕地）不透明绿、层 30（湿耕地）不透明蓝，
/// 层 60（床尾南向）不透明琥珀，其余层填不透明灰占位。各色两两可按主导通道
/// 区分，断言不需要逐字节相等。
fn test_colors() -> Vec<[u8; 4]> {
    let mut colors = vec![[90u8, 90, 90, 255]; 68];
    colors[MAT_RED as usize] = [200, 60, 60, 255];
    colors[MAT_FARMLAND_DRY as usize] = [60, 200, 60, 255];
    colors[MAT_FARMLAND_WET as usize] = [60, 90, 220, 255];
    colors[BED_MATERIAL_FIRST as usize] = [220, 170, 60, 255];
    colors
}

/// u64 quad 序列 → 上传字节（小端，8 字节/条）。
fn quad_bytes(quads: &[u64]) -> Vec<u8> {
    let mut out = Vec::with_capacity(quads.len() * 8);
    for quad in quads {
        out.extend_from_slice(&quad.to_le_bytes());
    }
    out
}

/// 一个区段的两条流（耕地走不透明 terrain 流，经 cull compute 后绘制）。
struct SectionData {
    pos: (i32, i32, i32),
    opaque: Vec<u64>,
    water: Vec<u64>,
}

/// 建渲染器、上传 atlas 与区段、渲染**一帧**并回读 BGRA 图像。
///
/// 每次都新建渲染器：首帧 HiZ 必然停用、无上一帧状态，图像只取决于本用例的
/// 输入。无 GPU 适配器时返回 None（调用方跳过，与既有约定一致）。
fn render_once(view_proj: [f32; 16], sections: &[SectionData]) -> Option<Vec<u8>> {
    let mut renderer = OffscreenRenderer::new(VIEW, VIEW).ok()?;
    assert!(renderer.upload_atlas(test_colors().len() as u32, &atlas_bytes(&test_colors())));
    let mut visible = Vec::new();
    for section in sections {
        assert!(renderer.upload_section(
            section.pos,
            &quad_bytes(&section.opaque),
            &quad_bytes(&section.water),
        ));
        visible.push(section.pos);
    }
    let mut identity = [0.0f32; 16];
    for i in 0..4 {
        identity[i * 4 + i] = 1.0;
    }
    let frame = FrameInput {
        view_proj,
        view_proj_inv: identity,
        pos: DEFAULT_CAMERA,
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

/// 取一个像素，返回 `[r, g, b]`（回读是 BGRA）。
fn pixel(image: &[u8], x: u32, row: u32) -> [u8; 3] {
    let i = ((row * VIEW + x) * 4) as usize;
    [image[i + 2], image[i + 1], image[i]]
}

/// image_diff 返回 `(不同像素数, 首个不同像素坐标)`；完全相同时返回 None。
fn image_diff(a: &[u8], b: &[u8]) -> Option<(usize, (u32, u32))> {
    let mut count = 0usize;
    let mut first = None;
    for row in 0..VIEW {
        for x in 0..VIEW {
            if pixel(a, x, row) != pixel(b, x, row) {
                count += 1;
                first.get_or_insert((x, row));
            }
        }
    }
    first.map(|at| (count, at))
}

/// 俯视相机下方块 `(bx, bz)` 的中心像素。
fn cell_pixel(image: &[u8], bx: u32, bz: u32) -> [u8; 3] {
    // clip.x = (bx + 0.5) / 2 - 4，屏幕 x = (clip.x + 1) / 2 * 64。
    let x = (((bx as f32 + 0.5) / 2.0 - 4.0) + 1.0) / 2.0 * VIEW as f32;
    let clip_y = (bz as f32 + 0.5) / 2.0 - 4.0;
    let row = (1.0 - clip_y) / 2.0 * VIEW as f32;
    pixel(image, x as u32, row as u32)
}

/// 从 `src` 里截取 `marker` 起到函数体结束（收在首个 `}` 行）的一段，
/// 供字面量扫描。按行截取而不是按字节，避免切进多字节 UTF-8 字符。
fn segment_after(src: &str, marker: &str) -> String {
    let at = src
        .find(marker)
        .unwrap_or_else(|| panic!("shader 源里找不到 {marker}"));
    src[at..]
        .lines()
        .take(12)
        .take_while(|line| !line.trim_start().starts_with('}'))
        .collect::<Vec<_>>()
        .join("\n")
}

/// 区间常量必须等于 Go `assets.LayerFarmlandDry/Wet` 的当前枚举值。
///
/// 三处手工同步、无共享定义：Go 层枚举（真值源）、本 crate 常量、terrain.wgsl
/// 的 `farmland_material` 字面量。Go 侧钉子是 internal/assets 的
/// `TestFarmlandLayerNumbersMatchClientShaderContract`，shader 一侧由下一条
/// 用例扫源码钉住。在 Go 枚举耕地之前插层会整体平移区间——客户端会把耕地当
/// 普通满格方块渲染（顶面不下沉）、甚至把相邻层误判成短方块，那两处守卫就是
/// 仅有的报警点。
#[test]
fn farmland_range_constants_match_go_layer_enum() {
    // 这两个字面量与 internal/assets/blocks.go 的层枚举 iota 逐一对应：
    // …`LayerWater`(28)、`LayerFarmlandDry`(29)、`LayerFarmlandWet`(30)、`LayerWheat0`(31)…
    assert_eq!(FARMLAND_MATERIAL_FIRST, 29, "Go `LayerFarmlandDry`=29");
    assert_eq!(FARMLAND_MATERIAL_LAST, 30, "Go `LayerFarmlandWet`=30");
    // 区间必须恰好覆盖干湿两层：放宽会把水层(28)或小麦层(31)卷进角高度路径。
    assert_eq!(
        FARMLAND_MATERIAL_LAST - FARMLAND_MATERIAL_FIRST,
        1,
        "耕地区间只允许覆盖干湿两层"
    );
}

/// terrain.wgsl 的耕地区间字面量必须与本 crate 常量一致，cull.wgsl 必须保持
/// 「不解码 bit 16..19」的满格处理形态。
///
/// WGSL 没有 include/常量注入机制，shader 里的数值只能硬编码；这条源码扫描把
/// shader 字面量与 Rust 常量焊在一起——改常量不改 shader（或反过来）都会红。
#[test]
fn shader_sources_stay_pinned_to_the_range_constants() {
    let seg = segment_after(TERRAIN, "fn farmland_material");
    assert!(
        seg.contains(&format!(">= {FARMLAND_MATERIAL_FIRST}u")),
        "terrain.wgsl 的耕地区间下界与本 crate 常量不一致：\n{seg}"
    );
    assert!(
        seg.contains(&format!("<= {FARMLAND_MATERIAL_LAST}u")),
        "terrain.wgsl 的耕地区间上界与本 crate 常量不一致：\n{seg}"
    );

    // 火把层是角高度路径的第二个消费者(墙面火把倾斜薄板),同一套「Go 枚举、
    // 本 crate 常量、terrain.wgsl 字面量」三方手工同步,同一处扫描一并钉住。
    let torch_seg = segment_after(TERRAIN, "fn torch_material");
    assert!(
        torch_seg.contains(&format!("== {TORCH_MATERIAL}u")),
        "terrain.wgsl 的火把层号与本 crate 常量不一致：\n{torch_seg}"
    );

    // 床层区间是角高度路径的第三个消费者(9/16 半高板,五条 quad 全带角高度),
    // 同一套三方手工同步,同一处扫描一并钉住;判别函数必须真实接入角高度路径
    // 的分流门,只定义不接入时床 quad 仍会走 w/h 解码摊成巨型石板。
    let bed_seg = segment_after(TERRAIN, "fn bed_material");
    assert!(
        bed_seg.contains(&format!(">= {BED_MATERIAL_FIRST}u")),
        "terrain.wgsl 的床区间下界与本 crate 常量不一致：\n{bed_seg}"
    );
    assert!(
        bed_seg.contains(&format!("<= {BED_MATERIAL_LAST}u")),
        "terrain.wgsl 的床区间上界与本 crate 常量不一致：\n{bed_seg}"
    );
    assert!(
        TERRAIN.contains("torch_material(mat) || bed_material(mat)"),
        "terrain.wgsl 的角高度分流门必须包含床判别：床 quad 脱门即巨型石板回归"
    );

    // cull.wgsl 对耕地 quad 按**满格**做背面剔除（误差有界、只可能漏画，
    // 论证见 shader 内注释）：bit 16..19 在耕地 quad 上是角高度、在普通
    // quad 上是 h 尺寸，两种语义都不该被剔除路径读到。若有人给 cull 补上
    // 「尺寸」解码，这里当场红。覆盖边界：本钉是**字面形态**扫描，只拦
    // `>> 16u` 这一种写法——bit 12/55..62 的解码与不带 u 后缀的等价写法
    // （如 `>> 16`）不在钉内，须靠评审。
    assert!(
        !CULL.contains(">> 16u"),
        "cull.wgsl 不得解码 bit 16..19：耕地 quad 上那是角高度、普通 quad 上那是 \
         h 尺寸，剔除只需要 face 平面，误读只会放大 AABB 或错剔几何"
    );
}

/// 耕地顶面只覆盖自己那一格——巨型石板回归的直接锁。
///
/// 耕地 quad 的 bit 12..19 是角高度原值 14；若 terrain.wgsl 沿用 `w/h` 解码，
/// 它会摊成 15×15、贴在整格顶面平面上的石板，盖住右下方邻格。对照法刻意
/// **不放**与石板共面的地板来「挡」它——同平面深度并列时先画者胜，会把回归
/// 洗成假绿——而是对同一场景做「有/无耕地」两帧渲染，断言石板覆盖范围内的
/// 空格子逐像素不变、耕地格自身有变化。湿耕地走同一判别路径，一并覆盖。
#[test]
fn farmland_top_face_covers_only_its_own_cell() {
    let scene = |material: u16, with_farmland: bool| {
        // 地板只摆在石板范围之外（x<8），证明地面渲染本身不受影响。
        let mut opaque = vec![floor_cell(6, 6), floor_cell(6, 9)];
        if with_farmland {
            opaque.push(farmland_cell(8, 8, material, FARMLAND_TOP_RAW));
        }
        SectionData {
            pos: (0, 4, 0),
            opaque,
            water: vec![],
        }
    };
    for material in [MAT_FARMLAND_DRY, MAT_FARMLAND_WET] {
        let Some(without) = render_once(top_down_view_proj(), &[scene(material, false)]) else {
            return;
        };
        let with = render_once(top_down_view_proj(), &[scene(material, true)])
            .expect("首个场景已成功建过渲染器");
        // (9,8)/(8,9)/(9,9) 特意摆在 15×15 石板的必经之路上且**没有**别的几何：
        // 石板一旦出现，这些格子相对无耕地帧必然变色。
        for (bx, bz) in [(9u32, 8u32), (8, 9), (9, 9)] {
            assert_eq!(
                cell_pixel(&without, bx, bz),
                cell_pixel(&with, bx, bz),
                "material={material} 耕地不得越出自身一格：({bx},{bz}) 被石板化耕地覆盖"
            );
        }
        assert_ne!(
            cell_pixel(&without, 8, 8),
            cell_pixel(&with, 8, 8),
            "耕地格自身毫无变化：夹具空转，上面的邻格断言不承重"
        );
    }
}

/// 耕地顶面必须真的下沉，且幅度对得上位布局：与满格对照差恰好 1/16 世界高度。
///
/// 斜视投影把世界 `y` 以斜率 2 掺进 `clip.y`，高度差 1/16 即屏幕 4 行。断言
/// 位移落在 3..=6 行。本用例钉住的是两个性质：**斜率**（顶点高度每差一个
/// raw 单位，屏幕位移恰为 1/16 格）与**完全不解码为 0 位移**；两帧差分对
/// 仿射偏移不敏感——任何斜率正确的公式（无论常数项）位移都恒 4 行，因此
/// 绝对高度在这里没有锚点，「公式抄错但斜率不变」的形态要靠常量与公式两侧
/// 的源码钉子和评审兜住：
///
/// - 完全不解码角高度（顶点恒在 y+1）→ 两版重合，位移 0，红；
/// - 沿用 `w/h` 解码 → 面变成 15×15/16×16 石板，首行差异远超带宽或直接出屏，红；
/// - 斜率抄错（例如漏掉 /16）→ 位移偏离 4 行带宽，红。
#[test]
fn farmland_top_edge_sinks_below_a_full_height_control() {
    for material in [MAT_FARMLAND_DRY, MAT_FARMLAND_WET] {
        let empty = SectionData {
            pos: (0, 4, 0),
            opaque: vec![],
            water: vec![],
        };
        let Some(empty_image) = render_once(oblique_view_proj(), std::slice::from_ref(&empty))
        else {
            return;
        };
        let top_row = |raw: u8| {
            let image = render_once(
                oblique_view_proj(),
                &[SectionData {
                    pos: (0, 4, 0),
                    opaque: vec![farmland_cell(8, 8, material, raw)],
                    water: vec![],
                }],
            )
            .expect("首个场景已成功建过渲染器");
            (0..VIEW)
                .find(|&row| {
                    (0..VIEW).any(|x| {
                        let (a, b) = (pixel(&image, x, row), pixel(&empty_image, x, row));
                        (0..3).any(|c| a[c].abs_diff(b[c]) >= 8)
                    })
                })
                .unwrap_or_else(|| panic!("高度 {raw} 的耕地顶面完全没有出现在画面上"))
        };
        let sunk = top_row(FARMLAND_TOP_RAW);
        let full = top_row(FULL_TOP_RAW);
        assert!(
            sunk > full,
            "下沉的耕地顶面必须比满格对照更靠下（行号更大）：sunk={sunk} full={full}"
        );
        let shift = sunk - full;
        assert!(
            (3..=6).contains(&shift),
            "高度 15 → 14 的位移应约 4 行，实测 {shift} 行（sunk={sunk} full={full}）"
        );
    }
}

/// 全链路守卫排在最后：两幅不同材质的耕地画面必须有差异，证明上面的渲染夹具
/// 不是在比较两张空图。（干/绿 vs 湿/蓝在同一格必然逐像素可分。）
#[test]
fn dry_and_wet_farmland_render_distinguishably() {
    let scene = |material: u16| SectionData {
        pos: (0, 4, 0),
        opaque: vec![farmland_cell(8, 8, material, FARMLAND_TOP_RAW)],
        water: vec![],
    };
    let Some(dry) = render_once(top_down_view_proj(), &[scene(MAT_FARMLAND_DRY)]) else {
        return;
    };
    let wet = render_once(top_down_view_proj(), &[scene(MAT_FARMLAND_WET)])
        .expect("首个场景已成功建过渲染器");
    assert!(
        image_diff(&dry, &wet).is_some(),
        "干湿两态画面逐像素相同：材质层没有进入渲染路径"
    );
}

/// 床区间常量必须等于 Go `assets.LayerBedFootSouth`..`LayerBedHeadEast` 的
/// 当前冻结枚举值（60..67，火把层 59 之后、短草层 68 之前）。
///
/// 三处手工同步、无共享定义：Go 层枚举（真值源）、本 crate 常量、
/// terrain.wgsl 的 `bed_material` 字面量。Go 侧钉子是 internal/assets 的
/// `TestBedLayerNumbersMatchClientShaderContract`，shader 一侧由
/// `shader_sources_stay_pinned_to_the_range_constants` 扫源码钉住。在 Go 枚举
/// 床之前插层会整体平移区间——床 quad 会脱门走 `w/h` 解码摊成巨型石板，
/// 那两处守卫与下面的渲染回归是报警点。
#[test]
fn bed_range_constants_match_go_layer_enum() {
    assert_eq!(BED_MATERIAL_FIRST, 60, "Go `LayerBedFootSouth`=60");
    assert_eq!(BED_MATERIAL_LAST, 67, "Go `LayerBedHeadEast`=67");
    // 区间必须恰好覆盖床尾/床头 × 南西北东八层：放宽会把火把层(59)卷进
    // 区间，收紧会让床的一部分形态脱门走 w/h 解码。
    assert_eq!(
        BED_MATERIAL_LAST - BED_MATERIAL_FIRST,
        7,
        "床区间恰好覆盖八张床面层"
    );
}

/// 床顶面只覆盖自己那一格——巨型石板回归的直接锁。
///
/// 床是 9/16 短方块：顶面 quad 四角带高度原值 8；terrain.wgsl 若沿用 `w/h`
/// 解码，角 0/角 1 的 8 会被读成 9×9 巨型石板、贴在整格顶面平面上盖住右下
/// 邻格。对照法与耕地用例同款：对同一场景做「有/无床」两帧渲染，断言石板
/// 覆盖范围内的空格子逐像素不变、床格自身有变化。床面层取区间下界即可——
/// 判别按 material 区间分流，八层的解码路径同门。
#[test]
fn bed_top_face_covers_only_its_own_cell() {
    let scene = |with_bed: bool| {
        // 地板只摆在石板范围之外（x<8），证明地面渲染本身不受影响。
        let mut opaque = vec![floor_cell(6, 6), floor_cell(6, 9)];
        if with_bed {
            opaque.push(pack_quad(
                8,
                0,
                8,
                FACE_POS_Y,
                BED_MATERIAL_FIRST,
                0xFF,
                0xFF,
                [BED_TOP_RAW; 4],
            ));
        }
        SectionData {
            pos: (0, 4, 0),
            opaque,
            water: vec![],
        }
    };
    let Some(without) = render_once(top_down_view_proj(), &[scene(false)]) else {
        return;
    };
    let with = render_once(top_down_view_proj(), &[scene(true)]).expect("首个场景已成功建过渲染器");
    // (9,8)/(8,9)/(9,9) 摆在 9×9 石板的必经之路上且没有别的几何：石板一旦
    // 出现，这些格子相对无床帧必然变色。
    for (bx, bz) in [(9u32, 8u32), (8, 9), (9, 9)] {
        assert_eq!(
            cell_pixel(&without, bx, bz),
            cell_pixel(&with, bx, bz),
            "床不得越出自身一格：({bx},{bz}) 被石板化床顶覆盖"
        );
    }
    assert_ne!(
        cell_pixel(&without, 8, 8),
        cell_pixel(&with, 8, 8),
        "床格自身毫无变化：夹具空转，上面的邻格断言不承重"
    );
}

/// 床顶面必须真的下沉到 9/16，且幅度对得上位布局：与满格对照差 7/16 世界
/// 高度（角高度 8 → (8+1)/16，满格 15 → 1）。
///
/// 沿用耕地斜视投影的同一手法：高度差 Δy 对应屏幕 64Δy 行，7/16 即 28 行，
/// 带宽取 26..=30。断言顶面**出现了**且比满格对照更靠下——完全不解码角
/// 高度（顶点恒在 y+1）或沿用 w/h 解码（巨型石板首行远移或出屏）都会红。
#[test]
fn bed_top_edge_sinks_below_a_full_height_control() {
    let empty = SectionData {
        pos: (0, 4, 0),
        opaque: vec![],
        water: vec![],
    };
    let Some(empty_image) = render_once(oblique_view_proj(), std::slice::from_ref(&empty)) else {
        return;
    };
    let top_row = |raw: u8| {
        let image = render_once(
            oblique_view_proj(),
            &[SectionData {
                pos: (0, 4, 0),
                opaque: vec![pack_quad(
                    8,
                    0,
                    8,
                    FACE_POS_Y,
                    BED_MATERIAL_FIRST,
                    0xFF,
                    0xFF,
                    [raw; 4],
                )],
                water: vec![],
            }],
        )
        .expect("首个场景已成功建过渲染器");
        (0..VIEW)
            .find(|&row| {
                (0..VIEW).any(|x| {
                    let (a, b) = (pixel(&image, x, row), pixel(&empty_image, x, row));
                    (0..3).any(|c| a[c].abs_diff(b[c]) >= 8)
                })
            })
            .unwrap_or_else(|| panic!("高度 {raw} 的床顶面完全没有出现在画面上"))
    };
    let sunk = top_row(BED_TOP_RAW);
    let full = top_row(FULL_TOP_RAW);
    assert!(
        sunk > full,
        "下沉的床顶面必须比满格对照更靠下（行号更大）：sunk={sunk} full={full}"
    );
    let shift = sunk - full;
    assert!(
        (26..=30).contains(&shift),
        "高度 15 → 8 的位移应约 28 行，实测 {shift} 行（sunk={sunk} full={full}）"
    );
}

/// 角高度 quad 不论 material 落在哪个短方块集合，都必须走角高度解码。
///
/// 床的四片侧板读各自面的材质、生产注册表给的是与满格方块共享的橡木木板
/// 层——material 判别对它们原理性失效，侧板曾被摊成 1×9 长板。分流门为此
/// 保留了与 Go 侧 `UnpackQuad` 同语义的结构判别（角 2 非 0 ⟺ 角高度 quad）。
/// 本用例用一条「橡木木板 material + 四角高度 8」的 PosY quad 钉住该路由：
/// material 不在任何短方块集合里，几何上与床平顶同形——若门退回纯 material
/// 判别，它会摊成 9×9 石板盖住邻格，断言当场红。
#[test]
fn corner_height_quads_route_regardless_of_material() {
    let scene = |with_short_quad: bool| {
        let mut opaque = vec![floor_cell(6, 6), floor_cell(6, 9)];
        if with_short_quad {
            // 层 20 在生产 atlas 里是橡木木板层：刻意选一个不在任何短方块
            // material 集合里的层号，复现侧板的判别处境。
            opaque.push(pack_quad(
                8,
                0,
                8,
                FACE_POS_Y,
                20,
                0xFF,
                0xFF,
                [BED_TOP_RAW; 4],
            ));
        }
        SectionData {
            pos: (0, 4, 0),
            opaque,
            water: vec![],
        }
    };
    let Some(without) = render_once(top_down_view_proj(), &[scene(false)]) else {
        return;
    };
    let with = render_once(top_down_view_proj(), &[scene(true)]).expect("首个场景已成功建过渲染器");
    for (bx, bz) in [(9u32, 8u32), (8, 9), (9, 9)] {
        assert_eq!(
            cell_pixel(&without, bx, bz),
            cell_pixel(&with, bx, bz),
            "角高度 quad 不得因 material 在短方块集合之外而被摊成巨型石板：({bx},{bz}) 被覆盖"
        );
    }
    assert_ne!(
        cell_pixel(&without, 8, 8),
        cell_pixel(&with, 8, 8),
        "角高度 quad 自身毫无变化：夹具空转，上面的邻格断言不承重"
    );
}
