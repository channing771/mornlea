//! greedy mesh 的短方块常量顶面高度测试（耕地呈现），属 greedy mesh 测试集
//! （`greedy/mod.rs`）。共享夹具见 `greedy/test_support.rs`。

use super::mesh_section;
use super::test_support::{ENTRY_BYTES, REGISTRY_OFFSET, dark_light, parse, set_block};
use crate::quad::{Face, Quad};

// ---- 耕地常量下沉几何 ----------------------------------------------
//
// registry 夹具：id 0 = 空气、id 1 = 石头（同时是 barrier）、id 10 = 干耕地、
// id 11 = 湿耕地。二者的 `block_top_raw` 都取生产值 14——呈现高度
// (14+1)/16 = 15/16，恰等于物理碰撞体高度（internal/physics 的
// farmlandCollisionHeight = 0.9375）。可见性位图按 assets.FaceVisible 的规则
// 烘焙：石头对空气与两格耕地出面；耕地是不透明方块，只对空气出面
// （耕地—耕地、耕地—石头都不出面）。

const FARMLAND_ENTRIES: usize = 4;
const DRY_ID: u16 = 10;
const WET_ID: u16 = 11;
/// 生产夹具值：干/湿耕地共用的顶面高度原值。14 不是随手取的——高度域是
/// 1..=14（15 非法），而 14 是其中唯一让呈现高度等于既有碰撞高度的取值。
const FARMLAND_TOP_RAW: u8 = 14;

fn farmland_input() -> Vec<u8> {
    let mut bytes =
        vec![0; REGISTRY_OFFSET + FARMLAND_ENTRIES * ENTRY_BYTES + FARMLAND_ENTRIES * 8];
    bytes[0..4].copy_from_slice(b"MGM1");
    bytes[8..10].copy_from_slice(&(FARMLAND_ENTRIES as u16).to_le_bytes());
    bytes[10..12].copy_from_slice(&1_u16.to_le_bytes());
    bytes[12..14].copy_from_slice(&0_u16.to_le_bytes());
    bytes[14..16].copy_from_slice(&1_u16.to_le_bytes());

    let mut write_entry = |index: usize, id: u16, opaque: bool, block_top_raw: u8| {
        let entry = REGISTRY_OFFSET + index * ENTRY_BYTES;
        bytes[entry..entry + 2].copy_from_slice(&id.to_le_bytes());
        bytes[entry + 2] = u8::from(opaque);
        for face in 0..6 {
            // 材质取 100：刻意避开植物区间 [31, 38]，否则 mesher 会把耕地当成
            // 作物改出交叉斜面；石头沿用水面测试的 1。
            let material = if id == 1 { 1_u16 } else { 100 };
            bytes[entry + 4 + face * 2..entry + 6 + face * 2]
                .copy_from_slice(&material.to_le_bytes());
        }
        // 字节 16/17 是 fluid_height/light_attenuation：耕地是普通不透明固体，
        // 必须全 0；byte 18 才是本主题的 block_top_raw。
        bytes[entry + 16] = 0;
        bytes[entry + 17] = 0;
        bytes[entry + 18] = block_top_raw;
    };
    write_entry(0, 0, false, 0);
    write_entry(1, 1, true, 0);
    write_entry(2, DRY_ID, true, FARMLAND_TOP_RAW);
    write_entry(3, WET_ID, true, FARMLAND_TOP_RAW);

    // 可见性行：石头 = 对空气(列 0)与全部耕地(列 2、3)出面；
    // 耕地 = 只对空气出面（自身不透明，同类相邻互不出面）。
    for index in 0..FARMLAND_ENTRIES {
        let row: u64 = match index {
            0 => 0,
            1 => 0b1101,
            _ => 1,
        };
        let offset = REGISTRY_OFFSET + FARMLAND_ENTRIES * ENTRY_BYTES + index * 8;
        bytes[offset..offset + 8].copy_from_slice(&row.to_le_bytes());
    }
    bytes
}

/// mesh_farmland 网格化一份耕地夹具并解包成 Quad。
fn mesh_farmland(bytes: Vec<u8>) -> Vec<Quad> {
    let input = parse(bytes);
    let light = dark_light();
    let mut output = vec![0; 6 * 4096];
    let count = mesh_section(&input, &light, &mut output).unwrap();
    output[..count].iter().copied().map(Quad::unpack).collect()
}

/// face_at 取指定格朝指定面的 quad。
fn face_at(quads: &[Quad], face: Face, x: u8, y: u8, z: u8) -> Quad {
    *quads
        .iter()
        .find(|quad| quad.face == face && quad.x == x && quad.y == y && quad.z == z)
        .unwrap_or_else(|| panic!("缺少 ({x},{y},{z}) 的 {face:?} 面"))
}

/// 孤立耕地的顶面四角全为 registry 常量 14 且按 1×1 出面；干/湿两态走同一
/// 条通道；同夹具的石头对照组保持满格（四角为哨兵 0）。
///
/// 常量路径的意义就在「四角相等」：刚体短方块没有邻域插值，若任何一个角
/// 偏离 14 都说明常量被邻域平均或满格规则污染。
#[test]
fn farmland_top_face_sinks_to_registry_constant() {
    for id in [DRY_ID, WET_ID] {
        let mut bytes = farmland_input();
        set_block(&mut bytes, 8, 8, 8, id);
        let quads = mesh_farmland(bytes);
        let top = face_at(&quads, Face::PosY, 8, 8, 8);
        assert_eq!(
            top.corners, [FARMLAND_TOP_RAW; 4],
            "id={id} 的耕地顶面四角应全为常量 14"
        );
        assert_eq!((top.w, top.h), (1, 1), "短方块顶面必须 1×1 出面");
    }

    // 对照组：同一夹具的石头不带 block_top_raw，顶面必须仍是普通满格 quad，
    // 证明下沉只由 registry 字节驱动而不是夹具的其他副作用。
    let mut bytes = farmland_input();
    set_block(&mut bytes, 8, 8, 8, 1);
    let quads = mesh_farmland(bytes);
    assert_eq!(face_at(&quads, Face::PosY, 8, 8, 8).corners, [0; 4]);
}

/// 四个侧面只有上缘两角带常量高度、底面四角不动。
///
/// 角序沿袭 `fluid_corners` 的「仅 y == p[1]+1 顶点带高」形状：X 向面的
/// 面内第一轴是 y，角序为 [下,上,上,下]；Z 向面的第二轴是 y，角序为
/// [下,下,上,上]。底面贴在 y == p[1]，四角恒 0——这正是碰撞盒
/// （底面整格、顶面 15/16）的逐面镜像。
#[test]
fn farmland_side_upper_edges_sink_and_bottom_stays_flat() {
    let mut bytes = farmland_input();
    set_block(&mut bytes, 8, 8, 8, DRY_ID);
    let quads = mesh_farmland(bytes);

    assert_eq!(
        face_at(&quads, Face::NegX, 8, 8, 8).corners,
        [0, FARMLAND_TOP_RAW, FARMLAND_TOP_RAW, 0]
    );
    assert_eq!(
        face_at(&quads, Face::PosX, 8, 8, 8).corners,
        [0, FARMLAND_TOP_RAW, FARMLAND_TOP_RAW, 0]
    );
    assert_eq!(
        face_at(&quads, Face::NegZ, 8, 8, 8).corners,
        [0, 0, FARMLAND_TOP_RAW, FARMLAND_TOP_RAW]
    );
    assert_eq!(
        face_at(&quads, Face::PosZ, 8, 8, 8).corners,
        [0, 0, FARMLAND_TOP_RAW, FARMLAND_TOP_RAW]
    );
    assert_eq!(
        face_at(&quads, Face::NegY, 8, 8, 8).corners,
        [0; 4],
        "底面必须保持整格边界"
    );
}

/// 相邻同材质耕地不贪心合并：两格各自出 1×1 顶面。
///
/// 两格的 MaskCell 完全相等（同材质同光照同角高度），若合并条件漏掉 short
/// 标志，贪心循环会把它们并成一条跨格 quad——角高度位随即被当尺寸解读。
#[test]
fn adjacent_farmland_cells_do_not_merge() {
    let mut bytes = farmland_input();
    set_block(&mut bytes, 8, 8, 8, DRY_ID);
    set_block(&mut bytes, 9, 8, 8, DRY_ID);
    let quads = mesh_farmland(bytes);

    let tops: Vec<Quad> = quads
        .iter()
        .copied()
        .filter(|quad| quad.face == Face::PosY && quad.y == 8)
        .collect();
    assert_eq!(tops.len(), 2, "相邻耕地必须逐格出顶面，不得合并");
    assert!(tops.iter().all(|quad| quad.w == 1 && quad.h == 1));
    assert!(
        tops.iter()
            .all(|quad| quad.corners == [FARMLAND_TOP_RAW; 4])
    );
}
