//! greedy mesh 的植物交叉斜面测试（`plant-visual-presentation` 的几何），
//! 属 greedy mesh 测试集（`greedy/mod.rs`）。共享夹具见 `greedy/test_support.rs`。

use super::test_support::{ENTRY_BYTES, REGISTRY_OFFSET, dark_light, parse, set_block};
use super::{MeshError, PLANT_QUADS, PLANT_QUADS_PER_CELL, mesh_section};
use crate::light::{LIGHT_VOLUME, LightScratch};
use crate::quad::{Face, PLANT_MATERIAL_FIRST, Quad};

// ---- 植物交叉斜面 --------------------------------------------------
//
// 植物的 registry 夹具：id 0 = 空气、id 1 = 石头（同时是 barrier）、
// id 20 = 小麦（非不透明、material 落在植物区间、非流体、不额外衰减天空光）。
// 可见性位图按 assets.FaceVisible 的规则烘焙：石头对空气与小麦都出面，
// **小麦对谁都不出面**——轴向面一条不出，几何全部来自 mesh_plants。

const PLANT_ENTRIES: usize = 3;
const WHEAT_ID: u16 = 20;
/// 夹具里小麦用的材质层，取植物区间的第一层。
const WHEAT_MATERIAL: u16 = PLANT_MATERIAL_FIRST;
/// 短草追加在既有非植物层之后，形成植物材质集合的离散单点。
const SHORT_GRASS_MATERIAL: u16 = 68;

fn plant_input() -> Vec<u8> {
    plant_input_with_material(WHEAT_MATERIAL)
}

fn plant_input_with_material(material: u16) -> Vec<u8> {
    let mut bytes = vec![0; REGISTRY_OFFSET + PLANT_ENTRIES * ENTRY_BYTES + PLANT_ENTRIES * 8];
    bytes[0..4].copy_from_slice(b"MGM1");
    bytes[8..10].copy_from_slice(&(PLANT_ENTRIES as u16).to_le_bytes());
    bytes[10..12].copy_from_slice(&1_u16.to_le_bytes());
    bytes[12..14].copy_from_slice(&0_u16.to_le_bytes());
    bytes[14..16].copy_from_slice(&1_u16.to_le_bytes());

    let mut write_entry = |index: usize, id: u16, opaque: bool, material: u16| {
        let entry = REGISTRY_OFFSET + index * ENTRY_BYTES;
        bytes[entry..entry + 2].copy_from_slice(&id.to_le_bytes());
        bytes[entry + 2] = u8::from(opaque);
        for face in 0..6 {
            bytes[entry + 4 + face * 2..entry + 6 + face * 2]
                .copy_from_slice(&material.to_le_bytes());
        }
    };
    write_entry(0, 0, false, 0);
    write_entry(1, 1, true, 1);
    write_entry(2, WHEAT_ID, false, material);

    // 石头 = 对空气(列 0)与小麦(列 2)出面；空气与小麦整行为 0。
    for (index, row) in [0_u64, 1 | 1 << 2, 0].into_iter().enumerate() {
        let offset = REGISTRY_OFFSET + PLANT_ENTRIES * ENTRY_BYTES + index * 8;
        bytes[offset..offset + 8].copy_from_slice(&row.to_le_bytes());
    }
    bytes
}

/// mesh_with_light 网格化一份夹具并解包成 Quad，光照体积由调用方给定。
fn mesh_with_light(bytes: Vec<u8>, levels: Vec<u8>) -> Vec<Quad> {
    let input = parse(bytes);
    let light = LightScratch::new(
        Box::leak(levels.into_boxed_slice()),
        Box::leak(vec![0; LIGHT_VOLUME].into_boxed_slice()),
    );
    let mut output = vec![0; 6 * 4096];
    let count = mesh_section(&input, &light, &mut output).unwrap();
    output[..count].iter().copied().map(Quad::unpack).collect()
}

/// mesh_plant_fixture 网格化一份植物夹具（全暗光照）。
fn mesh_plant_fixture(bytes: Vec<u8>) -> Vec<Quad> {
    mesh_with_light(bytes, vec![0; LIGHT_VOLUME])
}

/// light_slot 复刻 `light::light_index`，供夹具直接写某一格的光照值。
fn light_slot(x: i32, y: i32, z: i32) -> usize {
    ((x + 16) as usize * 48 + (y + 16) as usize) * 48 + (z + 16) as usize
}

/// 一株孤立作物恰好出 4 条交叉斜面，且**一条轴向面都不出**。
///
/// 断言是位置性的：不只数条数，还逐条钉住 `(face, back)` 的次序、`w`/`h`
/// 与 material。若植物退回轴向满面，face 会落回 0..5，这条立刻红。
/// 末尾的对照断言承重——同一帧里紧挨着的石头**必须**照常出朝向作物的那一面，
/// 否则"作物不出轴向面"可能只是"这个夹具什么都没画"。
#[test]
fn isolated_plant_emits_four_cross_quads_and_no_axial_face() {
    let mut bytes = plant_input();
    set_block(&mut bytes, 8, 8, 8, WHEAT_ID);
    set_block(&mut bytes, 7, 8, 8, 1);
    let quads = mesh_plant_fixture(bytes);

    let plant: Vec<Quad> = quads
        .iter()
        .copied()
        .filter(|quad| quad.material == WHEAT_MATERIAL)
        .collect();
    assert_eq!(plant.len(), PLANT_QUADS_PER_CELL, "每格必须恰好 4 条面实例");
    assert_eq!(
        plant
            .iter()
            .map(|quad| (quad.face, quad.back))
            .collect::<Vec<_>>(),
        PLANT_QUADS.to_vec(),
        "四条 quad 的对角线与正背次序必须固定"
    );
    for quad in &plant {
        assert_eq!((quad.x, quad.y, quad.z), (8, 8, 8));
        assert_eq!((quad.w, quad.h), (1, 1), "植物 quad 必须是 1×1");
        assert_eq!(quad.corners, [0; 4], "植物 quad 不带角高度");
        assert!(quad.face.plant(), "植物 quad 的 face 必须是 6 或 7");
    }
    // 石头朝 +X（也就是朝作物）的那一面必须还在：作物非不透明、不遮挡邻居出面。
    assert!(
        quads
            .iter()
            .any(|quad| quad.face == Face::PosX && quad.x == 7 && quad.material == 1),
        "夹具无效：作物旁的石头没有画出朝向作物的面，"
    );
}

/// 离散的短草材质层复用同一条真实 mesher 路径，每格仍是四条 `u64` 实例。
#[test]
fn short_grass_material_emits_four_eight_byte_instances() {
    let mut bytes = plant_input_with_material(SHORT_GRASS_MATERIAL);
    set_block(&mut bytes, 8, 8, 8, WHEAT_ID);
    let plant: Vec<Quad> = mesh_plant_fixture(bytes)
        .into_iter()
        .filter(|quad| quad.material == SHORT_GRASS_MATERIAL)
        .collect();
    assert_eq!(plant.len(), PLANT_QUADS_PER_CELL);
    assert!(
        plant
            .iter()
            .all(|quad| quad.face.plant() && quad.w == 1 && quad.h == 1)
    );
    assert_eq!(std::mem::size_of::<u64>(), 8);
    assert!(
        plant
            .iter()
            .all(|quad| quad.pack().to_le_bytes().len() == 8)
    );
}

/// 相邻作物不合并：一整层 16×16 作物必须出 256 × 4 条 1×1 quad。
///
/// 对照组是同一片面积的石头板——它**会**合并成一条 16×16 顶面。两组放在一起，
/// "不合并"才是一条位置性断言而不是"这里恰好只有一格"。
#[test]
fn adjacent_plants_never_merge() {
    let mut bytes = plant_input();
    for x in 0..16 {
        for z in 0..16 {
            set_block(&mut bytes, x, 8, z, WHEAT_ID);
        }
    }
    let quads = mesh_plant_fixture(bytes);
    let plant: Vec<Quad> = quads
        .iter()
        .copied()
        .filter(|quad| quad.material == WHEAT_MATERIAL)
        .collect();
    assert_eq!(plant.len(), 256 * PLANT_QUADS_PER_CELL);
    assert!(plant.iter().all(|quad| quad.w == 1 && quad.h == 1));

    // 每一格都必须各自拥有整整 4 条：数量对得上、分布却塌到某几格上，
    // 上面那条断言照样绿。
    let mut per_cell = std::collections::BTreeMap::new();
    for quad in &plant {
        *per_cell.entry((quad.x, quad.y, quad.z)).or_insert(0_usize) += 1;
    }
    assert_eq!(per_cell.len(), 256, "植物 quad 没有铺满 256 格");
    assert!(
        per_cell.values().all(|&n| n == PLANT_QUADS_PER_CELL),
        "存在每格面实例数不等于 {PLANT_QUADS_PER_CELL} 的格子"
    );

    // 对照组：同样一整层石头顶面会贪心合并成一条，说明"不合并"确实是
    // 植物独有的规则，而不是这个 mesher 根本不会合并。
    let mut stone_bytes = plant_input();
    for x in 0..16 {
        for z in 0..16 {
            set_block(&mut stone_bytes, x, 8, z, 1);
        }
    }
    let stone_tops = mesh_plant_fixture(stone_bytes)
        .into_iter()
        .filter(|quad| quad.face == Face::PosY && quad.y == 8)
        .count();
    assert_eq!(
        stone_tops, 1,
        "对照组的石头顶面没有合并，整条断言失去分辨力"
    );
}

/// 光照取**正上方**相邻格，而不是植物自身那格、也不是任何侧邻。
///
/// 夹具刻意让六个方向的光照两两不同：上方 0x9a、下方 0x34、四个侧邻各不相同、
/// 植物自身 0x12。取错任何一个采样点，读数都会变——这正是"位置性"的要求。
#[test]
fn plant_light_is_sampled_from_the_cell_above() {
    let mut bytes = plant_input();
    set_block(&mut bytes, 8, 8, 8, WHEAT_ID);
    let mut levels = vec![0; LIGHT_VOLUME];
    levels[light_slot(8, 8, 8)] = 0x12;
    levels[light_slot(8, 9, 8)] = 0x9a;
    levels[light_slot(8, 7, 8)] = 0x34;
    levels[light_slot(7, 8, 8)] = 0x56;
    levels[light_slot(9, 8, 8)] = 0x78;
    levels[light_slot(8, 8, 7)] = 0xbc;
    levels[light_slot(8, 8, 9)] = 0xde;
    let quads = mesh_with_light(bytes, levels);

    let plant: Vec<Quad> = quads
        .iter()
        .copied()
        .filter(|quad| quad.material == WHEAT_MATERIAL)
        .collect();
    assert_eq!(plant.len(), PLANT_QUADS_PER_CELL);
    assert!(
        plant.iter().all(|quad| quad.light == 0x9a),
        "植物 quad 的光照必须取上方格 0x9a，实测 {:?}",
        plant.iter().map(|quad| quad.light).collect::<Vec<_>>()
    );
}

/// 输出缓冲差一条时，植物同样要报 OutputOverflow，而不是静默截断。
#[test]
fn plant_output_overflow_is_reported() {
    let mut bytes = plant_input();
    set_block(&mut bytes, 8, 8, 8, WHEAT_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; PLANT_QUADS_PER_CELL - 1];
    assert_eq!(
        mesh_section(&input, &light, &mut output),
        Err(MeshError::OutputOverflow)
    );
}

/// 整段塞满作物时，面实例总数恰好是 `4 × 4096`，仍在 Go 侧
/// `maxNativeQuads = 6 * BlocksPerSection` 之内。
///
/// 这条是"固定上界"的最坏情况证据：上界一旦从 4 涨上去，这里立刻红。
#[test]
fn a_section_full_of_plants_stays_within_the_fixed_bound() {
    let mut bytes = plant_input();
    for y in 0..16 {
        for z in 0..16 {
            for x in 0..16 {
                set_block(&mut bytes, x, y, z, WHEAT_ID);
            }
        }
    }
    let input = parse(bytes);
    let light = dark_light();
    let mut output = vec![0; 6 * 4096];
    let count = mesh_section(&input, &light, &mut output).unwrap();
    assert_eq!(count, PLANT_QUADS_PER_CELL * 4096);
    assert!(
        count <= 6 * 4096,
        "植物的每格上界超过了普通方块的 6，Go 侧 maxNativeQuads 不再覆盖最坏情况"
    );
}
