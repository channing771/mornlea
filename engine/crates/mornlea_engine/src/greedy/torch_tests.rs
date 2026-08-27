//! greedy mesh 的火把有限模型测试（placeable-torches 的几何），属 greedy mesh
//! 测试集（`greedy/mod.rs`）。共享夹具见 `greedy/test_support.rs`，几何实现见
//! `greedy/torch.rs`。

use super::test_support::{ENTRY_BYTES, REGISTRY_OFFSET, parse, set_block};
use super::torch::{TORCH_WALL_TOP_FAR_RAW, TORCH_WALL_TOP_NEAR_RAW};
use super::{TORCH_QUADS_PER_STANDING_CELL, TORCH_QUADS_PER_WALL_CELL, mesh_section};
use crate::light::{LIGHT_VOLUME, LightScratch};
use crate::quad::{Face, Quad};

// ---- 火把有限模型 --------------------------------------------------
//
// 夹具 registry：id 0 = 空气、id 1 = 石头（同时是 barrier）、id 62..66 = 五种
// 火把形态（model tag 1..5，与方块编号 62..66 同序）。可见性位图按
// assets.FaceVisible 的规则烘焙：石头对空气与全部火把出面，**火把对谁都不出
// 轴向面**——几何全部来自 emit_torch（与植物同一豁免路径）。

const TORCH_ENTRIES: usize = 7;
const TORCH_STANDING_ID: u16 = 62;
const TORCH_WALL_POS_X_ID: u16 = 63;
const TORCH_WALL_NEG_X_ID: u16 = 64;
const TORCH_WALL_POS_Z_ID: u16 = 65;
const TORCH_WALL_NEG_Z_ID: u16 = 66;
/// 夹具里火把共用的材质层：刻意取植物区间 [31,54] 与耕地区间 [29,30] 之外，
/// 证明火把 quad 的判别完全依赖 registry model tag、不与任何 material 区间
/// 判别串味。
const TORCH_MATERIAL: u16 = 90;

/// torch_input 造一份五形态火把全部登记（model 1..5）的 mesh 输入。
fn torch_input() -> Vec<u8> {
    let mut bytes = vec![0; REGISTRY_OFFSET + TORCH_ENTRIES * ENTRY_BYTES + TORCH_ENTRIES * 8];
    bytes[0..4].copy_from_slice(b"MGM1");
    bytes[8..10].copy_from_slice(&(TORCH_ENTRIES as u16).to_le_bytes());
    bytes[10..12].copy_from_slice(&1_u16.to_le_bytes());
    bytes[12..14].copy_from_slice(&0_u16.to_le_bytes());
    bytes[14..16].copy_from_slice(&1_u16.to_le_bytes());

    let mut write_entry = |index: usize, id: u16, opaque: bool, material: u16, model: u8| {
        let entry = REGISTRY_OFFSET + index * ENTRY_BYTES;
        bytes[entry..entry + 2].copy_from_slice(&id.to_le_bytes());
        bytes[entry + 2] = u8::from(opaque);
        for face in 0..6 {
            bytes[entry + 4 + face * 2..entry + 6 + face * 2]
                .copy_from_slice(&material.to_le_bytes());
        }
        bytes[entry + 19] = model;
    };
    write_entry(0, 0, false, 0, 0);
    write_entry(1, 1, true, 1, 0);
    write_entry(2, TORCH_STANDING_ID, false, TORCH_MATERIAL, 1);
    write_entry(3, TORCH_WALL_POS_X_ID, false, TORCH_MATERIAL, 2);
    write_entry(4, TORCH_WALL_NEG_X_ID, false, TORCH_MATERIAL, 3);
    write_entry(5, TORCH_WALL_POS_Z_ID, false, TORCH_MATERIAL, 4);
    write_entry(6, TORCH_WALL_NEG_Z_ID, false, TORCH_MATERIAL, 5);

    // 石头 = 对空气（列 0）与五个火把形态（列 2..6）出面；空气与火把整行为 0。
    let stone_row: u64 = 1 | (0b011111 << 2);
    for index in 0..TORCH_ENTRIES {
        let row = if index == 1 { stone_row } else { 0 };
        let offset = REGISTRY_OFFSET + TORCH_ENTRIES * ENTRY_BYTES + index * 8;
        bytes[offset..offset + 8].copy_from_slice(&row.to_le_bytes());
    }
    bytes
}

/// light_slot 复刻 `light::light_index`，供夹具直接写某一格的光照值。
fn light_slot(x: i32, y: i32, z: i32) -> usize {
    ((x + 16) as usize * 48 + (y + 16) as usize) * 48 + (z + 16) as usize
}

/// mesh_torch_fixture 网格化一份火把夹具（全暗光照）并解包成 Quad。
fn mesh_torch_fixture(bytes: Vec<u8>) -> Vec<Quad> {
    mesh_torch_with_light(bytes, vec![0; LIGHT_VOLUME])
}

/// mesh_torch_with_light 网格化一份火把夹具并解包成 Quad，光照体积由调用方给定。
fn mesh_torch_with_light(bytes: Vec<u8>, levels: Vec<u8>) -> Vec<Quad> {
    let input = parse(bytes);
    let light = LightScratch::new(
        Box::leak(levels.into_boxed_slice()),
        Box::leak(vec![0; LIGHT_VOLUME].into_boxed_slice()),
    );
    let mut output = vec![0; 6 * 4096];
    let count = mesh_section(&input, &light, &mut output).unwrap();
    output[..count].iter().copied().map(Quad::unpack).collect()
}

/// torch_quads 过滤出属于某个火把材质的全部 quad。
fn torch_quads(quads: &[Quad]) -> Vec<Quad> {
    quads
        .iter()
        .copied()
        .filter(|quad| quad.material == TORCH_MATERIAL)
        .collect()
}

/// 落地火把：恰好 4 条双面交叉斜面，竖直、居中、全部落在本格。
///
/// 「居中竖直窄柱」在 quad 8 字节格式里的唯一表达就是植物的交叉斜面编组
/// （face 6/7 + 正背位）：两条对角面都过格心的竖直线，视觉窄度由材质 alpha
/// 收窄。断言逐条钉住 (face, back) 次序与 1×1 尺寸——若火把退回轴向满面或
/// 丢了正背配对，立刻红。末尾的对照断言承重：旁边的石头必须照常出朝向火把
/// 的面，证明夹具本身没有静默。
#[test]
fn standing_torch_emits_centered_double_sided_cross_quads() {
    let mut bytes = torch_input();
    set_block(&mut bytes, 8, 8, 8, TORCH_STANDING_ID);
    set_block(&mut bytes, 7, 8, 8, 1);
    let quads = mesh_torch_fixture(bytes);

    let torch = torch_quads(&quads);
    assert_eq!(
        torch.len(),
        TORCH_QUADS_PER_STANDING_CELL,
        "落地火把必须恰好 {TORCH_QUADS_PER_STANDING_CELL} 条面实例"
    );
    let want_faces = [
        (Face::PlantDiagA, false),
        (Face::PlantDiagA, true),
        (Face::PlantDiagB, false),
        (Face::PlantDiagB, true),
    ];
    assert_eq!(
        torch
            .iter()
            .map(|quad| (quad.face, quad.back))
            .collect::<Vec<_>>(),
        want_faces.to_vec(),
        "落地火把的对角线与正背次序必须固定"
    );
    for quad in &torch {
        assert_eq!((quad.x, quad.y, quad.z), (8, 8, 8), "坐标必须全在本格");
        assert_eq!((quad.w, quad.h), (1, 1), "火把 quad 必须是 1×1");
        assert_eq!(quad.corners, [0; 4], "落地火把不带角高度");
        assert_eq!(quad.material, TORCH_MATERIAL);
        assert_eq!(quad.ao, 0xff, "格内几何无共面邻居，AO 记满");
    }
    assert!(
        quads
            .iter()
            .any(|quad| quad.face == Face::PosX && quad.x == 7 && quad.material == 1),
        "夹具无效：火把旁的石头没有画出朝向火把的面"
    );
}

/// 墙面火把：恰好 3 条 quad——承载倾斜的两片轴向薄板（顶缘向远离支撑方向
/// 抬高）＋贴住支撑面的一片平帽。逐形态钉住 face 集合、角高度非对称方向与
/// 贴面侧，倾斜方向写反任何一格都会红。
#[test]
fn wall_torches_hug_support_and_lean_away() {
    // 每个墙面形态：(火把编号, 两片斜板的 face 对, 贴面帽的 face, 斜板四角)。
    // 角顺序是 (du,dv) 的 (0,0)(1,0)(1,1)(0,1)；near/far 分别是支撑侧与远离
    // 支撑侧的顶缘角，far > near 即「向远离支撑的方向倾斜」。
    let near = TORCH_WALL_TOP_NEAR_RAW;
    let far = TORCH_WALL_TOP_FAR_RAW;
    let cases: [(u16, [Face; 2], Face, [u8; 4]); 4] = [
        // wall +X：支撑在 −X 侧，板法线 ±Z，顶缘 x+1 侧（角 2）远离支撑、更高。
        (
            TORCH_WALL_POS_X_ID,
            [Face::NegZ, Face::PosZ],
            Face::NegX,
            [0, 0, far, near],
        ),
        // wall −X：支撑在 +X 侧，角 3（x+0 侧）远离支撑、更高。
        (
            TORCH_WALL_NEG_X_ID,
            [Face::NegZ, Face::PosZ],
            Face::PosX,
            [0, 0, near, far],
        ),
        // wall +Z：支撑在 −Z 侧，板法线 ±X，角 2（z+1 侧）远离支撑、更高。
        (
            TORCH_WALL_POS_Z_ID,
            [Face::NegX, Face::PosX],
            Face::NegZ,
            [0, near, far, 0],
        ),
        // wall −Z：支撑在 +Z 侧，角 1（z+0 侧）远离支撑、更高。
        (
            TORCH_WALL_NEG_Z_ID,
            [Face::NegX, Face::PosX],
            Face::PosZ,
            [0, far, near, 0],
        ),
    ];
    for (id, plates, cap, corners) in cases {
        let mut bytes = torch_input();
        set_block(&mut bytes, 8, 8, 8, id);
        let quads = mesh_torch_fixture(bytes);

        let torch = torch_quads(&quads);
        assert_eq!(
            torch.len(),
            TORCH_QUADS_PER_WALL_CELL,
            "墙面火把（id={id}）必须恰好 {TORCH_QUADS_PER_WALL_CELL} 条面实例"
        );
        assert_eq!(
            torch.iter().map(|quad| quad.face).collect::<Vec<_>>()[..2],
            plates,
            "墙面火把（id={id}）的斜板 face 次序必须固定"
        );
        for quad in torch.iter().take(2) {
            assert_eq!((quad.x, quad.y, quad.z), (8, 8, 8), "坐标必须全在本格");
            assert_eq!((quad.w, quad.h), (1, 1), "火把 quad 必须是 1×1");
            assert_eq!(quad.corners, corners, "墙面火把（id={id}）斜板角高度错误");
            assert_eq!(quad.material, TORCH_MATERIAL);
            assert!(!quad.back, "轴向斜板不得设置正背位");
        }
        let cap_quad = &torch[2];
        assert_eq!(cap_quad.face, cap, "墙面火把（id={id}）的贴面帽 face 错误");
        assert_eq!(cap_quad.corners, [0; 4], "贴面帽是平面，不带角高度");
        assert_eq!((cap_quad.w, cap_quad.h), (1, 1));
        assert_eq!(cap_quad.material, TORCH_MATERIAL);
    }
}

/// 五形态的 quad 打包后必须仍是 8 字节格式且 bit 63 为 0，且坐标全部落在本格
/// （x/y/z 即火把所在格，尺寸恒 1×1，角高度 ≤ 14）。
#[test]
fn torch_quads_stay_eight_bytes_and_inside_their_cell() {
    for id in [
        TORCH_STANDING_ID,
        TORCH_WALL_POS_X_ID,
        TORCH_WALL_NEG_X_ID,
        TORCH_WALL_POS_Z_ID,
        TORCH_WALL_NEG_Z_ID,
    ] {
        let mut bytes = torch_input();
        set_block(&mut bytes, 11, 9, 5, id);
        let input = parse(bytes);
        let light = super::test_support::dark_light();
        let mut output = vec![0; 6 * 4096];
        let count = mesh_section(&input, &light, &mut output).unwrap();
        for packed in &output[..count] {
            assert_eq!(packed >> 63, 0, "火把 quad（id={id}）占用了 bit 63");
            let quad = Quad::unpack(*packed);
            assert_eq!(
                (quad.x, quad.y, quad.z),
                (11, 9, 5),
                "火把 quad（id={id}）坐标必须全在本格"
            );
            assert!(quad.corners.iter().all(|&corner| corner <= 14));
        }
    }
}

/// 相邻火把不合并：两个落地火把各自保有整整 4 条 1×1 quad；对照组的整层
/// 石头顶面照常贪心合并成一条，证明「不合并」是火把独有规则。
#[test]
fn adjacent_torches_never_merge() {
    let mut bytes = torch_input();
    set_block(&mut bytes, 8, 8, 8, TORCH_STANDING_ID);
    set_block(&mut bytes, 9, 8, 8, TORCH_STANDING_ID);
    let quads = mesh_torch_fixture(bytes);

    let torch = torch_quads(&quads);
    assert_eq!(torch.len(), 2 * TORCH_QUADS_PER_STANDING_CELL);
    assert!(torch.iter().all(|quad| quad.w == 1 && quad.h == 1));
    let mut per_cell = std::collections::BTreeMap::new();
    for quad in &torch {
        *per_cell.entry((quad.x, quad.y, quad.z)).or_insert(0_usize) += 1;
    }
    assert_eq!(per_cell.len(), 2, "火把 quad 没有分布在两格上");
    assert!(
        per_cell.values().all(|&n| n == TORCH_QUADS_PER_STANDING_CELL),
        "存在每格面实例数不等于 {TORCH_QUADS_PER_STANDING_CELL} 的格子"
    );

    let mut stone_bytes = torch_input();
    for x in 0..16 {
        for z in 0..16 {
            set_block(&mut stone_bytes, x, 8, z, 1);
        }
    }
    let stone_tops = mesh_torch_fixture(stone_bytes)
        .into_iter()
        .filter(|quad| quad.face == Face::PosY && quad.y == 8)
        .count();
    assert_eq!(
        stone_tops, 1,
        "对照组的石头顶面没有合并，整条断言失去分辨力"
    );
}

/// 光照与材质来自 registry 与邻域既有规则：与植物同一口径——光取正上方
/// 相邻格、AO 记满。六个方向的光照刻意两两不同，取错采样点读数就会变。
#[test]
fn torch_light_is_sampled_from_the_cell_above() {
    let mut bytes = torch_input();
    set_block(&mut bytes, 8, 8, 8, TORCH_STANDING_ID);
    set_block(&mut bytes, 4, 8, 8, TORCH_WALL_POS_X_ID);
    let mut levels = vec![0; LIGHT_VOLUME];
    levels[light_slot(8, 8, 8)] = 0x12;
    levels[light_slot(8, 9, 8)] = 0x9a;
    levels[light_slot(8, 7, 8)] = 0x34;
    levels[light_slot(4, 8, 8)] = 0x56;
    levels[light_slot(4, 9, 8)] = 0x78;
    levels[light_slot(3, 8, 8)] = 0xbc;
    let quads = mesh_torch_with_light(bytes, levels);

    let standing: Vec<&Quad> = quads
        .iter()
        .filter(|quad| quad.material == TORCH_MATERIAL && quad.x == 8)
        .collect();
    assert_eq!(standing.len(), TORCH_QUADS_PER_STANDING_CELL);
    assert!(
        standing.iter().all(|quad| quad.light == 0x9a),
        "落地火把光照必须取上方格 0x9a，实测 {:?}",
        standing.iter().map(|quad| quad.light).collect::<Vec<_>>()
    );
    let wall: Vec<&Quad> = quads
        .iter()
        .filter(|quad| quad.material == TORCH_MATERIAL && quad.x == 4)
        .collect();
    assert_eq!(wall.len(), TORCH_QUADS_PER_WALL_CELL);
    assert!(
        wall.iter().all(|quad| quad.light == 0x78),
        "墙面火把光照必须取上方格 0x78，实测 {:?}",
        wall.iter().map(|quad| quad.light).collect::<Vec<_>>()
    );
}

/// 整段塞满落地火把时，面实例总数恰好 `4 × 4096`，仍在 Go 侧
/// `maxNativeQuads = 6 * BlocksPerSection` 之内——「每格固定上界」的最坏
/// 情况证据，上界一旦从 4 涨上去这里立刻红。
#[test]
fn a_section_full_of_torches_stays_within_the_fixed_bound() {
    let mut bytes = torch_input();
    for y in 0..16 {
        for z in 0..16 {
            for x in 0..16 {
                set_block(&mut bytes, x, y, z, TORCH_STANDING_ID);
            }
        }
    }
    let input = parse(bytes);
    let light = super::test_support::dark_light();
    let mut output = vec![0; 6 * 4096];
    let count = mesh_section(&input, &light, &mut output).unwrap();
    assert_eq!(count, TORCH_QUADS_PER_STANDING_CELL * 4096);
    assert!(
        count <= 6 * 4096,
        "火把的每格上界超过了普通方块的 6，Go 侧 maxNativeQuads 不再覆盖最坏情况"
    );
}
