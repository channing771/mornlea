//! greedy mesh 的水面角高度测试（斜水面几何），属 greedy mesh 测试集
//! （`greedy/mod.rs`）。共享夹具见 `greedy/test_support.rs`。

use super::mesh_section;
use super::test_support::{ENTRY_BYTES, REGISTRY_OFFSET, dark_light, parse, set_block};
use crate::quad::{Face, Quad};

// ---- 斜水面几何 ----------------------------------------------------
//
// 水的 registry 夹具：id 0 = 空气、id 1 = 石头（同时是 barrier）、
// id 10..=17 = 流体等级 0..7，fluid_height 取 14 - level（h_raw）。
// 可见性位图按 assets.FaceVisible 的规则烘焙：石头对空气与水出面，
// 水只对空气出面（水—水、水—石头都不出面）。

const WATER_COUNT: usize = 8;
const WATER_ENTRIES: usize = 2 + WATER_COUNT;
const WATER_BASE_ID: u16 = 10;

/// water_id 返回等级 level 对应的方块编号。
fn water_id(level: u8) -> u16 {
    WATER_BASE_ID + u16::from(level)
}

/// water_raw 是等级 level 孤立时的 4-bit 高度原值：源 14、最弱 7。
fn water_raw(level: u8) -> u8 {
    14 - level
}

fn water_input() -> Vec<u8> {
    let mut bytes = vec![0; REGISTRY_OFFSET + WATER_ENTRIES * ENTRY_BYTES + WATER_ENTRIES * 8];
    bytes[0..4].copy_from_slice(b"MGM1");
    bytes[8..10].copy_from_slice(&(WATER_ENTRIES as u16).to_le_bytes());
    bytes[10..12].copy_from_slice(&1_u16.to_le_bytes());
    bytes[12..14].copy_from_slice(&0_u16.to_le_bytes());
    bytes[14..16].copy_from_slice(&1_u16.to_le_bytes());

    let mut write_entry = |index: usize, id: u16, opaque: bool, fluid_height: u8| {
        let entry = REGISTRY_OFFSET + index * ENTRY_BYTES;
        bytes[entry..entry + 2].copy_from_slice(&id.to_le_bytes());
        bytes[entry + 2] = u8::from(opaque);
        for face in 0..6 {
            let material = if id == 1 { 1_u16 } else { 100 };
            bytes[entry + 4 + face * 2..entry + 6 + face * 2]
                .copy_from_slice(&material.to_le_bytes());
        }
        bytes[entry + 16] = fluid_height;
        bytes[entry + 17] = u8::from(fluid_height != 0);
    };
    write_entry(0, 0, false, 0);
    write_entry(1, 1, true, 0);
    for level in 0..WATER_COUNT {
        write_entry(
            2 + level,
            water_id(level as u8),
            false,
            water_raw(level as u8),
        );
    }

    // 可见性行：石头 = 对空气(列 0)与全部水(列 2..=9)出面；水 = 只对空气出面。
    let stone_row: u64 = 1 | (((1_u64 << WATER_COUNT) - 1) << 2);
    for index in 0..WATER_ENTRIES {
        let row: u64 = match index {
            0 => 0,
            1 => stone_row,
            _ => 1,
        };
        let offset = REGISTRY_OFFSET + WATER_ENTRIES * ENTRY_BYTES + index * 8;
        bytes[offset..offset + 8].copy_from_slice(&row.to_le_bytes());
    }
    bytes
}

/// mesh_water 网格化一份水夹具并解包成 Quad。
fn mesh_water(bytes: Vec<u8>) -> Vec<Quad> {
    let input = parse(bytes);
    let light = dark_light();
    let mut output = vec![0; 6 * 4096];
    let count = mesh_section(&input, &light, &mut output).unwrap();
    output[..count].iter().copied().map(Quad::unpack).collect()
}

/// top_face 取指定格的顶面 quad。
fn top_face(quads: &[Quad], x: u8, y: u8, z: u8) -> Quad {
    *quads
        .iter()
        .find(|quad| quad.face == Face::PosY && quad.x == x && quad.y == y && quad.z == z)
        .unwrap_or_else(|| panic!("缺少 ({x},{y},{z}) 的顶面"))
}

/// neg_x_face 取指定格朝 -X 的侧面 quad。
fn neg_x_face(quads: &[Quad], x: u8, y: u8, z: u8) -> Quad {
    *quads
        .iter()
        .find(|quad| quad.face == Face::NegX && quad.x == x && quad.y == y && quad.z == z)
        .unwrap_or_else(|| panic!("缺少 ({x},{y},{z}) 的 -X 侧面"))
}

/// 单格高度：对 8 个流体编号穷举断言孤立水格的四角高度恒等于 h_raw = 14 - level。
///
/// 孤立格四周无水，角高度的「四列平均」退化为该格自身的 h_raw，于是这条断言
/// 同时钉住了 h_raw 的映射与平均的退化情形。
#[test]
fn isolated_cell_height_is_fourteen_minus_level() {
    for level in 0..8_u8 {
        let mut bytes = water_input();
        set_block(&mut bytes, 8, 8, 8, water_id(level));
        let quads = mesh_water(bytes);
        let top = top_face(&quads, 8, 8, 8);
        assert_eq!(
            top.corners,
            [water_raw(level); 4],
            "level={level} 的孤立水格四角应全为 h_raw"
        );
        assert_eq!((top.w, top.h), (1, 1), "水面必须按 1×1 出面");
    }
    // 最弱等级仍有半格（7 即 8/16），不会退化成零面积的水面。
    assert_eq!(water_raw(7), 7);
}

/// 水柱内部没有斜面：上方也是流体的格，其侧面上沿四角取满格 15 且彼此相等。
///
/// 该格的顶面被「水—水不出面」规则挡掉，可观察的是侧面上沿——上沿与上格底面
/// 齐平即水柱内部无缝。
#[test]
fn water_column_interior_is_full_height() {
    let mut bytes = water_input();
    // 故意用最弱等级：若「上方是流体则取满格」的规则丢失，高度会掉到 7。
    set_block(&mut bytes, 8, 8, 8, water_id(7));
    set_block(&mut bytes, 8, 9, 8, water_id(7));
    let quads = mesh_water(bytes);

    let side = neg_x_face(&quads, 8, 8, 8);
    assert_eq!(side.corners, [0, 15, 15, 0], "水柱内部侧面上沿应满格");
    assert!(
        !quads
            .iter()
            .any(|quad| quad.face == Face::PosY && quad.y == 8),
        "水柱内部不应出顶面"
    );

    // 顶格上方是空气，回到 h_raw = 7，与内部的 15 形成可分辨的对照。
    assert_eq!(top_face(&quads, 8, 9, 8).corners, [7; 4]);
}

/// 相邻不同水位之间连续过渡：共享边两侧顶面高度相等，且落在
/// `较低的孤立高度 <= shared < 较高的孤立高度`。
///
/// 夹具让整个区段只有这两格是水，满足 spec 里「共享边两端点周围只有这两格是
/// 流体」的 WHEN；并对**全部 56 组有序等级对**遍历，不依赖任何单一取值。
///
/// 上界为什么必须严格：`floor((a+b)/2) < max(a,b)`（`a != b`）对全部等级对成立，
/// 而 design D2 明文否决的 max 规则会让 `shared` 等于 `strong`——**上界严格是
/// max 唯一过不去的那道门**。下界只能取非严格：`shared = weak + floor(d/2)`，
/// 等级只差 1 时 `shared` 恰好等于较弱格的孤立高度（真实水体里最常见的构型），
/// 写成严格大于就是一条假的普遍规律。
#[test]
fn adjacent_levels_share_one_continuous_edge_height() {
    let mut checked = 0;
    let mut strictly_above_weak = 0;
    for a in 0..8_u8 {
        for b in 0..8_u8 {
            if a == b {
                continue;
            }
            let mut bytes = water_input();
            set_block(&mut bytes, 8, 8, 8, water_id(a));
            set_block(&mut bytes, 9, 8, 8, water_id(b));
            let quads = mesh_water(bytes);

            let left = top_face(&quads, 8, 8, 8);
            let right = top_face(&quads, 9, 8, 8);
            // 顶面 axis=1 时 u=z、v=x，四个角 (du,dv) 依次映射到 (dz,dx) =
            // (0,0) (1,0) (1,1) (0,1)。dx=1 的 index 2、3 落在左格的 x+1 边界，
            // 也就是右格 dx=0 的 index 1、0——它们是同一条共享边的同两个端点。
            assert_eq!(
                left.corners[3], right.corners[0],
                "共享边端点必须同高 (a={a}, b={b})"
            );
            assert_eq!(
                left.corners[2], right.corners[1],
                "共享边端点必须同高 (a={a}, b={b})"
            );

            let shared = left.corners[3];
            let (weak, strong) = (
                water_raw(a).min(water_raw(b)),
                water_raw(a).max(water_raw(b)),
            );
            assert!(
                shared >= weak,
                "共享边高度 {shared} 低于较低的孤立高度 {weak} (a={a}, b={b})"
            );
            assert!(
                shared < strong,
                "共享边高度 {shared} 未严格低于较高的孤立高度 {strong} (a={a}, b={b})"
            );
            // 远离共享边的一侧仍保留各自的孤立高度：这确实是一段斜面，
            // 而不是把两格整体抬平到同一高度。
            assert_eq!(left.corners[0], water_raw(a), "(a={a}, b={b})");
            assert_eq!(right.corners[2], water_raw(b), "(a={a}, b={b})");

            checked += 1;
            if shared > weak {
                strictly_above_weak += 1;
            }
        }
    }
    assert_eq!(checked, 56, "必须遍历全部有序等级对");
    // 防空转守卫排在真实故障断言之后：若每一对都恰好 shared == weak，下界的
    // `>=` 就退化成恒等、失去分辨力。等级差 >= 2 的 42 组必须严格高于较低值，
    // 等级差 == 1 的 14 组必须恰好相等——这两个数字把整条分布钉死。
    assert_eq!(
        strictly_above_weak, 42,
        "严格高于较低孤立高度的等级对数不对，斜面分布已退化"
    );
}

/// 等级越弱高度越低：等级更大的格顶面高度 MUST NOT 高于等级更小的，源最高。
///
/// 8 个等级各放在互不接触的格子里（间隔 2），四邻情况因此完全相同。
#[test]
fn weaker_levels_are_never_higher() {
    let mut bytes = water_input();
    for level in 0..8_u8 {
        set_block(&mut bytes, i32::from(level) * 2, 8, 8, water_id(level));
    }
    let quads = mesh_water(bytes);

    let mut previous = None;
    let mut source_height = 0;
    for level in 0..8_u8 {
        let top = top_face(&quads, level * 2, 8, 8);
        let height = top.corners[0];
        assert!(
            top.corners.iter().all(|&corner| corner == height),
            "孤立格四角应相等"
        );
        if let Some(previous) = previous {
            assert!(height <= previous, "level={level} 的高度反而升高了");
        }
        if level == 0 {
            source_height = height;
        }
        assert!(height <= source_height, "源方块必须是最高的");
        previous = Some(height);
    }
    assert!(
        source_height > top_face(&quads, 14, 8, 8).corners[0],
        "源与最弱等级必须真的不同高，否则单调性断言是恒真的"
    );
}

/// 高度派生是确定的：同一份方块数据重复生成，每个角逐次相同。
#[test]
fn corner_heights_are_deterministic_across_runs() {
    let mut bytes = water_input();
    // 造一片高低错落的水域，让绝大多数角都落在「多列平均」这条路径上。
    for x in 0..12 {
        for z in 0..12 {
            let level = ((x * 5 + z * 3) % 8) as u8;
            set_block(&mut bytes, x, 8, z, water_id(level));
        }
    }
    set_block(&mut bytes, 4, 9, 4, water_id(0));

    let first = mesh_water(bytes.clone());
    let second = mesh_water(bytes);
    assert_eq!(first.len(), second.len());
    assert!(
        first
            .iter()
            .zip(&second)
            .all(|(a, b)| a.corners == b.corners),
        "重复生成的角高度必须逐次相同"
    );
    // 防空转：这片水域必须真的出现过多种角高度，否则「相同」是恒真的。
    let mut seen: Vec<u8> = first
        .iter()
        .filter(|quad| quad.face == Face::PosY)
        .map(|quad| quad.corners[0])
        .collect();
    seen.sort_unstable();
    seen.dedup();
    assert!(
        seen.len() >= 3,
        "夹具退化：只出现了 {} 种角高度",
        seen.len()
    );
}

/// 水面不贪心合并：一整层同等级的水必须出 256 条 1×1 顶面，而非合并成 1 条。
#[test]
fn flat_water_surface_emits_unit_quads() {
    let mut bytes = water_input();
    for x in 0..16 {
        for z in 0..16 {
            set_block(&mut bytes, x, 8, z, water_id(0));
        }
    }
    let quads = mesh_water(bytes);
    let tops: Vec<Quad> = quads
        .iter()
        .copied()
        .filter(|quad| quad.face == Face::PosY && quad.y == 8)
        .collect();
    assert_eq!(tops.len(), 256);
    assert!(tops.iter().all(|quad| quad.w == 1 && quad.h == 1));
    assert!(tops.iter().all(|quad| quad.corners == [14; 4]));
}
