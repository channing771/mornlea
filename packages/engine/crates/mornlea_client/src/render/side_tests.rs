//! 方块侧脸纹理朝向的离屏守护测试（无窗口，走既有 `OffscreenRenderer` + `readback`）。
//!
//! 与 `plant_tests.rs` 同源但**不复用**它的私有夹具：那里的 atlas 是逐层纯色、
//! 相机是水平正交但只为区分两条对角线；本套夹具的核心是单层「上半红/下半绿」
//! 的 16×16 纹理（R0..R7 红、R8..R15 绿），把「纹理顶行必须落在方块顶部」直接
//! 读出来——侧面与交叉斜面的 v 轴是纵向，着色器若把 `world.y` 当作 v（或让 ±X
//! 面把 u 当纵向），结果都会翻转/旋转，本套用例以红上绿下的像素判据报警。
//!
//! 位打包同样手写一份（与 plant/water 同理由：位布局是 engine 与 client 两个
//! crate 之间的契约，任一侧改位而另一侧不改，这些用例会以图像不符的形式报出）。

use super::*;

/// ±X 面的正向（朝 +X）face 编号，与 engine `quad.rs` 的 `Face::PosX` 一致。
const FACE_POS_X: u32 = 1;
/// −Z 面的 face 编号，与 `Face::NegZ` 一致。
const FACE_NEG_Z: u32 = 4;
/// +Z 面的 face 编号，与 `Face::PosZ` 一致。
const FACE_POS_Z: u32 = 5;
/// 植物对角线 A 的 face 编号，与 `Face::PlantDiagA` 一致。
const FACE_PLANT_A: u32 = 6;
/// 单层测试 atlas 的层号（红上绿下）。
const MAT_TEST: u16 = 0;

/// 一个 atlas 层（含全部 mip）的字节数：16²+8²+4²+2²+1² 个 RGBA 像素。
const ATLAS_LAYER_BYTES: usize = (256 + 64 + 16 + 4 + 1) * 4;
/// 离屏画面边长。
const VIEW: u32 = 64;
/// 被观察的区段。`pos.1` 必须为 4：区段原点是 `pos * 16 + WORLD_MIN_Y`，
/// 而 `WORLD_MIN_Y = -64`，取 4 才让原点落在世界 `y = 0`（与 plant_tests.rs 同约）。
const SECTION: (i32, i32, i32) = (0, 4, 0);
/// 被观察格在区段里的局部坐标，也是世界坐标（区段原点在世界原点）。
const CELL: (u8, u8, u8) = (8, 8, 8);
/// 被观察格的中心世界坐标。
const CELL_CENTER: [f32; 3] = [8.5, 8.5, 8.5];
/// 方块面在离屏画面上占据的正中方块的屏幕行 / 列边界：[24, 40)。
const BLOCK_BEGIN: u32 = 24;
const BLOCK_END: u32 = 40;
/// 方块区域的垂直中点：上半为格子顶部（world.y 更大）、下半为格子底部。
const BLOCK_MID: u32 = 32;

/// pack_quad 复刻 `mornlea_engine::quad::Quad::pack` 的**植物与普通两条**位布局。
///
/// | 位 | 普通 quad | 植物 quad |
/// |---|---|---|
/// | 0..11 | x / y / z | 同左 |
/// | 12 | w-1 的最低位 | 正/背标志（0 = 正、1 = 背） |
/// | 13..19 | w-1 高位与 h-1 | 保留，必须为 0 |
/// | 20..22 | face 0..5 | face 6 / 7 |
/// | 23..38 | material | 同左 |
/// | 39..46 | ao | 同左 |
/// | 47..54 | light | 同左 |
/// | 63 | 永久留空 | 永久留空 |
#[allow(clippy::too_many_arguments)]
fn pack_quad(x: u8, y: u8, z: u8, w: u8, h: u8, face: u32, material: u16, back: bool) -> u64 {
    let low = if face >= 6 {
        assert!(w == 1 && h == 1, "植物 quad 必须是 1×1");
        u64::from(back) << 12
    } else {
        assert!(!back, "非植物 quad 不得设置正背标志");
        u64::from(w - 1) << 12 | u64::from(h - 1) << 16
    };
    let packed = u64::from(x)
        | u64::from(y) << 4
        | u64::from(z) << 8
        | low
        | u64::from(face) << 20
        | u64::from(material) << 23
        | 0xFFu64 << 39
        | 0xFFu64 << 47;
    assert_eq!(packed >> 63, 0, "quad 占用了必须留空的 bit 63");
    packed
}

/// u64 quad 序列 → 上传字节（小端，8 字节/条）。
fn quad_bytes(quads: &[u64]) -> Vec<u8> {
    let mut out = Vec::with_capacity(quads.len() * 8);
    for quad in quads {
        out.extend_from_slice(&quad.to_le_bytes());
    }
    out
}

/// 生成单层 atlas：每级 mip 的上半行不透明红、下半行不透明绿。
/// R0..R7 为红（纹理顶行）、R8..R15 为绿，在所有 mip 级保持一致，使无论采样
/// 落到哪一级 mip，顶行都是红、底行都是绿——「顶行=方块顶部」这一性质不变。
fn atlas_red_top_green_bottom() -> Vec<u8> {
    let red = [200u8, 60, 60, 255];
    let green = [60u8, 200, 60, 255];
    let mut out = Vec::with_capacity(ATLAS_LAYER_BYTES);
    for mip in 0..ATLAS_MIPS {
        let size = (ATLAS_TEX_SIZE >> mip).max(1) as usize;
        let half = size / 2;
        for row in 0..size {
            let color = if row < half { red } else { green };
            for _ in 0..size {
                out.extend_from_slice(&color);
            }
        }
    }
    assert_eq!(out.len(), ATLAS_LAYER_BYTES);
    out
}

/// 建渲染器、上传单层红上绿下 atlas 与一个区段、渲染**一帧**并回读 BGRA 图像。
///
/// 每次都新建渲染器：首帧 `have_last_camera` 为假，HiZ 必然停用，图像因此只取
/// 决于本用例的输入。无 GPU 适配器时返回 None（调用方跳过，与既有约定一致）。
fn render(camera_pos: [f32; 3], view_proj: [f32; 16], quads: &[u64]) -> Option<Vec<u8>> {
    let mut renderer = OffscreenRenderer::new(VIEW, VIEW).ok()?;
    assert!(renderer.upload_atlas(1, &atlas_red_top_green_bottom()));
    assert!(renderer.upload_section(SECTION, &quad_bytes(quads), &[]));
    let mut identity = [0.0f32; 16];
    for i in 0..4 {
        identity[i * 4 + i] = 1.0;
    }
    let frame = FrameInput {
        view_proj,
        view_proj_inv: identity,
        pos: camera_pos,
        daylight: 1.0,
        sun_direction: [0.0, 1.0, 0.0],
        star_visibility: 0.0,
        sky_color: [0.25, 0.5, 1.0, 1.0],
        cloud_macro_x: 0,
        cloud_local: 0.0,
        visible: vec![SECTION],
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

/// side_view_proj 造一台**水平**正交相机，视线方向为 `(dx, 0, dz)`。
///
/// ```text
/// clip.x = (右向量 · (world - 格心)) * 0.5      右向量 = up × forward = (dz, 0, -dx)
/// clip.y = (world.y - 格心.y) * 0.5
/// clip.z = 0.5 + (forward · (world - 格心)) / 200   （越远越大，全程落在 (0,1)）
/// ```
///
/// 于是被观察的那一格恰好落在画面正中的 16×16 像素方格里。矩阵是列主序，
/// `m[列*4 + 行]`。调用方须自行把相机摆到被测面法线所朝的一侧：`camera = 格心 - (dx, 0, dz) * 100`。
fn side_view_proj(dx: f32, dz: f32) -> [f32; 16] {
    let (rx, rz) = (dz, -dx);
    let c = CELL_CENTER;
    let mut m = [0.0f32; 16];
    m[0] = rx * 0.5;
    m[8] = rz * 0.5;
    m[12] = -(rx * c[0] + rz * c[2]) * 0.5;
    m[5] = 0.5;
    m[13] = -c[1] * 0.5;
    m[2] = dx / 200.0;
    m[10] = dz / 200.0;
    m[14] = 0.5 - (dx * c[0] + dz * c[2]) / 200.0;
    m[15] = 1.0;
    m
}

/// 主动色调（红 / 绿），用于通道主导判据。
#[derive(Clone, Copy)]
enum Tone {
    Red,
    Green,
}

/// 数出屏幕行区间 `[row0, row1)` 内、方块正中方块列上以某通道为主导的像素数。
///
/// 判据取通道间的相对关系而不是精确色值：地形着色会乘上 shade 与光照，精确
/// 匹配会因任何一处系数微调而误报。红 / 绿两层的主通道两两可分。
fn count_tone(image: &[u8], tone: Tone, row0: u32, row1: u32) -> usize {
    let mut count = 0;
    for row in row0..row1 {
        for x in BLOCK_BEGIN..BLOCK_END {
            let [r, g, b] = pixel(image, x, row).map(i32::from);
            let matched = match tone {
                Tone::Red => r > 100 && r > g + 40 && r > b + 40,
                Tone::Green => g > 100 && g > r + 40 && g > b + 40,
            };
            if matched {
                count += 1;
            }
        }
    }
    count
}

/// 渲染并读出方块区域上半 / 下半的红、绿像素数。
/// 序：(r_up, g_up, r_low, g_low)。无适配器时返回 None。
fn observe(camera: [f32; 3], view_proj: [f32; 16], quads: &[u64]) -> Option<[usize; 4]> {
    let image = render(camera, view_proj, quads)?;
    Some([
        count_tone(&image, Tone::Red, BLOCK_BEGIN, BLOCK_MID),
        count_tone(&image, Tone::Green, BLOCK_BEGIN, BLOCK_MID),
        count_tone(&image, Tone::Red, BLOCK_MID, BLOCK_END),
        count_tone(&image, Tone::Green, BLOCK_MID, BLOCK_END),
    ])
}

/// 断言一次观察满足「纹理顶行在方块顶部」：上半区块红主导、下半区块绿主导。
/// 无适配器（None）时静默跳过，不 fail。
fn assert_upright(name: &str, obs: Option<[usize; 4]>) {
    let Some([r_up, g_up, r_low, g_low]) = obs else {
        return;
    };
    assert!(
        r_up > 0 && r_up > g_up,
        "{name} 上半区块红不主导：r_up={r_up} g_up={g_up} r_low={r_low} g_low={g_low}",
    );
    assert!(
        g_low > 0 && g_low > r_low,
        "{name} 下半区块绿不主导：r_up={r_up} g_up={g_up} r_low={r_low} g_low={g_low}",
    );
}

/// Scenario「+Z 面（`Face::PosZ`）纹理顶行在方块顶部」。
///
/// 相机摆在 +Z 一侧（世界 z 更大处）朝 −Z 看，正对 +Z 面。判据是方块正中方块
/// 区域上半（世界 y 更大的格子顶部）红主导、下半绿主导。若着色器把 `world.y`
/// 当作 v，则纹理顶行落在方块底部，本用例会在「上半红不主导」处红。
#[test]
fn pos_z_face_texture_top_is_at_block_top() {
    let quads = [pack_quad(
        CELL.0, CELL.1, CELL.2, 1, 1, FACE_POS_Z, MAT_TEST, false,
    )];
    let camera = [CELL_CENTER[0], CELL_CENTER[1], CELL_CENTER[2] + 100.0];
    let obs = observe(camera, side_view_proj(0.0, -1.0), &quads);
    assert_upright("+Z 面", obs);
}

/// Scenario「+X 面（`Face::PosX`）纹理顶行在方块顶部」——守卫 90° 旋转回归。
///
/// 相机摆在 +X 一侧朝 −X 看，正对 +X 面。旧公式对 ±X 面用 `(world.y, world.z)`
/// 把 u 当纵向，整张纹理旋转 90°（画面变成左绿右红的竖条），本用例以红上绿下
/// 判据把它打回。
#[test]
fn pos_x_face_texture_top_is_at_block_top() {
    let quads = [pack_quad(
        CELL.0, CELL.1, CELL.2, 1, 1, FACE_POS_X, MAT_TEST, false,
    )];
    let camera = [CELL_CENTER[0] + 100.0, CELL_CENTER[1], CELL_CENTER[2]];
    let obs = observe(camera, side_view_proj(-1.0, 0.0), &quads);
    assert_upright("+X 面", obs);
}

/// Scenario「植物交叉斜面（`Face::PlantDiagA`）纹理顶行在格子顶部」。
///
/// 从 −Z 一侧朝 +Z 看（与 plant_tests 的「+Z 方向」命名一致）。交叉斜面的 v 轴
/// 就是纵向（world.y），必须与侧面同用 `-world.y` 才能让纹理顶行在格子顶部；
/// 旧公式 `(world.x, world.y)` 会让小麦等植物纹理上下颠倒。
#[test]
fn plant_diag_texture_top_is_at_block_top() {
    let quads = [pack_quad(
        CELL.0,
        CELL.1,
        CELL.2,
        1,
        1,
        FACE_PLANT_A,
        MAT_TEST,
        false,
    )];
    let camera = [CELL_CENTER[0], CELL_CENTER[1], CELL_CENTER[2] - 100.0];
    let obs = observe(camera, side_view_proj(0.0, 1.0), &quads);
    assert_upright("植物交叉斜面", obs);
}

/// Scenario「−Z 面（`Face::NegZ`）纹理顶行在方块顶部」——守卫负方向。
///
/// 相机摆在 −Z 一侧朝 +Z 看，正对 −Z 面。它把 v 轴符号相反的另一半也锁住，
/// 防止只修正向留下负向。
#[test]
fn neg_z_face_texture_top_is_at_block_top() {
    let quads = [pack_quad(
        CELL.0, CELL.1, CELL.2, 1, 1, FACE_NEG_Z, MAT_TEST, false,
    )];
    let camera = [CELL_CENTER[0], CELL_CENTER[1], CELL_CENTER[2] - 100.0];
    let obs = observe(camera, side_view_proj(0.0, 1.0), &quads);
    assert_upright("−Z 面", obs);
}

/// 夹具承重守卫：上空场景的方块正中方块区域不得数出红 / 绿主导像素。
///
/// 若把方块撤掉，红上绿下的计数必须归零；否则上面的四条断言只是在数背景
/// （天空色蓝色主导，不会命中红/绿判据，但归零断言更直接）。
#[test]
fn empty_scene_yields_no_red_or_green_in_block_region() {
    let camera = [CELL_CENTER[0], CELL_CENTER[1], CELL_CENTER[2] - 100.0];
    let image = render(camera, side_view_proj(0.0, 1.0), &[]);
    let Some(image) = image else {
        return;
    };
    let total = count_tone(&image, Tone::Red, BLOCK_BEGIN, BLOCK_END)
        + count_tone(&image, Tone::Green, BLOCK_BEGIN, BLOCK_END);
    assert_eq!(total, 0, "空场景在方块区域数出了红/绿像素，计数口径无效");
}
